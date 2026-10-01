-- The greeter's entry script. The code before the setup function runs
-- once per zone, when the file loads. The setup function runs once per
-- zone too, in the mod's context, which owns the handler and the timers.

local events = require "wa.events"
local timers = require "wa.timers"
local items = require "wa.items"
local messages = require "example.greeter.lib.messages"

local COOKIE = items.id "example.greeter:Welcome_Cookie"

return function()
  events.on("login", function(player)
    player:send_message(messages.welcome(player.name))
    -- The timer belongs to the mod's context, and follows the zone clock.
    timers.after(5000, function()
      player:send_message(messages.reminder(COOKIE))
    end)
  end)
end
