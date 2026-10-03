---@meta wa.channels
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Chat channels, as rAthena's channel system. Core's channels are the records of data/channels.yml, such as #global and #trade. A channel is one for the whole world.
---@class wa.channels
local channels = {}

---Makes a channel that the context owns, and returns it. The channel ends when its owner ends, and its members leave it. The same mod's context in another zone, or after a reload, gets the same channel. A name that core or another mod has raises an error.
---@param spec wa.ChannelSpec
---@return wa.Channel
function channels.create(spec) end

---A running channel: core's or a mod's. nil when none has the name.
---
---Needs the permission `game.read`.
---@param name string A channel's name, with or without its #, such as trade.
---@return wa.Channel?
function channels.get(name) end

return channels
