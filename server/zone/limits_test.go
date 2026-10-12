package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/storage"
)

// limitsOf is the player's last PlayerLimits by name.
func limitsOf(p *Player) map[string]int {
	got := eventsOf(p, (*wathenav1.Event).GetPlayerLimits, (*wathenav1.Event).HasPlayerLimits)
	if len(got) == 0 {
		return nil
	}
	out := map[string]int{}
	for _, l := range got[len(got)-1].GetLimits() {
		out[l.GetName()] = int(l.GetValue())
	}
	return out
}

// TestExpandLimits checks player:expand's zone side (review decision 32):
// each limit stays within its base and max, the change joins the next
// commit, the player gets PlayerLimits, and an account's characters
// share the storage's size.
func TestExpandLimits(t *testing.T) {
	z, a, _, cm := storageZone(t)
	r := rules.Base(true)
	var err error
	r.Limits, err = rules.LimitsOf(map[string]rules.RangeSet{
		"cart_slots": {Max: 150, HasMax: true}, "storage_slots": {Max: 700, HasMax: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	z.Mods = &Mods{Rules: r}

	// The inventory: savedCharacter has 110 slots, of the range 100 to
	// 200.
	for _, c := range []struct{ amount, want int }{{20, 130}, {500, 200}, {-1000, 100}, {0, 100}, {15, 115}} {
		got, err := z.expandLimit(a, "inventory_slots", c.amount)
		if err != nil || got != c.want || a.inventory.Size != c.want || len(a.inventory.Slots) < c.want {
			t.Errorf("inventory %+d: %d, %v, size %d with %d slots, want %d", c.amount, got, err, a.inventory.Size, len(a.inventory.Slots), c.want)
		}
	}
	z.commitTransfers()
	if len(cm.got) != 1 || len(cm.got[0].saves) != 1 || cm.got[0].saves[0].Character.InventorySize != 115 {
		t.Fatalf("the inventory's commit %+v", cm.got)
	}
	z.stepAt(z.now + 50)
	if got := limitsOf(a); got["inventory_slots"] != 115 || got["cart_slots"] != 100 || got["storage_slots"] != 600 {
		t.Errorf("PlayerLimits %v", got)
	}

	// The cart and the storage.
	if got, err := z.expandLimit(a, "cart_slots", 30); err != nil || got != 130 {
		t.Errorf("cart: %d, %v", got, err)
	}
	if got, err := z.expandLimit(a, "storage_slots", 500); err != nil || got != 700 {
		t.Errorf("storage: %d, %v", got, err)
	}
	z.commitTransfers()
	sizes := map[storage.Holder][2]int{}
	for _, s := range cm.got[len(cm.got)-1].storages {
		sizes[s.Key.Holder] = [2]int{s.SizeBase, s.Size}
	}
	if sizes[storage.HolderCart] != [2]int{0, 130} || sizes[storage.HolderAccount] != [2]int{0, 700} {
		t.Errorf("storage commits %v", sizes)
	}
	if st := z.cartOf(a); st == nil || z.prepareCart(st).Size != 130 {
		t.Error("the cart's box does not have 130 slots")
	}

	// Another character of the account shares the storage's size, and
	// sees it at login.
	c := savedCharacter()
	c.ID, c.Name, c.X = 9, "Twin", 13
	twin := addCharacter(z, c, &saves{})
	z.stepAt(z.now + 50)
	if got := limitsOf(twin); got["storage_slots"] != 700 || got["cart_slots"] != 100 || got["inventory_slots"] != 110 {
		t.Errorf("the twin's PlayerLimits %v", got)
	}
	if !z.openPersonalStorage(twin, nil) {
		t.Fatal("the twin cannot open the storage")
	}
	z.stepAt(z.now + 50)
	so := eventsOf(twin, (*wathenav1.Event).GetStorageOpen, (*wathenav1.Event).HasStorageOpen)
	if len(so) != 1 || so[0].GetSize() != 700 {
		t.Errorf("the twin's storage %v", so)
	}

	// Lowering to the base saves no expansion.
	if got, err := z.expandLimit(twin, "storage_slots", -10_000); err != nil || got != 600 {
		t.Errorf("storage lowered: %d, %v", got, err)
	}
	z.commitTransfers()
	for _, s := range cm.got[len(cm.got)-1].storages {
		if s.Key.Holder == storage.HolderAccount && (s.SizeBase != 700 || s.Size != 0) {
			t.Errorf("the lowered storage's commit %d to %d", s.SizeBase, s.Size)
		}
	}
	if got, _ := z.playerLimit(a, "storage_slots"); got != 600 {
		t.Errorf("the first character reads %d storage slots", got)
	}
}

// TestRestoredInventoryClamped checks that a saved inventory size outside
// the limit's range loads within it.
func TestRestoredInventoryClamped(t *testing.T) {
	l, err := rules.LimitsOf(map[string]rules.RangeSet{"inventory_slots": {Base: 120, Max: 150, HasBase: true, HasMax: true}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ saved, want int }{{0, 120}, {100, 120}, {130, 130}, {300, 150}} {
		ch := savedCharacter()
		ch.InventorySize = c.saved
		if got := restoreInventory(storageTestItems(), ch, l).Size; got != c.want {
			t.Errorf("saved %d: size %d, want %d", c.saved, got, c.want)
		}
	}
}
