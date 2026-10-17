package modscript

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestGeneration: generation.on("map") layers get the map and their
// node, and return places, objects and bosses; the node's values repeat
// for the world and differ by mod and map; wa.random raises an error in
// a layer; a layer that fails or returns a bad result is left out; and
// both calls need world.generate (review decision 46).
func TestGeneration(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h,
		modScripts("example.a", []string{"world.generate"}, nil, "scripts/main.lua", `
local generation = require "wa.generation"
local monsters = require "wa.monsters"
return function()
  generation.on("map", function(map, node)
    if map.kind ~= "field" or map.biome ~= "forest" or map.tileset ~= "forest" or map.band[1] ~= 9 or map.band[2] ~= 18
      or map.width ~= 64 or map.height ~= 48 or map.places[1].tags[1] ~= "clearing" or map.places[1].x ~= 20 then
      error("wrong map")
    end
    local s = node:child("stones"):stream()
    local again = node:child("stones"):stream()
    local a, b = s:integer(1, 1000000), again:integer(1, 1000000)
    if a ~= b then error("two streams of one node differ") end
    return {
      places = { { tags = { "shrine", "shrine:moss" }, x = 20, y = 30 } },
      objects = { { model = "rock", x = 21, y = 30, turn = s:integer(0, 3) } },
      bosses = { { monster = monsters.id("PORING"), role = "mini_boss", x = 22, y = 30 },
        { monster = monsters.id("PORING"), role = "field_boss", respawn = 60000 } },
      hash = nil,
    }
  end)
  generation.quest_template { name = "pilgrimage", kind = "delivery", title = "Walk to {town}", reward = 40 }
end`),
		modScripts("example.b", []string{"world.generate"}, nil, "scripts/main.lua", `
local generation = require "wa.generation"
local random = require "wa.random"
return function()
  -- A layer that draws from wa.random, one that fails, and one that
  -- returns a bad result are left out. The last returns nothing.
  generation.on("map", function(map, node) return { places = { { tags = { "x" }, x = random.integer(1, 2), y = 1 } } } end)
  generation.on("map", function(map, node) error("broken layer") end)
  generation.on("map", function(map, node) return { objects = { { model = "rock", x = -1, y = 2 } } } end)
  generation.on("map", function(map, node)
    if node:hash(3, 4) ~= node:hash(3, 4) or node:hash(3, 4) == node:hash(4, 3) then error("hash") end
    return nil
  end)
  local ok, err = pcall(generation.quest_template, { name = "bad", kind = "hunt", title = "Hunt {nothing}" })
  if ok then error("a bad template passed") end
end`),
		modScripts("example.c", nil, nil, "scripts/main.lua", `
local generation = require "wa.generation"
return function()
  generation.on("map", function(map, node) return nil end)
end`))
	if !r.HasGeneration() {
		t.Fatal("no generation")
	}
	m := content.LayerMap{ID: "gen:field:1:2", Kind: gen.KindField, Biome: "forest", Tileset: "forest", Band: [2]int{9, 18},
		Width: 64, Height: 48, Places: []gen.Place{{Tags: []string{"clearing"}, X: 20, Y: 20}}}
	secret, _ := rng.NewRoot([]byte("generation-test"))
	names := func(id int64) (string, bool) { return "PORING", id == 1002 }
	out, notes := r.RunGeneration(m, secret, names)
	if len(out) != 2 || out[0].Mod != "example.a" || out[1].Mod != "example.b" {
		t.Fatalf("results %+v, notes %v, logs %s", out, notes, logs)
	}
	a := out[0]
	if len(a.Places) != 1 || !reflect.DeepEqual(a.Places[0], content.LayerPlace{Tags: []string{"shrine", "shrine:moss"}, X: 20, Y: 30}) {
		t.Errorf("places %+v", a.Places)
	}
	if len(a.Objects) != 1 || a.Objects[0].Model != "rock" || a.Objects[0].X != 21 || a.Objects[0].Turn < 0 || a.Objects[0].Turn > 3 {
		t.Errorf("objects %+v", a.Objects)
	}
	want := []content.LayerBoss{{Monster: "PORING", Role: "mini_boss", X: 22, Y: 30, HasCell: true}, {Monster: "PORING", Role: "field_boss", RespawnMs: 60000}}
	if !reflect.DeepEqual(a.Bosses, want) {
		t.Errorf("bosses %+v", a.Bosses)
	}
	// The same world gives the same result. Another map or world gives
	// another stream.
	again, _ := r.RunGeneration(m, secret, names)
	if !reflect.DeepEqual(again, out) {
		t.Errorf("a second run differs: %+v", again)
	}
	other, _ := rng.NewRoot([]byte("generation-test-2"))
	turns := map[int]bool{}
	for i := range 16 {
		m2 := m
		m2.ID = gen.FieldID(int64(i), 0)
		res, _ := r.RunGeneration(m2, other, names)
		turns[res[0].Objects[0].Turn] = true
	}
	if len(turns) < 2 {
		t.Errorf("16 maps drew the turns %v", turns)
	}
	got := strings.Join(notes, "\n") + logs.String()
	for _, w := range []string{"wa.random does not run in a generation layer", "broken layer", "x and y must be integers from 0 to 1024",
		"generation.on needs the permission world.generate"} {
		if !strings.Contains(got, w) {
			t.Errorf("notes and logs lack %q: %s", w, got)
		}
	}
	tps := r.GenerationTemplates()
	if len(tps) != 1 || tps[0].Name != "example.a:pilgrimage" || tps[0].Kind != content.KindDelivery || tps[0].Reward != 40 {
		t.Errorf("templates %+v", tps)
	}
}
