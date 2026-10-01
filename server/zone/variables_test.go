package zone

import (
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/variables"
)

const counter = `
local events = require "wa.events"
local variables = require "wa.variables"

local visits = variables.character_integer "visits"
local seen = variables.account_boolean "seen"
local last = variables.character_string "last"
local bag = variables.character_table "bag"
local busy = variables.character_temporary_integer "busy"
local total = variables.server_integer "total"

return function()
  events.on("click", function(player, conversation)
    local n = visits:add(player, 1)
    seen:set(player, true)
    last:set(player, "hello " .. n)
    local b = bag:get(player) or { items = {} }
    b.items[#b.items + 1] = n
    bag:set(player, b)
    busy:set(player, busy:get(player) + 10)
    total:add(5)
    conversation:say(n .. " " .. tostring(seen:get(player)) .. " " .. last:get(player) .. " " ..
      #bag:get(player).items .. " " .. busy:get(player) .. " " .. total:get())
  end)
end`

func TestVariables(t *testing.T) {
	z, p, n, logs := npcZone(t, counter)
	server := z.Mods.Server
	for i := 1; i <= 2; i++ {
		ev := click(z, p, n)
		want := []string{"1 true hello 1 1 10 5", "2 true hello 2 2 20 10"}[i-1]
		if got := lines(ev); got != want {
			t.Fatalf("click %d: %q, want %q, logs %s", i, got, want, logs)
		}
		answer(z, p, reply(lastDialog(ev).GetDialog(), func(b *wathenav1.NpcReply_builder) { b.Close = new(true) }))
	}
	c := z.snapshot(p)
	want := map[character.Variable]string{
		{Mod: "example.test", Name: "visits"}:              "2",
		{Mod: "example.test", Name: "seen", Account: true}: "true",
		{Mod: "example.test", Name: "last"}:                `"hello 2"`,
		{Mod: "example.test", Name: "bag"}:                 `{"items":[1,2]}`,
	}
	if len(c.Variables) != len(want) {
		t.Fatalf("saved variables %v", c.Variables)
	}
	for k, v := range want {
		if c.Variables[k] != v {
			t.Errorf("%v = %q, want %q", k, c.Variables[k], v)
		}
	}
	// Temporary variables are not saved.
	if len(p.temp) != 1 {
		t.Errorf("temporary variables %v", p.temp)
	}
	if v, _ := server.Get(variables.Key{Mod: "example.test", Name: "total"}); v != "10" {
		t.Errorf("server variable %q", v)
	}
	// A character that enters with saved variables reads them.
	c.Name = "again"
	q := addCharacter(z, c, &saves{})
	q.entity.x, q.entity.y = 19, 20
	z.stepAt(z.now + 50)
	events(q)
	if got := lines(click(z, q, n)); !strings.HasPrefix(got, "3 true hello 3 3 10 15") {
		t.Fatalf("a loaded character got %q", got)
	}
}

func TestVariableTypes(t *testing.T) {
	z, p, n, logs := npcZone(t, `
local events = require "wa.events"
local variables = require "wa.variables"
local count = variables.character_integer "count"
local list = variables.character_table "list"
local text = variables.character_string "text"
return function()
  events.on("click", function(player, conversation)
    local function try(f) local ok, err = pcall(f) return ok and "ok" or tostring(err):match(": ([^:]*)$") end
    conversation:say(
      try(function() count:set(player, "x") end),
      try(function() list:set(player, { 1, 2, a = 3 }) end),
      try(function() list:set(player, { f = function() end }) end),
      try(function() text:set(player, string.rep("a", 5000)) end),
      try(function() variables.instance_integer "room" end),
      try(function() variables.character_string "count" end),
      try(function() count:set(player, 7); count:set(player, nil) end),
      tostring(count:get(player)), tostring(list:get(player)), text:get(player) == "" and "empty" or "set")
  end)
end`)
	got := strings.Split(lines(click(z, p, n)), "|")
	want := []string{
		"want an integer, got string",
		"a table's keys are all strings, or 1 to n as a list",
		"a table can hold numbers, booleans, strings and tables, not function",
		"a string is at most 4096 bytes of UTF-8",
		"instances do not exist yet",
		"example.test already declared count as a integer variable",
		"ok", "0", "nil", "empty",
	}
	if len(got) != len(want) {
		t.Fatalf("got %q, logs %s", got, logs)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %q, want %q", i, got[i], want[i])
		}
	}
}
