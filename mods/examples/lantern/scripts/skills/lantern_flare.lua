-- Lantern Flare's entry script. Its setup function runs once per zone, in
-- the skill's context. A cast burns the target for 150% of the caster's
-- magic attack, and 50% more for each level.

local events = require "wa.events"

return function()
  events.on("cast", function(cast)
    cast:deal_damage(cast.target, { percent = 100 + 50 * cast.level })
  end)
end
