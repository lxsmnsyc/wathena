package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The Wizard and High Wizard skills: rAthena's SkillImpl classes of
// src/map/skills/mage. Their behavior in the zone is in
// server/zone/skills_wizard.go.

// Wizard and High Wizard skill IDs, from rAthena's skill_db.yml.
const (
	IDFirePillar   = 80   // WZ_FIREPILLAR
	IDSightRasher  = 81   // WZ_SIGHTRASHER
	IDMeteor       = 83   // WZ_METEOR
	IDJupitel      = 84   // WZ_JUPITEL
	IDVermilion    = 85   // WZ_VERMILION
	IDWaterBall    = 86   // WZ_WATERBALL
	IDIceWall      = 87   // WZ_ICEWALL
	IDFrostNova    = 88   // WZ_FROSTNOVA
	IDStormGust    = 89   // WZ_STORMGUST
	IDEarthSpike   = 90   // WZ_EARTHSPIKE
	IDHeavenDrive  = 91   // WZ_HEAVENDRIVE
	IDQuagmire     = 92   // WZ_QUAGMIRE
	IDEstimation   = 93   // WZ_ESTIMATION
	IDSoulDrain    = 364  // HW_SOULDRAIN
	IDMagicCrasher = 365  // HW_MAGICCRASHER
	IDMagicPower   = 366  // HW_MAGICPOWER
	IDNapalmVulcan = 400  // HW_NAPALMVULCAN
	IDGanbantein   = 483  // HW_GANBANTEIN
	IDGravitation  = 484  // HW_GRAVITATION
	IDSightBlaster = 1006 // WZ_SIGHTBLASTER
)

type (
	firePillar   struct{ Base }
	sightRasher  struct{ Base }
	meteorStorm  struct{ Base }
	jupitel      struct{ Base }
	vermilion    struct{ Base }
	waterBall    struct{ Base }
	iceWall      struct{ Base }
	frostNova    struct{ Base }
	stormGust    struct{ Base }
	earthSpike   struct{ Base }
	heavenDrive  struct{ Base }
	quagmire     struct{ Base }
	sense        struct{ Base }
	soulDrain    struct{ Base }
	magicCrasher struct{ WeaponSkill }
	magicPower   struct{ Base }
	napalmVulcan struct{ Base }
	ganbantein   struct{ Base }
	gravitation  struct{ Base }
	sightBlaster struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDFirePillar:   firePillar{},
		IDSightRasher:  sightRasher{},
		IDMeteor:       meteorStorm{},
		IDJupitel:      jupitel{},
		IDVermilion:    vermilion{},
		IDWaterBall:    waterBall{},
		IDIceWall:      iceWall{},
		IDFrostNova:    frostNova{},
		IDStormGust:    stormGust{},
		IDEarthSpike:   earthSpike{},
		IDHeavenDrive:  heavenDrive{},
		IDQuagmire:     quagmire{},
		IDEstimation:   sense{},
		IDSoulDrain:    soulDrain{},
		IDMagicCrasher: magicCrasher{},
		IDMagicPower:   magicPower{},
		IDNapalmVulcan: napalmVulcan{},
		IDGanbantein:   ganbantein{},
		IDGravitation:  gravitation{},
		IDSightBlaster: sightBlaster{},
	})
}

// SkillRatio is SkillFirePillar::calculateSkillRatio: 20% of MATK per
// hit, before the constant that battle_calc_magic_attack adds.
func (firePillar) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += -60 + 20*a.Level() }

// ModifyDamageData is SkillFirePillar::modifyDamageData: a player's
// damage is divided among the hits.
func (firePillar) ModifyDamageData(a *battle.SkillContext, d *battle.DamageData) {
	if a.Src.Player != nil && d.Div > 0 {
		d.Div = -d.Div
	}
}

// ConstantDamage is the constant that battle_calc_magic_attack adds to
// Fire Pillar after its ratio.
func (firePillar) ConstantDamage(a *battle.SkillContext) int64 { return int64(100 + 50*a.Level()) }

// SkillRatio is SkillSightRasher::calculateSkillRatio.
func (sightRasher) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 20 * a.Level() }

// SkillRatio is SkillMeteorStorm::calculateSkillRatio: 125% in renewal.
func (meteorStorm) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 25
	}
}

// SkillRatio is SkillLordOfVermilion::calculateSkillRatio. Monsters keep
// the pre-renewal formula in renewal.
func (vermilion) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal && a.Src.Player != nil {
		*ratio += 300 + a.Level()*100
		return
	}
	*ratio += 20*a.Level() - 20
}

// SkillRatio is SkillWaterBall::calculateSkillRatio.
func (waterBall) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 30 * a.Level() }

// SkillRatio is SkillFrostNova::calculateSkillRatio: Frost Diver's in
// renewal, two thirds of it in pre-renewal.
func (frostNova) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 10 * a.Level()
		return
	}
	*ratio += -100 + (100+a.Level()*10)*2/3
}

// SkillRatio is SkillStormGust::calculateSkillRatio.
func (stormGust) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio -= 30
		*ratio += 50 * a.Level()
		return
	}
	*ratio += 40 * a.Level()
}

// SkillRatio is SkillEarthSpike::calculateSkillRatio: 200% in renewal.
// SC_EARTH_CARE_OPTION, which multiplies it, comes from an elemental,
// and elementals do not exist yet.
func (earthSpike) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 100
	}
}

// SkillRatio is SkillHeavensDrive::calculateSkillRatio: 125% in renewal.
func (heavenDrive) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 25
	}
}

// SkillRatio is SkillSightBlaster::calculateSkillRatio: 600% in renewal.
func (sightBlaster) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 500
	}
}

// SkillRatio is SkillNapalmVulcan::calculateSkillRatio.
func (napalmVulcan) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += -100 + 70*a.Level()
		reLvlDmod(a, ratio, 100)
		return
	}
	*ratio += 25
}

// SkillRatio is SkillGravitationField::calculateSkillRatio: a magic
// ratio in renewal only.
func (gravitation) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += -100 + 100*a.Level()
		reLvlDmod(a, ratio, 100)
	}
}
