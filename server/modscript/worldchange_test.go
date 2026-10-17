package modscript

import (
	"strings"
	"testing"
)

// TestWorldChanged: dungeon_rotated runs with { dungeon, date }, and
// region_changed with { region, season }, both read-only (review
// decision 47). An NPC context hears them too.
func TestWorldChanged(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local players = {}
return function()
  events.on("login", function(p) players[#players + 1] = p end)
  events.on("dungeon_rotated", function(rotation)
    local ok = pcall(function() rotation.date = "x" end)
    for _, p in ipairs(players) do p:send_message("rotated " .. rotation.dungeon .. " " .. rotation.date .. " " .. tostring(ok)) end
  end)
  events.on("region_changed", function(change)
    local ok = pcall(function() change.season = 0 end)
    for _, p in ipairs(players) do
      p:send_message("changed " .. change.region .. " " .. change.season .. " " .. math.type(change.season) .. " " .. tostring(ok))
    end
  end)
end`))
	r.Fire("login", Player(1))
	r.FireWorldChange(WorldChange{Dungeon: "crypt", Date: "2026-10-23"})
	r.FireWorldChange(WorldChange{Region: "t2", Season: 3})
	want := "1:rotated crypt 2026-10-23 false|1:changed t2 3 integer false"
	if got := strings.Join(h.messages, "|"); got != want {
		t.Fatalf("messages = %q, logs: %s", got, logs)
	}
}
