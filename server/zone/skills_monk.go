package zone

import (
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// The Monk and the Champion: spirit spheres, combos, Blade Stop, Asura
// Strike and the other skills of rAthena's src/map/skills/acolyte
// classes for these jobs, with the parts of pc.cpp, skill.cpp, unit.cpp,
// battle.cpp and status.cpp that they need. Their damage formulas are in
// server/skill/monk.go.
//
// Viewers see the spheres in rAthena (clif_spiritball), and the combo
// window (clif_combo_delay) and Blade Stop (clif_bladestop). The protocol
// has no fields for these yet, so the zone keeps them without showing
// them.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDCallSpirits:      callSpirits{},
		skill.IDAbsorbSpirits:    absorbSpirits{},
		skill.IDTripleAttack:     tripleAttack{},
		skill.IDBodyRelocation:   bodyRelocation{},
		skill.IDInvestigate:      bladeStopAttack{},
		skill.IDFingerOffensive:  fingerOffensive{},
		skill.IDSteelBody:        statusSkill{},
		skill.IDBladeStop:        statusSkill{},
		skill.IDExplosionSpirits: statusSkill{},
		skill.IDExtremityFist:    extremityFist{},
		skill.IDChainCombo:       chainCombo{},
		skill.IDComboFinish:      comboFinish{},
		skill.IDKiTranslation:    kiTranslation{},
		skill.IDBalkyoung:        balkyoung{},
		skill.IDPalmStrike:       palmStrike{},
		skill.IDTigerFist:        tigerFist{},
		skill.IDChainCrush:       chainCrush{},
		skill.IDSoulCollect:      soulCollect{},
	})
}

const (
	// maxSpiritBall is MAX_SPIRITBALL.
	maxSpiritBall = 15
	// classBoss is CLASS_BOSS, a monster's class.
	classBoss = 1
)

// Map job IDs that the Monk's skills read, as rAthena's e_mapid.
const (
	mapidGunslinger = 0x8
	mapidRebellion  = jcSecond1 | mapidGunslinger
	mapidMonk       = jcSecond2 | 0x4
)

// Skill IDs of other jobs that the Monk's rules read.
const (
	idTKStormKick   = 413  // TK_STORMKICK
	idTKDownKick    = 415  // TK_DOWNKICK
	idTKTurnKick    = 417  // TK_TURNKICK
	idTKCounter     = 419  // TK_COUNTER
	idTKJumpKick    = 421  // TK_JUMPKICK
	idHTPower       = 499  // HT_POWER
	idSRDragonCombo = 2326 // SR_DRAGONCOMBO
	idSRFallenEmp   = 2329 // SR_FALLENEMPIRE
	idSRTigerCannon = 2330 // SR_TIGERCANNON
	idSRGateOfHell  = 2343 // SR_GATEOFHELL
)

// spheres are a player's spirit spheres.
type spheres struct {
	// spiritTimers are when each sphere ends, oldest first: their count
	// is sd->spiritball, and they are sd->spirit_timer.
	spiritTimers []int64
	// spiritBallOld is sd->spiritball_old: the spheres a skill counted
	// when it started.
	spiritBallOld int
}

// bladeStopState is a unit's side of Blade Stop.
type bladeStopState struct {
	// bladeStopPartner is the other unit, SC_BLADESTOP's val4.
	bladeStopPartner *entity
}

// spiritBalls is sd->spiritball.
func (p *Player) spiritBalls() int { return len(p.spiritTimers) }

// addSpiritBall is pc_addspiritball: a sphere for interval ms. At the
// maximum, the oldest sphere goes first.
func (z *Zone) addSpiritBall(p *Player, interval int64, maxBalls int) {
	maxBalls = min(maxBalls, maxSpiritBall)
	if n := len(p.spiritTimers); n > 0 && n >= maxBalls {
		p.spiritTimers = slices.Delete(p.spiritTimers, 0, 1)
	}
	due := z.now + interval
	// The new timer goes before the first one that ends later.
	i := 0
	for i < len(p.spiritTimers) && p.spiritTimers[i] <= due {
		i++
	}
	p.spiritTimers = slices.Insert(p.spiritTimers, i, due)
	z.syncBattlePlayer(p.entity)
}

// delSpiritBall is pc_delspiritball: the oldest count spheres go.
func (z *Zone) delSpiritBall(p *Player, count int) {
	if len(p.spiritTimers) == 0 || count <= 0 {
		return
	}
	count = min(count, len(p.spiritTimers))
	p.spiritTimers = slices.Delete(p.spiritTimers, 0, count)
	z.syncBattlePlayer(p.entity)
}

// advanceSpiritBalls is pc_spiritball_timer: spheres whose time ran out
// go, oldest first.
func (z *Zone) advanceSpiritBalls() {
	for e := range z.entities.all() {
		p := e.player
		if p == nil {
			continue
		}
		n := 0
		for n < len(p.spiritTimers) && p.spiritTimers[n] <= z.now {
			n++
		}
		if n > 0 {
			z.delSpiritBall(p, n)
		}
	}
}

// syncBattlePlayer copies what attacks read of a player and that changes
// outside the stat calculation into its battle status: the spheres and
// the current SP.
func (z *Zone) syncBattlePlayer(e *entity) {
	p := e.player
	if p == nil || e.status == nil || e.status.Player == nil {
		return
	}
	bp := e.status.Player
	bp.SpiritBall = p.spiritBalls()
	bp.SpiritBallOld = p.spiritBallOld
	bp.SP = int(p.sp)
}

// spheresLeft are a player's spheres as times left, which a map change
// carries.
func (z *Zone) spheresLeft(p *Player) []int64 {
	var out []int64
	for _, due := range p.spiritTimers {
		out = append(out, max(due-z.now, 0))
	}
	return out
}

// mapID is pc_jobid2mapid for a player, or 0.
func (z *Zone) mapID(p *Player) uint64 {
	if z.Status == nil {
		return 0
	}
	m, _ := z.Status.MapID(p.char.Job)
	return m
}

// soulLinkedMonk reports whether a unit has the Monk's soul link
// (SC_SPIRIT with SL_MONK).
func soulLinkedMonk(l *sc.List) bool {
	return l.Has(sc.SCSpirit) && l.Val2(sc.SCSpirit) == skill.IDSoulLinkMonk
}

// monkNoSPRegen is the part of status_calc_regen_rate that stops a
// Monk's natural SP regen: under Asura Strike, and in pre-renewal under
// Fury too, unless soul linked.
func (z *Zone) monkNoSPRegen(p *Player) bool {
	l := p.entity.sc
	if l.Empty() || z.mapID(p)&jcSecondMask != mapidMonk || soulLinkedMonk(l) {
		return false
	}
	if l.Has(sc.SCExtremityfist) {
		return true
	}
	return !z.skillRenewal(skill.IDExplosionSpirits) && l.Has(sc.SCExplosionspirits)
}

// furySPRegen is status_natural_heal's renewal rule that doubles a
// Monk's SP regen interval under Fury, unless soul linked.
func (z *Zone) furySPRegen(p *Player) bool {
	l := p.entity.sc
	return z.skillRenewal(skill.IDExplosionSpirits) && z.mapID(p)&jcSecondMask == mapidMonk && l.Has(sc.SCExplosionspirits) && !soulLinkedMonk(l)
}

// asuraNoItemSP is pc_itemheal's renewal rule: no SP from items after
// Asura Strike.
func (z *Zone) asuraNoItemSP(e *entity) bool {
	return z.rules().HealFormula && e.sc.Has(sc.SCExtremityfist)
}

// castBeginSpheres is the spirit sphere check of
// skill_check_condition_castbegin: the spheres a skill needs, which
// Finger Offensive lowers to those the player has.
func (z *Zone) castBeginSpheres(p *Player, d *skill.Def, need int) bool {
	balls := p.spiritBalls()
	if d.ID == skill.IDFingerOffensive && balls > 0 && balls < need {
		need = balls
	}
	return balls >= need
}

// consumeSpheres is the spirit sphere part of skill_consume_requirement
// with type 1: the skill's spheres go, or with -1 every sphere.
func (z *Zone) consumeSpheres(p *Player, d *skill.Def, need int) {
	if d.ID == skill.IDKiTranslation {
		// Spiritual Bestowment only uses spheres when it gives them.
		need = 0
	}
	switch {
	case need > 0:
		z.delSpiritBall(p, need)
	case need == -1:
		p.spiritBallOld = p.spiritBalls()
		z.delSpiritBall(p, p.spiritBalls())
	}
}

// bladeStopAllows is the SC_BLADESTOP part of status_check_skilluse: the
// skills a unit caught in Blade Stop may use, by its level.
func bladeStopAllows(lv, id int) bool {
	switch {
	case lv >= 5 && id == skill.IDExtremityFist:
		return true
	case lv >= 4 && id == skill.IDChainCombo:
		return true
	case lv >= 3 && id == skill.IDInvestigate:
		return true
	case lv >= 2 && id == skill.IDFingerOffensive:
		return true
	}
	return false
}

// skillIsCombo is skill_is_combo: 1 for a combo skill, 2 for a combo
// skill that takes the attack target even when its own target is set.
func skillIsCombo(id int) int {
	switch id {
	case skill.IDChainCombo, skill.IDComboFinish, skill.IDTigerFist, skill.IDChainCrush, skill.IDExtremityFist,
		idTKTurnKick, idTKStormKick, idTKDownKick, idTKCounter, idTKJumpKick, idHTPower, idSRDragonCombo:
		return 1
	case idSRFallenEmp, idSRTigerCannon, idSRGateOfHell:
		return 2
	}
	return 0
}

// comboTarget is the part of unit_skilluse_id2 that picks the target of
// a combo skill, and of a self skill that may not target its caster. It
// returns the target, nil when the skill cannot start, and whether the
// skill counts as a combo, which keeps the attack going.
func (z *Zone) comboTarget(src, target *entity, d *skill.Def, lv int) (*entity, bool) {
	combo := false
	if e := src.sc.Get(sc.SCCombo); e != nil && skillIsCombo(d.ID) != 0 &&
		(e.Val1 == d.ID || src.player != nil && z.checkCastBegin(src, d, lv)) {
		switch {
		case skillIsCombo(d.ID) == 2 && target == src && src.target != nil:
			target = src.target
		case e.Val2 != 0:
			// The combo's own target. The Monk's combos keep none: they
			// take the attack target.
		case target == src || src.target != nil:
			target = src.target
		}
		if d.Target == skill.TargetSelf && d.NK.Has(battle.NKNoDamage) {
			// Exploit fix.
			target = src
		}
		combo = true
	} else if target == src && d.Target == skill.TargetSelf && d.Has("NoTargetSelf") {
		// Auto-select the attack target.
		target = src.target
		combo = true
	}
	if src.player != nil && d.ID == skill.IDChainCombo && src.sc.Has(sc.SCBladestop) {
		// Raging Quadruple Blow hits the unit caught in Blade Stop.
		target = src.bladeStopPartner
	}
	return target, combo
}

// skillCombo is skill_combo: a skill ends the combo it came from, and a
// combo skill opens the window of the next one.
func (z *Zone) skillCombo(src, dsrc, target *entity, d *skill.Def, lv int) {
	if src.sc.Has(sc.SCCombo) {
		switch d.ID {
		case idTKTurnKick, idTKStormKick, idTKDownKick, idTKCounter:
			// The Taekwon ranking that extends the combo is not ported:
			// unit_cancel_combo.
			z.cancelCombo(src)
		default:
			// Ground skills are exceptions.
			if src == dsrc {
				z.scEnd(src, sc.SCCombo)
			}
		}
	}
	p := src.player
	if p == nil {
		// The combos of homunculi come with them.
		return
	}
	duration := int64(0)
	// The Monk's combos auto target (val2 0) and delay attacks and walks
	// (val3 0).
	nodelay := 0
	balls := p.spiritBalls()
	lvOf := p.skillLevel
	fury := src.sc.Has(sc.SCExplosionspirits)
	switch d.ID {
	case skill.IDTripleAttack:
		if lvOf(skill.IDChainCombo) > 0 || lvOf(idSRDragonCombo) > 0 {
			duration = 1
		}
	case skill.IDChainCombo:
		if lvOf(skill.IDComboFinish) > 0 && balls >= 1 {
			duration = 1
		}
	case skill.IDComboFinish:
		// SG_FRIEND's party bonus needs parties, which are not ported.
		switch {
		case lvOf(skill.IDTigerFist) > 0 && balls >= 1:
			duration = 1
		case lvOf(skill.IDChainCrush) > 0 && balls >= 2:
			duration = 1
		case lvOf(skill.IDExtremityFist) > 0 && balls >= 4 && fury:
			duration = 1
		}
	case skill.IDTigerFist:
		if lvOf(skill.IDChainCrush) > 0 && balls >= 2 {
			duration = 1
		}
	case skill.IDChainCrush:
		switch {
		case lvOf(skill.IDExtremityFist) > 0 && balls >= 1 && fury:
			duration = 1
		case !z.skillRenewal(skill.IDTigerFist) && lvOf(skill.IDTigerFist) > 0 && balls >= 1:
			duration = 1
		}
	case skill.IDDoubleStrafe:
		// The Hunter's Beast Strafing: neither attack nor walk delay.
		if lvOf(skill.IDBeastStrafe) > 0 {
			duration, nodelay = 2000, 1
		}
	}
	// The combos of the other jobs (the Sura's and the Star Emperor's)
	// come with their skills.
	if duration == 0 {
		return
	}
	if duration == 1 {
		// The window lasts until the caster may act again.
		duration = src.canAct - z.now
	}
	duration = max(int64(src.status.AMotion), duration)
	if !z.scStart4(src, src, sc.SCCombo, 100, d.ID, 0, nodelay, 0, duration) {
		return
	}
	// status_change_start's SC_COMBO case: no attacks, and no walking,
	// during the window.
	if e := src.sc.Get(sc.SCCombo); e != nil && (nodelay == 0 || nodelay == 2) {
		src.nextAttack = z.now + int64(e.Val4)
		if nodelay == 0 {
			z.setWalkDelay(src, int64(e.Val4))
		}
	}
}

// cancelCombo is unit_cancel_combo: the combo ends and the unit may
// attack after its attack motion.
func (z *Zone) cancelCombo(e *entity) bool {
	if !z.scEnd(e, sc.SCCombo) {
		return false
	}
	if e.status != nil {
		e.nextAttack = z.now + int64(e.status.AMotion)
	}
	return true
}

// weaponAttackSkills is the part of battle_weapon_attack before the
// attack's calculation that replaces a normal attack: Blade Stop catches
// it, or Raging Trifecta Blow fires instead. It reports whether the
// normal attack was replaced.
func (z *Zone) weaponAttackSkills(src, target *entity) bool {
	if z.Skills == nil {
		return false
	}
	if w := target.sc.Get(sc.SCBladestopWait); w != nil && (z.rules().BossProtection || !isBossClass(src)) &&
		(src.player != nil || chebyshev(src, target) <= 2) {
		lv := w.Val1
		d := z.Skills.Get(skill.IDBladeStop)
		duration := int64(d.Duration2.At(lv))
		if z.rules().BossProtection && isBossClass(src) {
			// Only 2 seconds for bosses.
			duration = 2000
		}
		z.scEnd(target, sc.SCBladestopWait)
		srcLv := 5
		if p := src.player; p != nil {
			srcLv = p.skillLevel(skill.IDBladeStop)
		}
		if z.scStart4(src, src, sc.SCBladestop, 100, srcLv, 0, 0, int(z.unit(target).Ref().ID), duration) {
			// The target is locked. Viewers see the attack miss.
			z.hits = append(z.hits, hit{source: src, target: target, result: battle.Result{Kind: battle.Miss, Hits: 1},
				delay: src.status.AMotion, hpPermille: target.hpPermille()})
			src.bladeStopPartner = target
			if z.scStart4(src, target, sc.SCBladestop, 100, lv, 0, 0, int(z.unit(src).Ref().ID), duration) {
				target.bladeStopPartner = src
			}
			return true
		}
	}
	p := src.player
	if p == nil {
		return false
	}
	lv := p.skillLevel(skill.IDTripleAttack)
	if lv <= 0 {
		return false
	}
	rate := 30
	if !z.skillRenewal(skill.IDTripleAttack) {
		rate = 30 - lv
	}
	if e := src.sc.Get(sc.SCSkillrateUp); e != nil && e.Val1 == skill.IDTripleAttack {
		rate += rate * e.Val2 / 100
		z.scEnd(src, sc.SCSkillrateUp)
	}
	if z.rand.IntN(100) >= rate {
		return false
	}
	d := z.Skills.Get(skill.IDTripleAttack)
	// The act delay applies here, since the skill skips
	// skill_castend_id.
	src.canAct = max(z.now+int64(d.Delay(z.caster(src), lv)), src.canAct)
	z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, 0)
	return true
}

// isBossClass reports whether a unit is of CLASS_BOSS.
func isBossClass(e *entity) bool {
	return e.monster != nil && e.status != nil && e.status.Class == classBoss
}

// monkStatusEnded is the part of status_change_end for the Monk's
// statuses: Blade Stop ends for both units.
func (z *Zone) monkStatusEnded(e *entity, entry *sc.Entry) {
	if entry.Type != sc.SCBladestop {
		return
	}
	o := e.bladeStopPartner
	e.bladeStopPartner = nil
	if entry.Val4 == 0 || o == nil {
		return
	}
	if oe := o.sc.Get(sc.SCBladestop); oe != nil && o.bladeStopPartner == e {
		oe.Val4 = 0
		o.bladeStopPartner = nil
		z.scEnd(o, sc.SCBladestop)
	}
}

// movePos is unit_movepos: the unit moves at once to a cell it can
// reach, with the easy path or a full path search, and stops walking and
// attacking. Viewers see it stop at the new cell.
func (z *Zone) movePos(e *entity, x, y int, easy bool) bool {
	if len(e.path) > 0 {
		e.path = e.path[:0]
		e.stopped = true
	}
	e.target = nil
	if x < 0 || y < 0 || x >= z.grid.Width || y >= z.grid.Height || z.grid.NoPass(x, y) {
		return false
	}
	if x != e.x || y != e.y {
		if _, ok := path.Search(z.grid, e.x, e.y, x, y, easy); !ok {
			return false
		}
	}
	e.dir = facingTo(x-e.x, y-e.y)
	e.x, e.y = x, y
	e.stopped = true
	z.skillUnitMove(e)
	return true
}

// blockSkill is skill_blockpc_start: a player cannot use a skill for a
// time.
func (z *Zone) blockSkill(p *Player, id int, ms int64) {
	if p.cooldowns == nil {
		p.cooldowns = map[int]int64{}
	}
	p.cooldowns[id] = z.now + ms
	p.pending = append(p.pending, cooldownEvent(id, ms))
}

// setSPZero is status_set_sp with 0.
func (z *Zone) setSPZero(e *entity) {
	p := e.player
	if p == nil {
		return
	}
	p.sp = 0
	if s := z.battleStats(p); s != nil {
		p.pending = append(p.pending, s)
	}
}

// asuraMove is the move of Asura Strike: 3 cells from the caster toward
// the target's direction, dir as map_calc_dir.
func (z *Zone) asuraMove(src *entity, dir int) {
	const cells = 3
	x, y := 0, 0
	switch {
	case dir > 0 && dir < 4:
		x = -cells
	case dir > 4:
		x = cells
	}
	switch {
	case dir > 2 && dir < 6:
		y = -cells
	case dir == 7 || dir < 2:
		y = cells
	}
	// clif_blown and clif_spiritball when it moves.
	z.movePos(src, src.x+x, src.y+y, true)
}

// asuraAfter is what Asura Strike does to its caster, whether it hit or
// failed: no SP, no SP regen for a while, and Fury and Blade Stop end.
func (z *Zone) asuraAfter(src *entity, d *skill.Def, lv int) {
	z.setSPZero(src)
	z.scStart(src, src, sc.SCExtremityfist, 100, lv, int64(d.Duration1.At(lv)))
	z.scEnd(src, sc.SCExplosionspirits)
	z.scEnd(src, sc.SCBladestop)
}

// asuraFailed is the failure path of skill_castend_id for Asura Strike:
// the cast costs its SP and spheres anyway, and the caster moves toward
// the target. Wall of Fog keeps the cost.
func (z *Zone) asuraFailed(src *entity, c *casting) {
	if src.player == nil || src.sc.Has(sc.SCFogwall) {
		return
	}
	d, lv := c.def, c.lv
	z.consumeRequirement(src, d, lv, 1)
	z.asuraAfter(src, d, lv)
	t := c.target
	if t == nil || !z.entities.has(t) {
		return
	}
	z.asuraMove(src, mapCalcDir(src, t.x, t.y))
	// clif_skill_damage with no damage.
	z.hits = append(z.hits, hit{source: src, target: t, result: battle.Result{Kind: battle.Hit, Hits: 1},
		delay: src.status.AMotion, hpPermille: t.hpPermille(), skill: d})
}

// callSpirits is SkillSummoningSpiritSphere.
type callSpirits struct{}

func (callSpirits) castBegin(z *Zone, src *entity, _ *skill.Def, lv int) wathenav1.SkillFailure {
	lv += src.sc.Val1(sc.SCRaisingdragon)
	if p := src.player; p != nil && p.spiritBalls() >= lv {
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (callSpirits) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	p := src.player
	if p == nil {
		return
	}
	limit := lv + src.sc.Val1(sc.SCRaisingdragon)
	z.skillEffect(src, target, d, lv, int64(lv), true)
	z.addSpiritBall(p, int64(d.Duration1.At(lv)), limit)
}

// soulCollect is SkillZen: five spheres at once.
type soulCollect struct{}

func (soulCollect) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	p := src.player
	if p == nil {
		return
	}
	limit := 5 + src.sc.Val1(sc.SCRaisingdragon)
	z.skillEffect(src, target, d, lv, int64(lv), true)
	for range limit {
		z.addSpiritBall(p, int64(d.Duration1.At(lv)), limit)
	}
}

// absorbSpirits is SkillAbsorbSpiritSphere: 7 SP per sphere of a player,
// or a chance of 2 SP per level of a monster.
type absorbSpirits struct{}

func (absorbSpirits) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	sp := 0
	switch {
	case target.player != nil && (target == src || isEnemy(src, target)) &&
		z.mapID(target.player)&jcFirstMask != mapidGunslinger:
		// Gunslingers' coins cannot be taken. Spirit charms are not
		// ported.
		if n := target.player.spiritBalls(); n > 0 {
			sp = n * 7
			z.delSpiritBall(target.player, n)
		}
	case target.monster != nil && target.status != nil && target.status.Mode&modeStatusImmune == 0 && z.rand.IntN(100) < 20:
		sp = 2 * z.unit(target).Level()
		// mob_target: the monster turns on the caster.
		z.mobTarget(target, src)
	default:
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	if p := src.player; p != nil && sp > 0 {
		if _, got := z.statusHeal(p, 0, int64(sp)); got > 0 {
			if s := z.battleStats(p); s != nil {
				p.pending = append(p.pending, s)
			}
		}
	}
	z.skillEffect(src, target, d, lv, int64(sp), sp != 0)
}

// kiTranslation is SkillKiTranslation: the caster's spheres go to the
// target. It is PartyOnly, so without parties the caster gives them to
// itself, as rAthena does for a player without a party.
type kiTranslation struct{}

func (kiTranslation) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	p, tp := src.player, target.player
	if tp != nil && p != nil {
		m := z.mapID(tp)
		if m&jcFirstMask != mapidGunslinger && m&jcSecondMask != mapidRebellion && tp.spiritBalls() < 5 {
			// The requirement says how many spheres move.
			n := z.requirement(src, d, lv).SpiritSphere
			z.delSpiritBall(p, n)
			for range n {
				z.addSpiritBall(tp, int64(d.Duration1.At(lv)), 5)
			}
			return
		}
	}
	if p != nil {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
	}
	*flag |= skillNoConsumeReq
}

// tripleAttack is SkillRagingTrifectaBlow: a weapon hit with the attack
// animation.
type tripleAttack struct{}

func (tripleAttack) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag|sdAnimation)
}

// bladeStopAttack is SkillOccultImpaction: a weapon hit that ends the
// caster's Blade Stop.
type bladeStopAttack struct{}

func (bladeStopAttack) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag)
	z.scEnd(src, sc.SCBladestop)
}

// fingerOffensive is SkillThrowSpiritSphere. finger_offensive_type is 0,
// so it is one hit of as many parts as spheres.
type fingerOffensive struct{ bladeStopAttack }

// castBegin is skill_check_condition_castbegin's MO_FINGEROFFENSIVE
// case: it throws the spheres the player has, up to its level.
func (fingerOffensive) castBegin(z *Zone, src *entity, d *skill.Def, lv int) wathenav1.SkillFailure {
	if p := src.player; p != nil {
		need := z.requirement(src, d, lv).SpiritSphere
		if n := p.spiritBalls(); n > 0 && n < need {
			p.spiritBallOld = n
		} else {
			p.spiritBallOld = need
		}
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

// chainCombo is SkillRagingQuadrupleBlow: it follows Raging Trifecta
// Blow, or Blade Stop.
type chainCombo struct{ bladeStopAttack }

func (chainCombo) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	l := src.sc
	if l.Has(sc.SCBladestop) || l.Has(sc.SCCombo) && l.Val1(sc.SCCombo) == skill.IDTripleAttack {
		return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
	}
	return skillFailSilent
}

// comboFinish is SkillRagingThrust: it follows Raging Quadruple Blow,
// and hits around the target when soul linked.
type comboFinish struct{}

func (comboFinish) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	if l := src.sc; l.Has(sc.SCCombo) && l.Val1(sc.SCCombo) == skill.IDChainCombo {
		return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
	}
	return skillFailSilent
}

func (comboFinish) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 == 0 && soulLinkedMonk(src.sc) {
		// A splash when soul linked: map_foreachinshootrange with
		// skill_area_sub.
		for _, t := range z.areaTargets(src, target.x, target.y, d.SplashFor(lv), true) {
			z.castendDamage(src, t, d, lv, *flag|1)
		}
		return
	}
	z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag)
}

// tigerFist is SkillGlacierFist: it follows Raging Thrust (or Chain Crush
// Combo in pre-renewal), and may hold the target in place.
type tigerFist struct{ attackSkill }

func (tigerFist) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	l := src.sc
	if !l.Has(sc.SCCombo) {
		return skillFailSilent
	}
	v := l.Val1(sc.SCCombo)
	if v == skill.IDComboFinish || !z.skillRenewal(skill.IDTigerFist) && v == skill.IDChainCrush {
		return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
	}
	return skillFailSilent
}

func (tigerFist) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	basetime := int64(d.Duration1.At(lv))
	mintime := int64(15 * (z.unit(src).Level() + 100))
	if target.status != nil && target.status.Mode&modeStatusImmune != 0 {
		basetime /= 5
	}
	agi := int64(1)
	if target.status != nil {
		agi = int64(target.status.Agi)
	}
	basetime = max(basetime*agi/-200+basetime, mintime)
	z.scStart(src, target, sc.SCAnkle, (1+lv)*10, 0, basetime)
}

// chainCrush is SkillChainCrushCombo: it follows Raging Thrust or Glacier
// Fist.
type chainCrush struct{ attackSkill }

func (chainCrush) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	l := src.sc
	if !l.Has(sc.SCCombo) {
		return skillFailSilent
	}
	if v := l.Val1(sc.SCCombo); v != skill.IDComboFinish && v != skill.IDTigerFist {
		return skillFailSilent
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

// extremityFist is SkillAsuraStrike.
type extremityFist struct{}

// castBegin is skill_check_condition_castbegin's MO_EXTREMITYFIST case:
// outside Blade Stop it follows Raging Thrust or Chain Crush Combo, or
// needs a caster that can move. It counts the spheres it starts with.
func (extremityFist) castBegin(z *Zone, src *entity, _ *skill.Def, _ int) wathenav1.SkillFailure {
	l := src.sc
	if l.Has(sc.SCBladestop) || l.Has(sc.SCCursedcircleAtker) {
		return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
	}
	if l.Has(sc.SCCombo) {
		if v := l.Val1(sc.SCCombo); v != skill.IDComboFinish && v != skill.IDChainCrush {
			return skillFailSilent
		}
	} else if src.cant().Move || z.now < src.canMove {
		// unit_can_move.
		return wathenav1.SkillFailure_SKILL_FAILURE_LEVEL
	}
	if p := src.player; p != nil {
		p.spiritBallOld = p.spiritBalls()
	}
	return wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED
}

func (extremityFist) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	dir := mapCalcDir(src, target.x, target.y)
	if p := src.player; p != nil && z.skillRenewal(skill.IDExtremityFist) && p.spiritBallOld > 5 {
		// More than five spheres double the damage.
		*flag |= 1
	}
	z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag)
	z.asuraAfter(src, d, lv)
	z.asuraMove(src, dir)
}

// balkyoung is SkillKiExplosion: a hit on the target, then the enemies
// around it are pushed away and may be stunned.
type balkyoung struct{}

func (balkyoung) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.area.main = target
	if z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag) == 0 {
		return
	}
	// map_foreachinallrange: no wall check.
	for _, t := range z.areaTargets(src, target.x, target.y, d.SplashFor(lv), false) {
		z.castendNoDamage(src, t, d, lv, *flag|1)
	}
}

func (balkyoung) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if z.area.main == target {
		return
	}
	z.skillBlown(src, target, d.Knockback.At(lv), -1, false)
	// skill_additional_effect with BF_MISC: the passive part stuns.
	z.additionalEffectFlags(src, target, battle.FlagMisc, d.ID)
	z.scStart(src, target, sc.SCStun, 70, lv, int64(d.Duration2.At(lv)))
}

// palmStrike is SkillRagingPalmStrike: the hit lands a second after the
// cast, plus the attack motion.
type palmStrike struct{}

func (palmStrike) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	// Viewers see an absorbed attack now.
	amotion := 0
	if src.status != nil {
		amotion = src.status.AMotion
	}
	z.hits = append(z.hits, hit{source: src, target: target, result: battle.Result{Kind: battle.Hit, Hits: 1},
		delay: amotion, hpPermille: target.hpPermille()})
	z.addTimerSkill(src, z.now+1000+int64(amotion), target, 0, 0, d, lv, battle.TypeWeapon, *flag)
}

// timerSkill is Palm Strike's part of skill_timerskill: it only pushes a
// hidden target, or from a hidden caster, and hits otherwise.
func (palmStrike) timerSkill(z *Zone, t *timerSkill) {
	src, target := t.src, t.target
	if target == nil {
		return
	}
	if optionHidden(target) || optionHidden(src) {
		z.skillBlown(src, target, t.def.Knockback.At(t.lv), -1, false)
		return
	}
	z.skillAttack(t.typ, src, src, target, t.def, t.lv, t.flag)
}

// bodyRelocation is SkillSnap: the caster moves to the cell at once, and
// cannot use Asura Strike for 2 seconds.
type bodyRelocation struct{}

func (bodyRelocation) castendPos(z *Zone, src *entity, x, y int, _ *skill.Def, _ int, _ *int) {
	if !z.movePos(src, x, y, false) {
		return
	}
	if p := src.player; p != nil {
		z.blockSkill(p, skill.IDExtremityFist, 2000)
	}
}
