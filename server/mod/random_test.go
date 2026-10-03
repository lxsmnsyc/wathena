package mod

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

// coreFS is core's random options and groups, as the converter writes
// them, for overlay tests.
var coreFS = fstest.MapFS{
	"data/random_options.yml": {Data: []byte(`
- {name: max_hp, id: 1, display_name: MaxHP, bonus: [[max_hp, {roll: value}]]}
- {name: luk, id: 8, display_name: LUK, bonus: [[luk, {roll: value}]]}
`)},
	"data/random_option_groups.yml": {Data: []byte(`
- name: Group_1
  slots:
    - [{option: max_hp, min: 1, max: 5, chance: 5000}]
`)},
}

func TestRandomOptionOverlay(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml": manifest("example.a"),
		"data/random_options.yml": `
- name: example.a:glow
  display_name: Glow
  bonus: [[luk, {roll: value}], [max_hp, {roll: param, multiply: 10}]]
- name: max_hp
  display_name: Max HP
`,
		"data/random_option_groups.yml": `
- name: example.a:SHINY
  slots:
    - [{option: "example.a:glow", min: 1, max: 3, param: 2, chance: 10000}]
  max_random: 1
  random:
    - {option: luk, min: 1, max: 1, chance: 100}
- name: Group_1
  slots:
    - [{option: luk, min: 1, max: 2, chance: 10000}]
`,
	})
	s, err := resolve(t, dir, Config{Core: coreFS})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.RandomOptions(testNames)
	if err != nil {
		t.Fatal(err)
	}
	if o := r.Option("max_hp"); o == nil || o.DisplayName != "Max HP" || o.ID != 1 || len(o.Static) != 1 {
		t.Errorf("max_hp %+v", o)
	}
	if o := r.Option("example.a:glow"); o == nil || len(o.Static) != 2 || o.Static[1].Args[0].Roll != "param" || o.Static[1].Args[0].Multiply != 10 {
		t.Errorf("glow %+v", o)
	}
	if g := r.Group("Group_1"); g == nil || g.Slots[0][0].Option != "luk" {
		t.Errorf("Group_1 %+v", g)
	}
	if g := r.Group("example.a:SHINY"); g == nil || g.MaxRandom != 1 || len(g.Random) != 1 {
		t.Errorf("SHINY %+v", g)
	}
}

func TestRandomOptionErrors(t *testing.T) {
	for _, c := range []struct{ file, data, want string }{
		{"data/random_options.yml", "- {name: example.a:x, bonus: [[luk, {roll: luck}]]}", "not \"luck\""},
		{"data/random_options.yml", "- {name: example.a:x, display_name: X}", "bonus is a list"},
		{"data/random_options.yml", "- {name: example.a:x, bonus: [[luk, {roll: value, times: 2}]]}", "not a field of a roll"},
		{"data/random_options.yml", "- {name: max_hp, id: 9}", "cannot be changed"},
		{"data/random_option_groups.yml", "- {name: example.a:G, slots: [[{option: nope, chance: 1}]]}", "no random option \"nope\""},
		{"data/random_option_groups.yml", "- {name: example.a:G, slots: [[{option: luk, min: 5, max: 1}]]}", "above max"},
		{"data/random_option_groups.yml", "- {name: example.a:G, slots: [[]]}", "has no options"},
		{"data/random_option_groups.yml", "- {name: example.a:G, max_random: 2}", "needs random options"},
		{"data/random_option_groups.yml", "- {name: example.a:G, slots: [[{option: luk, chance: 10001}]]}", "chance is 0 to 10000"},
	} {
		dir := t.TempDir()
		writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a"), c.file: c.data})
		s, err := resolve(t, dir, Config{Core: coreFS})
		if err == nil {
			_, err = s.RandomOptions(testNames)
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.data, err, c.want)
		}
	}
}

func TestItemRolls(t *testing.T) {
	for _, c := range []struct{ data, want string }{
		{"rolls: {luck: {min: 1, max: 5, default: 3}}\n  bonuses: [[luk, {roll: luck}]]", ""},
		{"rolls: {luck: {min: 1, max: 5, default: 3}}\n  bonuses: [[luk, {roll: power}]]", "has no roll \"power\""},
		{"rolls: {Luck: {min: 1, max: 5, default: 3}}", "a roll name is lower-case"},
		{"rolls: {luck: {min: 1, max: 5, default: 9}}", "default 9 is outside"},
		{"rolls: {a: {default: 1}, b: {default: 1}, c: {default: 1}, d: {default: 1}, e: {default: 1}, f: {default: 1}, g: {default: 1}, h: {default: 1}, i: {default: 1}}", "at most 8 rolls"},
		{"rolls: {luck: {min: 1.5, max: 5, default: 3}}", "min"},
		{"rolls: {luck: {default: 2, distribution: weighted, weights: [{value: [1, 3], weight: 1}]}}", ""},
	} {
		dir := t.TempDir()
		writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a"),
			"data/items.yml": "- name: example.a:Charm\n  type: Armor\n  " + c.data + "\n"})
		s, err := resolve(t, dir, Config{})
		if err != nil {
			t.Fatal(err)
		}
		ids, _ := LoadIDs(context.Background(), &MemoryIDs{})
		_, err = s.Items(context.Background(), baseItems, ids, testNames)
		if c.want == "" && err != nil || c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: %v, want %q", c.data, err, c.want)
		}
	}
}
