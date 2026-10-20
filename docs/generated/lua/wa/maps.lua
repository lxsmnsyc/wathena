---@meta wa.maps
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Maps.
---@class wa.maps
local maps = {}

---Shows a line to every player of a map, as mapannounce, or of a rectangle of it, as areaannounce. Players see the mod's ID with a mod's line.
---@param map string A map name, such as prontera.
---@param text string At most 255 bytes.
---@param options? wa.AnnounceOptions
function maps.announce(map, text, options) end

---The players of a map, as getmapusers, or of a rectangle of it, as getareausers (review decision 58). nil for a map that does not run in this world. Another map's count is from its zone's last step.
---
---Needs the permission `game.read`.
---@param map string A map name, such as prontera.
---@param options? wa.AreaOptions
---@return integer?
function maps.count_players(map, options) end

---The living monsters of a map, or those of a label, as mobcount. Monsters that spawning.monsters asked another zone to place count at once. nil for a map that does not run in this world.
---
---Needs the permission `game.read`.
---
---Provisional: not reviewed in proposal 0002 yet.
---@param map string A map name, such as prontera.
---@param options? wa.CountMonstersOptions
---@return integer?
function maps.count_monsters(map, options) end

---Kills monsters of a map with no exp and no drops, as killmonster and killmonsterall (review decision 58). Without options, every monster dies, and those of spawn lines come back. Their labels do not run unless run_labels is set. Another map's zone kills them after the current step.
---
---Needs the permission `npc.create`.
---@param map string A map name, such as prontera.
---@param options? wa.RemoveMonstersOptions
function maps.remove_monsters(map, options) end

---Moves the players of a map, as mapwarp and areawarp (review decision 58). A cell that is not on the target map is a random one. Dead players stay. The map's zone moves them after the current step.
---
---Needs the permission `player.warp`.
---@param map string The map the players are on.
---@param to_map string The map they go to.
---@param x integer
---@param y integer
---@param options? wa.WarpPlayersOptions
function maps.warp_players(map, to_map, x, y, options) end

---Turns a kind of cell on or off in a rectangle, as setcell (review decision 58). walkable and shootable change where units walk and shoot, and the map's players get the cells, as setwall sends them; players who enter later get them too. no_vending and no_buying_store refuse shops there. Kinds whose systems do not exist raise an error. The map's zone changes them after the current step.
---
---Needs the permission `npc.create`.
---@param map string A map name, such as prontera.
---@param x1 integer
---@param y1 integer
---@param x2 integer
---@param y2 integer The rectangle's opposite corner.
---@param kind wa.CellKind
---@param on boolean
function maps.set_cells(map, x1, y1, x2, y2, kind, on) end

---Reads a cell, as checkcell's map_getcellp: a cell outside the map, or on its last row or column, is no_pass and nothing else. npc reads the touch areas of the context's own map. Kinds whose systems do not exist raise an error.
---
---Needs the permission `game.read`.
---@param map string A map name, such as prontera.
---@param x integer
---@param y integer
---@param kind wa.CellCheck
---@return boolean
function maps.check_cell(map, x, y, kind) end

---Sets a map flag, as setmapflag and removemapflag (review decision 58). nomemo is the flag wAthena has. Other flags raise an error that says why: their systems do not exist, or their effects are not built. The map's zone sets it after the current step.
---@param map string A map name, such as prontera.
---@param flag string rAthena's name without MF_, such as nomemo.
---@param value boolean false removes it, as removemapflag.
function maps.set_flag(map, flag, value) end

---Reads a map flag, as getmapflag: a script's, or else the world layout's. A flag that set_flag refuses is false.
---
---Needs the permission `game.read`.
---
---Provisional: not reviewed in proposal 0002 yet.
---@param map string A map name, such as prontera.
---@param flag string rAthena's name without MF_, such as nomemo.
---@return boolean
function maps.get_flag(map, flag) end

---A random walkable cell, as getfreecell's map_search_freecell: 50 tries within the range of x and y, never x and y themselves, or anywhere at least 15 cells from the map's edges. With no range, x and y themselves when walkable. nil when no try found one.
---
---Needs the permission `game.read`.
---@param map string A map name, such as prontera.
---@param options? wa.FreeCellOptions
---@return integer?, integer?
function maps.find_free_cell(map, options) end

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
