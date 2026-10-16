package main

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// devWorldLabel starts the secret of a --dev procedural world without a
// database. --seed follows it, so a test gets the same fields each run.
const devWorldLabel = "wathena-dev-world:"

// proceduralWorld is what a world runs besides its asset maps: nothing
// for a classic world, generated fields for a procedural one.
type proceduralWorld struct {
	mode gateway.WorldMode
	// set holds the fields, or is nil for a classic world.
	set *fields.Set
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
	case len(cfg.Procedural.Fields) == 0:
		return proceduralWorld{}, nil
	case w.Name != "":
		return proceduralWorld{}, fmt.Errorf("--fields needs a procedural world, and the world %q is %s", w.Name, w.Mode)
	case !gen.Procedural:
		return proceduralWorld{}, errors.New("--fields needs a waserver built with -tags procedural")
	case !cfg.Dev:
		return proceduralWorld{}, errors.New("--fields without a procedural world needs --dev")
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
	return proceduralWorld{mode: mode, set: fields.NewSet(made, tilesets.JSON())}, nil
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
		start = proc.set.Fields()[0].ID
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
