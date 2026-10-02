package mod

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/item"
)

// writeMod writes a mod folder: mod.yaml and other files, by path.
func writeMod(t *testing.T, dir, name string, files map[string]string) {
	t.Helper()
	for p, text := range files {
		full := filepath.Join(dir, name, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// manifest returns a valid mod.yaml with extra lines.
func manifest(id string, extra ...string) string {
	return "id: " + id + "\nname: Test " + id + "\nversion: 1.0.0\napi: experimental\n" + strings.Join(extra, "\n") + "\n"
}

func resolve(t *testing.T, dir string, cfg Config) (*Set, error) {
	t.Helper()
	found, err := Discover(dir)
	if err != nil {
		return nil, err
	}
	return Resolve(found, cfg)
}

func ids(s *Set) []string {
	var out []string
	for _, m := range s.Mods {
		out = append(out, m.ID())
	}
	return out
}

func wantError(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want one with %q", parts)
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Fatalf("error %q does not contain %q", err, p)
		}
	}
}

func TestManifestValidation(t *testing.T) {
	cases := []struct {
		text  string
		parts []string
	}{
		{"", []string{"m/mod.yaml", "empty"}},
		{"id: Bad_ID\nname: x\nversion: 1.0.0\napi: experimental\n", []string{"m/mod.yaml: id:", "not a mod ID"}},
		{manifest("core.extra"), []string{"reserved"}},
		{"id: a\nname: x\nversion: 1.0\napi: experimental\n", []string{"version:", "not a semantic version"}},
		{"id: a\nname: x\nversion: 1.0.0\n", []string{"api: is required"}},
		{manifest("a", "depends:\n  - id: b\n    version: '>=x'"), []string{"depends[0].version"}},
		{manifest("a", "depends:\n  - id: a"), []string{"depends[0].id: a mod cannot depend on itself"}},
		{manifest("a", "permissions: [player.fly]"), []string{"permissions[0]", `"player.fly" is not a permission`}},
		{manifest("a", "supports:\n  rulesets:\n    base: [classic]"), []string{"supports.rulesets.base[0]"}},
		{manifest("a", "supports:\n  rulesets:\n    flags:\n      cast_time: fast"), []string{"cast_time", `"fast" is not one of renewal, pre_renewal`}},
		{manifest("a", "provides:\n  ruleset:\n    extends: core.ruleset.renewal\n    flags:\n      jump: on"), []string{`"jump" is not a ruleset flag`}},
		{manifest("a", "scripts: [main.lua]"), []string{"scripts[0]", "under scripts/"}},
		{manifest("a", "colour: red"), []string{"m/mod.yaml", "colour"}},
	}
	for _, c := range cases {
		_, err := ParseManifest("m/mod.yaml", []byte(c.text))
		wantError(t, err, c.parts...)
	}
	m, err := ParseManifest("m/mod.yaml", []byte(manifest("example.halloween-event",
		"authors:\n  - name: A",
		"depends:\n  - id: core\n    version: '>=0.1 <1.0'",
		"load_after: [example.seasonal-lib]",
		"supports:\n  rulesets:\n    base: [renewal, pre_renewal]\n    flags:\n      skill_cooldown: on",
		"permissions: [player.give_items, storage.mod]",
		"scripts: [scripts/main.lua]",
	)))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Supports.Rulesets.Flags["skill_cooldown"]; !slices.Equal(got, []string{"on"}) {
		t.Errorf("YAML's on was read as %q", got)
	}
	if m.SemVer().String() != "1.0.0" {
		t.Errorf("version %s", m.SemVer())
	}
}

func TestSemver(t *testing.T) {
	cases := []struct {
		r, v string
		ok   bool
	}{
		{">=1.0 <2.0", "1.5.0", true},
		{">=1.0 <2.0", "2.0.0", false},
		{">=1.0 <2.0", "2.0.0-beta", true},
		{"^1.2", "1.9.0", true},
		{"^1.2", "2.0.0", false},
		{"^0.2", "0.3.0", false},
		{"~1.2", "1.2.9", true},
		{"~1.2", "1.3.0", false},
		{"1.2.3", "1.2.3", true},
		{"<1.0 || >=3", "3.1.0", true},
		{"<1.0 || >=3", "2.0.0", false},
	}
	for _, c := range cases {
		r, err := ParseRange(c.r)
		if err != nil {
			t.Fatal(err)
		}
		v, err := ParseVersion(c.v)
		if err != nil {
			t.Fatal(err)
		}
		if r.Match(v) != c.ok {
			t.Errorf("%s matches %s: %v, want %v", c.r, c.v, !c.ok, c.ok)
		}
	}
	if a, b := mustVersion(t, "1.0.0-alpha.2"), mustVersion(t, "1.0.0-alpha.10"); a.Compare(b) >= 0 {
		t.Error("alpha.2 is not before alpha.10")
	}
}

func mustVersion(t *testing.T, s string) Version {
	t.Helper()
	v, err := ParseVersion(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestLoadOrder(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "z", map[string]string{"mod.yaml": manifest("example.z")})
	writeMod(t, dir, "b", map[string]string{"mod.yaml": manifest("example.b", "depends:\n  - id: example.z")})
	writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a", "load_after: [example.b, example.missing]")})
	writeMod(t, dir, "c", map[string]string{"mod.yaml": manifest("example.c")})
	writeMod(t, dir, "not-a-mod", map[string]string{"README.md": "no manifest"})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"core", "core.ruleset.renewal", "example.c", "example.z", "example.b", "example.a"}
	if got := ids(s); !slices.Equal(got, want) {
		t.Errorf("order %v, want %v", got, want)
	}
	// The same mods give the same order every time.
	for range 5 {
		s2, _ := resolve(t, dir, Config{})
		if !slices.Equal(ids(s2), want) {
			t.Fatalf("order changed: %v", ids(s2))
		}
	}
}

func TestDependencyErrors(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a", "depends:\n  - id: example.b\n    version: '>=2.0'")})
	writeMod(t, dir, "b", map[string]string{"mod.yaml": manifest("example.b")})
	writeMod(t, dir, "c", map[string]string{"mod.yaml": manifest("example.c", "depends:\n  - id: example.gone")})
	_, err := resolve(t, dir, Config{})
	wantError(t, err, "example.a depends on example.b >=2.0, and version 1.0.0 is installed",
		"example.c depends on example.gone", "not installed")

	_, err = resolve(t, dir, Config{Disable: []string{"example.b"}})
	wantError(t, err, "example.a depends on example.b, which is disabled in mods.disable")
}

func TestCycle(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a", "depends:\n  - id: example.b")})
	writeMod(t, dir, "b", map[string]string{"mod.yaml": manifest("example.b", "load_after: [example.c]")})
	writeMod(t, dir, "c", map[string]string{"mod.yaml": manifest("example.c", "depends:\n  - id: example.a")})
	writeMod(t, dir, "d", map[string]string{"mod.yaml": manifest("example.d")})
	_, err := resolve(t, dir, Config{})
	wantError(t, err, "load order cycle: example.a -> example.b -> example.c -> example.a")
}

func TestDuplicateID(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "one", map[string]string{"mod.yaml": manifest("example.a")})
	writeMod(t, dir, "two", map[string]string{"mod.yaml": manifest("example.a")})
	_, err := resolve(t, dir, Config{})
	wantError(t, err, "example.a is installed twice")
}

func TestZipMod(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "packed.zip"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, text := range map[string]string{
		"packed/mod.yaml":         manifest("example.packed", "scripts: [scripts/main.lua]"),
		"packed/scripts/main.lua": "return function() end",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(text))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := s.Scripts("example.packed")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Files["scripts/main.lua"] != "return function() end" {
		t.Errorf("files %v", sc.Files)
	}
}

func TestRulesets(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "classic", map[string]string{"mod.yaml": manifest("example.classic-plus",
		"provides:\n  ruleset:\n    extends: core.ruleset.pre_renewal\n    flags:\n      cast_time: renewal\n      level_penalty_drop: on")})
	writeMod(t, dir, "deeper", map[string]string{"mod.yaml": manifest("example.deeper",
		"provides:\n  ruleset:\n    extends: example.classic-plus\n    flags:\n      skill_cooldown: on")})
	writeMod(t, dir, "re-only", map[string]string{"mod.yaml": manifest("example.re-only", "supports:\n  rulesets:\n    base: [renewal]")})
	writeMod(t, dir, "needs-cd", map[string]string{"mod.yaml": manifest("example.needs-cd", "supports:\n  rulesets:\n    flags:\n      skill_cooldown: on")})

	// The default is core's renewal ruleset.
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Ruleset.Mod != CoreRenewal || s.Ruleset.Base != Renewal || s.Ruleset.Flag("skill_cooldown") != "on" || s.Ruleset.Flag("item_rolls") != "off" {
		t.Errorf("default ruleset %s", s.Ruleset)
	}
	if !slices.Contains(s.Notes, "ruleset mod example.classic-plus is installed but not used: the world's ruleset is core.ruleset.renewal") {
		t.Errorf("notes %q", s.Notes)
	}

	// A ruleset mod on a ruleset mod: flags apply from the base up. A mod
	// that supports only renewal is refused.
	_, err = resolve(t, dir, Config{Ruleset: "example.deeper"})
	wantError(t, err, "example.re-only is refused on the ruleset example.deeper: it supports the bases renewal, and the world's base is pre_renewal")

	_, err = resolve(t, dir, Config{Ruleset: "example.deeper", Disable: []string{"example.re-only"}})
	if err != nil {
		t.Fatal(err)
	}
	s, _ = resolve(t, dir, Config{Ruleset: "example.deeper", Disable: []string{"example.re-only"}})
	r := s.Ruleset
	if r.Base != PreRenewal || r.Flag("cast_time") != Renewal || r.Flag("level_penalty_drop") != "on" || r.Flag("skill_cooldown") != "on" || r.Flag("aspd") != PreRenewal {
		t.Errorf("ruleset %s", r)
	}
	if got := ids(s); !slices.Contains(got, "example.classic-plus") || slices.Index(got, "example.classic-plus") > slices.Index(got, "example.deeper") {
		t.Errorf("order %v", got)
	}

	// A flag the mod needs that the ruleset lacks.
	_, err = resolve(t, dir, Config{Ruleset: "example.classic-plus", Disable: []string{"example.re-only"}})
	wantError(t, err, "example.needs-cd is refused on the ruleset example.classic-plus: it needs skill_cooldown to be on, and the world's ruleset has off")

	// The world's base wins over a ruleset with another base.
	_, err = resolve(t, dir, Config{Ruleset: "example.classic-plus", Base: Renewal})
	wantError(t, err, "has the pre_renewal base, and the world's base is renewal")

	// A missing ruleset.
	_, err = resolve(t, dir, Config{Ruleset: "example.nothing"})
	wantError(t, err, "the ruleset example.nothing is not installed")
}

func TestFlagDependencies(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "fourth", map[string]string{"mod.yaml": manifest("example.fourth",
		"provides:\n  ruleset:\n    extends: core.ruleset.pre_renewal\n    flags:\n      fourth_job_traits: on")})
	_, err := resolve(t, dir, Config{Ruleset: "example.fourth"})
	wantError(t, err, "fourth_job_traits: on needs the renewal base")
}

func TestPermissions(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a", "permissions: [player.give_items, rules.override, event.listen]")})
	_, err := resolve(t, dir, Config{})
	wantError(t, err, "example.a requests permissions the server owner has not approved: player.give_items, rules.override", "mods.approve.example.a")

	s, err := resolve(t, dir, Config{Approve: map[string][]string{"example.a": {"player.give_items", "rules.override", "player.warp"}}})
	if err != nil {
		t.Fatal(err)
	}
	g := s.Mod("example.a").Grants
	if !g.Has("player.give_items") || !g.Has("rules.override") || !g.Has("event.listen") || !g.Has("game.read") || g.Has("player.warp") {
		t.Errorf("grants %v", g.List())
	}
	notes := strings.Join(s.Notes, "\n")
	if !strings.Contains(notes, "flagged permission rules.override") || !strings.Contains(notes, "approves player.warp, which the mod does not request") {
		t.Errorf("notes %s", notes)
	}
}

// TestWorldAnnouncePermission checks that world.announce needs the
// owner's approval, and that the built-in mods hold it.
func TestWorldAnnouncePermission(t *testing.T) {
	if p, ok := LookupPermission("world.announce"); !ok || !p.Approval || p.Flagged {
		t.Fatalf("world.announce %+v %v", p, ok)
	}
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a", "permissions: [world.announce]")})
	_, err := resolve(t, dir, Config{})
	wantError(t, err, "example.a requests permissions the server owner has not approved: world.announce")
	s, err := resolve(t, dir, Config{Approve: map[string][]string{"example.a": {"world.announce"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Mod("example.a").Grants.Has("world.announce") || !s.Mod("core").Grants.Has("world.announce") {
		t.Error("world.announce not granted")
	}
}

func TestAPIRange(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{"mod.yaml": "id: example.a\nname: A\nversion: 1.0.0\napi: '>=1.0 <2.0'\n"})
	_, err := resolve(t, dir, Config{})
	wantError(t, err, `api ">=1.0 <2.0" does not include this server's API, which is experimental`)
}

// testNames resolve the names of static bonuses in tests.
var testNames = &bonus.Tables{Constants: map[string]map[string]int64{"races": {"demi_human": 7}}}

var baseItems = &item.Data{Mode: "renewal", Items: []item.Def{
	{ID: 501, AegisName: "Red_Potion", Type: "Healing", Weight: 70, Stackable: true},
	{ID: 909, AegisName: "Jellopy", Type: "Etc", Weight: 10, Stackable: true},
}}

func TestItemOverlay(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml": manifest("example.a"),
		"data/items.yml": `
- name: example.a:Cookie
  type: Healing
  weight: 5
  script: scripts/items/cookie.lua
- name: Red_Potion
  weight: 50
- name: Jellopy
  bonuses: [[luk, 2], [damage, 10, {kind: physical, against: {race: demi_human}}]]
`,
	})
	writeMod(t, dir, "b", map[string]string{
		"mod.yaml": manifest("example.b", "load_after: [example.a]"),
		"data/items.yml": `
- name: Red_Potion
  weight: 60
- name: example.a:Cookie
  stackable: true
- name: example.b:Candy
`,
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	store := &MemoryIDs{}
	ids, _ := LoadIDs(context.Background(), store)
	got, err := s.Items(context.Background(), baseItems, ids, testNames)
	if err != nil {
		t.Fatal(err)
	}
	table := item.New(got.Data)
	if jellopy := table.Get(909); len(jellopy.Bonuses) != 2 {
		t.Errorf("Jellopy bonuses %v", jellopy.Bonuses)
	}
	if red := table.Get(501); red.Weight != 60 || red.Type != "Healing" || !red.Stackable {
		t.Errorf("Red_Potion %+v", red)
	}
	cookie := table.Get(got.IDs["example.a:Cookie"])
	if cookie.ID != 2_000_000 || cookie.Weight != 5 || !cookie.Stackable || cookie.Script != "example.a:scripts/items/cookie.lua" {
		t.Errorf("cookie %+v", cookie)
	}
	if candy := table.Get(got.IDs["example.b:Candy"]); candy.ID != 2_000_001 || candy.Type != "Etc" {
		t.Errorf("candy %+v", candy)
	}
	if len(baseItems.Items) != 2 || baseItems.Items[0].Weight != 70 {
		t.Error("the base items changed")
	}
	if notes := strings.Join(got.Overlay.Notes, "\n"); !strings.Contains(notes, "items Red_Potion: example.b sets weight, which example.a set before; example.b wins") {
		t.Errorf("notes %q", notes)
	}
	// Strict mode makes the same overlay an error.
	s.Strict = true
	_, err = s.Items(context.Background(), baseItems, ids, testNames)
	wantError(t, err, "b/data/items.yml: line 3", "strict mode makes this an error")
}

func TestItemOverlayErrors(t *testing.T) {
	cases := map[string][]string{
		"- name: Red_Potoin\n  weight: 1":                                     {`Red_Potoin is not a core record of items; a new record is named with the mod's ID, such as example.a:Red_Potoin`},
		"- name: example.z:Thing":                                             {"example.z has not added this record"},
		"- name: Red_Potion\n  id: 5":                                         {"id cannot be changed on a record the mod did not add"},
		"- name: Red_Potion\n  colour: red":                                   {`"colour" is not a field`},
		"- name: Red_Potion\n  weight: heavy":                                 {"weight: want int, got string"},
		"- name: Red_Potion\n  weight: -1":                                    {"cannot be negative"},
		"- name: example.a:X\n  type: Magic":                                  {`type "Magic" is not one of`},
		"- name: Jellopy\n- name: Jellopy":                                    {"Jellopy is listed twice"},
		"- weight: 1":                                                         {"the record has no name"},
		"name: Red_Potion":                                                    {"want a list of records"},
		"- name: Red_Potion\n  weight: 1\n  weight: 2":                        {"weight is set twice"},
		"- name: example.a:X\n  script: main.lua":                             {`script: "main.lua" is not a .lua file under scripts/`},
		"- name: Jellopy\n  bonuses: [[lucky, 1]]":                            {`there is no bonus "lucky"`},
		"- name: Jellopy\n  bonuses: [[luk, 1, {when: x}]]":                   {`when is a function, so it only works in Lua`},
		"- name: Jellopy\n  bonuses: [[luk, 1, {percent: true}]]":             {`takes no percent option`},
		"- name: Jellopy\n  bonuses: [[damage, 5, {against: {race: elves}}]]": {`there is no race "elves"`},
	}
	for text, parts := range cases {
		dir := t.TempDir()
		writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a"), "data/items.yml": text})
		s, err := resolve(t, dir, Config{})
		if err != nil {
			t.Fatal(err)
		}
		ids, _ := LoadIDs(context.Background(), &MemoryIDs{})
		_, err = s.Items(context.Background(), baseItems, ids, testNames)
		wantError(t, err, append(parts, "a/data/items.yml")...)
	}
}

func TestIDsStableAndNeverReused(t *testing.T) {
	store := &MemoryIDs{}
	ctx := context.Background()
	assign := func(names ...string) map[string]uint32 {
		ids, err := LoadIDs(ctx, store)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]uint32{}
		for _, n := range names {
			id, err := ids.ID(ctx, ItemIDs, n, "example.a")
			if err != nil {
				t.Fatal(err)
			}
			out[n] = id
		}
		return out
	}
	first := assign("example.a:A", "example.a:B")
	// A later start in another order, with A's mod removed, keeps B's ID,
	// and gives C a new one, never A's.
	second := assign("example.a:C", "example.a:B")
	if first["example.a:A"] != 2_000_000 || first["example.a:B"] != 2_000_001 {
		t.Errorf("first %v", first)
	}
	if second["example.a:B"] != 2_000_001 || second["example.a:C"] != 2_000_002 {
		t.Errorf("second %v", second)
	}
	// A comes back with its old ID.
	if third := assign("example.a:A"); third["example.a:A"] != 2_000_000 {
		t.Errorf("third %v", third)
	}
	// Kinds have their own ranges.
	ids, _ := LoadIDs(ctx, store)
	if id, _ := ids.ID(ctx, MonsterIDs, "example.a:A", "example.a"); id != 100_000 {
		t.Errorf("monster ID %d", id)
	}
}

func TestNPCs(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml":        manifest("example.a"),
		"scripts/npc.lua": "return function() end",
		"data/npcs.yml": `
- name: Healer#1
  map: prontera
  x: 150
  y: 180
  direction: west
  sprite: 4_F_KAFRA1
  script: scripts/npc.lua
- name: Guide
  map: map/prt_fild08
  x: 10
  y: 11
  sprite: "112"
  display_name: Field Guide
`,
	})
	writeMod(t, dir, "b", map[string]string{
		"mod.yaml":      manifest("example.b", "depends:\n  - id: example.a"),
		"data/npcs.yml": "- name: example.a:Guide\n  x: 12\n",
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	sprites := func(name string) (uint32, bool) {
		return map[string]uint32{"4_F_KAFRA1": 112}[name], name == "4_F_KAFRA1"
	}
	npcs, _, err := s.NPCs(sprites)
	if err != nil {
		t.Fatal(err)
	}
	want := []NPC{
		{Name: "example.a:Healer#1", Mod: "example.a", DisplayName: "Healer", Map: "map/prontera", X: 150, Y: 180, Direction: 2, Sprite: 112, Script: "scripts/npc.lua", ScriptMod: "example.a", Placed: true},
		{Name: "example.a:Guide", Mod: "example.a", DisplayName: "Field Guide", Map: "map/prt_fild08", X: 12, Y: 11, Sprite: 112, Placed: true},
	}
	if !reflect.DeepEqual(npcs, want) {
		t.Errorf("npcs\n%+v\nwant\n%+v", npcs, want)
	}

	writeMod(t, dir, "c", map[string]string{
		"mod.yaml":      manifest("example.c"),
		"data/npcs.yml": "- name: Broken\n  map: prontera\n  sprite: NOT_A_SPRITE\n  direction: up\n  script: scripts/none.lua\n",
	})
	s, err = resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = s.NPCs(sprites)
	wantError(t, err, "c/data/npcs.yml: line 1: npcs example.c:Broken", "x and y are required", `sprite "NOT_A_SPRITE"`, "direction is one of", "example.c has no scripts/none.lua")
}

func TestScriptsAndModulePaths(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml":                manifest("example.a", "scripts: [scripts/main.lua]"),
		"scripts/main.lua":        "return function() end",
		"scripts/lib/rewards.lua": "return {}",
		"scripts/notes.txt":       "not lua",
	})
	writeMod(t, dir, "b", map[string]string{"mod.yaml": manifest("example.b", "depends:\n  - id: example.a", "scripts: [scripts/missing.lua]")})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	sc, err := s.Scripts("example.a")
	if err != nil {
		t.Fatal(err)
	}
	if len(sc.Files) != 2 || sc.Files["scripts/lib/rewards.lua"] != "return {}" || !slices.Equal(sc.Visible, []string{"example.a"}) {
		t.Errorf("scripts %+v", sc)
	}
	_, err = s.Scripts("example.b")
	wantError(t, err, "scripts lists scripts/missing.lua, which does not exist")
	if !slices.Equal(s.Visible("example.b"), []string{"example.b", "example.a"}) {
		t.Errorf("visible %v", s.Visible("example.b"))
	}
	for rest, want := range map[string]string{"lib.rewards": "scripts/lib/rewards.lua", "main": "scripts/main.lua", "": "", "lib..x": "", "a/b": ""} {
		got, ok := ModulePath(rest)
		if got != want || ok != (want != "") {
			t.Errorf("ModulePath(%q) = %q, %v", rest, got, ok)
		}
	}
}

// TestSampleMod loads mods/examples, which the docs point mod authors to.
func TestSampleMod(t *testing.T) {
	s, err := resolve(t, filepath.Join("..", "..", "mods", "examples"), Config{Approve: map[string][]string{
		"example.halloween": {"player.give_items", "world.announce", "events.schedule", "rules.override"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	ids, _ := LoadIDs(context.Background(), &MemoryIDs{})
	items, err := s.Items(context.Background(), baseItems, ids, testNames)
	if err != nil {
		t.Fatal(err)
	}
	if items.IDs["example.greeter:Welcome_Cookie"] != 2_000_000 {
		t.Errorf("cookie ID %d", items.IDs["example.greeter:Welcome_Cookie"])
	}
	if items.IDs["example.halloween:Pumpkin_Hat"] != 2_000_001 {
		t.Errorf("hat ID %d", items.IDs["example.halloween:Pumpkin_Hat"])
	}
	npcs, _, err := s.NPCs(func(string) (uint32, bool) { return 112, true })
	if err != nil || len(npcs) != 4 || npcs[0].DisplayName != "Greeter" || npcs[0].Script != "scripts/npc/greeter.lua" {
		t.Errorf("npcs %+v, %v", npcs, err)
	}
	// The cookie stand is a shop that only scripts open.
	if shop := npcs[2]; shop.Placed || len(shop.Shop) != 2 || !shop.Discount || shop.Shop[1].Price != -1 {
		t.Errorf("shop %+v", shop)
	}
	sc, err := s.Scripts("example.greeter")
	if err != nil || len(sc.Entries) != 1 || sc.Texts["greeter.hello"] == "" {
		t.Errorf("scripts %+v, %v", sc, err)
	}
	monsters, err := s.Monsters(func(name string) bool { return name == "PORING" || name == "JAKK" })
	if err != nil || len(monsters.Records) != 2 || monsters.Records[0].Name != "example.halloween:PUMPKIN_PORING" {
		t.Errorf("monsters %+v, %v", monsters, err)
	}
	events, err := s.Events()
	if err != nil || len(events.Records) != 2 || events.Records[0].Name != "example.halloween:Pumpkin_King" {
		t.Errorf("events %+v, %v", events, err)
	}
}

func TestEventsNeedPermission(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml":        manifest("example.a"),
		"data/events.yml": "- name: Boss\n  trigger: {daily: \"20:00\"}\n  duration_minutes: 60\n  script: scripts/boss.lua\n",
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Events()
	wantError(t, err, "events example.a:Boss", "example.a needs the permission events.schedule")
	writeMod(t, dir, "a", map[string]string{"mod.yaml": manifest("example.a", "permissions: [events.schedule]")})
	if s, err = resolve(t, dir, Config{Approve: map[string][]string{"example.a": {"events.schedule"}}}); err != nil {
		t.Fatal(err)
	}
	o, err := s.Events()
	if err != nil || len(o.Records) != 1 || o.Records[0].Name != "example.a:Boss" {
		t.Fatalf("events %+v, %v", o, err)
	}
	if script, err := ScriptOf(o.Records[0]); err != nil || script != "example.a:scripts/boss.lua" {
		t.Fatalf("script %q, %v", script, err)
	}
}

// Build metadata must use semver's characters, since tools put versions
// in file names.
func TestVersionBuildMetadata(t *testing.T) {
	for _, v := range []string{"1.0.0+build.7", "1.0.0-rc.1+sha-abc"} {
		if _, err := ParseVersion(v); err != nil {
			t.Errorf("%s: %v", v, err)
		}
	}
	for _, v := range []string{"1.0.0+/../../x", "1.0.0+a/b", "1.0.0+", "1.0.0+a..b"} {
		if _, err := ParseVersion(v); err == nil {
			t.Errorf("%s is accepted", v)
		}
	}
}
