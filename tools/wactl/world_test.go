package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

func TestSplitPassword(t *testing.T) {
	for _, c := range []struct{ in, db, password string }{
		{"postgres://wathena_migrator:s3cret@postgres:5432/wathena?sslmode=disable", "postgres://wathena_migrator@postgres:5432/wathena?sslmode=disable", "s3cret"},
		{"postgresql://me@host/db?password=p%40ss&sslmode=require", "postgresql://me@host/db?sslmode=require", "p@ss"},
		{"postgres://me@host/db", "postgres://me@host/db", ""},
	} {
		db, password, err := splitPassword(c.in)
		if err != nil || db != c.db || password != c.password {
			t.Errorf("%s: %q %q %v", c.in, db, password, err)
		}
	}
	for _, bad := range []string{"host=x password=y", "mysql://a:b@c/d", "postgres://host/db"} {
		if _, _, err := splitPassword(bad); err == nil || strings.Contains(err.Error(), "y") && strings.Contains(bad, "password=y") {
			t.Errorf("%s: %v", bad, err)
		}
	}
}

func TestReadPassword(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(f, []byte("correct horse\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if p, err := readPassword(f); err != nil || p != "correct horse" {
		t.Errorf("%q, %v", p, err)
	}
	if _, err := readPassword(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("a missing file was read")
	}
}

// TestOwnersHint: the lines that world new prints are YAML that names the
// account in staff.owners (decision 0036).
func TestOwnersHint(t *testing.T) {
	hint := ownersHint("Admin")
	_, block, ok := strings.Cut(hint, "\n")
	if !ok {
		t.Fatalf("no config lines: %q", hint)
	}
	var cfg struct {
		Staff struct {
			Owners []string `yaml:"owners"`
		} `yaml:"staff"`
	}
	if err := yaml.Unmarshal([]byte(block), &cfg); err != nil || len(cfg.Staff.Owners) != 1 || cfg.Staff.Owners[0] != "Admin" {
		t.Errorf("%q: %+v, %v", block, cfg, err)
	}
}

// TestWorldVersions: classic records no generator, procedural the
// newest, and a build without the procedural tag refuses procedural.
func TestWorldVersions(t *testing.T) {
	if g, r, err := worldVersions(gen.ModeClassic); err != nil || g != 0 || r != "" {
		t.Errorf("classic: %d %q %v", g, r, err)
	}
	if _, _, err := worldVersions("other"); err == nil {
		t.Error("an unknown mode was taken")
	}
	g, r, err := worldVersions(gen.ModeProcedural)
	if gen.Procedural {
		latest, _ := gen.Latest()
		if err != nil || g != latest.Version || r != rng.Version {
			t.Errorf("procedural: %d %q %v", g, r, err)
		}
		if gen.DefaultMode() != gen.ModeProcedural {
			t.Errorf("default mode %s", gen.DefaultMode())
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), "-tags procedural") {
		t.Errorf("procedural without the tag: %d %q %v", g, r, err)
	}
	if gen.DefaultMode() != gen.ModeClassic {
		t.Errorf("default mode %s", gen.DefaultMode())
	}
}

// TestPublicNode: the world row's public node is the public branch of
// the seed's root, which yields every field's node, and differs from the
// root (decision 0039).
func TestPublicNode(t *testing.T) {
	seed := []byte("a world seed of thirty-two bytes")
	got, err := publicNode(seed)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := rng.NewRoot(seed)
	want := gen.PublicBranch(root).Key()
	rootKey := root.Key()
	if len(got) != 32 || string(got) != string(want[:]) || string(got) == string(rootKey[:]) {
		t.Errorf("public node %x, want %x", got, want)
	}
	if _, err := publicNode(nil); err == nil {
		t.Error("an empty seed gave a public node")
	}
}

// TestProceduralWorld: a procedural world's layout and spawns come from
// core's records, or from the mods, procedural.layout and spawns of
// --config, with a mod's home map, naming style, spawn tags and
// spawn_table hook (decisions 0041 and 0042).
func TestProceduralWorld(t *testing.T) {
	seed := []byte("wactl-procedural-world-seed-0001")
	g, ok := gen.Latest()
	if !ok {
		if _, _, err := proceduralWorld("", "renewal", seed, 3); err == nil {
			t.Error("a layout without the generator")
		}
		return
	}
	l, planned, err := proceduralWorld("", "renewal", seed, g.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Towns()) != 5 || l.Home != "gen:town:t1" || len(l.Maps) < 30+5+6 {
		t.Errorf("core's layout: %d towns, home %s, %d maps", len(l.Towns()), l.Home, len(l.Maps))
	}
	if !strings.Contains(layoutText(l), "towns 5,") || !strings.Contains(spawnText(planned), "bosses") {
		t.Errorf("text %q %q", layoutText(l), spawnText(planned))
	}
	want := 0
	for _, m := range l.Maps {
		if m.Kind == gen.KindField || m.Kind == gen.KindDungeon {
			want++
		}
	}
	if len(planned) != want {
		t.Fatalf("%d spawn lists for %d fields and floors", len(planned), want)
	}
	for _, p := range planned {
		if len(p.Spawns) < spawns.MinSpecies {
			t.Errorf("%s: %+v", p.Map, p.Spawns)
		}
	}
	// The same seed gives the same spawns.
	_, again, err := proceduralWorld("", "renewal", seed, g.Version)
	if err != nil || !reflect.DeepEqual(again, planned) {
		t.Errorf("a second run differs: %v", err)
	}
	mods := t.TempDir()
	for _, name := range []string{"home-village", "mossy-fields"} {
		src, err := filepath.Abs(filepath.Join("..", "..", "mods", "examples", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(src, filepath.Join(mods, name)); err != nil {
			t.Fatal(err)
		}
	}
	cfg := filepath.Join(t.TempDir(), "waserver.yml")
	b, _ := yaml.Marshal(map[string]any{
		"listen":     "127.0.0.1:1", // other keys are left alone
		"mods":       map[string]any{"dir": mods, "approve": map[string]any{"example.mossy-fields": []string{"spawns.modify"}}},
		"procedural": map[string]any{"field_size": 64, "layout": map[string]any{"grid": []int{2, 2}, "towns": 2, "dungeons": 1}},
		"spawns":     map[string]any{"town_field_boss_chance": 100, "density": map[string]any{"field": 200}},
	})
	if err := os.WriteFile(cfg, b, 0o600); err != nil {
		t.Fatal(err)
	}
	l, planned, err = proceduralWorld(cfg, "renewal", seed, g.Version)
	if err != nil {
		t.Fatal(err)
	}
	home := l.Map("gen:home:village")
	if home == nil || l.Home != home.ID || home.Name != "Willow Village" || home.Record != "example.home-village:village" || len(l.Towns()) != 2 {
		t.Fatalf("with the mod: home %+v, %d towns", home, len(l.Towns()))
	}
	if f := l.Map("gen:field:1:1"); f == nil || f.Size != 64 {
		t.Errorf("field 1:1 %+v", f)
	}
	// The hook puts 12 Moss Porings on each field below level 20.
	for _, p := range planned {
		m := l.Map(p.Map)
		mossy := slices.ContainsFunc(p.Spawns, func(s spawns.Spawn) bool {
			return s.Monster == "example.mossy-fields:MOSS_PORING" && s.Count == 12
		})
		if want := m.Kind == gen.KindField && m.Levels[0] < 20; mossy != want {
			t.Errorf("%s at %v: Moss Porings %v, want %v: %+v", p.Map, m.Levels, mossy, want, p.Spawns)
		}
	}
	for _, bad := range []map[string]any{
		{"spawns": map[string]any{"town_field_boss_chance": 101}},
		{"spawns": map[string]any{"town_field_boss_role": "rare"}},
		{"spawns": map[string]any{"density": map[string]any{"dungeon": -1}}},
	} {
		b, _ := yaml.Marshal(bad)
		if err := os.WriteFile(cfg, b, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := proceduralWorld(cfg, "renewal", seed, g.Version); err == nil || !strings.Contains(err.Error(), "spawns.") {
			t.Errorf("%v: %v", bad, err)
		}
	}
	if _, _, err := proceduralWorld(filepath.Join(t.TempDir(), "none.yml"), "renewal", seed, g.Version); err == nil {
		t.Error("a config that does not exist")
	}
}
