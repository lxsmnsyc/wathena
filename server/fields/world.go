package fields

import (
	"errors"
	"fmt"
	"io/fs"
	"testing/fstest"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/fields/core"
	"github.com/lxsmnsyc/wathena/server/layout"
	layoutcore "github.com/lxsmnsyc/wathena/server/layout/core"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Records are what a world layout reads from the loaded mods (decision
// 0041): the town templates, dungeon styles and naming styles, the chat
// filter for town names, and the home maps.
type Records struct {
	Styles    []Style
	Templates []Template
	Naming    []layout.NamingStyle
	Filter    *chat.Filter
	Homes     []layout.Home
}

// LoadLayoutRecords reads a layout's records from a mod set and checks
// them against the generator.
func LoadLayoutRecords(set *mod.Set, g gen.Generator) (Records, error) {
	var r Records
	var err error
	if r.Styles, r.Templates, _, err = LoadRecords(set); err != nil {
		return Records{}, err
	}
	if err := CheckRecords(g, r.Styles, r.Templates); err != nil {
		return Records{}, err
	}
	if r.Naming, _, err = layout.LoadNaming(set); err != nil {
		return Records{}, err
	}
	if err := layout.CheckNaming(g.Biomes, r.Naming); err != nil {
		return Records{}, err
	}
	if r.Filter, err = layout.LoadFilter(set); err != nil {
		return Records{}, err
	}
	if r.Homes, err = layout.FindHomes(set); err != nil {
		return Records{}, err
	}
	return r, nil
}

// Inputs returns the layout's inputs for a world, with home its home
// map or nil.
func (r Records) Inputs(g gen.Generator, public rng.Node, home *layout.Home) layout.Inputs {
	towns, styles := LayoutOf(r.Styles, r.Templates)
	return layout.Inputs{Generator: g, Public: public, Towns: towns, Styles: styles,
		Naming: layout.NewNaming(r.Naming), Blocked: layout.Blocked(r.Filter), Home: home.HomeMap()}
}

// HomeOf returns the home map folder a layout names, from the records'
// home maps, or nil for a layout without one.
func (r Records) HomeOf(l *layout.Layout) (*layout.Home, error) {
	for _, m := range l.Maps {
		if m.Kind != layout.KindHome {
			continue
		}
		for i := range r.Homes {
			if r.Homes[i].Record == m.Record {
				return &r.Homes[i], nil
			}
		}
		return nil, fmt.Errorf("the world's home map %s comes from %s, which no loaded mod has; install and approve the mod again", m.ID, m.Record)
	}
	return nil, nil
}

// BuildLayout makes a new world's layout from the loaded mods' records
// and the config (decision 0041). It returns the home map's folder too,
// or nil.
func BuildLayout(r Records, g gen.Generator, public rng.Node, opts layout.Options, fieldSize int) (*layout.Layout, *layout.Home, error) {
	cfg, err := opts.Config(fieldSize)
	if err != nil {
		return nil, nil, err
	}
	home, err := layout.PickHome(r.Homes, opts.Home)
	if err != nil {
		return nil, nil, err
	}
	if home != nil {
		if p := home.Map.Check(); len(p) > 0 {
			return nil, nil, fmt.Errorf("the home map %s: %v", home.Record, errors.New(p[0]))
		}
	}
	l, err := layout.Build(cfg, r.Inputs(g, public, home))
	if err != nil {
		return nil, nil, err
	}
	return l, home, nil
}

// LayoutCore returns core's files that a layout reads, as a mod.Config's
// Core: the dungeon styles, town templates, naming styles and chat
// filter. Tools that only make layouts, such as `wactl world new`, load
// the mods over them instead of converting core's scripts.
func LayoutCore() (fs.FS, error) {
	out := fstest.MapFS{}
	add := func(fsys fs.FS, from, to string) error {
		b, err := fs.ReadFile(fsys, from)
		if err != nil {
			return err
		}
		out[to] = &fstest.MapFile{Data: b, Mode: 0o644}
		return nil
	}
	err := errors.Join(
		add(core.Files, mod.DungeonStyleKind.File, mod.DungeonStyleKind.File),
		add(core.Files, mod.TownTemplateKind.File, mod.TownTemplateKind.File),
		add(layoutcore.Files, mod.NamingStyleKind.File, mod.NamingStyleKind.File),
		add(chat.CoreFiles, "core/"+chat.FilterFile, chat.FilterFile),
	)
	return out, err
}
