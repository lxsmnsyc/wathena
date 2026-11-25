package gamedata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/navi"
)

func naviSet(t *testing.T, file string) *mod.Set {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{"mod.yaml": "id: example.a\nname: A\nversion: 1.0.0\napi: 1.0\n"}
	if file != "" {
		files["data/navi.yml"] = file
	}
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
	set, err := mod.Resolve(found, mod.Config{Core: fstest.MapFS{}})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// TestModNavi reads a mod's extra edges, and checks that core has none.
func TestModNavi(t *testing.T) {
	edges, _, err := modNavi(naviSet(t, ""))
	if err != nil || len(edges) != 0 {
		t.Fatalf("no file: %v %v", edges, err)
	}
	edges, _, err = modNavi(naviSet(t, "- name: airship_up\n  from: izlude\n  to: airplane\n  x: 10\n  y: 20\n  to_x: 5\n  to_y: 6\n  service: airship\n"))
	if err != nil || len(edges) != 1 {
		t.Fatalf("%v %v", edges, err)
	}
	if e := edges[0]; e.From != "map/izlude" || e.To != "map/airplane" || e.Service != navi.Airship || e.ToY != 6 {
		t.Errorf("%+v", e)
	}
	_, _, err = modNavi(naviSet(t, "- name: bad\n  from: a\n  to: b\n  x: 1\n  y: 1\n  to_x: 1\n  to_y: 1\n  service: boat\n"))
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Errorf("a bad service: %v", err)
	}
}
