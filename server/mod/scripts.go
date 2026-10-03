package mod

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"go.yaml.in/yaml/v3"
)

// MaxScriptBytes is the most Lua source one mod may have, in bytes. It
// keeps a broken or hostile mod from filling memory at load.
const MaxScriptBytes = 8 << 20

// Scripts is a snapshot of one mod's Lua files, which each zone loads.
// A reload reads a new snapshot.
type Scripts struct {
	Mod string
	// Name is the manifest's name, which player:whisper shows by default.
	Name string
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
	// Texts are the lines of the mod's English locale, LocaleFile, by
	// key. Nested keys are joined with dots: healer.greeting.
	Texts map[string]string
}

// LocaleFile is the English locale of a mod (strategy section 6.13).
const LocaleFile = "locales/en.yml"

// MaxLocaleBytes is the largest locale file.
const MaxLocaleBytes = 4 << 20

// Scripts reads a mod's Lua files now.
func (s *Set) Scripts(id string) (*Scripts, error) {
	m := s.Mod(id)
	if m == nil {
		return nil, fmt.Errorf("%s is not loaded", id)
	}
	out := &Scripts{Mod: id, Name: m.Manifest.Name, Entries: m.Manifest.Scripts, Files: map[string]string{}, Grants: m.Grants, Visible: s.Visible(id)}
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
		// Core's converted item scripts are larger, and the server makes
		// them itself.
		if total > MaxScriptBytes && !m.Builtin {
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
	data, err := fs.ReadFile(fsys, LocaleFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("%s: %w", m.file(LocaleFile), err)
	case len(data) > MaxLocaleBytes:
		return nil, fmt.Errorf("%s: larger than %d MB", m.file(LocaleFile), MaxLocaleBytes>>20)
	default:
		if out.Texts, err = readLocale(data); err != nil {
			return nil, fmt.Errorf("%s: %w", m.file(LocaleFile), err)
		}
	}
	return out, nil
}

// readLocale reads a locale file: a mapping of keys to text, where a
// value may be a mapping of more keys.
func readLocale(data []byte) (map[string]string, error) {
	var root map[string]any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	out := map[string]string{}
	var walk func(prefix string, m map[string]any) error
	walk = func(prefix string, m map[string]any) error {
		for k, v := range m {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			switch v := v.(type) {
			case string:
				out[key] = v
			case map[string]any:
				if err := walk(key, v); err != nil {
					return err
				}
			default:
				return fmt.Errorf("%s: want text or a mapping of keys", key)
			}
		}
		return nil
	}
	return out, walk("", root)
}

// AllScripts reads the Lua files of every loaded mod that has any,
// core's converted item scripts included.
func (s *Set) AllScripts() ([]*Scripts, error) {
	var out []*Scripts
	for _, m := range s.Mods {
		if m.Builtin && m.open == nil {
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
