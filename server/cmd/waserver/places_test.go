package main

import (
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/fields"
	gencore "github.com/lxsmnsyc/wathena/server/fields/core"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// coreRecords returns core's dungeon styles and town templates, as the
// mod loader gives them.
func coreRecords(t *testing.T) ([]fields.Style, []fields.Template) {
	t.Helper()
	read := func(file string) []map[string]any {
		b, err := gencore.Files.ReadFile("data/" + file)
		if err != nil {
			t.Fatal(err)
		}
		var out []map[string]any
		if err := yaml.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	var styles []fields.Style
	for _, r := range read("dungeon_styles.yml") {
		name, _ := r["name"].(string)
		delete(r, "name")
		s, err := fields.ParseStyle(r)
		if err != nil {
			t.Fatal(err)
		}
		styles = append(styles, fields.Style{Name: name, Mod: mod.Core, Style: s})
	}
	var templates []fields.Template
	for _, r := range read("town_templates.yml") {
		name, _ := r["name"].(string)
		delete(r, "name")
		tp, err := fields.ParseTemplate(r)
		if err != nil {
			t.Fatal(err)
		}
		templates = append(templates, fields.Template{Name: name, Mod: mod.Core, Template: tp})
	}
	return styles, templates
}

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

// TestAddMaps: --towns and --dungeons make a procedural dev world with
// towns and dungeon floors, which link to the field, and NPCs placed by
// tag stand at their places (decision 0040).
func TestAddMaps(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	styles, templates := coreRecords(t)
	cfg := settings{options: defaultOptions()}
	cfg.Dev = true
	cfg.Procedural.FieldSize = 64
	cfg.Procedural.Towns = 2
	cfg.Procedural.Dungeons = list{"crypt:2"}

	// A classic world cannot run towns.
	classic := proceduralWorld{}
	if err := addMaps(cfg, &classic, styles, templates, log); err == nil || !strings.Contains(err.Error(), "procedural world") {
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
	if len(p.set.Fields()) != 1 {
		t.Fatalf("%d fields, want 0:0 alone", len(p.set.Fields()))
	}
	if err := addMaps(cfg, &p, styles, templates, log); err != nil {
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
	warps, unlinked := p.set.DevLinks()
	if len(unlinked) != 0 || len(warps["gen:town:t1"]) == 0 || len(warps["gen:dungeon:crypt:2"]) != 1 {
		t.Errorf("links %v, unlinked %v", warps, unlinked)
	}
	idx := p.set.Places(warps)
	scope, _ := places.ParseScope("town:any")
	smith := gamedata.PlaceNPC{NPC: zone.NPC{Name: "example.a:Smith", Sprite: 1}, Place: mod.NPCPlace{Scope: scope, Tag: "blacksmith"}}
	none := gamedata.PlaceNPC{NPC: zone.NPC{Name: "example.a:Nobody"}, Place: mod.NPCPlace{Scope: scope, Tag: "moon"}}
	npcs := placeNPCs(idx, []gamedata.PlaceNPC{smith, none}, log)
	for _, town := range []string{"gen:town:t1", "gen:town:t2"} {
		list := npcs[town]
		if len(list) != 1 || list[0].Name != "example.a:Smith" {
			t.Fatalf("%s: %+v", town, list)
		}
		pl := idx.Place(idx.Find(scope, "blacksmith")[slices.Index([]string{"gen:town:t1", "gen:town:t2"}, town)])
		if list[0].X != pl.X || list[0].Y != pl.Y {
			t.Errorf("%s: the smith stands at %d,%d, not at %d,%d", town, list[0].X, list[0].Y, pl.X, pl.Y)
		}
	}
	// A style that does not exist, and two dungeons with one ID.
	p, _ = fieldMaps(cfg, store.World{}, log)
	cfg.Procedural.Dungeons = list{"moon"}
	if err := addMaps(cfg, &p, styles, templates, log); err == nil || !strings.Contains(err.Error(), `no dungeon style "moon"`) {
		t.Errorf("an unknown style: %v", err)
	}
	cfg.Procedural.Dungeons = list{"crypt", "crypt:2"}
	if err := addMaps(cfg, &p, styles, templates, log); err == nil || !strings.Contains(err.Error(), "two dungeons") {
		t.Errorf("one ID twice: %v", err)
	}
	cfg.Procedural.Dungeons = nil
	if err := addMaps(cfg, &p, styles, nil, log); err == nil || !strings.Contains(err.Error(), "town template") {
		t.Errorf("towns without templates: %v", err)
	}
}
