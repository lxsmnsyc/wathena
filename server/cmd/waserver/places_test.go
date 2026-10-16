package main

import (
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// TestTownsConfig: --towns and --dungeons, from the file and the flags,
// and their checks.
func TestTownsConfig(t *testing.T) {
	assets := assetDir(t)
	p := writeConfig(t, `
assets: `+assets+`
dev: true
procedural:
  towns: 2
  dungeons: ["crypt:3", cave]
`)
	o, _, err := parseOptions("waserver all", []string{"--config", p, "--towns", "4"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if o.Procedural.Towns != 4 || !slices.Equal(o.Procedural.Dungeons, []string{"crypt:3", "cave"}) {
		t.Errorf("procedural %+v", o.Procedural)
	}
	if _, err := validate(o, secrets{}); err != nil {
		t.Error(err)
	}
	o, _, err = parseOptions("waserver all", []string{"--assets", assets, "--dev", "--towns", "100", "--dungeons", "crypt:0"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	_, err = validate(o, secrets{})
	for _, want := range []string{"procedural.towns", "procedural.dungeons"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("no %s problem in %v", want, err)
		}
	}
}

// coreSet is the mod set of core's records alone.
func coreSet(t *testing.T) *mod.Set {
	t.Helper()
	core, err := fields.LayoutCore()
	if err != nil {
		t.Fatal(err)
	}
	set, err := mod.Resolve(nil, mod.Config{Core: core, Base: mod.Renewal})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

// TestAddMaps: --towns and --dungeons make a procedural dev world with
// towns and dungeon floors, which link to the field through the world
// layout's code, NPCs placed by tag stand at their places (decision
// 0040), and the teleport service stands at each town's teleport place
// (decision 0041).
func TestAddMaps(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	set := coreSet(t)
	cfg := settings{options: defaultOptions()}
	cfg.Dev = true
	cfg.Procedural.FieldSize = 64
	cfg.Procedural.Towns = 2
	cfg.Procedural.Dungeons = list{"crypt:2"}

	// A classic world cannot run towns.
	classic := proceduralWorld{}
	if err := addMaps(cfg, &classic, set, nil, log); err == nil || !strings.Contains(err.Error(), "procedural world") {
		t.Errorf("towns in a classic world: %v", err)
	}
	p, err := fieldMaps(cfg, store.World{}, log)
	if !gen.Procedural {
		if err == nil {
			t.Error("towns without the tag")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := addMaps(cfg, &p, set, nil, log); err != nil {
		t.Fatal(err)
	}
	start, _, all, err := worldMaps(cfg, p)
	want := []string{"gen:dungeon:crypt:1", "gen:dungeon:crypt:2", "gen:field:0:0", "gen:town:t1", "gen:town:t2"}
	if err != nil || start != "gen:field:0:0" || !slices.Equal(all, want) {
		t.Fatalf("maps %s %v %v", start, all, err)
	}
	if p.set.Get("gen:town:t1").Name != "hamlet" || p.set.Get("gen:town:t2").Name != "village" {
		t.Error("the towns do not take the templates in order")
	}
	if len(p.warps["gen:town:t1"]) == 0 || len(p.warps["gen:dungeon:crypt:2"]) != 1 {
		t.Errorf("links %v", p.warps)
	}
	if n, _ := p.atlas.Name("gen:town:t1"); n == "" || n == "gen:town:t1" {
		t.Errorf("t1's name %q", n)
	}
	idx := p.set.Places(p.warps)
	scope, _ := places.ParseScope("town:any")
	smith := gamedata.PlaceNPC{NPC: zone.NPC{Name: "example.a:Smith", Sprite: 1}, Place: mod.NPCPlace{Scope: scope, Tag: "blacksmith"}}
	none := gamedata.PlaceNPC{NPC: zone.NPC{Name: "example.a:Nobody"}, Place: mod.NPCPlace{Scope: scope, Tag: "moon"}}
	npcs := placeNPCs(idx, []gamedata.PlaceNPC{smith, none}, log)
	teleports := zone.TeleportNPCs(p.atlas, idx, teleportSprite)
	for _, town := range []string{"gen:town:t1", "gen:town:t2"} {
		list := npcs[town]
		if len(list) != 1 || list[0].Name != "example.a:Smith" {
			t.Fatalf("%s: %+v", town, list)
		}
		pl := idx.Place(idx.Find(scope, "blacksmith")[slices.Index([]string{"gen:town:t1", "gen:town:t2"}, town)])
		if list[0].X != pl.X || list[0].Y != pl.Y {
			t.Errorf("%s: the smith stands at %d,%d, not at %d,%d", town, list[0].X, list[0].Y, pl.X, pl.Y)
		}
		if tp := teleports[town]; len(tp) != 1 || tp[0].Service != zone.ServiceTeleport || tp[0].DisplayName != zone.TeleportNPCName {
			t.Errorf("%s: teleport service %+v", town, tp)
		}
	}
	// A style that does not exist, and two dungeons with one ID.
	p, _ = fieldMaps(cfg, store.World{}, log)
	cfg.Procedural.Dungeons = list{"moon"}
	if err := addMaps(cfg, &p, set, nil, log); err == nil || !strings.Contains(err.Error(), `no dungeon style "moon"`) {
		t.Errorf("an unknown style: %v", err)
	}
	cfg.Procedural.Dungeons = list{"crypt", "crypt:2"}
	if err := addMaps(cfg, &p, set, nil, log); err == nil || !strings.Contains(err.Error(), "two dungeons") {
		t.Errorf("one ID twice: %v", err)
	}
	// --layout builds a whole layout as wactl world new does.
	cfg.Procedural.Towns, cfg.Procedural.Dungeons, cfg.Procedural.DevLayout = 0, nil, true
	cfg.Procedural.FieldSize = 48
	towns, dungeons := 2, 1
	cfg.Procedural.Layout = layout.Options{Grid: []int{2, 2}, Towns: &towns, Dungeons: &dungeons}
	p, err = fieldMaps(cfg, store.World{}, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := addMaps(cfg, &p, set, nil, log); err != nil {
		t.Fatal(err)
	}
	if len(p.atlas.Towns()) != 2 || p.start != "gen:town:t1" || p.set.Get("gen:field:1:1") == nil {
		t.Errorf("dev layout: %d towns, start %s", len(p.atlas.Towns()), p.start)
	}
}
