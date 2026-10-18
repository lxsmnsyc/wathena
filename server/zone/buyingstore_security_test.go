package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
)

// TestBuyingStoreSaleCountsSlotsTogether checks that a sale that names
// the same item in two slots cannot sell more than the store's line
// wants. Each slot alone is within the line's amount.
func TestBuyingStoreSaleCountsSlotsTogether(t *testing.T) {
	z, a, b, _ := marketZone(t, stack(0, 909, 10), stack(1, 909, 10))
	if a.inventory.Slots[0].ID != 909 || a.inventory.Slots[1].ID != 909 {
		t.Fatalf("the seller does not hold two stacks: %+v", a.inventory.Slots[:2])
	}
	b.inventory.AddExtra(z.Items.Get(909), 1, maxWeight, item.Extra{})
	b.record.Zeny = 100000
	if !z.setupBuyingStore(b, 2) {
		t.Fatal("setup")
	}
	z.buyingCommand(b, wathenav1.BuyingStoreCommand_builder{Open: wathenav1.BuyingStoreOpen_builder{Title: "Buying jellopy",
		ZenyLimit: 100000, Items: []*wathenav1.BuyingStoreLine{wathenav1.BuyingStoreLine_builder{ItemId: 909, Amount: 10, Price: 20}.Build()}}.Build()}.Build())
	z.stepAt(z.now + 50)
	if b.buying == nil {
		t.Fatalf("not open: %v", events(b))
	}
	line := func(slot int) *wathenav1.BuyingStoreSellLine {
		return wathenav1.BuyingStoreSellLine_builder{Slot: uint32(slot), ItemId: 909, Amount: 10}.Build() // #nosec G115 -- a test slot.
	}
	z.buyingCommand(a, wathenav1.BuyingStoreCommand_builder{Sell: wathenav1.BuyingStoreSell_builder{Buyer: netIDOf(a, b),
		Store: b.buying.id, Items: []*wathenav1.BuyingStoreSellLine{line(0), line(1)}}.Build()}.Build())
	z.commitTransfers()
	z.stepAt(z.now + 50)
	if got := inventoryCount(b.inventory, 909); got != 1 {
		t.Errorf("the store bought %d, want none of a sale over its amount", got-1)
	}
	if b.record.Zeny != 100000 {
		t.Errorf("the owner paid %d", 100000-b.record.Zeny)
	}
	if b.buying != nil && b.buying.lines[0].amount < 0 {
		t.Errorf("the line's amount went to %d", b.buying.lines[0].amount)
	}
}
