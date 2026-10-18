package zone

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// A player with one big stack can drop it one item at a time with no cap.
// A player that then comes into view gets every item in the first frame,
// which the gateway refuses to write when it is over MaxMessageBytes, so
// the connection ends.
func TestDropFloodFirstFrameStaysUnderMessageLimit(t *testing.T) {
	z, a, _ := invZone(t)
	a.maxWeight = 1 << 30
	z.addItem(a, 909, 30000, scriptOrigin)
	slot := -1
	for i, s := range a.inventory.Slots {
		if s.ID == 909 && s.Amount == 30000 {
			slot = i
		}
	}
	if slot < 0 {
		t.Fatal("no stack of 30000")
	}
	const drops = 3000
	for range drops {
		drop(z, a, slot, 1)
	}
	z.stepAt(100)
	events(a)

	c := add(z, "c")
	z.stepAt(150)
	biggest := 0
	for {
		select {
		case m := <-c.Out:
			biggest = max(biggest, proto.Size(m))
			continue
		default:
		}
		break
	}
	if biggest > schema.MaxMessageBytes {
		t.Errorf("the first frame of a joining player is %d bytes after %d drops; the gateway refuses over %d and ends the connection",
			biggest, drops, schema.MaxMessageBytes)
	}
}

// TestDropLimits checks the drop rate, the cap on a player's items on
// the ground, and the cap on a map's ground items.
func TestDropLimits(t *testing.T) {
	z, a, _ := invZone(t)
	a.maxWeight = 1 << 30
	z.addItem(a, 909, 30000, scriptOrigin)
	slot := slotOf(t, a, 909)
	count := func() int {
		n := 0
		for e := range z.entities.all() {
			if e.item != nil {
				n++
			}
		}
		return n
	}
	for range 3 * dropBurst {
		drop(z, a, slot, 1)
	}
	if n := count(); n != dropBurst {
		t.Fatalf("%d items on the ground after a burst, want %d", n, dropBurst)
	}
	failed := 0
	z.stepAt(z.now)
	for _, e := range events(a) {
		if f := e.GetDropItemFailed(); f != nil && f.GetReason() == wathenav1.DropItemFailure_DROP_ITEM_FAILURE_TOO_MANY {
			failed++
		}
	}
	if failed != 2*dropBurst {
		t.Errorf("%d drops refused as too many, want %d", failed, 2*dropBurst)
	}
	// One drop per dropEvery, up to maxPlayerDrops on the ground.
	for range 2 * maxPlayerDrops {
		z.now += dropEvery
		drop(z, a, slot, 1)
	}
	if n := count(); n != maxPlayerDrops || a.onGround != maxPlayerDrops {
		t.Fatalf("%d items on the ground, %d counted, want %d", n, a.onGround, maxPlayerDrops)
	}
	// Picking one up frees a place.
	for e := range z.entities.all() {
		if e.item != nil {
			z.entities.remove(e)
			break
		}
	}
	z.now += dropEvery
	drop(z, a, slot, 1)
	if a.onGround != maxPlayerDrops {
		t.Errorf("%d counted after a pick-up and a drop, want %d", a.onGround, maxPlayerDrops)
	}
	// A full map refuses players' drops.
	for i := range maxGroundItems {
		z.entities.add(&entity{x: 10 + i%20, y: 10 + i/20%20, item: &groundItem{id: 909, amount: 1, expires: z.now + 60000}})
	}
	b := add(z, "b")
	b.maxWeight = 1 << 30
	z.addItem(b, 909, 10, scriptOrigin)
	drop(z, b, slotOf(t, b, 909), 1)
	if b.onGround != 0 {
		t.Error("a player dropped on a full map")
	}
}

// TestManyGroundItemsSplitFrames checks that a player who comes into
// view of more ground items than one frame holds gets them all, in
// frames that each stay under schema.MaxMessageBytes.
func TestManyGroundItemsSplitFrames(t *testing.T) {
	z, a, _ := invZone(t)
	const items = 6000
	for i := range items {
		z.entities.add(&entity{x: a.entity.x - 5 + i%10, y: a.entity.y - 5 + i/10%10, item: &groundItem{id: 909, amount: 1 + i, expires: z.now + 60000}})
	}
	c := add(z, "c")
	z.stepAt(z.now + 50)
	frames, seen := 0, 0
	var tick uint32
	for {
		var m *wathenav1.ServerMessage
		select {
		case m = <-c.Out:
		default:
		}
		if m == nil {
			break
		}
		if size := proto.Size(m); size > schema.MaxMessageBytes {
			t.Errorf("a frame of %d bytes", size)
		}
		if f := m.GetFrame(); f != nil {
			if frames > 0 && f.GetTick() != tick {
				t.Errorf("frame tick %d, want %d", f.GetTick(), tick)
			}
			frames, tick = frames+1, f.GetTick()
			for _, e := range f.GetEvents() {
				if ap := e.GetAppear(); ap != nil && ap.HasItem() {
					seen++
				}
			}
		}
	}
	if seen != items || frames < 2 {
		t.Errorf("%d items in %d frames, want %d in more than one", seen, frames, items)
	}
}
