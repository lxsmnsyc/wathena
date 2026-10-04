-- Tests of the Guild Hall mod, which wactl mod test runs. Each case gets
-- a fresh world with this mod and core loaded.

local testing = require "wa.testing"
local guilds = require "wa.guilds"
local rules = require "wa.rules"
local skills = require "wa.skills"
local expect = testing.expect

local function texts(world, player)
  local out = {}
  for _, line in ipairs(world:get_chat(player)) do
    out[#out + 1] = line.channel .. " " .. line.from .. ": " .. line.text
  end
  return out
end

return function()
  testing.case("!guild makes a guild with the player as its master", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    world:chat(alice, "!guild")
    world:advance(100)
    local guild = alice.guild
    expect(guild == nil).to_be_false()
    expect(guild.name).to_be(alice.name .. "'s Guild")
    expect(guild.master).to_be(alice)
    expect(guild.master_name).to_be(alice.name)
    expect(guild.level).to_be(1)
    expect(guild.exp).to_be(0)
    expect(guild.member_count).to_be(1)
    expect(guild:get_member_names()).to_equal({ alice.name })
    expect(guild:get_position(alice).name).to_be("GuildMaster")
    expect(guild:get_position(alice).can_invite).to_be_true()
    expect(texts(world, alice)).to_contain("whisper Guild Hall: Welcome to " .. guild.name .. ", GuildMaster. " .. alice.name .. " is the master.")
    expect(guilds.find(guild.name)).to_be(guild)
    expect(guilds.get(guild.id)).to_be(guild)
  end)

  testing.case("a second !guild is refused", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    world:chat(alice, "!guild")
    world:advance(100)
    world:chat(alice, "!guild")
    world:advance(100)
    expect(texts(world, alice)).to_contain("whisper Guild Hall: You are in " .. alice.guild.name .. " already.")
  end)

  testing.case("members hear the guild events", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    local bob = world:new_player { map = "prontera", x = 152, y = 150 }
    local guild = guilds.create(alice, "Testers")
    expect(guild:add(bob)).to_be_true()
    world:advance(100)
    expect(texts(world, bob)).to_contain("whisper Guild Hall: Welcome to Testers, Newbie. " .. alice.name .. " is the master.")
    expect(guild:get_position(bob).exp_tax).to_be(0)
    expect(#guild:get_members()).to_be(2)
    -- 500,000 exp: level 3 with renewal's table, 0 with pre-renewal's 2.
    guild:give_exp(500000)
    world:advance(100)
    expect(guild.level >= 2).to_be_true()
    expect(texts(world, bob)).to_contain("system : Testers: level " .. guild.level .. ".")
    expect(guild:get_skill_level(skills.id("GD_EXTENSION"))).to_be(0)
    expect(guild:set_master(bob)).to_be_true()
    world:advance(100)
    expect(guild.master).to_be(bob)
    expect(texts(world, alice)).to_contain("system : Testers: " .. bob.name .. " is the new master.")
    expect(guild:remove(alice)).to_be_true()
    world:advance(100)
    expect(alice.guild).to_be_nil()
    expect(texts(world, alice)).to_contain("whisper Guild Hall: You left Testers (script).")
  end)

  testing.case("!notice sets the notice for the master only", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    local bob = world:new_player { map = "prontera", x = 152, y = 150 }
    local guild = guilds.create(alice, "Notices")
    guild:add(bob)
    world:chat(bob, "!notice hello")
    world:chat(alice, "!notice Meet at the fountain")
    world:advance(100)
    expect(guild.notice.title).to_be("From the master")
    expect(guild.notice.text).to_be("Meet at the fountain")
    expect(texts(world, bob)).to_contain("whisper Guild Hall: Only a guild master sets the notice.")
    expect(texts(world, bob)).to_contain("system : Notices: notice: Meet at the fountain")
  end)

  testing.case("a disbanded guild still reads its name", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    local other = world:new_player { map = "prontera", x = 152, y = 150 }
    local guild = guilds.create(alice, "Short Lived")
    local rival = guilds.create(other, "Rivals")
    expect(guild:is_allied(rival)).to_be_false()
    expect(guild:is_enemy(rival)).to_be_false()
    guild:disband()
    world:advance(100)
    expect(alice.guild).to_be_nil()
    expect(guild.name).to_be("Short Lived")
    expect(guilds.find("Short Lived")).to_be_nil()
    expect(texts(world, alice)).to_contain("whisper Guild Hall: You left Short Lived (disbanded).")
  end)

  testing.case("the limits come from the ruleset", function(world)
    local alice = world:new_player { map = "prontera", x = 150, y = 150 }
    expect(rules.limits.guild_members).to_be(16)
    expect(rules.limits.guild_positions).to_be(20)
    expect(rules.limits.guild_allies).to_be(3)
    expect(rules.limits.guild_enemies).to_be(3)
    expect(rules.limits.guild_storage).to_be(600)
    world:chat(alice, "!guild size")
    expect(texts(world, alice)).to_contain("whisper Guild Hall: A guild holds 16 members, and 6 more for each level of Guild Extension.")
  end)
end
