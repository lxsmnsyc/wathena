package modscript

import (
	"strings"
	"testing"
	"time"
)

// A next() function that a hook keeps must not run after its chain ends.
// Here mod M (no game.read) keeps next() from a drop_rate chain, and calls
// it from inside an experience chain whose slot 2 is mod V's hook (game.read).
// hook_enter(2) then switches to V's context while M's own function runs.
func TestStoredHookNextKeepsChainContext(t *testing.T) {
	m := modScripts("example.m", []string{"rules.override"}, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
local items = require "wa.items"
local stored
local leaked = false
return function()
  hooks.on("drop_rate", function(drop, next)
    stored = next
    return next(drop)
  end, { priority = 10 })
  hooks.on("drop_rate", function(drop, next)
    if pcall(items.id, "Red_Potion") then leaked = true end
    return 100
  end, { priority = 1 })
  hooks.on("experience", function(exp, next)
    if stored then pcall(stored, exp) end
    local b, j = next(exp)
    if leaked then b = 777 end
    return b, j
  end, { priority = 10 })
end`)
	delete(m.Grants, "game.read")
	v := modScripts("example.v", []string{"rules.override", "game.read"}, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
return function()
  hooks.on("experience", function(exp, next) return next(exp) end, { priority = 1 })
end`)
	r, _, logs := newWorldRuntime(t, time.UTC, m, v)
	r.RunDropRate(DropRate{Player: 1, Monster: 7, Item: 501, Rate: 100})
	b, _ := r.RunExperience(Experience{Player: 1, Monster: 7, Base: 5, Job: 3})
	if b == 777 {
		t.Fatalf("mod M ran its code with mod V's permissions (game.read) through a stored next(); logs: %s", logs)
	}
}

// A kept next() raises an error that says why, and the chain the hook
// ran in still gave its result.
func TestKeptHookNextRaises(t *testing.T) {
	m := modScripts("example.m", []string{"rules.override"}, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
local events = require "wa.events"
local stored
return function()
  hooks.on("drop_rate", function(drop, next)
    stored = next
    return next(drop) * 2
  end)
  events.on("login", function(player)
    local ok, err = pcall(stored, { rate = 1 })
    player:send_message(tostring(ok) .. " " .. tostring(err))
  end)
end`)
	r, h, logs := newWorldRuntime(t, time.UTC, m)
	if got := r.RunDropRate(DropRate{Player: 1, Monster: 7, Item: 501, Rate: 100}); got != 200 {
		t.Fatalf("drop rate = %d, want 200; logs: %s", got, logs)
	}
	r.Fire("login", Player(1))
	if len(h.messages) != 1 || !strings.Contains(h.messages[0], "false") || !strings.Contains(h.messages[0], "has returned") {
		t.Fatalf("messages = %q, want a failed call that says the hook has returned; logs: %s", h.messages, logs)
	}
}
