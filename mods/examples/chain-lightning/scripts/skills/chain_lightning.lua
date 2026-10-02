-- Chain Lightning's entry script. Its setup function runs once per zone,
-- in the skill's context.
--
-- A cast hits the target. Then the bolt jumps every 150 milliseconds to
-- the nearest enemy of the caster within 5 cells of the last target that
-- it has not hit yet. It jumps up to level + 1 times. It stops when no
-- enemy is near, or when deal_damage returns nil: the caster is gone or
-- cannot act.

local events = require "wa.events"
local maps = require "wa.maps"
local timers = require "wa.timers"

local JUMP_MILLISECONDS = 150
local JUMP_CELLS = 5

-- percent is the damage percentage of a hit: 100% plus 20% per level.
local function percent(level)
  return 100 + 20 * level
end

return function()
  events.on("cast", function(cast)
    local target = cast.target
    -- Read the cell before the hit, which may kill the target.
    local x, y = target.x, target.y
    if cast:deal_damage(target, { percent = percent(cast.level) }) == nil then
      return
    end
    local hit = { target }
    local jumps = cast.level + 1

    local function jump()
      local next_target = maps.find_entities {
        x = x, y = y, radius = JUMP_CELLS,
        enemies_of = cast.caster, exclude = hit, sort = "nearest", limit = 1,
      }[1]
      if next_target == nil then
        return
      end
      x, y = next_target.x, next_target.y
      if cast:deal_damage(next_target, { percent = percent(cast.level) }) == nil then
        return
      end
      hit[#hit + 1] = next_target
      jumps = jumps - 1
      if jumps > 0 then
        timers.after(JUMP_MILLISECONDS, jump)
      end
    end

    timers.after(JUMP_MILLISECONDS, jump)
  end)
end
