package transfer

import (
	"slices"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
)

// MaxZeny is the most zeny a wallet holds, rAthena's MAX_ZENY.
const MaxZeny = character.MaxZeny

// Box is a holder's items: slots with limits and stack rules. A Tx
// changes boxes only through these methods.
type Box interface {
	Owner() Owner
	// Slot returns slot i, or nil when there is none.
	Slot(i int) *item.Slot
	// Put adds an amount of a copy: the ID, refine, cards, broken state
	// and extra of s. It follows the box's stack rules and limits, and
	// returns the slot the amount went to. A new slot has a zero serial,
	// for the Tx to set.
	Put(s *item.Slot, amount int) (int, item.AddResult)
	// Take removes an amount from slot i. It returns false when the slot
	// holds less.
	Take(i, amount int) bool
	// Clone copies the box, for the checks Apply runs first.
	Clone() Box
}

// Wallet is a holder's zeny.
type Wallet interface {
	Owner() Owner
	Zeny() int64
	SetZeny(int64)
	// Clone copies the wallet, for the checks Apply runs first.
	Clone() Wallet
}

// InventoryBox is a character's inventory as a Box. Put is pc_additem's
// slot rules (item.Inventory.AddExtra): a stackable item joins the first
// slot with the same item, rolls and options, unless it is an
// unidentified copy with hidden rolls. Take is pc_delitem.
type InventoryBox struct {
	Inv *item.Inventory
	// Items is the item database, for weights and stack limits.
	Items *item.Table
	// MaxWeight is max_weight, in tenths.
	MaxWeight int
	Who       Owner
}

// Owner returns the inventory's owner.
func (b *InventoryBox) Owner() Owner { return b.Who }

// Slot returns slot i.
func (b *InventoryBox) Slot(i int) *item.Slot {
	if i < 0 || i >= len(b.Inv.Slots) {
		return nil
	}
	return &b.Inv.Slots[i]
}

// Put adds a copy, as pc_additem. A new slot gets the copy's refine,
// cards and broken state.
func (b *InventoryBox) Put(s *item.Slot, amount int) (int, item.AddResult) {
	d := b.Items.Get(s.ID)
	i, r := b.Inv.AddExtra(d, amount, b.MaxWeight, s.Extra)
	if r != item.AddSuccess {
		return i, r
	}
	n := &b.Inv.Slots[i]
	if n.Amount == amount {
		if s.Refine != 0 || len(s.Cards) > 0 {
			n.Refine, n.Cards = s.Refine, slices.Clone(s.Cards)
		}
		n.Broken = s.Broken
	}
	return i, r
}

// Take removes an amount from a slot, as pc_delitem.
func (b *InventoryBox) Take(i, amount int) bool { return b.Inv.Delete(b.Items, i, amount) }

// Clone copies the inventory. Slices in slots are shared, since Put and
// Take replace them and never change them in place.
func (b *InventoryBox) Clone() Box {
	c := *b
	c.Inv = b.Inv.Clone()
	return &c
}

// ListBox is a holder's slots with a slot limit and an optional weight
// limit, for the holders of later steps: storage, cart, guild storage,
// mail and vending. Its stack rules are the inventory's.
type ListBox struct {
	Slots []item.Slot
	// Size is how many slots may hold items.
	Size int
	// MaxWeight is the weight limit in tenths, or 0 for none. Weight is
	// the weight held.
	MaxWeight, Weight int
	Items             *item.Table
	Who               Owner
}

// Owner returns the box's owner.
func (b *ListBox) Owner() Owner { return b.Who }

// Slot returns slot i.
func (b *ListBox) Slot(i int) *item.Slot {
	if i < 0 || i >= len(b.Slots) {
		return nil
	}
	return &b.Slots[i]
}

// Put adds a copy with the inventory's stack rules.
func (b *ListBox) Put(s *item.Slot, amount int) (int, item.AddResult) {
	d := b.Items.Get(s.ID)
	switch {
	case d.ID == 0 || amount <= 0:
		return -1, item.AddInvalid
	case amount > item.MaxAmount:
		return -1, item.AddOverAmount
	case d.StackLimit > 0 && amount > d.StackLimit:
		return -1, item.AddStackLimit
	case b.MaxWeight > 0 && b.Weight+d.Weight*amount > b.MaxWeight:
		return -1, item.AddOverweight
	}
	if d.Stackable && !d.UniqueID && (!s.Unidentified || !d.HasHidden()) {
		for i := range b.Slots {
			o := &b.Slots[i]
			if o.ID != s.ID || o.Amount <= 0 || !o.Extra.Equal(&s.Extra) {
				continue
			}
			if amount > item.MaxAmount-o.Amount || d.StackLimit > 0 && amount > d.StackLimit-o.Amount {
				return -1, item.AddOverAmount
			}
			o.Amount += amount
			b.Weight += d.Weight * amount
			return i, item.AddSuccess
		}
	}
	// storage_additem: a full box is one whose filled slots reach its
	// size. A box over its size, after a limit was lowered, keeps its
	// items and takes no new stack.
	if b.used() >= b.Size {
		return -1, item.AddOverItem
	}
	i := slices.IndexFunc(b.Slots, func(o item.Slot) bool { return o.ID == 0 })
	if i < 0 {
		b.Slots = append(b.Slots, item.Slot{})
		i = len(b.Slots) - 1
	}
	b.Slots[i] = item.Slot{ID: s.ID, Amount: amount, Refine: s.Refine, Cards: slices.Clone(s.Cards), Extra: s.Clone()}
	b.Weight += d.Weight * amount
	return i, item.AddSuccess
}

// used is how many slots hold items.
func (b *ListBox) used() int {
	n := 0
	for i := range b.Slots {
		if b.Slots[i].ID != 0 {
			n++
		}
	}
	return n
}

// Take removes an amount from a slot.
func (b *ListBox) Take(i, amount int) bool {
	s := b.Slot(i)
	if s == nil || s.ID == 0 || amount <= 0 || s.Amount < amount {
		return false
	}
	b.Weight -= b.Items.Get(s.ID).Weight * amount
	s.Amount -= amount
	if s.Amount == 0 {
		*s = item.Slot{}
	}
	return true
}

// Clone copies the box.
func (b *ListBox) Clone() Box {
	c := *b
	c.Slots = slices.Clone(b.Slots)
	return &c
}

// ZenyWallet is a wallet over a zeny field, such as a character's.
type ZenyWallet struct {
	Who Owner
	Z   *int64
}

// Owner returns the wallet's owner.
func (w *ZenyWallet) Owner() Owner { return w.Who }

// Zeny returns the zeny held.
func (w *ZenyWallet) Zeny() int64 { return *w.Z }

// SetZeny sets the zeny held.
func (w *ZenyWallet) SetZeny(z int64) { *w.Z = z }

// Clone copies the wallet.
func (w *ZenyWallet) Clone() Wallet {
	z := *w.Z
	return &ZenyWallet{Who: w.Who, Z: &z}
}

// CurrencyWallet is a holder's balance of one currency of
// data/currencies.yml (proposal 0002, review decision 30). It moves as
// zeny does, with its own limit, and its log lines name the currency.
type CurrencyWallet struct {
	Who Owner
	// ID is the currency's ID, and Max the most the holder may hold.
	ID  uint32
	Max int64
	// Balances are the holder's balances by currency ID. A balance at 0
	// is not in the map.
	Balances map[uint32]int64
}

// Owner returns the wallet's owner.
func (w *CurrencyWallet) Owner() Owner { return w.Who }

// Zeny returns the balance.
func (w *CurrencyWallet) Zeny() int64 { return w.Balances[w.ID] }

// SetZeny sets the balance.
func (w *CurrencyWallet) SetZeny(n int64) {
	if n == 0 {
		delete(w.Balances, w.ID)
		return
	}
	w.Balances[w.ID] = n
}

// Clone copies the wallet's balance.
func (w *CurrencyWallet) Clone() Wallet {
	c := *w
	c.Balances = map[uint32]int64{w.ID: w.Balances[w.ID]}
	return &c
}

// Limit is the most the wallet holds.
func (w *CurrencyWallet) Limit() int64 { return w.Max }

// Currency is the currency's ID, for the logs.
func (w *CurrencyWallet) Currency() uint32 { return w.ID }

// limitOf is the most a wallet holds: its Limit, or MaxZeny.
func limitOf(w Wallet) int64 {
	if l, ok := w.(interface{ Limit() int64 }); ok {
		return l.Limit()
	}
	return MaxZeny
}

// currencyOf is a wallet's currency, or 0 for zeny.
func currencyOf(w Wallet) uint32 {
	if c, ok := w.(interface{ Currency() uint32 }); ok {
		return c.Currency()
	}
	return 0
}
