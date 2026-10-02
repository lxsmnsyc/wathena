package mod

import (
	"testing"
)

func coreSkill(name string) bool { return name == "MG_LIGHTNINGBOLT" || name == "SM_BASH" }

func TestSkillOverlay(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml": manifest("example.a"),
		"data/skills.yml": `- name: example.a:BOLT
  max_level: 3
  script: scripts/bolt.lua
- name: SM_BASH
  cast_time: 500
`,
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	o, err := s.Skills(coreSkill)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Records) != 2 || o.Records[0].Name != "example.a:BOLT" || o.Records[0].Mod != "example.a" || o.Records[1].Mod != Core {
		t.Fatalf("records %+v", o.Records)
	}
	if script, err := ScriptOf(o.Records[0]); err != nil || script != "example.a:scripts/bolt.lua" {
		t.Errorf("script %q, %v", script, err)
	}
}

func TestSkillOverlayNames(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml":        manifest("example.a"),
		"data/skills.yml": "- name: BOLT\n  max_level: 3\n",
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Skills(coreSkill)
	wantError(t, err, "BOLT is not a core record of skills; a new record is named with the mod's ID, such as example.a:BOLT")
}

func TestSkillScriptOnCoreSkillNeedsOverride(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml":        manifest("example.a"),
		"data/skills.yml": "- name: SM_BASH\n  script: scripts/bash.lua\n",
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Skills(coreSkill)
	wantError(t, err, "skills SM_BASH", "example.a sets script on a skill it did not add", "rules.override")

	writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a", "permissions: [rules.override]")})
	if s, err = resolve(t, dir, Config{Approve: map[string][]string{"example.a": {"rules.override"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Skills(coreSkill); err != nil {
		t.Fatal(err)
	}
}

func TestSkillTrees(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml": manifest("example.a"),
		"data/skill_trees.yml": `- job: mage
  skill: example.a:BOLT
  max_level: 3
  requires:
    - {skill: MG_LIGHTNINGBOLT, level: 3}
- job: 4010
  skill: SM_BASH
  max_level: 5
`,
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.SkillTrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Job != "mage" || list[0].Skill != "example.a:BOLT" || list[0].MaxLevel != 3 ||
		len(list[0].Requires) != 1 || list[0].Requires[0] != (TreeRequirement{"MG_LIGHTNINGBOLT", 3}) ||
		list[1].Job != "4010" || list[0].Mod != "example.a" || list[0].Where == "" {
		t.Fatalf("entries %+v", list)
	}
}

func TestSkillTreeErrors(t *testing.T) {
	cases := []struct{ file, want string }{
		{"job: mage\n", "want a list of entries"},
		{"- job: mage\n  skill: X\n", "an entry needs job, skill and max_level"},
		{"- job: mage\n  skill: X\n  max_level: 0\n", "max_level must be a whole number from 1"},
		{"- job: mage\n  skill: X\n  max_level: 1\n  level: 3\n", "level is not a field; the fields are job, skill, max_level and requires"},
		{"- job: mage\n  skill: X\n  max_level: 1\n  requires: [{skill: Y}]\n", "want { skill: MG_LIGHTNINGBOLT, level: 3 }"},
		{"- job: mage\n  skill: X\n  max_level: 1\n  requires: {skill: Y, level: 1}\n", "requires must be a list"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a"), "data/skill_trees.yml": c.file})
		s, err := resolve(t, dir, Config{})
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.SkillTrees()
		wantError(t, err, "data/skill_trees.yml", c.want)
	}
}
