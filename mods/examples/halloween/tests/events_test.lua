-- Tests of the Halloween mod's typed events (review decision 31): the
-- Headless Rider, the raid on Prontera and the candy hunt. Each case
-- drives the server clock with world:set_time and world:advance.

local testing = require "wa.testing"
local items = require "wa.items"
local monsters = require "wa.monsters"

local RIDER = monsters.id "example.halloween:HEADLESS_RIDER"
local PORING = monsters.id "example.halloween:PUMPKIN_PORING"
local CHEST = monsters.id "example.halloween:CANDY_CHEST"
local PUMPKIN = items.id "Pumpkin"
local CANDY = items.id "Candy"

local SECOND = 1000
local MINUTE = 60 * SECOND

-- has reports whether a list holds a value.
local function has(list, value)
  for _, v in ipairs(list) do
    if v == value then
      return true
    end
  end
  return false
end

-- heard reports whether a player got a chat line with the text.
local function heard(world, player, text)
  for _, line in ipairs(world:get_chat(player)) do
    if line.text == text then
      return true
    end
  end
  return false
end

return function()
  testing.case("the headless rider appears at 22:00 and rewards its participants", function(world)
    world:set_time("2026-10-31 21:59")
    local fighter = world:new_player { map = "prt_fild08", x = 100, y = 100 }
    local watcher = world:new_player { x = 110, y = 100 }
    testing.expect(#world:get_monsters(RIDER)).to_be(0)
    world:advance(MINUTE)
    local riders = world:get_monsters(RIDER)
    testing.expect(#riders).to_be(1)
    testing.expect(riders[1].name).to_be("Headless Rider")
    testing.expect(world:get_announcements()).to_contain("Headless Rider has appeared on prt_fild08!")
    local zeny = fighter.zeny
    world:kill(fighter, riders[1])
    world:advance(SECOND)
    testing.expect(world:get_announcements()).to_contain("Tester 1 defeated Headless Rider on prt_fild08!")
    -- The fighter did all the damage. The watcher did none, so it gets
    -- nothing.
    testing.expect(fighter.zeny).to_be(zeny + 3000)
    testing.expect(fighter:count_item(PUMPKIN)).to_be(5)
    testing.expect(watcher:count_item(PUMPKIN)).to_be(0)
    -- The script thanks the participants, from run:get_participants().
    testing.expect(heard(world, fighter, "The village thanks you, Tester 1.")).to_be_true()
    testing.expect(heard(world, watcher, "The village thanks you, Tester 2.")).to_be_false()
    -- The mod script hears world_event_end with the result.
    testing.expect(world:get_announcements()).to_contain("The Headless Rider's night is over: won.")
  end)

  testing.case("a headless rider nobody kills leaves after half an hour", function(world)
    world:set_time("2026-10-31 21:59")
    world:new_player { map = "prt_fild08", x = 100, y = 100 }
    world:advance(MINUTE)
    testing.expect(#world:get_monsters(RIDER)).to_be(1)
    world:advance(30 * MINUTE)
    testing.expect(#world:get_monsters(RIDER)).to_be(0)
    testing.expect(world:get_announcements()).to_contain("Headless Rider has left prt_fild08.")
    testing.expect(world:get_announcements()).to_contain("The Headless Rider's night is over: lost.")
  end)

  testing.case("the rider's script ends the run when the rider kills a player", function(world)
    world:set_time("2026-10-31 21:59")
    world:new_player { map = "prt_fild08", x = 100, y = 100 }
    world:advance(MINUTE)
    local rider = world:get_monsters(RIDER)[1]
    -- A novice next to the rider, which attacks players on sight.
    world:new_player { x = rider.x + 1, y = rider.y }
    world:advance(10 * SECOND)
    testing.expect(world:get_announcements()).to_contain("The Headless Rider claimed a soul and rode into the night.")
    testing.expect(#world:get_monsters(RIDER)).to_be(0)
    testing.expect(world:get_announcements()).to_contain("The Headless Rider's night is over: lost.")
  end)

  testing.case("the raid on prontera comes in two waves and is won", function(world)
    world:set_time("2026-10-31 22:59")
    local defender = world:new_player { map = "prontera", x = 150, y = 150 }
    world:advance(MINUTE)
    testing.expect(world:get_announcements()).to_contain("prontera is under attack! Defend the town!")
    testing.expect(world:get_announcements()).to_contain("Wave 1 of 2 attacks prontera!")
    local first = world:get_monsters(PORING)
    testing.expect(#first).to_be(5)
    for _, poring in ipairs(first) do
      world:kill(defender, poring)
    end
    -- The second wave comes a minute after the first started, cleared
    -- or not.
    world:advance(MINUTE)
    testing.expect(world:get_announcements()).to_contain("Wave 2 of 2 attacks prontera!")
    local second = world:get_monsters(PORING)
    testing.expect(#second).to_be(3)
    testing.expect(defender:count_item(CANDY)).to_be(0)
    for _, poring in ipairs(second) do
      world:kill(defender, poring)
    end
    world:advance(2 * SECOND)
    testing.expect(world:get_announcements()).to_contain("prontera drove back the invasion!")
    -- Everyone in Prontera gets the reward.
    testing.expect(defender:count_item(CANDY)).to_be(3)
  end)

  testing.case("the raid is lost when time runs out", function(world)
    world:set_time("2026-10-31 22:59")
    local player = world:new_player { map = "prontera", x = 150, y = 150 }
    world:advance(MINUTE)
    world:advance(20 * MINUTE)
    testing.expect(world:get_announcements()).to_contain("The invasion of prontera was not stopped in time.")
    testing.expect(#world:get_monsters(PORING)).to_be(0)
    testing.expect(player:count_item(CANDY)).to_be(0)
  end)

  testing.case("the candy hunt hides chests, and the candy witch tells where", function(world)
    world:set_time("2026-10-31 11:59")
    local hunter = world:new_player { map = "prt_fild08", x = 197, y = 202 }
    world:advance(MINUTE)
    local chests = world:get_monsters(CHEST)
    testing.expect(#chests).to_be(3)
    testing.expect(world:get_announcements()).to_contain("3 treasure chests are hidden on the fields! Ask the Candy Witch in prt_fild08 for hints.")
    local said = world:talk(hunter, "Candy Witch", {})
    testing.expect(said).to_contain("3 chests are still hidden.")
    testing.expect(said).to_contain("One lies to the")
    local zeny = hunter.zeny
    world:kill(hunter, chests[1])
    world:advance(SECOND)
    testing.expect(hunter.zeny).to_be(zeny + 500)
    -- The chest's drops are the loot.
    testing.expect(world:count_floor_items(CANDY)).to_be(1)
    testing.expect(world:get_announcements()).to_contain("Tester 1 found a treasure chest on prt_fild08! 2 chests are left.")
    testing.expect(world:talk(hunter, "Candy Witch", {})).to_contain("2 chests are still hidden.")
    world:kill(hunter, world:get_monsters(CHEST)[1])
    world:kill(hunter, world:get_monsters(CHEST)[1])
    world:advance(SECOND)
    testing.expect(world:get_announcements()).to_contain("Tester 1 found the last treasure chest on prt_fild08!")
    testing.expect(hunter.zeny).to_be(zeny + 1500)
    -- The hint NPC goes with the run.
    testing.expect(pcall(world.talk, world, hunter, "Candy Witch", {})).to_be_false()
  end)

  testing.case("the pumpkin witch starts a candy hunt with world_events.start", function(world)
    world:set_time("2026-10-31 15:00")
    local player = world:new_player { map = "prt_fild08", x = 199, y = 202 }
    testing.expect(#world:get_monsters(CHEST)).to_be(0)
    testing.expect(world:talk(player, "Pumpkin Witch", { 3 })).to_contain("I hid some candy chests on this field.")
    world:advance(SECOND)
    testing.expect(#world:get_monsters(CHEST)).to_be(3)
    -- It runs already.
    testing.expect(world:talk(player, "Pumpkin Witch", { 3 })).to_contain("The candy is hidden already.")
    -- It ends at its usual duration.
    world:advance(60 * MINUTE)
    testing.expect(#world:get_monsters(CHEST)).to_be(0)
    testing.expect(has(world:get_announcements(), "The treasure hunt is over. 3 chests were not found.")).to_be_true()
  end)
end
