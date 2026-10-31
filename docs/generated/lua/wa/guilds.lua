---@meta wa.guilds
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Guilds, as rAthena's guild system (review decision 28). A guild is one for the whole world. The write calls skip the position permissions and the invite, as rAthena's guild script commands. Castles are in wa.castles.
---@class wa.guilds
local guilds = {}

---Makes a guild with the player as its master and returns it. It takes no Emperium, so the script decides the cost. nil when the name is taken or the master has a guild. The guild_join event follows, with the reason "script". A test case needs no permission for it.
---
---Needs the permission `guild.manage`.
---@param master wa.Player
---@param name string 1 to 24 characters.
---@return wa.Guild?
function guilds.create(master, name) end

---A guild by ID, or nil.
---
---Needs the permission `game.read`.
---@param id integer A guild's ID, such as player.guild.id.
---@return wa.Guild?
function guilds.get(id) end

---A guild by name, or nil.
---
---Needs the permission `game.read`.
---@param name string A guild's name, without regard to case.
---@return wa.Guild?
function guilds.find(name) end

return guilds
