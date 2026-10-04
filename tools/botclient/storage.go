package botclient

import (
	"context"
	"maps"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Storage and the cart (phase 5 steps 5 and 6): what a bot knows of its
// open storage window and its cart from the server's events, and the
// commands it sends.

// Storage is a storage window the bot has open, or its cart.
type Storage struct {
	Kind wathenav1.StorageKind
	// Name is an extra storage's record name, empty for the account's
	// storage, the guild storage and the cart. DisplayName is the name
	// players see.
	Name, DisplayName string
	// Size is how many slots the storage has.
	Size int
	// Items are the filled slots, by slot.
	Items map[int]Item
	// Weight and MaxWeight are the cart's weight and limit, in tenths.
	Weight, MaxWeight int
	// Pages is how many StorageItems pages follow the StorageOpen, and
	// PagesSeen how many came so far.
	Pages, PagesSeen int
}

// Count is the amount of an item in the storage.
func (s *Storage) Count(id uint32) int {
	if s == nil {
		return 0
	}
	n := 0
	for _, it := range s.Items {
		if it.ID == id {
			n += it.Amount
		}
	}
	return n
}

// Complete reports whether every page of the storage's list came.
func (s *Storage) Complete() bool { return s != nil && s.PagesSeen >= s.Pages }

func (s *Storage) clone() *Storage {
	if s == nil {
		return nil
	}
	c := *s
	c.Items = maps.Clone(s.Items)
	return &c
}

// itemOf is an inventory or storage slot as the bot keeps it.
func itemOf(it *wathenav1.InventoryItem) Item {
	return Item{Slot: int(it.GetSlot()), ID: it.GetItemId(), Amount: int(it.GetAmount()), Equip: it.GetEquip(),
		Bound: it.GetBound(), Restrictions: it.GetRestrictions()}
}

// storageState is what a bot knows of its storage window and its cart.
// The caller of apply holds the bot's mu.
type storageState struct {
	window *Storage
	cart   *Storage
}

// of is the storage of a kind: the cart, or the window.
func (s *storageState) of(kind wathenav1.StorageKind) **Storage {
	if kind == wathenav1.StorageKind_STORAGE_KIND_CART {
		return &s.cart
	}
	return &s.window
}

func (s *storageState) apply(e *wathenav1.Event) {
	switch {
	case e.HasStorageOpen():
		o := e.GetStorageOpen()
		st := &Storage{Kind: o.GetKind(), Name: o.GetName(), DisplayName: o.GetDisplayName(), Size: int(o.GetSize()), Items: map[int]Item{},
			Weight: int(o.GetWeight()), MaxWeight: int(o.GetMaxWeight()), Pages: int(o.GetPages())}
		for _, it := range o.GetItems() {
			st.Items[int(it.GetSlot())] = itemOf(it)
		}
		*s.of(o.GetKind()) = st
	case e.HasStorageItems():
		p := e.GetStorageItems()
		st := *s.of(p.GetKind())
		if st == nil {
			return
		}
		for _, it := range p.GetItems() {
			st.Items[int(it.GetSlot())] = itemOf(it)
		}
		st.PagesSeen = max(st.PagesSeen, int(p.GetPage()))
	case e.HasStorageAdd():
		a := e.GetStorageAdd()
		st := *s.of(a.GetKind())
		if st == nil {
			return
		}
		// item is the slot after the change.
		it := itemOf(a.GetItem())
		st.Items[it.Slot] = it
		if a.GetKind() == wathenav1.StorageKind_STORAGE_KIND_CART {
			st.Weight = int(a.GetWeight())
		}
	case e.HasStorageRemove():
		r := e.GetStorageRemove()
		st := *s.of(r.GetKind())
		if st == nil {
			return
		}
		slot := int(r.GetSlot())
		it := st.Items[slot]
		it.Amount -= int(r.GetAmount())
		if it.Amount <= 0 {
			delete(st.Items, slot)
		} else {
			st.Items[slot] = it
		}
		if r.GetKind() == wathenav1.StorageKind_STORAGE_KIND_CART {
			st.Weight = int(r.GetWeight())
		}
	case e.HasStorageClose():
		*s.of(e.GetStorageClose().GetKind()) = nil
	}
}

func (b *Bot) storage(ctx context.Context, c wathenav1.StorageCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{Storage: c.Build()})
}

func storageMove(slot, amount int) *wathenav1.StorageMove {
	return wathenav1.StorageMove_builder{
		Slot: uint32(max(slot, 0)), Amount: uint32(max(amount, 0)), // #nosec G115 -- slots and amounts.
	}.Build()
}

// StoragePut moves an amount from an inventory slot into the open
// storage.
func (b *Bot) StoragePut(ctx context.Context, slot, amount int) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{Put: storageMove(slot, amount)})
}

// StorageTake moves an amount from a storage slot into the inventory.
func (b *Bot) StorageTake(ctx context.Context, slot, amount int) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{Take: storageMove(slot, amount)})
}

// CartPut moves an amount from an inventory slot into the cart.
func (b *Bot) CartPut(ctx context.Context, slot, amount int) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{CartPut: storageMove(slot, amount)})
}

// CartTake moves an amount from a cart slot into the inventory.
func (b *Bot) CartTake(ctx context.Context, slot, amount int) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{CartTake: storageMove(slot, amount)})
}

// CartToStorage moves an amount from a cart slot into the open storage.
func (b *Bot) CartToStorage(ctx context.Context, slot, amount int) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{CartToStorage: storageMove(slot, amount)})
}

// StorageToCart moves an amount from a slot of the open storage into the
// cart.
func (b *Bot) StorageToCart(ctx context.Context, slot, amount int) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{StorageToCart: storageMove(slot, amount)})
}

// StorageClose closes the storage window.
func (b *Bot) StorageClose(ctx context.Context) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{Close: new(true)})
}

// StorageLog asks for the guild storage log.
func (b *Bot) StorageLog(ctx context.Context) error {
	return b.storage(ctx, wathenav1.StorageCommand_builder{Log: new(true)})
}

// EquipItem puts on the item in an inventory slot, at every position it
// takes.
func (b *Bot) EquipItem(ctx context.Context, slot int) error {
	return b.command(ctx, wathenav1.Command_builder{EquipItem: wathenav1.EquipItem_builder{
		Slot: uint32(max(slot, 0)), // #nosec G115 -- inventory slots.
	}.Build()})
}
