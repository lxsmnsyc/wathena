package zone

import (
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Skill attacks: rAthena's skill_attack, its knockback (skill_blown),
// the splash of skills that hit an area (SkillImplRecursiveDamageSplash
// and skill_area_sub), and the procs that skills trigger
// (skill_additional_effect and skill_onskillusage).

// Display flags of skill_attack, as rAthena's SD_ bits, and the flag that
// keeps a skill from consuming its items.
const (
	sdLevel           = 0x1000
	sdAnimation       = 0x2000
	sdSplash          = 0x4000
	sdPreamble        = 0x8000
	skillNoConsumeReq = 0x2000000
)

// Elements that skills read, as rAthena's e_element.
const (
	eleNeutral = 0
	eleFire    = 3
	eleUndead  = 9
)

// skillTarget is one hit's target, for the splash helpers.
type areaTemp struct {
	// count is skill_area_temp[0], the number of targets of a skill that
	// splits its damage. main is skill_area_temp[1], the first target.
	// x and y are skill_area_temp[4] and [5], the center of a ground
	// skill.
	count int
	main  *entity
	x, y  int
}

// skillAttack is skill_attack: one skill hit from src on target. dsrc is
// where the hit comes from: src, or the cell of a ground skill. flag
// holds the SD_ bits and, in its low 12 bits, the battle flag. It
// returns the damage dealt.
func (z *Zone) skillAttack(typ int, src, dsrc, target *entity, d *skill.Def, lv, flag int) int64 {
	if lv <= 0 || target == nil || target.dead || !z.entities.has(target) || target.status == nil || src == nil {
		return 0
	}
	if target.monster != nil && target.status.Mode&modeSkillImmune != 0 {
		return 0
	}
	if src != dsrc {
		// A ground skill checks the target as the caster would.
		if !z.statusCheckSkillUse(src, target, d, 2) {
			return 0
		}
	} else if flag&sdAnimation != 0 && d.NK.Has(battle.NKSplash) {
		// Splash hits skip hidden units.
		if !z.statusCheckSkillUse(src, target, d, 2) {
			return 0
		}
	}
	// Trick Dead takes no skill damage.
	if target.sc.Has(sc.SCTrickdead) {
		return 0
	}
	res := z.calcSkillAttack(typ, src, dsrc, target, d, lv, flag&0xFFF, nil)
	return z.applySkillAttack(src, dsrc, target, d, lv, flag, res)
}

// calcSkillAttack is battle_calc_attack for a skill, in the zone's mode.
// impl, when not nil, replaces the skill's own formulas, as for a mod's
// skill.
func (z *Zone) calcSkillAttack(typ int, src, dsrc, target *entity, d *skill.Def, lv, flag int, impl battle.SkillImpl) battle.Result {
	srcSt := src.status
	if srcSt == nil {
		srcSt = &npcStatus
	}
	bsk := d.Battle(srcSt, lv, flag, chebyshev(src, target))
	if dsrc != src {
		// A hit from a ground unit shows no attack motion.
		bsk.Ground = true
	}
	if impl != nil {
		bsk.Impl = impl
	}
	bsk.Impl = z.hookSkillRatio(src, target, d, lv, bsk.Impl)
	tgt := z.defender(target)
	if z.renewal() {
		return battle.SkillAttack(typ, srcSt, tgt, bsk, z.rand)
	}
	return battle.SkillAttackPreRenewal(typ, srcSt, tgt, bsk, z.rand)
}

// applySkillAttack is the part of skill_attack after the calculation:
// the knockback count, the damage, the effects of the hit, and the
// knockback.
func (z *Zone) applySkillAttack(src, dsrc, target *entity, d *skill.Def, lv, flag int, res battle.Result) int64 {
	damage := res.Damage
	hits := max(res.Hits, 1)
	// Only a hit that dealt at least one damage per hit pushes.
	if damage < int64(hits) {
		res.Knockback = 0
	}
	if d.ID == skill.IDFireWall && target.status != nil &&
		(target.status.DefEle == eleFire || target.status.DefEle == eleUndead) {
		// Fire Wall's modifyDamageData: no knockback on fire or undead,
		// which do not flinch for 2 seconds.
		res.Knockback = 0
		target.endure = z.now + 2000
	}
	// A skill with a walk delay also delays the next normal attack.
	if wd := d.AfterCastWalkDelay.At(lv); wd > 0 && res.Kind != battle.LuckyDodge {
		src.nextAttack = max(src.nextAttack, z.now+int64(wd))
	}
	delay := 0
	if dsrc == src && d.Target != skill.TargetGround && src.status != nil {
		delay = src.status.AMotion
	}
	z.skillDamage(src, dsrc, target, d, lv, res, delay)
	if flag&4 == 0 && res.Knockback > 0 && !target.dead {
		z.skillAttackBlow(src, dsrc, target, res.Knockback, d)
	}
	if damage > 0 && src.player != nil && !src.dead {
		z.skillUsage(src, target, d, lv)
	}
	return damage
}

// skillDamage is battle_damage for a skill hit: the damage, the hit that
// viewers see, and the effects: cast interruption, statuses that damage
// ends, procs, and the monster's retaliation.
func (z *Zone) skillDamage(src, dsrc, target *entity, d *skill.Def, lv int, res battle.Result, delay int) {
	damage := res.Damage
	if src.player != nil || target.player != nil {
		z.logDamage(src, target, damage)
	}
	if damage > 0 {
		target.hp -= damage
		target.hpChanged = true
	}
	z.hits = append(z.hits, hit{source: dsrc, caster: src, target: target, result: res, delay: delay,
		hpPermille: target.hpPermille(), skill: d})
	z.later(func() { z.itemHitEvents(src, target, res) })
	landed := !res.Blocked && (res.Kind == battle.Hit || res.Kind == battle.Critical)
	if landed && damage > 0 {
		z.damaged(target, damage, src.monster != nil)
		z.castCancel(target, true)
	}
	if target.hp <= 0 {
		z.die(target, src)
		return
	}
	if !res.Blocked && res.Kind != battle.Miss && res.Kind != battle.LuckyDodge {
		z.skillAdditionalEffect(src, target, d, lv, res, landed)
		if damage > 0 {
			z.counterEffectFlags(src, target, res.Flag)
			z.autoSpellWhenHit(src, target, res.Flag, d.NK)
		}
	}
	// mob_attacked: a monster that can attack strikes back.
	if target.monster != nil && target.target == nil && target.status.Mode&modeCanAttack != 0 && !src.dead &&
		isEnemy(target, src) {
		target.target, target.repeat = src, true
		if len(target.path) > 0 {
			target.path = target.path[:0]
			target.stopped = true
		}
		target.nextAttack = max(target.nextAttack, z.now+int64(target.status.DMotion))
	}
}

// skillAdditionalEffect is skill_additional_effect for a skill hit: the
// caster's cause_status bonuses, those on this skill, and the skill's
// own effects when the hit connected.
func (z *Zone) skillAdditionalEffect(src, target *entity, d *skill.Def, lv int, res battle.Result, landed bool) {
	if target.dead {
		return
	}
	z.additionalEffectFlags(src, target, res.Flag, d.ID)
	if !landed {
		return
	}
	if impl, ok := skillImpls[d.ID].(additionalEffecter); ok {
		impl.additionalEffect(z, src, target, d, lv)
	}
	if !target.dead && !src.dead {
		z.autoSpellAttack(src, target, res.Flag)
	}
}

// skillAttackBlow is skill_attack_blow: the direction a skill pushes, then
// skill_blown.
func (z *Zone) skillAttackBlow(src, dsrc, target *entity, count int, d *skill.Def) {
	if count == 0 || target == dsrc || target.dead {
		return
	}
	dir := -1
	switch d.ID {
	case skill.IDFireWall:
		// Backwards from the way the target faces.
		dir = rDir(target)
	case skill.IDCartRevolution:
		// cart_revo_knockback: officially to the west.
		dir = 6
	case skill.IDArrowShower:
		// arrow_shower_knockback: away from the cast center.
		dir = mapCalcDir(target, z.area.x, z.area.y)
	}
	z.skillBlown(dsrc, target, count, dir, false)
}

// rDir is unit_getdir: an entity's facing as rAthena's directions (0
// north, counterclockwise).
func rDir(e *entity) int { return (4 - e.dir + 8) % 8 }

// mapCalcDir is map_calc_dir: the direction from src toward a cell, as
// rAthena's directions, with slopes of 3 to 1. The same cell gives east
// (knockback_left).
func mapCalcDir(src *entity, x, y int) int {
	dx, dy := x-src.x, y-src.y
	switch {
	case dx == 0 && dy == 0:
		return 6
	case dx >= 0 && dy >= 0:
		switch {
		case dx >= dy*3:
			return 6
		case dx*3 < dy:
			return 0
		}
		return 7
	case dx >= 0 && dy <= 0:
		switch {
		case dx >= -dy*3:
			return 6
		case dx*3 < -dy:
			return 4
		}
		return 5
	case dx <= 0 && dy <= 0:
		switch {
		case dx*3 >= dy:
			return 4
		case dx < dy*3:
			return 2
		}
		return 3
	}
	switch {
	case -dx*3 <= dy:
		return 0
	case -dx > dy*3:
		return 2
	}
	return 1
}

// skillBlown is skill_blown and unit_blown: the target slides count
// cells away from src, or opposite to dir, and stops at the first cell
// it cannot enter (path_blown_halt). ignoreImmune is
// BLOWN_IGNORE_NO_KNOCKBACK. It returns the cells moved.
func (z *Zone) skillBlown(src, target *entity, count, dir int, ignoreImmune bool) int {
	if count <= 0 || target.dead {
		return 0
	}
	if !ignoreImmune {
		if target.status != nil && target.monster != nil && target.status.Mode&modeKnockbackImm != 0 {
			return 0
		}
		if p := target.player; p != nil && p.bonuses.NoKnockback {
			return 0
		}
	}
	if dir == -1 {
		dir = mapCalcDir(target, src.x, src.y)
	}
	dx, dy := 0, 0
	if dir >= 0 && dir < 8 {
		dx, dy = -dirX[dir], -dirY[dir]
	}
	count = min(count, 25)
	x, y := target.x, target.y
	moved := 0
	for range count {
		nx, ny := x+dx, y+dy
		if z.grid.NoPass(nx, ny) {
			break
		}
		x, y = nx, ny
		moved++
	}
	if moved == 0 {
		return 0
	}
	target.path = target.path[:0]
	target.x, target.y = x, y
	target.stopped = true
	z.skillUnitMove(target)
	return moved
}

// areaTargets is map_foreachinrange or map_foreachinarea over characters
// (BL_CHAR) for a skill: the units in the square of radius r around a
// cell, with the wall check from the center when wall is set, that are
// enemies of src. Units in the order the zone added them, as the queries
// of decision 18.
func (z *Zone) areaTargets(src *entity, x, y, r int, wall bool) []*entity {
	q := &query{area: areaRadius, x: x, y: y, radius: r, kinds: queryPlayers | queryMonsters,
		enemiesOf: enemySide(src), lineOfSight: wall}
	return z.findEntities(q)
}

// enemySide is the unit whose enemies a skill hits: the caster.
func enemySide(src *entity) *entity { return src }

// splashDamager is a splash skill that changes how each target is hit,
// as SkillImplRecursiveDamageSplash::splashDamage.
type splashDamager interface {
	splashDamage(z *Zone, src, target *entity, d *skill.Def, lv, flag int)
}

// recursiveSplash is SkillImplRecursiveDamageSplash::castendDamageId on
// the first target: it counts the targets of a skill that splits its
// damage, then hits every enemy around the target, the target included.
func (z *Zone) recursiveSplash(src, target *entity, d *skill.Def, lv, flag int) {
	z.area = areaTemp{main: target}
	size := d.SplashFor(lv)
	if flag&sdLevel != 0 || d.NK.Has(battle.NKSplashSplit) {
		// map_foreachinallrange: counted without the wall check.
		z.area.count = max(len(z.areaTargets(src, target.x, target.y, size, false)), 1)
	}
	for _, t := range z.areaTargets(src, target.x, target.y, size, true) {
		sflag := z.area.count & 0xFFF
		if flag&sdLevel != 0 {
			sflag |= sdLevel
		}
		if t != z.area.main {
			sflag |= sdAnimation
		}
		if s, ok := skillImpls[d.ID].(splashDamager); ok {
			s.splashDamage(z, src, t, d, lv, sflag)
			continue
		}
		z.skillAttack(d.Type, src, src, t, d, lv, sflag)
	}
}

// groundSplash is SkillImplRecursiveDamageSplash::castendPos2: every
// enemy in the square around a cell takes a hit. skill_area_temp[0] and
// [1] keep what an earlier cast left there; they are 0 here, which only
// changes the animation flag.
func (z *Zone) groundSplash(src *entity, d *skill.Def, lv, x, y int) {
	z.area = areaTemp{x: x, y: y}
	size := d.SplashFor(lv)
	for _, t := range z.areaTargets(src, x, y, size, true) {
		z.skillAttack(d.Type, src, src, t, d, lv, sdAnimation)
	}
}

// skillUsage is skill_onskillusage: a skill that dealt damage, or a
// support skill, may trigger the caster's autospell on skill bonuses.
func (z *Zone) skillUsage(src, target *entity, d *skill.Def, lv int) {
	z.autoSpellOnSkill(src, target, d, lv)
}

// clearShot reports whether a straight shot from one cell to another is
// clear, as path_search_long with CELL_CHKWALL.
func (z *Zone) clearShot(x0, y0, x1, y1 int) bool {
	ok, _ := path.Shoot(z.grid, x0, y0, x1, y1)
	return ok
}
