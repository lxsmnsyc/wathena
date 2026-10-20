package modscript

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// Core declares as many variables as its converted NPCs need (review
// decision 58). Mods keep MaxModVariables (TestModVariableLimit).
func TestCoreHasNoVariableLimit(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts(coreMod, nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local variables = require "wa.variables"
return function()
  events.on("login", function(player)
    for i = 1, 300 do variables.server_integer("v" .. i) end
    player:send_message("all declared")
  end)
end`))
	r.Fire("login", Player(1))
	if len(h.messages) != 1 || h.messages[0] != "1:all declared" {
		t.Fatalf("messages = %q; logs: %s", h.messages, logs)
	}
}

// Temporary server variables are one value that every zone shares, and
// they are not the saved server variables of the same name.
func TestServerTemporaryVariables(t *testing.T) {
	h := newHost()
	temp := variables.NewTemporary()
	src := `
local events = require "wa.events"
local variables = require "wa.variables"
local count = variables.server_temporary_integer("count")
local saved = variables.server_integer("count")
local names = variables.server_temporary_table("names")
return function()
  events.on("login", function(player)
    count:add(2)
    names:set({ player.name })
    player:send_message(count:get() .. " " .. saved:get() .. " " .. names:get()[1])
  end)
end`
	server := variables.NewServer(nil, nil)
	r1, logs := newRuntimeOpts(t, h, Options{Temporary: temp, Server: server}, modScripts("example.a", nil, nil, "scripts/main.lua", src))
	r2, _ := newRuntimeOpts(t, h, Options{Temporary: temp, Server: server}, modScripts("example.a", nil, nil, "scripts/main.lua", src))
	r1.Fire("login", Player(1))
	r2.Fire("login", Player(2))
	if strings.Join(h.messages, ",") != "1:2 0 Alice,2:4 0 Bob" {
		t.Fatalf("messages = %q; logs: %s", h.messages, logs)
	}
}

// npc_event needs its label, and NPC variables need an NPC: a host
// without NPCs of the world raises an error.
func TestNPCEventOptions(t *testing.T) {
	for src, want := range map[string]string{
		`events.on("npc_event", function() end)`:                         `want the options`,
		`events.on("npc_event", function() end, { name = "On Enable" })`: "want an event label",
		`events.on("npc_event", function() end, { name = 1 })`:           "want an event label",
		`events.on("npc_event", function() end, { label = "OnX" })`:      "want an event label",
		`variables.npc_integer("n"):get(nil)`:                            "want an NPC",
		`world.fire_npc_event("OnX")`:                                    "NPCs of the world are not reachable here",
		`npc:fire_event("OnX")`:                                          "NPCs of the world are not reachable here",
		`npc:hide()`:                                                     "NPCs of the world are not reachable here",
		`npc:fire_event("OnX", { someone = 1 })`:                         "there is no option someone",
	} {
		h := newHost()
		temp := variables.NewTemporary()
		r, _ := newRuntimeOpts(t, h, Options{Temporary: temp}, modScripts("example.a", nil, nil, "scripts/npc.lua", `
local context = require "wa.context"
local events = require "wa.events"
local variables = require "wa.variables"
local world = require "wa.world"
local result = variables.server_temporary_string("result")
return function()
  local npc = context:get_instance()
  local ok, err = pcall(function() `+src+` end)
  result:set(tostring(err))
end`))
		if _, err := r.StartNPC(1, "example.a:scripts/npc.lua"); err != nil {
			t.Fatal(err)
		}
		got, _ := temp.Get(variables.TempKey{Mod: "example.a", Name: "result"})
		if !strings.Contains(got, want) {
			t.Errorf("%s: %q, want %q", src, got, want)
		}
	}
}

// newRuntimeOpts is newRuntime with more options.
func newRuntimeOpts(t *testing.T, h *fakeHost, opts Options, mods ...*mod.Scripts) (*Runtime, *logBuffer) {
	t.Helper()
	var buf logBuffer
	var ids []string
	for _, m := range mods {
		ids = append(ids, m.Mod)
	}
	opts.Host, opts.Log, opts.Mods = h, slog.New(slog.NewTextHandler(&buf, nil)), append(ids, "example.other")
	r, err := New(opts, mods)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, &buf
}
