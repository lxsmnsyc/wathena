-- The Desperado Mentor changes a Gunslinger at job level 40 into a
-- Desperado. player:change_job checks the job's changes_from, so the
-- mentor needs no permission.

local events = require "wa.events"
local jobs = require "wa.jobs"

local DESPERADO = jobs.id "example.desperado:Desperado"

return function()
  events.on("click", function(player, conversation)
    if player.job == DESPERADO then
      conversation:say("[Mentor]", "Keep your hand steady, Desperado.")
      return
    end
    if player.job ~= jobs.gunslinger then
      conversation:say("[Mentor]", "I teach Gunslingers only.")
      return
    end
    conversation:say("[Mentor]", "Do you want to become a Desperado?")
    conversation:wait_for_next()
    if conversation:choose("Yes", "Not yet") ~= 1 then
      return
    end
    if player:change_job(DESPERADO) then
      conversation:say("[Mentor]", "You are now a Desperado.")
    else
      conversation:say("[Mentor]", "Come back at job level 40.")
    end
  end)
end
