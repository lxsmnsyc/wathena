package modscript

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// waitNPC starts an NPC whose script logs to a temporary variable.
func waitNPC(t *testing.T, body string) (*Runtime, *NPC, *variables.Temporary, *fakeHost, *time.Time, *logBuffer) {
	t.Helper()
	h := newHost()
	temp := variables.NewTemporary()
	clock := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	var buf logBuffer
	scripts := modScripts("example.a", nil, nil, "scripts/main.lua", `return function() end`, "scripts/npc.lua", `
local events = require "wa.events"
local timers = require "wa.timers"
local variables = require "wa.variables"
local logged = variables.server_temporary_string("log")
local acc = ""
local function log(text) acc = acc .. text .. ";"; logged:set(acc) end
return function()
`+body+`
end`)
	r, err := New(Options{Host: h, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{"example.a"},
		Temporary: temp, Clock: func() time.Time { return clock }}, []*mod.Scripts{scripts})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	n, err := r.StartNPC(1, "example.a:scripts/npc.lua")
	if err != nil {
		t.Fatal(err)
	}
	return r, n, temp, h, &clock, &buf
}

func logOf(temp *variables.Temporary) string {
	got, _ := temp.Get(variables.TempKey{Mod: "example.a", Name: "log"})
	return strings.Trim(got, `"`)
}

// A handler of an event without a player waits on the zone clock.
func TestWaitInEventWithoutPlayer(t *testing.T) {
	r, n, temp, h, _, logs := waitNPC(t, `
  events.on("npc_event", function()
    log("a")
    timers.wait(1000)
    log("b")
    timers.wait(500)
    log("c")
  end, { name = "OnGo" })`)
	if _, ran, err := r.FireNPCEvent(n, "ongo", 0); err != nil || !ran {
		t.Fatalf("ran = %v, err = %v", ran, err)
	}
	if got := logOf(temp); got != "a;" {
		t.Fatalf("log = %q; logs: %s", got, logs)
	}
	advance(h, r, 999)
	if got := logOf(temp); got != "a;" {
		t.Fatalf("log at 999 = %q", got)
	}
	advance(h, r, 1000)
	if got := logOf(temp); got != "a;b;" {
		t.Fatalf("log at 1000 = %q; logs: %s", got, logs)
	}
	advance(h, r, 1500)
	if got := logOf(temp); got != "a;b;c;" {
		t.Fatalf("log at 1500 = %q; logs: %s", got, logs)
	}
}

// A timer callback of an NPC may wait, and so may a callback of a
// repeating timer, each run on its own.
func TestWaitInTimerCallback(t *testing.T) {
	r, _, temp, h, _, logs := waitNPC(t, `
  timers.every(1000, function()
    log("s")
    timers.wait(300)
    log("e")
  end)`)
	advance(h, r, 1000)
	advance(h, r, 1300)
	advance(h, r, 2000)
	if got := logOf(temp); got != "s;e;s;" {
		t.Fatalf("log = %q; logs: %s", got, logs)
	}
}

// A wait follows the clock of the timer that started the handler: the
// server clock for { clock = "server" }.
func TestWaitFollowsServerClock(t *testing.T) {
	r, _, temp, h, clock, logs := waitNPC(t, `
  timers.after(1000, function()
    log("s")
    timers.wait(2000)
    log("e")
  end, { clock = "server" })`)
	// The zone clock moves, and the server clock does not.
	advance(h, r, 5000)
	r.RunServerTimers(*clock)
	if got := logOf(temp); got != "" {
		t.Fatalf("log = %q", got)
	}
	if next, ok := r.NextSchedule(); !ok || !next.Equal(clock.Add(time.Second)) {
		t.Fatalf("next = %v, %v", next, ok)
	}
	*clock = clock.Add(time.Second)
	r.RunServerTimers(*clock)
	if got := logOf(temp); got != "s;" {
		t.Fatalf("log = %q; logs: %s", got, logs)
	}
	// The wait runs on the server clock: the zone clock is of no use.
	advance(h, r, 100000)
	if got := logOf(temp); got != "s;" {
		t.Fatalf("log after the zone clock = %q", got)
	}
	*clock = clock.Add(2 * time.Second)
	r.RunServerTimers(*clock)
	if got := logOf(temp); got != "s;e;" {
		t.Fatalf("log = %q; logs: %s", got, logs)
	}
}

// A server clock timer does not move when the zone sleeps, and a zone
// clock timer does (Shift).
func TestShiftLeavesServerTimers(t *testing.T) {
	r, _, temp, h, clock, logs := waitNPC(t, `
  timers.after(1000, function() log("zone") end)
  timers.after(1000, function() log("server") end, { clock = "server" })`)
	r.Shift(5000)
	*clock = clock.Add(1500 * time.Millisecond)
	advance(h, r, 2000)
	r.RunServerTimers(*clock)
	if got := logOf(temp); got != "server;" {
		t.Fatalf("log = %q; logs: %s", got, logs)
	}
}

// A wait outside a handler without a conversation raises an error, and
// the options of a timer are checked.
func TestWaitErrors(t *testing.T) {
	for src, want := range map[string]string{
		`timers.wait(10)`:  "runs only in a handler",
		`timers.wait("x")`: "milliseconds must be an integer",
		`timers.after(1, function() end, { clock = "x" })`: "clock is zone or server",
		`timers.after(1, function() end, { when = "x" })`:  "the only option is clock",
		`timers.after(1, function() end, 3)`:               "the options must be a table",
	} {
		r, _, temp, h, _, _ := waitNPC(t, `
  local ok, err = pcall(function() `+src+` end)
  log(tostring(err))`)
		_ = r
		_ = h
		if got := logOf(temp); !strings.Contains(got, want) {
			t.Errorf("%s: %q, want %q", src, got, want)
		}
	}
}

func TestWaitLimit(t *testing.T) {
	r, n, temp, _, _, logs := waitNPC(t, `
  events.on("npc_event", function()
    local ok, err = pcall(timers.wait, 600001)
    log(tostring(err))
  end, { name = "OnGo" })`)
	if _, _, err := r.FireNPCEvent(n, "ongo", 0); err != nil {
		t.Fatal(err)
	}
	if got := logOf(temp); !strings.Contains(got, "milliseconds are 0 to 600000") {
		t.Fatalf("log = %q; logs: %s", got, logs)
	}
}

// A context that ends closes the handlers that wait in it.
func TestWaitEndsWithContext(t *testing.T) {
	r, n, temp, h, _, logs := waitNPC(t, `
  events.on("npc_event", function()
    timers.wait(1000)
    log("late")
  end, { name = "OnGo" })`)
	if _, _, err := r.FireNPCEvent(n, "ongo", 0); err != nil {
		t.Fatal(err)
	}
	r.end(n.ctx)
	advance(h, r, 2000)
	if got := logOf(temp); got != "" {
		t.Fatalf("log = %q; logs: %s", got, logs)
	}
}
