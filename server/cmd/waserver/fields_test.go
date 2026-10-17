package main

import (
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestFieldsConfig: --fields, --field-size and --tilesets, from the file
// and the flags, and their checks.
func TestFieldsConfig(t *testing.T) {
	assets := assetDir(t)
	p := writeConfig(t, `
assets: `+assets+`
dev: true
procedural:
  fields: ["0:0", "1:0:desert"]
  field_size: 128
`)
	o, _, err := parseOptions("waserver all", []string{"--config", p, "--field-size", "96"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(o.Procedural.Fields, []string{"0:0", "1:0:desert"}) || o.Procedural.FieldSize != 96 {
		t.Errorf("procedural %+v", o.Procedural)
	}
	if _, err := validate(o, secrets{}); err != nil {
		t.Error(err)
	}
	o, _, err = parseOptions("waserver all", []string{"--assets", assets, "--dev", "--fields", "2:3,4:x", "--field-size", "8", "--tilesets", "/no/such/file"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	_, err = validate(o, secrets{})
	for _, want := range []string{"procedural.field_size", "procedural.fields", "procedural.tilesets"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("no %s problem in %v", want, err)
		}
	}
	if defaultOptions().Procedural.FieldSize != 300 {
		t.Error("the default field size is not 300")
	}
}

// TestFieldMaps: which worlds run generated maps, where their nodes come
// from (decision 0039), and a world row's stored layout (decision 0041).
func TestFieldMaps(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := settings{options: defaultOptions()}
	cfg.Dev = true
	cfg.Procedural.FieldSize = 48

	// A classic world, or none, without --fields runs no fields.
	if p, err := fieldMaps(cfg, store.World{}, log); err != nil || p.set != nil || p.mode.Procedural {
		t.Errorf("no fields: %+v %v", p, err)
	}
	cfg.Procedural.Fields = list{"0:0", "1:0"}
	classic := store.World{Name: "C", Mode: gen.ModeClassic}
	if _, err := fieldMaps(cfg, classic, log); err == nil || !strings.Contains(err.Error(), "procedural world") {
		t.Errorf("a classic world with fields: %v", err)
	}
	p, err := fieldMaps(cfg, store.World{}, log)
	if !gen.Procedural {
		if err == nil || !strings.Contains(err.Error(), "-tags procedural") {
			t.Errorf("fields without the tag: %v", err)
		}
		return
	}
	if err != nil || !p.mode.Procedural {
		t.Fatalf("dev fields: %+v %v", p, err)
	}
	if err := addMaps(cfg, &p, coreSet(t), nil, log); err != nil || len(p.set.Fields()) != 2 {
		t.Fatalf("dev fields: %v", err)
	}
	latest, _ := gen.Latest()
	if p.mode.GeneratorVersion != latest.Version || p.mode.RNGVersion != rng.Version {
		t.Errorf("dev mode %+v", p.mode)
	}
	// The dev world's fields follow --seed.
	root, _ := rng.NewRoot([]byte(devWorldLabel))
	if got, want := p.set.Get("gen:field:1:0").Key, gen.FieldNode(gen.PublicBranch(root), 1, 0).Key(); got != want {
		t.Errorf("dev key %x, want %x", got, want)
	}
	noDev := cfg
	noDev.Dev = false
	if _, err := fieldMaps(noDev, store.World{}, log); err == nil {
		t.Error("fields without --dev or a world ran")
	}
	// worldMaps: the home area starts, here the first field, unless
	// --map names a generated map, and asset maps run only when --maps
	// lists them.
	start, assetMaps, all, err := worldMaps(cfg, p)
	if err != nil || start != "gen:field:0:0" || len(assetMaps) != 0 || !slices.Equal(all, []string{"gen:field:0:0", "gen:field:1:0"}) {
		t.Errorf("maps: %s %v %v %v", start, assetMaps, all, err)
	}
	cfg.StartMap, cfg.Maps = "gen:field:1:0", list{"placeholder/map"}
	start, assetMaps, all, err = worldMaps(cfg, p)
	if err != nil || start != "gen:field:1:0" || !slices.Equal(assetMaps, []string{"placeholder/map"}) || len(all) != 3 {
		t.Errorf("maps: %s %v %v %v", start, assetMaps, all, err)
	}
	if g := fieldGrids(p); len(g) != 2 || g[0].Width != 48 || g[1].Height != 48 {
		t.Error("field grids")
	}

	// A procedural world row runs the layout it was made with, from its
	// public node. The seed is never read, and --fields is an error.
	cfg = settings{options: defaultOptions()}
	public := gen.PublicBranch(rng.FromKey([32]byte{4}))
	key := public.Key()
	row := store.World{Name: "P", Mode: gen.ModeProcedural, GeneratorVersion: latest.Version, RNGVersion: rng.Version, PublicNode: key[:]}
	p, err = fieldMaps(cfg, row, log)
	if err != nil || p.set != nil {
		t.Fatalf("world: %+v %v", p, err)
	}
	recs, err := fields.LoadLayoutRecords(coreSet(t), latest)
	if err != nil {
		t.Fatal(err)
	}
	towns, dungeons := 2, 1
	p.stored, _, err = fields.BuildLayout(recs, latest, public, layout.Options{Grid: []int{2, 2}, Towns: &towns, Dungeons: &dungeons}, 48)
	if err != nil {
		t.Fatal(err)
	}
	if err := addMaps(cfg, &p, coreSet(t), nil, log); err != nil {
		t.Fatal(err)
	}
	if f := p.set.Get("gen:field:1:1"); f == nil || f.Key != gen.FieldNode(public, 1, 1).Key() || f.DisplayName == "" {
		t.Errorf("the world's field 1:1: %+v", f)
	}
	if start, _, all, _ := worldMaps(cfg, p); start != "gen:town:t1" || len(all) != len(p.stored.Maps) || !p.hasSpawn {
		t.Errorf("the world starts on %s with %d maps, spawn %v", start, len(all), p.hasSpawn)
	}
	if p.atlas == nil || len(p.atlas.Towns()) != 2 || len(p.warps["gen:town:t1"]) == 0 {
		t.Error("the world has no atlas or warps")
	}
	cfg.Procedural.Fields = list{"0:0"}
	if _, err := fieldMaps(cfg, row, log); err == nil || !strings.Contains(err.Error(), "runs the layout") {
		t.Errorf("--fields in a world: %v", err)
	}
	cfg.Procedural.Fields = nil
	row.PublicNode = nil
	if _, err := fieldMaps(cfg, row, log); err == nil || !strings.Contains(err.Error(), "public node") {
		t.Errorf("a world without a public node: %v", err)
	}
	row.PublicNode, row.GeneratorVersion = key[:], 1
	if _, err := fieldMaps(cfg, row, log); err == nil || !strings.Contains(err.Error(), "no fields") {
		t.Errorf("generator 1: %v", err)
	}
}

// TestZoneSpawns: a world row's stored spawn lists reach the zones with
// the loaded monsters' stats, their areas and clear cells, and a line of
// a monster that no longer loads is left out with a warning (decision
// 0042).
func TestZoneSpawns(t *testing.T) {
	var buf strings.Builder
	log := slog.New(slog.NewTextHandler(&buf, nil))
	monsters, err := gamedata.LoadMonsters(reference.Renewal, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	proc := proceduralWorld{spawns: map[string][]spawns.Spawn{
		"gen:field:0:0": {
			{Monster: "PORING", Count: 40, RespawnMs: 5000, Role: spawns.Common, Avoid: []spawns.Avoid{{X: 10, Y: 20, R: 15}}},
			{Monster: "example.gone:GHOST", Count: 3, RespawnMs: 5000, Role: spawns.Uncommon},
			{Monster: "RED_PLANT", Count: 4, RespawnMs: 600000, Role: spawns.Plant, Area: spawns.Area{X: 30, Y: 40, W: 8, H: 8}},
		},
	}}
	to := map[string][]zone.Spawn{"gen:field:0:0": {{Mob: 1, Amount: 1}}}
	zoneSpawns(proc, monsters, to, log)
	got := to["gen:field:0:0"]
	if len(got) != 3 {
		t.Fatalf("spawns %+v", got)
	}
	poring, plant := got[1], got[2]
	if poring.Mob != 1002 || poring.Amount != 40 || poring.RespawnMs != 5000 || poring.Status == nil ||
		len(poring.Avoid) != 1 || poring.Avoid[0] != (zone.Clear{X: 10, Y: 20, R: 15}) || poring.X != 0 || poring.Y != 0 {
		t.Errorf("Poring %+v", poring)
	}
	if plant.X != 30 || plant.Y != 40 || plant.W != 8 || plant.H != 8 || plant.Amount != 4 || plant.RespawnMs != 600000 {
		t.Errorf("Red Plant %+v", plant)
	}
	if !strings.Contains(buf.String(), "example.gone:GHOST") || !strings.Contains(buf.String(), "lines=2") {
		t.Errorf("log: %s", buf.String())
	}
}

// TestSpawnsConfig: waserver.yml's spawns section, which wactl world new
// reads, is checked at start (decision 0042).
func TestSpawnsConfig(t *testing.T) {
	assets := assetDir(t)
	p := writeConfig(t, `
assets: `+assets+`
dev: true
spawns:
  town_field_boss_chance: 25
  town_field_boss_role: field_boss
  density: {field: 60, dungeon: 80}
`)
	o, _, err := parseOptions("waserver all", []string{"--config", p}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validate(o, secrets{}); err != nil || *o.Spawns.TownFieldBossChance != 25 || o.Spawns.Density.Dungeon != 80 {
		t.Errorf("%+v %v", o.Spawns, err)
	}
	p = writeConfig(t, `
assets: `+assets+`
dev: true
spawns:
  town_field_boss_role: rare
`)
	if o, _, err = parseOptions("waserver all", []string{"--config", p}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := validate(o, secrets{}); err == nil || !strings.Contains(err.Error(), "town_field_boss_role") {
		t.Errorf("a bad role: %v", err)
	}
	// The economy checks and the rolled drops of decision 0046.
	p = writeConfig(t, `
assets: `+assets+`
dev: true
spawns:
  economy: false
procedural:
  drop_roll_levels: 20
`)
	if o, _, err = parseOptions("waserver all", []string{"--config", p}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := validate(o, secrets{}); err != nil || *o.Spawns.Economy || o.Procedural.dropRollLevels() != 20 {
		t.Errorf("%+v %v", o.Spawns, err)
	}
	if def := (proceduralOptions{}).dropRollLevels(); def != DefaultDropRollLevels {
		t.Errorf("drop_roll_levels defaults to %d", def)
	}
	p = writeConfig(t, `
assets: `+assets+`
dev: true
procedural:
  drop_roll_levels: -1
`)
	if o, _, err = parseOptions("waserver all", []string{"--config", p}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := validate(o, secrets{}); err == nil || !strings.Contains(err.Error(), "drop_roll_levels") {
		t.Errorf("a bad drop_roll_levels: %v", err)
	}
}
