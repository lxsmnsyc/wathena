-- Tests of the Lucky Clover mod, which wactl mod test runs. Each case
-- gets a fresh world with this mod and core loaded, and a fixed random
-- seed, so kills drop the same rolls each time.

local testing = require "wa.testing"
local items = require "wa.items"
local monsters = require "wa.monsters"

local CHARM = items.id "example.lucky-clover:Clover_Charm"
local CLOVER_PORING = monsters.id "example.lucky-clover:CLOVER_PORING"

-- A player next to the smith.
local function new_player(world, spec)
  spec = spec or {}
  spec.map, spec.x, spec.y = "prontera", 156, 191
  return world:new_player(spec)
end

return function()
  testing.case("the charm is a mod item with its name", function()
    testing.expect(CHARM >= 2000000).to_be_true()
    testing.expect(items.name(CHARM)).to_be("Clover Charm")
  end)

  testing.case("a test sets rolls without items.modify_rolls", function(world)
    local player = new_player(world)
    -- In a test case's own code, a table of rolls needs no permission.
    -- Values outside a roll's range are clamped.
    testing.expect(player:give_item(CHARM, 1, { roll = { luck = 5, fortune = 99 } })).to_be_true()
    testing.expect(player:give_item(CHARM, 1, { roll = "random" })).to_be_true()
    testing.expect(player:count_item(CHARM)).to_be(2)
  end)

  testing.case("a roll the charm lacks is an error", function(world)
    local player = new_player(world)
    local ok, err = pcall(player.give_item, player, CHARM, 1, { roll = { strength = 3 } })
    testing.expect(ok).to_be_false()
    testing.expect(tostring(err)).to_contain("has no roll \"strength\"")
    ok, err = pcall(player.give_item, player, CHARM, 1, { roll = "best" })
    testing.expect(ok).to_be_false()
    testing.expect(tostring(err)).to_contain("roll is \"random\", \"default\" or a table")
  end)

  testing.case("a clover poring drops a charm", function(world)
    local player = new_player(world)
    for _ = 1, 3 do
      local poring = world:spawn_monster { monster = CLOVER_PORING, x = player.x + 1, y = player.y }
      world:kill(player, poring)
    end
    testing.expect(world:count_floor_items(CHARM)).to_be(3)
  end)

  testing.case("the smith wants a worn charm", function(world)
    local player = new_player(world, { zeny = 5000 })
    player:give_item(CHARM, 1, { roll = { luck = 1 } })
    local said = world:talk(player, "Clover Smith#prontera", { 1 })
    testing.expect(said).to_contain("Wear a Clover Charm first.")
    -- Nothing was paid.
    testing.expect(player.zeny).to_be(5000)
  end)
end
