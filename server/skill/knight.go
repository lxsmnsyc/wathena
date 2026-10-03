package skill

import (
	"github.com/lxsmnsyc/wathena/server/battle"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The Knight and Lord Knight skills: their damage formulas, as the
// calculateSkillRatio, modifyHitRate, modifyDamageData and modifyElement
// methods of rAthena's SkillImpl classes in src/map/skills/swordman. The
// skills' other behavior is in server/zone/skills_knight.go.

// Skill IDs of the Knight and Lord Knight, from rAthena's skill_db.yml.
const (
	IDSpearMastery    = 55   // KN_SPEARMASTERY
	IDPierce          = 56   // KN_PIERCE
	IDBrandishSpear   = 57   // KN_BRANDISHSPEAR
	IDSpearStab       = 58   // KN_SPEARSTAB
	IDSpearBoomerang  = 59   // KN_SPEARBOOMERANG
	IDTwoHandQuicken  = 60   // KN_TWOHANDQUICKEN
	IDAutoCounter     = 61   // KN_AUTOCOUNTER
	IDBowlingBash     = 62   // KN_BOWLINGBASH
	IDRiding          = 63   // KN_RIDING
	IDCavalierMastery = 64   // KN_CAVALIERMASTERY
	IDAuraBlade       = 355  // LK_AURABLADE
	IDParrying        = 356  // LK_PARRYING
	IDLKConcentration = 357  // LK_CONCENTRATION
	IDTensionRelax    = 358  // LK_TENSIONRELAX
	IDBerserk         = 359  // LK_BERSERK
	IDSpiralPierce    = 397  // LK_SPIRALPIERCE
	IDHeadCrush       = 398  // LK_HEADCRUSH
	IDJointBeat       = 399  // LK_JOINTBEAT
	IDOneHandQuicken  = 495  // KN_ONEHAND
	IDChargeAttack    = 1001 // KN_CHARGEATK
)

type (
	pierce         struct{ WeaponSkill }
	brandishSpear  struct{ Base }
	spearStab      struct{ Base }
	spearBoomerang struct{ WeaponSkill }
	autoCounter    struct{ Base }
	bowlingBash    struct{ Base }
	chargeAttack   struct{ Base }
	spiralPierce   struct{ WeaponSkill }
	headCrush      struct{ WeaponSkill }
	jointBeat      struct{ Base }
	// knightStatus are the skills whose only formula is their status, as
	// StatusSkillImpl and SkillRelax: Two-Hand Quicken, One-Hand
	// Quicken, Aura Blade, Parrying, Concentration, Tension Relax and
	// Frenzy.
	knightStatus struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDPierce:          pierce{},
		IDBrandishSpear:   brandishSpear{},
		IDSpearStab:       spearStab{},
		IDSpearBoomerang:  spearBoomerang{},
		IDTwoHandQuicken:  knightStatus{},
		IDAutoCounter:     autoCounter{},
		IDBowlingBash:     bowlingBash{},
		IDOneHandQuicken:  knightStatus{},
		IDChargeAttack:    chargeAttack{},
		IDAuraBlade:       knightStatus{},
		IDParrying:        knightStatus{},
		IDLKConcentration: knightStatus{},
		IDTensionRelax:    knightStatus{},
		IDBerserk:         knightStatus{},
		IDSpiralPierce:    spiralPierce{},
		IDHeadCrush:       headCrush{},
		IDJointBeat:       jointBeat{},
	})
}

// SkillRatio is SkillPierce::calculateSkillRatio: 100% + 10% per level.
// The double damage of SC_CHARGINGPIERCE_COUNT needs the Dragon Knight's
// Charging Pierce, which is not ported.
func (pierce) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 10 * a.Level() }

// HitRate is SkillPierce::modifyHitRate: 5% more per level.
func (pierce) HitRate(a *battle.SkillContext, hit *int) { *hit += *hit * 5 * a.Level() / 100 }

// ModifyDamageData is SkillPierce::modifyDamageData: one hit per size
// step, so 1 on small, 2 on medium and 3 on large targets.
func (pierce) ModifyDamageData(a *battle.SkillContext, d *battle.DamageData) {
	if d.Div > 0 {
		d.Div = a.Target.Size + 1
	} else {
		d.Div = -(a.Target.Size + 1)
	}
}

// SkillRatio is SkillBrandishSpear::calculateSkillRatio. Renewal: 400% +
// 100% per level + 3% per STR. Pre-renewal: 100% + 20% per level, with
// more for the rows nearer the caster (flag 0 the farthest, then 1 and
// 2), from levels 4, 7 and 10.
func (brandishSpear) SkillRatio(a *battle.SkillContext, ratio *int) {
	lv := a.Level()
	if a.Renewal {
		*ratio += -100 + 400 + 100*lv + a.Src.Str*3
		return
	}
	r := 100 + 20*lv
	*ratio += -100 + r
	if lv > 3 && a.Flag == 0 {
		*ratio += r / 2
	}
	if lv > 6 && a.Flag == 0 {
		*ratio += r / 4
	}
	if lv > 9 && a.Flag == 0 {
		*ratio += r / 8
	}
	if lv > 6 && a.Flag == 1 {
		*ratio += r / 2
	}
	if lv > 9 && a.Flag == 1 {
		*ratio += r / 4
	}
	if lv > 9 && a.Flag == 2 {
		*ratio += r / 2
	}
}

// SkillRatio is SkillSpearStab::calculateSkillRatio: 100% + 20% per
// level.
func (spearStab) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 20 * a.Level() }

// ModifyDamageData is SkillSpearStab::modifyDamageData: the hit does not
// push; the skill's behavior pushes instead.
func (spearStab) ModifyDamageData(_ *battle.SkillContext, d *battle.DamageData) { d.Knockback = 0 }

// SkillRatio is SkillSpearBoomerang::calculateSkillRatio: 100% + 50% per
// level.
func (spearBoomerang) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 50 * a.Level() }

// ModifyDamageData is SkillCounterAttack::modifyDamageData: the counter
// counts as a normal attack.
func (autoCounter) ModifyDamageData(_ *battle.SkillContext, d *battle.DamageData) {
	d.Flag = d.Flag&^battle.FlagSkillMask | battle.FlagNormal
}

// HitRate is SkillCounterAttack::modifyHitRate: 20% more.
func (autoCounter) HitRate(_ *battle.SkillContext, hit *int) { *hit += *hit * 20 / 100 }

// SkillRatio is SkillBowlingBash::calculateSkillRatio: 100% + 40% per
// level.
func (bowlingBash) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 40 * a.Level() }

// ModifyDamageData is SkillBowlingBash::modifyDamageData. Renewal: with a
// two-handed sword, 3 hits on 2 or 3 targets and 4 hits on 4 or more.
// Pre-renewal: the hit does not push; the skill's behavior pushes.
func (bowlingBash) ModifyDamageData(a *battle.SkillContext, d *battle.DamageData) {
	if !a.Renewal {
		d.Knockback = 0
		return
	}
	if sd := a.Src.Player; sd != nil && sd.Weapon == battle.Weapon2HSword {
		switch {
		case a.Flag >= 4:
			d.Div = 4
		case a.Flag >= 2:
			d.Div = 3
		}
	}
}

// SkillRatio is SkillChargeAttack::calculateSkillRatio. Renewal: 700%.
// Pre-renewal: 100% more per 3 cells of distance (the flag), at most
// 500%.
func (chargeAttack) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 600
		return
	}
	k := min(max((a.Flag-1)/3, 0), 4)
	*ratio += 100 * k
}

// SkillRatio is SkillSpiralPierce::calculateSkillRatio: renewal's 150% +
// 50% per level, scaled by base level above 100 (RE_LVL_DMOD). The
// double damage of SC_CHARGINGPIERCE_COUNT needs Charging Pierce, which
// is not ported. Pre-renewal's level bonus is in the base damage.
func (spiralPierce) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		return
	}
	*ratio += 50 + 50*a.Level()
	// RE_LVL_DMOD(100). At level 100 the ratio does not change.
	reLvlDmod(a, ratio, 100)
}

// ModifyDamageData is SkillSpiralPierce::modifyDamageData: a unit that
// is not a player deals a ranged misc hit.
func (spiralPierce) ModifyDamageData(a *battle.SkillContext, d *battle.DamageData) {
	if a.Src.Player == nil {
		d.Flag = d.Flag&^(battle.FlagRangeMask|battle.FlagWeaponMask) | battle.FlagLong | battle.FlagMisc
	}
}

// ModifyElement is SkillSpiralPierce::modifyElement: neutral for units
// that are not players.
func (spiralPierce) ModifyElement(a *battle.SkillContext, element *int) {
	if a.Src.Player == nil {
		*element = battle.Neutral
	}
}

// SkillRatio is SkillTraumaticBlow::calculateSkillRatio: 100% + 40% per
// level.
func (headCrush) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 40 * a.Level() }

// SkillRatio is SkillVitalStrike::calculateSkillRatio: 50% + 10% per
// level, doubled when this hit breaks the neck (the flag) or the neck is
// already broken.
func (jointBeat) SkillRatio(a *battle.SkillContext, ratio *int) {
	*ratio += 10*a.Level() - 50
	if a.Flag&sc.BreakNeck != 0 || a.Target.Changes != nil && a.Target.Changes.JointBeat&sc.BreakNeck != 0 {
		*ratio *= 2
	}
}
