//go:build procedural

package gen

import (
	"slices"
	"strconv"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Generator version 3 adds dungeons, towns and named places (strategy
// sections 8.5 and 8.6, proposal 0002, review decision 42, decision
// 0040). Its fields are version 2's, with places: one per portal and a
// few clearings. Dungeon floors and towns are built on a grid of blocks
// of 2 × 2 cells, the size of a WMAP ground surface, so a wall's surface
// matches its cells and can stand up with side tiles.

const (
	// v3Attempts is the most tries for one dungeon floor or town.
	v3Attempts = 8
	// v3Clearings is the most clearings of a field.
	v3Clearings = 8
)

// v3Tileset is the art and light of a dungeon or town tileset. Texture
// and prop names are its slots: texture/gen/<name>/<slot> and
// model/gen/<name>/<slot>.
type v3Tileset struct {
	Name, Kind string
	// Floor, Accent and Wall are a dungeon's textures. Lot, Road, Side,
	// Square, Wall and Rough are a town's: Wall serves both.
	Floor, Accent, Wall          string
	Lot, Road, Side, Square, Out string
	// Props are the tileset's props. Decor are the props that clutter a
	// dungeon's rooms, with weights.
	Props []string
	Decor []v2Prop
	// Pillar is the prop at the corners of large rooms, or "".
	Pillar string
	// WallHeight is the height of walls, in cells.
	WallHeight       rng.Fixed
	Diffuse, Ambient [3]rng.Fixed
}

// textures lists the tileset's textures, sorted.
func (t *v3Tileset) textures() []string {
	var out []string
	for _, s := range []string{t.Floor, t.Accent, t.Wall, t.Lot, t.Road, t.Side, t.Square, t.Out} {
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return out
}

// hasProp reports whether the tileset has a prop.
func (t *v3Tileset) hasProp(p string) bool { return slices.Contains(t.Props, p) }

// Props every dungeon tileset has.
var v3StairProps = []string{"stairs_up", "stairs_down", "altar"}

// v3Tilesets are the dungeon and town tilesets of version 3. Their
// names differ from the field tilesets', so every texture and model name
// is one slot of one tileset.
var v3Sets = []v3Tileset{
	{
		Name: "cave", Kind: KindDungeon, Floor: "dirt", Accent: "mud", Wall: "rock",
		Props: append([]string{"stalagmite", "rock"}, v3StairProps...),
		Decor: []v2Prop{{"stalagmite", 3}, {"rock", 2}}, WallHeight: 3 * rng.One,
		Diffuse: [3]rng.Fixed{rng.One * 55 / 100, rng.One / 2, rng.One * 45 / 100},
		Ambient: [3]rng.Fixed{rng.One * 3 / 10, rng.One * 28 / 100, rng.One * 26 / 100},
	},
	{
		Name: "crypt", Kind: KindDungeon, Floor: "stone", Accent: "moss", Wall: "brick",
		Props: append([]string{"pillar", "coffin", "bones", "brazier"}, v3StairProps...),
		Decor: []v2Prop{{"coffin", 2}, {"bones", 3}, {"brazier", 1}}, Pillar: "pillar", WallHeight: 3 * rng.One,
		Diffuse: [3]rng.Fixed{rng.One / 2, rng.One / 2, rng.One * 6 / 10},
		Ambient: [3]rng.Fixed{rng.One * 28 / 100, rng.One * 28 / 100, rng.One * 34 / 100},
	},
	{
		Name: "tower", Kind: KindDungeon, Floor: "plank", Accent: "stone", Wall: "brick",
		Props: append([]string{"pillar", "crate", "brazier"}, v3StairProps...),
		Decor: []v2Prop{{"crate", 3}, {"brazier", 1}}, Pillar: "pillar", WallHeight: 3 * rng.One,
		Diffuse: [3]rng.Fixed{rng.One * 7 / 10, rng.One * 65 / 100, rng.One * 6 / 10},
		Ambient: [3]rng.Fixed{rng.One * 32 / 100, rng.One * 3 / 10, rng.One * 3 / 10},
	},
	{
		Name: "ruins", Kind: KindDungeon, Floor: "cobble", Accent: "grass", Wall: "rock",
		Props: append([]string{"pillar", "rock", "bush"}, v3StairProps...),
		Decor: []v2Prop{{"rock", 3}, {"bush", 2}}, Pillar: "pillar", WallHeight: 2 * rng.One,
		Diffuse: [3]rng.Fixed{rng.One * 9 / 10, rng.One * 9 / 10, rng.One * 85 / 100},
		Ambient: [3]rng.Fixed{rng.One * 38 / 100, rng.One * 38 / 100, rng.One * 36 / 100},
	},
	{
		Name: "town", Kind: KindTown, Wall: "stone", Lot: "grass", Road: "paving", Side: "dirt", Square: "cobble", Out: "grass",
		Props:      []string{"fountain", "lamp", "tower", "gate", "tree", "bush", "well", "crate"},
		WallHeight: rng.One * 5 / 2,
		Diffuse:    [3]rng.Fixed{rng.One, rng.One, rng.One * 95 / 100},
		Ambient:    [3]rng.Fixed{rng.One * 4 / 10, rng.One * 4 / 10, rng.One * 38 / 100},
	},
}

// v3Set returns a tileset of a kind by name.
func v3Set(kind, name string) *v3Tileset {
	for i := range v3Sets {
		if v3Sets[i].Kind == kind && v3Sets[i].Name == name {
			return &v3Sets[i]
		}
	}
	return nil
}

// v3Tilesets lists the field tilesets of version 2 and the dungeon and
// town tilesets.
func v3Tilesets() []Tileset {
	out := v2Tilesets()
	for i := range out {
		out[i].Kind = KindField
	}
	for i := range v3Sets {
		s := &v3Sets[i]
		props := slices.Clone(s.Props)
		slices.Sort(props)
		out = append(out, Tileset{Name: s.Name, Kind: s.Kind, Textures: s.textures(), Models: props})
	}
	return out
}

// fieldV3 is a version 2 field with its places: a portal place on each
// edge, and clearings.
func fieldV3(node rng.Node, p Params) (Field, error) {
	f, err := fieldV2(node, p)
	if err != nil {
		return f, err
	}
	a := node
	if f.Attempt > 0 {
		a = node.Child("attempt", int64(f.Attempt))
	}
	f.Places = fieldPlaces(&f, a.Child("places"))
	return f, nil
}

// EdgeNames names the edges, by index.
var EdgeNames = [4]string{"south", "east", "north", "west"}

// fieldPlaces returns a field's places: each portal, as portal and
// portal:<edge>, then up to v3Clearings clearings. A clearing is a
// Poisson-disk point 24 cells from the others, whose 7 × 7 cells are dry
// walkable ground, at least 12 cells from every portal.
func fieldPlaces(f *Field, n rng.Node) []Place {
	out := make([]Place, 0, len(f.Portals)+v3Clearings)
	for _, p := range f.Portals {
		out = append(out, Place{Tags: []string{"portal", "portal:" + EdgeNames[p.Edge]}, X: p.X, Y: p.Y})
	}
	clearings := 0
	for _, pt := range poisson(n.Stream(), f.Width, f.Height, 24) {
		if clearings == v3Clearings {
			break
		}
		x, y := pt[0], pt[1]
		if x < 3 || y < 3 || x >= f.Width-3 || y >= f.Height-3 {
			continue
		}
		near := false
		for _, p := range f.Portals {
			if (p.X-x)*(p.X-x)+(p.Y-y)*(p.Y-y) < 144 {
				near = true
			}
		}
		open := !near
		for dy := -3; dy <= 3 && open; dy++ {
			for dx := -3; dx <= 3 && open; dx++ {
				open = f.Types[(y+dy)*f.Width+x+dx] == CellWalkable
			}
		}
		if open {
			out = append(out, Place{Tags: []string{"clearing"}, X: x, Y: y})
			clearings++
		}
	}
	return out
}

// mapV3 makes a dungeon floor or a town from its params.
func mapV3(node rng.Node, p MapParams) (Field, error) {
	switch p.Kind {
	case KindDungeon:
		dp, err := DecodeDungeon(p.Params)
		if err != nil {
			return Field{}, err
		}
		return dungeonV3(node, dp)
	case KindTown:
		tp, err := DecodeTown(p.Params)
		if err != nil {
			return Field{}, err
		}
		return townV3(node, tp)
	}
	return Field{}, ErrKind
}

// attempts runs build on the node, then on attempt(1) to attempt(7),
// until one passes. It returns ErrNoMap with each attempt's problem when
// all fail.
func attempts(node rng.Node, build func(a rng.Node) (*v3Blocks, string)) (Field, error) {
	problems := ""
	for n := range v3Attempts {
		a := node
		if n > 0 {
			a = node.Child("attempt", int64(n))
		}
		m, problem := build(a)
		if problem == "" {
			f := m.field()
			f.Attempt = n
			return f, nil
		}
		if problems != "" {
			problems += "; "
		}
		problems += "attempt " + strconv.Itoa(n) + ": " + problem
	}
	return Field{}, wrapError(ErrNoMap, problems)
}

// v3Object is a prop of a block map. Its position is in half cells, so a
// building's center can sit on a cell's corner.
type v3Object struct {
	prop   string
	hx, hy int
	turn   int
	scale  rng.Fixed
}

// v3Blocks is a dungeon floor or a town on a grid of blocks of 2 × 2
// cells.
type v3Blocks struct {
	ts     *v3Tileset
	bw, bh int
	// walk marks walkable blocks. height is each block's flat height and
	// mat its texture, an index into ts.textures().
	walk    []bool
	prop    []bool
	height  []rng.Fixed
	mat     []uint8
	objects []v3Object
	places  []Place
	tex     []string
}

func newV3Blocks(ts *v3Tileset, bw, bh int) *v3Blocks {
	n := bw * bh
	return &v3Blocks{
		ts: ts, bw: bw, bh: bh,
		walk: make([]bool, n), prop: make([]bool, n), height: make([]rng.Fixed, n), mat: make([]uint8, n),
		tex: ts.textures(),
	}
}

// texture returns the index of a texture of the tileset.
func (m *v3Blocks) texture(name string) uint8 {
	return uint8(slices.Index(m.tex, name)) // #nosec G115 -- a few textures, all present.
}

// in reports whether block (x, y) is on the map.
func (m *v3Blocks) in(x, y int) bool { return x >= 0 && y >= 0 && x < m.bw && y < m.bh }

// clear reports whether a blocking prop may stand on block (x, y): the
// block and its 8 neighbors are walkable and not reserved. Since its 8
// neighbors stay walkable, the prop never splits a walkable area.
func (m *v3Blocks) clear(x, y int, reserved []bool) bool {
	if x < 1 || y < 1 || x >= m.bw-1 || y >= m.bh-1 {
		return false
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			i := (y+dy)*m.bw + x + dx
			if !m.walk[i] || reserved != nil && reserved[i] {
				return false
			}
		}
	}
	return true
}

// reserve marks a block and its 8 neighbors.
func (m *v3Blocks) reserve(reserved []bool, x, y int) {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if m.in(x+dx, y+dy) {
				reserved[(y+dy)*m.bw+x+dx] = true
			}
		}
	}
}

// propAt puts a prop on the middle of block (x, y), turned and scaled by
// a hash. A blocking prop makes its block unwalkable.
func (m *v3Blocks) propAt(prop string, x, y int, hv uint64, blocks bool) {
	if blocks {
		m.walk[y*m.bw+x] = false
		m.prop[y*m.bw+x] = true
	}
	m.objects = append(m.objects, v3Object{
		prop: prop, hx: 4*x + 2, hy: 4*y + 2,
		turn:  int(hv >> 32 % 16),                                 // #nosec G115 -- below 16.
		scale: rng.One*9/10 + rng.Fixed(hv>>40%uint64(rng.One/5)), // #nosec G115 -- below 0.2.
	})
}

// blockDistances returns each block's distance in blocks from block
// start over walkable blocks along the 4 axes, or -1.
func (m *v3Blocks) blockDistances(start int) []int32 {
	dist := make([]int32, len(m.walk))
	for i := range dist {
		dist[i] = -1
	}
	dist[start] = 0
	queue := []int{start}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		x, y := c%m.bw, c/m.bw
		for _, d := range v2Neighbors8[:4] {
			nx, ny := x+d[0], y+d[1]
			if !m.in(nx, ny) {
				continue
			}
			j := ny*m.bw + nx
			if dist[j] < 0 && m.walk[j] {
				dist[j] = dist[c] + 1
				queue = append(queue, j)
			}
		}
	}
	return dist
}

// checkPlaces returns a problem when a place's block is not walkable or
// not reached from block start, or "".
func (m *v3Blocks) checkPlaces(start int) string {
	if len(m.places) == 0 {
		return "the map has no places"
	}
	d := m.blockDistances(start)
	for _, p := range m.places {
		bx, by := p.X/2, p.Y/2
		if !m.in(bx, by) || d[by*m.bw+bx] < 0 {
			return "the place " + p.Tags[0] + " at " + strconv.Itoa(p.X) + "," + strconv.Itoa(p.Y) + " is not reached"
		}
	}
	return ""
}

// field returns the map: each block's 2 × 2 cells take its walkability
// and height, and the WMAP has one ground surface per block.
func (m *v3Blocks) field() Field {
	w, h := 2*m.bw, 2*m.bh
	g := Grid{
		Width: w, Height: h, WaterLevel: -16 * rng.One,
		Types: make([]uint8, w*h), Heights: make([]rng.Fixed, 4*w*h),
	}
	for y := range h {
		for x := range w {
			b := (y/2)*m.bw + x/2
			i := y*w + x
			if !m.walk[b] {
				g.Types[i] = CellBlocked
			}
			for k := range 4 {
				g.Heights[4*i+k] = m.height[b]
			}
		}
	}
	f := Field{Grid: g, Places: m.places}
	for _, o := range m.objects {
		f.Objects = append(f.Objects, Object{Prop: o.prop, X: o.hx / 2, Y: o.hy / 2, Turn: o.turn, Scale: o.scale})
	}
	f.WMAP = m.encode(&g)
	return f
}

// v3Lightmap is the lightmap of every dungeon floor and town.
const v3Lightmap = v2Lightmap

// encode writes the map as WMAP:
//
//   - CELL: each cell's flags and heights.
//   - GRND: one surface per block at its height, its texture on top,
//     and a side tile of the wall texture toward a north or east
//     neighbor at another height.
//   - LMAP, LGHT: no baked light, and the tileset's light.
//   - OBJS: the props. SNDS, EFCT: none. DEPS: every name, sorted.
func (m *v3Blocks) encode(g *Grid) []byte {
	ts := m.ts
	size := 1024 + 17*g.Width*g.Height + 28*m.bw*m.bh + 64*len(m.tex) + 128*len(m.objects)
	w := &wmapWriter{b: make([]byte, 0, size)}
	w.b = append(w.b, "WMAP"...)
	w.u16(1)
	w.u16(g.Width)
	w.u16(g.Height)
	w.section("CELL", func(w *wmapWriter) {
		for i, t := range g.Types {
			w.u8(Flags(t))
			for k := range 4 {
				w.fixed(g.Heights[4*i+k])
			}
		}
	})
	// One top tile per texture in use, in the order of the texture list,
	// then one side tile of the wall texture.
	used := make([]bool, len(m.tex))
	for _, t := range m.mat {
		used[t] = true
	}
	wall := m.texture(ts.Wall)
	used[wall] = true
	var textures []string
	texIndex := make([]int, len(m.tex))
	for i, u := range used {
		if u {
			texIndex[i] = len(textures)
			textures = append(textures, TextureName(ts.Name, m.tex[i]))
		}
	}
	topTile := func(t uint8) int { return slices.Index(textures, TextureName(ts.Name, m.tex[t])) }
	side := len(textures)
	w.section("GRND", func(w *wmapWriter) {
		w.u16(m.bw)
		w.u16(m.bh)
		w.u16(len(textures))
		for _, t := range textures {
			w.str(t)
		}
		w.u32(len(textures) + 1)
		one := f32bits(1, 0)
		for t := range textures {
			for _, u := range [8]uint32{0, one, 0, one, one, one, 0, 0} {
				w.bits(u)
			}
			w.u16(t)
			w.u32(0)
			w.b = append(w.b, 255, 255, 255, 255)
		}
		// The side tile: the wall texture, once up the wall.
		for _, u := range [8]uint32{0, one, 0, one, 0, 0, one, one} {
			w.bits(u)
		}
		w.u16(texIndex[wall])
		w.u32(0)
		w.b = append(w.b, 255, 255, 255, 255)
		for j := range m.bh {
			for i := range m.bw {
				b := j*m.bw + i
				for range 4 {
					w.fixed(m.height[b])
				}
				w.i32(int32(topTile(m.mat[b]))) // #nosec G115 -- a few tiles.
				north, east := int32(-1), int32(-1)
				if j+1 < m.bh && m.height[b+m.bw] != m.height[b] {
					north = int32(side) // #nosec G115 -- a few tiles.
				}
				if i+1 < m.bw && m.height[b+1] != m.height[b] {
					east = int32(side) // #nosec G115 -- a few tiles.
				}
				w.i32(north)
				w.i32(east)
			}
		}
	})
	deps := slices.Clone(textures)
	deps = append(deps, v3Lightmap)
	w.section("LMAP", func(w *wmapWriter) {
		w.str(v3Lightmap)
		w.u32(1)
		w.u16(1)
	})
	w.section("LGHT", func(w *wmapWriter) {
		w.i32(45)
		w.i32(45)
		for _, c := range ts.Diffuse {
			w.fixed(c)
		}
		for _, c := range ts.Ambient {
			w.fixed(c)
		}
		w.fixed(rng.One)
		w.fixed(-rng.One / 2)
		w.fixed(46341)
		w.fixed(rng.One / 2)
	})
	w.section("OBJS", func(w *wmapWriter) {
		w.u32(len(m.objects))
		for _, o := range m.objects {
			name := ModelName(ts.Name, o.prop)
			deps = append(deps, name)
			w.str(name)
			s := int64(o.scale)
			cs := v2Turn[o.turn]
			c, sn := s*cs[0], s*cs[1]
			bx, by := min(max(o.hx/4, 0), m.bw-1), min(max(o.hy/4, 0), m.bh-1)
			ground := m.height[by*m.bw+bx]
			zero := uint32(0)
			for _, v := range [16]uint32{
				f32bits(c, 32), zero, f32bits(-sn, 32), zero,
				zero, f32bits(s, 16), zero, zero,
				f32bits(sn, 32), zero, f32bits(c, 32), zero,
				f32bits(int64(o.hx), 1), f32bits(int64(ground), 16), f32bits(-int64(o.hy), 1), f32bits(1, 0),
			} {
				w.bits(v)
			}
			w.i32(0)
			w.bits(f32bits(1, 0))
			w.i32(0)
		}
	})
	w.section("SNDS", func(w *wmapWriter) { w.u32(0) })
	w.section("EFCT", func(w *wmapWriter) { w.u32(0) })
	slices.Sort(deps)
	deps = slices.Compact(deps)
	w.section("DEPS", func(w *wmapWriter) {
		w.u32(len(deps))
		for _, d := range deps {
			w.str(d)
		}
	})
	return w.b
}
