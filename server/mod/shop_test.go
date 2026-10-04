package mod

import (
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func shopOf(t *testing.T, src string) (*Shop, error) {
	t.Helper()
	var v any
	if err := yaml.Unmarshal([]byte(src), &v); err != nil {
		t.Fatal(err)
	}
	return readShop(v)
}

// TestShopTable reads the shop table of review decision 30 and the list
// form, and refuses what does not fit.
func TestShopTable(t *testing.T) {
	sh, err := shopOf(t, `
buys_from_players: false
items:
  - {item: Red_Potion}
  - {item: Fly_Wing, cost: {zeny: 5, items: [{item: Jellopy, amount: 2}], currency: {cash: 3}}, stock: 4, restock: 6h}
`)
	if err != nil {
		t.Fatal(err)
	}
	if sh.BuysFromPlayers || !sh.Discount || len(sh.Items) != 2 || sh.Items[0].Price != -1 {
		t.Fatalf("shop %+v", sh)
	}
	it := sh.Items[1]
	if it.Cost == nil || it.Cost.Zeny != 5 || it.Cost.Items[0].Amount != 2 || it.Cost.Currencies["cash"] != 3 || it.Stock != 4 || it.Restock != "6h" {
		t.Errorf("item %+v %+v", it, it.Cost)
	}
	if sh, err := shopOf(t, `[{item: Red_Potion, price: 10}, {item: Red_Potion}]`); err != nil || len(sh.Items) != 2 || !sh.BuysFromPlayers {
		t.Errorf("list %+v %v", sh, err)
	}
	for src, want := range map[string]string{
		`{items: [{item: A, price: 1, cost: {zeny: 1}}]}`: "price and cost",
		`{items: [{item: A, restock: daily}]}`:            "restock needs a stock",
		`{items: [{item: A}, {item: A}]}`:                 "twice",
		`{items: [{item: A, stock: 0}]}`:                  "stock is 1",
		`{items: [{item: A, stock: 1, restock: 30s}]}`:    "restock is daily",
		`{items: [{item: A, cost: {zeny: 1, gems: 2}}]}`:  `"gems" is not a field`,
		`{items: [{item: A}], sells: true}`:               `"sells" is not a field`,
		`{items: [{item: A, cost: {items: [{item: B}, {item: C}, {item: D}], currency: {a: 1, b: 1, c: 1}}}]}`: "at most 5",
	} {
		if _, err := shopOf(t, src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", src, err, want)
		}
	}
}

// TestParseRestock reads each restock kind.
func TestParseRestock(t *testing.T) {
	for s, want := range map[string]Restock{"": {}, "daily": {Daily: true}, "weekly": {Weekly: true}, "30m": {Every: 30 * time.Minute},
		"6h": {Every: 6 * time.Hour}, "2d": {Every: 48 * time.Hour}} {
		if got, err := ParseRestock(s); err != nil || got != want {
			t.Errorf("%q: %+v %v", s, got, err)
		}
	}
	for _, s := range []string{"0h", "5", "1w", "h"} {
		if _, err := ParseRestock(s); err == nil {
			t.Errorf("%q: no error", s)
		}
	}
}
