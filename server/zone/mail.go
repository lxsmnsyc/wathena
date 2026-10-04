package zone

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mail"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Mail (RODEX), as rAthena's mail.cpp and the RODEX parts of clif.cpp
// (phase 5 step 7, proposal 0002, review decision 30):
//   - A player's mailbox comes from the world's mail registry
//     (server/mail) at login, and goes after the logout's last commit.
//   - A mail holds a title, a body, zeny and up to mail_items items.
//     Sending checks each attachment as mail_setitem does: the no_mail
//     restriction, bound and equipped copies, the count and
//     mail_attachment_weight. The sender pays the zeny, mail_zeny_fee
//     percent of it, and mail_attachment_price per item.
//   - A mail to a character who is not online goes to the database. The
//     recipient is found by name in the registry, or in the database on
//     another goroutine, after which the send finishes on the zone's.
//   - Every move of an attachment goes through server/transfer, and
//     joins one commit with the character and the mail's row. A mail
//     reaches its recipient's mailbox only after that commit is queued,
//     so the recipient's later commits come after it.
//   - The mail hook may refuse a player's mail. The mail event runs after
//     every mail.

// mailState is a player's mail.
type mailState struct {
	// box is the player's mailbox, and open is set while the window is.
	box  *mail.Mailbox
	open bool
	// next is when the player may send again, as cansendmail_tick.
	next time.Time
	// sending is set while a send waits for its recipient.
	sending bool
}

// mailRegistry is the world's mail registry, made on first use for a
// zone without one.
func (z *Zone) mailRegistry() *mail.Registry {
	if z.Mail == nil {
		z.Mail = mail.NewRegistry(nil, z.Items)
	}
	return z.Mail
}

// mailKey is the key of a character's mailbox.
func mailKey(id int64, name string) mail.Key { return mail.KeyOf(id, name) }

// acquireMailbox takes a session's mailbox from the registry. It runs on
// the goroutine that loads the character.
func (z *Zone) acquireMailbox(ctx context.Context, s *Session) error {
	if s.player == nil || s.player.load == nil {
		return nil
	}
	c := s.player.load
	b, err := z.mailRegistry().Acquire(ctx, mailKey(c.ID, c.Name), c.Name, z.limits().MailItems)
	if err != nil {
		return err
	}
	s.mailbox = b
	return nil
}

// mailboxOf is the player's mailbox: its session's, or one taken on
// first use for a player without a session, as in tests.
func (z *Zone) mailboxOf(p *Player) *mail.Mailbox {
	if p.sess != nil && p.sess.mailbox != nil {
		return p.sess.mailbox
	}
	if p.mail.box == nil {
		b, err := z.mailRegistry().Acquire(context.Background(), mailKey(p.record.ID, p.Name), p.Name, z.limits().MailItems)
		if err != nil {
			return nil
		}
		p.mail.box = b
	}
	return p.mail.box
}

// mailLeave lets go of a leaving player's mailbox after the next commit.
func (z *Zone) mailLeave(p *Player) {
	p.mail.open = false
	if p.sess != nil && p.sess.mailbox != nil {
		z.mailRelease = append(z.mailRelease, p.sess.mailbox)
		p.sess.mailbox = nil
	}
	if p.mail.box != nil {
		z.mailRelease = append(z.mailRelease, p.mail.box)
		p.mail.box = nil
	}
}

func mailResult(o wathenav1.MailOutcome, id int64, msg string) *wathenav1.Event {
	return wathenav1.Event_builder{MailResult: wathenav1.MailResult_builder{Outcome: o, Mail: uint64(max(id, 0)), // #nosec G115 -- checked.
		Message: msg}.Build()}.Build()
}

// mailBusy is mail_openmail's check: a storage, a store or a trade.
func mailBusy(p *Player) bool {
	return p.storage != nil || p.guild.storage != nil || p.hasStore() || p.trade != nil
}

// mailCommand runs a mail command.
func (z *Zone) mailCommand(p *Player, c *wathenav1.MailCommand) {
	switch {
	case c.HasOpen():
		z.openMail(p)
	case c.HasRead():
		z.readMail(p, int64(c.GetRead())) // #nosec G115 -- a mail ID.
	case c.HasSend():
		z.sendMail(p, c.GetSend())
	case c.HasTake():
		t := c.GetTake()
		z.takeMail(p, int64(t.GetMail()), t.GetZeny(), t.GetItems()) // #nosec G115 -- a mail ID.
	case c.HasDelete():
		z.deleteMail(p, int64(c.GetDelete())) // #nosec G115 -- a mail ID.
	case c.HasClose():
		p.mail.open = false
	}
}

// openMail is mail_openmail. It returns false when the player is busy.
func (z *Zone) openMail(p *Player) bool {
	if mailBusy(p) {
		p.pending = append(p.pending, mailResult(wathenav1.MailOutcome_MAIL_OUTCOME_BUSY, 0, ""))
		return false
	}
	p.mail.open = true
	z.sendMailBox(p)
	return true
}

// expiresAt is when a mail returns or is deleted, or zero for never.
func (z *Zone) expiresAt(m *mail.Mail) time.Time {
	r := z.marketRules()
	days := r.MailReturnDays
	if m.Returned || m.From == (mail.Key{}) {
		days = r.MailDeleteDays
	}
	if days <= 0 {
		return time.Time{}
	}
	return m.Sent.Add(time.Duration(days) * 24 * time.Hour)
}

// mailSummary is a mail as the list shows it. The lock must be held.
func (z *Zone) mailSummary(m *mail.Mail) *wathenav1.MailSummary {
	b := wathenav1.MailSummary_builder{Id: uint64(m.ID), From: m.FromName, Title: m.Title, SentMs: m.Sent.UnixMilli(), // #nosec G115 -- IDs from 1.
		Read: m.Read, Zeny: uint32(m.Zeny), Items: uint32(m.Count()), Returned: m.Returned} // #nosec G115 -- at most MAX_ZENY and mail_items.
	if t := z.expiresAt(m); !t.IsZero() {
		b.ExpiresMs = t.UnixMilli()
	}
	return b.Build()
}

// sendMailBox sends the mailbox's list, newest first.
func (z *Zone) sendMailBox(p *Player) {
	b := z.mailboxOf(p)
	if b == nil {
		return
	}
	mails := b.Mails()
	list := make([]*wathenav1.MailSummary, 0, len(mails))
	for i := len(mails) - 1; i >= 0; i-- {
		m := mails[i]
		m.Lock()
		if !m.Deleted() {
			list = append(list, z.mailSummary(m))
		}
		m.Unlock()
	}
	r := z.marketRules()
	sent := 0
	if p.record.MailDay == mailDay(z.wallClock()) {
		sent = p.record.MailSent
	}
	p.pending = append(p.pending, wathenav1.Event_builder{MailBox: wathenav1.MailBox_builder{
		Mails: list, Size: uint32(z.limits().Mailbox), SentToday: uint32(sent), Daily: uint32(r.MailDailyCount), // #nosec G115 -- small limits.
	}.Build()}.Build())
}

// mailDay is the day as YYYYMMDD, for mail_daily_count.
func mailDay(t time.Time) int {
	y, m, d := t.Date()
	return y*10000 + int(m)*100 + d
}

// readMail is clif_Mail_read.
func (z *Zone) readMail(p *Player, id int64) {
	b := z.mailboxOf(p)
	if b == nil {
		return
	}
	m := b.Find(id)
	if m == nil {
		p.pending = append(p.pending, mailResult(wathenav1.MailOutcome_MAIL_OUTCOME_NOT_FOUND, id, ""))
		return
	}
	m.Lock()
	defer m.Unlock()
	if m.Deleted() {
		p.pending = append(p.pending, mailResult(wathenav1.MailOutcome_MAIL_OUTCOME_NOT_FOUND, id, ""))
		return
	}
	if !m.Read {
		m.Read = true
		m.Touch()
		z.mailChanged(m)
	}
	z.sendMailRead(p, m)
}

// sendMailRead shows one mail. The lock must be held.
func (z *Zone) sendMailRead(p *Player, m *mail.Mail) {
	var items []*wathenav1.InventoryItem
	for i := range m.Box.Slots {
		if s := &m.Box.Slots[i]; s.ID != 0 {
			items = append(items, z.itemMessage(i, s, s.Amount))
		}
	}
	p.pending = append(p.pending, wathenav1.Event_builder{MailRead: wathenav1.MailRead_builder{
		Mail: z.mailSummary(m), Body: m.Body, Items: items}.Build()}.Build())
}

// mailChanged puts a mail in the next commit.
func (z *Zone) mailChanged(m *mail.Mail) {
	if !slices.Contains(z.txMails, m) {
		z.txMails = append(z.txMails, m)
	}
}

// mailWrites are the rows of the mails the zone changed since the last
// commit.
func (z *Zone) mailWrites() []mail.Write {
	var out []mail.Write
	for _, m := range z.txMails {
		m.Lock()
		if w, ok := m.Snapshot(); ok {
			out = append(out, w)
		}
		m.Unlock()
	}
	z.txMails = nil
	return out
}

// deliverMails puts the mails sent since the last commit in their
// recipients' mailboxes, once the commit is queued.
func (z *Zone) deliverMails() {
	for _, m := range z.mailDeliver {
		z.mailRegistry().Deliver(m)
	}
	z.mailDeliver = nil
	for _, b := range z.mailRelease {
		z.mailRegistry().Release(b)
	}
	z.mailRelease = nil
}

// mailNotices tells online players of the mails that arrived, as
// clif_Mail_new. It runs each tick.
func (z *Zone) mailNotices() {
	for _, p := range z.players {
		if p.detached {
			continue
		}
		b := z.mailboxOf(p)
		if b == nil {
			continue
		}
		fresh := b.Fresh()
		if len(fresh) == 0 {
			continue
		}
		unread := 0
		for _, m := range b.Mails() {
			m.Lock()
			if !m.Read && !m.Deleted() {
				unread++
			}
			m.Unlock()
		}
		for _, m := range fresh {
			m.Lock()
			p.pending = append(p.pending, wathenav1.Event_builder{MailNew: wathenav1.MailNew_builder{
				Id: uint64(m.ID), From: m.FromName, Title: m.Title, Unread: uint32(unread), // #nosec G115 -- IDs from 1, and a count.
			}.Build()}.Build())
			m.Unlock()
		}
		if p.mail.open {
			z.sendMailBox(p)
		}
	}
}

// mailAttach is one attachment a player sends.
type mailAttach struct {
	slot   int
	serial item.Serial
	amount int
}

// pendingMail is a player's mail that waits for its recipient.
type pendingMail struct {
	to          string
	title, body string
	zeny        int64
	items       []mailAttach
}

// sendMail is mail_send with mail_setitem's checks of each attachment.
func (z *Zone) sendMail(p *Player, s *wathenav1.MailSend) {
	fail := func(o wathenav1.MailOutcome) {
		p.pending = append(p.pending, mailResult(o, 0, ""))
	}
	if p.trade != nil || p.mail.sending {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_FAILED)
		return
	}
	now := z.wallClock()
	if now.Before(p.mail.next) {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_TOO_FAST)
		return
	}
	r := z.marketRules()
	if r.MailDailyCount > 0 && p.record.MailDay == mailDay(now) && p.record.MailSent >= r.MailDailyCount {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_DAILY_LIMIT)
		return
	}
	title, body, to := strings.TrimSpace(s.GetTitle()), s.GetBody(), strings.TrimSpace(s.GetTo())
	if title == "" || len(s.GetTitle()) > mail.MaxTitle || len(body) > mail.MaxBody || !utf8.ValidString(title) ||
		!utf8.ValidString(body) || to == "" || utf8.RuneCountInString(to) > mail.MaxName {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_FAILED)
		return
	}
	pm := pendingMail{to: to, title: title, body: body, zeny: int64(s.GetZeny())}
	if o := z.checkAttachments(p, s.GetItems(), &pm); o != wathenav1.MailOutcome_MAIL_OUTCOME_UNSPECIFIED {
		fail(o)
		return
	}
	reg := z.mailRegistry()
	if rc, found, ok := reg.Find(to, 0); ok {
		if !found {
			fail(wathenav1.MailOutcome_MAIL_OUTCOME_NO_RECIPIENT)
			return
		}
		z.finishMail(p, &pm, rc)
		return
	}
	// The database knows the recipient: ask on another goroutine, then
	// finish on the zone's.
	p.mail.sending = true
	loader := reg.Loader
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		rc, found, err := loader.FindRecipient(ctx, to, 0)
		cancel()
		z.post(func() {
			p.mail.sending = false
			if z.players[p.charID] != p || p.gone() {
				return
			}
			switch {
			case err != nil:
				z.warn("cannot find a mail's recipient", "err", err)
				p.pending = append(p.pending, mailResult(wathenav1.MailOutcome_MAIL_OUTCOME_FAILED, 0, ""))
			case !found:
				p.pending = append(p.pending, mailResult(wathenav1.MailOutcome_MAIL_OUTCOME_NO_RECIPIENT, 0, ""))
			default:
				z.finishMail(p, &pm, rc)
			}
		})
	}()
}

// post runs f on the zone's goroutine. It does not block a zone that
// stopped.
func (z *Zone) post(f func()) {
	if z.sends == nil {
		// A zone without its goroutine, as in tests: run it now.
		f()
		return
	}
	select {
	case z.control <- f:
	case <-z.stopped:
	}
}

// checkAttachments checks the attachments as mail_setitem does, and
// keeps them in pm. It returns the outcome of the first that fails.
func (z *Zone) checkAttachments(p *Player, moves []*wathenav1.StorageMove, pm *pendingMail) wathenav1.MailOutcome {
	r := z.marketRules()
	if len(moves) > z.limits().MailItems {
		return wathenav1.MailOutcome_MAIL_OUTCOME_TOO_MANY
	}
	weight := 0
	for _, mv := range moves {
		slot, amount := int(mv.GetSlot()), int(mv.GetAmount())
		if slot < 0 || slot >= len(p.inventory.Slots) || amount < 1 {
			return wathenav1.MailOutcome_MAIL_OUTCOME_FAILED
		}
		s := &p.inventory.Slots[slot]
		taken := 0
		for _, a := range pm.items {
			if a.slot == slot {
				taken += a.amount
			}
		}
		if s.ID == 0 || s.Amount < amount+taken {
			return wathenav1.MailOutcome_MAIL_OUTCOME_FAILED
		}
		if s.Equip != 0 || z.Items.Restricted(s, item.NoMail) || !item.BoundAllows(s.Bound, item.PlaceMail, false) {
			return wathenav1.MailOutcome_MAIL_OUTCOME_RESTRICTED
		}
		weight += amount * (z.Items.Get(s.ID).Weight / 10)
		if r.MailAttachmentWeight > 0 && weight > r.MailAttachmentWeight {
			return wathenav1.MailOutcome_MAIL_OUTCOME_TOO_HEAVY
		}
		if i := slices.IndexFunc(pm.items, func(a mailAttach) bool { return a.slot == slot }); i >= 0 {
			pm.items[i].amount += amount
		} else {
			pm.items = append(pm.items, mailAttach{slot: slot, serial: s.Serial, amount: amount})
		}
	}
	if pm.zeny < 0 || pm.zeny > transfer.MaxZeny || mailCost(r.MailZenyFee, r.MailAttachmentPrice, pm.zeny, len(pm.items)) > p.record.Zeny {
		return wathenav1.MailOutcome_MAIL_OUTCOME_ZENY
	}
	return wathenav1.MailOutcome_MAIL_OUTCOME_UNSPECIFIED
}

// mailCost is what a mail costs its sender: the zeny, mail_zeny_fee
// percent of it, and mail_attachment_price per item, as
// mail_setattachment.
func mailCost(feePercent, perItem, zeny int64, items int) int64 {
	return zeny + zeny*feePercent/100 + int64(items)*perItem
}

// finishMail sends a checked mail to a found recipient: the hook, then
// one transfer of the attachments, the zeny and the fees.
func (z *Zone) finishMail(p *Player, pm *pendingMail, rc mail.Recipient) {
	fail := func(o wathenav1.MailOutcome, msg string) {
		p.pending = append(p.pending, mailResult(o, 0, msg))
	}
	if rc.Mails >= z.limits().Mailbox {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_MAILBOX_FULL, "")
		return
	}
	// The attachments may have changed while the recipient was found.
	for _, a := range pm.items {
		s := &p.inventory.Slots[a.slot]
		if s.Serial != a.serial || s.Amount < a.amount || s.Equip != 0 {
			fail(wathenav1.MailOutcome_MAIL_OUTCOME_FAILED, "")
			return
		}
	}
	r := z.marketRules()
	if mailCost(r.MailZenyFee, r.MailAttachmentPrice, pm.zeny, len(pm.items)) > p.record.Zeny {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_ZENY, "")
		return
	}
	var sold []modscript.SaleItem
	for _, a := range pm.items {
		sold = append(sold, modscript.SaleItem{Item: p.inventory.Slots[a.slot].ID, Amount: a.amount})
	}
	ev := modscript.SaleEvent{From: p.Name, To: rc.Name, Items: sold, Zeny: pm.zeny}
	if sr := z.scripts; sr != nil && sr.HasHooks("mail") {
		if ok, msg := sr.RunSaleHook("mail", ev); !ok {
			fail(wathenav1.MailOutcome_MAIL_OUTCOME_HOOK, msg)
			return
		}
	}
	now := z.wallClock()
	m := z.mailRegistry().NewMail(z.limits().MailItems)
	m.Lock()
	m.From, m.FromName, m.To, m.ToName = mailKey(p.record.ID, p.Name), p.Name, rc.Key, rc.Name
	m.Title, m.Body, m.Sent = pm.title, pm.body, now
	box := z.box(p)
	tx := transfer.New(transfer.ActionMail)
	for _, a := range pm.items {
		tx.Move(box, a.slot, a.serial, a.amount, &m.Box)
	}
	if pm.zeny > 0 {
		tx.MoveZeny(z.wallet(p), pm.zeny, &transfer.ZenyWallet{Who: m.Owner(), Z: &m.Zeny})
	}
	if fee := mailCost(r.MailZenyFee, r.MailAttachmentPrice, pm.zeny, len(pm.items)) - pm.zeny; fee > 0 {
		tx.Pay(z.wallet(p), fee, transfer.End(transfer.KindFee))
	}
	err := z.apply(p, tx, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_MAILED)
	m.Touch()
	m.Unlock()
	if err != nil {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_FAILED, "")
		return
	}
	z.countMail(p, now)
	z.mailChanged(m)
	z.mailDeliver = append(z.mailDeliver, m)
	if !p.transferred {
		p.transferred = true
		z.txPlayers = append(z.txPlayers, p)
	}
	p.pending = append(p.pending, mailResult(wathenav1.MailOutcome_MAIL_OUTCOME_SENT, m.ID, ""))
	z.scriptEvents = append(z.scriptEvents, scriptEvent{name: "mail", sale: &ev})
}

// countMail counts a mail for mail_daily_count and mail_delay.
func (z *Zone) countMail(p *Player, now time.Time) {
	day := mailDay(now)
	if p.record.MailDay != day {
		p.record.MailDay, p.record.MailSent = day, 0
	}
	p.record.MailSent++
	p.mail.next = now.Add(time.Duration(z.marketRules().MailDelay) * time.Millisecond)
}

// takeMail is clif_parse_Mail_getattach and mail_getattachment: the
// zeny, the items, or both, move to the player in one transfer.
func (z *Zone) takeMail(p *Player, id int64, zeny, items bool) {
	fail := func(o wathenav1.MailOutcome) { p.pending = append(p.pending, mailResult(o, id, "")) }
	b := z.mailboxOf(p)
	if b == nil || p.trade != nil {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_FAILED)
		return
	}
	m := b.Find(id)
	if m == nil {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_NOT_FOUND)
		return
	}
	m.Lock()
	defer m.Unlock()
	if m.Deleted() {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_NOT_FOUND)
		return
	}
	zeny = zeny && m.Zeny > 0
	items = items && m.HasItems()
	if !zeny && !items {
		return
	}
	if zeny && m.Zeny+p.record.Zeny > transfer.MaxZeny {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_CANNOT_CARRY)
		return
	}
	tx := transfer.New(transfer.ActionMail)
	if zeny {
		tx.MoveZeny(&transfer.ZenyWallet{Who: m.Owner(), Z: &m.Zeny}, m.Zeny, z.wallet(p))
	}
	if items {
		box := z.box(p)
		for i := range m.Box.Slots {
			if s := &m.Box.Slots[i]; s.ID != 0 {
				tx.Move(&m.Box, i, s.Serial, s.Amount, box)
			}
		}
	}
	if err := z.apply(p, tx, 0); err != nil {
		fail(wathenav1.MailOutcome_MAIL_OUTCOME_CANNOT_CARRY)
		return
	}
	m.Touch()
	z.mailChanged(m)
	if !p.transferred {
		p.transferred = true
		z.txPlayers = append(z.txPlayers, p)
	}
	p.pending = append(p.pending, mailResult(wathenav1.MailOutcome_MAIL_OUTCOME_TAKEN, id, ""))
	z.sendMailRead(p, m)
	if p.mail.open {
		m.Unlock()
		z.sendMailBox(p)
		m.Lock()
	}
}

// deleteMail is clif_parse_Mail_delete: a mail with zeny or items stays.
func (z *Zone) deleteMail(p *Player, id int64) {
	b := z.mailboxOf(p)
	if b == nil {
		return
	}
	m := b.Find(id)
	if m == nil {
		p.pending = append(p.pending, mailResult(wathenav1.MailOutcome_MAIL_OUTCOME_NOT_FOUND, id, ""))
		return
	}
	m.Lock()
	if m.Zeny > 0 || m.HasItems() {
		m.Unlock()
		p.pending = append(p.pending, mailResult(wathenav1.MailOutcome_MAIL_OUTCOME_NOT_EMPTY, id, ""))
		return
	}
	m.Delete()
	m.Unlock()
	b.Remove(id)
	z.mailChanged(m)
	p.pending = append(p.pending, mailResult(wathenav1.MailOutcome_MAIL_OUTCOME_DELETED, id, ""))
	if p.mail.open {
		z.sendMailBox(p)
	}
}

// sendScriptMail is player:send_mail: a mail from a script, whose items
// are made for it, to a character online or not. The mail event runs
// once it went out.
func (z *Zone) sendScriptMail(p *Player, sm modscript.ScriptMail) error {
	return z.mailFrom(p, sm, transfer.ActionMail, transfer.End(transfer.KindScript), true)
}

// mailFrom sends a mail whose items and zeny are made for it, from an
// end such as a script or a world event, with action in the logs. The
// mail event runs for it when event is set.
func (z *Zone) mailFrom(p *Player, sm modscript.ScriptMail, action transfer.Action, from transfer.Owner, event bool) error {
	reg := z.mailRegistry()
	finish := func(rc mail.Recipient) {
		m := reg.NewMail(max(z.limits().MailItems, len(sm.Items)))
		m.Lock()
		m.FromName, m.To, m.ToName = sm.Sender, rc.Key, rc.Name
		m.Title, m.Body, m.Sent, m.Zeny = sm.Title, sm.Body, z.wallClock(), 0
		tx := transfer.New(action)
		if z.Clock != nil {
			tx.Now = z.Clock()
		}
		var sold []modscript.SaleItem
		for _, it := range sm.Items {
			d, ok := z.Items.Lookup(it.Item)
			if !ok {
				continue
			}
			step := it.Amount
			if d.UniqueID || !d.Stackable {
				step = 1
			}
			for given := 0; given < it.Amount; given += step {
				x := z.rolledExtra(d, it.Roll)
				x.Bound = it.Bound
				tx.Create(from, item.Slot{ID: it.Item, Extra: x}, step, &m.Box, item.Serial{})
			}
			sold = append(sold, modscript.SaleItem{Item: it.Item, Amount: it.Amount})
		}
		if sm.Zeny > 0 {
			tx.Earn(from, sm.Zeny, &transfer.ZenyWallet{Who: m.Owner(), Z: &m.Zeny}, true)
		}
		logs, err := tx.Apply(nil)
		m.Touch()
		m.Unlock()
		if err != nil {
			z.warn("a script's mail did not fit", "to", rc.Name, "err", err)
			return
		}
		z.txLogs.Add(logs)
		z.mailChanged(m)
		z.mailDeliver = append(z.mailDeliver, m)
		if event {
			z.scriptEvents = append(z.scriptEvents, scriptEvent{name: "mail", sale: &modscript.SaleEvent{
				From: sm.Sender, To: rc.Name, Items: sold, Zeny: sm.Zeny}})
		}
	}
	name, id := sm.To, sm.ToID
	if name == "" && id == 0 {
		finish(mail.Recipient{Key: mailKey(p.record.ID, p.Name), Name: p.Name})
		return nil
	}
	if rc, found, ok := reg.Find(name, id); ok {
		if found {
			finish(rc)
		} else {
			z.warn("a script's mail has no recipient", "to", name, "id", id)
		}
		return nil
	}
	loader := reg.Loader
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		rc, found, err := loader.FindRecipient(ctx, name, id)
		cancel()
		z.post(func() {
			switch {
			case err != nil:
				z.warn("cannot find a mail's recipient", "err", err)
			case !found:
				z.warn("a script's mail has no recipient", "to", name, "id", id)
			default:
				finish(rc)
				z.runScripts()
			}
		})
	}()
	return nil
}

// rolledExtra is the extra of a new copy with a give_item roll option.
func (z *Zone) rolledExtra(d *item.Def, roll modscript.GiveRoll) item.Extra {
	x := z.newExtra(d, roll.Mode == modscript.RollRandom, rollSource{kind: sourceScript}, "")
	if roll.Mode == modscript.RollSet {
		for name, v := range roll.Values {
			if r, ok := d.Rolls[name]; ok {
				x.Rolls = x.Rolls.With(name, min(max(v, r.Min), r.Max))
			}
		}
	}
	return x
}
