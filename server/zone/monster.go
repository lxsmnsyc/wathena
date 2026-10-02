package zone

import (
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Spawn places monsters of one kind on the map.
type Spawn struct {
	Mob    uint32
	Name   string
	Amount int
	// Area is a center cell and how far from it monsters may appear. A
	// center of (0, 0) means anywhere on the map.
	X, Y, W, H int
	// Speed is the walk speed in milliseconds per cell.
	Speed int
	// CanMove is rAthena's MD_CANMOVE mode flag.
	CanMove bool
	// Status is the monster's battle status. Monsters without one cannot
	// be attacked.
	Status *battle.Status
	// RespawnMs is how long after dying a monster comes back.
	RespawnMs int
	// Reward is the monster's level, exp and drops. Monsters without one
	// give nothing when they die.
	Reward *progression.Mob
	// Sprite is the monster ID whose sprite players see, for a monster
	// of a mod that reuses another's. 0 shows Mob's own.
	Sprite uint32

	// owner is the script context that spawned the monsters, or 0.
	// noRespawn keeps dead monsters from coming back.
	owner     int64
	noRespawn bool
}

type monster struct {
	spawn *Spawn
	// nextWalk is when the monster next tries to wander, in zone
	// milliseconds, or -1 before its first try.
	nextWalk int64
	// dmglog is who fought the monster, for exp shares and loot.
	dmglog progression.DamageLog
	// handle is the monster's script handle, or 0 before a script needs
	// one.
	handle int64
}

// Random walk timing, as rAthena's mob.cpp.
const (
	// minRandomWalk is MIN_RANDOMWALKTIME.
	minRandomWalk = 4000
	// randomWalkRange is how far a random walk goes, d in mob_randomwalk.
	randomWalkRange = 7
	// thinkMs is how often a monster that found no cell tries again, as
	// rAthena's AI interval.
	thinkMs = 100
)

func (z *Zone) spawn(spawns []Spawn) {
	for i := range spawns {
		for range spawns[i].Amount {
			z.spawnOne(&spawns[i])
		}
	}
}

// spawnOne places one monster of a spawn. It returns nil when it found
// no cell.
func (z *Zone) spawnOne(s *Spawn) *entity {
	x, y, ok := z.spawnCell(s)
	if !ok {
		return nil
	}
	speed := s.Speed
	if speed <= 0 {
		speed = WalkSpeed
	}
	e := &entity{
		monster: &monster{spawn: s, nextWalk: -1},
		x:       x, y: y,
		dir:   z.rand.IntN(8),
		speed: speed,
	}
	if s.Status != nil {
		e.status, e.hp = s.Status, int64(s.Status.MaxHP)
	}
	z.entities.add(e)
	return e
}

// spawnCell picks a random walkable cell in a spawn's area, or anywhere
// when the area is empty or full.
func (z *Zone) spawnCell(s *Spawn) (int, int, bool) {
	if s.X > 0 || s.Y > 0 {
		for range 50 {
			x := s.X + z.rand.IntN(2*s.W+1) - s.W
			y := s.Y + z.rand.IntN(2*s.H+1) - s.H
			if !z.grid.NoPass(x, y) {
				return x, y, true
			}
		}
	}
	for range 1000 {
		x, y := z.rand.IntN(z.grid.Width), z.rand.IntN(z.grid.Height)
		if !z.grid.NoPass(x, y) {
			return x, y, true
		}
	}
	return 0, 0, false
}

// think runs monster AI for this tick: for now, wandering.
func (z *Zone) think() {
	for e := range z.entities.all() {
		m := e.monster
		if m == nil || !m.spawn.CanMove || len(e.path) > 0 || e.target != nil {
			continue
		}
		if m.nextWalk < 0 {
			m.nextWalk = z.now + int64(z.rand.IntN(1000)+minRandomWalk)
			continue
		}
		if z.now < m.nextWalk {
			continue
		}
		if steps, ok := z.randomWalk(e); ok {
			m.nextWalk = z.now + int64(z.rand.IntN(1000)+minRandomWalk+path.PathTime(steps, e.speed))
		} else {
			m.nextWalk = z.now + thinkMs
		}
	}
}

// randomWalk is rAthena's mob_randomwalk: try a random cell within 7,
// then step through the square in jumps of 7 in a random direction, so
// monsters do not stick to walls, until a walk starts.
func (z *Zone) randomWalk(e *entity) ([]path.Dir, bool) {
	const d = randomWalkRange
	const side = 2*d + 1
	// rAthena draws a random int32 and derives both offsets from it.
	r := z.rand.IntN(1 << 31)
	dir := z.rand.IntN(4)
	dx := r%side - d
	dy := r/side%side - d
	for range side * side {
		x, y := e.x+dx, e.y+dy
		if (x != e.x || y != e.y) && !z.grid.NoPass(x, y) {
			if steps, ok := path.Search(z.grid, e.x, e.y, x, y, false); ok && len(steps) > 0 {
				e.path = steps
				e.stepStart = z.now
				e.walkChanged = true
				return steps, true
			}
		}
		switch dir {
		case 0:
			if dx += d; dx > d {
				dx -= side
				if dy += d; dy > d {
					dy -= side
				}
			}
		case 1:
			if dx -= d; dx < -d {
				dx += side
				if dy -= d; dy < -d {
					dy += side
				}
			}
		case 2:
			if dy += d; dy > d {
				dy -= side
				if dx += d; dx > d {
					dx -= side
				}
			}
		case 3:
			if dy -= d; dy < -d {
				dy += side
				if dx -= d; dx < -d {
					dx += side
				}
			}
		}
	}
	return nil, false
}
