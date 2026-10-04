-- The Pumpkin Witch gives each character one Pumpkin Hat. A character
-- variable remembers who got one.

local events = require "wa.events"
local items = require "wa.items"
local text = require "wa.text"
local variables = require "wa.variables"
local emotions = require "wa.emotions"
local context = require "wa.context"
local world_events = require "wa.world_events"

local HAT = items.id "example.halloween:Pumpkin_Hat"
local got_hat = variables.character_boolean "got_hat"

return function()
  local npc = context:get_instance()
  events.on("click", function(player, conversation)
    conversation:say("[" .. npc.name .. "]", text "witch.hello")
    conversation:wait_for_next()
    local choice = conversation:choose("Trick or treat!", "Not now", "Hide some candy")
    if choice == 3 then
      -- world_events.start needs events.control. The hunt starts within
      -- a second.
      if world_events.start("Candy_Hunt") then
        conversation:say(text "witch.hunt")
      else
        conversation:say(text "witch.hunting")
      end
      return
    end
    if choice ~= 1 then
      return conversation:close()
    end
    if got_hat:get(player) then
      conversation:say(text "witch.again")
      return
    end
    if not player:can_carry(HAT, 1) or not player:give_item(HAT, 1) then
      conversation:say(text "witch.full")
      return
    end
    got_hat:set(player, true)
    npc:show_emotion(emotions.thanks)
    conversation:say(text "witch.gift", "You got a " .. text.item_link(HAT) .. ".")
  end)
end
