//go:build procedural

package gen

import (
	"slices"
	"strconv"
	"sync"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Generator version 2 makes fields from biome rules (strategy section
// 8.5, "Field steps", decision 0039). Each step uses its own child of the
// attempt's node, so a change to one step leaves the others alone:
//
//   - "terrain" and "warp": fractal gradient noise with domain warping.
//   - "rivers": rivers from high points down the steepest way.
//   - "portals": one portal area on each edge.
//   - "objects", "density": props by Poisson-disk sampling.
//   - "materials": the patches of the ground.
//
// The water level comes from the heights: the biome sets the share of
// cells below it. Every map is validated. One that fails is made again
// from the child attempt(n), up to v2Attempts tries.

const (
	// v2MinSize is the smallest field side.
	v2MinSize = 32
	// v2Attempts is the most tries for one field.
	v2Attempts = 8
	// v2Pocket is the size below which an unreachable walkable area is
	// blocked instead of joined by a road.
	v2Pocket = 200
	// v2MaxRoads is the most roads one attempt carves.
	v2MaxRoads = 24
	// v2PortalRadius is the radius of a portal's flat area, in cells.
	v2PortalRadius = 4
	// v2PortalInset is a portal's distance from its edge, in cells.
	v2PortalInset = 4
)

// Cell marks of v2Map.mark.
const (
	markPortal = 1 << iota
	markRoad
	markRoadSide
	markObject
	markThicket
)

// v2Map is the state of one attempt.
type v2Map struct {
	b *v2Biome
	// w and h are the size in cells. vw is the vertex row length, w + 1.
	w, h, vw int
	// vert holds the heights of (w + 1) × (h + 1) vertices.
	vert []rng.Fixed
	// mean and spread are each cell's corner mean and its highest corner
	// minus its lowest.
	mean, spread []rng.Fixed
	types        []uint8
	mark         []uint8
	level        rng.Fixed
	portals      []Portal
	objects      []Object
	material     []uint8
	rank         []uint8
	roads        int
	// buf holds the slices, which the next attempt and the next field
	// reuse.
	buf *v2Buffers
}

// v2Buffers are the large slices of an attempt. A pool keeps them
// between attempts and fields, so a large field does not allocate a few
// hundred megabytes over its attempts. The WebAssembly module's garbage
// collector does not return memory, so this keeps its heap small.
type v2Buffers struct {
	vert, mean, spread, noise   []rng.Fixed
	types, mark, material, rank []uint8
	flag                        []bool
	label, parent               []int32
	dist                        []uint32
	lo, hi                      []int32
}

var v2Pool = sync.Pool{New: func() any { return new(v2Buffers) }}

// fit returns s with length n, reusing its array when it is large
// enough. The values are not cleared.
func fit[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	return s[:n]
}

// fieldV2 generates a field. It tries the node, then attempt(1) to
// attempt(7), and returns ErrNoField with each attempt's problem when
// all fail.
func fieldV2(node rng.Node, p Params) (Field, error) {
	if err := p.Check(); err != nil {
		return Field{}, err
	}
	if p.Width < v2MinSize || p.Height < v2MinSize {
		return Field{}, ErrFieldSize
	}
	if p.Biome < 0 || p.Biome >= len(v2Biomes) {
		return Field{}, ErrBiome
	}
	problems := ""
	buf, _ := v2Pool.Get().(*v2Buffers)
	defer v2Pool.Put(buf)
	for n := range v2Attempts {
		a := node
		if n > 0 {
			a = node.Child("attempt", int64(n))
		}
		m := newV2Map(&v2Biomes[p.Biome], p.Width, p.Height, buf)
		m.generate(a)
		problem := m.validate()
		if problem == "" {
			f := m.field()
			f.Biome, f.Attempt = p.Biome, n
			f.WMAP = m.encode()
			return f, nil
		}
		if problems != "" {
			problems += "; "
		}
		problems += "attempt " + strconv.Itoa(n) + ": " + problem
	}
	return Field{}, wrapError(ErrNoField, problems)
}

// gridV2 is the grid of fieldV2.
func gridV2(node rng.Node, p Params) (Grid, error) {
	f, err := fieldV2(node, p)
	return f.Grid, err
}

// fieldError is an error with details after a sentinel error.
type fieldError struct {
	err    error
	detail string
}

func (e *fieldError) Error() string { return e.err.Error() + ": " + e.detail }
func (e *fieldError) Unwrap() error { return e.err }

func wrapError(err error, detail string) error { return &fieldError{err, detail} }

// newV2Map starts an attempt on the slices of buf. Every slice but mark
// is written in full before it is read, so only mark is cleared.
func newV2Map(b *v2Biome, w, h int, buf *v2Buffers) *v2Map {
	n := w * h
	buf.vert = fit(buf.vert, (w+1)*(h+1))
	buf.mean, buf.spread = fit(buf.mean, n), fit(buf.spread, n)
	buf.types, buf.mark = fit(buf.types, n), fit(buf.mark, n)
	buf.material, buf.rank = fit(buf.material, n), fit(buf.rank, n)
	clear(buf.mark)
	return &v2Map{
		b: b, w: w, h: h, vw: w + 1,
		vert: buf.vert, mean: buf.mean, spread: buf.spread,
		types: buf.types, mark: buf.mark, material: buf.material, rank: buf.rank,
		buf: buf,
	}
}

// generate runs the field steps.
func (m *v2Map) generate(a rng.Node) {
	m.heights(a.Child("terrain"), a.Child("warp"))
	m.cells()
	m.level = m.quantile(m.b.WaterShare)
	if m.b.WaterShare == 0 {
		// No water: the level sits below every cell.
		m.level = m.quantile(0) - 4*rng.One
	}
	m.rivers(a.Child("rivers"))
	m.placePortals(a.Child("portals"))
	m.cells()
	m.classify()
	m.thickets(a.Child("thickets"))
	m.connect()
	m.placeObjects(a.Child("objects"), a.Child("density"))
	m.paint(a.Child("materials"))
}

// field returns the attempt's result.
func (m *v2Map) field() Field {
	g := Grid{
		Width: m.w, Height: m.h, WaterLevel: m.level,
		Types:   slices.Clone(m.types),
		Heights: make([]rng.Fixed, 4*m.w*m.h),
	}
	for y := range m.h {
		for x := range m.w {
			i := y*m.w + x
			v := y*m.vw + x
			g.Heights[4*i] = m.vert[v]
			g.Heights[4*i+1] = m.vert[v+1]
			g.Heights[4*i+2] = m.vert[v+m.vw]
			g.Heights[4*i+3] = m.vert[v+m.vw+1]
		}
	}
	return Field{Grid: g, Portals: m.portals, Objects: m.objects}
}

// validate returns the attempt's first problem, or "" (strategy section
// 8.5, "Validation and retries"):
//
//   - Every portal reaches every other one.
//   - The walkable share, water included, is in the biome's range.
func (m *v2Map) validate() string {
	label, _ := m.components()
	first := label[m.portals[0].Y*m.w+m.portals[0].X]
	if first < 0 {
		return "a portal is not walkable"
	}
	for _, p := range m.portals[1:] {
		if label[p.Y*m.w+p.X] != first {
			return "the portal on edge " + strconv.Itoa(p.Edge) + " is not connected"
		}
	}
	walk := 0
	for _, t := range m.types {
		if t == CellWalkable || t == CellWater {
			walk++
		}
	}
	share := walk * 1000 / len(m.types)
	if share < m.b.MinWalk || share > m.b.MaxWalk {
		return "the walkable share is " + strconv.Itoa(share) + " in 1000, not " +
			strconv.Itoa(m.b.MinWalk) + " to " + strconv.Itoa(m.b.MaxWalk)
	}
	return ""
}

// heights fills the vertex heights: fractal gradient noise sampled at
// warped points.
func (m *v2Map) heights(terrain, warp rng.Node) {
	b := m.b
	pad := int64(2*b.Warp>>16) + 2
	minX, minY := -pad, -pad
	maxX, maxY := int64(m.w)+pad, int64(m.h)+pad
	// The warp: two fields of two coarse octaves each.
	var warps [2][2]v2Lattice
	for f := range 2 {
		for o := range 2 {
			warps[f][o] = newV2Lattice(warp, int64(2*f+o), b.Shift-uint(o), minX, minY, maxX, maxY)
		}
	}
	octaves := make([]v2Lattice, b.Octaves)
	for o := range octaves {
		octaves[o] = newV2Lattice(terrain, int64(o), max(b.Shift-uint(o), 1), minX, minY, maxX, maxY)
	}
	limit := 2 * int64(b.Warp)
	var edge int
	if b.Ramp != 0 {
		edge = terrain.Stream().IntN(4)
	}
	basin := -int64(b.Amp) / 4
	for y := 0; y <= m.h; y++ {
		for x := 0; x <= m.w; x++ {
			px, py := int64(x)<<16, int64(y)<<16
			var off [2]int64
			for f := range 2 {
				s := warps[f][0].sample(px, py) + warps[f][1].sample(px, py)/2
				off[f] = min(max(s*int64(b.Warp)>>16, -limit), limit)
			}
			sx, sy := px+off[0], py+off[1]
			var h int64
			amp := int64(b.Amp)
			for o := range octaves {
				n := octaves[o].sample(sx, sy)
				if b.Ridged {
					if n < 0 {
						n = -n
					}
					n = int64(rng.One) - 2*n
				}
				h += n * amp >> 16
				amp /= 2
			}
			if b.Terrace != 0 {
				h = terrace(h, int64(b.Terrace))
			}
			if b.Basin && h < basin {
				h = basin + (h-basin)/8
			}
			if b.Ramp != 0 {
				h += m.ramp(edge, x, y)
			}
			m.vert[y*m.vw+x] = rng.Fixed(h) // #nosec G115 -- heights stay within a few hundred cells.
		}
	}
}

// terrace turns a height into steps of the given height. Within a step
// the height stays flat, then rises over the middle eighth of the step.
func terrace(h, step int64) int64 {
	k := h / step
	if h < 0 && h%step != 0 {
		k--
	}
	f := (h - k*step) * int64(rng.One) / step
	f = min(max((f-int64(rng.One)/2)*8+int64(rng.One)/2, 0), int64(rng.One))
	return k*step + f*step>>16
}

// ramp is a beach's slope: it rises from -Ramp at one edge to +Ramp at
// the far one.
func (m *v2Map) ramp(edge, x, y int) int64 {
	var d, size int
	switch edge {
	case EdgeSouth:
		d, size = y, m.h
	case EdgeNorth:
		d, size = m.h-y, m.h
	case EdgeWest:
		d, size = x, m.w
	default:
		d, size = m.w-x, m.w
	}
	return (2*int64(d)*int64(rng.One)/int64(size) - int64(rng.One)) * int64(m.b.Ramp) >> 16
}

// cells computes each cell's mean and spread from its corners.
func (m *v2Map) cells() {
	for y := range m.h {
		for x := range m.w {
			v := y*m.vw + x
			c := [4]rng.Fixed{m.vert[v], m.vert[v+1], m.vert[v+m.vw], m.vert[v+m.vw+1]}
			lo, hi := c[0], c[0]
			var sum int64
			for _, h := range c {
				lo, hi = min(lo, h), max(hi, h)
				sum += int64(h)
			}
			i := y*m.w + x
			m.mean[i] = rng.Fixed(sum >> 2) // #nosec G115 -- the mean of four Fixed values.
			m.spread[i] = hi - lo
		}
	}
}

// quantile returns the cell mean height below which share in 1000 cells
// lie, rounded up to 1/64 cell.
func (m *v2Map) quantile(share int) rng.Fixed { return quantileOf(m.mean, share) }

// quantileOf returns the value below which share in 1000 values lie,
// rounded up to 1/64.
func quantileOf(values []rng.Fixed, share int) rng.Fixed {
	lo, hi := values[0], values[0]
	for _, h := range values {
		lo, hi = min(lo, h), max(hi, h)
	}
	shift := uint(10)
	for int64(hi-lo)>>shift >= 1<<16 {
		shift++
	}
	count := make([]int, int64(hi-lo)>>shift+1)
	for _, h := range values {
		count[int64(h-lo)>>shift]++
	}
	want := len(values) * share / 1000
	sum := 0
	for i, c := range count {
		if sum+c > want {
			return lo + rng.Fixed(int64(i)<<shift) // #nosec G115 -- within the heights' range.
		}
		sum += c
	}
	return hi + 1
}

// classify sets each cell's type from its heights: deep water is
// blocked, shallow water walkable water, steep cells cliffs. Portal and
// road cells stay walkable.
func (m *v2Map) classify() {
	b := m.b
	for i := range m.types {
		switch {
		case m.mark[i]&(markPortal|markRoad) != 0 && m.mean[i] >= m.level:
			m.types[i] = CellWalkable
		case m.mark[i]&(markPortal|markRoad) != 0:
			m.types[i] = CellWater
		case m.mark[i]&markThicket != 0:
			m.types[i] = CellBlocked
		case m.mean[i] < m.level-b.Shallow:
			m.types[i] = CellBlocked
		case m.mean[i] < m.level:
			m.types[i] = CellWater
		case m.spread[i] > b.Slope:
			m.types[i] = b.Cliff
		default:
			m.types[i] = CellWalkable
		}
	}
}
