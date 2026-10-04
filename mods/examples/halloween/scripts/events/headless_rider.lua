-- The script of the Headless_Rider boss event. Core places the rider,
-- announces it and gives the reward. The script gets the run with
-- context:get_event():
--   - when the rider dies, it thanks each participant;
--   - when the rider kills a player, the night is lost and the run ends.

local context = require "wa.context"
local events = require "wa.events"
local monsters = require "wa.monsters"
local world = require "wa.world"

local RIDER = monsters.id "example.halloween:HEADLESS_RIDER"

return function()
  local run = context:get_event()
  local rider = run:get_monsters()[1]
  events.on("death", function()
    for _, player in ipairs(run:get_participants()) do
      player:send_message("The village thanks you, " .. player.name .. ".")
    end
  end, { victim = rider })
  events.on("death", function(death)
    if death.killer ~= nil and death.killer.kind == "monster" and death.killer.id == RIDER then
      world.announce("The Headless Rider claimed a soul and rode into the night.")
      run:finish("lost")
    end
  end, { victim_kind = "player" })
end
