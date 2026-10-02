package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The skills of the Bard, the Dancer, the Clown and the Gypsy, as the
// SkillImpl classes in rAthena's src/map/skills/archer. Their behavior
// (songs, dances, ensembles, Frost Joker, Tarot Card) is in the zone's
// skills_bard.go. The types here hold the damage formulas.

// Skill IDs of the Bard, Dancer, Clown and Gypsy trees, from
// skill_db.yml.
const (
	IDAdaptation      = 304  // BD_ADAPTATION
	IDEncore          = 305  // BD_ENCORE
	IDLullaby         = 306  // BD_LULLABY
	IDRichmanKim      = 307  // BD_RICHMANKIM
	IDEternalChaos    = 308  // BD_ETERNALCHAOS
	IDDrumBattlefield = 309  // BD_DRUMBATTLEFIELD
	IDRingNibelungen  = 310  // BD_RINGNIBELUNGEN
	IDRokisWeil       = 311  // BD_ROKISWEIL
	IDIntoAbyss       = 312  // BD_INTOABYSS
	IDSiegfried       = 313  // BD_SIEGFRIED
	IDMusicalLesson   = 315  // BA_MUSICALLESSON
	IDMusicalStrike   = 316  // BA_MUSICALSTRIKE
	IDDissonance      = 317  // BA_DISSONANCE
	IDFrostJoker      = 318  // BA_FROSTJOKER
	IDWhistle         = 319  // BA_WHISTLE
	IDAssassinCross   = 320  // BA_ASSASSINCROSS
	IDPoemBragi       = 321  // BA_POEMBRAGI
	IDAppleIdun       = 322  // BA_APPLEIDUN
	IDDancingLesson   = 323  // DC_DANCINGLESSON
	IDThrowArrow      = 324  // DC_THROWARROW
	IDUglyDance       = 325  // DC_UGLYDANCE
	IDScream          = 326  // DC_SCREAM
	IDHumming         = 327  // DC_HUMMING
	IDDontForgetMe    = 328  // DC_DONTFORGETME
	IDFortuneKiss     = 329  // DC_FORTUNEKISS
	IDServiceForYou   = 330  // DC_SERVICEFORYOU
	IDArrowVulcan     = 394  // CG_ARROWVULCAN
	IDMoonlit         = 395  // CG_MOONLIT
	IDMarionette      = 396  // CG_MARIONETTE
	IDLongingFreedom  = 487  // CG_LONGINGFREEDOM
	IDHermode         = 488  // CG_HERMODE
	IDTarotCard       = 489  // CG_TAROTCARD
	IDPangVoice       = 1010 // BA_PANGVOICE
	IDWinkCharm       = 1011 // DC_WINKCHARM
	IDSpecialSinger   = 5068 // CG_SPECIALSINGER
)

// ringSPConsume is RINGNBL_SPCONSUM: SC_NIBELUNGEN's val2 for 30% less
// SP.
const ringSPConsume = 9

type (
	melodyStrike      struct{ WeaponSkill }
	unchainedSerenade struct{ WeaponSkill }
	slingingArrow     struct{ WeaponSkill }
	vulcanArrow       struct{ WeaponSkill }
	// bardSkill is the skills of these jobs whose only formulas are in
	// the zone: songs, dances, ensembles, the lessons and the rest.
	bardSkill struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDAdaptation:      bardSkill{},
		IDEncore:          bardSkill{},
		IDLullaby:         bardSkill{},
		IDRichmanKim:      bardSkill{},
		IDEternalChaos:    bardSkill{},
		IDDrumBattlefield: bardSkill{},
		IDRingNibelungen:  bardSkill{},
		IDRokisWeil:       bardSkill{},
		IDIntoAbyss:       bardSkill{},
		IDSiegfried:       bardSkill{},
		IDMusicalLesson:   bardSkill{},
		IDMusicalStrike:   melodyStrike{},
		IDDissonance:      unchainedSerenade{},
		IDFrostJoker:      bardSkill{},
		IDWhistle:         bardSkill{},
		IDAssassinCross:   bardSkill{},
		IDPoemBragi:       bardSkill{},
		IDAppleIdun:       bardSkill{},
		IDDancingLesson:   bardSkill{},
		IDThrowArrow:      slingingArrow{},
		IDUglyDance:       bardSkill{},
		IDScream:          bardSkill{},
		IDHumming:         bardSkill{},
		IDDontForgetMe:    bardSkill{},
		IDFortuneKiss:     bardSkill{},
		IDServiceForYou:   bardSkill{},
		IDArrowVulcan:     vulcanArrow{},
		IDMoonlit:         bardSkill{},
		IDMarionette:      bardSkill{},
		IDLongingFreedom:  bardSkill{},
		IDHermode:         bardSkill{},
		IDTarotCard:       bardSkill{},
		IDPangVoice:       bardSkill{},
		IDWinkCharm:       bardSkill{},
		IDSpecialSinger:   bardSkill{},
	})
}

// SkillRatio is SkillMelodyStrike::calculateSkillRatio.
func (melodyStrike) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 10 + 40*a.Level()
	} else {
		*ratio += -40 + 40*a.Level()
	}
}

// SkillRatio is SkillSlingingArrow::calculateSkillRatio, the same as
// Musical Strike's.
func (slingingArrow) SkillRatio(a *battle.SkillContext, ratio *int) {
	melodyStrike{}.SkillRatio(a, ratio)
}

// SkillRatio is SkillUnchainedSerenade::calculateSkillRatio. Only
// renewal's magic hit has a ratio: 10% + 50% per level more, times the
// job level over 10 for a player.
func (unchainedSerenade) SkillRatio(a *battle.SkillContext, ratio *int) {
	if !a.Renewal {
		return
	}
	*ratio += 10 + a.Level()*50
	if p := a.Src.Player; p != nil {
		*ratio = *ratio * p.JobLevel / 10
	}
}

// SkillRatio is SkillVulcanArrow::calculateSkillRatio.
func (vulcanArrow) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += 400 + 100*a.Level()
		reLvlDmod(a, ratio, 100)
		return
	}
	*ratio += 100 + 100*a.Level()
}
