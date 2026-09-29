package grf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeGRF(t *testing.T, path string, entries []WriteEntry) {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, entries); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClientPriority(t *testing.T) {
	dir := t.TempDir()
	writeGRF(t, filepath.Join(dir, "rdata.grf"), []WriteEntry{
		{Name: `data\both.txt`, Data: []byte("rdata")},
	})
	writeGRF(t, filepath.Join(dir, "data.grf"), []WriteEntry{
		{Name: `data\both.txt`, Data: []byte("data")},
		{Name: `data\only.txt`, Data: []byte("data only")},
		{Name: `data\loose.txt`, Data: []byte("grf")},
	})
	ini := "[Data]\n2=data.grf\n1=rdata.grf\n"
	if err := os.WriteFile(filepath.Join(dir, "DATA.INI"), []byte(ini), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "LOOSE.txt"), []byte("loose"), 0o600); err != nil {
		t.Fatal(err)
	}

	c, err := OpenClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if got := c.Archives(); len(got) != 2 || got[0] != "rdata.grf" {
		t.Errorf("archives %v, want rdata.grf first", got)
	}
	for name, want := range map[string]string{
		`data\both.txt`:  "rdata",
		`data\only.txt`:  "data only",
		`data\loose.txt`: "loose",
	} {
		s, ok := c.Lookup(name)
		if !ok {
			t.Fatalf("%s not found", name)
		}
		got, err := c.ReadFile(s)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if n := len(c.Files()); n != 3 {
		t.Errorf("%d files, want 3", n)
	}
}

func TestClientRejectsPathsOutside(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "outside.grf")
	writeGRF(t, outside, []WriteEntry{{Name: "x", Data: []byte("x")}})
	defer func() { _ = os.Remove(outside) }()
	ini := "[Data]\n1=../outside.grf\n"
	if err := os.WriteFile(filepath.Join(dir, "DATA.INI"), []byte(ini), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, err := OpenClient(dir); err == nil {
		_ = c.Close()
		t.Error("DATA.INI opened a GRF outside the client folder")
	}
}
