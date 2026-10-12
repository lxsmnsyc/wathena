-- A staff-only NPC (proposal 0002, review decision 33). It talks to
-- players with a staff role and turns everyone else away. A role counts
-- only when the player logged in with two-factor login, so a stolen
-- password alone does not open it.

local events = require "wa.events"

return function()
  events.on("click", function(player, conversation)
    local role = player:get_staff_role()
    if role == nil then
      conversation:say("[Staff Desk]", "This desk is for staff.")
      return
    end
    conversation:say("[Staff Desk]", "Welcome back. Your role is " .. role .. ".")
    if player:has_staff_permission("moderation.mute") then
      conversation:say("The report queue is on the admin screens.")
    end
    if player:has_staff_permission("staff.assign") then
      conversation:say("You can give roles to other staff.")
    end
  end)
end
