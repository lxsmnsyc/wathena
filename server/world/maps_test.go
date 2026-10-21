package world

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// mapsScript is the script of TestMapsAcrossZones: the floating Hunter
// of map/a spawns monsters with its label on map/b, changes b's cells
// and flag, and kills the monsters with their labels. Each death runs
// the Hunter's label in its own zone, which reads b's counts, cells and
// flag.
const mapsScript = `
local context = require "wa.context"
local events = require "wa.events"
local maps = require "wa.maps"
local spawning = require "wa.spawning"
local variables = require "wa.variables"
local log = variables.server_temporary_string("log")
local function note(s) log:set(log:get() .. s .. ";") end
return function()
  local npc = context:get_instance()
  events.on("npc_event", function(event)
    note("dead " .. tostring(event.player) .. " " .. maps.count_monsters("b", { label = "OnMyMobDead" })
      .. " " .. tostring(maps.check_cell("b", 1, 1, "no_pass")) .. " " .. tostring(maps.get_flag("b", "nomemo")))
  end, { name = "OnMyMobDead" })
  if npc.unique_name == "Hunter" then
    local list = spawning.monsters { map = "b", monster = 1002, amount = 3, label = "OnMyMobDead" }
    note("spawned " .. #list .. " " .. maps.count_monsters("b", { label = "OnMyMobDead" }) .. " " .. maps.count_monsters("b")
      .. " " .. maps.count_players("b"))
    maps.set_cells("b", 1, 1, 2, 2, "walkable", false)
    maps.set_flag("b", "nomemo", true)
    maps.remove_monsters("b", { label = "OnMyMobDead", run_labels = true })
  end
end`

// TestMapsAcrossZones checks the calls of review decision 58, area B8,
// on another zone's map: a spawn, its count before the spawn ran, cells,
// a map flag, kills with labels, and the labels that run in the NPC's
// zone without a player. It runs the world's goroutines, so go test
// -race checks it.
func TestMapsAcrossZones(t *testing.T) {
	a, b := testZone("map/a"), testZone("map/b")
	sc := &mod.Scripts{Mod: "example.test", Files: map[string]string{"scripts/npc.lua": mapsScript}, Grants: mod.Grants{}, Visible: []string{"example.test"}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	temp := variables.NewTemporary()
	st := &battle.Status{Level: 3, MaxHP: 30, Mode: 0x81, RHW: battle.Weapon{Range: 1}}
	mods := &zone.Mods{Scripts: []*mod.Scripts{sc}, IDs: []string{"core", "example.test"}, Log: slog.New(slog.DiscardHandler),
		Server: variables.NewServer(nil, nil), Temporary: temp,
		Monsters: map[uint32]zone.Spawn{1002: {Mob: 1002, Name: "Poring", Amount: 1, Speed: 400, Status: st}}}
	a.SetNPCs([]zone.NPC{{Name: "example.test:Hunter", DisplayName: "Hunter", Script: "example.test:scripts/npc.lua", Floating: true}})
	a.Mods, b.Mods = mods, mods
	w, err := New("map/a", a, b)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	defer func() {
		cancel()
		<-w.Stopped()
	}()
	get := func() string {
		v, _ := temp.Get(variables.TempKey{Mod: "example.test", Name: "log"})
		return strings.Trim(v, `"`)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && strings.Count(get(), "dead") < 3 {
		time.Sleep(10 * time.Millisecond)
	}
	// The handles come back at once (review decision 59), and each label
	// reads the counts of the moment its monster died, as rAthena runs it
	// inside killmonster's loop.
	want := "spawned 3 3 3 0;dead nil 2 true true;dead nil 1 true true;dead nil 0 true true;"
	if got := get(); got != want {
		t.Fatalf("log %q, want %q", got, want)
	}
}
