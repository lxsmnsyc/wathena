-- The rebirth NPC of procedural worlds (decision 0044, strategy section
-- 11.4). It stands in the largest town of the highest band and follows
-- the Valkyrie's rules (npc/jobs/valkyrie.txt):
--   - A transcendent or baby job cannot be reborn again.
--   - A second class at base level 99 and job level 50 is reborn, as
--     player:rebirth, when it carries no items and no zeny, has no cart,
--     falcon or mount, and has no skill points left.
--   - The reborn High Novice gets a Knife and a Cotton Shirt.

local events = require "wa.events"
local jobs = require "wa.jobs"
local effects = require "wa.effects"
local internal = require "wa.internal"

local NAME = "[Valkyrie]"
local KNIFE, COTTON_SHIRT = 1202, 2302

-- second_class reports whether a job is a second class that may be
-- reborn: neither a Novice, a first job, transcendent, baby nor third.
local function second_class(job)
  return job ~= jobs.novice and job ~= jobs.super_novice and jobs.first_job(job) ~= job
    and not jobs.is_transcendent(job) and not jobs.is_baby(job) and not jobs.is_third(job)
end

return function()
  events.on("click", function(player, conversation)
    local job = player.job
    if jobs.is_transcendent(job) or jobs.is_baby(job) then
      conversation:say(NAME, "Your soul has walked this road already.")
      return
    end
    if not second_class(job) or player.base_level < 99 or player.job_level < 50 then
      conversation:say(NAME, "Come back as a second class", "at base level 99 and job level 50.")
      return
    end
    if internal.weight(player) > 0 or player.zeny > 0 or player.cart ~= nil or player.companion ~= nil or player.mount ~= nil then
      conversation:say(NAME, "You cannot bring anything into your new life.", "Leave your items, your zeny, your cart,",
        "your falcon and your mount behind.")
      return
    end
    if internal.skill_points(player) > 0 then
      conversation:say(NAME, "Use all your skill points first.")
      return
    end
    conversation:say(NAME, "You have mastered your job.", "Do you want to be reborn as a High Novice?")
    conversation:wait_for_next()
    if conversation:choose("Be reborn", "Not now") ~= 1 then
      return
    end
    if not player:rebirth() then
      conversation:say(NAME, "You cannot be reborn now.")
      return
    end
    player:give_item(KNIFE, 1)
    player:give_item(COTTON_SHIRT, 1)
    player:show_effect(effects.angel)
    conversation:say(NAME, "Welcome to your new life.")
  end)
end
