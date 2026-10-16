---@meta wa.places
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Named places of generated maps (review decision 42): cells with tags, such as blacksmith, square, gate, entrance, exit or boss_room, that the map generator records. Content finds places by tag, not by coordinates. A classic world has no places: find returns an empty list and nearest nil.
---@class wa.places
local places = {}

---Every place of the scope with the tag, in a fixed order: by map ID, then in the order the generator made them. Empty when none matches.
---
---Needs the permission `game.read`.
---@param scope string Where to look: "town:any", "town:<id>" such as "town:t7", "dungeon:any", "dungeon:<id>" such as "dungeon:crypt", or "map:<id>" such as "map:gen:field:3:-1". Another form raises an error.
---@param tag string A tag, such as "blacksmith" or "shop:tool": 1 to 64 lowercase letters, digits, _, -, . and :.
---@return wa.Place[]
function places.find(scope, tag) end

---The place with the tag closest to the player: the fewest warps away on the map graph, then the shortest distance. On another map, the distance counts from where the player would arrive. Maps no warp leads to are left out. nil when no place qualifies.
---
---Needs the permission `game.read`.
---@param player wa.Player A player handle.
---@param tag string A tag, as for find.
---@return wa.Place?
function places.nearest(player, tag) end

return places
