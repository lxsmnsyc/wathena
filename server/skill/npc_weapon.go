package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The monster weapon attacks: the damage formulas of the NPC_ skills that
// hit, drain or break, as the calculateSkillRatio and modifyHitRate
// methods of rAthena's SkillImpl classes in src/map/skills/npc. The
// skills' other behavior is in server/zone/skills_npc_weapon.go.

// Skill IDs of the monster weapon attacks, from rAthena's skill_db.yml.
const (
	idNpcPiercingAtt        = 158 // NPC_PIERCINGATT
	idNpcMentalBreaker      = 159 // NPC_MENTALBREAKER
	idNpcRangeAttack        = 160 // NPC_RANGEATTACK
	idNpcCriticalSlash      = 170 // NPC_CRITICALSLASH
	idNpcComboAttack        = 171 // NPC_COMBOATTACK
	idNpcGuidedAttack       = 172 // NPC_GUIDEDATTACK
	idNpcSplashAttack       = 174 // NPC_SPLASHATTACK
	idNpcRandomAttack       = 183 // NPC_RANDOMATTACK
	idNpcWaterAttack        = 184 // NPC_WATERATTACK
	idNpcGroundAttack       = 185 // NPC_GROUNDATTACK
	idNpcFireAttack         = 186 // NPC_FIREATTACK
	idNpcWindAttack         = 187 // NPC_WINDATTACK
	idNpcPoisonAttack       = 188 // NPC_POISONATTACK
	idNpcHolyAttack         = 189 // NPC_HOLYATTACK
	idNpcDarknessAttack     = 190 // NPC_DARKNESSATTACK
	idNpcTelekinesisAttack  = 191 // NPC_TELEKINESISATTACK
	idNpcBloodDrain         = 199 // NPC_BLOODDRAIN
	idNpcEnergyDrain        = 200 // NPC_ENERGYDRAIN
	idNpcArmorBrake         = 344 // NPC_ARMORBRAKE
	idNpcHelmBrake          = 345 // NPC_HELMBRAKE
	idNpcShieldBrake        = 346 // NPC_SHIELDBRAKE
	idNpcUndeadAttack       = 347 // NPC_UNDEADATTACK
	idNpcPulseStrike        = 661 // NPC_PULSESTRIKE
	idNpcCriticalWound      = 673 // NPC_CRITICALWOUND
	idNpcPulseStrike2       = 712 // NPC_PULSESTRIKE2
	idNpcDancingBladeAttack = 714 // NPC_DANCINGBLADE_ATK
	idNpcDarkPiercing       = 715 // NPC_DARKPIERCING
	idNpcPhantomThrust      = 741 // NPC_PHANTOMTHRUST
)

type (
	// npcElementAttack is the element attacks, as
	// SkillEarthAttributeAttack, SkillWaterAttributeAttack,
	// SkillFireAttributeAttack, SkillWindAttributeAttack,
	// SkillShadowAttributeAttack, SkillPoisonAttributeAttack,
	// SkillHolyAttributeAttack, SkillGhostAttributeAttack and
	// SkillUndeadElementAttack.
	npcElementAttack struct{ WeaponSkill }
	// npcComboAttack is SkillMultiStageAttack.
	npcComboAttack struct{ WeaponSkill }
	// npcPiercingAttack is SkillPiercingAttack.
	npcPiercingAttack struct{ WeaponSkill }
	// npcLevelRatio is the skills whose ratio grows 100% per level after
	// the first: SkillRandomAttack, SkillSuckingBlood and SkillPulseStrike.
	npcLevelRatio struct{ Base }
	// npcEnergyDrain is SkillEnergyDrain.
	npcEnergyDrain struct{ Base }
	// npcMentalBreaker is SkillSpiritDestruction.
	npcMentalBreaker struct{ WeaponSkill }
	// npcPulseStrike2 is SkillPulseStrike2.
	npcPulseStrike2 struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		idNpcGroundAttack:      npcElementAttack{},
		idNpcWaterAttack:       npcElementAttack{},
		idNpcFireAttack:        npcElementAttack{},
		idNpcWindAttack:        npcElementAttack{},
		idNpcDarknessAttack:    npcElementAttack{},
		idNpcPoisonAttack:      npcElementAttack{},
		idNpcHolyAttack:        npcElementAttack{},
		idNpcTelekinesisAttack: npcElementAttack{},
		idNpcUndeadAttack:      npcElementAttack{},
		idNpcComboAttack:       npcComboAttack{},
		// WeaponSkillImpl without formulas of their own.
		idNpcCriticalSlash: WeaponSkill{},
		idNpcGuidedAttack:  WeaponSkill{},
		idNpcRangeAttack:   WeaponSkill{},
		idNpcCriticalWound: WeaponSkill{},
		idNpcArmorBrake:    WeaponSkill{},
		idNpcHelmBrake:     WeaponSkill{},
		idNpcShieldBrake:   WeaponSkill{},
		idNpcPhantomThrust: WeaponSkill{},
		// SkillImplRecursiveDamageSplash and SkillImpl without formulas.
		idNpcSplashAttack:       Base{},
		idNpcDarkPiercing:       Base{},
		idNpcDancingBladeAttack: Base{},
		idNpcPiercingAtt:        npcPiercingAttack{},
		idNpcRandomAttack:       npcLevelRatio{},
		idNpcBloodDrain:         npcLevelRatio{},
		idNpcPulseStrike:        npcLevelRatio{},
		idNpcEnergyDrain:        npcEnergyDrain{},
		idNpcMentalBreaker:      npcMentalBreaker{},
		idNpcPulseStrike2:       npcPulseStrike2{},
	})
}

// SkillRatio is the element attacks' calculateSkillRatio: 100% more per
// level after the first.
func (npcElementAttack) SkillRatio(a *battle.SkillContext, ratio *int) {
	*ratio += 100 * (a.Level() - 1)
}

// HitRate is the element attacks' modifyHitRate: 20% more.
func (npcElementAttack) HitRate(_ *battle.SkillContext, hit *int) { *hit += *hit * 20 / 100 }

// SkillRatio is SkillMultiStageAttack::calculateSkillRatio: 25% more per
// level.
func (npcComboAttack) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 25 * a.Level() }

// SkillRatio is SkillPiercingAttack::calculateSkillRatio: 75% base
// damage.
func (npcPiercingAttack) SkillRatio(_ *battle.SkillContext, ratio *int) { *ratio += -25 }

// SkillRatio is the calculateSkillRatio of SkillRandomAttack,
// SkillSuckingBlood and SkillPulseStrike: 100% more per level after the
// first.
func (npcLevelRatio) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 100 * (a.Level() - 1) }

// SkillRatio is SkillEnergyDrain::calculateSkillRatio: 100% more per
// level.
func (npcEnergyDrain) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 100 * a.Level() }

// HitRate is SkillSpiritDestruction::modifyHitRate: 20% more.
func (npcMentalBreaker) HitRate(_ *battle.SkillContext, hit *int) { *hit += *hit * 20 / 100 }

// SkillRatio is SkillPulseStrike2::calculateSkillRatio: 100% more.
func (npcPulseStrike2) SkillRatio(_ *battle.SkillContext, ratio *int) { *ratio += 100 }
