-- The Party Hall: parties made from chat, and the party events.

local events = require "wa.events"
local parties = require "wa.parties"
local rules = require "wa.rules"

local function say(player, text)
  player:whisper(text, { from = "Party Hall" })
end

return function()
  -- "!party" in local chat makes a party with the player as its leader,
  -- with even exp share. "!party size" tells how many members a party
  -- may hold in this world's ruleset.
  events.on("chat", function(message)
    local player = message.player
    if message.text == "!party size" then
      say(player, "A party holds up to " .. rules.limits.party_members .. " members here.")
      return
    end
    if message.text ~= "!party" then
      return
    end
    if player.party ~= nil then
      say(player, "You are in " .. player.party.name .. " already.")
      return
    end
    local party = parties.create(player, player.name .. "'s Hall")
    if party == nil then
      say(player, "That party name is taken.")
      return
    end
    party:set_options { exp_share = "even" }
  end, { channel = "public" })

  -- party_join and party_leave run on the map of the player who joined
  -- or left.
  events.on("party_join", function(join)
    say(join.player, "Welcome to " .. join.party.name .. ". " .. join.party.leader_name .. " leads it.")
  end)
  events.on("party_leave", function(leave)
    say(leave.player, "You left " .. leave.party.name .. " (" .. leave.reason .. ").")
  end)

  -- party_change runs on the map where the change was made. The members
  -- here hear about it.
  events.on("party_change", function(change)
    for _, member in ipairs(change.party:get_members()) do
      member:send_message("Party exp share: " .. change.options.exp_share .. ".")
    end
  end)
end
