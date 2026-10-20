package modscript

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// itemMod is a mod with item scripts and no entry scripts, as core.
func itemMod(files ...string) *mod.Scripts {
	sc := modScripts("core", []string{"player.give_items", "player.warp", "player.status"}, nil, files...)
	sc.Entries = nil
	return sc
}

var testConstants = &constants.Data{
	Constants: map[string]int64{"RC_DEMIHUMAN": 7, "RC_ALL": 9999, "EF_HEAL": 312, "SC_STUN": 2, "JOB_KNIGHT": 7, "JOB_LORD_KNIGHT": 4008, "JOB_SWORDMAN": 1},
	Skills:    map[string]int64{"SM_BASH": 5},
}

// recorder is a BonusSink that keeps the bonuses that are active.
type recorder struct {
	r      *Runtime
	active []*Bonus
	log    []string
}

func (s *recorder) BonusAdded(b *Bonus) {
	s.active = append(s.active, b)
	s.log = append(s.log, "+"+b.Name)
}

func (s *recorder) BonusRemoved(b *Bonus) {
	s.active = slices.DeleteFunc(s.active, func(x *Bonus) bool { return x == b })
	s.log = append(s.log, "-"+b.Name)
}

// values runs every active bonus and writes name=args.
func (s *recorder) values(t *testing.T) string {
	t.Helper()
	var out []string
	for _, b := range s.active {
		args, chance, active, err := s.r.Values(b)
		if err != nil {
			t.Fatal(err)
		}
		if !active {
			continue
		}
		e := fmt.Sprintf("%s=%v", b.Name, args)
		if b.Options.Has("chance") {
			e += fmt.Sprintf("@%d", chance)
		}
		out = append(out, e)
	}
	return strings.Join(out, " ")
}

func newItemRuntime(t *testing.T, h *fakeHost, files ...string) *Runtime {
	t.Helper()
	r, err := New(Options{Host: h, Mods: []string{"core"}, Constants: testConstants}, []*mod.Scripts{itemMod(files...)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}

func TestUseItem(t *testing.T) {
	h := newHost()
	r := newItemRuntime(t, h, "scripts/items/box.lua", `
local events = require "wa.events"
local items = require "wa.items"
local effects = require "wa.effects"
local random = require "wa.random"
local context = require "wa.context"
local BLUE_BOX = items.group "BLUEBOX"
local POTION = items.id "Red_Potion"
return function()
  assert(context:get_kind() == "item" and context:get_instance().item == 603)
  events.on("use", function(player)
    player:item_heal { hp = { 45, 65 } }
    player:heal_percent { hp = 50, sp = 10 }
    player:give_item_from_group(BLUE_BOX)
    player:give_item(POTION, 2)
    player:give_item(POTION, 1, { rent_seconds = 3600 })
    player:give_item(({ POTION, 909 })[random.integer(1, 2)], 1)
    player:show_effect(effects.heal)
    player:warp_to_random_cell()
  end)
end`)
	if err := r.UseItem(1, 603, "core:scripts/items/box.lua"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"heal 1 false [45 65] [0 0]", "heal 1 true [50 50] [10 10]", "group 1 1",
		"give 1 501 x2 rent 0", "give 1 501 x1 rent 3600", "give 1 909 x1 rent 0",
		"effect 1 312 false", "warp 1 random  0 0",
	}
	if !slices.Equal(h.calls, want) {
		t.Errorf("calls\n%q\nwant\n%q", h.calls, want)
	}
	// The use context ended: a second use runs a fresh setup.
	if err := r.UseItem(1, 603, "core:scripts/items/box.lua"); err != nil || len(h.calls) != 16 {
		t.Errorf("second use: %v, %d calls", err, len(h.calls))
	}
}

func TestUseItemErrors(t *testing.T) {
	h := newHost()
	r := newItemRuntime(t, h, "scripts/items/bad.lua", `
local events = require "wa.events"
local bonuses = require "wa.bonuses"
return function()
  events.on("use", function(player)
    bonuses.add("str", 1)
  end)
end`, "scripts/items/status.lua", `
local events = require "wa.events"
local statuses = require "wa.statuses"
return function()
  events.on("use", function(player) player:start_status(statuses.stun, { milliseconds = 1000 }) end)
end`)
	err := r.UseItem(1, 1, "core:scripts/items/bad.lua")
	if err == nil || !strings.Contains(err.Error(), "bonuses belong to an equipped item") {
		t.Errorf("bonus in a use: %v", err)
	}
	// A host without status effects refuses them.
	err = r.UseItem(1, 1, "core:scripts/items/status.lua")
	if err == nil || !strings.Contains(err.Error(), "this zone has no status effects") {
		t.Errorf("start_status: %v", err)
	}
	if err := r.UseItem(1, 1, "core:scripts/items/missing.lua"); err == nil {
		t.Error("a missing script ran")
	}
}

func TestEquipBonuses(t *testing.T) {
	h := newHost()
	h.equipped[10] = fakeEquipped{item: 1101, refine: 7, wearer: 1}
	h.skills[5] = 3
	r := newItemRuntime(t, h, "scripts/items/sword.lua", `
local bonuses = require "wa.bonuses"
local context = require "wa.context"
local races = require "wa.races"
local skills = require "wa.skills"
local BASH = skills.id "SM_BASH"
return function()
  local equipped = context:get_instance()
  local player = equipped.wearer
  bonuses.add("str", 2)
  bonuses.add("damage", 10, { kind = "physical", against = { race = races.demi_human } })
  bonuses.add("atk", function() return equipped.refine * 5 end)
  bonuses.add("damage", function() return player:get_skill_level(BASH) end, { skill = BASH })
  bonuses.add("unbreakable_weapon", { when = function() return equipped.refine >= 9 end })
  bonuses.add("skill", BASH, function() return equipped.refine >= 7 and 3 or 0 end)
  bonuses.add("cause_status", 2, { chance = function() return equipped.refine * 10 end })
end`)
	sink := &recorder{r: r}
	c, err := r.Equip("core:scripts/items/sword.lua", 10, 1, sink)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sink.values(t), "str=[2] damage=[10] atk=[35] damage=[3] skill=[5 3] cause_status=[2]@70"; got != want {
		t.Errorf("bonuses %q, want %q", got, want)
	}
	// A computed bonus reads the copy again: the refine changed.
	h.equipped[10] = fakeEquipped{item: 1101, refine: 10, wearer: 1}
	if got, want := sink.values(t), "str=[2] damage=[10] atk=[50] damage=[3] unbreakable_weapon=[] skill=[5 3] cause_status=[2]@100"; got != want {
		t.Errorf("bonuses %q, want %q", got, want)
	}
	if err := r.Unequip(c); err != nil {
		t.Fatal(err)
	}
	if len(sink.active) != 0 {
		t.Errorf("%d bonuses after unequip", len(sink.active))
	}
}

func TestBonusChecks(t *testing.T) {
	cases := map[string]string{
		`bonuses.add("lucky", 1)`:                                                  `there is no bonus "lucky"`,
		`bonuses.add("luk", 1, { percent = true })`:                                "takes no percent option",
		`bonuses.add("move_speed", 10)`:                                            "add percent = true",
		`bonuses.add("cause_status", 2)`:                                           "needs the chance option",
		`bonuses.add("damage_taken", -5, { kind = "magic", from = { race = 7 } })`: "has no rAthena sum",
		`bonuses.add("luk", "two")`:                                                "must be an integer or a function",
		`bonuses.add("attack_element", 3, { stacking = "highest" })`:               "keeps one value",
		`bonuses.add("atk", function() player:send_message("x") return 1 end)`:     "",
	}
	for code, want := range cases {
		h := newHost()
		h.equipped[10] = fakeEquipped{item: 1, wearer: 1}
		r := newItemRuntime(t, h, "scripts/items/x.lua", `
local bonuses = require "wa.bonuses"
local context = require "wa.context"
return function()
  local player = context:get_instance().wearer
  `+code+`
end`)
		sink := &recorder{r: r}
		_, err := r.Equip("core:scripts/items/x.lua", 10, 1, sink)
		if want == "" {
			// A computed function cannot change the game.
			if err != nil {
				t.Fatalf("%s: %v", code, err)
			}
			if _, _, _, err := r.Values(sink.active[0]); err == nil || !strings.Contains(err.Error(), "cannot run in a computed bonus") {
				t.Errorf("%s: %v", code, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", code, err, want)
		}
	}
}

func TestAutobonusEvents(t *testing.T) {
	h := newHost()
	h.equipped[10] = fakeEquipped{item: 1, wearer: 1}
	r := newItemRuntime(t, h, "scripts/items/proc.lua", `
local bonuses = require "wa.bonuses"
local context = require "wa.context"
local events = require "wa.events"
local random = require "wa.random"
local timers = require "wa.timers"
local effects = require "wa.effects"
return function()
  local player = context:get_instance().wearer
  local active
  events.on("damage", function(hit)
    if hit.wearer_is_source and hit.kind == "physical" and random.chance(1000, 1000) then
      active = timers.with_bonuses(3000, function()
        bonuses.add("aspd", 100, { percent = true })
      end, { restart = active })
      player:show_effect(effects.heal)
    end
  end)
  events.on("skill_used", function(use)
    if use.level == 5 then
      timers.with_bonuses(1000, function() bonuses.add("str", 1) end)
    end
  end)
  events.on("unequip", function(wearer)
    wearer:send_message("bye")
  end)
end`)
	sink := &recorder{r: r}
	c, err := r.Equip("core:scripts/items/proc.lua", 10, 1, sink)
	if err != nil {
		t.Fatal(err)
	}
	hit := Hit{Source: 1, SourceKind: "player", Target: 2, TargetKind: "player", Kind: "physical", Range: "melee", Amount: 50, WearerIsSource: true}
	if err := r.FireHit(c, hit); err != nil {
		t.Fatal(err)
	}
	if got := sink.values(t); got != "aspd=[100]" {
		t.Errorf("after the hit: %q", got)
	}
	// A second trigger restarts the duration instead of stacking.
	h.now = 2000
	if err := r.FireHit(c, hit); err != nil {
		t.Fatal(err)
	}
	if got := sink.values(t); got != "aspd=[100]" {
		t.Errorf("after the second hit: %q", got)
	}
	advance(h, r, 4000)
	if got := sink.values(t); got != "aspd=[100]" {
		t.Errorf("the restarted bonus ended early: %q", got)
	}
	advance(h, r, 5000)
	if got := sink.values(t); got != "" {
		t.Errorf("the bonus did not end: %q", got)
	}
	// A hit the wearer takes does not trigger it.
	hit.WearerIsSource = false
	if err := r.FireHit(c, hit); err != nil {
		t.Fatal(err)
	}
	if err := r.FireSkillUse(c, SkillUse{Skill: 5, Level: 5, X: 1, Y: 2}); err != nil {
		t.Fatal(err)
	}
	if got := sink.values(t); got != "str=[1]" {
		t.Errorf("after the skill: %q", got)
	}
	if err := r.Unequip(c); err != nil {
		t.Fatal(err)
	}
	if len(sink.active) != 0 || !slices.Contains(h.messages, "1:bye") {
		t.Errorf("after unequip: %d bonuses, messages %v", len(sink.active), h.messages)
	}
	if want := []string{"effect 1 312 false", "effect 1 312 false"}; !slices.Equal(h.calls, want) {
		t.Errorf("calls %q", h.calls)
	}
}

func TestItemEventNames(t *testing.T) {
	for event, want := range map[string]string{
		"damage": "", "skill_used": "", "use": "", "unequip": "",
		"attack":    `there is no event "attack"`,
		"hit_taken": `there is no event "hit_taken"`,
		"login":     `"login" is an event of mod and npc and event contexts`,
	} {
		h := newHost()
		r := newItemRuntime(t, h, "scripts/items/x.lua", `
local events = require "wa.events"
return function() events.on("`+event+`", function() end) end`)
		err := r.UseItem(1, 1, "core:scripts/items/x.lua")
		if want == "" && err != nil || want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Errorf("%s: %v", event, err)
		}
	}
}

func TestIntegersAndRandom(t *testing.T) {
	h := newHost()
	r := newItemRuntime(t, h, "scripts/items/x.lua", `
local events = require "wa.events"
local integers = require "wa.integers"
local random = require "wa.random"
local jobs = require "wa.jobs"
local races = require "wa.races"
return function()
  events.on("use", function(player)
    assert(integers.divide_truncated(-7, 2) == -3 and integers.divide_truncated(7, -2) == -3 and integers.divide_truncated(7, 2) == 3)
    assert(integers.remainder_truncated(-7, 2) == -1 and integers.remainder_truncated(7, -2) == 1)
    assert(random.integer(45, 65) == 65)
    assert(random.chance(3, 1000) == false and random.chance(1000, 1000) == true)
    assert(races.demi_human == 7 and races.all == 9999 and jobs.knight == 7)
  end)
end`)
	if err := r.UseItem(1, 1, "core:scripts/items/x.lua"); err != nil {
		t.Fatal(err)
	}
	if want := []int64{21, 1000, 1000}; !slices.Equal(h.draws, want) {
		t.Errorf("draws %v", h.draws)
	}
}

// A zone keeps at most MaxItemScripts item scripts loaded. Using one
// more drops the oldest, which loads again on its next use.
func TestItemScriptCap(t *testing.T) {
	h := newHost()
	var files []string
	for i := range MaxItemScripts + 2 {
		files = append(files, fmt.Sprintf("scripts/items/i%d.lua", i), `
local events = require "wa.events"
loads = (loads or 0) + 1
return function() events.on("use", function(player) player:send_message("used " .. loads) end) end`)
	}
	r := newItemRuntime(t, h, files...)
	for i := range MaxItemScripts + 2 {
		if err := r.UseItem(1, 1, fmt.Sprintf("core:scripts/items/i%d.lua", i)); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.setups) != MaxItemScripts || r.setups["core:scripts/items/i0.lua"] != nil {
		t.Fatalf("%d loaded", len(r.setups))
	}
	if err := r.UseItem(1, 1, "core:scripts/items/i0.lua"); err != nil {
		t.Fatal(err)
	}
	if last := h.messages[len(h.messages)-1]; last != fmt.Sprintf("1:used %d", MaxItemScripts+3) {
		t.Errorf("the dropped script did not load again: %s", last)
	}
}

// bonuses.add returns a handle. remove ends that one bonus, a second
// remove does nothing, and the context's end removes what is left.
func TestBonusHandles(t *testing.T) {
	h := newHost()
	h.equipped[10] = fakeEquipped{item: 1, refine: 4, wearer: 1}
	r := newItemRuntime(t, h, "scripts/items/x.lua", `
local bonuses = require "wa.bonuses"
local context = require "wa.context"
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  local equipped = context:get_instance()
  local str = bonuses.add("str", 1)
  local atk = bonuses.add("atk", function() return equipped.refine * 2 end)
  local flag = bonuses.add("see_hidden", { when = function() return equipped.refine > 0 end })
  local kept = bonuses.add("agi", 2)
  local timed
  events.on("skill_used", function(use)
    if use.level == 1 then
      str:remove()
      str:remove()
      atk:remove()
    elseif use.level == 2 then
      flag:remove()
      timers.with_bonuses(1000, function() timed = bonuses.add("luk", 3) bonuses.add("dex", 4) end)
    elseif use.level == 3 then
      timed:remove()
    end
  end)
  events.on("unequip", function() kept:remove() end)
end`)
	sink := &recorder{r: r}
	c, err := r.Equip("core:scripts/items/x.lua", 10, 1, sink)
	if err != nil {
		t.Fatal(err)
	}
	step := func(level int64, want string) {
		t.Helper()
		if err := r.FireSkillUse(c, SkillUse{Skill: 1, Level: level}); err != nil {
			t.Fatal(err)
		}
		if got := sink.values(t); got != want {
			t.Errorf("after %d: %q, want %q", level, got, want)
		}
	}
	if got := sink.values(t); got != "str=[1] atk=[8] see_hidden=[] agi=[2]" {
		t.Errorf("start: %q", got)
	}
	step(1, "see_hidden=[] agi=[2]")
	step(2, "agi=[2] luk=[3] dex=[4]")
	// A bonus of with_bonuses can end early by its handle. The timer
	// still ends the rest.
	step(3, "agi=[2] dex=[4]")
	advance(h, r, 1000)
	if got := sink.values(t); got != "agi=[2]" {
		t.Errorf("after the timer: %q", got)
	}
	// The unequip handler removes a bonus the context would remove
	// anyway. Neither fails.
	if err := r.Unequip(c); err != nil {
		t.Fatal(err)
	}
	if len(sink.active) != 0 || len(r.bonuses) != 0 {
		t.Errorf("%d active, %d handles", len(sink.active), len(r.bonuses))
	}
	if !slices.Equal(sink.log[len(sink.log)-1:], []string{"-agi"}) {
		t.Errorf("log %v", sink.log)
	}
}
