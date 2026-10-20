package zone

import (
	"fmt"
	"slices"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/quest"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Quests in play (proposal 0002, review decision 45, decision 0043), as
// rAthena's quest.cpp:
//
//   - Each character has a quest log, saved with it. Scripts change it
//     with the calls of wa.Player, and the client gets the active and
//     inactive quests: QuestList at login and after a map change, then
//     QuestUpdate and QuestRemove.
//   - A monster's death counts for the quests of the player that killed
//     it, as mob_dead calls quest_update_objective. When that player has
//     a party, every member on the map within AREA_SIZE (14 cells) of
//     the monster counts it instead, dead or not, as
//     quest_update_objective_sub. A quest's item drops go to each
//     counting player's inventory.
//   - The client's quest window marks a quest active or inactive
//     (QuestActive).
//   - npc:show_quest_icon shows an icon over an NPC to one player, as
//     showevent. The zone keeps the icon and sends it again when the
//     NPC comes back into view. A map change drops the icons.

// questPartyRange is AREA_SIZE, the range of a party's shared kills.
const questPartyRange = ViewRange

// QuestItemLost is what a player whose quest drop did not fit sees, as
// clif_additem's failure.
const QuestItemLost = "You cannot carry more items."

// quests returns the quest table, or nil.
func (z *Zone) quests() *quest.Table {
	if z.Mods == nil {
		return nil
	}
	return z.Mods.Quests
}

// raceNames, sizeNames and elementNames are the words of
// clif_quest_string, by rAthena's RC_, Size_ and ELE_ values.
var (
	raceNames    = []string{"Formless", "Undead", "Brute", "Plant", "Insect", "Fish", "Demon", "Demihuman", "Angel", "Dragon"}
	sizeNames    = []string{"Small", "Medium", "Large"}
	elementNames = []string{"Neutral Element", "Water Element", "Earth Element", "Fire Element", "Wind Element", "Poison Element",
		"Holy Element", "Shadow Element", "Ghost Element", "Undead Element"}
)

// objectiveLabel is the text the quest window shows for an objective:
// the monster's name, or clif_quest_string's filters.
func (z *Zone) objectiveLabel(t *quest.Target) string {
	if t.Monster != 0 {
		if z.Mods != nil {
			if s, ok := z.Mods.Monsters[t.Monster]; ok && s.Name != "" {
				return s.Name
			}
		}
		return fmt.Sprintf("Monster %d", t.Monster)
	}
	var parts []string
	if t.MapName != "" {
		parts = append(parts, t.MapName)
	}
	for _, f := range []struct {
		v     int
		names []string
	}{{t.Race, raceNames}, {t.Size, sizeNames}, {t.Element, elementNames}} {
		if f.v >= 0 && f.v < len(f.names) {
			parts = append(parts, f.names[f.v])
		}
	}
	return strings.Join(parts, ", ")
}

// questInfo is one quest as the quest window shows it.
func (z *Zone) questInfo(q *character.Quest, r *quest.Record) *wathenav1.QuestInfo {
	b := wathenav1.QuestInfo_builder{Id: q.ID, Title: r.Title, Active: q.State == character.QuestActive, LimitMs: -1}
	if !q.End.IsZero() {
		b.LimitMs = max(q.End.Sub(z.wallClock()).Milliseconds(), 0)
	}
	for j := range r.Targets {
		t := &r.Targets[j]
		b.Objectives = append(b.Objectives, wathenav1.QuestObjective_builder{
			Label: z.objectiveLabel(t), Monster: t.Monster,
			Count: uint32(q.Counts[j]), Goal: uint32(t.Count), // #nosec G115 -- counts are 0 to 65535.
			MinLevel: uint32(t.MinLevel), MaxLevel: uint32(t.MaxLevel), // #nosec G115 -- levels are 0 to 1000.
		}.Build())
	}
	return b.Build()
}

// sendQuestList sends the player's active and inactive quests whose
// records exist, in log order.
func (z *Zone) sendQuestList(p *Player) {
	t := z.quests()
	b := wathenav1.QuestList_builder{}
	for i := range p.quests {
		q := &p.quests[i]
		if q.State == character.QuestComplete {
			continue
		}
		if r := t.ByID(q.ID); r != nil {
			b.Quests = append(b.Quests, z.questInfo(q, r))
		}
	}
	if len(b.Quests) == 0 && t == nil {
		return
	}
	p.pending = append(p.pending, wathenav1.Event_builder{QuestList: b.Build()}.Build())
}

// sendQuest sends one quest of the log again.
func (z *Zone) sendQuest(p *Player, id uint32) {
	i := quest.Find(p.quests, id)
	r := z.quests().ByID(id)
	if i < 0 || r == nil || p.quests[i].State == character.QuestComplete {
		return
	}
	p.pending = append(p.pending, wathenav1.Event_builder{QuestUpdate: wathenav1.QuestUpdate_builder{Quest: z.questInfo(&p.quests[i], r)}.Build()}.Build())
}

// sendQuestRemove tells the client a quest left the window.
func sendQuestRemove(p *Player, id uint32) {
	p.pending = append(p.pending, wathenav1.Event_builder{QuestRemove: wathenav1.QuestRemove_builder{Id: id}.Build()}.Build())
}

// questEvent queues a quest event for the scripts.
func (z *Zone) questEvent(e modscript.QuestEvent) {
	if z.scripts == nil {
		return
	}
	z.scriptEvents = append(z.scriptEvents, scriptEvent{name: e.Kind, quest: &e})
}

// AddQuest is setquest.
func (h scriptHost) AddQuest(id int64, r *quest.Record) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	// setquest, completequest, erasequest and changequest show the quest
	// icons again, whether or not the log changed (script.cpp 21073).
	p.questInfoChanged()
	z := h.z
	if !quest.Add(&p.quests, r, z.wallClock()) {
		return false, nil
	}
	z.sendQuest(p, r.ID)
	z.questEvent(modscript.QuestEvent{Kind: "quest_add", Player: int64(p.charID), Quest: r})
	return true, nil
}

// CompleteQuest is completequest.
func (h scriptHost) CompleteQuest(id int64, r *quest.Record) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	// setquest, completequest, erasequest and changequest show the quest
	// icons again, whether or not the log changed (script.cpp 21073).
	p.questInfoChanged()
	if !quest.SetState(p.quests, r.ID, character.QuestComplete) {
		return false, nil
	}
	sendQuestRemove(p, r.ID)
	h.z.questEvent(modscript.QuestEvent{Kind: "quest_complete", Player: int64(p.charID), Quest: r})
	return true, nil
}

// RemoveQuest is erasequest.
func (h scriptHost) RemoveQuest(id int64, r *quest.Record) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	// setquest, completequest, erasequest and changequest show the quest
	// icons again, whether or not the log changed (script.cpp 21073).
	p.questInfoChanged()
	i := quest.Find(p.quests, r.ID)
	if i < 0 {
		return false, nil
	}
	shown := p.quests[i].State != character.QuestComplete
	quest.Remove(&p.quests, r.ID)
	if shown {
		sendQuestRemove(p, r.ID)
	}
	return true, nil
}

// ChangeQuest is changequest.
func (h scriptHost) ChangeQuest(id int64, from, to *quest.Record) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	// setquest, completequest, erasequest and changequest show the quest
	// icons again, whether or not the log changed (script.cpp 21073).
	p.questInfoChanged()
	z := h.z
	if !quest.Change(&p.quests, from.ID, to, z.wallClock()) {
		return false, nil
	}
	sendQuestRemove(p, from.ID)
	z.sendQuest(p, to.ID)
	z.questEvent(modscript.QuestEvent{Kind: "quest_add", Player: int64(p.charID), Quest: to})
	return true, nil
}

// QuestState is the state of a quest in a player's log.
func (h scriptHost) QuestState(id int64, r *quest.Record) (string, error) {
	p, err := h.player(id)
	if err != nil {
		return "", err
	}
	i := quest.Find(p.quests, r.ID)
	if i < 0 {
		return quest.StateNone, nil
	}
	return quest.StateName(p.quests[i].State), nil
}

// QuestProgress is an objective's kills.
func (h scriptHost) QuestProgress(id int64, r *quest.Record, objective int) (int, bool, error) {
	p, err := h.player(id)
	if err != nil {
		return 0, false, err
	}
	i := quest.Find(p.quests, r.ID)
	if i < 0 || objective < 0 || objective >= character.MaxQuestObjectives {
		return 0, false, nil
	}
	return p.quests[i].Counts[objective], true, nil
}

// QuestTimedOut reports whether a quest's time limit passed.
func (h scriptHost) QuestTimedOut(id int64, r *quest.Record) (bool, error) {
	p, err := h.player(id)
	if err != nil {
		return false, err
	}
	i := quest.Find(p.quests, r.ID)
	if i < 0 {
		return false, nil
	}
	return quest.TimedOut(&p.quests[i], h.z.wallClock()), nil
}

// ShowQuestIcon is showevent: the icon goes to the player now when it
// sees the NPC, and when the NPC comes into view.
func (h scriptHost) ShowQuestIcon(npcID, id int64, icon, mark string) error {
	e, err := h.npc(npcID)
	if err != nil {
		return err
	}
	p, err := h.player(id)
	if err != nil {
		return err
	}
	h.z.showQuestIcon(p, e, icon, mark)
	return nil
}

// QuestActive asks to mark a quest of the log active or inactive.
func (s *Session) QuestActive(seq, id uint32, active bool) bool {
	return s.send(command{seq: seq, questActive: &questActiveCommand{id: id, active: active}})
}

type questActiveCommand struct {
	id     uint32
	active bool
}

// questActive is clif_parse_questStateAck: quest_update_status with
// Q_ACTIVE or Q_INACTIVE.
func (z *Zone) questActive(p *Player, c *questActiveCommand) {
	state := character.QuestInactive
	if c.active {
		state = character.QuestActive
	}
	if z.quests().ByID(c.id) == nil || !quest.SetState(p.quests, c.id, state) {
		return
	}
	z.sendQuest(p, c.id)
}

// questKill counts a monster's death for the quests of its killer, or
// of the killer's party members near the monster.
func (z *Zone) questKill(e, killer *entity) {
	t := z.quests()
	if t == nil || killer == nil || killer.player == nil || e.monster == nil {
		return
	}
	k := quest.Kill{Monster: e.monster.spawn.Mob, Level: e.monster.level, Race: -1, Size: -1, Element: -1, Map: strings.TrimPrefix(z.Map, "map/")}
	if st := e.status; st != nil {
		k.Race, k.Size, k.Element = st.Race, st.Size, st.DefEle
	}
	who := []*Player{killer.player}
	if party := killer.player.partyID(); party != 0 {
		who = who[:0]
		for _, p := range z.playersByID() {
			if p.entity == nil || p.partyID() != party || abs(p.entity.x-e.x) > questPartyRange || abs(p.entity.y-e.y) > questPartyRange {
				continue
			}
			who = append(who, p)
		}
	}
	for _, p := range who {
		z.countQuests(p, t, k, e)
		// quest_update_objective ends with pc_show_questinfo (quest.cpp
		// 837).
		p.questInfoChanged()
	}
}

// playersByID lists the zone's players in the order they joined, so
// party members count a kill in a fixed order.
func (z *Zone) playersByID() []*Player {
	ids := make([]uint32, 0, len(z.players))
	for id := range z.players {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	out := make([]*Player, 0, len(ids))
	for _, id := range ids {
		if p := z.players[id]; !p.gone() {
			out = append(out, p)
		}
	}
	return out
}

// countQuests is quest_update_objective for one player.
func (z *Zone) countQuests(p *Player, t *quest.Table, k quest.Kill, monster *entity) {
	progress, gifts := quest.Count(p.quests, t, k, z.rand.IntN)
	for _, pr := range progress {
		q := &p.quests[pr.Index]
		r := t.ByID(q.ID)
		z.sendQuest(p, q.ID)
		for _, j := range pr.Objectives {
			z.questEvent(modscript.QuestEvent{Kind: "quest_progress", Player: int64(p.charID), Quest: r,
				Objective: j + 1, Count: q.Counts[j], Goal: r.Targets[j].Count})
		}
	}
	for _, g := range gifts {
		if z.Items == nil {
			continue
		}
		if res := z.addItem(p, g.Item, g.Count, origin{action: transfer.ActionQuest, from: monsterOwner(monster)}); res != item.AddSuccess {
			z.answer(p, QuestItemLost)
		}
	}
}
