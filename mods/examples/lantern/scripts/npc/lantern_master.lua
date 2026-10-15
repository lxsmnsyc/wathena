-- The Lantern Master changes a Merchant at job level 40 into a Lantern
-- Keeper, and gives a Lantern Keeper a Lantern Wisp. player:change_job
-- checks the job's changes_from, and player:set_companion the wisp's
-- requires, so the master needs no permission beyond game.read for the
-- IDs.

local companions = require "wa.companions"
local events = require "wa.events"
local jobs = require "wa.jobs"

local KEEPER = jobs.id "example.lantern:Lantern_Keeper"
local WISP = companions.id "example.lantern:Lantern_Wisp"

return function()
  events.on("click", function(player, conversation)
    if player.job == KEEPER then
      -- A Lantern Keeper gets a Lantern Wisp, whose sprite is the mod's
      -- art. A second click sends it home.
      if player.companion == WISP then
        player:set_companion(nil)
        conversation:say("[Lantern Master]", "Your wisp rests now. Keep the lanterns lit.")
      elseif player:set_companion(WISP) then
        conversation:say("[Lantern Master]", "Take this wisp. It lights your way.")
      else
        conversation:say("[Lantern Master]", "Keep the lanterns lit, Lantern Keeper.")
      end
      return
    end
    conversation:say("[Lantern Master]", "Do you want to become a Lantern Keeper?")
    conversation:wait_for_next()
    if conversation:choose("Yes", "Not yet") ~= 1 then
      return
    end
    if player:change_job(KEEPER) then
      conversation:say("[Lantern Master]", "You are now a Lantern Keeper.")
    else
      conversation:say("[Lantern Master]", "Come back as a Merchant at job level 40.")
    end
  end)
end
