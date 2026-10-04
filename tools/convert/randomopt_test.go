package convert

import (
	"os"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
)

// TestRandomOptions converts the random options of both modes. Renewal
// has 249 options: every one converts but the 10 of bSubRace with a
// BF_WEAPON flag, which has no damage_taken sum. Pre-renewal has none.
func TestRandomOptions(t *testing.T) {
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
	for _, c := range []struct {
		mode              reference.Mode
		converted, failed int
	}{{reference.Renewal, 239, 10}, {reference.PreRenewal, 0, 0}} {
		o := loadCore(t, c.mode).RandomOptions
		if len(o.Options) != c.converted || len(o.Failed) != c.failed {
			t.Errorf("%s: %d converted, %d failed", c.mode, len(o.Options), len(o.Failed))
		}
		for _, f := range o.Failed {
			if !strings.HasPrefix(f.RAthena, "RACE_WEAPON_TOLERACE_") {
				t.Errorf("%s: %s does not convert: %v", c.mode, f.RAthena, f.Err)
			}
		}
		seen := map[string]bool{}
		for _, opt := range o.Options {
			if seen[opt.Name] {
				t.Errorf("%s: name %s twice", c.mode, opt.Name)
			}
			seen[opt.Name] = true
		}
		if c.mode != reference.Renewal {
			continue
		}
		data := o.Files()["data/random_options.yml"]
		for _, want := range []string{
			"- {name: max_hp, id: 1, display_name: MaxHP, bonus: [[max_hp, {roll: value}]]}",
			"- {name: physical_damage_against_boss, id: 148, display_name: Physical damage against bosses, bonus: [[damage, {roll: value}, {kind: physical, against: {class: boss}}]]}",
			"- {name: crit_against_angel, id: 125, display_name: CRIT against Angel, bonus: [[crit, {roll: value, divide: 10}, {against: {race: angel}}]]}",
			"- {name: variable_cast_time_percent, id: 170, display_name: Variable cast time, bonus: [[variable_cast_time, {roll: value, multiply: -1}, {percent: true}]]}",
			`- {name: attack_element_fire, id: 178, display_name: "Attack element: Fire", bonus: [[attack_element, fire]]}`,
		} {
			if !strings.Contains(data, want+"\n") {
				t.Errorf("random_options.yml lacks %s", want)
			}
		}
		if n := strings.Count(o.Files()["data/random_option_groups.yml"], "\n- name: "); n != 108 {
			t.Errorf("%d groups, want 108", n)
		}
	}
}

func TestRandomOptionReportsCurrent(t *testing.T) {
	for _, mode := range reference.Modes {
		path := "../../docs/generated/random-options-" + string(mode) + ".md"
		got := loadCore(t, mode).RandomOptions.Report(string(mode))
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != got {
			t.Errorf("%s is not current: run go run ./tools/wactl convert random-options --mode %s --report %s", path, mode, strings.TrimPrefix(path, "../../"))
		}
	}
}
