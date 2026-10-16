package modscript

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// placeHost is a fake zone whose player 1 stands on a town's square.
type placeHost struct{ *fakeHost }

func (h placeHost) PlayerField(id int64, field string) (script.Value, error) {
	switch field {
	case "map":
		return script.String("gen:town:t1"), nil
	case "x":
		return script.Int(48), nil
	case "y":
		return script.Int(45), nil
	}
	return h.fakeHost.PlayerField(id, field)
}

// testPlaces is two towns and a crypt's floor, with a warp from t1 to
// the crypt.
func testPlaces() *places.Index {
	return places.New([]places.Map{
		{ID: "gen:town:t2", Places: []gen.Place{{Tags: []string{"blacksmith"}, X: 5, Y: 6}}},
		{ID: "gen:town:t1", Places: []gen.Place{
			{Tags: []string{"square"}, X: 48, Y: 45},
			{Tags: []string{"blacksmith"}, X: 20, Y: 70},
			{Tags: []string{"gate", "gate:south"}, X: 40, Y: 11},
		}},
		{ID: "gen:dungeon:crypt:1", Places: []gen.Place{{Tags: []string{"entrance"}, X: 8, Y: 9}}},
	}, []places.Link{{From: "gen:town:t1", X: 90, Y: 50, To: "gen:dungeon:crypt:1", ToX: 8, ToY: 12}})
}

// runPlaces runs a mod's setup with the index and returns the messages
// to player 1 and the log.
func runPlaces(t *testing.T, idx *places.Index, read bool, src string) (string, string) {
	t.Helper()
	var buf logBuffer
	h := newHost()
	sc := modScripts("example.a", nil, nil, "scripts/main.lua", src)
	if !read {
		delete(sc.Grants, "game.read")
	}
	r, err := New(Options{Host: placeHost{h}, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{"example.a"}, Places: idx},
		[]*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	r.Fire("login", Player(1))
	return strings.ReplaceAll(strings.Join(h.messages, "|"), "1:", ""), buf.String()
}

const placesScript = `
local events = require "wa.events"
local places = require "wa.places"
return function()
  events.on("login", function(player)
    local out = {}
    for _, p in ipairs(places.find("town:any", "blacksmith")) do
      local x, y = p:get_position()
      out[#out + 1] = p:get_map() .. "@" .. x .. "," .. y .. "/" .. p:get_area()
    end
    player:send_message(table.concat(out, " "))
    local gate = places.find("town:t1", "gate:south")[1]
    player:send_message(table.concat(gate:get_tags(), "+"))
    player:send_message(tostring(#places.find("dungeon:crypt", "entrance")) .. " " .. tostring(#places.find("map:gen:town:t2", "square")))
    local near = places.nearest(player, "blacksmith")
    player:send_message(near:get_map() .. " " .. tostring(near == places.find("town:t1", "blacksmith")[1]))
    local entrance = places.nearest(player, "entrance")
    player:send_message(entrance:get_area() .. " " .. tostring(places.nearest(player, "inn")))
    for _, bad in ipairs({
      function() places.find("field:any", "x") end,
      function() places.find("town:any", "Bad Tag") end,
      function() places.nearest(nil, "x") end,
      function() gate.x = 1 end,
    }) do
      local ok, err = pcall(bad)
      player:send_message(tostring(ok) .. ": " .. tostring(err))
    end
  end)
end
`

// TestPlaces checks wa.places (review decision 42): find's order and
// scopes, the handle's methods, nearest by the map graph and distance,
// and the errors.
func TestPlaces(t *testing.T) {
	got, logs := runPlaces(t, testPlaces(), true, placesScript)
	lines := strings.Split(got, "|")
	want := []string{
		"gen:town:t1@20,70/town:t1 gen:town:t2@5,6/town:t2",
		"gate+gate:south",
		"1 0",
		"gen:town:t1 true",
		"dungeon:crypt:1 nil",
	}
	if len(lines) < len(want)+4 {
		t.Fatalf("messages %q, log %s", got, logs)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: %q, want %q", i, lines[i], w)
		}
	}
	for i, frag := range []string{"a scope is", "the tag must be", "want a player", "read-only"} {
		if l := lines[len(want)+i]; !strings.HasPrefix(l, "false") || !strings.Contains(l, frag) {
			t.Errorf("error %d: %q, want %q", i, l, frag)
		}
	}
}

// TestPlacesClassic: a classic world has no places, and the calls need
// game.read.
func TestPlacesClassic(t *testing.T) {
	got, logs := runPlaces(t, nil, true, `
local events = require "wa.events"
local places = require "wa.places"
return function()
  events.on("login", function(player)
    player:send_message(tostring(#places.find("town:any", "blacksmith")) .. " " .. tostring(places.nearest(player, "square")))
  end)
end
`)
	if got != "0 nil" {
		t.Errorf("classic: %q, log %s", got, logs)
	}
	got, logs = runPlaces(t, testPlaces(), false, `
local events = require "wa.events"
local places = require "wa.places"
return function()
  events.on("login", function(player)
    local ok, err = pcall(places.find, "town:any", "blacksmith")
    player:send_message(tostring(err))
  end)
end
`)
	if !strings.Contains(got, "needs the permission game.read") {
		t.Errorf("without game.read: %q, log %s", got, logs)
	}
}
