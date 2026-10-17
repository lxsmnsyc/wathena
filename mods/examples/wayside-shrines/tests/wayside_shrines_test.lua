-- Tests of the generation layer. They run in the test world, whose
-- generated maps the layer saw when the test world was made. They need a
-- wactl built with -tags procedural.

local testing = require "wa.testing"
local monsters = require "wa.monsters"
local places = require "wa.places"

local VAGABOND_WOLF = monsters.id "VAGABOND_WOLF"

return function()
  testing.case("every town has a shrine with a keeper", function(world)
    local shrines = places.find("town:any", "shrine")
    local towns = places.find("town:any", "square")
    if #towns == 0 then
      -- A wactl without -tags procedural has no generated towns.
      return
    end
    testing.expect(#shrines).to_be(#towns)
    local x, y = shrines[1]:get_position()
    local player = world:new_player { map = shrines[1]:get_map(), x = x + 1, y = y }
    testing.expect(world:talk(player, "Shrine Keeper")).to_contain("This shrine stands at " .. shrines[1]:get_map())
  end)

  testing.case("the field has a ring of stones and its guard", function(world)
    local rings = places.find("map:gen:field:0:0", "stone_ring")
    if #places.find("town:any", "square") == 0 then
      return
    end
    testing.expect(#rings).to_be(1)
    local x, y = rings[1]:get_position()
    world:new_player { map = "gen:field:0:0", x = x, y = y }
    local wolves = world:get_monsters(VAGABOND_WOLF)
    testing.expect(#wolves).to_be(1)
  end)

  testing.case("the quest boards ask for offerings", function(world)
    local boards = places.find("town:any", "quest_board")
    if #boards == 0 then
      return
    end
    local x, y = boards[1]:get_position()
    local player = world:new_player { map = boards[1]:get_map(), x = x + 1, y = y }
    -- Each quest of the board shows its title when picked, and Not now
    -- leaves it. One of the six asks for offerings.
    local found = false
    for i = 1, 6 do
      local ok, said = pcall(world.talk, world, player, "Quest Board", { i, 2 })
      if ok and said:find("for the shrines", 1, true) then
        found = true
      end
    end
    testing.expect(found).to_be_true()
  end)
end
