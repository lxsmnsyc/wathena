-- The Shrine Keeper stands at every place tagged shrine, which the
-- generation layer put in each town.

local events = require "wa.events"
local places = require "wa.places"

return function()
  events.on("click", function(player, conversation)
    local shrine = places.nearest(player, "shrine")
    local stones = places.nearest(player, "stone_ring")
    local where = "nowhere near"
    if stones ~= nil then
      where = "out on " .. stones:get_map()
    end
    conversation:say("[Shrine Keeper]", "This shrine stands at " .. shrine:get_map() .. ".", "The nearest ring of stones is " .. where .. ".")
  end)
end
