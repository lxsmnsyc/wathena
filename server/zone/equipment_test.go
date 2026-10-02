package zone

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// equipItems are renewal items with their equipment data from
// equipment.json: a sword, a shield, a two-handed sword, a hat, a shirt
// with a card slot, the Poring Card with its static bonuses, a bow, arrows,
// and the Academy Axe, whose converted script computes its bonuses.
func equipItems(t *testing.T) *item.Table {
	t.Helper()
	var d item.Data
	if err := reference.LoadFile("extracts/renewal/items.json", &d); err != nil {
		t.Fatal(err)
	}
	var e item.EquipData
	if err := reference.LoadFile("extracts/renewal/equipment.json", &e); err != nil {
		t.Fatal(err)
	}
	d.MergeEquipment(&e)
	keep := map[uint32]bool{1101: true, 2101: true, 1151: true, 2220: true, 2302: true, 4001: true, 1701: true, 1750: true, 1317: true, 909: true}
	d.Items = slices.DeleteFunc(d.Items, func(x item.Def) bool { return !keep[x.ID] })
	for i := range d.Items {
		x := &d.Items[i]
		switch x.ID {
		case 4001:
			// The Poring Card's bonuses, as core's item data has them.
			for _, b := range []struct {
				name string
				v    int64
			}{{"luk", 2}, {"perfect_dodge", 1}} {
				x.Static = append(x.Static, &bonus.Bonus{Name: b.name, Args: []bonus.Num{{Value: b.v}}, Options: bonus.Options{Set: map[string]bool{}}})
			}
		case 1317:
			x.Script = "core:scripts/items/Academy_Axe.lua"
		}
	}
	return item.New(&d)
}

// equipZone makes a zone with the equipment items, renewal job tables and
// two players next to each other at base level 50: a Novice and a
// Merchant.
func equipZone(t *testing.T) (*Zone, *Player, *Player) {
	t.Helper()
	tb := jobTables(t)
	z := New("map/test", open(40), rng.FromKey([32]byte{9}).Stream(), nil)
	z.Items = equipItems(t)
	z.Progress = testTables()
	z.Status = tb
	z.Start = status.Character{Male: true, Stats: status.Stats{Str: 50, Agi: 30, Vit: 30, Int: 1, Dex: 30, Luk: 1}}
	coreMods(t, z)
	a, b := add(z, "a"), add(z, "b")
	b.char.Job = 5
	for _, p := range []*Player{a, b} {
		p.progress.BaseLevel = 50
		z.recalc(p)
	}
	b.entity.x = a.entity.x + 1
	z.stepAt(50)
	events(a)
	events(b)
	return z, a, b
}

// give puts an item instance in a player's inventory and returns its
// slot.
func give(t *testing.T, z *Zone, p *Player, id uint32, amount int) int {
	t.Helper()
	if r := z.addItem(p, id, amount); r != item.AddSuccess {
		t.Fatalf("add %d: %v", id, r)
	}
	for i, s := range p.inventory.Slots {
		if s.ID == id {
			return i
		}
	}
	t.Fatalf("item %d not found", id)
	return -1
}

func equip(z *Zone, p *Player, slot int) {
	z.handle(command{player: p, equip: &equipCommand{slot: slot}})
}

func unequip(z *Zone, p *Player, slot int) {
	z.handle(command{player: p, unequip: &slot})
}

func TestEquipAndUnequip(t *testing.T) {
	z, a, b := equipZone(t)
	sword := give(t, z, a, 1101, 1)
	shield := give(t, z, a, 2101, 1)
	slayer := give(t, z, a, 1151, 1)
	hat := give(t, z, a, 2220, 1)
	before := *a.entity.status
	a.pending = nil

	equip(z, a, sword)
	equip(z, a, shield)
	equip(z, a, hat)
	z.stepAt(100)
	got := events(a)
	if !hasEquipped(got, sword, item.EqpHandR) || !hasEquipped(got, shield, item.EqpHandL) || !hasEquipped(got, hat, item.EqpHeadTop) {
		t.Fatalf("equip events: %v", got)
	}
	st := a.entity.status
	if st.WAtk != 25 || st.Def != before.Def+20+2 || st.Player.Weapon != 2 {
		t.Errorf("weapon ATK %d, DEF %d, weapon %d; want 25, %d, 2", st.WAtk, st.Def, st.Player.Weapon, before.Def+22)
	}
	if a.Look.Weapon != 2 || a.Look.Shield != 1 || a.Look.HeadTop != 16 {
		t.Errorf("look %+v", a.Look)
	}
	// The other player sees the new look.
	if !slices.ContainsFunc(events(b), func(e *wathenav1.Event) bool {
		return e.HasLookChange() && e.GetLookChange().GetLook().GetHeadTop() == 16
	}) {
		t.Error("the other player saw no LookChange")
	}

	// A Slayer needs a Swordman. The Novice cannot wear it.
	equip(z, a, slayer)
	z.stepAt(150)
	if f := findEvent(events(a), (*wathenav1.Event).HasEquipFailed); f == nil || f.GetEquipFailed().GetReason() != wathenav1.EquipFailure_EQUIP_FAILURE_CANNOT_EQUIP {
		t.Errorf("Slayer on a Novice: %v", f)
	}

	// Unequipping the sword takes its ATK away.
	unequip(z, a, sword)
	z.stepAt(200)
	if !slices.ContainsFunc(events(a), func(e *wathenav1.Event) bool {
		return e.HasItemUnequipped() && int(e.GetItemUnequipped().GetSlot()) == sword
	}) || a.entity.status.WAtk != 0 || a.Look.Weapon != 0 {
		t.Errorf("after unequip: weapon ATK %d, look %+v", a.entity.status.WAtk, a.Look)
	}

	// Equipped items cannot be used or sold, and leave the equipment
	// when they leave the inventory.
	if !z.deleteItem(a, hat, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_TAKEN) || a.Look.HeadTop != 0 || a.worn[hat] != nil {
		t.Errorf("a taken hat stays worn: look %+v", a.Look)
	}
}

func TestTwoHandedReplacesShield(t *testing.T) {
	z, a, _ := equipZone(t)
	a.char.Job = 1 // Swordman
	z.recalc(a)
	sword := give(t, z, a, 1101, 1)
	shield := give(t, z, a, 2101, 1)
	slayer := give(t, z, a, 1151, 1)
	equip(z, a, sword)
	equip(z, a, shield)
	z.stepAt(100)
	events(a)
	equip(z, a, slayer)
	z.stepAt(150)
	got := events(a)
	for _, slot := range []int{sword, shield} {
		if !slices.ContainsFunc(got, func(e *wathenav1.Event) bool {
			return e.HasItemUnequipped() && int(e.GetItemUnequipped().GetSlot()) == slot
		}) {
			t.Errorf("slot %d did not come off: %v", slot, got)
		}
	}
	if a.inventory.Slots[slayer].Equip != item.EqpArms || a.Look.Shield != 0 || a.Look.Weapon != 3 {
		t.Errorf("Slayer at %#x, look %+v", a.inventory.Slots[slayer].Equip, a.Look)
	}
}

func TestCardsAndSaving(t *testing.T) {
	z, a, _ := equipZone(t)
	shirt := give(t, z, a, 2302, 1)
	card := give(t, z, a, 4001, 2)
	luk := a.entity.status.Luk
	z.handle(command{player: a, insertCard: &insertCardCommand{card, shirt}})
	z.stepAt(100)
	got := events(a)
	if findEvent(got, (*wathenav1.Event).HasCardInserted) == nil || !slices.Equal(a.inventory.Slots[shirt].Cards, []uint32{4001}) || a.inventory.Slots[card].Amount != 1 {
		t.Fatalf("insert: %v, cards %v", got, a.inventory.Slots[shirt].Cards)
	}
	// The shirt has one slot, so a second card does not fit.
	z.handle(command{player: a, insertCard: &insertCardCommand{card, shirt}})
	z.stepAt(150)
	if findEvent(events(a), (*wathenav1.Event).HasInsertCardFailed) == nil {
		t.Error("a second card went into a one-slot shirt")
	}
	a.inventory.Slots[shirt].Refine = 4
	equip(z, a, shirt)
	z.stepAt(200)
	events(a)
	if a.entity.status.Luk != luk+2 {
		t.Errorf("LUK %d with a Poring Card, want %d", a.entity.status.Luk, luk+2)
	}

	// The saved character keeps the equipped shirt, its refine and its
	// card, and a character made from it wears it at once.
	c := z.snapshot(a)
	i := slices.IndexFunc(c.Items, func(it character.Item) bool { return it.ID == 2302 })
	if i < 0 || c.Items[i].Equip != item.EqpArmor || c.Items[i].Refine != 4 || !slices.Equal(c.Items[i].Cards, []uint32{4001}) {
		t.Fatalf("saved items %+v", c.Items)
	}
	inv := restoreInventory(z.Items, c)
	if s := inv.Slots[c.Items[i].Slot]; s.Equip != item.EqpArmor || s.Refine != 4 || len(s.Cards) != 1 {
		t.Errorf("restored slot %+v", s)
	}
}

func TestEquipScripts(t *testing.T) {
	z, _, b := equipZone(t)
	b.progress.BaseLevel = 90
	z.recalc(b)
	axe := give(t, z, b, 1317, 1)
	maxHP := b.entity.status.MaxHP
	equip(z, b, axe)
	z.stepAt(100)
	events(b)
	// bMaxHP,200-(40*(BaseLevel/10)): -160 at level 90.
	if b.worn[axe] == nil || b.worn[axe].item == nil {
		t.Fatal("the Academy Axe's script did not start")
	}
	if got := b.entity.status.MaxHP; got >= maxHP {
		t.Errorf("max HP %d with the Academy Axe, want less than %d", got, maxHP)
	}
	// A level up runs the computed bonus again.
	at90 := b.entity.status.MaxHP
	b.progress.BaseLevel = 30
	z.recalc(b)
	if b.entity.status.MaxHP == at90 {
		t.Error("the computed bonus did not follow the level")
	}
	unequip(z, b, axe)
	z.stepAt(150)
	if b.worn[axe] != nil {
		t.Error("the axe's context did not end")
	}
}

func TestAmmunition(t *testing.T) {
	z, a, _ := equipZone(t)
	a.char.Job = 3 // Archer
	z.recalc(a)
	bow := give(t, z, a, 1701, 1)
	arrows := give(t, z, a, 1750, 100)
	sword := give(t, z, a, 1101, 1)
	// Arrows need a bow.
	equip(z, a, arrows)
	z.stepAt(100)
	if f := findEvent(events(a), (*wathenav1.Event).HasEquipFailed); f == nil || f.GetEquipFailed().GetReason() != wathenav1.EquipFailure_EQUIP_FAILURE_WRONG_AMMUNITION {
		t.Fatalf("arrows without a bow: %v", f)
	}
	equip(z, a, bow)
	equip(z, a, arrows)
	z.stepAt(150)
	events(a)
	if a.inventory.Slots[arrows].Equip != item.EqpAmmo || a.entity.status.Player.AmmoAtk != 25 {
		t.Fatalf("arrows at %#x, ammunition ATK %d", a.inventory.Slots[arrows].Equip, a.entity.status.Player.AmmoAtk)
	}
	// ammo_unequip: a sword in place of the bow takes the arrows off.
	a.char.Job = 0
	equip(z, a, sword)
	z.stepAt(200)
	if a.inventory.Slots[arrows].Equip != 0 {
		t.Error("arrows stayed on with a sword")
	}
}

func hasEquipped(events []*wathenav1.Event, slot int, position uint32) bool {
	return slices.ContainsFunc(events, func(e *wathenav1.Event) bool {
		return e.HasItemEquipped() && int(e.GetItemEquipped().GetSlot()) == slot && e.GetItemEquipped().GetPosition() == position
	})
}

func findEvent(events []*wathenav1.Event, has func(*wathenav1.Event) bool) *wathenav1.Event {
	for _, e := range events {
		if has(e) {
			return e
		}
	}
	return nil
}
