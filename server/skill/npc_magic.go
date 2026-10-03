package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The monster magic, breath and area skills: the damage formulas of the
// SkillImpl classes of rAthena's src/map/skills/npc, and the cases of
// battle_calc_magic_attack and battle_calc_misc_attack that name them.
// Their behavior in the zone is in server/zone/skills_npc_magic.go.

// Monster skill IDs of this file, from rAthena's skill_db.yml.
const (
	IDNPCMagicalAttack  = 192 // NPC_MAGICALATTACK
	IDNPCDarkBreath     = 202 // NPC_DARKBREATH
	IDNPCDarkCross      = 338 // NPC_DARKCROSS
	IDNPCGrandDarkness  = 339 // NPC_GRANDDARKNESS
	IDNPCDarkStrike     = 340 // NPC_DARKSTRIKE
	IDNPCDarkThunder    = 341 // NPC_DARKTHUNDER
	IDNPCFireBreath     = 654 // NPC_FIREBREATH
	IDNPCIceBreath      = 655 // NPC_ICEBREATH
	IDNPCThunderBreath  = 656 // NPC_THUNDERBREATH
	IDNPCAcidBreath     = 657 // NPC_ACIDBREATH
	IDNPCDarknessBreath = 658 // NPC_DARKNESSBREATH
	IDNPCHellJudgement  = 662 // NPC_HELLJUDGEMENT
	IDNPCIceBreath2     = 765 // NPC_ICEBREATH2
	IDNPCHellJudgement2 = 768 // NPC_HELLJUDGEMENT2
	IDNPCEarthquake     = 653 // NPC_EARTHQUAKE
	IDNPCEvilLand       = 670 // NPC_EVILLAND
	IDNPCVenomFog       = 706 // NPC_VENOMFOG
	IDNPCComet          = 708 // NPC_COMET
	IDNPCIceMine        = 709 // NPC_ICEMINE
	IDNPCFlameCross     = 711 // NPC_FLAMECROSS
	IDNPCHellBurning    = 719 // NPC_HELLBURNING
	IDNPCJackFrost      = 720 // NPC_JACKFROST
	IDNPCStormGust2     = 723 // NPC_STORMGUST2
	IDNPCReverberation  = 725 // NPC_REVERBERATION
	IDNPCArrowStorm     = 728 // NPC_ARROWSTORM
	IDNPCCursedCircle   = 730 // NPC_SR_CURSEDCIRCLE
	IDNPCDragonBreath   = 731 // NPC_DRAGONBREATH
	IDNPCFatalMenace    = 732 // NPC_FATALMENACE
	IDNPCMagmaEruption  = 733 // NPC_MAGMA_ERUPTION
	IDNPCMandragora     = 735 // NPC_MANDRAGORA
	IDNPCPsychicWave    = 736 // NPC_PSYCHIC_WAVE
	IDNPCRayOfGenesis   = 737 // NPC_RAYOFGENESIS
	IDNPCVenomImpress   = 738 // NPC_VENOMIMPRESS
	IDNPCCloudKill      = 739 // NPC_CLOUD_KILL
	IDNPCIgnitionBreak  = 740 // NPC_IGNITIONBREAK
	IDNPCPoisonBuster   = 742 // NPC_POISON_BUSTER
	IDNPCElectricWalk   = 744 // NPC_ELECTRICWALK
	IDNPCFireWalk       = 745 // NPC_FIREWALK
	IDNPCRainOfMeteor   = 769 // NPC_RAINOFMETEOR
	IDNPCCaneOfEvilEye  = 780 // NPC_CANE_OF_EVIL_EYE
	// The eruption hits of NPC_MAGMA_ERUPTION's unit.
	idNPCMagmaEruptionDot = 734  // NPC_MAGMA_ERUPTION_DOTDAMAGE
	idNCMagmaEruptionDot  = 5015 // NC_MAGMA_ERUPTION_DOTDAMAGE
	// IDNPCReverberationAtk is the hit of NPC_REVERBERATION's unit.
	IDNPCReverberationAtk = 726 // NPC_REVERBERATION_ATK
)

type (
	// npcBreath is the breath skills: SkillFireBreath, SkillIceBreath,
	// SkillIceBreath2, SkillThunderBreath, SkillAcidBreath and
	// SkillDarknessBreath. Their formulas are the same.
	npcBreath struct{ Base }
	// npcHellJudgement is SkillHellsJudgement and SkillHellsJudgement2.
	npcHellJudgement struct{ Base }
	npcDarkCross     struct{ WeaponSkill }
	npcDarkBreath    struct{ Base }
	// npcMagic is the monster skills with no formula of their own:
	// SkillSoulStrikeOfDarkness, SkillDarknessJupitel,
	// SkillDemonShockAttack, SkillGrandCrossOfDarkness, SkillEarthquake
	// (its base damage is in server/battle), SkillReverberation2,
	// SkillNpcCursedCircle, SkillNpcHowlingOfMandragora and
	// SkillNpcVenomImpress.
	npcMagic struct{ Base }
	// npcRatio is the skills whose calculateSkillRatio adds a fixed part
	// and a part per level: add + perLv * level.
	npcRatio struct {
		Base
		add, perLv int
	}
	npcEvilLand struct{ Base }
	// npcWeaponAtk is SkillIceMine and SkillFlameCross.
	npcWeaponAtk    struct{ Base }
	npcArrowStorm   struct{ Base }
	npcDragonBreath struct{ WeaponSkill }
	npcIgnition     struct{ Base }
	npcCaneEvilEye  struct{ Base }
	npcFatalMenace  struct{ WeaponSkill }
	npcMagmaErupt   struct{ WeaponSkill }
	// npcMagmaDot is NPC_MAGMA_ERUPTION_DOTDAMAGE and ncMagmaDot is
	// SkillMagmaEruptionDotDamage (NC_MAGMA_ERUPTION_DOTDAMAGE).
	npcMagmaDot struct{ Base }
	ncMagmaDot  struct{ Base }
)

// NPCComet is SkillComet2's formula. CenterDistance is the distance from
// the target to the caster's last comet (sc->comet_x and comet_y), which
// the zone sets for each hit.
type NPCComet struct {
	Base
	CenterDistance int
}

// NPCJackFrost is SkillJackFrost2's formula. Freezing is set when the
// target has SC_FREEZING, which the zone sets for each hit.
type NPCJackFrost struct {
	Base
	Freezing bool
}

func init() {
	register(map[int]battle.SkillImpl{
		IDNPCMagicalAttack:    npcMagic{},
		IDNPCDarkStrike:       npcMagic{},
		IDNPCDarkThunder:      npcMagic{},
		IDNPCDarkBreath:       npcDarkBreath{},
		IDNPCDarkCross:        npcDarkCross{},
		IDNPCFireBreath:       npcBreath{},
		IDNPCIceBreath:        npcBreath{},
		IDNPCIceBreath2:       npcBreath{},
		IDNPCThunderBreath:    npcBreath{},
		IDNPCAcidBreath:       npcBreath{},
		IDNPCDarknessBreath:   npcBreath{},
		IDNPCHellJudgement:    npcHellJudgement{},
		IDNPCHellJudgement2:   npcHellJudgement{},
		IDNPCGrandDarkness:    npcMagic{},
		IDNPCEarthquake:       npcMagic{},
		IDNPCEvilLand:         npcEvilLand{},
		IDNPCStormGust2:       npcRatio{add: 0, perLv: 200},
		IDNPCCloudKill:        npcRatio{add: -100, perLv: 50},
		IDNPCVenomFog:         npcRatio{add: 600, perLv: 100},
		IDNPCPsychicWave:      npcRatio{add: -100, perLv: 500},
		IDNPCRayOfGenesis:     npcRatio{add: -100, perLv: 200},
		IDNPCPoisonBuster:     npcRatio{add: -100, perLv: 1500},
		IDNPCElectricWalk:     npcRatio{add: -100, perLv: 100},
		IDNPCFireWalk:         npcRatio{add: -100, perLv: 100},
		IDNPCRainOfMeteor:     npcRatio{add: 350, perLv: 0},
		IDNPCHellBurning:      npcRatio{add: 900, perLv: 0},
		IDNPCComet:            NPCComet{CenterDistance: 8},
		IDNPCJackFrost:        NPCJackFrost{},
		IDNPCArrowStorm:       npcArrowStorm{},
		IDNPCDragonBreath:     npcDragonBreath{},
		IDNPCIgnitionBreak:    npcIgnition{},
		IDNPCIceMine:          npcWeaponAtk{},
		IDNPCFlameCross:       npcWeaponAtk{},
		IDNPCCaneOfEvilEye:    npcCaneEvilEye{},
		IDNPCReverberation:    npcMagic{},
		IDNPCVenomImpress:     npcMagic{},
		IDNPCCursedCircle:     npcMagic{},
		IDNPCMandragora:       npcMagic{},
		IDNPCFatalMenace:      npcFatalMenace{},
		IDNPCMagmaEruption:    npcMagmaErupt{},
		idNPCMagmaEruptionDot: npcMagmaDot{},
		idNCMagmaEruptionDot:  ncMagmaDot{},
		IDNPCReverberationAtk: npcRatio{add: 400, perLv: 200},
	})
}

// SkillRatio is the breaths' calculateSkillRatio: 100% more per level
// above 1.
func (npcBreath) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 100 * (a.Level() - 1) }

// HitRate is the breaths' modifyHitRate: twice the chance to hit.
func (npcBreath) HitRate(_ *battle.SkillContext, hit *int) { *hit *= 2 }

// SkillRatio is SkillHellsJudgement::calculateSkillRatio and
// SkillHellsJudgement2's: 100% more per level above 1.
func (npcHellJudgement) SkillRatio(a *battle.SkillContext, ratio *int) {
	*ratio += 100 * (a.Level() - 1)
}

// SkillRatio is SkillCrossOfDarkness::calculateSkillRatio.
func (npcDarkCross) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 35 * a.Level() }

// BaseDamage is the NPC_DARKBREATH case of battle_calc_magic_attack: a
// part of the target's HP, from 10% at level 1 to 50% at level 5 and
// above.
func (npcDarkBreath) BaseDamage(a *battle.SkillContext, _ battle.Rand) int64 {
	lv := a.Level()
	pct := 50
	if lv <= 5 {
		pct = 100 / (2 * (6 - lv))
	}
	return a.Target.HP * int64(pct) / 100
}

// SkillRatio is calculateSkillRatio of SkillStormGust2,
// SkillNpcCloudKill, SkillVenomFog, SkillNpcPsychicWave,
// SkillNpcRayOfGenesis, SkillNpcPoisonBuster, SkillNpcElectricWalk,
// SkillNpcFireWalk, SkillRainOfMeteor, SkillHellBurning and
// SkillReverberationAttack.
func (n npcRatio) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += n.add + n.perLv*a.Level() }

// BaseDamage is the NPC_EVILLAND case of battle_calc_misc_attack:
// skill_calc_heal with heal false.
func (npcEvilLand) BaseDamage(a *battle.SkillContext, r battle.Rand) int64 {
	return CalcHeal(a.Src, a.Target, IDNPCEvilLand, a.Level(), false, a.Ruleset().HealFormula, r)
}

// SkillRatio is SkillComet2::calculateSkillRatio: 2500% + 500% per level,
// less 500% per 2 cells from the comet's center, from 1 to 4 steps. It
// replaces the ratio.
func (c NPCComet) SkillRatio(a *battle.SkillContext, ratio *int) {
	i := min(max(c.CenterDistance/2, 1), 4)
	*ratio = 2500 + (a.Level()-i+1)*500
}

// SkillRatio is SkillJackFrost2::calculateSkillRatio: more against a
// target with SC_FREEZING. Both parts grow with the caster's level in
// renewal (RE_LVL_DMOD).
func (j NPCJackFrost) SkillRatio(a *battle.SkillContext, ratio *int) {
	if j.Freezing {
		*ratio += 900 + 300*a.Level()
		reLvlDmod(a, ratio, 100)
		return
	}
	*ratio += 400 + 100*a.Level()
	reLvlDmod(a, ratio, 150)
}

// SkillRatio is SkillNpcArrowStorm::calculateSkillRatio.
func (npcArrowStorm) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Level() > 4 {
		*ratio += 1900
	} else {
		*ratio += 900
	}
}

// SkillRatio is SkillNpcDragonBreath::calculateSkillRatio: levels 1 to
// 5 are fire, as RK_DRAGONBREATH, and 6 to 10 water, as
// RK_DRAGONBREATH_WATER.
func (npcDragonBreath) SkillRatio(a *battle.SkillContext, ratio *int) {
	if lv := a.Level(); lv > 5 {
		*ratio += 500 + 500*(lv-5)
	} else {
		*ratio += 500 + 500*lv
	}
}

// SkillRatio is SkillNpcIgnitionBreak::calculateSkillRatio: less with
// the distance from the caster.
func (npcIgnition) SkillRatio(a *battle.SkillContext, ratio *int) {
	lv := a.Level()
	switch i := a.Skill.Distance; {
	case i < 2:
		*ratio += -100 + 500*(lv+1)
	case i < 4:
		*ratio += -100 + 250 + 500*lv
	default:
		*ratio += -100 + 500*lv
	}
}

// BaseDamage is the NPC_CANE_OF_EVIL_EYE case of
// battle_calc_misc_attack: 15000.
func (npcCaneEvilEye) BaseDamage(*battle.SkillContext, battle.Rand) int64 { return 15000 }

// BaseDamage is the NPC_ICEMINE and NPC_FLAMECROSS case of
// battle_calc_magic_attack: the weapon's ATK times 20 per level.
func (npcWeaponAtk) BaseDamage(a *battle.SkillContext, _ battle.Rand) int64 {
	return int64(a.Src.RHW.Atk) * 20 * int64(a.Level())
}

// BaseDamage is the NPC_MAGMA_ERUPTION_DOTDAMAGE case of
// battle_calc_misc_attack: 1000 per level.
func (npcMagmaDot) BaseDamage(a *battle.SkillContext, _ battle.Rand) int64 {
	return 1000 * int64(a.Level())
}

// BaseDamage is the NC_MAGMA_ERUPTION_DOTDAMAGE case of
// battle_calc_misc_attack: 800 + 200 per level.
func (ncMagmaDot) BaseDamage(a *battle.SkillContext, _ battle.Rand) int64 {
	return 800 + 200*int64(a.Level())
}
