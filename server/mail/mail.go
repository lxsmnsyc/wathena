// Package mail keeps mail (RODEX) while the world runs (phase 5 step 7,
// proposal 0002, review decision 30), as rAthena's mail.cpp and
// int_mail.cpp:
//   - A mailbox is a character's mails. It loads when the character logs
//     in, and stays while the character is online. A mail to a character
//     that is offline goes to the database only, and shows at its next
//     login.
//   - A mail holds a title, a body, zeny and up to mail_items items. Its
//     items are a transfer box of kind mail, so every move of an
//     attachment goes through server/transfer and its logs.
//   - A mail that is not taken within mail_return_days goes back to its
//     sender, with "RE:" before its title, as mail_return_timer. A mail
//     that came back, or a script's mail, is deleted after
//     mail_delete_days, as mail_delete_timer.
//
// One Registry serves every zone of a process. Each mail has its own
// lock, which every reader and writer holds, so a zone that takes
// attachments and the expiry work never race.
package mail

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Lengths of a mail, as MAIL_TITLE_LENGTH and MAIL_BODY_LENGTH of RODEX,
// less their ending zero.
const (
	MaxTitle = 39
	MaxBody  = 499
	// MaxName is NAME_LENGTH less its ending zero.
	MaxName = 23
)

// Key names a mailbox: a character's database ID, or for a character
// that only lives in memory, its name in lower case.
type Key struct {
	ID   int64
	Name string
}

// KeyOf is the key of a character's mailbox.
func KeyOf(id int64, name string) Key {
	if id > 0 {
		return Key{ID: id}
	}
	return Key{Name: strings.ToLower(name)}
}

// Saved reports whether the key's mailbox is saved in the database.
func (k Key) Saved() bool { return k.ID > 0 }

// Mail is one mail. Hold its lock to read or change it.
type Mail struct {
	mu sync.Mutex
	// ID is the mail's ID, from 1. It is the transfer owner's ID of its
	// box.
	ID int64
	// From is the sender's character key, or a zero key for a script's
	// mail, as rAthena's send_id 0. FromName is the name shown.
	From     Key
	FromName string
	// To is the recipient, and ToName its name.
	To     Key
	ToName string
	Title  string
	Body   string
	// Zeny is the zeny the mail holds.
	Zeny int64
	// Box holds the items. Its Who is the mail.
	Box transfer.ListBox
	// Sent is when the mail was sent, or when it came back.
	Sent time.Time
	Read bool
	// Returned is set for a mail that came back to its sender, as
	// MAIL_INBOX_RETURNED.
	Returned bool
	// deleted is set once the mail left its mailbox.
	deleted bool
	// dirty is set while a change waits for a commit.
	dirty bool
}

// Lock and Unlock hold the mail.
func (m *Mail) Lock()   { m.mu.Lock() }
func (m *Mail) Unlock() { m.mu.Unlock() }

// Deleted reports whether the mail left its mailbox. The lock must be
// held.
func (m *Mail) Deleted() bool { return m.deleted }

// Delete marks the mail deleted, for its commit. The lock must be held.
func (m *Mail) Delete() { m.deleted, m.dirty = true, true }

// Touch marks a change for the next commit. The lock must be held.
func (m *Mail) Touch() { m.dirty = true }

// Owner is the mail as a transfer owner.
func (m *Mail) Owner() transfer.Owner { return transfer.Owner{Kind: transfer.KindMail, ID: m.ID} }

// Items are the mail's items, as the store saves them. The lock must be
// held.
func (m *Mail) Items() []character.Item { return storage.SlotsOf(m.Box.Slots) }

// HasItems reports whether the mail holds items. The lock must be held.
func (m *Mail) HasItems() bool {
	for i := range m.Box.Slots {
		if m.Box.Slots[i].ID != 0 {
			return true
		}
	}
	return false
}

// Count is how many item stacks the mail holds. The lock must be held.
func (m *Mail) Count() int {
	n := 0
	for i := range m.Box.Slots {
		if m.Box.Slots[i].ID != 0 {
			n++
		}
	}
	return n
}

// Row is a mail as the store saves it.
type Row struct {
	ID               int64
	From, To         Key
	FromName, ToName string
	Title, Body      string
	Zeny             int64
	Sent             time.Time
	Read, Returned   bool
	Items            []character.Item
}

// Write is one mail a commit saves: its row, or its deletion.
type Write struct {
	ID int64
	// Row is nil for a mail to delete.
	Row *Row
	// To is the recipient's mailbox, so a later load waits for the write.
	To Key
}

// Snapshot returns the mail's write for a commit, when a change waits.
// The lock must be held.
func (m *Mail) Snapshot() (Write, bool) {
	if !m.dirty {
		return Write{}, false
	}
	m.dirty = false
	w := Write{ID: m.ID, To: m.To}
	if !m.deleted {
		w.Row = &Row{ID: m.ID, From: m.From, To: m.To, FromName: m.FromName, ToName: m.ToName, Title: m.Title, Body: m.Body,
			Zeny: m.Zeny, Sent: m.Sent, Read: m.Read, Returned: m.Returned, Items: m.Items()}
	}
	return w, true
}

// FromRow makes a mail from a saved row. items and slots set its box.
func FromRow(r *Row, items *item.Table, slots int) *Mail {
	m := &Mail{ID: r.ID, From: r.From, FromName: r.FromName, To: r.To, ToName: r.ToName, Title: r.Title, Body: r.Body,
		Zeny: r.Zeny, Sent: r.Sent, Read: r.Read, Returned: r.Returned}
	m.Box = transfer.ListBox{Size: max(slots, len(r.Items)), Items: items, Who: m.Owner()}
	for _, it := range r.Items {
		for len(m.Box.Slots) <= it.Slot {
			m.Box.Slots = append(m.Box.Slots, item.Slot{})
		}
		m.Box.Slots[it.Slot] = item.Slot{ID: it.ID, Amount: it.Amount, Serial: it.Serial, Refine: it.Refine,
			Cards: slices.Clone(it.Cards), Extra: it.Clone()}
		if items != nil {
			m.Box.Weight += items.Get(it.ID).Weight * it.Amount
		}
	}
	return m
}

// Loader reads mailboxes and finds recipients in the database.
type Loader interface {
	// LoadMailbox reads a character's mails. It waits for the mailbox's
	// commits that are not written yet.
	LoadMailbox(ctx context.Context, character int64) ([]Row, error)
	// FindRecipient finds a character by name, or by ID when id is above
	// 0, and counts its mails. ok is false when there is none.
	FindRecipient(ctx context.Context, name string, id int64) (r Recipient, ok bool, err error)
}

// Recipient is a character that mail may go to.
type Recipient struct {
	Key  Key
	Name string
	// Mails is how many mails it holds, for the mailbox limit.
	Mails int
}

// Mailbox is one character's mails, oldest first.
type Mailbox struct {
	mu    sync.Mutex
	key   Key
	name  string
	mails []*Mail
	// fresh are mails that arrived since the owner's zone last looked.
	fresh []*Mail
	refs  int
}

// Key is the mailbox's key.
func (b *Mailbox) Key() Key { return b.key }

// Mails are the mailbox's mails, oldest first, without deleted ones.
func (b *Mailbox) Mails() []*Mail {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.mails)
}

// Len is how many mails the mailbox holds.
func (b *Mailbox) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.mails)
}

// Find returns a mail by ID, or nil.
func (b *Mailbox) Find(id int64) *Mail {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, m := range b.mails {
		if m.ID == id {
			return m
		}
	}
	return nil
}

// Fresh returns the mails that arrived since the last call.
func (b *Mailbox) Fresh() []*Mail {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.fresh
	b.fresh = nil
	return out
}

// add puts a mail in the mailbox, unless it holds it already.
func (b *Mailbox) add(m *Mail, fresh bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if slices.ContainsFunc(b.mails, func(o *Mail) bool { return o.ID == m.ID }) {
		return
	}
	i, _ := slices.BinarySearchFunc(b.mails, m.ID, func(o *Mail, id int64) int { return int(o.ID - id) })
	b.mails = slices.Insert(b.mails, i, m)
	if fresh {
		b.fresh = append(b.fresh, m)
	}
}

// Remove takes a mail out of the mailbox, once it was deleted.
func (b *Mailbox) Remove(id int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mails = slices.DeleteFunc(b.mails, func(m *Mail) bool { return m.ID == id })
	b.fresh = slices.DeleteFunc(b.fresh, func(m *Mail) bool { return m.ID == id })
}

// Registry holds the mailboxes of the characters online.
type Registry struct {
	// Loader reads saved mailboxes. Without one, mailboxes live in
	// memory while the process runs, and never leave the registry.
	Loader Loader
	// Items is the item table, for the weight of mail boxes.
	Items *item.Table

	mu     sync.Mutex
	boxes  map[Key]*Mailbox
	nextID int64
	// names are the characters the registry knows without a database,
	// by lower-case name: every character that logged in.
	names map[string]string
	// recent are mails sent to mailboxes that were not loaded, kept a
	// while so a login that loaded before their commit still sees them.
	recent []recentMail
	// gone are mails that expired, so a mailbox that loaded before the
	// expiry's commit leaves them out.
	gone map[int64]bool
}

type recentMail struct {
	m  *Mail
	at time.Time
}

// recentFor is how long a mail sent to a mailbox that was not loaded
// stays in recent.
const recentFor = 5 * time.Minute

// NewRegistry makes a registry. loader may be nil.
func NewRegistry(loader Loader, items *item.Table) *Registry {
	return &Registry{Loader: loader, Items: items, boxes: map[Key]*Mailbox{}, names: map[string]string{}}
}

// SetIDFloor makes new mails take IDs above floor, the highest saved ID.
func (r *Registry) SetIDFloor(floor int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID = max(r.nextID, floor)
}

// NewID gives a new mail its ID.
func (r *Registry) NewID() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return r.nextID
}

// NewMail makes an empty mail with a new ID. slots is the mail_items
// limit.
func (r *Registry) NewMail(slots int) *Mail {
	m := &Mail{ID: r.NewID(), dirty: true}
	m.Box = transfer.ListBox{Size: slots, Items: r.Items, Who: m.Owner()}
	return m
}

// Acquire returns a character's mailbox, loading it when it is not in
// memory, and counts the caller as its user. Release ends the use. It
// runs on the goroutine that loads a character, never on a zone's.
func (r *Registry) Acquire(ctx context.Context, k Key, name string, slots int) (*Mailbox, error) {
	r.mu.Lock()
	if k.Name != "" {
		r.names[k.Name] = name
	}
	if b := r.boxes[k]; b != nil {
		b.refs++
		r.mu.Unlock()
		return b, nil
	}
	r.mu.Unlock()
	var rows []Row
	if r.Loader != nil && k.Saved() {
		var err error
		if rows, err = r.Loader.LoadMailbox(ctx, k.ID); err != nil {
			return nil, fmt.Errorf("mailbox %d: %w", k.ID, err)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.boxes[k]
	if b == nil {
		b = &Mailbox{key: k, name: name}
		for i := range rows {
			if !r.gone[rows[i].ID] {
				b.mails = append(b.mails, FromRow(&rows[i], r.Items, slots))
			}
		}
		r.boxes[k] = b
		// Mails sent while it loaded.
		for _, rm := range r.recent {
			rm.m.Lock()
			to, deleted := rm.m.To, rm.m.deleted
			rm.m.Unlock()
			if to == k && !deleted {
				b.add(rm.m, false)
			}
		}
	}
	b.refs++
	return b, nil
}

// Release ends a use of a mailbox. A saved mailbox that no character
// online uses leaves memory.
func (r *Registry) Release(b *Mailbox) {
	if b == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	b.refs--
	if b.refs <= 0 && r.Loader != nil && b.key.Saved() && r.boxes[b.key] == b {
		delete(r.boxes, b.key)
	}
}

// Loaded returns a mailbox in memory, or nil.
func (r *Registry) Loaded(k Key) *Mailbox {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.boxes[k]
}

// box returns the mailbox of a key, making an empty one in memory for a
// registry without a database. It is nil for a saved mailbox that is
// not loaded.
func (r *Registry) box(k Key) *Mailbox {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := r.boxes[k]
	if b == nil && (r.Loader == nil || !k.Saved()) {
		b = &Mailbox{key: k, name: r.names[k.Name]}
		r.boxes[k] = b
	}
	return b
}

// Deliver puts a sent mail in its recipient's mailbox when it is in
// memory, and tells the owner's zone. Call it after the mail's commit
// was queued, so the recipient's later commits come after it.
func (r *Registry) Deliver(m *Mail) {
	m.Lock()
	to := m.To
	m.Unlock()
	if b := r.box(to); b != nil {
		b.add(m, true)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.recent = slices.DeleteFunc(r.recent, func(rm recentMail) bool { return now.Sub(rm.at) > recentFor })
	r.recent = append(r.recent, recentMail{m: m, at: now})
}

// Find finds a recipient by name, or by ID when id is above 0, among
// the mailboxes in memory and the characters the registry knows. ok is
// false when the registry cannot tell: ask the Loader.
func (r *Registry) Find(name string, id int64) (rc Recipient, found, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, b := range r.boxes {
		if id > 0 && k.ID == id || id <= 0 && strings.EqualFold(b.name, name) {
			b.mu.Lock()
			n := len(b.mails)
			b.mu.Unlock()
			return Recipient{Key: k, Name: b.name, Mails: n}, true, true
		}
	}
	if r.Loader != nil {
		return Recipient{}, false, false
	}
	if n, known := r.names[strings.ToLower(name)]; known && id <= 0 {
		return Recipient{Key: Key{Name: strings.ToLower(name)}, Name: n}, true, true
	}
	return Recipient{}, false, true
}

// Holding is what one mail holds, for the duplication tests' audit.
type Holding struct {
	Mail  int64
	Zeny  int64
	Items []character.Item
}

// Audit lists what every mail in memory holds, by mail ID.
func (r *Registry) Audit() []Holding {
	r.mu.Lock()
	var all []*Mail
	seen := map[int64]bool{}
	for _, b := range r.boxes {
		b.mu.Lock()
		for _, m := range b.mails {
			if !seen[m.ID] {
				seen[m.ID] = true
				all = append(all, m)
			}
		}
		b.mu.Unlock()
	}
	for _, rm := range r.recent {
		if !seen[rm.m.ID] {
			seen[rm.m.ID] = true
			all = append(all, rm.m)
		}
	}
	r.mu.Unlock()
	out := make([]Holding, 0, len(all))
	for _, m := range all {
		m.Lock()
		if !m.deleted {
			out = append(out, Holding{Mail: m.ID, Zeny: m.Zeny, Items: m.Items()})
		}
		m.Unlock()
	}
	slices.SortFunc(out, func(a, b Holding) int { return int(a.Mail - b.Mail) })
	return out
}
