package e2e

import (
	"context"
	"fmt"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// Storage and cart steps (phase 5 step 6).

// Items of the fixture mod after the charm, in the order of its
// data/items.yml, and the skills and job of the cart scenarios.
var (
	// Contraband is refused by the fixture's trade hook.
	Contraband = Charm + 1
	// Heirloom has no_trade, Ledger no_storage and Anchor no_drop.
	Heirloom = Charm + 2
	Ledger   = Charm + 3
	Anchor   = Charm + 4
	// Anvil weighs 2,500 tenths: a cart of 8,000 takes 3.
	Anvil = Charm + 5
)

const (
	// SkillEnlargeWeight is MC_INCCARRY, which Pushcart needs at level 5,
	// and SkillPushcart is MC_PUSHCART.
	SkillEnlargeWeight uint32 = 36
	SkillPushcart      uint32 = 39
	// MerchantJob makes new characters Merchants at base level 50 and
	// job level 40, for Options.StartJob.
	MerchantJob = "5:50:40"
	// VaultName is the fixture's extra storage.
	VaultName = "e2e.fixture:Vault"
	// CartMaxWeight is max_cart_weight, in tenths.
	CartMaxWeight = 8000
)

// Command says one of the fixture's chat commands, such as
// "!give Jellopy 5 to=storage", and returns the fixture's answer: the
// text after "e2e <name>: ".
func (p *Player) Command(text string) string {
	p.t.Helper()
	name := strings.TrimPrefix(strings.Fields(text)[0], "!")
	prefix := "e2e " + name + ": "
	p.send("chat", func(ctx context.Context) error { return p.Chat(ctx, text) })
	c := p.Expect(fmt.Sprintf("the fixture's answer to %q", text), func(e *wathenav1.Event) bool {
		if r := e.GetChatRefused(); r != nil {
			p.t.Fatalf("%s: %q was refused: %s", p.Name, text, r.GetReason())
		}
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM && strings.HasPrefix(c.GetText(), prefix)
	}).GetChat()
	return strings.TrimPrefix(c.GetText(), prefix)
}

// MustCommand says a fixture command and fails the test unless the
// answer is "true".
func (p *Player) MustCommand(text string) {
	p.t.Helper()
	if got := p.Command(text); got != "true" {
		p.t.Fatalf("%s: %q answered %q", p.Name, text, got)
	}
}

// GetCart learns Enlarge Weight Limit to level 5 and Pushcart, which the
// player needs as a Merchant, and gets a cart with the fixture's "!cart".
// It returns the cart's window.
func (p *Player) GetCart() *wathenav1.StorageOpen {
	p.t.Helper()
	for range 5 {
		p.Learn(SkillEnlargeWeight)
	}
	p.Learn(SkillPushcart)
	mark := p.mark
	p.MustCommand("!cart")
	answered := p.mark
	p.mark = mark
	o := p.Expect("the cart", func(e *wathenav1.Event) bool {
		return e.GetStorageOpen().GetKind() == wathenav1.StorageKind_STORAGE_KIND_CART
	}).GetStorageOpen()
	p.mark = max(p.mark, answered)
	return o
}

// OpenStorage opens the account's storage, or an extra storage by name,
// with the fixture's "!storage", and waits until every page of its list
// came.
func (p *Player) OpenStorage(name string) *wathenav1.StorageOpen {
	p.t.Helper()
	mark := p.mark
	p.MustCommand(strings.TrimSpace("!storage " + name))
	answered := p.mark
	p.mark = mark
	o := p.storageOpened()
	p.mark = max(p.mark, answered)
	return o
}

// OpenKafra opens a storage at the Kafra NPC: "Storage" or "Vault".
func (p *Player) OpenKafra(option string) *wathenav1.StorageOpen {
	p.t.Helper()
	p.Talk("Kafra").Pick(option)
	return p.storageOpened()
}

// storageOpened waits for a personal StorageOpen and its pages.
func (p *Player) storageOpened() *wathenav1.StorageOpen {
	p.t.Helper()
	o := p.Expect("the storage", func(e *wathenav1.Event) bool {
		if r := e.GetStorageResult(); r != nil {
			p.t.Fatalf("%s: opening the storage: %s", p.Name, r.GetOutcome())
		}
		return e.GetStorageOpen().GetKind() == wathenav1.StorageKind_STORAGE_KIND_PERSONAL
	}).GetStorageOpen()
	p.Until("every page of the storage", func(_ View, g Game) bool { return g.Storage != nil && g.Storage.Complete() })
	return o
}

// storageResult waits for the StorageResult of a refused move, or for
// the event done accepts, and returns the outcome or
// STORAGE_OUTCOME_UNSPECIFIED.
func (p *Player) storageResult(what string, done func(*wathenav1.Event) bool) wathenav1.StorageOutcome {
	p.t.Helper()
	e := p.Expect(what, func(e *wathenav1.Event) bool { return e.HasStorageResult() || done(e) })
	return e.GetStorageResult().GetOutcome()
}

// storageSlot is the first slot of a storage that holds an item, or -1.
func storageSlot(st *Storage, id uint32) int {
	slot := -1
	if st != nil {
		for s, it := range st.Items {
			if it.ID == id && (slot < 0 || s < slot) {
				slot = s
			}
		}
	}
	return slot
}

// Storage is a storage window or the cart, Item a slot, and LimitRange
// a limit's base and max, as a bot sees them.
type (
	Storage    = botclient.Storage
	Item       = botclient.Item
	LimitRange = botclient.LimitRange
)

// TryStorePut puts an amount of an item from its first inventory slot
// into the open storage, and returns the outcome: unspecified when the
// item went in.
func (p *Player) TryStorePut(id uint32, amount int) wathenav1.StorageOutcome {
	p.t.Helper()
	slot := p.Game().SlotOf(id)
	if slot < 0 {
		p.t.Fatalf("%s has no item %d to store", p.Name, id)
	}
	p.send("store", func(ctx context.Context) error { return p.StoragePut(ctx, slot, amount) })
	return p.storageResult(fmt.Sprintf("item %d in the storage", id), func(e *wathenav1.Event) bool {
		a := e.GetStorageAdd()
		return a != nil && a.GetKind() == wathenav1.StorageKind_STORAGE_KIND_PERSONAL && a.GetItem().GetItemId() == id
	})
}

// TryCartPut puts an amount of an item from its first inventory slot
// into the cart, and returns the outcome.
func (p *Player) TryCartPut(id uint32, amount int) wathenav1.StorageOutcome {
	p.t.Helper()
	slot := p.Game().SlotOf(id)
	if slot < 0 {
		p.t.Fatalf("%s has no item %d to put in the cart", p.Name, id)
	}
	p.send("put in the cart", func(ctx context.Context) error { return p.CartPut(ctx, slot, amount) })
	return p.storageResult(fmt.Sprintf("item %d in the cart", id), func(e *wathenav1.Event) bool {
		a := e.GetStorageAdd()
		return a != nil && a.GetKind() == wathenav1.StorageKind_STORAGE_KIND_CART && a.GetItem().GetItemId() == id
	})
}

// TryCartTake takes an amount of an item from its first cart slot into
// the inventory, and returns the outcome.
func (p *Player) TryCartTake(id uint32, amount int) wathenav1.StorageOutcome {
	p.t.Helper()
	slot := storageSlot(p.Game().Cart, id)
	if slot < 0 {
		p.t.Fatalf("%s sees no item %d in the cart", p.Name, id)
	}
	p.send("take from the cart", func(ctx context.Context) error { return p.CartTake(ctx, slot, amount) })
	return p.storageResult(fmt.Sprintf("item %d from the cart", id), func(e *wathenav1.Event) bool {
		a := e.GetInventoryAdd()
		return a != nil && a.GetItemId() == id
	})
}

// TryCartToStorage moves an amount of an item from its first cart slot
// into the open storage, and returns the outcome.
func (p *Player) TryCartToStorage(id uint32, amount int) wathenav1.StorageOutcome {
	p.t.Helper()
	slot := storageSlot(p.Game().Cart, id)
	if slot < 0 {
		p.t.Fatalf("%s sees no item %d in the cart", p.Name, id)
	}
	p.send("move from the cart to the storage", func(ctx context.Context) error { return p.CartToStorage(ctx, slot, amount) })
	return p.storageResult(fmt.Sprintf("item %d from the cart in the storage", id), func(e *wathenav1.Event) bool {
		a := e.GetStorageAdd()
		return a != nil && a.GetKind() == wathenav1.StorageKind_STORAGE_KIND_PERSONAL && a.GetItem().GetItemId() == id
	})
}

// TryStorageToCart moves an amount of an item from its first slot of
// the open storage into the cart, and returns the outcome.
func (p *Player) TryStorageToCart(id uint32, amount int) wathenav1.StorageOutcome {
	p.t.Helper()
	slot := storageSlot(p.Game().Storage, id)
	if slot < 0 {
		p.t.Fatalf("%s sees no item %d in the storage", p.Name, id)
	}
	p.send("move from the storage to the cart", func(ctx context.Context) error { return p.StorageToCart(ctx, slot, amount) })
	return p.storageResult(fmt.Sprintf("item %d from the storage in the cart", id), func(e *wathenav1.Event) bool {
		a := e.GetStorageAdd()
		return a != nil && a.GetKind() == wathenav1.StorageKind_STORAGE_KIND_CART && a.GetItem().GetItemId() == id
	})
}

// TryDrop drops an amount from the first slot that holds an item, and
// returns the failure, or DROP_ITEM_FAILURE_UNSPECIFIED when it dropped.
func (p *Player) TryDrop(id uint32, amount int) wathenav1.DropItemFailure {
	p.t.Helper()
	slot := p.Game().SlotOf(id)
	if slot < 0 {
		p.t.Fatalf("%s has no item %d to drop", p.Name, id)
	}
	p.send("drop", func(ctx context.Context) error { return p.DropItem(ctx, slot, amount) })
	e := p.Expect(fmt.Sprintf("item %d to be dropped", id), func(e *wathenav1.Event) bool {
		r := e.GetInventoryRemove()
		return e.HasDropItemFailed() || r != nil && int(r.GetSlot()) == slot && r.GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_DROPPED
	})
	return e.GetDropItemFailed().GetReason()
}

// SlotWhere is the first inventory slot whose item ok accepts, or -1.
func (p *Player) SlotWhere(ok func(Item) bool) int {
	best := -1
	for slot, it := range p.Game().Items {
		if ok(it) && (best < 0 || slot < best) {
			best = slot
		}
	}
	return best
}
