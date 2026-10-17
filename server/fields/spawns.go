package fields

import (
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// waterBlock is the side of the blocks that give a field's water points:
// the first water cell of each block, by row and column, is one.
const waterBlock = 32

// SpawnMaps returns what spawn selection reads of a layout's fields and
// dungeon floors (decision 0042), in the layout's order, from the
// generated maps and the warps of LayoutWarps:
//
//   - a field's biome, and a floor's tileset as its biome;
//   - the walkable cells, those of ground and water;
//   - the features: water and clearing on fields, room and boss_room on
//     floors, with their points: the first water cell of each 32 × 32
//     block, and each clearing's or room's place;
//   - the arrival cells of every warp to the map, and the warps on it
//     that lead to a town;
//   - whether a field is one warp from a town, and its region: the town
//     the fewest warps away, ties to the lower number.
func (s *Set) SpawnMaps(l *layout.Layout, warps map[string][]zone.Warp) ([]spawns.Map, error) {
	return SpawnMapsOf(l, warps, s.Get, nil)
}

// SpawnMapsOf is SpawnMaps for the maps that only reports, or every
// field and floor when only is nil, with get finding the generated maps
// (decision 0045).
func SpawnMapsOf(l *layout.Layout, warps map[string][]zone.Warp, get func(id string) *Field, only func(id string) bool) ([]spawns.Map, error) {
	atlas := layout.NewAtlas(l, layout.AtlasOptions{})
	kinds := map[string]string{}
	for _, m := range l.Maps {
		kinds[m.ID] = m.Kind
	}
	nextToTown := map[string]bool{}
	for _, k := range l.Links {
		if kinds[k.A.Map] == gen.KindTown {
			nextToTown[k.B.Map] = true
		}
		if kinds[k.B.Map] == gen.KindTown {
			nextToTown[k.A.Map] = true
		}
	}
	arrivals := map[string][]spawns.Point{}
	for _, id := range sortedKeys(warps) {
		for _, w := range warps[id] {
			arrivals[w.To] = append(arrivals[w.To], spawns.Point{X: w.ToX, Y: w.ToY})
		}
	}
	var out []spawns.Map
	for _, m := range l.Maps {
		if m.Kind != gen.KindField && m.Kind != gen.KindDungeon || only != nil && !only(m.ID) {
			continue
		}
		f := get(m.ID)
		if f == nil {
			return nil, fmt.Errorf("the map %s is not generated", m.ID)
		}
		sm := spawns.Map{ID: m.ID, Kind: m.Kind, Band: m.Levels, Floor: m.Floor, Floors: m.Floors,
			NextToTown: nextToTown[m.ID], Arrivals: arrivals[m.ID]}
		if m.Kind == gen.KindField {
			sm.Biome = f.BiomeName
		} else {
			p, err := gen.DecodeDungeon(m.Params)
			if err != nil {
				return nil, fmt.Errorf("the floor %s: %w", m.ID, err)
			}
			sm.Biome = p.Style.Tileset
		}
		if t, ok := atlas.NearestTown(m.ID); ok {
			sm.Region = gen.TownID(t.Town)
		}
		for _, w := range warps[m.ID] {
			if kinds[w.To] == gen.KindTown {
				sm.TownPortals = append(sm.TownPortals, spawns.Point{X: w.X, Y: w.Y})
			}
		}
		g := f.Field
		water := map[[2]int]bool{}
		for y := range g.Height {
			for x := range g.Width {
				switch g.Types[y*g.Width+x] {
				case gen.CellWalkable:
					sm.Walkable++
				case gen.CellWater:
					sm.Walkable++
					if b := [2]int{x / waterBlock, y / waterBlock}; !water[b] {
						water[b] = true
						sm.FeaturePoints = append(sm.FeaturePoints, spawns.Point{X: x, Y: y})
					}
				}
			}
		}
		if len(water) > 0 {
			sm.Features = append(sm.Features, "water")
		}
		for _, p := range g.Places {
			switch {
			case p.HasTag("clearing"), p.HasTag("room"):
				tag := "clearing"
				if p.HasTag("room") {
					tag = "room"
				}
				if !slices.Contains(sm.Features, tag) {
					sm.Features = append(sm.Features, tag)
				}
				sm.FeaturePoints = append(sm.FeaturePoints, spawns.Point{X: p.X, Y: p.Y})
			case p.HasTag("boss_room"):
				sm.Features = append(sm.Features, "boss_room")
				sm.BossRoom = &spawns.Point{X: p.X, Y: p.Y}
			}
		}
		slices.Sort(sm.Features)
		out = append(out, sm)
	}
	return out, nil
}
