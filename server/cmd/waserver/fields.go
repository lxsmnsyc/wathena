package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/mapfolder"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// devWorldLabel starts the secret of a --dev procedural world without a
// database. --seed follows it, so a test gets the same maps each run.
const devWorldLabel = "wathena-dev-world:"

// maxTowns is the most towns --towns runs.
const maxTowns = 99

// teleportSprite is the look of the teleport service: rAthena's Kafra,
// 4_F_KAFRA1.
const teleportSprite = 112

// proceduralWorld is what a world runs besides its asset maps: nothing
// for a classic world, the maps of its layout for a procedural one
// (decision 0041).
type proceduralWorld struct {
	mode gateway.WorldMode
	// set holds the generated maps, or is nil for a classic world.
	set *fields.Set
	// g is the world's generator and public its public branch.
	g      gen.Generator
	public rng.Node
	// stored is the layout the database keeps, or nil for a dev world.
	stored *layout.Layout
	// atlas is the layout while the server runs, and warps the warps of
	// its links, by map ID.
	atlas *layout.Atlas
	warps map[string][]zone.Warp
	// start is the home area's map, and spawn its start cell.
	start    string
	spawn    [2]int
	hasSpawn bool
	// spawns are the stored spawn lists of the fields and dungeon
	// floors, by map ID (decision 0042). A dev world has none.
	spawns map[string][]spawns.Spawn
	// content is the stored content: places, objects and quests
	// (decision 0044). A dev world has none.
	content *content.World
	// changes are the stored rotations and seasons, and devSecret the
	// secret branch of a dev world, which gives its rotations' and
	// seasons' keys (decision 0045).
	changes   store.WorldChanges
	devSecret *rng.Node
	// setup is the state the world starts in, and host makes the later
	// changes.
	setup changeSetup
	host  *changeHost
}

// wantsDevMaps reports whether the config lists generated maps of its
// own: --fields, --towns or --dungeons.
func wantsDevMaps(p proceduralOptions) bool {
	return len(p.Fields) > 0 || p.Towns > 0 || len(p.Dungeons) > 0
}

// wantsMaps reports whether the config asks for a dev procedural world.
func wantsMaps(p proceduralOptions) bool { return wantsDevMaps(p) || p.DevLayout }

// fieldMaps decides a world's generator and public branch (decisions
// 0039 and 0041). The maps follow the mods (addMaps).
//
//   - A procedural world row: its generator, and the public branch the
//     row holds. The game role never reads the seed. Its layout comes
//     from the database.
//   - No world row, with --dev and --fields, --towns, --dungeons or
//     --layout: a procedural world on the newest generator, with a
//     secret from --seed. Development only.
//   - A classic world: no maps, and those flags are errors.
func fieldMaps(cfg settings, w store.World, log *slog.Logger) (proceduralWorld, error) {
	var g gen.Generator
	var public rng.Node
	var devSecret *rng.Node
	switch {
	case w.Mode == gen.ModeProcedural:
		g, _ = gen.Lookup(w.GeneratorVersion)
		if len(w.PublicNode) != 32 {
			return proceduralWorld{}, fmt.Errorf("the world %q was made before generated fields and has no public node; make it again with wactl world new", w.Name)
		}
		if wantsMaps(cfg.Procedural) {
			return proceduralWorld{}, fmt.Errorf("the world %q runs the layout it was made with; leave out --fields, --towns, --dungeons and --layout", w.Name)
		}
		public = rng.FromKey([32]byte(w.PublicNode))
	case !wantsMaps(cfg.Procedural):
		return proceduralWorld{}, nil
	case w.Name != "":
		return proceduralWorld{}, fmt.Errorf("--fields, --towns, --dungeons and --layout need a procedural world, and the world %q is %s", w.Name, w.Mode)
	case !gen.Procedural:
		return proceduralWorld{}, errors.New("--fields, --towns, --dungeons and --layout need a waserver built with -tags procedural")
	case !cfg.Dev:
		return proceduralWorld{}, errors.New("--fields, --towns, --dungeons and --layout without a procedural world need --dev")
	default:
		g, _ = gen.Latest()
		root, err := rng.NewRoot([]byte(devWorldLabel + cfg.Seed))
		if err != nil {
			return proceduralWorld{}, err
		}
		public = gen.PublicBranch(root)
		secret := spawns.SecretBranch(root)
		devSecret = &secret
		log.Warn("a development procedural world without a database: its maps follow --seed", "generator", g.Version)
	}
	if g.Field == nil {
		return proceduralWorld{}, fmt.Errorf("map generator %d makes no fields; fields need generator 2 or later", g.Version)
	}
	mode := worldMode(w)
	if !mode.Procedural {
		mode = gateway.WorldMode{Procedural: true, GeneratorVersion: g.Version, RNGVersion: g.RNG}
	}
	return proceduralWorld{mode: mode, g: g, public: public, devSecret: devSecret}, nil
}

// loadLayout reads a procedural world's layout from the database.
func loadLayout(ctx context.Context, pool *pgxpool.Pool, w store.World, proc *proceduralWorld) error {
	if w.Mode != gen.ModeProcedural || pool == nil {
		return nil
	}
	l, err := store.LoadLayout(ctx, pool)
	if errors.Is(err, store.ErrNoLayout) {
		return fmt.Errorf("the world %q was made before the world layout; make it again with wactl world new", w.Name)
	}
	if err != nil {
		return fmt.Errorf("the world layout: %w", err)
	}
	proc.stored = l
	if proc.spawns, err = store.LoadSpawns(ctx, pool); err != nil {
		return fmt.Errorf("the world's spawns: %w", err)
	}
	if proc.content, err = store.LoadContent(ctx, pool); err != nil {
		return fmt.Errorf("the world's content: %w", err)
	}
	if proc.changes, err = store.LoadWorldChanges(ctx, pool); err != nil {
		return fmt.Errorf("the world's rotations and seasons: %w", err)
	}
	return nil
}

// worldQuests makes the quest records of a world's stored quests, adds
// them to the quests every zone reads, and gives the zones the quests of
// core's boards and guilds (decision 0044). A quest whose monster or item
// no longer loads is left out with a warning.
func worldQuests(proc proceduralWorld, loaded *mods, log *slog.Logger) error {
	if proc.content == nil {
		return nil
	}
	recs, world, missing := gamedata.WorldQuests(proc.content, loaded.all.Monsters, loaded.items)
	for _, m := range missing {
		log.Warn("a world quest names a monster or an item that does not load; the quest is left out", "quest", m)
	}
	t, err := gamedata.WithWorldQuests(loaded.zone.Quests, recs)
	if err != nil {
		return err
	}
	loaded.zone.Quests, loaded.all.Quests, loaded.zone.World = t, t, world
	log.Info("world content loaded", "quests", len(recs), "places", len(proc.content.Places), "objects", len(proc.content.Objects))
	return nil
}

// zoneSpawns adds the stored spawn lists to the zones' spawns, with the
// stats of the loaded monsters. A line whose monster no longer loads,
// such as one of a removed mod, is left out with a warning.
func zoneSpawns(proc proceduralWorld, monsters map[uint32]gamedata.Monster, to map[string][]zone.Spawn, log *slog.Logger) {
	if len(proc.spawns) == 0 {
		return
	}
	lines, count := 0, 0
	for _, id := range slices.Sorted(maps.Keys(proc.spawns)) {
		list, missing := gamedata.ZoneSpawns(proc.spawns[id], monsters)
		for _, m := range missing {
			log.Warn("a stored spawn names a monster that does not load; its line is left out", "map", id, "monster", m)
		}
		to[id] = append(to[id], list...)
		lines += len(list)
		for _, s := range list {
			count += s.Amount
		}
	}
	log.Info("world spawns loaded", "maps", len(proc.spawns), "lines", lines, "monsters", count)
}

// addMaps makes the maps of a procedural world once the mods gave their
// records (decision 0041):
//
//   - A world row's layout, from the database.
//   - A dev world with --layout: a layout built as `wactl world new`
//     builds one, from procedural.layout and --seed.
//   - A dev world with --fields, --towns and --dungeons: those maps,
//     linked as phase 7 step 3's dev links linked them (layout.Dev).
//     Town n takes the template n - 1 modulo their number, in record
//     order. A dungeon's ID is its style's name without its mod prefix.
//
// It generates every map, puts the warps on the links, and makes the
// atlas. pool saves renames, or is nil.
func addMaps(cfg settings, proc *proceduralWorld, set *mod.Set, pool *pgxpool.Pool, log *slog.Logger) error {
	p := cfg.Procedural
	if proc.mode.GeneratorVersion == 0 {
		if wantsMaps(p) {
			return errors.New("--fields, --towns, --dungeons and --layout need a procedural world")
		}
		return nil
	}
	if !proc.g.Makes(gen.KindTown) {
		return fmt.Errorf("map generator %d makes no towns; a world layout needs generator 3 or later", proc.g.Version)
	}
	recs, err := fields.LoadLayoutRecords(set, proc.g)
	if err != nil {
		return fmt.Errorf("mods:\n%w", indent(err))
	}
	var l *layout.Layout
	var home *layout.Home
	switch {
	case proc.stored != nil:
		l = proc.stored
		home, err = recs.HomeOf(l)
	case p.DevLayout:
		l, home, err = fields.BuildLayout(recs, proc.g, proc.public, p.Layout, p.FieldSize)
	default:
		var unlinked []string
		l, unlinked, err = devLayout(cfg, proc, recs)
		for _, id := range unlinked {
			log.Warn("a generated map has no warp to it: add fields with free edges", "map", id)
		}
	}
	if err != nil {
		return err
	}
	var homeMap *mapfolder.Map
	if home != nil {
		homeMap = home.Map
	}
	// The world runs the state its stored rotations and seasons lead to
	// by now (decision 0045).
	_, styles := fields.LayoutOf(recs.Styles, recs.Templates)
	if proc.setup, err = setupChanges(cfg, proc, l, styles, time.Now(), log); err != nil {
		return err
	}
	run := l.Apply(proc.public, len(proc.g.Biomes), proc.setup.state.State)
	began := time.Now()
	made, err := fields.GenerateLayout(proc.g, proc.public, run, homeMap)
	if err != nil {
		return err
	}
	for _, f := range made {
		attrs := []any{"map", f.ID, "name", f.DisplayName, "kind", f.Kind, "size", f.Field.Width, "attempt", f.Field.Attempt,
			"levels", strconv.Itoa(f.Levels[0]) + "-" + strconv.Itoa(f.Levels[1]), "places", len(f.Field.Places), "wmap_bytes", len(f.Field.WMAP)}
		switch f.Kind {
		case gen.KindField:
			attrs = append(attrs, "biome", f.BiomeName)
		default:
			attrs = append(attrs, "record", f.Name)
		}
		log.Info("map generated", attrs...)
	}
	log.Info("world layout generated", "maps", len(made), "links", len(run.Links), "home", run.Home, "generator", proc.g.Version,
		"took", time.Since(began).Round(time.Millisecond))
	tilesets, err := fields.LoadTilesets(proc.g, p.Tilesets, fields.TemplatesOf(l)...)
	if err != nil {
		return err
	}
	proc.set = fields.NewSet(made, tilesets.JSON())
	missing, err := proc.set.AddContent(runningContent(proc.content, run))
	if err != nil {
		return err
	}
	for _, id := range missing {
		log.Warn("the world's content names a map the world does not run; its places and objects are left out", "map", id)
	}
	if proc.warps, err = proc.set.LayoutWarps(run); err != nil {
		return err
	}
	opts := layout.AtlasOptions{Blocked: layout.Blocked(recs.Filter)}
	if pool != nil && proc.stored != nil {
		opts.Save = func(ctx context.Context, names []layout.Rename) error { return store.RenameMaps(ctx, pool, names) }
	}
	proc.atlas = layout.NewAtlas(l, opts)
	proc.atlas.Replace(run)
	proc.set.SetNames(proc.atlas.Name)
	proc.start = run.Home
	if f := proc.set.Get(run.Home); f != nil {
		proc.spawn, proc.hasSpawn = spawnOf(f)
	}
	return nil
}

// runningContent leaves out the stored content of maps that run
// otherwise than at world creation: rotating floors and the fields of
// changed regions (decision 0045). Their layers' places and objects
// stood on cells that changed.
func runningContent(w *content.World, run *layout.Layout) *content.World {
	if w == nil {
		return nil
	}
	same := func(id string) bool {
		m := run.Map(id)
		return m != nil && m.Season == 0
	}
	out := *w
	out.Places, out.Objects = nil, nil
	for _, p := range w.Places {
		if same(p.Map) {
			out.Places = append(out.Places, p)
		}
	}
	for _, o := range w.Objects {
		if same(o.Map) {
			out.Objects = append(out.Objects, o)
		}
	}
	return &out
}

// spawnOf is where new characters start on the home area: the first
// place tagged start, else a town's square, else none.
func spawnOf(f *fields.Field) ([2]int, bool) {
	for _, tag := range []string{"start", "square"} {
		for _, pl := range f.Field.Places {
			if pl.HasTag(tag) {
				return [2]int{pl.X, pl.Y}, true
			}
		}
	}
	return [2]int{}, false
}

// devLayout is the layout of --fields, --towns and --dungeons.
func devLayout(cfg settings, proc *proceduralWorld, recs fields.Records) (*layout.Layout, []string, error) {
	p := cfg.Procedural
	list := p.Fields
	if len(list) == 0 {
		list = []string{"0:0"}
	}
	spec := layout.DevSpec{FieldSize: p.FieldSize, BandWidth: p.Layout.BandWidth, BandStep: p.Layout.BandStep, MaxLevel: p.Layout.MaxLevel}
	for _, s := range list {
		sp, err := fields.ParseSpec(s)
		if err != nil {
			return nil, nil, err
		}
		biome := proc.g.Climate(proc.public, sp.X, sp.Y)
		if sp.Biome != "" {
			b, ok := proc.g.BiomeIndex(sp.Biome)
			if !ok {
				return nil, nil, fmt.Errorf("field %s: unknown biome %q; generator %d has %s", gen.FieldID(sp.X, sp.Y), sp.Biome, proc.g.Version, strings.Join(proc.g.Biomes, ", "))
			}
			biome = b
		}
		if slices.ContainsFunc(spec.Fields, func(f layout.DevField) bool { return f.X == sp.X && f.Y == sp.Y }) {
			return nil, nil, fmt.Errorf("field %s is listed twice", gen.FieldID(sp.X, sp.Y))
		}
		spec.Fields = append(spec.Fields, layout.DevField{X: sp.X, Y: sp.Y, Biome: biome})
	}
	towns, styles := fields.LayoutOf(recs.Styles, recs.Templates)
	if p.Towns > 0 && len(towns) == 0 {
		return nil, nil, errors.New("--towns needs a town template, and no mod has one")
	}
	for n := range p.Towns {
		spec.Towns = append(spec.Towns, layout.DevTown{N: int64(n + 1), Record: towns[n%len(towns)]})
	}
	ids := map[string]bool{}
	for _, d := range p.Dungeons {
		name, floors, err := fields.ParseDungeonSpec(d)
		if err != nil {
			return nil, nil, err
		}
		i := slices.IndexFunc(styles, func(s layout.StyleRecord) bool { return s.Name == name })
		if i < 0 {
			return nil, nil, fmt.Errorf("dungeon %q: no dungeon style %q; the styles are %s", d, name, strings.Join(fields.StyleNames(recs.Styles), ", "))
		}
		if floors == 0 {
			floors = styles[i].Style.Floors
		}
		id := fields.DungeonID(styles[i].Name)
		if ids[id] {
			return nil, nil, fmt.Errorf("dungeon %q: two dungeons have the ID %s", d, id)
		}
		ids[id] = true
		spec.Dungeons = append(spec.Dungeons, layout.DevDungeon{ID: id, Style: styles[i], Floors: floors})
	}
	return layout.Dev(spec, recs.Inputs(proc.g, proc.public, nil))
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
// start map is the home area unless --map names a generated map, and
// asset maps run only when --maps lists them.
func worldMaps(cfg settings, proc proceduralWorld) (start string, assetMaps, all []string, err error) {
	start = assetName(cfg.StartMap)
	if proc.set == nil {
		assetMaps, err = selectMaps(cfg.Assets, start, joinList(cfg.Maps))
		return start, assetMaps, assetMaps, err
	}
	start = proc.start
	if proc.set.Get(cfg.StartMap) != nil {
		start = cfg.StartMap
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

// fieldGrids returns the cells of the generated maps, in the order of
// the set.
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
