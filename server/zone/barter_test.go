package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/shop"
)

// TestBarterRefinedCost buys with a cost that asks for a refine, as
// npc_barter_purchase (decision 0072): the copy at the refine or the
// lowest refine above it goes, never one below it or one with cards.
func TestBarterRefinedCost(t *testing.T) {
	knife := func(slot, refine int, cards ...uint32) character.Item {
		return character.Item{Slot: slot, ID: 1201, Amount: 1, Refine: refine, Cards: cards, Serial: item.NewSerial()}
	}
	z, a, _, _ := marketZone(t, knife(0, 7), knife(1, 12), knife(2, 10), knife(3, 9, 7001), stack(4, 909, 10))
	s := shop.New("core:barter_ill_test", "Illusion", false, false, "core", []shop.Item{
		{ID: 501, Items: []shop.CostItem{{ID: 1201, Amount: 1, Refine: 9}, {ID: 909, Amount: 2}}, Stock: -1, Initial: -1},
	})
	z.Mods = &Mods{Shops: shop.NewRegistry([]*shop.Shop{s})}
	a.maxWeight = 1 << 30
	buyNow := func() wathenav1.ShopOutcome {
		z.openShop(a, s, nil)
		z.buy(a, 0, []buyLine{{id: 501, amount: 1}})
		z.commitTransfers()
		z.stepAt(z.now + 50)
		o, _ := shopResultOf(events(a))
		return o
	}
	refines := func() []int {
		var out []int
		for _, sl := range a.inventory.Slots {
			if sl.ID == 1201 && sl.Amount > 0 {
				out = append(out, sl.Refine)
			}
		}
		return out
	}
	if o := buyNow(); o != wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT {
		t.Fatalf("first buy: %v", o)
	}
	if got := refines(); len(got) != 3 || got[0] != 7 || got[1] != 12 || got[2] != 9 {
		t.Errorf("after the first buy, knives at %v, want the +10 gone", got)
	}
	if o := buyNow(); o != wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT {
		t.Fatalf("second buy: %v", o)
	}
	if o := buyNow(); o != wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_ITEMS {
		t.Errorf("with a +7 and a carded +9 left: %v", o)
	}
	if got := refines(); len(got) != 2 || inventoryCount(a.inventory, 909) != 6 || inventoryCount(a.inventory, 501) != 2 {
		t.Errorf("knives %v, jellopy %d, potions %d", got, inventoryCount(a.inventory, 909), inventoryCount(a.inventory, 501))
	}
}

// TestBarterRefineCapped asks for a refine above MAX_REFINE, which
// barter_parseBodyNode caps: a copy at MAX_REFINE pays it.
func TestBarterRefineCapped(t *testing.T) {
	z, a, _, _ := marketZone(t, character.Item{Slot: 0, ID: 1201, Amount: 1, Refine: 20, Serial: item.NewSerial()})
	s := shop.New("core:barter_cap", "Cap", false, false, "core", []shop.Item{
		{ID: 501, Items: []shop.CostItem{{ID: 1201, Amount: 1, Refine: 25}}, Stock: -1, Initial: -1},
	})
	z.Mods = &Mods{Shops: shop.NewRegistry([]*shop.Shop{s})}
	a.maxWeight = 1 << 30
	z.openShop(a, s, nil)
	z.buy(a, 0, []buyLine{{id: 501, amount: 1}})
	z.commitTransfers()
	z.stepAt(z.now + 50)
	if o, _ := shopResultOf(events(a)); o != wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT {
		t.Errorf("buy with a +20 for a +25 cost: %v", o)
	}
}
