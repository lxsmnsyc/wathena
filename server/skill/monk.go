package skill

import (
	"github.com/lxsmnsyc/wathena/server/battle"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The skills of the Monk and the Champion: their damage formulas, as the
// calculateSkillRatio and modifyDamageData of rAthena's SkillImpl
// classes in src/map/skills/acolyte, and the parts of skill.cpp and
// unit.cpp that change their costs and cast times. Their other behavior
// is in server/zone/skills_monk.go.

// Skill IDs of the Monk and the Champion, from rAthena's skill_db.yml.
// MO_TRIPLEATTACK, MO_CHAINCOMBO, MO_COMBOFINISH, CH_TIGERFIST and
// CH_CHAINCRUSH are in cast.go.
const (
	IDIronHand         = 259  // MO_IRONHAND
	IDSpiritsRecovery  = 260  // MO_SPIRITSRECOVERY
	IDCallSpirits      = 261  // MO_CALLSPIRITS
	IDAbsorbSpirits    = 262  // MO_ABSORBSPIRITS
	IDBodyRelocation   = 264  // MO_BODYRELOCATION
	IDDodge            = 265  // MO_DODGE
	IDInvestigate      = 266  // MO_INVESTIGATE
	IDFingerOffensive  = 267  // MO_FINGEROFFENSIVE
	IDSteelBody        = 268  // MO_STEELBODY
	IDBladeStop        = 269  // MO_BLADESTOP
	IDExplosionSpirits = 270  // MO_EXPLOSIONSPIRITS
	IDExtremityFist    = 271  // MO_EXTREMITYFIST
	IDPalmStrike       = 370  // CH_PALMSTRIKE
	IDSoulCollect      = 401  // CH_SOULCOLLECT
	IDKiTranslation    = 1015 // MO_KITRANSLATION
	IDBalkyoung        = 1016 // MO_BALKYOUNG
	// IDSoulLinkMonk is SL_MONK: SC_SPIRIT's val2 for the Monk's soul
	// link.
	IDSoulLinkMonk = 447
)

type (
	// tripleAttack is SkillRagingTrifectaBlow.
	tripleAttack struct{ WeaponSkill }
	// investigate is SkillOccultImpaction.
	investigate struct{ WeaponSkill }
	// fingerOffensive is SkillThrowSpiritSphere.
	fingerOffensive struct{ WeaponSkill }
	// extremityFist is SkillAsuraStrike.
	extremityFist struct{ WeaponSkill }
	// chainCombo is SkillRagingQuadrupleBlow.
	chainCombo struct{ WeaponSkill }
	// comboFinish is SkillRagingThrust.
	comboFinish struct{ WeaponSkill }
	// balkyoung is SkillKiExplosion.
	balkyoung struct{ Base }
	// palmStrike is SkillRagingPalmStrike.
	palmStrike struct{ Base }
	// tigerFist is SkillGlacierFist.
	tigerFist struct{ WeaponSkill }
	// chainCrush is SkillChainCrushCombo.
	chainCrush struct{ WeaponSkill }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDTripleAttack:    tripleAttack{},
		IDInvestigate:     investigate{},
		IDFingerOffensive: fingerOffensive{},
		IDExtremityFist:   extremityFist{},
		IDChainCombo:      chainCombo{},
		IDComboFinish:     comboFinish{},
		IDBalkyoung:       balkyoung{},
		IDPalmStrike:      palmStrike{},
		IDTigerFist:       tigerFist{},
		IDChainCrush:      chainCrush{},
		// Skills whose behavior is in the zone only.
		IDCallSpirits:      Base{},
		IDAbsorbSpirits:    Base{},
		IDBodyRelocation:   Base{},
		IDSteelBody:        Base{},
		IDBladeStop:        Base{},
		IDExplosionSpirits: Base{},
		IDSoulCollect:      Base{},
		IDKiTranslation:    Base{},
	})
}

// targetBladeStopped reports whether the target is caught in Blade Stop.
func targetBladeStopped(a *battle.SkillContext) bool {
	return a.Target.Changes != nil && a.Target.Changes.BladeStop
}

// SkillRatio is SkillRagingTrifectaBlow::calculateSkillRatio.
func (tripleAttack) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 20 * a.Level() }

// SkillRatio is SkillOccultImpaction::calculateSkillRatio. In renewal,
// a target caught in Blade Stop takes half again as much.
func (investigate) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		*ratio += 75 * a.Level()
		return
	}
	*ratio += -100 + 100*a.Level()
	if targetBladeStopped(a) {
		*ratio += *ratio / 2
	}
}

// SkillRatio is SkillThrowSpiritSphere::calculateSkillRatio.
func (fingerOffensive) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		*ratio += 50 * a.Level()
		return
	}
	*ratio += 500 + a.Level()*200
	if targetBladeStopped(a) {
		*ratio += *ratio / 2
	}
}

// ModifyDamageData is SkillThrowSpiritSphere::modifyDamageData.
// finger_offensive_type is 0: in pre-renewal it hits once per sphere it
// used, at most its level.
func (fingerOffensive) ModifyDamageData(a *battle.SkillContext, d *battle.DamageData) {
	sd := a.Src.Player
	if sd == nil || a.Renewal {
		return
	}
	if n := sd.SpiritBall + sd.SpiritBallOld; n < d.Div {
		d.Div = n
	}
}

// SkillRatio is SkillAsuraStrike::calculateSkillRatio: 800% plus 10% per
// SP. In renewal, flag 1 (more than five spheres) doubles it.
func (extremityFist) SkillRatio(a *battle.SkillContext, ratio *int) {
	sp := a.Src.MaxSP
	if sd := a.Src.Player; sd != nil {
		sp = sd.SP
	}
	*ratio += 700 + sp*10
	if a.Renewal && a.Flag&1 != 0 {
		*ratio *= 2
	}
	// Stop at about 50,000 SP for overflow protection.
	*ratio = min(500000, *ratio)
}

// SkillRatio is SkillRagingQuadrupleBlow::calculateSkillRatio. In
// renewal, knuckles double it.
func (chainCombo) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		*ratio += 50 + 50*a.Level()
		return
	}
	*ratio += 150 + 50*a.Level()
	if sd := a.Src.Player; sd != nil && sd.Weapon == battle.WeaponKnuckle {
		*ratio *= 2
	}
}

// ModifyDamageData is SkillRagingQuadrupleBlow::modifyDamageData: in
// renewal, knuckles hit six times.
func (chainCombo) ModifyDamageData(a *battle.SkillContext, d *battle.DamageData) {
	if sd := a.Src.Player; a.Renewal && sd != nil && sd.Weapon == battle.WeaponKnuckle {
		d.Div = -6
	}
}

// SkillRatio is SkillRagingThrust::calculateSkillRatio. SC_GT_ENERGYGAIN
// comes with the Sura's skills.
func (comboFinish) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 450 + 50*a.Level() + a.Src.Str
	} else {
		*ratio += 140 + 60*a.Level()
	}
}

// SkillRatio is SkillKiExplosion::calculateSkillRatio.
func (balkyoung) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 700
	} else {
		*ratio += 200
	}
}

// ModifyDamageData is SkillKiExplosion::modifyDamageData: the hit itself
// pushes nothing. The splash around it pushes.
func (balkyoung) ModifyDamageData(_ *battle.SkillContext, d *battle.DamageData) { d.Knockback = 0 }

// SkillRatio is SkillRagingPalmStrike::calculateSkillRatio.
func (palmStrike) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		*ratio += 100 + 100*a.Level()
		return
	}
	*ratio += 100 + 100*a.Level() + a.Src.Str
	reLvlDmod(a, ratio, 100)
}

// SkillRatio is SkillGlacierFist::calculateSkillRatio.
func (tigerFist) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		*ratio += -60 + 100*a.Level()
		return
	}
	*ratio += 400 + 150*a.Level()
	reLvlDmod(a, ratio, 100)
}

// SkillRatio is SkillChainCrushCombo::calculateSkillRatio.
func (chainCrush) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		*ratio += 300 + 100*a.Level()
		return
	}
	*ratio += -100 + 200*a.Level()
	reLvlDmod(a, ratio, 100)
}

// monkRequirement is the part of skill_get_requirement that changes the
// costs of the Monk and Champion skills: Soul Link's cheap combos, Snap
// without spheres under Fury, and Asura Strike's spheres.
func monkRequirement(d *Def, c *Caster, r *Requirement, renewal bool) {
	l := c.Changes
	switch d.ID {
	case IDChainCombo, IDComboFinish, IDTigerFist, IDChainCrush:
		// The Monk Spirit makes the combos cost 2 SP.
		if l.Has(sc.SCSpirit) && l.Val2(sc.SCSpirit) == IDSoulLinkMonk {
			r.SP = 2
		}
	case IDBodyRelocation:
		if l.Has(sc.SCExplosionspirits) {
			r.SpiritSphere = 0
		}
	case IDExtremityFist:
		if l == nil || l.Empty() {
			return
		}
		balls := c.SpiritBalls
		switch {
		case l.Has(sc.SCBladestop):
			r.SpiritSphere--
		case l.Has(sc.SCCombo):
			if renewal {
				r.SpiritSphere = max(balls, 1)
				break
			}
			switch l.Val1(sc.SCCombo) {
			case IDComboFinish:
				r.SpiritSphere = 4
			case IDChainCrush:
				// It uses whatever is left, at least 1.
				r.SpiritSphere = max(balls, 1)
			}
		case l.Has(sc.SCRaisingdragon) && balls > 5:
			// It uses every sphere.
			r.SpiritSphere = balls
		}
	}
}

// monkCastTime is the part of unit_skilluse_id2 that changes the cast
// time of the Monk's skills, between skill_castfix and
// skill_castfix_sc or skill_vfcastfix:
//   - Pre-renewal Finger Offensive takes its cast time once more for
//     each sphere it throws.
//   - Asura Strike right after Raging Thrust or Chain Crush Combo casts
//     at once (-1).
func monkCastTime(d *Def, c *Caster, lv int, t float64, renewal bool) float64 {
	switch d.ID {
	case IDFingerOffensive:
		if !renewal && c.cast() != nil {
			t += t * float64(min(lv, c.SpiritBalls))
		}
	case IDExtremityFist:
		if l := c.Changes; l.Has(sc.SCCombo) {
			if v := l.Val1(sc.SCCombo); v == IDComboFinish || v == IDChainCrush {
				t = -1
			}
		}
	}
	return t
}
