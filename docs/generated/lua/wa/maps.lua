---@meta wa.maps
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Maps.
---@class wa.maps
local maps = {}

---Shows a line to every player of a map, as mapannounce.
---@param map string A map name, such as prontera.
---@param text string At most 255 bytes.
function maps.announce(map, text) end

return maps
