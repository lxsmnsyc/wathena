-- Owl Strike's entry script. A cast strikes the target for 150% damage.
-- The requirement in data/skills.yml checks the companion before the
-- cast starts.

local events = require "wa.events"

return function()
  events.on("cast", function(cast)
    cast:deal_damage(cast.target, { percent = 150 })
  end)
end
