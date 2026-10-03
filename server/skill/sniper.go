package skill

import (
	"github.com/lxsmnsyc/wathena/server/battle"
)

// The Sniper's skills: the SkillImpl classes of rAthena's
// src/map/skills/archer. True Sight and Wind Walker are statuses; their
// behavior is in the zone.

// Skill IDs of the Sniper, from rAthena's skill_db.yml.
const (
	IDTrueSight     = 380 // SN_SIGHT
	IDFalconAssault = 381 // SN_FALCONASSAULT
	IDSharpShooting = 382 // SN_SHARPSHOOTING
	IDWindWalk      = 383 // SN_WINDWALK
)

// blitzBeatHits is skill_get_num(HT_BLITZBEAT, 5): the hits of Blitz Beat
// at level 5, which Falcon Assault multiplies its base by.
const blitzBeatHits = 5

type (
	// sniperStatus is a Sniper skill that starts a status: True Sight
	// and Wind Walker.
	sniperStatus  struct{ Base }
	falconAssault struct{ Base }
	sharpShooting struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDTrueSight:     sniperStatus{},
		IDFalconAssault: falconAssault{},
		IDSharpShooting: sharpShooting{},
		IDWindWalk:      sniperStatus{},
	})
}

// BaseDamage is SN_FALCONASSAULT's damage in battle_calc_misc_attack:
// Blitz Beat's base at Falcon Assault's level, times Blitz Beat's hits at
// level 5 (DAMAGE_DIV_FIX2), times 150% + 70% per level.
func (falconAssault) BaseDamage(a *battle.SkillContext, _ battle.Rand) int64 {
	dmg := blitzDamage(a, a.Level())
	if blitzBeatHits > 1 {
		dmg *= blitzBeatHits
	}
	return dmg * int64(150+70*a.Level()) / 100
}

// SkillRatio is SkillFocusedArrowStrike::calculateSkillRatio. A monster
// deals more to the targets of the line (wflag 2). In renewal a player's
// ratio grows with base levels above 99 (RE_LVL_DMOD(100)).
func (sharpShooting) SkillRatio(a *battle.SkillContext, ratio *int) {
	lv := a.Level()
	if a.Src.Player == nil {
		if a.Flag&2 != 0 {
			*ratio += -100 + 140*lv
		} else {
			*ratio += 100 + 50*lv
		}
		return
	}
	if a.Renewal {
		*ratio += -100 + 300 + 300*lv
		reLvlDmod(a, ratio, 100)
		return
	}
	*ratio += 100 + 50*lv
}
