---@meta wa.text
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Text for players, from the mod's locales/en.yml. Call the module with a key: text "greeter.hello".
---@class wa.text
---@overload fun(key: string): string
local text = {}

---A link that conversation:say shows as the item's name, as mesitemlink.
---@param item integer An item from items.id.
---@return string
function text.item_link(item) end

return text
