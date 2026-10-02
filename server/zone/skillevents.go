package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/skill"
)

// Skill events: what viewers see of skills. They are rAthena's
// clif_skillcasting, clif_skillcastcancel, clif_skill_nodamage and
// clif_skill_poseffect. Skill damage goes with the other hits, as
// clif_skill_damage.

type skillEventKind int

const (
	skillEventCast skillEventKind = iota
	skillEventCancel
	skillEventEffect
)

// skillEvent is one skill event of this tick.
type skillEvent struct {
	kind skillEventKind
	src  *entity
	// target is the entity the skill aims at, or nil for a cell.
	target *entity
	x, y   int
	cell   bool
	skill  *skill.Def
	lv     int
	castMs int
	// value is what an effect shows, such as the HP healed. ok is false
	// for an effect that failed on its target.
	value int64
	ok    bool
}

// shownSkill is the skill ID the client shows for a skill: the skill a
// mod's skill borrows its icon and animation from, or its own.
func shownSkill(d *skill.Def) uint32 {
	if d.Icon != 0 {
		return uint32(d.Icon) // #nosec G115 -- a skill ID.
	}
	return uint32(d.ID) // #nosec G115 -- skill IDs are positive.
}

// skillEffect queues clif_skill_nodamage: a skill took effect on a
// target, with the value it shows.
func (z *Zone) skillEffect(src, target *entity, d *skill.Def, lv int, value int64, ok bool) {
	z.skillEvents = append(z.skillEvents, skillEvent{kind: skillEventEffect, src: src, target: target, skill: d, lv: lv,
		value: value, ok: ok})
}

// skillPosEffect queues clif_skill_poseffect: a ground skill took effect
// on a cell.
func (z *Zone) skillPosEffect(src *entity, d *skill.Def, lv, x, y int) {
	z.skillEvents = append(z.skillEvents, skillEvent{kind: skillEventEffect, src: src, x: x, y: y, cell: true, skill: d,
		lv: lv, ok: true})
}

// skillEventsFor are the skill events a player sees this tick: those of
// sources it sees.
func (z *Zone) skillEventsFor(p *Player) []*wathenav1.Event {
	var events []*wathenav1.Event
	for i := range z.skillEvents {
		ev := &z.skillEvents[i]
		src, ok := p.netIDs.get(ev.src)
		if !ok {
			continue
		}
		var target *uint32
		if ev.target != nil {
			if id, ok := p.netIDs.get(ev.target); ok {
				target = &id
			}
		}
		var cell *wathenav1.Cell
		if ev.cell {
			cell = wathenav1.Cell_builder{X: uint32(ev.x), Y: uint32(ev.y)}.Build() // #nosec G115 -- map cells are at most 1024.
		}
		switch ev.kind {
		case skillEventCast:
			b := wathenav1.SkillCast_builder{
				Source:  src,
				SkillId: shownSkill(ev.skill),
				Level:   uint32(max(ev.lv, 0)),     // #nosec G115 -- a small level.
				CastMs:  uint32(max(ev.castMs, 0)), // #nosec G115 -- a cast time.
				Entity:  target,
				Cell:    cell,
				Element: uint32(max(ev.skill.Element.At(ev.lv), 0)), // #nosec G115 -- an element.
			}
			events = append(events, wathenav1.Event_builder{SkillCast: b.Build()}.Build())
		case skillEventCancel:
			events = append(events, wathenav1.Event_builder{SkillCastCancel: wathenav1.SkillCastCancel_builder{Source: src}.Build()}.Build())
		case skillEventEffect:
			if ev.target != nil && target == nil {
				continue
			}
			b := wathenav1.SkillEffect_builder{
				Source:  src,
				SkillId: shownSkill(ev.skill),
				Level:   uint32(max(ev.lv, 0)), // #nosec G115 -- a small level.
				Entity:  target,
				Cell:    cell,
				Value:   ev.value,
				Ok:      ev.ok,
			}
			events = append(events, wathenav1.Event_builder{SkillEffect: b.Build()}.Build())
		}
	}
	return events
}
