-- Three Dummies on the start map, at fixed cells, that come back a second
-- after they die. On every map, "!guild level <n>" said in local chat
-- gives the speaker's guild the exp that takes it to level n, so the
-- tests reach the guild skills without thousands of kills.

local context = require "wa.context"
local events = require "wa.events"
local guilds = require "wa.guilds"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"

local DUMMY = monsters.id "e2e.fixture:DUMMY"

-- next_exp is renewal's exp_guild.yml: level n needs n * n * 100000 exp
-- to reach the next one. The tests run in renewal.
local function next_exp(level)
  return level * level * 100000
end

return function()
  events.on("chat", function(message)
    local target = tonumber(message.text:match("^!guild level (%d+)$"))
    local guild = message.player.guild
    if target == nil or guild == nil then
      return
    end
    -- guilds.get reads the guild again, as a test of wa.guilds.
    guild = guilds.get(guild.id)
    local need = -guild.exp
    for level = guild.level, target - 1 do
      need = need + next_exp(level)
    end
    if need > 0 then
      guild:give_exp(need)
    end
  end, { channel = "public" })

  if context:get_instance().map ~= "placeholder/map" then
    return
  end
  for _, x in ipairs({ 9, 12, 15 }) do
    spawning.monsters { monster = DUMMY, amount = 1, x = x, y = 15, respawn = true, respawn_milliseconds = 1000 }
  end
end
