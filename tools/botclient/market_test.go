package botclient

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// TestCurrencyAndMailState: CurrencyUpdate replaces the list, MailBox
// keeps the mailbox, and MailRead and MailNew are kept too.
func TestCurrencyAndMailState(t *testing.T) {
	b := offline(1)
	cur := func(amounts map[string]uint64) *wathenav1.Event {
		var l []*wathenav1.CurrencyAmount
		for n, a := range amounts {
			l = append(l, wathenav1.CurrencyAmount_builder{Name: n, DisplayName: n, Amount: a}.Build())
		}
		return wathenav1.Event_builder{Currencies: wathenav1.CurrencyUpdate_builder{Currencies: l}.Build()}.Build()
	}
	b.apply(frame(cur(map[string]uint64{"cash": 5, "e2e.fixture:tokens": 3})))
	if c := b.Game().Currencies; c["cash"] != 5 || c["e2e.fixture:tokens"] != 3 {
		t.Fatalf("currencies %v", c)
	}
	b.apply(frame(cur(map[string]uint64{"e2e.fixture:tokens": 1})))
	if c := b.Game().Currencies; len(c) != 1 || c["e2e.fixture:tokens"] != 1 {
		t.Fatalf("currencies after a change %v", c)
	}

	mail := wathenav1.MailSummary_builder{Id: 7, From: "Bob", Title: "Hi", Zeny: 100, Items: 1}.Build()
	b.apply(frame(
		wathenav1.Event_builder{MailBox: wathenav1.MailBox_builder{Mails: []*wathenav1.MailSummary{mail}, Size: 100, SentToday: 2, Daily: 100}.Build()}.Build(),
		wathenav1.Event_builder{MailRead: wathenav1.MailRead_builder{Mail: mail, Body: "Body", Items: []*wathenav1.InventoryItem{slotItem(0, 909, 3)}}.Build()}.Build(),
		wathenav1.Event_builder{MailNew: wathenav1.MailNew_builder{Id: 8, From: "Postmaster", Title: "Parcel", Unread: 2}.Build()}.Build(),
	))
	g := b.Game()
	if g.Mailbox == nil || g.Mailbox.Size != 100 || g.Mailbox.SentToday != 2 || g.Mailbox.Mail(7).GetFrom() != "Bob" || g.Mailbox.Mail(8) != nil {
		t.Fatalf("mailbox %+v", g.Mailbox)
	}
	if g.MailRead.GetBody() != "Body" || len(g.MailRead.GetItems()) != 1 || g.Unread != 2 {
		t.Fatalf("read %v, unread %d", g.MailRead, g.Unread)
	}
}

// TestVendingAndBuyingState: the setup windows, the bot's own stores and
// the stores it browsed, the shop boards of the entities it sees, and
// the closes.
func TestVendingAndBuyingState(t *testing.T) {
	b := offline(1)
	b.apply(frame(
		wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{Id: 2, Kind: wathenav1.EntityKind_ENTITY_KIND_PLAYER, Name: "Bob"}.Build()}.Build(),
		wathenav1.Event_builder{EntityShop: wathenav1.EntityShop_builder{Id: 2, Title: "Cheap"}.Build()}.Build(),
		wathenav1.Event_builder{VendingSetup: wathenav1.VendingSetup_builder{MaxItems: 5}.Build()}.Build(),
	))
	if e := b.View().Entities[0]; e.ShopTitle != "Cheap" || e.ShopBuying {
		t.Fatalf("Bob's board %+v", e)
	}
	if g := b.Game(); g.VendingSetup != 5 || g.Vending != nil {
		t.Fatalf("setup %d, shop %v", g.VendingSetup, g.Vending)
	}
	line := wathenav1.VendingListItem_builder{Index: 0, Item: slotItem(3, 909, 4), Price: 10}.Build()
	b.apply(frame(
		wathenav1.Event_builder{VendingOpened: wathenav1.VendingOpened_builder{Title: "Mine", Items: []*wathenav1.VendingListItem{line}}.Build()}.Build(),
		wathenav1.Event_builder{VendingList: wathenav1.VendingList_builder{Vendor: 2, Shop: 9, Title: "Cheap", Seller: "Bob", Items: []*wathenav1.VendingListItem{line}}.Build()}.Build(),
		wathenav1.Event_builder{EntityShop: wathenav1.EntityShop_builder{Id: 2, Title: "Wanted", Buying: true}.Build()}.Build(),
	))
	g := b.Game()
	if g.VendingSetup != 0 || g.Vending.GetTitle() != "Mine" || g.VendingList.GetShop() != 9 || g.VendingList.GetSeller() != "Bob" {
		t.Fatalf("shop %v, list %v", g.Vending, g.VendingList)
	}
	if e := b.View().Entities[0]; e.ShopTitle != "Wanted" || !e.ShopBuying {
		t.Fatalf("Bob's board %+v", e)
	}
	b.apply(frame(
		wathenav1.Event_builder{VendingResult: wathenav1.VendingResult_builder{Outcome: wathenav1.VendingOutcome_VENDING_OUTCOME_CLOSED}.Build()}.Build(),
		wathenav1.Event_builder{EntityShop: wathenav1.EntityShop_builder{Id: 2, Remove: true}.Build()}.Build(),
		wathenav1.Event_builder{BuyingStoreSetup: wathenav1.BuyingStoreSetup_builder{Slots: 2}.Build()}.Build(),
	))
	g = b.Game()
	if g.Vending != nil || g.BuyingSetup != 2 || b.View().Entities[0].ShopTitle != "" {
		t.Fatalf("after the close: shop %v, buying setup %d, board %+v", g.Vending, g.BuyingSetup, b.View().Entities[0])
	}
	want := wathenav1.BuyingStoreItem_builder{ItemId: 909, Amount: 5, Price: 30}.Build()
	b.apply(frame(
		wathenav1.Event_builder{BuyingStoreOpened: wathenav1.BuyingStoreOpened_builder{Title: "Wanted", ZenyLimit: 500, Items: []*wathenav1.BuyingStoreItem{want}}.Build()}.Build(),
		wathenav1.Event_builder{BuyingStoreList: wathenav1.BuyingStoreList_builder{Buyer: 2, Store: 4, Owner: "Bob", Items: []*wathenav1.BuyingStoreItem{want}}.Build()}.Build(),
	))
	g = b.Game()
	if g.BuyingSetup != 0 || g.BuyingStore.GetZenyLimit() != 500 || g.BuyingList.GetStore() != 4 {
		t.Fatalf("store %v, list %v", g.BuyingStore, g.BuyingList)
	}
	b.apply(frame(wathenav1.Event_builder{BuyingStoreResult: wathenav1.BuyingStoreResult_builder{
		Outcome: wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_ALL_BOUGHT}.Build()}.Build()))
	if g := b.Game(); g.BuyingStore != nil {
		t.Fatalf("the store stays after ALL_BOUGHT: %v", g.BuyingStore)
	}
}

// TestStoreSearchState: the window's uses, the pages of results, a new
// search that replaces them, and the last result.
func TestStoreSearchState(t *testing.T) {
	b := offline(1)
	hit := func(i uint32) *wathenav1.StoreSearchHit {
		return wathenav1.StoreSearchHit_builder{Index: i, Owner: "Bob", Map: "map/e2e_field", X: 3, Y: 4, Item: slotItem(i, 909, 1), Price: 10}.Build()
	}
	b.apply(frame(
		wathenav1.Event_builder{StoreSearchOpen: wathenav1.StoreSearchOpen_builder{Uses: 3}.Build()}.Build(),
		wathenav1.Event_builder{StoreSearchResults: wathenav1.StoreSearchResults_builder{Results: []*wathenav1.StoreSearchHit{hit(0), hit(1)}, Uses: 2, More: true}.Build()}.Build(),
		wathenav1.Event_builder{StoreSearchResults: wathenav1.StoreSearchResults_builder{Results: []*wathenav1.StoreSearchHit{hit(2)}, Uses: 2}.Build()}.Build(),
	))
	s := b.Game().StoreSearch
	if s == nil || s.Uses != 2 || s.More || s.Open || len(s.Results) != 3 || s.Results[2].GetIndex() != 2 {
		t.Fatalf("search %+v", s)
	}
	b.apply(frame(
		wathenav1.Event_builder{StoreSearchResults: wathenav1.StoreSearchResults_builder{Results: []*wathenav1.StoreSearchHit{hit(0)}, Uses: 1}.Build()}.Build(),
		wathenav1.Event_builder{StoreSearchResult: wathenav1.StoreSearchResult_builder{Outcome: wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_POSITION,
			Map: "map/e2e_field", X: 3, Y: 4}.Build()}.Build(),
	))
	s = b.Game().StoreSearch
	if len(s.Results) != 1 || s.Uses != 1 || s.Result.GetOutcome() != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_POSITION || s.Result.GetX() != 3 {
		t.Fatalf("search after a new query %+v", s)
	}
}

// TestShopItem finds an item of a shop window with its costs and stock.
func TestShopItem(t *testing.T) {
	shop := wathenav1.ShopOpen_builder{Items: []*wathenav1.ShopItem{
		wathenav1.ShopItem_builder{ItemId: 501, Price: 50}.Build(),
		wathenav1.ShopItem_builder{ItemId: 512, Price: 10, Stock: new(uint32(3)),
			CostItems: []*wathenav1.ShopCostItem{wathenav1.ShopCostItem_builder{ItemId: 909, Amount: 2}.Build()}}.Build(),
	}}.Build()
	it := ShopItem(shop, 512)
	if it == nil || !it.HasStock() || it.GetStock() != 3 || it.GetCostItems()[0].GetItemId() != 909 || ShopItem(shop, 909) != nil {
		t.Fatalf("item %v", it)
	}
}
