package zone

import (
	"fmt"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// open returns a size × size grid with every cell open.
func open(size int) *path.Grid {
	g := &path.Grid{Width: size, Height: size, Cells: make([]uint8, size*size)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	return g
}

// add joins a player directly, as Run would.
func add(z *Zone, name string) *Player {
	out := make(chan *wathenav1.ServerMessage, outBuffer)
	p := &Player{Name: name, Out: out, out: out, fresh: true}
	z.join(p)
	return p
}

// events drains a player's messages and returns their events.
func events(p *Player) []*wathenav1.Event {
	var out []*wathenav1.Event
	for {
		select {
		case m, ok := <-p.Out:
			if !ok {
				return out
			}
			out = append(out, m.GetFrame().GetEvents()...)
		default:
			return out
		}
	}
}

func TestSeeAndWalk(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{}).Stream(), nil)
	a := add(z, "a")
	b := add(z, "b")
	z.stepAt(50)
	// Each sees itself and the other, then gets its inventory.
	for _, p := range []*Player{a, b} {
		ev := events(p)
		if len(ev) != 3 || ev[0].GetAppear() == nil || ev[1].GetAppear() == nil || ev[2].GetInventory() == nil {
			t.Fatalf("%s: %v", p.Name, ev)
		}
	}

	// a walks three cells east. b sees the walk start.
	ax, ay := a.entity.x, a.entity.y
	z.handle(command{player: a, seq: 1, walkTo: &[2]int{ax + 3, ay}})
	z.stepAt(100)
	ev := events(b)
	if len(ev) != 1 || ev[0].GetMove() == nil {
		t.Fatalf("b sees %v", ev)
	}
	m := ev[0].GetMove()
	if m.GetFromX() != uint32(ax) || m.GetWalkMs() != WalkSpeed {
		t.Errorf("move %v", m)
	}
	// Three east steps: facing 6 each, then padding.
	if got := m.GetSteps(); len(got) != 2 || got[0] != 0x66 || got[1] != 0xF6 {
		t.Errorf("steps %x", got)
	}
	frames := events(a)
	if len(frames) != 1 {
		t.Fatalf("a sees %v", frames)
	}

	// After 450 ms the walk is done.
	z.stepAt(100 + 3*WalkSpeed)
	if a.entity.x != ax+3 || len(a.entity.path) != 0 || a.entity.dir != 6 {
		t.Errorf("a is at %d, path %v, facing %d", a.entity.x, a.entity.path, a.entity.dir)
	}
	if a.ackSeq != 1 {
		t.Errorf("ack %d", a.ackSeq)
	}
}

func TestViewRangeAndLeave(t *testing.T) {
	z := New("map/test", open(80), rng.FromKey([32]byte{}).Stream(), nil)
	a := add(z, "a")
	b := add(z, "b")
	z.stepAt(50)
	events(a)
	events(b)

	// b walks out of a's view, 15 cells away, in steps of at most 32.
	b.entity.x = a.entity.x + 15
	z.stepAt(100)
	ev := events(a)
	if len(ev) != 1 || ev[0].GetVanish().GetReason() != wathenav1.VanishReason_VANISH_REASON_OUT_OF_SIGHT {
		t.Fatalf("a sees %v", ev)
	}
	b.entity.x = a.entity.x + 14
	z.stepAt(150)
	if ev := events(a); len(ev) != 1 || ev[0].GetAppear() == nil {
		t.Fatalf("a sees %v", ev)
	}

	z.leave(b, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	ev = events(a)
	if len(ev) != 1 || ev[0].GetVanish().GetReason() != wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT {
		t.Fatalf("a sees %v", ev)
	}
	// b's channel closes after its last frames.
	for range b.Out {
	}
	if !b.dropped {
		t.Error("b is still in the zone")
	}
}

func TestSlowPlayerIsDropped(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{}).Stream(), nil)
	a := add(z, "a")
	for range outBuffer + 1 {
		z.queue(a, &wathenav1.ServerMessage{})
	}
	if !a.dropped {
		t.Error("a full queue did not drop the player")
	}

	// With the sender, as in Run, the zone sees the drop once the
	// sender delivered the batch.
	b := add(z, "b")
	z.startSender()
	for range outBuffer + 1 {
		z.queue(b, &wathenav1.ServerMessage{})
	}
	if b.gone() {
		t.Error("b was dropped before the batch went out")
	}
	z.stopSender()
	if !b.gone() {
		t.Error("a full queue did not drop b")
	}
	n := 0
	for range b.Out {
		n++
	}
	if n != outBuffer {
		t.Errorf("b got %d messages, want %d", n, outBuffer)
	}
}

// TestSenderOrder checks that the sender keeps a batch in order, steps
// included.
func TestSenderOrder(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{}).Stream(), nil)
	a := add(z, "a")
	events(a)
	z.startSender()
	var got []uint32
	for i := range uint32(3) {
		z.queue(a, frame(i, 0))
	}
	z.then(func() {
		for range 3 {
			got = append(got, (<-a.Out).GetFrame().GetTick())
		}
	})
	z.drop(a)
	z.stopSender()
	if len(got) != 3 || got[0] != 0 || got[2] != 2 {
		t.Errorf("frames in the order %v", got)
	}
	if _, open := <-a.Out; open {
		t.Error("the channel is open after drop")
	}
}

func TestPackSteps(t *testing.T) {
	// North is facing 4, south-west 1.
	if got := PackSteps([]path.Dir{path.North, path.SouthWest}); len(got) != 1 || got[0] != 0x14 {
		t.Errorf("%x", got)
	}
}

func TestMonsters(t *testing.T) {
	spawns := []Spawn{
		{Mob: 1002, Name: "Poring", Amount: 5, X: 20, Y: 20, W: 3, H: 3, Speed: 400, CanMove: true},
		{Mob: 1078, Name: "Red Plant", Amount: 2, Speed: 2000},
	}
	z := New("map/test", open(40), rng.FromKey([32]byte{1}).Stream(), spawns)
	var porings, plants []*entity
	for e := range z.entities.all() {
		switch e.monster.spawn.Mob {
		case 1002:
			porings = append(porings, e)
			if e.x < 17 || e.x > 23 || e.y < 17 || e.y > 23 {
				t.Errorf("poring spawned at (%d, %d), outside its area", e.x, e.y)
			}
		case 1078:
			plants = append(plants, e)
		}
	}
	if len(porings) != 5 || len(plants) != 2 {
		t.Fatalf("%d porings, %d plants", len(porings), len(plants))
	}

	// A player near the porings sees them as monsters.
	p := add(z, "a")
	p.entity.x, p.entity.y = 20, 20
	z.stepAt(50)
	seen := 0
	for _, e := range events(p) {
		if e.GetAppear().GetKind() == wathenav1.EntityKind_ENTITY_KIND_MONSTER && e.GetAppear().GetLook().GetBody() == 1002 {
			seen++
		}
	}
	if seen != 5 {
		t.Errorf("sees %d porings", seen)
	}

	// Within 5 seconds of the first think, every poring starts wandering,
	// at most 7 cells away. Plants cannot move.
	start := map[*entity][2]int{}
	for _, e := range porings {
		start[e] = [2]int{e.x, e.y}
	}
	walked := map[*entity]bool{}
	for now := int64(100); now <= 5200; now += 50 {
		z.stepAt(now)
		for _, e := range porings {
			if len(e.path) > 0 {
				walked[e] = true
				tx, ty := e.x, e.y
				for _, d := range e.path {
					dx, dy := d.Step()
					tx += dx
					ty += dy
				}
				if abs(tx-start[e][0]) > 7+1 || abs(ty-start[e][1]) > 7+1 {
					t.Errorf("poring walks to (%d, %d) from %v", tx, ty, start[e])
				}
			}
		}
	}
	if len(walked) != 5 {
		t.Errorf("%d of 5 porings wandered", len(walked))
	}
	for _, e := range plants {
		if len(e.path) > 0 || e.monster.nextWalk != -1 && e.monster.spawn.CanMove {
			t.Error("a plant moved")
		}
	}
	// The player saw the walks as moves.
	moves := 0
	for _, e := range events(p) {
		if e.HasMove() {
			moves++
		}
	}
	if moves < 5 {
		t.Errorf("saw %d monster moves", moves)
	}
}

// BenchmarkIdleTick is one tick of a map with monsters and no players,
// which a sleeping zone skips. 60 is the average count of monsters per
// map over all maps with spawns in renewal, and 215 the average per field.
func BenchmarkIdleTick(b *testing.B) {
	for _, n := range []int{60, 215} {
		b.Run(fmt.Sprintf("monsters=%d", n), func(b *testing.B) {
			spawns := []Spawn{{Mob: 1002, Name: "Poring", Amount: n, Speed: 400, CanMove: true}}
			z := New("map/test", open(300), rng.FromKey([32]byte{4}).Stream(), spawns)
			now := int64(0)
			for b.Loop() {
				now += 50
				z.stepAt(now)
			}
		})
	}
}
