package zone

import (
	"errors"
	"slices"
	"strings"
)

// Player timers: rAthena's addtimer and deltimer (pc_addeventtimer,
// pc.cpp). A timer runs an NPC's label with the player attached when its
// time comes (review decision 60, C2).
//
//   - The player owns the timers. They go with it to another map, with
//     the time left, and end when it logs out.
//   - The label names an NPC, by its record name or its plain name, as
//     modscript resolves them. The NPC may be on any map. The label
//     runs as an event with the player, as npc:fire_event(label,
//     { player }) does: in a conversation that waits while the player
//     talks, or with a handle of the player when it is on another map.
//   - A player has at most maxEventTimers. An NPC that is gone when the
//     time comes ends the timer without an event.

// maxEventTimers is MAX_EVENTTIMER.
const maxEventTimers = 32

// eventTimer is one timer of player:add_timer. npc is the NPC's name as
// the script wrote it, and mod the mod that wrote it, for a plain name.
// due is in zone milliseconds, and the time left while a handoff holds
// it.
type eventTimer struct {
	due      int64
	npc, mod string
	label    string
}

// AddPlayerTimer starts a timer of a player: after ms milliseconds, the
// label of the NPC named name (or the NPC with the handle npc, when name
// is empty) runs with the player.
func (h scriptHost) AddPlayerTimer(player, ms, npc int64, name, modID, label string) error {
	z := h.z
	p, err := h.player(player)
	if err != nil {
		return err
	}
	if len(p.eventTimers) >= maxEventTimers {
		return errors.New("a player has at most 32 timers")
	}
	t := eventTimer{due: z.now + ms, label: strings.ToLower(label), mod: modID}
	if name == "" {
		e := z.npcHandles[npc]
		if e == nil {
			return errors.New("the NPC is not on this map")
		}
		name = e.npc.Name
	}
	t.npc = name
	p.eventTimers = append(p.eventTimers, t)
	return nil
}

// RemovePlayerTimer ends the first timer of the player that names the
// label of the NPC, as pc_deleventtimer. It reports whether one ended.
func (h scriptHost) RemovePlayerTimer(player, npc int64, name, modID, label string) (bool, error) {
	z := h.z
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	if name == "" {
		e := z.npcHandles[npc]
		if e == nil {
			return false, errors.New("the NPC is not on this map")
		}
		name = e.npc.Name
	}
	label = strings.ToLower(label)
	for i, t := range p.eventTimers {
		if t.npc == name && t.label == label && (t.mod == modID || strings.Contains(name, ":")) {
			p.eventTimers = slices.Delete(p.eventTimers, i, i+1)
			return true, nil
		}
	}
	return false, nil
}

// eventTimersLeft is the player's timers with the time left, for a
// handoff to another zone.
func (z *Zone) eventTimersLeft(p *Player) []eventTimer {
	out := slices.Clone(p.eventTimers)
	for i := range out {
		out[i].due = max(out[i].due-z.now, 0)
	}
	return out
}

// runEventTimers runs the timers that are due.
func (z *Zone) runEventTimers() {
	for _, p := range z.players {
		if len(p.eventTimers) == 0 || p.gone() {
			continue
		}
		var due []eventTimer
		p.eventTimers = slices.DeleteFunc(p.eventTimers, func(t eventTimer) bool {
			if t.due > z.now {
				return false
			}
			due = append(due, t)
			return true
		})
		for _, t := range due {
			z.fireEventTimer(p, t)
		}
	}
}

// fireEventTimer sends a timer's label to its NPC, with the player.
func (z *Zone) fireEventTimer(p *Player, t eventTimer) {
	if d := z.npcDir(); d != nil {
		if en := d.find(t.npc, t.mod); en != nil {
			z.postNPC(en.zone, npcMessage{kind: npcEventMessage, handle: en.handle, label: t.label, playerName: p.Name})
			return
		}
	}
	// A zone without a world directory reaches its own NPCs.
	for _, e := range z.npcsInOrder() {
		for _, full := range fullNames(t.npc, t.mod) {
			if e.npc.Name == full && !e.npc.removed {
				z.postNPC(z, npcMessage{kind: npcEventMessage, handle: e.npc.handle, label: t.label, playerName: p.Name})
				return
			}
		}
	}
}
