//go:build procedural

package gen

import (
	"cmp"
	"slices"
	"strconv"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Dungeon floors of generator version 3 (strategy section 8.5,
// "Dungeons"). Each attempt runs from its node a:
//
//   - "rooms" or "cave": the layout.
//   - "props": pillars and clutter.
//   - "materials": patches of the accent texture.
//   - "heights": the floor and wall heights of caves.
//
// A floor has an entrance. A floor before the last has an exit in the
// room farthest from the entrance. The last floor has a boss room there
// instead.

const (
	// v3CaveFill is the share of blocks that start blocked, in percent.
	v3CaveFill = 45
	// v3CaveRounds is the number of cellular automaton rounds.
	v3CaveRounds = 5
	// v3CaveBlocked is how many of a block's 8 neighbors must be
	// blocked for it to become blocked.
	v3CaveBlocked = 5
	// v3CaveSmall is the size below which a cave area is filled
	// instead of joined by a tunnel, in blocks.
	v3CaveSmall = 6
	// v3BossRadius is the radius of a cave's boss chamber, in blocks.
	v3BossRadius = 4
)

// v3Rect is a room, in blocks.
type v3Rect struct{ x, y, w, h int }

// center returns the room's middle block.
func (r v3Rect) center() (int, int) { return r.x + r.w/2, r.y + r.h/2 }

// v3Floor is the state of one attempt at a floor.
type v3Floor struct {
	*v3Blocks
	p    DungeonParams
	last bool
	// entrance and goal are blocks: the goal holds the exit, or the boss
	// room on the last floor.
	entrance, goal int
	rooms          []v3Rect
	reserved       []bool
}

// dungeonV3 makes a dungeon floor.
func dungeonV3(node rng.Node, p DungeonParams) (Field, error) {
	if err := p.Check(); err != nil {
		return Field{}, wrapError(ErrParams, err.Error())
	}
	ts := v3Set(KindDungeon, p.Style.Tileset)
	if ts == nil {
		return Field{}, wrapError(ErrTileset, "no dungeon tileset "+strconv.Quote(p.Style.Tileset))
	}
	return attempts(node, func(a rng.Node) (*v3Blocks, string) {
		f := &v3Floor{v3Blocks: newV3Blocks(ts, p.Style.Width/2, p.Style.Height/2), p: p, last: p.Floor == p.Floors}
		f.reserved = make([]bool, f.bw*f.bh)
		var problem string
		if p.Style.Method == MethodCave {
			problem = f.cave(a.Child("cave"))
		} else {
			problem = f.roomsLayout(a.Child("rooms"))
		}
		if problem != "" {
			return nil, problem
		}
		f.placesAndStairs()
		f.props(a.Child("props"))
		f.paint(a.Child("materials"), a.Child("heights"))
		return f.v3Blocks, f.validate()
	})
}

// share returns the walkable blocks in a thousand.
func (f *v3Floor) share() int {
	n := 0
	for _, w := range f.walk {
		if w {
			n++
		}
	}
	return n * 1000 / len(f.walk)
}

// carve makes block (x, y) walkable, inside the border.
func (f *v3Floor) carve(x, y int) {
	if x >= 1 && y >= 1 && x < f.bw-1 && y < f.bh-1 {
		f.walk[y*f.bw+x] = true
	}
}

// roomsLayout places rooms without overlap and joins them with a
// minimum spanning tree and a few more corridors, so the floor has
// loops. The first room holds the entrance.
func (f *v3Floor) roomsLayout(n rng.Node) string {
	s := n.Stream()
	lo, hi := f.p.Style.RoomMin/2, f.p.Style.RoomMax/2
	avg := (lo + hi) / 2
	target := min(max(f.bw*f.bh/(avg*avg*6), 4), 40)
	for try := 0; try < 30*target && len(f.rooms) < target; try++ {
		w, h := lo+s.IntN(hi-lo+1), lo+s.IntN(hi-lo+1)
		r := v3Rect{x: 1 + s.IntN(f.bw-w-1), y: 1 + s.IntN(f.bh-h-1), w: w, h: h}
		free := true
		for _, o := range f.rooms {
			if r.x-1 < o.x+o.w && o.x-1 < r.x+r.w && r.y-1 < o.y+o.h && o.y-1 < r.y+r.h {
				free = false
				break
			}
		}
		if free {
			f.rooms = append(f.rooms, r)
		}
	}
	if len(f.rooms) < 3 {
		return "only " + strconv.Itoa(len(f.rooms)) + " rooms fit"
	}
	for _, r := range f.rooms {
		for y := r.y; y < r.y+r.h; y++ {
			for x := r.x; x < r.x+r.w; x++ {
				f.carve(x, y)
			}
		}
	}
	dist := func(i, j int) int {
		ax, ay := f.rooms[i].center()
		bx, by := f.rooms[j].center()
		return abs(ax-bx) + abs(ay-by)
	}
	// Prim's algorithm: the shortest edge from the tree, ties by the
	// lower room indexes.
	k := len(f.rooms)
	in := make([]bool, k)
	in[0] = true
	tree := make([]bool, k*k)
	var edges [][2]int
	for range k - 1 {
		best, bi, bj := -1, 0, 0
		for i := range k {
			if !in[i] {
				continue
			}
			for j := range k {
				if in[j] {
					continue
				}
				if d := dist(i, j); best < 0 || d < best {
					best, bi, bj = d, i, j
				}
			}
		}
		in[bj] = true
		tree[bi*k+bj], tree[bj*k+bi] = true, true
		edges = append(edges, [2]int{bi, bj})
	}
	// Loops: of the other pairs, nearest first, each joins with a chance
	// of one in three, up to a fifth of the rooms.
	extra := max(1, k/5)
	var others [][2]int
	for i := range k {
		for j := i + 1; j < k; j++ {
			if !tree[i*k+j] {
				others = append(others, [2]int{i, j})
			}
		}
	}
	slices.SortFunc(others, func(a, b [2]int) int {
		return cmp.Or(cmp.Compare(dist(a[0], a[1]), dist(b[0], b[1])), cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
	})
	for _, e := range others {
		if extra == 0 || dist(e[0], e[1]) > 3*(f.bw+f.bh)/4 {
			break
		}
		if s.IntN(3) == 0 {
			edges = append(edges, e)
			extra--
		}
	}
	for _, e := range edges {
		ax, ay := f.rooms[e[0]].center()
		bx, by := f.rooms[e[1]].center()
		if s.IntN(2) == 0 {
			f.line(ax, ay, bx, ay)
			f.line(bx, ay, bx, by)
		} else {
			f.line(ax, ay, ax, by)
			f.line(ax, by, bx, by)
		}
	}
	cx, cy := f.rooms[0].center()
	f.entrance = cy*f.bw + cx
	d := f.blockDistances(f.entrance)
	best := -1
	for i := 1; i < k; i++ {
		x, y := f.rooms[i].center()
		if c := d[y*f.bw+x]; c > int32(best) {
			best, f.goal = int(c), y*f.bw+x
		}
	}
	if best <= 0 {
		return "no room is reached from the entrance"
	}
	return ""
}

// line carves a straight corridor between two blocks on one axis.
func (f *v3Floor) line(x0, y0, x1, y1 int) {
	for x, y := x0, y0; ; {
		f.carve(x, y)
		if x == x1 && y == y1 {
			return
		}
		x += sign(x1 - x)
		y += sign(y1 - y)
	}
}

// cave runs the cellular automaton: about 45% of blocks start blocked,
// then 5 rounds of the rule "a block becomes blocked if 5 or more of its
// 8 neighbors are blocked, and a blocked block stays blocked with 4". It
// keeps the largest open area, fills small
// ones, and joins the others with tunnels.
func (f *v3Floor) cave(n rng.Node) string {
	s := n.Stream()
	for y := 1; y < f.bh-1; y++ {
		for x := 1; x < f.bw-1; x++ {
			f.walk[y*f.bw+x] = s.IntN(100) >= v3CaveFill
		}
	}
	next := make([]bool, len(f.walk))
	for range v3CaveRounds {
		for y := 1; y < f.bh-1; y++ {
			for x := 1; x < f.bw-1; x++ {
				blocked := 0
				for _, d := range v2Neighbors8 {
					if !f.walk[(y+d[1])*f.bw+x+d[0]] {
						blocked++
					}
				}
				// A blocked block stays blocked with one neighbor fewer,
				// the usual 4-5 rule, so the cave keeps its walls.
				keep := 0
				if !f.walk[y*f.bw+x] {
					keep = 1
				}
				next[y*f.bw+x] = blocked+keep < v3CaveBlocked
			}
		}
		f.walk, next = next, f.walk
	}
	label, sizes := f.areas()
	if len(sizes) == 0 {
		return "the cave has no open area"
	}
	main := 0
	for id, sz := range sizes {
		if sz > sizes[main] {
			main = id
		}
	}
	// One search from the main area through every inner block finds the
	// way from each other area to it.
	parent := make([]int32, len(f.walk))
	for i := range parent {
		parent[i] = -2
	}
	var queue []int
	for i, l := range label {
		if l == int32(main) { // #nosec G115 -- area IDs fit in int32.
			parent[i] = -1
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		x, y := c%f.bw, c/f.bw
		for _, d := range v2Neighbors8[:4] {
			nx, ny := x+d[0], y+d[1]
			if nx < 1 || ny < 1 || nx >= f.bw-1 || ny >= f.bh-1 {
				continue
			}
			j := ny*f.bw + nx
			if parent[j] == -2 {
				parent[j] = int32(c) // #nosec G115 -- blocks fit in int32.
				queue = append(queue, j)
			}
		}
	}
	firsts := make([]int, len(sizes))
	for i := range firsts {
		firsts[i] = -1
	}
	for i, l := range label {
		if l >= 0 && firsts[l] < 0 {
			firsts[l] = i
		}
	}
	for id, sz := range sizes {
		if id == main {
			continue
		}
		if sz < v3CaveSmall {
			for i, l := range label {
				if l == int32(id) { // #nosec G115 -- as above.
					f.walk[i] = false
				}
			}
			continue
		}
		for c := firsts[id]; c >= 0; c = int(parent[c]) {
			f.walk[c] = true
		}
	}
	// The entrance is a block whose 8 neighbors are open, picked by the
	// stream. The goal is the open such block farthest from it.
	var spacious []int
	for y := 1; y < f.bh-1; y++ {
		for x := 1; x < f.bw-1; x++ {
			if f.clear(x, y, nil) {
				spacious = append(spacious, y*f.bw+x)
			}
		}
	}
	if len(spacious) == 0 {
		return "the cave has no open spot"
	}
	f.entrance = spacious[s.IntN(len(spacious))]
	d := f.blockDistances(f.entrance)
	best := int32(0)
	for _, b := range spacious {
		if d[b] > best {
			best, f.goal = d[b], b
		}
	}
	if int(best) < (f.bw+f.bh)/6 {
		return "the cave's farthest spot is " + strconv.Itoa(int(best)) + " blocks from the entrance"
	}
	if f.last {
		gx, gy := f.goal%f.bw, f.goal/f.bw
		r := v3BossRadius
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				if dx*dx+dy*dy <= r*r {
					f.carve(gx+dx, gy+dy)
				}
			}
		}
	}
	return ""
}

// areas labels the open areas along the 4 axes and returns each area's
// size.
func (f *v3Floor) areas() (label []int32, sizes []int) {
	label = make([]int32, len(f.walk))
	for i := range label {
		label[i] = -1
	}
	var stack []int
	for i, w := range f.walk {
		if !w || label[i] >= 0 {
			continue
		}
		id := int32(len(sizes)) // #nosec G115 -- areas fit in int32.
		sizes = append(sizes, 0)
		label[i] = id
		stack = append(stack[:0], i)
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			sizes[id]++
			x, y := c%f.bw, c/f.bw
			for _, d := range v2Neighbors8[:4] {
				j := (y+d[1])*f.bw + x + d[0]
				if f.in(x+d[0], y+d[1]) && f.walk[j] && label[j] < 0 {
					label[j] = id
					stack = append(stack, j)
				}
			}
		}
	}
	return label, sizes
}

// placeCell returns the cell of a block's place: its south-west cell.
func (f *v3Floor) placeCell(b int) (int, int) { return 2 * (b % f.bw), 2 * (b / f.bw) }

// placesAndStairs records the places: the entrance first, then the exit
// or the boss room, then the other rooms. The stairs stand on the
// entrance and the exit, and do not block.
func (f *v3Floor) placesAndStairs() {
	ex, ey := f.placeCell(f.entrance)
	f.places = append(f.places, Place{Tags: []string{"entrance"}, X: ex, Y: ey})
	f.reserve(f.reserved, f.entrance%f.bw, f.entrance/f.bw)
	f.objects = append(f.objects, v3Object{prop: "stairs_up", hx: 2*ex + 1, hy: 2*ey + 1, scale: rng.One})
	gx, gy := f.placeCell(f.goal)
	f.reserve(f.reserved, f.goal%f.bw, f.goal/f.bw)
	if f.last {
		f.places = append(f.places, Place{Tags: []string{"boss_room"}, X: gx, Y: gy})
	} else {
		f.places = append(f.places, Place{Tags: []string{"exit"}, X: gx, Y: gy})
		f.objects = append(f.objects, v3Object{prop: "stairs_down", hx: 2*gx + 1, hy: 2*gy + 1, scale: rng.One})
	}
	for _, r := range f.rooms {
		x, y := r.center()
		b := y*f.bw + x
		if b == f.entrance || b == f.goal {
			continue
		}
		cx, cy := f.placeCell(b)
		f.places = append(f.places, Place{Tags: []string{"room"}, X: cx, Y: cy})
		f.reserve(f.reserved, x, y)
	}
}

// props puts the boss's altar, pillars at the corners of large rooms,
// and clutter on about 4 in 100 open blocks. Every prop blocks its block
// and keeps its 8 neighbors open.
func (f *v3Floor) props(n rng.Node) {
	ts := f.ts
	if f.last {
		gx, gy := f.goal%f.bw, f.goal/f.bw
		if f.clear(gx, gy+2, f.reserved) {
			f.propAt("altar", gx, gy+2, n.Hash(int64(gx), int64(gy), 2), true)
		}
	}
	if ts.Pillar != "" {
		for _, r := range f.rooms {
			if r.w < 6 || r.h < 6 {
				continue
			}
			for _, c := range [4][2]int{{r.x + 1, r.y + 1}, {r.x + r.w - 2, r.y + 1}, {r.x + 1, r.y + r.h - 2}, {r.x + r.w - 2, r.y + r.h - 2}} {
				if f.clear(c[0], c[1], f.reserved) {
					f.propAt(ts.Pillar, c[0], c[1], n.Hash(int64(c[0]), int64(c[1]), 1), true)
				}
			}
		}
	}
	var total uint64
	for _, p := range ts.Decor {
		total += p.Weight
	}
	for y := 1; y < f.bh-1; y++ {
		for x := 1; x < f.bw-1; x++ {
			hv := n.Hash(int64(x), int64(y), 0)
			if hv%100 >= 4 || !f.clear(x, y, f.reserved) {
				continue
			}
			pick := hv >> 8 % total
			prop := ts.Decor[0].Name
			for _, p := range ts.Decor {
				if pick < p.Weight {
					prop = p.Name
					break
				}
				pick -= p.Weight
			}
			f.propAt(prop, x, y, hv, true)
		}
	}
}

// paint sets the textures and heights: the floor texture with patches of
// the accent, walls of the wall texture standing WallHeight high. A
// cave's floor rolls by up to 3/4 of a cell, in steps of 1/8, and its
// walls by as much again.
func (f *v3Floor) paint(materials, heights rng.Node) {
	ts := f.ts
	floor, accent, wall := f.texture(ts.Floor), f.texture(ts.Accent), f.texture(ts.Wall)
	patch := v2Value{n: materials, shift: 3}
	roll := v2Value{n: heights, shift: 3}
	cave := f.p.Style.Method == MethodCave
	for y := range f.bh {
		for x := range f.bw {
			i := y*f.bw + x
			var h rng.Fixed
			if cave {
				h = rng.Fixed(roll.at(int64(x), int64(y))*3/4) &^ (rng.One/8 - 1) // #nosec G115 -- below one cell.
			}
			// A block with a prop stays floor below the prop.
			if !f.walk[i] && !f.prop[i] {
				f.mat[i] = wall
				f.height[i] = ts.WallHeight + h
				continue
			}
			f.mat[i] = floor
			if patch.at(int64(x), int64(y)) > int64(rng.One)*7/10 {
				f.mat[i] = accent
			}
			f.height[i] = h
		}
	}
}

// validate returns the floor's first problem, or "":
//
//   - The walkable share is 10% to 75%, and at least 25% for caves.
//   - Every place is reached from the entrance.
func (f *v3Floor) validate() string {
	share := f.share()
	lo := 100
	if f.p.Style.Method == MethodCave {
		lo = 250
	}
	if share < lo || share > 750 {
		return "the walkable share is " + strconv.Itoa(share) + " in 1000, not " + strconv.Itoa(lo) + " to 750"
	}
	return f.checkPlaces(f.entrance)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
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
