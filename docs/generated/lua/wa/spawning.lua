---@meta wa.spawning
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Monsters that the context owns. They go when it ends. Monsters of another map belong to no context.
---@class wa.spawning
local spawning = {}

---Places monsters on a map and returns their handles, as rAthena's monster and areamonster. Without x and y, each lands on a random walkable cell away from the map's edges, as map_search_freecell. On another map, the map's zone places them after the current step, and the list is empty: maps.count_monsters counts them at once (review decision 58).
---
---Needs the permission `npc.create`.
---@param spec wa.SpawnSpec
---@return wa.Monster[]
function spawning.monsters(spec) end

return spawning
