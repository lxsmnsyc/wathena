package item

import (
	"slices"
	"testing"
)

// testTable has a few items as rAthena's renewal item_db has them, and a
// knife with no weight, for filling slots.
func testTable() *Table {
	return New(&Data{Mode: "renewal", Items: []Def{
		{ID: 501, Type: "Healing", Weight: 70, Stackable: true},
		{ID: 909, Type: "Etc", Weight: 10, Stackable: true},
		{ID: 1201, Type: "Weapon", Weight: 400},
		{ID: 1202, Type: "Weapon"},
		{ID: 7000, Type: "Etc", Weight: 1, Stackable: true, StackLimit: 10},
		{ID: 7001, Type: "Etc", Weight: 1, Stackable: true, UniqueID: true},
	}})
}

func TestAddStacks(t *testing.T) {
	tb := testTable()
	inv := NewInventory()
	add := func(id uint32, amount int) (int, AddResult) {
		return inv.Add(tb.Get(id), amount, 20000)
	}
	for _, c := range []struct {
		id     uint32
		amount int
		slot   int
	}{
		{501, 5, 0}, {909, 1, 1}, {501, 3, 0},
		// Weapons never stack.
		{1201, 1, 2}, {1201, 1, 3},
		// Items with a unique ID do not either.
		{7001, 1, 4}, {7001, 1, 5},
		// An item the database lacks is an Etc item with no weight.
		{123456, 2, 6}, {123456, 2, 6},
	} {
		if slot, r := add(c.id, c.amount); r != AddSuccess || slot != c.slot {
			t.Errorf("add %d × %d: slot %d, %d; want slot %d", c.id, c.amount, slot, r, c.slot)
		}
	}
	if inv.Slots[0] != (Slot{ID: 501, Amount: 8}) || inv.Count(123456) != 4 {
		t.Errorf("slots %v", inv.Slots[:7])
	}
	// 8 × 7 + 10 + 2 × 40 + 2 × 0.1.
	if inv.Weight != 8*70+10+2*400+2 {
		t.Errorf("weight %d", inv.Weight)
	}
	// An emptied slot takes the next new item.
	if !inv.Delete(tb, 1, 1) || inv.Slots[1] != (Slot{}) {
		t.Errorf("slot 1 %v", inv.Slots[1])
	}
	if slot, _ := add(1201, 1); slot != 1 {
		t.Errorf("new item in slot %d, want 1", slot)
	}
}

func TestAddLimits(t *testing.T) {
	tb := testTable()
	inv := NewInventory()
	for _, c := range []struct {
		id     uint32
		amount int
		want   AddResult
	}{
		{0, 1, AddInvalid},
		{501, 0, AddInvalid},
		{909, MaxAmount + 1, AddOverAmount},
		{909, MaxAmount, AddSuccess},
		{909, 1, AddOverAmount},
		{7000, 11, AddStackLimit},
		{7000, 6, AddSuccess},
		{7000, 5, AddOverAmount},
	} {
		if _, r := inv.Add(tb.Get(c.id), c.amount, 1_000_000); r != c.want {
			t.Errorf("add %d × %d: %d, want %d", c.id, c.amount, r, c.want)
		}
	}
	if inv.Count(909) != MaxAmount || inv.Count(7000) != 6 {
		t.Errorf("amounts %d, %d", inv.Count(909), inv.Count(7000))
	}
}

func TestAddOverweight(t *testing.T) {
	tb := testTable()
	inv := NewInventory()
	// 10 Red Potions weigh 70.0, the maximum.
	if _, r := inv.Add(tb.Get(501), 10, 700); r != AddSuccess {
		t.Fatal(r)
	}
	if _, r := inv.Add(tb.Get(501), 1, 700); r != AddOverweight {
		t.Errorf("11th potion: %d", r)
	}
	// Items with no weight still fit.
	if _, r := inv.Add(tb.Get(1202), 1, 700); r != AddSuccess {
		t.Errorf("knife: %d", r)
	}
	if inv.Weight != 700 || inv.Count(501) != 10 {
		t.Errorf("weight %d, potions %d", inv.Weight, inv.Count(501))
	}
}

func TestAddFull(t *testing.T) {
	tb := testTable()
	inv := NewInventory()
	if _, r := inv.Add(tb.Get(909), 1, 1000); r != AddSuccess {
		t.Fatal(r)
	}
	for i := 1; i < BaseSlots; i++ {
		if _, r := inv.Add(tb.Get(1202), 1, 1000); r != AddSuccess {
			t.Fatalf("slot %d: %d", i, r)
		}
	}
	if inv.Blank() != 0 {
		t.Errorf("blank %d", inv.Blank())
	}
	// Slots past inventory_slots take nothing new.
	if _, r := inv.Add(tb.Get(1202), 1, 1000); r != AddOverItem {
		t.Errorf("knife: %d", r)
	}
	if _, r := inv.Add(tb.Get(501), 1, 1000); r != AddOverItem {
		t.Errorf("new stack: %d", r)
	}
	// A stack already held still grows.
	if slot, r := inv.Add(tb.Get(909), 1, 1000); r != AddSuccess || slot != 0 {
		t.Errorf("stack: slot %d, %d", slot, r)
	}
	// A stack in a slot past inventory_slots does not.
	inv.Slots[150] = Slot{ID: 501, Amount: 1}
	if _, r := inv.Add(tb.Get(501), 1, 1000); r != AddOverAmount {
		t.Errorf("stack past the size: %d", r)
	}
}

func TestDelete(t *testing.T) {
	tb := testTable()
	inv := NewInventory()
	inv.Add(tb.Get(501), 3, 1000)
	for _, c := range []struct {
		slot, amount int
		ok           bool
	}{{-1, 1, false}, {MaxInventory, 1, false}, {1, 1, false}, {0, 0, false}, {0, 4, false}, {0, 2, true}, {0, 1, true}, {0, 1, false}} {
		if got := inv.Delete(tb, c.slot, c.amount); got != c.ok {
			t.Errorf("delete %d × %d: %v", c.slot, c.amount, got)
		}
	}
	if inv.Slots[0] != (Slot{}) || inv.Weight != 0 {
		t.Errorf("slot %v, weight %d", inv.Slots[0], inv.Weight)
	}
}

func TestOverweight(t *testing.T) {
	for _, c := range []struct {
		renewal        bool
		weight, levels []int
	}{
		{true, []int{0, 699, 700, 899, 900, 1000}, []int{WeightNormal, WeightNormal, Weight50, Weight50, Weight90, Weight90}},
		{false, []int{499, 500, 899, 900}, []int{WeightNormal, Weight50, Weight50, Weight90}},
	} {
		cfg := DefaultConfig(c.renewal)
		var got []int
		for _, w := range c.weight {
			got = append(got, cfg.Overweight(w, 1000))
		}
		if !slices.Equal(got, c.levels) {
			t.Errorf("renewal %v: %v, want %v", c.renewal, got, c.levels)
		}
	}
	// pc_getpercentweight rounds down.
	if p := PercentWeight(1799, 2000); p != 89 {
		t.Errorf("percent %d", p)
	}
}
