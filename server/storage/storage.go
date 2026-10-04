// Package storage keeps the storages and carts of the players a process
// has online (phase 5 step 6, decision 0023):
//   - An account's storage, rAthena's storage, which every character of
//     the account shares.
//   - Extra storages, records of data/storages.yml, that an account or a
//     character owns, as rAthena's premium storages of inter_server.yml.
//   - Carts, rAthena's cart_inventory, one per character.
//
// One Registry serves every zone of a process. A storage loads when the
// first character that uses it logs in, and stays while one does. Zones
// move items through server/transfer with the storage's lock held, and
// commit a storage's slots as a diff with the characters the moves
// changed, in one transaction (server/store).
//
// Only one character at a time has a storage open, as rAthena's
// storage_flag. An account's two characters online at once share the
// storage, and the second to open it waits for the first to close it.
package storage

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Holder is what owns a storage.
type Holder uint8

// The holders.
const (
	HolderAccount Holder = iota + 1
	HolderCharacter
	HolderCart
)

var holderNames = [...]string{HolderAccount: "account", HolderCharacter: "character", HolderCart: "cart"}

// String is the holder's name in the database.
func (h Holder) String() string {
	if int(h) < len(holderNames) {
		return holderNames[h]
	}
	return ""
}

// Key names one storage.
type Key struct {
	Holder Holder
	// ID is the account's or the character's database ID. It is 0 for a
	// character that only lives in memory, which Name then names: its
	// name in lower case. Such a character is its own account.
	ID   int64
	Name string
	// Storage is the record ID of an extra storage, or 0 for an account's
	// storage and for a cart.
	Storage uint32
}

// Saved reports whether the key's storage is saved in the database.
func (k Key) Saved() bool { return k.ID > 0 }

// String names the key in logs.
func (k Key) String() string {
	if k.ID == 0 {
		return fmt.Sprintf("%s:%s:%d", k.Holder, k.Name, k.Storage)
	}
	return fmt.Sprintf("%s:%d:%d", k.Holder, k.ID, k.Storage)
}

// Owner is the storage as an owner in transfer logs.
func (k Key) Owner() transfer.Owner {
	if k.Holder == HolderCart {
		return transfer.Owner{Kind: transfer.KindCart, ID: k.ID}
	}
	return transfer.Owner{Kind: transfer.KindStorage, ID: k.ID, Storage: k.Storage}
}

// Keys are the keys of a character's storages: the account's storage,
// the cart, and one per extra storage record.
func Keys(c *character.Info, records []*Record) []Key {
	account, char := c.AccountID, c.ID
	name := ""
	if char == 0 {
		account, name = 0, strings.ToLower(c.Name)
	}
	keys := []Key{{Holder: HolderAccount, ID: account, Name: name}, {Holder: HolderCart, ID: char, Name: name}}
	for _, r := range records {
		if r.Account {
			keys = append(keys, Key{Holder: HolderAccount, ID: account, Name: name, Storage: r.ID})
		} else {
			keys = append(keys, Key{Holder: HolderCharacter, ID: char, Name: name, Storage: r.ID})
		}
	}
	return keys
}

// Record is one extra storage of data/storages.yml (review decision 29).
type Record struct {
	// ID is the storage's ID in the database and the logs: 1 and on.
	// Mod records get IDs from 10,000, as other mod records.
	ID uint32
	// Name is the record's full name, such as example.bank:vault.
	Name        string
	DisplayName string
	// Slots is how many slots it holds.
	Slots int
	// Account is set for a storage the account owns, which its
	// characters share. A character's own storage has it unset.
	Account bool
}

// Loader reads a storage's slots from the database. It waits for the
// storage's commits that are not written yet.
type Loader interface {
	LoadStorage(ctx context.Context, k Key) ([]character.Item, error)
}

// Storage is one storage. Its zone reads and changes it with its lock
// held.
type Storage struct {
	mu  sync.Mutex
	key Key
	box transfer.ListBox
	// opener is who has it open, or nil.
	opener any
	// saved is what the last commit wrote, for the next commit's diff.
	saved []character.Item
	// refs counts the characters online that use it.
	refs int
	// weighed is set once the box's weight was counted with the item
	// table.
	weighed bool
}

// Prepare sets the box's item table, size and weight limit before a
// move or a list. The first call counts the weight the storage holds.
// The lock must be held.
func (st *Storage) Prepare(items *item.Table, size, maxWeight int) *transfer.ListBox {
	b := &st.box
	b.Items, b.Size, b.MaxWeight = items, size, maxWeight
	if !st.weighed {
		st.weighed = true
		b.Weight = 0
		for i := range b.Slots {
			if s := &b.Slots[i]; s.ID != 0 {
				b.Weight += items.Get(s.ID).Weight * s.Amount
			}
		}
	}
	return b
}

func newStorage(k Key, slots []character.Item) *Storage {
	st := &Storage{key: k, saved: slices.Clone(slots)}
	st.box.Who = k.Owner()
	for _, s := range slots {
		for len(st.box.Slots) <= s.Slot {
			st.box.Slots = append(st.box.Slots, item.Slot{})
		}
		st.box.Slots[s.Slot] = item.Slot{ID: s.ID, Amount: s.Amount, Serial: s.Serial, Refine: s.Refine, Cards: slices.Clone(s.Cards),
			Extra: s.Clone()}
	}
	return st
}

// Key is the storage's key.
func (st *Storage) Key() Key { return st.key }

// Lock and Unlock hold the storage while a zone moves items.
func (st *Storage) Lock()   { st.mu.Lock() }
func (st *Storage) Unlock() { st.mu.Unlock() }

// Box is the storage as a transfer box. The lock must be held. The
// caller sets its Items, Size and MaxWeight before a move.
func (st *Storage) Box() *transfer.ListBox { return &st.box }

// Slots are the slots that hold items. The lock must be held.
func (st *Storage) Slots() []character.Item { return SlotsOf(st.box.Slots) }

// SlotsOf lists the slots that hold items, as the store saves them.
func SlotsOf(slots []item.Slot) []character.Item {
	var out []character.Item
	for i, s := range slots {
		if s.ID != 0 && s.Amount > 0 {
			out = append(out, character.Item{Slot: i, ID: s.ID, Amount: s.Amount, Serial: s.Serial, Refine: s.Refine,
				Cards: slices.Clone(s.Cards), Extra: s.Clone()})
		}
	}
	return out
}

// Used is how many slots hold items. The lock must be held.
func (st *Storage) Used() int {
	n := 0
	for i := range st.box.Slots {
		if st.box.Slots[i].ID != 0 {
			n++
		}
	}
	return n
}

// Commit returns what the last commit wrote and what the storage holds
// now, and keeps the second for the next commit. The lock must be held.
func (st *Storage) Commit() (base, cur []character.Item) {
	cur = st.Slots()
	base, st.saved = st.saved, cur
	return base, cur
}

// Open makes who the storage's opener. It returns false when another
// has it open. The lock must not be held.
func (st *Storage) Open(who any) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.opener != nil && st.opener != who {
		return false
	}
	st.opener = who
	return true
}

// Close ends who's opening. The lock must not be held.
func (st *Storage) Close(who any) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.opener == who {
		st.opener = nil
	}
}

// OpenFor reports whether who has the storage open. The lock must be
// held.
func (st *Storage) OpenFor(who any) bool { return who != nil && st.opener == who }

// Registry holds the storages of the characters online.
type Registry struct {
	// Loader reads saved storages. Without one, storages live in memory
	// while the process runs, and never leave the registry.
	Loader Loader

	mu      sync.Mutex
	storage map[Key]*Storage
}

// NewRegistry makes a registry. loader may be nil.
func NewRegistry(loader Loader) *Registry {
	return &Registry{Loader: loader, storage: map[Key]*Storage{}}
}

// Acquire returns the storages of keys, loading the ones not in memory,
// and counts the caller as a user of each. Release ends the use. It runs
// on the goroutine that loads a character, never on a zone's.
func (r *Registry) Acquire(ctx context.Context, keys []Key) ([]*Storage, error) {
	out := make([]*Storage, len(keys))
	for i, k := range keys {
		st, err := r.acquire(ctx, k)
		if err != nil {
			for _, o := range out[:i] {
				r.Release(o)
			}
			return nil, fmt.Errorf("storage %s: %w", k, err)
		}
		out[i] = st
	}
	return out, nil
}

func (r *Registry) acquire(ctx context.Context, k Key) (*Storage, error) {
	r.mu.Lock()
	if st := r.storage[k]; st != nil {
		st.refs++
		r.mu.Unlock()
		return st, nil
	}
	r.mu.Unlock()
	var slots []character.Item
	if r.Loader != nil && k.Saved() {
		var err error
		if slots, err = r.Loader.LoadStorage(ctx, k); err != nil {
			return nil, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.storage[k]
	if st == nil {
		// Another login may have loaded it meanwhile; the first wins.
		st = newStorage(k, slots)
		r.storage[k] = st
	}
	st.refs++
	return st, nil
}

// Release ends a use of a storage. A saved storage that no character
// online uses leaves memory. Its zone must have handed its last commit
// to the store first, so a later load waits for it.
func (r *Registry) Release(st *Storage) {
	if st == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st.refs--
	if st.refs <= 0 && r.Loader != nil && st.key.Saved() && r.storage[st.key] == st {
		delete(r.storage, st.key)
	}
}

// Holding is what one storage holds, for the duplication tests' audit.
type Holding struct {
	Key   Key
	Items []character.Item
}

// Audit lists what every storage in memory holds, by key.
func (r *Registry) Audit() []Holding {
	r.mu.Lock()
	all := make([]*Storage, 0, len(r.storage))
	for _, st := range r.storage {
		all = append(all, st)
	}
	r.mu.Unlock()
	out := make([]Holding, 0, len(all))
	for _, st := range all {
		st.mu.Lock()
		out = append(out, Holding{Key: st.key, Items: st.Slots()})
		st.mu.Unlock()
	}
	slices.SortFunc(out, func(a, b Holding) int { return strings.Compare(a.Key.String(), b.Key.String()) })
	return out
}
