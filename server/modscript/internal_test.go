package modscript

import (
	"strings"
	"testing"
)

// TestInternalHasNoPublicCalls checks that core's wa.internal holds none
// of the calls that are public (review decision 62, E7): player:give_exp,
// player.weight and player.skill_points replaced internal.give_exp,
// internal.weight and internal.skill_points.
func TestInternalHasNoPublicCalls(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("core", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local internal = require "wa.internal"
return function()
  events.on("login", function(player)
    player:send_message(tostring(internal.give_exp) .. " " .. tostring(internal.skill_points) .. " " .. tostring(internal.weight))
  end)
end`))
	r.Fire("login", Player(1))
	if got := strings.Join(h.messages, "|"); got != "1:nil nil nil" {
		t.Fatalf("messages = %q, logs: %s", got, logs)
	}
}
