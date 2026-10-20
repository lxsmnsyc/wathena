---@meta wa.rules
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---The world's ruleset (strategy section 3.6, review decisions 27 and 32). Read-only. Its calls need no permission. An unknown name raises an error that lists the valid names.
---@class wa.rules
---@field limits wa.Limits The limits that rAthena compiles in, as the ruleset sets them, such as rules.limits.party_members. Each is the limit's base.
local rules = {}

---Whether a ruleset flag is on: true at its renewal value or at on. A skill_rebalance flag is the flag the skill follows: its own, or the base.
---@param flag string A ruleset flag, such as "cast_time", or "skill_rebalance.<skill>" for a rebalanced skill or a song, such as "skill_rebalance.MG_FIREBALL".
---@return boolean
function rules.is(flag) end

---The value of a ruleset rule: a boolean for a rule that is yes or no, else an integer. A server setting is the number getbattleflag reads (review decision 58): the exp rates in percent, and the highest look values of player:set_look.
---@param name string A ruleset rule, such as "basic_skill_check" or "party_share_level", or a server setting: base_exp_rate, job_exp_rate, quest_exp_rate, max_hair_style, max_hair_color or max_cloth_color.
---@return integer|boolean
function rules.get(name) end

---A limit's base, what a player or guild starts with, and its max, where expansions stop. rules.limits holds the base too.
---@param name string A limit, such as "inventory_slots".
---@return integer, integer
function rules.get_limit(name) end

return rules
