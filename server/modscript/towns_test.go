package modscript

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// townHost is placeHost, whose player 1 entered t1 and the crypt.
type townHost struct{ placeHost }

func (h townHost) HasVisited(id int64, mapID string) (bool, error) {
	return id == 1 && (mapID == "gen:town:t1" || mapID == "gen:dungeon:crypt:1"), nil
}

// testAtlas is the layout of testPlaces: t1 links to the crypt's first
// floor, and t2 stands apart.
func testAtlas() *layout.Atlas {
	l := &layout.Layout{
		Maps: []layout.Map{
			{ID: "gen:town:t1", Kind: gen.KindTown, Town: 1, Name: "Valheim", Levels: [2]int{1, 10}},
			{ID: "gen:town:t2", Kind: gen.KindTown, Town: 2, Name: "Skarvik", Levels: [2]int{40, 49}, Distance: 5},
			{ID: "gen:dungeon:crypt:1", Kind: gen.KindDungeon, Dungeon: "crypt", Floor: 1, Floors: 1, Name: "Crypt of Valheim F1", Levels: [2]int{6, 15}},
		},
		Links: []layout.Link{{A: layout.End{Map: "gen:town:t1", Tag: "portal"}, B: layout.End{Map: "gen:dungeon:crypt:1", Tag: "entrance"}}},
		Home:  "gen:town:t1",
	}
	return layout.NewAtlas(l, layout.AtlasOptions{})
}

// runTowns runs a mod's setup with the places and the atlas, and returns
// the messages to player 1 and the log.
func runTowns(t *testing.T, a *layout.Atlas, read bool, src string) (string, string) {
	t.Helper()
	var buf logBuffer
	h := newHost()
	sc := modScripts("example.a", nil, nil, "scripts/main.lua", src)
	if !read {
		delete(sc.Grants, "game.read")
	}
	opts := Options{Host: townHost{placeHost{h}}, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{"example.a"}, Atlas: a}
	if a != nil {
		opts.Places = testPlaces()
	}
	r, err := New(opts, []*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	r.Fire("login", Player(1))
	return strings.ReplaceAll(strings.Join(h.messages, "|"), "1:", ""), buf.String()
}

const townsScript = `
local events = require "wa.events"
local towns = require "wa.towns"
local maps = require "wa.maps"
local places = require "wa.places"
return function()
  events.on("login", function(player)
    local out = {}
    for _, town in ipairs(towns.list()) do
      local lo, hi = town:get_band()
      out[#out + 1] = town:get_id() .. "=" .. town:get_name() .. "@" .. town:get_map() .. " " .. lo .. "-" .. hi
    end
    player:send_message(table.concat(out, " "))
    local visited = towns.list { visited_by = player }
    player:send_message(#visited .. " " .. visited[1]:get_name() .. " " .. tostring(visited[1] == towns.get("t1")))
    player:send_message(towns.nearest(player):get_id() .. " " .. tostring(towns.get("t9")))
    player:send_message(tostring(player:has_visited(towns.get("t1"))) .. " " .. tostring(player:has_visited(towns.get("t2"))))
    player:send_message(maps.get_display_name("gen:dungeon:crypt:1") .. " " .. tostring(maps.get_display_name("prontera")))
    local lo, hi = places.find("dungeon:crypt", "entrance")[1]:get_band()
    player:send_message(lo .. "-" .. hi)
    for _, bad in ipairs({
      function() towns.get("Valheim") end,
      function() towns.list { visited = player } end,
      function() player:has_visited("t1") end,
      function() towns.get("t1").name = "x" end,
    }) do
      local ok, err = pcall(bad)
      player:send_message(tostring(ok) .. ": " .. tostring(err))
    end
  end)
end
`

// TestTowns checks wa.towns, player:has_visited, maps.get_display_name
// and place:get_band (review decision 43).
func TestTowns(t *testing.T) {
	got, logs := runTowns(t, testAtlas(), true, townsScript)
	lines := strings.Split(got, "|")
	want := []string{
		"t1=Valheim@gen:town:t1 1-10 t2=Skarvik@gen:town:t2 40-49",
		"1 Valheim true",
		"t1 nil",
		"true false",
		"Crypt of Valheim F1 nil",
		"6-15",
	}
	if len(lines) < len(want)+4 {
		t.Fatalf("messages %q, log %s", got, logs)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: %q, want %q", i, lines[i], w)
		}
	}
	for i, frag := range []string{"a town's ID is t and a number", "unknown option", "want a town", "read-only"} {
		if l := lines[len(want)+i]; !strings.HasPrefix(l, "false") || !strings.Contains(l, frag) {
			t.Errorf("error %d: %q, want %q", i, l, frag)
		}
	}
}

// TestTownsClassic: a classic world has no towns and no display names,
// and the calls need game.read.
func TestTownsClassic(t *testing.T) {
	got, logs := runTowns(t, nil, true, `
local events = require "wa.events"
local towns = require "wa.towns"
local maps = require "wa.maps"
return function()
  events.on("login", function(player)
    player:send_message(#towns.list() .. " " .. tostring(towns.nearest(player)) .. " " .. tostring(towns.get("t1")) .. " " .. tostring(maps.get_display_name("prontera")))
  end)
end
`)
	if got != "0 nil nil nil" {
		t.Errorf("classic: %q, log %s", got, logs)
	}
	_, logs = runTowns(t, testAtlas(), false, `
local events = require "wa.events"
local towns = require "wa.towns"
return function()
  events.on("login", function(player)
    towns.list()
  end)
end
`)
	if !strings.Contains(logs, "game.read") {
		t.Errorf("without game.read: %s", logs)
	}
}
