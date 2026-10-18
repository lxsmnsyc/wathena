package mod

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// A .zip mod's files are checked against their limits before they are
// read: scripts, data files and the manifest.
func TestZipModLimits(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "big.zip"))
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, text := range map[string]string{
		"big/mod.yaml":         manifest("example.big", "scripts: [scripts/main.lua]"),
		"big/scripts/main.lua": "return function() end --" + strings.Repeat(" ", MaxScriptBytes),
		"big/data/items.yml":   "#" + strings.Repeat(" ", MaxDataBytes),
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
	if _, err := s.Scripts("example.big"); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("scripts: err = %v", err)
	}
	if _, err := s.Mod("example.big").readFile("data/items.yml"); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("data: err = %v", err)
	}
}

// ReadLimited refuses a file by its stated size before it reads it, and
// by what it reads when the size is not known.
func TestReadLimited(t *testing.T) {
	fsys := fstest.MapFS{"a": {Data: bytes.Repeat([]byte("x"), 100)}}
	if _, err := ReadLimited(fsys, "a", 99); err == nil {
		t.Error("a file over the limit was read")
	}
	if b, err := ReadLimited(fsys, "a", 100); err != nil || len(b) != 100 {
		t.Errorf("%d bytes, %v", len(b), err)
	}
	dir := t.TempDir()
	writeMod(t, dir, "m", map[string]string{"mod.yaml": manifest("example.m", "description: "+strings.Repeat("x", MaxManifestBytes))})
	if _, err := Discover(dir); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("manifest: err = %v", err)
	}
}
