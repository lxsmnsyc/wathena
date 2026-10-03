---@meta wa.hooks
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---Hooks change a game result in a chain ordered by priority. Each hook calls next to get the rest of the chain's result. The innermost result is rAthena's value. Hooks only compute: calls that change the game raise an error in them. The context owns them.
---@class wa.hooks
local hooks = {}

---Adds a hook. Higher priority runs first; equal priorities run in the order they were added.
---
---Needs the permission `rules.override`.
---@param name wa.HookName The hook's name. An unknown name raises an error.
---@param hook? fun(object: wa.Drop|wa.Exp|wa.StatusHook|wa.SkillHook|wa.MonsterTargetChoice, next: fun(object?: table): integer|boolean|wa.Player|wa.Monster|wa.Npc|nil, integer?): integer|boolean|wa.Player|wa.Monster|wa.Npc|nil, integer Gets the object and next. drop_rate returns a rate per 10,000; experience returns base and job exp; status_chance returns a chance per 10,000; status_duration and cast_time return milliseconds; skill_damage returns the damage percentage; skill_cost returns SP; cast_allowed returns true or false; monster_target returns one of the candidates, or nil for none.
---@param options? wa.HookOptions
---@return wa.HookHandle
function hooks.on(name, hook, options) end

return hooks
