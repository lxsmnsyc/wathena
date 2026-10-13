package zone

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestModOffInZone: a mod that staff turn off in a zone stops its event
// handlers and timers there, and keeps its data: once on, its counter
// goes on from where it was (decision 0031).
func TestModOffInZone(t *testing.T) {
	z, a, _, logs := scriptZone(t, `
local events = require "wa.events"
local timers = require "wa.timers"
local ticks = 0
return function()
  timers.every(100, function() ticks = ticks + 1 end)
  events.on("login", function(player) player:send_message("ticks " .. ticks) end)
end`)
	z.stepAt(50)
	events(a)
	for now := int64(100); now <= 1000; now += 100 {
		z.stepAt(now)
	}
	if err := z.setModOff("example.test", true); err != nil {
		t.Fatal(err)
	}
	b := add(z, "b")
	z.runScripts()
	for now := int64(1100); now <= 5000; now += 100 {
		z.stepAt(now)
	}
	if got := systemLines(events(b)); len(got) != 0 {
		t.Fatalf("the login handler ran while the mod was off: %q", got)
	}
	if err := z.setModOff("example.test", false); err != nil {
		t.Fatal(err)
	}
	c := add(z, "c")
	z.runScripts()
	z.stepAt(5050)
	// 10 ticks before it was off. The timer kept its rhythm and skipped
	// the runs that came while the mod was off.
	if got := strings.Join(systemLines(events(c)), "|"); got != "ticks 10" {
		t.Fatalf("after turning it on: %q, logs: %s", got, logs)
	}
	if err := z.setModOff(mod.Core, true); err == nil {
		t.Fatal("core was turned off")
	}
}

// TestModOffStopsNPCClicks: an NPC of a mod that is off does not answer
// clicks, and its conversations close.
func TestModOffStopsNPCClicks(t *testing.T) {
	z, p, n, logs := npcZone(t, chooser)
	if ev := click(z, p, n); lines(ev) != "[Guide]|Where to?" {
		t.Fatalf("before: %q, logs: %s", lines(ev), logs)
	}
	if err := z.setModOff("example.test", true); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	if p.talk != nil {
		t.Fatal("the conversation goes on while the mod is off")
	}
	events(p)
	if ev := click(z, p, n); len(dialogs(ev)) != 0 {
		t.Fatalf("a click while off: %q", lines(ev))
	}
	if err := z.setModOff("example.test", false); err != nil {
		t.Fatal(err)
	}
	if ev := click(z, p, n); lines(ev) != "[Guide]|Where to?" {
		t.Fatalf("after: %q, logs: %s", lines(ev), logs)
	}
}

// TestModsOffAtStart: a zone whose mod is off at start runs its scripts
// once staff turn it on, through SetModOff on the zone's goroutine. The
// error counts outlive the change.
func TestModsOffAtStart(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{3}).Stream(), nil)
	logs := &syncBuffer{}
	z.Mods = &Mods{Scripts: []*mod.Scripts{testMod(`return function() error("setup ran") end`)}, IDs: []string{"example.test"},
		Log: slog.New(slog.NewTextHandler(logs, nil))}
	z.SetModsOff([]string{"example.test"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go z.Run(ctx)
	wait, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	if _, err := z.Summary(wait); err != nil {
		t.Fatal(err)
	}
	if got := z.ModErrors(); len(got) != 0 {
		t.Fatalf("the setup ran while the mod was off: %+v", got)
	}
	if err := z.SetModOff(wait, "example.test", false); err == nil || !strings.Contains(err.Error(), "setup ran") {
		t.Fatalf("turning it on: %v", err)
	}
	if e := z.ModErrors()["example.test"]; e.Count != 1 || !strings.Contains(e.Last, "setup ran") {
		t.Fatalf("errors %+v, logs: %s", e, logs)
	}
}
