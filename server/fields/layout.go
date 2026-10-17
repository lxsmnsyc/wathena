package fields

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/mapfolder"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// The maps of a world layout (decision 0041): GenerateLayout makes every
// map a layout lists, and LayoutWarps puts a warp at each end of each
// link, at the place the end names.

// GenerateLayout makes the maps of a layout, several at once: each field
// from its node with its size and biome, each town and dungeon floor
// from its node and the params the layout keeps, and the home map from
// its folder. home is the home map's folder, or nil for a layout without
// one. A map that fails is an error that names its public node.
func GenerateLayout(g gen.Generator, public rng.Node, l *layout.Layout, home *mapfolder.Map) ([]*Field, error) {
	if g.Field == nil || !g.Makes(gen.KindTown) {
		return nil, fmt.Errorf("map generator %d makes no towns; a world layout needs generator 3 or later", g.Version)
	}
	out := make([]*Field, len(l.Maps))
	errs := make([]error, len(l.Maps))
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), len(l.Maps)) {
		wg.Go(func() {
			for i := range next {
				out[i], errs[i] = generateLayoutMap(g, public, &l.Maps[i], home)
			}
		})
	}
	for i := range l.Maps {
		next <- i
	}
	close(next)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

func generateLayoutMap(g gen.Generator, public rng.Node, m *layout.Map, home *mapfolder.Map) (*Field, error) {
	var f *Field
	var err error
	switch m.Kind {
	case gen.KindField:
		if m.Biome < 0 || m.Biome >= len(g.Biomes) {
			return nil, fmt.Errorf("field %s: generator %d has no biome %d", m.ID, g.Version, m.Biome)
		}
		f, err = generate(g, public, Spec{X: m.X, Y: m.Y, Biome: g.Biomes[m.Biome]}, m.Size)
	case gen.KindTown:
		f, err = generateMap(g, mapJob{id: m.ID, kind: gen.KindTown, name: m.Record, node: gen.TownNode(public, m.Town),
			params: m.Params, town: m.Town})
	case gen.KindDungeon:
		f, err = generateMap(g, mapJob{id: m.ID, kind: gen.KindDungeon, name: m.Record,
			node: gen.FloorNode(gen.DungeonNode(public, m.Dungeon), m.Floor), params: m.Params,
			dungeon: m.Dungeon, floor: m.Floor, floors: m.Floors})
	case layout.KindHome:
		f, err = homeField(g, m, home)
	default:
		return nil, fmt.Errorf("map %s: unknown kind %q", m.ID, m.Kind)
	}
	if err != nil {
		return nil, err
	}
	f.Levels, f.DisplayName = m.Levels, m.Name
	return f, nil
}

// homeField makes the field of a home map from its folder.
func homeField(g gen.Generator, m *layout.Map, home *mapfolder.Map) (*Field, error) {
	if home == nil || layout.HomeMapID(home.ID) != m.ID {
		return nil, fmt.Errorf("the home map %s (%s) is not loaded: the mod that has it must stay installed", m.ID, m.Record)
	}
	if p := home.Check(); len(p) > 0 {
		return nil, fmt.Errorf("the home map %s: %s", m.ID, strings.Join(p, "; "))
	}
	w, err := home.WMAP()
	if err != nil {
		return nil, fmt.Errorf("the home map %s: %w", m.ID, err)
	}
	grid := home.Grid()
	f := &Field{ID: m.ID, Kind: layout.KindHome, Name: m.Record, Version: g.Version, RNG: g.RNG, Biome: -1,
		Field: gen.Field{Grid: grid, Places: home.Places, WMAP: w}}
	f.Checksum, f.WMAPSum = grid.Checksum(), sha256.Sum256(w)
	if b, ok := g.BiomeIndex(home.Biome); ok {
		f.BiomeName = g.Biomes[b]
	}
	return f, nil
}

// placeAt returns the index-th place of a map with a tag.
func placeAt(f *Field, tag string, index int) (gen.Place, bool) {
	k := 0
	for _, p := range f.Field.Places {
		if p.HasTag(tag) {
			if k == index {
				return p, true
			}
			k++
		}
	}
	return gen.Place{}, false
}

// LayoutWarps puts a warp at both ends of each link of a layout. A warp
// stands on its end's place, and covers it and the 8 cells around it. A
// player arrives on a walkable cell near the other end that no warp
// covers: 3 cells inward from a field's portal, as between touching
// fields, else 3 to 6 cells from the place. An end whose map or place
// the set lacks is an error.
func (s *Set) LayoutWarps(l *layout.Layout) (map[string][]zone.Warp, error) {
	type end struct {
		f *Field
		p gen.Place
	}
	resolve := func(e layout.End) (end, error) {
		f := s.Get(e.Map)
		if f == nil {
			return end{}, fmt.Errorf("the link end %s: the map does not run", e)
		}
		p, ok := placeAt(f, e.Tag, e.Index)
		if !ok {
			return end{}, fmt.Errorf("the link end %s: the map has no such place", e)
		}
		return end{f, p}, nil
	}
	var errs []error
	pairs := make([][2]end, 0, len(l.Links))
	for _, k := range l.Links {
		a, errA := resolve(k.A)
		b, errB := resolve(k.B)
		if errA != nil || errB != nil {
			errs = append(errs, errA, errB)
			continue
		}
		pairs = append(pairs, [2]end{a, b})
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	// Warps first, so arrivals can keep out of every warp's area.
	areas := map[string][]zone.Warp{}
	for _, pr := range pairs {
		for _, e := range pr {
			areas[e.f.ID] = append(areas[e.f.ID], zone.Warp{X: e.p.X, Y: e.p.Y, XS: 1, YS: 1})
		}
	}
	covered := func(id string, x, y int) bool {
		for _, w := range areas[id] {
			if x >= w.X-w.XS && x <= w.X+w.XS && y >= w.Y-w.YS && y <= w.Y+w.YS {
				return true
			}
		}
		return false
	}
	arrival := func(e end) (int, int) {
		if e.f.Kind == gen.KindField {
			for edge := range 4 {
				if e.p.HasTag(layout.FieldPortal(edge)) {
					dx, dy := edgeStep(edge)
					x, y := e.p.X-3*int(dx), e.p.Y-3*int(dy)
					if !covered(e.f.ID, x, y) && walkable(e.f, x, y) {
						return x, y
					}
				}
			}
		}
		return arrivalAvoiding(e.f, e.p.X, e.p.Y, func(x, y int) bool { return covered(e.f.ID, x, y) })
	}
	warps := map[string][]zone.Warp{}
	add := func(from, to end) {
		tx, ty := arrival(to)
		warps[from.f.ID] = append(warps[from.f.ID], zone.Warp{
			Name: fmt.Sprintf("gen_%s_%d", from.f.ID, len(warps[from.f.ID])),
			X:    from.p.X, Y: from.p.Y, XS: 1, YS: 1, To: to.f.ID, ToX: tx, ToY: ty,
		})
	}
	for _, pr := range pairs {
		add(pr[0], pr[1])
		add(pr[1], pr[0])
	}
	return warps, nil
}

// walkable reports whether a cell of a map is walkable ground or water.
func walkable(f *Field, x, y int) bool {
	w, h := f.Field.Width, f.Field.Height
	if x < 0 || y < 0 || x >= w || y >= h {
		return false
	}
	t := f.Field.Types[y*w+x]
	return t == gen.CellWalkable || t == gen.CellWater
}

// arrivalAvoiding returns a walkable cell 3 to 6 cells from (x, y) that
// avoid does not refuse, in a fixed order, then any walkable cell of
// that ring, then (x, y).
func arrivalAvoiding(f *Field, x, y int, avoid func(x, y int) bool) (int, int) {
	for _, strict := range []bool{true, false} {
		for r := 3; r <= 6; r++ {
			for dy := -r; dy <= r; dy++ {
				for dx := -r; dx <= r; dx++ {
					if max(abs(dx), abs(dy)) != r {
						continue
					}
					cx, cy := x+dx, y+dy
					if walkable(f, cx, cy) && (!strict || !avoid(cx, cy)) {
						return cx, cy
					}
				}
			}
		}
	}
	return x, y
}

// LayoutOf turns the town templates and dungeon styles into a layout's
// inputs.
func LayoutOf(styles []Style, templates []Template) ([]layout.TownRecord, []layout.StyleRecord) {
	towns := make([]layout.TownRecord, len(templates))
	for i, t := range templates {
		towns[i] = layout.TownRecord{Name: t.Name, Template: t.Template}
	}
	out := make([]layout.StyleRecord, len(styles))
	for i, s := range styles {
		out[i] = layout.StyleRecord{Name: s.Name, Style: s.Style}
	}
	return towns, out
}

// TemplatesOf returns the town templates of a layout's towns, from the
// params it keeps, for the tileset table's buildings.
func TemplatesOf(l *layout.Layout) []Template {
	var out []Template
	for _, m := range l.Maps {
		if m.Kind != gen.KindTown {
			continue
		}
		if p, err := gen.DecodeTown(m.Params); err == nil {
			out = append(out, Template{Name: m.Record, Template: p.Template})
		}
	}
	return out
}

// MapsOf returns the generated maps by map ID, for content (decision
// 0044).
func MapsOf(list []*Field) map[string]*gen.Field {
	out := make(map[string]*gen.Field, len(list))
	for _, f := range list {
		out[f.ID] = &f.Field
	}
	return out
}
