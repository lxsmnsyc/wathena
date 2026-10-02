-- The Job Master: a job change NPC of core, written for wAthena, since
-- rAthena's job change quests do not convert yet (docs/generated,
-- "Job change NPCs"). It changes jobs when rAthena's level rules are met:
--   - A Novice or a High Novice at job level 10 with Basic Skill 9 becomes
--     a first class. A Super Novice also needs base level 45.
--   - A first class at job level 40 becomes a second class.
--   - A second class at base level 99 and job level 50 is reborn as a High
--     Novice (player:rebirth), and later climbs its own transcendent line.

local events = require "wa.events"
local jobs = require "wa.jobs"
local skills = require "wa.skills"
local effects = require "wa.effects"

-- The names players see for the jobs the Job Master offers.
local names = {
  [jobs.swordman] = "Swordman", [jobs.mage] = "Mage", [jobs.archer] = "Archer",
  [jobs.acolyte] = "Acolyte", [jobs.merchant] = "Merchant", [jobs.thief] = "Thief",
  [jobs.knight] = "Knight", [jobs.priest] = "Priest", [jobs.wizard] = "Wizard",
  [jobs.blacksmith] = "Blacksmith", [jobs.hunter] = "Hunter", [jobs.assassin] = "Assassin",
  [jobs.crusader] = "Crusader", [jobs.monk] = "Monk", [jobs.sage] = "Sage", [jobs.rogue] = "Rogue",
  [jobs.alchemist] = "Alchemist", [jobs.bard] = "Bard", [jobs.dancer] = "Dancer",
  [jobs.super_novice] = "Super Novice",
  [jobs.swordman_high] = "High Swordman", [jobs.mage_high] = "High Mage", [jobs.archer_high] = "High Archer",
  [jobs.acolyte_high] = "High Acolyte", [jobs.merchant_high] = "High Merchant", [jobs.thief_high] = "High Thief",
  [jobs.lord_knight] = "Lord Knight", [jobs.high_priest] = "High Priest", [jobs.high_wizard] = "High Wizard",
  [jobs.whitesmith] = "Whitesmith", [jobs.sniper] = "Sniper", [jobs.assassin_cross] = "Assassin Cross",
  [jobs.paladin] = "Paladin", [jobs.champion] = "Champion", [jobs.professor] = "Professor",
  [jobs.stalker] = "Stalker", [jobs.creator] = "Creator", [jobs.clown] = "Clown", [jobs.gypsy] = "Gypsy",
}

local BASIC_SKILL = skills.id "NV_BASIC"

-- is_first_class reports whether a job is a first class, transcendent or
-- not: its first job is itself and it is not a Novice.
local function is_first_class(job)
  return job ~= jobs.novice and job ~= jobs.novice_high and jobs.first_job(job) == job
end

-- offers lists the jobs the player may change into now, with the reason
-- when none.
local function offers(player)
  local job = player.job
  local next = jobs.next_jobs(job)
  if job == jobs.novice or job == jobs.novice_high then
    if player.job_level < 10 or player:get_skill_level(BASIC_SKILL) < 9 then
      return {}, "You need job level 10 and Basic Skill at level 9."
    end
    local out = {}
    for _, j in ipairs(next) do
      if j ~= jobs.super_novice or player.base_level >= 45 then
        out[#out + 1] = j
      end
    end
    return out
  end
  if is_first_class(job) then
    if player.job_level < 40 then
      return {}, "You need job level 40."
    end
    local out = {}
    for _, j in ipairs(next) do
      -- Bards are men and Dancers women, and the same for their
      -- transcendent jobs.
      local female_job = j == jobs.dancer or j == jobs.gypsy
      local male_job = j == jobs.bard or j == jobs.clown
      if not (female_job and player.sex == "male") and not (male_job and player.sex == "female") then
        out[#out + 1] = j
      end
    end
    return out
  end
  return {}, nil
end

-- can_be_reborn reports whether the player is a second class that the
-- rebirth rules accept.
local function can_be_reborn(player)
  return #jobs.next_jobs(player.job) == 0 and not jobs.is_transcendent(player.job)
    and not jobs.is_baby(player.job) and not jobs.is_third(player.job)
    and player.job ~= jobs.novice and player.job ~= jobs.super_novice
    and not is_first_class(player.job)
end

return function()
  events.on("click", function(player, conversation)
    conversation:say("[Job Master]", "I guide adventurers to their next job.")
    local list, why = offers(player)
    if #list > 0 then
      local labels = {}
      for i, j in ipairs(list) do
        labels[i] = "Become a " .. (names[j] or tostring(j))
      end
      labels[#labels + 1] = "Not now"
      conversation:wait_for_next()
      local choice = conversation:choose(table.unpack(labels))
      local job = list[choice]
      if job == nil then
        conversation:say("[Job Master]", "Come back when you are ready.")
        return
      end
      if player:change_job(job) then
        player:show_effect(effects.angel)
        conversation:say("[Job Master]", "You are now a " .. (names[job] or "new job") .. ". Good luck!")
      else
        conversation:say("[Job Master]", "That path is not yours to walk.")
      end
      return
    end
    if can_be_reborn(player) then
      if player.base_level < 99 or player.job_level < 50 then
        conversation:say("[Job Master]", "Reach base level 99 and job level 50,", "and I can give you a new life.")
        return
      end
      conversation:say("[Job Master]", "You have mastered your job.", "Do you want to be reborn as a High Novice?",
        "You go back to level 1, and keep your path.")
      conversation:wait_for_next()
      if conversation:choose("Be reborn", "Not now") == 1 then
        if player:rebirth() then
          player:show_effect(effects.angel)
          conversation:say("[Job Master]", "Welcome to your new life.")
        else
          conversation:say("[Job Master]", "You cannot be reborn now.")
        end
      end
      return
    end
    conversation:say("[Job Master]", why or "I have nothing more to teach you.")
  end)
end
