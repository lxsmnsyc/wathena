-- The shops of procedural towns (decision 0044): a dealer opens core's
-- shop of its kind for the town's band. A town of a higher band sells
-- what lower towns sell and more. The shops are records of core's
-- data/npcs.yml without a map, named shop_<kind>_<tier>.

local events = require "wa.events"
local towns = require "wa.towns"

-- Tiers is how many shops each kind has: one per 20 levels of the
-- town's band, the last from level 80 up.
local TIERS = 5

-- tier is the shop tier of the player's town, by its band's lowest
-- level.
local function tier(player)
  local town = towns.nearest(player)
  if town == nil then
    return 1
  end
  local lowest = town:get_band()
  return math.min(lowest // 20 + 1, TIERS)
end

return function(kind)
  return function()
    events.on("click", function(player)
      player:open_shop("shop_" .. kind .. "_" .. tier(player))
    end)
  end
end
