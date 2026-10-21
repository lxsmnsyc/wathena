package convert

import (
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// The player and item calls of review decision 58, areas B7 and B9, on
// the test host: they succeed and change nothing, so every converted
// NPC's labels run.

func (h *testHost) GainExp(int64, int64, int64, bool) error            { return nil }
func (h *testHost) HealFixed(int64, int64, int64) error                { return nil }
func (h *testHost) SetHP(int64, int64) error                           { return nil }
func (h *testHost) SetBlocked(int64, uint32, bool) error               { return nil }
func (h *testHost) Blocked(int64) (uint32, error)                      { return 0, nil }
func (h *testHost) SetItemsLocked(*modscript.Conversation, bool) error { return nil }
func (h *testHost) UnequipAll(int64) error                             { return nil }
func (h *testHost) Push(int64, int, int) error                         { return nil }
func (h *testHost) Transform(int64, int64, int64) (bool, error)        { return true, nil }
func (h *testHost) OpenBook(int64, int64, int64) error                 { return nil }
func (h *testHost) SavePoint(int64) (string, int, int, error)          { return "prontera", 156, 191, nil }
func (h *testHost) Inventory(int64) ([]modscript.InventoryEntry, error) {
	return []modscript.InventoryEntry{{Slot: 0, Item: 501, Amount: 3, Identified: true, Tradable: true}}, nil
}
func (h *testHost) SetPlayerLook(int64, modscript.PlayerLook) error { return nil }
func (h *testHost) HasOption(int64, int) (bool, error)              { return false, nil }

func (h *testHost) GiveItemSpec(int64, uint32, int, modscript.ItemSpec, modscript.GiveRoll, item.Bound) (bool, error) {
	return true, nil
}
func (h *testHost) CountItemSpec(int64, uint32, modscript.CountSpec) (int, error)    { return 0, nil }
func (h *testHost) DropItem(string, int, int, uint32, int, modscript.ItemSpec) error { return nil }
func (h *testHost) CanRefine(modscript.ItemRef) (bool, error)                        { return true, nil }
func (h *testHost) RefineChance(modscript.ItemRef, int, bool) (int, error)           { return 100, nil }
func (h *testHost) EquippedRefine(int64, int) (int, error)                           { return 1, nil }
func (h *testHost) EquippedDestroy(int64, bool) error                                { return nil }
func (h *testHost) BrokenItems(int64) ([]int64, error)                               { return nil, nil }
func (h *testHost) RepairItem(int64, int) (bool, error)                              { return false, nil }
func (h *testHost) RepairAll(int64) (int, error)                                     { return 0, nil }
func (h *testHost) RunItemScript(int64, uint32) error                                { return nil }
func (h *testHost) MergeItems(int64) (int, error)                                    { return 0, nil }
func (h *testHost) Setting(name string) (int, bool) {
	v, ok := modscript.DefaultSettings[name]
	return v, ok
}
func (h *testHost) RefineCost(modscript.ItemRef, int, string) (int64, int64, bool, error) {
	return 1000, 984, true, nil
}

// ItemInfo knows the items of the test's data.
func (h *testHost) ItemInfo(id uint32) (modscript.ItemInfo, bool) {
	for name, i := range h.items {
		if i == id {
			return modscript.ItemInfo{ID: id, AegisName: name, Name: name, Type: "etc", Weight: 10}, true
		}
	}
	return modscript.ItemInfo{}, false
}

var (
	_ modscript.PlayerActionHost = (*testHost)(nil)
	_ modscript.ItemActionHost   = (*testHost)(nil)
	_ modscript.SettingsHost     = (*testHost)(nil)
)

// The job calls on the test host, which permanent_grant and keep_line
// reach now: they succeed and change nothing.
func (h *testHost) NextJobs(int) []int                                           { return nil }
func (h *testHost) ChangeJob(int64, int, bool) (bool, error)                     { return true, nil }
func (h *testHost) Rebirth(int64) (bool, error)                                  { return true, nil }
func (h *testHost) ResetSkills(int64) error                                      { return nil }
func (h *testHost) ResetStats(int64) error                                       { return nil }
func (h *testHost) ResetSkill(int64, int, int, bool) (bool, error)               { return true, nil }
func (h *testHost) ResetStat(int64, int, int) (bool, error)                      { return true, nil }
func (h *testHost) GiveSkill(int64, int, int, modscript.SkillKind) (bool, error) { return true, nil }

var _ modscript.JobHost = (*testHost)(nil)
