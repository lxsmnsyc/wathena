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
