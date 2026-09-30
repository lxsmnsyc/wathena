package world

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

func testZone(name string) *zone.Zone {
	g := &path.Grid{Width: 30, Height: 30, Cells: make([]uint8, 900)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	return zone.New(name, g, rng.FromKey([32]byte{3}).Stream(), nil)
}

func TestNew(t *testing.T) {
	if _, err := New("map/a", testZone("map/a"), testZone("map/a")); err == nil {
		t.Error("a map listed twice is accepted")
	}
	if _, err := New("map/c", testZone("map/a")); err == nil {
		t.Error("a start map that does not run is accepted")
	}
	a, b := testZone("map/a"), testZone("map/b")
	w, err := New("map/a", b, a)
	if err != nil {
		t.Fatal(err)
	}
	if w.Zone("map/b") != b || w.Zone("map/none") != nil || w.Start() != a || a.World != w || b.World != w {
		t.Error("zones are not found")
	}
	if z := w.Zones(); len(z) != 2 || z[0] != a || z[1] != b {
		t.Error("Zones is not sorted by map")
	}
}

func TestJoinGoesToTheCharactersMap(t *testing.T) {
	a, b := testZone("map/a"), testZone("map/b")
	w, err := New("map/a", a, b)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	c := w.NewCharacter("Mover", zone.Look{Sex: 1})
	if c.Map != "map/a" {
		t.Fatalf("a new character starts on %s", c.Map)
	}
	c.Map, c.X, c.Y = "map/b", 7, 8
	s, err := w.JoinCharacter(ctx, &c, nil)
	if err != nil || s.Map != "map/b" {
		t.Fatalf("joined %v, %v", s, err)
	}
	lost := w.NewCharacter("Lost", zone.Look{})
	lost.Map = "map/gone"
	s2, err := w.JoinCharacter(ctx, &lost, nil)
	if err != nil || s2.Map != "map/a" {
		t.Fatalf("a character on a map that does not run joined %v, %v", s2, err)
	}
	cancel()
	select {
	case <-w.Stopped():
	case <-time.After(5 * time.Second):
		t.Fatal("the world did not stop")
	}
	// Both sessions ended with the world.
	for range s.Out {
	}
	for range s2.Out {
	}
}

func TestLoadWarps(t *testing.T) {
	for _, mode := range reference.Modes {
		warps, err := LoadWarps(mode)
		if err != nil {
			t.Fatal(err)
		}
		// prontera's south gate leads to prt_fild08 in both modes.
		found := false
		for _, w := range warps["map/prontera"] {
			if w.X == 156 && w.Y == 22 {
				found = true
				if w.To != "map/prt_fild08" || w.ToX != 170 || w.ToY != 375 || w.XS != 3 || w.YS != 2 {
					t.Errorf("%s: prontera's south gate %+v", mode, w)
				}
			}
		}
		if !found {
			t.Errorf("%s: no warp at prontera 156,22", mode)
		}
		if len(warps) < 400 {
			t.Errorf("%s: warps on %d maps", mode, len(warps))
		}
	}
}

func TestByMapKeepsEnabledWarpsInOrder(t *testing.T) {
	var d WarpData
	add := func(m string, x int, state string) {
		d.Warps = append(d.Warps, struct {
			Map   string `json:"map"`
			X     int    `json:"x"`
			Y     int    `json:"y"`
			Name  string `json:"name"`
			XS    int    `json:"xs"`
			YS    int    `json:"ys"`
			ToMap string `json:"to_map"`
			ToX   int    `json:"to_x"`
			ToY   int    `json:"to_y"`
			Kind  string `json:"kind"`
			State string `json:"state"`
		}{Map: m, X: x, Y: 1, XS: 1, YS: 1, ToMap: "b", State: state})
	}
	add("a", 3, "enabled")
	add("a", 1, "disabled")
	add("a", 2, "enabled")
	add("c", 5, "hidden")
	got := d.ByMap()
	if len(got) != 1 || len(got["map/a"]) != 2 || got["map/a"][0].X != 3 || got["map/a"][1].X != 2 || got["map/a"][0].To != "map/b" {
		t.Errorf("by map %+v", got)
	}
}

// countSaves counts the saves of one session.
type countSaves struct {
	mu          sync.Mutex
	saves, last int
}

func (c *countSaves) Save(_ *character.Character, final bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if final {
		c.last++
	} else {
		c.saves++
	}
}

// Many players cross between two running maps at once. Each ends in one
// zone, and each session saves once per map change and once at the end.
func TestPlayersCrossMapsConcurrently(t *testing.T) {
	a, b := testZone("map/a"), testZone("map/b")
	// Each map's center is a warp to the other map, 5 cells south of
	// its center.
	a.SetWarps([]zone.Warp{{Name: "ab", X: 15, Y: 15, To: "map/b", ToX: 15, ToY: 20}})
	b.SetWarps([]zone.Warp{{Name: "ba", X: 15, Y: 15, To: "map/a", ToX: 15, ToY: 20}})
	w, err := New("map/a", a, b)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	const n = 40
	sessions := make([]*zone.Session, n)
	savers := make([]*countSaves, n)
	for i := range n {
		c := w.NewCharacter("P", zone.Look{})
		c.X, c.Y = 15, 20
		savers[i] = &countSaves{}
		s, err := w.JoinCharacter(ctx, &c, savers[i])
		if err != nil {
			t.Fatal(err)
		}
		sessions[i] = s
		go func() {
			for range s.Out {
			}
		}()
	}
	// Everyone walks north through the center, on whichever map they
	// are, again and again.
	stop := time.Now().Add(2 * time.Second)
	for seq := uint32(2); time.Now().Before(stop); seq++ {
		for _, s := range sessions {
			s.WalkTo(seq, 15, 10)
		}
		time.Sleep(100 * time.Millisecond)
		for _, s := range sessions {
			s.WalkTo(seq, 15, 20)
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, s := range sessions {
		s.Leave()
	}
	changes := 0
	for i, c := range savers {
		c.mu.Lock()
		if c.last != 1 {
			t.Errorf("player %d: %d final saves", i, c.last)
		}
		changes += c.saves
		c.mu.Unlock()
	}
	if changes < n {
		t.Errorf("%d map changes for %d players", changes, n)
	}
	cancel()
	<-w.Stopped()
}
