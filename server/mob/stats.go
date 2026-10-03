package mob

import (
	"math"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// Calc is a monster's battle status from its base stats: the end of
// MobDatabase::loadingFinished, which fills mob_db's status with
// status_calc_misc, and status_calc_mob_ for a monster without special
// status, which copies it. renewal picks the formulas. Monsters that level
// up, change size, guard a castle or follow a master are not covered:
// the server has none yet.
func Calc(d *Def, renewal bool) battle.Status {
	s := battle.Status{
		Level: d.Level,
		MaxHP: max(d.HP, 1),
		MaxSP: max(d.SP, 1),
		Str:   d.Str, Agi: d.Agi, Vit: d.Vit, Int: d.Int, Dex: d.Dex, Luk: d.Luk,
		Speed:   d.WalkSpeed,
		AMotion: d.AttackMotion,
		ADelay:  max(d.AttackDelay, d.AttackMotion),
		DMotion: d.DamageMotion,
		Mode:    d.Mode,
		Def:     d.Defense,
		MDef:    d.MagicDefense,
		Res:     d.Resistance,
		MRes:    d.MagicResistance,
		DefEle:  d.Element,
		EleLv:   d.ElementLevel,
		Size:    d.Size,
		Race:    d.Race,
		Class:   d.Class,
		ID:      int(d.ID),
		RHW:     battle.Weapon{Atk: d.Attack, Range: d.AttackRange},
	}
	s.DamageTaken = d.DamageTaken
	calcMisc(&s, d.Level, d.Attack2, renewal)
	if d.ID == EmperiumID {
		// status_calc_mob_ gives the Emperium a special status (flag 4),
		// which runs status_calc_misc again over mob_db's status. In
		// renewal this scales its ATK by 80% and 120% a second time.
		// Its castle bonuses need a castle, which no map has yet.
		calcMisc(&s, d.Level, d.Attack2, renewal)
	}
	return s
}

// EmperiumID is MOBID_EMPERIUM.
const EmperiumID = 1288

// calcMisc is status_calc_misc for a monster: HIT, FLEE, soft DEF and
// MDEF, ATK and MATK from the stats. attack2 is mob_db's Attack2.
// Monsters have no critical rate and no perfect dodge, as enable_critical
// and enable_perfect_flee leave them out.
func calcMisc(s *battle.Status, level, attack2 int, renewal bool) {
	if renewal {
		s.RHW.Matk = attack2
		s.Hit = capInt(level+s.Dex+150, 1, math.MaxInt16)
		s.Flee = capInt(level+s.Agi+100, 1, math.MaxInt16)
		s.Def2 = capInt(int(float32(level+s.Vit)/2), 0, math.MaxInt16)
		s.MDef2 = capInt(int(float32(s.Int+level)/4), 0, math.MaxInt16)
		s.Res = capInt(s.Res, 0, math.MaxInt16)
		s.MRes = capInt(s.MRes, 0, math.MaxInt16)
		// status_base_atk_min and status_base_atk_max: 80% and 120% of
		// Attack.
		atk := s.RHW.Atk
		s.RHW.Atk2 = atk * 120 / 100
		s.RHW.Atk = atk * 80 / 100
		// status_base_matk_min and status_base_matk_max.
		s.MAtkMin = s.Int + level + attack2*70/100
		s.MAtkMax = s.Int + level + attack2*130/100
		// status_base_atk: STR plus the level, as enable_baseatk_renewal
		// includes monsters.
		s.BAtk = capInt(s.Str+level, 0, math.MaxUint16)
		return
	}
	s.RHW.Atk2 = attack2
	// status_base_matk_min and status_base_matk_max of pre-renewal.
	s.MAtkMin = s.Int + (s.Int/7)*(s.Int/7)
	s.MAtkMax = s.Int + (s.Int/5)*(s.Int/5)
	s.Hit = capInt(level+s.Dex, 1, math.MaxInt16)
	s.Flee = capInt(level+s.Agi, 1, math.MaxInt16)
	s.Def2 = capInt(s.Vit, 0, math.MaxInt16)
	s.MDef2 = capInt(s.Int+s.Vit/2, 0, math.MaxInt16)
	// enable_baseatk leaves monsters out: they have no base ATK.
	s.BAtk = 0
}

func capInt(v, lo, hi int) int { return min(max(v, lo), hi) }
