---@meta wa.towns
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---The towns of the world layout (review decision 43): each with an ID such as t7, a display name, a map and a level band. A classic world has no towns: list returns an empty list, and nearest and get nil.
---@class wa.towns
local towns = {}

---Every town by number, or with visited_by only the towns that player entered.
---
---Needs the permission `game.read`.
---@param options? wa.TownListOptions
---@return wa.Town[]
function towns.list(options) end

---The town the fewest warps from the player's map, ties to the lower number: the player's own town when it stands in one. nil when no town is reached.
---
---Needs the permission `game.read`.
---@param player wa.Player A player handle.
---@return wa.Town?
function towns.nearest(player) end

---A town by its ID, or nil when the world has none with it.
---
---Needs the permission `game.read`.
---@param id string A town's ID, such as "t7". Another form raises an error.
---@return wa.Town?
function towns.get(id) end

return towns
