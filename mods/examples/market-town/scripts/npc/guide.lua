-- The Town Guide stands on every town square. It finds the nearest
-- blacksmith with wa.places, counts the towns with a lamp shop, and
-- tells the latest rotation and world change it heard of.

local events = require "wa.events"
local places = require "wa.places"
local context = require "wa.context"

return function()
  local npc = context:get_instance()
  local news = {}

  -- A rotating dungeon got new floors, or a region changed. Every zone
  -- hears it, after the players there moved to the nearest town.
  events.on("dungeon_rotated", function(rotation)
    news[#news + 1] = "The " .. rotation.dungeon .. " has new floors since " .. rotation.date .. "."
  end)
  events.on("region_changed", function(change)
    news[#news + 1] = "The fields of " .. change.region .. " changed with season " .. change.season .. "."
  end)

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
    for _, line in ipairs(news) do
      conversation:say(line)
    end
  end)
end
