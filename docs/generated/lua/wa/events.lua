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
---@param options wa.DeathOptions|wa.StatusEventOptions|wa.SkillUsedOptions|wa.ChatOptions|wa.TradeOptions|nil Only the death, status, skill_used, chat and trade events take options: they narrow which events run the handler.
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
---@overload fun(name: "chat", handler: fun(message: wa.ChatMessage)): wa.EventHandle
---@overload fun(name: "party_join", handler: fun(join: wa.PartyMembership)): wa.EventHandle
---@overload fun(name: "party_leave", handler: fun(leave: wa.PartyMembership)): wa.EventHandle
---@overload fun(name: "party_change", handler: fun(change: wa.PartyChange)): wa.EventHandle
---@overload fun(name: "guild_join", handler: fun(join: wa.GuildMembership)): wa.EventHandle
---@overload fun(name: "guild_leave", handler: fun(leave: wa.GuildMembership)): wa.EventHandle
---@overload fun(name: "guild_change", handler: fun(change: wa.GuildChange)): wa.EventHandle
---@overload fun(name: "trade", handler: fun(trade: wa.Trade)): wa.EventHandle
---@overload fun(name: "shop_buy", handler: fun(sale: wa.ShopSale)): wa.EventHandle
---@overload fun(name: "shop_sell", handler: fun(sale: wa.ShopSale)): wa.EventHandle
---@overload fun(name: "vend", handler: fun(sale: wa.StoreSale)): wa.EventHandle
---@overload fun(name: "buying_store_sale", handler: fun(sale: wa.StoreSale)): wa.EventHandle
---@overload fun(name: "mail", handler: fun(mail: wa.Mail)): wa.EventHandle
---@overload fun(name: "world_event_start", handler: fun(event: wa.WorldEvent)): wa.EventHandle
---@overload fun(name: "world_event_end", handler: fun(event: wa.WorldEvent)): wa.EventHandle
---@overload fun(name: "restart_planned", handler: fun(restart: wa.RestartPlan)): wa.EventHandle
function events.on(name, handler, options) end

return events
