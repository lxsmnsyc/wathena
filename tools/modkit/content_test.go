//go:build procedural

package modkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// TestProceduralContent plays core's NPCs of procedural worlds in the
// test world, with the quests the test world made (decision 0044). It
// writes the cases of testdata/content-check with those quests, places
// and IDs, and runs them on both bases:
//   - a quest board's hunt, collect quest and delivery, with rewards;
//   - the Swordman guild's tests for a Novice and a Swordman, which end
//     in their job changes, and the changes of a High Novice and a High
//     Swordman without a test;
//   - the rebirth NPC, which refuses a Knight with zeny and makes one
//     without anything a High Novice;
//   - the Kafra's storage fee, the inn, the tool dealer and the refiner.
func TestProceduralContent(t *testing.T) {
	testrace.SkipUnderRace(t, "its mod test worlds run on one goroutine")
	e := env(t)
	for _, base := range []string{mod.Renewal, mod.PreRenewal} {
		dir := t.TempDir()
		b, err := os.ReadFile("testdata/content-check/mod.yaml")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "mod.yaml"), b, 0o600); err != nil {
			t.Fatal(err)
		}
		l, err := e.Load(context.Background(), dir, "", base, nil)
		if err != nil {
			t.Fatal(err)
		}
		g, err := l.generatedMaps()
		if err != nil || g == nil {
			t.Fatalf("%s: generated maps: %v", base, err)
		}
		checkTestWorldContent(t, base, g)
		if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "tests", "content_test.lua"), []byte(contentCases(t, g)), 0o600); err != nil {
			t.Fatal(err)
		}
		l, err = e.Load(context.Background(), dir, "", base, nil)
		if err != nil {
			t.Fatal(err)
		}
		results, err := e.RunTests(context.Background(), l, "", func(CaseResult) {})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 14 {
			t.Errorf("%s: %d cases", base, len(results))
		}
		for _, r := range results {
			if r.Failed() {
				t.Errorf("%s: %s: %v\n%s", base, r.Name, r.Err, strings.Join(r.Logs, "\n"))
			}
		}
	}
}

// checkTestWorldContent checks the test world's content: a quest board
// with quests in each town, every guild once with its two tests, and the
// rebirth NPC, all standing where their places are.
func checkTestWorldContent(t *testing.T, base string, g *generated) {
	t.Helper()
	towns := 0
	for _, f := range g.set.Fields() {
		if f.Kind != gen.KindTown {
			continue
		}
		towns++
		if len(g.places.Find(scope(t, "map:"+f.ID), content.TagQuestBoard)) != 1 {
			t.Errorf("%s: %s has no quest board", base, f.ID)
		}
		if !slices.ContainsFunc(g.world, func(q modscript.CoreQuest) bool { return q.Town == f.ID && q.Purpose == content.PurposeBoard }) {
			t.Errorf("%s: %s has no board quests", base, f.ID)
		}
	}
	for _, family := range content.Families {
		if n := len(g.places.Find(scope(t, "town:any"), content.GuildTag(family))); n != 1 {
			t.Errorf("%s: %d %s guilds", base, n, family)
		}
		for _, stage := range content.Stages {
			steps := 0
			for _, q := range g.world {
				if q.Guild == family && q.Stage == stage {
					steps++
				}
			}
			if steps != 2 {
				t.Errorf("%s: the %s guild's %s test has %d steps", base, family, stage, steps)
			}
		}
	}
	if n := len(g.places.Find(scope(t, "town:any"), content.TagRebirth)); n != 1 || towns == 0 {
		t.Errorf("%s: %d rebirth places in %d towns", base, n, towns)
	}
	for _, name := range []string{"Quest Board#gen", "Swordman Guild Master#gen", "Valkyrie#gen", "Kafra Employee#gen", "Refiner#gen"} {
		found := false
		for _, list := range g.npcs {
			found = found || slices.ContainsFunc(list, func(n zone.NPC) bool { return n.Name == "core:"+name || n.Name == name })
		}
		if !found {
			t.Errorf("%s: no %s", base, name)
		}
	}
}

// contentCases writes the Lua cases of TestProceduralContent.
func contentCases(t *testing.T, g *generated) string {
	t.Helper()
	boardOf := func(town string) []modscript.CoreQuest {
		var out []modscript.CoreQuest
		for _, q := range g.world {
			if q.Purpose == content.PurposeBoard && q.Town == town {
				out = append(out, q)
			}
		}
		return out
	}
	// The first town with a quest board quest of each kind.
	pick := func(kind string) (modscript.CoreQuest, int) {
		for _, f := range g.set.Fields() {
			for i, q := range boardOf(f.ID) {
				if q.Kind == kind {
					return q, i + 1
				}
			}
		}
		t.Fatalf("no board quest of kind %s", kind)
		return modscript.CoreQuest{}, 0
	}
	hunt, huntAt := pick(content.KindHunt)
	collect, collectAt := pick(content.KindCollect)
	delivery, _ := pick(content.KindDelivery)
	guild := map[string]modscript.CoreQuest{}
	for _, q := range g.world {
		if q.Guild == "swordman" {
			guild[fmt.Sprintf("%s%d", q.Stage, q.Step)] = q
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, `local testing = require "wa.testing"
local jobs = require "wa.jobs"
local skills = require "wa.skills"
local items = require "wa.items"
local places = require "wa.places"

local BASIC = skills.id "NV_BASIC"

-- place returns the map and cell of the first place with a tag.
local function place(tag)
  local p = places.find("town:any", tag)[1]
  local x, y = p:get_position()
  return p:get_map(), x, y
end

-- near adds a player on a walkable cell next to a place with a tag.
local function near(world, tag, spec)
  local map, x, y = place(tag)
  for _, d in ipairs({ { 1, 0 }, { -1, 0 }, { 0, 1 }, { 0, -1 }, { 1, 1 }, { -1, -1 } }) do
    spec.map, spec.x, spec.y = map, x + d[1], y + d[2]
    local ok, player = pcall(world.new_player, world, spec)
    if ok then
      return player
    end
  end
  error("no walkable cell next to " .. tag)
end

local function kill(world, player, monster, n)
  for _ = 1, n do
    world:kill(player, world:spawn_monster { monster = monster, x = player.x + 1, y = player.y })
  end
end

return function()
`)
	q := func(name string, x modscript.CoreQuest) {
		fmt.Fprintf(&b, "  local %s = { id = %d, monster = %d, item = %d, count = %d, zeny = %d, town = %q, to = %q }\n",
			name, x.ID, x.Monster, x.Item, x.Count, x.Zeny, x.Town, x.To)
	}
	q("HUNT", hunt)
	q("COLLECT", collect)
	q("DELIVERY", delivery)
	q("FIRST1", guild["first1"])
	q("FIRST2", guild["first2"])
	q("SECOND1", guild["second1"])
	q("SECOND2", guild["second2"])
	fmt.Fprintf(&b, "  local HUNT_AT, COLLECT_AT = %d, %d\n", huntAt, collectAt)
	b.WriteString(`
  -- board returns a player next to the quest board of a town.
  local function at_board(world, town, spec)
    for _, p in ipairs(places.find("map:" .. town, "quest_board")) do
      local x, y = p:get_position()
      spec.map, spec.x, spec.y = town, x + 1, y
      return world:new_player(spec)
    end
    error("no quest board in " .. town)
  end

  testing.case("a board's hunt gives its reward", function(world)
    local player = at_board(world, HUNT.town, {})
    world:talk(player, "Quest Board", { HUNT_AT, 1 })
    testing.expect(player:get_quest_state(HUNT.id)).to_be("active")
    kill(world, player, HUNT.monster, HUNT.count)
    testing.expect(world:talk(player, "Quest Board", { HUNT_AT })).to_contain("Quest complete")
    testing.expect(player:get_quest_state(HUNT.id)).to_be("complete")
    testing.expect(player.zeny).to_be(HUNT.zeny)
    -- A done quest may be taken again.
    world:talk(player, "Quest Board", { HUNT_AT, 1 })
    testing.expect(player:get_quest_state(HUNT.id)).to_be("active")
  end)

  testing.case("a board's collect quest takes the items", function(world)
    local player = at_board(world, COLLECT.town, { items = { { COLLECT.item, COLLECT.count + 1 } } })
    world:talk(player, "Quest Board", { COLLECT_AT, 1 })
    testing.expect(world:talk(player, "Quest Board", { COLLECT_AT })).to_contain("Quest complete")
    testing.expect(player:count_item(COLLECT.item)).to_be(1)
    testing.expect(player.zeny).to_be(COLLECT.zeny)
  end)

  testing.case("a delivery ends at the board of its town", function(world)
    local spec = {}
    if DELIVERY.item ~= 0 then
      spec.items = { { DELIVERY.item, 1 } }
    end
    local player = at_board(world, DELIVERY.to, spec)
    player:add_quest(DELIVERY.id)
    testing.expect(world:talk(player, "Quest Board", { 1 })).to_contain("Delivered")
    testing.expect(player:get_quest_state(DELIVERY.id)).to_be("complete")
    testing.expect(player.zeny).to_be(DELIVERY.zeny)
  end)

  testing.case("a Novice passes the Swordman test and changes job", function(world)
    local player = near(world, "guild:swordman", { job_level = 10, skills = { { BASIC, 9 } },
      items = { { FIRST2.item, FIRST2.count } } })
    world:talk(player, "Swordman Guild Master", { 1 })
    testing.expect(player:get_quest_state(FIRST1.id)).to_be("active")
    kill(world, player, FIRST1.monster, FIRST1.count)
    world:talk(player, "Swordman Guild Master", { 1 })
    testing.expect(player:get_quest_state(FIRST1.id)).to_be("complete")
    testing.expect(player:get_quest_state(FIRST2.id)).to_be("active")
    testing.expect(world:talk(player, "Swordman Guild Master", { 1 })).to_contain("You are now a Swordman")
    testing.expect(player.job).to_be(jobs.swordman)
    testing.expect(player:count_item(FIRST2.item)).to_be(0)
  end)

  testing.case("a Novice below job level 10 is turned away", function(world)
    local player = near(world, "guild:swordman", { job_level = 9, skills = { { BASIC, 9 } } })
    testing.expect(world:talk(player, "Swordman Guild Master")).to_contain("reach job level 10")
  end)

  testing.case("a Swordman passes the second test and becomes a Knight", function(world)
    local player = near(world, "guild:swordman", { job = jobs.swordman, base_level = 50, job_level = 40,
      items = { { SECOND2.item, SECOND2.count } } })
    world:talk(player, "Swordman Guild Master", { 1 })
    kill(world, player, SECOND1.monster, SECOND1.count)
    world:talk(player, "Swordman Guild Master", { 1 })
    testing.expect(world:talk(player, "Swordman Guild Master", { 1 })).to_contain("You are now a Knight")
    testing.expect(player.job).to_be(jobs.knight)
  end)

  testing.case("a Swordman with skill points left waits", function(world)
    local player = near(world, "guild:swordman", { job = jobs.swordman, base_level = 50, job_level = 40, skill_points = 1 })
    testing.expect(world:talk(player, "Swordman Guild Master")).to_contain("Use all your skill points")
  end)

  testing.case("a High Novice changes without a test", function(world)
    local player = near(world, "guild:swordman", { job = jobs.novice_high, job_level = 10, skills = { { BASIC, 9 } } })
    world:talk(player, "Swordman Guild Master", { 1 })
    testing.expect(player.job).to_be(jobs.swordman_high)
  end)

  testing.case("the Valkyrie reborns a Knight with nothing", function(world)
    local rich = near(world, "rebirth", { job = jobs.knight, base_level = 99, job_level = 50, zeny = 100 })
    testing.expect(world:talk(rich, "Valkyrie")).to_contain("Leave your items")
    local player = near(world, "rebirth", { job = jobs.knight, base_level = 99, job_level = 50 })
    world:talk(player, "Valkyrie", { 1 })
    testing.expect(player.job).to_be(jobs.novice_high)
    testing.expect(player:count_item(1202)).to_be(1)
  end)

  testing.case("the Kafra charges the storage fee", function(world)
    local player = near(world, "storage", { zeny = 1000, skills = { { BASIC, 6 } } })
    local lowest = require("wa.towns").nearest(player):get_band()
    local fee = math.min(40 + 20 * (lowest // 20), 100)
    -- Use Storage: F_KafStor takes the fee and opens the storage, which
    -- ends the talk.
    world:talk(player, "Kafra Employee", { 1 })
    testing.expect(player.zeny).to_be(1000 - fee)
  end)

  testing.case("the inn saves and rests", function(world)
    local player = near(world, "inn", { zeny = 5000 })
    testing.expect(world:talk(player, "Innkeeper", { 1 })).to_contain("respawn point")
    world:talk(player, "Innkeeper", { 2 })
    testing.expect(player.zeny).to_be(0)
  end)

  testing.case("the refiner takes nothing without the ore", function(world)
    local knife = items.id "Knife"
    local player = near(world, "blacksmith", { zeny = 1000, items = { { knife, 1 } } })
    world:equip(player, knife)
    testing.expect(world:talk(player, "Refiner", { 1, 1 })).to_contain("enough Zeny or")
    testing.expect(player:get_equipped("right_hand").refine).to_be(0)
    testing.expect(player.zeny).to_be(1000)
  end)

  testing.case("the tool dealer opens the shop of the town's band", function(world)
    local player = near(world, "shop:tool", { zeny = 1000 })
    world:talk(player, "Tool Dealer")
    local lowest = require("wa.towns").nearest(player):get_band()
    local ok = world:buy(player, "core:shop_tool_" .. math.min(lowest // 20 + 1, 5), { [items.id "Red_Potion"] = 2 })
    testing.expect(ok).to_be_true()
    testing.expect(player:count_item(items.id "Red_Potion")).to_be(2)
  end)

  testing.case("the refiner refines a Knife to +1", function(world)
    local knife, phracon = items.id "Knife", items.id "Phracon"
    local player = near(world, "blacksmith", { zeny = 1000, items = { { knife, 1 }, { phracon, 1 } } })
    world:equip(player, knife)
    testing.expect(world:talk(player, "Refiner", { 1, 1 })).to_contain("Perfect")
    testing.expect(player:get_equipped("right_hand").refine).to_be(1)
    testing.expect(player.zeny).to_be(950)
    testing.expect(player:count_item(phracon)).to_be(0)
  end)
end
`)
	return b.String()
}

// scope reads a places scope.
func scope(t *testing.T, s string) places.Scope {
	t.Helper()
	sc, err := places.ParseScope(s)
	if err != nil {
		t.Fatal(err)
	}
	return sc
}
