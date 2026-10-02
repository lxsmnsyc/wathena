-- The guide NPC. data/npcs.yml places it. Its setup function runs once
-- for each placement, and the click handler runs for each conversation.

local events = require "wa.events"
local variables = require "wa.variables"
local items = require "wa.items"
local text = require "wa.text"
local context = require "wa.context"

local BADGE = items.id "{%.ID%}:Guide_Badge"

-- Saved with the character, under this mod's ID.
local visits = variables.character_integer "visits"
local has_badge = variables.character_boolean "has_badge"

return function()
  local npc = context:get_instance()

  events.on("click", function(player, conversation)
    local count = visits:add(player, 1)
    conversation:say("[" .. npc.name .. "]", text "guide.hello", string.format(text "guide.visits", count))
    conversation:wait_for_next()
    local choice = conversation:choose(text "guide.ask_badge", text "guide.goodbye")
    if choice ~= 1 then
      conversation:say(text "guide.bye")
      return
    end
    if has_badge:get(player) then
      conversation:say(text "guide.already")
    elseif player:give_item(BADGE, 1) then
      has_badge:set(player, true)
      conversation:say(text "guide.given")
    else
      conversation:say(text "guide.full")
    end
  end)
end
