package gamedata

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// TestItemRestrictions checks that item_trade's flags become the core
// items' restrictions, with no_drop and no_sell in step.
func TestItemRestrictions(t *testing.T) {
	d, err := LoadItemData(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	tb := item.New(d)
	coin, _ := tb.Lookup(671)
	if coin == nil || !slices.Equal(coin.Restrictions, item.RestrictionNames()) || !coin.NoDrop || !coin.NoSell {
		t.Errorf("Gold_Coin: %+v", coin)
	}
	potion, _ := tb.Lookup(547)
	if potion == nil || !slices.Equal(potion.Restrictions, []string{"no_sell"}) || potion.NoDrop || !potion.NoSell {
		t.Errorf("White_Slim_Potion: %+v", potion)
	}
	n := 0
	for _, it := range d.Items {
		if len(it.Restrictions) > 0 {
			n++
		}
	}
	if n < 7000 {
		t.Errorf("%d items with restrictions", n)
	}
}

// TestBuyingStoreFlags checks that rAthena's BuyingStore flags of
// shops.json reach the item data: Jellopy has it, a Knife does not.
func TestBuyingStoreFlags(t *testing.T) {
	for _, mode := range []reference.Mode{reference.Renewal, reference.PreRenewal} {
		d, err := LoadItemData(mode)
		if err != nil {
			t.Fatal(err)
		}
		tab := item.New(d)
		if !tab.Get(909).BuyingStore || tab.Get(1201).BuyingStore {
			t.Errorf("%s: jellopy %v, knife %v", mode, tab.Get(909).BuyingStore, tab.Get(1201).BuyingStore)
		}
	}
}

// TestMarketRulesMatchRathena checks that the vending, store search and
// mail rules default to the values rAthena's conf files set, as
// shops.json read them.
func TestMarketRulesMatchRathena(t *testing.T) {
	for _, mode := range []reference.Mode{reference.Renewal, reference.PreRenewal} {
		var shops struct {
			Settings map[string]struct {
				Value int `json:"value"`
			} `json:"settings"`
		}
		if err := reference.LoadFile("extracts/"+string(mode)+"/shops.json", &shops); err != nil {
			t.Fatal(err)
		}
		if len(shops.Settings) != 16 {
			t.Errorf("%s: %d settings", mode, len(shops.Settings))
		}
		for name, s := range shops.Settings {
			rule := name
			if name == "feature.autotrade" {
				rule = "autotrade_restore"
			}
			b, ok := rules.FindBound(rules.RuleBounds, rule)
			if !ok {
				t.Errorf("%s: no rule %s", mode, rule)
				continue
			}
			if b.Default != s.Value {
				t.Errorf("%s: %s is %d, rAthena's %d", mode, rule, b.Default, s.Value)
			}
		}
	}
}
