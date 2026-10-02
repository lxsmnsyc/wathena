-- One run of the Pumpkin_King event: the boss appears on a random cell
-- within 20 cells of the field's center, with a world announcement, and
-- its death is announced.
-- The run owns the boss: when the hour ends, a boss still alive goes.

local events = require "wa.events"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"
local text = require "wa.text"
local world = require "wa.world"

local PUMPKIN_KING = monsters.id "example.halloween:PUMPKIN_KING"

return function()
  local king = spawning.monsters { monster = PUMPKIN_KING, x = 200, y = 200, area = 20 }[1]
  world.announce(text "king.risen")
  events.on("death", function(death)
    local who = death.killer and death.killer.name or "Someone"
    world.announce(who .. " defeated the Pumpkin King at " .. death.x .. "," .. death.y .. "!")
  end, { victim = king })
end
