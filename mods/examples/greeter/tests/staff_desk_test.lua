-- Tests of the staff-only NPC. world:set_staff_role gives a test
-- player a role, as if it logged in with two-factor login.

local testing = require "wa.testing"
local expect = testing.expect

return function()
  testing.case("the staff desk turns players away", function(world)
    local player = world:new_player { map = "prontera", x = 158, y = 196 }
    expect(player:get_staff_role()).to_be_nil()
    expect(world:talk(player, "Staff Desk")).to_be("[Staff Desk]\nThis desk is for staff.")
  end)

  testing.case("the staff desk talks to a moderator", function(world)
    local player = world:new_player { map = "prontera", x = 158, y = 196 }
    world:set_staff_role(player, "moderator")
    expect(player:get_staff_role()).to_be("moderator")
    expect(player:has_staff_permission("moderation.mute")).to_be(true)
    expect(player:has_staff_permission("staff.assign")).to_be(false)
    local said = world:talk(player, "Staff Desk")
    expect(said).to_contain("Your role is moderator.")
    expect(said).to_contain("The report queue is on the admin screens.")
    expect(said:find("give roles", 1, true)).to_be_nil()
  end)

  testing.case("an admin may give roles, until the role is taken away", function(world)
    local player = world:new_player { map = "prontera", x = 158, y = 196 }
    world:set_staff_role(player, "admin")
    expect(world:talk(player, "Staff Desk")).to_contain("You can give roles to other staff.")
    world:set_staff_role(player, nil)
    expect(world:talk(player, "Staff Desk")).to_contain("This desk is for staff.")
  end)

  testing.case("an unknown staff permission is an error", function(world)
    local player = world:new_player { map = "prontera", x = 158, y = 196 }
    expect(function()
      player:has_staff_permission("moderation.*")
    end).to_fail_with("there is no staff permission")
    expect(function()
      world:set_staff_role(player, "owner")
    end).to_fail_with("is not a role")
  end)
end
