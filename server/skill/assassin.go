package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The Assassin and Assassin Cross skills: their damage formulas, from
// the SkillImpl classes of rAthena's src/map/skills/thief. The skills'
// other behavior is in server/zone's skills_assassin.go. The masteries
// (AS_RIGHT, AS_LEFT, AS_KATAR and ASC_KATAR) and Sonic Acceleration
// change damage in server/battle.

// Skill IDs of the Assassin and Assassin Cross, from rAthena's
// skill_db.yml.
const (
	IDRightHandMastery  = 132  // AS_RIGHT
	IDLeftHandMastery   = 133  // AS_LEFT
	IDKatarMastery      = 134  // AS_KATAR
	IDCloaking          = 135  // AS_CLOAKING
	IDSonicBlow         = 136  // AS_SONICBLOW
	IDGrimtooth         = 137  // AS_GRIMTOOTH
	IDEnchantPoison     = 138  // AS_ENCHANTPOISON
	IDPoisonReact       = 139  // AS_POISONREACT
	IDVenomDust         = 140  // AS_VENOMDUST
	IDVenomSplasher     = 141  // AS_SPLASHER
	IDSonicAcceleration = 1003 // AS_SONICACCEL
	IDVenomKnife        = 1004 // AS_VENOMKNIFE
	IDAdvKatarMastery   = 376  // ASC_KATAR
	IDEnchantDeadly     = 378  // ASC_EDP
	IDSoulBreaker       = 379  // ASC_BREAKER
	IDMeteorAssault     = 406  // ASC_METEORASSAULT
	IDCreateDeadly      = 407  // ASC_CDP
)

type (
	sonicBlow     struct{ WeaponSkill }
	grimtooth     struct{ Base }
	venomSplasher struct{ Base }
	venomKnife    struct{ WeaponSkill }
	soulBreaker   struct{ WeaponSkill }
	meteorAssault struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDRightHandMastery:  Base{},
		IDLeftHandMastery:   Base{},
		IDKatarMastery:      Base{},
		IDCloaking:          Base{},
		IDSonicBlow:         sonicBlow{},
		IDGrimtooth:         grimtooth{},
		IDEnchantPoison:     Base{},
		IDPoisonReact:       WeaponSkill{},
		IDVenomDust:         Base{},
		IDVenomSplasher:     venomSplasher{},
		IDSonicAcceleration: Base{},
		IDVenomKnife:        venomKnife{},
		IDAdvKatarMastery:   Base{},
		IDEnchantDeadly:     Base{},
		IDSoulBreaker:       soulBreaker{},
		IDMeteorAssault:     meteorAssault{},
		IDCreateDeadly:      Base{},
	})
}

// SkillRatio is SkillSonicBlow::calculateSkillRatio.
//   - Renewal: 200% + 100% per level, and half again against a target
//     below half its HP.
//   - Pre-renewal: 300% + 50% per level, and a tenth more with Sonic
//     Acceleration.
func (sonicBlow) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 100 + 100*a.Level()
		if t := a.Target; t.CurrentHP() < int64(t.MaxHP/2) {
			*ratio += *ratio / 2
		}
		return
	}
	*ratio += 200 + 50*a.Level()
	if a.Src.Player != nil && a.Src.Player.Skills[IDSonicAcceleration] > 0 {
		*ratio += *ratio / 10
	}
}

// HitRate is SkillSonicBlow::modifyHitRate: Sonic Acceleration adds 90%
// of the hit rate in renewal and 50% in pre-renewal.
func (sonicBlow) HitRate(a *battle.SkillContext, hit *int) {
	if a.Src.Player == nil || a.Src.Player.Skills[IDSonicAcceleration] <= 0 {
		return
	}
	if a.Renewal {
		*hit += *hit * 90 / 100
	} else {
		*hit += *hit * 50 / 100
	}
}

// SkillRatio is SkillGrimtooth::calculateSkillRatio: 100% + 20% per
// level.
func (grimtooth) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 20 * a.Level() }

// SkillRatio is SkillVenomSplasher::calculateSkillRatio: 400% + 100% per
// level in renewal, 500% + 50% per level in pre-renewal, and 20% per
// level of Poison React the player learned.
func (venomSplasher) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += -100 + 400 + 100*a.Level()
	} else {
		*ratio += 400 + 50*a.Level()
	}
	if sd := a.Src.Player; sd != nil {
		*ratio += 20 * sd.Skills[IDPoisonReact]
	}
}

// SkillRatio is SkillThrowVenomKnife::calculateSkillRatio: 500% in
// renewal.
func (venomKnife) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 400
	}
}

// SkillRatio is SkillSoulDestroyer::calculateSkillRatio. In renewal it
// is 150% per level plus STR and INT. In pre-renewal it is the weapon
// part, 100% per level; the INT part is a misc attack that
// server/battle adds.
func (soulBreaker) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += -100 + 150*a.Level() + a.Src.Str + a.Src.Int
		reLvlDmod(a, ratio, 100)
		return
	}
	*ratio += -100 + 100*a.Level()
}

// SkillRatio is SkillMeteorAssault::calculateSkillRatio.
func (meteorAssault) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 100 + 120*a.Level()
		reLvlDmod(a, ratio, 100)
		return
	}
	*ratio += -60 + 40*a.Level()
}
