-- Tests of the Town Crier mod, which wactl mod test runs. Each case gets
-- a fresh world with this mod and core loaded.

local testing = require "wa.testing"
local channels = require "wa.channels"
local expect = testing.expect

local function texts(world, player)
  local out = {}
  for _, line in ipairs(world:get_chat(player)) do
    out[#out + 1] = line.channel .. " " .. line.from .. ": " .. line.text
  end
  return out
end

return function()
  testing.case("the hook softens a word for everyone", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    local bob = world:new_player { map = "prontera", x = 152, y = 150 }
    world:chat(alice, "that was lol funny")
    expect(texts(world, bob)).to_contain("public " .. alice.name .. ": that was (laughs) funny")
  end)

  testing.case("the hook stops links", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    local bob = world:new_player { map = "prontera", x = 152, y = 150 }
    world:chat(alice, "visit https://example.com")
    expect(#texts(world, bob)).to_be(0)
    expect(#texts(world, alice)).to_be(0)
  end)

  testing.case("the filter replaces its word before the hook", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    world:chat(alice, "no grief here")
    expect(texts(world, alice)).to_contain("public " .. alice.name .. ": no ***** here")
  end)

  testing.case("!events adds the player to the channel", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    local bob = world:new_player { map = "prontera", x = 152, y = 150 }
    local crier = channels.get("events")
    expect(crier == nil).to_be_false()
    world:chat(alice, "!events")
    expect(crier:get_members()).to_equal({ alice.name })
    local heard = texts(world, alice)
    expect(heard).to_contain("whisper Town Crier: Welcome to #events. Say a line there with #events.")
    expect(heard).to_contain("#events : " .. alice.name .. " joined the crowd.")
    -- Bob is not a member: a line in the channel is refused, and only
    -- Alice hears her own line there.
    world:chat(bob, "hello?", { channel = "events" })
    world:chat(alice, "first!", { channel = "events" })
    expect(texts(world, alice)).to_contain("#events " .. alice.name .. ": first!")
    for _, line in ipairs(texts(world, bob)) do
      expect(line:find("first!", 1, true)).to_be_nil()
    end
  end)

  testing.case("a player is blocked or not", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    local bob = world:new_player { map = "prontera", x = 152, y = 150 }
    expect(alice:is_blocking(bob)).to_be_false()
  end)
end
