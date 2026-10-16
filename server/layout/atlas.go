package layout

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/shared/gen"
)

// Atlas is a world's layout while the server runs (decision 0041): the
// maps with their display names and bands, the towns, and the distances
// of the map graph. Every zone shares one. Display names change with
// Rename; the rest never changes while the server runs. Its methods are
// safe for concurrent use.
type Atlas struct {
	mu   sync.RWMutex
	l    *Layout
	at   map[string]int
	adj  [][]int
	town []int
	// fromTown holds each town's distance to every map, by the town's
	// place in town.
	fromTown [][]int
	opts     AtlasOptions
}

// AtlasOptions are what an atlas needs for renames.
type AtlasOptions struct {
	// Blocked reports whether a name has a word the chat filter blocks.
	Blocked func(name string) bool
	// Save writes renamed maps, as one change. nil keeps renames in
	// memory, as in a dev world.
	Save func(ctx context.Context, names []Rename) error
}

// Rename is a new display name of a map.
type Rename struct {
	Map, Name string
}

// NewAtlas makes the atlas of a layout. It keeps l and changes its
// names on renames, so the caller must not use l afterwards.
func NewAtlas(l *Layout, opts AtlasOptions) *Atlas {
	a := &Atlas{l: l, at: index(l), adj: adjacency(l), opts: opts}
	for i, m := range l.Maps {
		if m.Kind == gen.KindTown {
			a.town = append(a.town, i)
		}
	}
	slices.SortStableFunc(a.town, func(x, y int) int { return int(l.Maps[x].Town - l.Maps[y].Town) })
	for _, t := range a.town {
		a.fromTown = append(a.fromTown, distances(a.adj, t))
	}
	return a
}

// Home returns the map ID of the home area.
func (a *Atlas) Home() string {
	if a == nil {
		return ""
	}
	return a.l.Home
}

// Map returns a map by ID.
func (a *Atlas) Map(id string) (Map, bool) {
	if a == nil {
		return Map{}, false
	}
	i, ok := a.at[id]
	if !ok {
		return Map{}, false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.l.Maps[i], true
}

// Maps returns every map, in the layout's order.
func (a *Atlas) Maps() []Map {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return slices.Clone(a.l.Maps)
}

// Links returns the links.
func (a *Atlas) Links() []Link {
	if a == nil {
		return nil
	}
	return slices.Clone(a.l.Links)
}

// Name returns a map's display name.
func (a *Atlas) Name(id string) (string, bool) {
	m, ok := a.Map(id)
	return m.Name, ok
}

// Find finds a map by its ID, else by its display name in any case.
func (a *Atlas) Find(name string) (Map, bool) {
	if a == nil {
		return Map{}, false
	}
	if m, ok := a.Map(name); ok {
		return m, true
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, m := range a.l.Maps {
		if strings.EqualFold(m.Name, strings.TrimSpace(name)) {
			return m, true
		}
	}
	return Map{}, false
}

// Towns returns the towns by number.
func (a *Atlas) Towns() []Map {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]Map, len(a.town))
	for i, t := range a.town {
		out[i] = a.l.Maps[t]
	}
	return out
}

// Town finds a town by its ID, such as t3, or its map ID.
func (a *Atlas) Town(id string) (Map, bool) {
	if !strings.HasPrefix(id, "gen:") {
		id = "gen:town:" + id
	}
	m, ok := a.Map(id)
	if !ok || m.Kind != gen.KindTown {
		return Map{}, false
	}
	return m, true
}

// townIndex returns a town's place in a.town.
func (a *Atlas) townIndex(mapID string) int {
	i, ok := a.at[mapID]
	if !ok {
		return -1
	}
	return slices.Index(a.town, i)
}

// Distance returns the number of warps between two maps when one of
// them is a town, or -1 when no way joins them or neither is a town.
func (a *Atlas) Distance(from, to string) int {
	if a == nil {
		return -1
	}
	if t := a.townIndex(from); t >= 0 {
		if j, ok := a.at[to]; ok {
			return a.fromTown[t][j]
		}
	}
	if t := a.townIndex(to); t >= 0 {
		if j, ok := a.at[from]; ok {
			return a.fromTown[t][j]
		}
	}
	return -1
}

// NearestTown returns the town the fewest warps from a map, ties to the
// lower number. ok is false when no town is reached.
func (a *Atlas) NearestTown(mapID string) (Map, bool) {
	if a == nil {
		return Map{}, false
	}
	j, ok := a.at[mapID]
	if !ok {
		return Map{}, false
	}
	best := -1
	for t := range a.town {
		if d := a.fromTown[t][j]; d >= 0 && (best < 0 || d < a.fromTown[best][j]) {
			best = t
		}
	}
	if best < 0 {
		return Map{}, false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.l.Maps[a.town[best]], true
}

// Errors of renames.
var (
	ErrNotTown    = errors.New("not a town")
	ErrTownName   = fmt.Errorf("a town's name has %d to %d letters", MinNameLetters, MaxNameLetters)
	ErrNameTaken  = errors.New("another map has that name")
	ErrNameFilter = errors.New("the name has a word the server does not allow")
)

// CheckRename returns why a town cannot take a name, or nil.
func (a *Atlas) CheckRename(town, name string) error {
	if a == nil {
		return ErrNotTown
	}
	t, ok := a.Town(town)
	if !ok {
		return ErrNotTown
	}
	if !ValidTownName(name) {
		return ErrTownName
	}
	if a.opts.Blocked != nil && a.opts.Blocked(name) {
		return ErrNameFilter
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, m := range a.l.Maps {
		if m.ID != t.ID && strings.EqualFold(m.Name, name) {
			return ErrNameTaken
		}
	}
	return nil
}

// Rename gives a town a new display name, and the maps named after it
// theirs (DeriveNames). It saves the changed names first, and changes
// nothing when the save fails. It returns the changes.
func (a *Atlas) Rename(ctx context.Context, town, name string) ([]Rename, error) {
	if err := a.CheckRename(town, name); err != nil {
		return nil, err
	}
	t, _ := a.Town(town)
	a.mu.Lock()
	defer a.mu.Unlock()
	next := &Layout{Maps: slices.Clone(a.l.Maps), Links: a.l.Links, Home: a.l.Home}
	next.Maps[a.at[t.ID]].Name = name
	DeriveNames(next)
	var changes []Rename
	for i, m := range next.Maps {
		if m.Name != a.l.Maps[i].Name {
			changes = append(changes, Rename{Map: m.ID, Name: m.Name})
		}
	}
	// The town goes first, so the names stay unique after each row.
	slices.SortStableFunc(changes, func(x, y Rename) int {
		return boolInt(y.Map == t.ID) - boolInt(x.Map == t.ID)
	})
	if a.opts.Save != nil {
		if err := a.opts.Save(ctx, changes); err != nil {
			return nil, err
		}
	}
	a.l.Maps = next.Maps
	return changes, nil
}

// Node is a map of the world map a character has found.
type Node struct {
	Map
	// Visited is set for a map the character entered. Other nodes are
	// maps a warp of a visited map leads to; their Name and Levels are
	// left empty.
	Visited bool
}

// Discovered returns the part of the graph a character has found: the
// maps it visited, the maps their warps lead to, in the layout's order,
// and the links that touch a visited map, each pair once, as map ID
// pairs.
func (a *Atlas) Discovered(visited []string) (nodes []Node, links [][2]string) {
	if a == nil {
		return nil, nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	seen := make([]int, len(a.l.Maps))
	for _, v := range visited {
		if i, ok := a.at[v]; ok {
			seen[i] = 2
		}
	}
	for i, s := range seen {
		if s == 2 {
			for _, j := range a.adj[i] {
				seen[j] = max(seen[j], 1)
			}
		}
	}
	for i, s := range seen {
		switch s {
		case 2:
			nodes = append(nodes, Node{Map: a.l.Maps[i], Visited: true})
		case 1:
			m := a.l.Maps[i]
			nodes = append(nodes, Node{Map: Map{ID: m.ID, Kind: m.Kind, X: m.X, Y: m.Y, Floor: m.Floor, Biome: -1}})
		}
	}
	var pairs [][2]int
	for i := range a.adj {
		if seen[i] != 2 {
			continue
		}
		for _, j := range a.adj[i] {
			p := [2]int{min(i, j), max(i, j)}
			if !slices.Contains(pairs, p) {
				pairs = append(pairs, p)
			}
		}
	}
	slices.SortFunc(pairs, func(x, y [2]int) int {
		if x[0] != y[0] {
			return x[0] - y[0]
		}
		return x[1] - y[1]
	})
	for _, p := range pairs {
		links = append(links, [2]string{a.l.Maps[p[0]].ID, a.l.Maps[p[1]].ID})
	}
	return nodes, links
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
