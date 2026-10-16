package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lxsmnsyc/wathena/server/mapfolder"
)

// mapValidate checks map folders (strategy section 8.12,
// docs/specs/map-folder-v1.md): their files against the format, then
// the map generator's checks, as the server runs them on a mod's home
// map. It prints each folder's problems and fails when any has one.
func mapValidate(args []string) error {
	fs := newFlags("map validate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("usage: wactl map validate <map folder> [<map folder> ...]")
	}
	failed := false
	for _, dir := range fs.Args() {
		m, err := mapfolder.Read(os.DirFS(dir), ".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
			failed = true
			continue
		}
		problems := m.Check()
		if base := filepath.Base(filepath.Clean(dir)); base != m.ID {
			problems = append(problems, fmt.Sprintf("map.yaml's id %q is not the folder's name %q; a mod's maps/<id>/ must match", m.ID, base))
		}
		if _, err := m.WMAP(); err != nil {
			problems = append(problems, "the WMAP: "+err.Error())
		}
		if len(problems) > 0 {
			failed = true
			for _, p := range problems {
				fmt.Fprintf(os.Stderr, "%s: %s\n", dir, p)
			}
			continue
		}
		home := ""
		if m.HasTag(mapfolder.TagHome) {
			home = ", a home map"
		}
		fmt.Printf("%s: %q, %d × %d cells, %d objects, %d places%s: no problems\n", dir, m.Name, m.Width, m.Height, len(m.Objects), len(m.Places), home)
	}
	if failed {
		return errFailed
	}
	return nil
}
