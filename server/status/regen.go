package status

import "github.com/lxsmnsyc/wathena/server/battle"

// Skill IDs status_calc_regen reads, from rAthena's skill_db.yml.
const (
	skillIncHPRecovery = 4   // SM_RECOVERY
	skillIncSPRecovery = 9   // MG_SRECOVERY
	skillNinpou        = 533 // NJ_NINPOU
)

// Regen is a player's regen_data: the HP and SP each natural regen tick
// heals, and the HP and SP each skill regen tick heals.
type Regen struct {
	HP, SP           int
	SkillHP, SkillSP int
}

// RegenOf is status_calc_regen for a player with no equipment, standing:
//
//   - HP: VIT / 5 plus max HP / 200, at least 1 of the latter.
//   - SP: 1 plus INT / 6 plus max SP / 100, and more from INT 120.
//     Meditatio raises it by 3% per level.
//   - Skill HP: Increase HP Recovery, 5 per level plus max HP / 500 per
//     level.
//   - Skill SP: Increase SP Recovery and Ninpou Training, 3 per level plus
//     max SP / 500 per level each, and Lesson, 3 plus 3 per level.
//
// The regen of sitting skills is left out, since players cannot sit yet.
func RegenOf(st *battle.Status, skills map[int]int) Regen {
	var r Regen
	r.HP = min(st.Vit/5+max(1, st.MaxHP/200), 0x7fff)
	sp := 1 + st.Int/6 + st.MaxSP/100
	if st.Int >= 120 {
		sp += (st.Int-120)/2 + 4
	}
	r.SP = min(sp, 0x7fff)
	if lv := skills[skillMeditatio]; lv > 0 {
		r.SP = min(max(r.SP*(100+3*lv)/100, 1), 0x7fff)
	}
	if lv := skills[skillIncHPRecovery]; lv > 0 {
		r.SkillHP = min(lv*5+lv*st.MaxHP/500, 0x7fff)
	}
	skillSP := 0
	if lv := skills[skillIncSPRecovery]; lv > 0 {
		skillSP += lv*3 + lv*st.MaxSP/500
	}
	if lv := skills[skillNinpou]; lv > 0 {
		skillSP += lv*3 + lv*st.MaxSP/500
	}
	if lv := skills[skillLesson]; lv > 0 {
		skillSP += 3 + 3*lv
	}
	r.SkillSP = min(skillSP, 0x7fff)
	return r
}
