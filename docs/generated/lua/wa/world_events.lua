---@meta wa.world_events
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Starting and stopping world events of data/events.yml (review decision 31). A request takes effect within a second, on the scheduler's next check.
---@class wa.world_events
local world_events = {}

---Starts a run of the event now, for its duration, as if its trigger fired. Returns false when there is no such event, or it runs already. An event the config turned off does not exist.
---
---Needs the permission `events.control`.
---@param name string An event's full name, such as core:Field_Boss, or a plain name, which is the calling mod's event.
---@return boolean
function world_events.start(name) end

---Ends the event's run now, with the result "stopped". Its monsters and NPCs go. Returns false when there is no such event, or it does not run.
---
---Needs the permission `events.control`.
---@param name string An event's full name, or a plain name, which is the calling mod's event.
---@return boolean
function world_events.stop(name) end

return world_events
