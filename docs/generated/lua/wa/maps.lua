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

---Plays background music to every player of a map, as playBGMall with a map. The map's own music comes back with each player's next map change.
---@param map string A map name, such as prontera.
---@param file string A music name of the assets, without music/, such as 08.
function maps.play_music(map, file) end

---Plays a sound to every player of a map, as soundeffectall with a map.
---@param map string A map name, such as prontera.
---@param file string A sound name of the assets, without sound/.
---@param options? wa.SoundOptions
function maps.play_sound(map, file, options) end

---Marks a cell on the minimap of every player of a map, as viewpointmap. A mark with the same ID replaces it. Players who come later do not see it.
---@param map string A map name, such as prontera.
---@param id integer
---@param x integer
---@param y integer
---@param color? integer 0xRRGGBB, white by default.
function maps.mark_minimap(map, id, x, y, color) end

---Removes a minimap mark from every player of a map, as viewpointmap with type 2.
---@param map string A map name, such as prontera.
---@param id integer
function maps.unmark_minimap(map, id) end

---Drops items on a free cell next to the cell of a map, as makeitem and makeitem2: anyone may pick them up, and they vanish after flooritem_lifetime. The map's zone drops them after the current step.
---
---Needs the permission `player.give_items`.
---@param map string A map name, such as prontera.
---@param x integer
---@param y integer
---@param item integer An item from items.id.
---@param amount integer 1 to 30000, in one pile.
---@param options? wa.DropItemOptions
function maps.drop_item(map, x, y, item, amount, options) end

return maps
