package progression

// Dir is a drop direction, as rAthena's directions: 0 north, turning
// counterclockwise, and DirCenter for the monster's own cell.
type Dir int

const (
	DirCenter    Dir = -1
	DirNorth     Dir = 0
	DirWest      Dir = 2
	DirSouthEast Dir = 5
	// dirMax is DIR_MAX, which picks a random cell around the target.
	dirMax Dir = 8
)

// dirX and dirY are rAthena's dirx and diry: the cell offset of each
// direction. y grows to the north.
var (
	dirX = [8]int{0, -1, -1, -1, 0, 1, 1, 1}
	dirY = [8]int{1, 1, 0, -1, -1, -1, 0, 1}
)

// NextDropDir is the direction of the next item of a kill, from
// mob_process_drop_list. The first item falls on the monster's cell, then
// the items go south-east, west and north, and around again.
func NextDropDir(d Dir) Dir {
	switch {
	case d <= DirNorth:
		return DirSouthEast
	case d == DirSouthEast:
		return DirWest
	}
	return DirNorth
}

// rndValue is rAthena's rnd_value: a number in [lo, hi]. It draws once
// even when lo equals hi.
func rndValue(r Rand, lo, hi int) int { return lo + r.IntN(hi-lo+1) }

// Placement is where an item lies: its cell, and its place inside the
// cell from 0 to 15.
type Placement struct {
	X, Y       int
	SubX, SubY int
}

// PlaceItem is the placement part of map_addflooritem for a monster's
// drop around (x, y), in direction dir. free is map_cell_free: whether a
// cell is walkable, away from the map edge, and has no unit on it.
//
// An item for another direction moves one cell that way. When that cell is
// not free, it goes to a random free cell next to it, or stays there when
// none is free, since drops may share a cell with other items.
func PlaceItem(free func(x, y int) bool, x, y int, dir Dir, r Rand) Placement {
	if dir > DirCenter && dir < dirMax {
		x += dirX[dir]
		y += dirY[dir]
	}
	if dir == dirMax || dir != DirCenter && !free(x, y) {
		x, y, _ = searchFreeCellDist(free, x, y, 1, 1, r)
	}
	subX := rndValue(r, 1, 4) * 3
	subY := rndValue(r, 1, 4) * 3
	return Placement{X: x, Y: y, SubX: subX, SubY: subY}
}

// searchFreeCellDist is map_search_freecell_dist: it tries each of the 8
// directions once, each at a random distance, starting from a random
// quadrant. It returns the cell unchanged when none is free.
func searchFreeCellDist(free func(x, y int) bool, x, y, distMin, distMax int, r Rand) (int, int, bool) {
	mirrorX, mirrorY := 1, 1
	if r.IntN(2) != 0 {
		mirrorX = -1
	}
	if r.IntN(2) != 0 {
		mirrorY = -1
	}
	for i := -1; i <= 1; i++ {
		for j := -1; j <= 1; j++ {
			if i == 0 && j == 0 {
				continue
			}
			// rAthena draws both distances, even for a 0 offset.
			cx := x + mirrorX*i*rndValue(r, distMin, distMax)
			cy := y + mirrorY*j*rndValue(r, distMin, distMax)
			if free(cx, cy) {
				return cx, cy, true
			}
		}
	}
	return x, y, false
}

// Rights is who may pick up a ground item and from when, as
// flooritem_data's first_get_charid to third_get_tick. Times are in the
// caller's milliseconds.
type Rights struct {
	First, Second, Third             uint32
	FirstTick, SecondTick, ThirdTick int64
}

// NewRights is the part of map_addflooritem that sets pick-up rights for
// an item dropped at now. Drops of bosses and MVPs are protected for
// longer.
func (t *Tables) NewRights(first, second, third uint32, now int64, boss BossType) Rights {
	get := t.ItemGetTime
	if boss != BossNone {
		get = t.MVPItemGetTime
	}
	r := Rights{First: first, Second: second, Third: third}
	r.FirstTick = now + get[0]
	r.SecondTick = r.FirstTick + get[1]
	r.ThirdTick = r.SecondTick + get[2]
	return r
}

// CanTake is the rights check of pc_takeitem for a player without a
// party. The first looter may take the item at once, the second after the
// first's time, the third after the second's, and anyone else after the
// third's.
func (r Rights) CanTake(charID uint32, now int64) bool {
	wait := r.ThirdTick
	switch {
	case r.First == 0 || r.First == charID:
		wait = 0
	case r.Second > 0 && r.Second == charID:
		wait = r.FirstTick
	case r.Third > 0 && r.Third == charID:
		wait = r.SecondTick
	}
	return now >= wait
}
