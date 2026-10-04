-- The Kafra opens the account's storage or the fixture's Vault.

local events = require "wa.events"

return function()
  events.on("click", function(player, conversation)
    local choice = conversation:choose("Storage", "Vault", "Leave")
    if choice == 1 then
      player:open_storage()
    elseif choice == 2 then
      player:open_storage("Vault")
    end
  end)
end
