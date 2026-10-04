-- The arena's market rules: a vend hook that keeps trophies out of
-- vending shops, and a vend event that pays the seller a token.

local events = require "wa.events"
local hooks = require "wa.hooks"
local items = require "wa.items"

local TROPHY = items.id "example.arena:Arena_Trophy"

return function()
  -- The hook runs before a vending sale. It returns true, or false and
  -- the message the buyer sees.
  hooks.on("vend", function(sale, next)
    for _, it in ipairs(sale.items) do
      if it.item == TROPHY then
        return false, "Arena trophies are not for sale."
      end
    end
    return next(sale)
  end)

  -- The event runs after a sale on this map: the seller gets a token.
  events.on("vend", function(sale)
    sale.seller:give_currency("tokens", 1)
  end)
end
