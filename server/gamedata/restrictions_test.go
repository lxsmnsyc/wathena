package gamedata

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/item"
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
