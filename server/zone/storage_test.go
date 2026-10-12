package zone

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// storageTestItems has items with each restriction.
func storageTestItems() *item.Table {
	return item.New(&item.Data{Mode: "renewal", Items: []item.Def{
		{ID: 501, AegisName: "Red_Potion", Type: "Healing", Weight: 70, Stackable: true},
		{ID: 909, AegisName: "Jellopy", Type: "Etc", Weight: 10, Stackable: true},
		{ID: 1201, AegisName: "Knife", Type: "Weapon", Weight: 400, Slots: 3},
		{ID: 7001, AegisName: "No_Store", Type: "Etc", Weight: 1, Stackable: true, Restrictions: []string{"no_storage"}},
		{ID: 7002, AegisName: "No_Cart", Type: "Etc", Weight: 1, Stackable: true, Restrictions: []string{"no_cart"}},
		{ID: 7003, AegisName: "No_Trade", Type: "Etc", Weight: 1, Stackable: true, Restrictions: []string{"no_trade", "no_drop"}},
		{ID: 7004, AegisName: "No_Trade_Card", Type: "Card", Weight: 1, Stackable: true, Restrictions: []string{"no_trade"}},
		{ID: 7005, AegisName: "Heavy", Type: "Etc", Weight: 3000, Stackable: true},
	}})
}

// storageZone is a zone with the storage items and two players next to
// each other, with Basic Skill and room to carry.
func storageZone(t *testing.T, items ...character.Item) (*Zone, *Player, *Player, *commits) {
	t.Helper()
	z := saveZone()
	z.Items = storageTestItems()
	cm := &commits{}
	z.Committer = cm
	ac := savedCharacter()
	ac.Items, ac.Skills = items, map[int]int{skillBasic: 9}
	a := addCharacter(z, ac, &saves{})
	bc := savedCharacter()
	bc.ID, bc.AccountID, bc.Name, bc.X, bc.Items, bc.Skills = 8, 4, "Other", 12, nil, map[int]int{skillBasic: 9}
	b := addCharacter(z, bc, &saves{})
	a.maxWeight, b.maxWeight = maxWeight, maxWeight
	z.stepAt(50)
	events(a)
	events(b)
	return z, a, b, cm
}

func stack(slot int, id uint32, amount int) character.Item {
	return character.Item{Slot: slot, ID: id, Amount: amount, Serial: item.NewSerial()}
}

func move(z *Zone, p *Player, kind StorageMoveKind, slot, amount int) []*wathenav1.Event {
	z.handle(command{player: p, seq: 1, storage: &storageCommand{move: &storageMove{kind: kind, slot: slot, amount: amount}}})
	z.commitTransfers()
	z.stepAt(z.now + 50)
	return events(p)
}

func storageOutcomes(ev []*wathenav1.Event) []wathenav1.StorageOutcome {
	var out []wathenav1.StorageOutcome
	for _, e := range ev {
		if e.HasStorageResult() {
			out = append(out, e.GetStorageResult().GetOutcome())
		}
	}
	return out
}

func TestPersonalStorageMoves(t *testing.T) {
	z, a, _, cm := storageZone(t, stack(0, 909, 10), stack(1, 501, 5))
	if !z.openPersonalStorage(a, nil) {
		t.Fatal("the storage did not open")
	}
	z.stepAt(100)
	so := eventsOf(a, (*wathenav1.Event).GetStorageOpen, (*wathenav1.Event).HasStorageOpen)
	if len(so) != 1 || so[0].GetKind() != wathenav1.StorageKind_STORAGE_KIND_PERSONAL || so[0].GetSize() != 600 || so[0].GetDisplayName() != "Storage" {
		t.Fatalf("open %v", so)
	}
	// A second open does nothing, as storage_flag.
	if z.openPersonalStorage(a, nil) {
		t.Error("opened twice")
	}
	ev := move(z, a, MovePut, 0, 4)
	adds := eventsOf2(ev, (*wathenav1.Event).GetStorageAdd, (*wathenav1.Event).HasStorageAdd)
	if len(adds) != 1 || adds[0].GetItem().GetAmount() != 4 || a.inventory.Count(909) != 6 {
		t.Fatalf("put %v, inventory %d", adds, a.inventory.Count(909))
	}
	last := cm.got[len(cm.got)-1]
	if len(last.storages) != 1 || last.storages[0].Key.Holder != storage.HolderAccount || last.storages[0].Key.ID != 3 ||
		len(last.storages[0].Cur) != 1 || len(last.logs.Items) != 1 || last.logs.Items[0].Action != transfer.ActionStorage ||
		last.logs.Items[0].To.Kind != transfer.KindStorage || last.logs.Items[0].To.ID != 3 || len(last.saves) != 1 {
		t.Fatalf("commit %+v", last)
	}
	ev = move(z, a, MoveTake, 0, 1)
	if rm := eventsOf2(ev, (*wathenav1.Event).GetStorageRemove, (*wathenav1.Event).HasStorageRemove); len(rm) != 1 || a.inventory.Count(909) != 7 {
		t.Fatalf("take %v", rm)
	}
	// Closing lets go: another character of the account may open it.
	z.handle(command{player: a, seq: 2, storage: &storageCommand{close: true}})
	if a.storage != nil {
		t.Fatal("still open")
	}
	z.stepAt(z.now + 50)
	// Closed, a put does nothing.
	if ev := move(z, a, MovePut, 0, 1); a.inventory.Count(909) != 7 || len(eventsOf2(ev, (*wathenav1.Event).GetStorageAdd, (*wathenav1.Event).HasStorageAdd)) != 0 {
		t.Error("a put with no storage open moved items")
	}
	// The storage keeps its items for the next open.
	z.openPersonalStorage(a, nil)
	z.stepAt(z.now + 50)
	so = eventsOf(a, (*wathenav1.Event).GetStorageOpen, (*wathenav1.Event).HasStorageOpen)
	if len(so) != 1 || len(so[0].GetItems()) != 1 || so[0].GetItems()[0].GetAmount() != 3 {
		t.Fatalf("reopen %v", so)
	}
}

func TestStorageRestrictionsAndBound(t *testing.T) {
	items := []character.Item{stack(0, 7001, 1), stack(1, 7002, 1), stack(2, 909, 1), stack(3, 909, 1), stack(4, 909, 1)}
	items[2].Bound = item.BoundCharacter
	items[3].Bound = item.BoundAccount
	items[4].Bound = item.BoundGuild
	z, a, _, _ := storageZone(t, items...)
	z.setCart(a, 0)
	a.char.Skills[skillPushCart] = 5
	if !z.setCart(a, 1) {
		t.Fatal("no cart")
	}
	z.openPersonalStorage(a, nil)
	z.stepAt(100)
	events(a)
	for _, c := range []struct {
		name string
		kind StorageMoveKind
		slot int
		want wathenav1.StorageOutcome
	}{
		{"no_storage", MovePut, 0, wathenav1.StorageOutcome_STORAGE_OUTCOME_RESTRICTED},
		{"no_cart", MoveCartPut, 1, wathenav1.StorageOutcome_STORAGE_OUTCOME_RESTRICTED},
		{"character-bound to storage", MovePut, 2, wathenav1.StorageOutcome_STORAGE_OUTCOME_BOUND},
		{"character-bound to cart", MoveCartPut, 2, wathenav1.StorageOutcome_STORAGE_OUTCOME_BOUND},
		{"guild-bound to storage", MovePut, 4, wathenav1.StorageOutcome_STORAGE_OUTCOME_BOUND},
		{"account-bound to storage", MovePut, 3, 0},
		{"no_storage to the cart", MoveCartPut, 0, 0},
		{"no_cart to storage", MovePut, 1, 0},
	} {
		got := storageOutcomes(move(z, a, c.kind, c.slot, 1))
		if c.want == 0 && len(got) != 0 || c.want != 0 && (len(got) != 1 || got[0] != c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	// The no_storage item in the cart cannot go to storage either.
	cart := z.cartOf(a)
	cart.Lock()
	slot := -1
	for i, s := range cart.Box().Slots {
		if s.ID == 7001 {
			slot = i
		}
	}
	cart.Unlock()
	if got := storageOutcomes(move(z, a, MoveCartToStorage, slot, 1)); len(got) != 1 || got[0] != wathenav1.StorageOutcome_STORAGE_OUTCOME_RESTRICTED {
		t.Errorf("cart to storage: %v", got)
	}
}

func TestStorageSlotsLimit(t *testing.T) {
	z, a, _, _ := storageZone(t, stack(0, 909, 1), stack(1, 501, 1), stack(2, 1201, 1))
	r := rules.Base(true)
	var err error
	if r.Limits, err = rules.LimitsOf(map[string]rules.RangeSet{"storage_slots": {Base: 2, HasBase: true}}); err != nil {
		t.Fatal(err)
	}
	z.Mods = &Mods{Rules: r}
	z.openPersonalStorage(a, nil)
	z.stepAt(100)
	if so := eventsOf(a, (*wathenav1.Event).GetStorageOpen, (*wathenav1.Event).HasStorageOpen); len(so) != 1 || so[0].GetSize() != 2 {
		t.Fatalf("open %v", so)
	}
	move(z, a, MovePut, 0, 1)
	move(z, a, MovePut, 1, 1)
	if got := storageOutcomes(move(z, a, MovePut, 2, 1)); len(got) != 1 || got[0] != wathenav1.StorageOutcome_STORAGE_OUTCOME_FULL {
		t.Errorf("third stack: %v", got)
	}
	// Lowering the limit deletes nothing: the storage keeps both stacks
	// and takes no new one, but a stack may still grow.
	r.Limits, _ = rules.LimitsOf(map[string]rules.RangeSet{"storage_slots": {Base: 1, HasBase: true}})
	st := z.findStorage(a, nil)
	st.Lock()
	n := st.Used()
	st.Unlock()
	if n != 2 {
		t.Errorf("used %d after lowering", n)
	}
}

func TestCartSlotsLimit(t *testing.T) {
	z, a, _, _ := storageZone(t, stack(0, 909, 1), stack(1, 501, 1), stack(2, 1201, 1))
	r := rules.Base(true)
	r.Limits, _ = rules.LimitsOf(map[string]rules.RangeSet{"cart_slots": {Base: 2, HasBase: true}})
	z.Mods = &Mods{Rules: r}
	// No cart: the move is refused.
	if got := storageOutcomes(move(z, a, MoveCartPut, 0, 1)); len(got) != 1 || got[0] != wathenav1.StorageOutcome_STORAGE_OUTCOME_NO_CART {
		t.Errorf("no cart: %v", got)
	}
	a.char.Skills[skillPushCart] = 5
	z.setCart(a, 1)
	z.stepAt(z.now + 50)
	if so := eventsOf(a, (*wathenav1.Event).GetStorageOpen, (*wathenav1.Event).HasStorageOpen); len(so) != 1 ||
		so[0].GetKind() != wathenav1.StorageKind_STORAGE_KIND_CART || so[0].GetSize() != 2 || so[0].GetMaxWeight() != 8000 {
		t.Fatalf("cart list %v", so)
	}
	move(z, a, MoveCartPut, 0, 1)
	move(z, a, MoveCartPut, 1, 1)
	if got := storageOutcomes(move(z, a, MoveCartPut, 2, 1)); len(got) != 1 || got[0] != wathenav1.StorageOutcome_STORAGE_OUTCOME_FULL {
		t.Errorf("third stack: %v", got)
	}
	ev := move(z, a, MoveCartTake, 0, 1)
	if rm := eventsOf2(ev, (*wathenav1.Event).GetStorageRemove, (*wathenav1.Event).HasStorageRemove); len(rm) != 1 || a.inventory.Count(909) != 1 {
		t.Errorf("take %v", rm)
	}
	// Losing the cart closes it; its items stay, as pc_setcart.
	z.setCart(a, 0)
	z.stepAt(z.now + 50)
	if cl := eventsOf(a, (*wathenav1.Event).GetStorageClose, (*wathenav1.Event).HasStorageClose); len(cl) != 1 ||
		cl[0].GetKind() != wathenav1.StorageKind_STORAGE_KIND_CART {
		t.Errorf("close %v", cl)
	}
	if n := z.countStorage(a, z.cartOf(a), true, 501); n != -1 {
		t.Errorf("count without a cart: %d", n)
	}
	z.setCart(a, 2)
	if n := z.countStorage(a, z.cartOf(a), true, 501); n != 1 {
		t.Errorf("count after the cart came back: %d", n)
	}
}

func TestCartWeight(t *testing.T) {
	z, a, _, _ := storageZone(t, stack(0, 7005, 3))
	a.char.Skills[skillPushCart] = 5
	z.setCart(a, 1)
	// 3000 each: two fit in 8000, the third does not.
	move(z, a, MoveCartPut, 0, 2)
	ev := move(z, a, MoveCartPut, 0, 1)
	if got := storageOutcomes(ev); len(got) != 1 || got[0] != wathenav1.StorageOutcome_STORAGE_OUTCOME_CART_OVERWEIGHT {
		t.Errorf("third: %v", got)
	}
	r := rules.Base(true)
	r.Items, _ = rules.ItemsOf(map[string]int{"max_cart_weight": 9000})
	z.Mods = &Mods{Rules: r}
	ev = move(z, a, MoveCartPut, 0, 1)
	adds := eventsOf2(ev, (*wathenav1.Event).GetStorageAdd, (*wathenav1.Event).HasStorageAdd)
	if len(adds) != 1 || adds[0].GetWeight() != 9000 {
		t.Errorf("with max_cart_weight 9000: %v %v", adds, storageOutcomes(ev))
	}
	// Cart Revolution reads the cart's weight from the battle status.
	if pl := a.entity.status.Player; pl == nil || pl.CartWeight != 9000 || pl.CartWeightMax != 9000 {
		t.Errorf("battle status cart weight %+v", a.entity.status.Player)
	}
	if !hadCart(a.record.Options) || a.record.Options&companion.OptionCart == 0 {
		t.Error("no cart bit")
	}
}

func TestInventorySlotsLimit(t *testing.T) {
	z := saveZone()
	z.Items = storageTestItems()
	r := rules.Base(true)
	r.Limits, _ = rules.LimitsOf(map[string]rules.RangeSet{"inventory_slots": {Base: 150, Max: 300, HasBase: true, HasMax: true}})
	z.Mods = &Mods{Rules: r}
	n := z.NewCharacter("New", Look{})
	if n.InventorySize != 150 {
		t.Errorf("a new character has %d slots", n.InventorySize)
	}
	c := savedCharacter()
	c.InventorySize = 0
	c.Items = []character.Item{stack(250, 909, 1)}
	p := addCharacter(z, c, &saves{})
	if len(p.inventory.Slots) != 251 || p.inventory.Size != 150 || p.inventory.Count(909) != 1 {
		t.Errorf("slots %d, size %d", len(p.inventory.Slots), p.inventory.Size)
	}
	// A limit lowered below a saved slot keeps the item.
	r.Limits, _ = rules.LimitsOf(nil)
	q := addCharacter(z, &character.Character{Info: character.Info{ID: 9, Name: "Low", Map: "map/test", X: 5, Y: 5, InventorySize: 100},
		Items: []character.Item{stack(250, 909, 2)}}, &saves{})
	if len(q.inventory.Slots) != 251 || q.inventory.Count(909) != 2 {
		t.Errorf("lowered: %d slots, %d items", len(q.inventory.Slots), q.inventory.Count(909))
	}
}

// TestStoragePages checks that a full storage of the engine's ceiling
// goes out in frames under the message limit.
func TestStoragePages(t *testing.T) {
	z, a, _, _ := storageZone(t)
	r := rules.Base(true)
	r.Limits, _ = rules.LimitsOf(map[string]rules.RangeSet{"storage_slots": {Base: 10000, HasBase: true}})
	z.Mods = &Mods{Rules: r}
	st := z.findStorage(a, nil)
	st.Lock()
	box := st.Prepare(z.Items, 10000, 0)
	for i := range 10000 {
		s := &item.Slot{ID: 1201, Cards: []uint32{7004, 7004, 7004}, Refine: 10}
		if _, res := box.Put(s, 1); res != item.AddSuccess {
			t.Fatalf("put %d: %v", i, res)
		}
	}
	st.Unlock()
	z.openPersonalStorage(a, nil)
	got, pages, frames := 0, 0, 0
	for tick := range 10 {
		z.stepAt(int64(100 + 50*tick))
		if len(a.Out) > outBuffer/2 {
			t.Fatalf("%d messages wait after tick %d", len(a.Out), tick)
		}
	drain:
		for {
			select {
			case m := <-a.Out:
				frames++
				if n := proto.Size(m); n > schema.MaxMessageBytes {
					t.Fatalf("a frame of %d bytes", n)
				}
				for _, e := range m.GetFrame().GetEvents() {
					if e.HasStorageOpen() {
						got += len(e.GetStorageOpen().GetItems())
						pages = int(e.GetStorageOpen().GetPages())
					}
					if e.HasStorageItems() {
						got += len(e.GetStorageItems().GetItems())
					}
				}
			default:
				break drain
			}
		}
	}
	if got != 10000 || pages != 99 || frames != 100 {
		t.Errorf("%d items in %d frames, %d pages", got, frames, pages)
	}
}

// TestStorageAcrossCharacters checks that two characters of one account
// share its storage, one opener at a time.
func TestStorageAcrossCharacters(t *testing.T) {
	z, a, b, _ := storageZone(t, stack(0, 909, 5))
	b.record.AccountID = a.record.AccountID
	b.ownStorages = nil
	b.ownStorages = z.storagesOf(b)
	if z.findStorage(a, nil) != z.findStorage(b, nil) {
		t.Fatal("two storages for one account")
	}
	if z.cartOf(a) == z.cartOf(b) {
		t.Fatal("one cart for two characters")
	}
	z.openPersonalStorage(a, nil)
	if z.openPersonalStorage(b, nil) {
		t.Error("two openers")
	}
	z.stepAt(100)
	if got := storageOutcomes(events(b)); len(got) != 1 || got[0] != wathenav1.StorageOutcome_STORAGE_OUTCOME_IN_USE {
		t.Errorf("second opener: %v", got)
	}
	move(z, a, MovePut, 0, 5)
	z.closeStorage(a)
	if !z.openPersonalStorage(b, nil) {
		t.Fatal("b cannot open after a closed")
	}
	move(z, b, MoveTake, 0, 5)
	if b.inventory.Count(909) != 5 {
		t.Errorf("b took %d", b.inventory.Count(909))
	}
}
