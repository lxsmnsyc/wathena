package modscript

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
)

// TestModOffStopsScripts: a mod that is off in a zone runs no event
// handlers, timers, hooks or schedules, and its handlers run again once
// it is on. Its Lua state keeps its data, and other mods go on
// (decision 0031).
func TestModOffStopsScripts(t *testing.T) {
	r, h, logs := newWorldRuntime(t, time.UTC,
		modScripts("example.a", []string{"rules.override"}, nil, "scripts/main.lua", `
local events = require "wa.events"
local timers = require "wa.timers"
local hooks = require "wa.hooks"
local schedules = require "wa.schedules"
local logins, ticks, once, daily = 0, 0, 0, 0
return function()
  events.on("login", function(p)
    logins = logins + 1
    p:send_message("a " .. logins .. " " .. ticks .. " " .. once .. " " .. daily)
  end)
  timers.every(100, function() ticks = ticks + 1 end)
  timers.after(150, function() once = once + 1 end)
  hooks.on("drop_rate", function(drop, next) return next(drop) * 2 end)
  schedules.daily("20:00", function() daily = daily + 1 end)
end`),
		modScripts("example.b", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(p) p:send_message("b") end)
end`))
	say := func() string {
		h.messages = nil
		r.Fire("login", Player(1))
		return strings.Join(h.messages, "|")
	}
	if got := say(); got != "1:a 1 0 0 0|1:b" {
		t.Fatalf("before: %q, logs: %s", got, logs)
	}
	if err := r.SetOff("example.a", true); err != nil {
		t.Fatal(err)
	}
	if !r.Off("example.a") || r.Off("example.b") {
		t.Fatal("Off is wrong")
	}
	// Its handlers, timers, hooks and schedules are skipped.
	advance(h.fakeHost, r, 250)
	h.clock = time.Date(2026, 10, 31, 20, 0, 5, 0, time.UTC)
	r.RunSchedules(h.clock)
	if got := say(); got != "1:b" {
		t.Fatalf("while off: %q", got)
	}
	if got := r.RunDropRate(DropRate{Player: 1, Monster: 7, Item: 501, Rate: 100}); got != 100 {
		t.Fatalf("the hook ran while off: %d", got)
	}
	// Once on, they run again. Its data stayed. The timer that runs once
	// was skipped, and the schedule's time passed.
	if err := r.SetOff("example.a", false); err != nil {
		t.Fatal(err)
	}
	advance(h.fakeHost, r, 350)
	if got := say(); got != "1:a 2 1 0 0|1:b" {
		t.Fatalf("after: %q", got)
	}
	if got := r.RunDropRate(DropRate{Player: 1, Monster: 7, Item: 501, Rate: 100}); got != 200 {
		t.Fatalf("the hook did not come back: %d", got)
	}
	if err := r.SetOff(mod.Core, true); err == nil {
		t.Fatal("core was turned off")
	}
}

// TestModOffAtStart: a mod that is off when the zone starts does not run
// its entry scripts, NPC setups or item scripts. Turning it on runs
// them.
func TestModOffAtStart(t *testing.T) {
	h := newHost()
	var buf logBuffer
	sc := modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(p) p:send_message("started") end)
end`, "scripts/npc.lua", `return function() end`, "scripts/item.lua", `
local events = require "wa.events"
return function()
  events.on("use", function(p) p:send_message("used") end)
end`)
	r, err := New(Options{Host: h, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{"example.a"}, Off: []string{"example.a"}},
		[]*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Context("example.a") != nil {
		t.Fatal("the entry script ran while the mod was off")
	}
	n, err := r.StartNPC(1, "example.a:scripts/npc.lua")
	if err != nil || n == nil || n.Context() != nil {
		t.Fatalf("NPC of a mod that is off: %v %v", n, err)
	}
	if err := r.UseItem(1, 501, "example.a:scripts/item.lua"); !errors.Is(err, ErrModOff) {
		t.Fatalf("item script of a mod that is off: %v", err)
	}
	r.Fire("login", Player(1))
	if len(h.messages) != 0 {
		t.Fatalf("messages while off: %q", h.messages)
	}
	if err := r.SetOff("example.a", false); err != nil {
		t.Fatal(err)
	}
	if r.Context("example.a") == nil || n.Context() == nil {
		t.Fatalf("turning the mod on did not start it: %s", &buf)
	}
	r.Fire("login", Player(1))
	if err := r.UseItem(1, 501, "example.a:scripts/item.lua"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(h.messages, "|"); got != "1:started|1:used" {
		t.Fatalf("messages: %q", got)
	}
}

// TestErrorLog: failed handlers, timers and setups count per mod, with
// the last error and its time.
func TestErrorLog(t *testing.T) {
	h := newHost()
	var buf logBuffer
	errs := &ErrorLog{}
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	mods := []*mod.Scripts{
		modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  events.on("login", function(p) error("broken login") end)
  timers.after(10, function() error("broken timer") end)
end`),
		modScripts("example.b", nil, nil, "scripts/main.lua", `return function() error("broken setup") end`),
		modScripts("example.c", nil, nil, "scripts/main.lua", `return function() end`),
	}
	r, err := New(Options{Host: h, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{"example.a", "example.b", "example.c"},
		Errors: errs, Clock: func() time.Time { return at }}, mods)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Fire("login", Player(1))
	advance(h, r, 10)
	got := errs.Snapshot()
	if a := got["example.a"]; a.Count != 2 || !strings.Contains(a.Last, "broken timer") || !a.At.Equal(at) {
		t.Errorf("example.a: %+v", a)
	}
	if b := got["example.b"]; b.Count != 1 || !strings.Contains(b.Last, "broken setup") {
		t.Errorf("example.b: %+v", b)
	}
	if _, ok := got["example.c"]; ok {
		t.Error("example.c has errors")
	}
	// A mod that is off adds no errors for the scripts it skips.
	if err := r.SetOff("example.a", true); err != nil {
		t.Fatal(err)
	}
	r.Fire("login", Player(1))
	_ = r.UseItem(1, 501, "example.a:scripts/main.lua")
	if a := errs.Snapshot()["example.a"]; a.Count != 2 {
		t.Errorf("errors while off: %+v", a)
	}
	// Long errors are cut.
	errs.add("example.c", errors.New(strings.Repeat("é", MaxErrorText)), at)
	if c := errs.Snapshot()["example.c"]; len(c.Last) > MaxErrorText || !strings.HasPrefix(c.Last, "é") {
		t.Errorf("long error kept %d bytes", len(c.Last))
	}
}

// TestRestartPlanned: restart_planned runs with { at, reason }, and with
// { cancelled = true } (review decision 36).
func TestRestartPlanned(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local players = {}
return function()
  events.on("login", function(p) players[#players + 1] = p end)
  events.on("restart_planned", function(restart)
    local text
    if restart.cancelled then
      text = "cancelled " .. tostring(restart.at)
    else
      text = "at " .. restart.at .. " for " .. restart.reason .. " " .. tostring(restart.cancelled)
    end
    local ok = pcall(function() restart.at = 0 end)
    for _, p in ipairs(players) do p:send_message(text .. " writable " .. tostring(ok)) end
  end)
end`))
	r.Fire("login", Player(1))
	at := time.Date(2026, 10, 13, 22, 0, 0, 0, time.UTC)
	r.FireRestartPlanned(RestartPlanned{At: at, Reason: "maintenance"})
	r.FireRestartPlanned(RestartPlanned{Cancelled: true})
	want := "1:at 1791928800 for maintenance nil writable false|1:cancelled nil writable false"
	if got := strings.Join(h.messages, "|"); got != want {
		t.Fatalf("messages = %q, logs: %s", got, logs)
	}
}
