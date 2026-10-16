package modkit

import (
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sync"

	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/mapfolder"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Generated maps of test worlds (decision 0040). A wactl built with the
// procedural tag gives every test world the same small procedural world,
// so a case can find places and walk on generated maps:
//
//   - the field gen:field:0:0, 96 cells square,
//   - one town per town template, t1 to t<n>, in record order,
//   - one dungeon per dungeon style, with the style's floors, named by
//     the style without its mod prefix,
//
// all from the secret TestWorldLabel, joined as waserver --dev joins them
// (layout.Dev), with town names from the naming styles.
// NPC records with a place stand at their places.

// TestWorldLabel is the world secret of the test worlds' generated maps.
// It is public on purpose, so every run makes the same maps.
const TestWorldLabel = "wactl-mod-test" // #nosec G101 -- a public label, not a credential.

// testFieldSize is the side of the test world's field.
const testFieldSize = 96

// generated is the generated part of a mod's test worlds.
type generated struct {
	set    *fields.Set
	places *places.Index
	warps  map[string][]zone.Warp
	npcs   map[string][]zone.NPC
	// atlas is the test world's layout, for wa.towns.
	atlas *layout.Atlas
}

type genResult struct {
	once sync.Once
	g    *generated
	err  error
}

var genWorlds sync.Map // *Loaded -> *genResult

// generatedMaps returns the generated maps of the mod's test worlds,
// made once, or nil in a build without the procedural tag.
func (l *Loaded) generatedMaps() (*generated, error) {
	v, _ := genWorlds.LoadOrStore(l, &genResult{})
	r, _ := v.(*genResult)
	r.once.Do(func() { r.g, r.err = l.makeGenerated() })
	return r.g, r.err
}

func (l *Loaded) makeGenerated() (*generated, error) {
	g, ok := gen.Latest()
	if !ok || !g.Makes(gen.KindTown) {
		return nil, nil
	}
	root, err := rng.NewRoot([]byte(TestWorldLabel))
	if err != nil {
		return nil, err
	}
	public := gen.PublicBranch(root)
	if err := fields.CheckRecords(g, l.DungeonStyles, l.TownTemplates); err != nil {
		return nil, err
	}
	towns, styles := fields.LayoutOf(l.DungeonStyles, l.TownTemplates)
	spec := layout.DevSpec{FieldSize: testFieldSize, Fields: []layout.DevField{{X: 0, Y: 0, Biome: g.Climate(public, 0, 0)}}}
	for i, t := range towns {
		spec.Towns = append(spec.Towns, layout.DevTown{N: int64(i + 1), Record: t})
	}
	ids := map[string]bool{}
	for _, s := range styles {
		id := fields.DungeonID(s.Name)
		for n := 2; ids[id]; n++ {
			id = fmt.Sprintf("%s-%d", fields.DungeonID(s.Name), n)
		}
		ids[id] = true
		spec.Dungeons = append(spec.Dungeons, layout.DevDungeon{ID: id, Style: s, Floors: s.Style.Floors})
	}
	in := layout.Inputs{Generator: g, Public: public, Naming: l.Naming}
	lay, _, err := layout.Dev(spec, in)
	if err != nil {
		return nil, err
	}
	made, err := fields.GenerateLayout(g, public, lay, nil)
	if err != nil {
		return nil, err
	}
	tilesets, err := fields.LoadTilesets(g, "", l.TownTemplates...)
	if err != nil {
		return nil, err
	}
	set := fields.NewSet(made, tilesets.JSON())
	warps, err := set.LayoutWarps(lay)
	if err != nil {
		return nil, err
	}
	idx := set.Places(warps)
	out := &generated{set: set, places: idx, warps: warps, npcs: map[string][]zone.NPC{}, atlas: layout.NewAtlas(lay, layout.AtlasOptions{})}
	for _, pn := range l.PlaceNPCs {
		for _, i := range idx.Find(pn.Place.Scope, pn.Place.Tag) {
			p := idx.Place(i)
			n := pn.NPC
			n.X, n.Y = p.X, p.Y
			out.npcs[p.Map] = append(out.npcs[p.Map], n)
		}
	}
	return out, nil
}

// errNoGenerator is the error for a case that names a generated map in a
// build without the procedural tag.
var errNoGenerator = errors.New("this case runs on a generated map, which needs a wactl built with -tags procedural")

// placesOf returns the places of the mod's test worlds, or nil.
func (l *Loaded) placesOf(log *slog.Logger) *places.Index {
	g, err := l.generatedMaps()
	if err != nil {
		if log != nil {
			log.Warn("the test world's generated maps failed: " + err.Error())
		}
		return nil
	}
	if g == nil {
		return nil
	}
	return g.places
}

// checkMaps checks the mod's map folders under maps/ (strategy section
// 8.12): their files, the generator's checks, and that each folder's
// name is its id. With the procedural tag, its naming styles must name
// biomes of the generator too.
func (l *Loaded) checkMaps() []string {
	var out []string
	fsys, closeFS, err := l.Mod.Open()
	if err != nil {
		return []string{err.Error()}
	}
	defer func() { _ = closeFS() }()
	dirs, err := mapfolder.Find(fsys)
	if err != nil {
		out = append(out, err.Error())
	}
	for _, d := range dirs {
		m, err := mapfolder.Read(fsys, d)
		if err != nil {
			out = append(out, err.Error())
			continue
		}
		if base := path.Base(d); base != m.ID {
			out = append(out, fmt.Sprintf("%s: map.yaml's id %q is not the folder's name", d, m.ID))
		}
		for _, p := range m.Check() {
			out = append(out, d+": "+p)
		}
		if _, err := m.WMAP(); err != nil {
			out = append(out, d+": "+err.Error())
		}
	}
	if g, ok := gen.Latest(); ok && l.Naming != nil {
		if err := layout.CheckNaming(g.Biomes, l.Naming.Styles()); err != nil {
			out = append(out, err.Error())
		}
	}
	return out
}
