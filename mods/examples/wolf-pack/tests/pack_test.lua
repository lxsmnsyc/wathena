-- Tests of the Wolf Pack mod, which wactl mod test runs. world:advance
-- runs the monster AI, and the tests read each monster's target and
-- state.

local testing = require "wa.testing"
local jobs = require "wa.jobs"
local monsters = require "wa.monsters"

local ALPHA = monsters.id "example.wolf-pack:ALPHA_WOLF"
local PACK = monsters.id "example.wolf-pack:PACK_WOLF"

return function()
  testing.case("the alpha calls its pack", function(world)
    world:new_player { map = "prt_fild08", x = 150, y = 150, job = jobs.novice }
    local alpha = world:spawn_monster { monster = ALPHA, x = 158, y = 150 }
    world:advance(3000)
    local pack = world:get_monsters(PACK)
    testing.expect(#pack).to_be(2)
    for _, wolf in ipairs(pack) do
      testing.expect(wolf.master).to_be(alpha)
      testing.expect(wolf.name).to_be("Pack Wolf")
    end
  end)

  testing.case("the alpha hunts, and its pack follows its lead", function(world)
    -- A level 99 Swordman lives through the wolves' bites.
    local player = world:new_player { map = "prt_fild08", x = 150, y = 150, job = jobs.swordman, base_level = 99, job_level = 50 }
    local alpha = world:spawn_monster { monster = ALPHA, x = 158, y = 150 }
    world:advance(4000)
    testing.expect(alpha.target).to_be(player)
    testing.expect(alpha.state ~= "idle").to_be_true()
    local pack = world:get_monsters(PACK)
    testing.expect(#pack).to_be(2)
    for _, wolf in ipairs(pack) do
      testing.expect(wolf.target).to_be(player)
    end
  end)

  testing.case("the pack leaves novices alone", function(world)
    world:new_player { map = "prt_fild08", x = 150, y = 150, job = jobs.novice }
    local alpha = world:spawn_monster { monster = ALPHA, x = 156, y = 150 }
    world:advance(5000)
    testing.expect(alpha.target).to_be_nil()
    -- It stands or wanders.
    testing.expect(alpha.state == "idle" or alpha.state == "walk").to_be_true()
  end)

  testing.case("a script steers a wolf", function(world)
    local player = world:new_player { map = "prt_fild08", x = 150, y = 150, job = jobs.swordman, base_level = 99, job_level = 50 }
    local wolf = world:spawn_monster { monster = PACK, x = 160, y = 160 }
    -- A passive wolf ignores the player until told.
    world:advance(2000)
    testing.expect(wolf.target).to_be_nil()
    testing.expect(wolf:set_target(player)).to_be_true()
    testing.expect(wolf.target).to_be(player)
    wolf:stop()
    testing.expect(wolf.target).to_be_nil()
    testing.expect(wolf:walk_to(165, 160)).to_be_true()
    world:advance(3000)
    testing.expect(wolf.x).to_be(165)
    testing.expect(wolf.y).to_be(160)
    wolf:talk("Grr.")
  end)
end
