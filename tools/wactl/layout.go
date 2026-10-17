package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/server/economy"
	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/mapfolder"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/rotation"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
	"github.com/lxsmnsyc/wathena/tools/modkit"
)

// worldConfig is the part of waserver.yml that a new world reads: the
// mods, which give the records of its layout and spawns, the monsters
// and the spawn_table hooks, procedural.layout and spawns. Other keys
// are left alone.
type worldConfig struct {
	Mods struct {
		Dir     string              `yaml:"dir"`
		Ruleset string              `yaml:"ruleset"`
		Strict  bool                `yaml:"strict"`
		Approve map[string][]string `yaml:"approve"`
		Disable []string            `yaml:"disable"`
	} `yaml:"mods"`
	Procedural struct {
		FieldSize int            `yaml:"field_size"`
		Layout    layout.Options `yaml:"layout"`
	} `yaml:"procedural"`
	Spawns spawns.Settings `yaml:"spawns"`
}

// readWorldConfig reads waserver.yml, or nothing for "".
func readWorldConfig(path string) (worldConfig, error) {
	var c worldConfig
	if path == "" {
		return c, nil
	}
	b, err := os.ReadFile(path) // #nosec G304 -- the file the operator named.
	if err != nil {
		return c, fmt.Errorf("--config: %w", err)
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("--config %s: %w", path, err)
	}
	return c, nil
}

// proceduralWorld makes a procedural world's layout and spawns from the
// mods of the config file, or core's data and the defaults without one
// (decisions 0041 and 0042):
//
//   - It loads the game data of the ruleset's mode and the mods, as
//     waserver does: the layout's records, the monsters with their spawn
//     tags, the spawn themes and the mods' scripts.
//   - The layout comes from the public branch of the seed, and each map
//     generates from it, so the spawns know the maps' cells, features
//     and warps.
//   - The spawns come from the secret branch of the seed, which only this
//     command holds, then the mods' spawn_table hooks change them.
//   - The content follows (decision 0044): the mods' generation layers
//     run on every generated map, then each town's quests, the job
//     guilds and the rebirth place come from the stored spawns and the
//     quest templates, with the secret branch. The returned spawns hold
//     the layers' bosses.
func proceduralWorld(configPath, ruleset string, seed []byte, version int) (*madeWorld, error) {
	g, ok := gen.Lookup(version)
	if !ok || !g.Makes(gen.KindTown) {
		return nil, fmt.Errorf("map generator %d makes no towns; a world layout needs generator 3 or later", version)
	}
	c, err := readWorldConfig(configPath)
	if err != nil {
		return nil, err
	}
	core, err := spawns.LoadCore()
	if err != nil {
		return nil, err
	}
	settings, err := c.Spawns.Resolve(core.Densities)
	if err != nil {
		return nil, err
	}
	root, err := rng.NewRoot(seed)
	if err != nil {
		return nil, err
	}
	var found []*mod.Mod
	if c.Mods.Dir != "" {
		if found, err = mod.Discover(c.Mods.Dir); err != nil {
			return nil, fmt.Errorf("mods.dir: %w", err)
		}
	}
	mode := reference.Mode(ruleset)
	base, ok := mod.ModeBase(ruleset)
	if !ok {
		return nil, fmt.Errorf("the ruleset %q is not renewal or pre-renewal", ruleset)
	}
	env, err := modkit.LoadEnv(mode, "")
	if err != nil {
		return nil, err
	}
	// The mods' log goes to standard error at warning level, as the
	// operator runs this command once.
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx, cancel := commandContext(5 * time.Minute)
	defer cancel()
	loaded, err := gamedata.LoadMods(ctx, found, mod.Config{Base: base, Ruleset: c.Mods.Ruleset, Approve: c.Mods.Approve,
		Disable: c.Mods.Disable, Strict: c.Mods.Strict}, mode, env.Items, env.Monsters, env.Tables, env.Jobs, &mod.MemoryIDs{}, env.Core, log)
	if err != nil {
		return nil, fmt.Errorf("mods: %w", err)
	}
	recs, err := fields.LoadLayoutRecords(loaded.Set, g)
	if err != nil {
		return nil, fmt.Errorf("mods: %w", err)
	}
	public := gen.PublicBranch(root)
	l, home, err := fields.BuildLayout(recs, g, public, c.Procedural.Layout, c.Procedural.FieldSize)
	if err != nil {
		return nil, fmt.Errorf("the world layout: %w", err)
	}
	var homeMap *mapfolder.Map
	if home != nil {
		homeMap = home.Map
	}
	made, err := fields.GenerateLayout(g, public, l, homeMap)
	if err != nil {
		return nil, err
	}
	set := fields.NewSet(made, nil)
	warps, err := set.LayoutWarps(l)
	if err != nil {
		return nil, err
	}
	maps, err := set.SpawnMaps(l, warps)
	if err != nil {
		return nil, err
	}
	secret := spawns.SecretBranch(root)
	// The economy simulator on the world's game data, with the monsters
	// and items of the mods, for the economy checks of fields (decision
	// 0046).
	econ, targets, err := economyWorld(loaded, env)
	if err != nil {
		return nil, err
	}
	if settings.Economy {
		settings.Rate = gamedata.EconomyRater(econ, targets)
	}
	planned := spawns.Plan(secret, maps, spawns.NewPool(loaded.SpawnMonsters, loaded.SpawnThemes), settings)
	idx := gamedata.SpawnIndex(loaded.Monsters, loaded.SpawnMonsters)
	var layers content.Layers
	if zone.HasSpawnHooks(loaded.Zone) || zone.HasGeneration(loaded.Zone) {
		// The hooks' state has a random stream of its own, from the
		// secret branch, so a hook's draws repeat for the seed. The
		// generation layers run in the same state, each with its node.
		hooks, err := zone.NewSpawnHooks(loaded.Zone, secret.Child("hooks").Key())
		if err != nil {
			return nil, err
		}
		defer hooks.Close()
		for i := range planned {
			planned[i].Spawns = hooks.Run(maps[i], planned[i].Spawns, idx, log)
		}
		layers = hooks.Layers(secret, idx)
	}
	w, err := content.Generate(content.Input{
		Generator: g, Layout: l, Maps: fields.MapsOf(made), SpawnMaps: maps, Spawns: planned,
		SpawnMonsters: idx.Monsters, Data: gamedata.ContentData(loaded), Templates: loaded.QuestTemplates,
		Secret: secret, Layers: layers,
	})
	if err != nil {
		return nil, err
	}
	for _, n := range w.Notes {
		log.Warn("generation: " + n)
	}
	out := &madeWorld{layout: l, spawns: w.Spawns, content: w, maps: maps, econ: econ, targets: targets}
	// The rotating dungeon, and the keys of its spawns and the seasons'
	// for the game role (decision 0045).
	_, styles := fields.LayoutOf(recs.Styles, recs.Templates)
	if r, ok := layout.PickRotation(l, styles); ok {
		key := rotation.RotationKey(secret, r.Dungeon).Key()
		out.rotations = append(out.rotations, store.WorldRotation{Rotation: r, Key: key[:]})
	}
	sk := rotation.SeasonKey(secret).Key()
	out.seasonKey = sk[:]
	return out, nil
}

// economyWorld is the economy simulator on a world's game data, with
// core's targets.
func economyWorld(loaded *gamedata.Mods, env *modkit.Env) (*economy.World, economy.Targets, error) {
	cfg, err := economy.LoadConfig()
	if err != nil {
		return nil, nil, err
	}
	ts, err := economy.LoadTargets()
	if err != nil {
		return nil, nil, err
	}
	jobs := loaded.JobTables
	if jobs == nil {
		jobs = env.Jobs
	}
	return gamedata.EconomyWorld(cfg, item.New(loaded.Items.Data), jobs, loaded.Tables, loaded.Rules, loaded.Monsters), ts, nil
}

// madeWorld is a new procedural world's layout, spawns, content, and
// its rotating dungeon with the derived keys (decision 0045).
type madeWorld struct {
	// maps are the fields' and floors' spawn inputs, and econ and
	// targets the economy simulator and targets, for wactl sim economy.
	maps    []spawns.Map
	econ    *economy.World
	targets economy.Targets

	layout    *layout.Layout
	spawns    []spawns.Planned
	content   *content.World
	rotations []store.WorldRotation
	seasonKey []byte
}

// layoutText describes a layout for the operator.
func layoutText(l *layout.Layout) string {
	var fieldCount, floors int
	var towns, dungeons []string
	for _, m := range l.Maps {
		switch m.Kind {
		case gen.KindField:
			fieldCount++
		case gen.KindTown:
			towns = append(towns, fmt.Sprintf("%s (%s)", m.Name, gen.TownID(m.Town)))
		case gen.KindDungeon:
			floors++
			if m.Floor == 1 {
				dungeons = append(dungeons, m.Dungeon)
			}
		}
	}
	homeName := l.Home
	if h := l.Map(l.Home); h != nil {
		homeName = h.Name + " (" + h.ID + ")"
	}
	return fmt.Sprintf("the layout: fields %d, towns %d, dungeons %d with %d floors, links %d\n"+
		"towns: %s\ndungeons: %s\nhome area: %s\n",
		fieldCount, len(towns), len(dungeons), floors, len(l.Links), strings.Join(towns, ", "), strings.Join(dungeons, ", "), homeName)
}

// spawnText describes a world's spawns for the operator.
func spawnText(planned []spawns.Planned) string {
	lines, monsters, bosses := 0, 0, 0
	species := map[string]bool{}
	for _, p := range planned {
		for _, s := range p.Spawns {
			lines++
			monsters += s.Count
			species[s.Monster] = true
			if s.Role.Boss() {
				bosses++
			}
		}
	}
	return fmt.Sprintf("spawns: %d maps, %d lines, %d monsters of %d kinds, %d bosses\n", len(planned), lines, monsters, len(species), bosses)
}

// contentText describes a world's content for the operator.
func contentText(w *content.World) string {
	if w == nil {
		return ""
	}
	board, guild := 0, 0
	for _, q := range w.Quests {
		if q.Purpose == content.PurposeBoard {
			board++
		} else {
			guild++
		}
	}
	return fmt.Sprintf("content: %d board quests, %d guild test quests, %d places, %d objects\n", board, guild, len(w.Places), len(w.Objects))
}

// rotationText describes a world's rotating dungeon for the operator.
func rotationText(l *layout.Layout, rotations []store.WorldRotation) string {
	if len(rotations) == 0 {
		return "rotating dungeon: none; no dungeon style of the world has a rotation\n"
	}
	var parts []string
	for _, r := range rotations {
		name := r.Dungeon
		if m := l.Map(gen.DungeonMapID(r.Dungeon, 1)); m != nil {
			name = strings.TrimSuffix(m.Name, " F1") + " (" + r.Dungeon + ")"
		}
		parts = append(parts, name+", "+r.Rule.String())
	}
	return "rotating dungeon: " + strings.Join(parts, "; ") + ", in waserver's time zone\n"
}
