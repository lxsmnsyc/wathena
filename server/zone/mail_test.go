package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

func mailOutcomes(ev []*wathenav1.Event) []wathenav1.MailOutcome {
	var out []wathenav1.MailOutcome
	for _, e := range ev {
		if e.HasMailResult() {
			out = append(out, e.GetMailResult().GetOutcome())
		}
	}
	return out
}

func sendMailCmd(z *Zone, p *Player, to string, zeny uint32, items ...*wathenav1.StorageMove) []wathenav1.MailOutcome {
	z.mailCommand(p, wathenav1.MailCommand_builder{Send: wathenav1.MailSend_builder{To: to, Title: "Hello", Body: "Some text",
		Zeny: zeny, Items: items}.Build()}.Build())
	z.commitTransfers()
	z.stepAt(z.now + 50)
	return mailOutcomes(events(p))
}

func att(slot, amount uint32) *wathenav1.StorageMove {
	return wathenav1.StorageMove_builder{Slot: slot, Amount: amount}.Build()
}

// TestMailSendRules sends mail between two players: the items and zeny
// move into the mail with the fee, as mail_setattachment, the recipient
// sees the mail and takes it, and each rule refuses what it should:
// mail_items, mail_attachment_weight, mail_zeny_fee with
// mail_attachment_price, mail_delay and mail_daily_count.
func TestMailSendRules(t *testing.T) {
	z, a, b, cm := marketZone(t, stack(0, 909, 10), stack(1, 501, 5), stack(2, 7005, 10), stack(3, 1201, 1), stack(4, 7006, 1))
	r := rules.Base(true)
	r.Limits, _ = rules.LimitsOf(map[string]rules.RangeSet{"mail_items": {Base: 2, HasBase: true}})
	r.Market, _ = rules.MarketOf(map[string]int{"mail_daily_count": 2, "mail_delay": 5000, "mail_attachment_weight": 100})
	z.Mods = &Mods{Rules: r}
	z.mailboxOf(b)
	a.record.Zeny = 10_000
	if got := sendMailCmd(z, a, "Other", 0, att(0, 1), att(1, 1), att(3, 1)); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_TOO_MANY {
		t.Errorf("three items: %v", got)
	}
	// 10 heavy items weigh 3000 / 10 each, over 100.
	if got := sendMailCmd(z, a, "Other", 0, att(2, 1)); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_TOO_HEAVY {
		t.Errorf("heavy: %v", got)
	}
	if got := sendMailCmd(z, a, "Other", 0, att(4, 1)); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_RESTRICTED {
		t.Errorf("no_mail: %v", got)
	}
	if got := sendMailCmd(z, a, "Nobody", 0, att(0, 1)); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_NO_RECIPIENT {
		t.Errorf("no recipient: %v", got)
	}
	// 1000 zeny, its fee of 2%, and 2 items at 2500.
	a.record.Zeny = 6000
	if got := sendMailCmd(z, a, "Other", 1000, att(0, 4), att(1, 2)); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_ZENY {
		t.Errorf("too little zeny: %v", got)
	}
	a.record.Zeny = 7000
	cm.got = nil
	if got := sendMailCmd(z, a, "Other", 1000, att(0, 4), att(1, 2)); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_SENT {
		t.Fatalf("send: %v", got)
	}
	if a.record.Zeny != 7000-1000-20-5000 || inventoryCount(a.inventory, 909) != 6 || inventoryCount(a.inventory, 501) != 3 {
		t.Errorf("sender after: zeny %d, jellopy %d, potions %d", a.record.Zeny, inventoryCount(a.inventory, 909), inventoryCount(a.inventory, 501))
	}
	var writes, fees int
	for _, c := range cm.got {
		writes += len(c.writes.Mails)
		for _, l := range c.logs.Zeny {
			if l.To == transfer.End(transfer.KindFee) {
				fees += int(l.Amount)
			}
		}
	}
	if writes != 1 || fees != 5020 {
		t.Errorf("commit: %d mail writes, %d fees", writes, fees)
	}
	// The recipient sees the new mail at its next tick.
	z.stepAt(z.now + 50)
	nw := eventsOf(b, (*wathenav1.Event).GetMailNew, (*wathenav1.Event).HasMailNew)
	if len(nw) != 1 || nw[0].GetFrom() != a.Name || nw[0].GetUnread() != 1 {
		t.Fatalf("mail new %v", nw)
	}
	id := int64(nw[0].GetId())
	// mail_delay: the next mail is too soon.
	if got := sendMailCmd(z, a, "Other", 0); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_TOO_FAST {
		t.Errorf("too fast: %v", got)
	}
	z.now += 5000
	if got := sendMailCmd(z, a, "Other", 0); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_SENT {
		t.Errorf("second: %v", got)
	}
	z.now += 5000
	if got := sendMailCmd(z, a, "Other", 0); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_DAILY_LIMIT {
		t.Errorf("daily limit: %v", got)
	}
	// A mail with attachments cannot be deleted; taking empties it.
	z.mailCommand(b, wathenav1.MailCommand_builder{Delete: new(uint64(id))}.Build())
	z.stepAt(z.now + 50)
	if got := mailOutcomes(events(b)); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_NOT_EMPTY {
		t.Errorf("delete: %v", got)
	}
	b.record.Zeny = 0
	z.mailCommand(b, wathenav1.MailCommand_builder{Take: wathenav1.MailTake_builder{Mail: uint64(id), Zeny: true, Items: true}.Build()}.Build())
	z.commitTransfers()
	z.stepAt(z.now + 50)
	if got := mailOutcomes(events(b)); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_TAKEN {
		t.Fatalf("take: %v", got)
	}
	if b.record.Zeny != 1000 || inventoryCount(b.inventory, 909) != 4 || inventoryCount(b.inventory, 501) != 2 {
		t.Errorf("recipient: zeny %d, jellopy %d, potions %d", b.record.Zeny, inventoryCount(b.inventory, 909), inventoryCount(b.inventory, 501))
	}
	// Taking again does nothing.
	z.mailCommand(b, wathenav1.MailCommand_builder{Take: wathenav1.MailTake_builder{Mail: uint64(id), Zeny: true, Items: true}.Build()}.Build())
	z.stepAt(z.now + 50)
	if b.record.Zeny != 1000 {
		t.Error("taken twice")
	}
	z.mailCommand(b, wathenav1.MailCommand_builder{Delete: new(uint64(id))}.Build())
	z.stepAt(z.now + 50)
	if got := mailOutcomes(events(b)); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_DELETED {
		t.Errorf("delete after take: %v", got)
	}
}

// TestMailboxLimit refuses mail to a full mailbox.
func TestMailboxLimit(t *testing.T) {
	z, a, b, _ := marketZone(t, stack(0, 909, 10))
	r := rules.Base(true)
	r.Limits, _ = rules.LimitsOf(map[string]rules.RangeSet{"mailbox": {Base: 1, HasBase: true}})
	r.Market, _ = rules.MarketOf(map[string]int{"mail_delay": 1000})
	z.Mods = &Mods{Rules: r}
	z.mailboxOf(b)
	a.record.Zeny = 10_000
	if got := sendMailCmd(z, a, "Other", 0); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_SENT {
		t.Fatalf("first: %v", got)
	}
	z.stepAt(z.now + 50)
	z.now += 1000
	if got := sendMailCmd(z, a, "Other", 0, att(0, 1)); len(got) != 1 || got[0] != wathenav1.MailOutcome_MAIL_OUTCOME_MAILBOX_FULL {
		t.Errorf("full: %v", got)
	}
	if inventoryCount(a.inventory, 909) != 10 {
		t.Error("an item left for a full mailbox")
	}
}

// TestScriptMail sends a script's mail, whose items are made for it, and
// takes it.
func TestScriptMail(t *testing.T) {
	z, a, b, _ := marketZone(t)
	z.mailboxOf(b)
	err := z.sendScriptMail(a, modscript.ScriptMail{To: "Other", Sender: "Postman", Title: "Prize", Zeny: 50,
		Items: []modscript.ScriptMailItem{{Item: 501, Amount: 3, Bound: item.BoundCharacter}}})
	if err != nil {
		t.Fatal(err)
	}
	z.commitTransfers()
	z.stepAt(z.now + 50)
	nw := eventsOf(b, (*wathenav1.Event).GetMailNew, (*wathenav1.Event).HasMailNew)
	if len(nw) != 1 || nw[0].GetFrom() != "Postman" {
		t.Fatalf("new %v", nw)
	}
	b.record.Zeny = 0
	z.takeMail(b, int64(nw[0].GetId()), true, true)
	if b.record.Zeny != 50 || inventoryCount(b.inventory, 501) != 3 {
		t.Errorf("taken: %d zeny, %d potions", b.record.Zeny, inventoryCount(b.inventory, 501))
	}
	for _, s := range b.inventory.Slots {
		if s.ID == 501 && s.Bound != item.BoundCharacter {
			t.Error("the copy is not bound")
		}
	}
}

// TestAutotrade keeps a vendor without its connection: its store stays
// open and sells, and the vendor leaves once it sold out.
func TestAutotrade(t *testing.T) {
	z, a, b, _ := marketZone(t, stack(0, 909, 2))
	vendor(t, z, a, 0)
	openShopFor(z, a, "Away", vline(0, 2, 10))
	z.Autotraders = NewAutotraders()
	if !z.startAutotrade(a) {
		t.Fatal("autotrade")
	}
	if !a.detached || a.gone() || z.Autotraders.Len() != 1 {
		t.Fatalf("detached %v gone %v", a.detached, a.gone())
	}
	// The connection closes; the player stays.
	z.leave(a, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	if z.players[a.charID] != a || a.vend == nil {
		t.Fatal("the autotrader left")
	}
	b.record.Zeny = 100
	z.vendingCommand(b, wathenav1.VendingCommand_builder{Buy: wathenav1.VendingBuy_builder{Vendor: netIDOf(b, a), Shop: a.vend.id,
		Items: []*wathenav1.VendingBuyLine{wathenav1.VendingBuyLine_builder{Index: 0, Amount: 2}.Build()}}.Build()}.Build())
	if inventoryCount(b.inventory, 909) != 2 {
		t.Fatal("no sale")
	}
	if z.players[a.charID] != nil || z.Autotraders.Len() != 0 {
		t.Error("the autotrader stayed after it sold out")
	}
}
