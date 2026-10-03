-- The Familiar Keeper hands out the Owl and the Horse, and takes them
-- back. set_companion and set_mount return false when the player lacks
-- the skills of the record's requires.

local companions = require "wa.companions"
local events = require "wa.events"
local mounts = require "wa.mounts"

local OWL = companions.id "example.familiars:Owl"
local HORSE = mounts.id "example.familiars:Horse"

return function()
  events.on("click", function(player, conversation)
    conversation:say("[Keeper]", "Which familiar do you want?")
    conversation:wait_for_next()
    local choice = conversation:choose("An Owl", "A Horse", "Take mine back")
    if choice == 1 then
      if player:set_companion(OWL) then
        conversation:say("[Keeper]", "The Owl is yours.")
      else
        conversation:say("[Keeper]", "Learn Falconry Mastery first.")
      end
    elseif choice == 2 then
      if player:set_mount(HORSE) then
        conversation:say("[Keeper]", "The Horse is yours.")
      else
        conversation:say("[Keeper]", "Learn Riding first.")
      end
    elseif choice == 3 then
      player:set_companion(nil)
      player:set_mount(nil)
      conversation:say("[Keeper]", "I will look after them.")
    end
  end)
end
