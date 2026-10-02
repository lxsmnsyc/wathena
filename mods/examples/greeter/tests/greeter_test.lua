-- Tests of the greeter, which wactl mod test runs. Each case gets a fresh
-- world with this mod and core loaded, a fixed random seed, and no
-- database.

local testing = require "wa.testing"
local variables = require "wa.variables"
local items = require "wa.items"

-- The test file runs as the mod, so it reads the NPC's variables.
local visits = variables.character_integer "visits"
local COOKIE = items.id "example.greeter:Welcome_Cookie"

return function()
  testing.case("the greeter counts visits", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 191 }
    local said = world:talk(player, "Greeter#prontera", { 4 })
    testing.expect(said).to_contain("You have visited me 1 times.")
    testing.expect(said).to_contain("Come back any time.")
    world:talk(player, "Greeter#prontera", { 4 })
    testing.expect(visits:get(player)).to_be(2)
  end)

  testing.case("resting waits for the progress bar", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 191 }
    local said = world:talk(player, "Greeter", { 3 })
    testing.expect(said).to_contain("Feeling better?")
  end)

  testing.case("a welcome cookie is used up", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 191, items = { { COOKIE, 2 } } }
    world:use_item(player, COOKIE)
    testing.expect(player:count_item(COOKIE)).to_be(1)
  end)

  testing.case("the greeter needs an answer to its menu", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 191 }
    testing.expect(function()
      world:talk(player, "Greeter#prontera", {})
    end).to_fail_with("asks for a menu choice")
  end)
end
