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

---An online player of the context's map by name, as rAthena's map_nick2sd (review decision 58). nil for a player who is offline or on another map: a player handle acts in its own zone.
---
---Needs the permission `game.read`.
---@param name string A character name, matched exactly.
---@return wa.Player?
function maps.find_player(name) end

---An online player of the context's map by account ID, or else by character ID, as rAthena's convertpcinfo reads map_id2sd, then map_charid2sd (review decision 58). nil for a player who is offline or on another map.
---
---Needs the permission `game.read`.
---@param id integer An account ID, or a character ID, as player.account_id and player.character_id.
---@return wa.Player?
function maps.find_player_by_id(id) end

return maps
