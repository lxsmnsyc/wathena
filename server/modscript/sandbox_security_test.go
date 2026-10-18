package modscript

import (
	"strings"
	"testing"
)

// Mods see no heap addresses: tostring numbers tables, functions and
// threads, string.format shows %s the same way and refuses %p, also as
// a string method, and the prelude's errors do too (review decision 52).
func TestNoHeapAddresses(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    local t, f = {}, function() end
    local out = {
      tostring(t), tostring(f), tostring(coroutine.create(f)), tostring(t),
      string.format("%s|%5s|%%s|%d", t, "x", 7),
      ("%s"):format(f),
      tostring(setmetatable({}, { __tostring = function() return "custom" end })),
    }
    local ok1, e1 = pcall(string.format, "%p", t)
    local ok2, e2 = pcall(function() return ("%p"):format(t) end)
    local ok3, e3 = pcall(function() events[t] = 1 end)
    out[#out + 1] = tostring(ok1) .. " " .. e1
    out[#out + 1] = tostring(ok2) .. " " .. e2
    out[#out + 1] = tostring(ok3) .. " " .. e3
    player:send_message(table.concat(out, "\n"))
  end)
end`))
	r.Fire("login", Player(1))
	if len(h.messages) != 1 {
		t.Fatalf("messages = %q; logs: %s", h.messages, logs)
	}
	got := h.messages[0]
	if strings.Contains(got, "0x") {
		t.Errorf("an address shows:\n%s", got)
	}
	lines := strings.Split(strings.TrimPrefix(got, "1:"), "\n")
	if len(lines) != 10 {
		t.Fatalf("lines = %q", lines)
	}
	for i, want := range []string{"table: #", "function: #", "thread: #"} {
		if !strings.HasPrefix(lines[i], want) {
			t.Errorf("line %d = %q, want %q", i, lines[i], want)
		}
	}
	if lines[0] != lines[3] || lines[0] == lines[1] {
		t.Errorf("one table has two names, or two objects one: %q", lines[:4])
	}
	if lines[4] != lines[0]+"|    x|%s|7" || lines[5] != lines[1] || lines[6] != "custom" {
		t.Errorf("format: %q", lines[4:7])
	}
	for _, l := range lines[7:9] {
		if !strings.HasPrefix(l, "false ") || !strings.Contains(l, "%p is not allowed") {
			t.Errorf("%%p: %q", l)
		}
	}
	if !strings.HasPrefix(lines[9], "false ") || !strings.Contains(lines[9], "table: #") {
		t.Errorf("error message: %q", lines[9])
	}
}
