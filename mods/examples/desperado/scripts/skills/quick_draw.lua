-- Quick Draw's entry script. Its setup function runs once per zone, in
-- the skill's context. A cast fires two shots at the target, the second
-- one 200 milliseconds after the first.

local events = require "wa.events"
local timers = require "wa.timers"

local SECOND_SHOT_MILLISECONDS = 200

-- percent is the damage percentage of a shot: 100% plus 30% per level.
local function percent(level)
  return 100 + 30 * level
end

return function()
  events.on("cast", function(cast)
    local target = cast.target
    if cast:deal_damage(target, { percent = percent(cast.level) }) == nil then
      return
    end
    timers.after(SECOND_SHOT_MILLISECONDS, function()
      if target:exists() and target.is_alive then
        cast:deal_damage(target, { percent = percent(cast.level) })
      end
    end)
  end)
end
