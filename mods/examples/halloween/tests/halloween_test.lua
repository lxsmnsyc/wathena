-- Tests of the Halloween mod. wactl mod test runs them in headless
-- worlds on prt_fild08, the map the mod's events and NPC use.

local testing = require "wa.testing"
local items = require "wa.items"
local monsters = require "wa.monsters"
local variables = require "wa.variables"

local HAT = items.id "example.halloween:Pumpkin_Hat"
local PUMPKIN = items.id "Pumpkin"
local PUMPKIN_PORING = monsters.id "example.halloween:PUMPKIN_PORING"
local PUMPKIN_KING = monsters.id "example.halloween:PUMPKIN_KING"
local got_hat = variables.character_boolean "got_hat"

local MINUTE = 60 * 1000

-- kill_porings has the player kill n new Pumpkin Porings next to it, and
-- returns how many Pumpkins lie on the map after.
local function kill_porings(world, player, n)
  for _ = 1, n do
    local poring = world:spawn_monster { monster = PUMPKIN_PORING, x = player.x + 1, y = player.y }
    world:kill(player, poring)
  end
  return world:count_floor_items(PUMPKIN)
end

return function()
  testing.case("the witch gives one hat per character", function(world)
    local player = world:new_player { map = "prt_fild08", x = 199, y = 202 }
    local said = world:talk(player, "Pumpkin Witch#prt_fild08", { 1 })
    testing.expect(said).to_contain("Take this hat.")
    testing.expect(player:count_item(HAT)).to_be(1)
    testing.expect(got_hat:get(player)).to_be_true()
    said = world:talk(player, "Pumpkin Witch", {})
    testing.expect(said).to_contain("You already have my hat.")
    testing.expect(player:count_item(HAT)).to_be(1)
    -- Another character gets its own.
    local other = world:new_player { x = 200, y = 202 }
    world:talk(other, "Pumpkin Witch", { 1 })
    testing.expect(other:count_item(HAT)).to_be(1)
  end)

  testing.case("the hat is a mod item with its name", function(world)
    testing.expect(HAT >= 2000000).to_be_true()
    testing.expect(items.name(HAT)).to_be("Pumpkin Hat")
  end)

  testing.case("pumpkin porings spawn on the field and come back", function(world)
    local player = world:new_player { map = "prt_fild08", x = 200, y = 200 }
    local porings = world:get_monsters(PUMPKIN_PORING)
    testing.expect(#porings).to_be(15)
    testing.expect(porings[1].name).to_be("Pumpkin Poring")
    testing.expect(porings[1].level).to_be(12)
    testing.expect(porings[1].max_hp).to_be(260)
    -- Five kills give a novice its second base level: 121 exp each after
    -- the renewal level penalty, and 548 to level up.
    for i = 1, 5 do
      world:kill(player, porings[i])
    end
    testing.expect(#world:get_monsters(PUMPKIN_PORING)).to_be(10)
    testing.expect(player.base_level).to_be(2)
    world:advance(6000)
    testing.expect(#world:get_monsters(PUMPKIN_PORING)).to_be(15)
  end)

  testing.case("night doubles pumpkin poring drops", function(world)
    world:set_time("2026-10-31 17:50")
    local player = world:new_player { map = "prt_fild08", x = 100, y = 100 }
    local day = kill_porings(world, player, 300)
    -- Floor items expire after a minute: the day's pumpkins are gone by
    -- the time night falls.
    world:advance(10 * MINUTE)
    testing.expect(world:get_time()).to_be("2026-10-31 18:00")
    testing.expect(world:count_floor_items(PUMPKIN)).to_be(0)
    testing.expect(world:get_announcements()).to_contain("Night falls. Pumpkin Porings drop twice as much until dawn.")
    local night = kill_porings(world, player, 300)
    testing.expect(night > day * 3 // 2).to_be_true()
    testing.expect(night < day * 5 // 2).to_be_true()
  end)

  testing.case("a server that starts at night has the night bonus, until dawn", function(world)
    world:set_time("2026-11-01 05:58", "Asia/Manila")
    local player = world:new_player { map = "prt_fild08", x = 100, y = 100 }
    testing.expect(world:get_announcements()).to_contain("Night falls. Pumpkin Porings drop twice as much until dawn.")
    local night = kill_porings(world, player, 300)
    world:advance(3 * MINUTE)
    testing.expect(world:get_time()).to_be("2026-11-01 06:01")
    local day = kill_porings(world, player, 300)
    testing.expect(night > day * 3 // 2).to_be_true()
  end)

  testing.case("the pumpkin king rises at 20:00 and its death is announced", function(world)
    world:set_time("2026-10-31 19:58")
    local player = world:new_player { map = "prt_fild08", x = 200, y = 200 }
    -- The night began at 18:00, so its run starts with the world.
    testing.expect(world:get_announcements()).to_equal({ "Night falls. Pumpkin Porings drop twice as much until dawn." })
    world:advance(MINUTE)
    testing.expect(#world:get_monsters(PUMPKIN_KING)).to_be(0)
    testing.expect(#world:get_announcements()).to_be(1)
    world:advance(MINUTE)
    local kings = world:get_monsters(PUMPKIN_KING)
    testing.expect(#kings).to_be(1)
    testing.expect(kings[1].name).to_be("Pumpkin King")
    testing.expect(world:get_announcements()).to_contain("The Pumpkin King has risen on the Prontera fields!")
    local x, y = kings[1].x, kings[1].y
    world:kill(player, kings[1])
    testing.expect(world:get_announcements()).to_contain("Tester 1 defeated the Pumpkin King at " .. x .. "," .. y .. "!")
    testing.expect(#world:get_monsters(PUMPKIN_KING)).to_be(0)
    -- A boss does not come back, and the next one rises tomorrow.
    world:advance(61 * MINUTE)
    testing.expect(#world:get_monsters(PUMPKIN_KING)).to_be(0)
  end)

  testing.case("a pumpkin king nobody kills goes when the hour ends", function(world)
    world:set_time("2026-10-31 19:59")
    world:new_player { map = "prt_fild08", x = 200, y = 200 }
    world:advance(2 * MINUTE)
    testing.expect(#world:get_monsters(PUMPKIN_KING)).to_be(1)
    world:advance(58 * MINUTE)
    testing.expect(#world:get_monsters(PUMPKIN_KING)).to_be(1)
    world:advance(MINUTE)
    testing.expect(world:get_time()).to_be("2026-10-31 21:00")
    testing.expect(#world:get_monsters(PUMPKIN_KING)).to_be(0)
    -- The pumpkin porings of the mod script stay.
    testing.expect(#world:get_monsters(PUMPKIN_PORING)).to_be(15)
  end)
end
