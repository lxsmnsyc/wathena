-- Verve's entry script. The setup function defines the status and
-- listens to its ticks.
--
-- While Verve lasts, a player has 100 more max HP per level. Every 5
-- seconds, the status_tick event heals 2% of max HP. Death and Dispell
-- end it.

local events = require "wa.events"
local statuses = require "wa.statuses"

-- EFST_FOOD_STR: an icon the client already has.
local ICON = 241

return function()
  statuses.define {
    name = "verve",
    icon = ICON,
    kind = "buff",
    ends_on = { "death", "dispell" },
    bonuses = function(level)
      return { { "max_hp", 100 * level } }
    end,
    tick_milliseconds = 5000,
  }

  events.on("status_tick", function(tick)
    tick.target:heal_percent { hp = 2 }
  end, { status = statuses.verve, target_kind = "player" })
end
