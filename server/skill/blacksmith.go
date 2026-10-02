package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The Blacksmith and Whitesmith skills: Cart Termination's damage
// formula, as SkillCartTermination::calculateSkillRatio in
// src/map/skills/merchant. The other skills change no damage formula:
// their statuses are in server/status, server/statuschange and
// server/battle, and their behavior is in
// server/zone/skills_blacksmith.go.

// Skill IDs of the Blacksmith and Whitesmith, from rAthena's
// skill_db.yml.
const (
	IDIron            = 94   // BS_IRON
	IDSteel           = 95   // BS_STEEL
	IDEnchantedStone  = 96   // BS_ENCHANTEDSTONE
	IDOrideocon       = 97   // BS_ORIDEOCON
	IDSmithDagger     = 98   // BS_DAGGER
	IDSmithSword      = 99   // BS_SWORD
	IDSmithTwoHand    = 100  // BS_TWOHANDSWORD
	IDSmithAxe        = 101  // BS_AXE
	IDSmithMace       = 102  // BS_MACE
	IDSmithKnuckle    = 103  // BS_KNUCKLE
	IDSmithSpear      = 104  // BS_SPEAR
	IDHiltBinding     = 105  // BS_HILTBINDING
	IDFindingOre      = 106  // BS_FINDINGORE
	IDWeaponResearch  = 107  // BS_WEAPONRESEARCH
	IDRepairWeapon    = 108  // BS_REPAIRWEAPON
	IDSkinTemper      = 109  // BS_SKINTEMPER
	IDHammerFall      = 110  // BS_HAMMERFALL
	IDAdrenaline      = 111  // BS_ADRENALINE
	IDWeaponPerfect   = 112  // BS_WEAPONPERFECT
	IDOverThrust      = 113  // BS_OVERTHRUST
	IDMaximize        = 114  // BS_MAXIMIZE
	IDMeltdown        = 384  // WS_MELTDOWN
	IDCartBoost       = 387  // WS_CARTBOOST
	IDAdrenaline2     = 459  // BS_ADRENALINE2
	IDWeaponRefine    = 477  // WS_WEAPONREFINE
	IDCartTermination = 485  // WS_CARTTERMINATION
	IDOverThrustMax   = 486  // WS_OVERTHRUSTMAX
	IDGreed           = 1013 // BS_GREED
)

type (
	cartTermination struct{ WeaponSkill }
	// smithSkill are the Blacksmith and Whitesmith skills without a
	// damage formula: the forging and passive skills, which item
	// creation and the stat calculation read, and the skills whose
	// effects are statuses, an area status, a timed status or an item
	// window.
	smithSkill struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDIron:            smithSkill{},
		IDSteel:           smithSkill{},
		IDEnchantedStone:  smithSkill{},
		IDOrideocon:       smithSkill{},
		IDSmithDagger:     smithSkill{},
		IDSmithSword:      smithSkill{},
		IDSmithTwoHand:    smithSkill{},
		IDSmithAxe:        smithSkill{},
		IDSmithMace:       smithSkill{},
		IDSmithKnuckle:    smithSkill{},
		IDSmithSpear:      smithSkill{},
		IDHiltBinding:     smithSkill{},
		IDFindingOre:      smithSkill{},
		IDWeaponResearch:  smithSkill{},
		IDRepairWeapon:    smithSkill{},
		IDSkinTemper:      smithSkill{},
		IDHammerFall:      smithSkill{},
		IDAdrenaline:      smithSkill{},
		IDWeaponPerfect:   smithSkill{},
		IDOverThrust:      smithSkill{},
		IDMaximize:        smithSkill{},
		IDMeltdown:        smithSkill{},
		IDCartBoost:       smithSkill{},
		IDAdrenaline2:     smithSkill{},
		IDWeaponRefine:    smithSkill{},
		IDCartTermination: cartTermination{},
		IDOverThrustMax:   smithSkill{},
		IDUnfairTrick:     smithSkill{},
		IDGreed:           smithSkill{},
	})
}

// SkillRatio is SkillCartTermination::calculateSkillRatio: the cart's
// weight over a divisor of 10 per level below 16, scaled to the maximum
// cart weight, so that a full cart deals 80000% over the divisor.
// Units that are not players deal what a full cart deals.
func (cartTermination) SkillRatio(a *battle.SkillContext, ratio *int) {
	i := max(10*(16-a.Level()), 1)
	if sd := a.Src.Player; sd != nil && sd.CartWeight != 0 {
		*ratio += sd.CartWeight/i*80000/MaxCartWeight - 100
	} else if sd == nil {
		*ratio += 80000/i - 100
	}
}
