// Package shop holds the shops of data/npcs.yml while the world runs
// (proposal 0002, review decision 30), as rAthena's shop, itemshop,
// pointshop, cashshop, marketshop and barter shops in one type:
//   - An item's cost mixes zeny, items and currencies. Without a cost it
//     is the item's price in zeny.
//   - An item may have a stock that every player shares, as a
//     marketshop or a barter's stock, and a restock that fills it again.
//   - Scripts change shops with world:get_shop.
//
// One Registry serves every zone of a process. A zone locks a shop while
// it runs a purchase, so two players never buy the same stock. The stock
// is saved with the purchase's commit, and loads again at start.
package shop

import (
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
)

// Shop is one shop.
type Shop struct {
	// Name is the shop's full record name, such as core:Tool Dealer#prt1.
	Name string
	// DisplayName is what the shop's window shows.
	DisplayName string
	// Discount lets Discount and Compulsion Discount lower the zeny part
	// of the costs, as npc_shop_discount.
	Discount bool
	// BuysFromPlayers gives the shop a sell tab.
	BuysFromPlayers bool
	// Mod is the mod that added the shop, for currency names.
	Mod string

	mu    sync.Mutex
	items []Item
	// dirty are the items whose stock changed since the last commit.
	dirty map[uint32]bool
}

// Item is one item a shop sells.
type Item struct {
	ID uint32
	// Zeny is the zeny of one before Discount.
	Zeny int
	// Items and Currencies are the rest of the cost of one.
	Items      []CostItem
	Currencies []CostCurrency
	// Stock is how many are left for every player, or -1 for no limit.
	// Initial is what a restock fills it to.
	Stock, Initial int
	Restock        mod.Restock
	// Filled is when the stock last filled: the start, a restock, or a
	// script's set_stock.
	Filled time.Time
	// Added is set for an item a script added, which is not saved.
	Added bool
}

// CostItem is an item a cost takes, for one bought.
type CostItem struct {
	ID     uint32
	Amount int
}

// CostCurrency is a currency a cost takes, for one bought.
type CostCurrency struct {
	ID     uint32
	Amount int64
}

// Limited reports whether the item has a stock.
func (it *Item) Limited() bool { return it.Stock >= 0 }

// New makes a shop.
func New(name, display string, discount, buys bool, modID string, items []Item) *Shop {
	return &Shop{Name: name, DisplayName: display, Discount: discount, BuysFromPlayers: buys, Mod: modID, items: items, dirty: map[uint32]bool{}}
}

// Lock and Unlock hold the shop while a zone reads or changes it.
func (s *Shop) Lock()   { s.mu.Lock() }
func (s *Shop) Unlock() { s.mu.Unlock() }

// Items are the shop's items, after restocks due at now. The lock must
// be held. The slice is the shop's: copy what outlives the lock.
func (s *Shop) Items(now time.Time, loc *time.Location) []Item {
	for i := range s.items {
		if s.restock(&s.items[i], now, loc) {
			s.dirty[s.items[i].ID] = true
		}
	}
	return s.items
}

// Find returns the item's index, or -1. The lock must be held.
func (s *Shop) Find(id uint32) int {
	return slices.IndexFunc(s.items, func(it Item) bool { return it.ID == id })
}

// Take takes an amount from an item's stock. The lock must be held, and
// the stock checked.
func (s *Shop) Take(i, amount int) {
	it := &s.items[i]
	if it.Stock >= 0 {
		it.Stock = max(it.Stock-amount, 0)
		s.dirty[it.ID] = true
	}
}

// Add adds or replaces an item, as npcshopadditem. The lock must be
// held.
func (s *Shop) Add(it Item, now time.Time) {
	it.Added, it.Filled = true, now
	if i := s.Find(it.ID); i >= 0 {
		it.Added = s.items[i].Added
		s.items[i] = it
	} else {
		s.items = append(s.items, it)
	}
	s.dirty[it.ID] = true
}

// Remove removes an item, as npcshopdelitem. It reports whether the
// shop had it. The lock must be held.
func (s *Shop) Remove(id uint32) bool {
	i := s.Find(id)
	if i < 0 {
		return false
	}
	s.items = slices.Delete(s.items, i, i+1)
	s.dirty[id] = true
	return true
}

// SetStock sets an item's stock, and makes it its restock amount. -1
// takes the limit away. The lock must be held.
func (s *Shop) SetStock(id uint32, n int, now time.Time) bool {
	i := s.Find(id)
	if i < 0 {
		return false
	}
	it := &s.items[i]
	it.Stock, it.Initial, it.Filled = n, n, now
	s.dirty[id] = true
	return true
}

// restock fills an item's stock when a restock time passed since it last
// filled. Filled moves to the last restock time, so a stock that stayed
// full waits for the next one. It reports whether the stock changed.
func (s *Shop) restock(it *Item, now time.Time, loc *time.Location) bool {
	if it.Stock < 0 || it.Initial < 0 {
		return false
	}
	if it.Filled.IsZero() {
		// A stock that never filled counts from now.
		it.Filled = now
		return false
	}
	next, ok := NextRestock(it.Restock, it.Filled, loc)
	if !ok || now.Before(next) {
		return false
	}
	changed := it.Stock != it.Initial
	it.Stock, it.Filled = it.Initial, LastRestock(it.Restock, it.Filled, now, loc)
	return changed
}

// LastRestock is the last restock time at or before now, after filled.
func LastRestock(r mod.Restock, filled, now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	t := now.In(loc)
	y, m, d := t.Date()
	switch {
	case r.Daily:
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	case r.Weekly:
		back := (int(t.Weekday()) + 6) % 7
		return time.Date(y, m, d-back, 0, 0, 0, 0, loc)
	case r.Every > 0:
		return filled.Add(now.Sub(filled) / r.Every * r.Every)
	}
	return now
}

// NextRestock is when a stock filled at filled fills again, or false for
// one that never does.
func NextRestock(r mod.Restock, filled time.Time, loc *time.Location) (time.Time, bool) {
	if loc == nil {
		loc = time.Local
	}
	t := filled.In(loc)
	switch {
	case r.Daily:
		y, m, d := t.Date()
		return time.Date(y, m, d+1, 0, 0, 0, 0, loc), true
	case r.Weekly:
		y, m, d := t.Date()
		// The next Monday, at the start of the day.
		days := (8 - int(t.Weekday())) % 7
		if days == 0 {
			days = 7
		}
		return time.Date(y, m, d+days, 0, 0, 0, 0, loc), true
	case r.Every > 0:
		return filled.Add(r.Every), true
	}
	return time.Time{}, false
}

// StockRow is one item's saved stock.
type StockRow struct {
	Shop   string
	Item   uint32
	Stock  int
	Filled time.Time
	// Delete is set for an item that left the shop or lost its limit.
	Delete bool
}

// Dirty returns the stock rows that changed since the last call. The
// lock must be held.
func (s *Shop) Dirty() []StockRow {
	if len(s.dirty) == 0 {
		return nil
	}
	var out []StockRow
	for _, id := range slices.Sorted(maps.Keys(s.dirty)) {
		i := s.Find(id)
		if i < 0 || s.items[i].Stock < 0 || s.items[i].Added {
			out = append(out, StockRow{Shop: s.Name, Item: id, Delete: true})
			continue
		}
		it := s.items[i]
		out = append(out, StockRow{Shop: s.Name, Item: id, Stock: it.Stock, Filled: it.Filled})
	}
	clear(s.dirty)
	return out
}

// Registry holds the world's shops by full record name.
type Registry struct {
	// Location is the time zone of daily and weekly restocks.
	Location *time.Location

	mu    sync.Mutex
	shops map[string]*Shop
}

// NewRegistry makes a registry of shops.
func NewRegistry(shops []*Shop) *Registry {
	r := &Registry{shops: map[string]*Shop{}}
	for _, s := range shops {
		r.shops[s.Name] = s
	}
	return r
}

// Get finds a shop by full record name, or nil.
func (r *Registry) Get(name string) *Shop {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shops[name]
}

// Names are the shops' names, sorted.
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.shops))
	for n := range r.shops {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// LoadStock sets the saved stock of the shops' limited items. A row of
// an item a shop no longer sells, or no longer limits, is left out, as
// npc_market_checkall drops rows of items a market no longer sells. A
// row above the item's stock is cut to it.
func (r *Registry) LoadStock(rows []StockRow) (used int) {
	for _, row := range rows {
		s := r.Get(row.Shop)
		if s == nil {
			continue
		}
		s.mu.Lock()
		if i := s.Find(row.Item); i >= 0 && s.items[i].Initial >= 0 && !s.items[i].Added {
			it := &s.items[i]
			it.Stock, it.Filled = min(max(row.Stock, 0), it.Initial), row.Filled
			used++
		}
		s.mu.Unlock()
	}
	return used
}
