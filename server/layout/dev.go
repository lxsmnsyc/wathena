package layout

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/shared/gen"
)

// The layout of a development world (decision 0041): `waserver --dev`
// with --fields, --towns and --dungeons runs the maps it lists, joined as
// phase 7 step 3's dev links joined them. The links become a Layout, so
// the server runs a dev world with the same code as a stored layout,
// with names and bands.

// DevField is a field of a dev world. Biome is its biome's index, from
// the world's climate or the --fields list.
type DevField struct {
	X, Y  int64
	Biome int
}

// DevTown is a town of a dev world: its number and its template.
type DevTown struct {
	N      int64
	Record TownRecord
}

// DevDungeon is a dungeon of a dev world: its ID, its style and its
// floors.
type DevDungeon struct {
	ID     string
	Style  StyleRecord
	Floors int
}

// DevSpec is a dev world's maps.
type DevSpec struct {
	Fields    []DevField
	Towns     []DevTown
	Dungeons  []DevDungeon
	FieldSize int
	// BandWidth, BandStep and MaxLevel are as Config's. 0 takes the
	// defaults.
	BandWidth, BandStep, MaxLevel int
}

// Dev makes a dev world's layout:
//
//   - Touching fields link through the portals on their shared border.
//   - The portals of fields that touch no other field are free ends, by
//     field ID and edge.
//   - Each town, by number, links its first portal to the first free
//     end. Its other portals become the first free ends, so dungeons
//     open from towns first. Its seat is the field it links to.
//   - Each dungeon, by ID, links its first floor's entrance to the first
//     free end, and each floor's exit to the next floor's entrance.
//
// The home area is the first field by map ID, where a dev world started
// before the layout, so the dev checks keep their walks. Maps left without
// a free end are listed in unlinked. Town names follow in.Naming when it
// is set; without it, a town is named after its ID, such as T1.
func Dev(spec DevSpec, in Inputs) (l *Layout, unlinked []string, err error) {
	l = &Layout{}
	bc := BandConfig{Width: cmp.Or(spec.BandWidth, DefaultBandWidth), Step: cmp.Or(spec.BandStep, DefaultBandStep),
		MaxLevel: cmp.Or(spec.MaxLevel, DefaultMaxLevel)}
	fs := slices.Clone(spec.Fields)
	slices.SortFunc(fs, func(a, b DevField) int { return strings.Compare(gen.FieldID(a.X, a.Y), gen.FieldID(b.X, b.Y)) })
	has := func(x, y int64) bool {
		return slices.ContainsFunc(fs, func(f DevField) bool { return f.X == x && f.Y == y })
	}
	byXY := func(x, y int64) *DevField {
		for i := range fs {
			if fs[i].X == x && fs[i].Y == y {
				return &fs[i]
			}
		}
		return nil
	}
	for _, f := range spec.Fields {
		l.Maps = append(l.Maps, Map{ID: gen.FieldID(f.X, f.Y), Kind: gen.KindField, X: f.X, Y: f.Y, Size: spec.FieldSize, Biome: f.Biome})
	}
	var free []End
	for _, f := range fs {
		for e := range 4 {
			nx, ny := f.X+edgeDX[e], f.Y+edgeDY[e]
			if !has(nx, ny) {
				free = append(free, End{gen.FieldID(f.X, f.Y), FieldPortal(e), 0})
				continue
			}
			// Each touching pair links once, from the lower field ID.
			if gen.FieldID(f.X, f.Y) < gen.FieldID(nx, ny) {
				l.Links = append(l.Links, Link{End{gen.FieldID(f.X, f.Y), FieldPortal(e), 0}, End{gen.FieldID(nx, ny), FieldPortal((e + 2) % 4), 0}})
			}
		}
	}
	seatOf := func(e End) (int64, int64, int) {
		if x, y, ok := gen.ParseFieldID(e.Map); ok {
			if f := byXY(x, y); f != nil {
				return x, y, f.Biome
			}
		}
		if m := l.Map(e.Map); m != nil {
			return m.X, m.Y, m.Biome
		}
		return 0, 0, -1
	}
	towns := slices.Clone(spec.Towns)
	slices.SortFunc(towns, func(a, b DevTown) int { return cmp.Compare(a.N, b.N) })
	for _, t := range towns {
		params, err := gen.EncodeTown(gen.TownParams{Template: t.Record.Template})
		if err != nil {
			return nil, nil, fmt.Errorf("town %s (%s): %w", gen.TownID(t.N), t.Record.Name, err)
		}
		id := gen.TownMapID(t.N)
		m := Map{ID: id, Kind: gen.KindTown, Biome: -1, Record: t.Record.Name, Params: params, Town: t.N}
		gates := t.Record.Template.Gates
		if len(free) == 0 || gates == 0 {
			l.Maps = append(l.Maps, m)
			unlinked = append(unlinked, id)
			continue
		}
		m.X, m.Y, m.Biome = seatOf(free[0])
		l.Maps = append(l.Maps, m)
		l.Links = append(l.Links, Link{free[0], End{id, TagPortal, 0}})
		var rest []End
		for k := 1; k < gates; k++ {
			rest = append(rest, End{id, TagPortal, k})
		}
		free = append(rest, free[1:]...)
	}
	ds := slices.Clone(spec.Dungeons)
	slices.SortFunc(ds, func(a, b DevDungeon) int { return strings.Compare(a.ID, b.ID) })
	for _, d := range ds {
		if !gen.ValidDungeonID(d.ID) {
			return nil, nil, fmt.Errorf("dungeon %q: an ID is 1 to 32 lowercase letters, digits, _ and -", d.ID)
		}
		var x, y int64
		linked := len(free) > 0
		if linked {
			x, y, _ = seatOf(free[0])
		}
		for floor := 1; floor <= d.Floors; floor++ {
			params, err := gen.EncodeDungeon(gen.DungeonParams{Style: d.Style.Style, Floor: floor, Floors: d.Floors})
			if err != nil {
				return nil, nil, fmt.Errorf("dungeon %s (%s): %w", d.ID, d.Style.Name, err)
			}
			mid := gen.DungeonMapID(d.ID, floor)
			l.Maps = append(l.Maps, Map{ID: mid, Kind: gen.KindDungeon, X: x, Y: y, Biome: -1, Record: d.Style.Name,
				Params: params, Dungeon: d.ID, Floor: floor, Floors: d.Floors})
			switch {
			case floor > 1:
				l.Links = append(l.Links, Link{End{gen.DungeonMapID(d.ID, floor-1), TagExit, 0}, End{mid, TagEntrance, 0}})
			case linked:
				l.Links = append(l.Links, Link{free[0], End{mid, TagEntrance, 0}})
				free = free[1:]
			default:
				unlinked = append(unlinked, mid)
			}
		}
	}
	order(l)
	l.Home = HomeOf(l)
	if len(fs) > 0 {
		l.Home = gen.FieldID(fs[0].X, fs[0].Y)
	}
	Bands(l, bc, nil)
	if in.Naming != nil {
		if err := nameTowns(l, in); err != nil {
			return nil, nil, err
		}
	} else {
		for i := range l.Maps {
			if l.Maps[i].Kind == gen.KindTown {
				l.Maps[i].Name = strings.ToUpper(gen.TownID(l.Maps[i].Town))
			}
		}
	}
	DeriveNames(l)
	for i := range l.Maps {
		if l.Maps[i].Name == "" {
			l.Maps[i].Name = l.Maps[i].ID
		}
	}
	return l, unlinked, nil
}

// kindRank orders the kinds of a layout's maps.
func kindRank(kind string) int {
	switch kind {
	case gen.KindField:
		return 0
	case gen.KindTown:
		return 1
	case gen.KindDungeon:
		return 2
	}
	return 3
}

// order sorts a layout's maps: fields by row then column, towns by
// number, dungeon floors in the order they were added, then the home
// map.
func order(l *Layout) {
	slices.SortStableFunc(l.Maps, func(a, b Map) int {
		if c := cmp.Compare(kindRank(a.Kind), kindRank(b.Kind)); c != 0 {
			return c
		}
		switch a.Kind {
		case gen.KindField:
			return cmp.Or(cmp.Compare(a.Y, b.Y), cmp.Compare(a.X, b.X))
		case gen.KindTown:
			return cmp.Compare(a.Town, b.Town)
		}
		return 0
	})
}
