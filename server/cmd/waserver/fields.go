package main

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// devWorldLabel starts the secret of a --dev procedural world without a
// database. --seed follows it, so a test gets the same fields each run.
const devWorldLabel = "wathena-dev-world:"

// maxTowns is the most towns --towns runs.
const maxTowns = 99

// proceduralWorld is what a world runs besides its asset maps: nothing
// for a classic world, generated maps for a procedural one.
type proceduralWorld struct {
	mode gateway.WorldMode
	// set holds the generated maps, or is nil for a classic world.
	set *fields.Set
	// g is the world's generator and public its public branch.
	g      gen.Generator
	public rng.Node
}

// wantsMaps reports whether the config lists generated maps of its own.
func wantsMaps(p proceduralOptions) bool {
	return len(p.Fields) > 0 || p.Towns > 0 || len(p.Dungeons) > 0
}

// fieldMaps decides the generated fields of a world (decision 0039):
//
//   - A procedural world row: its generator, and the public branch the
//     row holds. The game role never reads the seed.
//   - No world row, with --dev and --fields: a procedural world on the
//     newest generator, with a secret from --seed. Development only.
//   - A classic world: no fields, and --fields is an error.
//
// It generates each field of --fields, or 0:0 when none is listed.
func fieldMaps(cfg settings, w store.World, log *slog.Logger) (proceduralWorld, error) {
	var g gen.Generator
	var public rng.Node
	switch {
	case w.Mode == gen.ModeProcedural:
		g, _ = gen.Lookup(w.GeneratorVersion)
		if len(w.PublicNode) != 32 {
			return proceduralWorld{}, fmt.Errorf("the world %q was made before generated fields and has no public node; make it again with wactl world new", w.Name)
		}
		public = rng.FromKey([32]byte(w.PublicNode))
	case !wantsMaps(cfg.Procedural):
		return proceduralWorld{}, nil
	case w.Name != "":
		return proceduralWorld{}, fmt.Errorf("--fields, --towns and --dungeons need a procedural world, and the world %q is %s", w.Name, w.Mode)
	case !gen.Procedural:
		return proceduralWorld{}, errors.New("--fields, --towns and --dungeons need a waserver built with -tags procedural")
	case !cfg.Dev:
		return proceduralWorld{}, errors.New("--fields, --towns and --dungeons without a procedural world need --dev")
	default:
		g, _ = gen.Latest()
		root, err := rng.NewRoot([]byte(devWorldLabel + cfg.Seed))
		if err != nil {
			return proceduralWorld{}, err
		}
		public = gen.PublicBranch(root)
		log.Warn("a development procedural world without a database: its fields follow --seed", "generator", g.Version)
	}
	list := cfg.Procedural.Fields
	if len(list) == 0 {
		list = []string{"0:0"}
	}
	specs := make([]fields.Spec, 0, len(list))
	for _, s := range list {
		sp, err := fields.ParseSpec(s)
		if err != nil {
			return proceduralWorld{}, err
		}
		specs = append(specs, sp)
	}
	began := time.Now()
	made, err := fields.Generate(g, public, specs, cfg.Procedural.FieldSize)
	if err != nil {
		return proceduralWorld{}, err
	}
	for _, f := range made {
		log.Info("field generated", "map", f.ID, "biome", f.BiomeName, "size", f.Field.Width, "attempt", f.Field.Attempt,
			"props", len(f.Field.Objects), "wmap_bytes", len(f.Field.WMAP))
	}
	log.Info("fields generated", "fields", len(made), "generator", g.Version, "took", time.Since(began).Round(time.Millisecond))
	tilesets, err := fields.LoadTilesets(g, cfg.Procedural.Tilesets)
	if err != nil {
		return proceduralWorld{}, err
	}
	mode := worldMode(w)
	if !mode.Procedural {
		mode = gateway.WorldMode{Procedural: true, GeneratorVersion: g.Version, RNGVersion: g.RNG}
	}
	return proceduralWorld{mode: mode, set: fields.NewSet(made, tilesets.JSON()), g: g, public: public}, nil
}

// addMaps generates the towns and dungeons of the config, once the mods
// gave the town templates and dungeon styles (decision 0040). Town n
// takes the template n - 1 modulo their number, in record order: core's
// first, then the mods'. A dungeon's ID is its style's name without its
// mod prefix. It replaces the set and the tileset table, which now holds
// the templates' buildings.
func addMaps(cfg settings, proc *proceduralWorld, styles []fields.Style, templates []fields.Template, log *slog.Logger) error {
	p := cfg.Procedural
	if proc.set == nil {
		if p.Towns > 0 || len(p.Dungeons) > 0 {
			return errors.New("--towns and --dungeons need a procedural world")
		}
		return nil
	}
	if err := fields.CheckRecords(proc.g, styles, templates); err != nil {
		return fmt.Errorf("mods:\n%w", indent(err))
	}
	var towns []fields.TownSpec
	if p.Towns > 0 && len(templates) == 0 {
		return errors.New("--towns needs a town template, and no mod has one")
	}
	for n := range p.Towns {
		towns = append(towns, fields.TownSpec{N: int64(n + 1), Template: templates[n%len(templates)]})
	}
	var dungeons []fields.DungeonSpec
	ids := map[string]bool{}
	for _, d := range p.Dungeons {
		name, floors, err := fields.ParseDungeonSpec(d)
		if err != nil {
			return err
		}
		st, ok := fields.FindStyle(styles, name)
		if !ok {
			return fmt.Errorf("dungeon %q: no dungeon style %q; the styles are %s", d, name, strings.Join(fields.StyleNames(styles), ", "))
		}
		if floors == 0 {
			floors = st.Style.Floors
		}
		id := fields.DungeonID(st.Name)
		if ids[id] {
			return fmt.Errorf("dungeon %q: two dungeons have the ID %s", d, id)
		}
		ids[id] = true
		dungeons = append(dungeons, fields.DungeonSpec{ID: id, Style: st, Floors: floors, Levels: fields.DevLevels})
	}
	began := time.Now()
	made, err := fields.GenerateMaps(proc.g, proc.public, towns, dungeons)
	if err != nil {
		return err
	}
	for _, f := range made {
		attrs := []any{"map", f.ID, "kind", f.Kind, "record", f.Name, "size", f.Field.Width, "attempt", f.Field.Attempt,
			"props", len(f.Field.Objects), "places", len(f.Field.Places), "wmap_bytes", len(f.Field.WMAP)}
		if f.Kind == gen.KindDungeon {
			attrs = append(attrs, "levels", fmt.Sprintf("%d-%d", f.Levels[0], f.Levels[1]))
		}
		log.Info("map generated", attrs...)
	}
	if len(made) > 0 {
		log.Info("towns and dungeons generated", "maps", len(made), "generator", proc.g.Version, "took", time.Since(began).Round(time.Millisecond))
	}
	tilesets, err := fields.LoadTilesets(proc.g, p.Tilesets, templates...)
	if err != nil {
		return err
	}
	proc.set = fields.NewSet(append(slices.Clone(proc.set.Fields()), made...), tilesets.JSON())
	return nil
}

// placeNPCs puts each NPC whose record gives a place at every place of
// its scope with its tag (review decision 42), and returns the NPCs by
// map. A classic world has no places, so these NPCs do not appear.
func placeNPCs(idx *places.Index, list []gamedata.PlaceNPC, log *slog.Logger) map[string][]zone.NPC {
	out := map[string][]zone.NPC{}
	for _, pn := range list {
		found := idx.Find(pn.Place.Scope, pn.Place.Tag)
		if len(found) == 0 {
			log.Info("an NPC has no place in this world", "npc", pn.NPC.Name, "scope", pn.Place.Scope.String(), "tag", pn.Place.Tag)
			continue
		}
		for _, i := range found {
			p := idx.Place(i)
			n := pn.NPC
			n.X, n.Y = p.X, p.Y
			out[p.Map] = append(out[p.Map], n)
		}
	}
	return out
}

// worldMaps returns the start map, the asset maps to run and the map
// names of every zone, asset maps first. In a procedural world, the
// start map is the first field unless --map names a field, and asset
// maps run only when --maps lists them.
func worldMaps(cfg settings, proc proceduralWorld) (start string, assetMaps, all []string, err error) {
	start = assetName(cfg.StartMap)
	if proc.set == nil {
		assetMaps, err = selectMaps(cfg.Assets, start, joinList(cfg.Maps))
		return start, assetMaps, assetMaps, err
	}
	if proc.set.Get(cfg.StartMap) != nil {
		start = cfg.StartMap
	} else {
		// The first field, else the first map of the set.
		start = proc.set.Fields()[0].ID
		for _, f := range proc.set.Fields() {
			if f.Kind == gen.KindField {
				start = f.ID
				break
			}
		}
	}
	if len(cfg.Maps) > 0 {
		for _, n := range cfg.Maps {
			if n == "all" {
				return "", nil, nil, errors.New("--maps all does not run in a procedural world; list the asset maps")
			}
			assetMaps = append(assetMaps, assetName(n))
		}
		slices.Sort(assetMaps)
		assetMaps = slices.Compact(assetMaps)
	}
	all = slices.Clone(assetMaps)
	for _, f := range proc.set.Fields() {
		all = append(all, f.ID)
	}
	return start, assetMaps, all, nil
}

// fieldGrids returns the cells of the fields, in the order of the set.
func fieldGrids(proc proceduralWorld) []*path.Grid {
	if proc.set == nil {
		return nil
	}
	out := make([]*path.Grid, 0, len(proc.set.Fields()))
	for _, f := range proc.set.Fields() {
		out = append(out, f.Grid())
	}
	return out
}

func joinList(l list) string {
	out := ""
	for i, s := range l {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}
