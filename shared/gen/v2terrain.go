//go:build procedural

package gen

import "github.com/lxsmnsyc/wathena/shared/rng"

// v2Envelope pulls vertex heights toward seed vertices whose heights are
// fixed: every vertex stays within slope per cell of each seed's height,
// by chamfer distance (2 along an axis, 3 along a diagonal). Rivers,
// portal areas and roads use it, so their banks are slopes, not cliffs.
type v2Envelope struct {
	lo, hi []int32
}

// envNone marks a bound that no seed set. Heights and cones stay far
// below it.
const envNone = int32(1) << 30

// envelope starts an envelope on the map's buffers.
func (m *v2Map) envelope() *v2Envelope {
	n := len(m.vert)
	m.buf.lo, m.buf.hi = fit(m.buf.lo, n), fit(m.buf.hi, n)
	e := &v2Envelope{lo: m.buf.lo, hi: m.buf.hi}
	for i := range e.lo {
		e.lo[i], e.hi[i] = -envNone, envNone
	}
	return e
}

// seed fixes a vertex's height. A vertex seeded twice keeps the lower.
func (e *v2Envelope) seed(v int, h rng.Fixed) {
	e.hi[v] = min(e.hi[v], int32(h))
	e.lo[v] = e.hi[v]
}

// apply spreads the seeds over the vertex grid in two passes and clamps
// the heights: slope is the most rise per cell along an axis, and 1.5
// times it along a diagonal. Heights above a seed's cone always come
// down. With raise, heights below it come up too.
func (e *v2Envelope) apply(m *v2Map, slope rng.Fixed, raise bool) {
	orth, diag := int32(slope), int32(slope)*3/2
	vw, vh := m.vw, m.h+1
	relax := func(v, u int, w int32) {
		if e.hi[u] != envNone {
			e.hi[v] = min(e.hi[v], e.hi[u]+w)
		}
		if e.lo[u] != -envNone {
			e.lo[v] = max(e.lo[v], e.lo[u]-w)
		}
	}
	for y := range vh {
		for x := range vw {
			v := y*vw + x
			if x > 0 {
				relax(v, v-1, orth)
			}
			if y > 0 {
				relax(v, v-vw, orth)
				if x > 0 {
					relax(v, v-vw-1, diag)
				}
				if x < vw-1 {
					relax(v, v-vw+1, diag)
				}
			}
		}
	}
	for y := vh - 1; y >= 0; y-- {
		for x := vw - 1; x >= 0; x-- {
			v := y*vw + x
			if x < vw-1 {
				relax(v, v+1, orth)
			}
			if y < vh-1 {
				relax(v, v+vw, orth)
				if x < vw-1 {
					relax(v, v+vw+1, diag)
				}
				if x > 0 {
					relax(v, v+vw-1, diag)
				}
			}
		}
	}
	for v, h := range m.vert {
		c := int32(h)
		if e.hi[v] != envNone && c > e.hi[v] {
			c = e.hi[v]
		}
		if raise && e.lo[v] != -envNone && c < e.lo[v] {
			c = e.lo[v]
		}
		m.vert[v] = rng.Fixed(c)
	}
}

// cornerVertices returns the four vertices of a cell.
func (m *v2Map) cornerVertices(i int) [4]int {
	x, y := i%m.w, i/m.w
	v := y*m.vw + x
	return [4]int{v, v + 1, v + m.vw, v + m.vw + 1}
}

// v2Neighbors8 are the 8 neighbor offsets, in a fixed order.
var v2Neighbors8 = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}}

// rivers carves rivers: each starts on a high cell and steps to its
// lowest unvisited neighbor until it reaches water or an edge. Its bed is
// shallow water, with banks that rise at the biome's walkable slope.
func (m *v2Map) rivers(n rng.Node) {
	if m.b.Rivers == 0 {
		return
	}
	s := n.Stream()
	high := m.quantile(800)
	bed := m.level - m.b.Shallow/2
	env := m.envelope()
	carved := false
	m.buf.flag = fit(m.buf.flag, len(m.mean))
	visited := m.buf.flag
	clear(visited)
	for range m.b.Rivers {
		start := -1
		for range 32 {
			i := s.IntN(len(m.mean))
			if m.mean[i] >= high {
				start = i
				break
			}
		}
		if start < 0 {
			continue
		}
		cur := start
		for range 4 * (m.w + m.h) {
			visited[cur] = true
			for _, v := range m.cornerVertices(cur) {
				env.seed(v, bed)
			}
			carved = true
			x, y := cur%m.w, cur/m.w
			if m.mean[cur] < m.level || x == 0 || y == 0 || x == m.w-1 || y == m.h-1 {
				break
			}
			next := -1
			for _, d := range v2Neighbors8 {
				j := (y+d[1])*m.w + x + d[0]
				if !visited[j] && (next < 0 || m.mean[j] < m.mean[next]) {
					next = j
				}
			}
			if next < 0 {
				break
			}
			cur = next
		}
	}
	if carved {
		env.apply(m, m.b.Slope*2/5, false)
	}
}

// placePortals puts one portal on each edge, v2PortalInset cells in, at
// a place from the node's stream, and flattens a circle around it above
// the water.
func (m *v2Map) placePortals(n rng.Node) {
	s := n.Stream()
	env := m.envelope()
	m.portals = m.portals[:0]
	for edge := range 4 {
		long := m.w
		if edge == EdgeEast || edge == EdgeWest {
			long = m.h
		}
		margin := min(long/4, 16)
		t := margin + s.IntN(long-2*margin)
		var p Portal
		switch edge {
		case EdgeSouth:
			p = Portal{Edge: edge, X: t, Y: v2PortalInset}
		case EdgeNorth:
			p = Portal{Edge: edge, X: t, Y: m.h - 1 - v2PortalInset}
		case EdgeWest:
			p = Portal{Edge: edge, X: v2PortalInset, Y: t}
		default:
			p = Portal{Edge: edge, X: m.w - 1 - v2PortalInset, Y: t}
		}
		m.portals = append(m.portals, p)
		h := max(m.mean[p.Y*m.w+p.X], m.level+rng.One/2)
		r := v2PortalRadius
		for dy := -r; dy <= r+1; dy++ {
			for dx := -r; dx <= r+1; dx++ {
				vx, vy := p.X+dx, p.Y+dy
				if vx < 0 || vy < 0 || vx > m.w || vy > m.h || (2*dx-1)*(2*dx-1)+(2*dy-1)*(2*dy-1) > 4*r*r {
					continue
				}
				env.seed(vy*m.vw+vx, h)
			}
		}
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				cx, cy := p.X+dx, p.Y+dy
				if cx < 0 || cy < 0 || cx >= m.w || cy >= m.h || dx*dx+dy*dy > r*r {
					continue
				}
				m.mark[cy*m.w+cx] |= markPortal
			}
		}
	}
	env.apply(m, m.b.Slope*2/5, true)
}
