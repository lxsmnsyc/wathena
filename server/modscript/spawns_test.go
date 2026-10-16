package modscript

import (
	"strings"
	"testing"
)

// TestSpawnTable: the spawn_table hook gets the map and a copy of its
// list, next runs the rest of the chain, priority orders the hooks, a
// hook that fails or returns a bad list is left out, and the hook needs
// spawns.modify (review decision 44).
func TestSpawnTable(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h,
		modScripts("example.a", []string{"spawns.modify"}, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
local monsters = require "wa.monsters"
return function()
  hooks.on("spawn_table", function(map, spawns, next)
    if map.kind ~= "field" or map.biome ~= "forest" or map.band[1] ~= 9 or map.band[2] ~= 18 or map.features[1] ~= "water" then
      error("wrong map")
    end
    -- The list is a copy: changing it changes only what this hook passes on.
    spawns[1].count = spawns[1].count + 5
    local inner = next(spawns)
    table.insert(inner, { monster = monsters.id("PORING"), count = 3, respawn = 60000, role = "rare",
      area = { x = 10, y = 20, w = 4, h = 4 } })
    return inner
  end, { priority = 1 })
end`),
		modScripts("example.b", []string{"spawns.modify"}, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
return function()
  -- Runs inside a's hook: a passes its changed list.
  hooks.on("spawn_table", function(map, spawns, next)
    if spawns[1].count ~= 15 then error("not a's list") end
    local out = next()
    out[1].role = "uncommon"
    return out
  end)
  -- A hook that fails, and one that returns a bad list, are left out.
  hooks.on("spawn_table", function(map, spawns, next) error("broken spawn hook") end, { priority = -1 })
  hooks.on("spawn_table", function(map, spawns, next)
    return { { monster = 1002, count = 0, respawn = 5000, role = "common" } }
  end, { priority = -2 })
end`),
		modScripts("example.c", nil, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
return function()
  hooks.on("spawn_table", function(map, spawns, next) return {} end)
end`))
	m := SpawnMap{ID: "gen:field:1:2", Kind: "field", Biome: "forest", Band: [2]int{9, 18}, Features: []string{"water"}}
	list := []SpawnEntry{{Monster: 1002, Count: 10, Respawn: 5000, Role: "common"}}
	known := func(id int64) bool { return id == 1002 }
	out, ok := r.RunSpawnTable(m, list, known)
	if !ok || len(out) != 2 {
		t.Fatalf("got %+v %v, logs: %s", out, ok, logs)
	}
	if out[0].Monster != 1002 || out[0].Count != 15 || out[0].Role != "uncommon" || out[0].Area != nil {
		t.Errorf("the first spawn %+v", out[0])
	}
	if a := out[1].Area; out[1].Role != "rare" || out[1].Respawn != 60000 || a == nil || *a != (SpawnArea{10, 20, 4, 4}) {
		t.Errorf("the added spawn %+v %+v", out[1], out[1].Area)
	}
	if list[0].Count != 10 {
		t.Error("the hook changed the caller's list")
	}
	got := logs.String()
	for _, want := range []string{"broken spawn hook", "count must be an integer from 1 to 1000", `hooks.on("spawn_table") needs the permission spawns.modify`} {
		if !strings.Contains(got, want) {
			t.Errorf("logs lack %q: %s", want, got)
		}
	}
	// An unknown monster fails the hook that returns it.
	out, ok = r.RunSpawnTable(m, list, func(int64) bool { return false })
	if ok || len(out) != 1 || out[0] != list[0] {
		t.Errorf("unknown monsters: %+v %v", out, ok)
	}
	if !strings.Contains(logs.String(), "monster must be a monster ID") {
		t.Errorf("logs: %s", logs)
	}
}

// TestSpawnTableRules: the hook is a mod context's only, and without
// hooks the list stays.
func TestSpawnTableRules(t *testing.T) {
	h := newHost()
	r, _ := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `return function() end`))
	list := []SpawnEntry{{Monster: 1002, Count: 1, Respawn: 5000, Role: "common"}}
	if out, ok := r.RunSpawnTable(SpawnMap{}, list, nil); ok || len(out) != 1 {
		t.Errorf("no hooks: %+v %v", out, ok)
	}
	if kinds := Hooks["spawn_table"]; len(kinds) != 1 || kinds[0] != KindMod {
		t.Errorf("kinds %v", kinds)
	}
}
