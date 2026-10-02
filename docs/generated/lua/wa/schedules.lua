---@meta wa.schedules
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Schedules on the server clock, in the server's time zone (time_zone in the config). A time that does not exist on a day clocks change runs at the next valid minute, and a time that happens twice runs once. A time that passed while the zone slept or the server was stopped runs once, or not at all. The context owns them.
---@class wa.schedules
local schedules = {}

---Runs the handler every day at the time.
---@param time string HH:MM, such as "20:00".
---@param handler fun()
---@return wa.Schedule
function schedules.daily(time, handler) end

---Runs the handler every week on the day at the time.
---@param day wa.Weekday
---@param time string HH:MM.
---@param handler fun()
---@return wa.Schedule
function schedules.weekly(day, time, handler) end

---Runs the handler every hour at the minute.
---@param minute integer 0 to 59.
---@param handler fun()
---@return wa.Schedule
function schedules.hourly(minute, handler) end

---Runs the handler once the condition has held for lasting_minutes. The condition runs once a minute, at the start of each minute of the server clock. The handler runs once for each time the condition holds.
---@param condition fun(): boolean Reads the game and returns true or false. Calls that change the game raise an error in it.
---@param handler fun()
---@param options? wa.WhenOptions
---@return wa.Schedule
function schedules.when(condition, handler, options) end

return schedules
