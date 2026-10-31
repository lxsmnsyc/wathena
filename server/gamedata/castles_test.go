package gamedata

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/castle"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// castleSet makes a set of core, with core's castles converted from the
// mode's extract, and one mod with files.
func castleSet(t *testing.T, mode reference.Mode, files map[string]string) *mod.Set {
	t.Helper()
	data, err := reference.ReadFile("extracts/" + string(mode) + "/castles.json")
	if err != nil {
		t.Fatal(err)
	}
	res, err := convert.ConvertCastles(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	core := fstest.MapFS{}
	for p, s := range res.Files() {
		core[p] = &fstest.MapFile{Data: []byte(s)}
	}
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
	set, err := mod.Resolve(found, mod.Config{Core: core})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func loadCastles(t *testing.T, mode reference.Mode, files map[string]string) ([]*castle.Record, *mod.Overlay, error) {
	t.Helper()
	ids, err := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	if err != nil {
		t.Fatal(err)
	}
	return modCastles(context.Background(), castleSet(t, mode, files), ids, mode)
}

// TestCoreCastles loads core's 44 renewal and 34 pre-renewal castles.
func TestCoreCastles(t *testing.T) {
	for mode, want := range map[reference.Mode]int{reference.Renewal: 44, reference.PreRenewal: 34} {
		recs, _, err := loadCastles(t, mode, map[string]string{})
		if err != nil {
			t.Fatal(err)
		}
		if len(recs) != want {
			t.Errorf("%s: %d castles, want %d", mode, len(recs), want)
		}
		var k *castle.Record
		for _, r := range recs {
			if r.Name == "Kriemhild" {
				k = r
			}
		}
		if k == nil || k.ID != 15 || k.Map != "prtg_cas01" || k.Edition != "fe" || k.NPC != "Agit#prtg_cas01" ||
			k.DisplayName != "Kriemhild" || k.ClientID != 1 {
			t.Fatalf("%s: Kriemhild %+v", mode, k)
		}
		if mode == reference.Renewal && (k.Teleport == nil || *k.Teleport != castle.Teleport{X: 107, Y: 180, Cost: 100, SiegeCost: 100000}) {
			t.Errorf("renewal: Kriemhild's teleport %+v", k.Teleport)
		}
		if mode == reference.PreRenewal && k.Teleport != nil {
			t.Errorf("pre-renewal: Kriemhild has a teleport")
		}
	}
}

// TestModCastles checks a mod's new castle, a change to core's, and the
// fields a mod cannot change or that break the world's rules.
func TestModCastles(t *testing.T) {
	recs, o, err := loadCastles(t, reference.PreRenewal, map[string]string{"data/castles.yml": "" +
		"- {name: keep, map: keep_cas, edition: se, npc: example.a:steward, teleport: {x: 10, y: 12}}\n" +
		"- {name: twin, map: prtg_cas01, edition: fe, npc: example.a:twin}\n" +
		"- {name: trial, map: trial_cas, edition: te, npc: example.a:trial}\n" +
		"- {name: Kriemhild, npc: example.a:flag, display_name: Krimhilde}\n"})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*castle.Record{}
	for _, r := range recs {
		byName[r.Name] = r
	}
	if k := byName["example.a:keep"]; k == nil || k.ID != 10000 || k.DisplayName != "keep" ||
		*k.Teleport != (castle.Teleport{X: 10, Y: 12, Cost: 100, SiegeCost: 100000}) {
		t.Errorf("keep %+v", k)
	}
	if k := byName["Kriemhild"]; k.NPC != "example.a:flag" || k.DisplayName != "Krimhilde" || k.Map != "prtg_cas01" {
		t.Errorf("Kriemhild %+v", k)
	}
	for _, name := range []string{"example.a:twin", "example.a:trial"} {
		if byName[name] != nil {
			t.Errorf("%s was not left out", name)
		}
	}
	if notes := strings.Join(o.Notes, "\n"); !strings.Contains(notes, "edition te needs the renewal ruleset") ||
		!strings.Contains(notes, "prtg_cas01 already has the castle") {
		t.Errorf("notes: %s", notes)
	}

	for _, c := range []struct{ file, want string }{
		{"- {name: Kriemhild, map: prtg_cas02}\n", "map cannot be changed"},
		{"- {name: Kriemhild, id: 3}\n", "id cannot be changed"},
		{"- {name: Kriemhild, teleport: {x: 1, y: 1}}\n", "needs the permission data.core_override"},
		{"- {name: keep, map: keep_cas, edition: woe, npc: x}\n", "edition is fe, se or te"},
		{"- {name: keep, map: keep_cas, edition: fe, npc: x, teleport: {x: 1, y: 1, cost: -1}}\n", "teleport.cost is 0"},
		{"- {name: keep, map: keep_cas, edition: fe, npc: x, owner: 1}\n", `"owner" is not a field`},
	} {
		_, _, err := loadCastles(t, reference.Renewal, map[string]string{"data/castles.yml": c.file})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.file, err, c.want)
		}
	}
}
