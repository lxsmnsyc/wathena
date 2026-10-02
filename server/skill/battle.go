package skill

import (
	"github.com/lxsmnsyc/wathena/server/battle"
)

// Skill IDs that this package reads, from rAthena's skill_db.yml.
const (
	IDBasic          = 1    // NV_BASIC
	IDSwordMastery   = 2    // SM_SWORD
	IDTwoHandMastery = 3    // SM_TWOHAND
	IDRecovery       = 4    // SM_RECOVERY
	IDBash           = 5    // SM_BASH
	IDProvoke        = 6    // SM_PROVOKE
	IDMagnumBreak    = 7    // SM_MAGNUM
	IDEndure         = 8    // SM_ENDURE
	IDSPRecovery     = 9    // MG_SRECOVERY
	IDSight          = 10   // MG_SIGHT
	IDNapalmBeat     = 11   // MG_NAPALMBEAT
	IDSafetyWall     = 12   // MG_SAFETYWALL
	IDSoulStrike     = 13   // MG_SOULSTRIKE
	IDColdBolt       = 14   // MG_COLDBOLT
	IDFrostDiver     = 15   // MG_FROSTDIVER
	IDStoneCurse     = 16   // MG_STONECURSE
	IDFireBall       = 17   // MG_FIREBALL
	IDFireWall       = 18   // MG_FIREWALL
	IDFireBolt       = 19   // MG_FIREBOLT
	IDLightningBolt  = 20   // MG_LIGHTNINGBOLT
	IDThunderStorm   = 21   // MG_THUNDERSTORM
	IDDivineProt     = 22   // AL_DP
	IDDemonBane      = 23   // AL_DEMONBANE
	IDRuwach         = 24   // AL_RUWACH
	IDPneuma         = 25   // AL_PNEUMA
	IDTeleport       = 26   // AL_TELEPORT
	IDWarp           = 27   // AL_WARP
	IDResurrection   = 54   // ALL_RESURRECTION
	IDHeal           = 28   // AL_HEAL
	IDIncAgi         = 29   // AL_INCAGI
	IDDecAgi         = 30   // AL_DECAGI
	IDHolyWater      = 31   // AL_HOLYWATER
	IDCrucis         = 32   // AL_CRUCIS
	IDAngelus        = 33   // AL_ANGELUS
	IDBlessing       = 34   // AL_BLESSING
	IDCure           = 35   // AL_CURE
	IDIncCarry       = 36   // MC_INCCARRY
	IDDiscount       = 37   // MC_DISCOUNT
	IDOvercharge     = 38   // MC_OVERCHARGE
	IDPushcart       = 39   // MC_PUSHCART
	IDIdentify       = 40   // MC_IDENTIFY
	IDVending        = 41   // MC_VENDING
	IDMammonite      = 42   // MC_MAMMONITE
	IDOwlEye         = 43   // AC_OWL
	IDVultureEye     = 44   // AC_VULTURE
	IDConcentration  = 45   // AC_CONCENTRATION
	IDDoubleStrafe   = 46   // AC_DOUBLE
	IDArrowShower    = 47   // AC_SHOWER
	IDDoubleAttack   = 48   // TF_DOUBLE
	IDImproveDodge   = 49   // TF_MISS
	IDSteal          = 50   // TF_STEAL
	IDHiding         = 51   // TF_HIDING
	IDEnvenom        = 52   // TF_POISON
	IDDetoxify       = 53   // TF_DETOXIFY
	IDFirstAid       = 142  // NV_FIRSTAID
	IDTrickDead      = 143  // NV_TRICKDEAD
	IDMovingRecovery = 144  // SM_MOVINGRECOVERY
	IDFatalBlow      = 145  // SM_FATALBLOW
	IDAutoBerserk    = 146  // SM_AUTOBERSERK
	IDMakingArrow    = 147  // AC_MAKINGARROW
	IDChargeArrow    = 148  // AC_CHARGEARROW
	IDSandAttack     = 149  // TF_SPRINKLESAND
	IDBackSliding    = 150  // TF_BACKSLIDING
	IDPickStone      = 151  // TF_PICKSTONE
	IDThrowStone     = 152  // TF_THROWSTONE
	IDCartRevolution = 153  // MC_CARTREVOLUTION
	IDChangeCart     = 154  // MC_CHANGECART
	IDLoud           = 155  // MC_LOUD
	IDHolyLight      = 156  // AL_HOLYLIGHT
	IDEnergyCoat     = 157  // MG_ENERGYCOAT
	IDMeditatio      = 363  // HP_MEDITATIO
	IDSnakeEye       = 510  // GS_SNAKEEYE
	IDBreakthrough   = 5075 // NV_BREAKTHROUGH
	IDTranscendence  = 5077 // NV_TRANSCENDENCE
)

// RangeFor is skill_get_range2 with isServer set: the skill's range at a
// level for a caster. A negative range in skill_db is a weapon skill's
// range (skillrange_from_weapon is off, so it only flips its sign).
// Vulture's Eye and Snake Eye lengthen the skills that read them. A unit
// that is not a player uses 9 for a range of 0.
func (d *Def) RangeFor(caster *battle.Status, lv int) int {
	r := d.Range.At(lv)
	if r < 0 {
		r = -r
	}
	r = min(r, 14)
	player := caster != nil && caster.Player != nil
	if d.Has("AlterRangeVulture") || d.Has("AlterRangeSnakeEye") {
		if player {
			if d.Has("AlterRangeVulture") {
				r += caster.Player.Skills[IDVultureEye]
			}
			if d.Has("AlterRangeSnakeEye") {
				r += caster.Player.Skills[IDSnakeEye]
			}
		}
		// monster_eye_range_bonus is 0.
	}
	if r == 0 && !player {
		return 9
	}
	return r
}

// Battle is the battle.Skill of a skill at a level, for a caster. flag
// is the attack's wflag or mflag, such as the number of targets of a
// skill that splits its damage. distance is the distance from the caster
// to the target.
func (d *Def) Battle(caster *battle.Status, lv, flag, distance int) *battle.Skill {
	return &battle.Skill{
		ID: d.ID, Level: lv,
		Hits:               d.HitCount.At(lv),
		Element:            d.Element.At(lv),
		NK:                 d.NK,
		Ground:             d.Target == TargetGround,
		Range:              d.RangeFor(caster, lv),
		Distance:           distance,
		Knockback:          d.Knockback.At(lv),
		Ammo:               d.Requires.Ammo != 0,
		Trap:               d.Has("IsTrap"),
		IgnoreNonCritBonus: d.Has("IgnoreNonCritAtkBonus"),
		Flag:               flag,
		Impl:               implOf(d),
	}
}
