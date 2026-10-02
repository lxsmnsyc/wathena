-- The Verve Tonic's use script: Verve at level 3 for one minute.
local events = require "wa.events"
local statuses = require "wa.statuses"

return function()
  events.on("use", function(player)
    player:start_status(statuses.verve, { milliseconds = 60000, level = 3 })
  end)
end
