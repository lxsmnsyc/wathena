package botclient

import (
	"context"
	"maps"
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Mail, vending, buying stores, the store search and currencies (phase 5
// step 7): what a bot knows of them from the server's events, and the
// commands it sends.

// Mailbox is the bot's mailbox window, from the last MailBox.
type Mailbox struct {
	// Mails are newest first.
	Mails []*wathenav1.MailSummary
	// Size is the mailbox limit. SentToday and Daily are the mails sent
	// today and the most per day, or 0 for no limit.
	Size, SentToday, Daily int
}

// Mail returns a mail by ID, or nil.
func (m *Mailbox) Mail(id uint64) *wathenav1.MailSummary {
	if m == nil {
		return nil
	}
	for _, s := range m.Mails {
		if s.GetId() == id {
			return s
		}
	}
	return nil
}

// StoreSearch is the bot's store search window.
type StoreSearch struct {
	// Uses is how many searches are left. Open is set when a click opens
	// the store; it shows its place otherwise.
	Uses int
	Open bool
	// Results are the results of the last search that came so far, by
	// their index. More is set when another page waits.
	Results []*wathenav1.StoreSearchHit
	More    bool
	// Result is the last StoreSearchResult, or nil.
	Result *wathenav1.StoreSearchResult
}

// marketState is what a bot knows of its mail, stores, store search and
// currencies. The caller of apply holds the bot's mu.
type marketState struct {
	currencies map[string]int64
	mailbox    *Mailbox
	mailRead   *wathenav1.MailRead
	unread     int
	// vendingSetup and buyingSetup are the setup windows' sizes, 0 when
	// they are closed.
	vendingSetup, buyingSetup int
	vending                   *wathenav1.VendingOpened
	vendingList               *wathenav1.VendingList
	buying                    *wathenav1.BuyingStoreOpened
	buyingList                *wathenav1.BuyingStoreList
	search                    *StoreSearch
}

func (s *marketState) apply(e *wathenav1.Event) {
	switch {
	case e.HasCurrencies():
		s.currencies = map[string]int64{}
		for _, c := range e.GetCurrencies().GetCurrencies() {
			s.currencies[c.GetName()] = int64(c.GetAmount()) // #nosec G115 -- at most MAX_ZENY.
		}
	case e.HasMailBox():
		b := e.GetMailBox()
		s.mailbox = &Mailbox{Mails: b.GetMails(), Size: int(b.GetSize()), SentToday: int(b.GetSentToday()), Daily: int(b.GetDaily())}
	case e.HasMailRead():
		s.mailRead = e.GetMailRead()
	case e.HasMailNew():
		s.unread = int(e.GetMailNew().GetUnread())
	case e.HasVendingSetup():
		s.vendingSetup = int(e.GetVendingSetup().GetMaxItems())
	case e.HasVendingOpened():
		s.vendingSetup, s.vending = 0, e.GetVendingOpened()
	case e.HasVendingList():
		s.vendingList = e.GetVendingList()
	case e.HasVendingResult():
		switch e.GetVendingResult().GetOutcome() {
		case wathenav1.VendingOutcome_VENDING_OUTCOME_CLOSED:
			s.vending = nil
		case wathenav1.VendingOutcome_VENDING_OUTCOME_FAILED, wathenav1.VendingOutcome_VENDING_OUTCOME_RESTRICTED,
			wathenav1.VendingOutcome_VENDING_OUTCOME_TOO_CLOSE:
			// A shop that did not open closes the setup window.
			if s.vending == nil {
				s.vendingSetup = 0
			}
		}
	case e.HasBuyingStoreSetup():
		s.buyingSetup = int(e.GetBuyingStoreSetup().GetSlots())
	case e.HasBuyingStoreOpened():
		s.buyingSetup, s.buying = 0, e.GetBuyingStoreOpened()
	case e.HasBuyingStoreList():
		s.buyingList = e.GetBuyingStoreList()
	case e.HasBuyingStoreResult():
		switch e.GetBuyingStoreResult().GetOutcome() {
		case wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_CLOSED, wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_ALL_BOUGHT,
			wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_LIMIT_REACHED:
			s.buying = nil
		case wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_FAILED, wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_OVERWEIGHT:
			if s.buying == nil {
				s.buyingSetup = 0
			}
		}
	case e.HasStoreSearchOpen():
		o := e.GetStoreSearchOpen()
		s.search = &StoreSearch{Uses: int(o.GetUses()), Open: o.GetOpen()}
	case e.HasStoreSearchResults():
		if s.search == nil {
			return
		}
		r := e.GetStoreSearchResults()
		// The first page of a search starts at index 0 and replaces the
		// results of the last search.
		if l := r.GetResults(); len(l) == 0 || l[0].GetIndex() == 0 {
			s.search.Results = nil
		}
		s.search.Results = append(s.search.Results, r.GetResults()...)
		s.search.Uses, s.search.More = int(r.GetUses()), r.GetMore()
	case e.HasStoreSearchResult():
		if s.search != nil {
			s.search.Result = e.GetStoreSearchResult()
		}
	}
}

// Market is what a bot knows of its mail, its stores, the stores it
// browsed, its store search and its currencies.
type Market struct {
	// Currencies are the amounts above 0, by the currency's full name.
	Currencies map[string]int64
	// Mailbox is the last mailbox list, or nil. MailRead is the last mail
	// shown. Unread is the count of the last MailNew.
	Mailbox  *Mailbox
	MailRead *wathenav1.MailRead
	Unread   int
	// VendingSetup is the setup window's most lines, 0 when it is closed.
	// Vending is the bot's own open shop, or nil. VendingList is the last
	// shop the bot browsed.
	VendingSetup int
	Vending      *wathenav1.VendingOpened
	VendingList  *wathenav1.VendingList
	// BuyingSetup is the setup window's slots, 0 when it is closed.
	// BuyingStore is the bot's own open store, or nil. BuyingList is the
	// last store the bot browsed.
	BuyingSetup int
	BuyingStore *wathenav1.BuyingStoreOpened
	BuyingList  *wathenav1.BuyingStoreList
	// StoreSearch is the store search window, or nil.
	StoreSearch *StoreSearch
}

func (s *marketState) clone() Market {
	m := Market{Currencies: maps.Clone(s.currencies), MailRead: s.mailRead, Unread: s.unread, VendingSetup: s.vendingSetup,
		Vending: s.vending, VendingList: s.vendingList, BuyingSetup: s.buyingSetup, BuyingStore: s.buying, BuyingList: s.buyingList}
	if s.mailbox != nil {
		b := *s.mailbox
		b.Mails = slices.Clone(b.Mails)
		m.Mailbox = &b
	}
	if s.search != nil {
		c := *s.search
		c.Results = slices.Clone(c.Results)
		m.StoreSearch = &c
	}
	return m
}

// ShopItem returns an item of a shop window, or nil.
func ShopItem(shop *wathenav1.ShopOpen, id uint32) *wathenav1.ShopItem {
	for _, it := range shop.GetItems() {
		if it.GetItemId() == id {
			return it
		}
	}
	return nil
}

func (b *Bot) mail(ctx context.Context, c wathenav1.MailCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{Mail: c.Build()})
}

// MailOpen opens the mailbox window.
func (b *Bot) MailOpen(ctx context.Context) error {
	return b.mail(ctx, wathenav1.MailCommand_builder{Open: new(true)})
}

// MailClose closes the mailbox window.
func (b *Bot) MailClose(ctx context.Context) error {
	return b.mail(ctx, wathenav1.MailCommand_builder{Close: new(true)})
}

// MailRead asks for a mail's text and attachments.
func (b *Bot) MailRead(ctx context.Context, id uint64) error {
	return b.mail(ctx, wathenav1.MailCommand_builder{Read: &id})
}

// MailDelete deletes a mail without attachments.
func (b *Bot) MailDelete(ctx context.Context, id uint64) error {
	return b.mail(ctx, wathenav1.MailCommand_builder{Delete: &id})
}

// MailTake takes a mail's zeny, its items, or both.
func (b *Bot) MailTake(ctx context.Context, id uint64, zeny, items bool) error {
	return b.mail(ctx, wathenav1.MailCommand_builder{Take: wathenav1.MailTake_builder{Mail: id, Zeny: zeny, Items: items}.Build()})
}

// Attachment is an inventory slot and an amount to attach to a mail.
type Attachment struct {
	Slot, Amount int
}

// MailSend sends a mail to a character by name, with zeny and items.
func (b *Bot) MailSend(ctx context.Context, to, title, body string, zeny uint32, items []Attachment) error {
	moves := make([]*wathenav1.StorageMove, 0, len(items))
	for _, a := range items {
		moves = append(moves, storageMove(a.Slot, a.Amount))
	}
	return b.mail(ctx, wathenav1.MailCommand_builder{Send: wathenav1.MailSend_builder{To: to, Title: title, Body: body, Zeny: zeny,
		Items: moves}.Build()})
}

func (b *Bot) vendingCmd(ctx context.Context, c wathenav1.VendingCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{Vending: c.Build()})
}

// VendLine is one line of a vending shop: a cart slot, the amount for
// sale and the price of one.
type VendLine struct {
	CartSlot, Amount int
	Price            uint32
}

// VendingOpen opens a vending shop from the cart, after the Vending
// skill showed the setup window.
func (b *Bot) VendingOpen(ctx context.Context, title string, lines []VendLine) error {
	l := make([]*wathenav1.VendingLine, 0, len(lines))
	for _, v := range lines {
		l = append(l, wathenav1.VendingLine_builder{CartSlot: uint32(max(v.CartSlot, 0)), Amount: uint32(max(v.Amount, 0)), // #nosec G115 -- cart slots and amounts.
			Price: v.Price}.Build())
	}
	return b.vendingCmd(ctx, wathenav1.VendingCommand_builder{Open: wathenav1.VendingOpen_builder{Title: title, Items: l}.Build()})
}

// VendingClose closes the bot's shop.
func (b *Bot) VendingClose(ctx context.Context) error {
	return b.vendingCmd(ctx, wathenav1.VendingCommand_builder{Close: new(true)})
}

// VendingCancel closes the setup window without a shop.
func (b *Bot) VendingCancel(ctx context.Context) error {
	return b.vendingCmd(ctx, wathenav1.VendingCommand_builder{Cancel: new(true)})
}

// VendingBrowse asks for the shop of the player with an entity ID.
func (b *Bot) VendingBrowse(ctx context.Context, vendor uint32) error {
	return b.vendingCmd(ctx, wathenav1.VendingCommand_builder{Browse: &vendor})
}

// VendingBuy buys from a shop: amounts by VendingListItem.index. vendor
// is 0 for the shop a store search opened.
func (b *Bot) VendingBuy(ctx context.Context, vendor, shop uint32, lines map[uint32]int) error {
	l := make([]*wathenav1.VendingBuyLine, 0, len(lines))
	for _, i := range slices.Sorted(maps.Keys(lines)) {
		l = append(l, wathenav1.VendingBuyLine_builder{Index: i, Amount: uint32(max(lines[i], 0))}.Build()) // #nosec G115 -- amounts.
	}
	return b.vendingCmd(ctx, wathenav1.VendingCommand_builder{Buy: wathenav1.VendingBuy_builder{Vendor: vendor, Shop: shop, Items: l}.Build()})
}

func (b *Bot) buyingCmd(ctx context.Context, c wathenav1.BuyingStoreCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{BuyingStore: c.Build()})
}

// BuyLine is one line of a buying store: the item, how many and the
// price of one.
type BuyLine struct {
	ItemID uint32
	Amount int
	Price  uint32
}

// BuyingStoreOpen opens a buying store, after player:open_buying_store
// showed the setup window.
func (b *Bot) BuyingStoreOpen(ctx context.Context, title string, zenyLimit uint32, lines []BuyLine) error {
	l := make([]*wathenav1.BuyingStoreLine, 0, len(lines))
	for _, v := range lines {
		l = append(l, wathenav1.BuyingStoreLine_builder{ItemId: v.ItemID, Amount: uint32(max(v.Amount, 0)), Price: v.Price}.Build()) // #nosec G115 -- amounts.
	}
	return b.buyingCmd(ctx, wathenav1.BuyingStoreCommand_builder{Open: wathenav1.BuyingStoreOpen_builder{Title: title, ZenyLimit: zenyLimit,
		Items: l}.Build()})
}

// BuyingStoreClose closes the bot's buying store.
func (b *Bot) BuyingStoreClose(ctx context.Context) error {
	return b.buyingCmd(ctx, wathenav1.BuyingStoreCommand_builder{Close: new(true)})
}

// BuyingStoreCancel closes the setup window without a store.
func (b *Bot) BuyingStoreCancel(ctx context.Context) error {
	return b.buyingCmd(ctx, wathenav1.BuyingStoreCommand_builder{Cancel: new(true)})
}

// BuyingStoreBrowse asks for the store of the player with an entity ID.
func (b *Bot) BuyingStoreBrowse(ctx context.Context, buyer uint32) error {
	return b.buyingCmd(ctx, wathenav1.BuyingStoreCommand_builder{Browse: &buyer})
}

// SellLine is one line of a sale to a buying store: an inventory slot,
// its item and how many.
type SellLine struct {
	Slot   int
	ItemID uint32
	Amount int
}

// BuyingStoreSell sells inventory items to a store. buyer is 0 for the
// store a store search opened.
func (b *Bot) BuyingStoreSell(ctx context.Context, buyer, store uint32, lines []SellLine) error {
	l := make([]*wathenav1.BuyingStoreSellLine, 0, len(lines))
	for _, v := range lines {
		l = append(l, wathenav1.BuyingStoreSellLine_builder{Slot: uint32(max(v.Slot, 0)), ItemId: v.ItemID, // #nosec G115 -- slots and amounts.
			Amount: uint32(max(v.Amount, 0))}.Build()) // #nosec G115 -- as above.
	}
	return b.buyingCmd(ctx, wathenav1.BuyingStoreCommand_builder{Sell: wathenav1.BuyingStoreSell_builder{Buyer: buyer, Store: store,
		Items: l}.Build()})
}

func (b *Bot) searchCmd(ctx context.Context, c wathenav1.StoreSearchCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{StoreSearch: c.Build()})
}

// SearchQuery is a store search: vending shops, or buying stores with
// Buying set, that sell or buy one of the items, in a price range (0 for
// no bound).
type SearchQuery struct {
	Buying             bool
	Items, Cards       []uint32
	MinPrice, MaxPrice uint32
}

// StoreSearchQuery runs a search in the open store search window.
func (b *Bot) StoreSearchQuery(ctx context.Context, q SearchQuery) error {
	return b.searchCmd(ctx, wathenav1.StoreSearchCommand_builder{Query: wathenav1.StoreSearchQuery_builder{Buying: q.Buying,
		ItemIds: q.Items, CardIds: q.Cards, MinPrice: q.MinPrice, MaxPrice: q.MaxPrice}.Build()})
}

// StoreSearchNext asks for the next page of results.
func (b *Bot) StoreSearchNext(ctx context.Context) error {
	return b.searchCmd(ctx, wathenav1.StoreSearchCommand_builder{Next: new(true)})
}

// StoreSearchClick picks a result by its index.
func (b *Bot) StoreSearchClick(ctx context.Context, index uint32) error {
	return b.searchCmd(ctx, wathenav1.StoreSearchCommand_builder{Click: &index})
}

// StoreSearchClose closes the store search window. The bot forgets it.
func (b *Bot) StoreSearchClose(ctx context.Context) error {
	b.mu.Lock()
	b.marketSt.search = nil
	b.mu.Unlock()
	return b.searchCmd(ctx, wathenav1.StoreSearchCommand_builder{Close: new(true)})
}
