---@meta wa.mounts
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Mounts by name: core's, such as peco_peco, and the mods' records of data/mounts.yml.
---@class wa.mounts
local mounts = {}

---The mount's ID. An unknown name raises an error.
---
---Needs the permission `game.read`.
---@param name string A core mount's name, such as peco_peco, grand_peco, gryphon, dragon, warg or madogear, or a mod mount's full name.
---@return integer
function mounts.id(name) end

return mounts
