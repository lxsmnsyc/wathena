package zone

import (
	"testing"
	"time"
)

// timersScript uses the timers of review decision 60 on npc_event labels
// and notes results in the variable seen.
const timersScript = `
local events = require "wa.events"
local maps = require "wa.maps"
local timers = require "wa.timers"
local variables = require "wa.variables"
local seen = variables.server_temporary_string("seen")
local function note(s) seen:set(seen:get() .. s .. ";") end
return function()
  local function on(name, f) events.on("npc_event", f, { name = name }) end
  on("OnArm", function()
    local p = maps.find_player("a")
    p:add_timer(1000, "OnTick")
    p:add_timer(2000, "example.test:Guide::OnNamed")
    p:add_timer(3000, "OnGone")
    note("removed " .. tostring(p:remove_timer("OnGone")) .. " " .. tostring(p:remove_timer("OnGone")))
  end)
  on("OnTick", function(event) note("tick " .. event.player.name) end)
  on("OnNamed", function(event) note("named " .. event.player.name) end)
  on("OnGone", function() note("gone") end)
  on("OnTooMany", function()
    local p = maps.find_player("a")
    for i = 1, 33 do
      local ok, err = pcall(p.add_timer, p, 10, "OnTick")
      if not ok then note("stopped at " .. i) break end
    end
  end)
  on("OnServer", function()
    timers.after(1000, function()
      note("server start")
      timers.wait(500)
      note("server end")
    end, { clock = "server" })
  end)
  on("OnWaitZone", function()
    note("zone start")
    timers.wait(500)
    note("zone end")
  end)
end`

// TestPlayerTimers checks player:add_timer and remove_timer (review
// decision 60, C2): a label of the context's NPC and a named NPC run
// with the player when the time comes, and a removed timer does not.
func TestPlayerTimers(t *testing.T) {
	z, _, n := followupZoneWith(t, timersScript)
	fire(t, z, n, "OnArm")
	if got := seen(t, z); got != "removed true false;" {
		t.Fatalf("got %q", got)
	}
	z.stepAt(z.now + 900)
	z.runScripts()
	if got := seen(t, z); got != "" {
		t.Fatalf("a timer ran early: %q", got)
	}
	z.stepAt(z.now + 200)
	z.runScripts()
	if got := seen(t, z); got != "tick a;" {
		t.Fatalf("got %q", got)
	}
	z.stepAt(z.now + 1000)
	z.runScripts()
	z.stepAt(z.now + 2000)
	z.runScripts()
	if got := seen(t, z); got != "named a;" {
		t.Fatalf("got %q", got)
	}
}

// TestPlayerTimersCap checks that a player has 32 timers at most.
func TestPlayerTimersCap(t *testing.T) {
	z, _, n := followupZoneWith(t, timersScript)
	fire(t, z, n, "OnTooMany")
	if got := seen(t, z); got != "stopped at 33;" {
		t.Fatalf("got %q", got)
	}
}

// TestPlayerTimersMove checks that the timers go with a player to
// another zone with the time they have left, and end with the player.
func TestPlayerTimersMove(t *testing.T) {
	z, p, n := followupZoneWith(t, timersScript)
	fire(t, z, n, "OnArm")
	seen(t, z)
	z.stepAt(z.now + 400)
	left := z.eventTimersLeft(p)
	if len(left) != 2 || left[0].due != 600 || left[1].due != 1600 {
		t.Fatalf("timers left = %+v", left)
	}
	if left[0].label != "ontick" {
		t.Fatalf("label = %q", left[0].label)
	}
}

// TestServerTimerWakesZone checks that a timer of the server clock
// runs from the schedule check, with the zone clock standing still, and
// that a wait in its callback follows the server clock.
func TestServerTimerWakesZone(t *testing.T) {
	z, _, n := followupZoneWith(t, timersScript)
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	z.Clock = func() time.Time { return at }
	fire(t, z, n, "OnServer")
	next, ok := z.scripts.NextSchedule()
	if !ok || !next.Equal(at.Add(time.Second)) {
		t.Fatalf("next = %v, %v", next, ok)
	}
	z.runSchedules()
	z.runScripts()
	if got := seen(t, z); got != "" {
		t.Fatalf("early: %q", got)
	}
	at = at.Add(time.Second)
	z.runSchedules()
	z.runScripts()
	if got := seen(t, z); got != "server start;" {
		t.Fatalf("got %q", got)
	}
	at = at.Add(500 * time.Millisecond)
	z.runSchedules()
	z.runScripts()
	if got := seen(t, z); got != "server end;" {
		t.Fatalf("got %q", got)
	}
}

// TestWaitZoneClock checks a wait in a label sent without a player.
func TestWaitZoneClock(t *testing.T) {
	z, _, n := followupZoneWith(t, timersScript)
	fire(t, z, n, "OnWaitZone")
	if got := seen(t, z); got != "zone start;" {
		t.Fatalf("got %q", got)
	}
	z.stepAt(z.now + 600)
	z.runScripts()
	if got := seen(t, z); got != "zone end;" {
		t.Fatalf("got %q", got)
	}
}
