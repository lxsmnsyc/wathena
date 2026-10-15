package zone

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// worldZone is scriptZone for a mod with every permission, monster
// records for spawning.monsters, and a server clock that starts at
// 2026-10-31 19:59 UTC and moves with the zone clock.
func worldZone(t *testing.T, files ...string) (*Zone, *Player, *entity, *syncBuffer) {
	t.Helper()
	mob := &battle.Status{MaxHP: 30, Hit: 200, ADelay: 1000, AMotion: 500, DMotion: 300, Mode: 0x81, RHW: battle.Weapon{Atk: 10, Atk2: 10, Range: 1}}
	reward := &progression.Mob{ID: 1002, Level: 3, MaxHP: 30, BaseExp: 15, JobExp: 25, Drops: []progression.Drop{{Item: 909, Rate: 1}}}
	mob.Level = 3
	poring := Spawn{Mob: 1002, Name: "Poring", Amount: 1, X: 20, Y: 20, Speed: 400, ViewRange: 10, ChaseRange: 12, Status: mob, RespawnMs: 60000, Reward: reward}
	z := New("map/test", open(40), rng.FromKey([32]byte{3}).Stream(), []Spawn{poring})
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 100, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	start := time.Date(2026, 10, 31, 19, 59, 0, 0, time.UTC)
	z.Clock = func() time.Time { return start.Add(time.Duration(z.now) * time.Millisecond) }
	sc := &mod.Scripts{Mod: "example.test", Entries: []string{"scripts/main.lua"}, Files: map[string]string{}, Visible: []string{"example.test"},
		Grants: mod.Grants{"game.read": true, "event.listen": true, "npc.create": true, "rules.override": true}}
	for i := 0; i+1 < len(files); i += 2 {
		sc.Files[files[i]] = files[i+1]
	}
	bigger := poring
	bigger.Mob, bigger.Name, bigger.Sprite = 100_000, "Pumpkin Poring", 1002
	logs := &syncBuffer{}
	z.Mods = &Mods{Limits: testLimits, Scripts: []*mod.Scripts{sc}, IDs: []string{"core", "example.test"}, Log: slog.New(slog.NewTextHandler(logs, nil)),
		Monsters: map[uint32]Spawn{1002: poring, 100_000: bigger}}
	z.startScripts()
	t.Cleanup(z.stopScripts)
	if z.scripts == nil || z.scripts.Context("example.test") == nil {
		t.Fatalf("the mod did not start: %s", logs)
	}
	p := add(z, "a")
	z.runScripts()
	var m *entity
	for e := range z.entities.all() {
		if e.monster != nil && e.monster.spawn.owner == 0 {
			m = e
		}
	}
	p.entity.x, p.entity.y = m.x+2, m.y
	return z, p, m, logs
}

func monstersOf(z *Zone, id uint32) []*entity {
	var out []*entity
	for e := range z.entities.all() {
		if e.monster != nil && e.monster.spawn.Mob == id {
			out = append(out, e)
		}
	}
	return out
}

func TestSpawnedMonstersAndDeath(t *testing.T) {
	z, p, m, logs := worldZone(t, "scripts/main.lua", `
local events = require "wa.events"
local spawning = require "wa.spawning"
return function()
  local pumpkins = spawning.monsters { monster = 100000, amount = 3, x = 30, y = 30, area = 2 }
  events.on("death", function(death)
    death.killer:send_message("pumpkin " .. death.victim.name .. " " .. tostring(death.victim == pumpkins[1]))
  end, { victims = pumpkins })
  events.on("death", function(death)
    death.victim:send_message("killed by " .. death.killer.name)
  end, { victim_kind = "player" })
end`)
	z.stepAt(50)
	pumpkins := monstersOf(z, 100_000)
	if len(pumpkins) != 3 {
		t.Fatalf("%d pumpkins, logs: %s", len(pumpkins), logs)
	}
	for _, e := range pumpkins {
		if abs(e.x-30) > 2 || abs(e.y-30) > 2 {
			t.Errorf("a pumpkin at %d,%d, outside the area", e.x, e.y)
		}
	}
	// Players see the sprite of the monster it reuses.
	saw := false
	for _, ev := range events(p) {
		if a := ev.GetAppear(); a != nil && a.GetName() == "Pumpkin Poring" {
			saw = a.GetLook().GetBody() == 1002
		}
	}
	p.entity.x, p.entity.y = 29, 29
	z.stepAt(100)
	for _, ev := range events(p) {
		if a := ev.GetAppear(); a != nil && a.GetName() == "Pumpkin Poring" {
			saw = a.GetLook().GetBody() == 1002
		}
	}
	if !saw {
		t.Error("the pumpkin does not show with the poring sprite")
	}
	z.die(pumpkins[0], p.entity)
	z.runScripts()
	z.die(p.entity, m)
	z.stepAt(150)
	got := strings.Join(systemLines(events(p)), "|")
	if got != "pumpkin Pumpkin Poring true|killed by Poring" {
		t.Fatalf("got %q, logs: %s", got, logs)
	}
	// A spawn without respawn does not come back.
	z.stepAt(20_000)
	if n := len(monstersOf(z, 100_000)); n != 2 {
		t.Fatalf("%d pumpkins after one died", n)
	}
	// A reload ends the mod's context: its monsters go.
	if err := z.reloadScripts(testModFiles("return function() end")); err != nil {
		t.Fatal(err)
	}
	if n := len(monstersOf(z, 100_000)); n != 0 {
		t.Fatalf("%d pumpkins after the context ended", n)
	}
	if n := len(monstersOf(z, 1002)); n != 0 && n != 1 {
		t.Fatalf("the map's own porings changed: %d", n)
	}
	if len(z.owned) != 0 {
		t.Errorf("owned spawns left: %v", z.owned)
	}
}

func testModFiles(main string) *mod.Scripts {
	return &mod.Scripts{Mod: "example.test", Entries: []string{"scripts/main.lua"}, Files: map[string]string{"scripts/main.lua": main},
		Visible: []string{"example.test"}, Grants: mod.Grants{"game.read": true, "event.listen": true, "npc.create": true, "rules.override": true}}
}

func TestRespawnWhileContextLives(t *testing.T) {
	z, _, _, logs := worldZone(t, "scripts/main.lua", `
local spawning = require "wa.spawning"
return function()
  spawning.monsters { monster = 100000, amount = 2, respawn = true }
end`)
	pumpkins := monstersOf(z, 100_000)
	if len(pumpkins) != 2 {
		t.Fatalf("%d pumpkins, logs: %s", len(pumpkins), logs)
	}
	z.die(pumpkins[0], nil)
	z.stepAt(modscript.DefaultRespawnMs + 100)
	if n := len(monstersOf(z, 100_000)); n != 2 {
		t.Fatalf("%d pumpkins after the respawn time", n)
	}
	z.die(monstersOf(z, 100_000)[0], nil)
	if err := z.reloadScripts(testModFiles(`
local spawning = require "wa.spawning"
return function()
  spawning.monsters { monster = 100000, respawn = true, respawn_milliseconds = 20000 }
end`)); err != nil {
		t.Fatal(err)
	}
	if n := len(monstersOf(z, 100_000)); n != 1 {
		t.Fatalf("%d pumpkins after the reload", n)
	}
	start := z.now
	z.die(monstersOf(z, 100_000)[0], nil)
	z.stepAt(start + modscript.DefaultRespawnMs + 100)
	if n := len(monstersOf(z, 100_000)); n != 0 {
		t.Fatalf("a pumpkin with a 20 second respawn came back after 5")
	}
	z.stepAt(start + 20_100)
	if n := len(monstersOf(z, 100_000)); n != 1 {
		t.Fatalf("%d pumpkins after 20 seconds", n)
	}
	if err := z.reloadScripts(testModFiles("return function() end")); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 30_000)
	if n := len(monstersOf(z, 100_000)); n != 0 {
		t.Fatalf("%d pumpkins came back after the context ended", n)
	}
}

func TestHooksChangeKills(t *testing.T) {
	z, p, m, logs := worldZone(t, "scripts/main.lua", `
local hooks = require "wa.hooks"
return function()
  hooks.on("drop_rate", function(drop, next)
    if drop.monster.name == "Poring" and drop.player.name == "a" then return 10000 end
    return next(drop)
  end)
  hooks.on("experience", function(exp, next)
    local base, job = next(exp)
    return base * 2, job
  end)
end`)
	z.stepAt(50)
	events(p)
	kill(t, z, p, m)
	// 30 base exp, twice the 15, levels up once and keeps 9, the most a
	// level up without multi_level_up leaves. 15 would keep 5.
	if p.progress.BaseLevel != 2 || p.progress.BaseExp != 9 {
		t.Errorf("progress %+v, logs: %s", p.progress, logs)
	}
	items := 0
	for e := range z.entities.all() {
		if e.item != nil && e.item.id == 909 {
			items++
		}
	}
	if items != 1 {
		t.Errorf("%d jellopies fell, want 1 at a hooked rate of 100%%", items)
	}
}

func TestSchedulesInZone(t *testing.T) {
	z, p, _, logs := worldZone(t, "scripts/main.lua", `
local schedules = require "wa.schedules"
local events = require "wa.events"
return function()
  events.on("login", function(player)
    schedules.daily("20:00", function() player:send_message("eight") end)
  end)
end`)
	z.stepAt(50)
	events(p)
	z.stepAt(59_000)
	if got := systemLines(events(p)); len(got) != 0 {
		t.Fatalf("before 20:00: %q", got)
	}
	z.stepAt(60_050)
	if got := strings.Join(systemLines(events(p)), "|"); got != "eight" {
		t.Fatalf("at 20:00: %q, logs: %s", got, logs)
	}
}

func TestEventRunInZone(t *testing.T) {
	z, p, _, logs := worldZone(t, "scripts/main.lua", "return function() end", "scripts/events/boss.lua", `
local spawning = require "wa.spawning"
local maps = require "wa.maps"
local context = require "wa.context"
return function()
  spawning.monsters { monster = 100000, x = 10, y = 10 }
  maps.announce("test", context:get_instance().name .. " began")
end`)
	if err := z.startEvent(EventRun{ID: 1, Name: "example.test:Boss", Entry: "example.test:scripts/events/boss.lua"}); err != nil {
		t.Fatalf("%v, logs: %s", err, logs)
	}
	if n := len(monstersOf(z, 100_000)); n != 1 {
		t.Fatalf("%d bosses", n)
	}
	if err := z.startEvent(EventRun{ID: 1, Name: "example.test:Boss", Entry: "example.test:scripts/events/boss.lua"}); err == nil {
		t.Fatal("a run started twice")
	}
	z.endEvent(1)
	if n := len(monstersOf(z, 100_000)); n != 0 {
		t.Fatalf("%d bosses after the run", n)
	}
	select {
	case a := <-z.announces:
		if a.text != "example.test:Boss began" {
			t.Fatalf("announced %q", a.text)
		}
	default:
		t.Fatal("no announcement")
	}
	_ = p
}
