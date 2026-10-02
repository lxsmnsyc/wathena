---@meta wa.random
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Random numbers from the zone's secret stream.
---@class wa.random
local random = {}

---A whole number from minimum to maximum.
---@param minimum integer
---@param maximum integer
---@return integer
function random.integer(minimum, maximum) end

---True with a chance of numerator in denominator.
---@param numerator integer
---@param denominator integer Above 0.
---@return boolean
function random.chance(numerator, denominator) end

return random
