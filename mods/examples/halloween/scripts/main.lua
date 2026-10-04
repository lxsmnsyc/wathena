-- The mod script runs once in every zone. On prt_fild08 it places the
-- Pumpkin Porings. respawn = true makes them a normal spawn: a dead one
-- comes back while the mod runs. It also tells the field when a world
-- event starts or ends.

local context = require "wa.context"
local events = require "wa.events"
local maps = require "wa.maps"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"

local PUMPKIN_PORING = monsters.id "example.halloween:PUMPKIN_PORING"

return function()
  if context:get_instance().map ~= "prt_fild08" then
    return
  end
  spawning.monsters { monster = PUMPKIN_PORING, amount = 15, respawn = true }
  -- world_event_start and world_event_end run in every zone, for every
  -- world event.
  events.on("world_event_end", function(event)
    if event.name == "example.halloween:Headless_Rider" then
      maps.announce("prt_fild08", "The Headless Rider's night is over: " .. event.result .. ".")
    end
  end)
end
