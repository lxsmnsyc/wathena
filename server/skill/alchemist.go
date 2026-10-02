package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The Alchemist and Creator skills: their damage formulas, from the
// SkillImpl classes of rAthena's src/map/skills/merchant. The skills'
// other behavior is in server/zone's skills_alchemist.go. Axe Mastery
// and Potion Research change damage and heals in server/battle and
// server/item.

// Skill IDs of the Alchemist and Creator, from rAthena's skill_db.yml.
const (
	IDAxeMastery        = 226 // AM_AXEMASTERY
	IDLearningPotion    = 227 // AM_LEARNINGPOTION
	IDPharmacy          = 228 // AM_PHARMACY
	IDBomb              = 229 // AM_DEMONSTRATION
	IDAcidTerror        = 230 // AM_ACIDTERROR
	IDPotionPitcher     = 231 // AM_POTIONPITCHER
	IDCannibalize       = 232 // AM_CANNIBALIZE
	IDSphereMine        = 233 // AM_SPHEREMINE
	IDCPWeapon          = 234 // AM_CP_WEAPON
	IDCPShield          = 235 // AM_CP_SHIELD
	IDCPArmor           = 236 // AM_CP_ARMOR
	IDCPHelm            = 237 // AM_CP_HELM
	IDBioethics         = 238 // AM_BIOETHICS
	IDCallHomunculus    = 243 // AM_CALLHOMUN
	IDVaporize          = 244 // AM_REST
	IDResurrectHomun    = 247 // AM_RESURRECTHOMUN
	IDBerserkPitcher    = 446 // AM_BERSERKPITCHER
	IDSlimPitcher       = 478 // CR_SLIMPITCHER
	IDFullProtection    = 479 // CR_FULLPROTECTION
	IDAcidDemonstration = 490 // CR_ACIDDEMONSTRATION
	IDCultivation       = 491 // CR_CULTIVATION
	IDTwilight1         = 496 // AM_TWILIGHT1
	IDTwilight2         = 497 // AM_TWILIGHT2
	IDTwilight3         = 498 // AM_TWILIGHT3
)

type (
	bomb              struct{ Base }
	acidTerror        struct{ WeaponSkill }
	acidDemonstration struct{ WeaponSkill }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDAxeMastery:        Base{},
		IDLearningPotion:    Base{},
		IDPharmacy:          Base{},
		IDBomb:              bomb{},
		IDAcidTerror:        acidTerror{},
		IDPotionPitcher:     Base{},
		IDCannibalize:       Base{},
		IDSphereMine:        Base{},
		IDCPWeapon:          Base{},
		IDCPShield:          Base{},
		IDCPArmor:           Base{},
		IDCPHelm:            Base{},
		IDBioethics:         Base{},
		IDCallHomunculus:    Base{},
		IDVaporize:          Base{},
		IDResurrectHomun:    Base{},
		IDBerserkPitcher:    Base{},
		IDSlimPitcher:       Base{},
		IDFullProtection:    Base{},
		IDAcidDemonstration: acidDemonstration{},
		IDCultivation:       Base{},
		IDTwilight1:         Base{},
		IDTwilight2:         Base{},
		IDTwilight3:         Base{},
	})
}

// SkillRatio is SkillBomb::calculateSkillRatio: 100% + 20% per level.
func (bomb) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 20 * a.Level() }

// SkillRatio is SkillAcidTerror::calculateSkillRatio.
//   - Renewal: 200% per level, and 100% more for a player who learned
//     Potion Research.
//   - Pre-renewal: 50% + 50% per level.
func (acidTerror) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += -100 + 200*a.Level()
		if sd := a.Src.Player; sd != nil && sd.Skills[IDLearningPotion] > 0 {
			*ratio += 100
		}
		return
	}
	*ratio += -50 + 50*a.Level()
}

// SkillRatio is SkillAcidDemonstration::calculateSkillRatio. In renewal
// it is 200% per level plus the caster's INT and the target's VIT, and
// half of that against a player. In pre-renewal the skill is a misc
// attack, whose damage BaseDamage gives.
func (acidDemonstration) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		return
	}
	*ratio += -100 + 200*a.Level() + a.Src.Int + a.Target.Vit
	if a.Target.Player != nil {
		*ratio /= 2
	}
}

// BaseDamage is battle_calc_misc_attack for pre-renewal's
// CR_ACIDDEMONSTRATION: 0.7 * VIT * INT^2 / (VIT + INT) of the target's
// VIT and the caster's INT, halved against a player.
func (acidDemonstration) BaseDamage(a *battle.SkillContext, _ battle.Rand) int64 {
	vit, int_ := a.Target.Vit, a.Src.Int
	if vit+int_ == 0 {
		return 0
	}
	d := int64(int32(int64(7) * int64(vit) * int64(int_) * int64(int_) / (10 * int64(vit+int_)))) // #nosec G115 -- rAthena keeps md.damage's formula in an int32 here.
	if a.Target.Player != nil {
		d /= 2
	}
	return d
}

// SkillHealBonus is pc_skillheal_bonus: the healer's percent more
// healing for a skill. bHealPower counts for the skills of
// skill_add_heal_rate (Aid Potion, not Aid Condensed Potion), and
// bSkillHeal for its skill.
func SkillHealBonus(sd *battle.Player, id int) int {
	if sd == nil {
		return 0
	}
	b := 0
	if healPowerSkills[id] {
		b = sd.AddHealRate
	}
	return b + sd.SkillHeal[id]
}

// SkillHeal2Bonus is pc_skillheal2_bonus: the target's percent more
// healing received, from bAddHeal2Rate and bSkillHeal2.
func SkillHeal2Bonus(tsd *battle.Player, id int) int {
	if tsd == nil {
		return 0
	}
	return tsd.AddHeal2Rate + tsd.SkillHeal2[id]
}
