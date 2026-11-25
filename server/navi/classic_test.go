package navi_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/navi"
	"github.com/lxsmnsyc/wathena/server/world"
)

// classicGraph builds the graph of rAthena's enabled warps, as a zone
// does: a warp that stays on its map is no edge, and maps go in order.
func classicGraph(t *testing.T, mode reference.Mode) *navi.Graph {
	t.Helper()
	byMap, err := world.LoadWarps(mode)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(byMap))
	for name := range byMap {
		names = append(names, name)
	}
	slices.Sort(names)
	g := navi.New()
	for _, name := range names {
		for _, w := range byMap[name] {
			if w.To == name {
				continue
			}
			g.Add(navi.Edge{From: name, To: w.To, X: w.X, Y: w.Y, ToX: w.ToX, ToY: w.ToY})
		}
	}
	return g
}

// TestClassicRoutes routes over the warps of both modes.
func TestClassicRoutes(t *testing.T) {
	for _, mode := range []reference.Mode{reference.Renewal, reference.PreRenewal} {
		g := classicGraph(t, mode)
		if g.Len() < 400 {
			t.Fatalf("%s: %d edges", mode, g.Len())
		}
		// Prontera to Yuno (Juno), a far town: the route has legs
		// and each leg leaves the map the last one arrived on.
		steps, ok := g.Route("map/prontera", "map/yuno", 0)
		if !ok || len(steps) < 2 || steps[0].Map != "map/prontera" {
			t.Fatalf("%s: prontera to yuno: %v %v", mode, steps, ok)
		}
		if steps[0].X == 0 && steps[0].Y == 0 {
			t.Errorf("%s: the first leg has no cell: %+v", mode, steps[0])
		}
		// Every leg's map is the one the previous leg's warp leads to,
		// and the route is the shortest: no shorter way exists.
		for i := 1; i < len(steps); i++ {
			prev := steps[i-1]
			ok := false
			for _, to := range g.From(prev.Map) {
				if to.X == prev.X && to.Y == prev.Y && to.To == steps[i].Map {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s: leg %d leaves %s but leg %d does not lead there", mode, i, steps[i].Map, i-1)
			}
		}
		// A town one warp away is one leg.
		if s, ok := g.Route("map/prontera", "map/prt_fild08", 0); !ok || len(s) != 1 {
			t.Errorf("%s: prontera to prt_fild08: %v %v", mode, s, ok)
		}
		// A map no warp leads to has no route.
		if s, ok := g.Route("map/prontera", "map/no_such_map", 0); ok {
			t.Errorf("%s: a route to a map that does not exist: %v", mode, s)
		}
		if !strings.HasPrefix(steps[0].Map, "map/") {
			t.Errorf("%s: %q is not an asset name", mode, steps[0].Map)
		}
	}
}
