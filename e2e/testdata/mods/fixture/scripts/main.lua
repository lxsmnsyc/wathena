-- Three Dummies on the start map, at fixed cells, that come back a second
-- after they die.

local context = require "wa.context"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"

local DUMMY = monsters.id "e2e.fixture:DUMMY"

return function()
  if context:get_instance().map ~= "placeholder/map" then
    return
  end
  for _, x in ipairs({ 9, 12, 15 }) do
    spawning.monsters { monster = DUMMY, amount = 1, x = x, y = 15, respawn = true, respawn_milliseconds = 1000 }
  end
end
