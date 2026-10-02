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

return world
