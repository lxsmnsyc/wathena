-- The banker: the account's storage for free, the vault for card
-- holders, for a fee, and more inventory slots, as rAthena's inventory
-- expansion. Scripts charge any fee: open_storage and expand take none.

local events = require "wa.events"
local items = require "wa.items"
local rules = require "wa.rules"

local CARD = items.id "example.bank:Bank_Card"

-- The fee to open the vault, in zeny.
local FEE = 100

-- The slots one bag expansion adds, as rAthena's Inventory Expansion
-- Coupon, and its price in zeny.
local BAG_SLOTS = 10
local BAG_FEE = 5000

-- open_vault opens the vault, data/storages.yml's example.bank:vault.
local function open_vault(player, conversation)
  if player:count_item(CARD) == 0 then
    conversation:say("[Banker]", "The vault is for card holders.")
    return
  end
  if player.zeny < FEE then
    conversation:say("[Banker]", "The vault costs " .. FEE .. " zeny.")
    return
  end
  -- open_storage returns false when the player has a storage open, or
  -- another character of the account has the vault open.
  if not player:open_storage("vault") then
    conversation:say("[Banker]", "Your vault is in use.")
    return
  end
  -- The fee is paid only when the vault opened.
  player:pay_zeny(FEE)
  conversation:say("[Banker]", "Here is your vault.")
end

-- give_card gives one Bank Card, bound to the account. A bound card
-- cannot be traded, dropped or sold.
local function give_card(player, conversation)
  if player:count_item(CARD) > 0 then
    conversation:say("[Banker]", "You have a card already.")
    return
  end
  if not player:give_item(CARD, 1, { bound = "account" }) then
    conversation:say("[Banker]", "You cannot carry the card.")
    return
  end
  conversation:say("[Banker]", "Here is your Bank Card.")
end

-- expand_bag sells BAG_SLOTS more inventory slots, up to the ruleset's
-- max of inventory_slots.
local function expand_bag(player, conversation)
  local _, max = rules.get_limit("inventory_slots")
  -- An amount of 0 changes nothing and returns the player's slots.
  if player:expand("inventory_slots", 0) >= max then
    conversation:say("[Banker]", "Your bag cannot grow any more.")
    return
  end
  if not player:pay_zeny(BAG_FEE) then
    conversation:say("[Banker]", "A bigger bag costs " .. BAG_FEE .. " zeny.")
    return
  end
  local slots = player:expand("inventory_slots", BAG_SLOTS)
  conversation:say("[Banker]", "Your bag holds " .. slots .. " items now.")
end

return function()
  events.on("click", function(player, conversation)
    conversation:say("[Banker]", "Welcome to the bank.")
    conversation:wait_for_next()
    local choice = conversation:choose("My storage", "The vault (" .. FEE .. " zeny)", "A Bank Card",
      "A bigger bag (" .. BAG_FEE .. " zeny)", "Goodbye")
    if choice == 1 then
      -- Without a name, open_storage opens the account's storage.
      if not player:open_storage() then
        conversation:say("[Banker]", "Your storage is in use.")
      end
    elseif choice == 2 then
      open_vault(player, conversation)
    elseif choice == 3 then
      give_card(player, conversation)
    elseif choice == 4 then
      expand_bag(player, conversation)
    end
  end)
end
