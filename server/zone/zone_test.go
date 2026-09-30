package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/shared/path"
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
	p := &Player{Name: name, Out: out, out: out, netIDs: map[*entity]uint32{}, fresh: true}
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
	z := New("map/test", open(40))
	a := add(z, "a")
	b := add(z, "b")
	z.stepAt(50)
	// Each sees itself and the other.
	for _, p := range []*Player{a, b} {
		ev := events(p)
		if len(ev) != 2 || ev[0].GetAppear() == nil || ev[1].GetAppear() == nil {
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
	z := New("map/test", open(80))
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
	z := New("map/test", open(40))
	a := add(z, "a")
	for range outBuffer + 1 {
		a.queue(&wathenav1.ServerMessage{})
	}
	if !a.dropped {
		t.Error("a full queue did not drop the player")
	}
}

func TestPackSteps(t *testing.T) {
	// North is facing 4, south-west 1.
	if got := PackSteps([]path.Dir{path.North, path.SouthWest}); len(got) != 1 || got[0] != 0x14 {
		t.Errorf("%x", got)
	}
}
