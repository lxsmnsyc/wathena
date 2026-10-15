-- The Lantern Master changes a Merchant at job level 40 into a Lantern
-- Keeper. player:change_job checks the job's changes_from, so the master
-- needs no permission.

local events = require "wa.events"
local jobs = require "wa.jobs"

local KEEPER = jobs.id "example.lantern:Lantern_Keeper"

return function()
  events.on("click", function(player, conversation)
    if player.job == KEEPER then
      conversation:say("[Lantern Master]", "Keep the lanterns lit, Lantern Keeper.")
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
