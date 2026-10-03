// Package mod finds, checks and orders mods (strategy section 6): their
// manifests, dependencies, load order, ruleset, permissions, data
// overlays and the IDs of the records they add. Mod scripts run in
// server/modscript.
package mod

import (
	"archive/zip"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/lxsmnsyc/wathena/server/rules"
)

// Core is the ID of the built-in mod that stands for the base game. It
// has no files yet: the base game's data still comes from the reference
// extracts. It anchors dependencies and owns the game's own records.
const Core = "core"

// CoreVersion is the version of the built-in core mods. It moves with
// the server's releases.
const CoreVersion = "0.1.0"

// Mod is one mod: a folder, a .zip file, or a built-in mod.
type Mod struct {
	Manifest *Manifest
	// Path is the folder or .zip file the mod came from, for messages.
	// Built-in mods have "(built in)".
	Path string
	// Builtin is set for core and the core ruleset mods.
	Builtin bool
	// Grants are the permissions the mod holds, once resolved.
	Grants Grants

	// open opens the mod's files. Each call reads the current files, so
	// a reload sees changes. It is nil for built-in mods.
	open func() (fs.FS, func() error, error)
}

// ID returns the mod's ID.
func (m *Mod) ID() string { return m.Manifest.ID }

// Open returns the mod's files and a function that closes them. A
// built-in mod has no files: it returns an empty file system.
func (m *Mod) Open() (fs.FS, func() error, error) {
	if m.open == nil {
		return emptyFS{}, func() error { return nil }, nil
	}
	return m.open()
}

type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// builtins returns the built-in mods: core and its two ruleset mods.
func builtins() []*Mod {
	v, _ := ParseVersion(CoreVersion)
	core := &Mod{Path: "(built in)", Builtin: true, Manifest: &Manifest{
		ID: Core, Name: "Core", Version: CoreVersion, API: API, License: "GPL-3.0-or-later", version: v,
	}}
	out := []*Mod{core}
	for _, base := range []string{Renewal, PreRenewal} {
		dep := Dependency{ID: Core, Version: CoreVersion}
		dep.versions, _ = ParseRange(CoreVersion)
		m := &Manifest{ID: "core.ruleset." + base, Name: "Core ruleset: " + base, Version: CoreVersion, API: API, version: v, Depends: []Dependency{dep}}
		m.Provides.Ruleset = &RulesetSpec{}
		out = append(out, &Mod{Path: "(built in)", Builtin: true, Manifest: m})
	}
	return out
}

// Discover finds the mods in a folder: each folder in it that has a
// mod.yaml, and each .zip file with a mod.yaml at its root or in its one
// top folder. A symbolic link to a folder or a .zip file counts as one,
// as wactl dev uses. Other entries, and names that start with ".", are
// skipped. Mods come back sorted by path.
func Discover(dir string) ([]*Mod, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []*Mod
	var errs []error
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		p := filepath.Join(dir, name)
		isDir := e.IsDir()
		if e.Type()&fs.ModeSymlink != 0 {
			fi, err := os.Stat(p)
			if err != nil {
				continue
			}
			isDir = fi.IsDir()
		}
		switch {
		case isDir:
			if _, err := os.Stat(filepath.Join(p, ManifestFile)); err != nil {
				continue
			}
		case !strings.EqualFold(filepath.Ext(name), ".zip"):
			continue
		}
		m, err := Read(p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, m)
	}
	return out, errors.Join(errs...)
}

// Read reads one mod: a folder with a mod.yaml, or a .zip file.
func Read(p string) (*Mod, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	var open func() (fs.FS, func() error, error)
	if fi.IsDir() {
		open = func() (fs.FS, func() error, error) {
			root, err := os.OpenRoot(p)
			if err != nil {
				return nil, nil, err
			}
			return root.FS(), root.Close, nil
		}
	} else {
		open = func() (fs.FS, func() error, error) { return openZip(p) }
	}
	return readMod(p, open)
}

// openZip opens a mod packed as a .zip file. The manifest is at the
// root, or in the zip's only top folder.
func openZip(p string) (fs.FS, func() error, error) {
	r, err := zip.OpenReader(p)
	if err != nil {
		return nil, nil, err
	}
	if _, err := fs.Stat(r, ManifestFile); err == nil {
		return r, r.Close, nil
	}
	tops, err := fs.ReadDir(r, ".")
	if err == nil && len(tops) == 1 && tops[0].IsDir() {
		sub, err := fs.Sub(r, tops[0].Name())
		if err == nil {
			if _, err := fs.Stat(sub, ManifestFile); err == nil {
				return sub, r.Close, nil
			}
		}
	}
	_ = r.Close()
	return nil, nil, fmt.Errorf("%s has no %s at its root or in its one top folder", p, ManifestFile)
}

func readMod(p string, open func() (fs.FS, func() error, error)) (*Mod, error) {
	fsys, closeFS, err := open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeFS() }()
	data, err := fs.ReadFile(fsys, ManifestFile)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	m, err := ParseManifest(filepath.ToSlash(filepath.Join(p, ManifestFile)), data)
	if err != nil {
		return nil, err
	}
	return &Mod{Manifest: m, Path: p, open: open}, nil
}

// Config is how the server owner set up mods.
type Config struct {
	// Ruleset is the ID of the world's ruleset mod. Empty picks the core
	// ruleset of Base.
	Ruleset string
	// Base is the world's base, from the world row or waserver's --mode:
	// renewal or pre_renewal. The ruleset must have it. Empty accepts
	// any, and picks renewal when Ruleset is empty too.
	Base string
	// Approve lists the permissions the owner approved, by mod ID.
	Approve map[string][]string
	// Disable lists mods that are installed and must not load.
	Disable []string
	// Strict turns overlay warnings into errors.
	Strict bool
	// Core holds core's files: the item data and scripts converted from
	// rAthena when the server starts (mods/README.md, "Item scripts").
	// Without it, core has no files.
	Core fs.FS
}

// Set is the mods that load, in load order, with the world's ruleset.
type Set struct {
	// Mods are in load order. core comes first.
	Mods    []*Mod
	Ruleset *Ruleset
	// Notes are things the owner should know, such as skipped ruleset
	// mods and flagged permissions. They are not errors.
	Notes []string
	// Strict is Config.Strict.
	Strict bool

	byID map[string]*Mod
}

// Mod returns a loaded mod by ID, or nil.
func (s *Set) Mod(id string) *Mod { return s.byID[id] }

// Resolve checks the found mods against each other and the owner's
// config, and orders them. Every problem is listed in the error. found
// must not hold built-in IDs: Discover's manifests cannot.
func Resolve(found []*Mod, cfg Config) (*Set, error) {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	s := &Set{byID: map[string]*Mod{}, Strict: cfg.Strict}
	all := map[string]*Mod{}
	for _, m := range builtins() {
		if m.ID() == Core && cfg.Core != nil {
			core := cfg.Core
			m.open = func() (fs.FS, func() error, error) { return core, func() error { return nil }, nil }
		}
		all[m.ID()] = m
	}
	for _, m := range found {
		if other, ok := all[m.ID()]; ok {
			fail("%s is installed twice: %s and %s", m.ID(), other.Path, m.Path)
			continue
		}
		all[m.ID()] = m
	}
	disabled := map[string]bool{}
	for _, id := range cfg.Disable {
		if all[id] == nil {
			s.Notes = append(s.Notes, fmt.Sprintf("mods.disable lists %s, which is not installed", id))
			continue
		}
		if all[id].Builtin {
			fail("mods.disable: %s is built in and cannot be disabled", id)
			continue
		}
		disabled[id] = true
	}

	// The ruleset, and the ruleset mods it is built from.
	rs, chain, err := resolveRuleset(all, disabled, cfg)
	if err != nil {
		return nil, err
	}
	s.Ruleset = rs
	inChain := map[string]bool{}
	for _, id := range chain {
		inChain[id] = true
	}
	active := map[string]*Mod{}
	skipped := map[string]string{}
	for id, m := range all {
		switch {
		case disabled[id]:
			skipped[id] = "is disabled in mods.disable"
		case m.Manifest.Provides.Ruleset != nil && !inChain[id]:
			skipped[id] = "is a ruleset mod, and the world's ruleset is " + rs.Mod
			if !m.Builtin {
				s.Notes = append(s.Notes, fmt.Sprintf("ruleset mod %s is installed but not used: the world's ruleset is %s", id, rs.Mod))
			}
		default:
			active[id] = m
		}
	}

	for _, id := range slices.Sorted(maps.Keys(active)) {
		m := active[id]
		man := m.Manifest
		if !man.SupportsAPI() {
			fail("%s: api %q does not include this server's API, which is %s; use api: %s or a range that includes it", m.Path, man.API, API, API)
		}
		if man.Provides.Ruleset == nil {
			if ok, why := man.supports(rs); !ok {
				fail("%s is refused on the ruleset %s: %s", id, rs.Mod, why)
			}
		}
		for _, d := range man.Depends {
			dep := active[d.ID]
			if dep == nil {
				if why, ok := skipped[d.ID]; ok {
					fail("%s depends on %s, which %s", id, d.ID, why)
				} else {
					fail("%s depends on %s %s, which is not installed", id, d.ID, d.versions)
				}
				continue
			}
			if !d.versions.Match(dep.Manifest.version) {
				fail("%s depends on %s %s, and version %s is installed", id, d.ID, d.versions, dep.Manifest.version)
			}
		}
		g, missing := grant(man.Permissions, cfg.Approve[id])
		if m.Builtin {
			// The built-in mods are the base game, which the server
			// trusts: core's item scripts give items, heal and warp.
			for _, p := range Permissions {
				g[p.Name] = true
			}
		}
		m.Grants = g
		if len(missing) > 0 {
			fail("%s requests permissions the server owner has not approved: %s. Review them, then approve them in the config under mods.approve.%s",
				id, strings.Join(missing, ", "), id)
		}
		for _, p := range man.Permissions {
			if perm, _ := LookupPermission(p); perm.Flagged && g[p] {
				s.Notes = append(s.Notes, fmt.Sprintf("%s has the flagged permission %s: it can %s", id, p, perm.Allows))
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(cfg.Approve)) {
		m := active[id]
		if m == nil {
			s.Notes = append(s.Notes, fmt.Sprintf("mods.approve lists %s, which does not load", id))
			continue
		}
		for _, p := range cfg.Approve[id] {
			if _, ok := LookupPermission(p); !ok {
				fail("mods.approve.%s: %q is not a permission", id, p)
			} else if !m.Manifest.requests(p) {
				s.Notes = append(s.Notes, fmt.Sprintf("mods.approve.%s approves %s, which the mod does not request", id, p))
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	order, err := loadOrder(active)
	if err != nil {
		return nil, err
	}
	s.Mods = order
	for _, m := range order {
		s.byID[m.ID()] = m
	}
	sort.Strings(s.Notes)
	return s, nil
}

// resolveRuleset finds the world's ruleset mod and builds its ruleset.
// It returns the chain of ruleset mods, from the world's own down to a
// core one.
func resolveRuleset(all map[string]*Mod, disabled map[string]bool, cfg Config) (*Ruleset, []string, error) {
	id := cfg.Ruleset
	if id == "" {
		id = CoreRenewal
		if cfg.Base == PreRenewal {
			id = CorePreRenewal
		}
	}
	var chain []string
	seen := map[string]bool{}
	for cur := id; ; {
		m := all[cur]
		switch {
		case m == nil && len(chain) == 0:
			return nil, nil, fmt.Errorf("the ruleset %s is not installed; use %s, %s, or a ruleset mod in the mods folder", cur, CoreRenewal, CorePreRenewal)
		case m == nil:
			return nil, nil, fmt.Errorf("the ruleset %s extends %s, which is not installed", chain[len(chain)-1], cur)
		case disabled[cur]:
			return nil, nil, fmt.Errorf("the ruleset %s is disabled in mods.disable", cur)
		case m.Manifest.Provides.Ruleset == nil:
			return nil, nil, fmt.Errorf("%s is not a ruleset mod: its manifest has no provides.ruleset", cur)
		case seen[cur]:
			return nil, nil, fmt.Errorf("ruleset cycle: %s -> %s", strings.Join(chain, " -> "), cur)
		}
		seen[cur] = true
		chain = append(chain, cur)
		if cur == CoreRenewal || cur == CorePreRenewal {
			break
		}
		cur = m.Manifest.Provides.Ruleset.Extends
	}
	base := strings.TrimPrefix(chain[len(chain)-1], "core.ruleset.")
	rs := &Ruleset{Mod: id, Base: base, Flags: coreFlags(base), Limits: map[string]int{}, Rules: map[string]int{}}
	for i := len(chain) - 1; i >= 0; i-- {
		spec := all[chain[i]].Manifest.Provides.Ruleset
		for name, v := range spec.Flags {
			rs.Flags[name] = string(v)
		}
		for name, v := range spec.Limits {
			n, err := checkBound(rules.LimitBounds, "limit", name, string(v))
			if err != nil {
				return nil, nil, fmt.Errorf("the ruleset %s: %w", chain[i], err)
			}
			rs.Limits[name] = n
		}
		for name, v := range spec.Rules {
			n, err := checkBound(rules.RuleBounds, "rule", name, string(v))
			if err != nil {
				return nil, nil, fmt.Errorf("the ruleset %s: %w", chain[i], err)
			}
			rs.Rules[name] = n
		}
	}
	if err := checkRuleset(rs); err != nil {
		return nil, nil, fmt.Errorf("the ruleset %s: %w", id, err)
	}
	if cfg.Base != "" && cfg.Base != base {
		return nil, nil, fmt.Errorf("the ruleset %s has the %s base, and the world's base is %s; a world cannot change its base", id, base, cfg.Base)
	}
	return rs, chain, nil
}

// loadOrder sorts mods so each loads after its dependencies, the mods in
// its load_after that are present, and core. Among mods that could load
// next, the lowest ID goes first, so the order is the same on every
// start. A cycle is an error that shows it.
func loadOrder(active map[string]*Mod) ([]*Mod, error) {
	after := map[string][]string{}
	for id, m := range active {
		var deps []string
		if id != Core {
			deps = append(deps, Core)
		}
		for _, d := range m.Manifest.Depends {
			deps = append(deps, d.ID)
		}
		for _, a := range m.Manifest.LoadAfter {
			if active[a] != nil {
				deps = append(deps, a)
			}
		}
		if r := m.Manifest.Provides.Ruleset; r != nil && active[r.Extends] != nil {
			deps = append(deps, r.Extends)
		}
		slices.Sort(deps)
		after[id] = slices.Compact(deps)
	}
	done := map[string]bool{}
	var order []*Mod
	for len(order) < len(active) {
		var next string
		for _, id := range slices.Sorted(maps.Keys(active)) {
			if done[id] {
				continue
			}
			ready := true
			for _, d := range after[id] {
				if !done[d] {
					ready = false
					break
				}
			}
			if ready {
				next = id
				break
			}
		}
		if next == "" {
			return nil, fmt.Errorf("load order cycle: %s", findCycle(after, done))
		}
		done[next] = true
		order = append(order, active[next])
	}
	return order, nil
}

// findCycle returns one cycle among the mods not done, as "a -> b -> a",
// where a must load after b.
func findCycle(after map[string][]string, done map[string]bool) string {
	state := map[string]int{} // 1 visiting, 2 finished
	var stack []string
	var found []string
	var visit func(id string) bool
	visit = func(id string) bool {
		state[id] = 1
		stack = append(stack, id)
		for _, d := range after[id] {
			if done[d] {
				continue
			}
			if state[d] == 1 {
				i := slices.Index(stack, d)
				found = append(slices.Clone(stack[i:]), d)
				return true
			}
			if state[d] == 0 && visit(d) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
		return false
	}
	for _, id := range slices.Sorted(maps.Keys(after)) {
		if !done[id] && state[id] == 0 && visit(id) {
			break
		}
	}
	// Each mod in the cycle loads after the next one.
	return strings.Join(found, " -> ") + " (each mod must load after the next)"
}

// Visible returns the mods whose script files a mod may require: itself
// and the mods in its depends.
func (s *Set) Visible(id string) []string {
	m := s.byID[id]
	if m == nil {
		return nil
	}
	out := []string{id}
	for _, d := range m.Manifest.Depends {
		out = append(out, d.ID)
	}
	return out
}

// readFile reads one file of a mod.
func (m *Mod) readFile(name string) ([]byte, error) {
	fsys, closeFS, err := m.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeFS() }()
	return fs.ReadFile(fsys, name)
}

// file names a file of the mod in messages.
func (m *Mod) file(name string) string {
	if m.Builtin {
		return m.ID() + ": " + name
	}
	return path.Join(filepath.ToSlash(m.Path), name)
}
