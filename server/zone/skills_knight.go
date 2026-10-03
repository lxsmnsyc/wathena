package zone

import (
	"math"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// The Knight and Lord Knight skills, as the SkillImpl classes of
// rAthena's src/map/skills/swordman, and the Peco Peco that Knights
// ride (pc_setriding). The damage formulas are in
// server/skill/knight.go; the statuses' effects are in server/status,
// server/statuschange and server/battle.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDPierce:          attackSkill{},
		skill.IDBrandishSpear:   brandishSpear{},
		skill.IDSpearStab:       spearStab{},
		skill.IDSpearBoomerang:  attackSkill{},
		skill.IDTwoHandQuicken:  statusSkill{},
		skill.IDAutoCounter:     autoCounter{},
		skill.IDBowlingBash:     bowlingBash{},
		skill.IDOneHandQuicken:  statusSkill{},
		skill.IDChargeAttack:    chargeAttack{},
		skill.IDAuraBlade:       statusSkill{},
		skill.IDParrying:        statusSkill{},
		skill.IDLKConcentration: statusSkill{},
		skill.IDTensionRelax:    tensionRelax{},
		skill.IDBerserk:         statusSkill{},
		skill.IDSpiralPierce:    spiralPierce{},
		skill.IDHeadCrush:       headCrush{},
		skill.IDJointBeat:       jointBeat{},
	})
}

// mathDistance is distance_math_bl: the straight distance between two
// units, truncated.
func mathDistance(a, b *entity) int {
	dx, dy := a.x-b.x, a.y-b.y
	return int(math.Sqrt(float64(dx*dx + dy*dy)))
}

// cellWall is map_getcell with CELL_CHKWALL: a cell that neither lets
// units walk nor lets shots through.
func (z *Zone) cellWall(x, y int) bool {
	g := z.grid
	if x < 0 || x >= g.Width || y < 0 || y >= g.Height {
		return false
	}
	return g.Cells[x+y*g.Width]&(path.Walkable|path.Shootable) == 0
}

// unitsInDir is map_foreachindir over characters, with skill_area_sub's
// enemy check: the enemies of src in the path from (x0, y0) toward
// (x1, y1), length cells long starting offset cells out, and rng cells
// to each side. Diagonal paths count in half cells so that paths of
// different offsets fit together. skill_eightpath_same_cell is on, so
// units on the start cell count. Every unit needs a clear shot from the
// start. Players come first, then monsters, as the block lists.
func (z *Zone) unitsInDir(src *entity, x0, y0, x1, y1, rng, length, offset int) []*entity {
	if rng < 0 || length < 1 || offset < 0 {
		return nil
	}
	dir := mapCalcDir(&entity{x: x0, y: y0}, x1, y1)
	dx, dy := dirX[dir], dirY[dir]
	diagonal := dir%2 == 1
	if offset > 0 && diagonal {
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
	side := (dir + 2) % 8
	mx0 -= abs(rng * dirX[side])
	my0 -= abs(rng * dirY[side])
	mx1 += abs(rng * dirX[side])
	my1 += abs(rng * dirY[side])
	var players, monsters []*entity
	for _, t := range z.findEntities(&query{area: areaRect, x0: mx0, y0: my0, x1: mx1, y1: my1,
		kinds: queryPlayers | queryMonsters, enemiesOf: src}) {
		rx, ry := (t.x-x0)*dx, (t.y-y0)*dy
		if diagonal {
			if rx+ry < offset || rx+ry > 2*(length+offset/2-1) {
				continue
			}
			if abs(rx-ry) > 2*rng {
				continue
			}
		}
		if !z.clearShot(x0, y0, t.x, t.y) {
			continue
		}
		if t.player != nil {
			players = append(players, t)
		} else {
			monsters = append(monsters, t)
		}
	}
	return append(players, monsters...)
}

// brandishSpear is SkillBrandishSpear: rows of the area in front of the
// caster, toward the target. In pre-renewal, the rows nearer the caster
// from levels 4, 7 and 10 deal more, as the flag tells the formula.
type brandishSpear struct{}

func (brandishSpear) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	splash, maxCount := d.SplashFor(lv), d.ActiveInstance.At(lv)
	hit := func(rows []*entity, f int) {
		for _, t := range rows {
			brandishSpear{}.castendDamage(z, src, t, d, lv, &f)
		}
	}
	if z.renewal() {
		hit(z.unitsInDir(src, src.x, src.y, target.x, target.y, splash, maxCount, 0), *flag)
		return
	}
	z.area = areaTemp{main: target}
	rowFlag := func(n int) int {
		if src.player != nil {
			return n
		}
		return 0
	}
	if lv >= 10 {
		hit(z.unitsInDir(src, src.x, src.y, target.x, target.y, splash, 1, maxCount-1), *flag|rowFlag(3))
	}
	if lv >= 7 {
		hit(z.unitsInDir(src, src.x, src.y, target.x, target.y, splash, 1, maxCount-2), *flag|rowFlag(2))
	}
	if lv >= 4 {
		hit(z.unitsInDir(src, src.x, src.y, target.x, target.y, splash, 1, maxCount-3), *flag|rowFlag(1))
	}
	hit(z.unitsInDir(src, src.x, src.y, target.x, target.y, splash, maxCount-3, 0), *flag)
}

func (brandishSpear) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if z.renewal() || target == z.area.main {
		z.skillAttack(d.Type, src, src, target, d, lv, *flag)
		return
	}
	z.skillAttack(d.Type, src, src, target, d, lv, *flag|sdAnimation)
}

// spearStab is SkillSpearStab: the target and every enemy on the 4
// cells from the target toward the caster are hit and pushed away.
type spearStab struct{}

func (spearStab) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	x, y := target.x, target.y
	dir := mapCalcDir(target, src.x, src.y)
	z.area = areaTemp{main: target, count: d.Knockback.At(lv)}
	if z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, 0) != 0 {
		z.skillBlown(src, target, z.area.count, -1, false)
	}
	for range 4 {
		for _, t := range z.findEntities(&query{area: areaCells, cells: [][2]int{{x, y}},
			kinds: queryPlayers | queryMonsters, enemiesOf: src}) {
			if t == z.area.main {
				continue
			}
			if z.skillAttack(battle.TypeWeapon, src, src, t, d, lv, sdAnimation) != 0 {
				z.skillBlown(src, t, z.area.count, -1, false)
			}
		}
		x += dirX[dir]
		y += dirY[dir]
	}
}

// autoCounter is SkillCounterAttack: the caster waits for the next hit.
// rAthena shows the skill through a timer skill 100 ms later; the zone
// shows it at once.
type autoCounter struct{}

func (autoCounter) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// autoCounterBlock is Auto Counter's part of battle_weapon_attack: a
// normal attack from the front, or from the same cell, on a unit under
// SC_AUTOCOUNTER misses, ends the status, and the unit counters with a
// critical hit. It reports whether the attack was blocked.
func (z *Zone) autoCounterBlock(src, target *entity) bool {
	e := target.sc.Get(sc.SCAutocounter)
	if e == nil || z.Skills == nil {
		return false
	}
	d := z.Skills.Get(skill.IDAutoCounter)
	if d == nil || !z.statusCheckSkillUse(target, src, d, 1) {
		return false
	}
	dir := mapCalcDir(target, src.x, src.y)
	dist := chebyshev(src, target)
	reach := 1
	if target.status != nil {
		reach = target.status.RHW.Range + 1
	}
	if dist > 0 && (!facesDir(dir, rDir(target)) || dist > reach) {
		return false
	}
	lv := e.Val1
	z.castCancel(target, false)
	amotion := 0
	if src.status != nil {
		amotion = src.status.AMotion
	}
	z.hits = append(z.hits, hit{source: src, target: target, result: battle.Result{Kind: battle.Miss, Hits: 1},
		delay: amotion, hpPermille: target.hpPermille()})
	z.scEnd(target, sc.SCAutocounter)
	z.skillAttack(battle.TypeWeapon, target, target, src, d, lv, 0)
	return true
}

// facesDir is !map_check_dir: whether a direction is the other one or
// next to it.
func facesDir(a, b int) bool {
	diff := (a - b + 8) % 8
	return diff == 0 || diff == 1 || diff == 7
}

// bowlingBash is SkillBowlingBash. Renewal: a splash around the target
// that hits more often with a two-handed sword. Pre-renewal: the target
// rolls away from the caster, hitting what it meets, which rolls on in a
// random direction (bowling_bash_area 0: the gutter lines of 40 cells).
type bowlingBash struct{}

func (bowlingBash) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if z.renewal() {
		size := d.SplashFor(lv)
		count := len(z.areaTargets(src, target.x, target.y, size, false))
		f := count
		if count > 0 {
			f |= sdAnimation
		}
		for _, t := range z.areaTargets(src, target.x, target.y, size, false) {
			z.skillAttack(d.Type, src, src, t, d, lv, f)
		}
		return
	}
	depth := *flag & 0xFFF
	c := (lv - depth + 1) / 2
	minX := (src.x - c) - (src.x-c)%40
	minX = max(minX, 0)
	maxX := minX + 39
	minY := (src.y - c) - (src.y-c)%40
	minY = max(minY, 0)
	maxY := minY + 39
	var dir int
	if depth > 0 {
		if target.x < minX || target.x > maxX || target.y < minY || target.y > maxY {
			return
		}
		if z.bowled[target] {
			return
		}
		dir = z.rand.IntN(8)
	} else {
		z.bowled = map[*entity]bool{}
		dir = (rDir(src) + 4) % 8
	}
	z.bowled[target] = true
	animation := func(f int) int {
		if depth > 0 {
			return sdAnimation | f
		}
		return f
	}
	tx, ty := target.x, target.y
	for range c {
		tx -= dirX[dir]
		ty -= dirY[dir]
		if z.cellWall(tx, ty) {
			break
		}
		z.skillBlown(src, target, 1, dir, false)
		ax0, ay0 := max(minX, tx-1), max(minY, ty-1)
		ax1, ay1 := min(maxX, tx+1), min(maxY, ty+1)
		if ax0 > ax1 || ay0 > ay1 {
			continue
		}
		q := &query{area: areaRect, x0: ax0, y0: ay0, x1: ax1, y1: ay1, kinds: queryPlayers | queryMonsters, enemiesOf: src}
		count := len(z.findEntities(q))
		if count == 0 {
			continue
		}
		for _, t := range z.findEntities(q) {
			f := *flag + 1
			bowlingBash{}.castendDamage(z, src, t, d, lv, &f)
		}
		// The target hits what it rolled into.
		if target.x >= minX && target.x <= maxX && target.y >= minY && target.y <= maxY {
			z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, animation(count))
		}
		break
	}
	z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, animation(0))
}

// chargeAttack is SkillChargeAttack: the caster dashes next to the target
// and, with a straight path, hits and pushes it. Pre-renewal's damage and
// knockback grow with the distance; renewal's knockback is the skill's.
type chargeAttack struct{}

func (chargeAttack) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	straight := z.clearShot(src.x, src.y, target.x, target.y)
	dist := mathDistance(src, target)
	if z.renewal() {
		dist = d.Knockback.At(lv)
	}
	dir := mapCalcDir(target, src.x, src.y)
	// skill_check_unit_movepos: battlegrounds and guild wars do not
	// exist yet, so only the path decides.
	z.unitMovePos(src, target.x+dirX[dir], target.y+dirY[dir])
	if straight && z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, dist) != 0 {
		// PvP maps, which push 2 more in renewal, do not exist yet.
		z.skillBlown(src, target, dist, dir, false)
	}
}

// tensionRelax is SkillRelax: SC_TENSIONRELAX with Duration2 as val4.
type tensionRelax struct{}

func (tensionRelax) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart4(src, target, d.Status, 100, lv, 0, 0, d.Duration2.At(lv), int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// spiralPierce is SkillSpiralPierce: a weapon hit that holds the target
// in place, unless it is a monster immune to statuses.
type spiralPierce struct{ attackSkill }

func (spiralPierce) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if target.player != nil || target.monster != nil && target.status.Mode&modeStatusImmune == 0 {
		z.scStart(src, target, sc.SCAnkle, 100, 0, int64(d.Duration2.At(lv)))
	}
}

// headCrush is SkillTraumaticBlow: a weapon hit that may bleed. It fails
// on bosses.
type headCrush struct{}

func (headCrush) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if target.status != nil && target.status.Class == battle.ClassBoss {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	z.skillAttack(d.Type, src, src, target, d, lv, *flag)
}

func (headCrush) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	st := target.status
	if battle.IsUndead(st.Race, st.DefEle) || st.Race == battle.RaceDemon {
		return
	}
	z.scStart4(src, target, sc.SCBleeding, 50, lv, int(z.unit(src).Ref().ID), 0, 0, int64(d.Duration2.At(lv)))
}

// jointBeat is SkillVitalStrike: a weapon hit on a random joint, which
// may break it. A broken neck stays the target of every later hit.
type jointBeat struct{}

func (jointBeat) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	f := 1 << z.rand.IntN(6)
	if f != sc.BreakNeck && target.sc.Val2(sc.SCJointbeat)&sc.BreakNeck != 0 {
		f = sc.BreakNeck
	}
	*flag = f
	if z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, f) == 0 {
		return
	}
	en := z.statusEngine()
	if en == nil {
		return
	}
	str := 0
	if target.status != nil {
		str = target.status.Str
	}
	su := z.unit(src)
	en.Start(su, z.unit(target), sc.Start{Type: sc.SCJointbeat, Rate: (50*(lv+1) - (270*str)/100) * 10, Val1: lv,
		Val2: f & sc.BreakFlags, Val3: int(su.Ref().ID), Duration: int64(d.Duration2.At(lv)), Reason: sc.ReasonSkill,
		Source: su.Ref()})
}
