package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

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
