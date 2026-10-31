package rathena

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestFilesMatchDisk checks that the embedded files are the ones in this
// folder, and that each mode has them all.
func TestFilesMatchDisk(t *testing.T) {
	count := 0
	err := fs.WalkDir(Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		count++
		got, err := fs.ReadFile(Files, p)
		if err != nil {
			return err
		}
		want, err := os.ReadFile(filepath.FromSlash(p))
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from the file on disk", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 64 {
		t.Errorf("%d embedded files, want 64", count)
	}
}
