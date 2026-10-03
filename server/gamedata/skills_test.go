package gamedata

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/skill"
)

// skillSet makes a set of core and one mod with files, by path under
// the mod's folder.
func skillSet(t *testing.T, manifest string, files map[string]string) *mod.Set {
	t.Helper()
	dir := t.TempDir()
	files["mod.yaml"] = "id: example.a\nname: A\nversion: 1.0.0\napi: 1.0\n" + manifest
	for p, s := range files {
		full := filepath.Join(dir, "a", filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	found, err := mod.Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	set, err := mod.Resolve(found, mod.Config{Approve: map[string][]string{"example.a": {"rules.override"}}})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func testConstants(t *testing.T) *constants.Data {
	t.Helper()
	data, err := reference.ReadFile("extracts/renewal/script_constants.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := constants.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func loadModSkills(t *testing.T, set *mod.Set) (*skill.DB, *skill.Trees, error) {
	t.Helper()
	scripts, err := set.AllScripts()
	if err != nil {
		t.Fatal(err)
	}
	ids, err := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	if err != nil {
		t.Fatal(err)
	}
	db, trees, _, err := modSkills(context.Background(), set, reference.Renewal, ids, testConstants(t),
		map[string]uint32{"Red_Gemstone": 716}, scripts, nil)
	return db, trees, err
}

func TestModSkills(t *testing.T) {
	set := skillSet(t, "", map[string]string{
		"data/skills.yml": `
- name: example.a:CHAIN
  display_name: Chain
  icon: MG_LIGHTNINGBOLT
  max_level: 5
  type: magic
  target: enemy
  element: wind
  range: 9
  cast_time: [1000, 1200]
  requirements:
    sp: [20, 24, 28, 32, 36]
    items: [{item: Red_Gemstone}]
  script: scripts/skills/chain.lua
- name: MG_LIGHTNINGBOLT
  cast_time: 500
`,
		"data/skill_trees.yml": `
- job: mage
  skill: example.a:CHAIN
  max_level: 5
  requires: [{skill: MG_LIGHTNINGBOLT, level: 3}]
`,
		"scripts/skills/chain.lua": "return function() end",
	})
	db, trees, err := loadModSkills(t, set)
	if err != nil {
		t.Fatal(err)
	}
	core, coreTrees, err := LoadSkills(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	bolt := db.ByName("MG_LIGHTNINGBOLT")
	d := db.ByName("example.a:CHAIN")
	if d == nil || d.ID != 20_000 || d.Mod != "example.a" || d.Icon != bolt.ID || d.Type != battle.TypeMagic || d.Target != skill.TargetEnemy ||
		d.Element.At(1) != 4 || d.Range.At(5) != 9 || d.CastTime.At(5) != 1200 || d.Requires.SP.At(5) != 36 ||
		d.Requires.Items[0] != [2]int{716, 1} || d.Script != "example.a:scripts/skills/chain.lua" || d.DisplayName != "Chain" {
		t.Fatalf("chain %+v", d)
	}
	if bolt.CastTime.At(5) != 500 || core.ByName("MG_LIGHTNINGBOLT").CastTime.At(5) == 500 {
		t.Error("the overlay of Lightning Bolt should change the mods' database only")
	}
	// Mages and the jobs that take their tree learn it.
	for _, job := range []int{2, 9, 4010} {
		e := trees.Job(job).Skill(d.ID)
		if e == nil || e.MaxLevel != 5 || len(e.Requires) != 1 || e.Requires[0] != [2]int{bolt.ID, 3} {
			t.Errorf("job %d: %+v", job, e)
		}
	}
	if coreTrees.Job(2).Skill(d.ID) != nil || trees.Job(1).Skill(d.ID) != nil {
		t.Error("the tree entry went where it should not")
	}
}

func TestModSkillErrors(t *testing.T) {
	cases := []struct {
		files map[string]string
		want  []string
	}{
		{map[string]string{"data/skills.yml": "- name: example.a:X\n  max_level: 20\n  target: pet\n"},
			[]string{"skills example.a:X: max_level: want 1 to 13", "skills example.a:X: target: want passive"}},
		{map[string]string{"data/skills.yml": "- name: example.a:X\n  script: scripts/x.lua\n"},
			[]string{"skills example.a:X: script example.a:scripts/x.lua does not exist"}},
		{map[string]string{"data/skill_trees.yml": "- job: pirate\n  skill: SM_BASH\n  max_level: 1\n"},
			[]string{"skill_trees: pirate is not a job"}},
		{map[string]string{"data/skill_trees.yml": "- job: mage\n  skill: NOPE\n  max_level: 1\n"},
			[]string{"skill_trees: there is no skill NOPE"}},
		{map[string]string{"data/skill_trees.yml": "- job: mage\n  skill: SM_BASH\n  max_level: 11\n"},
			[]string{"SM_BASH: max_level 11 is above the skill's max_level 10"}},
		{map[string]string{"data/skill_trees.yml": "- job: mage\n  skill: SM_BASH\n  max_level: 1\n  requires: [{skill: MG_LIGHTNINGBOLT, level: 11}]\n"},
			[]string{"requires MG_LIGHTNINGBOLT level 11, above its max_level 10"}},
	}
	for _, c := range cases {
		_, _, err := loadModSkills(t, skillSet(t, "", c.files))
		if err == nil {
			t.Errorf("%v: no error", c.files)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("error %q lacks %q", err, w)
			}
		}
	}
}

func TestModSkillScriptOnCoreSkill(t *testing.T) {
	files := map[string]string{
		"data/skills.yml":       "- name: SM_BASH\n  script: scripts/bash.lua\n",
		"scripts/bash.lua":      "return function() end",
		"data/skill_trees.yml":  "",
		"scripts/lib/other.lua": "return 1",
	}
	if _, _, err := loadModSkills(t, skillSet(t, "", files)); err == nil || !strings.Contains(err.Error(), "rules.override") {
		t.Fatalf("error %v, want one that names rules.override", err)
	}
	db, _, err := loadModSkills(t, skillSet(t, "permissions: [rules.override]\n", files))
	if err != nil {
		t.Fatal(err)
	}
	if d := db.ByName("SM_BASH"); d.Script != "example.a:scripts/bash.lua" || d.Mod != "" {
		t.Errorf("bash %+v", d)
	}
}
