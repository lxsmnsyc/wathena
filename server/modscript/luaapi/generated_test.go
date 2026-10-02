package luaapi

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/constants"
)

var update = flag.Bool("update", false, "write the generated files in docs/generated")

// generatedDir is docs/generated, from this package's folder.
const generatedDir = "../../../docs/generated"

// Generated returns every generated file under docs/generated, by path.
func generated(t *testing.T) map[string]string {
	t.Helper()
	data, err := reference.ReadFile("extracts/renewal/script_constants.json")
	if err != nil {
		t.Fatal(err)
	}
	consts, err := constants.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{
		"lua-api.md":       Markdown(),
		"lua-constants.md": ConstantsMarkdown(consts),
	}
	for p, s := range LuaLS(consts) {
		out["lua/"+p] = s
	}
	return out
}

// TestGeneratedFiles checks that docs/generated holds the definitions
// and the reference of today's API. Run with -update to write them.
func TestGeneratedFiles(t *testing.T) {
	files := generated(t)
	if *update {
		if err := os.RemoveAll(filepath.Join(generatedDir, "lua")); err != nil {
			t.Fatal(err)
		}
		for p, s := range files {
			full := filepath.Join(generatedDir, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(s), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	for p, want := range files {
		got, err := os.ReadFile(filepath.Join(generatedDir, filepath.FromSlash(p)))
		if err != nil || string(got) != want {
			t.Errorf("docs/generated/%s is not current; run go test ./server/modscript/luaapi -update", p)
		}
	}
	_ = fs.WalkDir(os.DirFS(generatedDir), "lua", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".lua") {
			if _, ok := files[p]; !ok {
				t.Errorf("docs/generated/%s is no longer generated; run go test ./server/modscript/luaapi -update", p)
			}
		}
		return nil
	})
}
