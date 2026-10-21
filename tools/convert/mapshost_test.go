package convert

import (
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// The MapHost of the NPC runtime tests (review decision 58, area B8):
// every map runs and has one player, spawned monsters keep their labels,
// and a death sends its label's event, which the test runs after each
// step, as the zone does.

// mobLabels are the labels of spawned monsters, by handle ID.
var _ modscript.MapHost = (*npcHost)(nil)

func (h *npcHost) CountPlayers(string, *modscript.Rect) (int, bool) { return 1, true }
func (h *npcHost) CountMonsters(_, label string) (int, bool) {
	n := 0
	for id := range h.spawned {
		if label == "" || h.labels[id].Key == label {
			n++
		}
	}
	return n, true
}
func (h *npcHost) RemoveMonsters(_ string, r modscript.RemoveMonsters) error {
	for _, id := range sortedIDs(h.spawned) {
		if r.Label != "" && h.labels[id].Key != r.Label {
			continue
		}
		h.die(id, 0, r.RunLabels)
	}
	return nil
}
func (h *npcHost) WarpPlayers(string, modscript.WarpPlayers) error           { return nil }
func (h *npcHost) SetCells(string, modscript.Rect, string, bool, bool) error { return nil }
func (h *npcHost) CheckCell(_ string, _, _ int, kind string) (bool, error) {
	return kind == "pass" || kind == "reach", nil
}
func (h *npcHost) SetMapFlag(string, string, bool) error                     { return nil }
func (h *npcHost) MapFlag(string, string) (bool, error)                      { return false, nil }
func (h *npcHost) AnnounceArea(string, string, string, modscript.Rect) error { return nil }
func (h *npcHost) MonsterAct(int64, modscript.MonsterAction) (bool, error)   { return true, nil }
func (h *npcHost) PartyWarp(int64, modscript.PartyWarp) error                { return nil }
func (h *npcHost) FindMonster(id int64) (int64, bool) {
	_, ok := h.spawned[id]
	return id, ok
}
func (h *npcHost) LabelOwner(name, mod string) int64 { id, _ := h.FindNPC(name, mod); return id }
func (h *npcHost) FindFreeCell(_ string, q modscript.FreeCell) (int, int, bool, error) {
	return q.X + 1, q.Y, true, nil
}
func (h *npcHost) MonsterInfo(id uint32) (modscript.MonsterInfo, bool) {
	if id == 0 {
		return modscript.MonsterInfo{}, false
	}
	return modscript.MonsterInfo{ID: id, Name: "Monster", Level: 1, HP: 50, Modes: 0x81}, true
}

// die ends a spawned monster: the death event runs, and its label's
// event waits in fired, with the killer.
func (h *npcHost) die(id, killer int64, labels bool) {
	delete(h.spawned, id)
	if l := h.labels[id]; labels && l.NPC != 0 {
		h.fired = append(h.fired, firedEvent{l.NPC, l.Event, killer})
	}
	delete(h.labels, id)
}
