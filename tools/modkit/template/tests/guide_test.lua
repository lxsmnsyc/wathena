-- Tests that wactl mod test runs. Each case gets a fresh world with this
-- mod and core loaded, a fixed random seed, and no database.

local testing = require "wa.testing"
local items = require "wa.items"
local variables = require "wa.variables"

local BADGE = items.id "{%.ID%}:Guide_Badge"
-- The test file runs as the mod, so it reads the NPC's variables.
local visits = variables.character_integer "visits"

return function()
  testing.case("the guide gives one badge", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 183 }
    local said = world:talk(player, "Guide#prontera", { 1 })
    testing.expect(said).to_contain("Here is your badge.")
    testing.expect(player:count_item(BADGE)).to_be(1)

    said = world:talk(player, "Guide#prontera", { 1 })
    testing.expect(said).to_contain("You already have a badge.")
    testing.expect(player:count_item(BADGE)).to_be(1)
  end)

  testing.case("the guide counts visits", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 183 }
    world:talk(player, "Guide#prontera", { 2 })
    local said = world:talk(player, "Guide#prontera", { 2 })
    testing.expect(said).to_contain("You have visited me 2 times.")
    testing.expect(visits:get(player)).to_be(2)
  end)
end
