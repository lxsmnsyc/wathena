---@meta wa.timers
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Timers on the zone clock. The context owns them, and cancels them when it ends.
---@class wa.timers
local timers = {}

---Runs the callback once, after the time.
---@param milliseconds integer 0 or more.
---@param callback fun()
---@return wa.Timer
function timers.after(milliseconds, callback) end

---Runs the callback every time the interval passes. A timer that falls behind skips the runs it missed.
---@param milliseconds integer Above 0.
---@param callback fun()
---@return wa.Timer
function timers.every(milliseconds, callback) end

---Runs the function now. The bonuses it adds end after the time. Item contexts only.
---@param milliseconds integer
---@param add fun() Adds bonuses with bonuses.add.
---@param options? wa.WithBonusesOptions
---@return wa.Timer
function timers.with_bonuses(milliseconds, add, options) end

return timers
