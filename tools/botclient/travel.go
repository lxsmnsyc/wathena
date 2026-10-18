package botclient

import (
	"math/rand/v2"

	"github.com/lxsmnsyc/wathena/shared/path"
)

// legSteps is the longest leg of a travel route a bot walks with one
// command. Paths are at most path.MaxWalkPath steps, and a route's cells
// may need a longer path around corners, so a leg stays shorter.
const legSteps = 24

// Route returns the cells of a shortest walk from one cell to another,
// both ends included, by breadth-first search over the 8 neighbours. A
// diagonal step needs both cells beside it walkable, so the route never
// cuts a corner. It returns nil when the target cannot be reached.
func Route(g *path.Grid, x0, y0, x1, y1 int) [][2]int {
	if g == nil || g.NoPass(x0, y0) || g.NoPass(x1, y1) {
		return nil
	}
	w := g.Width
	start, goal := y0*w+x0, y1*w+x1
	from := make([]int32, w*g.Height)
	for i := range from {
		from[i] = -1
	}
	from[start] = int32(start) // #nosec G115 -- a cell index of a map of at most 1,024 × 1,024 cells.
	queue := []int{start}
	for len(queue) > 0 && from[goal] < 0 {
		c := queue[0]
		queue = queue[1:]
		cx, cy := c%w, c/w
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				nx, ny := cx+dx, cy+dy
				if (dx == 0 && dy == 0) || g.NoPass(nx, ny) {
					continue
				}
				if dx != 0 && dy != 0 && (g.NoPass(cx+dx, cy) || g.NoPass(cx, cy+dy)) {
					continue
				}
				n := ny*w + nx
				if from[n] >= 0 {
					continue
				}
				from[n] = int32(c) // #nosec G115 -- a cell index of a map of at most 1,024 × 1,024 cells.
				queue = append(queue, n)
			}
		}
	}
	if from[goal] < 0 {
		return nil
	}
	var out [][2]int
	for c := goal; ; c = int(from[c]) {
		out = append(out, [2]int{c % w, c / w})
		if c == start {
			break
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// travel is a bot's route to a portal on its map.
type travel struct {
	mapName string
	route   [][2]int
	at      int
	// stuck counts walks that did not move the bot.
	stuck        int
	lastX, lastY int
}

// leg returns the next cell to walk to along the route: the farthest
// route cell within legSteps that a path reaches from the bot's cell.
// It returns false when the route is done or no leg is found.
func (t *travel) leg(g *path.Grid, x, y int) (int, int, bool) {
	// The bot may stand anywhere along the route after its last walk.
	best, dist := t.at, 1<<30
	for i := t.at; i < len(t.route) && i <= t.at+2*legSteps; i++ {
		if d := chebyshev(x, y, t.route[i][0], t.route[i][1]); d < dist {
			best, dist = i, d
		}
	}
	t.at = best
	if t.at >= len(t.route)-1 && dist == 0 {
		return 0, 0, false
	}
	for k := min(t.at+legSteps, len(t.route)-1); k > t.at; k -= 4 {
		c := t.route[k]
		if steps, ok := path.Search(g, x, y, c[0], c[1], false); ok && len(steps) > 0 {
			return c[0], c[1], true
		}
	}
	c := t.route[min(t.at+1, len(t.route)-1)]
	if steps, ok := path.Search(g, x, y, c[0], c[1], false); ok && len(steps) > 0 {
		return c[0], c[1], true
	}
	return 0, 0, false
}

// travelPick chooses where a travelling bot walks next: along a route
// to a portal of its map that is not the one it arrived at. It returns
// false when the map has no grid or no portal the bot can reach, and the
// caller then walks as without Travel.
func (beh Behaviour) travelPick(t *travel, v View, rnd *rand.Rand) (int, int, bool) {
	if beh.Grids == nil || beh.Portals == nil {
		return 0, 0, false
	}
	g := beh.Grids(v.Map)
	if g == nil {
		return 0, 0, false
	}
	if t.mapName != v.Map {
		*t = travel{mapName: v.Map}
	}
	if t.route != nil {
		if v.X == t.lastX && v.Y == t.lastY {
			t.stuck++
		} else {
			t.stuck = 0
		}
		if t.stuck >= 3 {
			// The bot stands on the route's end without a map change, or
			// cannot walk on: another portal.
			t.route = nil
		}
	}
	t.lastX, t.lastY = v.X, v.Y
	if t.route == nil {
		ports := beh.Portals(v.Map)
		var far [][2]int
		for _, p := range ports {
			if chebyshev(v.X, v.Y, p[0], p[1]) > 12 {
				far = append(far, p)
			}
		}
		if len(far) == 0 {
			far = ports
		}
		for _, i := range rnd.Perm(len(far)) {
			if r := Route(g, v.X, v.Y, far[i][0], far[i][1]); len(r) > 1 {
				t.route, t.at, t.stuck = r, 0, 0
				break
			}
		}
		if t.route == nil {
			return 0, 0, false
		}
	}
	x, y, ok := t.leg(g, v.X, v.Y)
	if !ok {
		t.route = nil
	}
	return x, y, ok
}
