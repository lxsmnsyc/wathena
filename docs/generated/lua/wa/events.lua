---@meta wa.events
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Event handlers. The context owns them, and removes them when it ends.
---@class wa.events
local events = {}

---Registers a handler for an event.
---
---Needs the permission `event.listen`.
---@param name wa.EventName The event's name. An unknown name, or an event of another context kind, raises an error.
---@param handler function Runs each time the event fires, in registration order.
---@param options wa.DeathOptions|wa.StatusEventOptions|wa.SkillUsedOptions|nil Only the death, status and skill_used events take options: they narrow which events run the handler.
---@return wa.EventHandle
---@overload fun(name: "login", handler: fun(player: wa.Player)): wa.EventHandle
---@overload fun(name: "logout", handler: fun(player: wa.Player)): wa.EventHandle
---@overload fun(name: "map_enter", handler: fun(player: wa.Player)): wa.EventHandle
---@overload fun(name: "death", handler: fun(death: wa.Death)): wa.EventHandle
---@overload fun(name: "base_level_up", handler: fun(player: wa.Player)): wa.EventHandle
---@overload fun(name: "job_level_up", handler: fun(player: wa.Player)): wa.EventHandle
---@overload fun(name: "job_change", handler: fun(change: wa.JobChange)): wa.EventHandle
---@overload fun(name: "click", handler: fun(player: wa.Player, conversation: wa.Conversation)): wa.EventHandle
---@overload fun(name: "touch", handler: fun(player: wa.Player)): wa.EventHandle
---@overload fun(name: "use", handler: fun(player: wa.Player)): wa.EventHandle
---@overload fun(name: "damage", handler: fun(hit: wa.Damage)): wa.EventHandle
---@overload fun(name: "skill_used", handler: fun(use: wa.SkillUse)): wa.EventHandle
---@overload fun(name: "cast", handler: fun(cast: wa.Cast)): wa.EventHandle
---@overload fun(name: "unequip", handler: fun(player: wa.Player)): wa.EventHandle
---@overload fun(name: "status_start", handler: fun(event: wa.StatusEvent)): wa.EventHandle
---@overload fun(name: "status_end", handler: fun(event: wa.StatusEvent)): wa.EventHandle
---@overload fun(name: "status_tick", handler: fun(event: wa.StatusEvent)): wa.EventHandle
function events.on(name, handler, options) end

return events
