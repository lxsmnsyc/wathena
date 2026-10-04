package mail

import (
	"context"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

func testItems() *item.Table {
	return item.New(&item.Data{Mode: "renewal", Items: []item.Def{{ID: 909, AegisName: "Jellopy", Type: "Etc", Weight: 10, Stackable: true}}})
}

// mailTo makes a mail from one key to another with an item and zeny,
// in the recipient's mailbox.
func mailTo(t *testing.T, r *Registry, from, to Key, sent time.Time, items, zeny int) *Mail {
	t.Helper()
	m := r.NewMail(5)
	m.From, m.FromName, m.To, m.ToName, m.Title, m.Sent, m.Zeny = from, "Alice", to, "Bob", "Hi", sent, int64(zeny)
	if items > 0 {
		tx := transfer.New(transfer.ActionMail).Create(transfer.End(transfer.KindScript), item.Slot{ID: 909}, items, &m.Box, item.Serial{})
		if _, err := tx.Apply(nil); err != nil {
			t.Fatal(err)
		}
	}
	r.Deliver(m)
	return m
}

// TestExpiry checks mail_return_days, mail_delete_days and
// mail_return_empty: a mail goes back to its sender with its items and
// zeny, a mail that came back is deleted later, a script's mail is
// deleted, and an empty mail is deleted when mail_return_empty is off.
func TestExpiry(t *testing.T) {
	r := NewRegistry(nil, testItems())
	alice, bob := KeyOf(0, "Alice"), KeyOf(0, "Bob")
	r.names["alice"], r.names["bob"] = "Alice", "Bob"
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	full := mailTo(t, r, alice, bob, start, 3, 100)
	empty := mailTo(t, r, alice, bob, start, 0, 0)
	script := mailTo(t, r, Key{}, bob, start, 1, 0)
	e := Expiry{ReturnDays: 15, DeleteDays: 15, ReturnEmpty: false, Slots: 5}
	var writes []Write
	var logs transfer.Logs
	commit := func(w []Write, l transfer.Logs) { writes = append(writes, w...); logs.Add(l) }
	// Not yet due.
	if err := r.Expire(context.Background(), start.Add(14*24*time.Hour), e, commit); err != nil || len(writes) != 0 {
		t.Fatalf("early: %v %v", writes, err)
	}
	if err := r.Expire(context.Background(), start.Add(15*24*time.Hour), e, commit); err != nil {
		t.Fatal(err)
	}
	// A script's mail has no sender: it is deleted after mail_delete_days,
	// which is due too.
	if !full.Deleted() || !empty.Deleted() || !script.Deleted() {
		t.Errorf("deleted: full %v, empty %v, script %v", full.Deleted(), empty.Deleted(), script.Deleted())
	}
	back := r.box(alice).Mails()
	if len(back) != 1 || !back[0].Returned || back[0].Zeny != 100 || back[0].Count() != 1 || back[0].Title != "RE:Hi" || back[0].To != alice {
		t.Fatalf("returned %+v", back)
	}
	if len(r.box(bob).Mails()) != 0 {
		t.Errorf("bob keeps %d mails", len(r.box(bob).Mails()))
	}
	// The returned mail is deleted 15 days after it came back.
	if err := r.Expire(context.Background(), start.Add(30*24*time.Hour), e, commit); err != nil {
		t.Fatal(err)
	}
	if len(r.box(alice).Mails()) != 0 {
		t.Error("the returned mail stays")
	}
	moved, destroyed := 0, 0
	for _, l := range logs.Items {
		switch l.To.Kind {
		case transfer.KindMail:
			moved += l.Amount
		case transfer.KindUsed:
			destroyed += l.Amount
		}
	}
	if moved != 3 || destroyed != 4 {
		t.Errorf("logs: %d moved, %d destroyed", moved, destroyed)
	}
	// With mail_return_empty on, an empty mail goes back.
	e.ReturnEmpty = true
	empty2 := mailTo(t, r, alice, bob, start.Add(30*24*time.Hour), 0, 0)
	if err := r.Expire(context.Background(), start.Add(45*24*time.Hour), e, commit); err != nil {
		t.Fatal(err)
	}
	if !empty2.Deleted() || len(r.box(alice).Mails()) != 1 {
		t.Errorf("empty mail with mail_return_empty: deleted %v, alice has %d", empty2.Deleted(), len(r.box(alice).Mails()))
	}
}

// fakeLoader is a database of mail rows by recipient.
type fakeLoader struct{ rows map[int64][]Row }

func (f *fakeLoader) LoadMailbox(_ context.Context, character int64) ([]Row, error) {
	return f.rows[character], nil
}

func (f *fakeLoader) FindRecipient(context.Context, string, int64) (Recipient, bool, error) {
	return Recipient{}, false, nil
}

// TestDeletedMailStaysDeleted: a mail sent to a mailbox that was not
// loaded waits in recent. Its recipient logs in, takes it and deletes
// it, and logs in again. The mail must not come back from recent with
// its items, which the recipient already took, and the audit must not
// count recent's copy of it. The phase 5 exit check found this.
func TestDeletedMailStaysDeleted(t *testing.T) {
	db := &fakeLoader{rows: map[int64][]Row{}}
	r := NewRegistry(db, testItems())
	alice, bob := KeyOf(1, "alice"), KeyOf(2, "bob")
	sent := mailTo(t, r, alice, bob, time.Now(), 3, 100)
	if a := r.Audit(); len(a) != 1 || a[0].Zeny != 100 {
		t.Fatalf("audit of the mail in recent: %+v", a)
	}
	// Its commit is written: the row holds the mail.
	sent.Lock()
	w, _ := sent.Snapshot()
	sent.Unlock()
	db.rows[2] = []Row{*w.Row}
	box, err := r.Acquire(context.Background(), bob, "Bob", 5)
	if err != nil {
		t.Fatal(err)
	}
	// Bob takes the zeny and the items from the loaded copy, and deletes
	// the mail. Its commit removes the row.
	m := box.Find(sent.ID)
	m.Lock()
	m.Zeny = 0
	m.Box.Slots = nil
	m.Delete()
	m.Unlock()
	box.Remove(sent.ID)
	delete(db.rows, 2)
	if a := r.Audit(); len(a) != 0 {
		t.Errorf("audit after the take and the delete: %+v", a)
	}
	r.Release(box)
	box, err = r.Acquire(context.Background(), bob, "Bob", 5)
	if err != nil {
		t.Fatal(err)
	}
	if got := box.Mails(); len(got) != 0 {
		t.Errorf("the deleted mail came back with %d zeny", got[0].Zeny)
	}
}

// TestMailSentWhileLoading: a mail sent while its recipient's mailbox
// loads, after the load read the rows, still reaches the mailbox.
func TestMailSentWhileLoading(t *testing.T) {
	r := NewRegistry(nil, testItems())
	alice, bob := KeyOf(1, "alice"), KeyOf(2, "bob")
	var sent *Mail
	r.Loader = loaderFunc(func() { sent = mailTo(t, r, alice, bob, time.Now(), 1, 5) })
	box, err := r.Acquire(context.Background(), bob, "Bob", 5)
	if err != nil {
		t.Fatal(err)
	}
	if got := box.Mails(); len(got) != 1 || got[0] != sent {
		t.Errorf("mailbox %v, want the mail sent while it loaded", got)
	}
}

// loaderFunc runs a function while it loads, and finds no rows.
type loaderFunc func()

func (f loaderFunc) LoadMailbox(context.Context, int64) ([]Row, error) {
	f()
	return nil, nil
}

func (f loaderFunc) FindRecipient(context.Context, string, int64) (Recipient, bool, error) {
	return Recipient{}, false, nil
}
