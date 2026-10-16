-- The Town Guide stands on every town square. It finds the nearest
-- blacksmith with wa.places, and counts the towns with a lamp shop.

local events = require "wa.events"
local places = require "wa.places"
local context = require "wa.context"

return function()
  local npc = context:get_instance()

  events.on("click", function(player, conversation)
    conversation:say("[" .. npc.name .. "]")
    local smith = places.nearest(player, "blacksmith")
    if smith == nil then
      conversation:say("I know of no blacksmith.")
    else
      local x, y = smith:get_position()
      conversation:say("The nearest blacksmith is in " .. smith:get_area() .. " at " .. x .. ", " .. y .. ".")
    end
    local shops = places.find("town:any", "shop:lamp")
    conversation:say(#shops .. " towns sell lamps.")
  end)
end
