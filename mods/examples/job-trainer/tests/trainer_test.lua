-- Tests of the trainer and of core's Job Master, which wactl mod test
-- runs. Each case gets a fresh world with this mod and core loaded.

local testing = require "wa.testing"
local jobs = require "wa.jobs"
local skills = require "wa.skills"

local BASIC = skills.id "NV_BASIC"
local BASH = skills.id "SM_BASH"
local MAGNUM = skills.id "SM_MAGNUM"

-- A player next to the Job Master and the trainer.
local function new_player(world, spec)
  spec.map, spec.x, spec.y = "prontera", 156, 191
  return world:new_player(spec)
end

return function()
  testing.case("a Novice at job level 10 becomes a Swordman", function(world)
    local player = new_player(world, { job_level = 10, skills = { { BASIC, 9 } } })
    local said = world:talk(player, "core:Job Master#prontera", { 1 })
    testing.expect(said).to_contain("You are now a Swordman.")
    testing.expect(player.job).to_be(jobs.swordman)
    testing.expect(player.job_level).to_be(1)
    testing.expect(player:get_skill_level(BASIC)).to_be(9)
  end)

  testing.case("a Novice without Basic Skill 9 waits", function(world)
    local player = new_player(world, { job_level = 10 })
    local said = world:talk(player, "core:Job Master#prontera")
    testing.expect(said).to_contain("You need job level 10 and Basic Skill at level 9.")
    testing.expect(player.job).to_be(jobs.novice)
  end)

  testing.case("a Swordman at job level 40 becomes a Knight", function(world)
    local player = new_player(world, { job = jobs.swordman, base_level = 50, job_level = 40 })
    world:talk(player, "core:Job Master#prontera", { 1 })
    testing.expect(player.job).to_be(jobs.knight)
    testing.expect(jobs.next_jobs(jobs.swordman)).to_equal({ jobs.knight, jobs.crusader })
  end)

  testing.case("a female Archer becomes a Dancer", function(world)
    local player = new_player(world, { job = jobs.archer, sex = "female", base_level = 50, job_level = 40 })
    -- Hunter, then Dancer: a woman is not offered the Bard.
    world:talk(player, "core:Job Master#prontera", { 2 })
    testing.expect(player.job).to_be(jobs.dancer)
  end)

  testing.case("change_job refuses a job that is not next", function(world)
    local player = new_player(world, { job = jobs.swordman, job_level = 40 })
    testing.expect(player:change_job(jobs.wizard)).to_be_false()
    testing.expect(player.job).to_be(jobs.swordman)
  end)

  testing.case("a Knight at 99/50 is reborn and keeps its line", function(world)
    local player = new_player(world, { job = jobs.knight, base_level = 99, job_level = 50 })
    world:talk(player, "core:Job Master#prontera", { 1 })
    testing.expect(player.job).to_be(jobs.novice_high)
    testing.expect(player.base_level).to_be(1)
    testing.expect(player:change_job(jobs.mage_high)).to_be_false()
    testing.expect(player:change_job(jobs.swordman_high)).to_be_true()
    testing.expect(player:change_job(jobs.paladin)).to_be_false()
    testing.expect(player:change_job(jobs.lord_knight)).to_be_true()
  end)

  testing.case("reset_skill keeps a skill that another needs", function(world)
    local player = new_player(world, { job = jobs.swordman, skills = { { BASH, 10 }, { MAGNUM, 3 } } })
    testing.expect(player:reset_skill(BASH, { to_level = 4 })).to_be_false()
    testing.expect(player:get_skill_level(BASH)).to_be(10)
    testing.expect(player:reset_skill(BASH, { to_level = 5 })).to_be_true()
    testing.expect(player:get_skill_level(BASH)).to_be(5)
  end)

  testing.case("the trainer makes a Swordman forget Bash and Magnum Break", function(world)
    local player = new_player(world, { job = jobs.swordman, zeny = 1000, skills = { { BASH, 10 }, { MAGNUM, 3 } } })
    -- Forget a skill, SM_BASH, then agree to forget Magnum Break too.
    local said = world:talk(player, "Trainer#prontera", { 1, 1, 1 })
    testing.expect(said).to_contain("Other skills need this one.")
    testing.expect(player:get_skill_level(BASH)).to_be(0)
    testing.expect(player:get_skill_level(MAGNUM)).to_be(0)
    testing.expect(player.zeny).to_be(500)
  end)

  testing.case("the trainer takes no fee when nothing changes", function(world)
    local player = new_player(world, { job = jobs.swordman, zeny = 1000, skills = { { BASH, 10 }, { MAGNUM, 3 } } })
    world:talk(player, "Trainer#prontera", { 1, 1, 2 })
    testing.expect(player:get_skill_level(BASH)).to_be(10)
    testing.expect(player.zeny).to_be(1000)
  end)

  testing.case("the trainer lowers STR", function(world)
    local player = new_player(world, { job = jobs.swordman, zeny = 600, stats = { str = 50 } })
    world:talk(player, "Trainer#prontera", { 2, 1 })
    testing.expect(player.base_str).to_be(1)
    testing.expect(player.zeny).to_be(100)
  end)
end
