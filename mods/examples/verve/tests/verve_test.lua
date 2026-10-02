-- Tests of Verve, which wactl mod test runs.

local testing = require "wa.testing"
local items = require "wa.items"
local statuses = require "wa.statuses"

local TONIC = items.id "example.verve:Verve_Tonic"

return function()
  testing.case("the tonic starts verve and raises max HP", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 191, items = { { TONIC, 1 } } }
    local max_hp = player.max_hp
    world:use_item(player, TONIC)
    testing.expect(player:has_status(statuses.verve)).to_be_true()
    local verve = player:get_status(statuses.verve)
    testing.expect(verve.level).to_be(3)
    testing.expect(verve.remaining_milliseconds).to_be(60000)
    testing.expect(player.max_hp).to_be(max_hp + 300)
  end)

  testing.case("verve heals on each tick", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 191, items = { { TONIC, 1 } } }
    world:use_item(player, TONIC)
    -- Max HP rose, so the player is 300 HP short.
    local hp = player.hp
    world:advance(5000)
    testing.expect(player.hp >= hp + player.max_hp * 2 // 100).to_be_true()
  end)

  testing.case("verve ends after its time", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 191, items = { { TONIC, 1 } } }
    local max_hp = player.max_hp
    world:use_item(player, TONIC)
    world:advance(60000)
    testing.expect(player:has_status(statuses.verve)).to_be_false()
    testing.expect(player.max_hp).to_be(max_hp)
  end)
end
