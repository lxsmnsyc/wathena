package botclient

import (
	"math/rand/v2"
	"testing"

	"github.com/lxsmnsyc/wathena/shared/path"
)

// walled is an open grid with a wall across x = 10, open only at y = 18.
func walled() *path.Grid {
	g := open(20)
	for y := range 20 {
		if y != 18 {
			g.Cells[y*20+10] = 0
		}
	}
	return g
}

func TestRoute(t *testing.T) {
	g := walled()
	r := Route(g, 2, 2, 17, 2)
	if len(r) == 0 || r[0] != [2]int{2, 2} || r[len(r)-1] != [2]int{17, 2} {
		t.Fatalf("route %v", r)
	}
	for i := 1; i < len(r); i++ {
		a, b := r[i-1], r[i]
		dx, dy := b[0]-a[0], b[1]-a[1]
		if max(abs(dx), abs(dy)) != 1 || g.NoPass(b[0], b[1]) {
			t.Fatalf("step %v to %v", a, b)
		}
		if dx != 0 && dy != 0 && (g.NoPass(a[0]+dx, a[1]) || g.NoPass(a[0], a[1]+dy)) {
			t.Fatalf("step %v to %v cuts a corner", a, b)
		}
	}
	if !slicesContain(r, [2]int{10, 18}) {
		t.Errorf("the route does not go through the gap: %v", r)
	}
	// Closed off: no route.
	g.Cells[18*20+10] = 0
	if r := Route(g, 2, 2, 17, 2); r != nil {
		t.Errorf("a route through a wall: %v", r)
	}
	if r := Route(g, 2, 2, 10, 5); r != nil {
		t.Errorf("a route to a wall: %v", r)
	}
}

func slicesContain(r [][2]int, c [2]int) bool {
	for _, x := range r {
		if x == c {
			return true
		}
	}
	return false
}

// TestTravelPick walks a bot leg by leg along its route to a portal, as
// Play does, and checks that it picks a portal other than the one it
// stands by and gets there.
func TestTravelPick(t *testing.T) {
	g := walled()
	beh := Behaviour{
		Travel:  true,
		Grids:   func(string) *path.Grid { return g },
		Portals: func(string) [][2]int { return [][2]int{{1, 1}, {18, 1}} },
	}
	rnd := rand.New(rand.NewPCG(1, 2))
	var trip travel
	v := View{Map: "gen:field:0:0", X: 2, Y: 2}
	for range 20 {
		x, y, ok := beh.travelPick(&trip, v, rnd)
		if !ok {
			break
		}
		if steps, found := path.Search(g, v.X, v.Y, x, y, false); !found || len(steps) == 0 || len(steps) > path.MaxWalkPath {
			t.Fatalf("leg from %d,%d to %d,%d has no path", v.X, v.Y, x, y)
		}
		v.X, v.Y = x, y
	}
	if v.X != 18 || v.Y != 1 {
		t.Errorf("the bot ended at %d,%d, want the far portal 18,1", v.X, v.Y)
	}
	// Without a grid, the bot walks as without Travel.
	beh.Grids = func(string) *path.Grid { return nil }
	if _, _, ok := beh.travelPick(&trip, v, rnd); ok {
		t.Error("a pick without a grid")
	}
}
