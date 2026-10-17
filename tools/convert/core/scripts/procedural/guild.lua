-- The job guilds of procedural worlds (decision 0044, strategy section
-- 11.4): one per class family, at the guild:<family> place of the town
-- `wactl world new` chose. They follow rAthena's job change rules:
--   - A Novice at job level 10 with Basic Skill level 9 takes the test of
--     the first job, then changes.
--   - A first job at job level 40 with no skill points left takes the
--     test of the second jobs, then picks one of the two.
--   - A High Novice and a transcendent first job change without a test,
--     as rAthena's job NPCs let a reborn player, and player:change_job
--     keeps them in the line they had before their rebirth.
-- A test is the guild's quests of the stage, a hunt and a collect quest,
-- in order, and then a talk with the guild master.

local events = require "wa.events"
local jobs = require "wa.jobs"
local skills = require "wa.skills"
local effects = require "wa.effects"
local internal = require "wa.internal"
local Q = require "core.procedural.quests"

local BASIC_SKILL = skills.id "NV_BASIC"

-- The names players see.
local NAMES = {
  [jobs.swordman] = "Swordman", [jobs.mage] = "Mage", [jobs.archer] = "Archer",
  [jobs.acolyte] = "Acolyte", [jobs.merchant] = "Merchant", [jobs.thief] = "Thief",
  [jobs.knight] = "Knight", [jobs.crusader] = "Crusader", [jobs.wizard] = "Wizard", [jobs.sage] = "Sage",
  [jobs.hunter] = "Hunter", [jobs.bard] = "Bard", [jobs.dancer] = "Dancer",
  [jobs.priest] = "Priest", [jobs.monk] = "Monk", [jobs.blacksmith] = "Blacksmith", [jobs.alchemist] = "Alchemist",
  [jobs.assassin] = "Assassin", [jobs.rogue] = "Rogue",
  [jobs.swordman_high] = "High Swordman", [jobs.mage_high] = "High Mage", [jobs.archer_high] = "High Archer",
  [jobs.acolyte_high] = "High Acolyte", [jobs.merchant_high] = "High Merchant", [jobs.thief_high] = "High Thief",
  [jobs.lord_knight] = "Lord Knight", [jobs.paladin] = "Paladin", [jobs.high_wizard] = "High Wizard",
  [jobs.professor] = "Professor", [jobs.sniper] = "Sniper", [jobs.clown] = "Clown", [jobs.gypsy] = "Gypsy",
  [jobs.high_priest] = "High Priest", [jobs.champion] = "Champion", [jobs.whitesmith] = "Whitesmith",
  [jobs.creator] = "Creator", [jobs.assassin_cross] = "Assassin Cross", [jobs.stalker] = "Stalker",
}

-- fits reports whether a job suits the player's sex: Bards and Clowns
-- are men, Dancers and Gypsies women.
local function fits(player, job)
  local female = job == jobs.dancer or job == jobs.gypsy
  local male = job == jobs.bard or job == jobs.clown
  return not (female and player.sex == "male") and not (male and player.sex == "female")
end

return function(family)
  local first, high = jobs[family], jobs[family .. "_high"]
  local title = "[" .. NAMES[first] .. " Guild Master]"

  -- change changes the player into one of the jobs, after a choice when
  -- there are two.
  local function change(player, conversation, list)
    local options = {}
    for _, j in ipairs(list) do
      if fits(player, j) then
        options[#options + 1] = j
      end
    end
    if #options == 0 then
      conversation:say(title, "That path is not yours to walk.")
      return
    end
    local labels = {}
    for i, j in ipairs(options) do
      labels[i] = "Become a " .. (NAMES[j] or tostring(j))
    end
    labels[#labels + 1] = "Not now"
    conversation:wait_for_next()
    local job = options[conversation:choose(table.unpack(labels))]
    if job == nil then
      conversation:say(title, "Come back when you are ready.")
      return
    end
    if player:change_job(job) then
      player:show_effect(effects.angel)
      conversation:say(title, "You are now a " .. (NAMES[job] or "new job") .. ". Good luck!")
    else
      conversation:say(title, "That path is not yours to walk.")
    end
  end

  -- test runs the test of a stage. It returns true when every step is
  -- done, so the talk that ends it may follow.
  local function test(player, conversation, stage)
    local steps = Q.list(function(q)
      return q.purpose == "guild" and q.guild == family and q.stage == stage
    end)
    table.sort(steps, function(a, b)
      return a.step < b.step
    end)
    for i, q in ipairs(steps) do
      local state = player:get_quest_state(q.id)
      if state == "none" then
        conversation:say(title, "Your test, part " .. i .. " of " .. #steps .. ":", q.title .. ".")
        conversation:wait_for_next()
        if conversation:choose("Begin", "Not now") == 1 then
          player:add_quest(q.id)
          conversation:say(title, "Come back when it is done.")
        end
        return false
      end
      if state ~= "complete" then
        if not Q.done(player, q) or not Q.take(player, q) then
          conversation:say(title, q.title .. ":", Q.progress(player, q))
          return false
        end
        player:complete_quest(q.id)
        conversation:say(title, "Well done. That part of your test is passed.")
        conversation:wait_for_next()
      end
    end
    return true
  end

  return function()
    events.on("click", function(player, conversation)
      local job = player.job
      if job == jobs.novice or job == jobs.novice_high then
        if player.job_level < 10 or player:get_skill_level(BASIC_SKILL) < 9 then
          conversation:say(title, "To become a " .. NAMES[first] .. ", reach job level 10", "and learn Basic Skill to level 9.")
          return
        end
        if job == jobs.novice_high then
          conversation:say(title, "Your soul remembers this path.")
          change(player, conversation, { high })
          return
        end
        if test(player, conversation, "first") then
          conversation:say(title, "You passed the " .. NAMES[first] .. " test.")
          change(player, conversation, { first })
        end
        return
      end
      if job == first or job == high then
        if player.job_level < 40 then
          conversation:say(title, "Come back at job level 40.")
          return
        end
        if internal.skill_points(player) > 0 then
          conversation:say(title, "Use all your skill points first.")
          return
        end
        if job == high then
          change(player, conversation, jobs.next_jobs(high))
          return
        end
        if test(player, conversation, "second") then
          conversation:say(title, "You passed the test of our second jobs.")
          change(player, conversation, jobs.next_jobs(first))
        end
        return
      end
      conversation:say(title, "I train " .. NAMES[first] .. "s and their second jobs.")
    end)
  end
end
