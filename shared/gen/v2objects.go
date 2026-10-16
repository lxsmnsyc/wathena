//go:build procedural

package gen

import "github.com/lxsmnsyc/wathena/shared/rng"

// Props and ground materials for generator version 2 (strategy section
// 8.5, field steps 7 and 8).

// poisson returns points of the w × h cells at least r cells apart, by
// Bridson's algorithm on integer offsets: each try draws an offset in
// [-2r, 2r] on both axes and keeps it when its length is r to 2r.
func poisson(s *rng.Rand, w, h, r int) [][2]int {
	cs := max(r*7/10, 1)
	gw, gh := (w+cs-1)/cs, (h+cs-1)/cs
	grid := make([]int32, gw*gh)
	for i := range grid {
		grid[i] = -1
	}
	var pts [][2]int
	var active []int
	add := func(x, y int) {
		grid[(y/cs)*gw+x/cs] = int32(len(pts)) // #nosec G115 -- at most one point per cell.
		active = append(active, len(pts))
		pts = append(pts, [2]int{x, y})
	}
	free := func(x, y int) bool {
		gx, gy := x/cs, y/cs
		for j := max(gy-2, 0); j <= min(gy+2, gh-1); j++ {
			for i := max(gx-2, 0); i <= min(gx+2, gw-1); i++ {
				if k := grid[j*gw+i]; k >= 0 {
					dx, dy := pts[k][0]-x, pts[k][1]-y
					if dx*dx+dy*dy < r*r {
						return false
					}
				}
			}
		}
		return true
	}
	add(s.IntN(w), s.IntN(h))
	for len(active) > 0 {
		a := s.IntN(len(active))
		p := pts[active[a]]
		found := false
		for range 20 {
			dx, dy := s.IntN(4*r+1)-2*r, s.IntN(4*r+1)-2*r
			if d := dx*dx + dy*dy; d < r*r || d > 4*r*r {
				continue
			}
			x, y := p[0]+dx, p[1]+dy
			if x < 0 || y < 0 || x >= w || y >= h || !free(x, y) {
				continue
			}
			add(x, y)
			found = true
			break
		}
		if !found {
			active[a] = active[len(active)-1]
			active = active[:len(active)-1]
		}
	}
	return pts
}

// thickets blocks the dry walkable cells where a slow noise is highest,
// the biome's share of the map, outside portal areas. Roads may cut
// through them later.
func (m *v2Map) thickets(n rng.Node) {
	if m.b.Thicket == 0 {
		return
	}
	m.buf.noise = fit(m.buf.noise, len(m.types))
	values := m.buf.noise
	v2Value{n: n, shift: 4}.grid(m.w, m.h, values)
	cut := quantileOf(values, 1000-m.b.Thicket)
	for i, v := range values {
		if v >= cut && m.types[i] == CellWalkable && m.mark[i]&markPortal == 0 {
			m.types[i] = CellBlocked
			m.mark[i] |= markThicket
		}
	}
}

// placeObjects puts props on Poisson-disk points. A point becomes a prop
// when its cell and its 8 neighbors are dry walkable ground, it is not on
// a road, beside one, or in a portal area, and its hash passes the
// density there. Since the 8 neighbors stay walkable around each prop,
// no prop cuts a walkable area in two.
func (m *v2Map) placeObjects(objects, density rng.Node) {
	b := m.b
	var total uint64
	for _, p := range b.Props {
		total += p.Weight
	}
	dens := v2Value{n: density, shift: 5}
	// Thickets first: a prop on every point inside one that stays
	// blocked.
	if b.Thicket != 0 {
		for _, p := range poisson(objects.Child("thickets").Stream(), m.w, m.h, 3) {
			x, y := p[0], p[1]
			i := y*m.w + x
			if m.mark[i]&markThicket == 0 || m.types[i] != CellBlocked {
				continue
			}
			hv := objects.Hash(int64(x), int64(y), 1)
			m.mark[i] |= markObject
			m.objects = append(m.objects, Object{
				Prop: b.ThicketProp, X: x, Y: y,
				Turn:  int(hv >> 32 % 16),                                  // #nosec G115 -- below 16.
				Scale: rng.One*4/5 + rng.Fixed(hv>>40%uint64(rng.One*2/5)), // #nosec G115 -- below 0.4.
			})
		}
	}
	for _, p := range poisson(objects.Stream(), m.w, m.h, b.Spacing) {
		x, y := p[0], p[1]
		i := y*m.w + x
		if !m.clear(x, y) {
			continue
		}
		hv := objects.Hash(int64(x), int64(y), 0)
		chance := b.Density * (int64(rng.One)/2 + dens.at(int64(x), int64(y))) >> 16
		if int64(hv%1000) >= chance { // #nosec G115 -- below 1000.
			continue
		}
		pick := hv >> 10 % total
		prop := b.Props[0].Name
		for _, pr := range b.Props {
			if pick < pr.Weight {
				prop = pr.Name
				break
			}
			pick -= pr.Weight
		}
		m.types[i] = CellBlocked
		m.mark[i] |= markObject
		m.objects = append(m.objects, Object{
			Prop: prop, X: x, Y: y,
			Turn:  int(hv >> 32 % 16),                                  // #nosec G115 -- below 16.
			Scale: rng.One*4/5 + rng.Fixed(hv>>40%uint64(rng.One*2/5)), // #nosec G115 -- below 0.4.
		})
	}
}

// clear reports whether a prop may stand on cell (x, y).
func (m *v2Map) clear(x, y int) bool {
	if x < 1 || y < 1 || x >= m.w-1 || y >= m.h-1 {
		return false
	}
	i := y*m.w + x
	if m.types[i] != CellWalkable || m.mark[i]&(markPortal|markRoad|markRoadSide) != 0 {
		return false
	}
	for _, d := range v2Neighbors8 {
		if m.types[(y+d[1])*m.w+x+d[0]] != CellWalkable {
			return false
		}
	}
	return true
}

// Material ranks: a ground surface of 2 × 2 cells takes the material of
// its highest rank.
const (
	rankGround = iota
	rankPatch
	rankShore
	rankRock
	rankRoad
)

// paint picks each cell's material: roads, then water and its shore,
// then steep rock, then patches over the biome's ground. m.rank holds
// the rule that chose it.
func (m *v2Map) paint(n rng.Node) {
	b := m.b
	m.buf.noise = fit(m.buf.noise, len(m.types))
	patch := m.buf.noise
	v2Value{n: n, shift: 4}.grid(m.w, m.h, patch)
	near := m.nearWater()
	for i := range m.types {
		mat, r := b.Ground, rankGround
		switch {
		case m.mark[i]&markRoad != 0:
			mat, r = b.Road, rankRoad
		case m.mean[i] < m.level:
			mat, r = b.Shore, rankShore
		case m.spread[i] > b.Slope/2:
			mat, r = matRock, rankRock
		case near[i]:
			mat, r = b.Shore, rankShore
		case patch[i] > rng.One*3/5:
			mat, r = b.Patch, rankPatch
		}
		m.material[i] = uint8(mat) // #nosec G115 -- below matCount.
		m.rank[i] = uint8(r)       // #nosec G115 -- small.
	}
}

// nearWater marks the cells within 2 cells of a cell below the water
// level.
func (m *v2Map) nearWater() []bool {
	m.buf.flag = fit(m.buf.flag, len(m.types))
	near := m.buf.flag
	clear(near)
	for i, h := range m.mean {
		if h >= m.level {
			continue
		}
		x, y := i%m.w, i/m.w
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				nx, ny := x+dx, y+dy
				if nx >= 0 && ny >= 0 && nx < m.w && ny < m.h {
					near[ny*m.w+nx] = true
				}
			}
		}
	}
	return near
}
