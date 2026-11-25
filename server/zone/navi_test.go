package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/navi"
)

// listed is a Directory of zones that also lists them, as the world does.
type listed struct{ zones }

func (d listed) Zones() []*Zone {
	var out []*Zone
	for _, name := range []string{"map/a", "map/b", "map/c", "map/d"} {
		if z := d.zones[name]; z != nil {
			out = append(out, z)
		}
	}
	return out
}

// naviWorld makes four maps: a leads to b, b to c and back to a, and d
// has no way in. A warp of a to c is hidden.
func naviWorld() (a, b, c, d *Zone) {
	a, b, c, d = saveZone(), saveZone(), saveZone(), saveZone()
	a.Map, b.Map, c.Map, d.Map = "map/a", "map/b", "map/c", "map/d"
	dir := listed{zones{"map/a": a, "map/b": b, "map/c": c, "map/d": d}}
	for _, z := range []*Zone{a, b, c, d} {
		z.World = dir
	}
	a.SetWarps([]Warp{
		{Name: "a_c_hidden", X: 5, Y: 5, To: "map/c", ToX: 5, ToY: 5, Hidden: true},
		{Name: "a_b", X: 20, Y: 20, XS: 1, YS: 1, To: "map/b", ToX: 5, ToY: 6},
	})
	b.SetWarps([]Warp{
		{Name: "b_a", X: 4, Y: 4, To: "map/a", ToX: 20, ToY: 18},
		{Name: "b_c", X: 30, Y: 31, To: "map/c", ToX: 8, ToY: 9},
	})
	c.SetWarps([]Warp{{Name: "c_c", X: 1, Y: 1, To: "map/c", ToX: 2, ToY: 2}})
	return a, b, c, d
}

func routesOf(p *Player) []*wathenav1.NaviRoute {
	var out []*wathenav1.NaviRoute
	for _, e := range events(p) {
		if r := e.GetNaviRoute(); r != nil {
			out = append(out, r)
		}
	}
	return out
}

func stepsOf(r *wathenav1.NaviRoute) []navi.Step {
	var out []navi.Step
	for _, s := range r.GetSteps() {
		out = append(out, navi.Step{Map: s.GetMap(), X: int(s.GetX()), Y: int(s.GetY())})
	}
	return out
}

// TestNaviRoute sends the route over the warps, skips a hidden warp and
// a warp on its own map, and sends a map with no way in alone.
func TestNaviRoute(t *testing.T) {
	a, _, _, _ := naviWorld()
	p := onMap(a, "Guide", 10, 10, &saves{})
	a.stepAt(50)
	events(p)

	a.handle(command{player: p, navi: &naviCommand{target: naviTarget{name: "c", x: 100, y: 120, hideWindow: true}}})
	a.stepAt(100)
	rs := routesOf(p)
	if len(rs) != 1 {
		t.Fatalf("%d routes", len(rs))
	}
	r := rs[0]
	want := []navi.Step{{Map: "a", X: 20, Y: 20}, {Map: "b", X: 30, Y: 31}}
	if !r.GetFound() || r.GetMap() != "c" || r.GetX() != 100 || r.GetY() != 120 || !r.GetHideWindow() || !equalSteps(stepsOf(r), want) {
		t.Errorf("route %v, want steps %v", r, want)
	}

	// Another request within a second is dropped, whatever it asks.
	a.handle(command{player: p, navi: &naviCommand{target: naviTarget{name: "d"}}})
	a.stepAt(150)
	if got := routesOf(p); len(got) != 0 || p.navi.name != "c" {
		t.Errorf("a second request within the second: %v", got)
	}
	p.naviAt = 0

	// A map that does not run is ignored.
	a.handle(command{player: p, navi: &naviCommand{target: naviTarget{name: "nowhere"}}})
	a.stepAt(200)
	if got := routesOf(p); len(got) != 0 || p.navi.name != "c" {
		t.Errorf("a map that does not run: %v", got)
	}

	// A map nobody can walk to is sent alone.
	p.naviAt = 0
	a.handle(command{player: p, navi: &naviCommand{target: naviTarget{name: "d", x: 7, y: 8}}})
	a.stepAt(250)
	rs = routesOf(p)
	if len(rs) != 1 || rs[0].GetFound() || len(rs[0].GetSteps()) != 0 || rs[0].GetMap() != "d" || rs[0].GetX() != 7 {
		t.Errorf("a map without a route: %v", rs)
	}

	// A cancel ends it, once.
	a.handle(command{player: p, navi: &naviCommand{cancel: true}})
	a.handle(command{player: p, navi: &naviCommand{cancel: true}})
	a.stepAt(300)
	rs = routesOf(p)
	if len(rs) != 1 || !rs[0].GetCancel() || p.navi != nil {
		t.Errorf("a cancel: %v, target %v", rs, p.navi)
	}
}

func equalSteps(a, b []navi.Step) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestNaviFollowsMapChange makes the route again on the next map, and
// drops the target on the target's map.
func TestNaviFollowsMapChange(t *testing.T) {
	a, b, c, _ := naviWorld()
	p := onMap(a, "Walker", 20, 18, &saves{})
	if ok, err := (scriptHost{a}).Navigate(int64(p.charID), modscript.NaviTarget{Map: "c", Services: 101}); err != nil || !ok {
		t.Fatalf("navigate_to: %v %v", ok, err)
	}
	a.stepAt(50)
	if rs := routesOf(p); len(rs) != 1 || len(rs[0].GetSteps()) != 2 {
		t.Fatalf("first route %v", rs)
	}

	// One step north enters the warp's area.
	a.handle(command{player: p, walkTo: &[2]int{20, 19}})
	a.stepAt(a.now + 200)
	q := arrive(t, b)
	if q.navi == nil || q.navi.name != "c" {
		t.Fatalf("the target did not follow the player: %+v", q.navi)
	}
	b.stepAt(50)
	rs := routesOf(q)
	if len(rs) != 1 || !rs[0].GetFound() || !equalSteps(stepsOf(rs[0]), []navi.Step{{Map: "b", X: 30, Y: 31}}) {
		t.Fatalf("route on b: %v", rs)
	}

	// On the target map the route has no steps and the target is gone.
	b.handle(command{player: q, walkTo: &[2]int{30, 31}})
	for now := int64(100); b.entities.has(q.entity); now += 50 {
		b.stepAt(now)
		if now > 20000 {
			t.Fatal("the player never reached the warp")
		}
	}
	r := arrive(t, c)
	c.stepAt(50)
	rs = routesOf(r)
	if len(rs) != 1 || !rs[0].GetFound() || len(rs[0].GetSteps()) != 0 || r.navi != nil {
		t.Errorf("route on c: %v, target %v", rs, r.navi)
	}
}

// TestNaviScript checks the script call: false for a map that does not
// run, true with a route sent right away, and the extra edges of
// data/navi.yml for the services a request allows.
func TestNaviScript(t *testing.T) {
	a, _, _, d := naviWorld()
	p := onMap(a, "Scripter", 10, 10, &saves{})
	h := scriptHost{a}
	if ok, err := h.Navigate(int64(p.charID), modscript.NaviTarget{Map: "nowhere"}); err != nil || ok {
		t.Errorf("a map that does not run: %v %v", ok, err)
	}
	if _, err := h.Navigate(9999, modscript.NaviTarget{Map: "c"}); err == nil {
		t.Error("no error for a player who is not here")
	}

	a.NaviExtra = []navi.Edge{{From: "map/a", To: "map/d", X: 3, Y: 3, ToX: 4, ToY: 4, Service: navi.Airship}}
	for _, c := range []struct {
		services int
		found    bool
	}{{0, false}, {100, false}, {1, true}, {101, true}} {
		if ok, err := h.Navigate(int64(p.charID), modscript.NaviTarget{Map: "d", Services: c.services}); err != nil || !ok {
			t.Fatal(ok, err)
		}
		a.stepAt(a.now + 50)
		rs := routesOf(p)
		if len(rs) != 1 || rs[0].GetFound() != c.found {
			t.Errorf("services %d: %v, want found %v", c.services, rs, c.found)
		}
	}
	_ = d
}

// TestNaviWithoutWorld routes over the zone's own warps when the
// directory cannot list zones.
func TestNaviWithoutWorld(t *testing.T) {
	a, b := twoMaps()
	p := onMap(a, "Alone", 10, 10, &saves{})
	if ok, _ := (scriptHost{a}).Navigate(int64(p.charID), modscript.NaviTarget{Map: "b"}); !ok {
		t.Fatal("no navigation")
	}
	a.stepAt(50)
	rs := routesOf(p)
	if len(rs) != 1 || !rs[0].GetFound() || len(rs[0].GetSteps()) != 1 {
		t.Errorf("%v", rs)
	}
	_ = b
}
