-- The Town Crier: an #events channel, a chat hook and a chat event.

local channels = require "wa.channels"
local events = require "wa.events"
local hooks = require "wa.hooks"

return function()
  -- The channel is one for the whole world. Every zone's context of
  -- this mod shares it, and it ends when the mod's last context ends.
  local crier = channels.create { name = "events", display_name = "[Events]", color = 0xFFA500, join = "invite" }

  -- The hook sees each line after the server's chat filter. It returns
  -- the text to send, or nil to stop the line. Hooks only compute: they
  -- cannot send messages.
  hooks.on("chat", function(message, next)
    local text = next(message)
    if text == nil then
      return nil
    end
    if text:find("https?://") then
      return nil
    end
    return (text:gsub("%f[%a]lol%f[%A]", "(laughs)"))
  end)

  -- The event runs after a line went out. "!events" in local chat puts
  -- the player in the channel, which only takes players the mod adds.
  events.on("chat", function(message)
    if message.text ~= "!events" then
      return
    end
    if crier:add(message.player) then
      message.player:whisper("Welcome to #events. Say a line there with #events.", { from = "Town Crier" })
      crier:send(message.player.name .. " joined the crowd.")
    end
  end, { channel = "public" })

  -- Staff planned a restart, or cancelled it. The crier tells the
  -- channel, so its members can finish what they do.
  events.on("restart_planned", function(restart)
    if restart.cancelled then
      crier:send("The planned restart is off.")
      return
    end
    crier:send("A restart is planned: " .. restart.reason .. ". Finish what you are doing.")
  end)
end
