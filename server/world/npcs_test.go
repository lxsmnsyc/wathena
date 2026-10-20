package world

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// crossScript is the script of the NPCs of TestNPCsAcrossZones. The
// floating Caller runs in the start zone and, at its setup, sends the
// Listener of map/b fifty events in order, changes an NPC variable of
// it, hides it, asks for its state, and sends ::OnAll to every NPC. The
// Listener notes each event in the temporary server variable log.
const crossScript = `
local context = require "wa.context"
local events = require "wa.events"
local world = require "wa.world"
local variables = require "wa.variables"
local log = variables.npc_string("log")
local hits = variables.npc_integer("hits")
local results = variables.server_temporary_string("results")
return function()
  local npc = context:get_instance()
  local function note(s) log:set(npc, log:get(npc) .. s .. ";") end
  for i = 1, 50 do
    events.on("npc_event", function(event)
      hits:add(npc, 1)
      note(event.name)
    end, { name = "OnE" .. i })
  end
  events.on("npc_event", function(event) note("state=" .. npc.state .. ",hits=" .. hits:get(npc)) end, { name = "OnReport" })
  events.on("npc_event", function(event) note("all") end, { name = "OnAll" })
  if npc.unique_name == "Caller" then
    local listener = world.get_npc("Listener#l1")
    local fired = {}
    for i = 1, 50 do
      fired[#fired + 1] = tostring(listener:fire_event("OnE" .. i))
    end
    hits:add(listener, 100)
    listener:hide()
    listener:fire_event("OnReport")
    results:set(table.concat(fired, ",", 1, 2) .. " " .. listener.map .. " " .. listener.full_name .. " " .. world.fire_npc_event("OnAll"))
  end
end`

// TestNPCsAcrossZones checks review decision 58's messages between
// zones: events reach an NPC of another zone in the order sent, after
// the step that sent them, and start a zone that has not started its
// NPCs. NPC variables are the same value in every zone, and a change of
// another zone's NPC runs before the next event the same zone sends.
// It runs the world's goroutines, so go test -race checks it.
func TestNPCsAcrossZones(t *testing.T) {
	a, b, c := testZone("map/a"), testZone("map/b"), testZone("map/c")
	sc := &mod.Scripts{Mod: "example.test", Files: map[string]string{"scripts/npc.lua": crossScript}, Grants: mod.Grants{}, Visible: []string{"example.test"}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	temp := variables.NewTemporary()
	mods := &zone.Mods{Scripts: []*mod.Scripts{sc}, IDs: []string{"core", "example.test"}, Log: slog.New(slog.DiscardHandler),
		Server: variables.NewServer(nil, nil), Temporary: temp}
	s := "example.test:scripts/npc.lua"
	a.SetNPCs([]zone.NPC{{Name: "example.test:Caller", DisplayName: "Caller", Script: s, Floating: true}})
	b.SetNPCs([]zone.NPC{{Name: "example.test:Listener#l1", DisplayName: "Listener", X: 5, Y: 5, Sprite: 112, Script: s}})
	c.SetNPCs([]zone.NPC{{Name: "example.test:Listener", DisplayName: "Listener", X: 5, Y: 5, Sprite: 112, Script: s}})
	for _, z := range []*zone.Zone{a, b, c} {
		z.Mods = mods
	}
	w, err := New("map/a", a, b, c)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	defer func() {
		cancel()
		<-w.Stopped()
	}()
	get := func(npc, name string) string {
		v, _ := temp.Get(variables.TempKey{Mod: "example.test", NPC: npc, Name: name})
		return strings.Trim(v, `"`)
	}
	logs := func() []string {
		return []string{get("example.test:Caller", "log"), get("example.test:Listener#l1", "log"), get("example.test:Listener", "log")}
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		done := true
		for _, l := range logs() {
			done = done && strings.HasSuffix(l, "all;")
		}
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	want := ""
	for i := 1; i <= 50; i++ {
		want += fmt.Sprintf("one%d;", i)
	}
	want += "state=disabled,hits=150;all;"
	if got := logs(); got[0] != "all;" || got[1] != want || got[2] != "all;" {
		t.Fatalf("logs:\n%q\nwant the Listener#l1's %q", got, want)
	}
	// fire_event to a zone that has not started its NPCs is true, and
	// fire_npc_event counts the NPCs of the zones that started: the
	// Caller.
	v, _ := temp.Get(variables.TempKey{Mod: "example.test", Name: "results"})
	if got := strings.Trim(v, `"`); got != "true,true b Listener#l1 1" {
		t.Fatalf("results: %q", got)
	}
}

// presentScript is the script of TestNPCPresentAcrossZones: the floating
// Teller renames the Sign of map/b, gives it a title, moves it, and asks
// it what it is, and the Sign notes its name, title and cell. The Teller
// then reads the Sign's name and cell from its own zone.
const presentScript = `
local context = require "wa.context"
local events = require "wa.events"
local world = require "wa.world"
local variables = require "wa.variables"
local seen = variables.server_temporary_string("seen")
local read = variables.server_temporary_string("read")
return function()
  local npc = context:get_instance()
  events.on("npc_event", function(event)
    seen:set(npc.name .. " " .. npc.x .. "," .. npc.y)
    world.get_npc("Teller"):fire_event("OnRead")
  end, { name = "OnCheck" })
  events.on("npc_event", function(event)
    local sign = world.get_npc("Sign")
    read:set(sign.name .. " " .. sign.x .. "," .. sign.y)
  end, { name = "OnRead" })
  if npc.unique_name == "Teller" then
    local sign = world.get_npc("Sign")
    sign:set_display_name("Renamed")
    sign:set_title("<Title>")
    sign:move_to(7, 8)
    sign:say_aloud("hello", { to = "world" })
    sign:fire_event("OnCheck")
  end
end`

// TestNPCPresentAcrossZones checks that the presentation calls of review
// decision 58, area B5, reach an NPC of another zone in the order sent,
// before an event sent after them, and that the world's directory gives
// the NPC's new name and cell to the zone that changed it. It runs the
// world's goroutines, so go test -race checks it.
func TestNPCPresentAcrossZones(t *testing.T) {
	a, b := testZone("map/a"), testZone("map/b")
	sc := &mod.Scripts{Mod: "example.test", Files: map[string]string{"scripts/npc.lua": presentScript}, Grants: mod.Grants{}, Visible: []string{"example.test"}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	temp := variables.NewTemporary()
	mods := &zone.Mods{Scripts: []*mod.Scripts{sc}, IDs: []string{"core", "example.test"}, Log: slog.New(slog.DiscardHandler),
		Server: variables.NewServer(nil, nil), Temporary: temp}
	s := "example.test:scripts/npc.lua"
	a.SetNPCs([]zone.NPC{{Name: "example.test:Teller", DisplayName: "Teller", Script: s, Floating: true}})
	b.SetNPCs([]zone.NPC{{Name: "example.test:Sign", DisplayName: "Sign", X: 5, Y: 5, Sprite: 112, Script: s}})
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
	get := func(name string) string {
		v, _ := temp.Get(variables.TempKey{Mod: "example.test", Name: name})
		return strings.Trim(v, `"`)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && get("read") == "" {
		time.Sleep(10 * time.Millisecond)
	}
	if got := get("seen"); got != "Renamed 7,8" {
		t.Errorf("the Sign saw itself as %q", got)
	}
	if got := get("read"); got != "Renamed 7,8" {
		t.Errorf("the Teller read the Sign as %q", got)
	}
}
