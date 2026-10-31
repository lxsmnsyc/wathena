---@meta wa.castles
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Guild castles (proposal 0004): the records of data/castles.yml, such as core's Kriemhild, and their saved state, as rAthena's castle_db and getcastledata. A castle is one for the whole world, and each change saves at once.
---@class wa.castles
local castles = {}

---A castle, or nil.
---
---Needs the permission `game.read`.
---@param name string A record's full name, a plain name (the calling mod's, then core's), or a castle's map, such as "prtg_cas01", as rAthena's castle commands take.
---@return wa.Castle?
function castles.get(name) end

---The castles by ID, as castle_db.
---
---Needs the permission `game.read`.
---@param options? wa.CastleListOptions
---@return wa.Castle[]
function castles.list(options) end

---The castles a guild owns, by ID.
---
---Needs the permission `game.read`.
---@param guild wa.Guild
---@return wa.Castle[]
function castles.of(guild) end

return castles
