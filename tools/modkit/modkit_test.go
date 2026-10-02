package modkit

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/mod"
)

var testEnv = sync.OnceValues(func() (*Env, error) { return LoadEnv(reference.Renewal, "") })

func env(t *testing.T) *Env {
	t.Helper()
	e, err := testEnv()
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// TestGreeterTests runs the sample mod's tests.
func TestGreeterTests(t *testing.T) {
	e := env(t)
	l, err := e.Load(context.Background(), "../../mods/examples/greeter", "", mod.Renewal, nil)
	if err != nil {
		t.Fatal(err)
	}
	results, err := e.RunTests(context.Background(), l, "", func(r CaseResult) {
		t.Logf("%s: %s: %v %v (%s)", r.File, r.Name, r.Err, r.Logs, r.Took)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 4 {
		t.Fatalf("%d results", len(results))
	}
	for _, r := range results {
		if r.Failed() {
			t.Errorf("%s: %v %v", r.Name, r.Err, r.Logs)
		}
	}
}

// writeMod writes a mod's files into a temporary folder.
func writeMod(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, s := range files {
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const testManifest = `id: example.tested
name: Tested
version: 1.0.0
api: experimental
depends:
  - id: core
permissions:
  - player.give_items
scripts:
  - scripts/main.lua
`

// TestWorldMethods runs a mod's tests that use every world method:
// core's converted Kafra, timers on the zone clock, a monster kill,
// items, and checks that fail on purpose.
func TestWorldMethods(t *testing.T) {
	dir := writeMod(t, map[string]string{
		"mod.yaml": testManifest,
		"scripts/main.lua": `
local events = require "wa.events"
local timers = require "wa.timers"
local variables = require "wa.variables"
local ticks = variables.server_integer "ticks"
local kills = variables.character_integer "kills"
return function()
  events.on("login", function(player)
    timers.after(5000, function() ticks:add(1) end)
  end)
  events.on("death", function(death)
    kills:add(death.killer, 1)
  end, { victim_kind = "monster" })
end`,
		"tests/world_test.lua": `
local testing = require "wa.testing"
local items = require "wa.items"
local monsters = require "wa.monsters"
local variables = require "wa.variables"
local ticks = variables.server_integer "ticks"
local kills = variables.character_integer "kills"
local RED_POTION = items.id "Red_Potion"
local GREEN_POTION = items.id "Green_Potion"

return function()
  testing.case("the kafra saves and charges for a teleport", function(world)
    local player = world:new_player { map = "prontera", x = 153, y = 326, zeny = 5000 }
    local said = world:talk(player, "core:kaf_prontera", { 1 })
    testing.expect(said).to_contain("has been saved here")
    world:talk(player, "core:kaf_prontera", { 3, 1 })
    testing.expect(player.zeny).to_be(4400)
  end)

  testing.case("timers follow the zone clock", function(world)
    world:new_player { map = "prontera", x = 150, y = 150 }
    world:advance(4950)
    testing.expect(ticks:get()).to_be(0)
    world:advance(100)
    testing.expect(ticks:get()).to_be(1)
  end)

  testing.case("a kill fires death", function(world)
    local player = world:new_player { x = 150, y = 150 }
    local poring = world:spawn_monster { monster = monsters.id "PORING", x = 152, y = 150 }
    testing.expect(poring.is_alive).to_be_true()
    world:kill(player, poring)
    testing.expect(function() return poring.is_alive end).to_fail_with("gone")
    testing.expect(kills:get(player)).to_be(1)
    testing.expect(function() world:kill(player, poring) end).to_fail_with("dead or gone")
  end)

  testing.case("items are used up and refused", function(world)
    local player = world:new_player { x = 150, y = 150, items = { { RED_POTION, 2 }, { GREEN_POTION, 1 } } }
    world:use_item(player, RED_POTION)
    world:use_item(player, RED_POTION)
    testing.expect(player:count_item(RED_POTION)).to_be(0)
    testing.expect(function() world:use_item(player, GREEN_POTION) end).to_fail_with("not supported")
    testing.expect(function() world:use_item(player, RED_POTION) end).to_fail_with("has no Red_Potion")
  end)

  testing.case("expectations", function(world)
    testing.expect({ a = { 1, 2 } }).to_equal({ a = { 1, 2 } })
    testing.expect({ 1, { x = 2 } }).to_contain({ x = 2 })
    testing.expect(nil).to_be_nil()
    testing.expect(function() error("boom") end).to_fail_with("boom")
    testing.expect(function() testing.expect(1).to_be(2) end).to_fail_with("expected 2, got 1")
  end)

  testing.case("a world runs one map", function(world)
    world:new_player { map = "prontera", x = 150, y = 150 }
    testing.expect(function() world:new_player { map = "izlude", x = 1, y = 1 } end).to_fail_with("runs one map")
  end)
end`,
		"tests/fail_test.lua": `
local testing = require "wa.testing"
return function()
  testing.case("fails at its line", function(world)
    testing.expect(1 + 1).to_be(3)
  end)
  testing.case("asks for more answers", function(world)
    local player = world:new_player { x = 153, y = 326 }
    world:talk(player, "core:kaf_prontera", {})
  end)
end`,
	})
	e := env(t)
	l, err := e.Load(context.Background(), dir, "", mod.Renewal, nil)
	if err != nil {
		t.Fatal(err)
	}
	results, err := e.RunTests(context.Background(), l, "", func(CaseResult) {})
	if err != nil {
		t.Fatal(err)
	}
	failures := map[string]string{}
	for _, r := range results {
		if r.File == "tests/fail_test.lua" {
			failures[r.Name] = fmt.Sprint(r.Err)
			continue
		}
		if r.Failed() {
			t.Errorf("%s: %v %v", r.Name, r.Err, r.Logs)
		}
	}
	if len(results) != 8 {
		t.Errorf("%d results", len(results))
	}
	if got := failures["fails at its line"]; got != "tests/fail_test.lua:5: expected 3, got 2" {
		t.Errorf("failure: %q", got)
	}
	if got := failures["asks for more answers"]; !strings.HasPrefix(got, "tests/fail_test.lua:9: world:talk: the conversation asks for a menu choice") {
		t.Errorf("failure: %q", got)
	}
}
