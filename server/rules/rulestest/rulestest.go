// Package rulestest makes the sampled mixed rulesets that the parity
// tests run (strategy section 3.6, "Testing", and decision 0019). Each
// package's tests run the recordings whose flags a mix keeps at the
// recording's mode, through the code paths the mix picks.
package rulestest

import (
	"maps"
	"math/rand/v2"
	"slices"

	"github.com/lxsmnsyc/wathena/server/rules"
)

// PerBase is the number of mixes per base that Mixes returns.
const PerBase = 8

// Mixes returns PerBase mixed rulesets per base, renewal's first, drawn
// from a fixed seed, so every package tests the same mixes:
//   - Each flag takes a random value. fourth_job_traits stays off on a
//     pre-renewal base, which the loader requires.
//   - Each skill of rules.RebalancedSkills gets a skill_rebalance flag
//     with a chance of one in four, with a random value.
//   - The songs follow ensemble_songs, as gamedata.RulesOf sets them.
func Mixes() []*rules.Rules {
	rnd := rand.New(rand.NewPCG(20261003, 19)) // #nosec G404 -- a fixed seed for repeatable tests.
	coin := func() bool { return rnd.IntN(2) == 1 }
	ids := slices.Sorted(maps.Values(rules.RebalancedSkills))
	var out []*rules.Rules
	for _, base := range []bool{true, false} {
		for range PerBase {
			rl := &rules.Rules{
				Renewal: base, ASPD: coin(), SizeTable: coin(), CastTime: coin(), SkillCooldown: coin(),
				StatPointCost: coin(), LevelPenaltyExp: coin(), LevelPenaltyDrop: coin(), LevelDamageMod: coin(),
				ElementTable: coin(), StatusResist: coin(), HealFormula: coin(), BossProtection: coin(),
				EnsembleSongs: coin(), Homunculus: coin(), BusyRestrictions: coin(), FourthJobTraits: coin() && base,
				ItemRolls: coin(), Skills: map[int]bool{},
			}
			for _, id := range ids {
				if rnd.IntN(4) == 0 {
					rl.Skills[id] = coin()
				}
			}
			if rl.EnsembleSongs != base {
				for _, id := range rules.SongSkills {
					rl.Skills[id] = rl.EnsembleSongs
				}
			}
			out = append(out, rl)
		}
	}
	return out
}

// Flags returns the ruleset mod flags that make rl: every flag, and a
// skill_rebalance flag for each skill of rules.RebalancedSkills that rl
// sets.
func Flags(rl *rules.Rules) map[string]string {
	mode := func(v bool) string {
		if v {
			return "renewal"
		}
		return "pre_renewal"
	}
	onOff := func(v bool) string {
		if v {
			return "on"
		}
		return "off"
	}
	f := map[string]string{
		"aspd": mode(rl.ASPD), "size_table": mode(rl.SizeTable), "cast_time": mode(rl.CastTime),
		"skill_cooldown": onOff(rl.SkillCooldown), "stat_point_cost": mode(rl.StatPointCost),
		"level_penalty_exp": onOff(rl.LevelPenaltyExp), "level_penalty_drop": onOff(rl.LevelPenaltyDrop),
		"level_damage_mod": onOff(rl.LevelDamageMod), "element_table": mode(rl.ElementTable),
		"status_resist": mode(rl.StatusResist), "heal_formula": mode(rl.HealFormula),
		"boss_protection": mode(rl.BossProtection), "ensemble_songs": mode(rl.EnsembleSongs),
		"homunculus": mode(rl.Homunculus), "busy_restrictions": onOff(rl.BusyRestrictions),
		"fourth_job_traits": onOff(rl.FourthJobTraits), "item_rolls": onOff(rl.ItemRolls),
	}
	for name, id := range rules.RebalancedSkills {
		if v, ok := rl.Skills[id]; ok {
			f["skill_rebalance."+name] = mode(v)
		}
	}
	return f
}

// Away counts the flags of rl that differ from its base, and the
// skill_rebalance flags that differ.
func Away(rl *rules.Rules) (flags, skills int) {
	b := rules.Base(rl.Renewal)
	for _, d := range []bool{
		rl.ASPD != b.ASPD, rl.SizeTable != b.SizeTable, rl.CastTime != b.CastTime, rl.SkillCooldown != b.SkillCooldown,
		rl.StatPointCost != b.StatPointCost, rl.LevelPenaltyExp != b.LevelPenaltyExp, rl.LevelPenaltyDrop != b.LevelPenaltyDrop,
		rl.LevelDamageMod != b.LevelDamageMod, rl.ElementTable != b.ElementTable, rl.StatusResist != b.StatusResist,
		rl.HealFormula != b.HealFormula, rl.BossProtection != b.BossProtection, rl.EnsembleSongs != b.EnsembleSongs,
		rl.Homunculus != b.Homunculus, rl.BusyRestrictions != b.BusyRestrictions, rl.FourthJobTraits != b.FourthJobTraits,
		rl.ItemRolls != b.ItemRolls,
	} {
		if d {
			flags++
		}
	}
	for _, id := range rules.RebalancedSkills {
		if v, ok := rl.Skills[id]; ok && v != rl.Renewal {
			skills++
		}
	}
	return flags, skills
}
