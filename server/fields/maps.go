package fields

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Dungeon floors and towns (decision 0040). Phase 7 step 4 places them
// on the map graph. Until then a server runs the towns and dungeons its
// config lists, and DevLinks joins them to its fields.

// TownSpec is a town to run: its number and its template.
type TownSpec struct {
	N        int64
	Template Template
}

// DungeonSpec is a dungeon to run: its ID, its style, its floors, and
// the level band of its first floor.
type DungeonSpec struct {
	ID     string
	Style  Style
	Floors int
	Levels [2]int
}

// DevLevels is the level band of a dungeon's first floor until the map
// graph gives dungeons their bands (phase 7 step 4).
var DevLevels = [2]int{1, 10}

// ParseDungeonSpec reads "style", "style:floors", or a mod's style with
// its prefix, such as "example.mod:sewer:2". floors is 0 when left out.
func ParseDungeonSpec(s string) (style string, floors int, err error) {
	s = strings.TrimSpace(s)
	style = s
	if i := strings.LastIndexByte(s, ':'); i >= 0 {
		if n, err := strconv.Atoi(s[i+1:]); err == nil {
			if n < 1 || n > 99 {
				return "", 0, fmt.Errorf("dungeon %q: floors is 1 to 99", s)
			}
			style, floors = s[:i], n
		}
	}
	if style == "" {
		return "", 0, fmt.Errorf("dungeon %q: want a style, or style:floors, such as crypt:3", s)
	}
	return style, floors, nil
}

// DungeonID is the ID a dungeon of a style gets until the map graph
// names dungeons: the style's name without its mod prefix.
func DungeonID(style string) string {
	if local := mod.Local(style); local != "" {
		return local
	}
	return style
}

// FindStyle and FindTemplate find a record by full name.
func FindStyle(styles []Style, name string) (Style, bool) {
	i := slices.IndexFunc(styles, func(s Style) bool { return s.Name == name })
	if i < 0 {
		return Style{}, false
	}
	return styles[i], true
}

// FindTemplate finds a town template by full name.
func FindTemplate(templates []Template, name string) (Template, bool) {
	i := slices.IndexFunc(templates, func(t Template) bool { return t.Name == name })
	if i < 0 {
		return Template{}, false
	}
	return templates[i], true
}

// StyleNames and TemplateNames list the records' names.
func StyleNames(styles []Style) []string {
	out := make([]string, len(styles))
	for i, s := range styles {
		out[i] = s.Name
	}
	return out
}

// TemplateNames lists the town templates' names.
func TemplateNames(templates []Template) []string {
	out := make([]string, len(templates))
	for i, t := range templates {
		out[i] = t.Name
	}
	return out
}

// mapJob is one floor or town to generate.
type mapJob struct {
	id, kind, name string
	node           rng.Node
	params         []byte
	town           int64
	dungeon        string
	floor, floors  int
	levels         [2]int
}

// GenerateMaps makes the towns and every floor of the dungeons from the
// world's public branch, several at once. A map that fails validation in
// every attempt is an error that names its public node.
func GenerateMaps(g gen.Generator, public rng.Node, towns []TownSpec, dungeons []DungeonSpec) ([]*Field, error) {
	var jobs []mapJob
	var errs []error
	for _, t := range towns {
		if !g.Makes(gen.KindTown) {
			return nil, fmt.Errorf("map generator %d makes no towns; towns need generator 3 or later", g.Version)
		}
		b, err := gen.EncodeTown(gen.TownParams{Template: t.Template.Template})
		if err != nil {
			errs = append(errs, fmt.Errorf("town %s (%s): %w", gen.TownID(t.N), t.Template.Name, err))
			continue
		}
		jobs = append(jobs, mapJob{id: gen.TownMapID(t.N), kind: gen.KindTown, name: t.Template.Name,
			node: gen.TownNode(public, t.N), params: b, town: t.N})
	}
	for _, d := range dungeons {
		if !g.Makes(gen.KindDungeon) {
			return nil, fmt.Errorf("map generator %d makes no dungeons; dungeons need generator 3 or later", g.Version)
		}
		if !gen.ValidDungeonID(d.ID) {
			errs = append(errs, fmt.Errorf("dungeon %q: an ID is 1 to 32 lowercase letters, digits, _ and -", d.ID))
			continue
		}
		dn := gen.DungeonNode(public, d.ID)
		for floor := 1; floor <= d.Floors; floor++ {
			b, err := gen.EncodeDungeon(gen.DungeonParams{Style: d.Style.Style, Floor: floor, Floors: d.Floors})
			if err != nil {
				errs = append(errs, fmt.Errorf("dungeon %s (%s): %w", d.ID, d.Style.Name, err))
				break
			}
			step := (floor - 1) * d.Style.Style.LevelStep
			jobs = append(jobs, mapJob{id: gen.DungeonMapID(d.ID, floor), kind: gen.KindDungeon, name: d.Style.Name,
				node: gen.FloorNode(dn, floor), params: b, dungeon: d.ID, floor: floor, floors: d.Floors,
				levels: [2]int{d.Levels[0] + step, d.Levels[1] + step}})
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	out := make([]*Field, len(jobs))
	errs = make([]error, len(jobs))
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), len(jobs)) {
		wg.Go(func() {
			for i := range next {
				out[i], errs[i] = generateMap(g, jobs[i])
			}
		})
	}
	for i := range jobs {
		next <- i
	}
	close(next)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, f := range out {
		if seen[f.ID] {
			return nil, fmt.Errorf("map %s is listed twice", f.ID)
		}
		seen[f.ID] = true
	}
	return out, nil
}

func generateMap(g gen.Generator, j mapJob) (*Field, error) {
	m, err := g.Map(j.node, gen.MapParams{Kind: j.kind, Params: j.params})
	if err != nil {
		key := j.node.Key()
		return nil, fmt.Errorf("%s %s (%s, node %s): %w", j.kind, j.id, j.name, hex.EncodeToString(key[:]), err)
	}
	return &Field{
		ID: j.id, Kind: j.kind, Params: j.params, Name: j.name,
		Town: j.town, Dungeon: j.dungeon, Floor: j.floor, Floors: j.floors, Levels: j.levels,
		Version: g.Version, RNG: g.RNG, Key: j.node.Key(), Field: m,
		Checksum: m.Checksum(), WMAPSum: sha256.Sum256(m.WMAP),
	}, nil
}

// placeOf returns the first place of a map with a tag.
func placeOf(f *Field, tag string) (gen.Place, bool) {
	for _, p := range f.Field.Places {
		if p.HasTag(tag) {
			return p, true
		}
	}
	return gen.Place{}, false
}

// arrivalNear returns a walkable cell 3 to 6 cells from (x, y), in a
// fixed order, so a player who arrives does not stand on the warp.
func arrivalNear(f *Field, x, y int) (int, int) {
	w, h := f.Field.Width, f.Field.Height
	for r := 3; r <= 6; r++ {
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if max(abs(dx), abs(dy)) != r {
					continue
				}
				cx, cy := x+dx, y+dy
				if cx >= 0 && cy >= 0 && cx < w && cy < h && f.Field.Types[cy*w+cx] == gen.CellWalkable {
					return cx, cy
				}
			}
		}
	}
	return x, y
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// end is one side of a link: a cell of a map where a warp stands.
type end struct {
	f    *Field
	x, y int
}

// DevLinks joins the towns and dungeons to the fields with warps, until
// the map graph of phase 7 step 4 does:
//
//   - Touching fields link as Warps does.
//   - The portals of fields that touch no other field are free ends, by
//     field ID and edge.
//   - Each town, by number, links its first gate's portal to the first
//     free end. Its other gates' portals become the first free ends, so
//     dungeons open from towns first.
//   - Each dungeon, by ID, links its first floor's entrance to the first
//     free end, and each floor's exit to the next floor's entrance.
//
// A warp stands on the place's cell, and a player arrives 3 to 6 cells
// from the other end's place. Maps left without a free end are listed in
// unlinked.
func (s *Set) DevLinks() (warps map[string][]zone.Warp, unlinked []string) {
	warps = s.Warps()
	var free []end
	for _, f := range s.fields {
		if f.Kind != gen.KindField {
			continue
		}
		for _, p := range f.Field.Portals {
			dx, dy := edgeStep(p.Edge)
			if s.Get(gen.FieldID(f.X+dx, f.Y+dy)) == nil {
				free = append(free, end{f, p.X, p.Y})
			}
		}
	}
	link := func(a, b end) {
		ax, ay := arrivalNear(a.f, a.x, a.y)
		bx, by := arrivalNear(b.f, b.x, b.y)
		warps[a.f.ID] = append(warps[a.f.ID], zone.Warp{
			Name: fmt.Sprintf("gen_%s_link%d", a.f.ID, len(warps[a.f.ID])), X: a.x, Y: a.y, XS: 1, YS: 1, To: b.f.ID, ToX: bx, ToY: by,
		})
		warps[b.f.ID] = append(warps[b.f.ID], zone.Warp{
			Name: fmt.Sprintf("gen_%s_link%d", b.f.ID, len(warps[b.f.ID])), X: b.x, Y: b.y, XS: 1, YS: 1, To: a.f.ID, ToX: ax, ToY: ay,
		})
	}
	var towns, floors []*Field
	for _, f := range s.fields {
		switch f.Kind {
		case gen.KindTown:
			towns = append(towns, f)
		case gen.KindDungeon:
			floors = append(floors, f)
		}
	}
	slices.SortFunc(towns, func(a, b *Field) int { return cmp.Compare(a.Town, b.Town) })
	slices.SortFunc(floors, func(a, b *Field) int {
		return cmp.Or(strings.Compare(a.Dungeon, b.Dungeon), cmp.Compare(a.Floor, b.Floor))
	})
	for _, t := range towns {
		var gates []end
		for _, p := range t.Field.Places {
			if p.HasTag("portal") {
				gates = append(gates, end{t, p.X, p.Y})
			}
		}
		if len(free) == 0 || len(gates) == 0 {
			unlinked = append(unlinked, t.ID)
			continue
		}
		link(free[0], gates[0])
		free = append(gates[1:], free[1:]...)
	}
	for i, f := range floors {
		entrance, okE := placeOf(f, "entrance")
		if !okE {
			unlinked = append(unlinked, f.ID)
			continue
		}
		if f.Floor == 1 {
			if len(free) == 0 {
				unlinked = append(unlinked, f.ID)
				continue
			}
			link(free[0], end{f, entrance.X, entrance.Y})
			free = free[1:]
			continue
		}
		prev := floors[i-1]
		exit, okX := placeOf(prev, "exit")
		if prev.Dungeon != f.Dungeon || prev.Floor != f.Floor-1 || !okX {
			unlinked = append(unlinked, f.ID)
			continue
		}
		link(end{prev, exit.X, exit.Y}, end{f, entrance.X, entrance.Y})
	}
	return warps, unlinked
}

// Places returns the index of every place of the set's maps, with the
// warps as the map graph.
func (s *Set) Places(warps map[string][]zone.Warp) *places.Index {
	maps := make([]places.Map, 0, len(s.fields))
	var links []places.Link
	for _, f := range s.fields {
		maps = append(maps, places.Map{ID: f.ID, Places: f.Field.Places})
		for _, w := range warps[f.ID] {
			links = append(links, places.Link{From: f.ID, X: w.X, Y: w.Y, To: w.To, ToX: w.ToX, ToY: w.ToY})
		}
	}
	return places.New(maps, links)
}
