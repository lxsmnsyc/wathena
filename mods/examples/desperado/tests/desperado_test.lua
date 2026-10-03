-- Tests of the Desperado, which wactl mod test runs. Each case gets a
-- fresh world with this mod and core loaded.

local testing = require "wa.testing"
local items = require "wa.items"
local jobs = require "wa.jobs"
local skills = require "wa.skills"

local DESPERADO = jobs.id "example.desperado:Desperado"
local QUICK_DRAW = skills.id "example.desperado:QUICK_DRAW"
local SINGLE_ACTION = skills.id "GS_SINGLEACTION"
local HAT = items.id "example.desperado:Desperado_Hat"

-- A Gunslinger next to the mentor.
local function gunslinger(world, job_level)
  return world:new_player { map = "prontera", x = 150, y = 191, job = jobs.gunslinger, base_level = 60, job_level = job_level }
end

return function()
  testing.case("the job and the skill have IDs of the mod range", function()
    testing.expect(DESPERADO >= 10000).to_be_true()
    testing.expect(QUICK_DRAW >= 20000).to_be_true()
    testing.expect(jobs.next_jobs(jobs.gunslinger)).to_equal({ DESPERADO })
    testing.expect(jobs.is_transcendent(DESPERADO)).to_be_false()
    testing.expect(jobs.first_job(DESPERADO)).to_be(jobs.gunslinger)
    testing.expect(jobs.base_job(DESPERADO)).to_be(jobs.gunslinger)
  end)

  testing.case("a Gunslinger at job level 40 becomes a Desperado and gets a hat", function(world)
    local player = gunslinger(world, 40)
    local said = world:talk(player, "Mentor#prontera", { 1 })
    testing.expect(said).to_contain("You are now a Desperado.")
    testing.expect(player.job).to_be(DESPERADO)
    testing.expect(player.job_level).to_be(1)
    -- The job_change event gave the hat, which only a Desperado wears.
    testing.expect(player:count_item(HAT)).to_be(1)
    local hat = world:equip(player, HAT)
    testing.expect(hat.item).to_be(HAT)
  end)

  testing.case("a Gunslinger below job level 40 waits", function(world)
    local player = gunslinger(world, 39)
    local said = world:talk(player, "Mentor#prontera", { 1 })
    testing.expect(said).to_contain("Come back at job level 40.")
    testing.expect(player.job).to_be(jobs.gunslinger)
    testing.expect(player:count_item(HAT)).to_be(0)
  end)

  testing.case("change_job follows changes_from", function(world)
    local knight = world:new_player { map = "prontera", x = 150, y = 191, job = jobs.swordman, job_level = 40 }
    testing.expect(knight:change_job(DESPERADO)).to_be_false()
    local player = gunslinger(world, 45)
    testing.expect(player:change_job(DESPERADO)).to_be_true()
    testing.expect(player.job).to_be(DESPERADO)
    -- No way back without force.
    testing.expect(player:change_job(jobs.gunslinger)).to_be_false()
  end)

  testing.case("other jobs cannot wear the hat", function(world)
    local player = gunslinger(world, 40)
    testing.expect(function() world:equip(player, HAT) end).to_fail_with("refused")
  end)

  testing.case("a Desperado keeps its skills and learns Quick Draw", function(world)
    local player = world:new_player {
      map = "prontera", x = 150, y = 191, job = DESPERADO, base_level = 70, job_level = 20,
      skills = { { SINGLE_ACTION, 3 }, { QUICK_DRAW, 5 } },
    }
    testing.expect(player:get_skill_level(SINGLE_ACTION)).to_be(3)
    testing.expect(player:get_skill_level(QUICK_DRAW)).to_be(5)
  end)
end
