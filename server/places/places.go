// Package places holds the named places of a procedural world's
// generated maps, for wa.places and for NPC records placed by tag
// (proposal 0002, review decision 42, decision 0040):
//
//   - Index lists every place of every generated map the server runs, in
//     a fixed order: by map ID, then in the order the generator made
//     them.
//   - Find returns the places of a scope with a tag.
//   - Nearest returns the place with a tag closest to a cell: the fewest
//     warps away on the map graph, then the shortest distance.
//
// Places are public layout: the generator makes them from public nodes.
// A classic world has no index, and every lookup finds nothing.
package places

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/shared/gen"
)

// Place is a named place of a generated map.
type Place struct {
	// Map is the map's ID, such as gen:town:t7.
	Map string
	X   int
	Y   int
	// Tags are the place's tags, its main tag first.
	Tags []string
}

// Area returns the place's area: its map ID without gen:, such as
// town:t7, dungeon:crypt:2 or field:3:-1.
func (p Place) Area() string { return gen.Area(p.Map) }

// HasTag reports whether the place has a tag.
func (p Place) HasTag(tag string) bool { return slices.Contains(p.Tags, tag) }

// Map is a generated map with its places, as the generator made them.
type Map struct {
	ID     string
	Places []gen.Place
}

// Link is one way between maps, a warp: from a cell of a map to an
// arrival cell of another.
type Link struct {
	From     string
	X, Y     int
	To       string
	ToX, ToY int
}

// Index is every place of a world's generated maps, and the warps
// between the maps.
type Index struct {
	places []Place
	// links are by From, then in the order given.
	links []Link
}

// New makes an index. maps may come in any order: the index sorts them
// by ID and keeps each map's places in the order given.
func New(maps []Map, links []Link) *Index {
	ms := slices.Clone(maps)
	slices.SortStableFunc(ms, func(a, b Map) int { return strings.Compare(a.ID, b.ID) })
	x := &Index{}
	for _, m := range ms {
		for _, p := range m.Places {
			x.places = append(x.places, Place{Map: m.ID, X: p.X, Y: p.Y, Tags: slices.Clone(p.Tags)})
		}
	}
	x.links = slices.Clone(links)
	slices.SortStableFunc(x.links, func(a, b Link) int { return strings.Compare(a.From, b.From) })
	return x
}

// Len is the number of places.
func (x *Index) Len() int {
	if x == nil {
		return 0
	}
	return len(x.places)
}

// Place returns place i, from 0 to Len() - 1.
func (x *Index) Place(i int) Place { return x.places[i] }

// Scope is where Find looks: "town:any", "town:<id>", "dungeon:any",
// "dungeon:<id>" or "map:<id>".
type Scope struct {
	kind, id string
}

// String returns the scope as written.
func (s Scope) String() string { return s.kind + ":" + s.id }

// ErrScope is the error for a scope that is not one of the five forms.
var ErrScope = errors.New(`a scope is "town:any", "town:<id>", "dungeon:any", "dungeon:<id>" or "map:<id>"`)

// ParseScope reads a scope.
func ParseScope(s string) (Scope, error) {
	kind, id, ok := strings.Cut(s, ":")
	if !ok || id == "" || len(s) > 160 {
		return Scope{}, fmt.Errorf("%q: %w", s, ErrScope)
	}
	switch kind {
	case "town":
		if id != "any" {
			if _, ok := gen.ParseTownMapID("gen:town:" + id); !ok {
				return Scope{}, fmt.Errorf("%q: a town's ID is t and a number, such as t7: %w", s, ErrScope)
			}
		}
	case "dungeon":
		if id != "any" && !gen.ValidDungeonID(id) {
			return Scope{}, fmt.Errorf("%q: a dungeon's ID is 1 to 32 lowercase letters, digits, _ and -: %w", s, ErrScope)
		}
	case "map":
	default:
		return Scope{}, fmt.Errorf("%q: %w", s, ErrScope)
	}
	return Scope{kind: kind, id: id}, nil
}

// Matches reports whether a map is in the scope.
func (s Scope) Matches(mapID string) bool {
	switch s.kind {
	case "town":
		_, ok := gen.ParseTownMapID(mapID)
		return ok && (s.id == "any" || mapID == "gen:town:"+s.id)
	case "dungeon":
		id, _, ok := gen.ParseDungeonMapID(mapID)
		return ok && (s.id == "any" || id == s.id)
	case "map":
		return mapID == s.id
	}
	return false
}

// Find returns the indexes of the places in the scope with the tag, in
// the index's order.
func (x *Index) Find(s Scope, tag string) []int {
	if x == nil {
		return nil
	}
	var out []int
	for i, p := range x.places {
		if s.Matches(p.Map) && p.HasTag(tag) {
			out = append(out, i)
		}
	}
	return out
}

// Nearest returns the index of the place with the tag closest to cell
// (px, py) of a map: the fewest warps away on the map graph, then the
// shortest straight distance. On another map, the distance is from the
// arrival cell of the first warp that reaches that map on the shortest
// way. Ties go to the earlier place. Maps that no way reaches are left
// out. ok is false when no place qualifies.
func (x *Index) Nearest(mapID string, px, py int, tag string) (int, bool) {
	if x == nil {
		return 0, false
	}
	type reach struct {
		hops, x, y int
	}
	// A breadth-first search over the maps, from the player's map. The
	// maps are few, so a list does.
	var seen []string
	var at []reach
	find := func(id string) int { return slices.Index(seen, id) }
	seen, at = append(seen, mapID), append(at, reach{0, px, py})
	for q := 0; q < len(seen); q++ {
		from := seen[q]
		lo, found := slices.BinarySearchFunc(x.links, from, func(l Link, id string) int { return strings.Compare(l.From, id) })
		if !found {
			continue
		}
		for _, l := range x.links[lo:] {
			if l.From != from {
				break
			}
			if find(l.To) < 0 {
				seen = append(seen, l.To)
				at = append(at, reach{at[q].hops + 1, l.ToX, l.ToY})
			}
		}
	}
	best, bestKey := -1, [2]int{}
	for i, p := range x.places {
		k := find(p.Map)
		if k < 0 || !p.HasTag(tag) {
			continue
		}
		dx, dy := p.X-at[k].x, p.Y-at[k].y
		key := [2]int{at[k].hops, dx*dx + dy*dy}
		if best < 0 || cmp.Or(cmp.Compare(key[0], bestKey[0]), cmp.Compare(key[1], bestKey[1])) < 0 {
			best, bestKey = i, key
		}
	}
	return best, best >= 0
}
