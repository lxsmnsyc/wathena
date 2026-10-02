package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The Rogue's skills (job 17), from rAthena's src/map/skills/thief.
// The damage formulas are here; the rest of each skill is in the zone's
// skills_rogue.go.

// Skill IDs of the Rogue, from rAthena's skill_db.yml.
const (
	IDSnatcher     = 210  // RG_SNATCHER
	IDStealCoin    = 211  // RG_STEALCOIN
	IDBackStab     = 212  // RG_BACKSTAP
	IDTunnelDrive  = 213  // RG_TUNNELDRIVE
	IDRaid         = 214  // RG_RAID
	IDStripWeapon  = 215  // RG_STRIPWEAPON
	IDStripShield  = 216  // RG_STRIPSHIELD
	IDStripArmor   = 217  // RG_STRIPARMOR
	IDStripHelm    = 218  // RG_STRIPHELM
	IDIntimidate   = 219  // RG_INTIMIDATE
	IDGraffiti     = 220  // RG_GRAFFITI
	IDFlagGraffiti = 221  // RG_FLAGGRAFFITI
	IDCleaner      = 222  // RG_CLEANER
	IDGangster     = 223  // RG_GANGSTER
	IDCompulsion   = 224  // RG_COMPULSION
	IDPlagiarism   = 225  // RG_PLAGIARISM
	IDCloseConfine = 1005 // RG_CLOSECONFINE
)

type (
	// snatcher is RG_SNATCHER: a passive that steals on normal attacks.
	snatcher struct{ Base }
	// stealCoin is SkillMug.
	stealCoin struct{ Base }
	// backStab is SkillBackStab.
	backStab struct{ Base }
	// tunnelDrive is RG_TUNNELDRIVE: a passive that status_calc_speed
	// reads.
	tunnelDrive struct{ Base }
	// raid is SkillSightlessMind.
	raid struct{ Base }
	// strip is SkillDivestWeapon, SkillDivestShield, SkillDivestArmor and
	// SkillDivestHelm.
	strip struct{ Base }
	// intimidate is SkillSnatch, a WeaponSkillImpl.
	intimidate struct{ WeaponSkill }
	// graffiti is SkillScribble.
	graffiti struct{ Base }
	// flagGraffiti is RG_FLAGGRAFFITI, which has no SkillImpl class.
	flagGraffiti struct{ Base }
	// cleaner is SkillRemover.
	cleaner struct{ Base }
	// gangster is RG_GANGSTER: a passive that monsters read.
	gangster struct{ Base }
	// compulsion is RG_COMPULSION: a passive that shop prices read.
	compulsion struct{ Base }
	// plagiarism is RG_PLAGIARISM: a passive that copies skills.
	plagiarism struct{ Base }
	// closeConfine is SkillCloseConfine.
	closeConfine struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDSnatcher:     snatcher{},
		IDStealCoin:    stealCoin{},
		IDBackStab:     backStab{},
		IDTunnelDrive:  tunnelDrive{},
		IDRaid:         raid{},
		IDStripWeapon:  strip{},
		IDStripShield:  strip{},
		IDStripArmor:   strip{},
		IDStripHelm:    strip{},
		IDIntimidate:   intimidate{},
		IDGraffiti:     graffiti{},
		IDFlagGraffiti: flagGraffiti{},
		IDCleaner:      cleaner{},
		IDGangster:     gangster{},
		IDCompulsion:   compulsion{},
		IDPlagiarism:   plagiarism{},
		IDCloseConfine: closeConfine{},
	})
}

// backstabBowPenalty is backstab_bow_penalty of conf/battle/skill.conf.
const backstabBowPenalty = true

// ModifyDamageData is SkillBackStab::modifyDamageData: in renewal, a
// player with a dagger hits twice.
func (backStab) ModifyDamageData(a *battle.SkillContext, d *battle.DamageData) {
	if a.Renewal && a.Src.Player != nil && a.Src.Player.Weapon == battle.WeaponDagger {
		d.Div = 2
	}
}

// SkillRatio is SkillBackStab::calculateSkillRatio: 300% + 40% per
// level, half of the bonus with a bow.
func (backStab) SkillRatio(a *battle.SkillContext, ratio *int) {
	if sd := a.Src.Player; sd != nil && sd.Weapon == battle.WeaponBow && backstabBowPenalty {
		*ratio += (200 + 40*a.Level()) / 2
	} else {
		*ratio += 200 + 40*a.Level()
	}
}

// HitRate is SkillBackStab::modifyHitRate: renewal adds the level.
func (backStab) HitRate(a *battle.SkillContext, hit *int) {
	if a.Renewal {
		*hit += a.Level()
	}
}

// SkillRatio is SkillSightlessMind::calculateSkillRatio.
func (raid) SkillRatio(a *battle.SkillContext, ratio *int) {
	if a.Renewal {
		*ratio += -100 + 50 + a.Level()*150
	} else {
		*ratio += 40 * a.Level()
	}
}

// SkillRatio is SkillSnatch::calculateSkillRatio.
func (intimidate) SkillRatio(a *battle.SkillContext, ratio *int) { *ratio += 30 * a.Level() }
