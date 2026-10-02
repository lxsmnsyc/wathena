package zone

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// testMod is a mod with one entry script, scripts/main.lua.
func testMod(src string) *mod.Scripts {
	return &mod.Scripts{
		Mod: "example.test", Entries: []string{"scripts/main.lua"},
		Files:   map[string]string{"scripts/main.lua": src},
		Grants:  mod.Grants{"game.read": true, "event.listen": true},
		Visible: []string{"example.test"},
	}
}

// syncBuffer is a log that the zone goroutine writes while a test reads.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.ReplaceAll(s.b.String(), `\"`, `"`)
}

// scriptZone is lootZone with a mod's scripts running. The player stands
// two cells from a monster that gives a level when it dies.
func scriptZone(t *testing.T, src string) (*Zone, *Player, *entity, *syncBuffer) {
	t.Helper()
	mob := &battle.Status{MaxHP: 30, Hit: 200, ADelay: 1000, AMotion: 500, DMotion: 300, RHW: battle.Weapon{Atk: 10, Atk2: 10, Range: 1}}
	reward := &progression.Mob{ID: 1002, Level: 3, MaxHP: 30, BaseExp: 15, JobExp: 25}
	mob.Level = 3
	spawns := []Spawn{{Mob: 1002, Name: "Poring", Amount: 1, X: 20, Y: 20, Speed: 400, Status: mob, RespawnMs: 60000, Reward: reward}}
	z := New("map/test", open(40), rng.FromKey([32]byte{3}).Stream(), spawns)
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 100, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	logs := &syncBuffer{}
	z.Mods = &Mods{Scripts: []*mod.Scripts{testMod(src)}, IDs: []string{"core", "example.test"}, Log: slog.New(slog.NewTextHandler(logs, nil))}
	z.startScripts()
	t.Cleanup(z.stopScripts)
	if z.scripts == nil || z.scripts.Context("example.test") == nil {
		t.Fatalf("the mod did not start: %s", logs)
	}
	p := add(z, "a")
	z.runScripts()
	var m *entity
	for e := range z.entities.all() {
		if e.monster != nil {
			m = e
		}
	}
	p.entity.x, p.entity.y = m.x+2, m.y
	return z, p, m, logs
}

// systemLines are the lines of server text in events.
func systemLines(ev []*wathenav1.Event) []string {
	var out []string
	for _, e := range ev {
		if c := e.GetChat(); c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM {
			out = append(out, c.GetText())
		}
	}
	return out
}

const recordEvents = `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  for _, name in ipairs { "login", "map_enter", "base_level_up", "job_level_up" } do
    events.on(name, function(player)
      player:send_message(name .. " " .. player.name)
    end)
  end
  events.on("monster_killed", function(player, monster)
    player:send_message("monster_killed " .. player.name .. " " .. monster.name .. " " .. monster.id ..
      " level " .. monster.level .. " hp " .. monster.hp .. "/" .. monster.max_hp ..
      " alive " .. tostring(monster.is_alive) .. " on " .. monster.map)
    timers.after(0, function()
      local ok, err = pcall(function() return monster.name end)
      player:send_message("after the event: " .. tostring(ok) .. " " .. tostring(err):match("the monster is gone.*"))
    end)
  end)
end`

func TestModEventsFromZoneActions(t *testing.T) {
	z, p, m, logs := scriptZone(t, recordEvents)
	z.stepAt(50)
	if got := strings.Join(systemLines(events(p)), "|"); got != "login a|map_enter a" {
		t.Fatalf("after joining: %q, logs: %s", got, logs)
	}
	got := strings.Join(systemLines(kill(t, z, p, m)), "|")
	// The timer runs after the event, when the handle no longer reads.
	want := "monster_killed a Poring 1002 level 3 hp 0/30 alive false on test|base_level_up a|job_level_up a|" +
		"after the event: false the monster is gone from this map"
	if got != want {
		t.Fatalf("after the kill: %q, logs: %s", got, logs)
	}
	if len(z.monsterHandles) != 0 {
		t.Errorf("%d monster handles left", len(z.monsterHandles))
	}
}

func TestModLogout(t *testing.T) {
	z, a, _, logs := scriptZone(t, `
local events = require "wa.events"
local last = "nobody"
return function()
  events.on("logout", function(player) last = player.name end)
  events.on("login", function(player) player:send_message("last logout: " .. last) end)
end`)
	z.leave(a, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	b := add(z, "b")
	z.runScripts()
	z.stepAt(50)
	if got := strings.Join(systemLines(events(b)), "|"); got != "last logout: a" {
		t.Fatalf("got %q, logs: %s", got, logs)
	}
}

func TestModTimersFollowZoneClock(t *testing.T) {
	z, p, _, logs := scriptZone(t, `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  events.on("login", function(player)
    timers.after(1000, function() player:send_message("one second") end)
  end)
end`)
	z.stepAt(950)
	if got := systemLines(events(p)); len(got) != 0 {
		t.Fatalf("at 950 ms: %q", got)
	}
	z.stepAt(1000)
	if got := strings.Join(systemLines(events(p)), "|"); got != "one second" {
		t.Fatalf("at 1000 ms: %q, logs: %s", got, logs)
	}
}

// TestModTimerStartsFromZoneClock starts a timer long after the zone
// started: it waits its full time from then.
func TestModTimerStartsFromZoneClock(t *testing.T) {
	z, a, _, logs := scriptZone(t, `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  events.on("login", function(player)
    timers.after(1000, function() player:send_message("one second for " .. player.name) end)
  end)
end`)
	z.stepAt(60_000)
	events(a)
	b := add(z, "b")
	z.runScripts()
	z.stepAt(60_950)
	if got := systemLines(events(b)); len(got) != 0 {
		t.Fatalf("at 950 ms after the login: %q, logs: %s", got, logs)
	}
	z.stepAt(61_000)
	if got := strings.Join(systemLines(events(b)), "|"); got != "one second for b" {
		t.Fatalf("at 1000 ms after the login: %q, logs: %s", got, logs)
	}
}

func TestModLimitDoesNotBreakZone(t *testing.T) {
	z, p, m, logs := scriptZone(t, `
local events = require "wa.events"
return function()
  events.on("monster_killed", function(player) while true do end end)
  events.on("monster_killed", function(player) player:send_message("after the loop") end)
end`)
	z.stepAt(50)
	events(p)
	got := systemLines(kill(t, z, p, m))
	if strings.Join(got, "|") != "after the loop" || !strings.Contains(logs.String(), "CPU limit exceeded") {
		t.Fatalf("got %q, logs: %s", got, logs)
	}
	// The zone goes on: the player still levels and the monster returns.
	if p.progress.BaseLevel != 2 {
		t.Errorf("progress %+v", p.progress)
	}
}

func TestNPCsAppear(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{3}).Stream(), nil)
	z.SetNPCs([]NPC{{Name: "example.test:Greeter#1", DisplayName: "Greeter", X: 21, Y: 20, Direction: 2, Sprite: 112}})
	p := add(z, "a")
	p.entity.x, p.entity.y = 20, 20
	z.stepAt(50)
	var found *wathenav1.EntityAppear
	for _, e := range events(p) {
		if a := e.GetAppear(); a != nil && a.GetKind() == wathenav1.EntityKind_ENTITY_KIND_NPC {
			found = a
		}
	}
	if found == nil || found.GetName() != "Greeter" || found.GetLook().GetBody() != 112 || found.GetX() != 21 || found.GetDir() != 2 {
		t.Fatalf("NPC appear %v", found)
	}
	// An NPC cannot be attacked.
	z.handle(command{player: p, seq: 1, attack: &attackCommand{target: found.GetId(), repeat: true}})
	if p.entity.target != nil {
		t.Error("the player targets an NPC")
	}
}

// TestModReloadWhileRunning reloads a mod through Run's control channel:
// the old handler is gone, and the new one runs for the next login.
func TestModReloadWhileRunning(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{3}).Stream(), nil)
	logs := &syncBuffer{}
	v1 := testMod(`
local events = require "wa.events"
return function() events.on("login", function(p) p:send_message("v1") end) end`)
	z.Mods = &Mods{Scripts: []*mod.Scripts{v1}, IDs: []string{"example.test"}, Log: slog.New(slog.NewTextHandler(logs, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go z.Run(ctx)
	login := func(name string) []string {
		t.Helper()
		c := z.NewCharacter(name, Look{})
		s, err := z.JoinCharacter(ctx, &c, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Leave()
		var lines []string
		deadline := time.After(5 * time.Second)
		for len(lines) == 0 {
			select {
			case m := <-s.Out:
				lines = append(lines, systemLines(m.GetFrame().GetEvents())...)
			case <-deadline:
				t.Fatalf("no line for %s, logs: %s", name, logs)
			}
		}
		return lines
	}
	if got := login("a"); strings.Join(got, "|") != "v1" {
		t.Fatalf("v1: %q", got)
	}
	v2 := testMod(`
local events = require "wa.events"
return function() events.on("login", function(p) p:send_message("v2") end) end`)
	if err := z.ReloadMod(ctx, v2); err != nil {
		t.Fatal(err)
	}
	if got := login("b"); strings.Join(got, "|") != "v2" {
		t.Fatalf("after the reload: %q, logs: %s", got, logs)
	}
}

// TestModReloadBeforeLuaState reloads a mod in a zone that has not made
// its Lua state: the state it makes later uses the new files.
func TestModReloadBeforeLuaState(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{3}).Stream(), nil)
	v1 := testMod(`return function() end`)
	v1.Entries = nil
	z.Mods = &Mods{Scripts: []*mod.Scripts{v1}, IDs: []string{"example.test"}}
	t.Cleanup(z.stopScripts)
	v2 := testMod(`
local events = require "wa.events"
return function() events.on("login", function(p) p:send_message("v2") end) end`)
	if err := z.reloadScripts(v2); err != nil {
		t.Fatal(err)
	}
	z.startScripts()
	p := add(z, "a")
	z.stepAt(50)
	if got := systemLines(events(p)); strings.Join(got, "|") != "v2" {
		t.Fatalf("after the reload: %q", got)
	}
}
