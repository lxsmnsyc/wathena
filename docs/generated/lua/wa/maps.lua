---@meta wa.maps
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Maps.
---@class wa.maps
local maps = {}

---Shows a line to every player of a map, as mapannounce. Players see the mod's ID with a mod's line.
---@param map string A map name, such as prontera.
---@param text string At most 255 bytes.
function maps.announce(map, text) end

---The entities of an area of the context's map that pass every filter, sorted, and cut to the limit. Ties sort by kind, then by entity. A query with no area, two areas, or a field it does not know raises an error.
---
---Needs the permission `game.read`.
---@param query wa.EntityQuery One area, filters, and an order.
---@return (wa.Player|wa.Monster|wa.Npc)[]
function maps.find_entities(query) end

---The name players see for a map of the world layout (review decision 43), such as "North Valheim Fields 2". It changes when staff rename a town. nil for another map, and for every map of a classic world until locale files exist.
---
---Needs the permission `game.read`.
---@param map string A map ID, such as "gen:town:t7".
---@return string?
function maps.get_display_name(map) end

return maps
