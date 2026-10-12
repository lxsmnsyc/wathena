package rules

import (
	"slices"
	"strings"
)

// Reading rules from scripts (proposal 0002, review decision 32):
//   - Is reads a ruleset flag by its name in a ruleset mod. A flag is on
//     at its renewal value or at on.
//   - Rule reads a ruleset rule by name.
//   - ExpandableLimits are the limits a script may raise for one player
//     (player:expand).

// FlagNames are the ruleset flags of strategy section 3.6, sorted. The
// skill_rebalance.<skill> flags are not in it: SkillFlag reads them.
var FlagNames = []string{
	"aspd", "boss_protection", "busy_restrictions", "cast_time", "element_table", "ensemble_songs", "fourth_job_traits",
	"heal_formula", "homunculus", "item_rolls", "level_damage_mod", "level_penalty_drop", "level_penalty_exp",
	"size_table", "skill_cooldown", "stat_point_cost", "status_resist",
}

// SkillFlagPrefix starts the name of a skill_rebalance flag.
const SkillFlagPrefix = "skill_rebalance."

// Is reports whether a ruleset flag is on: its renewal value, or on. ok
// is false for a name that is not a flag. A skill_rebalance.<skill> flag
// names a skill of RebalancedSkills or SongSkills, and is the flag the
// skill follows: its own, or the base.
func (r *Rules) Is(name string) (on, ok bool) {
	if sk, found := strings.CutPrefix(name, SkillFlagPrefix); found {
		id := RebalancedSkills[sk]
		if id == 0 {
			id = SongSkills[sk]
		}
		if id == 0 {
			return false, false
		}
		return r.Skill(id), true
	}
	v, ok := map[string]bool{
		"aspd": r.ASPD, "boss_protection": r.BossProtection, "busy_restrictions": r.BusyRestrictions, "cast_time": r.CastTime,
		"element_table": r.ElementTable, "ensemble_songs": r.EnsembleSongs, "fourth_job_traits": r.FourthJobTraits,
		"heal_formula": r.HealFormula, "homunculus": r.Homunculus, "item_rolls": r.ItemRolls,
		"level_damage_mod": r.LevelDamageMod, "level_penalty_drop": r.LevelPenaltyDrop, "level_penalty_exp": r.LevelPenaltyExp,
		"size_table": r.SizeTable, "skill_cooldown": r.SkillCooldown, "stat_point_cost": r.StatPointCost,
		"status_resist": r.StatusResist,
	}[name]
	return v, ok
}

// RuleValuesOf turns the rules a ruleset set into every rule's value. A
// rule it does not set has rAthena's value.
func RuleValuesOf(set map[string]int) (map[string]int, error) {
	return valuesOf(RuleBounds, set)
}

// DefaultValues are rAthena's values of every rule, by name.
func DefaultValues() map[string]int {
	v, _ := valuesOf(RuleBounds, nil)
	return v
}

// Rule is a ruleset rule's value: a number, or 1 and 0 for a rule that
// is yes or no. ok is false for a name that is not a rule. Rules made
// without values, such as Base's, have rAthena's.
func (r *Rules) Rule(name string) (v int, ok bool) {
	b, ok := FindBound(RuleBounds, name)
	if !ok {
		return 0, false
	}
	if r != nil {
		if v, set := r.Values[name]; set {
			return v, true
		}
	}
	return b.Default, true
}

// RuleNames are the names of the ruleset rules, in RuleBounds order.
func RuleNames() []string {
	out := make([]string, len(RuleBounds))
	for i, b := range RuleBounds {
		out[i] = b.Name
	}
	return out
}

// LimitNames are the names of the limits, in LimitBounds order.
func LimitNames() []string {
	out := make([]string, len(LimitBounds))
	for i, b := range LimitBounds {
		out[i] = b.Name
	}
	return out
}

// RangeOf is a limit's base and max by name. ok is false for a name that
// is not a limit.
func (l Limits) RangeOf(name string) (Range, bool) {
	for i, b := range LimitBounds {
		if b.Name == name {
			return Range{Base: l.bases()[i], Max: l.Max[i]}, true
		}
	}
	return Range{}, false
}

// ExpandableLimits are the limits that one player holds, which
// player:expand raises (review decision 32):
//   - inventory_slots saves with the character.
//   - cart_slots saves with the character's cart.
//   - storage_slots saves with the account's storage, which the
//     account's characters share.
var ExpandableLimits = []string{"inventory_slots", "storage_slots", "cart_slots"}

// Expandable reports whether a limit is one player:expand raises.
func Expandable(name string) bool { return slices.Contains(ExpandableLimits, name) }

// Clamp keeps a player's value of a limit within its range. A value of 0
// or less, one never expanded, is the base.
func (r Range) Clamp(v int) int {
	if v <= 0 {
		return r.Base
	}
	return min(max(v, r.Base), max(r.Max, r.Base))
}
