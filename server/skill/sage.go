package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The skills of the Sage and the Professor. Only Soul Siphon has a damage
// formula of its own; the others register a type so that the zone's
// behavior counts as ported.

// Skill IDs of the Sage and the Professor.
const (
	IDAdvancedBook    = 274  // SA_ADVANCEDBOOK
	IDCastCancel      = 275  // SA_CASTCANCEL
	IDMagicRod        = 276  // SA_MAGICROD
	IDSpellBreaker    = 277  // SA_SPELLBREAKER
	IDFreeCast        = 278  // SA_FREECAST
	IDAutoSpell       = 279  // SA_AUTOSPELL
	IDFlameLauncher   = 280  // SA_FLAMELAUNCHER
	IDFrostWeapon     = 281  // SA_FROSTWEAPON
	IDLightningLoader = 282  // SA_LIGHTNINGLOADER
	IDSeismicWeapon   = 283  // SA_SEISMICWEAPON
	IDDragonology     = 284  // SA_DRAGONOLOGY
	IDVolcano         = 285  // SA_VOLCANO
	IDDeluge          = 286  // SA_DELUGE
	IDViolentGale     = 287  // SA_VIOLENTGALE
	IDLandProtector   = 288  // SA_LANDPROTECTOR
	IDDispell         = 289  // SA_DISPELL
	IDAbracadabra     = 290  // SA_ABRACADABRA
	IDHPConversion    = 373  // PF_HPCONVERSION
	IDSoulChange      = 374  // PF_SOULCHANGE
	IDSoulBurn        = 375  // PF_SOULBURN
	IDMindBreaker     = 402  // PF_MINDBREAKER
	IDMemorize        = 403  // PF_MEMORIZE
	IDFogWall         = 404  // PF_FOGWALL
	IDSpiderWeb       = 405  // PF_SPIDERWEB
	IDDoubleCasting   = 482  // PF_DOUBLECASTING
	IDCreateCon       = 1007 // SA_CREATECON
	IDElementWater    = 1008 // SA_ELEMENTWATER
	IDElementGround   = 1017 // SA_ELEMENTGROUND
	IDElementFire     = 1018 // SA_ELEMENTFIRE
	IDElementWind     = 1019 // SA_ELEMENTWIND
)

// soulSiphon is SkillSoulSiphon.
type soulSiphon struct{ Base }

// BaseDamage is Soul Siphon's case of battle_calc_magic_attack: twice the
// target's SP.
func (soulSiphon) BaseDamage(a *battle.SkillContext, _ battle.Rand) int64 {
	return int64(a.Target.SP) * 2
}

func init() {
	register(map[int]battle.SkillImpl{
		// Passives, which status_calc_pc and battle read.
		IDAdvancedBook: Base{},
		IDFreeCast:     Base{},
		IDDragonology:  Base{},

		IDCastCancel:      Base{},
		IDMagicRod:        Base{},
		IDSpellBreaker:    Base{},
		IDAutoSpell:       Base{},
		IDFlameLauncher:   Base{},
		IDFrostWeapon:     Base{},
		IDLightningLoader: Base{},
		IDSeismicWeapon:   Base{},
		IDVolcano:         Base{},
		IDDeluge:          Base{},
		IDViolentGale:     Base{},
		IDLandProtector:   Base{},
		IDDispell:         Base{},
		IDElementWater:    Base{},
		IDElementGround:   Base{},
		IDElementFire:     Base{},
		IDElementWind:     Base{},
		IDHPConversion:    Base{},
		IDSoulChange:      Base{},
		IDSoulBurn:        soulSiphon{},
		IDMindBreaker:     Base{},
		IDMemorize:        Base{},
		IDFogWall:         Base{},
		IDSpiderWeb:       Base{},
		IDDoubleCasting:   Base{},
	})
}
