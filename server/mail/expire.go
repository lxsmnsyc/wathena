package mail

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Expiry, as int_mail.cpp's mail_return_timer and mail_delete_timer:
//   - A mail older than mail_return_days goes back to its sender: its
//     items and zeny move to a new mail of the sender, from the
//     recipient, with "RE:" before the title. A script's mail, which has
//     no sender, is deleted instead. With mail_return_empty off, a mail
//     without attachments is deleted instead.
//   - A mail that came back, or a script's mail, is deleted
//     mail_delete_days after it was sent or came back. Its items go with
//     it, to the end used.
//
// Each change moves through server/transfer and goes to the commit
// function in one commit with its log lines.

// ExpiryLoader finds the saved mails that are due, for mailboxes not in
// memory.
type ExpiryLoader interface {
	// ExpiredMails returns, with their items, up to limit mails sent
	// before returnBefore that did not come back, and mails that came
	// back, or have no sender, sent before deleteBefore. A zero time
	// leaves its kind out.
	ExpiredMails(ctx context.Context, returnBefore, deleteBefore time.Time, limit int) ([]Row, error)
}

// Expiry are the rules of expiry.
type Expiry struct {
	// ReturnDays and DeleteDays are mail_return_days and
	// mail_delete_days, 0 for never.
	ReturnDays, DeleteDays int
	// ReturnEmpty is mail_return_empty.
	ReturnEmpty bool
	// Slots is mail_items, for the new mails' boxes.
	Slots int
}

// due reports whether a mail is due at now, and whether it returns.
func (e Expiry) due(m *Mail, now time.Time) (due, ret bool) {
	day := 24 * time.Hour
	if m.Returned || m.From == (Key{}) {
		return e.DeleteDays > 0 && !now.Before(m.Sent.Add(time.Duration(e.DeleteDays)*day)), false
	}
	if e.ReturnDays == 0 || now.Before(m.Sent.Add(time.Duration(e.ReturnDays)*day)) {
		return false, false
	}
	return true, e.ReturnEmpty || m.HasItems() || m.Zeny > 0
}

// expiryBatch is how many saved mails one run reads, as
// MAIL_ITERATION_SIZE.
const expiryBatch = 100

// Expire returns and deletes the mails that are due at now: those of the
// mailboxes in memory, and up to 100 saved ones of mailboxes that are
// not. commit gets the writes and the log lines.
func (r *Registry) Expire(ctx context.Context, now time.Time, e Expiry, commit func([]Write, transfer.Logs)) error {
	if e.ReturnDays == 0 && e.DeleteDays == 0 {
		return nil
	}
	var due []*Mail
	seen := map[int64]bool{}
	// copies are the mails read from rows, whose mailboxes were not in
	// memory.
	copies := map[*Mail]bool{}
	r.mu.Lock()
	for _, b := range r.boxes {
		b.mu.Lock()
		for _, m := range b.mails {
			m.Lock()
			if d, _ := e.due(m, now); d && !m.deleted {
				due = append(due, m)
				seen[m.ID] = true
			}
			m.Unlock()
		}
		b.mu.Unlock()
	}
	r.mu.Unlock()
	if el, ok := r.Loader.(ExpiryLoader); ok && r.Loader != nil {
		var rb, db time.Time
		if e.ReturnDays > 0 {
			rb = now.Add(-time.Duration(e.ReturnDays) * 24 * time.Hour)
		}
		if e.DeleteDays > 0 {
			db = now.Add(-time.Duration(e.DeleteDays) * 24 * time.Hour)
		}
		rows, err := el.ExpiredMails(ctx, rb, db, expiryBatch)
		if err != nil {
			return err
		}
		for i := range rows {
			if seen[rows[i].ID] {
				continue
			}
			// A mailbox that loaded meanwhile holds the live copy.
			if b := r.Loaded(rows[i].To); b != nil {
				if m := b.Find(rows[i].ID); m != nil {
					due = append(due, m)
					seen[m.ID] = true
				}
				continue
			}
			r.mu.Lock()
			if r.gone[rows[i].ID] {
				r.mu.Unlock()
				continue
			}
			r.mu.Unlock()
			m := FromRow(&rows[i], r.Items, max(e.Slots, len(rows[i].Items)))
			copies[m] = true
			due = append(due, m)
		}
	}
	slices.SortFunc(due, func(a, b *Mail) int { return int(a.ID - b.ID) })
	for _, m := range due {
		r.expireOne(m, copies[m], now, e, commit)
	}
	return nil
}

// expireOne returns or deletes one mail. fromRow is set for a mail read
// from its row, whose mailbox was not in memory: it is marked gone
// before it changes, unless its mailbox loaded meanwhile, such as for a
// restore (decision 0029), which then holds the live mail. Either a
// load leaves the mail out, or the expiry leaves it to the next run.
func (r *Registry) expireOne(m *Mail, fromRow bool, now time.Time, e Expiry, commit func([]Write, transfer.Logs)) {
	if fromRow {
		r.mu.Lock()
		if r.boxes[m.To] != nil || r.gone[m.ID] {
			r.mu.Unlock()
			return
		}
		if r.gone == nil {
			r.gone = map[int64]bool{}
		}
		r.gone[m.ID] = true
		r.mu.Unlock()
	}
	m.Lock()
	d, ret := e.due(m, now)
	if !d || m.deleted {
		m.Unlock()
		r.unmark(m, fromRow)
		return
	}
	tx := transfer.New(transfer.ActionMail)
	tx.Now = now
	var back *Mail
	if ret {
		back = r.NewMail(max(e.Slots, m.Count()))
		back.From, back.FromName, back.To, back.ToName = m.To, m.ToName, m.From, m.FromName
		title := "RE:" + m.Title
		if len(title) > MaxTitle {
			title = title[:MaxTitle]
			for !validUTF8(title) {
				title = title[:len(title)-1]
			}
		}
		back.Title, back.Body, back.Sent, back.Returned = title, m.Body, now, true
		for i := range m.Box.Slots {
			if s := &m.Box.Slots[i]; s.ID != 0 {
				tx.Move(&m.Box, i, s.Serial, s.Amount, &back.Box)
			}
		}
		if m.Zeny > 0 {
			tx.MoveZeny(&transfer.ZenyWallet{Who: m.Owner(), Z: &m.Zeny}, m.Zeny, &transfer.ZenyWallet{Who: back.Owner(), Z: &back.Zeny})
		}
	} else {
		for i := range m.Box.Slots {
			if s := &m.Box.Slots[i]; s.ID != 0 {
				tx.Destroy(&m.Box, i, s.Serial, s.Amount, transfer.End(transfer.KindUsed))
			}
		}
		if m.Zeny > 0 {
			tx.Pay(&transfer.ZenyWallet{Who: m.Owner(), Z: &m.Zeny}, m.Zeny, transfer.End(transfer.KindUsed))
		}
	}
	logs, err := tx.Apply(nil)
	if err != nil {
		m.Unlock()
		r.unmark(m, fromRow)
		return
	}
	m.Delete()
	var writes []Write
	if w, ok := m.Snapshot(); ok {
		writes = append(writes, w)
	}
	to := m.To
	m.Unlock()
	if back != nil {
		back.Lock()
		if w, ok := back.Snapshot(); ok {
			writes = append(writes, w)
		}
		back.Unlock()
	}
	r.mu.Lock()
	if r.gone == nil {
		r.gone = map[int64]bool{}
	}
	r.gone[m.ID] = true
	r.mu.Unlock()
	commit(writes, logs)
	if b := r.Loaded(to); b != nil {
		b.Remove(m.ID)
	}
	if back != nil {
		r.Deliver(back)
	}
}

// unmark takes back the gone mark of a copy that did not change.
func (r *Registry) unmark(m *Mail, fromRow bool) {
	if !fromRow {
		return
	}
	r.mu.Lock()
	delete(r.gone, m.ID)
	r.mu.Unlock()
}

func validUTF8(s string) bool { return strings.ToValidUTF8(s, "�") == s }
