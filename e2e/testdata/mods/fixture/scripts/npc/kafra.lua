-- The Kafra opens the account's storage, the fixture's Vault, or the
-- player's guild storage.

local events = require "wa.events"

return function()
  events.on("click", function(player, conversation)
    local choice = conversation:choose("Storage", "Vault", "Guild storage", "Leave")
    if choice == 1 then
      player:open_storage()
    elseif choice == 2 then
      player:open_storage("Vault")
    elseif choice == 3 then
      local ok, why = player:open_guild_storage()
      if not ok then
        conversation:say("[Kafra]", "The guild storage cannot open: " .. why .. ".")
      end
    end
  end)
end
