package zone

import (
	"errors"
	"fmt"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// The presentation and quest icons of review decision 58, areas B5 and
// B6 of proposal 0002b:
//   - Who gets what follows rAthena's send targets: AREA is every player
//     who sees the entity, ALL_SAMEMAP every player of the zone,
//     ALL_CLIENT every player of every zone, and SELF one player.
//     AREA_CHAT_WOC, which unittalk uses, is local chat (chat.go).
//   - A change of an NPC of another zone runs in its zone after the
//     current step, as the view changes of decision 0062 do.
//   - A change of an NPC's view, name or title makes it appear again to
//     the players who see it, as unit_refresh and clif_name_area.
//   - NPCs walk at DEFAULT_NPC_WALK_SPEED until npcspeed changes it, and
//     the players they walk into touch them (unit_walktoxy_ontouch).
//   - Quest icons follow pc_show_questinfo: each player sees, for each
//     NPC with icons, the first whose condition holds. The check runs at
//     the end of the step in which something changed for the player.

// defaultNPCSpeed is DEFAULT_NPC_WALK_SPEED, in milliseconds per cell.
const defaultNPCSpeed = 200

// npcLook is the look of an NPC whose view is a job, as rAthena's
// npc_data view_data: the UNPC_ looks of setunitdata.
type npcLook struct {
	hair, hairColor, clothColor, headTop, headMid, headBottom uint32
	male                                                      bool
}

// questIconShown is a quest icon a player sees over an NPC: the icon's
// name and its minimap mark.
type questIconShown struct {
	icon, mark string
}

func questIconEvent(net uint32, icon, mark string) *wathenav1.Event {
	return wathenav1.Event_builder{QuestIcon: wathenav1.QuestIcon_builder{Npc: net, Icon: icon, Mark: mark}.Build()}.Build()
}

// npcLookMessage is an NPC's look, with the kind of its view: rAthena's
// status_set_viewdata takes a job (pcdb_checkid), then a monster
// (mobdb_checkid), then an NPC sprite.
func (z *Zone) npcLookMessage(n *npc) *wathenav1.Look {
	l := wathenav1.Look_builder{Body: n.Sprite}
	switch {
	case mod.IsJobView(n.Sprite) || z.Mods != nil && z.Mods.OwnLookJobs[int(n.Sprite)]:
		l.NpcView = wathenav1.NpcView_NPC_VIEW_JOB
		l.Hair, l.HairColor, l.ClothColor = n.look.hair, n.look.hairColor, n.look.clothColor
		l.HeadTop, l.HeadMid, l.HeadBottom = n.look.headTop, n.look.headMid, n.look.headBottom
		if n.look.male {
			l.Sex = 1
		}
	case z.isMonsterView(n.Sprite):
		l.NpcView = wathenav1.NpcView_NPC_VIEW_MONSTER
	}
	return l.Build()
}

// isMonsterView is mobdb_checkid: a monster of the world's data.
func (z *Zone) isMonsterView(c uint32) bool {
	if z.Mods == nil {
		return false
	}
	_, ok := z.Mods.Monsters[c]
	return ok
}

// onNPC runs f on an NPC: at once for an NPC of this zone, or in its
// zone after the current step. change is set for calls that change the
// NPC, which a world event's hint NPC refuses.
func (h scriptHost) onNPC(id int64, change bool, f func(z *Zone, e *entity)) error {
	t, err := h.npcTarget(id)
	if err != nil {
		return err
	}
	if t.local != nil {
		if change && t.local.npc.hint != 0 {
			return errors.New("a world event's hint NPC cannot change")
		}
		f(h.z, t.local)
		return nil
	}
	h.z.postNPC(t.entry.zone, npcMessage{kind: npcPresentMessage, handle: t.entry.handle, run: f})
	return nil
}

// localNPC is an NPC of this zone that is on its map.
func (h scriptHost) localNPC(id int64) (*entity, error) {
	t, err := h.npcTarget(id)
	if err != nil {
		return nil, err
	}
	if t.local == nil {
		return nil, errors.New("the NPC is on another map")
	}
	if t.local.npc.floating {
		return nil, errors.New("the NPC has no map")
	}
	return t.local, nil
}

// ZoneLister is a Directory that lists its zones, for what goes to every
// player of the world. The world implements it.
type ZoneLister interface {
	Zones() []*Zone
}

// everyZone runs f in every zone of the world: this one at once, and the
// others after the current step. A zone without a world runs it alone.
func (z *Zone) everyZone(f func(z *Zone)) {
	f(z)
	l, ok := z.World.(ZoneLister)
	if !ok {
		return
	}
	for _, to := range l.Zones() {
		if to != z {
			z.postNPC(to, npcMessage{kind: npcZoneMessage, zoneRun: f})
		}
	}
}

// onMap runs f in the zone of a map: at once for this zone, or after the
// current step.
func (z *Zone) onMap(mapName string, f func(z *Zone)) error {
	to := z.zoneOf("map/" + mapName)
	if to == nil {
		return fmt.Errorf("the map %s does not run in this world", mapName)
	}
	if to == z {
		f(z)
		return nil
	}
	z.postNPC(to, npcMessage{kind: npcZoneMessage, zoneRun: f})
	return nil
}

func chatEvent(from uint32, name, text string, color uint32) *wathenav1.Event {
	return wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
		Channel:  wathenav1.ChatChannel_CHAT_CHANNEL_LOCAL,
		From:     from,
		FromName: name,
		Text:     text,
		Color:    color,
	}.Build()}.Build()
}

// lineTo sends a line said aloud to one player: from the speaker when
// the player sees it, or with the speaker's name alone.
func lineTo(p *Player, speaker *entity, name, text string, color uint32) {
	var from uint32
	if speaker != nil {
		from, _ = p.netIDs.get(speaker)
	}
	p.pending = append(p.pending, chatEvent(from, name, text, color))
}

// SayAloud is npctalk on an NPC and unittalk on a player.
func (h scriptHost) SayAloud(speaker modscript.Entity, text string, to modscript.SendTarget, color int64) error {
	z := h.z
	c := uint32(max(color, 0)) // #nosec G115 -- 0xRRGGBB, checked by the runtime.
	var target *Player
	if to.To == "player" {
		p, err := h.player(to.Player)
		if err != nil {
			return err
		}
		target = p
	}
	if speaker.Kind == "player" {
		p, err := h.player(speaker.ID)
		if err != nil {
			return err
		}
		e := p.entity
		switch to.To {
		case "area":
			// clif_disp_overhead_: AREA_CHAT_WOC, and back to the speaker.
			z.chats = append(z.chats, chatLine{speaker: e, name: p.Name, text: text})
		case "player":
			lineTo(target, e, p.Name, text, 0)
		default:
			sayToZones(z, e, p.Name, text, 0, to.To == "world")
		}
		return nil
	}
	if target != nil {
		// clif_messagecolor_target with SELF: the line goes to the player
		// alone, from an NPC of any map.
		t, err := h.npcTarget(speaker.ID)
		if err != nil {
			return err
		}
		if t.local != nil {
			lineTo(target, t.local, t.local.npc.DisplayName, text, c)
			return nil
		}
		display, _, _ := z.npcDir().place(t.entry)
		lineTo(target, nil, display, text, c)
		return nil
	}
	return h.onNPC(speaker.ID, false, func(z *Zone, e *entity) {
		n := e.npc
		switch {
		case to.To == "world":
			sayToZones(z, e, n.DisplayName, text, c, true)
		case n.floating:
			// An NPC without a map has no area and no map (nd->m < 0).
		case to.To == "map":
			sayToZones(z, e, n.DisplayName, text, c, false)
		default:
			// clif_messagecolor with AREA: every player who sees the NPC.
			for _, p := range z.playersInOrder() {
				if id, ok := p.netIDs.get(e); ok {
					p.pending = append(p.pending, chatEvent(id, n.DisplayName, text, c))
				}
			}
		}
	})
}

// sayToZones sends a line to every player of the zone, and with world
// to every player of every zone: ALL_SAMEMAP and ALL_CLIENT.
func sayToZones(z *Zone, speaker *entity, name, text string, color uint32, world bool) {
	src := z
	send := func(z *Zone) {
		for _, p := range z.playersInOrder() {
			if z == src {
				lineTo(p, speaker, name, text, color)
			} else {
				lineTo(p, nil, name, text, color)
			}
		}
	}
	if world {
		z.everyZone(send)
		return
	}
	send(z)
}

// PlayerEmotion is emotion on a player: everyone who sees it.
func (h scriptHost) PlayerEmotion(id, emotion int64) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	h.z.emotions = append(h.z.emotions, npcEmotion{p.entity, uint32(emotion)}) // #nosec G115 -- checked by the runtime.
	return nil
}

func effectEvent(id, effect uint32) *wathenav1.Event {
	return wathenav1.Event_builder{Effect: wathenav1.EffectPlay_builder{EffectId: effect, Entity: &id}.Build()}.Build()
}

// ShowEffectTo is specialeffect and specialeffect2 with a send target.
// The client draws an effect on an entity it sees, so the players of the
// map who do not see the entity see nothing, as rAthena's clients do.
func (h scriptHost) ShowEffectTo(ref modscript.Entity, effectID int64, to modscript.SendTarget) error {
	ef := uint32(effectID) // #nosec G115 -- checked by the runtime.
	var target *Player
	if to.To == "player" {
		p, err := h.player(to.Player)
		if err != nil {
			return err
		}
		target = p
	}
	show := func(z *Zone, e *entity) {
		switch to.To {
		case "player":
			// clif_specialeffect_single to the attached player.
			if id, ok := target.netIDs.get(e); ok && z == h.z {
				target.pending = append(target.pending, effectEvent(id, ef))
			}
		case "map":
			for _, p := range z.playersInOrder() {
				if id, ok := p.netIDs.get(e); ok {
					p.pending = append(p.pending, effectEvent(id, ef))
				}
			}
		default:
			z.effects = append(z.effects, effect{e, ef})
		}
	}
	if ref.Kind == "npc" {
		return h.onNPC(ref.ID, false, show)
	}
	e, err := h.entityOf(ref)
	if err != nil {
		return err
	}
	show(h.z, e)
	return nil
}

// RemoveEffect is removespecialeffect: everyone who sees the entity
// stops the effect.
func (h scriptHost) RemoveEffect(ref modscript.Entity, effectID int64) error {
	ef := uint32(effectID) // #nosec G115 -- checked by the runtime.
	remove := func(z *Zone, e *entity) {
		for _, p := range z.playersInOrder() {
			if id, ok := p.netIDs.get(e); ok {
				p.pending = append(p.pending, wathenav1.Event_builder{EffectRemove: wathenav1.EffectRemove_builder{Entity: id, EffectId: ef}.Build()}.Build())
			}
		}
	}
	if ref.Kind == "npc" {
		return h.onNPC(ref.ID, false, remove)
	}
	e, err := h.entityOf(ref)
	if err != nil {
		return err
	}
	remove(h.z, e)
	return nil
}

// refreshNPC shows an NPC again to the players who see it, as
// unit_refresh, and keeps its name and cell in the world's directory.
func (z *Zone) refreshNPC(e *entity) {
	e.lookChanged = true
	if d := z.npcDir(); d != nil {
		d.mirrorPlace(e)
	}
}

// SetNPCTitle is setunittitle on an NPC: clif_name_area.
func (h scriptHost) SetNPCTitle(id int64, title string) error {
	return h.onNPC(id, true, func(z *Zone, e *entity) {
		e.npc.title = title
		z.refreshNPC(e)
	})
}

// SetNPCDisplayName is npc_setdisplayname.
func (h scriptHost) SetNPCDisplayName(id int64, name string) error {
	return h.onNPC(id, true, func(z *Zone, e *entity) {
		// npc_setdisplayname replaces nd->name, # part too: players see
		// the part before #, and strnpcinfo reads the rest.
		e.npc.DisplayName, e.npc.Hidden, _ = strings.Cut(name, "#")
		z.refreshNPC(e)
	})
}

// SetNPCLook is npc_setclass and setunitdata's UNPC_ looks.
func (h scriptHost) SetNPCLook(id int64, look modscript.NPCLook) error {
	set := func(dst *uint32, v *int64) {
		if v != nil {
			*dst = uint32(*v) // #nosec G115 -- checked by the runtime.
		}
	}
	return h.onNPC(id, true, func(z *Zone, e *entity) {
		n := e.npc
		if look.Sprite != nil {
			set(&n.Sprite, look.Sprite)
			// A class shows an NPC that had no view.
			n.NoSprite = false
		}
		l := &n.look
		set(&l.hair, look.HairStyle)
		set(&l.hairColor, look.HairColor)
		set(&l.clothColor, look.ClothColor)
		set(&l.headTop, look.HeadTop)
		set(&l.headMid, look.HeadMiddle)
		set(&l.headBottom, look.HeadBottom)
		if look.Male != nil {
			l.male = *look.Male
		}
		z.refreshNPC(e)
	})
}

// SendNPCSprite is classchange (script.cpp 15163): the NPC shows with
// another view to the players who see it, or to one player, and keeps
// its own, so players who see it later see the old view.
// clif_class_change sends nothing for a job's view (clif.cpp 1477).
func (h scriptHost) SendNPCSprite(id, sprite int64, to modscript.SendTarget) error {
	var target *Player
	if to.To == "player" {
		p, err := h.player(to.Player)
		if err != nil {
			return err
		}
		target = p
	}
	view := uint32(sprite) // #nosec G115 -- checked by the runtime.
	return h.onNPC(id, false, func(z *Zone, e *entity) {
		if mod.IsJobView(view) || z.Mods != nil && z.Mods.OwnLookJobs[int(view)] {
			return
		}
		n := *e.npc
		n.Sprite, n.NoSprite = view, false
		shown := *e
		shown.npc = &n
		for _, p := range z.playersInOrder() {
			if target != nil && (p != target || z != h.z) {
				continue
			}
			if net, ok := p.netIDs.get(e); ok {
				p.pending = append(p.pending, z.npcAppear(net, &shown))
			}
		}
	})
}

// MoveNPC is npc_movenpc (npc.cpp 5046): the cell is capped to the map,
// the players near the old cell lose sight of the NPC (clif_outsight),
// and those near the new one see it come (clif_insight). dir is
// movenpc's direction, 0 north counterclockwise, which it sets before
// the move (script.cpp 16161), or -1 to keep the NPC's.
func (h scriptHost) MoveNPC(id int64, x, y, dir int) error {
	return h.onNPC(id, true, func(z *Zone, e *entity) {
		if e.npc.floating {
			return
		}
		if dir >= 0 {
			e.dir = facing(path.Dir(dir % 8))
		}
		x = min(max(x, 0), z.grid.Width-1)
		y = min(max(y, 0), z.grid.Height-1)
		e.path = e.path[:0]
		for _, p := range z.playersInOrder() {
			if id, ok := p.netIDs.get(e); ok {
				p.netIDs.del(e)
				z.queue(p, frame(z.tick, p.ackSeq, vanish(id, wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT)))
			}
		}
		e.x, e.y = x, y
		if d := z.npcDir(); d != nil {
			d.mirrorPlace(e)
		}
	})
}

// WalkNPC is npcwalkto and unitwalk on an NPC: unit_walktoxy with the
// NPC's own speed, on cells that are not CELL_CHKNOPASS. An NPC of
// another zone walks there after the current step, and true comes back.
// A label is unitwalk's: its event runs, with no player, when a walk of
// the NPC ends on the cell it walks to (unit.cpp 650), and it stays
// until one does, as ud->walk_done_event.
func (h scriptHost) WalkNPC(id int64, x, y int, label *modscript.MonsterLabel) (bool, error) {
	t, err := h.npcTarget(id)
	if err != nil {
		return false, err
	}
	done, err := h.resolveLabel(label)
	if err != nil {
		return false, err
	}
	walk := func(z *Zone, e *entity) bool {
		if done != nil {
			// buildin_unitwalk sets the label whether or not the walk
			// starts (script.cpp 20049).
			e.npc.walkDone = done
		}
		// buildin_npcwalkto: an NPC without a map, or a cell outside the
		// map, fails.
		if e.npc.floating || x >= z.grid.Width || y >= z.grid.Height {
			return false
		}
		e.npc.walkX, e.npc.walkY = x, y
		if x == e.x && y == e.y && len(e.path) == 0 {
			return true
		}
		z.walk(e, x, y)
		return len(e.path) > 0
	}
	if t.local != nil {
		if t.local.npc.hint != 0 {
			return false, errors.New("a world event's hint NPC cannot change")
		}
		return walk(h.z, t.local), nil
	}
	h.z.postNPC(t.entry.zone, npcMessage{kind: npcPresentMessage, handle: t.entry.handle, run: func(z *Zone, e *entity) { walk(z, e) }})
	return true, nil
}

// SetNPCSpeed is npcspeed: nd->speed, which status_get_speed reads for
// each step.
func (h scriptHost) SetNPCSpeed(id int64, ms int) error {
	return h.onNPC(id, true, func(_ *Zone, e *entity) { e.speed = ms })
}

// StopNPC is npcstop with its default flags USW_FIXPOS,
// USW_MOVE_FULL_CELL and USW_FORCE_STOP (script.cpp 16337,
// unit_stop_walking): an NPC past half of its step finishes the cell,
// then stands, and the players who see it get its cell. Without
// sendCell it is unitstopwalk's default USW_NONE (script.cpp 20186): the
// NPC stands on its cell at once, and the players get nothing, so their
// clients go on drawing the walk (unit.cpp 1725 and 1732).
func (h scriptHost) StopNPC(id int64, sendCell bool) error {
	return h.onNPC(id, true, func(z *Zone, e *entity) {
		if len(e.path) == 0 {
			return
		}
		if !sendCell {
			e.path = e.path[:0]
			if dir := z.npcDir(); dir != nil {
				dir.mirrorPlace(e)
			}
			return
		}
		d := e.path[0]
		step := int64(path.StepTime(d, e.speed))
		if e.stepStart+step-z.now <= step/2 {
			dx, dy := d.Step()
			e.x += dx
			e.y += dy
			e.dir = facing(d)
			z.npcStepped(e)
		}
		e.path = e.path[:0]
		e.stopped = true
		if dir := z.npcDir(); dir != nil {
			dir.mirrorPlace(e)
		}
	})
}

// npcStepped is the end of unit_walktoxy_timer for an NPC that walked
// one cell: unless it is disabled, the players in its touch area touch
// it, and those who left it untouch it (unit_walktoxy_ontouch).
func (z *Zone) npcStepped(e *entity) {
	n := e.npc
	if d := z.npcDir(); d != nil {
		d.mirrorPlace(e)
	}
	if l := n.walkDone; l != nil && e.x == n.walkX && e.y == n.walkY {
		// unit_walktoxy_timer: the walk ended on its cell, so the label
		// runs once, with the NPC's own ID and no player (unit.cpp 650).
		n.walkDone = nil
		if l.zone != nil {
			z.postNPC(l.zone, npcMessage{kind: npcEventMessage, handle: l.handle, label: l.event})
		}
	}
	if n.invisible || n.Touch == nil || n.script == nil {
		return
	}
	for _, p := range z.playersInOrder() {
		z.touchAreas(p)
	}
}

func npcBarEvent(id uint32, ms int64, color uint32) *wathenav1.Event {
	return wathenav1.Event_builder{NpcProgressBar: wathenav1.NpcProgressBar_builder{
		Npc: id, DurationMs: uint32(ms), Color: color, // #nosec G115 -- at most MaxWait.
	}.Build()}.Build()
}

// npcBarEvents are the progress bar a player who comes to see an NPC
// sees: the time left, as clif_progressbar_npc.
func (z *Zone) npcBarEvents(id uint32, e *entity) []*wathenav1.Event {
	if left := e.npc.barEnd - z.now; left > 0 {
		return []*wathenav1.Event{npcBarEvent(id, left, e.npc.barColor)}
	}
	return nil
}

// ShowNPCProgress is progressbar_npc's bar: clif_progressbar_npc_area.
func (h scriptHost) ShowNPCProgress(id int64, ms int64, color uint32) error {
	return h.onNPC(id, false, func(z *Zone, e *entity) {
		if e.npc.floating {
			return
		}
		e.npc.barEnd, e.npc.barColor = z.now+ms, color
		for _, p := range z.playersInOrder() {
			if net, ok := p.netIDs.get(e); ok {
				p.pending = append(p.pending, npcBarEvent(net, ms, color))
			}
		}
	})
}

// npcSkillStatus is status_calc_npc_ (status.cpp): 1 HP and SP, neutral,
// demi-human, with the stat in every stat and the level, and the rest
// from status_calc_misc.
func (z *Zone) npcSkillStatus(n *npc) *battle.Status {
	s := mob.NPCStatus(n.skillLevel, n.skillStat, z.renewal())
	s.Speed = defaultNPCSpeed
	return &s
}

// NPCUseSkill is npcskill (script.cpp 23286): the NPC's status from the
// stat and level, then unit_skilluse_id2 or unit_skilluse_pos2 with no
// cast time of the script's own and no range check.
func (h scriptHost) NPCUseSkill(id, skillID, level int64, target modscript.Entity, stat, npcLevel int64) (bool, error) {
	z := h.z
	e, err := h.localNPC(id)
	if err != nil {
		return false, err
	}
	if z.Skills == nil {
		return false, errNoSkills
	}
	d := z.Skills.Get(int(skillID))
	if d == nil {
		return false, fmt.Errorf("there is no skill %d", skillID)
	}
	t, err := h.entityOf(target)
	if err != nil {
		return false, err
	}
	n := e.npc
	if stat >= 0 {
		n.skillStat = int(stat)
	}
	if npcLevel >= 0 {
		n.skillLevel = int(npcLevel)
	}
	e.status = z.npcSkillStatus(n)
	e.hp = 1
	o := &castOverride{ignoreRange: true}
	if d.Target == skill.TargetGround {
		return z.skillUsePosWith(e, d, int(level), t.x, t.y, o), nil
	}
	return z.skillUseIDWith(e, t, d, int(level), o), nil
}

// ShowSkillEffect is skilleffect's script_skill_effect: the effect of a
// skill's cast type on the unit itself, with nothing else.
func (h scriptHost) ShowSkillEffect(ref modscript.Entity, skillID, level int64) error {
	z := h.z
	if z.Skills == nil {
		return errNoSkills
	}
	d := z.Skills.Get(int(skillID))
	if d == nil {
		return fmt.Errorf("there is no skill %d", skillID)
	}
	lv := int(level)
	show := func(z *Zone, e *entity) {
		switch d.CastType() {
		case skill.CastGround:
			z.skillPosEffect(e, d, lv, e.x, e.y)
		case skill.CastNoDamage:
			z.skillEffect(e, e, d, lv, int64(lv), true)
		default:
			// clif_skill_damage with 0 damage and 1 hit.
			z.hits = append(z.hits, hit{source: e, target: e, skill: d, result: battle.Result{Kind: battle.Hit, Hits: 1}, hpPermille: e.hpPermille()})
		}
	}
	if ref.Kind == "npc" {
		return h.onNPC(ref.ID, false, show)
	}
	e, err := h.entityOf(ref)
	if err != nil {
		return err
	}
	show(z, e)
	return nil
}

func soundEvent(name string, repeat bool, from uint32) *wathenav1.Event {
	return wathenav1.Event_builder{Sound: wathenav1.SoundPlay_builder{Name: "sound/" + name, Repeat: repeat, Entity: from}.Build()}.Build()
}

// PlaySound is soundeffect, soundeffectall and maps.play_sound.
// soundeffect and a map's sound come from each player itself
// (soundeffect_sub), and an NPC's area sound from the NPC.
func (h scriptHost) PlaySound(source modscript.Entity, mapName, name, to string, repeat bool) error {
	z := h.z
	toMap := func(z *Zone) {
		for _, p := range z.playersInOrder() {
			p.pending = append(p.pending, soundEvent(name, repeat, 0))
		}
	}
	switch source.Kind {
	case "player":
		p, err := h.player(source.ID)
		if err != nil {
			return err
		}
		if to == "area" {
			// soundeffectall without a map, with an attached player: every
			// player who sees it hears it, from it (script.cpp 15315,
			// clif_soundeffect with AREA).
			for _, o := range z.playersInOrder() {
				if id, ok := o.netIDs.get(p.entity); ok {
					o.pending = append(o.pending, soundEvent(name, repeat, id))
				}
			}
			return nil
		}
		p.pending = append(p.pending, soundEvent(name, repeat, 0))
		return nil
	case "npc":
		return h.onNPC(source.ID, false, func(z *Zone, e *entity) {
			if e.npc.floating {
				return
			}
			if to == "map" {
				toMap(z)
				return
			}
			for _, p := range z.playersInOrder() {
				if id, ok := p.netIDs.get(e); ok {
					p.pending = append(p.pending, soundEvent(name, repeat, id))
				}
			}
		})
	}
	return z.onMap(mapName, toMap)
}

// PlayMusic is playBGM to a player, and playBGMall (script.cpp 15253):
// to the players of a map, of a rectangle of it (map_foreachinallarea),
// or with no map to every player of the world (map_foreachpc).
func (h scriptHost) PlayMusic(player int64, mapName, name string, area *modscript.Rect) error {
	ev := func() *wathenav1.Event {
		return wathenav1.Event_builder{Music: wathenav1.MusicPlay_builder{Name: "music/" + name}.Build()}.Build()
	}
	if player != 0 {
		p, err := h.player(player)
		if err != nil {
			return err
		}
		p.pending = append(p.pending, ev())
		return nil
	}
	play := func(z *Zone) {
		for _, p := range z.playersInOrder() {
			if e := p.entity; area != nil && (e == nil || !area.Contains(e.x, e.y)) {
				continue
			}
			p.pending = append(p.pending, ev())
		}
	}
	if mapName == "" {
		h.z.everyZone(play)
		return nil
	}
	return h.z.onMap(mapName, play)
}

// SendColoredMessage is dispbottom with a color: clif_messagecolor to
// the player alone.
func (h scriptHost) SendColoredMessage(id int64, text string, color uint32) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	p.pending = append(p.pending, wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
		Channel: wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM,
		Text:    text,
		Color:   color,
	}.Build()}.Build())
	return nil
}

// ShowDigits is showdigit (script.cpp 22745): types 1 and 2 send the
// negated absolute value, and type 3 the absolute value, at most 99.
func (h scriptHost) ShowDigits(id int64, kind int, value int64) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	switch kind {
	case 1, 2:
		value = -abs64(value)
	case 3:
		value = min(abs64(value), 99)
	}
	p.pending = append(p.pending, wathenav1.Event_builder{Digits: wathenav1.DigitsShow_builder{
		Kind:  wathenav1.DigitsKind(kind),    // #nosec G115 -- 0 to 3.
		Value: int32(max(value, -(1<<31)+1)), // #nosec G115 -- a 32-bit value, checked by the runtime.
	}.Build()}.Build())
	return nil
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// MarkMinimapOfMap is viewpointmap: clif_viewpoint to every player of
// the map.
func (h scriptHost) MarkMinimapOfMap(mapName string, mark int64, x, y int, color uint32, remove bool) error {
	return h.z.onMap(mapName, func(z *Zone) {
		for _, p := range z.playersInOrder() {
			p.pending = append(p.pending, wathenav1.Event_builder{MinimapMark: wathenav1.MinimapMark_builder{
				Id: uint32(mark), X: uint32(x), Y: uint32(y), Color: color, Remove: remove, // #nosec G115 -- checked by the runtime.
			}.Build()}.Build())
		}
	})
}

// OpenQuestLog is open_quest_ui: clif_ui_open with OUT_UI_QUEST.
func (h scriptHost) OpenQuestLog(id, quest int64) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	p.pending = append(p.pending, wathenav1.Event_builder{QuestLogOpen: wathenav1.QuestLogOpen_builder{Quest: uint32(quest)}.Build()}.Build()) // #nosec G115 -- a quest ID.
	return nil
}

// RefreshQuestIcons is questinfo_refresh: pc_show_questinfo at the end
// of the step.
func (h scriptHost) RefreshQuestIcons(id int64) error {
	p, err := h.player(id)
	if err != nil {
		return err
	}
	p.questInfoDirty = true
	return nil
}

// QuestIconsChanged is questinfo's part of the map: the NPC joins the
// map's qi_npc, and every player of the map checks its icons again.
func (h scriptHost) QuestIconsChanged(id int64) error {
	z := h.z
	e := z.npcByHandle(id)
	if e == nil || id >= remoteBase {
		return errors.New("the NPC is on another map")
	}
	if e.npc.floating {
		// buildin_questinfo: "No NPC attached" for nd->m == -1.
		return errors.New("the NPC has no map")
	}
	found := false
	for _, o := range z.questInfoNPCs {
		found = found || o == e
	}
	if !found {
		z.questInfoNPCs = append(z.questInfoNPCs, e)
	}
	// buildin_questinfo shows nothing to anyone. The players already on
	// the map see the NPC's icons at their next check, and no more once
	// the NPC is new to the map's list: pc_show_questinfo skips a player
	// whose qi_display is not the size of the map's qi_npc until it
	// enters the map again (pc.cpp 15396).
	return nil
}

// questInfoChanged asks for a player's quest icons to be checked at the
// end of the step: pc_show_questinfo after an item, a level, a job or a
// quest changed.
func (p *Player) questInfoChanged() { p.questInfoDirty = true }

// showQuestIcon shows a quest icon over an NPC to a player, or removes
// it, now when the player sees the NPC, and when the NPC comes into
// view (clif_quest_show_event).
func (z *Zone) showQuestIcon(p *Player, e *entity, icon, mark string) {
	if icon == "" {
		delete(p.questIcons, e)
	} else {
		if p.questIcons == nil {
			p.questIcons = map[*entity]questIconShown{}
		}
		p.questIcons[e] = questIconShown{icon, mark}
	}
	if net, ok := p.netIDs.get(e); ok {
		p.pending = append(p.pending, questIconEvent(net, icon, mark))
	}
}

// refreshQuestInfo is pc_show_questinfo (pc.cpp) for the players whose
// icons wait for a check: for each NPC of the map with icons, the first
// icon whose condition holds shows, when it differs from what the player
// sees, and the icon goes when none holds.
func (z *Zone) refreshQuestInfo() {
	if z.scripts == nil || len(z.questInfoNPCs) == 0 {
		for _, p := range z.players {
			if p.questInfoReinit {
				p.questInfoReinit, p.questInfoSize = false, 0
			}
			p.questInfoDirty = false
		}
		return
	}
	for _, p := range z.playersInOrder() {
		if !p.questInfoDirty {
			continue
		}
		p.questInfoDirty = false
		if p.questInfoReinit {
			// pc_show_questinfo_reinit: what the player sees starts again
			// with the map's NPCs of now. Scripts that started with the
			// player's arrival added theirs first, as OnInit runs before
			// any player in rAthena.
			p.questInfoReinit, p.questInfoSize = false, len(z.questInfoNPCs)
			p.questInfo = nil
		}
		if p.questInfoSize != len(z.questInfoNPCs) {
			continue
		}
		for _, e := range z.questInfoNPCs {
			if e.npc.removed {
				continue
			}
			icon, mark, ok := z.scripts.QuestIcon(e.npc.handle, int64(p.charID))
			cur, active := p.questInfo[e]
			switch {
			case ok && (!active || cur != questIconShown{icon, mark}):
				if p.questInfo == nil {
					p.questInfo = map[*entity]questIconShown{}
				}
				p.questInfo[e] = questIconShown{icon, mark}
				z.showQuestIcon(p, e, icon, mark)
			case !ok && active:
				delete(p.questInfo, e)
				z.showQuestIcon(p, e, "", "")
			}
		}
	}
}

var _ modscript.PresentHost = scriptHost{}
