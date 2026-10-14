package build

import (
	"encoding/json"
	"maps"
	"os"
	"strings"
	"testing"
)

func TestClosure(t *testing.T) {
	entries := map[string]entry{
		"map/a":     {H: "1", T: "map", D: "x"},
		"model/b":   {H: "2", T: "model", D: "y"},
		"texture/c": {H: "3", T: "texture"},
		"texture/d": {H: "4", T: "texture"},
	}
	refs := map[string][]string{
		"map/a":   {"model/b", "texture/c", "texture/gone", "map/a"},
		"model/b": {"texture/d", "texture/c"},
	}
	got := closure("map/a", refs, entries)
	// Names come through models, missing names and the asset itself are
	// left out, and entries have no index of their own.
	want := map[string]entry{
		"model/b":   {H: "2", T: "model"},
		"texture/c": {H: "3", T: "texture"},
		"texture/d": {H: "4", T: "texture"},
	}
	if !maps.Equal(got, want) {
		t.Errorf("closure %v", got)
	}
}

func TestIndexes(t *testing.T) {
	out := checkBuild(t)
	_, all := readManifest(t, out)
	m := all["map/tiny"]
	if m.D == "" || all["placeholder/sprite"].D != "" {
		t.Fatalf("map/tiny %+v, placeholder/sprite %+v", m, all["placeholder/sprite"])
	}
	root, err := os.OpenRoot(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	data, err := readJSON(root.FS(), m.D)
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]entry
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	if len(index) != 2 || index["placeholder/sprite"] != all["placeholder/sprite"] || index["placeholder/texture"] != all["placeholder/texture"] {
		t.Errorf("index %v", index)
	}

	// Check notices an index that no longer matches.
	if err := root.WriteFile(jsonPath(m.D), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := Check(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Broken) != 1 || !strings.HasPrefix(rep.Broken[0], "map/tiny: dependency index") {
		t.Errorf("broken %v", rep.Broken)
	}

	// gc keeps indexes.
	g, err := GC(out, true)
	if err != nil {
		t.Fatal(err)
	}
	if g.Files != 0 {
		t.Errorf("gc would remove %d files", g.Files)
	}
}
