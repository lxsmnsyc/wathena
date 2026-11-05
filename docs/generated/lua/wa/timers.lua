---@meta wa.timers
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Timers on the zone clock, or on the server clock with { clock = "server" }. The context owns them, and cancels them when it ends.
---@class wa.timers
local timers = {}

---Runs the callback once, after the time. In an NPC context the callback may call timers.wait.
---@param milliseconds integer 0 or more.
---@param callback fun()
---@param options? wa.TimerOptions
---@return wa.Timer
function timers.after(milliseconds, callback, options) end

---Runs the callback every time the interval passes. A timer that falls behind skips the runs it missed. In an NPC context the callback may call timers.wait.
---@param milliseconds integer Above 0.
---@param callback fun()
---@param options? wa.TimerOptions
---@return wa.Timer
function timers.every(milliseconds, callback, options) end

---Waits, as sleep without a player (review decision 60). It runs in a handler of an NPC context that has no conversation: an npc_event handler sent without a player or with a player of another map, a callback of timers.after or timers.every, or a schedule handler. The wait follows the clock the handler started on: the zone clock, or the server clock for the callback of a timer with { clock = "server" } and for a schedule handler. A handler with a conversation waits with conversation:wait. Elsewhere it raises an error. The wait ends when the context ends.
---@param milliseconds integer 0 to 10 minutes.
function timers.wait(milliseconds) end

---Runs the function now. The bonuses it adds end after the time. Item contexts only.
---@param milliseconds integer
---@param add fun() Adds bonuses with bonuses.add.
---@param options? wa.WithBonusesOptions
---@return wa.Timer
function timers.with_bonuses(milliseconds, add, options) end

return timers
