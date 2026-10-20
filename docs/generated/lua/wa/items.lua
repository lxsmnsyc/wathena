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

---The item's record, as getiteminfo and getitemslots, or nil for an unknown item.
---
---Needs the permission `game.read`.
---@param item integer An item from items.id.
---@return wa.ItemInfo?
function items.get(item) end

---Whether the item can be refined, as getequipisenableref: a weapon, an armor or shadow gear that is not no_refine. A worn copy that is a rental cannot.
---
---Needs the permission `game.read`.
---@param item integer|wa.EquippedItem An item from items.id, or a worn copy.
---@return boolean
function items.can_refine(item) end

---The chance in percent to refine the item from refine to refine + 1, as getequippercentrefinery, from refine.yml. 0 at MAX_REFINE or for an item that is not refined. The data holds the Normal chances alone: enriched raises an error.
---
---Needs the permission `game.read`.
---@param item integer|wa.EquippedItem An item from items.id, or a worn copy.
---@param refine? integer The refine it has. A worn copy's own by default.
---@param options? wa.RefineChanceOptions
---@return integer
function items.refine_chance(item, refine, options) end

---The zeny and the material's item ID of refining the item from refine to refine + 1, as getequiprefinecost, from refine.yml. nil when the data has no cost. The data holds the Normal costs alone: hd and enriched raise an error.
---
---Needs the permission `game.read`.
---@param item integer|wa.EquippedItem An item from items.id, or a worn copy.
---@param refine? integer The refine it has. A worn copy's own by default.
---@param kind? wa.RefineKind normal by default.
---@return integer?, integer?
function items.refine_cost(item, refine, kind) end

return items
