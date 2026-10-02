---@meta wa.context
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---The current instance, kind and mod. Calls outside a setup function or a handler raise an error.
---@class wa.context
local context = {}

---The instance the context owns: the mod, the NPC, the equipped copy, the used item, or the event run.
---@return wa.ModInstance|wa.Npc|wa.EquippedItem|wa.ItemUse|wa.EventInstance
function context:get_instance() end

---The context's kind.
---@return wa.ContextKind
function context:get_kind() end

---The ID of the context's mod.
---@return string
function context:get_mod() end

return context
