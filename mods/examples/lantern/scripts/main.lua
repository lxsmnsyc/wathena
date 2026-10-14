-- The mod's entry script runs once in every zone. On prontera and on the
-- placeholder map it places Lantern Ghosts, which come back after they
-- are killed.

local context = require "wa.context"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"

local GHOST = monsters.id "example.lantern:LANTERN_GHOST"

local MAPS = { prontera = true, ["placeholder/map"] = true }

return function()
  if MAPS[context:get_instance().map] then
    spawning.monsters { monster = GHOST, amount = 3, respawn = true }
  end
end
