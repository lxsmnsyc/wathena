package navi

import (
	"slices"
	"testing"
)

func TestRoute(t *testing.T) {
	g := New()
	g.Add(Edge{From: "map/a", To: "map/b", X: 1, Y: 2, ToX: 3, ToY: 4})
	g.Add(Edge{From: "map/b", To: "map/c", X: 5, Y: 6})
	g.Add(Edge{From: "map/a", To: "map/c", X: 9, Y: 9, Service: Airship})
	g.Add(Edge{From: "map/c", To: "map/d", X: 7, Y: 7, Service: Kafra})
	g.Add(Edge{From: "map/a", To: "map/a"})
	if g.Len() != 4 {
		t.Fatalf("%d edges, want 4: an edge on its own map is left out", g.Len())
	}

	steps, ok := g.Route("map/a", "map/c", 0)
	want := []Step{{"map/a", 1, 2}, {"map/b", 5, 6}}
	if !ok || !slices.Equal(steps, want) {
		t.Errorf("walk only: %v %v, want %v", steps, ok, want)
	}
	steps, ok = g.Route("map/a", "map/c", AllowAirship)
	if !ok || !slices.Equal(steps, []Step{{"map/a", 9, 9}}) {
		t.Errorf("airship: %v %v, want one leg", steps, ok)
	}
	if _, ok := g.Route("map/a", "map/d", AllowAirship); ok {
		t.Error("a Kafra edge was used without Kafra")
	}
	steps, ok = g.Route("map/a", "map/d", AllowAirship|AllowKafra)
	if !ok || len(steps) != 2 || steps[1] != (Step{"map/c", 7, 7}) {
		t.Errorf("airship and Kafra: %v %v", steps, ok)
	}
	if steps, ok := g.Route("map/a", "map/a", 0); !ok || len(steps) != 0 {
		t.Errorf("a route to the same map: %v %v, want empty", steps, ok)
	}
	if _, ok := g.Route("map/d", "map/a", AllowAirship|AllowKafra|AllowScroll); ok {
		t.Error("edges are one way")
	}
	if _, ok := g.Route("map/a", "map/nowhere", 0); ok {
		t.Error("a route to a map no edge reaches")
	}
}

func TestFromFlag(t *testing.T) {
	for _, c := range []struct {
		flag uint32
		want Services
	}{
		{0, 0},
		{1, AllowAirship},
		{10, AllowScroll},
		{11, AllowAirship | AllowScroll},
		{100, AllowKafra},
		{101, AllowKafra | AllowAirship},
		{110, AllowKafra | AllowScroll},
		{111, AllowKafra | AllowScroll | AllowAirship},
		{255, AllowKafra | AllowScroll | AllowAirship},
	} {
		if got := FromFlag(c.flag); got != c.want {
			t.Errorf("FromFlag(%d) = %b, want %b", c.flag, got, c.want)
		}
	}
}

func TestFromRecord(t *testing.T) {
	e, err := FromRecord(map[string]any{"from": "prontera", "to": "map/izlude", "x": 10, "y": 20, "to_x": 3, "to_y": 4, "service": "airship"})
	want := Edge{From: "map/prontera", To: "map/izlude", X: 10, Y: 20, ToX: 3, ToY: 4, Service: Airship}
	if err != nil || e != want {
		t.Errorf("%+v %v, want %+v", e, err, want)
	}
	for _, bad := range []map[string]any{
		{"from": "a", "to": "a", "x": 1, "y": 1, "to_x": 1, "to_y": 1, "service": "kafra"},
		{"from": "a", "to": "b", "x": 1, "y": 1, "to_x": 1, "to_y": 1, "service": "boat"},
		{"from": "a", "to": "b", "x": -1, "y": 1, "to_x": 1, "to_y": 1, "service": "kafra"},
		{"from": "a", "to": "b", "x": 1, "y": 1, "to_x": 1, "service": "kafra"},
		{"from": "a", "x": 1, "y": 1, "to_x": 1, "to_y": 1, "service": "kafra"},
		{"from": "a", "to": "b", "x": 1, "y": 1, "to_x": 1, "to_y": 1, "service": "kafra", "cost": 1},
	} {
		if _, err := FromRecord(bad); err == nil {
			t.Errorf("%v: no error", bad)
		}
	}
}
