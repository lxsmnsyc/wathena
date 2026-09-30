package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
)

// checkBuild builds the placeholder sprite and a 1 × 1 map whose
// dependencies are one present and one missing name.
func checkBuild(t *testing.T) string {
	t.Helper()
	m := &wmap.Map{
		Width: 1, Height: 1,
		Cells:  []wmap.Cell{{Flags: wmap.Walkable}},
		Ground: wmap.Ground{Lightmap: "lightmap/tiny"},
		Deps:   []string{"placeholder/sprite", "placeholder/texture", "texture/gone"},
	}
	out := t.TempDir()
	_, err := Run(Options{Out: out, Brotli: 1, Sources: []Source{Placeholder{}, fakeSource{"maps", []Item{{
		Name: "map/tiny", Ext: "wmap", CacheKey: "tiny",
		Produce: func() ([]byte, error) { return m.Encode() },
	}}}}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCheck(t *testing.T) {
	out := checkBuild(t)
	rep, err := Check(out, []Reference{{From: "jobs.yml", Name: "sprite/body/novice/m"}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Assets != 3 || rep.Files != 3 || len(rep.Broken) != 0 {
		t.Fatalf("report %+v", rep)
	}
	if len(rep.Missing) != 2 || rep.Missing["texture/gone"][0] != "map/tiny" || rep.Missing["sprite/body/novice/m"][0] != "jobs.yml" {
		t.Errorf("missing %v", rep.Missing)
	}

	// A changed file no longer matches its hash.
	_, all := readManifest(t, out)
	e := all["placeholder/sprite"]
	p := filepath.Join(out, StoredPath(e.H, "wspr"))
	if err := os.WriteFile(p, []byte("not brotli"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err = Check(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Broken) != 1 || !strings.HasPrefix(rep.Broken[0], "placeholder/sprite:") {
		t.Errorf("broken %v", rep.Broken)
	}
}

func TestGC(t *testing.T) {
	out := checkBuild(t)
	orphan := filepath.Join(out, "ab", strings.Repeat("ab", 32)+".wspr.br")
	tmp := filepath.Join(out, "cd", strings.Repeat("cd", 32)+".webp.tmp0123456789abcdef")
	notes := filepath.Join(out, "ab", "notes.txt")
	other := filepath.Join(out, "private", strings.Repeat("ef", 32)+".wspr.br")
	for _, p := range []string{orphan, tmp, notes, other} {
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := GC(out, true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 2 {
		t.Errorf("dry run found %d files, want 2", rep.Files)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Error("dry run removed a file")
	}
	if _, err := GC(out, false); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]bool{orphan: false, tmp: false, notes: true, other: true} {
		if _, err := os.Stat(p); (err == nil) != want {
			t.Errorf("%s exists = %v, want %v", p, err == nil, want)
		}
	}
	// The build still checks clean.
	rep2, err := Check(out, nil)
	if err != nil || len(rep2.Broken) != 0 {
		t.Errorf("check after gc: %v %v", err, rep2)
	}
}
