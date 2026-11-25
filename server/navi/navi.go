// Package navi finds routes for the client's navigation window
// (proposal 0006, section 5; decision 0079). The graph has one node per
// map. Its edges are the warps that players walk through, and the extra
// edges of data/navi.yml: airship, Kafra and scroll trips. A route is the
// list of cells to leave a map at, found by breadth first search, so it
// has the fewest map changes.
package navi

import (
	"errors"
	"fmt"
	"strings"
)

// Service is a kind of edge.
type Service uint8

const (
	// Warp is a warp NPC, which every route may use.
	Warp Service = iota
	// Airship is a trip by airship.
	Airship
	// Kafra is a Kafra teleport.
	Kafra
	// Scroll is a warp scroll or teleport item.
	Scroll
)

// String is the name data/navi.yml gives a service.
func (s Service) String() string {
	switch s {
	case Airship:
		return "airship"
	case Kafra:
		return "kafra"
	case Scroll:
		return "scroll"
	}
	return "warp"
}

// Services is the set of services a request allows. Warps are always
// allowed.
type Services uint8

const (
	AllowAirship Services = 1 << iota
	AllowKafra
	AllowScroll
)

// FromFlag reads rAthena's NAV_* value, which keeps one service in each
// decimal digit: ones for the airship (1 to 9), tens for scrolls (10 to
// 99) and hundreds for Kafra (100 to 255).
func FromFlag(flag uint32) Services {
	var s Services
	if flag%10 != 0 {
		s |= AllowAirship
	}
	if flag/10%10 != 0 {
		s |= AllowScroll
	}
	if flag/100 != 0 {
		s |= AllowKafra
	}
	return s
}

// allows reports whether a request may use an edge of a service.
func (s Services) allows(k Service) bool {
	switch k {
	case Airship:
		return s&AllowAirship != 0
	case Kafra:
		return s&AllowKafra != 0
	case Scroll:
		return s&AllowScroll != 0
	}
	return true
}

// Edge is a way from one map to another: walk to (X, Y) of From and
// take it, and arrive at (ToX, ToY) of To. Maps are asset names, such
// as map/prontera.
type Edge struct {
	From, To string
	X, Y     int
	ToX, ToY int
	Service  Service
}

// Step is one leg of a route: leave Map at (X, Y).
type Step struct {
	Map  string
	X, Y int
}

// Graph is the maps and their edges. Build it with Add, then ask for
// routes. A graph that nothing adds to is safe for concurrent use.
type Graph struct {
	out map[string][]Edge
}

// New makes an empty graph.
func New() *Graph { return &Graph{out: map[string][]Edge{}} }

// Add adds an edge. Edges of one map are tried in the order they were
// added, so the first of two parallel edges wins. An edge that stays on
// its map is left out.
func (g *Graph) Add(e Edge) {
	if e.From == e.To {
		return
	}
	g.out[e.From] = append(g.out[e.From], e)
}

// From lists the edges that leave a map, in the order they were added.
func (g *Graph) From(m string) []Edge { return g.out[m] }

// Len is the number of edges.
func (g *Graph) Len() int {
	n := 0
	for _, es := range g.out {
		n += len(es)
	}
	return n
}

// Route finds the shortest route from one map to another, with the
// services the request allows. ok is false when there is none. A route
// from a map to itself is empty.
func (g *Graph) Route(from, to string, allow Services) (steps []Step, ok bool) {
	if from == to {
		return nil, true
	}
	// prev holds the edge that first reached each map.
	prev := map[string]Edge{}
	seen := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		for _, e := range g.out[at] {
			if seen[e.To] || !allow.allows(e.Service) {
				continue
			}
			seen[e.To] = true
			prev[e.To] = e
			if e.To == to {
				return unwind(prev, from, to), true
			}
			queue = append(queue, e.To)
		}
	}
	return nil, false
}

// unwind turns the edges that reached to into steps, first leg first.
func unwind(prev map[string]Edge, from, to string) []Step {
	var rev []Step
	for at := to; at != from; {
		e := prev[at]
		rev = append(rev, Step{Map: e.From, X: e.X, Y: e.Y})
		at = e.From
	}
	steps := make([]Step, len(rev))
	for i, s := range rev {
		steps[len(rev)-1-i] = s
	}
	return steps
}

// Asset turns a map name such as prontera into its asset name,
// map/prontera.
func Asset(name string) string {
	return "map/" + strings.TrimPrefix(strings.ToLower(name), "map/")
}

// FromRecord builds an edge from a record of data/navi.yml. Its fields:
// from, to (maps), x, y (the cell to leave from), to_x, to_y (the cell
// to arrive at), and service: airship, kafra or scroll.
func FromRecord(fields map[string]any) (Edge, error) {
	var e Edge
	for k := range fields {
		switch k {
		case "from", "to", "x", "y", "to_x", "to_y", "service":
		default:
			return e, fmt.Errorf("%q is not a field; the fields are from, to, x, y, to_x, to_y and service", k)
		}
	}
	text := func(k string) string { s, _ := fields[k].(string); return s }
	cell := func(k string) (int, error) {
		n, ok := fields[k].(int)
		if !ok || n < 0 || n >= 1<<16 {
			return 0, fmt.Errorf("%s is a cell coordinate, 0 to 65535", k)
		}
		return n, nil
	}
	if text("from") == "" || text("to") == "" {
		return e, errors.New("from and to are maps")
	}
	e.From, e.To = Asset(text("from")), Asset(text("to"))
	if e.From == e.To {
		return e, errors.New("from and to are the same map")
	}
	for _, c := range []struct {
		k string
		p *int
	}{{"x", &e.X}, {"y", &e.Y}, {"to_x", &e.ToX}, {"to_y", &e.ToY}} {
		n, err := cell(c.k)
		if err != nil {
			return e, err
		}
		*c.p = n
	}
	switch text("service") {
	case "airship":
		e.Service = Airship
	case "kafra":
		e.Service = Kafra
	case "scroll":
		e.Service = Scroll
	default:
		return e, errors.New(`service is "airship", "kafra" or "scroll"`)
	}
	return e, nil
}
