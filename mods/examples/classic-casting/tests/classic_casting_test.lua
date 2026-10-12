-- Tests of the Classic Casting ruleset, which wactl mod test runs. Each
-- case gets a fresh world whose ruleset is this mod. A Venom Bug, whose
-- MATK is always 340, casts Fire Bolt at a Knight in elemental armor six
-- cells away. The bolt lands within a second, before the bug walks up to
-- bite.

local testing = require "wa.testing"
local items = require "wa.items"
local jobs = require "wa.jobs"
local monsters = require "wa.monsters"
local rules = require "wa.rules"
local skills = require "wa.skills"

local FIRE_BOLT = skills.id "MG_FIREBOLT"
local VENOM_BUG = monsters.id "VENOM_BUG"

-- bolt has a Venom Bug cast Fire Bolt level 1 at a level 99 Knight in an
-- armor, on row y, and returns the HP the Knight lost.
local function bolt(world, armor, y)
  local knight = world:new_player { map = "prontera", x = 150, y = y, job = jobs.knight, base_level = 99, job_level = 50 }
  world:equip(knight, items.id(armor))
  local bug = world:spawn_monster { monster = VENOM_BUG, x = 156, y = y }
  local before = knight.hp
  testing.expect(bug:use_skill(FIRE_BOLT, 1, knight)).to_be_true()
  world:advance(1200)
  return before - knight.hp
end

return function()
  testing.case("fire hurts water armor a third as much as earth armor", function(world)
    local wet = bolt(world, "Water_Sprits_Armor", 150)
    local dry = bolt(world, "Earth_Sprits_Armor", 160)
    -- Pre-renewal's table: fire deals 50% to water 1 and 150% to earth 1.
    -- Renewal's would deal 90% to water 1, three fifths of earth's.
    testing.expect(wet > 0).to_be_true()
    testing.expect(math.abs(wet * 3 - dry) <= 3).to_be_true()
  end)

  testing.case("scripts read the ruleset's flags", function()
    -- rules.is is true at a flag's renewal value or at on.
    testing.expect(rules.is("cast_time")).to_be_false()
    testing.expect(rules.is("element_table")).to_be_false()
    testing.expect(rules.is("skill_cooldown")).to_be_false()
    testing.expect(rules.is("aspd")).to_be_true()
    -- A skill_rebalance flag is the one the skill follows: its own, or
    -- the base.
    testing.expect(rules.is("skill_rebalance.HW_MAGICPOWER")).to_be_false()
    testing.expect(rules.is("skill_rebalance.MG_FIREBALL")).to_be_true()
  end)
end
