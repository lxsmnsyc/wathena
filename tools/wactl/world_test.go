package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
