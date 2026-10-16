-- Tests of the Home Village sample. They run on the test world's
-- generated maps: a field and a town per town template, named by the
-- naming styles. They need a wactl built with -tags procedural.

local testing = require "wa.testing"
local towns = require "wa.towns"
local maps = require "wa.maps"
local places = require "wa.places"

return function()
  testing.case("every town has a name of 4 to 12 letters", function()
    local list = towns.list()
    testing.expect(#list).to_be(3)
    for i, town in ipairs(list) do
      testing.expect(town:get_id()).to_be("t" .. i)
      testing.expect(town:get_map()).to_be("gen:town:t" .. i)
      local name = town:get_name()
      testing.expect(#name >= 4 and #name <= 12).to_be_true()
      testing.expect(name:match("^%u%l+$")).to_be(name)
      testing.expect(maps.get_display_name(town:get_map())).to_be(name)
    end
  end)

  testing.case("a player who enters a town has visited it", function(world)
    local t2 = towns.get("t2")
    local square = places.find("town:t2", "square")[1]
    local x, y = square:get_position()
    local player = world:new_player { map = t2:get_map(), x = x, y = y }
    testing.expect(player:has_visited(t2)).to_be_true()
    testing.expect(player:has_visited(towns.get("t1"))).to_be_false()
    local visited = towns.list { visited_by = player }
    testing.expect(#visited).to_be(1)
    testing.expect(visited[1] == t2).to_be_true()
    testing.expect(towns.nearest(player) == t2).to_be_true()
  end)

  testing.case("a town's places share its band", function()
    local lo, hi = towns.get("t1"):get_band()
    testing.expect(lo >= 1).to_be_true()
    testing.expect(hi).to_be(lo + 9)
    local square = places.find("town:t1", "square")[1]
    local plo, phi = square:get_band()
    testing.expect(plo).to_be(lo)
    testing.expect(phi).to_be(hi)
    testing.expect(towns.get("t99")).to_be_nil()
  end)
end
