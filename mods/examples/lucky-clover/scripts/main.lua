-- The mod's entry script runs once in every zone. It adds a hook on the
-- rolls of new Clover Charms, and on prt_fild09 it places Clover Porings.

local context = require "wa.context"
local hooks = require "wa.hooks"
local items = require "wa.items"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"

local CHARM = items.id "example.lucky-clover:Clover_Charm"
local CLOVER_PORING = monsters.id "example.lucky-clover:CLOVER_PORING"

return function()
  -- A charm that a Clover Poring drops never rolls a luck below 2. The
  -- server clamps what a hook returns to the roll's range.
  hooks.on("item_roll", function(roll, next)
    local value = next()
    if roll.item == CHARM and roll.name == "luck" and roll.source.kind == "drop"
        and roll.source.monster.id == CLOVER_PORING then
      return math.max(value, 2)
    end
    return value
  end)
  if context:get_instance().map == "prt_fild09" then
    spawning.monsters { monster = CLOVER_PORING, amount = 10, respawn = true }
  end
end
