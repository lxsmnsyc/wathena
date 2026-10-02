package skill

import (
	"github.com/lxsmnsyc/wathena/server/battle"
)

// The Hunter's skills and Beast Strafing and Phantasmic Arrow, its quest
// skills: the SkillImpl classes of rAthena's src/map/skills/archer. The
// traps' misc damage is battle_calc_misc_attack's, given here as each
// trap's base damage. The traps' units, Remove Trap, Spring Trap, Detect
// and the falcon are in the zone.

// Skill IDs of the Hunter, from rAthena's skill_db.yml.
const (
	IDSkidTrap     = 115  // HT_SKIDTRAP
	IDLandMine     = 116  // HT_LANDMINE
	IDAnkleSnare   = 117  // HT_ANKLESNARE
	IDShockwave    = 118  // HT_SHOCKWAVE
	IDSandman      = 119  // HT_SANDMAN
	IDFlasher      = 120  // HT_FLASHER
	IDFreezingTrap = 121  // HT_FREEZINGTRAP
	IDBlastMine    = 122  // HT_BLASTMINE
	IDClaymoreTrap = 123  // HT_CLAYMORETRAP
	IDRemoveTrap   = 124  // HT_REMOVETRAP
	IDTalkieBox    = 125  // HT_TALKIEBOX
	IDBeastBane    = 126  // HT_BEASTBANE
	IDFalcon       = 127  // HT_FALCON
	IDSteelCrow    = 128  // HT_STEELCROW
	IDBlitzBeat    = 129  // HT_BLITZBEAT
	IDDetecting    = 130  // HT_DETECTING
	IDSpringTrap   = 131  // HT_SPRINGTRAP
	IDBeastStrafe  = 499  // HT_POWER
	IDPhantasmic   = 1009 // HT_PHANTASMIC
	IDResearchTrap = 2248 // RA_RESEARCHTRAP
)

type (
	// trap is a trap without a damage formula: Skid Trap, Ankle Snare,
	// Shockwave Trap, Sandman, Flasher and Talkie Box. Their units are in
	// the zone.
	trap         struct{ Base }
	landMine     struct{ Base }
	blastMine    struct{ Base }
	claymoreTrap struct{ Base }
	freezingTrap struct{ Base }
	// hunterSkill is a Hunter skill whose behavior is all in the zone:
	// Remove Trap, Spring Trap and Detect.
	hunterSkill   struct{ Base }
	blitzBeat     struct{ Base }
	beastStrafing struct{ WeaponSkill }
	phantasmic    struct{ WeaponSkill }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDSkidTrap:     trap{},
		IDLandMine:     landMine{},
		IDAnkleSnare:   trap{},
		IDShockwave:    trap{},
		IDSandman:      trap{},
		IDFlasher:      trap{},
		IDFreezingTrap: freezingTrap{},
		IDBlastMine:    blastMine{},
		IDClaymoreTrap: claymoreTrap{},
		IDRemoveTrap:   hunterSkill{},
		IDTalkieBox:    trap{},
		IDBlitzBeat:    blitzBeat{},
		IDDetecting:    hunterSkill{},
		IDSpringTrap:   hunterSkill{},
		IDBeastStrafe:  beastStrafing{},
		IDPhantasmic:   phantasmic{},
	})
}

// renewalTrapDamage is battle_calc_misc_attack's damage of Land Mine,
// Blast Mine and Claymore Trap in renewal: DEX, base level and INT, a
// roll of -10% to +9%, then 40 per level of Research Trap.
func renewalTrapDamage(a *battle.SkillContext, r battle.Rand) int64 {
	st := a.Src
	dmg := int64(float64(a.Level()*st.Dex) * (3.0 + float64(float32(st.Level))/100.0) *
		(1.0 + float64(float32(st.Int))/35.0))
	dmg += dmg * int64(r.IntN(20)-10) / 100
	if st.Player != nil {
		dmg += int64(st.Player.Skills[IDResearchTrap] * 40)
	}
	return dmg
}

// BaseDamage is HT_LANDMINE's damage in battle_calc_misc_attack.
func (landMine) BaseDamage(a *battle.SkillContext, r battle.Rand) int64 {
	if a.Renewal {
		return renewalTrapDamage(a, r)
	}
	st := a.Src
	return int64(float64(a.Level()) * (float64(st.Dex) + 75.0) * (100.0 + float64(st.Int)) / 100.0)
}

// BaseDamage is HT_BLASTMINE's damage in battle_calc_misc_attack.
func (blastMine) BaseDamage(a *battle.SkillContext, r battle.Rand) int64 {
	if a.Renewal {
		return renewalTrapDamage(a, r)
	}
	st := a.Src
	return int64(float64(a.Level()) * (float64(st.Dex)/2.0 + 50.0) * (100.0 + float64(st.Int)) / 100.0)
}

// BaseDamage is HT_CLAYMORETRAP's damage in battle_calc_misc_attack.
func (claymoreTrap) BaseDamage(a *battle.SkillContext, r battle.Rand) int64 {
	if a.Renewal {
		return renewalTrapDamage(a, r)
	}
	st := a.Src
	return int64(float64(a.Level()) * (float64(st.Dex)/2.0 + 75.0) * (100.0 + float64(st.Int)) / 100.0)
}

// blitzDamage is battle_calc_misc_attack's damage of HT_BLITZBEAT and
// the base of SN_FALCONASSAULT: Steel Crow, AGI and DEX in renewal; DEX,
// INT and Steel Crow in pre-renewal.
func blitzDamage(a *battle.SkillContext, lv int) int64 {
	st := a.Src
	steelCrow := 0
	if st.Player != nil {
		steelCrow = st.Player.Skills[IDSteelCrow]
	}
	if a.Renewal {
		return int64(lv*20 + steelCrow*6 + (st.Agi/2)*2 + (st.Dex/10)*2)
	}
	return int64((st.Dex/10 + st.Int/2 + steelCrow*3 + 40) * 2)
}

// BaseDamage is HT_BLITZBEAT's damage in battle_calc_misc_attack. In
// pre-renewal, an automatic Blitz Beat splits its damage among its
// targets (mflag above 1); battle sets the split.
func (blitzBeat) BaseDamage(a *battle.SkillContext, _ battle.Rand) int64 {
	return blitzDamage(a, a.Level())
}

// SkillRatio is SkillBeastStrafing::calculateSkillRatio.
func (beastStrafing) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += -50 + 8*a.Src.Str }

// SkillRatio is SkillPhantasmicArrow::calculateSkillRatio.
func (phantasmic) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 400
	} else {
		*ratio += 50
	}
}
