package mod

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

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
// on needs the renewal base.
func TestEveryFlagBothValues(t *testing.T) {
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
