package zone

import (
	"cmp"
	"slices"

	"github.com/lxsmnsyc/wathena/shared/path"
)

// Entity queries (proposal 0002, review decision 18): find the entities
// of an area, with filters and an order. Skills in Go and scripts share
// them, so a skill's splash and a mod's chain lightning find the same
// targets.
//
// Areas follow rAthena's map_foreach functions:
//   - a radius is map_foreachinrange: the square of cells within the
//     radius on both axes, or with Circle the cells check_distance allows
//     under CIRCULAR_AREA;
//   - a rectangle is map_foreachinarea, corners included;
//   - a line is map_foreachinpath: the units within a width of the line
//     from one cell to the other;
//   - cells is a list of cells, as map_foreachincell for each.
// Line of sight is skill_wall_check's path_search_long with CELL_CHKWALL
// from the area's center.

// areaKind is the shape of a query's area.
type areaKind int

const (
	areaRadius areaKind = iota
	areaRect
	areaLine
	areaCells
)

// Kinds of entity a query returns, as bits.
const (
	queryPlayers = 1 << iota
	queryMonsters
	queryNPCs
)

// query is one entity query.
type query struct {
	area areaKind
	// x and y are the center of a radius, and radius its size. For a
	// rectangle, x0 to y1 are its corners; for a line, its ends.
	x, y, radius   int
	x0, y0, x1, y1 int
	// width is a line's width, and length the length it is cut or
	// stretched to (0 to keep it), as map_foreachinpath's range and
	// length.
	width, length int
	cells         [][2]int
	circle        bool
	// kinds is a set of query kinds, 0 for every kind.
	kinds int
	// dead includes dead units; queries leave them out by default.
	// onlyDead keeps dead units only.
	dead, onlyDead bool
	// enemiesOf and alliesOf keep the enemies or the allies of a unit.
	enemiesOf, alliesOf *entity
	lineOfSight         bool
	exclude             map[*entity]bool
	// sort is "", "nearest", "farthest", "lowest_hp" or "random".
	sort  string
	limit int
}

// center is the cell that line of sight and distances start from.
func (q *query) center() (int, int) {
	switch q.area {
	case areaRect:
		x0, x1 := min(q.x0, q.x1), max(q.x0, q.x1)
		y0, y1 := min(q.y0, q.y1), max(q.y0, q.y1)
		return x0 + (x1-x0)/2, y0 + (y1-y0)/2
	case areaLine:
		return q.x0, q.y0
	case areaCells:
		if len(q.cells) > 0 {
			return q.cells[0][0], q.cells[0][1]
		}
	}
	return q.x, q.y
}

// checkDistance is rAthena's check_distance: the square range, or with
// circle the circle that CIRCULAR_AREA uses, with one cell of grace for
// diagonals.
func checkDistance(dx, dy, r int, circle bool) bool {
	if circle {
		grace := 0
		if dx != 0 && dy != 0 {
			grace = 1
		}
		return dx*dx+dy*dy <= r*r+grace
	}
	return max(abs(dx), abs(dy)) <= r
}

// inArea reports whether a cell is in the query's area.
func (z *Zone) inArea(q *query, x, y int) bool {
	switch q.area {
	case areaRadius:
		return checkDistance(x-q.x, y-q.y, q.radius, q.circle)
	case areaRect:
		x0, x1 := min(q.x0, q.x1), max(q.x0, q.x1)
		y0, y1 := min(q.y0, q.y1), max(q.y0, q.y1)
		return x >= x0 && x <= x1 && y >= y0 && y <= y1
	case areaLine:
		return z.inPath(q, x, y)
	case areaCells:
		for _, c := range q.cells {
			if c[0] == x && c[1] == y {
				return true
			}
		}
	}
	return false
}

// inPath is map_foreachinpath's test of one unit: the projection of its
// cell onto the line falls within the line, and it is within the width
// of the line, in rAthena's int32 math with 4 bits of precision. Units
// past the line's first end need a clear shot.
func (z *Zone) inPath(q *query, xi, yi int) bool {
	x0, y0, x1, y1 := q.x0, q.y0, q.x1, q.y1
	mag := (x1-x0)*(x1-x0) + (y1-y0)*(y1-y0)
	if mag < 1 {
		return false
	}
	limit := mag
	if q.length != 0 {
		k := isqrt(mag)
		mx1 := x0 + (x1-x0)*q.length/k
		my1 := y0 + (y1-y0)*q.length/k
		limit = (mx1-x0)*(mx1-x0) + (my1-y0)*(my1-y0)
	}
	k := (xi-x0)*(x1-x0) + (yi-y0)*(y1-y0)
	if k < 0 || k > limit {
		return false
	}
	if k > mag {
		if ok, _ := path.Shoot(z.grid, x0, y0, xi, yi); !ok {
			return false
		}
	}
	k = (k << 4) / mag
	xu := (x0 << 4) + k*(x1-x0)
	yu := (y0 << 4) + k*(y1-y0)
	dx, dy := (xi<<4)-xu, (yi<<4)-yu
	return dx*dx+dy*dy <= q.width*(q.width<<8)
}

// isqrt is (int32)sqrt((float)n) for the non-negative values of
// map_foreachinpath.
func isqrt(n int) int {
	r := 0
	for (r+1)*(r+1) <= n {
		r++
	}
	return r
}

// queryKind is an entity's query kind, or 0 for one that queries never
// return, such as an item on the ground or a warp.
func queryKind(e *entity) int {
	switch {
	case e.player != nil:
		return queryPlayers
	case e.monster != nil:
		return queryMonsters
	case e.npc != nil:
		return queryNPCs
	}
	return 0
}

// isEnemy is battle_check_target with BCT_ENEMY for the units this server
// has: players and monsters are enemies of each other. Players do not
// fight players outside PvP, which is not ported, and monsters do not
// fight monsters.
func isEnemy(a, b *entity) bool {
	if a == b {
		return false
	}
	// A summoned monster is on its master's side. Players may hit floras
	// and marine spheres as alchemist_summon_setting says, and other
	// monsters treat them as friends (BCT_PARTY). A summon's enemies are
	// its master's: monsters without a master.
	if m := b.monster; m != nil && m.master != nil {
		return a.player != nil && m.playersHit
	}
	if m := a.monster; m != nil && m.master != nil {
		return b.monster != nil && b.monster.master == nil
	}
	return a.player != nil && b.monster != nil || a.monster != nil && b.player != nil
}

// isAlly is battle_check_target with BCT_NOENEMY between units: the unit
// itself, and units of its own side.
func isAlly(a, b *entity) bool {
	if a == b {
		return true
	}
	return a.player != nil && b.player != nil || a.monster != nil && b.monster != nil
}

// findEntities runs a query and returns the entities it finds, in its
// order. Ties sort by entity ID, the order the zone added them in.
func (z *Zone) findEntities(q *query) []*entity {
	cx, cy := q.center()
	var out []*entity
	for e := range z.entities.all() {
		k := queryKind(e)
		if k == 0 || q.kinds != 0 && q.kinds&k == 0 {
			continue
		}
		if !q.dead && (e.dead || e.status == nil && e.npc == nil) || q.onlyDead && !e.dead {
			continue
		}
		if q.exclude[e] {
			continue
		}
		if q.enemiesOf != nil && !isEnemy(q.enemiesOf, e) {
			continue
		}
		if q.alliesOf != nil && !isAlly(q.alliesOf, e) {
			continue
		}
		if !z.inArea(q, e.x, e.y) {
			continue
		}
		if q.lineOfSight {
			if ok, _ := path.Shoot(z.grid, cx, cy, e.x, e.y); !ok {
				continue
			}
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b *entity) int { return cmp.Compare(a.uid, b.uid) })
	dist := func(e *entity) int {
		dx, dy := e.x-cx, e.y-cy
		if q.circle {
			return dx*dx + dy*dy
		}
		return max(abs(dx), abs(dy))
	}
	switch q.sort {
	case "nearest":
		slices.SortStableFunc(out, func(a, b *entity) int { return cmp.Compare(dist(a), dist(b)) })
	case "farthest":
		slices.SortStableFunc(out, func(a, b *entity) int { return cmp.Compare(dist(b), dist(a)) })
	case "lowest_hp":
		slices.SortStableFunc(out, func(a, b *entity) int { return cmp.Compare(a.hp, b.hp) })
	case "random":
		// Fisher-Yates from the zone's secret stream.
		for i := len(out) - 1; i > 0; i-- {
			j := z.rand.IntN(i + 1)
			out[i], out[j] = out[j], out[i]
		}
	}
	if q.limit > 0 && len(out) > q.limit {
		out = out[:q.limit]
	}
	return out
}

// inDir is map_foreachindir for characters: the alive players and
// monsters in a path of the given width (range) and length from a cell,
// in one of the eight directions toward another cell, with a clear shot
// from the source. offset moves the start of the path away from the
// source. Units come in the order the zone added them.
func (z *Zone) inDir(x0, y0, x1, y1, rng, length, offset int) []*entity {
	if rng < 0 || length < 1 || offset < 0 {
		return nil
	}
	dir := mapCalcDir(&entity{x: x0, y: y0}, x1, y1)
	dx, dy := dirX[dir], dirY[dir]
	diagonal := dir%2 == 1
	if offset != 0 && diagonal {
		// Diagonal paths work with half cells so they can join.
		offset = 2*offset - 1
		length++
	}
	step := offset / (dir%2 + 1)
	mx0, my0 := x0+dx*step, y0+dy*step
	mx1, my1 := x0+dx*(step+length-1), y0+dy*(step+length-1)
	if mx0 > mx1 {
		mx0, mx1 = mx1, mx0
	}
	if my0 > my1 {
		my0, my1 = my1, my0
	}
	// The width turns 90 degrees from the direction.
	side := (dir + 2) % 8
	mx0 -= abs(rng * dirX[side])
	my0 -= abs(rng * dirY[side])
	mx1 += abs(rng * dirX[side])
	my1 += abs(rng * dirY[side])
	mx0, my0 = max(mx0, 0), max(my0, 0)
	mx1, my1 = min(mx1, z.grid.Width-1), min(my1, z.grid.Height-1)
	var out []*entity
	for e := range z.entities.all() {
		if (e.player == nil && e.monster == nil) || e.dead || e.status == nil {
			continue
		}
		if e.x < mx0 || e.x > mx1 || e.y < my0 || e.y > my1 {
			continue
		}
		// skill_eightpath_same_cell is on: the source's own cell counts.
		rx, ry := (e.x-x0)*dx, (e.y-y0)*dy
		if diagonal {
			if rx+ry < offset || rx+ry > 2*(length+offset/2-1) {
				continue
			}
			if abs(rx-ry) > 2*rng {
				continue
			}
		}
		if ok, _ := path.Shoot(z.grid, x0, y0, e.x, e.y); !ok {
			continue
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b *entity) int { return cmp.Compare(a.uid, b.uid) })
	return out
}
