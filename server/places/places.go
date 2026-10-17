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
	"strconv"
	"strings"
	"sync"
	"time"

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
// between the maps. Update changes its maps while the server runs
// (decision 0045): a place keeps its index, so handles stay, and the
// places of a removed or replaced map are gone from Find and Nearest.
// Its methods are safe for concurrent use.
type Index struct {
	mu     sync.RWMutex
	places []Place
	// ord is each place's place among its map's places, and gone marks
	// the places of maps that no longer run.
	ord  []int
	gone []bool
	// links are by From, then in the order given.
	links []Link
}

// New makes an index. maps may come in any order: the index sorts them
// by ID and keeps each map's places in the order given.
func New(maps []Map, links []Link) *Index {
	ms := slices.Clone(maps)
	slices.SortStableFunc(ms, func(a, b Map) int { return strings.Compare(a.ID, b.ID) })
	x := &Index{}
	x.add(ms)
	x.setLinks(links)
	return x
}

func (x *Index) add(ms []Map) {
	for _, m := range ms {
		for k, p := range m.Places {
			x.places = append(x.places, Place{Map: m.ID, X: p.X, Y: p.Y, Tags: slices.Clone(p.Tags)})
			x.ord, x.gone = append(x.ord, k), append(x.gone, false)
		}
	}
}

func (x *Index) setLinks(links []Link) {
	x.links = slices.Clone(links)
	slices.SortStableFunc(x.links, func(a, b Link) int { return strings.Compare(a.From, b.From) })
}

// Update removes the places of the maps of remove and of add, then adds
// add's places after the others, and replaces the links (decision
// 0045). Places keep their indexes.
func (x *Index) Update(remove []string, add []Map, links []Link) {
	x.mu.Lock()
	defer x.mu.Unlock()
	for i, p := range x.places {
		if slices.Contains(remove, p.Map) || slices.ContainsFunc(add, func(m Map) bool { return m.ID == p.Map }) {
			x.gone[i] = true
		}
	}
	ms := slices.Clone(add)
	slices.SortStableFunc(ms, func(a, b Map) int { return strings.Compare(a.ID, b.ID) })
	x.add(ms)
	x.setLinks(links)
}

// Len is the number of places, gone ones too.
func (x *Index) Len() int {
	if x == nil {
		return 0
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.places)
}

// Place returns place i, from 0 to Len() - 1. A gone place still reads
// as it was.
func (x *Index) Place(i int) Place {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.places[i]
}

// Gone reports whether place i's map no longer runs.
func (x *Index) Gone(i int) bool {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.gone[i]
}

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
		id, ok := dungeonOf(mapID)
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
	x.mu.RLock()
	defer x.mu.RUnlock()
	var out []int
	for i, p := range x.places {
		if !x.gone[i] && s.Matches(p.Map) && p.HasTag(tag) {
			out = append(out, i)
		}
	}
	// Places added later still come by map ID.
	slices.SortStableFunc(out, func(a, b int) int {
		return cmp.Or(strings.Compare(x.places[a].Map, x.places[b].Map), cmp.Compare(x.ord[a], x.ord[b]))
	})
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
	x.mu.RLock()
	defer x.mu.RUnlock()
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
		if k < 0 || x.gone[i] || !p.HasTag(tag) {
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

// dungeonOf returns the dungeon ID of a floor's map ID: gen:dungeon:<id>:<floor>,
// or a rotating floor's gen:dungeon:<id>:<date>:<floor>, whose map ID
// carries its date (decision 0045).
func dungeonOf(mapID string) (string, bool) {
	if id, _, ok := gen.ParseDungeonMapID(mapID); ok {
		return id, true
	}
	rest, ok := strings.CutPrefix(mapID, "gen:dungeon:")
	if !ok {
		return "", false
	}
	parts := strings.Split(rest, ":")
	if len(parts) != 3 || !gen.ValidDungeonID(parts[0]) {
		return "", false
	}
	if _, err := time.Parse("2006-01-02", parts[1]); err != nil {
		return "", false
	}
	if f, err := strconv.Atoi(parts[2]); err != nil || f < 1 {
		return "", false
	}
	return parts[0], true
}

// Clone returns a copy of the index, which Update changes alone, such as
// a test world's.
func (x *Index) Clone() *Index {
	if x == nil {
		return nil
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	return &Index{places: slices.Clone(x.places), ord: slices.Clone(x.ord), gone: slices.Clone(x.gone), links: slices.Clone(x.links)}
}
