package rules

import "testing"

// TestBaseAndSkill checks that a core ruleset sets every flag to its
// base, with item_rolls off, and that a skill without a skill_rebalance
// flag follows the base.
func TestBaseAndSkill(t *testing.T) {
	for _, base := range []bool{true, false} {
		r := Base(base)
		for name, v := range map[string]bool{
			"aspd": r.ASPD, "size_table": r.SizeTable, "cast_time": r.CastTime, "skill_cooldown": r.SkillCooldown,
			"stat_point_cost": r.StatPointCost, "level_penalty_exp": r.LevelPenaltyExp, "level_penalty_drop": r.LevelPenaltyDrop,
			"level_damage_mod": r.LevelDamageMod, "element_table": r.ElementTable, "status_resist": r.StatusResist,
			"heal_formula": r.HealFormula, "boss_protection": r.BossProtection, "ensemble_songs": r.EnsembleSongs,
			"homunculus": r.Homunculus, "busy_restrictions": r.BusyRestrictions, "fourth_job_traits": r.FourthJobTraits,
		} {
			if v != base {
				t.Errorf("base %v: %s is %v", base, name, v)
			}
		}
		if r.ItemRolls {
			t.Errorf("base %v: item_rolls is on", base)
		}
		if Of(base).Renewal != base || r.Skill(17) != base {
			t.Errorf("base %v: Of or Skill", base)
		}
		r.Skills = map[int]bool{17: !base}
		if r.Skill(17) == base || r.Skill(18) != base {
			t.Errorf("base %v: a skill_rebalance flag is not read", base)
		}
	}
	for name, id := range RebalancedSkills {
		if _, song := SongSkills[name]; song || id <= 0 {
			t.Errorf("%s: %d is a song or no ID", name, id)
		}
	}
}
