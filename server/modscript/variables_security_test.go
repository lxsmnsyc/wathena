package modscript

import (
	"strings"
	"testing"
)

// Declaring the same variable again must not grow the runtime's table:
// a handler that declares on every event would leak memory for as long as
// the zone runs.
func TestRedeclaringVariableDoesNotGrow(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local variables = require "wa.variables"
return function()
  events.on("login", function(player)
    for i = 1, 500 do variables.server_integer("v") end
  end)
end`))
	for i := 0; i < 3; i++ {
		r.Fire("login", Player(1))
	}
	if n := len(r.variables); n > 1 {
		t.Fatalf("one variable declared 1500 times holds %d entries; logs: %s", n, logs)
	}
}

// A mod declares at most MaxModVariables variables.
func TestModVariableLimit(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local variables = require "wa.variables"
return function()
  events.on("login", function(player)
    for i = 1, 300 do
      local ok, err = pcall(variables.server_integer, "v" .. i)
      if not ok then player:send_message(i .. " " .. err) return end
    end
    player:send_message("all declared")
  end)
end`))
	r.Fire("login", Player(1))
	if len(h.messages) != 1 || !strings.HasPrefix(h.messages[0], "1:257 ") || !strings.Contains(h.messages[0], "the most a mod may") {
		t.Fatalf("messages = %q; logs: %s", h.messages, logs)
	}
}

// A name may start with a digit, as rAthena's do (review decision 60,
// F1). A name with other characters, or of 65 characters, is refused.
func TestVariableNameStartsWithDigit(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local variables = require "wa.variables"
local room = variables.server_temporary_integer("3rdgc_room01")
return function()
  events.on("login", function(player)
    room:set(4)
    local ok = pcall(variables.server_integer, "3 room")
    local long = pcall(variables.server_integer, string.rep("1", 65))
    local full = pcall(variables.server_integer, string.rep("1", 64))
    player:send_message(room:get() .. " " .. tostring(ok) .. " " .. tostring(long) .. " " .. tostring(full))
  end)
end`))
	r.Fire("login", Player(1))
	if len(h.messages) != 1 || h.messages[0] != "1:4 false false true" {
		t.Fatalf("messages = %q; logs: %s", h.messages, logs)
	}
}
