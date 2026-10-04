// Package market lists the world's vending shops and buying stores
// (phase 5 step 7, proposal 0002, review decision 30), as rAthena's
// vending_db and buyingstore_db:
//   - Each zone puts a copy of a store here when it opens, after each
//     sale and when it closes, so a store search reads every map.
//   - A store's row is saved in the same commit as its sales, so an
//     autotrade store opens again after a restart, as
//     do_init_vending_autotrade and do_init_buyingstore_autotrade.
package market

import (
	"slices"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/server/item"
)

// Store is a copy of one open store.
type Store struct {
	// Owner is the owner's character ID, or 0 for a character that only
	// lives in memory, which Name then names.
	Owner int64
	Name  string
	// ID changes each time the owner opens a store, as vender_id.
	ID uint32
	// Buying is set for a buying store.
	Buying bool
	Title  string
	// Map is the map's asset name, and X and Y the owner's cell.
	Map  string
	X, Y int
	// Dir is the owner's facing, and Sit whether it sits.
	Dir int
	Sit bool
	// ZenyLimit is what a buying store may still spend.
	ZenyLimit int64
	// Autotrade is set while the owner is away.
	Autotrade bool
	Lines     []Line
}

// Line is one line of a store.
type Line struct {
	// Index is the line's place in the store.
	Index int
	// Slot is the copy for sale with the amount left, in its cart slot,
	// or for a buying store the item and the amount it still buys.
	Slot item.Slot
	// CartSlot is the line's cart slot, for a vending shop.
	CartSlot int
	Price    int64
}

// Key names a store: its owner and its kind.
type Key struct {
	Owner  int64
	Name   string
	Buying bool
}

// KeyOf is a store's key.
func (s *Store) KeyOf() Key {
	if s.Owner > 0 {
		return Key{Owner: s.Owner, Buying: s.Buying}
	}
	return Key{Name: strings.ToLower(s.Name), Buying: s.Buying}
}

// Clone copies a store.
func (s *Store) Clone() *Store {
	c := *s
	c.Lines = make([]Line, len(s.Lines))
	for i, l := range s.Lines {
		l.Slot.Cards = slices.Clone(l.Slot.Cards)
		l.Slot.Extra = l.Slot.Clone()
		c.Lines[i] = l
	}
	return &c
}

// Registry lists the open stores of the world.
type Registry struct {
	mu     sync.Mutex
	stores map[Key]*Store
	nextID uint32
}

// NewRegistry makes an empty registry.
func NewRegistry() *Registry { return &Registry{stores: map[Key]*Store{}} }

// NewID gives a newly opened store its ID, as vending_getuid.
func (r *Registry) NewID() uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	return r.nextID
}

// Put sets a store's copy. The registry keeps s; the caller must not
// change it after.
func (r *Registry) Put(s *Store) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stores[s.KeyOf()] = s
}

// Remove takes a store away.
func (r *Registry) Remove(k Key) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.stores, k)
}

// Get returns a store's copy, or nil.
func (r *Registry) Get(k Key) *Store {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stores[k]
}

// Query is a store search, as searchstore_query.
type Query struct {
	Buying bool
	// Items are the items to find, and Cards the cards one of which a
	// result must hold.
	Items, Cards []uint32
	// MinPrice and MaxPrice bound the price, 0 for no bound.
	MinPrice, MaxPrice int64
	// Rolls bound rolls by name, and Options random options by ID.
	Rolls   []Bound
	Options []Bound
	// Skip is the store of the searching player, which is left out.
	Skip Key
	// Map keeps only stores of one map, or every map when empty.
	Map string
	// Max is searchstore_maxresults.
	Max int
}

// Bound is a range of one rolled value.
type Bound struct {
	Name     string
	Min, Max int
}

// Hit is one result.
type Hit struct {
	Store *Store
	Line  Line
}

// Search runs a query over every store, in the order the stores opened.
// full is set when the results stopped at Max, as SSI_FAILED_OVER_MAXCOUNT.
func (r *Registry) Search(q Query, slotted func(id uint32) int) (hits []Hit, full bool) {
	r.mu.Lock()
	all := make([]*Store, 0, len(r.stores))
	for _, s := range r.stores {
		all = append(all, s)
	}
	r.mu.Unlock()
	slices.SortFunc(all, func(a, b *Store) int { return int(a.ID) - int(b.ID) })
	for _, s := range all {
		if s.Buying != q.Buying || s.KeyOf() == q.Skip || q.Map != "" && s.Map != q.Map {
			continue
		}
		for _, l := range s.Lines {
			if !q.match(&l, slotted) {
				continue
			}
			if q.Max > 0 && len(hits) >= q.Max {
				return hits, true
			}
			hits = append(hits, Hit{Store: s, Line: l})
		}
	}
	return hits, false
}

// match is vending_searchall's and buyingstore_searchall's checks of one
// line, with the roll bounds of strategy section 5.4.
func (q *Query) match(l *Line, slotted func(id uint32) int) bool {
	if l.Slot.Amount <= 0 || !slices.Contains(q.Items, l.Slot.ID) {
		return false
	}
	if q.MinPrice > 0 && l.Price < q.MinPrice || q.MaxPrice > 0 && l.Price > q.MaxPrice {
		return false
	}
	if len(q.Cards) > 0 {
		n := len(l.Slot.Cards)
		if slotted != nil {
			n = min(n, slotted(l.Slot.ID))
		}
		found := false
		for _, c := range l.Slot.Cards[:n] {
			if c != 0 && slices.Contains(q.Cards, c) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, b := range q.Rolls {
		// An unidentified copy shows no rolls, so it matches no bound.
		v, ok := l.Slot.Rolls.Get(b.Name)
		if l.Slot.Unidentified || !ok || v < b.Min || v > b.Max {
			return false
		}
	}
	for _, b := range q.Options {
		found := false
		for _, o := range l.Slot.Options {
			if strings.EqualFold(o.Name, b.Name) && o.Value >= b.Min && o.Value <= b.Max {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Write is a store's row for a commit: its copy, or its deletion when
// Store is nil.
type Write struct {
	Key   Key
	Store *Store
}
