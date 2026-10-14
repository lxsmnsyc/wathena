package modkit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	clientdata "github.com/lxsmnsyc/wathena/client/data"
	"github.com/lxsmnsyc/wathena/internal/assets"
	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/pack"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modassets"
)

// A mod's art (proposal 0002, review decision 38): sources in art/, in the
// pack format with pack.yaml and as loose RO files, built into an asset
// build in assets/. Every name is under mod/<mod id>/.

// ArtSource is the build source of a mod's art/ folder.
func ArtSource(modDir, modID string) *pack.Source {
	return &pack.Source{Dir: filepath.Join(modDir, mod.ArtDir), Prefix: assets.ModPrefix(modID), Formats: pack.Both}
}

// HasArt reports whether a mod's folder has an art/ folder.
func HasArt(modDir string) bool {
	fi, err := os.Stat(filepath.Join(modDir, mod.ArtDir))
	return err == nil && fi.IsDir()
}

// ErrArtRejected is the error of a build that left files of art/ out.
var ErrArtRejected = errors.New("art/ has files the build cannot use")

// BuildArt builds a mod folder's art/ into out, which is the mod's
// assets/ folder when empty. quality is the Brotli level. A file the
// build cannot use is an error, after the build: the report lists them.
func BuildArt(modDir, out string, quality int) (*build.Report, error) {
	return buildArt(modDir, out, quality, time.Time{})
}

// buildArt builds with a fixed build time, or now for the zero time.
func buildArt(modDir, out string, quality int, at time.Time) (*build.Report, error) {
	m, err := mod.Read(modDir)
	if err != nil {
		return nil, err
	}
	if !HasArt(modDir) {
		return nil, fmt.Errorf("%s has no %s folder", modDir, mod.ArtDir)
	}
	if out == "" {
		out = filepath.Join(modDir, mod.AssetsDir)
	}
	src := ArtSource(modDir, m.ID())
	defer func() { _ = src.Close() }()
	rep, err := build.Run(build.Options{Out: out, Sources: []build.Source{src}, Brotli: quality, Time: at})
	if err != nil {
		return nil, err
	}
	if len(rep.Rejected) > 0 {
		return rep, ErrArtRejected
	}
	return rep, nil
}

// ArtNames lists the names a mod's art/ provides, without building it,
// and the files the build would leave out. A mod without art/ has none.
func ArtNames(modDir, modID string) (map[string]bool, []string, error) {
	if !HasArt(modDir) {
		return map[string]bool{}, nil, nil
	}
	src := ArtSource(modDir, modID)
	defer func() { _ = src.Close() }()
	items, err := src.Items()
	if err != nil {
		return nil, nil, err
	}
	out := make(map[string]bool, len(items))
	for _, it := range items {
		if assets.ValidName(it.Name) == nil {
			out[it.Name] = true
		}
	}
	return out, src.Rejected(), nil
}

// buildFiles lists the files of a build folder to pack, with forward
// slashes, sorted. The build's cache is left out.
func buildFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out, err
}

// providedArt lists the art names a mod provides: from its art/ sources
// for a folder, or from its built assets/ for a .zip file or a folder
// without sources. rejected lists the art files a build would leave out.
func providedArt(m *mod.Mod) (names map[string]bool, rejected []string, err error) {
	if fi, err := os.Stat(m.Path); err == nil && fi.IsDir() && HasArt(m.Path) {
		return ArtNames(m.Path, m.ID())
	}
	fsys, closeFS, err := m.Open()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = closeFS() }()
	sub, err := fs.Sub(fsys, mod.AssetsDir)
	if err != nil {
		return nil, nil, err
	}
	list, err := build.ListFS(sub)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]bool{}, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", mod.AssetsDir, err)
	}
	names = make(map[string]bool, len(list))
	for n := range list {
		names[n] = true
	}
	return names, nil, nil
}

// checkArt reports the art names the mod's records name that no mod
// provides, art files its build would leave out, and client data tables
// that do not pass their checks (proposal 0002, review decision 38).
func checkArt(l *Loaded, rep *Report) {
	bad := func(format string, args ...any) { rep.Problems = append(rep.Problems, fmt.Sprintf(format, args...)) }
	provided := map[string]map[string]bool{}
	for _, m := range l.Set.Mods {
		if m.Builtin {
			continue
		}
		names, rejected, err := providedArt(m)
		if err != nil {
			bad("%s: art: %v", m.ID(), err)
			continue
		}
		provided[m.ID()] = names
		if m.ID() == l.Mod.ID() {
			for _, r := range rejected {
				bad("art: %s", r)
			}
		}
	}
	for _, r := range modassets.ArtRefs(l.Mods) {
		if r.Mod != l.Mod.ID() {
			continue
		}
		owner, _, ok := assets.SplitMod(r.Name)
		if !ok {
			// Core's art is in the server's build, which validate does
			// not have. wactl assets check lists names a build lacks.
			continue
		}
		names, loaded := provided[owner]
		switch {
		case !loaded:
			bad("%s: %s is the art of %s, which does not load with this mod; add it to depends", r.Record, r.Name, owner)
		case !names[r.Name]:
			bad("%s: %s is not in %s's art", r.Record, r.Name, owner)
		}
	}
	var overrides []clientdata.Override
	for _, m := range l.Set.Mods {
		if m.Builtin {
			continue
		}
		fsys, closeFS, err := m.Open()
		if err != nil {
			continue
		}
		tables, err := modassets.ClientTables(fsys, m.ID())
		_ = closeFS()
		if err != nil {
			bad("%v", err)
			continue
		}
		overrides = append(overrides, tables...)
	}
	if _, err := clientdata.Merge(overrides); err != nil {
		addErrors(rep, err)
	}
}
