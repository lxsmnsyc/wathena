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

---The monster's record, as getmonsterinfo (review decision 58), or nil for an unknown monster. Exp is after the server's rates, as rAthena applies them when it loads mob_db.
---
---Needs the permission `game.read`.
---@param monster integer From monsters.id.
---@return wa.MonsterInfo?
function monsters.get(monster) end

return monsters
