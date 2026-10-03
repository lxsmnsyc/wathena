-- The Desperado's entry script: each new Desperado gets a hat.

local events = require "wa.events"
local items = require "wa.items"
local jobs = require "wa.jobs"

local DESPERADO = jobs.id "example.desperado:Desperado"
local HAT = items.id "example.desperado:Desperado_Hat"

return function()
  events.on("job_change", function(change)
    if change.to == DESPERADO and change.player:can_carry(HAT, 1) then
      change.player:give_item(HAT, 1)
    end
  end)
end
