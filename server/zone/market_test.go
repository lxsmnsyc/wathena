package zone

import (
	"slices"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/currency"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/market"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/shop"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// marketTestItems are the storage items, with Jellopy that buying stores
// may buy.
func marketTestItems() *item.Table {
	d := &item.Data{Mode: "renewal"}
	for _, id := range []uint32{501, 909, 1201, 7001, 7002, 7003, 7004, 7005} {
		d.Items = append(d.Items, *storageTestItems().Get(id))
	}
	d.Items = append(d.Items, item.Def{ID: 7006, AegisName: "No_Mail", Type: "Etc", Weight: 1, Stackable: true, Restrictions: []string{"no_mail"}})
	for i := range d.Items {
		if d.Items[i].ID == 909 {
			d.Items[i].BuyingStore = true
			d.Items[i].Buy = 6
		}
	}
	return item.New(d)
}

// marketZone is storageZone with the market items and a clock.
func marketZone(t *testing.T, items ...character.Item) (*Zone, *Player, *Player, *commits) {
	t.Helper()
	z, a, b, cm := storageZone(t, items...)
	z.Items = marketTestItems()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	z.Clock = func() time.Time { return now.Add(time.Duration(z.now) * time.Millisecond) }
	return z, a, b, cm
}

func shopResultOf(ev []*wathenav1.Event) (wathenav1.ShopOutcome, uint32) {
	for _, e := range ev {
		if r := e.GetShopResult(); r != nil {
			return r.GetOutcome(), r.GetItemId()
		}
	}
	return 0, 0
}

// TestShopMixedCosts buys from a shop whose costs mix zeny, items and a
// currency, with a stock every player shares: each part is taken in one
// transfer, the stock goes down and joins the commit, and each lack has
// its outcome.
func TestShopMixedCosts(t *testing.T) {
	z, a, b, cm := marketZone(t, stack(0, 909, 10))
	a.record.Zeny = 1000
	s := shop.New("example.arena:Prize Desk", "Prize Desk", true, false, "example.arena", []shop.Item{
		{ID: 501, Zeny: 100, Items: []shop.CostItem{{ID: 909, Amount: 3}}, Currencies: []shop.CostCurrency{{ID: currency.Cash, Amount: 5}},
			Stock: 2, Initial: 2, Restock: mod.Restock{Daily: true}},
	})
	z.Mods = &Mods{Shops: shop.NewRegistry([]*shop.Shop{s})}
	if !z.changeCurrency(a, z.currencies().ByID(currency.Cash), 12) {
		t.Fatal("give cash")
	}
	if n := z.balance(a, z.currencies().ByID(currency.Cash)); n != 12 {
		t.Fatalf("cash %d", n)
	}
	// The account's cash is the other character's too, when it shares the
	// account: b has its own account.
	if n := z.balance(b, z.currencies().ByID(currency.Cash)); n != 0 {
		t.Fatalf("b's cash %d", n)
	}
	z.openShop(a, s, nil)
	z.stepAt(z.now + 50)
	ev := events(a)
	so := shopOpen(ev)
	if so == nil || so.GetBuysFromPlayers() || len(so.GetSellable()) != 0 || len(so.GetItems()) != 1 {
		t.Fatalf("open %v", so)
	}
	it := so.GetItems()[0]
	if it.GetPrice() != 100 || len(it.GetCostItems()) != 1 || it.GetCostItems()[0].GetAmount() != 3 ||
		len(it.GetCostCurrencies()) != 1 || it.GetCostCurrencies()[0].GetCurrency() != "cash" || !it.HasStock() || it.GetStock() != 2 {
		t.Fatalf("item %v", it)
	}
	buyNow := func(amount int) (wathenav1.ShopOutcome, uint32) {
		z.openShop(a, s, nil)
		z.buy(a, 0, []buyLine{{id: 501, amount: amount}})
		z.commitTransfers()
		z.stepAt(z.now + 50)
		return shopResultOf(events(a))
	}
	if o, id := buyNow(3); o != wathenav1.ShopOutcome_SHOP_OUTCOME_OUT_OF_STOCK || id != 501 {
		t.Errorf("over the stock: %v %d", o, id)
	}
	cm.got = nil
	if o, _ := buyNow(2); o != wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT {
		t.Fatalf("buy: %v", o)
	}
	if a.record.Zeny != 800 || inventoryCount(a.inventory, 909) != 4 || inventoryCount(a.inventory, 501) != 2 ||
		z.balance(a, z.currencies().ByID(currency.Cash)) != 2 {
		t.Errorf("after: zeny %d, jellopy %d, potions %d, cash %d", a.record.Zeny, inventoryCount(a.inventory, 909),
			inventoryCount(a.inventory, 501), z.balance(a, z.currencies().ByID(currency.Cash)))
	}
	var stock []shop.StockRow
	var cashLog bool
	for _, c := range cm.got {
		stock = append(stock, c.writes.Stock...)
		for _, l := range c.logs.Zeny {
			if l.Currency == currency.Cash && l.Amount == 10 && l.To == transfer.End(transfer.KindShop) {
				cashLog = true
			}
		}
	}
	if len(stock) != 1 || stock[0].Stock != 0 || stock[0].Shop != s.Name {
		t.Errorf("stock writes %+v", stock)
	}
	if !cashLog {
		t.Error("no cash log line")
	}
	if o, _ := buyNow(1); o != wathenav1.ShopOutcome_SHOP_OUTCOME_OUT_OF_STOCK {
		t.Errorf("sold out: %v", o)
	}
	// A day later the stock is full again.
	z.now += 24 * 3600 * 1000
	s.Lock()
	s.Items(z.wallClock(), time.UTC)
	s.Unlock()
	if o, _ := buyNow(1); o != wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_CURRENCY {
		t.Errorf("without cash: %v", o)
	}
	z.changeCurrency(a, z.currencies().ByID(currency.Cash), 100)
	z.testTake(a, 909, 4)
	if o, id := buyNow(1); o != wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_ITEMS || id != 909 {
		t.Errorf("without jellopy: %v %d", o, id)
	}
	// A shop that does not buy from players refuses a sale.
	z.openShop(a, s, nil)
	z.sell(a, 0, []sellLine{{slot: 1, amount: 1}})
	z.stepAt(z.now + 50)
	if o, _ := shopResultOf(events(a)); o != wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED {
		t.Errorf("sale: %v", o)
	}
}

// testTake takes items from a player's inventory for a test.
func (z *Zone) testTake(p *Player, id uint32, amount int) {
	for i := range p.inventory.Slots {
		if p.inventory.Slots[i].ID == id {
			z.deleteItem(p, i, min(amount, p.inventory.Slots[i].Amount), 0)
			return
		}
	}
}

// TestAccountCurrencyShared checks that two characters of one account
// share an account currency, and that a character currency is each's
// own.
func TestAccountCurrencyShared(t *testing.T) {
	z, a, b, cm := marketZone(t)
	b.record.AccountID = a.record.AccountID
	b.sess.storages, b.ownStorages = nil, nil
	tab := currency.NewTable([]*currency.Record{
		{ID: currency.Cash, Name: "cash", DisplayName: "Cash", Account: true, Max: currency.MaxAmount},
		{ID: 10000, Name: "example.arena:tokens", Mod: "example.arena", DisplayName: "Tokens", Max: 50},
	})
	z.Mods = &Mods{Currencies: tab}
	// Both players use the same registry storage for the account.
	b.sess.storages = a.sess.storages
	z.changeCurrency(a, tab.ByID(currency.Cash), 30)
	if n := z.balance(b, tab.ByID(currency.Cash)); n != 30 {
		t.Errorf("b sees %d cash", n)
	}
	z.changeCurrency(a, tab.ByID(10000), 80)
	if n := z.balance(a, tab.ByID(10000)); n != 50 {
		t.Errorf("tokens over the max: %d", n)
	}
	if n := z.balance(b, tab.ByID(10000)); n != 0 {
		t.Errorf("b's tokens: %d", n)
	}
	if !z.changeCurrency(b, tab.ByID(currency.Cash), -30) || z.changeCurrency(b, tab.ByID(currency.Cash), -1) {
		t.Error("take")
	}
	z.commitTransfers()
	var acc, char bool
	for _, c := range cm.got {
		for _, st := range c.storages {
			if st.HasCurrencies {
				acc = true
			}
		}
		for _, sv := range c.saves {
			if sv.Character.Currencies[10000] == 50 {
				char = true
			}
		}
	}
	if !acc || !char {
		t.Errorf("commits: account %v, character %v", acc, char)
	}
	z.stepAt(z.now + 50)
	if cu := eventsOf(a, (*wathenav1.Event).GetCurrencies, (*wathenav1.Event).HasCurrencies); len(cu) == 0 {
		t.Error("no CurrencyUpdate")
	}
}

// vendor gives a a cart with the items of the slots, and Vending.
func vendor(t *testing.T, z *Zone, a *Player, slots ...int) {
	t.Helper()
	a.char.Skills[skillPushCart] = 5
	a.char.Skills[skill.IDVending] = 3
	z.setCart(a, 1)
	for _, s := range slots {
		move(z, a, MoveCartPut, s, a.inventory.Slots[s].Amount)
	}
	events(a)
}

func vendingOutcomes(ev []*wathenav1.Event) []wathenav1.VendingOutcome {
	var out []wathenav1.VendingOutcome
	for _, e := range ev {
		if e.HasVendingResult() {
			out = append(out, e.GetVendingResult().GetOutcome())
		}
	}
	return out
}

func openShopFor(z *Zone, a *Player, title string, lines ...*wathenav1.VendingLine) []wathenav1.VendingOutcome {
	a.prevend = 5
	z.vendingCommand(a, wathenav1.VendingCommand_builder{Open: wathenav1.VendingOpen_builder{Title: title, Items: lines}.Build()}.Build())
	z.commitTransfers()
	z.stepAt(z.now + 50)
	return vendingOutcomes(events(a))
}

func vline(slot, amount, price uint32) *wathenav1.VendingLine {
	return wathenav1.VendingLine_builder{CartSlot: slot, Amount: amount, Price: price}.Build()
}

// TestVendingTax opens a shop and sells to another player: the buyer
// pays the price, the seller gets it less vending_tax from
// vending_tax_min, the items move from the cart, and the vend event and
// a commit with both characters, the cart and the store's row follow.
func TestVendingTax(t *testing.T) {
	z, a, b, cm := marketZone(t, stack(0, 909, 10), stack(1, 501, 5))
	vendor(t, z, a, 0, 1)
	r := rules.Base(true)
	r.Market, _ = rules.MarketOf(map[string]int{"vending_tax": 1000, "vending_tax_min": 500})
	z.Mods = &Mods{Rules: r}
	if got := openShopFor(z, a, "Cheap stuff", vline(0, 10, 50), vline(1, 5, 200)); !slices.Equal(got, []wathenav1.VendingOutcome{wathenav1.VendingOutcome_VENDING_OUTCOME_OPENED}) {
		t.Fatalf("open: %v", got)
	}
	if z.market().Get(storeKey(a, false)) == nil {
		t.Fatal("not in the market")
	}
	b.record.Zeny, a.record.Zeny = 5000, 0
	buyFrom := func(lines ...*wathenav1.VendingBuyLine) []wathenav1.VendingOutcome {
		z.vendingCommand(b, wathenav1.VendingCommand_builder{Buy: wathenav1.VendingBuy_builder{Vendor: netIDOf(b, a), Shop: a.vend.id,
			Items: lines}.Build()}.Build())
		z.commitTransfers()
		z.stepAt(z.now + 50)
		return vendingOutcomes(events(b))
	}
	bl := func(i, n uint32) *wathenav1.VendingBuyLine {
		return wathenav1.VendingBuyLine_builder{Index: i, Amount: n}.Build()
	}
	// 4 Jellopy for 200: below the tax's minimum.
	cm.got = nil
	if got := buyFrom(bl(0, 4)); len(got) != 1 || got[0] != wathenav1.VendingOutcome_VENDING_OUTCOME_BOUGHT {
		t.Fatalf("buy: %v", got)
	}
	if a.record.Zeny != 200 || b.record.Zeny != 4800 || inventoryCount(b.inventory, 909) != 4 {
		t.Errorf("after the first sale: seller %d, buyer %d, jellopy %d", a.record.Zeny, b.record.Zeny, inventoryCount(b.inventory, 909))
	}
	var stores []market.Write
	saves := 0
	for _, c := range cm.got {
		stores = append(stores, c.writes.Stores...)
		saves += len(c.saves)
	}
	if len(stores) == 0 || stores[len(stores)-1].Store == nil || stores[len(stores)-1].Store.Lines[0].Slot.Amount != 6 || saves != 2 {
		t.Errorf("commit: %d saves, stores %+v", saves, stores)
	}
	// 3 potions for 600: the tax of 10% takes 60.
	if got := buyFrom(bl(1, 3)); len(got) != 1 || got[0] != wathenav1.VendingOutcome_VENDING_OUTCOME_BOUGHT {
		t.Fatalf("second buy: %v", got)
	}
	if a.record.Zeny != 200+540 || b.record.Zeny != 4200 {
		t.Errorf("after the taxed sale: seller %d, buyer %d", a.record.Zeny, b.record.Zeny)
	}
	if sold := eventsOf(a, (*wathenav1.Event).GetVendingSold, (*wathenav1.Event).HasVendingSold); len(sold) == 0 {
		t.Error("the seller got no report")
	}
	// More than is left fails, and nothing moves.
	if got := buyFrom(bl(1, 3)); len(got) != 1 || got[0] != wathenav1.VendingOutcome_VENDING_OUTCOME_OUT_OF_STOCK {
		t.Errorf("over the stock: %v", got)
	}
	// Selling the rest closes the shop.
	buyFrom(bl(0, 6), bl(1, 2))
	if a.vend != nil || z.market().Get(storeKey(a, false)) != nil {
		t.Error("the shop is still open")
	}
	if !slices.ContainsFunc(z.scriptEvents, func(e scriptEvent) bool { return e.name == "vend" }) && z.scripts == nil {
		// Without scripts the events are dropped; the sale's event is
		// checked by the modscript and e2e tests.
		t.Log("no script runtime")
	}
}

// TestVendingOpenRules checks vending_max_value, vending_over_max and
// min_npc_vendchat_distance, and the items a shop refuses.
func TestVendingOpenRules(t *testing.T) {
	z, a, _, _ := marketZone(t, stack(0, 909, 10), stack(1, 7003, 1))
	vendor(t, z, a, 0, 1)
	r := rules.Base(true)
	r.Market, _ = rules.MarketOf(map[string]int{"vending_max_value": 100, "vending_over_max": 0})
	z.Mods = &Mods{Rules: r}
	if got := openShopFor(z, a, "x", vline(1, 1, 10)); len(got) != 1 || got[0] != wathenav1.VendingOutcome_VENDING_OUTCOME_RESTRICTED {
		t.Errorf("no_trade item: %v", got)
	}
	a.record.Zeny = transfer.MaxZeny - 50
	if got := openShopFor(z, a, "x", vline(0, 1, 1000)); len(got) != 1 || got[0] != wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED {
		t.Errorf("over the zeny cap: %v", got)
	}
	a.record.Zeny = 0
	if got := openShopFor(z, a, "x", vline(0, 1, 1000)); len(got) != 1 || got[0] != wathenav1.VendingOutcome_VENDING_OUTCOME_OPENED {
		t.Fatalf("open: %v", got)
	}
	if a.vend.lines[0].price != 100 {
		t.Errorf("price %d, want vending_max_value", a.vend.lines[0].price)
	}
	z.closeVending(a)
	z.stepAt(z.now + 50)
	events(a)
	// An NPC within min_npc_vendchat_distance stops the shop.
	z.SetNPCs([]NPC{{Name: "core:Guide", DisplayName: "Guide", X: a.entity.x + 2, Y: a.entity.y, Sprite: 112}})
	if got := openShopFor(z, a, "x", vline(0, 1, 10)); len(got) != 1 || got[0] != wathenav1.VendingOutcome_VENDING_OUTCOME_TOO_CLOSE {
		t.Errorf("near an NPC: %v", got)
	}
	if f := (vendingSkill{}).castBegin(z, a.entity, nil, 1); f == wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		t.Error("the skill starts near an NPC")
	}
}

// TestBuyingStore opens a buying store and sells to it: the items and
// zeny move in one transfer, and the store closes once it bought all.
func TestBuyingStore(t *testing.T) {
	z, a, b, _ := marketZone(t, stack(0, 909, 10))
	b.inventory.AddExtra(z.Items.Get(909), 1, maxWeight, item.Extra{})
	b.record.Zeny = 1000
	if !z.setupBuyingStore(b, 2) {
		t.Fatal("setup")
	}
	open := func(lines ...*wathenav1.BuyingStoreLine) {
		z.buyingCommand(b, wathenav1.BuyingStoreCommand_builder{Open: wathenav1.BuyingStoreOpen_builder{Title: "Buying jellopy",
			ZenyLimit: 500, Items: lines}.Build()}.Build())
		z.stepAt(z.now + 50)
	}
	open(wathenav1.BuyingStoreLine_builder{ItemId: 909, Amount: 5, Price: 20}.Build())
	if b.buying == nil {
		t.Fatalf("not open: %v", events(b))
	}
	sell := func(n uint32) []*wathenav1.Event {
		z.buyingCommand(a, wathenav1.BuyingStoreCommand_builder{Sell: wathenav1.BuyingStoreSell_builder{Buyer: netIDOf(a, b),
			Store: b.buying.id, Items: []*wathenav1.BuyingStoreSellLine{wathenav1.BuyingStoreSellLine_builder{Slot: 0, ItemId: 909, Amount: n}.Build()}}.Build()}.Build())
		z.commitTransfers()
		z.stepAt(z.now + 50)
		return events(a)
	}
	if ev := sell(6); len(ev) == 0 || inventoryCount(a.inventory, 909) != 10 {
		t.Errorf("over the count: %v", ev)
	}
	sell(3)
	if inventoryCount(a.inventory, 909) != 7 || inventoryCount(b.inventory, 909) != 4 || a.record.Zeny != 560 || b.record.Zeny != 940 {
		t.Errorf("after a sale: %d %d %d %d", inventoryCount(a.inventory, 909), inventoryCount(b.inventory, 909), a.record.Zeny, b.record.Zeny)
	}
	sell(2)
	if b.buying != nil {
		t.Error("the store is still open after buying all")
	}
	// A store needs an item the owner holds, and one buying stores buy.
	z.setupBuyingStore(b, 1)
	open(wathenav1.BuyingStoreLine_builder{ItemId: 501, Amount: 1, Price: 20}.Build())
	if b.buying != nil {
		t.Error("a store opened for an item the owner lacks")
	}
}

// TestStoreSearchRules checks searchstore_querydelay and
// searchstore_maxresults, and roll bounds.
func TestStoreSearchRules(t *testing.T) {
	z, a, b, _ := marketZone(t, stack(0, 909, 10), stack(1, 501, 5))
	vendor(t, z, a, 0, 1)
	r := rules.Base(true)
	r.Market, _ = rules.MarketOf(map[string]int{"searchstore_querydelay": 5, "searchstore_maxresults": 1})
	z.Mods = &Mods{Rules: r}
	openShopFor(z, a, "Two things", vline(0, 10, 50), vline(1, 5, 200))
	if !z.openSearch(b, 2, false) {
		t.Fatal("open")
	}
	query := func(ids ...uint32) []*wathenav1.Event {
		z.searchCommand(b, wathenav1.StoreSearchCommand_builder{Query: wathenav1.StoreSearchQuery_builder{ItemIds: ids}.Build()}.Build())
		z.stepAt(z.now + 50)
		return events(b)
	}
	ev := query(909, 501)
	res := eventsOf2(ev, (*wathenav1.Event).GetStoreSearchResults, (*wathenav1.Event).HasStoreSearchResults)
	out := eventsOf2(ev, (*wathenav1.Event).GetStoreSearchResult, (*wathenav1.Event).HasStoreSearchResult)
	if len(res) != 1 || len(res[0].GetResults()) != 1 || len(out) != 1 || out[0].GetOutcome() != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_TOO_MANY {
		t.Errorf("first search: %v %v", res, out)
	}
	out = eventsOf2(query(909), (*wathenav1.Event).GetStoreSearchResult, (*wathenav1.Event).HasStoreSearchResult)
	if len(out) != 1 || out[0].GetOutcome() != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_TOO_FAST {
		t.Errorf("too soon: %v", out)
	}
	z.now += 5000
	z.searchCommand(b, wathenav1.StoreSearchCommand_builder{Click: new(uint32(0))}.Build())
	z.stepAt(z.now + 50)
	if out := eventsOf(b, (*wathenav1.Event).GetStoreSearchResult, (*wathenav1.Event).HasStoreSearchResult); len(out) != 1 ||
		out[0].GetOutcome() != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_POSITION {
		t.Errorf("click: %v", out)
	}
	query(909)
	out = eventsOf2(query(909), (*wathenav1.Event).GetStoreSearchResult, (*wathenav1.Event).HasStoreSearchResult)
	if len(out) != 1 || out[0].GetOutcome() != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_NO_USES && out[0].GetOutcome() != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_TOO_FAST {
		t.Errorf("no uses: %v", out)
	}
	q := market.Query{Items: []uint32{909}, Rolls: []market.Bound{{Name: "x", Min: 1, Max: 2}}}
	if hits, _ := z.market().Search(q, nil); len(hits) != 0 {
		t.Errorf("a roll bound matched a copy without the roll: %v", hits)
	}
}
