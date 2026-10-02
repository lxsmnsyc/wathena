-- The mod script runs once in every zone. On prt_fild08 it places the
-- Pumpkin Porings. respawn = true makes them a normal spawn: a dead one
-- comes back while the mod runs.

local context = require "wa.context"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"

local PUMPKIN_PORING = monsters.id "example.halloween:PUMPKIN_PORING"

return function()
  if context:get_instance().map ~= "prt_fild08" then
    return
  end
  spawning.monsters { monster = PUMPKIN_PORING, amount = 15, respawn = true }
end
