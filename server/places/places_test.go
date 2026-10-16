package places

import (
	"errors"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/shared/gen"
)

// world is two towns, two floors of a crypt and a field. The field links
// to town t1, t1 to the crypt's first floor, and the floors to each
// other. Town t2 has no link.
func world() *Index {
	maps := []Map{
		{ID: "gen:town:t2", Places: []gen.Place{{Tags: []string{"square"}, X: 10, Y: 10}, {Tags: []string{"blacksmith"}, X: 5, Y: 5}}},
		{ID: "gen:field:0:0", Places: []gen.Place{{Tags: []string{"portal", "portal:south"}, X: 30, Y: 4}}},
		{ID: "gen:town:t1", Places: []gen.Place{
			{Tags: []string{"square"}, X: 48, Y: 45},
			{Tags: []string{"blacksmith"}, X: 20, Y: 70},
			{Tags: []string{"shop:tool"}, X: 60, Y: 60},
			{Tags: []string{"gate", "gate:south"}, X: 40, Y: 11},
		}},
		{ID: "gen:dungeon:crypt:2", Places: []gen.Place{{Tags: []string{"entrance"}, X: 8, Y: 8}, {Tags: []string{"blacksmith"}, X: 9, Y: 9}}},
		{ID: "gen:dungeon:crypt:1", Places: []gen.Place{{Tags: []string{"entrance"}, X: 8, Y: 8}, {Tags: []string{"exit"}, X: 90, Y: 90}}},
	}
	links := []Link{
		{From: "gen:field:0:0", X: 30, Y: 4, To: "gen:town:t1", ToX: 40, ToY: 6},
		{From: "gen:town:t1", X: 40, Y: 3, To: "gen:field:0:0", ToX: 30, ToY: 7},
		{From: "gen:town:t1", X: 90, Y: 50, To: "gen:dungeon:crypt:1", ToX: 8, ToY: 11},
		{From: "gen:dungeon:crypt:1", X: 90, Y: 90, To: "gen:dungeon:crypt:2", ToX: 8, ToY: 11},
	}
	return New(maps, links)
}

func maps(x *Index, idx []int) []string {
	var out []string
	for _, i := range idx {
		out = append(out, x.Place(i).Map)
	}
	return out
}

// TestFind: every scope form, in the index's order: by map ID, then the
// generator's order.
func TestFind(t *testing.T) {
	x := world()
	cases := []struct {
		scope, tag string
		want       []string
	}{
		{"town:any", "blacksmith", []string{"gen:town:t1", "gen:town:t2"}},
		{"town:t2", "blacksmith", []string{"gen:town:t2"}},
		{"town:t9", "blacksmith", nil},
		{"dungeon:any", "entrance", []string{"gen:dungeon:crypt:1", "gen:dungeon:crypt:2"}},
		{"dungeon:crypt", "blacksmith", []string{"gen:dungeon:crypt:2"}},
		{"dungeon:cave", "entrance", nil},
		{"map:gen:field:0:0", "portal:south", []string{"gen:field:0:0"}},
		{"map:gen:field:0:0", "gate", nil},
		{"town:any", "smithy", nil},
	}
	for _, c := range cases {
		s, err := ParseScope(c.scope)
		if err != nil {
			t.Fatalf("%s: %v", c.scope, err)
		}
		if got := maps(x, x.Find(s, c.tag)); !slices.Equal(got, c.want) {
			t.Errorf("%s %s: %v, want %v", c.scope, c.tag, got, c.want)
		}
	}
	p := x.Place(x.Find(Scope{kind: "town", id: "t1"}, "gate")[0])
	if p.Area() != "town:t1" || p.X != 40 || p.Y != 11 || !slices.Equal(p.Tags, []string{"gate", "gate:south"}) {
		t.Errorf("gate %+v in %s", p, p.Area())
	}
	if x.Len() != 11 {
		t.Errorf("%d places", x.Len())
	}
}

// TestParseScope: the five forms, and what is refused.
func TestParseScope(t *testing.T) {
	for _, s := range []string{"town:any", "town:t7", "dungeon:any", "dungeon:old-crypt", "map:gen:field:3:-1", "map:prontera"} {
		if sc, err := ParseScope(s); err != nil || sc.String() != s {
			t.Errorf("%s: %v %v", s, sc, err)
		}
	}
	for _, s := range []string{"", "town", "town:", "town:7", "town:t0", "dungeon:Crypt", "dungeon:a:b", "field:any", "map:", "town:any:x"} {
		if _, err := ParseScope(s); !errors.Is(err, ErrScope) {
			t.Errorf("%q: %v", s, err)
		}
	}
}

// TestNearest: the fewest warps first, then the distance, and maps that
// no way reaches are left out.
func TestNearest(t *testing.T) {
	x := world()
	at := func(m string, px, py int, tag string) string {
		i, ok := x.Nearest(m, px, py, tag)
		if !ok {
			return ""
		}
		p := x.Place(i)
		return p.Map + " " + p.Tags[0]
	}
	// On the field, t1 is one warp away and the crypt's second floor
	// three; t2 has no way in.
	if got := at("gen:field:0:0", 30, 30, "blacksmith"); got != "gen:town:t1 blacksmith" {
		t.Errorf("from the field: %s", got)
	}
	// On the second floor, its own blacksmith is nearest.
	if got := at("gen:dungeon:crypt:2", 50, 50, "blacksmith"); got != "gen:dungeon:crypt:2 blacksmith" {
		t.Errorf("from the crypt: %s", got)
	}
	// On t2, without links, only t2's own places count.
	if got := at("gen:town:t2", 0, 0, "entrance"); got != "" {
		t.Errorf("from t2: %s", got)
	}
	// Two entrances on floors 1 and 2: floor 1 is fewer warps away.
	if got := at("gen:town:t1", 48, 45, "entrance"); got != "gen:dungeon:crypt:1 entrance" {
		t.Errorf("from t1: %s", got)
	}
	// A map without places or links, such as an asset map, finds none.
	if got := at("prontera", 1, 1, "square"); got != "" {
		t.Errorf("from prontera: %s", got)
	}
	var none *Index
	if _, ok := none.Nearest("gen:town:t1", 0, 0, "square"); ok || none.Find(Scope{kind: "town", id: "any"}, "square") != nil || none.Len() != 0 {
		t.Error("a nil index found a place")
	}
}
