package gamedata

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/skill"
)

// companionSet makes a set of core, with core's companion files, and one
// mod with files.
func companionSet(t *testing.T, files map[string]string) *mod.Set {
	t.Helper()
	dir := t.TempDir()
	files["mod.yaml"] = "id: example.a\nname: A\nversion: 1.0.0\napi: 1.0\n"
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
	core, err := fs.Sub(companion.CoreFiles, "core")
	if err != nil {
		t.Fatal(err)
	}
	set, err := mod.Resolve(found, mod.Config{Core: core})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func loadCompanions(t *testing.T, set *mod.Set, skills *skill.DB) (*companion.Set, error) {
	t.Helper()
	ids, err := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := modCompanions(context.Background(), set, ids, skills, nil)
	return c, err
}

func coreSkills(t *testing.T) *skill.DB {
	t.Helper()
	base, _, err := LoadSkillData(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	db, err := skill.Load(base)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// TestModCompanions checks core's records through the core mod, a mod's
// new records with IDs from 10,000, and a mod's change to a core record.
func TestModCompanions(t *testing.T) {
	skills := coreSkills(t)
	set := companionSet(t, map[string]string{
		"data/companions.yml": "- {name: example.a:Owl, look: falcon, acts_as: falcon, requires: [{skill: HT_FALCON, level: 1}]}\n",
		"data/mounts.yml":     "- {name: example.a:Horse, look: peco_peco, acts_as: peco_peco, speed: 20}\n- {name: peco_peco, display_name: Pecopeco}\n",
	})
	c, err := loadCompanions(t, set, skills)
	if err != nil {
		t.Fatal(err)
	}
	falcon := c.Companions.ByName("falcon")
	if falcon == nil || falcon.ID != 1 || falcon.Rule != companion.RuleFalcon || falcon.Option != companion.OptionFalcon ||
		len(falcon.Requires) != 1 || falcon.Requires[0].Skill != 127 {
		t.Fatalf("falcon %+v", falcon)
	}
	owl := c.Companions.ByName("example.a:Owl")
	if owl == nil || owl.ID != 10000 || !owl.Is("falcon") || owl.Mod != "example.a" {
		t.Fatalf("owl %+v", owl)
	}
	horse := c.Mounts.ByName("example.a:Horse")
	if horse == nil || horse.Rule != companion.RuleRiding || !horse.HasSpeed || horse.Speed != 20 || len(horse.Requires) != 0 {
		t.Fatalf("horse %+v", horse)
	}
	if p := c.Mounts.ByName("peco_peco"); p.DisplayName != "Pecopeco" || p.ID != 1 {
		t.Errorf("peco peco %+v", p)
	}
	for name, rule := range map[string]companion.Rule{"grand_peco": companion.RuleRiding, "gryphon": companion.RuleRiding,
		"dragon": companion.RuleDragon, "warg": companion.RuleWugRider, "madogear": companion.RuleMadogear} {
		if r := c.Mounts.ByName(name); r == nil || r.Rule != rule {
			t.Errorf("%s: %+v", name, r)
		}
	}
	// Core's records parse the same without the skill database.
	for _, n := range c.Mounts.Names() {
		if r := companion.Core().Mounts.ByName(n); n != "example.a:Horse" && (r == nil || r.ID != c.Mounts.ByName(n).ID) {
			t.Errorf("companion.Core lacks mount %s", n)
		}
	}
}

func TestModCompanionErrors(t *testing.T) {
	skills := coreSkills(t)
	for _, c := range []struct{ file, data, want string }{
		{"data/companions.yml", "- {name: example.a:Owl, look: owl}\n", "look must be one of falcon, warg"},
		{"data/companions.yml", "- {name: example.a:Owl, look: falcon, acts_as: peco_peco}\n", "acts_as must be a core companion record: falcon"},
		{"data/companions.yml", "- {name: example.a:Owl, look: falcon, requires: [{skill: NO_SUCH}]}\n", `there is no skill "NO_SUCH"`},
		{"data/companions.yml", "- {name: example.a:Owl, look: falcon, id: 5}\n", "id is given by the server"},
		{"data/companions.yml", "- {name: falcon, id: 5}\n", "id cannot be changed"},
		{"data/mounts.yml", "- {name: example.a:Horse, look: peco_peco, speed: 80}\n", "speed must be a whole number from 0 to 50"},
		{"data/mounts.yml", "- {name: example.a:Horse, look: peco_peco, sped: 20}\n", `"sped" is not a field`},
		{"data/skills.yml", "- {name: MG_FIREBOLT, requirements: {companion: owl}}\n", `there is no companion "owl"`},
	} {
		set := companionSet(t, map[string]string{c.file: c.data})
		var db = skills
		if c.file == "data/skills.yml" {
			var err error
			db, _, err = loadModSkills(t, set)
			if err != nil {
				t.Fatal(err)
			}
		}
		_, err := loadCompanions(t, set, db)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.data, err, c.want)
		}
	}
}
