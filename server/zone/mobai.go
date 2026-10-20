package zone

import (
	"cmp"
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/mob"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Monster AI, as rAthena's mob.cpp (proposal 0002, review decision 22):
// mob_ai_sub_hard for monsters near a player, every 100 ms, and
// mob_ai_sub_lazy for the rest. The settings are rAthena's shipped
// conf/battle values, with monster_ai 0. The rules that need no map are in
// server/mob.

const (
	// minMobThink is MIN_MOBTHINKTIME: the hard AI runs this often.
	minMobThink = 100
	// lazyThink is how often the lazy AI runs, MIN_MOBTHINKTIME * 10.
	lazyThink = 1000
	// minMobLink is MIN_MOBLINKTIME: how often a monster calls for help
	// or follows its master's target.
	minMobLink = 1000
	// mobSkillInterval is MOB_SKILL_INTERVAL: how often a monster checks
	// its skills.
	mobSkillInterval = 1000
	// activeAIRange is ACTIVE_AI_RANGE: monsters this far past a
	// player's view run the hard AI.
	activeAIRange = 4
	// chaseRefresh is monster_chase_refresh: a chasing monster walks this
	// many cells before it looks at its target again.
	chaseRefresh = 32
	// mobUnlockTime is mob_unlock_time: a monster that cannot move drops
	// its target after this long.
	mobUnlockTime = 2000
	// assistRange is assist_range.
	assistRange = 11
	// lootRange is loot_range.
	lootRange = 12
	// lootItemSize is LOOTITEM_SIZE.
	lootItemSize = 10
	// rudeAttackedCount is RUDE_ATTACKED_COUNT.
	rudeAttackedCount = 1
	// slaveDistance is MOB_SLAVEDISTANCE.
	slaveDistance = 2
	// multihitDelay is multihit_delay: the walk delay each hit after the
	// first adds.
	multihitDelay = 80
)

// mobHit is a mob_attacked timer: who hit the monster, when the AI
// learns of it, and whether it was a normal attack in range
// (mob_norm_attacked).
type mobHit struct {
	src    *entity
	at     int64
	normal bool
}

// startMonster is the part of mob_spawn that sets the AI's state. A
// monster starts idle, and wanders after 4 to 5 seconds.
func (z *Zone) startMonster(e *entity) {
	m := e.monster
	m.ai = mob.AIState{}
	var mode uint32
	if e.status != nil {
		mode = e.status.Mode
	}
	m.ai.SetState(mob.StateIdle, mode)
	m.nextThink = z.now
	m.nextWalk = z.now + int64(z.rand.IntN(1000)+minRandomWalk)
	m.lastSkillCheck = z.now
	m.lastLink, m.lastCanMove = 0, z.now
	m.skillIdx = -1
	m.skillDelay = make([]int64, len(m.spawn.Skills))
	m.level = 1
	if e.status != nil {
		m.level = e.status.Level
	}
	m.canLoot = mode&mob.ModeLooter != 0
}

// monsterActive reports whether a monster is within reach of a player's
// view, so it runs the hard AI (mob_ai_sub_foreachclient).
func (z *Zone) monsterActive(e *entity) bool {
	const r = ViewRange + activeAIRange
	for _, p := range z.players {
		pe := p.entity
		if pe != nil && max(abs(pe.x-e.x), abs(pe.y-e.y)) <= r && z.entities.has(pe) {
			return true
		}
	}
	return false
}

// think runs the monster AI for this tick: the hard AI every 100 ms for
// monsters near a player, and the lazy AI every second for the rest.
func (z *Zone) think() {
	hard := z.now/minMobThink != z.prevThink/minMobThink || z.prevThink == 0
	lazy := z.now/lazyThink != z.prevThink/lazyThink || z.prevThink == 0
	z.prevThink = z.now
	for e := range z.entities.all() {
		m := e.monster
		if m == nil || e.dead {
			continue
		}
		if (m.deleteAt > 0 || m.master != nil && m.specialAI != aiNone) && z.summonThink(e) {
			continue
		}
		if e.status == nil {
			z.wanderOnly(e)
			continue
		}
		z.mobHits(e)
		if !z.entities.has(e) || e.dead {
			continue
		}
		switch {
		case hard && z.monsterActive(e):
			z.mobThink(e)
		case lazy:
			z.mobThinkLazy(e)
		}
	}
}

// mobHits runs the monster's mob_attacked timers that are due.
func (z *Zone) mobHits(e *entity) {
	m := e.monster
	for len(m.hits) > 0 && z.now >= m.hits[0].at {
		h := m.hits[0]
		m.hits[0] = mobHit{}
		m.hits = m.hits[1:]
		if z.entities.has(h.src) && !h.src.dead {
			z.mobSetAttacked(e, h.src, h.normal)
		}
		if e.dead || !z.entities.has(e) {
			return
		}
	}
}

// mobAttackedLater is the end of battle_damage for a monster that a hit
// reached: the AI learns of it once the damage shows (shown) and the
// hit's walk delay passed (battle_calc_walkdelay), as the mob_attacked
// timer. normal is set for a
// normal attack from within the monster's attack range plus one while it
// attacks someone else (is_norm_attacked).
func (z *Zone) mobAttackedLater(t, src *entity, damage int64, hits int, normal bool, shown int64) {
	m := t.monster
	if m == nil || src == nil || src == t {
		return
	}
	var delay int64
	if damage >= 1 && hits != 0 {
		delay = int64(t.status.DMotion)
		if hits > 1 {
			if hits > 10 {
				hits = 2
			}
			delay += int64(multihitDelay * (hits - 1))
		}
		delay = max(delay, 1)
	}
	at := z.now + shown + delay
	i := len(m.hits)
	for i > 0 && m.hits[i-1].at > at {
		i--
	}
	m.hits = slices.Insert(m.hits, i, mobHit{src, at, normal})
	if at == z.now {
		z.mobHits(t)
	}
}

// mobSetAttacked is mob_set_attacked_id: the monster learns who hit it,
// leaves its aggressive state, and runs its AI at once. A summoned
// monster that hit it stands for its master (retaliate_to_master).
func (z *Zone) mobSetAttacked(e, src *entity, normal bool) {
	m := e.monster
	who := src
	if sm := src.monster; sm != nil && sm.master != nil {
		who = sm.master
	}
	m.attacked = who
	if normal {
		m.normAttacked = who
	}
	m.ai.Aggressive = false
	z.mobThink(e)
}

// isNormAttacked is the check of battle_weapon_attack: a monster that
// attacks someone else is hit by a normal attack from within its attack
// range plus one.
func isNormAttacked(src, t *entity) bool {
	m := t.monster
	if m == nil || m.ai.State != mob.StateBerserk || m.target == src {
		return false
	}
	return chebyshev(src, t) <= t.status.RHW.Range+1
}

// viewRange is the monster's view range (range2), or 1 while blind.
func (e *entity) viewRange() int {
	if e.sc.Has(sc.SCBlind) {
		return 1
	}
	return e.monster.spawn.ViewRange
}

// chaseRange is range3.
func (e *entity) chaseRange() int { return e.monster.spawn.ChaseRange }

// attackRange is status rhw.range.
func (e *entity) attackRange() int { return e.status.RHW.Range }

// unitCanMove is unit_can_move.
func (z *Zone) unitCanMove(e *entity) bool {
	if e.cast != nil && !freeCast(e) {
		return false
	}
	if z.now < e.canMove {
		return false
	}
	return !e.cant().Move
}

// unitCanReach is unit_can_reach_bl: whether a unit can walk next to
// another within range cells (no limit for 0). It returns the cell next
// to the target it would walk to.
func (z *Zone) unitCanReach(e, t *entity, rng int) (int, int, bool) {
	if e.x == t.x && e.y == t.y {
		return t.x, t.y, true
	}
	if rng > 0 && chebyshev(e, t) > rng {
		return 0, 0, false
	}
	dx, dy := sign(t.x-e.x), sign(t.y-e.y)
	if z.grid.NoPass(t.x-dx, t.y-dy) {
		i := 0
		for ; i < 8 && z.grid.NoPass(t.x-dirX[i], t.y-dirY[i]); i++ {
		}
		if i == 8 {
			return 0, 0, false
		}
		dx, dy = dirX[i], dirY[i]
	}
	x, y := t.x-dx, t.y-dy
	steps, ok := path.Search(z.grid, e.x, e.y, x, y, false)
	if !ok {
		return 0, 0, false
	}
	// OFFICIAL_WALKPATH: past 14 cells the way must be clear.
	if len(steps) > ViewRange {
		if clear, _ := path.Shoot(z.grid, e.x, e.y, x, y); !clear {
			return 0, 0, false
		}
	}
	return x, y, true
}

// inRange is battle_check_range between two units.
func (z *Zone) inRange(e, t *entity, rng int) bool { return z.checkRange(e, t.x, t.y, rng) }

// visible is status_check_visibility for a monster: the target is
// within its chase range, or 1 cell when blind, and not hidden from it.
func (z *Zone) visible(e, t *entity, checkBlind bool) bool {
	r := ViewRange
	if e.monster != nil {
		r = e.chaseRange()
	}
	if checkBlind && e.sc.Has(sc.SCBlind) {
		r = 1
	}
	if chebyshev(e, t) > r {
		return false
	}
	if gmHidden(t) {
		// status_check_visibility: no one sees a hidden GM.
		return false
	}
	l := t.sc
	if l.Empty() || t.player == nil {
		return true
	}
	boss := e.status != nil && e.status.Class == 1
	detector := e.status != nil && e.status.Mode&modeDetector != 0
	if l.Option&(sc.OptionHide|sc.OptionCloak|optionChasewalk) != 0 && !boss && !detector {
		return false
	}
	return true
}

// canTargetAI is status_check_skilluse without a skill, the checks of a
// normal attack: the monster can attack, and the target lives and is not
// hidden from it. A looter may target an item.
func (z *Zone) canTargetAI(e, t *entity) bool {
	if e.player == nil && e.dead || !z.entities.has(t) {
		return false
	}
	if e.cant().Attack || e.status == nil || e.status.Mode&mob.ModeCanAttack == 0 {
		return false
	}
	if t.dead {
		return false
	}
	if l := e.sc; !l.Empty() && (l.Has(sc.SCTrickdead) || l.Has(sc.SCBladestop) || e.gospelCaster()) {
		return false
	}
	if t == e {
		return true
	}
	if t.item != nil {
		return e.status.Mode&mob.ModeLooter != 0
	}
	if gmHidden(t) {
		// status_check_skilluse: a hidden GM is no target.
		return false
	}
	l := t.sc
	if l.Empty() {
		return true
	}
	if l.Has(sc.SCTrickdead) {
		return false
	}
	detect := e.status.Mode&mob.ModeDetector != 0
	if l.Option&(sc.OptionHide|sc.OptionCloak|optionChasewalk) != 0 {
		if t.player != nil {
			return e.status.Class == 1 || detect
		}
		return detect
	}
	return true
}

// mobUnlockTarget is mob_unlocktarget: the monster drops its target and
// goes idle.
func (z *Zone) mobUnlockTarget(e *entity) {
	m := e.monster
	switch m.ai.State {
	case mob.StateWalk:
		if len(e.path) > 0 {
			break
		}
		m.ai.SetState(mob.StateIdle, e.status.Mode)
	case mob.StateIdle:
	case mob.StateLoot:
		// Looters that lost their target stop soon.
		z.stopWalking(e)
		fallthrough
	default:
		e.target = nil
		m.ai.SetState(mob.StateIdle, e.status.Mode)
		m.nextWalk = z.now + int64(z.rand.IntN(1000)+minRandomWalk)
	}
	m.target = nil
	m.attackContinue = false
	m.chaseTo = nil
}

// stopWalking is unit_stop_walking.
func (z *Zone) stopWalking(e *entity) {
	if len(e.path) > 0 {
		e.path = e.path[:0]
		e.stopped = true
	}
}

// mobThinkLazy is mob_ai_sub_lazy with mob_active_time and
// boss_active_time at 0: a monster no player is near only drops its
// target. It neither wanders nor uses skills, since no player spotted
// it recently.
func (z *Zone) mobThinkLazy(e *entity) {
	m := e.monster
	if m.forceWalk || z.now-m.nextThink < 2*minMobThink || z.now-m.lastSkillCheck < mobSkillInterval {
		return
	}
	if m.master != nil {
		// A slave that no player spotted waits (slave_active_with_master
		// is off).
		return
	}
	z.mobUnlockTarget(e)
	// An invader far from players walks toward them.
	z.invaderMarch(e)
}

// mobThink is mob_ai_sub_hard. It reports whether the AI did something.
func (z *Zone) mobThink(e *entity) bool {
	m := e.monster
	if e.dead || !z.entities.has(e) || e.status == nil || m.forceWalk || z.now < m.nextThink {
		return false
	}
	m.nextThink = z.now
	if e.cast != nil {
		return false
	}
	// Abnormalities: stun, freeze, sleep and stone make it forget.
	if e.actsBlocked() || z.loseTarget(e) {
		m.target, m.attacked, m.normAttacked = nil, nil, nil
		return false
	}
	// A monster checks its skills before the rest of its AI.
	if len(e.path) == 0 {
		var ready bool
		if m.ai.State == mob.StateBerserk {
			ready = z.now >= e.nextAttack
		} else {
			ready = z.now-m.lastSkillCheck >= mobSkillInterval
		}
		if ready && z.mobSkillUse(e, mobEventNone, 0) {
			return true
		}
	}
	view := e.viewRange()
	mode := e.status.Mode
	canMove := mode&mob.ModeCanMove != 0 && z.unitCanMove(e)
	if canMove {
		m.lastCanMove = z.now
	}
	tbl := m.target
	if tbl != nil {
		invalid := !z.entities.has(tbl) || tbl.dead ||
			e.target == nil && !z.canTargetAI(e, tbl) ||
			tbl.player != nil && gangsterProtects(e, tbl)
		if invalid {
			if z.entities.has(tbl) && len(e.path) > 0 && (!canMove || m.walked <= chaseRefresh) {
				return true
			}
			z.mobUnlockTarget(e)
			tbl = nil
		}
		// A chasing monster that stopped looks at its target again.
		if tbl != nil && tbl.item == nil && len(e.path) == 0 && mob.IsChasing(m.ai.State) {
			if !z.inRange(e, tbl, e.attackRange()) && !z.visible(e, tbl, true) {
				z.mobUnlockTarget(e)
				return true
			}
		}
	}
	// A hit may change the target.
	if abl := m.attacked; abl != nil && mode&mob.ModeCanAttack != 0 {
		if abl == m.target {
			// Rude attacked: the monster cannot hit back nor reach.
			if !z.inRange(e, tbl, e.attackRange()) && (!canMove && z.now > e.canMove && (e.mobTrapped() || m.walkFails > 0) ||
				!z.mobCanReach(e, tbl, e.chaseRange())) {
				m.attackedCount++
				if m.attackedCount > rudeAttackedCount {
					z.mobSkillUse(e, mob.CondHitWhileStuck, 0)
				}
			}
		} else if z.entities.has(abl) && !abl.dead && (tbl == nil || m.ai.CanChangeTarget(mode, m.normAttacked == abl)) {
			dist := chebyshev(e, abl)
			switch {
			case dist > ViewRange || !isEnemy(e, abl) ||
				!z.inRange(e, abl, e.attackRange()) && (!canMove && z.now > e.canMove && (e.mobTrapped() || m.walkFails > 0) ||
					!z.mobCanReach(e, abl, dist+e.chaseRange())):
				// Rude attacked. Hurting itself is not.
				if abl != e {
					m.attackedCount++
					if m.attackedCount > rudeAttackedCount {
						z.mobSkillUse(e, mob.CondHitWhileStuck, 0)
					}
				}
			case !z.canTargetAI(e, abl):
				// It cannot attack back.
			default:
				// It takes the one that hit it as its target.
				if pick := z.monsterTargetHook(e, []*entity{abl}, abl, "hit"); pick != nil {
					m.target = pick
					if m.attackedCount > 0 {
						m.attackedCount--
					}
					tbl = pick
				}
			}
		}
		m.attacked, m.normAttacked = nil, nil
	}
	if m.attacked != nil {
		m.attacked, m.normAttacked = nil, nil
	}
	slaveLost := false
	if m.master != nil {
		if z.mobSlaveThink(e) {
			return true
		}
		slaveLost = true
	}
	// A looter looks for items.
	if canMove && mode&mob.ModeLooter != 0 && m.canLoot && z.now > e.canAct && len(m.loot) < lootItemSize {
		switch {
		case tbl == nil:
			tbl = z.mobLootSearch(e, lootRange, nil)
		case tbl.item != nil:
			// monster_loot_search_type 1 keeps the item it chose.
		}
	}
	if mode&mob.ModeAggressive != 0 && (tbl == nil || slaveLost) || m.ai.State == mob.StateFollow {
		prev := m.target
		if t := z.mobActiveSearch(e, view, tbl); t != nil {
			tbl = t
		}
		// A monster that finds a new target already in range rushes it.
		if tbl != nil && prev != m.target && z.inRange(e, tbl, e.attackRange()) {
			m.ai.Aggressive = false
			m.ai.SetState(mob.StateRush, mode)
		}
	} else if mode&mob.ModeChangeChase != 0 && (m.ai.State == mob.StateRush || m.ai.State == mob.StateFollow) {
		size := min(view, e.attackRange())
		if t := z.mobChangeChase(e, size, tbl); t != nil {
			tbl = t
		}
	}
	if tbl == nil {
		z.mobUnlockTarget(e)
		// An invader walks toward the players.
		if m.master == nil && z.invaderMarch(e) {
			return true
		}
		// Random walk.
		if m.master == nil && z.now >= m.nextWalk && !z.mobRandomWalk(e) {
			if m.nextWalk < e.canMove {
				m.nextWalk = e.canMove
			} else {
				m.nextWalk = z.now + int64(z.rand.IntN(1000))
			}
		}
		return true
	}
	if tbl.item != nil {
		return z.mobLoot(e, tbl, canMove)
	}
	// Attack when the target is in range.
	if z.inRange(e, tbl, e.attackRange()) {
		if e.sc == nil || e.sc.Option&sc.OptionHide == 0 {
			if e.target != tbl {
				z.unitAttack(e, tbl)
			}
		} else {
			// Hiding pretends the attack happened.
			m.ai.SetState(mob.StateBerserk, mode)
		}
		z.stopWalking(e)
		m.chaseTo = nil
		return true
	}
	// A chase goes on for a few cells before the monster looks again.
	if len(e.path) > 0 && (!canMove || m.walked <= chaseRefresh) && mob.IsChasing(m.ai.State) {
		return true
	}
	if mode&mob.ModeCanMove == 0 || !canMove && (e.cant().Move || z.now > e.canMove) {
		// It cannot chase: it uses idle skills, and drops its target after
		// a while.
		if e.target == nil {
			if z.now-m.lastCanMove > mobUnlockTime {
				z.mobUnlockTarget(e)
			} else {
				m.ai.SetState(mob.StateIdle, mode)
			}
		}
		return true
	}
	if len(e.path) > 0 && m.chaseTo == tbl {
		return true
	}
	// An angry monster only chases a target within its chase range.
	if m.ai.State == mob.StateAngry && !z.mobCanReach(e, tbl, e.chaseRange()) {
		z.mobUnlockTarget(e)
		return true
	}
	if !z.walkToBL(e, tbl, e.attackRange(), true) {
		z.mobUnlockTarget(e)
	}
	return true
}

// mobTrapped reports the statuses that keep a monster in place, which
// make a hit rude (SC_SPIDERWEB and the like).
func (e *entity) mobTrapped() bool {
	return e.sc.Has(sc.SCSpiderweb)
}

// mobCanReach is mob_can_reach: unit_can_reach_bl without easy walking.
func (z *Zone) mobCanReach(e, t *entity, rng int) bool {
	if t == nil {
		return false
	}
	_, _, ok := z.unitCanReach(e, t, rng)
	return ok
}

// unitAttack is unit_attack for a monster: it attacks the target,
// continuously, and sets the attack state. A monster with random_target
// attacks a random enemy in its attack range instead (mob_randomtarget).
// The attack happens at once when the attack delay passed, and the
// state is set after it, as unit_attack does.
func (z *Zone) unitAttack(e, t *entity) {
	m := e.monster
	if e.status.Mode&mob.ModeRandomTarget != 0 {
		t = z.mobGetEnemy(e, e.attackRange())
		if t == nil || !z.inRange(e, t, e.attackRange()) {
			return
		}
	}
	if !z.canTargetAI(e, t) || !isEnemy(e, t) {
		z.unitUnattackable(e)
		return
	}
	e.target, e.repeat = t, true
	m.chaseTo = nil
	m.attackContinue = true
	if z.now >= e.nextAttack {
		m.attackNow = true
	}
	m.ai.SetState(mob.StateBerserk, e.status.Mode)
}

// unitUnattackable is unit_unattackable for a monster: it stops its
// attack and its chase, and drops its target.
func (z *Zone) unitUnattackable(e *entity) {
	e.target = nil
	e.monster.attackContinue = false
	e.monster.chaseTo = nil
	z.mobUnlockTarget(e)
}

// enemyCandidates are the units of DEFAULT_ENEMY_TYPE within a range of
// a monster, in rAthena's block order: blocks by row, then column, and
// the newest unit first within a block.
func (z *Zone) enemyCandidates(e *entity, rng int) []*entity {
	var out []*entity
	add := func(o *entity) {
		if o != e && !o.dead && max(abs(o.x-e.x), abs(o.y-e.y)) <= rng {
			out = append(out, o)
		}
	}
	if m := e.monster; m.specialAI == aiNone {
		for _, p := range z.players {
			if pe := p.entity; pe != nil && z.entities.has(pe) {
				add(pe)
			}
		}
		// A script's monster with an AI and no master is an enemy too
		// (isEnemy).
		if z.aiMonsters {
			for o := range z.entities.all() {
				if om := o.monster; om != nil && om.master == nil && om.specialAI != aiNone {
					add(o)
				}
			}
		}
	} else {
		for o := range z.entities.all() {
			if o.player != nil || o.monster != nil {
				add(o)
			}
		}
	}
	slices.SortFunc(out, func(a, b *entity) int {
		if c := cmp.Compare(a.y/blockSize, b.y/blockSize); c != 0 {
			return c
		}
		if c := cmp.Compare(a.x/blockSize, b.x/blockSize); c != 0 {
			return c
		}
		return cmp.Compare(b.uid, a.uid)
	})
	return out
}

// mobActiveSearch is mob_ai_sub_hard_activesearch over the units in view:
// the closest enemy it may attack, that it can walk to within its view
// range when standing or chase range when walking (ACTIVEPATHSEARCH). It
// returns the new target, or nil when it kept the old one.
func (z *Zone) mobActiveSearch(e *entity, view int, cur *entity) *entity {
	m := e.monster
	mode := e.status.Mode
	var cands []*entity
	pick := cur
	for _, bl := range z.enemyCandidates(e, view) {
		if bl == pick || !z.canTargetAI(e, bl) {
			continue
		}
		if mode&mob.ModeTargetWeak != 0 && z.unitLevel(bl) >= m.level-5 {
			continue
		}
		if !isEnemy(e, bl) || gangsterProtects(e, bl) {
			continue
		}
		dist := chebyshev(e, bl)
		if !z.inRange(e, bl, m.spawn.ViewRange) {
			continue
		}
		steps, ok := path.Search(z.grid, e.x, e.y, bl.x, bl.y, false)
		if !ok {
			continue
		}
		if len(e.path) == 0 && len(steps) > m.spawn.ViewRange || len(e.path) > 0 && len(steps) > m.spawn.ChaseRange {
			continue
		}
		cands = append(cands, bl)
		if pick == nil || chebyshev(e, pick) > dist {
			pick = bl
		}
	}
	if pick == cur {
		pick = nil
	}
	if len(cands) == 0 {
		return nil
	}
	if p := z.monsterTargetHook(e, cands, pick, "aggressive"); p != nil {
		m.target = p
		return p
	}
	return nil
}

// mobChangeChase is mob_ai_sub_hard_changechase: a chasing monster takes
// an enemy it can hit now instead.
func (z *Zone) mobChangeChase(e *entity, size int, cur *entity) *entity {
	m := e.monster
	var cands []*entity
	pick := cur
	for _, bl := range z.enemyCandidates(e, size) {
		if bl == pick || !isEnemy(e, bl) || !z.canTargetAI(e, bl) {
			continue
		}
		if z.inRange(e, bl, e.attackRange()) {
			cands = append(cands, bl)
			pick = bl
		}
	}
	if len(cands) == 0 || pick == cur {
		return nil
	}
	if p := z.monsterTargetHook(e, cands, pick, "change"); p != nil {
		m.target = p
		return p
	}
	return nil
}

// unitLevel is status_get_lv.
func (z *Zone) unitLevel(e *entity) int {
	if p := e.player; p != nil {
		return max(p.progress.BaseLevel, 1)
	}
	if m := e.monster; m != nil {
		return m.level
	}
	if e.status != nil {
		return e.status.Level
	}
	return 1
}

// mobLinkSearch is mob_linksearch for the units that an assisting
// monster's attack calls: monsters of its kind that assist, have no
// target and did not answer a call in the last second take its target.
func (z *Zone) mobLinkSearch(e, target *entity) {
	m := e.monster
	if e.status.Mode&mob.ModeAssist == 0 || z.now-m.lastLink < minMobLink {
		return
	}
	m.lastLink = z.now
	for o := range z.entities.all() {
		om := o.monster
		if om == nil || o == e || o.dead || om.spawn.Mob != m.spawn.Mob || om.target != nil {
			continue
		}
		if chebyshev(e, o) > assistRange || o.status.Mode&mob.ModeAssist == 0 || z.now-om.lastLink < minMobLink {
			continue
		}
		if clear, _ := path.Shoot(z.grid, e.x, e.y, o.x, o.y); !clear {
			continue
		}
		om.lastLink = z.now
		if p := z.monsterTargetHook(o, []*entity{target}, target, "assist"); p != nil {
			om.target = p
		}
	}
}

// mobRandomWalk is mob_randomwalk.
func (z *Zone) mobRandomWalk(e *entity) bool {
	m := e.monster
	mode := e.status.Mode
	if z.now < m.nextWalk || mode&mob.ModeNoRandomWalk != 0 || !z.unitCanMove(e) || mode&mob.ModeCanMove == 0 {
		return false
	}
	m.attackContinue = false
	m.chaseTo = nil
	steps, ok := z.randomWalk(e)
	if !ok {
		return false
	}
	m.walked = 0
	m.ai.SetState(mob.StateWalk, mode)
	m.nextWalk = z.now + int64(z.rand.IntN(1000)+minRandomWalk+path.PathTime(steps, e.speed))
	return true
}

// walkToBL is unit_walktobl: the unit walks next to a target, for a
// monster the cell next to it on its side, and attacks it on arrival
// within rng when attack is set. A range of 0 walks onto the target's
// cell, as looters do.
func (z *Zone) walkToBL(e, t *entity, rng int, attack bool) bool {
	m := e.monster
	if e.status != nil && e.status.Mode&mob.ModeCanMove == 0 {
		return false
	}
	x, y, ok := z.unitCanReach(e, t, chebyshev(e, t)+1)
	if !ok {
		if m != nil {
			m.chaseTo = nil
		}
		return false
	}
	if rng == 0 {
		x, y = t.x, t.y
	}
	if e.sc.Has(sc.SCConfusion) {
		x, y = z.randomDir(e, x, y)
	}
	if m != nil {
		m.chaseTo, m.chaseRange, m.attackContinue = t, rng, attack
	}
	e.target = nil
	if !z.unitCanMove(e) {
		// The walk starts when the walk delay ends (unit_walktobl_sub).
		return z.now < e.canMove
	}
	steps, ok := path.Search(z.grid, e.x, e.y, x, y, false)
	if !ok {
		return false
	}
	if len(steps) > ViewRange {
		if clear, _ := path.Shoot(z.grid, e.x, e.y, x, y); !clear {
			return false
		}
	}
	if len(steps) == 0 {
		return true
	}
	if len(e.path) > 0 {
		// A walk in progress changes target at the next cell.
		dx, dy := e.path[0].Step()
		if more, ok := path.Search(z.grid, e.x+dx, e.y+dy, x, y, false); ok {
			e.path = append(e.path[:1], more...)
			e.walkChanged = true
		}
	} else {
		e.path = steps
		e.stepStart = z.now
		e.walkChanged = true
	}
	if m != nil {
		m.walked = 0
		if attack {
			m.ai.SetState(mob.StateRush, e.status.Mode)
		}
		z.mobNextCell(e)
	}
	return true
}

// mobNextCell is unit_walktoxy_nextcell for a monster about to take a
// step: it may use a chase or walk skill, and a chase checks its target.
func (z *Zone) mobNextCell(e *entity) {
	m := e.monster
	if len(e.path) == 0 || e.dead {
		return
	}
	if !m.forceWalk && z.now-m.lastSkillCheck > int64(mobSkillInterval-e.speed/2) && z.mobSkillUse(e, mobEventNone, 0) {
		id := m.lastSkill
		if (id != mob.IDSpeedUp || m.trickcasting == 0) && id != mob.IDEmotion && id != mob.IDEmotionOn && m.ai.State != mob.StateWalk {
			// The skill stops the walk.
			z.stopWalking(e)
			return
		}
	}
	if m.chaseTo != nil {
		z.mobUpdateChase(e, len(e.path) == 1)
	}
}

// mobUpdateChase is unit_update_chase: a chasing monster attacks a target
// in reach, drops one it cannot see, or follows it to its new cell.
func (z *Zone) mobUpdateChase(e *entity, full bool) bool {
	m := e.monster
	t := m.chaseTo
	if t != nil && t.item == nil && z.entities.has(t) && m.walked > 0 && chebyshev(e, t) <= m.chaseRange {
		if m.attackContinue {
			z.unitAttack(e, t)
		}
		z.stopWalking(e)
		m.chaseTo = nil
		return true
	}
	if t == nil || !z.entities.has(t) || t.dead || full && !z.visible(e, t, true) {
		z.mobUnlockTarget(e)
		return false
	}
	if full && m.walked > 0 && z.now >= e.canMove {
		return z.walkToBL(e, t, m.chaseRange, m.attackContinue)
	}
	return false
}

// mobStepped runs after a monster entered a cell: the next step's checks,
// or at the end of the walk, the chase's.
func (z *Zone) mobStepped(e *entity) {
	m := e.monster
	m.walked++
	m.walkFails = 0
	if len(e.path) > 0 {
		if !m.forceWalk {
			z.mobNextCell(e)
		}
		return
	}
	// A walk that a script asked for ends here.
	m.forceWalk = false
	if m.chaseTo != nil {
		z.mobUpdateChase(e, true)
	}
}

// mobLootSearch is mob_ai_sub_hard_lootsearch over the items within rng
// cells in a clear line: with monster_loot_search_type 1, the item with
// the lowest ID that the monster can reach wins.
func (z *Zone) mobLootSearch(e *entity, rng int, cur *entity) *entity {
	m := e.monster
	pick := cur
	for o := range z.entities.all() {
		if o.item == nil || max(abs(o.x-e.x), abs(o.y-e.y)) > rng {
			continue
		}
		if clear, _ := path.Shoot(z.grid, e.x, e.y, o.x, o.y); !clear {
			continue
		}
		if !z.mobCanReach(e, o, lootRange) {
			continue
		}
		if pick == nil || m.target != nil && m.target.uid > o.uid {
			pick = o
			m.target = o
		}
	}
	return pick
}

// mobLoot is the loot part of mob_ai_sub_hard: walk to the item, or pick
// it up once on its cell.
func (z *Zone) mobLoot(e, it *entity, canMove bool) bool {
	m := e.monster
	reach := 0
	if len(e.path) > 0 {
		if m.chaseTo == it {
			return true
		}
	}
	if chebyshev(e, it) > reach {
		if e.status.Mode&mob.ModeCanMove == 0 {
			z.mobUnlockTarget(e)
			return true
		}
		if !canMove {
			return true
		}
		m.ai.SetState(mob.StateLoot, e.status.Mode)
		if !z.walkToBL(e, it, 0, false) {
			z.mobUnlockTarget(e)
		}
		return true
	}
	if !m.canLoot {
		// No loot slots: it cannot loot.
		z.mobUnlockTarget(e)
		return true
	}
	if e.target != nil {
		return true
	}
	g := it.item
	looted := fallItem{id: g.id, amount: g.amount, refine: g.refine, cards: g.cards, extra: g.extra, origin: g.origin, from: g.from}
	if len(m.loot) < lootItemSize {
		m.loot = append(m.loot, looted)
	} else {
		// The first item looted is lost.
		m.loot = append(m.loot[1:], looted)
	}
	z.entities.remove(it)
	z.takes = append(z.takes, take{e, it})
	z.removed = append(z.removed, removal{it, wathenav1.VanishReason_VANISH_REASON_PICKED_UP})
	z.mobUnlockTarget(e)
	return true
}

// mobSlaveThink is mob_ai_sub_hard_slavemob: a slave follows its master
// and takes its master's target. It reports whether the slave acted.
func (z *Zone) mobSlaveThink(e *entity) bool {
	m := e.monster
	bl := m.master
	if bl == nil || bl.dead || !z.entities.has(bl) {
		e.hp = 0
		e.hpChanged = true
		z.die(e, nil)
		return true
	}
	if e.status.Mode&mob.ModeCanMove != 0 {
		dist := chebyshev(e, bl)
		if m.target != nil {
			// A player's slave comes back from too far.
			if bl.player != nil && dist > 5 {
				z.mobUnlockTarget(e)
				z.walkToBL(e, bl, slaveDistance, false)
				return true
			}
			return false
		}
		if (dist > slaveDistance || dist == 0) && z.unitCanMove(e) {
			if x, y, ok := z.searchFreeCellReach(bl.x, bl.y, slaveDistance, slaveDistance, nil); ok {
				if steps, ok := path.Search(z.grid, e.x, e.y, x, y, false); ok && len(steps) > 0 {
					e.target = nil
					m.chaseTo = nil
					e.path = steps
					e.stepStart = z.now
					e.walkChanged = true
					m.walked = 0
					return true
				}
				z.stopWalking(e)
				return false
			}
		}
	}
	// The slave takes its master's target.
	if z.now-m.lastLink >= minMobLink && m.target == nil {
		var tbl *entity
		switch {
		case bl.target != nil && bl.repeat:
			tbl = bl.target
		case bl.monster != nil && bl.monster.chaseTo != nil && bl.monster.attackContinue:
			tbl = bl.monster.chaseTo
		case bl.cast != nil && bl.cast.target != nil:
			if isEnemy(e, bl.cast.target) {
				tbl = bl.cast.target
			}
		case bl.monster != nil && bl.monster.target != nil:
			tbl = bl.monster.target
		}
		if tbl != nil {
			m.lastLink = z.now
			if z.canTargetAI(e, tbl) {
				m.target = tbl
				return true
			}
		}
	}
	return false
}

// searchFreeCellReach is map_search_freecell with flags 1 and 2 when
// from is set: a random walkable cell within rx and ry of a cell, which
// from can walk to.
func (z *Zone) searchFreeCellReach(bx, by, rx, ry int, from *entity) (int, int, bool) {
	if rx == 0 && ry == 0 {
		return bx, by, !z.grid.NoPass(bx, by)
	}
	w, h := z.grid.Width, z.grid.Height
	edgeValid := min(mapEdgeSize, 5)
	for range 50 {
		x := z.rndValue(bx-rx, bx+rx)
		y := z.rndValue(by-ry, by+ry)
		if x == bx && y == by {
			continue
		}
		if x < edgeValid || x > w-edgeValid || y < edgeValid || y > h-edgeValid {
			continue
		}
		if x >= w || y >= h || z.grid.NoPass(x, y) {
			continue
		}
		if from != nil {
			if _, ok := path.Search(z.grid, from.x, from.y, x, y, false); !ok {
				continue
			}
		}
		return x, y, true
	}
	return bx, by, false
}

// loseTarget is status_db.hasSCF with SCF_MOBLOSETARGET: a status that
// makes a monster forget its target, such as Hiding on itself.
func (z *Zone) loseTarget(e *entity) bool {
	if e.sc.Empty() || z.Statuses == nil {
		return false
	}
	for _, en := range e.sc.Sorted() {
		if d := z.Statuses.Get(en.Type); d != nil && d.Flags.Has(sc.FlagMobLoseTarget) {
			return true
		}
	}
	return false
}

// monsterTargetHook is the monster_target hook (proposal 0002, review
// decision 22): mods may pick another of the candidates, or none. pick is
// rAthena's choice. Without hooks it is the result.
func (z *Zone) monsterTargetHook(e *entity, candidates []*entity, pick *entity, reason string) *entity {
	if z.scripts == nil || !z.scripts.HasHooks("monster_target") {
		return pick
	}
	return z.runMonsterTargetHook(e, candidates, pick, reason)
}

// mobTarget is mob_target: the monster takes a target, unless its state
// keeps the one it has or it cannot target it. A provoked monster keeps
// to the one that provoked it.
func (z *Zone) mobTarget(e, bl *entity) {
	m := e.monster
	if m == nil || e.status == nil {
		return
	}
	if m.target != nil && !m.ai.CanChangeTarget(e.status.Mode, false) {
		return
	}
	if !z.canTargetAI(e, bl) {
		return
	}
	m.target = bl
	if m.provoke != nil && bl != m.provoke {
		m.provoke = nil
	}
	// An angry monster that is provoked answers as if hit.
	if m.provoke != nil && m.ai.Aggressive {
		m.ai.Aggressive = false
	}
}
