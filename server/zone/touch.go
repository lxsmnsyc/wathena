package zone

import (
	"slices"
)

// NPC touch areas (review decision 55), as rAthena's OnTouch areas:
//   - An NPC record's touch field gives the area: the cells within
//     XSpan and YSpan of the NPC's cell (npc_touch_areanpc).
//   - The zone checks a player's cell after each step, and once on the
//     cell it arrives on, after its first frame, as unit_walktoxy_timer
//     and clif_parse_LoadEndAck call npc_touch_area_allnpc. A step onto
//     a warp that takes the player checks no touch area, since rAthena
//     sorts warps first and stops at a warp that moves the player.
//   - A player who comes into an area fires the NPC's touch event, once
//     until it leaves, as rAthena's OnTouch and the player's areanpc
//     list. Hidden and dead players fire it too: npc_ontouch2_event
//     does not check them. Converted OnTouch_ labels skip them in Lua
//     (tools/convert's R.touch_one).
//   - A player who leaves an area fires untouch: a step out of it, a
//     warp to another cell or map, or a logout, as
//     npc_touchnext_areanpc does when the player leaves.
//   - The events run when the zone runs its script events, after the
//     walks. A touch whose player left the area or the zone by then does
//     not run, as npc_event_sub checks the area again.
//   - A touch runs the NPC's touch handlers in a conversation with the
//     player, so they may show a dialog as rAthena's OnTouch (review
//     decision 56). A touch while the player talks to an NPC waits until
//     the conversation ends, as npc_event_sub queues it in the player's
//     eventqueue: at most maxTouchQueue wait, and later ones are
//     dropped. The first runs touchQueueDelay after the conversation
//     ended, as npc_event_dequeue's timer, and the area is checked again
//     then. untouch runs at once, with no conversation.

// Touches that wait for a conversation to end: rAthena's MAX_EVENTQUEUE,
// and the delay of npc_event_dequeue's pc_addeventtimer, in
// milliseconds.
const (
	maxTouchQueue   = 2
	touchQueueDelay = 100
)

// TouchArea is an NPC's touch area: XSpan and YSpan cells around it.
type TouchArea struct {
	XSpan, YSpan int
}

// touchEvent is a player stepping into or out of an NPC's touch area.
type touchEvent struct {
	npc     *entity
	player  *Player
	untouch bool
	// label is set for an NPC event with a player (doevent), which waits
	// in the same queue (npcworld.go).
	label string
}

// touches reports whether an NPC's touch area holds a cell.
func (n *npc) touches(e *entity, x, y int) bool {
	t := n.Touch
	return t != nil && x >= e.x-t.XSpan && x <= e.x+t.XSpan && y >= e.y-t.YSpan && y <= e.y+t.YSpan
}

// touchAreas checks a player's cell against the touch areas: untouch
// for the areas it left, then touch for the areas it came into, in the
// order the NPCs were placed.
func (z *Zone) touchAreas(p *Player) {
	if len(p.touching) == 0 && len(z.touchNPCs) == 0 {
		return
	}
	// npc_touch_area_allnpc and unit_walktoxy_ontouch skip a player with
	// PCBLOCK_NPCCLICK (npc.cpp 1970, unit.cpp 511).
	if p.isBlocked(blockNPCClick) {
		return
	}
	e := p.entity
	kept := p.touching[:0]
	for _, n := range p.touching {
		if z.entities.has(n) && n.npc.touches(n, e.x, e.y) {
			kept = append(kept, n)
			continue
		}
		z.queueTouch(n, p, true)
	}
	clear(p.touching[len(kept):])
	p.touching = kept
	for _, n := range z.touchNPCs {
		if n.npc.touches(n, e.x, e.y) && z.entities.has(n) && n.npc.touchableBy(p) && !slices.Contains(p.touching, n) {
			p.touching = append(p.touching, n)
			z.queueTouch(n, p, false)
		}
	}
}

// untouchAll is a player leaving every touch area, when it leaves the
// zone.
func (z *Zone) untouchAll(p *Player) {
	for _, n := range p.touching {
		z.queueTouch(n, p, true)
	}
	p.touching = nil
	clear(p.touchQueue)
	p.touchQueue = p.touchQueue[:0]
}

func (z *Zone) queueTouch(n *entity, p *Player, untouch bool) {
	if z.scripts == nil || n.npc.script == nil {
		return
	}
	z.scriptEvents = append(z.scriptEvents, scriptEvent{touch: &touchEvent{npc: n, player: p, untouch: untouch}})
}

// runTouch runs an NPC's touch or untouch handlers.
func (z *Zone) runTouch(t *touchEvent) {
	n := t.npc.npc
	if n.script == nil || z.scripts == nil {
		return
	}
	p := t.player
	if t.untouch {
		_, _ = z.scripts.Touch(n.script, "untouch", int64(p.charID))
		if z.players[p.charID] == p {
			z.afterTalk(p)
		}
		return
	}
	// npc_event_sub: the player must still be in the area.
	if z.players[p.charID] != p || !slices.Contains(p.touching, t.npc) {
		return
	}
	if p.talk != nil {
		// npc_event_sub queues the event while the player talks.
		if len(p.touchQueue) < maxTouchQueue {
			p.touchQueue = append(p.touchQueue, t)
			p.touchAt = z.now + touchQueueDelay
		}
		return
	}
	conv, err := z.scripts.Touch(n.script, "touch", int64(p.charID))
	if err != nil {
		if z.Mods != nil && z.Mods.Log != nil {
			z.Mods.Log.Warn("NPC touch failed", "map", z.Map, "npc", n.Name, "err", err)
		}
		return
	}
	if conv != nil && !conv.Done && p.talk == nil {
		p.talk = &talk{conv: conv, npc: t.npc}
	}
	z.afterTalk(p)
}

// queuedTouches runs the touches that waited for a conversation, one at
// a time, touchQueueDelay after the player stopped talking, as
// npc_event_dequeue does when a script ends.
func (z *Zone) queuedTouches() {
	for _, p := range z.players {
		if len(p.touchQueue) == 0 {
			continue
		}
		if p.talk != nil {
			p.touchAt = z.now + touchQueueDelay
			continue
		}
		if z.now < p.touchAt {
			continue
		}
		t := p.touchQueue[0]
		p.touchQueue[0] = nil
		p.touchQueue = p.touchQueue[1:]
		if t.label != "" {
			z.runNPCEvent(t)
			continue
		}
		z.runTouch(t)
	}
}
