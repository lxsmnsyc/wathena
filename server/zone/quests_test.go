package zone

import (
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/quest"
	"github.com/lxsmnsyc/wathena/server/social"
)

// Quest test records: 1100 hunts 3 Porings within an hour and drops a
// Jellopy from every Poring, 1101 counts plants of levels 1 to 10 on
// map/a, and 7 has no objective.
func questTable() *quest.Table {
	return quest.NewTable([]*quest.Record{
		{ID: 1100, Name: "1100", Mod: "core", Title: "Porings", Limit: quest.Limit{Seconds: 3600},
			Targets: []quest.Target{{Monster: 1002, Count: 3, Race: -1, Size: -1, Element: -1}},
			Drops:   []quest.Drop{{Monster: 1002, Item: 909, Count: 2, Rate: 10000}}},
		{ID: 1101, Name: "1101", Mod: "core", Title: "Plants",
			Targets: []quest.Target{{Count: 2, MaxLevel: 10, MinLevel: 1, Race: 3, Size: -1, Element: -1, Map: "a", MapName: "Field A"}}},
		{ID: 7, Name: "7", Mod: "core", Title: "Talk"},
	})
}

// questWorld is a social world of one zone, map/a, with the quest table
// and Jellopy.
func questWorld(t *testing.T) (*socialWorld, *Zone) {
	t.Helper()
	w := newSocialWorld(nil, "map/a")
	z := w.zones[0]
	z.Mods = &Mods{Quests: questTable(), Monsters: map[uint32]Spawn{1002: {Mob: 1002, Name: "Poring"}}}
	z.Items = item.New(&item.Data{Mode: "renewal", Items: []item.Def{{ID: 909, AegisName: "Jellopy", Weight: 10, Stackable: true, Type: "Etc"}}})
	return w, z
}

// questMonster spawns a monster that does not come back.
func questMonster(z *Zone, mob uint32, level, race, x, y int) *entity {
	st := &battle.Status{MaxHP: 30, Level: level, Race: race, Size: 1, DefEle: 1}
	return z.spawnOne(&Spawn{Mob: mob, Name: "M", Amount: 1, X: x, Y: y, Speed: 400, Status: st, noRespawn: true})
}

func questUpdates(p *Player) []*wathenav1.QuestInfo {
	var out []*wathenav1.QuestInfo
	for _, e := range events(p) {
		if e.HasQuestUpdate() {
			out = append(out, e.GetQuestUpdate().GetQuest())
		}
	}
	return out
}

// TestQuestLog checks the host calls on a player's log, the events the
// client gets, the active toggle, and the snapshot.
func TestQuestLog(t *testing.T) {
	w, z := questWorld(t)
	p := w.join(z, "Ann")
	w.step(50)
	var list *wathenav1.QuestList
	for _, e := range events(p) {
		if e.HasQuestList() {
			list = e.GetQuestList()
		}
	}
	if list == nil || len(list.GetQuests()) != 0 {
		t.Fatalf("login quest list %v", list)
	}
	h := scriptHost{z}
	tab := questTable()
	id := int64(p.charID)
	if ok, _ := h.AddQuest(id, tab.ByID(1100)); !ok {
		t.Fatal("add")
	}
	if ok, _ := h.AddQuest(id, tab.ByID(1100)); ok {
		t.Fatal("added twice")
	}
	w.step(100)
	got := questUpdates(p)
	if len(got) != 1 || got[0].GetId() != 1100 || got[0].GetTitle() != "Porings" || !got[0].GetActive() ||
		got[0].GetLimitMs() != 3600_000 || len(got[0].GetObjectives()) != 1 || got[0].GetObjectives()[0].GetLabel() != "Poring" ||
		got[0].GetObjectives()[0].GetGoal() != 3 {
		t.Fatalf("update %v", got)
	}
	// The client marks it inactive, then active again.
	z.handle(command{player: p, seq: 1, questActive: &questActiveCommand{id: 1100}})
	w.step(150)
	if got := questUpdates(p); len(got) != 1 || got[0].GetActive() {
		t.Errorf("inactive %v", got)
	}
	if s, _ := h.QuestState(id, tab.ByID(1100)); s != "inactive" {
		t.Errorf("state %s", s)
	}
	z.handle(command{player: p, seq: 2, questActive: &questActiveCommand{id: 1100, active: true}})
	z.handle(command{player: p, seq: 3, questActive: &questActiveCommand{id: 999, active: true}})
	w.step(200)
	if got := questUpdates(p); len(got) != 1 || !got[0].GetActive() {
		t.Errorf("active %v", got)
	}
	// Time passes on the server clock.
	w.now = w.now.Add(2 * time.Hour)
	if v, _ := h.QuestTimedOut(id, tab.ByID(1100)); !v {
		t.Error("not timed out after 2 hours")
	}
	// Complete, change and remove.
	if ok, _ := h.ChangeQuest(id, tab.ByID(1100), tab.ByID(7)); !ok {
		t.Fatal("change")
	}
	if ok, _ := h.CompleteQuest(id, tab.ByID(7)); !ok {
		t.Fatal("complete")
	}
	w.step(250)
	var removed []uint32
	for _, e := range events(p) {
		if e.HasQuestRemove() {
			removed = append(removed, e.GetQuestRemove().GetId())
		}
	}
	if len(removed) != 2 || removed[0] != 1100 || removed[1] != 7 {
		t.Errorf("removed %v", removed)
	}
	c := z.snapshot(p)
	if len(c.Quests) != 1 || c.Quests[0].ID != 7 || c.Quests[0].State != character.QuestComplete {
		t.Errorf("snapshot %+v", c.Quests)
	}
	if ok, _ := h.RemoveQuest(id, tab.ByID(7)); !ok || len(p.quests) != 0 {
		t.Error("remove")
	}
	w.step(300)
	for _, e := range events(p) {
		if e.HasQuestRemove() {
			t.Errorf("a complete quest left the window again: %v", e)
		}
	}
}

// TestQuestKills checks the count of a kill, its cap, the drops, and the
// filters, as quest_update_objective.
func TestQuestKills(t *testing.T) {
	w, z := questWorld(t)
	p := w.join(z, "Ann")
	p.maxWeight = 10000
	h := scriptHost{z}
	tab := questTable()
	_, _ = h.AddQuest(int64(p.charID), tab.ByID(1100))
	_, _ = h.AddQuest(int64(p.charID), tab.ByID(1101))
	w.step(50)
	events(p)
	x, y := p.entity.x, p.entity.y
	for i := range 4 {
		m := questMonster(z, 1002, 5, 3, x+1, y)
		z.die(m, p.entity)
		w.step(int64(100 + 50*i))
	}
	q := p.quests[0]
	if q.Counts[0] != 3 {
		t.Errorf("Poring kills %v, want 3, the goal", q.Counts)
	}
	// Porings are plants of level 5 on map/a: they count for 1101 too.
	if p.quests[1].Counts[0] != 2 {
		t.Errorf("plant kills %v", p.quests[1].Counts)
	}
	if n := p.inventory.Count(909); n != 8 {
		t.Errorf("Jellopy %d, want 2 from each of 4 Porings", n)
	}
	// A plant above level 10 does not count, nor does a kill by nobody.
	_, _ = h.RemoveQuest(int64(p.charID), tab.ByID(1101))
	_, _ = h.AddQuest(int64(p.charID), tab.ByID(1101))
	z.die(questMonster(z, 1118, 11, 3, x+1, y), p.entity)
	z.die(questMonster(z, 1118, 9, 3, x+1, y), nil)
	z.die(questMonster(z, 1118, 9, 2, x+1, y), p.entity)
	if c := p.quests[1].Counts; c[0] != 0 {
		t.Errorf("filters let kills through: %v", c)
	}
	z.die(questMonster(z, 1118, 9, 3, x+1, y), p.entity)
	if c := p.quests[1].Counts; c[0] != 1 {
		t.Errorf("a level 9 plant: %v", c)
	}
}

// TestQuestPartyKills checks quest_update_objective_sub: party members
// within 14 cells of the monster count the kill, dead ones too, and
// others do not.
func TestQuestPartyKills(t *testing.T) {
	w, z := questWorld(t)
	ann, bob, cid, dan := w.join(z, "Ann"), w.join(z, "Bob"), w.join(z, "Cid"), w.join(z, "Dan")
	formParty(t, z, social.Options{}, ann, bob, cid)
	h := scriptHost{z}
	tab := questTable()
	for _, p := range []*Player{ann, bob, cid, dan} {
		_, _ = h.AddQuest(int64(p.charID), tab.ByID(1100))
	}
	w.step(50)
	m := questMonster(z, 1002, 5, 3, 30, 30)
	ann.entity.x, ann.entity.y = 30, 31
	bob.entity.x, bob.entity.y = 44, 16
	bob.entity.dead = true
	cid.entity.x, cid.entity.y = 45, 30
	dan.entity.x, dan.entity.y = 31, 30
	z.die(m, ann.entity)
	for _, c := range []struct {
		p    *Player
		want int
	}{{ann, 1}, {bob, 1}, {cid, 0}, {dan, 0}} {
		if got := c.p.quests[0].Counts[0]; got != c.want {
			t.Errorf("%s counted %d, want %d", c.p.Name, got, c.want)
		}
	}
	// Without a party, only the killer counts.
	z.die(questMonster(z, 1002, 5, 3, 30, 30), dan.entity)
	if dan.quests[0].Counts[0] != 1 || ann.quests[0].Counts[0] != 1 {
		t.Errorf("Dan's kill: Dan %v, Ann %v", dan.quests[0].Counts, ann.quests[0].Counts)
	}
	// The killer itself counts only within range.
	ann.entity.x, ann.entity.y = 10, 10
	z.die(questMonster(z, 1002, 5, 3, 30, 30), ann.entity)
	if ann.quests[0].Counts[0] != 1 || bob.quests[0].Counts[0] != 2 {
		t.Errorf("a kill from afar: Ann %v, Bob %v", ann.quests[0].Counts, bob.quests[0].Counts)
	}
}

// TestQuestIcons checks that an icon goes to the player, comes again
// when the NPC comes back into view, and goes away.
func TestQuestIcons(t *testing.T) {
	w, z := questWorld(t)
	z.SetNPCs([]NPC{{Name: "core:Guide", DisplayName: "Guide", X: 20, Y: 20, Sprite: 50}})
	p := w.join(z, "Ann")
	p.entity.x, p.entity.y = 21, 20
	w.step(50)
	events(p)
	var npc *entity
	for e := range z.entities.all() {
		if e.npc != nil {
			npc = e
		}
	}
	h := scriptHost{z}
	icons := func() []string {
		var out []string
		for _, e := range events(p) {
			if e.HasQuestIcon() {
				out = append(out, e.GetQuestIcon().GetIcon())
			}
		}
		return out
	}
	if err := h.ShowQuestIcon(npc.npc.handle, int64(p.charID), "quest"); err != nil {
		t.Fatal(err)
	}
	w.step(100)
	if got := icons(); len(got) != 1 || got[0] != "quest" {
		t.Fatalf("icons %v", got)
	}
	// Out of view and back.
	p.entity.x = 50
	w.step(150)
	events(p)
	p.entity.x = 22
	w.step(200)
	if got := icons(); len(got) != 1 || got[0] != "quest" {
		t.Errorf("icons on coming back %v", got)
	}
	if err := h.ShowQuestIcon(npc.npc.handle, int64(p.charID), ""); err != nil {
		t.Fatal(err)
	}
	w.step(250)
	if got := icons(); len(got) != 1 || got[0] != "" {
		t.Errorf("removed icon %v", got)
	}
}
