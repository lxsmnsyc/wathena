package botclient

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

func slotItem(slot, id, amount uint32) *wathenav1.InventoryItem {
	return wathenav1.InventoryItem_builder{Slot: slot, ItemId: id, Amount: amount}.Build()
}

// TestStorageAndCartState: inventory pages add to the list, a storage
// list arrives in pages, and the cart is kept apart from the storage
// window, with its weight.
func TestStorageAndCartState(t *testing.T) {
	b := offline(1)
	personal, cart := wathenav1.StorageKind_STORAGE_KIND_PERSONAL, wathenav1.StorageKind_STORAGE_KIND_CART
	bound := wathenav1.InventoryItem_builder{Slot: 2, ItemId: 909, Amount: 1, Bound: wathenav1.ItemBound_ITEM_BOUND_ACCOUNT, Restrictions: 2}.Build()
	b.apply(frame(
		wathenav1.Event_builder{Inventory: wathenav1.InventoryUpdate_builder{Items: []*wathenav1.InventoryItem{slotItem(0, 501, 10)}}.Build()}.Build(),
		wathenav1.Event_builder{Inventory: wathenav1.InventoryUpdate_builder{Items: []*wathenav1.InventoryItem{slotItem(1, 909, 20), bound}, More: true}.Build()}.Build(),
		wathenav1.Event_builder{StorageOpen: wathenav1.StorageOpen_builder{Kind: cart, Size: 100, Weight: 30, MaxWeight: 8000,
			Items: []*wathenav1.InventoryItem{slotItem(0, 909, 3)}}.Build()}.Build(),
		wathenav1.Event_builder{StorageOpen: wathenav1.StorageOpen_builder{Kind: personal, Size: 600, Name: "e2e.fixture:Vault", DisplayName: "Vault",
			Pages: 2, Items: []*wathenav1.InventoryItem{slotItem(0, 512, 1)}}.Build()}.Build(),
		wathenav1.Event_builder{StorageItems: wathenav1.StorageItems_builder{Kind: personal, Page: 1, Items: []*wathenav1.InventoryItem{slotItem(1, 512, 2)}}.Build()}.Build(),
	))
	g := b.Game()
	if g.Count(501) != 10 || g.Count(909) != 21 || g.Items[2].Bound != wathenav1.ItemBound_ITEM_BOUND_ACCOUNT || g.Items[2].Restrictions != 2 {
		t.Fatalf("inventory %+v", g.Items)
	}
	if st := g.Storage; st == nil || st.Complete() || st.Count(512) != 3 || st.DisplayName != "Vault" || st.Name != "e2e.fixture:Vault" {
		t.Fatalf("storage %+v", st)
	}
	if c := g.Cart; c == nil || c.Count(909) != 3 || c.Weight != 30 || c.MaxWeight != 8000 {
		t.Fatalf("cart %+v", c)
	}
	b.apply(frame(
		wathenav1.Event_builder{StorageItems: wathenav1.StorageItems_builder{Kind: personal, Page: 2, Items: []*wathenav1.InventoryItem{slotItem(2, 512, 4)}}.Build()}.Build(),
		wathenav1.Event_builder{StorageAdd: wathenav1.StorageAdd_builder{Kind: cart, Item: slotItem(0, 909, 5), Amount: 2, Weight: 50}.Build()}.Build(),
		wathenav1.Event_builder{StorageRemove: wathenav1.StorageRemove_builder{Kind: personal, Slot: 0, Amount: 1}.Build()}.Build(),
	))
	g = b.Game()
	if st := g.Storage; !st.Complete() || st.Count(512) != 6 || len(st.Items) != 2 {
		t.Fatalf("storage after its pages %+v", st)
	}
	if c := g.Cart; c.Count(909) != 5 || c.Weight != 50 {
		t.Fatalf("cart %+v", c)
	}
	b.apply(frame(wathenav1.Event_builder{StorageClose: wathenav1.StorageClose_builder{Kind: personal}.Build()}.Build()))
	if g := b.Game(); g.Storage != nil || g.Cart == nil {
		t.Fatalf("closing the storage: storage %+v, cart %+v", g.Storage, g.Cart)
	}
	b.apply(frame(wathenav1.Event_builder{StorageClose: wathenav1.StorageClose_builder{Kind: cart}.Build()}.Build()))
	if g := b.Game(); g.Cart != nil {
		t.Fatalf("losing the cart: %+v", g.Cart)
	}
}

// TestTradeState: the bot keeps a trade request, the open trade with
// both offers and locks, and the outcome of the close.
func TestTradeState(t *testing.T) {
	b := offline(1)
	b.apply(frame(wathenav1.Event_builder{TradeRequest: wathenav1.TradeRequest_builder{From: "Bob", BaseLevel: 50}.Build()}.Build()))
	if r := b.Game().TradeRequest; r.GetFrom() != "Bob" {
		t.Fatalf("request %v", r)
	}
	b.apply(frame(
		wathenav1.Event_builder{TradeOpen: wathenav1.TradeOpen_builder{Partner: "Bob", BaseLevel: 50}.Build()}.Build(),
		wathenav1.Event_builder{TradeOffer: wathenav1.TradeOffer_builder{Mine: true, Item: slotItem(4, 909, 2)}.Build()}.Build(),
		wathenav1.Event_builder{TradeOffer: wathenav1.TradeOffer_builder{Mine: true, Item: slotItem(4, 909, 1)}.Build()}.Build(),
		wathenav1.Event_builder{TradeOffer: wathenav1.TradeOffer_builder{Item: slotItem(0, 501, 1)}.Build()}.Build(),
		wathenav1.Event_builder{TradeOffer: wathenav1.TradeOffer_builder{Item: slotItem(0, 501, 2)}.Build()}.Build(),
		wathenav1.Event_builder{TradeOffer: wathenav1.TradeOffer_builder{Zeny: new(uint32(300))}.Build()}.Build(),
		wathenav1.Event_builder{TradeOffer: wathenav1.TradeOffer_builder{Mine: true, Zeny: new(uint32(100))}.Build()}.Build(),
		wathenav1.Event_builder{TradeLock: wathenav1.TradeLock_builder{}.Build()}.Build(),
	))
	g := b.Game()
	tr := g.Trade
	if g.TradeRequest != nil || tr == nil || tr.Partner != "Bob" || tr.Mine[4] != 3 || tr.Theirs[0].Amount != 3 || tr.Theirs[0].ID != 501 ||
		tr.MyZeny != 100 || tr.TheirZeny != 300 || tr.Locked || !tr.PartnerLocked {
		t.Fatalf("trade %+v", tr)
	}
	b.apply(frame(
		wathenav1.Event_builder{TradeLock: wathenav1.TradeLock_builder{Mine: true}.Build()}.Build(),
		wathenav1.Event_builder{TradeClose: wathenav1.TradeClose_builder{Outcome: wathenav1.TradeOutcome_TRADE_OUTCOME_COMPLETED}.Build()}.Build(),
	))
	if g := b.Game(); g.Trade != nil || g.TradeClosed != wathenav1.TradeOutcome_TRADE_OUTCOME_COMPLETED {
		t.Fatalf("after the close: %+v, %s", g.Trade, g.TradeClosed)
	}
}
