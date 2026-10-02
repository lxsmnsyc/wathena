---@meta wa.monsters
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Monsters by name.
---@class wa.monsters
local monsters = {}

---The monster's ID. An unknown name raises an error.
---
---Needs the permission `game.read`.
---@param name string An AegisName, such as PORING.
---@return integer
function monsters.id(name) end

return monsters
