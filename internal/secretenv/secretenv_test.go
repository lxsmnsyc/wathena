package secretenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	const name = "WATHENA_TEST_SECRET"
	dir := t.TempDir()
	file := filepath.Join(dir, "secret")
	if err := os.WriteFile(file, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(name, "")
	t.Setenv(name+"_FILE", "")
	if v, err := Get(name); err != nil || v != "" {
		t.Errorf("unset: %q, %v", v, err)
	}

	t.Setenv(name, "from-env")
	if v, err := Get(name); err != nil || v != "from-env" {
		t.Errorf("variable: %q, %v", v, err)
	}

	t.Setenv(name+"_FILE", file)
	if _, err := Get(name); err == nil || strings.Contains(err.Error(), "from-") {
		t.Errorf("both set: %v", err)
	}

	t.Setenv(name, "")
	if v, err := Get(name); err != nil || v != "from-file" {
		t.Errorf("file: %q, %v", v, err)
	}

	t.Setenv(name+"_FILE", filepath.Join(dir, "missing"))
	if _, err := Get(name); err == nil {
		t.Error("a missing file is accepted")
	}

	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(name+"_FILE", empty)
	if _, err := Get(name); err == nil {
		t.Error("an empty file is accepted")
	}
}
