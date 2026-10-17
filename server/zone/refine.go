package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// The refiner of procedural worlds (decision 0044), ported from
// rAthena's refinemain (npc/merchants/refine.txt): the refiner of core's
// procedural towns calls these through wa.internal.
//
//   - Items up to +9 refine, as refinemain refuses +10 and above.
//   - The price, the ore and the chance are refine.yml's REFINE_COST_NORMAL
//     for the item's type and level, as getequiprefinecost and
//     getequippercentrefinery read them.
//   - The ore and the zeny go first. Then a draw below the chance in
//     percent refines the item by 1, as successrefitem, and any other
//     draw destroys it, as failedrefitem.

// RefinerMax is the highest refine the refiner reaches: refinemain
// refuses an item at +10.
const RefinerMax = 10

// refineSlot finds the inventory slot of an equipment slot name, or -1.
func refineSlot(p *Player, slot string) int {
	bit := slotPositions[slot]
	if bit == 0 {
		return -1
	}
	for _, i := range wornSlots(p) {
		if p.inventory.Slots[i].Equip&bit != 0 {
			return i
		}
	}
	return -1
}

// RefineInfo reads an equipped item for the refiner.
func (h scriptHost) RefineInfo(player int64, slot string) (modscript.RefineInfo, bool, error) {
	z := h.z
	p, err := h.player(player)
	if err != nil {
		return modscript.RefineInfo{}, false, err
	}
	i := refineSlot(p, slot)
	if i < 0 || z.Items == nil {
		return modscript.RefineInfo{}, false, nil
	}
	s := &p.inventory.Slots[i]
	d := z.Items.Get(s.ID)
	info := modscript.RefineInfo{Item: int64(s.ID), Refine: s.Refine, Refineable: d.Refineable && (d.IsWeapon() || d.IsArmor())}
	if !info.Refineable {
		return info, true, nil
	}
	if s.Refine >= RefinerMax {
		info.Max = true
		return info, true, nil
	}
	level := d.ArmorLevel
	if d.IsWeapon() {
		level = d.WeaponLevel
	}
	if z.Status == nil {
		info.Refineable = false
		return info, true, nil
	}
	c, ok := z.Status.RefineCost(d.IsWeapon(), max(level, 1), s.Refine)
	if !ok {
		info.Refineable = false
		return info, true, nil
	}
	info.Chance, info.Price = c.Rate/100, int64(c.Price)
	if def, found := z.Items.ByName(c.Material); found {
		info.Material = int64(def.ID)
	}
	return info, true, nil
}

// Refine pays and refines an equipped item, or destroys it.
func (h scriptHost) Refine(player int64, slot string) (string, error) {
	z := h.z
	info, ok, err := h.RefineInfo(player, slot)
	switch {
	case err != nil:
		return "", err
	case !ok:
		return "empty", nil
	case !info.Refineable || info.Material == 0:
		return "not_refineable", nil
	case info.Max:
		return "max", nil
	}
	p, _ := h.player(player)
	i := refineSlot(p, slot)
	ore := -1
	for k, s := range p.inventory.Slots {
		if int64(s.ID) == info.Material && s.Amount > 0 && s.Equip == 0 {
			ore = k
			break
		}
	}
	switch {
	case ore < 0:
		return "no_material", nil
	case p.record.Zeny < info.Price:
		return "no_zeny", nil
	}
	if !z.deleteItem(p, ore, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_PAID) {
		return "no_material", nil
	}
	if !z.payZeny(p, info.Price, transfer.ActionTake, transfer.End(transfer.KindFee)) {
		return "no_zeny", nil
	}
	if int64(info.Chance) <= h.RandomN(100) {
		// failedrefitem: the item breaks and is gone.
		z.deleteItem(p, i, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
		return "failure", nil
	}
	// successrefitem: off, refined, shown again, and back on.
	position := p.inventory.Slots[i].Equip
	z.unequipItem(p, i)
	p.inventory.Slots[i].Refine++
	p.pending = append(p.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
		Slot: uint32(i), Amount: 1, // #nosec G115 -- below MaxInventory.
		Reason: wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED,
	}.Build()}.Build())
	z.sendAdd(p, i, 1)
	z.equipItem(p, i, position)
	return "success", nil
}

// Weight is the weight the player carries, rAthena's Weight.
func (h scriptHost) Weight(player int64) (int, error) {
	p, err := h.player(player)
	if err != nil {
		return 0, err
	}
	return p.inventory.Weight, nil
}

var _ modscript.CoreWorldHost = scriptHost{}
