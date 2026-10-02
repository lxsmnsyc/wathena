package modscript

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
)

// spawnHost is a fakeHost that spawns monsters and has a server clock.
type spawnHost struct {
	*fakeHost
	clock   time.Time
	next    int64
	owners  map[int64]int64 // monster handle -> owner context
	spawned []SpawnSpec
}

func (h *spawnHost) SpawnMonsters(owner int64, s SpawnSpec) ([]int64, error) {
	h.spawned = append(h.spawned, s)
	var ids []int64
	for range s.Amount {
		h.next++
		id := 100 + h.next
		h.owners[id] = owner
		h.monsters[id] = fmt.Sprintf("Spawned %d", s.Monster)
		ids = append(ids, id)
	}
	return ids, nil
}

func (h *spawnHost) Despawn(owner int64) {
	for id, o := range h.owners {
		if o == owner {
			delete(h.owners, id)
			delete(h.monsters, id)
		}
	}
}

func newWorldRuntime(t *testing.T, loc *time.Location, mods ...*mod.Scripts) (*Runtime, *spawnHost, *logBuffer) {
	t.Helper()
	h := &spawnHost{fakeHost: newHost(), clock: time.Date(2026, 10, 31, 19, 58, 30, 0, time.UTC), owners: map[int64]int64{}}
	var buf logBuffer
	var ids []string
	for _, m := range mods {
		ids = append(ids, m.Mod)
	}
	r, err := New(Options{Host: h, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: ids,
		Clock: func() time.Time { return h.clock }, Location: loc}, mods)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, h, &buf
}

func TestSchedules(t *testing.T) {
	r, h, logs := newWorldRuntime(t, time.UTC, modScripts("example.a", nil, nil, "scripts/main.lua", `
local schedules = require "wa.schedules"
local events = require "wa.events"
local players = {}
return function()
  events.on("login", function(player) players[#players + 1] = player end)
  local function say(text) for _, p in ipairs(players) do p:send_message(text) end end
  schedules.daily("20:00", function() say("daily") end)
  schedules.weekly("saturday", "20:01", function() say("weekly") end)
  local hourly = schedules.hourly(2, function() say("hourly") end)
  schedules.hourly(3, function() hourly:cancel() end)
  local open = false
  schedules.when(function() return open end, function() say("when") end, { lasting_minutes = 2 })
  schedules.daily("20:05", function() open = true end)
end`))
	r.Fire("login", Player(1))
	step := func(to time.Time) {
		for h.clock.Before(to) {
			h.clock = h.clock.Add(30 * time.Second)
			r.RunSchedules(h.clock)
		}
	}
	step(time.Date(2026, 10, 31, 21, 3, 0, 0, time.UTC))
	// 2026-10-31 is a Saturday. hourly at :02 runs at 20:02 and is
	// cancelled at 20:03. when holds from 20:06 and runs 2 minutes later.
	want := "1:daily|1:weekly|1:hourly|1:when"
	if got := strings.Join(h.messages, "|"); got != want {
		t.Fatalf("messages = %q, want %q, logs: %s", got, want, logs)
	}
	if next, ok := r.NextSchedule(); !ok || next.After(time.Date(2026, 10, 31, 21, 4, 0, 0, time.UTC)) {
		t.Fatalf("next schedule %v %v", next, ok)
	}
}

func TestScheduleTimeZoneAndErrors(t *testing.T) {
	manila, err := time.LoadLocation("Asia/Manila")
	if err != nil {
		t.Fatal(err)
	}
	r, h, logs := newWorldRuntime(t, manila, modScripts("example.a", nil, nil, "scripts/main.lua", `
local schedules = require "wa.schedules"
local events = require "wa.events"
return function()
  events.on("login", function(player)
    schedules.daily("04:00", function() player:send_message("4 in Manila") end)
    for _, f in ipairs({
      function() schedules.daily("4:00", function() end) end,
      function() schedules.weekly("caturday", "04:00", function() end) end,
      function() schedules.hourly(60, function() end) end,
      function() schedules.when(function() return true end, function() end, { lasting_minutes = -1 }) end,
      function() schedules.when(function() return true end, function() end, { minutes = 1 }) end,
    }) do
      local ok, err = pcall(f)
      player:send_message(tostring(ok) .. " " .. tostring(err))
    end
  end)
end`))
	r.Fire("login", Player(1))
	h.clock = time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC) // 04:00 in Manila
	r.RunSchedules(h.clock)
	got := strings.Join(h.messages, "\n")
	for _, want := range []string{"is not a time of day", `"caturday" is not a day`, "the minute must be 0 to 59", "lasting_minutes is 0 to", "the only option is lasting_minutes", "1:4 in Manila"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s\nlogs: %s", want, got, logs)
		}
	}
}

func TestHooks(t *testing.T) {
	r, h, logs := newWorldRuntime(t, time.UTC,
		modScripts("example.a", []string{"rules.override"}, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
return function()
  hooks.on("drop_rate", function(drop, next)
    return next(drop) * 2
  end, { priority = 1 })
  hooks.on("experience", function(exp, next)
    local base, job = next({ player = exp.player, monster = exp.monster, base = exp.base + 1, job = exp.job })
    return base * 10, job
  end)
end`),
		modScripts("example.b", []string{"rules.override"}, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
return function()
  -- Higher priority runs first: this one sees the rate before a doubles it.
  hooks.on("drop_rate", function(drop, next)
    if drop.monster.name ~= "Poring" or drop.player.name ~= "Alice" then error("wrong object") end
    return next() + 1
  end, { priority = 5 })
  -- A hook that fails is left out.
  hooks.on("drop_rate", function(drop, next) error("broken hook") end, { priority = 3 })
end`))
	if !r.HasHooks("drop_rate") || !r.HasHooks("experience") {
		t.Fatal("hooks missing")
	}
	// b(+1) runs outside a(*2): (100 * 2) + 1.
	if got := r.RunDropRate(DropRate{Player: 1, Monster: 7, Item: 501, Rate: 100}); got != 201 {
		t.Fatalf("drop rate %d, logs: %s", got, logs)
	}
	if !strings.Contains(logs.String(), "broken hook") {
		t.Fatalf("the failing hook was not logged: %s", logs)
	}
	if got := r.RunDropRate(DropRate{Player: 1, Monster: 7, Item: 501, Rate: 9000}); got != 10000 {
		t.Fatalf("a rate above 10,000 is %d", got)
	}
	if b, j := r.RunExperience(Experience{Player: 1, Monster: 7, Base: 5, Job: 3}); b != 60 || j != 3 {
		t.Fatalf("exp %d %d", b, j)
	}
	_ = h
}

func TestHookRules(t *testing.T) {
	_, _, logs := newWorldRuntime(t, time.UTC, modScripts("example.a", nil, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
return function()
  hooks.on("drop_rate", function(drop, next) return next(drop) end)
end`))
	if !strings.Contains(logs.String(), "hooks.on needs the permission rules.override") {
		t.Fatalf("logs: %s", logs)
	}
	r, h, logs := newWorldRuntime(t, time.UTC, modScripts("example.a", []string{"rules.override"}, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
local events = require "wa.events"
return function()
  local ok, err = pcall(hooks.on, "drop_chance", function() end)
  events.on("login", function(player) player:send_message(tostring(err)) end)
  hooks.on("drop_rate", function(drop, next)
    drop.player:send_message("side effect")
    return next(drop)
  end)
  hooks.on("experience", function(exp, next) return "lots" end)
end`))
	r.Fire("login", Player(1))
	if got := r.RunDropRate(DropRate{Player: 1, Monster: 7, Item: 501, Rate: 100}); got != 100 {
		t.Fatalf("rate %d", got)
	}
	if b, j := r.RunExperience(Experience{Player: 1, Monster: 7, Base: 5, Job: 3}); b != 5 || j != 3 {
		t.Fatalf("exp %d %d", b, j)
	}
	got := strings.Join(h.messages, "|") + logs.String()
	for _, want := range []string{`there is no hook "drop_chance"`, "player:send_message cannot run in a computed bonus", "must return two integers"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestSpawningAndDeath(t *testing.T) {
	r, h, logs := newWorldRuntime(t, time.UTC, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local spawning = require "wa.spawning"
return function()
  local list = spawning.monsters { monster = 1002, amount = 2, x = 150, y = 150, area = 3 }
  local boss = spawning.monsters { monster = 1039, respawn = true }[1]
  events.on("death", function(death)
    death.killer:send_message("boss " .. death.victim.name .. " " .. tostring(death.victim == boss) .. " " .. death.map)
  end, { victim = boss })
  events.on("death", function(death)
    death.killer:send_message("one of two at " .. death.x .. "," .. death.y)
  end, { victims = list })
  events.on("death", function(death)
    death.victim:send_message("you died")
  end, { victim_kind = "player" })
  events.on("death", function(death)
    death.killer:send_message("a poring")
  end, { monster = 1002, map = "prontera" })
  events.on("death", function() error("never") end, { map = "geffen" })
  for _, f in ipairs({
    function() spawning.monsters { monster = 1002, map = "geffen" } end,
    function() spawning.monsters { monster = 1002, x = 3 } end,
    function() spawning.monsters { monster = 1002, amount = 0 } end,
    function() spawning.monsters { monster = 1002, colour = "red" } end,
    function() events.on("login", function() end, { victim_kind = "player" }) end,
    function() events.on("death", function() end, { victim_kind = "npc" }) end,
  }) do
    local ok, err = pcall(f)
    if ok then error("passed") end
  end
end`))
	if len(h.spawned) != 2 || h.spawned[0].Area != 3 || !h.spawned[0].HasCell || !h.spawned[1].Respawn || h.spawned[1].HasCell {
		t.Fatalf("spawned %+v, logs: %s", h.spawned, logs)
	}
	ids := slices.Sorted(func(yield func(int64) bool) {
		for id := range h.owners {
			if !yield(id) {
				return
			}
		}
	})
	// 101 and 102 are the porings, 103 the boss.
	r.FireDeath(Death{Victim: 103, VictimKind: "monster", Monster: 1039, Killer: 1, KillerKind: "player"})
	r.FireDeath(Death{Victim: 101, VictimKind: "monster", Monster: 1002, Killer: 1, KillerKind: "player", X: 151, Y: 149})
	r.FireDeath(Death{Victim: 2, VictimKind: "player", Killer: 103, KillerKind: "monster"})
	want := "1:boss Spawned 1039 true prontera|1:one of two at 151,149|1:a poring|2:you died"
	if got := strings.Join(h.messages, "|"); got != want || len(ids) != 3 {
		t.Fatalf("messages = %q, want %q, ids %v, logs: %s", got, want, ids, logs)
	}
	// The mod's context owns the monsters: a reload removes them.
	if err := r.Reload(modScripts("example.a", nil, nil, "scripts/main.lua", "return function() end")); err != nil {
		t.Fatal(err)
	}
	if len(h.owners) != 0 {
		t.Fatalf("monsters left after the context ended: %v", h.owners)
	}
}

func TestEventContext(t *testing.T) {
	sc := modScripts("example.a", []string{"rules.override"}, nil, "scripts/main.lua", "return function() end",
		"scripts/events/boss.lua", `
local context = require "wa.context"
local events = require "wa.events"
local hooks = require "wa.hooks"
local spawning = require "wa.spawning"
local schedules = require "wa.schedules"
local timers = require "wa.timers"
return function()
  local run = context:get_instance()
  if context:get_kind() ~= "event" or run.name ~= "example.a:Boss" or run.map ~= "prontera" then error("instance") end
  spawning.monsters { monster = 1039 }
  hooks.on("drop_rate", function(drop, next) return next(drop) * 3 end)
  events.on("death", function() end)
  schedules.hourly(0, function() end)
  timers.every(1000, function() end)
end`,
		"scripts/events/conditions.lua", `
return {
  quiet = function() return true end,
  busy = function() return false end,
  bad = function() local s = require "wa.spawning"; s.monsters { monster = 1002 } end,
}`)
	r, h, logs := newWorldRuntime(t, time.UTC, sc)
	c, err := r.StartEvent("example.a:Boss", "example.a:scripts/events/boss.lua")
	if err != nil {
		t.Fatalf("%v, logs: %s", err, logs)
	}
	if len(h.owners) != 1 || r.RunDropRate(DropRate{Monster: 7, Item: 501, Rate: 10}) != 30 {
		t.Fatalf("the run's monster or hook is missing: %v", h.owners)
	}
	r.EndContext(c)
	if len(h.owners) != 0 || r.HasHooks("drop_rate") || len(r.handlers["death"]) != 0 || len(r.schedules) != 0 || len(r.timers) != 0 {
		t.Fatalf("the run left things behind: monsters %v hooks %v", h.owners, r.hooks)
	}
	for fn, want := range map[string]bool{"quiet": true, "busy": false} {
		if got, err := r.CheckCondition("example.a", "example.a.events.conditions", fn); err != nil || got != want {
			t.Errorf("%s: %v %v", fn, got, err)
		}
	}
	if _, err := r.CheckCondition("example.a", "example.a.events.conditions", "bad"); err == nil || !strings.Contains(err.Error(), "cannot run in a computed bonus") {
		t.Errorf("a condition that spawns: %v", err)
	}
	if _, err := r.CheckCondition("example.a", "example.a.events.conditions", "missing"); err == nil {
		t.Error("a missing condition passed")
	}
}
