//go:build procedural

package gen

import "github.com/lxsmnsyc/wathena/shared/rng"

// Connectivity for generator version 2 (strategy section 8.5, field
// step 5). Walkable cells, water included, connect along the 4 axes.

// walkableType reports whether a cell type can be walked on.
func walkableType(t uint8) bool { return t == CellWalkable || t == CellWater }

// components labels the walkable areas, from 0 in scan order, and -1 for
// other cells. first holds each area's first cell.
func (m *v2Map) components() (label []int32, first []int) {
	m.buf.label = fit(m.buf.label, len(m.types))
	label = m.buf.label
	for i := range label {
		label[i] = -1
	}
	queue := make([]int32, 0, 1024)
	for i, t := range m.types {
		if label[i] >= 0 || !walkableType(t) {
			continue
		}
		id := int32(len(first)) // #nosec G115 -- at most 1024 × 1024 areas.
		first = append(first, i)
		label[i] = id
		queue = append(queue[:0], int32(i)) // #nosec G115 -- cells fit in int32.
		for len(queue) > 0 {
			c := int(queue[len(queue)-1])
			queue = queue[:len(queue)-1]
			x, y := c%m.w, c/m.w
			for _, d := range v2Neighbors8[:4] {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= m.w || ny >= m.h {
					continue
				}
				j := ny*m.w + nx
				if label[j] < 0 && walkableType(m.types[j]) {
					label[j] = id
					queue = append(queue, int32(j)) // #nosec G115 -- cells fit in int32.
				}
			}
		}
	}
	return label, first
}

// connect joins every portal and every walkable area of at least
// v2Pocket cells to the first portal's area with roads, then blocks the
// walkable cells that still do not reach it.
func (m *v2Map) connect() {
	p0 := m.portals[0].Y*m.w + m.portals[0].X
	for range 8 {
		label, first := m.components()
		main := label[p0]
		sizes := make([]int, len(first))
		for _, l := range label {
			if l >= 0 {
				sizes[l]++
			}
		}
		var starts []int
		for _, p := range m.portals[1:] {
			if c := p.Y*m.w + p.X; label[c] != main {
				starts = append(starts, c)
			}
		}
		for id, f := range first {
			if int32(id) != main && sizes[id] >= v2Pocket { // #nosec G115 -- as in components.
				starts = append(starts, f)
			}
		}
		if len(starts) == 0 {
			break
		}
		if len(starts) > v2MaxRoads-m.roads {
			starts = starts[:v2MaxRoads-m.roads]
		}
		if len(starts) == 0 || main < 0 {
			break
		}
		env := m.envelope()
		carved := false
		for _, path := range m.roadsTo(starts, label, main) {
			if path == nil {
				continue
			}
			m.roads++
			carved = true
			m.layRoad(path, env)
		}
		if !carved {
			break
		}
		env.apply(m, m.b.Slope*3/5, true)
		m.cells()
		m.classify()
	}
	label, _ := m.components()
	main := label[p0]
	for i, l := range label {
		if l >= 0 && l != main {
			m.types[i] = CellBlocked
		}
	}
}

// roadCost is the cost of a road entering a cell: gentle slopes are
// cheap, cliffs and deep water dear.
func (m *v2Map) roadCost(i int) uint32 {
	b := m.b
	c := uint32(4)
	c += uint32(min(int64(m.spread[i])*16/int64(b.Slope), 64)) // #nosec G115 -- spreads are not negative.
	switch {
	case m.mean[i] < m.level-b.Shallow:
		c += 40
	case m.mean[i] < m.level:
		c += 4
	}
	if m.spread[i] > b.Slope {
		c += 24
	}
	return c
}

// roadsTo finds the cheapest way from each start cell to area main,
// along the 4 axes, with one Dijkstra search that starts from the edge
// of main. Each way lists its cells from the start to the first cell of
// main, or is nil when none exists.
func (m *v2Map) roadsTo(starts []int, label []int32, main int32) [][]int {
	const unseen = ^uint32(0)
	m.buf.dist, m.buf.parent = fit(m.buf.dist, len(m.types)), fit(m.buf.parent, len(m.types))
	dist, parent := m.buf.dist, m.buf.parent
	var h v2Heap
	for i := range dist {
		dist[i] = unseen
		if label[i] != main {
			continue
		}
		dist[i] = 0
		parent[i] = -1
		x, y := i%m.w, i/m.w
		for _, o := range v2Neighbors8[:4] {
			nx, ny := x+o[0], y+o[1]
			if nx >= 0 && ny >= 0 && nx < m.w && ny < m.h && label[ny*m.w+nx] != main {
				h.push(uint64(i)) // #nosec G115 -- cells are not negative.
				break
			}
		}
	}
	waiting := len(starts)
	m.buf.flag = fit(m.buf.flag, len(m.types))
	want := m.buf.flag
	clear(want)
	for _, s := range starts {
		want[s] = true
	}
	for len(h) > 0 && waiting > 0 {
		top := h.pop()
		c := int(top & 0xffffffff)
		d := uint32(top >> 32) // #nosec G115 -- costs fit in 32 bits.
		if d != dist[c] {
			continue
		}
		if want[c] {
			want[c] = false
			waiting--
		}
		x, y := c%m.w, c/m.w
		for _, o := range v2Neighbors8[:4] {
			nx, ny := x+o[0], y+o[1]
			if nx < 0 || ny < 0 || nx >= m.w || ny >= m.h {
				continue
			}
			j := ny*m.w + nx
			if nd := d + m.roadCost(j); nd < dist[j] {
				dist[j] = nd
				parent[j] = int32(c)               // #nosec G115 -- cells fit in int32.
				h.push(uint64(nd)<<32 | uint64(j)) // #nosec G115 -- cells are not negative.
			}
		}
	}
	out := make([][]int, len(starts))
	for k, s := range starts {
		if dist[s] == unseen {
			continue
		}
		for i := s; i >= 0; i = int(parent[i]) {
			out[k] = append(out[k], i)
		}
	}
	return out
}

// layRoad seeds a road's heights: the cells' means, cut down so the road
// rises at most a third of the biome's slope per cell, and kept at least
// half the shallow depth below the water level, as a ford. It marks the
// road and the cells beside it.
func (m *v2Map) layRoad(path []int, env *v2Envelope) {
	b := m.b
	d := b.Slope / 3
	floor := m.level - b.Shallow/2
	h := make([]rng.Fixed, len(path))
	for i, c := range path {
		h[i] = m.mean[c]
		if i > 0 {
			h[i] = min(h[i], h[i-1]+d)
		}
	}
	for i := len(path) - 2; i >= 0; i-- {
		h[i] = min(h[i], h[i+1]+d)
	}
	for i, c := range path {
		for _, v := range m.cornerVertices(c) {
			env.seed(v, max(h[i], floor))
		}
		m.mark[c] |= markRoad
		x, y := c%m.w, c/m.w
		for _, o := range v2Neighbors8 {
			nx, ny := x+o[0], y+o[1]
			if nx >= 0 && ny >= 0 && nx < m.w && ny < m.h {
				m.mark[ny*m.w+nx] |= markRoadSide
			}
		}
	}
}

// v2Heap is a binary min-heap of cost << 32 | cell. Equal costs come out
// by cell, so the search order is fixed.
type v2Heap []uint64

func (h *v2Heap) push(v uint64) {
	*h = append(*h, v)
	s := *h
	i := len(s) - 1
	for i > 0 {
		p := (i - 1) / 2
		if s[p] <= s[i] {
			break
		}
		s[p], s[i] = s[i], s[p]
		i = p
	}
}

func (h *v2Heap) pop() uint64 {
	s := *h
	top := s[0]
	last := len(s) - 1
	s[0] = s[last]
	s = s[:last]
	i := 0
	for {
		l, r, small := 2*i+1, 2*i+2, i
		if l < len(s) && s[l] < s[small] {
			small = l
		}
		if r < len(s) && s[r] < s[small] {
			small = r
		}
		if small == i {
			break
		}
		s[i], s[small] = s[small], s[i]
		i = small
	}
	*h = s
	return top
}
