package modscript

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// limitHost is a fake zone that also expands limits, as the zone's
// LimitHost, within rAthena's inventory range.
type limitHost struct {
	*fakeHost
	values map[string]int
}

func (h *limitHost) ExpandLimit(id int64, limit string, amount int) (int, error) {
	if _, ok := h.players[id]; !ok {
		return 0, fmt.Errorf("the player has left")
	}
	r, _ := rules.DefaultLimits().RangeOf(limit)
	h.values[limit] = r.Clamp(r.Clamp(h.values[limit]) + amount)
	return h.values[limit], nil
}

// runRules runs a mod's setup function with a ruleset and returns the
// messages it sent to player 1 and the log.
func runRules(t *testing.T, rl *rules.Rules, host Host, grants []string, src string) (string, string) {
	t.Helper()
	var buf logBuffer
	sc := modScripts("example.a", grants, nil, "scripts/main.lua", src)
	r, err := New(Options{Host: host, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: []string{"example.a"}, Rules: rl},
		[]*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	r.Fire("login", Player(1))
	var fh *fakeHost
	switch h := host.(type) {
	case *fakeHost:
		fh = h
	case *limitHost:
		fh = h.fakeHost
	}
	return strings.Join(fh.messages, "|"), buf.String()
}

// TestRulesRead checks rules.is, rules.get and rules.get_limit (review
// decision 32) against a ruleset with changed flags, rules and limits,
// and the errors for unknown names.
func TestRulesRead(t *testing.T) {
	rl := rules.Base(true)
	rl.CastTime = false
	rl.Skills = map[int]bool{rules.RebalancedSkills["MG_FIREBALL"]: false}
	var err error
	if rl.Values, err = rules.RuleValuesOf(map[string]int{"party_share_level": 30, "basic_skill_check": 0}); err != nil {
		t.Fatal(err)
	}
	if rl.Limits, err = rules.LimitsOf(map[string]rules.RangeSet{"inventory_slots": {Base: 120, Max: 300, HasBase: true, HasMax: true}}); err != nil {
		t.Fatal(err)
	}
	got, logs := runRules(t, rl, newHost(), nil, `
local events = require "wa.events"
local rules = require "wa.rules"
return function()
  events.on("login", function(player)
    local base, max = rules.get_limit("inventory_slots")
    local cart_base, cart_max = rules.get_limit("cart_slots")
    player:send_message(table.concat({
      tostring(rules.is("cast_time")), tostring(rules.is("element_table")), tostring(rules.is("item_rolls")),
      tostring(rules.is("skill_rebalance.MG_FIREBALL")), tostring(rules.is("skill_rebalance.AL_BLESSING")),
      tostring(rules.get("party_share_level")), tostring(rules.get("basic_skill_check")),
      tostring(rules.get("block_list_refuses_trade")), tostring(rules.get("max_cart_weight")),
      base, max, cart_base, cart_max, rules.limits.inventory_slots,
    }, " "))
    for _, f in ipairs({
      function() return rules.is("cast_tim") end,
      function() return rules.is("skill_rebalance.NOT_A_SKILL") end,
      function() return rules.get("party_share") end,
      function() return rules.get_limit("pet_slots") end,
      function() return rules.is(1) end,
    }) do
      local ok, e = pcall(f)
      player:send_message((tostring(ok) .. " " .. tostring(e)):sub(1, 250))
    end
  end)
end`)
	lines := strings.Split(got, "|")
	if len(lines) != 6 {
		t.Fatalf("messages %q, logs %s", got, logs)
	}
	if want := "1:false true false false true 30 false true 8000 120 300 100 100 120"; lines[0] != want {
		t.Errorf("values %q, want %q", lines[0], want)
	}
	for i, want := range []string{
		`there is no flag "cast_tim"; the flags are aspd, boss_protection`,
		`there is no flag "skill_rebalance.NOT_A_SKILL"`,
		`there is no rule "party_share"; the rules are party_share_level, party_even_share_bonus`,
		`there is no limit "pet_slots"; the limits are party_members, friends`,
		`rules.is: the flag must be a string`,
	} {
		if l := lines[i+1]; !strings.HasPrefix(l, "1:false ") || !strings.Contains(l, want) {
			t.Errorf("error %d: %q, want %q", i, l, want)
		}
	}
}

// TestRulesReadDefaults checks that a runtime without a ruleset reads
// rAthena's renewal rules.
func TestRulesReadDefaults(t *testing.T) {
	got, logs := runRules(t, nil, newHost(), nil, `
local events = require "wa.events"
local rules = require "wa.rules"
return function()
  events.on("login", function(player)
    local base, max = rules.get_limit("inventory_slots")
    player:send_message(tostring(rules.is("cast_time")) .. " " .. tostring(rules.get("basic_skill_check")) .. " " .. base .. " " .. max)
  end)
end`)
	if got != "1:true true 100 200" {
		t.Errorf("messages %q, logs %s", got, logs)
	}
}

// TestPlayerExpand checks player:expand: the permission, the limit names,
// the amount, and that it returns the zone's new value.
func TestPlayerExpand(t *testing.T) {
	src := `
local events = require "wa.events"
return function()
  events.on("login", function(player)
    for _, f in ipairs({
      function() return player:expand("inventory_slots", 10) end,
      function() return player:expand("inventory_slots", 500) end,
      function() return player:expand("inventory_slots", -20) end,
      function() return player:expand("inventory_slots", 0) end,
      function() return player:expand("party_members", 1) end,
      function() return player:expand("inventory_slots", 1.5) end,
      function() return player:expand("inventory_slots", 20000) end,
    }) do
      local ok, e = pcall(f)
      player:send_message(tostring(ok) .. " " .. tostring(e))
    end
  end)
end`
	h := &limitHost{fakeHost: newHost(), values: map[string]int{}}
	got, logs := runRules(t, nil, h, []string{"player.expand_limits"}, src)
	lines := strings.Split(got, "|")
	if len(lines) != 7 {
		t.Fatalf("messages %q, logs %s", got, logs)
	}
	for i, want := range []string{
		"1:true 110", "1:true 200", "1:true 180", "1:true 180",
		`1:false player:expand: "party_members" is not a limit a player expands; the limits are inventory_slots, storage_slots, cart_slots`,
		"1:false player:expand: the amount must be an integer",
		"1:false player:expand: the amount must be an integer from -10000 to 10000",
	} {
		if lines[i] != want {
			t.Errorf("call %d: %q, want %q", i, lines[i], want)
		}
	}

	// Without the permission, the call raises an error.
	h = &limitHost{fakeHost: newHost(), values: map[string]int{}}
	got, _ = runRules(t, nil, h, nil, src)
	if first := strings.Split(got, "|")[0]; !strings.Contains(first, "player:expand needs the permission player.expand_limits") {
		t.Errorf("without the permission: %q", first)
	}
	if len(h.values) != 0 {
		t.Errorf("values changed without the permission: %v", h.values)
	}
}
