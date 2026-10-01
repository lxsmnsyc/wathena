-- The greeter NPC. Both placements in data/npcs.yml run this file: the
-- setup function runs once for each, and its locals belong to that NPC.
--
-- It shows the saved variables of wa.variables: a count per character,
-- a flag per account, and a count for the whole server. They are stored
-- under this mod's ID and come back after a reconnect.

local events = require "wa.events"
local variables = require "wa.variables"
local emotions = require "wa.emotions"
local items = require "wa.items"
local text = require "wa.text"
local context = require "wa.context"

local COOKIE = items.id "example.greeter:Welcome_Cookie"

local visits = variables.character_integer "visits"
local met_on_account = variables.account_boolean "met"
local greetings = variables.server_integer "greetings"

return function()
  local npc = context:get_instance()
  -- Per-NPC state is a local of the setup function.
  local clicks_here = 0

  events.on("click", function(player, conversation)
    clicks_here = clicks_here + 1
    local n = visits:add(player, 1)
    local total = greetings:add(1)
    conversation:show_image("kafra_01", "right")
    npc:show_emotion(emotions.thanks)
    conversation:say("[" .. npc.name .. "]", text "greeter.hello", "You have visited me " .. n .. " times.")
    if met_on_account:get(player) then
      conversation:say(text "greeter.account")
    end
    met_on_account:set(player, true)
    conversation:say("I have greeted " .. total .. " times, " .. clicks_here .. " of them here.")
    conversation:wait_for_next()
    local choice = conversation:choose("Buy a " .. text.item_link(COOKIE), "Shout a greeting", "Rest a moment", "Goodbye")
    if choice == 1 then
      conversation:close()
      player:open_shop("Cookie Stand")
    elseif choice == 2 then
      npc:say_aloud(text "greeter.shout")
      conversation:say(text "greeter.done")
    elseif choice == 3 then
      conversation:say("Take three seconds.")
      conversation:show_progress(3)
      conversation:say("Feeling better?")
    else
      conversation:say(text "greeter.bye")
    end
    conversation:hide_image()
  end)
end
