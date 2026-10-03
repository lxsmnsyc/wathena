-- The Guide sends the player to the middle of the other map.

local events = require "wa.events"

return function()
  events.on("click", function(player, conversation)
    if conversation:choose("Go to the field", "Stay") == 1 then
      player:warp("e2e_field", 12, 12)
    end
  end)
end
