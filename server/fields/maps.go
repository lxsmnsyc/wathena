package fields

import (
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

// Dungeon floors and towns (decision 0040). The world layout places
// them on the map graph (decision 0041, layout.go).

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

// DungeonID is the ID of a dungeon of a style: the style's name without
// its mod prefix. The world layout adds -2, -3 and on for more of one
// style.
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

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Places returns the index of every place of the set's maps, with the
// warps as the map graph.
func (s *Set) Places(warps map[string][]zone.Warp) *places.Index {
	maps := make([]places.Map, 0, len(s.fields))
	var links []places.Link
	for _, f := range s.fields {
		maps = append(maps, places.Map{ID: f.ID, Places: f.AllPlaces()})
		for _, w := range warps[f.ID] {
			links = append(links, places.Link{From: f.ID, X: w.X, Y: w.Y, To: w.To, ToX: w.ToX, ToY: w.ToY})
		}
	}
	return places.New(maps, links)
}
