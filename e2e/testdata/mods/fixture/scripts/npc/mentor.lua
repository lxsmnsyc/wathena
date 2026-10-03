-- The Mentor changes a Swordman at job level 40 into a Knight.

local events = require "wa.events"
local jobs = require "wa.jobs"

return function()
  events.on("click", function(player, conversation)
    if player:change_job(jobs.knight) then
      conversation:say("[Mentor]", "You are a Knight now.")
    else
      conversation:say("[Mentor]", "Come back as a Swordman at job level 40.")
    end
  end)
end
