package skill

import (
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// testNames knows two skills, one item, two elements and two weapon
// types.
type testNames struct{}

func (testNames) Skill(name string) (int, bool) {
	id, ok := map[string]int{"MG_LIGHTNINGBOLT": 20, "example.a:BOLT": 20_000}[name]
	return id, ok
}

func (testNames) Item(name string) (int, bool) { return 716, name == "Red_Gemstone" }

func (testNames) Element(name string) (int, bool) {
	id, ok := map[string]int{"wind": 4, "fire": 3, "all": 10}[name]
	return id, ok
}

func (testNames) WeaponType(name string) (int, bool) {
	id, ok := map[string]int{"dagger": 1, "staff": 10}[name]
	return id, ok
}

func TestApply(t *testing.T) {
	d := NewModDef(20_000, "example.a:BOLT", "example.a")
	err := Apply(d, map[string]any{
		"display_name": "Bolt", "icon": "MG_LIGHTNINGBOLT", "max_level": 5, "type": "magic", "target": "enemy",
		"element": []any{"wind", "fire"}, "range": 9, "cast_time": []any{1000, 1200, 1400},
		"fixed_cast_time": -20, "after_cast_delay": 500, "cooldown": 0, "hit": "single", "hit_count": []any{1, 2},
		"damage_flags": []any{"ignore_flee", "splash"}, "splash": 2, "knockback": 3,
		"requirements": map[string]any{
			"sp": []any{10, 12}, "hp": 5, "zeny": 100, "sp_rate": -5, "weapons": []any{"dagger", "staff"},
			"items":          []any{map[string]any{"item": "Red_Gemstone", "amount": 2}, map[string]any{"item": "Red_Gemstone"}},
			"spirit_spheres": 1,
		},
		"script": "scripts/bolt.lua",
	}, testNames{})
	if err != nil {
		t.Fatal(err)
	}
	if d.DisplayName != "Bolt" || d.Icon != 20 || d.MaxLevel != 5 || d.Type != battle.TypeMagic || d.Target != TargetEnemy {
		t.Errorf("def %+v", d)
	}
	if d.Element.At(1) != 4 || d.Element.At(2) != 3 || d.Element.At(5) != 3 || d.Range.At(5) != 9 {
		t.Errorf("element %v range %v", d.Element, d.Range)
	}
	if d.CastTime.At(1) != 1000 || d.CastTime.At(3) != 1400 || d.CastTime.At(5) != 1400 || d.FixedCastTime.At(1) != -20 || d.AfterCastActDelay.At(2) != 500 {
		t.Errorf("times %v %v", d.CastTime, d.FixedCastTime)
	}
	if d.Hit != HitSingle || d.HitCount.At(1) != 1 || d.HitCount.At(4) != 2 || d.NK != battle.NKIgnoreFlee|battle.NKSplash || d.Splash.At(1) != 2 || d.Knockback.At(1) != 3 {
		t.Errorf("hits %+v", d)
	}
	q := d.Requires
	if q.SP.At(1) != 10 || q.SP.At(3) != 12 || q.HP.At(1) != 5 || q.Zeny.At(1) != 100 || q.SPRate.At(1) != -5 ||
		q.Weapon != 1<<1|1<<10 || q.Items[0] != [2]int{716, 2} || q.Items[1] != [2]int{716, 1} || q.SpiritSphere.At(1) != 1 {
		t.Errorf("requirements %+v", q)
	}
	if d.Script != "" {
		t.Errorf("Apply should leave script to the caller, got %q", d.Script)
	}
}

func TestApplyErrors(t *testing.T) {
	cases := []struct {
		fields map[string]any
		want   string
	}{
		{map[string]any{"max_level": 14}, "max_level: want 1 to 13"},
		{map[string]any{"type": "fire"}, "type: want weapon, magic, misc or none"},
		{map[string]any{"target": "trap"}, "target: want passive, enemy, ground, self or support"},
		{map[string]any{"element": "all"}, "element: all is not an element"},
		{map[string]any{"element": []any{"wind", "water"}}, "element: level 2: water is not an element"},
		{map[string]any{"range": 31}, "range: 31 is not a whole number from 0 to 30"},
		{map[string]any{"cast_time": []any{}}, "cast_time: a list has 1 to 13 values"},
		{map[string]any{"cast_time": 1.5}, "cast_time: 1.5 is not a whole number"},
		{map[string]any{"icon": "NOPE"}, "icon: want the name of a skill"},
		{map[string]any{"damage_flags": []any{"fast"}}, "damage_flags: fast is not a flag"},
		{map[string]any{"requirements": map[string]any{"mana": 1}}, "requirements: mana: not a requirement"},
		{map[string]any{"requirements": map[string]any{"weapons": []any{"spoon"}}}, "requirements: weapons: spoon is not a weapon type"},
		{map[string]any{"requirements": map[string]any{"items": []any{map[string]any{"item": "Nope"}}}}, "requirements: items: entry 1"},
		{map[string]any{"speed": 1}, "speed: not a field"},
	}
	for _, c := range cases {
		err := Apply(NewModDef(1, "x:Y", "x"), c.fields, testNames{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: error %v, want %q", c.fields, err, c.want)
		}
	}
}

func TestCloneAndPut(t *testing.T) {
	db, err := Load(&Data{Mode: "renewal", Skills: []RawSkill{{ID: 5, Name: "SM_BASH", MaxLevel: 10}, {ID: 20, Name: "MG_LIGHTNINGBOLT", MaxLevel: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	c := db.Clone()
	bash := c.Get(5).Copy()
	bash.MaxLevel = 12
	c.Put(bash)
	c.Put(NewModDef(20_000, "example.a:BOLT", "example.a"))
	if db.Get(5).MaxLevel != 10 || db.Get(20_000) != nil {
		t.Error("Put changed the original")
	}
	if c.Get(5).MaxLevel != 12 || c.ByName("example.a:BOLT").ID != 20_000 || c.ByName("sm_bash") != bash {
		t.Error("the copy lacks the changes")
	}
	var order []int
	for _, d := range c.All() {
		order = append(order, d.ID)
	}
	if !slices.Equal(order, []int{5, 20, 20_000}) {
		t.Errorf("order %v", order)
	}
	if !c.Renewal() {
		t.Error("the copy lost the mode")
	}
}

func TestTreesForMods(t *testing.T) {
	data := &TreeData{Mode: "renewal"}
	data.Trees = append(data.Trees,
		struct {
			Job     int         `json:"job"`
			Inherit []int       `json:"inherit"`
			Skills  []TreeSkill `json:"skills"`
		}{Job: 2, Skills: []TreeSkill{{Skill: 20, MaxLevel: 10}}},
		struct {
			Job     int         `json:"job"`
			Inherit []int       `json:"inherit"`
			Skills  []TreeSkill `json:"skills"`
		}{Job: 9, Inherit: []int{2}, Skills: []TreeSkill{{Skill: 20, MaxLevel: 10}, {Skill: 30, MaxLevel: 5}}},
		struct {
			Job     int         `json:"job"`
			Inherit []int       `json:"inherit"`
			Skills  []TreeSkill `json:"skills"`
		}{Job: 4010, Inherit: []int{9}, Skills: []TreeSkill{{Skill: 20, MaxLevel: 7}}},
	)
	if got := data.Inheritors(2); !slices.Equal(got, []int{9, 4010}) {
		t.Fatalf("inheritors %v", got)
	}
	base := LoadTrees(data)
	trees := base.Clone()
	trees.SetInherited(2, TreeSkill{Skill: 20_000, MaxLevel: 5, Requires: [][2]int{{20, 3}}}, data.Inheritors(2))
	trees.SetInherited(2, TreeSkill{Skill: 20, MaxLevel: 12}, data.Inheritors(2))
	if base.Job(2).Skill(20_000) != nil || base.Job(2).Skill(20).MaxLevel != 10 {
		t.Error("SetInherited changed the original trees")
	}
	for _, job := range []int{2, 9, 4010} {
		if e := trees.Job(job).Skill(20_000); e == nil || e.MaxLevel != 5 || !slices.Equal(e.Requires, [][2]int{{20, 3}}) {
			t.Errorf("job %d: entry %+v", job, e)
		}
	}
	// The Wizard's copy of the Mage's entry follows it; the transcendent
	// job lists the skill itself and keeps its own entry.
	if trees.Job(9).Skill(20).MaxLevel != 12 || trees.Job(4010).Skill(20).MaxLevel != 7 {
		t.Errorf("inherited entries %+v %+v", trees.Job(9).Skill(20), trees.Job(4010).Skill(20))
	}
	if !slices.Equal(trees.Jobs(), []int{2, 9, 4010}) {
		t.Errorf("jobs %v", trees.Jobs())
	}
}
