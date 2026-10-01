package script

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// sourcesDigest hashes every file under csrc and lua/src, with its path.
func sourcesDigest(t *testing.T) string {
	h := sha256.New()
	for _, dir := range []string{"csrc", "lua/src"} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path) // #nosec G304 -- files of this package.
			if err != nil {
				return err
			}
			h.Write([]byte(filepath.ToSlash(path)))
			h.Write([]byte{0})
			h.Write(data)
			h.Write([]byte{0})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

var digestLine = regexp.MustCompile(`(?m)^// sources sha256: \S+$`)

// TestSourcesDigest checks that lua_cgo.c names the current C sources, so
// the Go build cache never reuses Lua or glue.c compiled from older ones.
func TestSourcesDigest(t *testing.T) {
	want := "// sources sha256: " + sourcesDigest(t)
	data, err := os.ReadFile("lua_cgo.c")
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		data = digestLine.ReplaceAll(data, []byte(want))
		if err := os.WriteFile("lua_cgo.c", data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := digestLine.Find(data); string(got) != want {
		t.Errorf("lua_cgo.c has %q, want %q; run go test -run TestSourcesDigest -update", got, want)
	}
}
