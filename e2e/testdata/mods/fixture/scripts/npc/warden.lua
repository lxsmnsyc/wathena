-- Mutes the player who asks, for one minute, for the mute scenario.

local events = require "wa.events"

return function()
  events.on("click", function(player, conversation)
    conversation:say("[Warden]", "Quiet, please.")
    if conversation:choose("Mute me", "Leave") == 1 then
      player:mute(1, "e2e mute")
      conversation:say("You are muted for a minute.")
    end
    conversation:close()
  end)
end
