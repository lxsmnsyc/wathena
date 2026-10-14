package clientdata

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMerge(t *testing.T) {
	m, err := Merge([]Override{
		{Mod: "example.a", File: "weapons.yml", Data: []byte(`weapons:
  - {id: 1, constant: W_DAGGER, name: lantern dagger, view: 1, attack: [mod/example.a/sound/swish], hit: [sound/_hit_sword]}
`)},
		{Mod: "example.a", File: "effects.yml", Data: []byte(`effects:
  - {id: 900001, constant: EF_LANTERN, parts: [{effect: mod/example.a/effect/glow}]}
`)},
		{Mod: "example.b", File: "skills.yml", Data: []byte("skills:\n  - {id: 5, constant: SM_BASH, hit: [900001]}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, want := range []string{`"name":"lantern dagger"`, `"EF_LANTERN"`, `"hit":[900001]`} {
		if !strings.Contains(js, want) {
			t.Errorf("merged tables lack %s", want)
		}
	}
	weapons := m["weapons.yml"]["weapons"].([]any)
	core, _ := LoadWeapons()
	if len(weapons) != len(core.Weapons) {
		t.Errorf("%d weapons, want %d: an override replaces its entry", len(weapons), len(core.Weapons))
	}
	if strings.Contains(js, `"name":"dagger"`) {
		t.Error("core's dagger is still there")
	}
}

func TestMergeErrors(t *testing.T) {
	for _, o := range []Override{
		{Mod: "m", File: "items.yml", Data: []byte("{}")},
		{Mod: "m", File: "jobs.yml", Data: []byte("monsters: []")},
		{Mod: "m", File: "jobs.yml", Data: []byte("jobs: [{constant: X}]")},
		{Mod: "m", File: "jobs.yml", Data: []byte("jobs: [{id: 0, constant: JOB_NOVICE, look: no_such_look}]")},
		{Mod: "m", File: "weapons.yml", Data: []byte("weapons: [{id: 1, constant: W, name: x, view: 1, attack: [Bad], hit: []}]")},
		{Mod: "m", File: "effects.yml", Data: []byte("effects: [{id: 1, constant: E, parts: [{effect: e/x}], color: red}]")},
		{Mod: "m", File: "skills.yml", Data: []byte("skills: 3")},
		{Mod: "m", File: "skills.yml", Data: []byte(": :")},
	} {
		if _, err := Merge([]Override{o}); err == nil || !strings.Contains(err.Error(), "m: client/"+o.File) {
			t.Errorf("%s %q: %v", o.File, o.Data, err)
		}
	}
	if _, err := ParseSkills(skillsYAML); err != nil {
		t.Error(err)
	}
}

func FuzzMerge(f *testing.F) {
	f.Add("jobs.yml", []byte("looks: [{look: lantern_keeper, grf_sprite: x, race: human}]"))
	f.Add("skills.yml", []byte("auras: [{id: 1, constant: ELE_WATER, effect: 3}]"))
	f.Fuzz(func(t *testing.T, file string, data []byte) {
		m, err := Merge([]Override{{Mod: "fuzz", File: file, Data: data}})
		if err != nil {
			return
		}
		if _, err := json.Marshal(m); err != nil {
			t.Fatalf("merged tables do not encode: %v", err)
		}
	})
}
