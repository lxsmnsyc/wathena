-- Tests of the Familiars mod, which wactl mod test runs. Each case gets a
-- fresh world with this mod and core loaded.

local testing = require "wa.testing"
local companions = require "wa.companions"
local jobs = require "wa.jobs"
local mounts = require "wa.mounts"
local skills = require "wa.skills"

local OWL = companions.id "example.familiars:Owl"
local HORSE = mounts.id "example.familiars:Horse"
local FALCONRY = skills.id "HT_FALCON"
local RIDING = skills.id "KN_RIDING"
local PUSHCART = skills.id "MC_PUSHCART"

local function player(world, job, learned)
  return world:new_player { map = "prontera", x = 143, y = 192, job = job, base_level = 80, job_level = 40, skills = learned }
end

return function()
  testing.case("the records have IDs, the mod's from 10,000", function()
    testing.expect(companions.id "falcon").to_be(1)
    testing.expect(mounts.id "peco_peco").to_be(1)
    testing.expect(OWL >= 10000).to_be_true()
    testing.expect(HORSE >= 10000).to_be_true()
    testing.expect(function() companions.id "example.familiars:Cat" end).to_fail_with("no companion")
  end)

  testing.case("a Hunter with Falconry Mastery gets the Owl from the keeper", function(world)
    local hunter = player(world, jobs.hunter, { { FALCONRY, 1 } })
    local said = world:talk(hunter, "Keeper#prontera", { 1 })
    testing.expect(said).to_contain("The Owl is yours.")
    testing.expect(hunter.companion).to_be(OWL)
    -- The Owl replaces a falcon, and the falcon replaces the Owl.
    testing.expect(hunter:set_companion(companions.id "falcon")).to_be_true()
    testing.expect(hunter.companion).to_be(companions.id "falcon")
    testing.expect(hunter:set_companion(nil)).to_be_true()
    testing.expect(hunter.companion).to_be_nil()
  end)

  testing.case("the Owl needs Falconry Mastery", function(world)
    local archer = player(world, jobs.archer, {})
    local said = world:talk(archer, "Keeper#prontera", { 1 })
    testing.expect(said).to_contain("Learn Falconry Mastery first.")
    testing.expect(archer.companion).to_be_nil()
  end)

  testing.case("the Owl gives DEX while it stays", function(world)
    local hunter = player(world, jobs.hunter, { { FALCONRY, 1 } })
    hunter:set_companion(OWL)
    testing.expect(hunter.companion).to_be(OWL)
    -- A skill reset takes away Falconry Mastery, and the Owl with it.
    hunter:reset_skills()
    testing.expect(hunter.companion).to_be_nil()
  end)

  testing.case("a Knight with Riding rides the Horse", function(world)
    local knight = player(world, jobs.knight, { { RIDING, 1 } })
    local hp = knight.max_hp
    local said = world:talk(knight, "Keeper#prontera", { 2 })
    testing.expect(said).to_contain("The Horse is yours.")
    testing.expect(knight.mount).to_be(HORSE)
    testing.expect(knight.max_hp).to_be(hp + 200)
    said = world:talk(knight, "Keeper#prontera", { 3 })
    testing.expect(said).to_contain("I will look after them.")
    testing.expect(knight.mount).to_be_nil()
    testing.expect(knight.max_hp).to_be(hp)
  end)

  testing.case("losing Riding takes the mount away", function(world)
    local knight = player(world, jobs.knight, { { RIDING, 1 } })
    testing.expect(knight:set_mount(HORSE)).to_be_true()
    -- A core mount replaces the Horse.
    testing.expect(knight:set_mount(mounts.id "grand_peco")).to_be_true()
    testing.expect(knight.mount).to_be(mounts.id "grand_peco")
    knight:reset_skill(RIDING)
    testing.expect(knight.mount).to_be_nil()
    testing.expect(knight:set_mount(HORSE)).to_be_false()
  end)

  testing.case("a cart needs Pushcart", function(world)
    local merchant = player(world, jobs.merchant, {})
    testing.expect(merchant:set_cart(1)).to_be_false()
    testing.expect(merchant.cart).to_be_nil()
    local smith = player(world, jobs.blacksmith, { { PUSHCART, 5 } })
    testing.expect(smith:set_cart(3)).to_be_true()
    testing.expect(smith.cart).to_be(3)
    testing.expect(smith:set_cart(9)).to_be_false()
    testing.expect(smith:set_cart(nil)).to_be_true()
    testing.expect(smith.cart).to_be_nil()
  end)
end
