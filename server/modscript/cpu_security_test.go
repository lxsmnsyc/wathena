package modscript

import (
	"strings"
	"testing"
	"time"
)

// A handler that uses nearly its whole per-call budget, and returns
// normally, must not run forever on a hot event: the mod's total time has
// to be limited somehow.
func TestSlowButSuccessfulHandlerIsLimited(t *testing.T) {
	h := newHost()
	r, _ := newRuntime(t, h, modScripts("example.slow", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    local n = 0
    for i = 1, 3000000 do n = n + i end
    player:send_message("ran")
  end)
end`))
	const calls = 40
	start := time.Now()
	for i := 0; i < calls; i++ {
		r.Fire("login", Player(1))
	}
	t.Logf("%d calls took %v, %d ran", calls, time.Since(start), len(h.messages))
	if len(h.messages) == calls {
		t.Fatalf("all %d calls ran in full (%v): a mod can spend the zone's time with no limit", calls, time.Since(start))
	}
}

// A mod over its CPU budget for the tick: its handlers and hooks are
// refused without counting as failures, its timers wait, one warning is
// logged, and the next tick runs them again. Other mods are not touched.
func TestBudgetRefusesUntilNextTick(t *testing.T) {
	h := newHost()
	slow := modScripts("example.slow", []string{"rules.override"}, nil, "scripts/main.lua", `
local events = require "wa.events"
local hooks = require "wa.hooks"
local timers = require "wa.timers"
return function()
  events.on("login", function(player)
    local n = 0
    for i = 1, 3000000 do n = n + i end
    player:send_message("slow")
  end)
  hooks.on("drop_rate", function(drop, next) return next(drop) * 2 end)
  timers.after(0, function() end)
end`)
	other := modScripts("example.other", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(player) player:send_message("other") end)
end`)
	r, logs := newRuntime(t, h, slow, other)
	r.Fire("login", Player(1))
	r.Fire("login", Player(1))
	if strings.Join(h.messages, ",") != "1:slow,1:other,1:other" {
		t.Fatalf("messages = %q, want the slow handler once and the other mod's each time", h.messages)
	}
	if got := r.RunDropRate(DropRate{Player: 1, Monster: 7, Item: 501, Rate: 100}); got != 100 {
		t.Errorf("drop rate = %d: the hook of a mod over its budget ran", got)
	}
	timers := len(r.timers)
	r.Advance(1)
	if len(r.timers) != timers {
		t.Errorf("a timer of a mod over its budget ran")
	}
	if n := strings.Count(logs.String(), "used its Lua CPU budget"); n != 1 {
		t.Errorf("%d budget warnings, want 1; logs: %s", n, logs)
	}
	if strings.Contains(logs.String(), "mod handler failed") {
		t.Errorf("a refused call counted as a failure: %s", logs)
	}
	r.BeginTick()
	if got := r.RunDropRate(DropRate{Player: 1, Monster: 7, Item: 501, Rate: 100}); got != 200 {
		t.Errorf("drop rate = %d at the next tick, want 200", got)
	}
	r.Advance(1)
	if len(r.timers) != timers-1 {
		t.Errorf("the timer did not run at the next tick")
	}
	if u, ok := r.Usage("example.slow"); !ok || u.Memory <= 0 {
		t.Errorf("usage = %+v, %v", u, ok)
	}
	if u, ok := r.Usage("example.other"); !ok || u.CPUBudget != 2_000_000 {
		t.Errorf("other usage = %+v, %v", u, ok)
	}
}
