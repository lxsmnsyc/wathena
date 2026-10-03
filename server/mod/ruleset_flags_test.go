package mod

import (
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// flagTests names, for each ruleset flag, the tests that check its
// behavior at both values on both bases where the loader allows them, as
// "<package directory>:<test name>" from the repository root.
// skill_rebalance stands for every skill_rebalance.<skill> flag.
// TestEveryFlagBothValues fails for a flag without a test, unless
// pendingFlags gives the reason, and for a named test that does not
// exist.
var flagTests = map[string][]string{
	"cast_time":          {"server/skill:TestCastTimeFlag", "server/status:TestCastBonusFlag", "server/zone:TestCastTimeFlagZone"},
	"skill_cooldown":     {"server/gamedata:TestSkillCooldownFlag"},
	"aspd":               {"server/status:TestASPDFlag"},
	"stat_point_cost":    {"server/status:TestStatPointCostFlag"},
	"level_penalty_exp":  {"server/progression:TestLevelPenaltyFlags"},
	"level_penalty_drop": {"server/progression:TestLevelPenaltyFlags"},
	"level_damage_mod":   {"server/skill:TestLevelDamageModFlag"},
	"element_table":      {"server/battle:TestElementTableFlag"},
	"size_table":         {"server/battle:TestSizeTableFlagBothBases", "server/status:TestSizeTableFlag"},
	"status_resist":      {"server/statuschange:TestStatusResistFlag"},
	"heal_formula":       {"server/skill:TestHealFormulaFlag", "server/zone:TestHealFormulaFlagZone"},
	"boss_protection":    {"server/zone:TestBossProtectionFlag"},
	"ensemble_songs":     {"server/zone:TestEnsembleSongsFlag", "server/status:TestEnsembleSongsStats", "server/gamedata:TestSkillRecordFlags"},
	"busy_restrictions":  {"server/zone:TestBusyRestrictionsFlag"},
	"fourth_job_traits":  {"server/status:TestFourthJobTraitsFlag"},
	"item_rolls":         {"server/zone:TestRolledDropDistributions", "server/zone:TestDropRolls"},
	"skill_rebalance": {"server/skill:TestSkillRebalanceFlag", "server/zone:TestSkillRebalanceZone",
		"server/status:TestRebalancedStatusEffects", "server/gamedata:TestSkillRecordFlags"},
}

// pendingFlags are the flags that the loader accepts but nothing reads
// yet, with the reason.
var pendingFlags = map[string]string{
	"homunculus": "there is no homunculus system yet (decision 0019)",
}

// checkFlagTests checks that every flag has behavior tests that exist,
// or a reason in pendingFlags.
func checkFlagTests(t *testing.T) {
	t.Helper()
	root := filepath.Join("..", "..")
	sources := map[string]string{}
	src := func(dir string) string {
		if s, ok := sources[dir]; ok {
			return s
		}
		files, err := filepath.Glob(filepath.Join(root, dir, "*_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, f := range files {
			data, err := os.ReadFile(f) // #nosec G304 -- test files of this repository.
			if err != nil {
				t.Fatal(err)
			}
			b.Write(data)
		}
		sources[dir] = b.String()
		return sources[dir]
	}
	names := append(slices.Sorted(maps.Keys(flagValues)), "skill_rebalance")
	for _, name := range names {
		tests := flagTests[name]
		if len(tests) == 0 {
			if why, ok := pendingFlags[name]; ok {
				t.Logf("%s has no behavior: %s", name, why)
				continue
			}
			t.Errorf("the ruleset flag %s has no behavior test: add one to flagTests, or a reason to pendingFlags", name)
			continue
		}
		if _, ok := pendingFlags[name]; ok {
			t.Errorf("%s has tests and a pending reason", name)
		}
		for _, ref := range tests {
			dir, test, ok := strings.Cut(ref, ":")
			if !ok || !regexp.MustCompile(`(?m)^func `+regexp.QuoteMeta(test)+`\(t \*testing\.T\)`).MatchString(src(dir)) {
				t.Errorf("%s: the test %s does not exist", name, ref)
			}
		}
	}
	for name := range flagTests {
		if _, ok := flagValues[name]; !ok && name != "skill_rebalance" {
			t.Errorf("flagTests names %s, which is not a flag", name)
		}
	}
}

// TestCoreRulesetFlags checks that each core ruleset sets every flag of
// strategy section 3.6 to its base's value, with item_rolls off in both.
func TestCoreRulesetFlags(t *testing.T) {
	for _, base := range []string{Renewal, PreRenewal} {
		s, err := Resolve(nil, Config{Base: base})
		if err != nil {
			t.Fatal(err)
		}
		r := s.Ruleset
		if r.Mod != "core.ruleset."+base || r.Base != base {
			t.Errorf("%s: ruleset %s", base, r)
		}
		if got, want := slices.Sorted(maps.Keys(r.Flags)), slices.Sorted(maps.Keys(flagValues)); !slices.Equal(got, want) {
			t.Errorf("%s: flags %v, want %v", base, got, want)
		}
		for name, values := range flagValues {
			want := base
			switch {
			case values[0] != Renewal && name == "item_rolls":
				want = "off"
			case values[0] != Renewal && base == Renewal:
				want = "on"
			case values[0] != Renewal:
				want = "off"
			}
			if got := r.Flag(name); got != want {
				t.Errorf("%s: %s is %q, want %q", base, name, got, want)
			}
		}
	}
}

// TestEveryFlagBothValues sets each flag to each of its values on each
// base through a ruleset mod. The loader keeps the value, changes no
// other flag, and refuses a value the flag does not have. fourth_job_traits
// on needs the renewal base. Each flag must also have behavior tests at
// both values (flagTests), or a reason it has none (pendingFlags).
func TestEveryFlagBothValues(t *testing.T) {
	checkFlagTests(t)
	names := slices.Sorted(maps.Keys(flagValues))
	names = append(names, "skill_rebalance.MG_FIREBALL")
	for _, base := range []string{Renewal, PreRenewal} {
		core := coreFlags(base)
		for _, name := range names {
			values := flagValues[name]
			if values == nil {
				values = []string{Renewal, PreRenewal}
			}
			for _, v := range values {
				dir := t.TempDir()
				writeMod(t, dir, "r", map[string]string{"mod.yaml": manifest("example.flag",
					"provides:\n  ruleset:\n    extends: core.ruleset."+base+"\n    flags:\n      "+name+": \""+v+"\"")})
				s, err := resolve(t, dir, Config{Ruleset: "example.flag"})
				if name == "fourth_job_traits" && v == "on" && base == PreRenewal {
					wantError(t, err, "fourth_job_traits: on needs the renewal base")
					continue
				}
				if err != nil {
					t.Errorf("%s %s=%s: %v", base, name, v, err)
					continue
				}
				r := s.Ruleset
				want := maps.Clone(core)
				want[name] = v
				if r.Base != base || !maps.Equal(r.Flags, want) {
					t.Errorf("%s %s=%s: ruleset %s", base, name, v, r)
				}
				if changed := core[name] != v; changed != strings.Contains(r.String(), name+"="+v) {
					t.Errorf("%s %s=%s: String %q", base, name, v, r.String())
				}
			}
			dir := t.TempDir()
			writeMod(t, dir, "r", map[string]string{"mod.yaml": manifest("example.flag",
				"provides:\n  ruleset:\n    extends: core.ruleset."+base+"\n    flags:\n      "+name+": \"maybe\"")})
			_, err := resolve(t, dir, Config{Ruleset: "example.flag"})
			wantError(t, err, `"maybe" is not one of`)
		}
	}
}
