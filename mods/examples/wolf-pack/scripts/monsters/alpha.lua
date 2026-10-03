-- The AI script of the Alpha Wolf. Its setup runs once for each Alpha
-- Wolf that spawns, in a context whose instance is that wolf. The
-- context ends when the wolf dies, with its timer.

local context = require "wa.context"
local timers = require "wa.timers"
local monsters = require "wa.monsters"

local PACK = monsters.id "example.wolf-pack:PACK_WOLF"

return function()
  local alpha = context:get_instance()
  -- Once a second, the wolves of its pack that have nothing to do go
  -- after the alpha's target.
  timers.every(1000, function()
    local prey = alpha.target
    if prey == nil then
      return
    end
    for _, wolf in ipairs(alpha:find_nearby { radius = 14, kinds = { "monster" } }) do
      if wolf.id == PACK and wolf.master == alpha and wolf.target == nil then
        wolf:set_target(prey)
      end
    end
  end)
end
