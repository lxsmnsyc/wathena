-- Tests of the Hunting Board: the quest, its kills and drops, its reward
-- and its time limit. They run in Prontera, where the Hunt Master stands
-- in a classic world. The last case runs on a generated town's square,
-- with a wactl built with -tags procedural.

local testing = require "wa.testing"
local monsters = require "wa.monsters"
local places = require "wa.places"

local PORING = monsters.id "PORING"
local JELLOPY = 909
local APPLE = 512

-- hunter is a player next to the Hunt Master.
local function hunter(world)
  return world:new_player { map = "prontera", x = 150, y = 178 }
end

-- kill_porings has the player kill n Porings.
local function kill_porings(world, player, n)
  for _ = 1, n do
    local poring = world:spawn_monster { monster = PORING, x = 151, y = 178 }
    world:kill(player, poring)
  end
end

return function()
  testing.case("the Hunt Master gives the quest", function(world)
    local player = hunter(world)
    testing.expect(player:get_quest_state("porings")).to_be("none")
    local said = world:talk(player, "Hunt Master", { 1 })
    testing.expect(said).to_contain("three of them within the hour")
    testing.expect(player:get_quest_state("porings")).to_be("active")
    testing.expect(player:get_quest_state("example.hunting-board:porings")).to_be("active")
    local count, goal = player:get_quest_progress("porings", 1)
    testing.expect(count).to_be(0)
    testing.expect(goal).to_be(3)
  end)

  testing.case("Poring kills count up to three and drop a Jellopy each", function(world)
    local player = hunter(world)
    world:talk(player, "Hunt Master", { 1 })
    local jellopy = player:count_item(JELLOPY)
    kill_porings(world, player, 4)
    testing.expect((player:get_quest_progress("porings", 1))).to_be(3)
    testing.expect(player:count_item(JELLOPY)).to_be(jellopy + 4)
    testing.expect(world:talk(player, "Hunt Master")).to_contain("500 zeny and an Apple")
    testing.expect(player:get_quest_state("porings")).to_be("complete")
    testing.expect(player:count_item(APPLE)).to_be(1)
    -- A complete quest counts no more kills, and drops nothing.
    kill_porings(world, player, 1)
    testing.expect(player:count_item(JELLOPY)).to_be(jellopy + 4)
    testing.expect(world:talk(player, "Hunt Master")).to_contain("quiet thanks to you")
  end)

  testing.case("the Hunt Master counts the kills left", function(world)
    local player = hunter(world)
    world:talk(player, "Hunt Master", { 1 })
    kill_porings(world, player, 2)
    testing.expect(world:talk(player, "Hunt Master")).to_contain("2 of 3 Porings")
  end)

  testing.case("the quest times out after an hour", function(world)
    local player = hunter(world)
    world:talk(player, "Hunt Master", { 1 })
    testing.expect(player:quest_timed_out("porings")).to_be_false()
    world:advance(60 * 60 * 1000 + 1000)
    testing.expect(player:quest_timed_out("porings")).to_be_true()
    testing.expect(world:talk(player, "Hunt Master")).to_contain("The hour is up")
    testing.expect(player:get_quest_state("porings")).to_be("none")
  end)

  testing.case("an unknown quest raises an error", function(world)
    local player = hunter(world)
    local ok, err = pcall(function()
      player:add_quest("wolves")
    end)
    testing.expect(ok).to_be_false()
    testing.expect(tostring(err)).to_contain("no quest")
  end)

  testing.case("in a procedural world the Hunt Master stands on the square", function(world)
    local squares = places.find("town:any", "square")
    if #squares == 0 then
      -- A wactl without -tags procedural has no generated towns.
      return
    end
    local x, y = squares[1]:get_position()
    local player = world:new_player { map = squares[1]:get_map(), x = x + 1, y = y }
    world:talk(player, "Hunt Master", { 1 })
    testing.expect(player:get_quest_state("porings")).to_be("active")
    local poring = world:spawn_monster { monster = PORING, x = x + 2, y = y }
    world:kill(player, poring)
    testing.expect((player:get_quest_progress("porings", 1))).to_be(1)
  end)
end
