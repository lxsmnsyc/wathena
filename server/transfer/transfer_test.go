package transfer

import (
	"errors"
	"testing"

	"github.com/lxsmnsyc/wathena/server/item"
)

func testTable() *item.Table {
	return item.New(&item.Data{Mode: "renewal", Items: []item.Def{
		{ID: 501, Type: "Healing", Weight: 70, Stackable: true},
		{ID: 909, Type: "Etc", Weight: 10, Stackable: true},
		{ID: 1201, Type: "Weapon", Weight: 400},
		{ID: 7000, Type: "Etc", Weight: 1, Stackable: true, StackLimit: 10},
		{ID: 2000, Type: "Etc", Weight: 1, Stackable: true, Rolls: map[string]*item.RollDef{
			"luck": {Min: 1, Max: 5, Default: 3, Hidden: true},
		}},
	}})
}

func inventory(t *testing.T, tb *item.Table, id int64, maxWeight int) *InventoryBox {
	t.Helper()
	return &InventoryBox{Inv: item.NewInventory(), Items: tb, MaxWeight: maxWeight, Who: Inventory(id)}
}

// give puts items in a box through a Tx and returns the slot's serial.
func give(t *testing.T, b Box, id uint32, amount int) item.Serial {
	t.Helper()
	var slot int
	_, err := New(ActionGive).Create(End(KindScript), item.Slot{ID: id}, amount, b, item.Serial{}).Apply(func(c Change) { slot = c.Slot })
	if err != nil {
		t.Fatal(err)
	}
	return b.Slot(slot).Serial
}

func TestCreateAndMerge(t *testing.T) {
	tb := testTable()
	a := inventory(t, tb, 1, 10000)
	s := give(t, a, 909, 5)
	if s.IsZero() || a.Inv.Slots[0].Amount != 5 {
		t.Fatalf("slot 0 %+v", a.Inv.Slots[0])
	}
	origin := item.NewSerial()
	logs, err := New(ActionPickUp).Create(End(KindGround), item.Slot{ID: 909}, 3, a, origin).Apply(nil)
	if err != nil {
		t.Fatal(err)
	}
	// The amount joins the stack, which keeps its serial.
	if a.Inv.Slots[0].Amount != 8 || a.Inv.Slots[0].Serial != s {
		t.Errorf("slot 0 %+v", a.Inv.Slots[0])
	}
	l := logs.Items[0]
	if l.Serial != s || l.Source != origin || l.From != End(KindGround) || l.To != Inventory(1) || l.Amount != 3 || l.Action != ActionPickUp {
		t.Errorf("log %+v", l)
	}
	if a.Inv.Weight != 80 {
		t.Errorf("weight %d", a.Inv.Weight)
	}
}

func TestMoveSerials(t *testing.T) {
	tb := testTable()
	a, b := inventory(t, tb, 1, 10000), inventory(t, tb, 2, 10000)
	s := give(t, a, 909, 10)
	// Part of a stack gets a new serial and names its source.
	logs, err := New(ActionMove).Move(a, 0, s, 4, b).Apply(nil)
	if err != nil {
		t.Fatal(err)
	}
	split := b.Inv.Slots[0].Serial
	if split == s || split.IsZero() || logs.Items[0].Source != s || logs.Items[0].Serial != split {
		t.Errorf("split %v from %v, log %+v", split, s, logs.Items[0])
	}
	// The rest moves whole into a stack that exists: it merges and takes
	// that stack's serial.
	logs, err = New(ActionMove).Move(a, 0, s, 6, b).Apply(nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.Inv.Slots[0].Amount != 10 || b.Inv.Slots[0].Serial != split || logs.Items[0].Source != s || a.Inv.Slots[0].ID != 0 {
		t.Errorf("merge: %+v, log %+v", b.Inv.Slots[0], logs.Items[0])
	}
	// A whole stack into a new slot keeps its serial.
	w := give(t, a, 1201, 1)
	logs, err = New(ActionMove).Move(a, 0, w, 1, b).Apply(nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.Inv.Slots[1].Serial != w || !logs.Items[0].Source.IsZero() {
		t.Errorf("whole move: %+v, log %+v", b.Inv.Slots[1], logs.Items[0])
	}
}

func TestAllOrNothing(t *testing.T) {
	tb := testTable()
	a, b := inventory(t, tb, 1, 10000), inventory(t, tb, 2, 500)
	potions := give(t, a, 501, 5)
	jellopy := give(t, a, 909, 5)
	wa := &ZenyWallet{Who: Inventory(1), Z: new(int64(100))}
	// The potions fit, the weapon does not: b's limit is 500.
	weapon := give(t, a, 1201, 1)
	before := *a.Inv
	tx := New(ActionMove).Move(a, 0, potions, 5, b).Move(a, 1, jellopy, 5, b).Move(a, 2, weapon, 1, b).Pay(wa, 50, End(KindShop))
	_, err := tx.Apply(func(Change) { t.Error("a change was made") })
	var e *Error
	if !errors.As(err, &e) || e.Op != 2 || e.Reason != ReasonOverweight {
		t.Fatalf("error %v", err)
	}
	if !sameInventory(a.Inv, &before) || b.Inv.Slots[0].ID != 0 || wa.Zeny() != 100 {
		t.Error("the failed Tx changed something")
	}
	// A serial the slot does not hold.
	_, err = New(ActionMove).Move(a, 0, jellopy, 1, b).Apply(nil)
	if !errors.As(err, &e) || e.Reason != ReasonNotOwner {
		t.Errorf("wrong serial: %v", err)
	}
	// More than the slot holds.
	_, err = New(ActionMove).Move(a, 0, potions, 6, b).Apply(nil)
	if !errors.As(err, &e) || e.Reason != ReasonAmount {
		t.Errorf("too many: %v", err)
	}
	// Two moves of the same slot that pass alone and fail together.
	_, err = New(ActionMove).Move(a, 0, potions, 3, b).Move(a, 0, potions, 3, b).Apply(nil)
	if !errors.As(err, &e) || e.Op != 1 || e.Reason != ReasonAmount || a.Inv.Slots[0].Amount != 5 {
		t.Errorf("double spend: %v, %+v", err, a.Inv.Slots[0])
	}
}

func TestStackRules(t *testing.T) {
	tb := testTable()
	a := inventory(t, tb, 1, 10000)
	// A stack limit of 10.
	give(t, a, 7000, 8)
	_, err := New(ActionGive).Create(End(KindScript), item.Slot{ID: 7000}, 3, a, item.Serial{}).Apply(nil)
	var e *Error
	if !errors.As(err, &e) || e.Reason != ReasonOverAmount {
		t.Errorf("over the stack limit: %v", err)
	}
	// Copies with other rolls form their own stacks, and unidentified
	// copies with hidden rolls never stack.
	low := item.Slot{ID: 2000, Extra: item.Extra{Rolls: item.Rolls{{Name: "luck", Value: 1}}}}
	high := item.Slot{ID: 2000, Extra: item.Extra{Rolls: item.Rolls{{Name: "luck", Value: 5}}}}
	hidden := item.Slot{ID: 2000, Extra: item.Extra{Rolls: item.Rolls{{Name: "luck", Value: 5}}, Unidentified: true}}
	for _, c := range []item.Slot{low, high, low, hidden, hidden} {
		if _, err := New(ActionGive).Create(End(KindScript), c, 1, a, item.Serial{}).Apply(nil); err != nil {
			t.Fatal(err)
		}
	}
	if a.Inv.Slots[1].Amount != 2 || a.Inv.Slots[2].Amount != 1 || a.Inv.Slots[3].Amount != 1 || a.Inv.Slots[4].Amount != 1 {
		t.Errorf("stacks %+v", a.Inv.Slots[:5])
	}
	// A move keeps the rolls.
	b := inventory(t, tb, 2, 10000)
	if _, err := New(ActionMove).Move(a, 2, a.Inv.Slots[2].Serial, 1, b).Apply(nil); err != nil {
		t.Fatal(err)
	}
	if !b.Inv.Slots[0].Extra.Equal(&high.Extra) {
		t.Errorf("moved copy %+v", b.Inv.Slots[0])
	}
}

func TestZeny(t *testing.T) {
	a := &ZenyWallet{Who: Inventory(1), Z: new(int64(100))}
	b := &ZenyWallet{Who: Inventory(2), Z: new(int64(MaxZeny - 10))}
	if _, err := New(ActionBuy).Pay(a, 101, End(KindShop)).Apply(nil); err == nil {
		t.Error("paid more than the wallet holds")
	}
	if _, err := New(ActionMove).MoveZeny(a, 50, b).Apply(nil); err == nil {
		t.Error("passed MaxZeny")
	}
	logs, err := New(ActionSell).Earn(End(KindShop), 50, b, true).Apply(nil)
	if err != nil || b.Zeny() != MaxZeny || logs.Zeny[0].Amount != 10 {
		t.Errorf("earn with drop: %v, %d, %+v", err, b.Zeny(), logs.Zeny)
	}
	logs, err = New(ActionMove).MoveZeny(a, 60, &ZenyWallet{Who: Inventory(3), Z: new(int64(0))}).Apply(nil)
	if err != nil || a.Zeny() != 40 || logs.Zeny[0].From != Inventory(1) || logs.Zeny[0].To != Inventory(3) {
		t.Errorf("move: %v, %d, %+v", err, a.Zeny(), logs.Zeny)
	}
	if _, err := New(ActionMove).Pay(a, -1, End(KindShop)).Apply(nil); err == nil {
		t.Error("a negative amount")
	}
}

func TestListBox(t *testing.T) {
	tb := testTable()
	storage := &ListBox{Size: 2, Items: tb, Who: Owner{Kind: KindStorage, ID: 7}}
	a := inventory(t, tb, 1, 10000)
	s := give(t, a, 909, 5)
	w := give(t, a, 1201, 1)
	give(t, a, 501, 1)
	if _, err := New(ActionMove).Move(a, 0, s, 2, storage).Move(a, 0, s, 3, storage).Move(a, 1, w, 1, storage).Apply(nil); err != nil {
		t.Fatal(err)
	}
	if len(storage.Slots) != 2 || storage.Slots[0].Amount != 5 || storage.Slots[1].Serial != w || storage.Weight != 450 {
		t.Errorf("storage %+v, weight %d", storage.Slots, storage.Weight)
	}
	var e *Error
	if _, err := New(ActionMove).Move(a, 2, a.Inv.Slots[2].Serial, 1, storage).Apply(nil); !errors.As(err, &e) || e.Reason != ReasonFull {
		t.Errorf("a third slot: %v", err)
	}
	// Taking out frees the slot.
	if _, err := New(ActionMove).Move(storage, 1, w, 1, a).Apply(nil); err != nil {
		t.Fatal(err)
	}
	if storage.Slots[1].ID != 0 || storage.Weight != 50 {
		t.Errorf("storage %+v, weight %d", storage.Slots, storage.Weight)
	}
}

func TestKinds(t *testing.T) {
	for k := KindInventory; k <= KindUsed; k++ {
		if got, ok := ParseKind(k.String()); !ok || got != k {
			t.Errorf("%d: %q parses as %d", k, k.String(), got)
		}
	}
	if _, ok := ParseKind("none"); ok {
		t.Error("none parses")
	}
	if !KindVending.Holder() || KindGround.Holder() {
		t.Error("Holder")
	}
}

func sameInventory(a, b *item.Inventory) bool {
	if a.Size != b.Size || a.Weight != b.Weight {
		return false
	}
	for i := range a.Slots {
		if !a.Slots[i].Equal(&b.Slots[i]) {
			return false
		}
	}
	return true
}
