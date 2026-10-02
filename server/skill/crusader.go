package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The Crusader and Paladin skills: their damage formulas, as the
// calculateSkillRatio, modifyHitRate and modifyElement methods of
// rAthena's SkillImpl classes in src/map/skills/swordman. The base
// damage of Shield Boomerang, Shield Chain, Martyr's Reckoning and Grand
// Cross, which battle.cpp computes itself, is in server/battle. The
// skills' other behavior is in server/zone/skills_crusader.go.

// Skill IDs of the Crusader and Paladin, from rAthena's skill_db.yml.
const (
	IDFaith           = 248  // CR_TRUST
	IDAutoGuard       = 249  // CR_AUTOGUARD
	IDSmite           = 250  // CR_SHIELDCHARGE
	IDShieldBoomerang = 251  // CR_SHIELDBOOMERANG
	IDReflectShield   = 252  // CR_REFLECTSHIELD
	IDHolyCross       = 253  // CR_HOLYCROSS
	IDGrandCross      = 254  // CR_GRANDCROSS
	IDDevotion        = 255  // CR_DEVOTION
	IDProvidence      = 256  // CR_PROVIDENCE
	IDDefender        = 257  // CR_DEFENDER
	IDSpearQuicken    = 258  // CR_SPEARQUICKEN
	IDPressure        = 367  // PA_PRESSURE
	IDSacrifice       = 368  // PA_SACRIFICE
	IDGospel          = 369  // PA_GOSPEL
	IDShieldChain     = 480  // PA_SHIELDCHAIN
	IDShrink          = 1002 // CR_SHRINK
)

type (
	smite           struct{ WeaponSkill }
	shieldBoomerang struct{ WeaponSkill }
	holyCross       struct{ WeaponSkill }
	shieldChain     struct{ WeaponSkill }
	martyrs         struct{ WeaponSkill }
	gloriaDomini    struct{ Base }
	// grandCross is SkillGrandCross. Its ratio is in
	// battle_calc_magic_attack, which server/battle ports.
	grandCross struct{ Base }
	// crusaderStatus are the skills whose only formula is their status,
	// or whose behavior is all in the zone: Faith, Guard, Shield Reflect,
	// Sacrifice (Devotion), Resistant Souls, Defending Aura, Spear
	// Quicken, Shrink and Battle Chant.
	crusaderStatus struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDFaith:           crusaderStatus{},
		IDAutoGuard:       crusaderStatus{},
		IDSmite:           smite{},
		IDShieldBoomerang: shieldBoomerang{},
		IDReflectShield:   crusaderStatus{},
		IDHolyCross:       holyCross{},
		IDGrandCross:      grandCross{},
		IDDevotion:        crusaderStatus{},
		IDProvidence:      crusaderStatus{},
		IDDefender:        crusaderStatus{},
		IDSpearQuicken:    crusaderStatus{},
		IDShrink:          crusaderStatus{},
		IDPressure:        gloriaDomini{},
		IDSacrifice:       martyrs{},
		IDGospel:          crusaderStatus{},
		IDShieldChain:     shieldChain{},
	})
}

// SkillRatio is SkillSmite::calculateSkillRatio: 100% + 20% per level.
func (smite) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 20 * a.Level() }

// SkillRatio is SkillShieldBoomerang::calculateSkillRatio. Renewal: 80%
// per level. Pre-renewal: 100% + 30% per level.
func (shieldBoomerang) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += -100 + a.Level()*80
		return
	}
	*ratio += 30 * a.Level()
}

// ModifyElement is SkillShieldBoomerang::modifyElement: in renewal the
// damage counts as neutral. rAthena changes the element only when it
// computes the damage (flag 1). The base damage of Shield Boomerang
// reads no element, so the change holds for the whole attack.
func (shieldBoomerang) ModifyElement(a *battle.SkillContext, element *int) {
	if a.Renewal {
		*element = battle.Neutral
	}
}

// SkillRatio is SkillHolyCross::calculateSkillRatio: 100% + 35% per
// level, or 70% per level in renewal with a two-handed spear.
func (holyCross) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		if sd := a.Src.Player; sd != nil && sd.Weapon == battle.Weapon2HSpear {
			*ratio += 70 * a.Level()
			return
		}
	}
	*ratio += 35 * a.Level()
}

// SkillRatio is SkillGloriaDomini::calculateSkillRatio: renewal's 500%
// + 150% per level, scaled by base level (RE_LVL_DMOD). Pre-renewal's
// damage is fixed, in battle_calc_misc_attack.
func (gloriaDomini) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		return
	}
	*ratio += -100 + 500 + 150*a.Level()
	reLvlDmod(a, ratio, 100)
}

// SkillRatio is SkillMartyrsReckoning::calculateSkillRatio: 90% + 10% per
// level.
func (martyrs) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += -10 + 10*a.Level() }

// SkillRatio is SkillShieldChain::calculateSkillRatio. Renewal: 300% +
// 200% per level, more for the shield's weight and refine, scaled by
// base level. Pre-renewal: 100% + 30% per level. Shield Spell's
// SC_SHIELD_POWER and Shield Mastery belong to the Royal Guard, which
// is not ported.
func (shieldChain) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		*ratio += 30 * a.Level()
		return
	}
	*ratio = -100 + 300 + 200*a.Level()
	if sd := a.Src.Player; sd != nil && sd.Shield {
		*ratio += sd.ShieldWeight/10 + 4*sd.ShieldRefine
	}
	reLvlDmod(a, ratio, 100)
}
