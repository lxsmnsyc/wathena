package item

// Slot is one inventory slot, as rAthena's item in items_inventory. An
// empty slot has ID 0.
type Slot struct {
	ID     uint32
	Amount int
	// Serial identifies the stack or item instance. Add leaves it zero in
	// a slot it fills, for the caller to set.
	Serial Serial
}

// Inventory is a player's inventory, as mmo_charstatus's inventory with
// map_session_data's weight.
type Inventory struct {
	Slots [MaxInventory]Slot
	// Size is inventory_slots: slots from it on take no new items.
	Size int
	// Weight is the sum of weight times amount, in tenths.
	Weight int
}

// NewInventory is an empty inventory of a new character.
func NewInventory() *Inventory { return &Inventory{Size: BaseSlots} }

// AddResult is rAthena's e_additem_result.
type AddResult int

const (
	AddSuccess AddResult = iota
	// AddInvalid is an item ID of 0 or an amount below 1.
	AddInvalid
	// AddOverweight is an item the maximum weight has no room for.
	AddOverweight
	_
	// AddOverItem is a full inventory.
	AddOverItem
	// AddOverAmount is a stack that would pass MAX_AMOUNT or the item's
	// stack limit.
	AddOverAmount
	_
	// AddStackLimit is an amount over the item's stack limit.
	AddStackLimit
)

// Add is pc_additem for an item with no cards, refine, or expiry. It
// returns the slot the item went to. maxWeight is max_weight, in tenths.
//
// A stackable item joins the first slot with the same item. Anything
// else takes the first empty slot below Size.
func (inv *Inventory) Add(d *Def, amount, maxWeight int) (int, AddResult) {
	if d.ID == 0 || amount <= 0 {
		return -1, AddInvalid
	}
	if amount > MaxAmount {
		return -1, AddOverAmount
	}
	if d.StackLimit > 0 && amount > d.StackLimit {
		return -1, AddStackLimit
	}
	w := d.Weight * amount
	if inv.Weight+w > maxWeight {
		return -1, AddOverweight
	}
	i := -1
	if d.stacks() {
		for n := range inv.Slots {
			s := &inv.Slots[n]
			if s.ID != d.ID {
				continue
			}
			if amount > MaxAmount-s.Amount || d.StackLimit > 0 && amount > d.StackLimit-s.Amount {
				return -1, AddOverAmount
			}
			// A slot past the inventory's size takes no more.
			if n >= inv.Size {
				return -1, AddOverAmount
			}
			s.Amount += amount
			i = n
			break
		}
	}
	if i < 0 {
		// pc_search_inventory(sd, 0): the first empty slot.
		i = inv.search(0)
		if i < 0 || i >= inv.Size {
			return -1, AddOverItem
		}
		inv.Slots[i] = Slot{ID: d.ID, Amount: amount}
	}
	inv.Weight += w
	return i, AddSuccess
}

// search is pc_search_inventory: the first slot holding an item, or the
// first empty slot for ID 0. It is -1 when there is none.
func (inv *Inventory) search(id uint32) int {
	for i, s := range inv.Slots {
		if s.ID == id && (s.Amount > 0 || id == 0) {
			return i
		}
	}
	return -1
}

// Delete is pc_delitem: it takes an amount from a slot, and empties the
// slot when none is left. It returns false for an empty slot or an
// amount the slot does not hold.
func (inv *Inventory) Delete(t *Table, slot, amount int) bool {
	if slot < 0 || slot >= MaxInventory {
		return false
	}
	s := &inv.Slots[slot]
	if s.ID == 0 || amount <= 0 || s.Amount < amount {
		return false
	}
	inv.Weight -= t.Get(s.ID).Weight * amount
	s.Amount -= amount
	if s.Amount <= 0 {
		*s = Slot{}
	}
	return true
}

// Blank is pc_inventoryblank: the empty slots below Size.
func (inv *Inventory) Blank() int {
	n := 0
	for i := range inv.Size {
		if inv.Slots[i].ID == 0 {
			n++
		}
	}
	return n
}

// Count is the total amount of an item in the inventory.
func (inv *Inventory) Count(id uint32) int {
	n := 0
	for _, s := range inv.Slots {
		if s.ID == id {
			n += s.Amount
		}
	}
	return n
}

// PercentWeight is pc_getpercentweight: weight as a percent of the
// maximum, rounded down.
func PercentWeight(weight, maxWeight int) int {
	return weight * 100 / max(maxWeight, 1)
}

// Overweight levels, as pc_updateweightstatus's SC_WEIGHT50 and
// SC_WEIGHT90.
const (
	WeightNormal = iota
	// Weight50 stops natural HP and SP regen.
	Weight50
	// Weight90 also stops attacks and skills.
	Weight90
)

// Overweight is the level pc_updateweightstatus picks for a weight.
func (c Config) Overweight(weight, maxWeight int) int {
	p := PercentWeight(weight, maxWeight)
	switch {
	case p >= c.MajorOverweightRate:
		return Weight90
	case p >= c.NaturalHealWeightRate:
		return Weight50
	}
	return WeightNormal
}
