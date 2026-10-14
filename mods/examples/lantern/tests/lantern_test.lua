-- Tests of the lantern mod, which wactl mod test runs. The art is not
-- part of a test world: these tests check the records the art belongs to.

local testing = require "wa.testing"
local items = require "wa.items"
local monsters = require "wa.monsters"

local LANTERN = items.id "example.lantern:Paper_Lantern"
local UNLIT = items.id "example.lantern:Unlit_Lantern"
local GHOST = monsters.id "example.lantern:LANTERN_GHOST"

return function()
  testing.case("the lantern and the ghost are mod records", function()
    testing.expect(LANTERN >= 2000000).to_be_true()
    testing.expect(items.name(LANTERN)).to_be("Paper Lantern")
    testing.expect(GHOST >= 100000).to_be_true()
  end)

  testing.case("a lantern ghost drops a paper lantern", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 191 }
    local ghost = world:spawn_monster { monster = GHOST, x = player.x + 1, y = player.y }
    world:kill(player, ghost)
    testing.expect(world:count_floor_items(LANTERN)).to_be(1)
    testing.expect(world:count_floor_items(UNLIT) <= 1).to_be_true()
  end)
end
