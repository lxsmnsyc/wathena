package skill

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/status"
)

// castCase is one case of skill_casting.json.
type castCase struct {
	Job         int    `json:"job"`
	Build       string `json:"build"`
	Sex         *int   `json:"sex"`
	Skill       int    `json:"skill"`
	SkillName   string `json:"skill_name"`
	Level       int    `json:"level"`
	Cast        int    `json:"cast"`
	Delay       int    `json:"delay"`
	Cooldown    int    `json:"cooldown"`
	Requirement struct {
		HP         int      `json:"hp"`
		MaxHP      int      `json:"mhp"`
		SP         int      `json:"sp"`
		Zeny       int      `json:"zeny"`
		Weapon     int      `json:"weapon"`
		Ammo       int      `json:"ammo"`
		AmmoQty    int      `json:"ammo_qty"`
		State      int      `json:"state"`
		Spiritball int      `json:"spiritball"`
		Items      [][2]int `json:"items"`
	} `json:"requirement"`
	Heal *struct {
		ForcedMin int64 `json:"forced_min"`
		Min       int64 `json:"min"`
		Max       int64 `json:"max"`
	} `json:"heal"`
}

// castingGaps are skills whose recorded casting does not match yet, with
// the reason. Each job's test file adds its own in init.
var castingGaps = map[string]string{}

// TestCasting checks cast times, delays, cooldowns, requirements and
// Heal's amounts against skill_casting.json, for every skill of the first
// classes' trees in both modes.
func TestCasting(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			checkCasting(t, mode, nil)
		})
	}
}

// checkCasting runs a mode's skill_casting.json. rl, when not nil, is a
// mixed ruleset of the mode's base: each part of a case runs only when
// the flags it reads keep the mode's value (castingApplies), through
// the code the mix picks.
func checkCasting(t *testing.T, mode reference.Mode, rl *rules.Rules) {
	{
		{
			db := loadDB(t, mode)
			var jobs status.Data
			if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &jobs); err != nil {
				t.Fatal(err)
			}
			tables, err := status.New(&jobs)
			if err != nil {
				t.Fatal(err)
			}
			skills := map[string]map[int]int{}
			jobLevels := map[string]int{}
			var pcs struct {
				Players []struct {
					Job       int      `json:"job"`
					BaseLevel int      `json:"base_level"`
					JobLevel  int      `json:"job_level"`
					Build     string   `json:"build"`
					AllSkills bool     `json:"all_skills"`
					Skills    [][2]int `json:"skills"`
				} `json:"players"`
			}
			if err := reference.Load(mode, "pc_status", &pcs); err != nil {
				t.Fatal(err)
			}
			for _, p := range pcs.Players {
				if p.BaseLevel == 99 && p.AllSkills {
					m := map[int]int{}
					for _, s := range p.Skills {
						m[s[0]] = s[1]
					}
					key := playerKey(p.Job, p.Build, true)
					skills[key] = m
					jobLevels[key] = p.JobLevel
				}
			}
			var ref struct {
				Players []samplePlayer `json:"players"`
				Cases   []castCase     `json:"cases"`
			}
			if err := reference.Load(mode, "skill_casting", &ref); err != nil {
				t.Fatal(err)
			}
			// The recording's own players come first: they have the right
			// sex.
			for _, p := range ref.Players {
				key := playerKey(p.Job, p.Build, p.Sex == 1)
				skills[key] = learnedMap(p.Skills)
				jobLevels[key] = p.JobLevel
			}
			renewal := mode == reference.Renewal
			full := rl
			if full == nil {
				full = rules.Of(renewal)
			}
			matched, total := 0, 0
			bad := map[string]bool{}
			for i := range ref.Cases {
				c := &ref.Cases[i]
				if _, gap := castingGaps[c.SkillName]; gap {
					continue
				}
				ap := castingApplies(rl, renewal, c.Skill)
				if !ap.skill {
					continue
				}
				total++
				name := fmt.Sprintf("%s lv %d, job %d %s", c.SkillName, c.Level, c.Job, c.Build)
				def := db.Get(c.Skill)
				if def == nil {
					t.Fatalf("%s: skill missing", name)
				}
				male := c.Sex == nil || *c.Sex == 1
				key := playerKey(c.Job, c.Build, male)
				learned := skills[key]
				res, err := tables.Calc(status.Character{Job: c.Job, Male: male, BaseLevel: 99, JobLevel: jobLevels[key],
					Stats: builds[c.Build], Skills: learned})
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				st := res.Battle
				caster := &Caster{Status: &st, Renewal: renewal, Rules: rl}
				var diffs []string
				if got := def.CastTimeFor(caster, c.Level, full.CastTime, learned[c.Skill] > 0).Milliseconds; ap.cast && got != c.Cast {
					diffs = append(diffs, fmt.Sprintf("cast %d, want %d", got, c.Cast))
				}
				if got := def.Delay(caster, c.Level); got != c.Delay {
					diffs = append(diffs, fmt.Sprintf("delay %d, want %d", got, c.Delay))
				}
				if got := def.CooldownFor(caster, c.Level); ap.cooldown && got != c.Cooldown {
					diffs = append(diffs, fmt.Sprintf("cooldown %d, want %d", got, c.Cooldown))
				}
				// The recorder's players have 1 HP and 1 SP.
				req := def.RequirementFor(caster, c.Level, 1, 1)
				w := c.Requirement
				items := w.Items
				if items == nil {
					items = [][2]int{}
				}
				gotItems := req.Items
				if gotItems == nil {
					gotItems = [][2]int{}
				}
				if ap.requirement && (req.HP != w.HP || req.MaxHP != w.MaxHP || req.SP != w.SP || req.Zeny != w.Zeny || req.Weapon != w.Weapon ||
					req.Ammo != w.Ammo || req.AmmoAmount != w.AmmoQty || req.State != w.State || req.SpiritSphere != w.Spiritball ||
					!slices.Equal(gotItems, items)) {
					diffs = append(diffs, fmt.Sprintf("requirement %+v, want %+v", req, w))
				}
				if c.Heal != nil && ap.heal {
					if got := CalcHeal(&st, &st, c.Skill, c.Level, true, full.HealFormula, forcedMin{}); got != c.Heal.ForcedMin {
						diffs = append(diffs, fmt.Sprintf("heal %d, want %d", got, c.Heal.ForcedMin))
					}
				}
				if len(diffs) > 0 {
					bad[c.SkillName] = true
					t.Errorf("%s: %v", name, diffs)
					continue
				}
				matched++
			}
			t.Logf("%s: %d of %d casting cases match; skills with a mismatch: %v; gaps: %v", mode, matched, total,
				slices.Sorted(maps.Keys(bad)), slices.Sorted(maps.Keys(castingGaps)))
		}
	}
}

// castingParts are the parts of a skill_casting.json case that a mixed
// ruleset keeps at the recording's mode.
type castingParts struct {
	// skill is false when the case's skill has a flag of the other mode:
	// its record and its code are the other mode's.
	skill                             bool
	cast, cooldown, requirement, heal bool
}

// castingApplies reports which parts of a case of a mode run under a
// mixed ruleset. A nil ruleset is the mode's core ruleset.
func castingApplies(rl *rules.Rules, renewal bool, skillID int) castingParts {
	if rl == nil {
		return castingParts{true, true, true, true, true}
	}
	return castingParts{
		skill:       rl.Skill(skillID) == renewal,
		cast:        rl.CastTime == renewal,
		cooldown:    rl.SkillCooldown == renewal,
		requirement: rl.EnsembleSongs == renewal,
		heal:        rl.HealFormula == renewal,
	}
}
