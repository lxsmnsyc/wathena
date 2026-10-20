// Package path finds walk paths and checks lines of sight on a map's
// cells, exactly as rAthena's src/map/path.cpp does, so the server and the
// web client agree on every step. The web client runs it compiled to
// WebAssembly with TinyGo.
//
// Coordinates are server cells: x east, y north, from (0, 0) in the
// south-west corner.
//
// This is a port of rAthena's path.cpp (GPL-3.0), including its quirks:
// the last row and column of a map never allow walking, and the A* search
// keeps its nodes in a 32 × 32 table that fails on collisions.
package path

// Cell flags, as WMAP stores them.
const (
	Walkable  = 1
	Shootable = 2
)

// Costs and limits from rAthena's path.hpp.
const (
	MoveCost         = 10
	MoveDiagonalCost = 14
	// MaxWalkPath is the longest path, in steps.
	MaxWalkPath = 32
)

// Dir is a walking direction, as rAthena numbers them: 0 is north, turning
// counterclockwise.
type Dir int8

const (
	North Dir = iota
	NorthWest
	West
	SouthWest
	South
	SouthEast
	East
	NorthEast
)

// Step returns the cell offset of one step in a direction.
func (d Dir) Step() (dx, dy int) {
	switch d {
	case North:
		return 0, 1
	case NorthWest:
		return -1, 1
	case West:
		return -1, 0
	case SouthWest:
		return -1, -1
	case South:
		return 0, -1
	case SouthEast:
		return 1, -1
	case East:
		return 1, 0
	case NorthEast:
		return 1, 1
	}
	return 0, 0
}

// Diagonal reports whether a step in the direction moves on both axes.
func (d Dir) Diagonal() bool {
	return d == NorthWest || d == SouthWest || d == SouthEast || d == NorthEast
}

// walkChoices maps (-dy + 1, dx + 1) to a direction, as rAthena does.
var walkChoices = [3][3]Dir{
	{NorthWest, North, NorthEast},
	{West, -1, East},
	{SouthWest, South, SouthEast},
}

func dirOf(dx, dy int) Dir { return walkChoices[-dy+1][dx+1] }

// Grid is a map's cells.
type Grid struct {
	Width, Height int
	// Cells holds flags, row by row from y = 0.
	Cells []uint8
}

// NoPass reports whether a unit cannot stand on a cell. Cells outside the
// map, and the last row and column, block.
func (g *Grid) NoPass(x, y int) bool {
	if x < 0 || x >= g.Width-1 || y < 0 || y >= g.Height-1 {
		return true
	}
	return g.Cells[x+y*g.Width]&Walkable == 0
}

// wall reports whether a cell stops projectiles. Cells outside the map,
// and the last row and column, do not.
func (g *Grid) wall(x, y int) bool {
	if x < 0 || x >= g.Width-1 || y < 0 || y >= g.Height-1 {
		return false
	}
	return g.Cells[x+y*g.Width]&(Walkable|Shootable) == 0
}

// Search finds a walk path from (x0, y0) to (x1, y1), as rAthena's
// path_search. With easy set, it only tries the direct path, diagonally
// first, that monsters use. It returns the steps, or false when no path
// of at most MaxWalkPath steps exists.
func Search(g *Grid, x0, y0, x1, y1 int, easy bool) ([]Dir, bool) {
	// The start cell is not checked, so a unit on a blocked cell can
	// leave it.
	if x0 < 0 || x0 >= g.Width || y0 < 0 || y0 >= g.Height {
		return nil, false
	}
	if x1 < 0 || x1 >= g.Width || y1 < 0 || y1 >= g.Height || g.NoPass(x1, y1) {
		return nil, false
	}
	if easy {
		return easyPath(g, x0, y0, x1, y1)
	}
	return aStar(g, x0, y0, x1, y1)
}

func sign(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	}
	return 0
}

func easyPath(g *Grid, x0, y0, x1, y1 int) ([]Dir, bool) {
	dx, dy := sign(x1-x0), sign(y1-y0)
	x, y := x0, y0
	path := make([]Dir, 0, MaxWalkPath)
	for len(path) < MaxWalkPath {
		path = append(path, dirOf(dx, dy))
		x += dx
		y += dy
		if x == x1 {
			dx = 0
		}
		if y == y1 {
			dy = 0
		}
		if dx == 0 && dy == 0 {
			break
		}
		if g.NoPass(x, y) {
			break
		}
	}
	if x == x1 && y == y1 {
		return path, true
	}
	return nil, false
}

// node is an A* node. parent is an index into the node table, or -1.
type node struct {
	parent       int
	x, y         int16
	gCost, fCost int16
	closed       bool
	// used marks a slot that holds a node. rAthena counts a slot with
	// (0, 0) as empty instead, so a search that starts on (0, 0) could
	// write another cell over its start node and loop forever on the
	// parents.
	used bool
}

// nodeIndex is rAthena's calc_index: a slot in a table of MaxWalkPath²
// nodes. Two cells can share a slot, which fails the search.
func nodeIndex(x, y int) int {
	return (x + y*MaxWalkPath) & (MaxWalkPath*MaxWalkPath - 1)
}

// heuristic is rAthena's: Manhattan distance, which overestimates, as the
// game client does.
func heuristic(x0, y0, x1, y1 int) int {
	return MoveCost * (abs(x1-x0) + abs(y1-y0))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// search holds one A* run. The open set is a binary heap of node
// indexes, ordered by f cost, with rAthena's BHEAP_PUSH2 and BHEAP_POP2,
// which match the game client's heap and so its tie-breaking.
type search struct {
	nodes [MaxWalkPath * MaxWalkPath]node
	open  []int
}

func (s *search) less(a, b int) int { return int(s.nodes[a].fCost) - int(s.nodes[b].fCost) }

// siftDown is BHEAP_SIFTDOWN: move the item at i up toward start.
func (s *search) siftDown(start, i int) {
	for i > start {
		parent := (i - 1) / 2
		if s.less(s.open[parent], s.open[i]) <= 0 {
			break
		}
		s.open[parent], s.open[i] = s.open[i], s.open[parent]
		i = parent
	}
}

// siftUp is BHEAP_SIFTUP: move the item at start down to a leaf along the
// smaller children, then back up as far as start.
func (s *search) siftUp(start int) {
	i := start
	for child := 2*i + 1; child < len(s.open); child = 2*i + 1 {
		right := child + 1
		if right < len(s.open) && s.less(s.open[child], s.open[right]) >= 0 {
			child = right
		}
		s.open[i], s.open[child] = s.open[child], s.open[i]
		i = child
	}
	s.siftDown(start, i)
}

func (s *search) push(n int) {
	s.open = append(s.open, n)
	s.siftDown(0, len(s.open)-1)
}

func (s *search) pop() int {
	top := s.open[0]
	last := len(s.open) - 1
	s.open[0] = s.open[last]
	s.open = s.open[:last]
	if len(s.open) > 0 {
		s.siftUp(0)
	}
	return top
}

// update is BHEAP_UPDATE, after a node's cost fell.
func (s *search) update(n int) bool {
	for i, v := range s.open {
		if v == n {
			s.siftDown(0, i)
			s.siftUp(i)
			return true
		}
	}
	return false
}

// add is rAthena's add_path. It returns false when the node table has a
// collision, which fails the search.
func (s *search) add(x, y, gCost, parent, hCost int) bool {
	i := nodeIndex(x, y)
	n := &s.nodes[i]
	if int(n.x) == x && int(n.y) == y {
		if gCost < int(n.gCost) {
			n.gCost = int16(gCost) // #nosec G115 -- costs stay far below 2^15 within 32 steps.
			n.parent = parent
			n.fCost = int16(gCost + hCost) // #nosec G115 -- as above.
			if n.closed {
				s.push(i)
			} else if !s.update(i) {
				return false
			}
			n.closed = false
		}
		return true
	}
	// A slot that holds another cell is a collision, which fails the
	// search, as in rAthena. An empty slot matches (0, 0) above, as in
	// rAthena, so only a start node on (0, 0) differs: rAthena would
	// write over it.
	if n.used {
		return false
	}
	*n = node{parent: parent, x: int16(x), y: int16(y), gCost: int16(gCost), fCost: int16(gCost + hCost), used: true} // #nosec G115 -- map sizes and costs fit.
	s.push(i)
	return true
}

func aStar(g *Grid, x0, y0, x1, y1 int) ([]Dir, bool) {
	s := &search{open: make([]int, 0, 256)}
	start := nodeIndex(x0, y0)
	s.nodes[start] = node{parent: -1, x: int16(x0), y: int16(y0), fCost: int16(heuristic(x0, y0, x1, y1)), used: true} // #nosec G115 -- map sizes fit.
	s.push(start)
	xs, ys := g.Width-1, g.Height-1
	var current int
	for {
		if len(s.open) == 0 {
			return nil, false
		}
		current = s.pop()
		n := &s.nodes[current]
		n.closed = true
		x, y, gCost := int(n.x), int(n.y), int(n.gCost)
		if x == x1 && y == y1 {
			break
		}
		const (
			dirNorth = 1
			dirWest  = 2
			dirSouth = 4
			dirEast  = 8
		)
		allowed := 0
		if y < ys && !g.NoPass(x, y+1) {
			allowed |= dirNorth
		}
		if y > 0 && !g.NoPass(x, y-1) {
			allowed |= dirSouth
		}
		if x < xs && !g.NoPass(x+1, y) {
			allowed |= dirEast
		}
		if x > 0 && !g.NoPass(x-1, y) {
			allowed |= dirWest
		}
		can := func(d int) bool { return allowed&d == d }
		ok := true
		try := func(nx, ny, cost int) {
			ok = s.add(nx, ny, gCost+cost, current, heuristic(nx, ny, x1, y1)) && ok
		}
		// Neighbors in rAthena's order. Diagonals need both straight
		// neighbors open, so paths do not cut wall corners.
		if can(dirSouth|dirEast) && !g.NoPass(x+1, y-1) {
			try(x+1, y-1, MoveDiagonalCost)
		}
		if can(dirEast) {
			try(x+1, y, MoveCost)
		}
		if can(dirNorth|dirEast) && !g.NoPass(x+1, y+1) {
			try(x+1, y+1, MoveDiagonalCost)
		}
		if can(dirNorth) {
			try(x, y+1, MoveCost)
		}
		if can(dirNorth|dirWest) && !g.NoPass(x-1, y+1) {
			try(x-1, y+1, MoveDiagonalCost)
		}
		if can(dirWest) {
			try(x-1, y, MoveCost)
		}
		if can(dirSouth|dirWest) && !g.NoPass(x-1, y-1) {
			try(x-1, y-1, MoveDiagonalCost)
		}
		if can(dirSouth) {
			try(x, y-1, MoveCost)
		}
		if !ok {
			return nil, false
		}
	}
	length := 0
	for i := current; s.nodes[i].parent >= 0; i = s.nodes[i].parent {
		length++
	}
	if length > MaxWalkPath {
		return nil, false
	}
	path := make([]Dir, length)
	for i, j := current, length-1; j >= 0; i, j = s.nodes[i].parent, j-1 {
		p := s.nodes[s.nodes[i].parent]
		path[j] = dirOf(int(s.nodes[i].x-p.x), int(s.nodes[i].y-p.y))
	}
	return path, true
}

// Shoot reports whether a projectile can fly from (x0, y0) to (x1, y1),
// as rAthena's path_search_long with CELL_CHKWALL, and how many cells of
// its line it recorded, at most MaxWalkPath.
func Shoot(g *Grid, x0, y0, x1, y1 int) (clear bool, length int) {
	dx := x1 - x0
	if dx < 0 {
		x0, x1 = x1, x0
		y0, y1 = y1, y0
		dx = -dx
	}
	dy := y1 - y0
	length = 1
	weight := abs(dy)
	if dx > abs(dy) {
		weight = dx
	}
	wx, wy := 0, 0
	for x0 != x1 || y0 != y1 {
		wx += dx
		wy += dy
		if wx >= weight {
			wx -= weight
			x0++
		}
		if wy >= weight {
			wy -= weight
			y0++
		} else if wy < 0 {
			wy += weight
			y0--
		}
		if length < MaxWalkPath {
			length++
		}
		if (x0 != x1 || y0 != y1) && g.wall(x0, y0) {
			return false, length
		}
	}
	return true, length
}

// StepTime is how long a unit with a walk speed, in milliseconds per
// cell, takes for one step, as rAthena's unit_walktoxy_sub: diagonal
// steps take 14/10 as long, rounded down.
func StepTime(d Dir, speed int) int {
	if d.Diagonal() {
		return speed * MoveDiagonalCost / MoveCost
	}
	return speed
}

// PathTime is the time to walk a whole path, as rAthena's
// unit_get_walkpath_time.
func PathTime(steps []Dir, speed int) int {
	total := 0
	for _, d := range steps {
		total += StepTime(d, speed)
	}
	return total
}

// Valid reports whether a grid's cells match its size.
func (g *Grid) Valid() bool {
	return g.Width > 0 && g.Height > 0 && g.Width <= 1024 && g.Height <= 1024 && len(g.Cells) == g.Width*g.Height
}
