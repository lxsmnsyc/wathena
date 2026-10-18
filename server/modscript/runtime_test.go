package modscript

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// fakeHost is a zone with players that only hold a name, and a log of
// the messages scripts send.
type fakeHost struct {
	now      int64
	players  map[int64]string
	monsters map[int64]string
	messages []string
	items    map[string]uint32
	given    map[int64]int
	// calls logs the item API calls, and draws the random ranges.
	calls    []string
	draws    []int64
	skills   map[int64]int
	slots    map[string]int64
	equipped map[int64]fakeEquipped
}

type fakeEquipped struct {
	item   uint32
	refine int64
	wearer int64
}

func newHost() *fakeHost {
	return &fakeHost{
		players:  map[int64]string{1: "Alice", 2: "Bob"},
		monsters: map[int64]string{7: "Poring"},
		items:    map[string]uint32{"Red_Potion": 501, "example.a:Cookie": 2_000_000},
		given:    map[int64]int{},
		skills:   map[int64]int{},
		slots:    map[string]int64{},
		equipped: map[int64]fakeEquipped{},
	}
}

func (h *fakeHost) Map() string { return "prontera" }

func (h *fakeHost) Now() int64 { return h.now }

// advance moves the fake zone clock and runs the timers due.
func advance(h *fakeHost, r *Runtime, now int64) {
	h.now = now
	r.Advance(now)
}

func (h *fakeHost) PlayerField(id int64, field string) (script.Value, error) {
	name, ok := h.players[id]
	if !ok {
		return script.Nil, errors.New("the player has left")
	}
	switch field {
	case "name":
		return script.String(name), nil
	case "base_level":
		return script.Int(1), nil
	}
	return script.Nil, fmt.Errorf("player has no field %q", field)
}

func (h *fakeHost) MonsterField(id int64, field string) (script.Value, error) {
	name, ok := h.monsters[id]
	if !ok {
		return script.Nil, errors.New("the monster is gone")
	}
	switch field {
	case "name":
		return script.String(name), nil
	case "is_alive":
		return script.Bool(false), nil
	}
	return script.Nil, fmt.Errorf("monster has no field %q", field)
}

func (h *fakeHost) SendMessage(id int64, text string) error {
	if _, ok := h.players[id]; !ok {
		return errors.New("the player has left")
	}
	h.messages = append(h.messages, fmt.Sprintf("%d:%s", id, text))
	return nil
}

func (h *fakeHost) GiveItem(id int64, item uint32, amount int, rent int64) (bool, error) {
	h.given[id] += amount
	h.calls = append(h.calls, fmt.Sprintf("give %d %d x%d rent %d", id, item, amount, rent))
	return true, nil
}

func (h *fakeHost) GiveGroup(id int64, group int64) error {
	h.calls = append(h.calls, fmt.Sprintf("group %d %d", id, group))
	return nil
}

func (h *fakeHost) Heal(id int64, percent bool, hp, sp [2]int) error {
	h.calls = append(h.calls, fmt.Sprintf("heal %d %v %v %v", id, percent, hp, sp))
	return nil
}

func (h *fakeHost) Warp(id int64, where, mapName string, x, y int) (bool, error) {
	h.calls = append(h.calls, fmt.Sprintf("warp %d %s %s %d %d", id, where, mapName, x, y))
	return true, nil
}

func (h *fakeHost) ShowEffect(id int64, effect int64, headgear bool) error {
	h.calls = append(h.calls, fmt.Sprintf("effect %d %d %v", id, effect, headgear))
	return nil
}

func (h *fakeHost) SkillLevel(id int64, skill int64) (int, error) { return h.skills[skill], nil }

func (h *fakeHost) Equipped(id int64, slot string) (int64, bool, error) {
	e, ok := h.slots[slot]
	return e, ok, nil
}

func (h *fakeHost) EquippedField(id int64, field string) (script.Value, error) {
	e, ok := h.equipped[id]
	if !ok {
		return script.Nil, errors.New("the item is no longer equipped")
	}
	switch field {
	case "item":
		return script.Int(int64(e.item)), nil
	case "refine":
		return script.Int(e.refine), nil
	case "enchant_grade":
		return script.Int(0), nil
	case "wearer":
		return script.Int(e.wearer), nil
	}
	return script.Nil, fmt.Errorf("equipped item has no field %q", field)
}

func (h *fakeHost) RandomN(n int64) int64 {
	h.draws = append(h.draws, n)
	return n - 1
}

func (h *fakeHost) ItemGroupID(name string) (int64, bool) {
	if name == "BLUEBOX" {
		return 1, true
	}
	return 0, false
}

func (h *fakeHost) MonsterID(name string) (int64, bool) {
	if name == "PORING" {
		return 1002, true
	}
	return 0, false
}

func (h *fakeHost) CountItem(id int64, item uint32) (int, error) { return h.given[id], nil }

func (h *fakeHost) ItemID(name string) (uint32, bool) {
	id, ok := h.items[name]
	return id, ok
}

// modScripts makes a mod's scripts from files. The first file is its
// entry script.
func modScripts(id string, grants []string, depends []string, files ...string) *mod.Scripts {
	g := mod.Grants{"game.read": true, "event.listen": true, "npc.create": true, "storage.mod": true}
	for _, p := range grants {
		g[p] = true
	}
	sc := &mod.Scripts{Mod: id, Files: map[string]string{}, Grants: g, Visible: append([]string{id}, depends...)}
	for i := 0; i+1 < len(files); i += 2 {
		sc.Files[files[i]] = files[i+1]
		if i == 0 {
			sc.Entries = []string{files[i]}
		}
	}
	return sc
}

type logBuffer struct{ bytes.Buffer }

// String returns the log with slog's quoting of quotes undone.
func (b *logBuffer) String() string { return strings.ReplaceAll(b.Buffer.String(), `\"`, `"`) }

func newRuntime(t *testing.T, h *fakeHost, mods ...*mod.Scripts) (*Runtime, *logBuffer) {
	t.Helper()
	var buf logBuffer
	var ids []string
	for _, m := range mods {
		ids = append(ids, m.Mod)
	}
	r, err := New(Options{Host: h, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: append(ids, "example.other")}, mods)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, &buf
}

func TestLoginHandler(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local context = require "wa.context"
return function()
  local mod = context:get_mod()
  events.on("login", function(player)
    player:send_message("hello " .. player.name .. " from " .. mod .. " on " .. context:get_instance().map)
  end)
end`))
	r.Fire("login", Player(1))
	if got := strings.Join(h.messages, "|"); got != "1:hello Alice from example.a on prontera" {
		t.Fatalf("messages = %q, logs: %s", got, logs)
	}
}

func TestMonsterHandle(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  events.on("death", function(death)
    local player, monster = death.killer, death.victim
    player:send_message(monster.name .. " alive " .. tostring(monster.is_alive))
    local ok = pcall(function() monster.name = "x" end)
    player:send_message("writable " .. tostring(ok))
    timers.after(10, function()
      local ok, err = pcall(function() return monster.name end)
      player:send_message("later " .. tostring(ok) .. " " .. tostring(err):match("the monster is gone") )
    end)
  end)
end`))
	r.FireDeath(Death{Victim: 7, VictimKind: "monster", Monster: 1002, Killer: 1, KillerKind: "player"})
	delete(h.monsters, 7)
	advance(h, r, 10)
	want := "1:Poring alive false|1:writable false|1:later false the monster is gone"
	if got := strings.Join(h.messages, "|"); got != want {
		t.Fatalf("messages = %q, logs: %s", got, logs)
	}
}

func TestEntryMustReturnFunction(t *testing.T) {
	for _, src := range []string{`return 1`, `return {}`, ``} {
		_, logs := newRuntime(t, newHost(), modScripts("example.a", nil, nil, "scripts/main.lua", src))
		if !strings.Contains(logs.String(), "example.a/scripts/main.lua must return a setup function") {
			t.Errorf("%q: logs = %s", src, logs)
		}
	}
}

func TestHelperModulesReturnAnything(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil,
		"scripts/main.lua", `
local n = require "example.a.lib.number"
local f = require "example.a.lib.func"
local t = require "example.a.lib.table"
local none = require "example.a.lib.none"
local events = require "wa.events"
return function()
  events.on("login", function(p) p:send_message(tostring(n) .. f() .. t.x .. tostring(none)) end)
end`,
		"scripts/lib/number.lua", `return 7`,
		"scripts/lib/func.lua", `return function() return "f" end`,
		"scripts/lib/table.lua", `return { x = "t" }`,
		"scripts/lib/none.lua", `local x = 1`,
	))
	r.Fire("login", Player(1))
	if got := strings.Join(h.messages, ""); got != "1:7fttrue" {
		t.Fatalf("messages = %q, logs: %s", got, logs)
	}
}

func TestRequireRules(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"own file", `local m = require "example.b.lib.x"`, ""},
		{"dependency file", `local m = require "example.a.lib.shared"`, ""},
		{"not in depends", `local m = require "example.other.lib"`, "example.other is not in its depends"},
		{"missing file", `local m = require "example.b.lib.nothing"`, "example.b has no scripts/lib/nothing.lua"},
		{"unknown mod", `local m = require "nobody.lib"`, "no mod has the module nobody.lib"},
		{"unknown api", `local m = require "wa.nothing"`, "there is no API module wa.nothing"},
		{"self require", `local m = require "example.b.lib.loop"`, "requires itself while it loads"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := modScripts("example.a", nil, nil, "scripts/main.lua", `return function() end`, "scripts/lib/shared.lua", `return 1`)
			b := modScripts("example.b", nil, []string{"example.a"},
				"scripts/main.lua", c.src+"\nreturn function() end",
				"scripts/lib/x.lua", `return 2`,
				"scripts/lib/loop.lua", `local x = require "example.b.lib.loop"; return x`)
			r, logs := newRuntime(t, newHost(), a, b)
			if c.want == "" {
				if r.Context("example.b") == nil {
					t.Fatalf("example.b did not load: %s", logs)
				}
				return
			}
			if r.Context("example.b") != nil || !strings.Contains(logs.String(), c.want) {
				t.Fatalf("want an error with %q, logs: %s", c.want, logs)
			}
			// The error names the script line.
			if !strings.Contains(logs.String(), ".lua:1: require: ") {
				t.Errorf("the error does not name the line: %s", logs)
			}
		})
	}
}

func TestEnvironmentIsolation(t *testing.T) {
	h := newHost()
	a := modScripts("example.a", nil, nil, "scripts/main.lua", `
secret = "a's global"
local events = require "wa.events"
return function()
  events.on("login", function(p) p:send_message("a sees secret=" .. tostring(secret) .. " string.upper ok=" .. tostring(string.upper("x") == "X")) end)
end`)
	b := modScripts("example.b", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local results = {}
local function try(name, fn)
  local ok, err = pcall(fn)
  results[#results + 1] = name .. "=" .. (ok and "ok" or "refused")
end
try("read a's global", function() assert(secret == nil) end)
try("change string lib", function() string.upper = function() return "evil" end end)
try("write api module", function() events.on = function() end end)
try("rawset api module", function() rawset(events, "on", function() end) end)
try("new field on api module", function() events.extra = 1 end)
try("metatable of api module", function() assert(getmetatable(events) == false) end)
try("setmetatable on api module", function() setmetatable(events, {}) end)
try("reach real globals", function() assert(_G.secret == nil and _ENV == _G) end)
try("no load", function() assert(load == nil and loadstring == nil and dofile == nil) end)
try("os and io", function() assert(os == nil and io == nil and debug == nil and package == nil) end)
return function()
  events.on("login", function(p) p:send_message(table.concat(results, ",")) end)
end`)
	r, logs := newRuntime(t, h, a, b)
	r.Fire("login", Player(1))
	want := []string{
		"1:a sees secret=a's global string.upper ok=true",
		"1:read a's global=ok,change string lib=ok,write api module=refused,rawset api module=refused," +
			"new field on api module=refused,metatable of api module=ok,setmetatable on api module=refused," +
			"reach real globals=ok,no load=ok,os and io=ok",
	}
	if got := strings.Join(h.messages, "\n"); got != strings.Join(want, "\n") {
		t.Fatalf("messages:\n%s\nwant:\n%s\nlogs: %s", got, strings.Join(want, "\n"), logs)
	}
}

func TestPlayerFieldsAreReadOnly(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(p)
    local ok, err = pcall(function() p.name = "Mallory" end)
    p:send_message(tostring(ok) .. " " .. err .. " " .. p.name)
    local ok2, err2 = pcall(function() return p.nothing end)
    p:send_message(tostring(ok2) .. " " .. err2)
  end)
end`))
	r.Fire("login", Player(1))
	if len(h.messages) != 2 ||
		!strings.Contains(h.messages[0], "false example.a/scripts/main.lua:5: player.name is read-only; change it with a method Alice") ||
		!strings.Contains(h.messages[1], `false example.a/scripts/main.lua:7: player has no field "nothing"`) {
		t.Fatalf("messages = %q, logs: %s", h.messages, logs)
	}
}

func TestEventNamesChecked(t *testing.T) {
	cases := map[string]string{
		`events.on("logn", function() end)`:  `there is no event "logn"; a mod context can listen to base_level_up, buying_store_sale, chat, death, dungeon_rotated, guild_change, guild_join, guild_leave, job_change, job_level_up, login, logout, mail, map_enter`,
		`events.on("click", function() end)`: `"click" is an event of npc contexts, and this is a mod context`,
		`events.on("login", 3)`:              "the handler must be a function",
	}
	for src, want := range cases {
		r, logs := newRuntime(t, newHost(), modScripts("example.a", nil, nil, "scripts/main.lua",
			"local events = require 'wa.events'\nreturn function()\n"+src+"\nend"))
		if r.Context("example.a") != nil || !strings.Contains(logs.String(), want) || !strings.Contains(logs.String(), "main.lua:3") {
			t.Errorf("%s: logs = %s", src, logs)
		}
	}
}

func TestContextCallsFailOutsideContext(t *testing.T) {
	for _, src := range []string{
		`require("wa.events").on("login", function() end)`,
		`require("wa.timers").after(10, function() end)`,
		`require("wa.context"):get_kind()`,
	} {
		_, logs := newRuntime(t, newHost(), modScripts("example.a", nil, nil, "scripts/main.lua", src+"\nreturn function() end"))
		if !strings.Contains(logs.String(), "needs a context") {
			t.Errorf("%s: logs = %s", src, logs)
		}
	}
}

func TestContextKind(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local context = require "wa.context"
local events = require "wa.events"
return function()
  local inst = context:get_instance()
  events.on("login", function(p) p:send_message(context:get_kind() .. " " .. inst.id .. " " .. context:get_mod()) end)
end`))
	r.Fire("login", Player(2))
	if strings.Join(h.messages, "") != "2:mod example.a example.a" {
		t.Fatalf("messages = %q, logs: %s", h.messages, logs)
	}
}

func TestPermissionsAtCallTime(t *testing.T) {
	src := `
local events = require "wa.events"
local items = require "wa.items"
local POTION = items.id "Red_Potion"
return function()
  events.on("login", function(p) p:send_message(tostring(p:give_item(POTION, 2))) end)
end`
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", src))
	r.Fire("login", Player(1))
	if len(h.messages) != 0 || !strings.Contains(logs.String(), "player:give_item needs the permission player.give_items, which example.a does not have") {
		t.Fatalf("messages = %q, logs: %s", h.messages, logs)
	}
	h = newHost()
	r, logs = newRuntime(t, h, modScripts("example.a", []string{"player.give_items"}, nil, "scripts/main.lua", src))
	r.Fire("login", Player(1))
	if strings.Join(h.messages, "") != "1:true" || h.given[1] != 2 {
		t.Fatalf("messages = %q, given %v, logs: %s", h.messages, h.given, logs)
	}
}

func TestTimersFollowZoneClock(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local timers = require "wa.timers"
local events = require "wa.events"
return function()
  local n = 0
  timers.every(1000, function()
    n = n + 1
    events.on("login", function(p) end)
  end)
  local once = timers.after(2500, function() n = n + 100 end)
  local canceled = timers.after(1500, function() n = n + 10000 end)
  canceled:cancel()
  events.on("login", function(p) p:send_message(tostring(n)) end)
end`))
	say := func() string {
		h.messages = nil
		r.Fire("login", Player(1))
		return strings.Join(h.messages, "")
	}
	advance(h, r, 999)
	if got := say(); got != "1:0" {
		t.Fatalf("at 999 ms: %s, logs: %s", got, logs)
	}
	advance(h, r, 1000)
	if got := say(); got != "1:1" {
		t.Fatalf("at 1000 ms: %s", got)
	}
	// The zone slept for 10 seconds: its clock stopped, so the timers
	// wait as long after waking.
	h.now += 10_000
	r.Shift(10_000)
	advance(h, r, 11_500)
	if got := say(); got != "1:1" {
		t.Fatalf("after sleeping: %s", got)
	}
	advance(h, r, 12_500)
	if got := say(); got != "1:102" {
		t.Fatalf("at 12500 ms: %s", got)
	}
}

func TestLimitInHandlerKeepsRuntime(t *testing.T) {
	h := newHost()
	sc := modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(p) while true do end end)
  events.on("login", function(p) p:send_message("still here") end)
end`)
	// A budget per tick above the per-call limit, so the second handler
	// runs in the same tick (budget.go).
	sc.Budget.CPUPerTick = 2 * script.DefaultCPU
	r, logs := newRuntime(t, h, sc)
	r.Fire("login", Player(1))
	if strings.Join(h.messages, "") != "1:still here" || !strings.Contains(logs.String(), "CPU limit") {
		t.Fatalf("messages = %q, logs: %s", h.messages, logs)
	}
	// It keeps working, and fails too often after MaxFailures.
	for range MaxFailures {
		r.BeginTick()
		r.Fire("login", Player(1))
	}
	if r.Context("example.a") != nil || !strings.Contains(logs.String(), "failed too often") {
		t.Fatalf("the mod was not turned off: %s", logs)
	}
	h.messages = nil
	r.Fire("login", Player(1))
	if len(h.messages) != 0 {
		t.Fatalf("handlers of an ended context ran: %q", h.messages)
	}
}

func TestReloadReplacesHandlers(t *testing.T) {
	h := newHost()
	v1 := modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  events.on("login", function(p) p:send_message("v1") end)
  timers.every(100, function() end)
end`)
	r, logs := newRuntime(t, h, v1)
	old := r.Context("example.a")
	v2 := modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  events.on("login", function(p) p:send_message("v2") end)
end`)
	if err := r.Reload(v2); err != nil {
		t.Fatal(err)
	}
	if !old.Ended() || len(r.timers) != 0 {
		t.Fatalf("the old context did not end: ended %v, timers %d", old.Ended(), len(r.timers))
	}
	r.Fire("login", Player(1))
	if strings.Join(h.messages, ",") != "1:v2" {
		t.Fatalf("messages = %q, logs: %s", h.messages, logs)
	}
	// A reload that fails leaves the mod off.
	bad := modScripts("example.a", nil, nil, "scripts/main.lua", `return 5`)
	if err := r.Reload(bad); err == nil {
		t.Fatal("a bad reload succeeded")
	}
	h.messages = nil
	r.Fire("login", Player(1))
	if len(h.messages) != 0 {
		t.Fatalf("messages after a failed reload = %q", h.messages)
	}
}

func TestEventHandleRemove(t *testing.T) {
	h := newHost()
	r, _ := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function()
  local handle
  handle = events.on("login", function(p) p:send_message("once"); handle:remove() end)
end`))
	r.Fire("login", Player(1))
	r.Fire("login", Player(1))
	if strings.Join(h.messages, ",") != "1:once" {
		t.Fatalf("messages = %q", h.messages)
	}
}

func TestPlayerHandleAfterLeaving(t *testing.T) {
	h := newHost()
	r, logs := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  events.on("login", function(p)
    timers.after(10, function() p:send_message(p.name) end)
  end)
end`))
	r.Fire("login", Player(2))
	delete(h.players, 2)
	advance(h, r, 10)
	if len(h.messages) != 0 || !strings.Contains(logs.String(), "the player has left") {
		t.Fatalf("messages = %q, logs: %s", h.messages, logs)
	}
}
