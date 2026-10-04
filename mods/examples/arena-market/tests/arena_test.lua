-- Tests of the Arena Market mod, which wactl mod test runs. Each case
-- gets a fresh world with this mod and core loaded.

local testing = require "wa.testing"
local items = require "wa.items"
local expect = testing.expect

local TROPHY = items.id "example.arena:Arena_Trophy"
local JELLOPY = items.id "Jellopy"
local RED_POTION = items.id "Red_Potion"
local FLY_WING = items.id "Wing_Of_Fly"
local DESK = "example.arena:Prize Desk"

-- A player next to the arena master.
local function new_player(world, spec)
  spec.map, spec.x, spec.y = "prontera", 152, 188
  return world:new_player(spec)
end

return function()
  testing.case("tokens are the character's own, up to their max", function(world)
    local player = new_player(world, {})
    expect(player:get_currency("tokens")).to_be(0)
    player:give_currency("tokens", 7)
    expect(player:get_currency("example.arena:tokens")).to_be(7)
    expect(player:take_currency("tokens", 8)).to_be_false()
    expect(player:take_currency("tokens", 7)).to_be_true()
    player:give_currency("tokens", 5000)
    expect(player:get_currency("tokens")).to_be(1000)
    expect(function() player:get_currency("medals") end).to_fail_with("there is no such currency")
  end)

  testing.case("the arena master pays tokens and mails a trophy", function(world)
    local player = new_player(world, {})
    expect(world:talk(player, "Arena Master#prontera", { 1 })).to_contain("Here are 5 tokens.")
    expect(world:talk(player, "Arena Master#prontera", { 2 })).to_contain("A trophy costs 10 tokens.")
    world:talk(player, "Arena Master#prontera", { 1 })
    expect(world:talk(player, "Arena Master#prontera", { 2 })).to_contain("Your trophy is in the mail.")
    expect(player:get_currency("tokens")).to_be(0)
    local mails = world:get_mails(player)
    expect(#mails).to_be(1)
    expect(mails[1].from).to_be("Arena Master")
    expect(mails[1].items[TROPHY]).to_be(1)
  end)

  testing.case("the prize desk takes zeny, Jellopy and tokens, and runs out", function(world)
    local player = new_player(world, { zeny = 100, items = { { JELLOPY, 30 } } })
    local ok, why = world:buy(player, DESK, { [RED_POTION] = 1 })
    expect(ok).to_be_false()
    expect(why).to_be("not_enough_currency")
    player:give_currency("tokens", 10)
    expect((world:buy(player, DESK, { [RED_POTION] = 2 }))).to_be_true()
    expect(player.zeny).to_be(60)
    expect(player:get_currency("tokens")).to_be(6)
    expect(player:count_item(RED_POTION)).to_be(2)
    expect((world:buy(player, DESK, { [FLY_WING] = 3 }))).to_be_true()
    expect(player:count_item(JELLOPY)).to_be(15)
    expect(player:get_currency("tokens")).to_be(3)
    ok, why = world:buy(player, DESK, { [FLY_WING] = 1 })
    expect(ok).to_be_false()
    expect(why).to_be("out_of_stock")
  end)

  testing.case("trophies stay out of vending shops, and a sale pays a token", function(world)
    -- A vending shop keeps 3 cells from NPCs, as min_npc_vendchat_distance.
    local seller = world:new_player({ map = "prontera", x = 150, y = 170, items = { { TROPHY, 1 }, { JELLOPY, 10 } } })
    local buyer = world:new_player({ map = "prontera", x = 151, y = 170, zeny = 1000 })
    local ok, why = world:vend(seller, buyer, { { item = TROPHY, amount = 1, price = 500, buy = 1 } })
    expect(ok).to_be_false()
    expect(why).to_be("Arena trophies are not for sale.")
    expect(seller:get_currency("tokens")).to_be(0)
    expect((world:vend(seller, buyer, { { item = JELLOPY, amount = 10, price = 10, buy = 4 } }))).to_be_true()
    expect(buyer:count_item(JELLOPY)).to_be(4)
    expect(buyer.zeny).to_be(960)
    expect(seller.zeny).to_be(40)
    expect(seller:get_currency("tokens")).to_be(1)
  end)
end
