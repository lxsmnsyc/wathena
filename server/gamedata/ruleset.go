package gamedata

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Ruleset flags (strategy section 3.6, decision 0019): RulesOf turns a
// world's ruleset into the flags the game code reads, and the functions
// here apply the flags that change data:
//   - skill_cooldown takes the cooldowns of the flag's mode.
//   - ensemble_songs and skill_rebalance.<skill> take the skill_db
//     records of the songs, or of the skill, from the flag's mode.
//   - level_penalty_exp and level_penalty_drop turn the level penalty
//     on or off, with renewal's table on a pre-renewal base.
//   - aspd, size_table and the flags that player stats read go to the
//     job tables (ApplyRuleset).

// RulesOf turns a ruleset into the flags the game code reads. skills is
// the skill database whose names the skill_rebalance flags name: core's,
// as a mod's skill has no renewal version. It also returns notes for the
// owner, such as a skill_rebalance flag on a skill whose versions the
// server does not tell apart yet.
func RulesOf(r *mod.Ruleset, skills *skill.DB) (*rules.Rules, []string, error) {
	re := func(name string) bool { return r.Flag(name) == mod.Renewal || r.Flag(name) == "on" }
	rl := &rules.Rules{
		Renewal: r.Base == mod.Renewal, ASPD: re("aspd"), SizeTable: re("size_table"), CastTime: re("cast_time"),
		SkillCooldown: re("skill_cooldown"), StatPointCost: re("stat_point_cost"), LevelPenaltyExp: re("level_penalty_exp"),
		LevelPenaltyDrop: re("level_penalty_drop"), LevelDamageMod: re("level_damage_mod"), ElementTable: re("element_table"),
		StatusResist: re("status_resist"), HealFormula: re("heal_formula"), BossProtection: re("boss_protection"),
		EnsembleSongs: re("ensemble_songs"), Homunculus: re("homunculus"), BusyRestrictions: re("busy_restrictions"),
		FourthJobTraits: re("fourth_job_traits"), ItemRolls: re("item_rolls"),
	}
	var err error
	if rl.Limits, err = rules.LimitsOf(r.Limits); err != nil {
		return nil, nil, fmt.Errorf("the ruleset %s: %w", r.Mod, err)
	}
	if rl.Party, err = rules.PartyOf(r.Rules); err != nil {
		return nil, nil, fmt.Errorf("the ruleset %s: %w", r.Mod, err)
	}
	if rl.Guild, err = rules.GuildOf(r.Rules); err != nil {
		return nil, nil, fmt.Errorf("the ruleset %s: %w", r.Mod, err)
	}
	if rl.Items, err = rules.ItemsOf(r.Rules); err != nil {
		return nil, nil, fmt.Errorf("the ruleset %s: %w", r.Mod, err)
	}
	var notes []string
	var errs []string
	for _, name := range slices.Sorted(maps.Keys(r.Flags)) {
		sk, ok := strings.CutPrefix(name, "skill_rebalance.")
		if !ok {
			continue
		}
		d := skills.ByName(sk)
		switch {
		case d == nil || d.Mod != "":
			errs = append(errs, fmt.Sprintf("%s: there is no core skill %s", name, sk))
			continue
		case rules.SongSkills[d.Name] != 0:
			errs = append(errs, fmt.Sprintf("%s: songs and ensembles follow ensemble_songs", name))
			continue
		case rules.RebalancedSkills[d.Name] == 0:
			notes = append(notes, fmt.Sprintf("%s: the server has no separate versions of %s yet, so only its skill_db record changes", name, d.Name))
		}
		if rl.Skills == nil {
			rl.Skills = map[int]bool{}
		}
		rl.Skills[d.ID] = re(name)
	}
	if len(errs) > 0 {
		return nil, nil, fmt.Errorf("the ruleset %s: %s", r.Mod, strings.Join(errs, "; "))
	}
	if rl.EnsembleSongs != rl.Renewal {
		if rl.Skills == nil {
			rl.Skills = map[int]bool{}
		}
		for _, id := range rules.SongSkills {
			rl.Skills[id] = rl.EnsembleSongs
		}
	}
	return rl, notes, nil
}

// otherMode is the rAthena mode that is not mode.
func otherMode(mode reference.Mode) reference.Mode {
	if mode == reference.Renewal {
		return reference.PreRenewal
	}
	return reference.Renewal
}

// applySkillFlags applies the flags that change skill data to core, a
// skill database of mode that nothing else holds yet, before mods'
// overlays:
//   - A skill whose flag (skill_rebalance, or ensemble_songs for the
//     songs) is not the base's takes its record from the other mode.
//   - skill_cooldown takes the cooldowns of its own mode. A skill that
//     mode does not have keeps its cooldown.
func applySkillFlags(core *skill.DB, mode reference.Mode, rl *rules.Rules) error {
	var swap []int
	for _, id := range slices.Sorted(maps.Keys(rl.Skills)) {
		if rl.Skills[id] != rl.Renewal {
			swap = append(swap, id)
		}
	}
	otherCooldowns := rl.SkillCooldown != rl.Renewal
	if len(swap) == 0 && !otherCooldowns {
		return nil
	}
	data, _, err := LoadSkillData(otherMode(mode))
	if err != nil {
		return err
	}
	other, err := skill.Load(data)
	if err != nil {
		return err
	}
	kept := map[int]skill.Levels{}
	for _, id := range swap {
		d, o := core.Get(id), other.Get(id)
		if d == nil || o == nil || o.Name != d.Name {
			continue
		}
		kept[id] = d.Cooldown
		core.Put(o.Copy())
	}
	if otherCooldowns {
		core.UseCooldowns(other)
		return nil
	}
	// The cooldowns stay the base's, also for the records taken from
	// the other mode.
	for id, cd := range kept {
		core.Get(id).Cooldown = cd
	}
	return nil
}

// levelPenalty applies level_penalty_exp and level_penalty_drop to the
// exp and drop tables of mode.
func levelPenalty(t *progression.Tables, mode reference.Mode, rl *rules.Rules) (*progression.Tables, error) {
	renewal := mode == reference.Renewal
	if rl.LevelPenaltyExp == renewal && rl.LevelPenaltyDrop == renewal {
		return t, nil
	}
	var re *progression.Data
	if !renewal {
		_, d, err := LoadProgression(reference.Renewal)
		if err != nil {
			return nil, err
		}
		re = d
	}
	return t.WithLevelPenalty(rl.LevelPenaltyExp, rl.LevelPenaltyDrop, re)
}

// ApplyRuleset sets the ruleset flags that equipment and player stats
// read (strategy section 3.6): aspd, the ASPD formula, shield penalty
// and bonus stacking; size_table, the size table of knuckles and whips;
// and cast_time, stat_point_cost, fourth_job_traits, ensemble_songs and
// skill_rebalance through SetRules. The aspd flag of the other mode
// needs that mode's job table.
func ApplyRuleset(jobs *status.Tables, rl *rules.Rules) error {
	var other *status.Data
	if rl.ASPD != jobs.Renewal() {
		mode := reference.PreRenewal
		if rl.ASPD {
			mode = reference.Renewal
		}
		other = &status.Data{}
		if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", other); err != nil {
			return err
		}
	}
	if err := jobs.SetASPD(rl.ASPD, other); err != nil {
		return err
	}
	jobs.SetSizeTable(!rl.SizeTable)
	jobs.SetRules(rl)
	return nil
}
