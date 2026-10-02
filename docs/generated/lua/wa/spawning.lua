---@meta wa.spawning
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Monsters that the context owns. They go when it ends.
---@class wa.spawning
local spawning = {}

---Places monsters on the context's map and returns their handles. Without x and y, each lands on a random walkable cell.
---
---Needs the permission `npc.create`.
---@param spec wa.SpawnSpec
---@return wa.Monster[]
function spawning.monsters(spec) end

return spawning
