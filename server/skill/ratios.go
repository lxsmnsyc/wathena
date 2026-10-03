package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The damage formulas of the first class skills: rAthena's
// calculateSkillRatio and modifyHitRate of each skill's SkillImpl. The
// skills' other behavior is in their class's file.

// SkillRatio is SkillBash::calculateSkillRatio: 100% + 30% per level.
func (bash) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 30 * a.Level() }

// HitRate is SkillBash::modifyHitRate: 5% more of the final hit rate per
// level.
func (bash) HitRate(a *battle.SkillContext, hit *int) { *hit += *hit * 5 * a.Level() / 100 }

// SkillRatio is SkillMagnumBreak::calculateSkillRatio: the inner 3x3
// takes 100% + 20% per level, the outer 5x5 100% + 10% per level.
func (magnumBreak) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Flag == 1 {
		*ratio += 20 * a.Level()
	} else {
		*ratio += 10 * a.Level()
	}
}

// HitRate is SkillMagnumBreak::modifyHitRate: 10% more per level.
func (magnumBreak) HitRate(a *battle.SkillContext, hit *int) { *hit += *hit * 10 * a.Level() / 100 }

// SkillRatio is SkillNapalmBeat::calculateSkillRatio.
func (napalmBeat) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += -30 + 10*a.Level() }

// SkillRatio is SkillSoulStrike::calculateSkillRatio: 5% more per level
// against the undead.
func (soulStrike) SkillRatio(a *battle.SkillContext, ratio *int) {
	if battle.IsUndead(a.Target.Race, a.Target.DefEle) {
		*ratio += 5 * a.Level()
	}
}

// SkillRatio is SkillFrostDiver::calculateSkillRatio.
func (frostDiver) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 10 * a.Level() }

// SkillRatio is SkillFireBall::calculateSkillRatio. Targets at the edge
// of the area (flag 2) take 75%.
func (fireBall) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 40 + 20*a.Level()
	} else {
		*ratio += -30 + 10*a.Level()
	}
	if a.Flag == 2 {
		*ratio = *ratio * 3 / 4
	}
}

// SkillRatio is SkillFireWall::calculateSkillRatio.
func (fireWall) SkillRatio(_ *battle.SkillContext, ratio *int) { *ratio -= 50 }

// SkillRatio is SkillThunderStorm::calculateSkillRatio: 80% in
// pre-renewal.
func (thunderStorm) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		*ratio -= 20
	}
}

// SkillRatio is SkillDoubleStrafe::calculateSkillRatio.
func (doubleStrafe) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 10 * (a.Level() - 1) }

// SkillRatio is SkillArrowShower::calculateSkillRatio.
func (arrowShower) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 50 + 10*a.Level()
	} else {
		*ratio += -25 + 5*a.Level()
	}
}

// SkillRatio is SkillChargeArrow::calculateSkillRatio.
func (chargeArrow) SkillRatio(_ *battle.SkillContext, ratio *int) { *ratio += 50 }

// SkillRatio is SkillHolyLight::calculateSkillRatio. The Priest Spirit's
// fivefold damage needs Soul Linker skills, which are not ported.
func (holyLight) SkillRatio(_ *battle.SkillContext, ratio *int) { *ratio += 25 }

// SkillRatio is SkillRuwach::calculateSkillRatio.
func (ruwach) SkillRatio(_ *battle.SkillContext, ratio *int) { *ratio += 45 }

// SkillRatio is SkillMammonite::calculateSkillRatio.
func (mammonite) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 50 * a.Level() }

// SkillRatio is SkillCartRevolution::calculateSkillRatio: 150%, plus 1%
// per 1% of the cart's weight. Units that are not players deal the most.
func (cartRevolution) SkillRatio(a *battle.SkillContext, ratio *int) {
	*ratio += 50
	if sd := a.Src.Player; sd != nil && sd.CartWeight > 0 && sd.CartWeightMax > 0 {
		*ratio += 100 * sd.CartWeight / sd.CartWeightMax
	} else if sd == nil {
		*ratio += 100
	}
}

// SkillRatio is SkillSandAttack::calculateSkillRatio.
func (sandAttack) SkillRatio(_ *battle.SkillContext, ratio *int) { *ratio += 30 }

// BaseDamage is Heal's damage to the undead: skill_calc_heal with heal
// false, as battle_calc_magic_attack.
func (heal) BaseDamage(a *battle.SkillContext, r battle.Rand) int64 {
	return CalcHeal(a.Src, a.Target, IDHeal, a.Level(), false, a.Ruleset().HealFormula, r)
}

// reLvlDmod is rAthena's RE_LVL_DMOD macro, which the level_damage_mod
// flag turns on (RENEWAL_LVDMG): above base level 99, the ratio grows
// with the caster's level over val. Off, it does nothing.
func reLvlDmod(a *battle.SkillContext, ratio *int, val int) {
	if a.Ruleset().LevelDamageMod && a.Src.Level > 99 && val > 0 {
		*ratio = *ratio * a.Src.Level / val
	}
}
