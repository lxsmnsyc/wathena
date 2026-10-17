-- The Kafra storage of procedural towns (decision 0044). It stands at
-- each town's storage place and runs core's converted Kafra functions,
-- as a classic town's Kafra does: F_Kafra's storage menu, and
-- F_KafStor's rules, with Basic Skill level 6, the Free Ticket for Kafra
-- Storage, and the fee.

local events = require "wa.events"
local context = require "wa.context"
local towns = require "wa.towns"
local R = require "core.lib.rathena"

-- fee is the storage fee in zeny, by the town's band: 40 below level
-- 20, as in Prontera, and 20 more for each 20 levels, up to 100.
local function fee(player)
  local town = towns.nearest(player)
  if town == nil then
    return 40
  end
  local lowest = town:get_band()
  return math.min(40 + 20 * (lowest // 20), 100)
end

return function()
  local npc = context:get_instance()
  events.on("click", function(player, conversation)
    R.run(function()
      conversation:show_image("kafra_06", "right")
      require("core.functions.F_KafSet")(player, conversation, npc)
      conversation:say("[Kafra Employee]", "Welcome to the", "Kafra Corporation~", "The Kafra Services are", "always here to support", "you. So how can I be", "of service today?")
      -- F_Kafra's welcome 5 shows no message of its own, and menu 2 is
      -- storage only: the inn keeps the save point.
      require("core.functions.F_Kafra")(player, conversation, npc, 5, 2, 0, fee(player), 0)
    end)
  end)
end
