package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/rotation"
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

// TestOwnersHint: world new says the account is the owner by its ID,
// and needs no staff.owners line (review decision 54).
func TestOwnersHint(t *testing.T) {
	hint := ownersHint("Admin", 42)
	for _, want := range []string{"Admin (id 42) is the world's owner", "no staff.owners line", "PIN or fingerprint"} {
		if !strings.Contains(hint, want) {
			t.Errorf("%q lacks %q", hint, want)
		}
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
		if _, err := proceduralWorld("", "renewal", seed, 3); err == nil {
			t.Error("a layout without the generator")
		}
		return
	}
	pw, err := proceduralWorld("", "renewal", seed, g.Version)
	if err != nil {
		t.Fatal(err)
	}
	l, planned, w := pw.layout, pw.spawns, pw.content
	// The first dungeon rotates, and the game role gets its key and the
	// seasons' (decision 0045).
	root, _ := rng.NewRoot(seed)
	secret := spawns.SecretBranch(root)
	if len(pw.rotations) != 1 || len(pw.seasonKey) != 32 {
		t.Fatalf("rotations %+v, season key %x", pw.rotations, pw.seasonKey)
	}
	r := pw.rotations[0]
	first := slices.IndexFunc(l.Maps, func(m layout.Map) bool { return m.Kind == gen.KindDungeon })
	rk, sk := rotation.RotationKey(secret, r.Dungeon).Key(), rotation.SeasonKey(secret).Key()
	if r.Dungeon != l.Maps[first].Dungeon || r.Rule.Every == "" || !slices.Equal(r.Key, rk[:]) || !slices.Equal(pw.seasonKey, sk[:]) {
		t.Errorf("rotation %+v", r)
	}
	if text := rotationText(l, pw.rotations); !strings.Contains(text, r.Dungeon) || !strings.Contains(text, r.Rule.String()) {
		t.Errorf("text %q", text)
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
	again, err := proceduralWorld("", "renewal", seed, g.Version)
	if err != nil || !reflect.DeepEqual(again.spawns, planned) || !reflect.DeepEqual(again.content, w) ||
		!reflect.DeepEqual(again.rotations, pw.rotations) || !slices.Equal(again.seasonKey, pw.seasonKey) {
		t.Errorf("a second run differs: %v", err)
	}
	mods := t.TempDir()
	for _, name := range []string{"home-village", "mossy-fields", "wayside-shrines"} {
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
		"listen": "127.0.0.1:1", // other keys are left alone
		"mods": map[string]any{"dir": mods, "approve": map[string]any{"example.mossy-fields": []string{"spawns.modify"},
			"example.wayside-shrines": []string{"world.generate"}}},
		"procedural": map[string]any{"field_size": 64, "layout": map[string]any{"grid": []int{2, 2}, "towns": 2, "dungeons": 1}},
		"spawns":     map[string]any{"town_field_boss_chance": 100, "density": map[string]any{"field": 200}},
	})
	if err := os.WriteFile(cfg, b, 0o600); err != nil {
		t.Fatal(err)
	}
	pw, err = proceduralWorld(cfg, "renewal", seed, g.Version)
	if err != nil {
		t.Fatal(err)
	}
	l, planned, w = pw.layout, pw.spawns, pw.content
	home := l.Map("gen:home:village")
	if home == nil || l.Home != home.ID || home.Name != "Willow Village" || home.Record != "example.home-village:village" || len(l.Towns()) != 2 {
		t.Fatalf("with the mod: home %+v, %d towns", home, len(l.Towns()))
	}
	if f := l.Map("gen:field:1:1"); f == nil || f.Size != 64 {
		t.Errorf("field 1:1 %+v", f)
	}
	// The content: a quest board in each town, the guilds, the rebirth
	// place, and the Wayside Shrines layer's shrines, stones and wolves
	// (decision 0044). The wolves join the lists after the hooks.
	tags := map[string]int{}
	for _, p := range w.Places {
		tags[p.Tags[0]]++
	}
	if tags["quest_board"] != 2 || tags["shrine"] != 2 || tags["rebirth"] != 1 || tags["guild:thief"] != 1 || tags["stone_ring"] != 4 {
		t.Errorf("content places %v", tags)
	}
	if len(w.Objects) != 16 || len(w.Notes) != 0 {
		t.Errorf("%d objects, notes %v", len(w.Objects), w.Notes)
	}
	if !slices.ContainsFunc(w.Quests, func(q content.Quest) bool { return q.Template == "example.wayside-shrines:offerings" }) {
		t.Error("no offerings quest")
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
		wolf := slices.IndexFunc(p.Spawns, func(s spawns.Spawn) bool { return s.Monster == "VAGABOND_WOLF" && s.Role == spawns.MiniBoss })
		if want := m.Kind == gen.KindField; (wolf >= 0) != want || wolf >= 0 && wolf != len(p.Spawns)-1 {
			t.Errorf("%s: the wolf at %d of %+v", p.Map, wolf, p.Spawns)
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
		if _, err := proceduralWorld(cfg, "renewal", seed, g.Version); err == nil || !strings.Contains(err.Error(), "spawns.") {
			t.Errorf("%v: %v", bad, err)
		}
	}
	if _, err := proceduralWorld(filepath.Join(t.TempDir(), "none.yml"), "renewal", seed, g.Version); err == nil {
		t.Error("a config that does not exist")
	}
}
