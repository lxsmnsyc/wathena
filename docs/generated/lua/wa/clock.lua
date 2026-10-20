---@meta wa.clock
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---The server clock (review decision 58), as rAthena's gettimetick, gettime and gettimestr. Fields and formats are in the server's time zone (time_zone in the config). Its calls need no permission.
---@class wa.clock
local clock = {}

---Unix seconds, as gettimetick(2).
---@return integer
function clock.now() end

---Unix milliseconds, for measuring time, as gettimetick(0).
---@return integer
function clock.milliseconds() end

---A part of the date or time, as gettime: second, minute, hour, day_of_week (0 is Sunday), day_of_month, month (1 to 12), year, day_of_year (0 is January 1st) and date (YYYYMMDD).
---@param field wa.ClockField
---@return integer
function clock.get(field) end

---The time as text, as gettimestr's strftime in the C locale. A conversion that strftime does not have raises an error.
---@param format string At most 255 bytes, with the conversions of C's strftime, such as %Y-%m-%d %H:%M:%S.
---@param seconds? integer A time in Unix seconds. Now by default.
---@return string
function clock.format(format, seconds) end

return clock
