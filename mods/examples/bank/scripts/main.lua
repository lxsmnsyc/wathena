-- The bank's trade rules: a hook that refuses large zeny trades, and an
-- event that tells both players what moved.

local events = require "wa.events"
local hooks = require "wa.hooks"

-- The most zeny one player may give in a trade.
local MAX_ZENY = 100000

return function()
  -- The hook runs before a trade completes. It returns true, or false
  -- and the message the players see. Hooks only compute: they cannot
  -- send messages.
  hooks.on("trade", function(trade, next)
    for _, zeny in ipairs(trade.zeny) do
      if zeny > MAX_ZENY then
        return false, "Trades of more than " .. MAX_ZENY .. " zeny go through the bank."
      end
    end
    return next(trade)
  end)

  -- The event runs after a trade completed. The first player confirmed
  -- last.
  events.on("trade", function(trade)
    local count = 0
    for _, it in ipairs(trade.items) do
      count = count + it.amount
    end
    local a, b = trade.players[1], trade.players[2]
    local text = count .. " items and " .. (trade.zeny[1] + trade.zeny[2]) .. " zeny changed hands."
    a:whisper("Your trade with " .. b.name .. ": " .. text, { from = "Bank" })
    b:whisper("Your trade with " .. a.name .. ": " .. text, { from = "Bank" })
  end)
end
