-- The Quartermaster gives 5 Jellopy and 1,000 zeny, or takes 2 Jellopy.

local events = require "wa.events"
local items = require "wa.items"

local JELLOPY = items.id "Jellopy"

return function()
  events.on("click", function(player, conversation)
    local choice = conversation:choose("Give", "Take", "Leave")
    if choice == 1 then
      player:give_item(JELLOPY, 5)
      player:give_zeny(1000)
      conversation:say("[Quartermaster]", "Here you are.")
    elseif choice == 2 then
      if player:take_item(JELLOPY, 2) then
        conversation:say("[Quartermaster]", "Thank you.")
      else
        conversation:say("[Quartermaster]", "You have too few.")
      end
    end
  end)
end
