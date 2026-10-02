package modkit

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// templateFS holds the files of a new mod. {%.ID%} and {%.Name%} are
// replaced.
//
//go:embed template
var templateFS embed.FS

// New writes a new mod from the template into dir, which must not exist
// or be empty: a manifest, an NPC with a conversation that uses saved
// variables, an item overlay, a test, an English locale, a README, and
// the editor definitions. It returns the files it wrote.
func New(id, dir string, consts *constants.Data) ([]string, error) {
	if _, err := mod.ParseManifest(mod.ManifestFile, []byte("id: "+id+"\nname: x\nversion: 0.1.0\napi: experimental\n")); err != nil {
		return nil, err
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("%s is not empty", dir)
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	data := struct{ ID, Name string }{id, modName(id)}
	var written []string
	err := fs.WalkDir(templateFS, "template", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		src, err := templateFS.ReadFile(p)
		if err != nil {
			return err
		}
		t, err := template.New(p).Delims("{%", "%}").Parse(string(src))
		if err != nil {
			return err
		}
		var out bytes.Buffer
		if err := t.Execute(&out, data); err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "template/")
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(rel)), out.Bytes()); err != nil {
			return err
		}
		written = append(written, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	n, err := WriteTypes(dir, consts)
	if err != nil {
		return nil, err
	}
	written = append(written, LuaRCFile, fmt.Sprintf("%s/ (%d files)", TypesDir, n))
	return written, nil
}

// modName makes a name people see from a mod ID: the last word, with
// _ and - as spaces and each word capitalized.
func modName(id string) string {
	last := id[strings.LastIndex(id, ".")+1:]
	words := strings.FieldsFunc(last, func(r rune) bool { return r == '_' || r == '-' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func writeFile(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}
