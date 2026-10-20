package modscript

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// skillHost is a fake zone with entity queries and casts: monsters with
// cells, and a log of what the cast methods asked for.
type skillHost struct {
	*fakeHost
	cells   map[int64][2]int // monster cells
	queries []EntityQuery
	gone    map[Entity]bool
	// casterGone makes the cast methods report that the caster cannot
	// act.
	casterGone bool
	log        []string
	skillIDs   map[string]int64
}

func newSkillHost() *skillHost {
	return &skillHost{fakeHost: newHost(), cells: map[int64][2]int{}, gone: map[Entity]bool{},
		skillIDs: map[string]int64{"example.s:BOLT": 20_000}}
}

// addMonster places a monster at a cell.
func (h *skillHost) addMonster(id int64, name string, x, y int) {
	h.monsters[id] = name
	h.cells[id] = [2]int{x, y}
}

func (h *skillHost) MonsterField(id int64, field string) (script.Value, error) {
	if c, ok := h.cells[id]; ok && (field == "x" || field == "y") {
		if _, alive := h.monsters[id]; !alive {
			return script.Nil, errors.New("the monster is gone")
		}
		if field == "x" {
			return script.Int(int64(c[0])), nil
		}
		return script.Int(int64(c[1])), nil
	}
	return h.fakeHost.MonsterField(id, field)
}

func (h *skillHost) FindEntities(q EntityQuery) ([]Entity, error) {
	h.queries = append(h.queries, q)
	cx, cy, ok := q.Center()
	if !ok {
		// Players stand at 150,150.
		cx, cy = 150, 150
		if q.Around.Kind == "monster" {
			c := h.cells[q.Around.ID]
			cx, cy = c[0], c[1]
		}
	}
	dist := func(id int64) int {
		c := h.cells[id]
		return max(abs(c[0]-cx), abs(c[1]-cy))
	}
	var out []Entity
	for id := range h.monsters {
		e := Entity{Kind: "monster", ID: id}
		if _, placed := h.cells[id]; !placed || slices.Contains(q.Exclude, e) || len(q.Kinds) > 0 && !slices.Contains(q.Kinds, "monster") {
			continue
		}
		if (q.Area == AreaCell || q.Area == AreaAround) && dist(id) > q.Radius {
			continue
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b Entity) int {
		if q.Sort == SortNearest && dist(a.ID) != dist(b.ID) {
			return dist(a.ID) - dist(b.ID)
		}
		return int(a.ID - b.ID)
	})
	return out[:min(len(out), q.Limit)], nil
}

func (h *skillHost) EntityExists(e Entity) bool {
	if e.Kind == "monster" {
		_, ok := h.monsters[e.ID]
		return ok
	}
	_, ok := h.players[e.ID]
	return ok && !h.gone[e]
}

func (h *skillHost) ShowEntityEffect(e Entity, effect int64) error {
	h.log = append(h.log, fmt.Sprintf("effect %s:%d %d", e.Kind, e.ID, effect))
	return nil
}

func (h *skillHost) CastDealDamage(c Cast, target Entity, o CastDamage) (*CastDamageResult, error) {
	if h.casterGone {
		return nil, nil
	}
	h.log = append(h.log, fmt.Sprintf("damage %d %d by %s:%d to %s:%d %d%% hits %d %s %d", c.Skill, c.Level, c.Caster.Kind, c.Caster.ID,
		target.Kind, target.ID, o.Percent, o.Hits, o.Type, o.Element))
	return &CastDamageResult{Amount: o.Percent, Critical: true}, nil
}

func (h *skillHost) CastHeal(c Cast, target Entity, amount int64) (int64, bool, error) {
	if h.casterGone {
		return 0, false, nil
	}
	h.log = append(h.log, fmt.Sprintf("heal %s:%d %d", target.Kind, target.ID, amount))
	return amount, true, nil
}

func (h *skillHost) CastRunDefault(c Cast, target *Entity) error {
	if target == nil {
		target = c.Target
	}
	h.log = append(h.log, fmt.Sprintf("default %d on %s:%d", c.Skill, target.Kind, target.ID))
	return nil
}

func (h *skillHost) SkillID(name string) (int64, bool) {
	id, ok := h.skillIDs[name]
	return id, ok
}

// skillMod is a mod with a skill script at scripts/skills/s.lua and no
// entry script of its own, and more files after.
func skillMod(id string, grants []string, src string, more ...string) *mod.Scripts {
	sc := modScripts(id, grants, nil, append([]string{"scripts/skills/s.lua", src}, more...)...)
	sc.Entries = nil
	return sc
}

func newSkillRuntime(t *testing.T, h Host, mods ...*mod.Scripts) (*Runtime, *logBuffer) {
	t.Helper()
	var buf logBuffer
	var ids []string
	for _, m := range mods {
		ids = append(ids, m.Mod)
	}
	r, err := New(Options{Host: h, Log: slog.New(slog.NewTextHandler(&buf, nil)), Mods: ids}, mods)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, &buf
}

func advanceTo(h *skillHost, r *Runtime, now int64) {
	h.now = now
	r.Advance(now)
}

var alice = Entity{Kind: "player", ID: 1}

func TestCastEvent(t *testing.T) {
	h := newSkillHost()
	r, logs := newSkillRuntime(t, h, skillMod("example.s", nil, `
local events = require "wa.events"
local timers = require "wa.timers"
local context = require "wa.context"
return function()
  local skill = context:get_instance().skill
  local kind = context:get_kind()
  events.on("cast", function(cast)
    local me = cast.caster
    me:send_message(kind .. " " .. skill .. " " .. cast.skill .. " lv" .. cast.level .. " " .. cast.target.name ..
      " at " .. cast.x .. "," .. cast.y .. " on " .. cast.map .. " by " .. me.kind)
    local r = cast:deal_damage(cast.target, { percent = 150, hits = 2, type = "magic", element = 4 })
    me:send_message("dealt " .. r.amount .. " " .. tostring(r.is_critical) .. " " .. tostring(r.missed))
    me:send_message("healed " .. cast:heal(me, 50))
    cast:run_default()
    cast:run_default { target = me }
    local ok, err = pcall(function() cast.level = 3 end)
    me:send_message("writable " .. tostring(ok))
    timers.after(100, function()
      me:send_message("later " .. tostring(cast:deal_damage(cast.target)))
    end)
  end)
end`))
	h.addMonster(7, "Poring", 152, 150)
	target := Entity{Kind: "monster", ID: 7}
	c := Cast{ID: 99, Caster: alice, Skill: 20_000, Level: 3, Target: &target, X: 152, Y: 150}
	ran, err := r.RunCast("example.s:scripts/skills/s.lua", c)
	if err != nil || !ran {
		t.Fatalf("ran %v, err %v, logs %s", ran, err, logs)
	}
	first := r.skillCtxs[20_000]
	h.casterGone = true
	advanceTo(h, r, 100)
	want := []string{
		"1:skill 20000 20000 lv3 Poring at 152,150 on prontera by player",
		"1:dealt 150 true false",
		"1:healed 50",
		"1:writable false",
		"1:later nil",
	}
	if !slices.Equal(h.messages, want) {
		t.Errorf("messages\n%s\nwant\n%s\nlogs %s", strings.Join(h.messages, "\n"), strings.Join(want, "\n"), logs)
	}
	wantLog := []string{
		"damage 20000 3 by player:1 to monster:7 150% hits 2 magic 4",
		"heal player:1 50",
		"default 20000 on monster:7",
		"default 20000 on player:1",
	}
	if !slices.Equal(h.log, wantLog) {
		t.Errorf("host log %q", h.log)
	}
	// The setup function runs once per zone.
	h.casterGone = false
	if _, err := r.RunCast("example.s:scripts/skills/s.lua", c); err != nil {
		t.Fatal(err)
	}
	if r.skillCtxs[20_000] != first || first.Kind != KindSkill {
		t.Error("the second cast made a new context")
	}
}

func TestCastWithoutHandler(t *testing.T) {
	h := newSkillHost()
	r, logs := newSkillRuntime(t, h, skillMod("example.s", nil, `return function() end`))
	ran, err := r.RunCast("example.s:scripts/skills/s.lua", Cast{Caster: alice, Skill: 20_000, Level: 1})
	if err != nil || ran {
		t.Fatalf("ran %v, err %v, logs %s", ran, err, logs)
	}
}

func TestCastEventOnlyInSkillContexts(t *testing.T) {
	h := newSkillHost()
	_, logs := newSkillRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
return function() events.on("cast", function() end) end`))
	if !strings.Contains(logs.String(), `"cast" is an event of skill contexts, and this is a mod context`) {
		t.Errorf("logs %s", logs)
	}
}

func TestCastErrors(t *testing.T) {
	h := newSkillHost()
	r, logs := newSkillRuntime(t, h, skillMod("example.s", nil, `
local events = require "wa.events"
return function()
  events.on("cast", function(cast)
    local function try(f)
      local ok, err = pcall(f)
      cast.caster:send_message((tostring(err):gsub("^.-:%d+: ", "")))
    end
    try(function() cast:deal_damage(nil) end)
    try(function() cast:deal_damage(cast.caster, { power = 1 }) end)
    try(function() cast:deal_damage(cast.caster, { percent = -1 }) end)
    try(function() cast:deal_damage(cast.caster, { type = "fire" }) end)
    try(function() cast:deal_damage(cast.caster, { element = 10 }) end)
    try(function() cast:heal(cast.caster, -5) end)
    try(function() cast.deal_damage(cast.caster) end)
  end)
end`))
	if _, err := r.RunCast("example.s:scripts/skills/s.lua", Cast{Caster: alice, Skill: 20_000, Level: 1}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"1:cast:deal_damage: the target must be a player, monster or NPC handle",
		"1:cast:deal_damage: power is not an option; the options are percent, hits, type and element",
		"1:cast:deal_damage: percent must be a whole number of 0 or more",
		`1:cast:deal_damage: type is "weapon", "magic" or "misc"`,
		"1:cast:deal_damage: element must be an element of wa.elements, not all",
		"1:cast:heal: the amount must be a whole number of 0 or more",
		"1:not a cast: call the method with a colon, such as cast:deal_damage(target)",
	}
	if !slices.Equal(h.messages, want) {
		t.Errorf("messages\n%s\nlogs %s", strings.Join(h.messages, "\n"), logs)
	}
}

func TestSkillHooks(t *testing.T) {
	h := newSkillHost()
	hooks := modScripts("example.h", []string{"rules.override"}, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
return function()
  hooks.on("skill_damage", function(o, next)
    if o.caster.kind ~= "player" or o.target.kind ~= "monster" or o.skill ~= 20000 then error("bad object") end
    return next(o) * 2
  end)
  hooks.on("cast_time", function(o, next) return next(o) // 2 end)
  hooks.on("skill_cost", function(o, next) return next(o) + 5 end)
  hooks.on("cast_allowed", function(o, next) return o.level < 5 and next(o) end)
end`)
	r, logs := newSkillRuntime(t, h, hooks, skillMod("example.s", nil, `
local events = require "wa.events"
return function()
  events.on("cast", function(cast) cast:deal_damage(cast.target, { percent = 150 }) end)
end`))
	h.addMonster(7, "Poring", 152, 150)
	target := Entity{Kind: "monster", ID: 7}
	if _, err := r.RunCast("example.s:scripts/skills/s.lua", Cast{Caster: alice, Skill: 20_000, Level: 3, Target: &target}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"damage 20000 3 by player:1 to monster:7 300% hits 0  -1"}; !slices.Equal(h.log, want) {
		t.Errorf("host log %q, logs %s", h.log, logs)
	}
	hk := SkillHook{Caster: alice, Skill: 20_000, Level: 3, Target: &target}
	if got := r.RunSkillDamage(hk, 100); got != 200 {
		t.Errorf("skill_damage %d", got)
	}
	if got := r.RunCastTime(hk, 1000); got != 500 {
		t.Errorf("cast_time %d", got)
	}
	if got := r.RunSkillCost(SkillHook{Caster: alice, Skill: 20_000, Level: 3}, 10); got != 15 {
		t.Errorf("skill_cost %d", got)
	}
	if !r.RunCastAllowed(hk, true) || r.RunCastAllowed(hk, false) {
		t.Error("cast_allowed should keep rAthena's answer below level 5")
	}
	hk.Level = 5
	if r.RunCastAllowed(hk, true) {
		t.Error("cast_allowed should refuse level 5")
	}
	if !r.HasHooks("cast_allowed") || strings.Contains(logs.String(), "level=WARN") {
		t.Errorf("logs %s", logs)
	}
}

func TestSkillHooksWithout(t *testing.T) {
	h := newSkillHost()
	r, _ := newSkillRuntime(t, h)
	hk := SkillHook{Caster: alice, Skill: 1, Level: 1}
	if r.RunSkillDamage(hk, 120) != 120 || r.RunCastTime(hk, 700) != 700 || r.RunSkillCost(hk, 9) != 9 || !r.RunCastAllowed(hk, true) {
		t.Error("without hooks the values are rAthena's")
	}
}

func TestCastAllowedHookMustReturnBoolean(t *testing.T) {
	h := newSkillHost()
	r, logs := newSkillRuntime(t, h, modScripts("example.h", []string{"rules.override"}, nil, "scripts/main.lua", `
local hooks = require "wa.hooks"
return function() hooks.on("cast_allowed", function(o, next) return 1 end) end`))
	if !r.RunCastAllowed(SkillHook{Caster: alice, Skill: 1, Level: 1}, true) {
		t.Error("a failed hook should leave the answer")
	}
	if !strings.Contains(logs.String(), "the cast_allowed hook must return true or false") {
		t.Errorf("logs %s", logs)
	}
}

func TestPassiveSkill(t *testing.T) {
	h := newSkillHost()
	r, logs := newSkillRuntime(t, h, skillMod("example.s", nil, `
local context = require "wa.context"
local timers = require "wa.timers"
return function()
  local skill = context:get_instance()
  skill.owner:send_message("start " .. skill.skill .. " lv" .. skill.level .. " " .. skill.owner.kind .. " " .. context:get_kind())
  timers.every(1000, function() skill.owner:send_message("tick lv" .. skill.level) end)
end`))
	const entry = "example.s:scripts/skills/s.lua"
	if err := r.StartPassive(entry, 30, alice, 3); err != nil {
		t.Fatal(err)
	}
	advanceTo(h, r, 1000)
	// The same level again does nothing.
	if err := r.StartPassive(entry, 30, alice, 3); err != nil {
		t.Fatal(err)
	}
	// A new level starts a new context, and the old one's timer stops.
	if err := r.StartPassive(entry, 30, alice, 4); err != nil {
		t.Fatal(err)
	}
	advanceTo(h, r, 2000)
	advanceTo(h, r, 3000)
	r.StopPassive(30, alice)
	advanceTo(h, r, 4000)
	if err := r.StartPassive(entry, 31, alice, 1); err != nil {
		t.Fatal(err)
	}
	r.StopPassives(alice)
	advanceTo(h, r, 6000)
	want := []string{"1:start 30 lv3 player skill", "1:tick lv3", "1:start 30 lv4 player skill", "1:tick lv4", "1:tick lv4", "1:start 31 lv1 player skill"}
	if !slices.Equal(h.messages, want) {
		t.Errorf("messages %q, logs %s", h.messages, logs)
	}
	if len(r.passives) != 0 {
		t.Errorf("%d passive contexts left", len(r.passives))
	}
}

func TestReloadRestartsPassives(t *testing.T) {
	h := newSkillHost()
	sc := skillMod("example.s", nil, `
local context = require "wa.context"
return function()
  local i = context:get_instance()
  if i.owner then i.owner:send_message("v1") end
end`)
	r, _ := newSkillRuntime(t, h, sc)
	if err := r.StartPassive("example.s:scripts/skills/s.lua", 30, alice, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RunCast("example.s:scripts/skills/s.lua", Cast{Caster: alice, Skill: 40, Level: 1}); err != nil {
		t.Fatal(err)
	}
	next := skillMod("example.s", nil, `
local context = require "wa.context"
return function()
  local i = context:get_instance()
  if i.owner then i.owner:send_message("v2") end
end`)
	if err := r.Reload(next); err != nil {
		t.Fatal(err)
	}
	if len(r.skillCtxs) != 0 || len(r.passives) != 1 {
		t.Errorf("%d skill contexts, %d passives", len(r.skillCtxs), len(r.passives))
	}
	if want := []string{"1:v1", "1:v2"}; !slices.Equal(h.messages, want) {
		t.Errorf("messages %q", h.messages)
	}
}

func TestFindEntities(t *testing.T) {
	h := newSkillHost()
	h.addMonster(7, "Poring", 152, 150)
	h.addMonster(8, "Lunatic", 154, 150)
	h.addMonster(9, "Fabre", 170, 150)
	r, logs := newSkillRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local maps = require "wa.maps"
return function()
  events.on("login", function(player)
    local list = maps.find_entities { x = 150, y = 150, radius = 5, sort = "nearest" }
    local names = {}
    for i, m in ipairs(list) do names[i] = m.name .. ":" .. m.kind end
    player:send_message(table.concat(names, ","))
    local near = list[1]:find_nearby { radius = 3, exclude = { list[1] }, kinds = { "monster" }, enemies_of = player, limit = 1 }
    player:send_message(near[1].name .. " " .. tostring(near[1] == list[2]))
    maps.find_entities { rectangle = { { 140, 140 }, { 160, 160 } }, shape = nil, alive = false, line_of_sight = true, allies_of = player }
    maps.find_entities { line = { from = { 150, 150 }, to = { 155, 150 }, width = 3 }, map = "prontera" }
    maps.find_entities { cells = { { 152, 150 }, { 153, 150 } }, sort = "random" }
    maps.find_entities { around = player, radius = 2, shape = "circle", sort = "lowest_hp" }
    local function try(q)
      local ok, err = pcall(maps.find_entities, q)
      player:send_message((tostring(err):gsub("^.-:%d+: ", "")))
    end
    try { radius = 5 }
    try { x = 1, y = 1, radius = 2, cells = { { 1, 1 } } }
    try { x = 1, radius = 2 }
    try { x = 1, y = 1 }
    try { x = 1, y = 1, radius = 31 }
    try { cells = { { 1, 1 } }, radius = 2 }
    try { x = 1, y = 1, radius = 2, map = "geffen" }
    try { x = 1, y = 1, radius = 2, sort = "best" }
    try { x = 1, y = 1, radius = 2, kinds = { "pet" } }
    try { x = 1, y = 1, radius = 2, around_of = player }
    try { around = "Alice", radius = 2 }
    try { x = 1, y = 1, radius = 2, exclude = { 7 } }
    try { x = 1, y = 1, radius = 2, enemies_of = player, allies_of = player }
    try { rectangle = { { 1, 1 }, { 100, 1 } } }
    try { line = { from = { 1, 1 } } }
    try { x = 1, y = 1, radius = 2, limit = 0 }
    local ok, err = pcall(function() return player:find_nearby { x = 1, radius = 2 } end)
    player:send_message((tostring(err):gsub("^.-:%d+: ", "")))
  end)
end`))
	r.Fire("login", Player(1))
	want := []string{
		"Poring:monster,Lunatic:monster",
		"Lunatic true",
		"maps.find_entities: the query needs an area: around, x and y, rectangle, line or cells",
		"maps.find_entities: the query has one area, and this one has cells and x and y",
		"maps.find_entities: give both x and y",
		"maps.find_entities: x and y needs radius",
		"maps.find_entities: radius must be 0 to 30 cells",
		"maps.find_entities: radius and shape go with around, or x and y, and this area is cells",
		"maps.find_entities: this context runs on prontera, and finds entities on its own map only, not on geffen",
		`maps.find_entities: sort is "nearest", "farthest", "lowest_hp" or "random"`,
		`maps.find_entities: kinds holds "player", "monster" and "npc"`,
		`maps.find_entities: "around_of" is not a field; the fields are ` + queryFields,
		"maps.find_entities: around must be a player, monster or NPC handle",
		"maps.find_entities: exclude must be a list of player, monster or NPC handles",
		"maps.find_entities: give enemies_of or allies_of, not both",
		"maps.find_entities: a rectangle's sides are at most 61 cells",
		"maps.find_entities: line needs from and to",
		"maps.find_entities: limit must be 1 to 1000",
		"player:find_nearby: the area is around the player, so the query cannot have x",
	}
	var got []string
	for _, m := range h.messages {
		got = append(got, strings.TrimPrefix(m, "1:"))
	}
	if !slices.Equal(got, want) {
		t.Errorf("messages\n%s\nwant\n%s\nlogs %s", strings.Join(got, "\n"), strings.Join(want, "\n"), logs)
	}
	if len(h.queries) != 6 {
		t.Fatalf("%d queries", len(h.queries))
	}
	q := h.queries
	if q[0].Area != AreaCell || q[0].X != 150 || q[0].Radius != 5 || !q[0].Alive || q[0].Limit != MaxFound || q[0].Sort != SortNearest {
		t.Errorf("cell query %+v", q[0])
	}
	if q[1].Area != AreaAround || q[1].Around != (Entity{"monster", 7}) || !slices.Equal(q[1].Exclude, []Entity{{"monster", 7}}) ||
		*q[1].EnemiesOf != alice || q[1].Limit != 1 || !slices.Equal(q[1].Kinds, []string{"monster"}) {
		t.Errorf("nearby query %+v", q[1])
	}
	if q[2].Area != AreaRectangle || q[2].Corners != [2][2]int{{140, 140}, {160, 160}} || q[2].Alive || !q[2].LineOfSight || *q[2].AlliesOf != alice {
		t.Errorf("rectangle query %+v", q[2])
	}
	if q[3].Area != AreaLine || q[3].From != [2]int{150, 150} || q[3].To != [2]int{155, 150} || q[3].Width != 3 {
		t.Errorf("line query %+v", q[3])
	}
	if q[4].Area != AreaCells || len(q[4].Cells) != 2 || q[4].Sort != SortRandom {
		t.Errorf("cells query %+v", q[4])
	}
	if q[5].Area != AreaAround || q[5].Around != alice || !q[5].Circle || q[5].Sort != SortLowestHP {
		t.Errorf("around query %+v", q[5])
	}
}

func TestFindEntitiesNeedsHost(t *testing.T) {
	h := newHost()
	r, _ := newRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local maps = require "wa.maps"
return function()
  events.on("login", function(player)
    local ok, err = pcall(maps.find_entities, { x = 1, y = 1, radius = 1 })
    player:send_message((tostring(err):gsub("^.-:%d+: ", "")))
  end)
end`))
	r.Fire("login", Player(1))
	if want := "1:maps.find_entities: this zone cannot find entities"; len(h.messages) != 1 || h.messages[0] != want {
		t.Errorf("messages %q", h.messages)
	}
}

func TestExistsAndShowEffect(t *testing.T) {
	for _, withHost := range []bool{true, false} {
		sh := newSkillHost()
		var host Host = sh
		if !withHost {
			host = sh.fakeHost
		}
		r, logs := newSkillRuntime(t, host, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local timers = require "wa.timers"
return function()
  events.on("death", function(death)
    local player, monster = death.killer, death.victim
    player:send_message("player " .. tostring(player:exists()) .. " monster " .. tostring(monster:exists()))
    local ok = pcall(monster.show_effect, monster, 12)
    player:send_message("effect " .. tostring(ok))
    player:show_effect(13)
    timers.after(10, function()
      player:send_message("later " .. tostring(monster:exists()) .. " " .. tostring(monster.exists(nil)))
    end)
  end)
end`))
		r.FireDeath(Death{Victim: 7, VictimKind: "monster", Monster: 1002, Killer: 1, KillerKind: "player"})
		delete(sh.monsters, 7)
		advanceTo(sh, r, 10)
		want := []string{"1:player true monster true", fmt.Sprintf("1:effect %v", withHost), "1:later false false"}
		if !slices.Equal(sh.messages, want) {
			t.Errorf("host %v: messages %q, logs %s", withHost, sh.messages, logs)
		}
		if withHost && !slices.Equal(sh.log, []string{"effect monster:7 12"}) {
			t.Errorf("host log %q", sh.log)
		}
		if !slices.Contains(sh.calls, "effect 1 13 false") {
			t.Errorf("player effect: calls %q", sh.calls)
		}
	}
}

func TestSkillUsedEvent(t *testing.T) {
	h := newSkillHost()
	r, logs := newSkillRuntime(t, h, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local skills = require "wa.skills"
local BOLT = skills.id "example.s:BOLT"
return function()
  local function show(prefix)
    return function(use)
      local keys = {}
      for k in pairs(use) do keys[#keys + 1] = k end
      table.sort(keys)
      local target = use.target and (use.target.kind .. " " .. use.target.name) or "ground"
      local who = use.caster.kind == "player" and use.caster or nil
      if who then who:send_message(prefix .. " " .. use.skill .. " lv" .. use.level .. " " .. target .. " " .. use.x .. "," .. use.y .. " " .. table.concat(keys, ",")) end
    end
  end
  events.on("skill_used", show("any"))
  events.on("skill_used", show("bolt"), { skill = BOLT, caster_kind = "player", map = "prontera" })
  events.on("skill_used", show("geffen"), { map = "geffen" })
  local ok, err = pcall(events.on, "skill_used", show("x"), { level = 1 })
  if not ok and err:find("the options are skill, caster_kind and map", 1, true) == nil then error(err) end
end`))
	poring := Entity{Kind: "monster", ID: 7}
	r.FireSkillUsed(SkillUsed{Caster: alice, Skill: 20_000, Level: 2, Target: &poring, X: 3, Y: 4})
	r.FireSkillUsed(SkillUsed{Caster: alice, Skill: 5, Level: 1, X: 5, Y: 6})
	want := []string{
		"1:any 20000 lv2 monster Poring 3,4 caster,level,skill,target,x,y",
		"1:bolt 20000 lv2 monster Poring 3,4 caster,level,skill,target,x,y",
		"1:any 5 lv1 ground 5,6 caster,level,skill,x,y",
	}
	if !slices.Equal(h.messages, want) {
		t.Errorf("messages\n%s\nlogs %s", strings.Join(h.messages, "\n"), logs)
	}
}

// TestChainLightningExample runs the cast of mods/examples/chain-lightning
// against monsters in a row.
func TestChainLightningExample(t *testing.T) {
	src, err := os.ReadFile("../../mods/examples/chain-lightning/scripts/skills/chain_lightning.lua")
	if err != nil {
		t.Fatal(err)
	}
	const entry = "example.chain-lightning:scripts/skills/chain_lightning.lua"
	run := func(level int64, stopAt int) []string {
		h := newSkillHost()
		// The first target, then monsters 4, 3 and 6 cells further, one
		// that is nearer to the target than to the others, and one out of
		// reach.
		h.addMonster(10, "A", 150, 150)
		h.addMonster(11, "B", 154, 150)
		h.addMonster(12, "C", 157, 151)
		h.addMonster(13, "D", 152, 152)
		h.addMonster(14, "Far", 175, 150)
		sc := modScripts("example.chain-lightning", nil, nil, "scripts/skills/chain_lightning.lua", string(src))
		sc.Entries = nil
		r, logs := newSkillRuntime(t, h, sc)
		target := Entity{Kind: "monster", ID: 10}
		if _, err := r.RunCast(entry, Cast{Caster: alice, Skill: 20_000, Level: level, Target: &target, X: 150, Y: 150}); err != nil {
			t.Fatal(err)
		}
		for i := int64(1); i <= 10; i++ {
			if len(h.log) >= stopAt {
				h.casterGone = true
			}
			advanceTo(h, r, i*150)
		}
		if strings.Contains(logs.String(), "level=WARN") {
			t.Errorf("logs %s", logs)
		}
		var hits []string
		for _, l := range h.log {
			f := strings.Fields(l)
			hits = append(hits, f[6]+" "+f[7])
		}
		return hits
	}
	// Level 1 jumps twice: to the nearest of B and D from A, then from
	// there.
	if got, want := run(1, 100), []string{"monster:10 120%", "monster:13 120%", "monster:11 120%"}; !slices.Equal(got, want) {
		t.Errorf("level 1: %q, want %q", got, want)
	}
	// Level 5 jumps until no enemy is within 5 cells.
	if got, want := run(5, 100), []string{"monster:10 200%", "monster:13 200%", "monster:11 200%", "monster:12 200%"}; !slices.Equal(got, want) {
		t.Errorf("level 5: %q, want %q", got, want)
	}
	// A caster that cannot act stops the chain.
	if got, want := run(5, 2), []string{"monster:10 200%", "monster:13 200%"}; !slices.Equal(got, want) {
		t.Errorf("caster gone: %q, want %q", got, want)
	}
}
