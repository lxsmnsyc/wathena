package zone

import (
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
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
  on("OnArmW", function()
    maps.find_player("w"):add_timer(1000, "OnTick")
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
  on("OnServerShort", function()
    timers.after(50, function() note("short") end, { clock = "server" })
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

// joinWalker joins a player with a session, which can change zone.
func joinWalker(z *Zone, name string) *Player {
	c := savedCharacter()
	c.Name, c.Map, c.X, c.Y = name, z.Map, 21, 21
	sess := newSession(&saves{})
	p := newPlayer(sess, c)
	sess.zone, sess.player = z, p
	z.join(p)
	return p
}

// TestPlayerTimersMove checks that the timers go with a player to
// another zone with the time they have left, and run when it is back.
func TestPlayerTimersMove(t *testing.T) {
	z, _, n := followupZoneWith(t, timersScript)
	w := joinWalker(z, "w")
	z.stepAt(z.now + 50)
	fire(t, z, n, "OnArmW")
	z.stepAt(z.now + 400)
	// 600 ms are left when the player goes to the other zone.
	b := saveZone()
	b.Map = "map/b"
	z.depart(w, b, 5, 5)
	q := arrive(t, b)
	if len(q.eventTimers) != 1 || q.eventTimers[0].due != b.now+600 {
		t.Fatalf("timers on the new map: %+v at %d", q.eventTimers, b.now)
	}
	// The player comes back after 200 ms there: 600 ms are still left.
	b.stepAt(b.now + 200)
	b.depart(q, z, 21, 21)
	r := arrive(t, z)
	if len(r.eventTimers) != 1 || r.eventTimers[0].due != z.now+400 {
		t.Fatalf("timers back: %+v at %d", r.eventTimers, z.now)
	}
	z.stepAt(z.now + 300)
	z.runScripts()
	if got := seen(t, z); got != "" {
		t.Fatalf("early: %q", got)
	}
	z.stepAt(z.now + 150)
	z.runScripts()
	if got := seen(t, z); got != "tick w;" {
		t.Fatalf("got %q", got)
	}
}

// TestPlayerTimersEndAtLogout checks that a player who leaves takes its
// timers with it: no event runs.
func TestPlayerTimersEndAtLogout(t *testing.T) {
	z, _, n := followupZoneWith(t, timersScript)
	w := joinWalker(z, "w")
	z.stepAt(z.now + 50)
	fire(t, z, n, "OnArmW")
	z.leave(w, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	z.stepAt(z.now + 2000)
	z.runScripts()
	if got := seen(t, z); got != "" {
		t.Fatalf("got %q", got)
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

// TestServerTimerWakesSleepingZone checks the sleeping zone's path: with
// no player, the schedule wait fires when the server clock timer is due,
// the callback runs, and the zone clock stands still.
func TestServerTimerWakesSleepingZone(t *testing.T) {
	z, p, n := followupZoneWith(t, timersScript)
	z.Clock = time.Now
	fire(t, z, n, "OnServerShort")
	z.leave(p, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	if len(z.players) != 0 {
		t.Fatal("the zone still has players")
	}
	now := z.now
	var tm *time.Timer
	defer func() {
		if tm != nil {
			tm.Stop()
		}
	}()
	ch := z.scheduleWait(&tm)
	if ch == nil {
		t.Fatal("a sleeping zone with a server clock timer has nothing to wait for")
	}
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("the schedule wait did not fire")
	}
	z.runSchedules()
	z.runScripts()
	if got := seen(t, z); got != "short;" {
		t.Fatalf("got %q", got)
	}
	if z.now != now {
		t.Fatalf("the zone clock moved from %d to %d", now, z.now)
	}
	if z.scheduleWait(&tm) != nil {
		t.Fatal("the zone still waits for a timer that ran")
	}
}
