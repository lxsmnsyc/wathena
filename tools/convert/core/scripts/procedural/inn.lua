-- The inn of procedural towns (decision 0044). It stands at each town's
-- inn place and runs core's converted F_InnMaid, as rAthena's inns do:
-- Save sets the save point at the player's cell, and a rest for 5,000
-- zeny heals HP and SP fully.

local events = require "wa.events"
local context = require "wa.context"
local towns = require "wa.towns"
local R = require "core.lib.rathena"

return function()
  local npc = context:get_instance()
  events.on("click", function(player, conversation)
    local town = towns.nearest(player)
    local name = town ~= nil and (town:get_name() .. " Inn") or "the inn"
    R.run(require("core.functions.F_InnMaid"), player, conversation, npc, "[Innkeeper]", name, player.map, player.x, player.y)
  end)
end
