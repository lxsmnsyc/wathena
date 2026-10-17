package spawns

import (
	"strings"
	"testing"
)

// TestParseTags: a record's spawn field, over base tags, and its checks.
func TestParseTags(t *testing.T) {
	base := &Tags{Biomes: map[string]int{"grassland": 50}, Role: Common, Themes: []string{"cute"}}
	got, err := ParseTags(map[string]any{"biomes": map[string]any{"forest": 10, "swamp": 1000}}, base)
	if err != nil || got.Role != Common || got.Weight("forest") != 10 || got.Weight("grassland") != 0 || len(got.Themes) != 1 {
		t.Errorf("biomes over base: %+v %v", got, err)
	}
	if base.Weight("grassland") != 50 || base.Weight("forest") != 0 {
		t.Error("the base changed")
	}
	got, err = ParseTags(map[string]any{"role": "rare", "themes": []any{"undead", "undead"}}, nil)
	if err != nil || got.Role != Rare || len(got.Themes) != 1 || len(got.Biomes) != 0 {
		t.Errorf("role and themes: %+v %v", got, err)
	}
	if got, err := ParseTags(map[string]any{}, nil); err != nil || got.Role != "" {
		t.Errorf("no role: %+v %v", got, err)
	}
	for _, c := range []struct {
		v    any
		want string
	}{
		{"common", "must be a table"},
		{map[string]any{"role": "boss"}, `"boss" is not a role`},
		{map[string]any{"biomes": map[string]any{"forest": 0}}, "weight must be"},
		{map[string]any{"biomes": map[string]any{"Forest": 10}}, "is not a biome name"},
		{map[string]any{"biomes": []any{"forest"}}, "must map biome names"},
		{map[string]any{"themes": "cute"}, "must be a list"},
		{map[string]any{"weight": 3}, `"weight" is not a field`},
	} {
		if _, err := ParseTags(c.v, nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.v, err, c.want)
		}
	}
	boss := Monster{Name: "ECLIPSE", Boss: true, Tags: &Tags{Role: Common}}
	if err := boss.CheckTags(); err == nil || !strings.Contains(err.Error(), "never common") {
		t.Errorf("a common boss: %v", err)
	}
	boss.Tags.Role = MiniBoss
	if err := boss.CheckTags(); err != nil {
		t.Error(err)
	}
	if p := NewPool([]Monster{{Name: "B", Level: 5, Boss: true, Tags: &Tags{Role: Uncommon}}, {Name: "A", Level: 5, Tags: &Tags{Role: Common}}}, nil); len(p.Monsters()) != 1 {
		t.Errorf("the pool kept a common boss: %+v", p.Monsters())
	}
}

// TestParseTheme: a theme record's fields and checks, and which monsters
// it matches.
func TestParseTheme(t *testing.T) {
	races := func(s string) bool { return s == "undead" || s == "plant" }
	th, err := ParseTheme("example.m:mossy", map[string]any{"biomes": []any{"forest"}, "races": []any{"plant"}, "boost": 250}, races, nil)
	if err != nil || th.Boost != 250 || !th.Applies("forest") || th.Applies("desert") {
		t.Fatalf("%+v %v", th, err)
	}
	if !th.Matches(Monster{Race: "plant"}) || !th.Matches(Monster{Tags: &Tags{Themes: []string{"mossy"}}}) || th.Matches(Monster{Race: "brute"}) {
		t.Error("matches")
	}
	if th, _ := ParseTheme("any", map[string]any{}, nil, nil); th.Boost != 300 || !th.Applies("moon") {
		t.Errorf("defaults %+v", th)
	}
	for _, f := range []map[string]any{
		{"boost": 50}, {"races": []any{"robot"}}, {"biomes": "forest"}, {"colour": "green"},
	} {
		if _, err := ParseTheme("x", f, races, nil); err == nil {
			t.Errorf("%v was taken", f)
		}
	}
	if _, err := ParseTheme("Bad Name", map[string]any{}, nil, nil); err == nil {
		t.Error("a bad name was taken")
	}
}

// TestResolve: the spawn settings' defaults and checks.
func TestResolve(t *testing.T) {
	d := Densities{Field: 41, Dungeon: 48}
	r, err := Settings{}.Resolve(d)
	if err != nil || r.Rate != nil || r.TownFieldBossChance != 50 || r.TownFieldBossRole != MiniBoss || r.FieldDensity != 41 ||
		r.Density != 48 || !r.Economy {
		t.Errorf("defaults %+v %v", r, err)
	}
	off := false
	if r, err := (Settings{Economy: &off}).Resolve(d); err != nil || r.Economy {
		t.Errorf("economy off %+v %v", r, err)
	}
	zero := 0
	s := Settings{TownFieldBossChance: &zero, TownFieldBossRole: "field_boss"}
	s.Density.Field = 100
	if r, err := s.Resolve(d); err != nil || r.TownFieldBossChance != 0 || r.TownFieldBossRole != FieldBoss || r.FieldDensity != 100 || r.Density != 48 {
		t.Errorf("set %+v %v", r, err)
	}
	big := 101
	for _, bad := range []Settings{{TownFieldBossChance: &big}, {TownFieldBossRole: "rare"}} {
		if _, err := bad.Resolve(d); err == nil {
			t.Errorf("%+v was taken", bad)
		}
	}
	s = Settings{}
	s.Density.Dungeon = MaxDensity + 1
	if _, err := s.Resolve(d); err == nil {
		t.Error("a density too high")
	}
}

// TestLoadCore: core's spawn tags load, with rAthena's densities and the
// roles of section 8.11.
func TestLoadCore(t *testing.T) {
	c, err := LoadCore()
	if err != nil {
		t.Fatal(err)
	}
	if c.Densities.Field < 10 || c.Densities.Dungeon < 10 {
		t.Errorf("densities %+v", c.Densities)
	}
	for name, want := range map[string]Role{"PORING": Common, "RED_PLANT": Plant, "ECLIPSE": MiniBoss, "BAPHOMET": FieldBoss} {
		if got := c.Tags[name]; got == nil || got.Role != want {
			t.Errorf("%s: %+v, want %s", name, got, want)
		}
	}
	if c.Tags["PORING"].Weight("grassland") < c.Tags["PORING"].Weight("cave") {
		t.Errorf("Poring's weights %v", c.Tags["PORING"].Biomes)
	}
}
