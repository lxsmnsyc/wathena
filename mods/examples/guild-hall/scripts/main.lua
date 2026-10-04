-- The Guild Hall: guilds made from chat, and the guild events.

local events = require "wa.events"
local guilds = require "wa.guilds"
local rules = require "wa.rules"

local function say(player, text)
  player:whisper(text, { from = "Guild Hall" })
end

return function()
  events.on("chat", function(message)
    local player = message.player
    local text = message.text
    -- "!guild size" tells how many members a guild holds before Guild
    -- Extension in this world's ruleset.
    if text == "!guild size" then
      say(player, "A guild holds " .. rules.limits.guild_members .. " members, and 6 more for each level of Guild Extension.")
      return
    end
    -- "!guild" makes a guild with the player as its master. The mod
    -- asks no Emperium: guilds.create leaves the cost to the script.
    if text == "!guild" then
      if player.guild ~= nil then
        say(player, "You are in " .. player.guild.name .. " already.")
        return
      end
      if guilds.create(player, player.name .. "'s Guild") == nil then
        say(player, "That guild name is taken.")
      end
      return
    end
    -- "!notice <text>" sets the guild notice, for the master only.
    local notice = text:match("^!notice (.+)$")
    if notice ~= nil then
      local guild = player.guild
      if guild == nil or guild.master_name ~= player.name then
        say(player, "Only a guild master sets the notice.")
        return
      end
      if not guild:set_notice("From the master", notice) then
        say(player, "That notice is too long.")
      end
    end
  end, { channel = "public" })

  -- guild_join and guild_leave run on the map of the player who joined
  -- or left.
  events.on("guild_join", function(join)
    -- The player may have left again before the event ran.
    local position = join.guild:get_position(join.player)
    if position == nil then
      return
    end
    say(join.player, "Welcome to " .. join.guild.name .. ", " .. position.name .. ". " .. join.guild.master_name .. " is the master.")
  end)
  events.on("guild_leave", function(leave)
    say(leave.player, "You left " .. leave.guild.name .. " (" .. leave.reason .. ").")
  end)

  -- guild_change runs on the map where the change was made. The members
  -- here hear about it.
  events.on("guild_change", function(change)
    local guild = change.guild
    local text = guild.name .. ": "
    if change.change == "level" then
      text = text .. "level " .. guild.level .. "."
    elseif change.change == "notice" then
      text = text .. "notice: " .. guild.notice.text
    elseif change.change == "master" then
      text = text .. guild.master_name .. " is the new master."
    else
      text = text .. "a new emblem."
    end
    for _, member in ipairs(guild:get_members()) do
      member:send_message(text)
    end
  end)
end
