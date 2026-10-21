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

---The living monsters of a map, or those of a label, as mobcount. Monsters that spawning.monsters asked another zone to place count at once. A monster label's event reads the counts of the moment its monster died, as rAthena runs the label inside mob_dead and inside killmonster's loop. nil for a map that does not run in this world.
---
---Needs the permission `game.read`.
---@param map string A map name, such as prontera.
---@param options? wa.CountMonstersOptions
---@return integer?
function maps.count_monsters(map, options) end

---Kills monsters of a map with no exp and no drops, as killmonster and killmonsterall (review decision 58). Without options, every monster dies, and those of spawn lines come back. Their labels do not run unless run_labels is set. Another map's zone kills them after the current step.
---
---Needs the permission `maps.control`.
---@param map string A map name, such as prontera.
---@param options? wa.RemoveMonstersOptions
function maps.remove_monsters(map, options) end

---Moves the players of a map, as mapwarp and areawarp (review decision 58). A cell that is not on the target map is a random one. to_map "random" moves each player to a random cell of its own map, as areawarp's "Random" (x and y are left out). Dead players move too, as rAthena's: they lie dead on the new cell and return to their save point after the rest of their wait. The map's zone moves them after the current step.
---
---Needs the permission `player.warp`.
---@param map string The map the players are on.
---@param to_map string The map they go to.
---@param x integer
---@param y integer
---@param options? wa.WarpPlayersOptions
function maps.warp_players(map, to_map, x, y, options) end

---Turns a kind of cell on or off in a rectangle, as setcell (review decision 58). walkable and shootable change where units walk and shoot. The players' clients are not told, as rAthena's setcell sends nothing; with send, the map's players get the cells, and players who enter later get them too, as setwall sends a wall's cells. no_vending and no_buying_store refuse shops there. Kinds whose systems do not exist raise an error. The map's zone changes them after the current step.
---
---Needs the permission `maps.control`.
---@param map string A map name, such as prontera.
---@param x1 integer
---@param y1 integer
---@param x2 integer
---@param y2 integer The rectangle's opposite corner.
---@param kind wa.CellKind
---@param on boolean
---@param options? wa.SetCellsOptions
function maps.set_cells(map, x1, y1, x2, y2, kind, on, options) end

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

---The entities of an area of the context's map, or of the query's map, that pass every filter, sorted, and cut to the limit. Ties sort by kind, then by entity. On another map, the entities are those of the zone's last step, as the world keeps them: the area is x and y with radius, a rectangle or cells, and the query has no entity filters. A query with no area, two areas, or a field it does not know raises an error.
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

---An online player of any map by name, as rAthena's map_nick2sd (review decisions 58 and 59). nil for a player who is offline. A player of another map is a handle that reads the fields the world keeps and sends its actions to the player's map.
---
---Needs the permission `game.read`.
---@param name string A character name, matched exactly.
---@return wa.Player?
function maps.find_player(name) end

---An online player of any map by account ID, or else by character ID, as rAthena's convertpcinfo reads map_id2sd, then map_charid2sd (review decisions 58 and 59). nil for a player who is offline.
---
---Needs the permission `game.read`.
---@param id integer An account ID, or a character ID, as player.account_id and player.character_id.
---@return wa.Player?
function maps.find_player_by_id(id) end

---A living monster of any map by its game ID, as rAthena's map_id2md reads a GID of $@mobid (review decision 59). nil once it died. A monster of another map is a handle that reads what its zone shares and sends kill, remove, set_hp, set_damage_immune, set_modes, set_speed, use_skill_at and warp_to to its map.
---
---Needs the permission `game.read`.
---@param id integer A game ID, as monster.game_id.
---@return wa.Monster?
function maps.find_monster(id) end

---Plays background music to every player of a map, or of a rectangle of it, as playBGMall with a map. The map's own music comes back with each player's next map change.
---@param map string A map name, such as prontera.
---@param file string A music name of the assets, without music/, such as 08.
---@param options? wa.MusicOptions
function maps.play_music(map, file, options) end

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
