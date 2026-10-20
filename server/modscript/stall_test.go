package modscript

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
)

// stallHost is a host whose item lookups stall, as on a machine that
// swaps.
type stallHost struct {
	*fakeHost
	stall time.Duration
}

func (h *stallHost) ItemID(name string) (uint32, bool) {
	time.Sleep(h.stall)
	return h.fakeHost.ItemID(name)
}

// TestStalledSetupKeepsMod checks decision 0054 in a zone's Lua state: a
// mod's setup that a stalled machine stretches past the old 1-second
// wall-clock limit still runs, and the mod's handlers work. A setup that
// never ends is still stopped, by the CPU budget, and only its mod is
// off.
func TestStalledSetupKeepsMod(t *testing.T) {
	h := &stallHost{fakeHost: newHost(), stall: 1500 * time.Millisecond}
	var logs logBuffer
	r, err := New(Options{Host: h, Log: slog.New(slog.NewTextHandler(&logs, nil))},
		[]*mod.Scripts{
			modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local items = require "wa.items"
return function()
  local x = 0
  for i = 1, 5000 do x = x + 1 end
  local id = items.id("Red_Potion")
  for i = 1, 5000 do x = x + 1 end
  events.on("login", function(player)
    player:send_message("potion " .. id .. " after " .. x)
  end)
end`),
			modScripts("example.b", nil, nil, "scripts/main.lua", `
return function()
  while true do end
end`)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	r.Fire("login", Player(1))
	if got := strings.Join(h.messages, "|"); got != "1:potion 501 after 10000" {
		t.Fatalf("messages = %q, logs: %s", got, logs.String())
	}
	if !strings.Contains(logs.String(), "example.b") || !strings.Contains(logs.String(), "CPU limit exceeded") {
		t.Errorf("the looping setup was not stopped by the CPU budget: %s", logs.String())
	}
}
