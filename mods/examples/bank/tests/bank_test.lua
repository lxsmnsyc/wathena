-- Tests of the Bank mod, which wactl mod test runs. Each case gets a
-- fresh world with this mod and core loaded.

local testing = require "wa.testing"
local items = require "wa.items"
local rules = require "wa.rules"
local skills = require "wa.skills"
local expect = testing.expect

local CARD = items.id "example.bank:Bank_Card"
local JELLOPY = items.id "Jellopy"

-- A player next to the banker. Basic Skill at level 9 lets it trade.
local function new_player(world, spec)
  spec.map, spec.x, spec.y = "prontera", 150, 188
  spec.skills = { { skills.id "NV_BASIC", 9 } }
  return world:new_player(spec)
end

local function texts(world, player)
  local out = {}
  for _, line in ipairs(world:get_chat(player)) do
    out[#out + 1] = line.channel .. " " .. line.from .. ": " .. line.text
  end
  return out
end

return function()
  testing.case("give_item, count_item and take_item reach the vault and the storage", function(world)
    local player = new_player(world, {})
    expect(player:give_item(JELLOPY, 5, { to = "vault" })).to_be_true()
    expect(player:count_item(JELLOPY, { from = "vault" })).to_be(5)
    expect(player:count_item(JELLOPY)).to_be(0)
    -- The vault and the account's storage are apart.
    expect(player:count_item(JELLOPY, { from = "storage" })).to_be(0)
    expect(player:take_item(JELLOPY, 3, { from = "vault" })).to_be_true()
    expect(player:count_item(JELLOPY, { from = "vault" })).to_be(2)
    -- take_item takes nothing when there are too few.
    expect(player:take_item(JELLOPY, 5, { from = "vault" })).to_be_false()
    expect(player:count_item(JELLOPY, { from = "vault" })).to_be(2)
    expect(player:give_item(JELLOPY, 7, { to = "storage" })).to_be_true()
    expect(player:count_item(JELLOPY, { from = "storage" })).to_be(7)
    expect(player:take_item(JELLOPY, 7, { from = "storage" })).to_be_true()
    expect(player:count_item(JELLOPY, { from = "storage" })).to_be(0)
    expect(function() player:count_item(JELLOPY, { from = "safe" }) end).to_fail_with("there is no storage \"safe\"")
  end)

  testing.case("the banker opens the vault for card holders, for a fee", function(world)
    local player = new_player(world, { zeny = 1000 })
    expect(world:talk(player, "Banker#prontera", { 2 })).to_contain("The vault is for card holders.")
    expect(world:talk(player, "Banker#prontera", { 3 })).to_contain("Here is your Bank Card.")
    expect(player:count_item(CARD)).to_be(1)
    expect(world:talk(player, "Banker#prontera", { 3 })).to_contain("You have a card already.")
    expect(world:talk(player, "Banker#prontera", { 2 })).to_contain("Here is your vault.")
    expect(player.zeny).to_be(900)
    -- A storage window is open, so count_item cannot read a storage.
    expect(player:count_item(JELLOPY, { from = "vault" })).to_be_nil()
    -- A second storage does not open, and the fee is not paid again.
    expect(world:talk(player, "Banker#prontera", { 2 })).to_contain("Your vault is in use.")
    expect(player.zeny).to_be(900)
  end)

  testing.case("the banker opens the account's storage for free", function(world)
    local player = new_player(world, { zeny = 1000 })
    world:talk(player, "Banker#prontera", { 1 })
    expect(player.zeny).to_be(1000)
    expect(player:count_item(JELLOPY, { from = "storage" })).to_be_nil()
    expect(player:open_storage("vault")).to_be_false()
  end)

  testing.case("the Bank Card is bound to the account", function(world)
    local alice = new_player(world, {})
    local bob = new_player(world, {})
    world:talk(alice, "Banker#prontera", { 3 })
    local ok, why = world:trade(alice, bob, { items = { [CARD] = 1 } })
    expect(ok).to_be_false()
    expect(why).to_be("bound")
    expect(alice:count_item(CARD)).to_be(1)
    expect(bob:count_item(CARD)).to_be(0)
    -- Storage of the account takes account-bound copies only.
    expect(alice:give_item(CARD, 1, { to = "vault", bound = "account" })).to_be_true()
    expect(alice:give_item(CARD, 1, { to = "vault", bound = "character" })).to_be_false()
    expect(alice:count_item(CARD, { from = "vault" })).to_be(1)
  end)

  testing.case("the bank refuses a trade of more than 100,000 zeny", function(world)
    local alice = new_player(world, { zeny = 200000 })
    local bob = new_player(world, {})
    local ok, why = world:trade(alice, bob, { zeny = 150000 })
    expect(ok).to_be_false()
    expect(why).to_be("Trades of more than 100000 zeny go through the bank.")
    expect(alice.zeny).to_be(200000)
    expect(bob.zeny).to_be(0)
    expect(texts(world, bob)).to_equal({})
  end)

  testing.case("a smaller trade completes, and the bank tells both players", function(world)
    local alice = new_player(world, { items = { { JELLOPY, 10 } } })
    local bob = new_player(world, { zeny = 1000 })
    local ok, why = world:trade(alice, bob, { items = { [JELLOPY] = 4 } }, { zeny = 500 })
    expect(ok).to_be_true()
    expect(why).to_be_nil()
    expect(alice:count_item(JELLOPY)).to_be(6)
    expect(bob:count_item(JELLOPY)).to_be(4)
    expect(alice.zeny).to_be(500)
    expect(bob.zeny).to_be(500)
    -- Bob confirmed last, so he is the trade's first player.
    expect(texts(world, alice)).to_contain("whisper Bank: Your trade with " .. bob.name .. ": 4 items and 500 zeny changed hands.")
    expect(texts(world, bob)).to_contain("whisper Bank: Your trade with " .. alice.name .. ": 4 items and 500 zeny changed hands.")
  end)

  testing.case("the storage limits come from the ruleset", function(world)
    new_player(world, {})
    expect(rules.limits.storage_slots).to_be(600)
    expect(rules.limits.cart_slots).to_be(100)
    local base, max = rules.get_limit("inventory_slots")
    expect(base).to_be(100)
    expect(max).to_be(200)
    expect(rules.get("basic_skill_check")).to_be_true()
    expect(rules.get("block_list_refuses_trade")).to_be_true()
    expect(rules.is("item_rolls")).to_be_false()
    expect(function() rules.get("no_such_rule") end).to_fail_with("there is no rule \"no_such_rule\"")
  end)

  testing.case("the banker sells inventory slots up to the max", function(world)
    local player = new_player(world, { zeny = 100000 })
    expect(world:talk(player, "Banker#prontera", { 4 })).to_contain("Your bag holds 110 items now.")
    expect(player.zeny).to_be(95000)
    -- player:expand stops at the limit's max and returns the new value.
    expect(player:expand("inventory_slots", 1000)).to_be(200)
    expect(world:talk(player, "Banker#prontera", { 4 })).to_contain("Your bag cannot grow any more.")
    expect(player.zeny).to_be(95000)
    -- A negative amount lowers the slots, not below the base.
    expect(player:expand("inventory_slots", -1000)).to_be(100)
    -- The account's storage is at its max, 600, by default.
    expect(player:expand("storage_slots", 50)).to_be(600)
    expect(function() player:expand("party_members", 1) end).to_fail_with("not a limit a player expands")
  end)
end
