package world

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// playersScript is the script of TestPlayersAcrossZones. The floating
// Watcher of map/a hears the player events of a player of map/b: it
// reads the player's fields, fails a read of its items, gives it zeny
// at login and at logout, and finds it by name.
const playersScript = `
local events = require "wa.events"
local maps = require "wa.maps"
local variables = require "wa.variables"
local log = variables.server_temporary_string("log")
local function note(s) log:set(log:get() .. s .. ";") end
return function()
  events.on("login", function(player)
    note("login " .. player.name .. " " .. player.map .. " " .. tostring(player:exists()))
    local ok = pcall(function() return player:count_item(501) end)
    note("read " .. tostring(ok))
    player:give_zeny(7)
    local found = maps.find_player(player.name)
    note("found " .. tostring(found == player) .. " " .. found.map)
  end)
  events.on("map_enter", function(player) note("enter " .. player.map) end)
  events.on("logout", function(player)
    note("logout " .. player.name)
    player:give_zeny(11)
  end)
end`

// TestPlayersAcrossZones checks review decision 59's players across
// zones: an NPC of another map hears a player's login, map entry and
// logout, reads its fields from the world's directory, cannot read its
// items, finds it by name, and its actions run in the player's zone. The
// logout waits for them, so the last save has both gifts. It runs the
// world's goroutines, so go test -race checks it.
func TestPlayersAcrossZones(t *testing.T) {
	a, b := testZone("map/a"), testZone("map/b")
	sc := &mod.Scripts{Mod: "example.test", Files: map[string]string{"scripts/npc.lua": playersScript}, Grants: mod.Grants{}, Visible: []string{"example.test"}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	temp := variables.NewTemporary()
	mods := &zone.Mods{Scripts: []*mod.Scripts{sc}, IDs: []string{"core", "example.test"}, Log: slog.New(slog.DiscardHandler),
		Server: variables.NewServer(nil, nil), Temporary: temp}
	a.SetNPCs([]zone.NPC{{Name: "example.test:Watcher", DisplayName: "Watcher", Script: "example.test:scripts/npc.lua", Floating: true}})
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
	c := w.NewCharacter("Mover", zone.Look{})
	c.Map, c.X, c.Y = "map/b", 7, 8
	rec := &recorder{}
	s, err := w.JoinCharacter(ctx, &c, rec)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for range s.Out {
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(get(), "enter") {
		time.Sleep(10 * time.Millisecond)
	}
	s.Leave()
	if got := rec.last().Zeny; got != 18 {
		t.Errorf("the last save has %d zeny, want the 7 of login and the 11 of logout", got)
	}
	want := "login Mover b true;read false;found true b;enter b;logout Mover;"
	if got := get(); got != want {
		t.Fatalf("log %q, want %q", got, want)
	}
}
