-- A waiting room (proposal 0006, section 2) for the chat room scenario:
-- it takes 3 players, and when the second joins, its event warps the room's
-- members to the field and takes 10 zeny from each.

local context = require "wa.context"
local events = require "wa.events"

return function()
  local npc = context:get_instance()
  npc:open_chat_room {
    title = "E2E Arena",
    limit = 4,
    event = "e2e.fixture:Arena#e2e::OnStart",
    trigger = 2,
    zeny = 10,
  }
  events.on("npc_event", function()
    npc:chat_room():warp_members("e2e_field", 5, 5)
  end, { name = "OnStart" })
end
