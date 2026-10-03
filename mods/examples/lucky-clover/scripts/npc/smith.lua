-- The Clover Smith rerolls the luck of a worn Clover Charm for a fee. A
-- reroll draws a new value as a drop does, through the item_roll hooks,
-- so it needs items.modify_rolls.

local events = require "wa.events"
local items = require "wa.items"

local CHARM = items.id "example.lucky-clover:Clover_Charm"

-- The fee for one reroll, in zeny.
local FEE = 2000

-- worn_charm returns the Clover Charm the player wears, or nil.
local function worn_charm(player)
  for _, slot in ipairs({ "right_accessory", "left_accessory" }) do
    local copy = player:get_equipped(slot)
    if copy ~= nil and copy.item == CHARM then
      return copy
    end
  end
  return nil
end

return function()
  events.on("click", function(player, conversation)
    conversation:say("[Clover Smith]", "For " .. FEE .. " zeny, I roll the luck of a worn Clover Charm again.")
    conversation:wait_for_next()
    if conversation:choose("Reroll my charm", "Goodbye") ~= 1 then
      return
    end
    local charm = worn_charm(player)
    if charm == nil then
      conversation:say("[Clover Smith]", "Wear a Clover Charm first.")
      return
    end
    if not player:pay_zeny(FEE) then
      conversation:say("[Clover Smith]", "A reroll costs " .. FEE .. " zeny.")
      return
    end
    local before = charm.rolls.luck
    charm:reroll("luck")
    conversation:say("[Clover Smith]", "Its luck was " .. before .. ". Now it is " .. charm.rolls.luck .. ".")
  end)
end
