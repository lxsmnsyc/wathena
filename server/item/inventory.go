package item

import "slices"

// Slot is one inventory slot, as rAthena's item in items_inventory. An
// empty slot has ID 0.
type Slot struct {
	ID     uint32
	Amount int
	// Serial identifies the stack or item instance. Add leaves it zero in
	// a slot it fills, for the caller to set.
	Serial Serial
	// Equip holds the positions the item is equipped in (EQP_ bits), or 0,
	// as rAthena's item.equip.
	Equip uint32
	// Refine is the refine level.
	Refine int
	// Cards are the card IDs in the item's slots, in slot order. Only
	// filled slots are listed (strategy section 6.8: slots are a list).
	Cards []uint32
	// Broken is rAthena's item.attribute of 1: skill_break_equip broke
	// the item, and it cannot be equipped until it is repaired.
	Broken bool
	// Extra holds the copy's rolls, its cards' rolls, its random options
	// and whether it is identified.
	Extra
}

// Inventory is a player's inventory, as mmo_charstatus's inventory with
// map_session_data's weight.
type Inventory struct {
	// Slots are the inventory's slots. A new inventory has Size of them,
	// and grows up to Size when it holds more. A slot from Size on, from
	// before a limit was lowered, keeps its item and takes no new one.
	Slots []Slot
	// Size is inventory_slots: slots from it on take no new items.
	Size int
	// Weight is the sum of weight times amount, in tenths.
	Weight int
}

// NewInventory is an empty inventory of a new character with rAthena's
// limits, with all of MAX_INVENTORY's slots, for tests and tools that set
// slots directly.
func NewInventory() *Inventory {
	inv := NewInventoryOf(BaseSlots, DefaultSlots)
	inv.Grow(DefaultSlots)
	return inv
}

// NewInventoryOf is an empty inventory of size usable slots, at most
// capacity, the inventory_slots limit's max, and at most MaxInventory. It
// holds size slots, so memory follows the usable slots and not the max
// (decision 0023).
func NewInventoryOf(size, capacity int) *Inventory {
	size = min(max(size, 0), max(capacity, 1), MaxInventory)
	return &Inventory{Size: size, Slots: make([]Slot, size)}
}

// Grow makes the inventory hold at least n slots, up to MaxInventory.
func (inv *Inventory) Grow(n int) {
	n = min(n, MaxInventory)
	if n > len(inv.Slots) {
		inv.Slots = append(inv.Slots, make([]Slot, n-len(inv.Slots))...)
	}
}

// Clone copies the inventory. Slices inside slots are shared, since the
// zone replaces them and never changes them in place.
func (inv *Inventory) Clone() *Inventory {
	c := *inv
	c.Slots = slices.Clone(inv.Slots)
	return &c
}

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

// Add is pc_additem for an item with no cards, refine, rolls or expiry.
// It returns the slot the item went to. maxWeight is max_weight, in
// tenths.
//
// A stackable item joins the first slot with the same item. Anything
// else takes the first empty slot below Size.
func (inv *Inventory) Add(d *Def, amount, maxWeight int) (int, AddResult) {
	return inv.AddExtra(d, amount, maxWeight, Extra{})
}

// AddExtra is Add for a copy with rolls or random options. A stackable
// item joins the first slot with the same item and the same rolls and
// options (strategy section 5.4, "Stacking"), as pc_additem compares
// cards and options. An unidentified copy of an item with hidden rolls
// never stacks, so stacking cannot tell which hidden values are equal.
// A new slot keeps a copy of x.
func (inv *Inventory) AddExtra(d *Def, amount, maxWeight int, x Extra) (int, AddResult) {
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
	if d.stacks() && (!x.Unidentified || !d.HasHidden()) {
		for n := range inv.Slots {
			s := &inv.Slots[n]
			if s.ID != d.ID || !s.Extra.Equal(&x) {
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
		// pc_search_inventory(sd, 0): the first empty slot. An inventory
		// that holds fewer slots than its size grows by one.
		i = inv.search(0)
		if i < 0 && len(inv.Slots) < inv.Size {
			inv.Slots = append(inv.Slots, Slot{})
			i = len(inv.Slots) - 1
		}
		if i < 0 || i >= inv.Size {
			return -1, AddOverItem
		}
		inv.Slots[i] = Slot{ID: d.ID, Amount: amount, Extra: x.Clone()}
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
	if slot < 0 || slot >= len(inv.Slots) {
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
	n := max(inv.Size-len(inv.Slots), 0)
	for i := range min(inv.Size, len(inv.Slots)) {
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

// Equal reports whether two slots hold the same item instance.
func (s *Slot) Equal(o *Slot) bool {
	return s.ID == o.ID && s.Amount == o.Amount && s.Serial == o.Serial && s.Equip == o.Equip &&
		s.Refine == o.Refine && slices.Equal(s.Cards, o.Cards) && s.Extra.Equal(&o.Extra)
}
