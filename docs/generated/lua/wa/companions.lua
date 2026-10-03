---@meta wa.companions
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Companions by name: core's, such as falcon, and the mods' records of data/companions.yml.
---@class wa.companions
local companions = {}

---The companion's ID. An unknown name raises an error.
---
---Needs the permission `game.read`.
---@param name string A core companion's name, such as falcon, or a mod companion's full name, such as example.familiars:Owl.
---@return integer
function companions.id(name) end

return companions
