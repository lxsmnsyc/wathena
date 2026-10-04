package e2e

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// Mail, vending, buying store, store search, shop and currency steps
// (phase 5 step 7).

const (
	// SkillVending is MC_VENDING, which needs Pushcart at level 3.
	SkillVending uint32 = 41
	// Tokens is the fixture's currency, which each character owns, and
	// Cash core's cash points, which the account owns.
	Tokens = "e2e.fixture:tokens" // #nosec G101 -- a currency name, not a credential.
	Cash   = "cash"
	// ExchangeName is the fixture's shop of the table form.
	ExchangeName = "Exchange"
)

// Currency is the amount of a currency the player saw last, by its full
// name.
func (p *Player) Currency(name string) int64 { return p.Game().Currencies[name] }

// GiveCurrency gives a currency with the fixture's "!currency", and
// waits until CurrencyUpdate shows the amount the fixture answered.
func (p *Player) GiveCurrency(name string, n int) int64 {
	p.t.Helper()
	local := strings.TrimPrefix(name, "e2e.fixture:")
	var got int64
	if _, err := fmt.Sscan(p.Command(fmt.Sprintf("!currency %s %d", local, n)), &got); err != nil {
		p.t.Fatalf("%s: giving %d %s: %v", p.Name, n, name, err)
	}
	p.Until(fmt.Sprintf("%d %s", got, name), func(_ View, g Game) bool { return g.Currencies[name] == got })
	return got
}

// TryBuy buys from an open shop and returns the ShopResult, with the
// item an outcome is about.
func (p *Player) TryBuy(shop *wathenav1.ShopOpen, lines map[uint32]int) *wathenav1.ShopResult {
	p.t.Helper()
	p.send("buy", func(ctx context.Context) error { return p.Bot.Buy(ctx, shop.GetNpc(), lines) })
	return p.Expect("the shop's answer", func(e *wathenav1.Event) bool { return e.HasShopResult() }).GetShopResult()
}

// OpenMailbox opens the mailbox window and returns its list.
func (p *Player) OpenMailbox() *wathenav1.MailBox {
	p.t.Helper()
	p.send("open the mailbox", p.MailOpen)
	return p.mailbox()
}

// mailbox waits for a MailBox, and fails on a MailResult.
func (p *Player) mailbox() *wathenav1.MailBox {
	p.t.Helper()
	return p.Expect("the mailbox", func(e *wathenav1.Event) bool {
		if r := e.GetMailResult(); r != nil {
			p.t.Fatalf("%s: the mailbox did not open: %s", p.Name, r.GetOutcome())
		}
		return e.HasMailBox()
	}).GetMailBox()
}

// mailResult waits for a MailResult.
func (p *Player) mailResult(what string) *wathenav1.MailResult {
	p.t.Helper()
	return p.Expect(what, func(e *wathenav1.Event) bool { return e.HasMailResult() }).GetMailResult()
}

// SendMail sends a mail with zeny and items, amounts by item ID from
// their first slots, and returns the MailResult.
func (p *Player) SendMail(to, title string, zeny uint32, items map[uint32]int) *wathenav1.MailResult {
	p.t.Helper()
	var list []botclient.Attachment
	for _, id := range slices.Sorted(maps.Keys(items)) {
		slot := p.Game().SlotOf(id)
		if slot < 0 {
			p.t.Fatalf("%s has no item %d to mail", p.Name, id)
		}
		list = append(list, botclient.Attachment{Slot: slot, Amount: items[id]})
	}
	p.send("send a mail", func(ctx context.Context) error {
		return p.MailSend(ctx, to, title, "Sent by the end-to-end tests.", zeny, list)
	})
	return p.mailResult("the mail's result")
}

// ReadMail shows a mail.
func (p *Player) ReadMail(id uint64) *wathenav1.MailRead {
	p.t.Helper()
	p.send("read a mail", func(ctx context.Context) error { return p.MailRead(ctx, id) })
	return p.Expect(fmt.Sprintf("mail %d", id), func(e *wathenav1.Event) bool {
		if r := e.GetMailResult(); r != nil && r.GetMail() == id {
			p.t.Fatalf("%s: reading mail %d: %s", p.Name, id, r.GetOutcome())
		}
		return e.GetMailRead().GetMail().GetId() == id && e.HasMailRead()
	}).GetMailRead()
}

// TakeMail takes a mail's zeny, items, or both, and returns the outcome.
func (p *Player) TakeMail(id uint64, zeny, items bool) wathenav1.MailOutcome {
	p.t.Helper()
	p.send("take a mail's attachments", func(ctx context.Context) error { return p.MailTake(ctx, id, zeny, items) })
	return p.mailResult("the take's result").GetOutcome()
}

// DeleteMail deletes a mail and returns the outcome.
func (p *Player) DeleteMail(id uint64) wathenav1.MailOutcome {
	p.t.Helper()
	p.send("delete a mail", func(ctx context.Context) error { return p.MailDelete(ctx, id) })
	return p.mailResult("the delete's result").GetOutcome()
}

// MailArrives waits for a MailNew with a title.
func (p *Player) MailArrives(title string) *wathenav1.MailNew {
	p.t.Helper()
	return p.Expect(fmt.Sprintf("a mail titled %q", title), func(e *wathenav1.Event) bool {
		return e.GetMailNew().GetTitle() == title && e.HasMailNew()
	}).GetMailNew()
}

// LearnVending raises Pushcart to level 3 and learns Vending at level
// 1, after GetCart.
func (p *Player) LearnVending() {
	p.t.Helper()
	for p.Game().Skills[SkillPushcart].GetLevel() < 3 {
		p.Learn(SkillPushcart)
	}
	p.Learn(SkillVending)
}

// cartSlot is the first cart slot that holds an item.
func (p *Player) cartSlot(id uint32) int {
	p.t.Helper()
	slot := storageSlot(p.Game().Cart, id)
	if slot < 0 {
		p.t.Fatalf("%s sees no item %d in the cart", p.Name, id)
	}
	return slot
}

// Vend is one line of OpenVending: an item from its first cart slot, how
// many and the price of one.
type Vend struct {
	Item   uint32
	Amount int
	Price  uint32
}

// OpenVending casts Vending, waits for the setup window, and opens a
// shop from the cart. It returns the VendingResult: OPENED, or why the
// shop did not open.
func (p *Player) OpenVending(title string, lines ...Vend) *wathenav1.VendingResult {
	p.t.Helper()
	self := p.Self()
	p.send("cast Vending", func(ctx context.Context) error { return p.UseSkill(ctx, SkillVending, 1, self) })
	p.Expect("the vending setup", func(e *wathenav1.Event) bool {
		if f := e.GetSkillFailed(); f != nil && f.GetSkillId() == SkillVending {
			p.t.Fatalf("%s: Vending failed: %s", p.Name, f.GetReason())
		}
		return e.HasVendingSetup()
	})
	var l []botclient.VendLine
	for _, v := range lines {
		l = append(l, botclient.VendLine{CartSlot: p.cartSlot(v.Item), Amount: v.Amount, Price: v.Price})
	}
	p.send("open the shop", func(ctx context.Context) error { return p.VendingOpen(ctx, title, l) })
	return p.Expect("the shop to open", func(e *wathenav1.Event) bool { return e.HasVendingResult() }).GetVendingResult()
}

// BrowseVending opens another player's shop and returns its list.
func (p *Player) BrowseVending(vendor string) *wathenav1.VendingList {
	p.t.Helper()
	id := p.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, vendor).ID
	p.send("browse a shop", func(ctx context.Context) error { return p.VendingBrowse(ctx, id) })
	return p.Expect(vendor+"'s shop", func(e *wathenav1.Event) bool {
		return e.GetVendingList().GetSeller() == vendor && e.HasVendingList()
	}).GetVendingList()
}

// VendIndex is the index of a shop's line that sells an item, or -1.
func VendIndex(list *wathenav1.VendingList, id uint32) int {
	for _, it := range list.GetItems() {
		if it.GetItem().GetItemId() == id {
			return int(it.GetIndex())
		}
	}
	return -1
}

// VendBuy buys amounts by item ID from a shop the player browsed, and
// returns the VendingResult.
func (p *Player) VendBuy(list *wathenav1.VendingList, items map[uint32]int) *wathenav1.VendingResult {
	p.t.Helper()
	lines := map[uint32]int{}
	for id, n := range items {
		i := VendIndex(list, id)
		if i < 0 {
			p.t.Fatalf("%s's shop does not sell item %d", list.GetSeller(), id)
		}
		lines[uint32(i)] = n // #nosec G115 -- at most 12.
	}
	p.send("buy from a shop", func(ctx context.Context) error {
		return p.VendingBuy(ctx, list.GetVendor(), list.GetShop(), lines)
	})
	return p.Expect("the purchase's result", func(e *wathenav1.Event) bool { return e.HasVendingResult() }).GetVendingResult()
}

// SaleLine waits for the line the fixture's vend or buying_store_sale
// event sends, and returns it.
func (p *Player) SaleLine(event string) string {
	p.t.Helper()
	prefix := "e2e " + event + ": "
	return p.Expect("the fixture's "+event+" line", func(e *wathenav1.Event) bool {
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM && strings.HasPrefix(c.GetText(), prefix)
	}).GetChat().GetText()
}

// OpenBuyingStore opens the setup with the fixture's "!buyingstore", and
// opens a buying store. It returns the BuyingStoreResult: OPENED, or why
// the store did not open.
func (p *Player) OpenBuyingStore(title string, zenyLimit uint32, lines ...botclient.BuyLine) *wathenav1.BuyingStoreResult {
	p.t.Helper()
	mark := p.mark
	p.MustCommand(fmt.Sprintf("!buyingstore %d", len(lines)))
	answered := p.mark
	p.mark = mark
	p.Expect("the buying store setup", (*wathenav1.Event).HasBuyingStoreSetup)
	p.mark = max(p.mark, answered)
	p.send("open the buying store", func(ctx context.Context) error { return p.BuyingStoreOpen(ctx, title, zenyLimit, lines) })
	return p.Expect("the buying store to open", func(e *wathenav1.Event) bool { return e.HasBuyingStoreResult() }).GetBuyingStoreResult()
}

// BrowseBuyingStore opens another player's buying store and returns its
// list.
func (p *Player) BrowseBuyingStore(owner string) *wathenav1.BuyingStoreList {
	p.t.Helper()
	id := p.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, owner).ID
	p.send("browse a buying store", func(ctx context.Context) error { return p.BuyingStoreBrowse(ctx, id) })
	return p.Expect(owner+"'s buying store", func(e *wathenav1.Event) bool {
		return e.GetBuyingStoreList().GetOwner() == owner && e.HasBuyingStoreList()
	}).GetBuyingStoreList()
}

// SellToStore sells amounts by item ID, from their first slots, to a
// buying store the player browsed, and returns the BuyingStoreResult.
func (p *Player) SellToStore(list *wathenav1.BuyingStoreList, items map[uint32]int) *wathenav1.BuyingStoreResult {
	p.t.Helper()
	var lines []botclient.SellLine
	for _, id := range slices.Sorted(maps.Keys(items)) {
		slot := p.Game().SlotOf(id)
		if slot < 0 {
			p.t.Fatalf("%s has no item %d to sell", p.Name, id)
		}
		lines = append(lines, botclient.SellLine{Slot: slot, ItemID: id, Amount: items[id]})
	}
	p.send("sell to a buying store", func(ctx context.Context) error {
		return p.BuyingStoreSell(ctx, list.GetBuyer(), list.GetStore(), lines)
	})
	return p.Expect("the sale's result", func(e *wathenav1.Event) bool {
		r := e.GetBuyingStoreResult()
		// The owner's own results close its store; a seller gets SOLD or
		// why the sale failed.
		return r != nil
	}).GetBuyingStoreResult()
}

// OpenSearch opens the store search with the fixture's "!search".
func (p *Player) OpenSearch(uses int, effect string) *wathenav1.StoreSearchOpen {
	p.t.Helper()
	mark := p.mark
	p.MustCommand(fmt.Sprintf("!search %d %s", uses, effect))
	answered := p.mark
	p.mark = mark
	o := p.Expect("the store search", (*wathenav1.Event).HasStoreSearchOpen).GetStoreSearchOpen()
	p.mark = max(p.mark, answered)
	return o
}

// Search runs a store search. It returns the first page of results, or
// nil and the outcome of a search that failed.
func (p *Player) Search(q botclient.SearchQuery) (*wathenav1.StoreSearchResults, wathenav1.StoreSearchOutcome) {
	p.t.Helper()
	p.send("search the stores", func(ctx context.Context) error { return p.StoreSearchQuery(ctx, q) })
	var outcome wathenav1.StoreSearchOutcome
	e := p.Expect("the search's results", func(e *wathenav1.Event) bool {
		if r := e.GetStoreSearchResult(); r != nil {
			outcome = r.GetOutcome()
			// NOTHING and TOO_MANY come before a page; the others end the
			// search.
			return outcome != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_NOTHING &&
				outcome != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_TOO_MANY
		}
		return e.HasStoreSearchResults()
	})
	return e.GetStoreSearchResults(), outcome
}

// ClickResult picks a search result and returns the StoreSearchResult.
func (p *Player) ClickResult(index uint32) *wathenav1.StoreSearchResult {
	p.t.Helper()
	p.send("click a result", func(ctx context.Context) error { return p.StoreSearchClick(ctx, index) })
	return p.Expect("the click's result", (*wathenav1.Event).HasStoreSearchResult).GetStoreSearchResult()
}
