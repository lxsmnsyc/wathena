-- Tests of the market town. They run on the test world's generated maps:
-- a field, a town per town template and a dungeon per dungeon style. They
-- need a wactl built with -tags procedural.

local testing = require "wa.testing"
local places = require "wa.places"

-- The market town's lamp shop: its town has this mod's template.
local function lamp_shop()
  local found = places.find("town:any", "shop:lamp")
  testing.expect(#found).to_be(1)
  return found[1]
end

return function()
  testing.case("the template makes a town with a lamp shop", function()
    local shop = lamp_shop()
    testing.expect(shop:get_map()).to_contain("gen:town:t")
    testing.expect(shop:get_tags()).to_equal({ "shop:lamp" })
    -- The same town has the template's other places.
    local area = shop:get_area()
    testing.expect(#places.find(area, "blacksmith")).to_be(1)
    testing.expect(#places.find(area, "gate")).to_be(2)
  end)

  testing.case("the guide on the square points to the nearest blacksmith", function(world)
    local town = lamp_shop():get_area()
    local square = places.find(town, "square")[1]
    local x, y = square:get_position()
    local player = world:new_player { map = square:get_map(), x = x + 1, y = y }
    local smith = places.nearest(player, "blacksmith")
    testing.expect(smith:get_area()).to_be(town)
    local sx, sy = smith:get_position()
    local said = world:talk(player, "Town Guide")
    testing.expect(said).to_contain("The nearest blacksmith is in " .. town .. " at " .. sx .. ", " .. sy .. ".")
    testing.expect(said).to_contain("1 towns sell lamps.")
  end)

  testing.case("the lamp seller stands at the lamp shop", function(world)
    local shop = lamp_shop()
    local x, y = shop:get_position()
    local player = world:new_player { map = shop:get_map(), x = x, y = y, zeny = 1000 }
    local ok = world:buy(player, "example.market-town:Lamp Seller", { [601] = 2 })
    testing.expect(ok).to_be_true()
    testing.expect(player:count_item(601)).to_be(2)
  end)

  testing.case("a cellar has two floors and a boss room on the last", function()
    testing.expect(#places.find("dungeon:cellar", "entrance")).to_be(2)
    testing.expect(#places.find("dungeon:cellar", "exit")).to_be(1)
    local boss = places.find("dungeon:cellar", "boss_room")
    testing.expect(#boss).to_be(1)
    testing.expect(boss[1]:get_map()).to_be("gen:dungeon:cellar:2")
  end)

  testing.case("a rotation gives the cellar new floors, and the guide hears of it", function(world)
    world:set_time("2026-10-31 12:00", "Asia/Manila")
    local square = places.find(lamp_shop():get_area(), "square")[1]
    local x, y = square:get_position()
    local player = world:new_player { map = square:get_map(), x = x + 1, y = y }
    -- The cellar rotates daily at 06:00, so the next floors are the next
    -- day's.
    local date = world:rotate("cellar")
    testing.expect(date).to_be("2026-11-01")
    local boss = places.find("dungeon:cellar", "boss_room")
    testing.expect(#boss).to_be(1)
    testing.expect(boss[1]:get_map()).to_be("gen:dungeon:cellar:2026-11-01:2")
    -- A second rotation takes the day after.
    testing.expect(world:rotate("cellar")).to_be("2026-11-02")
    local said = world:talk(player, "Town Guide")
    testing.expect(said).to_contain("The cellar has new floors since 2026-11-01.")
    testing.expect(said).to_contain("The cellar has new floors since 2026-11-02.")
  end)

  testing.case("a world change gives the field's region its next season", function(world)
    local square = places.find(lamp_shop():get_area(), "square")[1]
    local x, y = square:get_position()
    local player = world:new_player { map = square:get_map(), x = x + 1, y = y }
    local region, season = world:change_region(0, 0)
    testing.expect(region).to_contain("t")
    testing.expect(season).to_be(1)
    testing.expect(select(2, world:change_region(0, 0))).to_be(2)
    testing.expect(world:talk(player, "Town Guide")).to_contain("The fields of " .. region .. " changed with season 2.")
  end)
end
