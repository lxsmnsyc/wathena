package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// The rAthena values the market scenarios check.
const (
	// mailFee is mail_zeny_fee in percent, and mailItemFee
	// mail_attachment_price.
	mailFee     = 2
	mailItemFee = 2500
	// mailDays is mail_return_days and mail_delete_days.
	mailDays = 15
	// vendTaxFrom is vending_tax_min: the 5% tax applies from there.
	vendTaxFrom = 100_000_000
)

// eventually polls ok every 100 ms for up to 15 seconds, and fails the
// test when it never passes.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(DefaultTimeout)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("waiting until %s", what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// count reads one number from the test's database.
func (s *Server) count(t *testing.T, query string, args ...any) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	var n int64
	if err := s.DB.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// auditMails adds up what the audit's mails hold, the entries named
// mail:<ID>: their items, and their zeny as item 0.
func auditMails(t *testing.T, srv *Server) map[uint32]int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	zones, err := srv.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := map[uint32]int{}
	for _, z := range zones {
		for _, p := range z.Players {
			if z.Map != GuildStorageMap || !strings.HasPrefix(p.Name, "mail:") {
				continue
			}
			out[0] += int(p.Zeny)
			for _, it := range p.Items {
				out[it.Item] += it.Amount
			}
		}
	}
	if _, twice := TotalsOf(zones); len(twice) > 0 {
		t.Errorf("serials held twice: %v", twice)
	}
	return out
}

// giveZeny gives zeny with the fixture's "!zeny" and waits for it.
func (p *Player) giveZeny(n int64) {
	p.t.Helper()
	want := p.zeny() + n
	p.MustCommand(fmt.Sprintf("!zeny %d", n))
	p.Until(fmt.Sprintf("%d zeny", want), func(_ View, g Game) bool { return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == want })
}

// TestShopCostsAndStock: the fixture's Exchange sells an Apple for 10
// zeny, 2 Jellopy and 1 token, with a stock of 3 that fills again an
// hour later. Purchases fail without the zeny, the currency, the items
// or the stock, and a purchase takes all three costs in one step.
func TestShopCostsAndStock(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	open := func() *wathenav1.ShopOpen { return alice.OpenShop(ExchangeName) }
	shop := open()
	if shop.GetName() != ExchangeName || shop.GetBuysFromPlayers() || len(shop.GetSellable()) != 0 || len(shop.GetItems()) != 3 {
		t.Errorf("the Exchange %v", shop)
	}
	apple := botclient.ShopItem(shop, Apple)
	if apple.GetPrice() != 10 || apple.GetStock() != 3 || !apple.HasStock() || len(apple.GetCostItems()) != 1 ||
		apple.GetCostItems()[0].GetItemId() != Jellopy || apple.GetCostItems()[0].GetAmount() != 2 || len(apple.GetCostCurrencies()) != 1 {
		t.Errorf("the Apple %v", apple)
	}
	if c := apple.GetCostCurrencies()[0]; c.GetCurrency() != Tokens || c.GetDisplayName() != "Arena Tokens" || c.GetAmount() != 1 {
		t.Errorf("the Apple's currency %v", c)
	}
	if p := botclient.ShopItem(shop, RedPotion); p.GetPrice() != 0 || p.HasStock() || p.GetCostCurrencies()[0].GetAmount() != 5 {
		t.Errorf("the Red Potion %v", p)
	}
	buy := func(lines map[uint32]int, want wathenav1.ShopOutcome, item uint32) {
		t.Helper()
		if r := alice.TryBuy(open(), lines); r.GetOutcome() != want || r.GetItemId() != item {
			t.Fatalf("buying %v: %v, want %s about item %d", lines, r, want, item)
		}
	}
	buy(map[uint32]int{Apple: 1}, wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_ZENY, 0)
	alice.giveZeny(1000)
	buy(map[uint32]int{Apple: 1}, wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_CURRENCY, 0)
	if n := alice.GiveCurrency(Tokens, 4); n != 4 {
		t.Fatalf("Alice holds %d tokens", n)
	}

	mark := alice.Mark()
	buy(map[uint32]int{Apple: 2}, wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT, 0)
	alice.Until("2 Apples for 20 zeny, 4 Jellopy and 2 tokens", func(_ View, g Game) bool {
		return g.Count(Apple) == 2 && g.Count(Jellopy) == 16 && g.Stats[wathenav1.StatId_STAT_ID_ZENY] == 980 && g.Currencies[Tokens] == 2
	})
	if !seenSince(alice.Bot, mark, func(e *wathenav1.Event) bool {
		return e.GetInventoryRemove().GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_PAID
	}) {
		t.Error("Alice saw no InventoryRemove with INVENTORY_REMOVE_REASON_PAID")
	}
	if s := botclient.ShopItem(open(), Apple).GetStock(); s != 1 {
		t.Errorf("the stock after 2 Apples: %d", s)
	}
	buy(map[uint32]int{Apple: 2}, wathenav1.ShopOutcome_SHOP_OUTCOME_OUT_OF_STOCK, Apple)
	buy(map[uint32]int{Apple: 1}, wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT, 0)
	if s := botclient.ShopItem(open(), Apple); !s.HasStock() || s.GetStock() != 0 {
		t.Errorf("the stock after 3 Apples: %v", s)
	}
	buy(map[uint32]int{Apple: 1}, wathenav1.ShopOutcome_SHOP_OUTCOME_OUT_OF_STOCK, Apple)

	// The stock fills again an hour after the last fill.
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	if err := srv.AdvanceClock(ctx, 61*time.Minute); err != nil {
		t.Fatal(err)
	}
	if s := botclient.ShopItem(open(), Apple).GetStock(); s != 3 {
		t.Errorf("the stock an hour later: %d", s)
	}
	alice.MustCommand("!take Jellopy 13")
	buy(map[uint32]int{Apple: 1}, wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_ITEMS, Jellopy)
	buy(map[uint32]int{RedPotion: 1}, wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_CURRENCY, 0)

	// A Jellopy costs a Jellopy: the stack that pays empties, and a new
	// one comes.
	mark = alice.Mark()
	buy(map[uint32]int{Jellopy: 1}, wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT, 0)
	alice.Until("the new Jellopy", func(View, Game) bool {
		return seenSince(alice.Bot, mark, func(e *wathenav1.Event) bool { return e.GetInventoryAdd().GetItemId() == Jellopy })
	})
	if alice.Count(Jellopy) != 1 {
		t.Errorf("Alice holds %d Jellopy", alice.Count(Jellopy))
	}

	// shop:set_stock sets a stock for every player.
	alice.MustCommand("!stock Apple 1")
	if s := botclient.ShopItem(open(), Apple).GetStock(); s != 1 {
		t.Errorf("the stock after shop:set_stock: %d", s)
	}
	alice.MustCommand("!pay tokens 1")
	if got := alice.Command("!pay tokens 1"); got != "false" {
		t.Errorf("taking a token Alice does not have: %s", got)
	}
	alice.Until("no tokens", func(_ View, g Game) bool { return len(g.Currencies) == 0 })
	srv.CheckLogs()
}

// playCurrencies gives Alice 7 tokens, which her character owns, and 3
// cash points, which her account owns, and checks CurrencyUpdate. It
// returns Alice.
func playCurrencies(t *testing.T, srv *Server) *Player {
	t.Helper()
	alice := srv.Join(t, "Alice")
	if n := alice.GiveCurrency(Tokens, 7); n != 7 {
		t.Errorf("tokens: %d", n)
	}
	if n := alice.GiveCurrency(Cash, 3); n != 3 {
		t.Errorf("cash: %d", n)
	}
	u := alice.Seen(func(e *wathenav1.Event) bool { return len(e.GetCurrencies().GetCurrencies()) == 2 }).GetCurrencies()
	names := map[string]string{}
	for _, c := range u.GetCurrencies() {
		names[c.GetName()] = c.GetDisplayName()
	}
	if names[Tokens] != "Arena Tokens" || names[Cash] != "Cash Points" {
		t.Errorf("CurrencyUpdate %v", u)
	}
	bob := srv.Join(t, "Bob")
	if got := bob.Command("!currency tokens 1"); got != "1" {
		t.Errorf("Bob's tokens: %s", got)
	}
	return alice
}

// TestCurrencies: a script gives a character's own currency and an
// account's currency, CurrencyUpdate lists both with their display
// names, and the player gets them again at its next login.
func TestCurrencies(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := playCurrencies(t, srv)
	_ = alice.Close()
	alice = srv.Join(t, "Alice")
	alice.Until("both currencies at login", func(_ View, g Game) bool { return g.Currencies[Tokens] == 7 && g.Currencies[Cash] == 3 })
	srv.CheckLogs()
}

// TestCurrenciesInTheDatabase: the same with PostgreSQL. The currencies
// come back after Restart, and their rows and zeny log lines are saved.
func TestCurrenciesInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	alice := playCurrencies(t, srv)
	_ = alice.Close()
	srv.Restart()
	alice = srv.Join(t, "Alice")
	alice.Until("both currencies after a restart", func(_ View, g Game) bool { return g.Currencies[Tokens] == 7 && g.Currencies[Cash] == 3 })
	if n := srv.count(t, `SELECT coalesce(sum(cc.amount), 0) FROM character_currencies cc JOIN characters c ON c.id = cc.character_id
		WHERE c.name = 'Alice' AND cc.currency >= 10000`); n != 7 {
		t.Errorf("saved tokens: %d", n)
	}
	if n := srv.count(t, `SELECT coalesce(sum(amount), 0) FROM account_currencies WHERE currency = 1`); n != 3 {
		t.Errorf("saved cash: %d", n)
	}
	if n := srv.count(t, `SELECT count(*) FROM zeny_log WHERE currency <> 0`); n != 3 {
		t.Errorf("%d zeny log lines of currencies, want 3", n)
	}
	srv.CheckLogs()
}

// TestShopStockInTheDatabase: with PostgreSQL, the Exchange's stock of
// Apples is saved with the purchase, and comes back after Restart.
func TestShopStockInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	alice := srv.Join(t, "Alice")
	alice.giveZeny(100)
	alice.GiveCurrency(Tokens, 2)
	if o := alice.Buy(alice.OpenShop(ExchangeName), map[uint32]int{Apple: 2}); o != wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT {
		t.Fatalf("buying 2 Apples: %s", o)
	}
	eventually(t, "the stock row", func() bool {
		return srv.count(t, `SELECT coalesce(min(stock), -1) FROM shop_stock WHERE item_id = $1`, Apple) == 1
	})
	_ = alice.Close()
	srv.Restart()
	alice = srv.Join(t, "Alice")
	if s := botclient.ShopItem(alice.OpenShop(ExchangeName), Apple).GetStock(); s != 1 {
		t.Errorf("the stock after a restart: %d", s)
	}
	alice.Until("the Apples, and no tokens", func(_ View, g Game) bool { return g.Count(Apple) == 2 && len(g.Currencies) == 0 })
	srv.CheckLogs()
}

// sendHello has Alice, with 10,000 zeny, send Bob 3 Jellopy and 1,000
// zeny. It checks the fee: 2% of the zeny and 2,500 for the item stack.
// It returns the mail's ID.
func sendHello(t *testing.T, alice *Player) uint64 {
	t.Helper()
	alice.giveZeny(10000)
	mark := alice.Mark()
	r := alice.SendMail("Bob", "Hello", 1000, map[uint32]int{Jellopy: 3})
	if r.GetOutcome() != wathenav1.MailOutcome_MAIL_OUTCOME_SENT || r.GetMail() == 0 {
		t.Fatalf("sending a mail: %v", r)
	}
	want := int64(10000 - 1000 - 1000*mailFee/100 - mailItemFee)
	alice.Until("the zeny and items mailed", func(_ View, g Game) bool {
		return g.Count(Jellopy) == 17 && g.Stats[wathenav1.StatId_STAT_ID_ZENY] == want
	})
	if !seenSince(alice.Bot, mark, func(e *wathenav1.Event) bool {
		return e.GetInventoryRemove().GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_MAILED
	}) {
		t.Error("Alice saw no InventoryRemove with INVENTORY_REMOVE_REASON_MAILED")
	}
	return r.GetMail()
}

// takeHello has Bob find Alice's mail, read it and take it.
func takeHello(t *testing.T, bob *Player, id uint64) {
	t.Helper()
	box := bob.OpenMailbox()
	var mail *wathenav1.MailSummary
	for _, m := range box.GetMails() {
		if m.GetId() == id {
			mail = m
		}
	}
	if mail == nil || mail.GetFrom() != "Alice" || mail.GetTitle() != "Hello" || mail.GetZeny() != 1000 || mail.GetItems() != 1 ||
		mail.GetExpiresMs()-mail.GetSentMs() != (mailDays*24*time.Hour).Milliseconds() {
		t.Fatalf("Bob's mailbox %v", box)
	}
	if o := bob.DeleteMail(id); o != wathenav1.MailOutcome_MAIL_OUTCOME_NOT_EMPTY {
		t.Errorf("deleting a mail with attachments: %s", o)
	}
	read := bob.ReadMail(id)
	if items := read.GetItems(); read.GetBody() == "" || len(items) != 1 || items[0].GetItemId() != Jellopy || items[0].GetAmount() != 3 {
		t.Errorf("the mail %v", read)
	}
	zeny, jellopy := bob.zeny(), bob.Count(Jellopy)
	if o := bob.TakeMail(id, true, true); o != wathenav1.MailOutcome_MAIL_OUTCOME_TAKEN {
		t.Fatalf("taking the attachments: %s", o)
	}
	bob.Until("the attachments", func(_ View, g Game) bool {
		return g.Count(Jellopy) == jellopy+3 && g.Stats[wathenav1.StatId_STAT_ID_ZENY] == zeny+1000
	})
	if o := bob.DeleteMail(id); o != wathenav1.MailOutcome_MAIL_OUTCOME_DELETED {
		t.Errorf("deleting the empty mail: %s", o)
	}
}

// TestMail: Alice mails Bob items and zeny, and pays the fee. Bob gets
// MailNew, sees the mail in his mailbox, reads it and takes it. A mail
// too soon after the last, to a name nobody has, or with too little zeny
// is refused. The audit counts the mail's items and zeny while it waits.
func TestMail(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	if l := alice.WorldLimits(); l.MailItems != 5 || l.Mailbox != 100 {
		t.Errorf("the mail limits %+v", l)
	}
	if box := alice.OpenMailbox(); len(box.GetMails()) != 0 || box.GetSize() != 100 || box.GetDaily() != 100 {
		t.Errorf("Alice's mailbox %v", box)
	}
	id := sendHello(t, alice)
	if r := alice.SendMail("Bob", "Again", 0, nil); r.GetOutcome() != wathenav1.MailOutcome_MAIL_OUTCOME_TOO_FAST {
		t.Errorf("a mail right after the last: %v", r)
	}
	if got := auditMails(t, srv); got[Jellopy] != 3 || got[0] != 1000 {
		t.Errorf("the audit's mails hold %v", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	if err := srv.AdvanceClock(ctx, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if r := alice.SendMail("Nobody", "Lost", 0, nil); r.GetOutcome() != wathenav1.MailOutcome_MAIL_OUTCOME_NO_RECIPIENT {
		t.Errorf("a mail to nobody: %v", r)
	}
	if r := alice.SendMail("Bob", "Rich", 100000, nil); r.GetOutcome() != wathenav1.MailOutcome_MAIL_OUTCOME_ZENY {
		t.Errorf("a mail with more zeny than Alice has: %v", r)
	}
	if n := bob.MailArrives("Hello"); n.GetFrom() != "Alice" || n.GetUnread() != 1 || n.GetId() != id {
		t.Errorf("MailNew %v", n)
	}
	takeHello(t, bob, id)
	if got := auditMails(t, srv); len(got) != 0 {
		t.Errorf("the audit's mails hold %v after the take", got)
	}
	// player:open_mail opens the mailbox from a script.
	mark := bob.Mark()
	bob.MustCommand("!mailbox")
	if !seenSince(bob.Bot, mark, (*wathenav1.Event).HasMailBox) {
		t.Error("player:open_mail sent no MailBox")
	}
	srv.CheckLogs()
}

// TestScriptMailToOfflineCharacter: a script mails items and zeny to a
// character who is offline. The character finds the mail at its next
// login and takes it.
func TestScriptMailToOfflineCharacter(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	_ = bob.Close()
	alice.MustCommand("!mail Bob Apple 2 300")
	bob = srv.Join(t, "Bob")
	takeParcel(t, bob)
	srv.CheckLogs()
}

// takeParcel opens the player's mailbox, finds the fixture's parcel of 2
// Apples and 300 zeny, and takes it.
func takeParcel(t *testing.T, p *Player) {
	t.Helper()
	p.OpenMailbox()
	var mail *wathenav1.MailSummary
	p.Until("the parcel in the mailbox", func(_ View, g Game) bool {
		if g.Mailbox == nil {
			return false
		}
		for _, m := range g.Mailbox.Mails {
			if m.GetTitle() == "Parcel" {
				mail = m
			}
		}
		return mail != nil
	})
	// A script's mail is deleted after mail_delete_days.
	if mail.GetFrom() != "Postmaster" || mail.GetZeny() != 300 || mail.GetItems() != 1 ||
		mail.GetExpiresMs()-mail.GetSentMs() != (mailDays*24*time.Hour).Milliseconds() {
		t.Errorf("the parcel %v", mail)
	}
	zeny := p.zeny()
	if o := p.TakeMail(mail.GetId(), true, true); o != wathenav1.MailOutcome_MAIL_OUTCOME_TAKEN {
		t.Fatalf("taking the parcel: %s", o)
	}
	p.Until("2 Apples and 300 zeny", func(_ View, g Game) bool {
		return g.Count(Apple) == 2 && g.Stats[wathenav1.StatId_STAT_ID_ZENY] == zeny+300
	})
}

// TestMailInTheDatabase: with PostgreSQL, Alice mails Bob, and a script
// mails Carol, who is offline. Both mails are saved, and after Restart
// Bob and Carol take them.
func TestMailInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	_ = carol.Close()
	id := sendHello(t, alice)
	alice.MustCommand("!mail Carol Apple 2 300")
	eventually(t, "both mails saved", func() bool {
		return srv.count(t, `SELECT count(*) FROM mails`) == 2 && srv.count(t, `SELECT coalesce(sum(amount), 0) FROM mail_items`) == 5
	})
	if n := srv.count(t, `SELECT coalesce(sum(zeny), 0) FROM mails`); n != 1300 {
		t.Errorf("the saved mails hold %d zeny", n)
	}
	_ = alice.Close()
	_ = bob.Close()
	srv.Restart()
	bob = srv.Join(t, "Bob")
	takeHello(t, bob, id)
	carol = srv.Join(t, "Carol")
	takeParcel(t, carol)
	eventually(t, "the mails emptied", func() bool {
		return srv.count(t, `SELECT count(*) FROM mail_items`) == 0 && srv.count(t, `SELECT coalesce(sum(zeny), 0) FROM mails`) == 0
	})
	srv.CheckLogs()
}

// TestMailExpiry: Bob leaves Alice's mail. After mail_return_days the
// expiry, which runs once a minute, sends it back to Alice with "RE:"
// before its title, and Alice takes her items and zeny back.
func TestMailExpiry(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	alice.giveZeny(5000)
	if r := alice.SendMail("Bob", "Hello", 500, map[uint32]int{Jellopy: 2}); r.GetOutcome() != wathenav1.MailOutcome_MAIL_OUTCOME_SENT {
		t.Fatalf("sending the mail: %v", r)
	}
	bob.MailArrives("Hello")
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	if err := srv.AdvanceClock(ctx, mailDays*24*time.Hour+time.Minute); err != nil {
		t.Fatal(err)
	}
	alice.Timeout = 90 * time.Second
	back := alice.MailArrives("RE:Hello")
	alice.Timeout = DefaultTimeout
	if back.GetFrom() != "Bob" {
		t.Errorf("the mail came back from %q", back.GetFrom())
	}
	if box := bob.OpenMailbox(); len(box.GetMails()) != 0 {
		t.Errorf("Bob's mailbox still holds %v", box.GetMails())
	}
	box := alice.OpenMailbox()
	if len(box.GetMails()) != 1 || !box.GetMails()[0].GetReturned() || box.GetMails()[0].GetZeny() != 500 {
		t.Fatalf("Alice's mailbox %v", box)
	}
	if o := alice.TakeMail(back.GetId(), true, true); o != wathenav1.MailOutcome_MAIL_OUTCOME_TAKEN {
		t.Fatalf("taking the returned mail: %s", o)
	}
	want := int64(5000 - 500*mailFee/100 - mailItemFee)
	alice.Until("the items and zeny back", func(_ View, g Game) bool {
		return g.Count(Jellopy) == 20 && g.Stats[wathenav1.StatId_STAT_ID_ZENY] == want
	})
	srv.CheckLogs()
}

// merchant gets a Merchant a cart and Vending, and puts items in its
// cart, amounts by item ID.
func (p *Player) merchant(cart map[uint32]int) {
	p.t.Helper()
	p.GetCart()
	p.LearnVending()
	for id, n := range cart {
		if o := p.TryCartPut(id, n); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
			p.t.Fatalf("%s: putting %d of item %d in the cart: %s", p.Name, n, id, o)
		}
	}
}

// vendOutcome buys from a vendor's shop, browsing it again first, and
// checks the outcome.
func vendOutcome(t *testing.T, buyer *Player, vendor string, items map[uint32]int, want wathenav1.VendingOutcome) *wathenav1.VendingResult {
	t.Helper()
	r := buyer.VendBuy(buyer.BrowseVending(vendor), items)
	if r.GetOutcome() != want {
		t.Fatalf("%s buys %v from %s: %v, want %s", buyer.Name, items, vendor, r, want)
	}
	return r
}

// TestVending: Alice opens a shop from her cart. Bob sees its title over
// her, browses it and buys. Alice sees VendingSold, and the zeny and the
// items move. A sale needs the zeny and the stock, the fixture's vend
// hook refuses Contraband, and a purchase of 100,000,000 zeny pays the
// 5% vending tax.
func TestVending(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartJob: MerchantJob})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	alice.MustCommand("!give e2e.fixture:Contraband 2")
	alice.merchant(map[uint32]int{Jellopy: 10, Contraband: 2, RedPotion: 1})
	r := alice.OpenVending("Alice's goods", Vend{Jellopy, 6, 100}, Vend{Contraband, 2, 10}, Vend{RedPotion, 1, vendTaxFrom})
	if r.GetOutcome() != wathenav1.VendingOutcome_VENDING_OUTCOME_OPENED {
		t.Fatalf("opening the shop: %v", r)
	}
	if own := alice.Game().Vending; own.GetTitle() != "Alice's goods" || len(own.GetItems()) != 3 {
		t.Errorf("Alice's shop %v", own)
	}
	bob.Until("the shop's title over Alice", func(v View, _ Game) bool {
		for _, e := range v.Entities {
			if e.Name == "Alice" && e.ShopTitle == "Alice's goods" && !e.ShopBuying {
				return true
			}
		}
		return false
	})
	list := bob.BrowseVending("Alice")
	if list.GetTitle() != "Alice's goods" || len(list.GetItems()) != 3 || list.GetItems()[VendIndex(list, Jellopy)].GetPrice() != 100 {
		t.Errorf("the shop %v", list)
	}
	vendOutcome(t, bob, "Alice", map[uint32]int{Jellopy: 1}, wathenav1.VendingOutcome_VENDING_OUTCOME_NOT_ENOUGH_ZENY)
	bob.giveZeny(2000)
	aliceZeny := alice.zeny()
	vendOutcome(t, bob, "Alice", map[uint32]int{Jellopy: 4}, wathenav1.VendingOutcome_VENDING_OUTCOME_BOUGHT)
	bob.Until("4 Jellopy for 400 zeny", func(_ View, g Game) bool {
		return g.Count(Jellopy) == 24 && g.Stats[wathenav1.StatId_STAT_ID_ZENY] == 1600
	})
	sold := alice.Expect("VendingSold", (*wathenav1.Event).HasVendingSold).GetVendingSold()
	if sold.GetAmount() != 4 || sold.GetBuyer() != "Bob" || sold.GetZeny() != 400 {
		t.Errorf("VendingSold %v", sold)
	}
	alice.Until("400 zeny and 6 Jellopy left in the cart", func(_ View, g Game) bool {
		return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == aliceZeny+400 && g.Cart.Count(Jellopy) == 6
	})
	want := "e2e vend: Bob bought 4 items from Alice for 400 zeny"
	for _, p := range []*Player{alice, bob} {
		if got := p.SaleLine("vend"); got != want {
			t.Errorf("%s got the vend line %q, want %q", p.Name, got, want)
		}
	}
	if r := vendOutcome(t, bob, "Alice", map[uint32]int{Contraband: 1}, wathenav1.VendingOutcome_VENDING_OUTCOME_HOOK); r.GetMessage() != "No contraband here." {
		t.Errorf("the vend hook's message %q", r.GetMessage())
	}
	vendOutcome(t, bob, "Alice", map[uint32]int{Jellopy: 3}, wathenav1.VendingOutcome_VENDING_OUTCOME_OUT_OF_STOCK)

	// A purchase of vending_tax_min pays vending_tax: the seller gets 95%.
	bob.giveZeny(vendTaxFrom)
	aliceZeny, bobZeny := alice.zeny(), bob.zeny()
	alice.Skip()
	vendOutcome(t, bob, "Alice", map[uint32]int{RedPotion: 1}, wathenav1.VendingOutcome_VENDING_OUTCOME_BOUGHT)
	taxed := int64(vendTaxFrom * 95 / 100)
	if s := alice.Expect("VendingSold", (*wathenav1.Event).HasVendingSold).GetVendingSold(); int64(s.GetZeny()) != taxed {
		t.Errorf("VendingSold of the taxed sale %v", s)
	}
	alice.Until("the taxed zeny", func(_ View, g Game) bool { return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == aliceZeny+taxed })
	bob.Until("the whole price paid", func(_ View, g Game) bool { return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == bobZeny-vendTaxFrom })

	alice.send("close the shop", alice.VendingClose)
	alice.Expect("the shop to close", func(e *wathenav1.Event) bool {
		return e.GetVendingResult().GetOutcome() == wathenav1.VendingOutcome_VENDING_OUTCOME_CLOSED
	})
	bob.Until("no title over Alice", func(v View, _ Game) bool {
		for _, e := range v.Entities {
			if e.Name == "Alice" {
				return e.ShopTitle == ""
			}
		}
		return false
	})
	if got := auditStorage(t, srv); got[Jellopy] != 6 || got[Contraband] != 2 || got[RedPotion] != 0 {
		t.Errorf("the audit's carts hold %v", got)
	}
	srv.CheckLogs()
}

// goAutotrade has a player with an open shop say "!autotrade", and waits
// until the server closes its connection with DISCONNECT_REASON_AUTOTRADE.
func goAutotrade(t *testing.T, p *Player) {
	t.Helper()
	p.send("chat", func(ctx context.Context) error { return p.Chat(ctx, "!autotrade") })
	select {
	case <-p.Done():
	case <-time.After(DefaultTimeout):
		t.Fatalf("%s: the connection did not close for autotrade", p.Name)
	}
	var d *botclient.DisconnectError
	if err := p.Err(); !errors.As(err, &d) || d.Reason != wathenav1.DisconnectReason_DISCONNECT_REASON_AUTOTRADE {
		t.Fatalf("%s: the connection closed with %v", p.Name, err)
	}
}

// TestVendingAutotrade: Alice goes on autotrade, and her connection
// closes with DISCONNECT_REASON_AUTOTRADE. Her shop stays, and Bob still
// buys from it. When it sells out she leaves, and her next login has the
// zeny.
func TestVendingAutotrade(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartJob: MerchantJob})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	alice.merchant(map[uint32]int{Jellopy: 5})
	if r := alice.OpenVending("Away", Vend{Jellopy, 5, 50}); r.GetOutcome() != wathenav1.VendingOutcome_VENDING_OUTCOME_OPENED {
		t.Fatalf("opening the shop: %v", r)
	}
	zeny := alice.zeny()
	goAutotrade(t, alice)
	bob.giveZeny(1000)
	vendOutcome(t, bob, "Alice", map[uint32]int{Jellopy: 2}, wathenav1.VendingOutcome_VENDING_OUTCOME_BOUGHT)
	mark := bob.Mark()
	vendOutcome(t, bob, "Alice", map[uint32]int{Jellopy: 3}, wathenav1.VendingOutcome_VENDING_OUTCOME_BOUGHT)
	// The Vanish can come in the frame of the purchase's result, before it.
	bob.Until("Alice to leave once her shop sold out", func(View, Game) bool {
		return seenSince(bob.Bot, mark, func(e *wathenav1.Event) bool {
			return e.GetVanish().GetReason() == wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT
		})
	})
	alice = srv.Join(t, "Alice")
	alice.Until("the zeny of the sales", func(_ View, g Game) bool {
		return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == zeny+250 && g.Cart != nil && g.Cart.Count(Jellopy) == 0
	})
	srv.CheckLogs()
}

// TestVendingAutotradeInTheDatabase: with PostgreSQL, Alice's autotrade
// shop is saved, opens again after Restart, and still sells. Her next
// login ends the autotrade, with the zeny and the cart that is left.
func TestVendingAutotradeInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartJob: MerchantJob, Database: true})
	alice := srv.Join(t, "Alice")
	alice.merchant(map[uint32]int{Jellopy: 5})
	if r := alice.OpenVending("Away", Vend{Jellopy, 5, 50}); r.GetOutcome() != wathenav1.VendingOutcome_VENDING_OUTCOME_OPENED {
		t.Fatalf("opening the shop: %v", r)
	}
	zeny := alice.zeny()
	goAutotrade(t, alice)
	eventually(t, "the autotrade store saved", func() bool {
		return srv.count(t, `SELECT count(*) FROM stores WHERE autotrade AND NOT buying`) == 1
	})
	srv.Restart()
	bob := srv.Join(t, "Bob")
	bob.giveZeny(1000)
	bob.Until("Alice's shop again", func(v View, _ Game) bool {
		for _, e := range v.Entities {
			if e.Name == "Alice" && e.ShopTitle == "Away" {
				return true
			}
		}
		return false
	})
	vendOutcome(t, bob, "Alice", map[uint32]int{Jellopy: 2}, wathenav1.VendingOutcome_VENDING_OUTCOME_BOUGHT)
	bob.Until("2 Jellopy", func(_ View, g Game) bool { return g.Count(Jellopy) == 22 })
	alice = srv.Join(t, "Alice")
	alice.Until("the zeny of the sale and 3 Jellopy in the cart", func(_ View, g Game) bool {
		return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == zeny+100 && g.Cart != nil && g.Cart.Count(Jellopy) == 3
	})
	if alice.Game().Vending != nil {
		t.Error("the shop is still open after Alice logged in")
	}
	eventually(t, "the store row gone", func() bool { return srv.count(t, `SELECT count(*) FROM stores`) == 0 })
	srv.CheckLogs()
}

// TestStoreSearch: Alice vends Jellopy on the other map. Bob's store
// search finds it, with her map and cell, and a click shows the place.
// A search too soon is refused, a search for buying stores finds
// nothing, a click that opens stores cannot open one on another map, and
// a window without uses refuses a search.
func TestStoreSearch(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartJob: MerchantJob})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	alice.merchant(map[uint32]int{Jellopy: 5})
	alice.Talk("Guide").Pick("Go to the field")
	at := alice.MapChange(FieldMap)
	if r := alice.OpenVending("Field goods", Vend{Jellopy, 5, 70}); r.GetOutcome() != wathenav1.VendingOutcome_VENDING_OUTCOME_OPENED {
		t.Fatalf("opening the shop: %v", r)
	}
	if o := bob.OpenSearch(3, "show"); o.GetUses() != 3 || o.GetOpen() {
		t.Errorf("the search window %v", o)
	}
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	jellopy := botclient.SearchQuery{Items: []uint32{Jellopy}}
	res, o := bob.Search(jellopy)
	if o != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_UNSPECIFIED || len(res.GetResults()) != 1 || res.GetUses() != 2 {
		t.Fatalf("the search: %v, %s", res, o)
	}
	hit := res.GetResults()[0]
	if hit.GetOwner() != "Alice" || hit.GetTitle() != "Field goods" || hit.GetMap() != FieldMap || hit.GetX() != at.GetX() || hit.GetY() != at.GetY() ||
		hit.GetPrice() != 70 || hit.GetItem().GetAmount() != 5 || hit.GetBuying() {
		t.Errorf("the result %v", hit)
	}
	if r := bob.ClickResult(hit.GetIndex()); r.GetOutcome() != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_POSITION ||
		r.GetMap() != FieldMap || r.GetX() != at.GetX() || r.GetY() != at.GetY() {
		t.Errorf("the click %v", r)
	}
	if _, o := bob.Search(jellopy); o != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_TOO_FAST {
		t.Errorf("a search right after the last: %s", o)
	}
	if err := srv.AdvanceClock(ctx, 11*time.Second); err != nil {
		t.Fatal(err)
	}
	if res, o := bob.Search(botclient.SearchQuery{Buying: true, Items: []uint32{Jellopy}}); o != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_NOTHING || len(res.GetResults()) != 0 {
		t.Errorf("a search of buying stores: %v, %s", res, o)
	}
	bob.send("close the search", bob.StoreSearchClose)
	if o := bob.OpenSearch(1, "open"); !o.GetOpen() || o.GetUses() != 1 {
		t.Errorf("the search window that opens stores %v", o)
	}
	if res, _ := bob.Search(jellopy); len(res.GetResults()) != 1 {
		t.Fatalf("the search: %v", res)
	}
	if r := bob.ClickResult(0); r.GetOutcome() != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_OTHER_MAP {
		t.Errorf("opening a store on another map: %v", r)
	}
	if err := srv.AdvanceClock(ctx, 11*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, o := bob.Search(jellopy); o != wathenav1.StoreSearchOutcome_STORE_SEARCH_OUTCOME_NO_USES {
		t.Errorf("a search without uses: %s", o)
	}
	srv.CheckLogs()
}

// errNoBuyingStoreFlag is the reason the buying store scenarios fail
// while the item table lacks rAthena's BuyingStore flag.
const errNoBuyingStoreFlag = "the store refused Jellopy; it needs rAthena's BuyingStore flag from item_buyingstore.txt"

// TestBuyingStore: a script opens Alice's buying store setup, and she
// opens a store that buys 5 Jellopy at 50 zeny. Bob sees its title,
// browses it, finds it with a store search and sells to it. Alice sees
// BuyingStoreBought, and the zeny and the items move. A sale of more
// than the store buys is refused, and the store closes once it bought
// everything.
func TestBuyingStore(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	alice.giveZeny(1000)
	r := alice.OpenBuyingStore("Buying Jellopy", 500, botclient.BuyLine{ItemID: Jellopy, Amount: 5, Price: 50})
	if r.GetOutcome() != wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_OPENED {
		t.Fatalf("opening the buying store: %v: %s", r, errNoBuyingStoreFlag)
	}
	bob.Until("the store's title over Alice", func(v View, _ Game) bool {
		for _, e := range v.Entities {
			if e.Name == "Alice" && e.ShopTitle == "Buying Jellopy" && e.ShopBuying {
				return true
			}
		}
		return false
	})
	list := bob.BrowseBuyingStore("Alice")
	if list.GetZenyLimit() != 500 || len(list.GetItems()) != 1 || list.GetItems()[0].GetPrice() != 50 || list.GetItems()[0].GetAmount() != 5 {
		t.Errorf("the buying store %v", list)
	}
	if s := bob.SellToStore(list, map[uint32]int{Jellopy: 3}); s.GetOutcome() != wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_SOLD {
		t.Fatalf("selling 3 Jellopy: %v", s)
	}
	bob.Until("150 zeny for 3 Jellopy", func(_ View, g Game) bool {
		return g.Count(Jellopy) == 17 && g.Stats[wathenav1.StatId_STAT_ID_ZENY] == 150
	})
	bought := alice.Expect("BuyingStoreBought", (*wathenav1.Event).HasBuyingStoreBought).GetBuyingStoreBought()
	if bought.GetItemId() != Jellopy || bought.GetAmount() != 3 || bought.GetSeller() != "Bob" || bought.GetZeny() != 150 || bought.GetZenyLimit() != 350 {
		t.Errorf("BuyingStoreBought %v", bought)
	}
	alice.Until("3 Jellopy for 150 zeny", func(_ View, g Game) bool {
		return g.Count(Jellopy) == 23 && g.Stats[wathenav1.StatId_STAT_ID_ZENY] == 850
	})
	want := "e2e buying_store_sale: Alice bought 3 items from Bob for 150 zeny"
	for _, p := range []*Player{alice, bob} {
		if got := p.SaleLine("buying_store_sale"); got != want {
			t.Errorf("%s got the line %q, want %q", p.Name, got, want)
		}
	}

	bob.OpenSearch(1, "show")
	res, _ := bob.Search(botclient.SearchQuery{Buying: true, Items: []uint32{Jellopy}})
	if hits := res.GetResults(); len(hits) != 1 || !hits[0].GetBuying() || hits[0].GetOwner() != "Alice" || hits[0].GetPrice() != 50 || hits[0].GetItem().GetAmount() != 2 {
		t.Errorf("the search of buying stores: %v", res)
	}
	if s := bob.SellToStore(bob.BrowseBuyingStore("Alice"), map[uint32]int{Jellopy: 3}); s.GetOutcome() != wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_COUNT {
		t.Errorf("selling more than the store buys: %v", s)
	}
	if s := bob.SellToStore(bob.BrowseBuyingStore("Alice"), map[uint32]int{Jellopy: 2}); s.GetOutcome() != wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_SOLD {
		t.Fatalf("selling the last 2 Jellopy: %v", s)
	}
	alice.Expect("the store to close", func(e *wathenav1.Event) bool {
		return e.GetBuyingStoreResult().GetOutcome() == wathenav1.BuyingStoreOutcome_BUYING_STORE_OUTCOME_ALL_BOUGHT
	})
	bob.Until("no title over Alice", func(v View, _ Game) bool {
		for _, e := range v.Entities {
			if e.Name == "Alice" {
				return e.ShopTitle == ""
			}
		}
		return false
	})
	srv.CheckLogs()
}
