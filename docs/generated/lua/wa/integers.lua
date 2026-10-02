---@meta wa.integers
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Integer division that rounds as C does, toward zero, for scripts converted from rAthena.
---@class wa.integers
local integers = {}

---a / b, rounded toward zero.
---@param a integer
---@param b integer Not 0.
---@return integer
function integers.divide_truncated(a, b) end

---The remainder of divide_truncated.
---@param a integer
---@param b integer Not 0.
---@return integer
function integers.remainder_truncated(a, b) end

return integers
