package shop

import (
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
)

// TestRestock checks the stock of each restock kind fills at its time,
// and stays empty before.
func TestRestock(t *testing.T) {
	start := time.Date(2026, 10, 7, 15, 0, 0, 0, time.UTC) // a Wednesday
	for _, c := range []struct {
		restock      string
		before, then time.Duration
	}{
		{"daily", 8 * time.Hour, 9 * time.Hour},
		{"weekly", 4*24*time.Hour + 8*time.Hour, 4*24*time.Hour + 9*time.Hour},
		{"6h", 5 * time.Hour, 6 * time.Hour},
	} {
		r, err := mod.ParseRestock(c.restock)
		if err != nil {
			t.Fatal(err)
		}
		s := New("core:x", "x", false, false, "core", []Item{{ID: 1, Stock: 5, Initial: 5, Restock: r}})
		s.Lock()
		s.Items(start, time.UTC)
		s.Take(0, 5)
		if got := s.Items(start.Add(c.before), time.UTC)[0].Stock; got != 0 {
			t.Errorf("%s: %d before the restock", c.restock, got)
		}
		if got := s.Items(start.Add(c.then), time.UTC)[0].Stock; got != 5 {
			t.Errorf("%s: %d after the restock", c.restock, got)
		}
		rows := s.Dirty()
		s.Unlock()
		if len(rows) != 1 || rows[0].Stock != 5 {
			t.Errorf("%s: rows %+v", c.restock, rows)
		}
	}
}

// TestLoadStock checks saved stock applies to the items that still have
// one, and is cut to their stock.
func TestLoadStock(t *testing.T) {
	s := New("core:x", "x", false, false, "core", []Item{{ID: 1, Stock: 5, Initial: 5}, {ID: 2, Stock: -1, Initial: -1}})
	r := NewRegistry([]*Shop{s})
	n := r.LoadStock([]StockRow{{Shop: "core:x", Item: 1, Stock: 9}, {Shop: "core:x", Item: 2, Stock: 1}, {Shop: "core:y", Item: 1, Stock: 1}})
	s.Lock()
	defer s.Unlock()
	if n != 1 || s.items[0].Stock != 5 || s.items[1].Stock != -1 {
		t.Errorf("used %d, items %+v", n, s.items)
	}
}
