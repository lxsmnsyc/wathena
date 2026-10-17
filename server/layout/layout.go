// Package layout builds the world layout of a procedural world: its map
// graph, its town names and its level bands (strategy section 8.10,
// proposal 0002, review decision 43, decision 0041):
//
//   - Build makes a world's layout from the public branch of its seed
//     tree when `wactl world new` makes the world. The database keeps it,
//     so a new generator version or a mod change never renames a town or
//     moves a warp.
//   - Dev makes the layout of a `waserver --dev` world from its --fields,
//     --towns and --dungeons, with the links that step 3's dev links
//     made.
//   - Atlas is the layout while the server runs: display names, towns,
//     the graph's distances, and the part of the graph a character has
//     seen.
//
// The layout uses integer math and slices only, so it gives the same
// result on every platform. Only the server runs it: clients get the
// maps and the part of the graph their character found.
package layout

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// KindHome is the kind of a mod's home map (strategy section 8.12). The
// other kinds are the generator's: gen.KindField, gen.KindTown and
// gen.KindDungeon.
const KindHome = "home"

// Place tags that link ends use.
const (
	// TagPortal is a town's or a home map's portal, and with an edge, as
	// portal:east, a field's.
	TagPortal = "portal"
	// TagEntrance and TagExit are a dungeon floor's stairs.
	TagEntrance = "entrance"
	TagExit     = "exit"
)

// Defaults of Config (proposal 0002, review decision 43).
const (
	DefaultWidth     = 6
	DefaultHeight    = 5
	DefaultTowns     = 5
	DefaultDungeons  = 6
	DefaultBandWidth = 10
	DefaultBandStep  = 8
	DefaultMaxLevel  = 99
	DefaultFieldSize = 300
)

// The coverage check (proposal 0002, review decision 44): every band of
// BandWidth levels from level 1 to CoverageLevel, the transcendent level
// cap, needs CoverageFields fields whose band overlaps it.
const (
	CoverageLevel  = 99
	CoverageFields = 3
)

// Limits of Config.
const (
	MaxGridSide  = 32
	MaxTowns     = 99
	MaxDungeons  = 64
	MaxLevelCap  = 999
	MinFieldSize = 32
)

// maxStyleID is the longest style name, without its mod prefix, that a
// layout's dungeons take as their ID, so -2 to -64 still fit.
const maxStyleID = gen.MaxDungeonID - 3

// Attempts is how many layouts Build tries: the layout node, then
// attempt(1) to attempt(7).
const Attempts = 8

// Config is the shape of a new world's layout. The server's config sets
// it under procedural.layout, and `wactl world new` reads it there.
type Config struct {
	// Width and Height are the grid of fields, 6 × 4 by default.
	Width, Height int
	// Towns and Dungeons are how many to place, 5 and 6 by default.
	Towns, Dungeons int
	// BandWidth is the levels in each map's band, 10 by default: the
	// home area is levels 1 to 10. BandStep is the levels each warp away
	// from the home area adds, 8 by default. MaxLevel is the highest
	// level of any band, 99 by default.
	BandWidth, BandStep, MaxLevel int
	// FieldSize is each field's side in cells, 300 by default.
	FieldSize int
}

// DefaultConfig is the layout of review decisions 43 and 44: a 6 × 5
// grid of fields, 5 towns and 6 dungeons, with bands of 10 levels that
// rise 8 levels per warp.
func DefaultConfig() Config {
	return Config{Width: DefaultWidth, Height: DefaultHeight, Towns: DefaultTowns, Dungeons: DefaultDungeons,
		BandWidth: DefaultBandWidth, BandStep: DefaultBandStep, MaxLevel: DefaultMaxLevel, FieldSize: DefaultFieldSize}
}

// Check returns the config's first problem, or nil.
func (c Config) Check() error {
	switch {
	case c.Width < 1 || c.Height < 1 || c.Width > MaxGridSide || c.Height > MaxGridSide:
		return fmt.Errorf("the grid is 1 to %d fields on each side", MaxGridSide)
	case c.Towns < 1 || c.Towns > MaxTowns:
		return fmt.Errorf("towns is 1 to %d", MaxTowns)
	case c.Dungeons < 0 || c.Dungeons > MaxDungeons:
		return fmt.Errorf("dungeons is 0 to %d", MaxDungeons)
	case c.MaxLevel < 1 || c.MaxLevel > MaxLevelCap:
		return fmt.Errorf("max_level is 1 to %d", MaxLevelCap)
	case c.BandWidth < 1 || c.BandWidth > c.MaxLevel:
		return errors.New("band_width is 1 to max_level")
	case c.BandStep < 1 || c.BandStep > c.BandWidth:
		return errors.New("band_step is 1 to band_width")
	case c.FieldSize < MinFieldSize || c.FieldSize > gen.MaxSize:
		return fmt.Errorf("field_size is %d to %d", MinFieldSize, gen.MaxSize)
	}
	return nil
}

// Map is one map of a layout.
type Map struct {
	// ID is the map ID, such as gen:field:3:1, gen:town:t2,
	// gen:dungeon:crypt:1 or gen:home:village.
	ID string
	// Kind is gen.KindField, gen.KindTown, gen.KindDungeon or KindHome.
	Kind string
	// Name is the display name (strategy section 8.10).
	Name string
	// Levels is the level band: the lowest and the highest level.
	Levels [2]int
	// Distance is the number of warps from the home area.
	Distance int
	// X and Y are a field's coordinates on the grid. A town and a home
	// map have their seat's, a field near which they stand, and a
	// dungeon floor its entrance field's. The world map draws them there.
	X, Y int64
	// Size is a field's side in cells.
	Size int
	// Biome is the index of a field's biome in the generator's table,
	// and a town's seat's. It is -1 for other maps.
	Biome int
	// Record is a town's template, a floor's dungeon style, or a home
	// map's mod and folder, as mod:folder.
	Record string
	// Params are a town's or a floor's params (generator-v3, "Inputs"),
	// kept so a changed record never changes a made map.
	Params []byte
	// Town is a town's number.
	Town int64
	// Dungeon, Floor and Floors name a dungeon floor.
	Dungeon       string
	Floor, Floors int
	// NoMemo forbids memo points on the map.
	NoMemo bool
	// Base and Date are a rotating floor's map ID in the stored layout,
	// such as gen:dungeon:crypt:1, and its rotation's date, such as
	// 2026-10-31 (decision 0045). Apply sets them; they are empty for
	// other maps.
	Base, Date string
	// Season is a field's season: 0 until its region first changes
	// (decision 0045).
	Season int
}

// End is one end of a link: the place on a map where its warp stands,
// as the Index-th place, from 0, that has Tag.
type End struct {
	Map   string
	Tag   string
	Index int
}

// String writes the end as map#tag/index, for messages.
func (e End) String() string { return e.Map + "#" + e.Tag + "/" + strconv.Itoa(e.Index) }

// Link is a pair of linked portals: a warp at each end leads to the
// other end.
type Link struct {
	A, B End
}

// Layout is a world's maps and links.
type Layout struct {
	// Maps are in a fixed order: fields by row then column, towns by
	// number, dungeon floors by dungeon then floor, then the home map.
	Maps  []Map
	Links []Link
	// Home is the map ID of the home area: the home map, or the first
	// town.
	Home string
	// Attempt is the attempt of Build that made the layout: 0 for the
	// layout node, n for attempt(n).
	Attempt int
}

// Map returns a map by ID, or nil.
func (l *Layout) Map(id string) *Map {
	for i := range l.Maps {
		if l.Maps[i].ID == id {
			return &l.Maps[i]
		}
	}
	return nil
}

// Towns returns the towns in number order.
func (l *Layout) Towns() []*Map {
	var out []*Map
	for i := range l.Maps {
		if l.Maps[i].Kind == gen.KindTown {
			out = append(out, &l.Maps[i])
		}
	}
	slices.SortStableFunc(out, func(a, b *Map) int { return int(a.Town - b.Town) })
	return out
}

// Errors of the layout.
var (
	// ErrNoLayout is the error when every attempt failed. It lists each
	// attempt's problem.
	ErrNoLayout = errors.New("layout: no attempt made a valid layout")
	// ErrNoName is the error when a town found no name.
	ErrNoName = errors.New("layout: no town name passed the rules")
)

// TownRecord is a town template a layout may use.
type TownRecord struct {
	Name     string
	Template gen.TownTemplate
}

// StyleRecord is a dungeon style a layout may use.
type StyleRecord struct {
	Name  string
	Style gen.DungeonStyle
	// Rotation is the style's rotation, or nil (decision 0045).
	Rotation *RotationRule
}

// HomeMap is a mod's home map (strategy section 8.12), tagged home.
type HomeMap struct {
	// Folder is the map's folder name, such as village. Its map ID is
	// gen:home:<folder>.
	Folder string
	// Record names it, as mod:folder.
	Record string
	// Name is its display name.
	Name string
	// NoMemo is its nomemo flag.
	NoMemo bool
}

// HomeMapID returns the map ID of a home map folder.
func HomeMapID(folder string) string { return "gen:home:" + folder }

// Inputs are what a layout is made from besides its config.
type Inputs struct {
	// Generator is the world's map generator. Its climate gives each
	// field's biome.
	Generator gen.Generator
	// Public is the public branch of the world's seed tree.
	Public rng.Node
	// Towns and Styles are the town templates and dungeon styles, in
	// record order.
	Towns  []TownRecord
	Styles []StyleRecord
	// Naming is the naming styles.
	Naming *Naming
	// Blocked reports whether a name has a word the chat filter blocks.
	// nil blocks nothing.
	Blocked func(name string) bool
	// Home is the mod's home map, or nil for the first town.
	Home *HomeMap
}

// LayoutNode returns the public node of the layout.
func LayoutNode(public rng.Node) rng.Node { return public.Child("layout") }

// edge steps, by gen's edge index: south, east, north, west.
var edgeDX = [4]int64{0, 1, 0, -1}
var edgeDY = [4]int64{-1, 0, 1, 0}

// FieldPortal is the tag of a field's portal on an edge, such as
// portal:east.
func FieldPortal(edge int) string { return TagPortal + ":" + edgeNames[edge] }

// edgeNames name the edges, by gen's edge index, as field portal places
// do (generator-v3, "Named places").
var edgeNames = [4]string{"south", "east", "north", "west"}

// grid is the state of one attempt.
type grid struct {
	cfg Config
	in  Inputs
	l   *Layout
	// free marks the field portals no link uses, 4 per field by edge.
	free [][4]bool
	// seats are the fields of the towns so far, dungeon the fields with
	// a dungeon entrance.
	seats   []int
	dungeon []bool
	// open marks the open borders, 4 per field by edge.
	open [][4]bool
	// root is the field the border tree grows from, the first town's
	// seat.
	root int
}

func (g *grid) field(x, y int64) int { return int(y)*g.cfg.Width + int(x) }

func (g *grid) xy(i int) (int64, int64) { return int64(i % g.cfg.Width), int64(i / g.cfg.Width) }

func manhattan(ax, ay, bx, by int64) int64 {
	dx, dy := ax-bx, ay-by
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

// Build makes a world's layout (proposal 0002, review decision 43,
// docs/specs/generator-v3.md, "World layout"). It tries the layout node,
// then attempt(1) to attempt(7), and returns ErrNoLayout with each
// attempt's problem when every attempt fails. The town names come last:
// a town that finds no name is ErrNoName.
func Build(cfg Config, in Inputs) (*Layout, error) {
	if err := cfg.Check(); err != nil {
		return nil, err
	}
	if err := in.check(cfg); err != nil {
		return nil, err
	}
	node := LayoutNode(in.Public)
	var problems []string
	for n := range Attempts {
		a := node
		if n > 0 {
			a = node.Child("attempt", int64(n))
		}
		l, problem := attempt(cfg, in, a)
		if problem == "" {
			l.Attempt = n
			if err := finish(l, cfg, in); err != nil {
				return nil, err
			}
			return l, nil
		}
		problems = append(problems, "attempt "+strconv.Itoa(n)+": "+problem)
	}
	return nil, fmt.Errorf("%w: %s", ErrNoLayout, strings.Join(problems, "; "))
}

// check returns the inputs' first problem.
func (in Inputs) check(cfg Config) error {
	switch {
	case in.Generator.Climate == nil || !in.Generator.Makes(gen.KindTown) || !in.Generator.Makes(gen.KindDungeon):
		return fmt.Errorf("layout: map generator %d makes no towns and dungeons; a layout needs generator 3 or later", in.Generator.Version)
	case len(in.Towns) == 0:
		return errors.New("layout: no town template")
	case cfg.Dungeons > 0 && len(in.Styles) == 0:
		return errors.New("layout: no dungeon style")
	case in.Naming == nil:
		return errors.New("layout: no naming styles")
	}
	for _, s := range in.Styles {
		if id := DungeonID(s.Name); !gen.ValidDungeonID(id) || len(id) > maxStyleID {
			return fmt.Errorf("layout: dungeon style %s: its name without the mod prefix is not a dungeon ID: 1 to %d lowercase letters, digits, _ and -", s.Name, maxStyleID)
		}
	}
	return nil
}

// attempt makes one layout from the node a, or returns its problem.
func attempt(cfg Config, in Inputs, a rng.Node) (*Layout, string) {
	l := &Layout{}
	g := &grid{cfg: cfg, in: in, l: l, free: make([][4]bool, cfg.Width*cfg.Height), dungeon: make([]bool, cfg.Width*cfg.Height),
		open: make([][4]bool, cfg.Width*cfg.Height)}
	for i := range g.free {
		g.free[i] = [4]bool{true, true, true, true}
		x, y := g.xy(i)
		l.Maps = append(l.Maps, Map{ID: gen.FieldID(x, y), Kind: gen.KindField, X: x, Y: y, Size: cfg.FieldSize,
			Biome: in.Generator.Climate(in.Public, x, y)})
	}
	g.edges(a.Child("edges").Stream())
	if p := g.towns(a.Child("towns").Stream()); p != "" {
		return nil, p
	}
	if p := g.dungeons(a.Child("dungeons").Stream()); p != "" {
		return nil, p
	}
	if p := reachAll(l); p != "" {
		return nil, p
	}
	order(l)
	Bands(l, BandConfig{Width: cfg.BandWidth, Step: cfg.BandStep, MaxLevel: cfg.MaxLevel}, in.Styles)
	if p := Coverage(l, cfg); p != "" {
		return nil, p
	}
	return l, ""
}

// edges opens the borders between neighboring fields (proposal 0002,
// review decision 44). It draws the root field, the first town's seat,
// and grows a tree of open borders from it in layers, so the warps from
// the home area rise steadily and the bands reach the top levels:
//
//   - The root is layer 0. Each next layer takes 2 fields, or 3 with a
//     chance of 1 in 3, among the fields not yet reached that touch the
//     layer before. Each pick is among those with the fewest unreached
//     neighbors, so the layers sweep the grid and leave few pockets. A
//     picked field opens its border to a drawn neighbor of the layer
//     before.
//   - When no field touches the last layer, each field still unreached,
//     by row and column and again until none is left, opens its border
//     to its reached neighbor of the most layers, the first by edge on
//     a tie.
//   - Then each closed border whose two fields stand at layers no more
//     than 1 apart opens with a chance of 1 in 2: the borders between
//     columns, row by row, then those between rows. Such a loop
//     shortens no way from the root.
func (g *grid) edges(s *rng.Rand) {
	cells := len(g.free)
	w, h := int64(g.cfg.Width), int64(g.cfg.Height)
	g.root = s.IntN(cells)
	depth := make([]int, cells)
	for i := range depth {
		depth[i] = -1
	}
	// link opens the border of field a on an edge. The link's first end
	// is the field to the west or south, as for every field link.
	link := func(a, edge int) int {
		ax, ay := g.xy(a)
		bx, by := ax+edgeDX[edge], ay+edgeDY[edge]
		b := g.field(bx, by)
		other := (edge + 2) % 4
		g.free[a][edge], g.free[b][other] = false, false
		g.open[a][edge], g.open[b][other] = true, true
		k := Link{A: End{gen.FieldID(ax, ay), FieldPortal(edge), 0}, B: End{gen.FieldID(bx, by), FieldPortal(other), 0}}
		if edge == gen.EdgeSouth || edge == gen.EdgeWest {
			k.A, k.B = k.B, k.A
		}
		g.l.Links = append(g.l.Links, k)
		return b
	}
	// neighbor returns the field across an edge, or -1 off the grid.
	neighbor := func(i, edge int) int {
		x, y := g.xy(i)
		nx, ny := x+edgeDX[edge], y+edgeDY[edge]
		if nx < 0 || ny < 0 || nx >= w || ny >= h {
			return -1
		}
		return g.field(nx, ny)
	}
	unreached := func(i int) int {
		n := 0
		for e := range 4 {
			if b := neighbor(i, e); b >= 0 && depth[b] < 0 {
				n++
			}
		}
		return n
	}
	depth[g.root] = 0
	layer := []int{g.root}
	for len(layer) > 0 {
		want := 2
		if s.IntN(3) == 0 {
			want = 3
		}
		var next []int
		for len(next) < want {
			// The fields that touch the layer, with the fewest unreached
			// neighbors.
			var frontier []int
			fewest := 5
			for i := range cells {
				if depth[i] >= 0 {
					continue
				}
				touches := false
				for e := range 4 {
					if b := neighbor(i, e); b >= 0 && depth[b] >= 0 && slices.Contains(layer, b) {
						touches = true
					}
				}
				if !touches {
					continue
				}
				switch n := unreached(i); {
				case n < fewest:
					fewest, frontier = n, []int{i}
				case n == fewest:
					frontier = append(frontier, i)
				}
			}
			if len(frontier) == 0 {
				break
			}
			f := frontier[s.IntN(len(frontier))]
			var parents []int
			for e := range 4 {
				if b := neighbor(f, e); b >= 0 && depth[b] >= 0 && slices.Contains(layer, b) {
					parents = append(parents, e)
				}
			}
			p := neighbor(f, parents[s.IntN(len(parents))])
			link(f, (slices.IndexFunc([]int{0, 1, 2, 3}, func(e int) bool { return neighbor(f, e) == p })))
			depth[f] = depth[p] + 1
			next = append(next, f)
		}
		layer = next
	}
	for {
		added := false
		for i := range cells {
			if depth[i] >= 0 {
				continue
			}
			best := -1
			for e := range 4 {
				if b := neighbor(i, e); b >= 0 && depth[b] >= 0 && (best < 0 || depth[b] > depth[neighbor(i, best)]) {
					best = e
				}
			}
			if best >= 0 {
				depth[i] = depth[neighbor(i, best)] + 1
				link(i, best)
				added = true
			}
		}
		if !added {
			break
		}
	}
	loop := func(a, edge int) {
		if !g.free[a][edge] {
			return
		}
		x, y := g.xy(a)
		b := g.field(x+edgeDX[edge], y+edgeDY[edge])
		if d := depth[a] - depth[b]; d < -1 || d > 1 {
			return
		}
		if s.IntN(2) == 0 {
			link(a, edge)
		}
	}
	for y := range h {
		for x := range w - 1 {
			loop(g.field(x, y), gen.EdgeEast)
		}
	}
	for y := range h - 1 {
		for x := range w {
			loop(g.field(x, y), gen.EdgeNorth)
		}
	}
}

// slot is a free field portal.
type slot struct {
	field, edge int
}

// takeSlot takes the first free portal of a field, by edge.
func (g *grid) takeSlot(f int) (slot, bool) {
	for e := range 4 {
		if g.free[f][e] {
			g.free[f][e] = false
			return slot{f, e}, true
		}
	}
	return slot{}, false
}

func (g *grid) hasSlot(f int) bool { return g.free[f] != [4]bool{} }

// pick draws one of the indexes that ok accepts, or -1 when none does.
func pick(s *rng.Rand, n int, ok func(int) bool) int {
	var c []int
	for i := range n {
		if ok(i) {
			c = append(c, i)
		}
	}
	if len(c) == 0 {
		return -1
	}
	return c[s.IntN(len(c))]
}

// townSeatSpacing is the least grid distance between two towns' seats
// while fields allow it.
const townSeatSpacing = 2

// townReach is the farthest field, in warps between fields from its
// seat, that a town links to.
const townReach = 2

// townSpread is the most warps from the home area by which a town's
// fields may differ, so a town never shortens the way to a field.
const townSpread = 2

// fieldSteps returns each field's number of warps from a field over
// the open borders, or -1 where none leads.
func (g *grid) fieldSteps(from int) []int {
	d := make([]int, len(g.free))
	for i := range d {
		d[i] = -1
	}
	d[from] = 0
	for q := []int{from}; len(q) > 0; q = q[1:] {
		f := q[0]
		x, y := g.xy(f)
		for e := range 4 {
			nx, ny := x+edgeDX[e], y+edgeDY[e]
			if !g.open[f][e] {
				continue
			}
			n := g.field(nx, ny)
			if d[n] < 0 {
				d[n] = d[f] + 1
				q = append(q, n)
			}
		}
	}
	return d
}

// towns places the towns, by number. Each draws a template. The first
// town's seat is the root of the border tree. Each other town draws its
// seat among the fields at least two grid steps from other seats while
// one is left, else one step, else any: it shuffles them and takes the
// first whose gates all find fields. Each gate's portal links to a free
// field portal, one per field, taking fields by warps from the seat, at
// most two, then by row and column. A field joins only while the
// town's fields stay within two warps of each other from the home area,
// so a town is never a shortcut. With a home map, the first town draws
// a template with 3 or more gates when one exists, and its first gate
// links to the home map.
func (g *grid) towns(s *rng.Rand) string {
	for n := int64(1); n <= int64(g.cfg.Towns); n++ {
		home := n == 1 && g.in.Home != nil
		t := pick(s, len(g.in.Towns), func(i int) bool { return !home || g.in.Towns[i].Template.Gates >= 3 })
		if t < 0 {
			t = s.IntN(len(g.in.Towns))
		}
		rec := g.in.Towns[t]
		gates := rec.Template.Gates
		first := 0
		if home {
			first = 1
		}
		// Each field's warps from the home area so far, before this
		// town links. The first town is the home area's way in.
		var fromHome []int
		if n > 1 {
			fromHome = g.homeSteps()
		}
		seats := []int{g.root}
		if n > 1 {
			seats = g.seatOrder(s)
		}
		seat, fields := -1, []int(nil)
		for _, c := range seats {
			if f := g.gateFields(c, gates-first, fromHome); len(f) == gates-first {
				seat, fields = c, f
				break
			}
		}
		if seat < 0 {
			sx, sy := g.xy(seats[0])
			found := len(g.gateFields(seats[0], gates-first, fromHome)) + first
			return fmt.Sprintf("town %s at field %d:%d found %d of its %d gates' free portals", gen.TownID(n), sx, sy, found, gates)
		}
		g.seats = append(g.seats, seat)
		sx, sy := g.xy(seat)
		id := gen.TownMapID(n)
		params, err := gen.EncodeTown(gen.TownParams{Template: rec.Template})
		if err != nil {
			return "town " + gen.TownID(n) + " (" + rec.Name + "): " + err.Error()
		}
		g.l.Maps = append(g.l.Maps, Map{ID: id, Kind: gen.KindTown, X: sx, Y: sy, Biome: g.l.Maps[seat].Biome,
			Record: rec.Name, Params: params, Town: n})
		if home {
			hm := g.in.Home
			g.l.Maps = append(g.l.Maps, Map{ID: HomeMapID(hm.Folder), Kind: KindHome, Name: hm.Name, X: sx, Y: sy, Biome: -1,
				Record: hm.Record, NoMemo: hm.NoMemo})
			g.l.Links = append(g.l.Links, Link{A: End{HomeMapID(hm.Folder), TagPortal, 0}, B: End{id, TagPortal, 0}})
		}
		for i, f := range fields {
			sl, _ := g.takeSlot(f)
			x, y := g.xy(f)
			g.l.Links = append(g.l.Links, Link{
				A: End{gen.FieldID(x, y), FieldPortal(sl.edge), 0},
				B: End{id, TagPortal, first + i},
			})
		}
	}
	return ""
}

// seatOrder shuffles the fields a town may sit on: those at least two
// grid steps from every seat so far, else one, else all.
func (g *grid) seatOrder(s *rng.Rand) []int {
	for _, spacing := range []int64{townSeatSpacing, 1, 0} {
		var c []int
		for i := range len(g.free) {
			x, y := g.xy(i)
			ok := true
			for _, o := range g.seats {
				ox, oy := g.xy(o)
				if manhattan(x, y, ox, oy) < spacing {
					ok = false
				}
			}
			if ok {
				c = append(c, i)
			}
		}
		if len(c) > 0 {
			s.Shuffle(len(c), func(i, j int) { c[i], c[j] = c[j], c[i] })
			return c
		}
	}
	return nil
}

// gateFields returns up to want fields with a free portal for the gates
// of a town at seat, in the order the gates link them. fromHome, when
// set, is each field's warps from the home area: the fields stay within
// townSpread of each other.
func (g *grid) gateFields(seat, want int, fromHome []int) []int {
	steps := g.fieldSteps(seat)
	order := make([]int, 0, len(steps))
	for i, d := range steps {
		if d >= 0 && d <= townReach {
			order = append(order, i)
		}
	}
	slices.SortStableFunc(order, func(a, b int) int { return steps[a] - steps[b] })
	var out []int
	lo, hi := -1, -1
	for _, f := range order {
		if len(out) >= want {
			break
		}
		if !g.hasSlot(f) {
			continue
		}
		if fromHome != nil {
			d := fromHome[f]
			if d < 0 {
				continue
			}
			nlo, nhi := d, d
			if lo >= 0 {
				nlo, nhi = min(lo, d), max(hi, d)
			}
			if nhi-nlo > townSpread {
				continue
			}
			lo, hi = nlo, nhi
		}
		out = append(out, f)
	}
	return out
}

// homeSteps returns each field's warps from the first town over the
// links so far, or -1 where none leads.
func (g *grid) homeSteps() []int {
	l := g.l
	d := distances(adjacency(l), index(l)[gen.TownMapID(1)])
	out := make([]int, len(g.free))
	at := index(l)
	for i := range out {
		x, y := g.xy(i)
		out[i] = d[at[gen.FieldID(x, y)]]
	}
	return out
}

// DungeonID is the ID of a dungeon of a style: the style's name without
// its mod prefix.
func DungeonID(style string) string {
	if _, local, ok := strings.Cut(style, ":"); ok {
		return local
	}
	return style
}

// dungeons places the dungeons, by number. Each draws a style, then a
// field with a free portal, one without a dungeon while one is left, and
// links that portal to the first floor's entrance. Each floor's exit
// links to the next floor's entrance. A dungeon's ID is its style's name
// without the mod prefix, then -2, -3 and on for more of one style.
func (g *grid) dungeons(s *rng.Rand) string {
	var ids []string
	for k := 1; k <= g.cfg.Dungeons; k++ {
		st := g.in.Styles[s.IntN(len(g.in.Styles))]
		f := pick(s, len(g.free), func(i int) bool { return g.hasSlot(i) && !g.dungeon[i] && !slices.Contains(g.seats, i) })
		if f < 0 {
			f = pick(s, len(g.free), func(i int) bool { return g.hasSlot(i) && !g.dungeon[i] })
		}
		if f < 0 {
			f = pick(s, len(g.free), g.hasSlot)
		}
		if f < 0 {
			return "dungeon " + strconv.Itoa(k) + " found no free field portal"
		}
		g.dungeon[f] = true
		sl, _ := g.takeSlot(f)
		x, y := g.xy(f)
		base := DungeonID(st.Name)
		id := base
		for n := 2; slices.Contains(ids, id); n++ {
			id = base + "-" + strconv.Itoa(n)
		}
		ids = append(ids, id)
		floors := st.Style.Floors
		for floor := 1; floor <= floors; floor++ {
			params, err := gen.EncodeDungeon(gen.DungeonParams{Style: st.Style, Floor: floor, Floors: floors})
			if err != nil {
				return "dungeon " + id + " (" + st.Name + "): " + err.Error()
			}
			mid := gen.DungeonMapID(id, floor)
			g.l.Maps = append(g.l.Maps, Map{ID: mid, Kind: gen.KindDungeon, X: x, Y: y, Biome: -1, Record: st.Name,
				Params: params, Dungeon: id, Floor: floor, Floors: floors})
			if floor == 1 {
				g.l.Links = append(g.l.Links, Link{A: End{gen.FieldID(x, y), FieldPortal(sl.edge), 0}, B: End{mid, TagEntrance, 0}})
			} else {
				g.l.Links = append(g.l.Links, Link{A: End{gen.DungeonMapID(id, floor-1), TagExit, 0}, B: End{mid, TagEntrance, 0}})
			}
		}
	}
	return ""
}

// homeOf returns the home area: the home map, else the first town.
func HomeOf(l *Layout) string {
	for _, m := range l.Maps {
		if m.Kind == KindHome {
			return m.ID
		}
	}
	if t := l.Towns(); len(t) > 0 {
		return t[0].ID
	}
	if len(l.Maps) > 0 {
		return l.Maps[0].ID
	}
	return ""
}

// index maps each map ID to its index in l.Maps.
func index(l *Layout) map[string]int {
	out := make(map[string]int, len(l.Maps))
	for i, m := range l.Maps {
		out[m.ID] = i
	}
	return out
}

// adjacency lists each map's neighbors, by index, in link order. Links
// go both ways.
func adjacency(l *Layout) [][]int {
	at := index(l)
	out := make([][]int, len(l.Maps))
	for _, k := range l.Links {
		a, okA := at[k.A.Map]
		b, okB := at[k.B.Map]
		if !okA || !okB {
			continue
		}
		out[a] = append(out[a], b)
		out[b] = append(out[b], a)
	}
	return out
}

// distances returns each map's number of warps from map from, or -1 for
// a map no way reaches.
func distances(adj [][]int, from int) []int {
	d := make([]int, len(adj))
	for i := range d {
		d[i] = -1
	}
	if from < 0 {
		return d
	}
	d[from] = 0
	q := []int{from}
	for len(q) > 0 {
		v := q[0]
		q = q[1:]
		for _, u := range adj[v] {
			if d[u] < 0 {
				d[u] = d[v] + 1
				q = append(q, u)
			}
		}
	}
	return d
}

// reachAll returns a problem naming the first map that the home area
// does not reach, or "".
func reachAll(l *Layout) string {
	l.Home = HomeOf(l)
	d := distances(adjacency(l), index(l)[l.Home])
	for i, m := range l.Maps {
		if d[i] < 0 {
			return "the map " + m.ID + " is not reachable from the home area " + l.Home
		}
	}
	return ""
}

// finish gives the layout its names. attempt gave it its bands.
func finish(l *Layout, _ Config, in Inputs) error {
	l.Home = HomeOf(l)
	if err := nameTowns(l, in); err != nil {
		return err
	}
	DeriveNames(l)
	return nil
}

// BandConfig shapes the level bands: Width levels in each band, Step
// levels per warp, and MaxLevel the top of the highest band.
type BandConfig struct {
	Width, Step, MaxLevel int
}

// Bands gives each map its distance from the home area and its level
// band (proposal 0002, review decision 44). The home area is levels 1
// to Width: the home map, the first town, and the fields nearest them.
// Each further warp adds Step levels, up to the band that ends at
// MaxLevel. So with d the warps of a field, a town or the home map, and
// d0 the fewest warps of any field, a map starts at level
// 1 + Step × max(0, d - d0). Each dungeon floor is its style's
// level_step above the floor before it, from the band of the field that
// holds its entrance, and stops at the highest band. A map no way
// reaches gets the first band.
func Bands(l *Layout, bc BandConfig, styles []StyleRecord) {
	at := index(l)
	adj := adjacency(l)
	home, ok := at[l.Home]
	if !ok {
		home = -1
	}
	d := distances(adj, home)
	near := -1
	for i, m := range l.Maps {
		l.Maps[i].Distance = max(d[i], 0)
		if m.Kind == gen.KindField && d[i] >= 0 && (near < 0 || d[i] < near) {
			near = d[i]
		}
	}
	near = max(near, 0)
	top := max(bc.MaxLevel-bc.Width+1, 1)
	band := func(low int) [2]int {
		low = min(max(low, 1), top)
		return [2]int{low, min(low+bc.Width-1, bc.MaxLevel)}
	}
	for i, m := range l.Maps {
		if m.Kind == gen.KindDungeon {
			continue
		}
		l.Maps[i].Levels = band(1 + bc.Step*max(d[i]-near, 0))
	}
	// A floor's band builds on its entrance's: the map its floor 1
	// entrance links to.
	entrance := map[string]string{}
	for _, k := range l.Links {
		for _, e := range [2][2]End{{k.A, k.B}, {k.B, k.A}} {
			if e[1].Tag == TagEntrance {
				if fm := l.Map(e[1].Map); fm != nil && fm.Floor == 1 {
					entrance[fm.Dungeon] = e[0].Map
				}
			}
		}
	}
	for i, m := range l.Maps {
		if m.Kind != gen.KindDungeon {
			continue
		}
		base := [2]int{1, 0}
		if j, ok := at[entrance[m.Dungeon]]; ok {
			base = l.Maps[j].Levels
		}
		step := 0
		if s, ok := findStyle(styles, m.Record); ok {
			step = s.Style.LevelStep
		} else if p, err := gen.DecodeDungeon(m.Params); err == nil {
			step = p.Style.LevelStep
		}
		l.Maps[i].Levels = band(base[0] + (m.Floor-1)*step)
	}
}

// CoverageBands returns the bands the coverage check counts: levels 1
// to Width, Width + 1 to 2 × Width, and on, up to CoverageLevel or the
// max level, whichever is lower.
func CoverageBands(cfg Config) [][2]int {
	limit := min(CoverageLevel, cfg.MaxLevel)
	var out [][2]int
	for lo := 1; lo <= limit; lo += cfg.BandWidth {
		out = append(out, [2]int{lo, min(lo+cfg.BandWidth-1, limit)})
	}
	return out
}

// Covers reports whether a grid has the fields to pass the coverage
// check: CoverageFields per band. A smaller grid skips it.
func Covers(cfg Config) bool {
	return cfg.Width*cfg.Height >= CoverageFields*len(CoverageBands(cfg))
}

// Coverage returns the first band of CoverageBands that fewer than
// CoverageFields fields overlap, as a problem, or "" (proposal 0002,
// review decision 44). A grid too small to pass passes.
func Coverage(l *Layout, cfg Config) string {
	if !Covers(cfg) {
		return ""
	}
	for _, b := range CoverageBands(cfg) {
		n := 0
		for _, m := range l.Maps {
			if m.Kind == gen.KindField && m.Levels[0] <= b[1] && m.Levels[1] >= b[0] {
				n++
			}
		}
		if n < CoverageFields {
			return fmt.Sprintf("levels %d to %d have %d fields, fewer than %d", b[0], b[1], n, CoverageFields)
		}
	}
	return ""
}

func findStyle(styles []StyleRecord, name string) (StyleRecord, bool) {
	for _, s := range styles {
		if s.Name == name {
			return s, true
		}
	}
	return StyleRecord{}, false
}

// Checksum returns the SHA-256 of the layout's canonical encoding: each
// map's fields, then each link's ends, each value with a length prefix.
// The golden layouts of the tests compare it.
func (l *Layout) Checksum() [32]byte {
	h := sha256.New()
	put := func(vals ...string) {
		for _, v := range vals {
			var n [4]byte
			binary.BigEndian.PutUint32(n[:], uint32(len(v))) // #nosec G115 -- values are short.
			h.Write(n[:])
			h.Write([]byte(v))
		}
	}
	itoa := func(v int64) string { return strconv.FormatInt(v, 10) }
	for _, m := range l.Maps {
		put("map", m.ID, m.Kind, m.Name, itoa(int64(m.Levels[0])), itoa(int64(m.Levels[1])), itoa(int64(m.Distance)),
			itoa(m.X), itoa(m.Y), itoa(int64(m.Size)), itoa(int64(m.Biome)), m.Record, string(m.Params), itoa(m.Town),
			m.Dungeon, itoa(int64(m.Floor)), itoa(int64(m.Floors)), strconv.FormatBool(m.NoMemo))
	}
	for _, k := range l.Links {
		put("link", k.A.Map, k.A.Tag, itoa(int64(k.A.Index)), k.B.Map, k.B.Tag, itoa(int64(k.B.Index)))
	}
	put("home", l.Home)
	var sum [32]byte
	h.Sum(sum[:0])
	return sum
}
