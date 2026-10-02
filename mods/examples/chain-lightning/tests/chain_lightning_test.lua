-- Tests of Chain Lightning, which wactl mod test runs. They check the
-- skill's data and the entity queries that its script uses to find the
-- next target.

local testing = require "wa.testing"
local maps = require "wa.maps"
local monsters = require "wa.monsters"
local skills = require "wa.skills"

local CHAIN_LIGHTNING = skills.id "example.chain-lightning:CHAIN_LIGHTNING"
local PORING = monsters.id "PORING"

return function()
  testing.case("chain lightning has an ID of the mod range", function(world)
    testing.expect(CHAIN_LIGHTNING >= 20000).to_be_true()
  end)

  testing.case("the next target is the nearest enemy not hit yet", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    local first = world:spawn_monster { monster = PORING, x = 155, y = 150 }
    local near = world:spawn_monster { monster = PORING, x = 157, y = 150 }
    local far = world:spawn_monster { monster = PORING, x = 159, y = 152 }
    -- Out of reach: 10 cells from the first target.
    world:spawn_monster { monster = PORING, x = 165, y = 150 }

    local found = maps.find_entities {
      x = first.x, y = first.y, radius = 5,
      enemies_of = player, exclude = { first }, sort = "nearest", limit = 1,
    }
    testing.expect(#found).to_be(1)
    testing.expect(found[1]).to_be(near)

    -- The same query from the monster, with both hit.
    found = first:find_nearby { radius = 5, enemies_of = player, exclude = { first, near }, sort = "nearest" }
    testing.expect(found).to_equal({ far })
  end)

  testing.case("a query needs one area", function(world)
    world:new_player { map = "prontera", x = 150, y = 150 }
    testing.expect(function() maps.find_entities { radius = 5 } end).to_fail_with("needs an area")
    testing.expect(function()
      maps.find_entities { x = 150, y = 150, radius = 5, cells = { { 150, 150 } } }
    end).to_fail_with("one area")
  end)

  testing.case("a killed monster no longer exists", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    local poring = world:spawn_monster { monster = PORING, x = 152, y = 150 }
    testing.expect(poring:exists()).to_be_true()
    world:kill(player, poring)
    testing.expect(poring:exists()).to_be_false()
  end)
end
