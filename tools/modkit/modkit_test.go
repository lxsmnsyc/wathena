package modkit

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
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
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
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
	if len(results) != 8 {
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
api: 1.0
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
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
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
local UNRIPE_APPLE = items.id "Unripe_Apple"

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
    local player = world:new_player { x = 150, y = 150, items = { { RED_POTION, 2 }, { UNRIPE_APPLE, 1 } } }
    world:use_item(player, RED_POTION)
    world:use_item(player, RED_POTION)
    testing.expect(player:count_item(RED_POTION)).to_be(0)
    testing.expect(function() world:use_item(player, UNRIPE_APPLE) end).to_fail_with("not supported")
    testing.expect(function() world:use_item(player, RED_POTION) end).to_fail_with("has no Red_Potion")
  end)

  testing.case("equipment goes on and is refused", function(world)
    local KNIFE = items.id "Knife"
    local player = world:new_player { x = 150, y = 150 }
    local knife = world:equip(player, KNIFE)
    testing.expect(knife.item).to_be(KNIFE)
    testing.expect(player:get_equipped("right_hand").item).to_be(KNIFE)
    testing.expect(player:count_item(KNIFE)).to_be(1)
    testing.expect(function() world:equip(player, RED_POTION) end).to_fail_with("refused to equip Red_Potion")
    testing.expect(function() world:equip(player, KNIFE, { refine = 7 }) end).to_fail_with("the only option is roll")
    testing.expect(function() world:equip(player, KNIFE, { roll = "best" }) end).to_fail_with("roll is \"random\"")
    testing.expect(function() world:equip(player, KNIFE, { roll = { power = 1 } }) end).to_fail_with("has no roll \"power\"")
    testing.expect(function() world:equip(player, 99999999) end).to_fail_with("there is no item")
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
	if len(results) != 9 {
		t.Errorf("%d results", len(results))
	}
	if got := failures["fails at its line"]; got != "tests/fail_test.lua:5: expected 3, got 2" {
		t.Errorf("failure: %q", got)
	}
	if got := failures["asks for more answers"]; !strings.HasPrefix(got, "tests/fail_test.lua:9: world:talk: the conversation asks for a menu choice") {
		t.Errorf("failure: %q", got)
	}
}

// TestStatusAPI runs a mod's tests of the status API of proposal 0002,
// review decision 19: starting, reading and ending statuses on players
// and monsters, the status events and hooks, a mod status, and timed
// bonuses.
func TestStatusAPI(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	dir := writeMod(t, map[string]string{
		"mod.yaml": `id: example.statuses
name: Statuses
version: 1.0.0
api: 1.0
depends:
  - id: core
permissions:
  - event.listen
  - game.read
  - rules.override
scripts:
  - scripts/main.lua
`,
		"scripts/main.lua": `
local events = require "wa.events"
local hooks = require "wa.hooks"
local statuses = require "wa.statuses"
local variables = require "wa.variables"
local log = variables.server_string "log"

local function note(s)
  log:set(log:get() .. s .. ";")
end

return function()
  statuses.define {
    name = "glow",
    tick_milliseconds = 1000,
    ends_on = { "death" },
    bonuses = function(level)
      return { { "max_sp", 10 * level } }
    end,
  }
  events.on("status_start", function(ev)
    note("start " .. ev.target.kind .. " " .. ev.level .. " " .. ev.reason .. " " .. tostring(ev.milliseconds))
  end, { status = statuses.blessing })
  events.on("status_end", function(ev)
    note("end " .. ev.target.kind .. " " .. ev.status .. " " .. ev.reason)
  end, { target_kind = "monster" })
  events.on("status_tick", function(ev)
    note("tick " .. ev.level)
  end, { status = statuses.glow })
  -- A handler reads the stats a status changes in the same call.
  events.on("status_start", function(ev)
    local player = ev.target
    local sp = player.max_sp
    player:start_status(statuses.glow, { milliseconds = 60000, level = 3 })
    local glowing = player.max_sp - sp
    player:end_status(statuses.glow)
    note("glow +" .. glowing .. " then +" .. (player.max_sp - sp))
  end, { status = statuses.increaseagi, target_kind = "player" })
  hooks.on("status_duration", function(h, next)
    if h.status == statuses.decreaseagi then
      return 1234
    end
    return next()
  end)
  hooks.on("status_chance", function(h, next)
    if h.status == statuses.curse then
      return 0
    end
    return next()
  end)
end`,
		"tests/status_test.lua": `
local testing = require "wa.testing"
local statuses = require "wa.statuses"
local monsters = require "wa.monsters"
local random = require "wa.random"
local variables = require "wa.variables"
local log = variables.server_string "log"

return function()
  testing.case("start, read and end a status", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    testing.expect(player:start_status(statuses.blessing, { milliseconds = 10000, level = 10 })).to_be_true()
    testing.expect(player:has_status(statuses.blessing)).to_be_true()
    local s = player:get_status(statuses.blessing)
    testing.expect(s.status).to_be(statuses.blessing)
    testing.expect(s.level).to_be(10)
    testing.expect(s.remaining_milliseconds).to_be(10000)
    testing.expect(s.source).to_be_nil()
    testing.expect(#player:get_statuses()).to_be(1)
    -- Events of a script's start run after the script's call.
    world:advance(50)
    testing.expect(log:get()).to_contain("start player 10 script 10000;")
    world:advance(3950)
    testing.expect(player:get_status(statuses.blessing).remaining_milliseconds).to_be(6000)
    testing.expect(player:end_status(statuses.blessing)).to_be_true()
    testing.expect(player:end_status(statuses.blessing)).to_be_false()
    testing.expect(player:get_status(statuses.blessing)).to_be_nil()
  end)

  testing.case("forever and values", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    player:start_status(statuses.strfood, { milliseconds = "forever", level = 5, values = { 1, 2, 3 } })
    local s = player:get_status(statuses.strfood)
    testing.expect(s.remaining_milliseconds).to_be("forever")
    testing.expect({ s.values[1], s.values[2], s.values[3] }).to_equal({ 1, 2, 3 })
  end)

  testing.case("hooks change the duration and the chance", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    player:start_status(statuses.decreaseagi, { milliseconds = 5000, cannot_be_avoided = true })
    testing.expect(player:get_status(statuses.decreaseagi).remaining_milliseconds).to_be(1234)
    testing.expect(player:start_status(statuses.curse, { milliseconds = 5000 })).to_be_false()
    testing.expect(player:has_status(statuses.curse)).to_be_false()
  end)

  testing.case("a monster's status, with its source", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    local poring = world:spawn_monster { monster = monsters.id "PORING", x = 152, y = 150 }
    testing.expect(poring:start_status(statuses.stun, { milliseconds = 3000, source = player, cannot_be_avoided = true })).to_be_true()
    testing.expect(poring:get_status(statuses.stun).source == player).to_be_true()
    world:advance(3000)
    testing.expect(poring:has_status(statuses.stun)).to_be_false()
    testing.expect(log:get()).to_contain("end monster " .. statuses.stun .. " expired;")
  end)

  testing.case("a mod status ticks and gives its bonuses", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    local sp = player.max_sp
    player:start_status(statuses.glow, { milliseconds = 3500, level = 2 })
    -- Stats change before start_status returns.
    testing.expect(player.max_sp).to_be(sp + 20)
    world:advance(3500)
    -- Ticks at 1, 2 and 3 seconds, and the last one at the end.
    testing.expect(log:get()).to_be("tick 2;tick 2;tick 2;tick 2;")
    testing.expect(player:has_status(statuses.glow)).to_be_false()
    testing.expect(player.max_sp).to_be(sp)
  end)

  testing.case("an event handler reads the stats of its statuses", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    player:start_status(statuses.increaseagi, { milliseconds = 10000, level = 1 })
    -- The handler runs after this call.
    testing.expect(log:get()).to_be("")
    world:advance(50)
    testing.expect(log:get()).to_be("glow +30 then +0;")
  end)

  testing.case("timed bonuses keep the first copy by default", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    local hp = player.max_hp
    testing.expect(player:add_timed_bonuses("tonic", 5000, { { "max_hp", 100 } })).to_be_true()
    -- Stats change before the call returns.
    testing.expect(player.max_hp).to_be(hp + 100)
    -- A second use does not draw its value again, as rAthena's bonus_script.
    local roll = random.integer(200, 300)
    testing.expect(player:add_timed_bonuses("tonic", 9000, { { "max_hp", roll } })).to_be_false()
    testing.expect(player.max_hp).to_be(hp + 100)
    world:advance(5000)
    testing.expect(player.max_hp).to_be(hp)
  end)

  testing.case("timed bonuses extend: the first values, the longer time", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    local hp = player.max_hp
    player:add_timed_bonuses("tonic", 5000, { { "max_hp", 100 } })
    testing.expect(player:add_timed_bonuses("tonic", 9000, { { "max_hp", 300 } }, { on_repeat = "extend" })).to_be_true()
    testing.expect(player.max_hp).to_be(hp + 100)
    -- A shorter time is ignored.
    testing.expect(player:add_timed_bonuses("tonic", 1000, { { "max_hp", 300 } }, { on_repeat = "extend" })).to_be_false()
    world:advance(5000)
    testing.expect(player.max_hp).to_be(hp + 100)
    world:advance(4000)
    testing.expect(player.max_hp).to_be(hp)
  end)

  testing.case("timed bonuses add a second copy", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    local hp = player.max_hp
    player:add_timed_bonuses("tonic", 5000, { { "max_hp", 100 } })
    testing.expect(player:add_timed_bonuses("tonic", 2000, { { "max_hp", 50 } }, { on_repeat = "add", kind = "buff" })).to_be_true()
    testing.expect(player.max_hp).to_be(hp + 150)
    world:advance(2000)
    testing.expect(player.max_hp).to_be(hp + 100)
    world:advance(3000)
    testing.expect(player.max_hp).to_be(hp)
  end)

  testing.case("timed bonuses replace: the new values and time", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    local hp = player.max_hp
    player:add_timed_bonuses("tonic", 5000, { { "max_hp", 100 } })
    player:add_timed_bonuses("tonic", 5000, { { "max_hp", 100 } }, { on_repeat = "add" })
    testing.expect(player:add_timed_bonuses("tonic", 2000, { { "max_hp", 50 } }, { on_repeat = "replace" })).to_be_true()
    testing.expect(player.max_hp).to_be(hp + 50)
    world:advance(2000)
    testing.expect(player.max_hp).to_be(hp)
  end)

  testing.case("bad options raise errors", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    testing.expect(function() player:start_status(statuses.blessing, { milliseconds = 0 }) end).to_fail_with("milliseconds must be from 1")
    testing.expect(function() player:start_status(statuses.blessing, { chance = 20000 }) end).to_fail_with("chance must be from 0 to 10000")
    testing.expect(function() player:start_status(statuses.blessing, { bogus = 1 }) end).to_fail_with("is not an option")
    testing.expect(function() player:start_status(99999999) end).to_fail_with("there is no status")
    testing.expect(function() player:add_timed_bonuses("k", 1000, { { "no_such_bonus", 1 } }) end).to_fail_with("no_such_bonus")
    testing.expect(function() player:add_timed_bonuses("k", 1000, {}, { ends_on = { "sunrise" } }) end).to_fail_with("sunrise")
    testing.expect(function() player:add_timed_bonuses("k", 1000, {}, { on_repeat = "twice" }) end).to_fail_with("on_repeat is")
    testing.expect(function() player:add_timed_bonuses("k", 1000, {}, { stack = true }) end).to_fail_with("on_repeat replaces it")
    testing.expect(function() player:add_timed_bonuses("k", 1000, {}, { ["repeat"] = "add" }) end).to_fail_with("on_repeat replaces it")
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
	if len(results) != 11 {
		t.Errorf("%d results", len(results))
	}
	for _, r := range results {
		if r.Failed() {
			t.Errorf("%s: %v %v", r.Name, r.Err, r.Logs)
		}
	}
}
