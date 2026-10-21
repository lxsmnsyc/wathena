package zone

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// mapsScript runs the calls of area B8 on npc_event labels. Each label
// notes its results with player:send_message, or in the variable seen.
const mapsScript = `
local events = require "wa.events"
local maps = require "wa.maps"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"
local variables = require "wa.variables"
local context = require "wa.context"
local seen = variables.server_temporary_string("seen")
local function note(s) seen:set(seen:get() .. s .. ";") end
local list = {}
return function()
  local npc = context:get_instance()
  local function on(name, f) events.on("npc_event", f, { name = name }) end
  on("OnSpawn", function()
    list = spawning.monsters { monster = 1002, amount = 3, x = 30, y = 30, area = 1, label = "OnMyMobDead",
      display_name = "Jelly", size = "big" }
    note("spawned " .. #list .. " " .. maps.count_monsters("test", { label = "OnMyMobDead" }) .. "/" .. maps.count_monsters("test"))
  end)
  on("OnMyMobDead", function(event)
    note("dead " .. (event.player and event.player.name or "-") .. " " .. maps.count_monsters("test", { label = npc.unique_name .. "::OnMyMobDead" }))
  end)
  on("OnKill", function() list[1]:kill() end)
  on("OnRemove", function() list[2]:remove() end)
  on("OnRemoveAll", function() maps.remove_monsters("test", { label = "OnMyMobDead", run_labels = true }) end)
  on("OnRect", function()
    local got = spawning.monsters { monster = 1002, amount = 4, rectangle = { { 5, 5 }, { 7, 6 } }, label = "OnRect" }
    for _, m in ipairs(got) do note("rect " .. m.x .. "," .. m.y) end
    local none = spawning.monsters { monster = 1002, amount = 2, rectangle = { { 2, 2 }, { 3, 3 } }, label = "OnWall" }
    note("walls " .. #none)
  end)
  on("OnCells", function()
    maps.set_cells("test", 10, 10, 11, 10, "walkable", false)
    maps.set_cells("test", 20, 20, 21, 20, "walkable", false, { send = true })
    maps.set_cells("test", 2, 2, 3, 3, "no_vending", true)
    note("cells " .. tostring(maps.check_cell("test", 10, 10, "no_pass")) .. " " .. tostring(maps.check_cell("test", 12, 10, "pass"))
      .. " " .. tostring(maps.check_cell("test", 39, 5, "no_pass")) .. " " .. tostring(maps.check_cell("test", 2, 3, "no_vending")))
    local ok, err = pcall(maps.set_cells, "test", 1, 1, 2, 2, "basilica", true)
    note("basilica " .. tostring(ok) .. " " .. (tostring(err):match("Basilica is a status") and "why" or tostring(err)))
  end)
  on("OnFlags", function()
    note("memo " .. tostring(maps.get_flag("test", "nomemo")))
    maps.set_flag("test", "nomemo", true)
    note("memo " .. tostring(maps.get_flag("test", "nomemo")) .. " pvp " .. tostring(maps.get_flag("test", "pvp")))
    local ok, err = pcall(maps.set_flag, "test", "pvp", true)
    note("pvp " .. tostring(ok) .. " " .. (tostring(err):match("PvP is not ported") and "why" or tostring(err)))
    ok = pcall(maps.get_flag, "test", "nonsense")
    note("nonsense " .. tostring(ok))
  end)
  on("OnFree", function()
    local x, y = maps.find_free_cell("test", { x = 20, y = 20, range_x = 2, range_y = 2 })
    note("free " .. tostring(math.abs(x - 20) <= 2 and math.abs(y - 20) <= 2 and not (x == 20 and y == 20)))
    local cx, cy = maps.find_free_cell("test", { x = 21, y = 21, range_x = 0, range_y = 0 })
    note("center " .. tostring(cx) .. "," .. tostring(cy))
    note("wall " .. tostring(maps.find_free_cell("test", { x = 10, y = 10, range_x = 0, range_y = 0 })))
  end)
  on("OnCount", function()
    note("players " .. maps.count_players("test") .. " " .. maps.count_players("test", { rectangle = { { 0, 0 }, { 21, 21 } } })
      .. " " .. tostring(maps.count_players("nowhere")))
  end)
  on("OnWarp", function()
    maps.warp_players("test", "test", 5, 5, { rectangle = { { 0, 0 }, { 25, 25 } } })
  end)
  on("OnAnnounce", function()
    maps.announce("test", "near", { rectangle = { { 0, 0 }, { 5, 5 } } })
    maps.announce("test", "far", { rectangle = { { 30, 30 }, { 35, 35 } } })
  end)
  on("OnInfo", function()
    local m = monsters.get(1002)
    note("info " .. m.name .. " " .. m.level .. " " .. m.hp .. " " .. m.base_exp .. " " .. table.concat(m.modes, ","))
    note("none " .. tostring(monsters.get(999999)))
  end)
  on("OnMethods", function()
    local m = spawning.monsters { monster = 1002, x = 25, y = 25 }[1]
    note("hp " .. tostring(m:set_hp(5)) .. " " .. m.hp .. " " .. tostring(m:set_hp(0)) .. " " .. tostring(m:set_hp(999)) .. " " .. m.hp)
    m:set_damage_immune(true)
    m:set_modes({ "can_move", "aggressive" })
    m:set_speed(100)
    note("warp " .. tostring(m:warp_to(26, 27)) .. " " .. m.x .. "," .. m.y)
  end)
end`

// mapsZone is npcZone with monster records, temporary server
// variables and the script of area B8.
func mapsZone(t *testing.T) (*Zone, *Player, *entity) {
	t.Helper()
	z := New("map/test", open(40), rng.FromKey([32]byte{5}).Stream(), nil)
	z.Items = testItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	logs := &syncBuffer{}
	st := &battle.Status{Level: 3, MaxHP: 30, Hit: 200, ADelay: 1000, AMotion: 500, DMotion: 300, Mode: 0x81, RHW: battle.Weapon{Atk: 10, Atk2: 10, Range: 1}}
	z.Mods = &Mods{Limits: testLimits, Scripts: []*mod.Scripts{npcMod(mapsScript)}, IDs: []string{"core", "example.test"},
		Log: slog.New(slog.NewTextHandler(logs, nil)), Server: variables.NewServer(nil, nil), Temporary: variables.NewTemporary(),
		Monsters: map[uint32]Spawn{1002: {Mob: 1002, Name: "Poring", Amount: 1, Speed: 400, Status: st}}}
	z.SetNPCs([]NPC{{Name: "example.test:Guide", DisplayName: "Guide", X: 20, Y: 20, Sprite: 112, Script: "example.test:scripts/npc.lua"}})
	t.Cleanup(func() {
		z.stopScripts()
		if strings.Contains(logs.String(), "level=WARN") || strings.Contains(logs.String(), "level=ERROR") {
			t.Errorf("logs: %s", logs)
		}
	})
	p := add(z, "a")
	p.entity.x, p.entity.y = 21, 20
	z.stepAt(50)
	events(p)
	n := npcNamed(z, "Guide")
	if n == nil || n.npc.script == nil {
		t.Fatalf("the NPC did not start: %s", logs)
	}
	return z, p, n
}

// fire runs an NPC event without a player, then the events it sent.
func fire(t *testing.T, z *Zone, n *entity, label string) {
	t.Helper()
	if _, _, err := z.scripts.FireNPCEvent(n.npc.script, strings.ToLower(label), 0); err != nil {
		t.Fatal(err)
	}
	z.runScripts()
	z.flushDeferred()
	z.runScripts()
}

// seen reads and clears what the script noted.
func seen(t *testing.T, z *Zone) string {
	t.Helper()
	k := variables.TempKey{Mod: "example.test", Name: "seen"}
	s, _ := z.Mods.Temporary.Get(k)
	if err := z.Mods.Temporary.Set(k, ""); err != nil {
		t.Fatal(err)
	}
	return strings.Trim(s, `"`)
}

// TestSpawnLabels checks spawning.monsters' label, name and size, and
// the kills of area B8: unitkill runs the label, killmonster does not
// unless run_labels is set, and mobcount follows.
func TestSpawnLabels(t *testing.T) {
	z, p, n := mapsZone(t)
	fire(t, z, n, "OnSpawn")
	if got := seen(t, z); got != "spawned 3 3/3;" {
		t.Fatalf("got %q", got)
	}
	jelly := monstersOf(z, 1002)
	if len(jelly) != 3 || jelly[0].monster.spawn.Name != "Jelly" {
		t.Fatalf("monsters: %d", len(jelly))
	}
	// Players see the name and the giant body effect.
	p.entity.x, p.entity.y = 30, 31
	z.stepAt(z.now + 50)
	var named, giant bool
	for _, ev := range events(p) {
		if a := ev.GetAppear(); a != nil && a.GetName() == "Jelly" {
			named = true
		}
		if e := ev.GetEffect(); e != nil && e.GetEffectId() == efGiantBody2 {
			giant = true
		}
	}
	if !named || !giant {
		t.Errorf("named %v, giant %v", named, giant)
	}
	fire(t, z, n, "OnKill")
	if got := seen(t, z); got != "dead - 2;" {
		t.Fatalf("after kill: %q", got)
	}
	fire(t, z, n, "OnRemove")
	if got := seen(t, z); got != "" {
		t.Fatalf("remove ran the label: %q", got)
	}
	// A player's kill runs the label with the player.
	left := monstersOf(z, 1002)
	if len(left) != 1 {
		t.Fatalf("%d left", len(left))
	}
	z.die(left[0], p.entity)
	z.runScripts()
	if got := seen(t, z); got != "dead a 0;" {
		t.Fatalf("player kill: %q", got)
	}
	fire(t, z, n, "OnSpawn")
	seen(t, z)
	fire(t, z, n, "OnRemoveAll")
	// Each label reads the count of the moment its monster died, as
	// rAthena runs it inside killmonster's loop (script.cpp 11463,
	// mob.cpp 3600): the monsters not killed yet still count.
	if got := seen(t, z); got != "dead - 2;dead - 1;dead - 0;" {
		t.Fatalf("remove_monsters with labels: %q", got)
	}
}

// TestSpawnRectangle checks areamonster's rectangle and its failure: a
// rectangle with no walkable cell places no monster.
func TestSpawnRectangle(t *testing.T) {
	z, _, n := mapsZone(t)
	for _, c := range [][2]int{{2, 2}, {3, 2}, {2, 3}, {3, 3}} {
		z.grid.Cells[c[0]+c[1]*z.grid.Width] = 0
	}
	fire(t, z, n, "OnRect")
	got := seen(t, z)
	if strings.Count(got, "rect ") != 4 || !strings.HasSuffix(got, "walls 0;") {
		t.Fatalf("got %q", got)
	}
	for _, part := range strings.Split(got, ";") {
		var x, y int
		if _, err := fmt.Sscanf(part, "rect %d,%d", &x, &y); err == nil && (x < 5 || x > 7 || y < 5 || y > 6) {
			t.Errorf("%s is outside the rectangle", part)
		}
	}
}

// TestCellsAndFlags checks setcell, checkcell, the cells sent to
// players, and the map flags. A cell change goes to the players only
// with send, as rAthena's setcell sends nothing (script.cpp 20631) and
// setwall sends its cells (map.cpp 3511, 3538).
func TestCellsAndFlags(t *testing.T) {
	z, p, n := mapsZone(t)
	events(p)
	fire(t, z, n, "OnCells")
	if got := seen(t, z); got != "cells true true true true;basilica false why;" {
		t.Fatalf("got %q", got)
	}
	if !z.grid.NoPass(10, 10) || !z.grid.NoPass(11, 10) || z.grid.NoPass(12, 10) {
		t.Error("the cells did not change")
	}
	cells := func(ev []*wathenav1.Event) []uint32 {
		var out []uint32
		for _, e := range ev {
			out = append(out, e.GetMapCells().GetCells()...)
		}
		return out
	}
	z.stepAt(z.now + 50)
	want := []uint32{packCell(20, 20, path.Shootable), packCell(21, 20, path.Shootable)}
	if got := cells(events(p)); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the player got %v, want only the sent cells %v", got, want)
	}
	// A player who enters gets the sent cells alone.
	q := add(z, "b")
	z.stepAt(z.now + 50)
	if got := cells(events(q)); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("a new player got %v, want %v", got, want)
	}
	// The other map's view of the cells is the shared copy.
	if g := z.shared.grid.Load(); !g.NoPass(10, 10) {
		t.Error("the shared grid did not change")
	}
	p.entity.x, p.entity.y = 2, 2
	if !z.cellMark(2, 2, markNoVending) {
		t.Error("no_vending is not set")
	}
	fire(t, z, n, "OnFlags")
	if got := seen(t, z); got != "memo false;memo true pvp false;pvp false why;nonsense false;" {
		t.Fatalf("flags: %q", got)
	}
	if !z.mapFlag("nomemo") {
		t.Error("nomemo is not set")
	}
}

// TestFreeCellsAndCounts checks getfreecell, getmapusers, getareausers,
// areawarp, areaannounce and getmonsterinfo.
func TestFreeCellsAndCounts(t *testing.T) {
	z, p, n := mapsZone(t)
	z.grid.Cells[10+10*z.grid.Width] = 0
	fire(t, z, n, "OnFree")
	if got := seen(t, z); got != "free true;center 21,21;wall nil;" {
		t.Fatalf("free cells: %q", got)
	}
	q := add(z, "b")
	q.entity.x, q.entity.y = 30, 30
	fire(t, z, n, "OnCount")
	if got := seen(t, z); got != "players 2 1 nil;" {
		t.Fatalf("counts: %q", got)
	}
	events(p)
	fire(t, z, n, "OnWarp")
	if p.entity.x != 5 || p.entity.y != 5 || q.entity.x != 30 {
		t.Errorf("after the warp: a at %d,%d, b at %d,%d", p.entity.x, p.entity.y, q.entity.x, q.entity.y)
	}
	fire(t, z, n, "OnAnnounce")
	z.stepAt(z.now + 50)
	var lines []string
	for _, e := range events(p) {
		if c := e.GetChat(); c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_ANNOUNCEMENT {
			lines = append(lines, c.GetText())
		}
	}
	if strings.Join(lines, "|") != "near" {
		t.Errorf("announcements: %v", lines)
	}
	fire(t, z, n, "OnInfo")
	if got := seen(t, z); got != "info Poring 3 30 0 can_move,can_attack;none nil;" {
		t.Fatalf("info: %q", got)
	}
}

// TestMonsterMethodsB8 checks setunitdata's UMOB_ values and unitwarp.
func TestMonsterMethodsB8(t *testing.T) {
	z, p, n := mapsZone(t)
	fire(t, z, n, "OnMethods")
	if got := seen(t, z); got != "hp true 5 false true 30;warp true 26,27;" {
		t.Fatalf("got %q", got)
	}
	var m *entity
	for _, e := range monstersOf(z, 1002) {
		m = e
	}
	if !m.monster.immune || isEnemy(p.entity, m) {
		t.Error("the monster is not immune")
	}
	if m.status.Mode != 0x5 || m.speed != 100 || z.Mods.Monsters[1002].Status.Mode != 0x81 {
		t.Errorf("mode %x speed %d, record's mode %x", m.status.Mode, m.speed, z.Mods.Monsters[1002].Status.Mode)
	}
}

// TestScriptAIEnemies checks battle_check_target for a script's monster
// with an AI and no master.
func TestScriptAIEnemies(t *testing.T) {
	player := &entity{player: &Player{}}
	plain := &entity{monster: &monster{}}
	attack := &entity{monster: &monster{specialAI: aiAttack}}
	wave := &entity{monster: &monster{specialAI: aiWaveMode}}
	for i, c := range []struct {
		a, b *entity
		want bool
	}{
		{attack, plain, true}, {plain, attack, true}, {attack, player, false}, {player, attack, true},
		{plain, wave, false}, {wave, plain, true}, {attack, wave, false}, {plain, plain, false},
	} {
		if got := isEnemy(c.a, c.b); got != c.want {
			t.Errorf("case %d: isEnemy = %v", i, got)
		}
	}
}
