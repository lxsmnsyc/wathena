// Package modkit is the mod tooling of wactl (strategy section 6.9):
// checking a mod, running its tests in a headless world, a new mod from
// a template, packing, installing, and editor definitions. wactl's mod
// commands are thin wrappers around it.
package modkit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// Env is the game data that checks and tests run against: rAthena's
// data of one mode, converted to core's files, as waserver loads it.
// Load it once per command.
type Env struct {
	Mode     reference.Mode
	Items    *item.Data
	Core     *convert.Core
	Tables   *progression.Tables
	Prog     *progression.Data
	Jobs     *status.Tables
	Start    status.Character
	Monsters map[uint32]gamedata.Monster
	Warps    map[string][]zone.Warp
	// Statuses is the status database, which test worlds share.
	Statuses *statuschange.DB
	// Assets is an asset build whose map cells tests use. Without it,
	// every cell of a map is walkable.
	Assets string

	mu    sync.Mutex
	grids map[string]*path.Grid
	// levels are the fields' levels, by asset name, read once.
	levelsOnce sync.Once
	levels     map[string]int
}

// fieldLevels are the levels of the fields among the maps a test world
// runs: its one map, when it is a field with monsters.
func (e *Env) fieldLevels(asset string) map[string]int {
	e.levelsOnce.Do(func() {
		e.levels = map[string]int{}
		spawns, err := gamedata.SpawnsOf(e.Mode, e.Monsters)
		if err != nil {
			return
		}
		for id, l := range gamedata.FieldLevels(spawns) {
			e.levels["map/"+id] = l
		}
	})
	out := map[string]int{}
	if l, ok := e.levels[asset]; ok {
		out[asset] = l
	}
	return out
}

// LoadEnv loads a mode's data. assets may be "".
func LoadEnv(mode reference.Mode, assets string) (*Env, error) {
	e := &Env{Mode: mode, Assets: assets, grids: map[string]*path.Grid{}}
	var err error
	if e.Items, err = gamedata.LoadItemData(mode); err != nil {
		return nil, err
	}
	if e.Core, err = convert.LoadCore(mode); err != nil {
		return nil, fmt.Errorf("core's scripts: %w", err)
	}
	if e.Tables, e.Prog, err = gamedata.LoadProgression(mode); err != nil {
		return nil, err
	}
	if e.Jobs, e.Start, err = gamedata.StartCharacter(mode, "all_1"); err != nil {
		return nil, err
	}
	if e.Statuses, _, err = e.Core.LoadStatuses(mode); err != nil {
		return nil, err
	}
	e.Jobs.SetStatuses(e.Statuses)
	if e.Monsters, err = gamedata.LoadMonsters(mode, e.Tables, e.Prog); err != nil {
		return nil, err
	}
	if e.Warps, err = world.LoadWarps(mode); err != nil {
		return nil, err
	}
	return e, nil
}

// Grid returns a map's cells: from the asset build, or an open grid
// that holds every NPC and warp of the map.
func (e *Env) Grid(mapName string, npcs []zone.NPC) (*path.Grid, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if g := e.grids[mapName]; g != nil {
		return g, nil
	}
	var g *path.Grid
	if e.Assets != "" {
		grids, err := world.LoadGrids(e.Assets, []string{mapName})
		if err != nil {
			return nil, err
		}
		g = grids[0]
	} else {
		size := 400
		for _, n := range npcs {
			size = max(size, n.X+32, n.Y+32)
		}
		for _, w := range e.Warps[mapName] {
			size = max(size, w.X+32, w.Y+32)
		}
		size = min(size, 1024)
		g = &path.Grid{Width: size, Height: size, Cells: make([]uint8, size*size)}
		for i := range g.Cells {
			g.Cells[i] = path.Walkable | path.Shootable
		}
	}
	e.grids[mapName] = g
	return g, nil
}

// Loaded is a mod with its dependencies and core, merged as waserver
// merges them.
type Loaded struct {
	// Mod is the mod the command works on.
	Mod *mod.Mod
	// Path is the mod's folder or .zip file.
	Path string
	*gamedata.Mods
}

// Load reads the mod at p, finds its dependencies in depsDir (may be
// ""), and merges them over core. The owner's approval is assumed for
// every permission the mods request. base is the ruleset base, renewal
// or pre_renewal. A ruleset mod loads as the world's ruleset, so its
// tests run with its flags, and its base must be the environment's
// mode.
func (e *Env) Load(ctx context.Context, p, depsDir, base string, log *slog.Logger) (*Loaded, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	m, err := mod.Read(p)
	if err != nil {
		return nil, err
	}
	found := []*mod.Mod{m}
	if depsDir != "" {
		others, err := mod.Discover(depsDir)
		if err != nil {
			return nil, err
		}
		for _, o := range others {
			if o.ID() != m.ID() && !samePath(o.Path, p) {
				found = append(found, o)
			}
		}
	}
	approve := map[string][]string{}
	for _, f := range found {
		approve[f.ID()] = f.Manifest.Permissions
	}
	// Only the mods the target needs: its dependencies, and theirs.
	found = needed(m, found)
	// The server owner's budget limits are not known here: a manifest may
	// ask for any budget up to the ceilings.
	cfg := mod.Config{Base: base, Approve: approve, Budgets: anyBudget}
	if m.Manifest.Provides.Ruleset != nil {
		b, _ := mod.ModeBase(string(e.Mode))
		cfg.Ruleset, cfg.Base = m.ID(), b
	}
	for id := range approve {
		if !slices.ContainsFunc(found, func(f *mod.Mod) bool { return f.ID() == id }) {
			delete(approve, id)
		}
	}
	loaded, err := gamedata.LoadMods(ctx, found, cfg, e.Mode, e.Items, e.Monsters, e.Tables, e.Jobs, &mod.MemoryIDs{}, e.Core, log)
	if err != nil {
		return nil, err
	}
	return &Loaded{Mod: loaded.Set.Mod(m.ID()), Path: p, Mods: loaded}, nil
}

// needed returns the target and the mods it depends on, directly or not.
// A missing dependency is left for the loader to report.
func needed(target *mod.Mod, found []*mod.Mod) []*mod.Mod {
	byID := map[string]*mod.Mod{}
	for _, f := range found {
		byID[f.ID()] = f
	}
	seen := map[string]bool{}
	var out []*mod.Mod
	var visit func(m *mod.Mod)
	visit = func(m *mod.Mod) {
		if seen[m.ID()] {
			return
		}
		seen[m.ID()] = true
		out = append(out, m)
		for _, d := range m.Manifest.Depends {
			if dep := byID[d.ID]; dep != nil {
				visit(dep)
			}
		}
		for _, a := range m.Manifest.LoadAfter {
			if dep := byID[a]; dep != nil {
				visit(dep)
			}
		}
	}
	visit(target)
	return out
}

func samePath(a, b string) bool {
	x, err1 := filepath.Abs(a)
	y, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && x == y
}

// ModPath returns the mod folder an argument names: the argument, or
// the working folder when it is empty.
func ModPath(arg string) (string, error) {
	if arg == "" {
		arg = "."
	}
	fi, err := os.Stat(arg)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		if _, err := os.Stat(filepath.Join(arg, mod.ManifestFile)); err != nil {
			return "", fmt.Errorf("%s has no %s: name a mod's folder", arg, mod.ManifestFile)
		}
		return arg, nil
	}
	if strings.EqualFold(filepath.Ext(arg), ".zip") {
		return arg, nil
	}
	return "", errors.New(arg + " is not a mod's folder or a .zip file")
}
