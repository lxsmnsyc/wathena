-- Three Dummies on the start map, at fixed cells, that come back a second
-- after they die. On every map, chat commands said in local chat help the
-- tests:
--   - "!guild level <n>" gives the speaker's guild the exp that takes it
--     to level n, so the tests reach the guild skills without thousands
--     of kills.
--   - "!give <item> <amount> [to=<place>] [bound=<kind>]" gives items to
--     the inventory, or to "storage", "cart" or "Vault".
--   - "!take <item> <amount> [<place>]" and "!count <item> [<place>]"
--     take and count items.
--   - "!storage [<name>]" opens the account's storage or the Vault.
--   - "!cart" gives the speaker a cart, which needs Pushcart.
--   - "!zeny <amount>" gives zeny.
--   - "!currency <name> <amount>" gives a currency and answers with the
--     new amount, and "!pay <name> <amount>" takes it.
--   - "!mailbox" opens the mailbox.
--   - "!mail <to> <item> <amount> [<zeny>]" sends a mail from the
--     "Postmaster" with items made for it, to a character by name, or to
--     the speaker with "self".
--   - "!buyingstore <slots>" opens the buying store setup.
--   - "!search <uses> <show|open>" opens the store search.
--   - "!autotrade" keeps the speaker's store open without its connection.
--   - "!stock <item> <n>" sets an item's stock at the Exchange.
--   - "!event start <name>" and "!event stop <name>" start and stop a
--     world event of data/events.yml, with world_events.
--   - "!expand <limit> <amount>" changes one of the speaker's limits with
--     player:expand and answers with the new value.
-- Each command but "!guild level" answers with a system line that starts
-- with "e2e <name>:". A trade hook and a vend hook refuse Contraband. The
-- trade, vend and buying_store_sale events tell both players what changed
-- hands.

local context = require "wa.context"
local events = require "wa.events"
local guilds = require "wa.guilds"
local hooks = require "wa.hooks"
local items = require "wa.items"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"
local world = require "wa.world"
local world_events = require "wa.world_events"

local DUMMY = monsters.id "e2e.fixture:DUMMY"
local CONTRABAND = items.id "e2e.fixture:Contraband"

-- next_exp is renewal's exp_guild.yml: level n needs n * n * 100000 exp
-- to reach the next one. The tests run in renewal.
local function next_exp(level)
  return level * level * 100000
end

local function level_guild(player, target)
  local guild = player.guild
  if guild == nil then
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
end

-- place is a command's place word, or nil for the inventory.
local function place(word)
  if word == nil then
    return nil
  end
  return { from = word }
end

-- commands run a chat command's words and return its answer.
local commands = {
  give = function(player, words)
    local options = {}
    for i = 4, #words do
      local k, v = words[i]:match("^(%a+)=(%S+)$")
      options[k] = v
    end
    return player:give_item(items.id(words[2]), tonumber(words[3]), options)
  end,
  take = function(player, words)
    return player:take_item(items.id(words[2]), tonumber(words[3]), place(words[4]))
  end,
  count = function(player, words)
    return player:count_item(items.id(words[2]), place(words[3]))
  end,
  storage = function(player, words)
    return player:open_storage(words[2])
  end,
  cart = function(player)
    return player:set_cart(1)
  end,
  zeny = function(player, words)
    player:give_zeny(tonumber(words[2]))
    return true
  end,
  currency = function(player, words)
    player:give_currency(words[2], tonumber(words[3]))
    return player:get_currency(words[2])
  end,
  pay = function(player, words)
    return player:take_currency(words[2], tonumber(words[3]))
  end,
  mailbox = function(player)
    return player:open_mail()
  end,
  mail = function(player, words)
    local to = words[2]
    if to == "self" then
      to = nil
    end
    player:send_mail {
      to = to,
      sender = "Postmaster",
      title = "Parcel",
      body = "From the fixture.",
      zeny = tonumber(words[5] or "0"),
      items = { { item = items.id(words[3]), amount = tonumber(words[4]) } },
    }
    return true
  end,
  buyingstore = function(player, words)
    return player:open_buying_store { slots = tonumber(words[2]) }
  end,
  search = function(player, words)
    return player:search_stores { uses = tonumber(words[2]), effect = words[3] }
  end,
  autotrade = function(player)
    return player:autotrade()
  end,
  stock = function(_, words)
    return world.get_shop("Exchange#e2e"):set_stock(items.id(words[2]), tonumber(words[3]))
  end,
  expand = function(player, words)
    return player:expand(words[2], tonumber(words[3]))
  end,
  event = function(_, words)
    if words[2] == "start" then
      return world_events.start(words[3])
    end
    return world_events.stop(words[3])
  end,
}

-- sale_line is the line the vend and buying_store_sale events send.
local function sale_line(name, sale)
  local amount = 0
  for _, it in ipairs(sale.items) do
    amount = amount + it.amount
  end
  return "e2e " .. name .. ": " .. sale.buyer.name .. " bought " .. amount .. " items from " .. sale.seller.name
    .. " for " .. sale.zeny .. " zeny"
end

return function()
  events.on("chat", function(message)
    local target = tonumber(message.text:match("^!guild level (%d+)$"))
    if target ~= nil then
      level_guild(message.player, target)
      return
    end
    local words = {}
    for w in message.text:gmatch("%S+") do
      words[#words + 1] = w
    end
    local name = (words[1] or ""):match("^!(%a+)$")
    local command = commands[name]
    if command == nil then
      return
    end
    message.player:send_message("e2e " .. name .. ": " .. tostring(command(message.player, words)))
  end, { channel = "public" })

  hooks.on("trade", function(trade, next)
    for _, it in ipairs(trade.items) do
      if it.item == CONTRABAND then
        return false, "No contraband here."
      end
    end
    return next()
  end)

  hooks.on("vend", function(sale, next)
    for _, it in ipairs(sale.items) do
      if it.item == CONTRABAND then
        return false, "No contraband here."
      end
    end
    return next()
  end)

  for _, name in ipairs({ "vend", "buying_store_sale" }) do
    events.on(name, function(sale)
      local line = sale_line(name, sale)
      sale.seller:send_message(line)
      sale.buyer:send_message(line)
    end)
  end

  events.on("trade", function(trade)
    local amount = 0
    for _, it in ipairs(trade.items) do
      amount = amount + it.amount
    end
    local line = "e2e trade: " .. trade.players[1].name .. " and " .. trade.players[2].name .. ", " .. amount .. " items, "
      .. trade.zeny[1] .. " and " .. trade.zeny[2] .. " zeny"
    for _, p in ipairs(trade.players) do
      p:send_message(line)
    end
  end)

  if context:get_instance().map ~= "placeholder/map" then
    return
  end
  for _, x in ipairs({ 9, 12, 15 }) do
    spawning.monsters { monster = DUMMY, amount = 1, x = x, y = 15, respawn = true, respawn_milliseconds = 1000 }
  end
end
