package zone

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// The item calls of review decision 58, area B9 (decision 0064). Each
// follows its rAthena command, script.cpp line given:
//   - give_item's options are getitem2 (7775), getnameditem (8085) and
//     rentitem (7922). count_item's are countitem2 (7224) and
//     rentalcountitem (7427).
//   - maps.drop_item is makeitem (8165) and makeitem2 (8237).
//   - items.get is getiteminfo (14761). The refine reads read
//     refine.yml as getequipisenableref (9539), getequippercentrefinery
//     (9684) and getequiprefinecost (26037). The data holds the Normal
//     cost alone, so the HD and Enriched ones raise an error.
//   - equipped:refine is successrefitem (9728) and downrefitem (9836),
//     equipped:destroy delequip (9885).
//   - get_broken_items, repair_item and repair_all are getbrokenid
//     (9422), repair (9452) and repairall (9481).
//   - run_item_script is consumeitem (23351), merge_items mergeitem2
//     (24128): the client has no merge window.

// The effects of a refine's result, clif_misceffect's REFINE_SUCCESS and
// REFINE_FAILURE, as the client shows them.
const (
	effectRefineOK   = 154
	effectRefineFail = 155
)

// wallSeconds is the server clock in Unix seconds, for rentals.
func (z *Zone) wallSeconds() int64 { return z.wallClock().Unix() }

// specExtra is a copy's extra from a script's spec, as getitem2 sets
// it: an item that is not equipment is identified, unbroken and
// unrefined (script.cpp 7853).
func (z *Zone) specExtra(d *item.Def, spec modscript.ItemSpec, roll modscript.GiveRoll, bound item.Bound) (item.Extra, int, error) {
	var x item.Extra
	switch roll.Mode {
	case modscript.RollSet:
		x.Rolls = z.newRolls(d, false, rollSource{kind: sourceScript})
		for i := range x.Rolls {
			if v, ok := roll.Values[x.Rolls[i].Name]; ok {
				x.Rolls[i].Value = d.Rolls[x.Rolls[i].Name].Clamp(v)
			}
		}
	default:
		x = z.newExtra(d, roll.Mode == modscript.RollRandom, rollSource{kind: sourceScript}, "")
	}
	x.Bound = bound
	refine := 0
	if d.IsWeapon() || d.IsArmor() || d.IsShadowGear() {
		refine = min(spec.Refine, z.maxRefine())
		x.Broken = spec.Broken
		x.Unidentified = x.Unidentified || spec.Unidentified
	}
	if spec.RentSeconds > 0 {
		x.Expires = z.wallSeconds() + spec.RentSeconds
	}
	if spec.SignedBy != 0 {
		signer, err := scriptHost{z}.player(spec.SignedBy)
		if err != nil {
			return x, 0, fmt.Errorf("signed_by: %w", err)
		}
		x.SignedBy = uint32(max(signer.record.ID, 0)) // #nosec G115 -- character IDs fit in 32 bits.
		if x.SignedBy == 0 {
			x.SignedBy = signer.charID
		}
		x.SignedName = signer.Name
	}
	return x, refine, nil
}

// maxRefine is MAX_REFINE of the zone's mode.
func (z *Zone) maxRefine() int {
	if z.Status != nil {
		return z.Status.MaxRefine()
	}
	return 20
}

// GiveItemSpec is getitem2, getnameditem and rentitem: a stackable item
// in one add, anything else and every rental one at a time. It stops at
// the first that does not fit.
func (h scriptHost) GiveItemSpec(player int64, itemID uint32, amount int, spec modscript.ItemSpec, roll modscript.GiveRoll, bound item.Bound) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	z := h.z
	d, ok := z.Items.Lookup(itemID)
	if !ok {
		return false, fmt.Errorf("there is no item %d", itemID)
	}
	for _, c := range spec.Cards {
		if _, ok := z.Items.Lookup(c); !ok {
			return false, fmt.Errorf("there is no card %d", c)
		}
	}
	step := amount
	if !d.Stackable || spec.RentSeconds > 0 {
		step = 1
	}
	for given := 0; given < amount; given += step {
		x, refine, err := z.specExtra(d, spec, roll, bound)
		if err != nil {
			return false, err
		}
		if r := z.addCopy(p, itemID, step, refine, slices.Clone(spec.Cards), x, scriptOrigin); r != item.AddSuccess {
			p.pending = append(p.pending, pickUpFailed(0, pickUpFailure(r)))
			return false, nil
		}
	}
	return true, nil
}

// CountItemSpec is countitem2 and rentalcountitem: the inventory's
// copies of the item that match.
func (h scriptHost) CountItemSpec(player int64, itemID uint32, spec modscript.CountSpec) (int, error) {
	p, err := h.player(player)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range p.inventory.Slots {
		s := &p.inventory.Slots[i]
		if s.ID != itemID || s.Amount <= 0 || (s.Expires != 0) != spec.Rentals {
			continue
		}
		if spec.Refine != nil && (s.Refine != *spec.Refine || s.Unidentified || s.Broken || !slices.Equal(s.Cards, spec.Cards)) {
			continue
		}
		n += s.Amount
	}
	return n, nil
}

// DropItem is makeitem and makeitem2: map_addflooritem with no loot
// rights, on a free cell next to the cell, in the map's zone.
func (h scriptHost) DropItem(mapName string, x, y int, itemID uint32, amount int, spec modscript.ItemSpec) error {
	z := h.z
	d, ok := z.Items.Lookup(itemID)
	if !ok {
		return fmt.Errorf("there is no item %d", itemID)
	}
	mapName = strings.TrimPrefix(mapName, "map/")
	return z.onMap(mapName, func(z *Zone) {
		if x >= z.grid.Width || y >= z.grid.Height || z.entities.items >= maxGroundItems {
			return
		}
		x0, refine, err := z.specExtra(d, spec, modscript.GiveRoll{}, item.BoundNone)
		if err != nil {
			return
		}
		lifetime := floorItemLifetime
		if z.Progress != nil {
			lifetime = z.Progress.FloorItemLifetime
		}
		pl := progression.PlaceDrop(z.freeNear(x, y, 2), x, y, z.rand)
		z.entities.add(&entity{x: pl.X, y: pl.Y, item: &groundItem{
			id: itemID, amount: amount, subX: pl.SubX, subY: pl.SubY,
			refine: refine, cards: slices.Clone(spec.Cards), extra: x0,
			from: scriptOrigin.from, expires: z.now + lifetime, fresh: true,
		}})
	})
}

// ItemInfo is getiteminfo.
func (h scriptHost) ItemInfo(itemID uint32) (modscript.ItemInfo, bool) {
	z := h.z
	if z.Items == nil {
		return modscript.ItemInfo{}, false
	}
	d, ok := z.Items.Lookup(itemID)
	if !ok {
		return modscript.ItemInfo{}, false
	}
	i := modscript.ItemInfo{ID: d.ID, Name: d.Name, AegisName: d.AegisName, Type: strings.ToLower(d.Type), Gender: strings.ToLower(d.Gender),
		Buy: d.Buy, Sell: d.Sell, Weight: d.Weight, Attack: d.Atk, Defense: d.Defense, Range: d.Range, Slots: d.Slots,
		View: d.View, MinLevel: d.MinLevel, MaxLevel: d.MaxLevel, Subtype: d.Subtype, Locations: uint32(d.Location),
		Refineable: d.Refineable, Stackable: d.Stackable}
	if d.IsWeapon() {
		i.WeaponLevel = d.WeaponLevel
	}
	if d.IsArmor() {
		i.ArmorLevel = d.ArmorLevel
	}
	if z.renewal() {
		// ITEMINFO_MAGICATTACK is 0 in pre-renewal.
		i.MagicAttack = d.Matk
	}
	return i, true
}

// refineTarget reads the item of a refine read: its data, and the copy's
// slot for an equipped copy.
func (h scriptHost) refineTarget(ref modscript.ItemRef) (*item.Def, *item.Slot, error) {
	z := h.z
	if z.Items == nil {
		return nil, nil, errors.New("the items are not loaded")
	}
	if ref.Equipped != 0 {
		p, slot, _, err := z.equippedSlot(ref.Equipped)
		if err != nil {
			return nil, nil, err
		}
		s := &p.inventory.Slots[slot]
		return z.Items.Get(s.ID), s, nil
	}
	d, ok := z.Items.Lookup(ref.Item)
	if !ok {
		return nil, nil, fmt.Errorf("there is no item %d", ref.Item)
	}
	return d, nil, nil
}

// CanRefine is getequipisenableref: the item may be refined, and a copy
// is not a rental.
func (h scriptHost) CanRefine(ref modscript.ItemRef) (bool, error) {
	d, s, err := h.refineTarget(ref)
	if err != nil {
		return false, err
	}
	if s != nil && s.Expires != 0 {
		return false, nil
	}
	return d.Refineable && (d.IsWeapon() || d.IsArmor() || d.IsShadowGear()), nil
}

// refineCost finds refine.yml's cost of refining from refine.
func (h scriptHost) refineCost(ref modscript.ItemRef, refine int) (*item.Def, int, bool, error) {
	d, s, err := h.refineTarget(ref)
	if err != nil {
		return nil, 0, false, err
	}
	if s != nil && refine < 0 {
		refine = s.Refine
	}
	z := h.z
	if z.Status == nil {
		return d, refine, false, nil
	}
	_, ok := z.Status.RefineCostOf(d, refine)
	return d, refine, ok, nil
}

var errRefineKind = errors.New("refine.yml's HD and Enriched costs are not in the server's refine data yet; only the Normal cost is")

// RefineChance is getequippercentrefinery: the chance in percent, 0
// for an item it cannot refine or a refine at or past MAX_REFINE.
func (h scriptHost) RefineChance(ref modscript.ItemRef, refine int, enriched bool) (int, error) {
	if enriched {
		return 0, errRefineKind
	}
	d, refine, ok, err := h.refineCost(ref, refine)
	if err != nil || !ok {
		return 0, err
	}
	c, _ := h.z.Status.RefineCostOf(d, refine)
	return c.Rate / 100, nil
}

// RefineCost is getequiprefinecost: the zeny and the material's ID.
func (h scriptHost) RefineCost(ref modscript.ItemRef, refine int, kind string) (zeny, material int64, ok bool, err error) {
	if kind != "normal" {
		return 0, 0, false, errRefineKind
	}
	d, refine, ok, err := h.refineCost(ref, refine)
	if err != nil || !ok {
		return 0, 0, false, err
	}
	c, _ := h.z.Status.RefineCostOf(d, refine)
	material = -1
	if def, found := h.z.Items.ByName(c.Material); found {
		material = int64(def.ID)
	}
	return int64(c.Price), material, true, nil
}

// EquippedRefine is successrefitem and downrefitem: the copy comes off,
// changes refine within 0 and MAX_REFINE, shows again and goes back on,
// and the player sees the refine's effect. successrefitem at MAX_REFINE
// does nothing.
func (h scriptHost) EquippedRefine(equipped int64, amount int) (int, error) {
	z := h.z
	p, slot, card, err := z.equippedSlot(equipped)
	if err != nil {
		return 0, err
	}
	if card >= 0 {
		return 0, errors.New("a card is not refined")
	}
	s := &p.inventory.Slots[slot]
	if amount > 0 && s.Refine >= z.maxRefine() {
		return s.Refine, nil
	}
	position := s.Equip
	z.unequipItem(p, slot)
	s = &p.inventory.Slots[slot]
	s.Refine = min(max(s.Refine+amount, 0), z.maxRefine())
	p.pending = append(p.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
		Slot: uint32(slot), Amount: 1, // #nosec G115 -- below MaxInventory.
		Reason: wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED,
	}.Build()}.Build())
	z.sendAdd(p, slot, 1)
	z.equipItem(p, slot, position)
	ef := uint32(effectRefineOK)
	if amount < 0 {
		ef = effectRefineFail
	}
	z.effects = append(z.effects, effect{p.entity, ef})
	return s.Refine, nil
}

// EquippedDestroy is delequip: the copy comes off and is deleted. With
// refineFailure it is failedrefitem (script.cpp 9802): the refine goes
// to 0, the copy comes off and is deleted, and the wearer shows
// clif_misceffect's REFINE_FAILURE.
func (h scriptHost) EquippedDestroy(equipped int64, refineFailure bool) error {
	z := h.z
	p, slot, card, err := z.equippedSlot(equipped)
	if err != nil {
		return err
	}
	if card >= 0 {
		return errors.New("a card cannot be destroyed alone")
	}
	if refineFailure {
		p.inventory.Slots[slot].Refine = 0
	}
	z.deleteItem(p, slot, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
	if refineFailure {
		z.effects = append(z.effects, effect{p.entity, effectRefineFail})
	}
	return nil
}

// brokenSlots are the slots of a player's broken copies, in inventory
// order.
func brokenSlots(p *Player) []int {
	var out []int
	for i := range p.inventory.Slots {
		if s := &p.inventory.Slots[i]; s.ID != 0 && s.Amount > 0 && s.Broken {
			out = append(out, i)
		}
	}
	return out
}

// BrokenItems is getbrokenid.
func (h scriptHost) BrokenItems(player int64) ([]int64, error) {
	p, err := h.player(player)
	if err != nil {
		return nil, err
	}
	var out []int64
	for _, i := range brokenSlots(p) {
		out = append(out, int64(p.inventory.Slots[i].ID))
	}
	return out, nil
}

// repair mends a broken copy and shows it again.
func (z *Zone) repair(p *Player, slot int) {
	s := &p.inventory.Slots[slot]
	s.Broken = false
	// The client gets the slot again, as repair's clif_equiplist.
	p.pending = append(p.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
		Slot: uint32(slot), Amount: uint32(s.Amount), // #nosec G115 -- below MaxInventory and MaxAmount.
		Reason: wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED,
	}.Build()}.Build())
	z.sendAdd(p, slot, s.Amount)
}

// RepairItem is repair: the n-th broken copy, with the refine success
// effect (clif_misceffect).
func (h scriptHost) RepairItem(player int64, n int) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	slots := brokenSlots(p)
	if n < 1 || n > len(slots) {
		return false, nil
	}
	h.z.repair(p, slots[n-1])
	h.z.effects = append(h.z.effects, effect{p.entity, effectRefineOK})
	return true, nil
}

// RepairAll is repairall: every broken copy, and the effect once.
func (h scriptHost) RepairAll(player int64) (int, error) {
	p, err := h.player(player)
	if err != nil {
		return 0, err
	}
	slots := brokenSlots(p)
	for _, i := range slots {
		h.z.repair(p, i)
	}
	if len(slots) > 0 {
		h.z.effects = append(h.z.effects, effect{p.entity, effectRefineOK})
	}
	return len(slots), nil
}

// RunItemScript is consumeitem: the item's script runs for the player,
// with no check and nothing spent.
func (h scriptHost) RunItemScript(player int64, itemID uint32) error {
	p, err := h.player(player)
	if err != nil {
		return err
	}
	z := h.z
	d, ok := z.Items.Lookup(itemID)
	if !ok {
		return fmt.Errorf("there is no item %d", itemID)
	}
	if d.Script == "" {
		return nil
	}
	if d.Unsupported != "" {
		return fmt.Errorf("the item's script needs a feature the server does not have yet: %s", d.Unsupported)
	}
	r := z.itemScripts()
	if r == nil || r.OffEntry(d.Script) {
		return nil
	}
	return r.UseItem(int64(p.charID), itemID, d.Script)
}

// MergeItems is mergeitem2 for every item: stacks of one item with the
// same copy data join the first, up to the stack limits. Rentals and
// equipped copies stay apart.
func (h scriptHost) MergeItems(player int64) (int, error) {
	p, err := h.player(player)
	if err != nil {
		return 0, err
	}
	z := h.z
	joined := 0
	inv := p.inventory
	for i := range inv.Slots {
		a := &inv.Slots[i]
		d := z.Items.Get(a.ID)
		if a.ID == 0 || a.Amount <= 0 || a.Equip != 0 || a.Expires != 0 || !d.Stackable {
			continue
		}
		for j := i + 1; j < len(inv.Slots); j++ {
			b := &inv.Slots[j]
			if b.ID != a.ID || b.Amount <= 0 || b.Equip != 0 || b.Expires != 0 || b.Refine != a.Refine ||
				!slices.Equal(b.Cards, a.Cards) || !b.Extra.Equal(&a.Extra) {
				continue
			}
			limit := item.MaxAmount
			if d.StackLimit > 0 {
				limit = d.StackLimit
			}
			if a.Amount+b.Amount > limit {
				continue
			}
			// The move takes the stack out and puts it where it stacks:
			// the first stack of the item, as pc_additem.
			tx := transfer.New(transfer.ActionMove).Move(z.box(p), j, b.Serial, b.Amount, z.box(p))
			if z.apply(p, tx, 0) == nil {
				joined++
			}
		}
	}
	return joined, nil
}

var _ modscript.ItemActionHost = scriptHost{}
