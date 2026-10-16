//go:build procedural

package gen

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Towns of generator version 3 (strategy section 8.5, "Towns"). Each
// attempt runs from its node a:
//
//   - "roads": the gates, the main roads from each gate to the central
//     square, and the side roads that branch from them.
//   - "buildings": the lots of the template's buildings, in order.
//   - "decorations": lamps along the main roads, the template's
//     decorations on empty lots, and the trees outside.
//   - "heights": the rough ground outside the walls.
//
// The town stands on a flat plateau. Outside its wall, or its hedge when
// it has none, the ground is rough and blocked, but for the roads from
// the gates to the map's edges.

// Block kinds of a town.
const (
	tLot = iota
	tRoad
	tSide
	tSquare
	tWall
	tRough
	tBuilding
	tDecor
)

// v3TownMargin is the width of the ground outside the wall, in blocks.
const v3TownMargin = 4

// v3Town is the state of one attempt at a town.
type v3Town struct {
	*v3Blocks
	t    TownTemplate
	kind []uint8
	// c is the middle block, rs the square's half side, lo and hi the
	// first and last blocks inside the wall, wm the main roads' width.
	c, rs, lo, hi, wm int
	// mainRoads are the blocks of each main road inside the wall, from
	// the gate inward, with their direction.
	mainRoads [][][3]int
	problem   string
}

// townV3 makes a town.
func townV3(node rng.Node, p TownParams) (Field, error) {
	t := p.Template
	if err := t.Check(); err != nil {
		return Field{}, wrapError(ErrParams, err.Error())
	}
	ts := v3Set(KindTown, t.Tileset)
	if ts == nil {
		return Field{}, wrapError(ErrTileset, "no town tileset "+strconv.Quote(t.Tileset))
	}
	for _, d := range t.Decorations {
		if !ts.hasProp(d) {
			return Field{}, wrapError(ErrTileset, "the tileset "+ts.Name+" has no prop "+strconv.Quote(d))
		}
	}
	for _, b := range t.Buildings {
		if ts.hasProp(b.Model) {
			return Field{}, wrapError(ErrTileset, "the building model "+strconv.Quote(b.Model)+" is a prop of the tileset "+ts.Name)
		}
	}
	return attempts(node, func(a rng.Node) (*v3Blocks, string) {
		b := t.Size / 2
		m := &v3Town{v3Blocks: newV3Blocks(ts, b, b), t: t, kind: make([]uint8, b*b)}
		m.layout(a.Child("roads"))
		m.buildings(a.Child("buildings"))
		if m.problem != "" {
			return nil, m.problem
		}
		m.decorate(a.Child("decorations"))
		m.paint(a.Child("heights"))
		return m.v3Blocks, m.validate()
	})
}

// walkKind reports whether a block kind is walkable.
func walkKind(k uint8) bool { return k == tLot || k == tRoad || k == tSide || k == tSquare }

// streetKind reports whether a block kind is a street a door can face.
func streetKind(k uint8) bool { return k == tRoad || k == tSide || k == tSquare }

// at returns the block of local coordinates (u, v) of a side: u runs
// along the side, v inward from the map's edge.
func (m *v3Town) at(edge, u, v int) (int, int) {
	switch edge {
	case EdgeSouth:
		return u, v
	case EdgeNorth:
		return u, m.bh - 1 - v
	case EdgeWest:
		return v, u
	}
	return m.bw - 1 - v, u
}

// cellAt is at for cells.
func (m *v3Town) cellAt(edge, u, v int) (int, int) {
	w := 2 * m.bw
	switch edge {
	case EdgeSouth:
		return u, v
	case EdgeNorth:
		return u, w - 1 - v
	case EdgeWest:
		return v, u
	}
	return w - 1 - v, u
}

// halfAt is at for points in half cells: (hu, hv) is a point, not a
// cell, so a mirror takes it from the far edge.
func (m *v3Town) halfAt(edge, hu, hv int) (int, int) {
	switch edge {
	case EdgeSouth:
		return hu, hv
	case EdgeNorth:
		return hu, 4*m.bh - hv
	case EdgeWest:
		return hv, hu
	}
	return 4*m.bw - hv, hu
}

// set changes a block's kind.
func (m *v3Town) set(x, y int, k uint8) {
	if m.in(x, y) {
		m.kind[y*m.bw+x] = k
	}
}

// layout marks the wall or hedge, the square with its fountain, the
// gates with their main roads, and the side roads.
func (m *v3Town) layout(n rng.Node) {
	s := n.Stream()
	b := m.bw
	ring := v3TownMargin
	m.lo, m.hi = ring+1, b-ring-2
	m.c = b / 2
	m.rs = max(2, b/14)
	m.wm = 1
	if b >= 64 {
		m.wm = 2
	}
	for y := range b {
		for x := range b {
			k := uint8(tLot)
			switch {
			case x < ring || y < ring || x > b-1-ring || y > b-1-ring:
				k = tRough
			case x == ring || y == ring || x == b-1-ring || y == b-1-ring:
				k = tRough
				if m.t.Walls {
					k = tWall
				}
			}
			m.kind[y*b+x] = k
		}
	}
	for y := m.c - m.rs; y <= m.c+m.rs; y++ {
		for x := m.c - m.rs; x <= m.c+m.rs; x++ {
			m.set(x, y, tSquare)
		}
	}
	// The fountain fills the 2 × 2 blocks at the middle. The square's
	// place is the cell south of it.
	for y := m.c - 1; y <= m.c; y++ {
		for x := m.c - 1; x <= m.c; x++ {
			m.set(x, y, tDecor)
		}
	}
	if m.ts.hasProp("fountain") {
		m.objects = append(m.objects, v3Object{prop: "fountain", hx: 4 * m.c, hy: 4 * m.c, scale: rng.One})
	}
	m.places = append(m.places, Place{Tags: []string{"square"}, X: 2 * m.c, Y: 2*m.c - 3})
	// The gates' sides.
	var edges []int
	switch m.t.Gates {
	case 4:
		edges = []int{EdgeSouth, EdgeEast, EdgeNorth, EdgeWest}
	case 3:
		skip := s.IntN(4)
		for e := range 4 {
			if e != skip {
				edges = append(edges, e)
			}
		}
	default:
		if s.IntN(2) == 0 {
			edges = []int{EdgeSouth, EdgeNorth}
		} else {
			edges = []int{EdgeEast, EdgeWest}
		}
	}
	q := max(1, (m.hi-m.lo)/6)
	// Side roads leave room for two buildings back to back: the mean
	// depth of the buildings, by count, rounded up.
	sum, count := 0, 0
	for _, bd := range m.t.Buildings {
		sum += bd.Count * max(bd.Width, bd.Depth) / 2
		count += bd.Count
	}
	depth := (sum + count - 1) / count
	for _, e := range edges {
		t := min(max(m.c+int(s.Range(int64(-q), int64(q))), m.lo+2), m.hi-2-m.wm)
		m.mainRoad(s, e, t)
	}
	m.sideRoads(s, 2*depth+3)
	if m.t.Walls && m.ts.hasProp("tower") {
		for _, c := range [4][2]int{{ring, ring}, {b - 1 - ring, ring}, {ring, b - 1 - ring}, {b - 1 - ring, b - 1 - ring}} {
			m.objects = append(m.objects, v3Object{prop: "tower", hx: 4*c[0] + 2, hy: 4*c[1] + 2, scale: rng.One})
		}
	}
}

// mainRoad runs a main road from the map's edge on a side, through the
// gate at lateral block t, to the square: inward, across to the square's
// columns at a row the stream picks, and inward again. It records the
// gate's places.
func (m *v3Town) mainRoad(s *rng.Rand, edge, t int) {
	ring := v3TownMargin
	// The square's near side, in blocks from this edge.
	near := m.c - m.rs
	if edge == EdgeNorth || edge == EdgeEast {
		near = m.bw - 1 - (m.c + m.rs)
	}
	target := min(max(t, m.c-m.rs), m.c+m.rs-m.wm+1)
	vt := near
	if target != t {
		vt = m.lo + 1 + s.IntN(max(1, near-m.lo-1-m.wm))
	}
	var road [][3]int
	mark := func(u, v int) {
		for k := range m.wm {
			x, y := m.at(edge, u+k, v)
			if m.kind[y*m.bw+x] != tSquare && m.kind[y*m.bw+x] != tDecor {
				m.set(x, y, tRoad)
			}
		}
	}
	step := func(u, v, du, dv int) {
		x, y := m.at(edge, u, v)
		nx, ny := m.at(edge, u+du, v+dv)
		if v > ring {
			road = append(road, [3]int{x, y, (nx-x+1)*3 + (ny - y + 1)})
		}
	}
	for v := range vt {
		mark(t, v)
		step(t, v, 0, 1)
	}
	for u := t; u != target; u += sign(target - t) {
		for k := range m.wm {
			mark(u, vt+k)
		}
		step(u, vt, sign(target-t), 0)
	}
	for v := vt; v < near; v++ {
		mark(target, v)
		step(target, v, 0, 1)
	}
	m.mainRoads = append(m.mainRoads, road)
	// The gate's places: the portal near the map's edge, and the gate
	// just inside the wall. The gate's arch stands in the wall.
	lateral := 2*t + m.wm
	gx, gy := m.cellAt(edge, lateral, 2*(ring+1)+1)
	px, py := m.cellAt(edge, lateral, 3)
	m.places = append(m.places,
		Place{Tags: []string{"gate", "gate:" + EdgeNames[edge]}, X: gx, Y: gy},
		Place{Tags: []string{"portal", "portal:" + EdgeNames[edge]}, X: px, Y: py})
	if m.t.Walls && m.ts.hasProp("gate") {
		hx, hy := m.halfAt(edge, 2*lateral, 4*ring+2)
		m.objects = append(m.objects, v3Object{prop: "gate", hx: hx, hy: hy, turn: [4]int{0, 4, 8, 12}[edge], scale: rng.One})
	}
}

// sideRoads branches side roads from the main roads every k or k + 1
// blocks, on both sides, across lots until they meet anything else.
func (m *v3Town) sideRoads(s *rng.Rand, k int) {
	for _, road := range m.mainRoads {
		next := k/2 + s.IntN(2)
		for i, r := range road {
			if i < next || i == 0 || i == len(road)-1 || road[i-1][2] != r[2] || road[i+1][2] != r[2] {
				continue
			}
			next = i + k + s.IntN(2)
			dx, dy := r[2]/3-1, r[2]%3-1
			for _, side := range [2][2]int{{dy, -dx}, {-dy, dx}} {
				x, y := r[0], r[1]
				for m.in(x, y) && m.kind[y*m.bw+x] == tRoad {
					x, y = x+side[0], y+side[1]
				}
				for x >= m.lo && y >= m.lo && x <= m.hi && y <= m.hi && m.kind[y*m.bw+x] == tLot {
					m.kind[y*m.bw+x] = tSide
					x, y = x+side[0], y+side[1]
				}
			}
		}
	}
}

// v3Lot is a place for a building: its blocks, the side its door faces
// and its distance from the square.
type v3Lot struct {
	x, y, w, h, door, score, order int
}

// prefix returns the 2D prefix sums of blocks that pass test.
func (m *v3Town) prefix(test func(k uint8) bool) []int32 {
	w := m.bw + 1
	p := make([]int32, w*(m.bh+1))
	for y := range m.bh {
		for x := range m.bw {
			v := int32(0)
			if test(m.kind[y*m.bw+x]) {
				v = 1
			}
			p[(y+1)*w+x+1] = v + p[y*w+x+1] + p[(y+1)*w+x] - p[y*w+x]
		}
	}
	return p
}

// count returns how many blocks of rectangle (x, y, w, h) a prefix
// counted. Blocks off the map do not count.
func (m *v3Town) count(p []int32, x, y, w, h int) int {
	x0, y0 := max(x, 0), max(y, 0)
	x1, y1 := min(x+w, m.bw), min(y+h, m.bh)
	if x1 <= x0 || y1 <= y0 {
		return 0
	}
	pw := m.bw + 1
	return int(p[y1*pw+x1] - p[y0*pw+x1] - p[y1*pw+x0] + p[y0*pw+x0])
}

// lots lists the lots of a footprint of fw × fd blocks: inside the wall,
// on empty lots, a block away from other buildings, with the whole door
// side on a street.
func (m *v3Town) lots(fw, fd int) []v3Lot {
	empty := m.prefix(func(k uint8) bool { return k != tLot })
	built := m.prefix(func(k uint8) bool { return k == tBuilding })
	street := m.prefix(streetKind)
	var out []v3Lot
	for door := range 4 {
		w, h := fw, fd
		if door == EdgeEast || door == EdgeWest {
			w, h = fd, fw
		}
		for y := m.lo; y+h-1 <= m.hi; y++ {
			for x := m.lo; x+w-1 <= m.hi; x++ {
				if m.count(empty, x, y, w, h) != 0 || m.count(built, x-1, y-1, w+2, h+2) != 0 {
					continue
				}
				var sx, sy, sw, sh int
				switch door {
				case EdgeSouth:
					sx, sy, sw, sh = x, y-1, w, 1
				case EdgeNorth:
					sx, sy, sw, sh = x, y+h, w, 1
				case EdgeWest:
					sx, sy, sw, sh = x-1, y, 1, h
				default:
					sx, sy, sw, sh = x+w, y, 1, h
				}
				if m.count(street, sx, sy, sw, sh) != sw*sh {
					continue
				}
				score := abs(sx+sw/2-m.c) + abs(sy+sh/2-m.c)
				out = append(out, v3Lot{x: x, y: y, w: w, h: h, door: door, score: score, order: len(out)})
			}
		}
	}
	return out
}

// buildings places the template's buildings in order. A building with
// places goes on one of the 6 lots nearest the square, picked by the
// stream, and must be placed. Others go on any lot, while lots remain.
func (m *v3Town) buildings(n rng.Node) {
	s := n.Stream()
	for _, bd := range m.t.Buildings {
		for range bd.Count {
			lots := m.lots(bd.Width/2, bd.Depth/2)
			if len(lots) == 0 {
				if len(bd.Places) > 0 {
					m.problem = "no lot for the building " + bd.Model
					return
				}
				break
			}
			var lot v3Lot
			if len(bd.Places) > 0 {
				slices.SortFunc(lots, func(a, b v3Lot) int { return cmp.Or(cmp.Compare(a.score, b.score), cmp.Compare(a.order, b.order)) })
				lot = lots[s.IntN(min(6, len(lots)))]
			} else {
				lot = lots[s.IntN(len(lots))]
			}
			m.build(bd, lot)
		}
	}
}

// build puts a building on a lot, with its places on the street at its
// door, one every 2 cells from the door's middle outward.
func (m *v3Town) build(bd Building, l v3Lot) {
	for y := l.y; y < l.y+l.h; y++ {
		for x := l.x; x < l.x+l.w; x++ {
			m.kind[y*m.bw+x] = tBuilding
		}
	}
	m.objects = append(m.objects, v3Object{
		prop: bd.Model, hx: 4*l.x + 2*l.w, hy: 4*l.y + 2*l.h,
		turn: [4]int{0, 4, 8, 12}[l.door], scale: rng.One,
	})
	half := bd.Width / 2
	for k, tag := range bd.Places {
		off := (k + 1) / 2 * 2
		if k%2 == 1 {
			off = -off
		}
		var x, y int
		switch l.door {
		case EdgeSouth:
			x, y = 2*l.x+half+off, 2*l.y-1
		case EdgeNorth:
			x, y = 2*l.x+half+off, 2*(l.y+l.h)
		case EdgeWest:
			x, y = 2*l.x-1, 2*l.y+half+off
		default:
			x, y = 2*(l.x+l.w), 2*l.y+half+off
		}
		m.places = append(m.places, Place{Tags: []string{tag}, X: x, Y: y})
	}
}

// decorate puts lamps beside the main roads every 4 blocks, the
// template's decorations on half of the Poisson-disk points of empty
// lots, 4 blocks apart, and trees
// outside the wall. A lamp or decoration blocks its block and keeps its 8
// neighbors walkable, so it never splits a walkable area.
func (m *v3Town) decorate(n rng.Node) {
	open := func(x, y int) bool {
		if x < m.lo || y < m.lo || x > m.hi || y > m.hi || m.kind[y*m.bw+x] != tLot {
			return false
		}
		for _, d := range v2Neighbors8 {
			if !walkKind(m.kind[(y+d[1])*m.bw+x+d[0]]) {
				return false
			}
		}
		return true
	}
	if m.ts.hasProp("lamp") {
		for _, road := range m.mainRoads {
			for i := 2; i < len(road); i += 4 {
				r := road[i]
				dx, dy := r[2]/3-1, r[2]%3-1
				side := [2]int{dy, -dx}
				if i/4%2 == 1 {
					side = [2]int{-dy, dx}
				}
				x, y := r[0], r[1]
				for m.in(x, y) && m.kind[y*m.bw+x] == tRoad {
					x, y = x+side[0], y+side[1]
				}
				if open(x, y) {
					m.kind[y*m.bw+x] = tDecor
					m.objects = append(m.objects, v3Object{prop: "lamp", hx: 4*x + 2, hy: 4*y + 2, scale: rng.One})
				}
			}
		}
	}
	if len(m.t.Decorations) > 0 {
		for _, p := range poisson(n.Stream(), m.bw, m.bh, 4) {
			x, y := p[0], p[1]
			hv := n.Hash(int64(x), int64(y), 0)
			if hv%2 == 0 || !open(x, y) {
				continue
			}
			prop := m.t.Decorations[hv>>8%uint64(len(m.t.Decorations))]
			m.kind[y*m.bw+x] = tDecor
			m.propAt(prop, x, y, hv, true)
		}
	}
	if m.ts.hasProp("tree") {
		for _, p := range poisson(n.Child("outside").Stream(), m.bw, m.bh, 2) {
			x, y := p[0], p[1]
			if m.kind[y*m.bw+x] == tRough {
				m.propAt("tree", x, y, n.Hash(int64(x), int64(y), 1), true)
			}
		}
	}
}

// paint sets each block's walkability, texture and height. The plateau
// is flat at 0, walls stand WallHeight high, and the rough ground rolls
// by up to 3/4 of a cell, in steps of 1/8.
func (m *v3Town) paint(n rng.Node) {
	ts := m.ts
	roll := v2Value{n: n, shift: 3}
	tex := [8]uint8{
		tLot: m.texture(ts.Lot), tRoad: m.texture(ts.Road), tSide: m.texture(ts.Side), tSquare: m.texture(ts.Square),
		tWall: m.texture(ts.Wall), tRough: m.texture(ts.Out), tBuilding: m.texture(ts.Side), tDecor: m.texture(ts.Lot),
	}
	for y := range m.bh {
		for x := range m.bw {
			i := y*m.bw + x
			k := m.kind[i]
			m.walk[i] = walkKind(k)
			m.mat[i] = tex[k]
			switch k {
			case tWall:
				m.height[i] = ts.WallHeight
			case tRough:
				m.height[i] = rng.Fixed(roll.at(int64(x), int64(y))*3/4) &^ (rng.One/8 - 1) // #nosec G115 -- below one cell.
			case tDecor:
				if x >= m.c-m.rs && x <= m.c+m.rs && y >= m.c-m.rs && y <= m.c+m.rs {
					m.mat[i] = tex[tSquare]
				}
			}
		}
	}
}

// validate returns the town's first problem, or "":
//
//   - Every place is reached from the square.
//   - At least a quarter of the town is walkable.
func (m *v3Town) validate() string {
	sq := m.places[0]
	if problem := m.checkPlaces((sq.Y/2)*m.bw + sq.X/2); problem != "" {
		return problem
	}
	walk := 0
	for _, w := range m.walk {
		if w {
			walk++
		}
	}
	if share := walk * 1000 / len(m.walk); share < 250 {
		return "the walkable share is " + strconv.Itoa(share) + " in 1000, below 250"
	}
	return ""
}
