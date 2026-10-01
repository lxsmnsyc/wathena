package zone

import (
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/progression"
)

// Inventory: each player has rAthena's inventory of slots, with weight
// and stacking from server/item. Players use healing items and drop items
// on the ground by slot. The player gets its whole inventory in the first
// frame, then each change with its slot.

// floorItemLifetime is flooritem_lifetime, for zones without progression
// tables.
var floorItemLifetime = progression.DefaultConfig.FloorItemLifetime

// StartItem is an item every player starts with, for development.
type StartItem struct {
	ID     uint32
	Amount int
}

type dropCommand struct {
	slot, amount int
}

// used is an item use that viewers see, as clif_useitemack sends to the
// area.
type used struct {
	e  *entity
	id uint32
}

// itemConfig is the item settings: the table's, or renewal's defaults.
func (z *Zone) itemConfig() item.Config {
	if z.Items != nil {
		return z.Items.Config
	}
	return item.DefaultConfig(true)
}

// inventoryList is the full inventory, for the first frame.
func inventoryList(p *Player) *wathenav1.Event {
	var slots []*wathenav1.InventoryItem
	for i, s := range p.inventory.Slots {
		if s.ID == 0 {
			continue
		}
		slots = append(slots, wathenav1.InventoryItem_builder{
			Slot:   uint32(i), // #nosec G115 -- below MaxInventory.
			ItemId: s.ID,
			Amount: uint32(s.Amount), // #nosec G115 -- at most MaxAmount.
		}.Build())
	}
	return wathenav1.Event_builder{Inventory: wathenav1.InventoryUpdate_builder{
		Items: slots,
		Size:  uint32(p.inventory.Size), // #nosec G115 -- at most MaxInventory.
	}.Build()}.Build()
}

// addItem is pc_additem: it puts items in a player's inventory and sends
// the slot and the new weight, or returns why it could not.
func (z *Zone) addItem(p *Player, id uint32, amount int) item.AddResult {
	slot, res := p.inventory.Add(z.Items.Get(id), amount, p.maxWeight)
	if res != item.AddSuccess {
		return res
	}
	// A new stack or instance gets its own serial. Items picked up from
	// the ground get a new one too, since ground items are not saved.
	if s := &p.inventory.Slots[slot]; s.Serial.IsZero() {
		s.Serial = item.NewSerial()
	}
	p.pending = append(p.pending, wathenav1.Event_builder{InventoryAdd: wathenav1.InventoryAdd_builder{
		ItemId: id,
		Amount: uint32(amount), // #nosec G115 -- at most MaxAmount.
		Slot:   uint32(slot),   // #nosec G115 -- below MaxInventory.
	}.Build()}.Build())
	z.updateWeight(p)
	return res
}

// deleteItem is pc_delitem: it takes an amount from a slot and sends the
// removal and the new weight.
func (z *Zone) deleteItem(p *Player, slot, amount int, reason wathenav1.InventoryRemoveReason) bool {
	if !p.inventory.Delete(z.Items, slot, amount) {
		return false
	}
	p.pending = append(p.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
		Slot:   uint32(slot),   // #nosec G115 -- below MaxInventory.
		Amount: uint32(amount), // #nosec G115 -- at most MaxAmount.
		Reason: reason,
	}.Build()}.Build())
	z.updateWeight(p)
	return true
}

// updateWeight sends the weight, as clif_updatestatus(SP_WEIGHT), and is
// pc_updateweightstatus: the player becomes overweight from
// natural_heal_weight_rate, and heavily overweight from
// major_overweight_rate. SC_WEIGHT90 has StopAttacking, so the player's
// attack stops when it starts.
func (z *Zone) updateWeight(p *Player) {
	if p.inventory == nil {
		return
	}
	level := z.itemConfig().Overweight(p.inventory.Weight, p.maxWeight)
	if level != p.overweight && level == item.Weight90 {
		p.entity.target = nil
	}
	p.overweight = level
	if s := z.battleStats(p); s != nil {
		p.pending = append(p.pending, s)
	}
}

// pickUpFailure is the reason clif_additem gives for a pc_additem result.
func pickUpFailure(r item.AddResult) wathenav1.PickUpFailure {
	switch r {
	case item.AddOverweight:
		return wathenav1.PickUpFailure_PICK_UP_FAILURE_OVERWEIGHT
	case item.AddOverItem:
		return wathenav1.PickUpFailure_PICK_UP_FAILURE_INVENTORY_FULL
	case item.AddOverAmount, item.AddStackLimit:
		return wathenav1.PickUpFailure_PICK_UP_FAILURE_TOO_MANY
	}
	return wathenav1.PickUpFailure_PICK_UP_FAILURE_UNSPECIFIED
}

func useItemFailed(slot int, reason wathenav1.UseItemFailure) *wathenav1.Event {
	return wathenav1.Event_builder{UseItemFailed: wathenav1.UseItemFailed_builder{
		Slot:   uint32(slot), // #nosec G115 -- below MaxInventory.
		Reason: reason,
	}.Build()}.Build()
}

func dropItemFailed(slot int, reason wathenav1.DropItemFailure) *wathenav1.Event {
	return wathenav1.Event_builder{DropItemFailed: wathenav1.DropItemFailed_builder{
		Slot:   uint32(slot), // #nosec G115 -- below MaxInventory.
		Reason: reason,
	}.Build()}.Build()
}

// canUse is the part of pc_isUseitem that applies here. It returns the
// reason the item cannot be used, or UNSPECIFIED when it can. Map flags,
// sitting, storage, item groups and mercenary items do not exist yet.
func (z *Zone) canUse(p *Player, d *item.Def) wathenav1.UseItemFailure {
	if !d.Consumable() {
		return wathenav1.UseItemFailure_USE_ITEM_FAILURE_NOT_USABLE
	}
	// Prevent mass item usage: canuseitem_tick.
	if z.now < p.canUseItem {
		return wathenav1.UseItemFailure_USE_ITEM_FAILURE_TOO_SOON
	}
	if d.Type == "Cash" {
		cfg := z.itemConfig()
		if item.PercentWeight(p.inventory.Weight, p.maxWeight) >= cfg.OpenBoxWeightRate {
			return wathenav1.UseItemFailure_USE_ITEM_FAILURE_OVERWEIGHT
		}
		// Official servers want more than 10 empty slots.
		if p.inventory.Blank() <= 10 {
			return wathenav1.UseItemFailure_USE_ITEM_FAILURE_INVENTORY_FULL
		}
	}
	male := p.char.Male
	if d.Gender == "Male" && !male || d.Gender == "Female" && male {
		return wathenav1.UseItemFailure_USE_ITEM_FAILURE_CANNOT_USE
	}
	level := 1
	if z.Progress != nil {
		level = p.progress.BaseLevel
	}
	if d.MinLevel > 0 && level < d.MinLevel || d.MaxLevel > 0 && level > d.MaxLevel {
		return wathenav1.UseItemFailure_USE_ITEM_FAILURE_CANNOT_USE
	}
	// pc_job_can_use_item is not ported, so items that leave jobs out
	// are refused.
	if d.JobRestricted {
		return wathenav1.UseItemFailure_USE_ITEM_FAILURE_CANNOT_USE
	}
	return wathenav1.UseItemFailure_USE_ITEM_FAILURE_UNSPECIFIED
}

// itemDelay is pc_itemcd_check: an item with a reuse delay starts it and
// may not be used again before it ends. A delay with a status belongs to
// the status, which every item naming it shares. It returns false while
// the delay runs.
func (z *Zone) itemDelay(p *Player, d *item.Def) bool {
	if d.DelayStatus != "" {
		if z.now < p.statusDelays[d.DelayStatus] {
			return false
		}
		p.statusDelays[d.DelayStatus] = z.now + d.DelayMs
		return true
	}
	if z.now < p.itemDelays[d.ID] {
		return false
	}
	p.itemDelays[d.ID] = z.now + d.DelayMs
	// pc_itemcd_add clears the delays that ended.
	for id, until := range p.itemDelays {
		if until <= z.now {
			delete(p.itemDelays, id)
		}
	}
	return true
}

// useItem is clif_parse_UseItem and pc_useitem for a slot. The item's
// script runs in Lua (server/modscript). An item whose script needs
// something the server cannot do yet is refused before anything is
// spent, and the player is told what is missing.
func (z *Zone) useItem(p *Player, slot int) {
	if slot < 0 || slot >= item.MaxInventory {
		return
	}
	s := p.inventory.Slots[slot]
	if s.ID == 0 || s.Amount <= 0 {
		p.pending = append(p.pending, useItemFailed(slot, wathenav1.UseItemFailure_USE_ITEM_FAILURE_EMPTY_SLOT))
		return
	}
	d := z.Items.Get(s.ID)
	if r := z.canUse(p, d); r != wathenav1.UseItemFailure_USE_ITEM_FAILURE_UNSPECIFIED {
		p.pending = append(p.pending, useItemFailed(slot, r))
		return
	}
	if d.Script == "" || d.Unsupported != "" {
		p.pending = append(p.pending, useItemFailed(slot, wathenav1.UseItemFailure_USE_ITEM_FAILURE_NOT_SUPPORTED))
		if d.Unsupported != "" {
			p.pending = append(p.pending, systemMessage("This item needs a feature the server does not have yet: "+d.Unsupported+"."))
		}
		return
	}
	r := z.itemScripts()
	if r == nil {
		p.pending = append(p.pending, useItemFailed(slot, wathenav1.UseItemFailure_USE_ITEM_FAILURE_NOT_SUPPORTED))
		return
	}
	if d.DelayMs > 0 && !z.itemDelay(p, d) {
		p.pending = append(p.pending, useItemFailed(slot, wathenav1.UseItemFailure_USE_ITEM_FAILURE_COOLDOWN))
		return
	}
	// clif_useitemack goes to the area. The item is spent before its
	// script runs, unless its NoConsume flag keeps it. A DelayConsume
	// item, such as a Fly Wing, is spent once its script has done its
	// work, as rAthena spends it when the skill it casts succeeds.
	z.used = append(z.used, used{p.entity, s.ID})
	delayed := strings.EqualFold(d.Type, "DelayConsume")
	if !d.NoConsume && !delayed {
		z.deleteItem(p, slot, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
	}
	p.canUseItem = z.now + z.itemConfig().ItemUseInterval
	err := r.UseItem(int64(p.charID), s.ID, d.Script)
	if err != nil && z.Mods.Log != nil {
		z.Mods.Log.Warn("item script failed", "map", z.Map, "item", d.AegisName, "script", d.Script, "err", err)
	}
	if delayed && !d.NoConsume && err == nil && p.inventory.Slots[slot].ID == s.ID {
		z.deleteItem(p, slot, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
	}
	z.finishWarp(p)
}

// heal runs a heal: itemheal through pc_itemheal, or percentheal
// through pc_percentheal, then status_heal. Amounts that are ranges are
// drawn as rand, HP before SP. The player sees the HP it gained as a
// heal number.
func (z *Zone) heal(p *Player, h *item.Heal) {
	e := p.entity
	st := e.status
	hp, sp := h.Roll(z.rand)
	if h.Command == "percentheal" {
		hp, sp = item.PercentHeal(hp, sp, st.MaxHP, st.MaxSP)
	} else {
		hp, sp = item.ItemHeal(hp, sp, st.Vit, st.Int, p.char.Skills)
	}
	gotHP, gotSP := z.statusHeal(p, int64(hp), int64(sp))
	if gotHP > 0 {
		id := p.netID(e)
		p.pending = append(p.pending, wathenav1.Event_builder{Damage: wathenav1.Damage_builder{
			Source: id, Target: id, Amount: gotHP, Hits: 1,
			Kind:             wathenav1.DamageKind_DAMAGE_KIND_HEAL,
			TargetHpPermille: e.hpPermille(),
		}.Build()}.Build())
	}
	if gotSP > 0 {
		if s := z.battleStats(p); s != nil {
			p.pending = append(p.pending, s)
		}
	}
}

// statusHeal is status_heal for a player: nothing for the dead, and at
// most the missing HP and SP. It returns what it healed.
func (z *Zone) statusHeal(p *Player, hp, sp int64) (int64, int64) {
	e := p.entity
	if e.dead || e.hp <= 0 {
		return 0, 0
	}
	hp = item.Cap(hp, e.hp, int64(e.status.MaxHP))
	sp = item.Cap(sp, p.sp, int64(e.status.MaxSP))
	if hp > 0 {
		e.hp += hp
		e.hpChanged = true
	}
	p.sp += sp
	return hp, sp
}

// dropItem is clif_parse_DropItem and pc_dropitem: the amount leaves the
// slot and lies at a free cell near the player, with no pick-up rights,
// as map_addflooritem with no looters.
func (z *Zone) dropItem(p *Player, slot, amount int) {
	fail := func(r wathenav1.DropItemFailure) {
		p.pending = append(p.pending, dropItemFailed(slot, r))
	}
	if slot < 0 || slot >= item.MaxInventory {
		return
	}
	s := p.inventory.Slots[slot]
	switch {
	case s.ID == 0 || s.Amount <= 0:
		fail(wathenav1.DropItemFailure_DROP_ITEM_FAILURE_EMPTY_SLOT)
		return
	case amount <= 0 || s.Amount < amount:
		fail(wathenav1.DropItemFailure_DROP_ITEM_FAILURE_BAD_AMOUNT)
		return
	case z.Items.Get(s.ID).NoDrop:
		// pc_candrop: the item's trade restriction.
		fail(wathenav1.DropItemFailure_DROP_ITEM_FAILURE_CANNOT_DROP)
		return
	}
	lifetime := floorItemLifetime
	if z.Progress != nil {
		lifetime = z.Progress.FloorItemLifetime
	}
	e := p.entity
	pl := progression.PlaceDrop(z.cellFree, e.x, e.y, z.rand)
	z.entities.add(&entity{x: pl.X, y: pl.Y, item: &groundItem{
		id: s.ID, amount: amount, subX: pl.SubX, subY: pl.SubY,
		expires: z.now + lifetime,
		fresh:   true,
	}})
	z.deleteItem(p, slot, amount, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_DROPPED)
}

// usedEvents are the item uses a player sees this tick.
func (z *Zone) usedEvents(p *Player) []*wathenav1.Event {
	var events []*wathenav1.Event
	for _, u := range z.used {
		if id, ok := p.netIDs.get(u.e); ok {
			events = append(events, wathenav1.Event_builder{ItemUsed: wathenav1.ItemUsed_builder{Source: id, ItemId: u.id}.Build()}.Build())
		}
	}
	return events
}
