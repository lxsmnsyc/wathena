-- Tests of the Party Hall mod, which wactl mod test runs. Each case gets
-- a fresh world with this mod and core loaded.

local testing = require "wa.testing"
local parties = require "wa.parties"
local rules = require "wa.rules"
local expect = testing.expect

local function texts(world, player)
  local out = {}
  for _, line in ipairs(world:get_chat(player)) do
    out[#out + 1] = line.channel .. " " .. line.from .. ": " .. line.text
  end
  return out
end

return function()
  testing.case("!party makes a party with even exp share", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    world:chat(alice, "!party")
    world:advance(100)
    local party = alice.party
    expect(party == nil).to_be_false()
    expect(party.name).to_be(alice.name .. "'s Hall")
    expect(party.leader_name).to_be(alice.name)
    expect(party.leader).to_be(alice)
    expect(party.exp_share).to_be("even")
    expect(party.item_share).to_be("each")
    expect(party:get_member_names()).to_equal({ alice.name })
    expect(texts(world, alice)).to_contain("whisper Party Hall: Welcome to " .. party.name .. ". " .. alice.name .. " leads it.")
    expect(parties.find(party.name)).to_be(party)
    expect(parties.get(party.id)).to_be(party)
  end)

  testing.case("a second !party is refused", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    world:chat(alice, "!party")
    world:advance(100)
    world:chat(alice, "!party")
    world:advance(100)
    expect(texts(world, alice)).to_contain("whisper Party Hall: You are in " .. alice.party.name .. " already.")
  end)

  testing.case("members hear the party events", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    local bob = world:new_player { map = "prontera", x = 152, y = 150 }
    local party = parties.create(alice, "Testers")
    expect(party:add(bob)).to_be_true()
    world:advance(100)
    expect(texts(world, bob)).to_contain("whisper Party Hall: Welcome to Testers. " .. alice.name .. " leads it.")
    expect(#party:get_members()).to_be(2)
    party:set_options { item_pickup = "shared" }
    world:advance(100)
    expect(party.item_pickup).to_be("shared")
    expect(texts(world, bob)).to_contain("system : Party exp share: each.")
    expect(party:remove(bob)).to_be_true()
    world:advance(100)
    expect(bob.party).to_be_nil()
    expect(texts(world, bob)).to_contain("whisper Party Hall: You left Testers (script).")
  end)

  testing.case("the leader's leave breaks the party up", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    local bob = world:new_player { map = "prontera", x = 152, y = 150 }
    local party = parties.create(alice, "Short Lived")
    party:add(bob)
    party:remove(alice)
    world:advance(100)
    expect(bob.party).to_be_nil()
    expect(texts(world, bob)).to_contain("whisper Party Hall: You left Short Lived (disbanded).")
  end)

  testing.case("the limits come from the ruleset", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    expect(rules.limits.party_members).to_be(12)
    expect(rules.limits.friends).to_be(40)
    expect(rules.limits.block_list).to_be(20)
    world:chat(alice, "!party size")
    expect(texts(world, alice)).to_contain("whisper Party Hall: A party holds up to 12 members here.")
  end)

  testing.case("players start without friends", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    local bob = world:new_player { map = "prontera", x = 152, y = 150 }
    expect(alice:is_friend(bob)).to_be_false()
    expect(alice:get_friend_names()).to_equal({})
  end)
end
