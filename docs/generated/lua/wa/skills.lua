---@meta wa.skills
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Skills by name.
---@class wa.skills
local skills = {}

---The skill's ID. An unknown name raises an error.
---
---Needs the permission `game.read`.
---@param name string A skill_db name, such as SM_BASH, or a mod skill's full name, such as example.chain-lightning:CHAIN_LIGHTNING.
---@return integer
function skills.id(name) end

return skills
