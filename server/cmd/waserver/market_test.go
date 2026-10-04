package main

import (
	"testing"

	"github.com/lxsmnsyc/wathena/server/market"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// TestAutotradeRestoreRule checks autotrade_restore: with it on, every
// saved autotrade store opens again; with it off, none does, and the
// rows go.
func TestAutotradeRestoreRule(t *testing.T) {
	saved := []*market.Store{{Owner: 1, Title: "a"}, {Owner: 2, Buying: true, Title: "b"}}
	if got := autotradesToRestore(rules.DefaultMarket(), saved); len(got) != 2 {
		t.Errorf("on: %d", len(got))
	}
	off, err := rules.MarketOf(map[string]int{"autotrade_restore": 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := autotradesToRestore(off, saved); len(got) != 0 {
		t.Errorf("off: %d", len(got))
	}
	if w := storeDeletes(saved); len(w.Stores) != 2 || w.Stores[0].Store != nil || w.Stores[1].Key != (market.Key{Owner: 2, Buying: true}) {
		t.Errorf("deletes %+v", w.Stores)
	}
}
