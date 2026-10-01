package mod

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// MaxScriptBytes is the most Lua source one mod may have, in bytes. It
// keeps a broken or hostile mod from filling memory at load.
const MaxScriptBytes = 8 << 20

// Scripts is a snapshot of one mod's Lua files, which each zone loads.
// A reload reads a new snapshot.
type Scripts struct {
	Mod string
	// Entries are the manifest's entry scripts, which run once per zone
	// with a mod context.
	Entries []string
	// Files maps each .lua file under scripts/ to its source.
	Files map[string]string
	// Grants are the mod's permissions.
	Grants Grants
	// Visible are the mods whose files the mod may require: itself and
	// its depends.
	Visible []string
}

// Scripts reads a mod's Lua files now.
func (s *Set) Scripts(id string) (*Scripts, error) {
	m := s.Mod(id)
	if m == nil {
		return nil, fmt.Errorf("%s is not loaded", id)
	}
	out := &Scripts{Mod: id, Entries: m.Manifest.Scripts, Files: map[string]string{}, Grants: m.Grants, Visible: s.Visible(id)}
	fsys, closeFS, err := m.Open()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Path, err)
	}
	defer func() { _ = closeFS() }()
	total := 0
	err = fs.WalkDir(fsys, "scripts", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == "scripts" && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipDir
			}
			return err
		}
		if d.IsDir() || path.Ext(p) != ".lua" {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		total += len(b)
		if total > MaxScriptBytes {
			return fmt.Errorf("the scripts are larger than %d MB", MaxScriptBytes>>20)
		}
		out.Files[p] = string(b)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.file("scripts"), err)
	}
	for _, e := range out.Entries {
		if _, ok := out.Files[e]; !ok {
			return nil, fmt.Errorf("%s: scripts lists %s, which does not exist", m.file(ManifestFile), e)
		}
	}
	return out, nil
}

// AllScripts reads the Lua files of every loaded mod that has any.
func (s *Set) AllScripts() ([]*Scripts, error) {
	var out []*Scripts
	for _, m := range s.Mods {
		if m.Builtin {
			continue
		}
		sc, err := s.Scripts(m.ID())
		if err != nil {
			return nil, err
		}
		if len(sc.Files) > 0 {
			out = append(out, sc)
		}
	}
	return out, nil
}

// ModulePath turns the part of a module name after the mod ID into the
// file it names: lib.rewards is scripts/lib/rewards.lua.
func ModulePath(rest string) (string, bool) {
	if rest == "" || strings.ContainsAny(rest, `/\`) {
		return "", false
	}
	p := "scripts/" + strings.ReplaceAll(rest, ".", "/") + ".lua"
	if checkScriptPath(p) != nil {
		return "", false
	}
	return p, true
}
