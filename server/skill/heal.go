package skill

import "github.com/lxsmnsyc/wathena/server/battle"

// Heal settings from rAthena's conf/battle/skill.conf: a skill level of
// max_heal_lv or more heals max_heal.
const (
	maxHealLv = 11
	maxHeal   = 9999
)

// Skill IDs that only the heal reads.
const (
	IDSanctuary  = 70   // PR_SANCTUARY
	IDPowerOfSea = 5148 // SU_POWEROFSEA
	idAppleIdun  = 322  // BA_APPLEIDUN
	idEvilLand   = 670  // NPC_EVILLAND
)

// CalcHeal is skill_calc_heal: what a skill heals, or with heal false
// its damage to the undead, which is half. src and target are battle
// statuses; a player has its Player set. The statuses that change heals
// (Offertorium, Critical Wound and others) are not ported.
func CalcHeal(src, target *battle.Status, id, lv int, heal, renewal bool, r battle.Rand) int64 {
	sd := src.Player
	skill := func(id int) int {
		if sd == nil {
			return 0
		}
		return sd.Skills[id]
	}
	var hp int64
	hpBonus := int64(0)
	switch {
	case id == idAppleIdun && !renewal:
		// Pre-renewal's Apple of Idun: 30 + 5 per level + half the VIT,
		// and 5 per level of Musical Lesson. Renewal's takes the default.
		hp = int64(30 + 5*lv + src.Vit/2)
		if sd != nil {
			hp += int64(5 * skill(IDMusicalLesson))
		}
	case id == IDSanctuary:
		if lv > 6 {
			hp = 777
		} else {
			hp = int64(lv) * 100
		}
	default:
		if lv >= maxHealLv {
			return maxHeal
		}
		if renewal {
			hp = int64((src.Level+src.Int)/5*30) * int64(lv) / 10
		} else {
			hp = int64((src.Level + src.Int) / 8 * (4 + lv*8))
		}
		if m := skill(IDMeditatio); sd != nil && m > 0 {
			if renewal {
				hpBonus += int64(m) * 2
			} else {
				hp += hp * int64(m) * 2 / 100
			}
		}
		// A female Super Novice heals her partner twice as much. Partners
		// are not ported.
	}

	if !heal {
		hp /= 2
	}

	add := func(pct int64) {
		if renewal {
			hpBonus += pct
		} else {
			hp += hp * pct / 100
		}
	}
	if sd != nil {
		if skill(IDPowerOfSea) > 0 {
			// The summoner bonus of 20 more needs Spirit of Sea skills,
			// which are not ported.
			add(10)
		}
		if l := skill(IDBreakthrough); l > 0 {
			add(int64(2 * l))
		}
		if l := skill(IDTranscendence); l > 0 {
			add(int64(3 * l))
		}
		// pc_skillheal_bonus: bHealPower for the skills of
		// skill_add_heal_rate, then bSkillHeal.
		b := 0
		if healPowerSkills[id] {
			b = sd.AddHealRate
		}
		if b += sd.SkillHeal[id]; b != 0 {
			add(int64(b))
		}
	}
	if tsd := target.Player; tsd != nil {
		// pc_skillheal2_bonus.
		if b := tsd.AddHeal2Rate + tsd.SkillHeal2[id]; b != 0 {
			add(int64(b))
		}
	}
	// The target's statuses. Renewal's Assumptio adds 2% per level.
	if tc := target.Changes; tc != nil && renewal && tc.Assumptio > 0 && id != idAppleIdun && id != idEvilLand {
		hpBonus += int64(tc.Assumptio) * 2
	}

	if !renewal {
		return hp
	}
	if hpBonus != 0 {
		hp += hp * hpBonus / 100
	}
	// The MATK part of the renewal heal. Item and skill MATK bonuses do
	// not apply.
	if id != IDSanctuary && id != idAppleIdun {
		lo, hi := baseMatkMin(src), baseMatkMax(src)
		if w := src.RHW.Matk; w > 0 {
			variance := w * src.RHW.Wlv / 10
			lo += w - variance
			hi += w + variance
		}
		if sd != nil && sd.Right.Overrefine > 0 {
			lo++
			hi += sd.Right.Overrefine - 1
		}
		if hi > lo {
			hp += int64(lo + r.IntN(hi-lo))
		} else {
			hp += int64(lo)
		}
	}
	hp = int64(int32(hp)) // #nosec G115 -- rAthena keeps the heal in an int32.
	if sd != nil && src.HPlus > 0 {
		hp += hp * int64(src.HPlus) / 100
	}
	if heal {
		return max(1, hp)
	}
	return hp
}

// baseMatkMin and baseMatkMax are the renewal status_base_matk_min and
// status_base_matk_max. Players have no MATK range; other units have 70%
// and 130% of their weapon MATK.
func baseMatkMin(st *battle.Status) int {
	if st.Player == nil {
		return st.Int + st.Level + st.RHW.Matk*70/100
	}
	return st.Int + st.Int/2 + st.Dex/5 + st.Luk/3 + st.Level/4 + 5*st.Spl
}

func baseMatkMax(st *battle.Status) int {
	if st.Player == nil {
		return st.Int + st.Level + st.RHW.Matk*130/100
	}
	return st.Int + st.Int/2 + st.Dex/5 + st.Luk/3 + st.Level/4 + 5*st.Spl
}

// healPowerSkills are the skills that bHealPower raises, from
// skill_add_heal_rate 487: Heal, Sanctuary, Potion Pitcher, Coluceo Heal,
// Highness Heal, Mediale Votum and Dilectio Heal.
var healPowerSkills = map[int]bool{IDHeal: true, IDSanctuary: true, 231: true, 2043: true, 2051: true, 5269: true, 5280: true}
