-- Tests of the lantern mod, which wactl mod test runs. The art is not
-- part of a test world: these tests check the records the art belongs to.

local testing = require "wa.testing"
local items = require "wa.items"
local jobs = require "wa.jobs"
local monsters = require "wa.monsters"
local skills = require "wa.skills"

local LANTERN = items.id "example.lantern:Paper_Lantern"
local UNLIT = items.id "example.lantern:Unlit_Lantern"
local HAT = items.id "example.lantern:Lantern_Hat"
local GHOST = monsters.id "example.lantern:LANTERN_GHOST"
local KEEPER = jobs.id "example.lantern:Lantern_Keeper"
local FLARE = skills.id "example.lantern:LANTERN_FLARE"

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

  testing.case("a Merchant at job level 40 becomes a Lantern Keeper", function(world)
    testing.expect(KEEPER >= 10000).to_be_true()
    testing.expect(FLARE >= 20000).to_be_true()
    local player = world:new_player { map = "prontera", x = 153, y = 191, job = jobs.merchant, base_level = 50, job_level = 40 }
    local said = world:talk(player, "Lantern_Master#prontera", { 1 })
    testing.expect(said).to_contain("You are now a Lantern Keeper.")
    testing.expect(player.job).to_be(KEEPER)
  end)

  testing.case("a Lantern Keeper knows Lantern Flare", function(world)
    local player = world:new_player {
      map = "prontera", x = 150, y = 191, job = KEEPER, base_level = 50, job_level = 10,
      skills = { { FLARE, 3 } },
    }
    testing.expect(player:get_skill_level(FLARE)).to_be(3)
  end)

  testing.case("the Lantern Hat goes on the head", function(world)
    local player = world:new_player { map = "prontera", x = 150, y = 191 }
    local hat = world:equip(player, HAT)
    testing.expect(hat.item).to_be(HAT)
  end)
end
