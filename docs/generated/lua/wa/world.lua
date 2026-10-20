---@meta wa.world
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---The world.
---@class wa.world
local world = {}

---Shows a line to every player of the world, as announce. Players see the mod's ID with a mod's line.
---
---Needs the permission `world.announce`.
---@param text string At most 255 bytes.
function world.announce(text) end

---A shop's handle, or nil when there is none. world:get_shop(name), as review decision 30 writes it, works too.
---
---Needs the permission `game.read`.
---@param name string A shop of data/npcs.yml: a full record name, or a plain name, which is the calling mod's record, then core's.
---@return wa.Shop?
function world.get_shop(name) end

---An NPC of any map, or one without a map, as rAthena's npc_name2id (review decision 58). nil when there is none, or it was removed. A handle of an NPC of another map reads its fields, and its changes and events run in that NPC's zone after the current step. Its other methods raise an error there.
---
---Needs the permission `game.read`.
---@param name string An NPC of data/npcs.yml: a full record name, such as core:Guard#1, or a plain name, which is the calling mod's record, then core's. Case counts.
---@return wa.Npc?
function world.get_npc(name) end

---Sends the event to every NPC with an npc_event handler of the label, on every map, as donpcevent "::OnX" (npc_event_doall_sub). The handlers run with no player, after the current step, each in its NPC's zone. Without npcs.control it reaches only the mod's own NPCs. Returns how many NPCs have a handler of the label, of the zones that started their NPC scripts. The event still starts the NPC scripts of a zone that did not, and reaches its NPCs.
---@param name string An event label, such as OnEnable, without regard to case.
---@return integer
function world.fire_npc_event(name) end

return world
