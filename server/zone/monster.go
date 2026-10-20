package zone

import (
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mob"
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
	// RespawnMs is how long after dying a monster comes back, and
	// RespawnVarianceMs the most time a death adds to it at random, as
	// rAthena's spawn delay1 and delay2 (mob_setdelayspawn).
	RespawnMs, RespawnVarianceMs int
	// Boss is set for rAthena's boss_monster lines.
	Boss bool
	// FixedCell places each monster on X and Y without looking for a
	// free cell, as mob_spawn does for a boss whose area is one cell. A
	// cell that cannot be walked on places it anywhere on the map.
	FixedCell bool
	// Reward is the monster's level, exp and drops. Monsters without one
	// give nothing when they die.
	Reward *progression.Mob
	// Sprite is the monster ID whose sprite players see, for a monster
	// of a mod that reuses another's. 0 shows Mob's own.
	Sprite uint32
	// Skills are the monster's skills, as mob_skill_db.
	Skills []mob.Skill
	// ViewRange is mob_db's SkillRange (range2): how far the monster
	// looks for targets. ChaseRange (range3) is how far it chases.
	ViewRange, ChaseRange int
	// Script is the monster's AI entry script, as mod:path, or "". Its
	// setup function runs once for each monster that spawns.
	Script string
	// Avoid are cells the spawn keeps clear: no monster of it appears
	// within R cells of each, by the larger axis distance. A procedural
	// world's stored spawns keep aggressive monsters off arrivals and
	// bosses off warps to towns (decision 0042).
	Avoid []Clear

	// owner is the script context that spawned the monsters, or 0.
	// noRespawn keeps dead monsters from coming back.
	owner     int64
	noRespawn bool
}

type monster struct {
	spawn *Spawn
	// touchNPC is the NPC whose monster_touch ran for the monster, until
	// it steps out of the touch areas, as md->areanpc_id.
	touchNPC *entity
	// nextWalk is next_walktime: when the monster next tries to wander,
	// in zone milliseconds.
	nextWalk int64
	// ai is the monster's skill state and aggressive flag.
	ai mob.AIState
	// target is target_id: the unit the AI follows, or an item a looter
	// goes for. The attack itself is the entity's fight target, as
	// rAthena keeps it in unit_data.
	target *entity
	// attacked and normAttacked are attacked_id and norm_attacked_id:
	// who hit the monster last, for the AI to answer. attackedCount is
	// state.attacked_count, for rude attacks.
	attacked, normAttacked *entity
	attackedCount          int
	// hits are the mob_attacked timers of hits that landed: the AI
	// learns who hit it once the hit's walk delay passed.
	hits []mobHit
	// nextThink, lastSkillCheck, lastLink and lastCanMove are
	// next_thinktime, last_skillcheck, last_linktime and last_canmove.
	nextThink, lastSkillCheck, lastLink, lastCanMove int64
	// skillDelay is skilldelay: when each of the monster's skills may
	// be used again. skillIdx is skill_idx: the skill being used, or -1.
	skillDelay []int64
	skillIdx   int
	// lastSkill is ud->skill_id: the skill the monster used last, for
	// after_skill. lastUse is the entry it used last and its target.
	lastSkill int
	lastUse   mobUse
	// chaseTo is ud.target_to: the unit a chase walk follows, and
	// chaseRange how close it gets. walked counts the steps of the walk
	// (walkpath.path_pos). walkFails is walktoxy_fail_count.
	chaseTo    *entity
	chaseRange int
	walked     int
	walkFails  int
	// attackContinue is ud.state.attack_continue: the chase attacks on
	// arrival. attackNow is set while unit_attack's own attack waits for
	// the fight, after which the state is set again.
	attackContinue bool
	attackNow      bool
	// forceWalk is ud.state.force_walk: a script walks the monster, so
	// the AI waits.
	forceWalk bool
	// loot are the items a looter picked up (lootitems). canLoot is set
	// when it has loot slots: when its modes made it a looter as it
	// spawned or changed class.
	loot    []fallItem
	canLoot bool
	// trickcasting is the end of NPC_RANDOMMOVE's fake cast.
	trickcasting int64
	// provoke is state.provoke_flag: the unit that provoked the monster.
	provoke *entity
	// level is md->level.
	level int
	// respawnAs is the spawn a monster that changed class comes back
	// as, or nil. reborn is state.rebirth: NPC_REBIRTH brought it back.
	respawnAs *Spawn
	reborn    bool
	// ctx is the context of the monster's AI script, or 0.
	ctx int64
	// dmglog is who fought the monster, for exp shares and loot.
	dmglog progression.DamageLog
	// handle is the monster's script handle, or 0 before a script needs
	// one.
	handle int64
	// stolen is set once a thief stole from the monster, or found it
	// cannot (steal_flag).
	stolen bool
	// stoleCoin is set once a Rogue mugged the monster
	// (steal_coin_flag).
	stoleCoin bool
	// soulChanged is set once Soul Exhale took the monster's SP
	// (soul_change_flag).
	soulChanged bool
	// noRewards is set when status_kill killed the monster, as
	// NPC_SUICIDE does: mob_dead's type 3 gives no exp and no drops.
	noRewards bool
	// master is the unit that summoned the monster (master_id), and
	// specialAI its special_state.ai, such as AI_FLORA. playersHit is set
	// when players may hit it (alchemist_summon_setting). deleteAt is when
	// its delete timer (mob_timer_delete) runs, or 0.
	master     *entity
	specialAI  int
	playersHit bool
	deleteAt   int64
	// event marks a typed world event's monster (autoevents.go), or is
	// nil.
	event *eventTag
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
	z.monsterSpawned(e)
	return e
}

// monsterSpawned is the end of mob_spawn: the AI starts, the monster's
// AI script runs, and its spawn skills are used.
func (z *Zone) monsterSpawned(e *entity) {
	if e.status == nil {
		return
	}
	z.startMonster(e)
	z.startMonsterScript(e)
	if !e.dead && z.entities.has(e) {
		z.mobSkillUse(e, mob.CondSpawned, 0)
	}
}

// respawnDelay is mob_setdelayspawn: the spawn's delay, plus a random
// part of its variance, and at least 1 second.
func (z *Zone) respawnDelay(s *Spawn) int64 {
	d := s.RespawnMs
	if s.RespawnVarianceMs > 0 {
		d += z.rand.IntN(s.RespawnVarianceMs)
	}
	return int64(max(d, 1000))
}

// Clear is a cell a spawn keeps clear, R cells around.
type Clear struct{ X, Y, R int }

// cleared reports whether a cell is within a spawn's clear cells.
func (s *Spawn) cleared(x, y int) bool {
	for _, c := range s.Avoid {
		if max(abs(x-c.X), abs(y-c.Y)) <= c.R {
			return true
		}
	}
	return false
}

// spawnCell picks a random walkable cell in a spawn's area, or anywhere
// when the area is empty or full, outside the spawn's clear cells.
func (z *Zone) spawnCell(s *Spawn) (int, int, bool) {
	if s.FixedCell && s.X > 0 && s.Y > 0 {
		if s.X < z.grid.Width && s.Y < z.grid.Height && !z.grid.NoPass(s.X, s.Y) {
			return s.X, s.Y, true
		}
		// npc_parse_mob spawns a fixed cell that cannot be reached
		// anywhere on the map.
		return z.anyCell(s)
	}
	if s.X > 0 || s.Y > 0 {
		for range 50 {
			x := s.X + z.rand.IntN(2*s.W+1) - s.W
			y := s.Y + z.rand.IntN(2*s.H+1) - s.H
			if !z.grid.NoPass(x, y) && !s.cleared(x, y) {
				return x, y, true
			}
		}
	}
	return z.anyCell(s)
}

// anyCell picks a random walkable cell of the map outside the spawn's
// clear cells.
func (z *Zone) anyCell(s *Spawn) (int, int, bool) {
	for range 1000 {
		x, y := z.rand.IntN(z.grid.Width), z.rand.IntN(z.grid.Height)
		if !z.grid.NoPass(x, y) && !s.cleared(x, y) {
			return x, y, true
		}
	}
	return 0, 0, false
}

// wanderOnly moves a monster without stats, which has no AI: it wanders
// when it can move, as mob_randomwalk.
func (z *Zone) wanderOnly(e *entity) {
	m := e.monster
	if !m.spawn.CanMove || len(e.path) > 0 || e.target != nil || e.cant().Move {
		return
	}
	if m.nextWalk < 0 {
		m.nextWalk = z.now + int64(z.rand.IntN(1000)+minRandomWalk)
		return
	}
	if z.now < m.nextWalk {
		return
	}
	if steps, ok := z.randomWalk(e); ok {
		m.nextWalk = z.now + int64(z.rand.IntN(1000)+minRandomWalk+path.PathTime(steps, e.speed))
	} else {
		m.nextWalk = z.now + thinkMs
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
