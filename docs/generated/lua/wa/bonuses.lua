---@meta wa.bonuses
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Item bonuses (proposal 0002a). Equip contexts only.
---@class wa.bonuses
local bonuses = {}

---Adds a bonus to the equipped copy. It ends when the copy is unequipped.
---@param name wa.BonusName The bonus name. It is checked when the call runs.
---@param ... integer|(fun(): integer)|wa.BonusOptions What the bonus applies to, the amount, other numbers, then the options table. A number can be a function: a computed value.
---@return wa.BonusHandle
function bonuses.add(name, ...) end

return bonuses
