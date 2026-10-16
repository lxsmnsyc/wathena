package modkit

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/lxsmnsyc/wathena/server/fields"
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
// all from the secret TestWorldLabel, joined as waserver --dev joins them.
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
	made, err := fields.Generate(g, public, []fields.Spec{{X: 0, Y: 0}}, testFieldSize)
	if err != nil {
		return nil, err
	}
	var towns []fields.TownSpec
	for i, t := range l.TownTemplates {
		towns = append(towns, fields.TownSpec{N: int64(i + 1), Template: t})
	}
	var dungeons []fields.DungeonSpec
	ids := map[string]bool{}
	for _, s := range l.DungeonStyles {
		id := fields.DungeonID(s.Name)
		for n := 2; ids[id]; n++ {
			id = fmt.Sprintf("%s-%d", fields.DungeonID(s.Name), n)
		}
		ids[id] = true
		dungeons = append(dungeons, fields.DungeonSpec{ID: id, Style: s, Floors: s.Style.Floors, Levels: fields.DevLevels})
	}
	maps, err := fields.GenerateMaps(g, public, towns, dungeons)
	if err != nil {
		return nil, err
	}
	tilesets, err := fields.LoadTilesets(g, "", l.TownTemplates...)
	if err != nil {
		return nil, err
	}
	set := fields.NewSet(append(made, maps...), tilesets.JSON())
	warps, _ := set.DevLinks()
	idx := set.Places(warps)
	out := &generated{set: set, places: idx, warps: warps, npcs: map[string][]zone.NPC{}}
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
