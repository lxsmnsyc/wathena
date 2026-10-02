package item

import (
	"encoding/json"
	"slices"
	"testing"
)

// Map job IDs of the test wearers, from rAthena's e_mapid.
const (
	testNovice   = 0x0
	testSwordman = 0x1
	testArcher   = 0x3
	testThief    = 0x6
	testAssassin = joblSecond1 | mapidThief
	testLordKn   = joblUpper | joblSecond1 | 0x1
)

// wearTable has equipment as rAthena's renewal item_db has it.
func wearTable() *Table {
	all := &JobMask{1<<testNovice | 1<<testSwordman | 1<<testThief, 1 << testSwordman, 1 << testSwordman}
	return New(&Data{Mode: "renewal", Items: []Def{
		{ID: 1101, Type: "Weapon", Subtype: Weapon1HSword, Location: Locations(EqpHandR), Atk: 25, Slots: 3, MinLevel: 2, Jobs: all},
		{ID: 1201, Type: "Weapon", Subtype: WeaponDagger, Location: Locations(EqpHandR), Atk: 17, Slots: 3},
		{ID: 1151, Type: "Weapon", Subtype: 3, Location: Locations(EqpArms), Atk: 90, Jobs: &JobMask{1 << testSwordman, 1 << testSwordman, 1 << testSwordman}},
		{ID: 1701, Type: "Weapon", Subtype: WeaponBow, Location: Locations(EqpArms), Atk: 15},
		{ID: 2101, Type: "Armor", Location: Locations(EqpHandL), Defense: 20, View: 1, Slots: 1},
		{ID: 2201, Type: "Armor", Location: Locations(EqpHeadTop | EqpHeadMid), View: 2, Gender: "Female"},
		{ID: 2601, Type: "Armor", Location: Locations(EqpAcc), Slots: 1},
		{ID: 2302, Type: "Armor", Location: Locations(EqpArmor), Slots: 1},
		{ID: 1750, Type: "Ammo", Subtype: AmmoArrow, Location: Locations(EqpAmmo), Stackable: true},
		{ID: 4001, Type: "Card", Location: Locations(EqpArmor), Stackable: true, Weight: 10},
		{ID: 4045, Type: "Card", Location: Locations(EqpAccL), Stackable: true},
		{ID: 4003, Type: "Card", Location: Locations(EqpShield), Stackable: true},
		{ID: 2602, Type: "Armor", Location: Locations(EqpAcc), Classes: ItemjUpper},
	}})
}

func wearer(mapID uint64, level int) *Wearer {
	return &Wearer{MapID: mapID, Male: true, BaseLevel: level, Renewal: true}
}

func put(inv *Inventory, slot int, id uint32) {
	inv.Slots[slot] = Slot{ID: id, Amount: 1}
}

func TestEquipPositions(t *testing.T) {
	tb := wearTable()
	inv := NewInventory()
	for slot, id := range []uint32{1101, 2101, 1151, 2601, 2601, 2201} {
		put(inv, slot, id)
	}
	sm := wearer(testSwordman, 50)
	equip := func(slot int, req uint32) Change {
		t.Helper()
		ch, r := inv.Equip(tb, slot, req, sm)
		if r != EquipOK {
			t.Fatalf("equip slot %d: result %d", slot, r)
		}
		return ch
	}
	equip(0, EqpHandR)
	equip(1, EqpHandL)
	// Accessories go right first, then left.
	if ch := equip(3, EqpAcc); ch.Position != EqpAccR {
		t.Errorf("first accessory at %#x", ch.Position)
	}
	if ch := equip(4, EqpAcc); ch.Position != EqpAccL {
		t.Errorf("second accessory at %#x", ch.Position)
	}
	// A two-handed weapon takes both hands.
	ch := equip(2, EqpArms)
	if ch.Position != EqpArms || len(ch.Unequipped) != 2 {
		t.Errorf("two-handed: %+v", ch)
	}
	w := inv.Worn(tb)
	if w.Index[EqiHandR] != 2 || w.Index[EqiHandL] != 2 || w.Weapon != 3 || w.Shield != 0 {
		t.Errorf("worn %+v", w)
	}
	// A female-only hat.
	if _, r := inv.Equip(tb, 5, EqpHelm, sm); r != EquipFail {
		t.Errorf("female hat on a male: %d", r)
	}
	// Already worn.
	if _, r := inv.Equip(tb, 2, EqpArms, sm); r != EquipFail {
		t.Errorf("worn item again: %d", r)
	}
	if ch, ok := inv.Unequip(tb, 2); !ok || ch.Unequipped[0].Position != EqpArms {
		t.Errorf("unequip: %+v %v", ch, ok)
	}
}

func TestEquipChecks(t *testing.T) {
	tb := wearTable()
	sword, _ := tb.Lookup(1101)
	slayer, _ := tb.Lookup(1151)
	ring, _ := tb.Lookup(2602)
	for _, c := range []struct {
		name string
		d    *Def
		w    *Wearer
		want EquipResult
	}{
		{"novice sword", sword, wearer(testNovice, 10), EquipOK},
		{"level 1 sword", sword, wearer(testNovice, 1), EquipFailLevel},
		{"archer sword", sword, wearer(testArcher, 10), EquipFail},
		{"novice slayer", slayer, wearer(testNovice, 99), EquipFail},
		// Jobs read the 2-1 mask for 2-1 jobs.
		{"lord knight slayer", slayer, wearer(testLordKn, 99), EquipOK},
		{"normal class, upper item", ring, wearer(testSwordman, 99), EquipFail},
		{"upper class, upper item", ring, wearer(testLordKn, 99), EquipOK},
	} {
		if got := CanEquip(c.d, c.w, WeaponFist); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

func TestDualWieldAndAmmunition(t *testing.T) {
	tb := wearTable()
	dagger, _ := tb.Lookup(1201)
	// Assassins wear daggers in either hand.
	if EquipPoint(dagger, wearer(testAssassin, 99)) != EqpArms || EquipPoint(dagger, wearer(testThief, 99)) != EqpHandR {
		t.Error("dual wield equip point")
	}
	inv := NewInventory()
	put(inv, 0, 1201)
	put(inv, 1, 1201)
	as := wearer(testAssassin, 99)
	inv.Equip(tb, 0, EqpArms, as)
	if ch, _ := inv.Equip(tb, 1, EqpArms, as); ch.Position != EqpHandL {
		t.Errorf("second dagger at %#x", ch.Position)
	}
	if w := inv.Worn(tb); w.Weapon != WeaponDoubleDD {
		t.Errorf("weapon %d, want W_DOUBLE_DD", w.Weapon)
	}

	// Arrows need a bow, and come off with it.
	inv = NewInventory()
	put(inv, 0, 1701)
	inv.Slots[1] = Slot{ID: 1750, Amount: 100}
	archer := wearer(testArcher, 50)
	if _, r := inv.Equip(tb, 1, EqpAmmo, archer); r != EquipFail {
		t.Error("arrows without a bow")
	}
	inv.Equip(tb, 0, EqpArms, archer)
	if _, r := inv.Equip(tb, 1, EqpAmmo, archer); r != EquipOK {
		t.Error("arrows with a bow")
	}
	if ch, _ := inv.Unequip(tb, 0); len(ch.Unequipped) != 2 || inv.Slots[1].Equip != 0 {
		t.Errorf("unequipping the bow: %+v", ch)
	}
}

func TestInsertCard(t *testing.T) {
	tb := wearTable()
	inv := NewInventory()
	put(inv, 0, 2302)
	inv.Slots[1] = Slot{ID: 4001, Amount: 2}
	inv.Weight = 20
	put(inv, 2, 1101)
	put(inv, 3, 2601)
	inv.Slots[4] = Slot{ID: 4045, Amount: 1}
	put(inv, 5, 1101)
	inv.Slots[6] = Slot{ID: 4003, Amount: 1}
	if !inv.InsertCard(tb, 1, 0, 4) || !slices.Equal(inv.Slots[0].Cards, []uint32{4001}) || inv.Slots[1].Amount != 1 || inv.Weight != 10 {
		t.Fatalf("insert: %+v", inv.Slots[:2])
	}
	for _, c := range []struct {
		name        string
		card, equip int
		max         int
	}{
		{"no free slot", 1, 0, 4},
		{"armor card in a weapon", 1, 2, 4},
		{"shield card in a weapon", 6, 5, 4},
		{"the slot limit", 1, 2, 0},
	} {
		if inv.InsertCard(tb, c.card, c.equip, c.max) {
			t.Errorf("%s: inserted", c.name)
		}
	}
	// A card for the left accessory only goes into a left accessory, not
	// into one for either side.
	if inv.InsertCard(tb, 4, 3, 4) {
		t.Error("a left accessory card went into an accessory for either side")
	}
	// Worn items take no cards.
	inv.Slots[5].Equip = EqpHandR
	inv.Slots[6] = Slot{ID: 4001, Amount: 1}
	if inv.InsertCard(tb, 6, 5, 4) {
		t.Error("card into a worn item")
	}
}

func TestLocationsJSON(t *testing.T) {
	var d Def
	if err := json.Unmarshal([]byte(`{"location": ["head_top", "head_middle"], "jobs": ["1", "2", "18446744073709551615"]}`), &d); err != nil {
		t.Fatal(err)
	}
	if uint32(d.Location) != EqpHeadTop|EqpHeadMid || d.Jobs[2] != 1<<64-1 {
		t.Errorf("location %#x, jobs %v", d.Location, d.Jobs)
	}
	if err := json.Unmarshal([]byte(`{"location": ["head"]}`), &d); err == nil {
		t.Error("an unknown position was read")
	}
	b, err := json.Marshal(JobMask{1, 2, 3})
	if err != nil || string(b) != `["1","2","3"]` {
		t.Errorf("marshal %s %v", b, err)
	}
}
