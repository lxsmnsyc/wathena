---@meta wa.items
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Items by name.
---@class wa.items
local items = {}

---The item's ID. An unknown name raises an error.
---
---Needs the permission `game.read`.
---@param name string An AegisName, such as Red_Potion, or a mod item's full name.
---@return integer
function items.id(name) end

---An item group, for player:give_item_from_group.
---
---Needs the permission `game.read`.
---@param name string The group's name in item_group_db, such as BLUEBOX.
---@return integer
function items.group(name) end

---The item's name from rAthena's item database.
---
---Needs the permission `game.read`.
---@param item integer An item from items.id.
---@return string
function items.name(item) end

return items
