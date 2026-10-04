---@meta wa.world
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---The world.
---@class wa.world
local world = {}

---Shows a line to every player of the world, as announce.
---
---Needs the permission `world.announce`.
---@param text string At most 255 bytes.
function world.announce(text) end

---A shop's handle, or nil when there is none. world:get_shop(name), as review decision 30 writes it, works too.
---
---Needs the permission `game.read`.
---@param name string A shop of data/npcs.yml: a full record name, or a plain name, which is the calling mod's record, then core's.
---@return wa.Shop?
function world.get_shop(name) end

return world
