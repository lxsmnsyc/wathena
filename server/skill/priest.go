package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// The skills of the Priest and the High Priest, as the SkillImpl classes
// in rAthena's src/map/skills/acolyte. Their behavior is in the zone's
// skills_priest.go. The types here hold the damage formulas.

// Skill IDs of the Priest and High Priest trees, from skill_db.yml.
const (
	IDMaceMastery    = 65   // PR_MACEMASTERY
	IDImpositio      = 66   // PR_IMPOSITIO
	IDSuffragium     = 67   // PR_SUFFRAGIUM
	IDAspersio       = 68   // PR_ASPERSIO
	IDBenedictio     = 69   // PR_BENEDICTIO
	IDSlowPoison     = 71   // PR_SLOWPOISON
	IDStatusRecovery = 72   // PR_STRECOVERY
	IDKyrie          = 73   // PR_KYRIE
	IDMagnificat     = 74   // PR_MAGNIFICAT
	IDGloria         = 75   // PR_GLORIA
	IDLexDivina      = 76   // PR_LEXDIVINA
	IDTurnUndead     = 77   // PR_TURNUNDEAD
	IDLexAeterna     = 78   // PR_LEXAETERNA
	IDMagnus         = 79   // PR_MAGNUS
	IDAssumptio      = 361  // HP_ASSUMPTIO
	IDBasilica       = 362  // HP_BASILICA
	IDManaRecharge   = 481  // HP_MANARECHARGE
	IDRedemptio      = 1014 // PR_REDEMPTIO
)

type (
	resurrection   struct{ Base }
	maceMastery    struct{ Base }
	impositio      struct{ Base }
	suffragium     struct{ Base }
	aspersio       struct{ Base }
	benedictio     struct{ Base }
	sanctuary      struct{ Base }
	slowPoison     struct{ Base }
	statusRecovery struct{ Base }
	kyrie          struct{ Base }
	magnificat     struct{ Base }
	gloria         struct{ Base }
	lexDivina      struct{ Base }
	turnUndead     struct{ Base }
	lexAeterna     struct{ Base }
	magnus         struct{ Base }
	redemptio      struct{ Base }
	assumptio      struct{ Base }
	basilica       struct{ Base }
	meditatio      struct{ Base }
	manaRecharge   struct{ Base }
)

func init() {
	register(map[int]battle.SkillImpl{
		IDResurrection:   resurrection{},
		IDMaceMastery:    maceMastery{},
		IDImpositio:      impositio{},
		IDSuffragium:     suffragium{},
		IDAspersio:       aspersio{},
		IDBenedictio:     benedictio{},
		IDSanctuary:      sanctuary{},
		IDSlowPoison:     slowPoison{},
		IDStatusRecovery: statusRecovery{},
		IDKyrie:          kyrie{},
		IDMagnificat:     magnificat{},
		IDGloria:         gloria{},
		IDLexDivina:      lexDivina{},
		IDTurnUndead:     turnUndead{},
		IDLexAeterna:     lexAeterna{},
		IDMagnus:         magnus{},
		IDRedemptio:      redemptio{},
		IDAssumptio:      assumptio{},
		IDBasilica:       basilica{},
		IDMeditatio:      meditatio{},
		IDManaRecharge:   manaRecharge{},
	})
}

// SkillRatio is SkillMagnusExorcismus::calculateSkillRatio: 30% more
// against the undead and demons.
func (magnus) SkillRatio(a *battle.SkillContext, ratio *int) {
	if t := a.Target; battle.IsUndead(t.Race, t.DefEle) || t.Race == battle.RaceDemon {
		*ratio += 30
	}
}

// BaseDamage is Aspersio's damage to the undead in
// battle_calc_magic_attack: 40.
func (aspersio) BaseDamage(*battle.SkillContext, battle.Rand) int64 { return 40 }

// BaseDamage is B.S. Sacramenti's damage in battle_calc_magic_attack:
// skill_calc_heal with heal false.
func (benedictio) BaseDamage(a *battle.SkillContext, r battle.Rand) int64 {
	return CalcHeal(a.Src, a.Target, IDBenedictio, a.Level(), false, a.Renewal, r)
}

// BaseDamage is Sanctuary's damage in battle_calc_magic_attack:
// skill_calc_heal with heal false.
func (sanctuary) BaseDamage(a *battle.SkillContext, r battle.Rand) int64 {
	return CalcHeal(a.Src, a.Target, IDSanctuary, a.Level(), false, a.Renewal, r)
}

// BaseDamage is Turn Undead's damage in battle_calc_magic_attack.
func (turnUndead) BaseDamage(a *battle.SkillContext, r battle.Rand) int64 {
	return turnUndeadDamage(a, r)
}

// BaseDamage is Resurrection's damage to the undead, which is Turn
// Undead's.
func (resurrection) BaseDamage(a *battle.SkillContext, r battle.Rand) int64 {
	return turnUndeadDamage(a, r)
}

// turnUndeadDamage is the ALL_RESURRECTION and PR_TURNUNDEAD case of
// battle_calc_magic_attack. A roll per 1,000 kills the target: its
// current HP is the damage. Bosses are never killed. Otherwise:
//   - Renewal: MATK times the level, in percent.
//   - Pre-renewal: base level plus INT plus 10 per level.
func turnUndeadDamage(a *battle.SkillContext, r battle.Rand) int64 {
	src, t := a.Src, a.Target
	lv := a.Level()
	maxHP := int64(max(t.MaxHP, 1))
	hp := t.CurrentHP()
	var i int
	if a.Renewal {
		i = 10*lv + src.Luk + src.Int + src.Level + 300 - int(300*hp/maxHP)
	} else {
		i = 20*lv + src.Luk + src.Int + src.Level + 200 - int(200*hp/maxHP)
	}
	i = min(i, 700)
	if r.IntN(1000) < i && t.Mode&battle.ModeStatusImmune == 0 {
		return hp
	}
	if !a.Renewal {
		return int64(src.Level + src.Int + lv*10)
	}
	var dmg int64
	if src.MAtkMax > src.MAtkMin {
		dmg = int64(src.MAtkMin + r.IntN(src.MAtkMax-src.MAtkMin))
	} else {
		dmg = int64(src.MAtkMin)
	}
	return dmg * int64(lv) / 100
}
