package modscript

import (
	"strings"
	"testing"
)

// Mod A keeps memory in a global until the shared Lua state is nearly full.
// Mod B's ordinary handler must still work, and must not be switched off
// for A's memory use.
func TestHoardingModCannotDisableOtherMods(t *testing.T) {
	h := newHost()
	a := modScripts("example.hoard", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local keep = {}
return function()
  events.on("login", function(player)
    local t = {}
    for i = 1, 60000 do t[i] = i end
    keep[#keep + 1] = t
  end)
end`)
	b := modScripts("example.victim", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("death", function(death)
    local t = {}
    for i = 1, 60000 do t[i] = i end
    death.killer:send_message("victim ok")
  end)
end`)
	r, logs := newRuntime(t, h, a, b)
	// Fill the state with A's data: about 1 MB per login, 16 MB in all.
	for i := 0; i < 40; i++ {
		r.Fire("login", Player(1))
	}
	for i := 0; i < 12; i++ {
		r.FireDeath(Death{Victim: 7, VictimKind: "monster", Monster: 1002, Killer: 1, KillerKind: "player"})
	}
	if strings.Contains(logs.String(), `mod=example.victim`) && strings.Contains(logs.String(), "off in this zone") {
		t.Fatalf("mod example.victim was switched off because mod example.hoard used the memory; logs: %s", logs)
	}
}
