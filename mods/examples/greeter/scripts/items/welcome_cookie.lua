-- The Welcome Cookie's use script: it heals 20 to 30 HP, with the
-- bonuses of rAthena's itemheal.
local events = require "wa.events"

return function()
  events.on("use", function(player)
    player:item_heal { hp = { 20, 30 } }
  end)
end
