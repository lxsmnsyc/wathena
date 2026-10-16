-- Helpers for scripts converted from rAthena (tools/convert). Generated
-- NPC scripts and functions require this module as core.lib.rathena.
-- It gives what rAthena's script language has and the API does not
-- need: arrays that start at 0, menus written as "A:B", and the end of a
-- script from inside a function.

local events = require "wa.events"
local spawning = require "wa.spawning"

local R = {}

-- STOP ends a converted script from any depth, as rAthena's end does
-- inside a callfunc. R.run catches it.
local STOP = setmetatable({}, {
	__tostring = function()
		return "the script ended"
	end,
})

-- run calls a converted handler. An end inside a function stops it
-- without an error.
function R.run(fn, ...)
	local ok, err = pcall(fn, ...)
	if not ok and err ~= STOP then
		error(err, 0)
	end
end

-- stop is rAthena's end, and close, inside a function.
function R.stop()
	error(STOP, 0)
end

-- unsupported stands for a part of a function that does not convert. It
-- fails only when a script reaches it.
function R.unsupported(what)
	error("this NPC needs " .. what .. ", which wAthena does not have yet", 2)
end

-- not_available answers a service that wAthena does not have yet, such as
-- pushcart rental. It returns 1, as the functions it replaces do.
function R.not_available(conversation, what)
	conversation:say(what .. " is not available yet.")
	return 1
end

-- truthy is rAthena's test of a value: not 0 and not "".
function R.truthy(v)
	return v ~= nil and v ~= 0 and v ~= ""
end

-- nonzero is a number, or nil for 0, for calls that take nil for none,
-- such as player:set_cart.
function R.nonzero(v)
	if v == 0 then
		return nil
	end
	return v
end

-- menu splits rAthena's menu text, "A:B:C", into options.
function R.menu(text)
	local out = {}
	for part in (tostring(text) .. ":"):gmatch("(.-):") do
		out[#out + 1] = part
	end
	return table.unpack(out)
end

-- arg is getarg(n, default): the nth argument from 0, or the default
-- when the caller gave fewer.
function R.arg(args, n, default)
	if n < args.n then
		return args[n + 1]
	end
	if default == nil then
		error("getarg(" .. n .. "): the function got " .. args.n .. " arguments", 2)
	end
	return default
end

-- Arrays are Lua tables with keys from 0, as rAthena's arrays.

-- size is getarraysize: one more than the last index that holds a value.
function R.size(t)
	local n = 0
	for k, v in pairs(t) do
		if math.type(k) == "integer" and k >= n and v ~= 0 and v ~= "" then
			n = k + 1
		end
	end
	return n
end

-- setarray sets elements from start on.
function R.setarray(t, start, ...)
	for i = 1, select("#", ...) do
		t[start + i - 1] = (select(i, ...))
	end
end

-- deletearray removes count elements from start, and moves the rest down.
-- Without count, it removes everything from start on.
function R.deletearray(t, start, count)
	local size = R.size(t)
	if count == nil or start + count > size then
		count = size - start
	end
	if count <= 0 then
		return
	end
	for i = start, size - 1 do
		t[i] = t[i + count]
	end
end

-- implode joins the elements from 0 with a separator.
function R.implode(t, sep)
	local parts = {}
	for i = 0, R.size(t) - 1 do
		local v = t[i]
		if v == nil then
			v = ""
		end
		parts[#parts + 1] = tostring(v)
	end
	return table.concat(parts, sep or "")
end

-- Arrays in saved and temporary variables are tables with string keys,
-- since variables store plain data. get and put turn them into tables
-- with keys from 0 and back. A server variable has no player.

local function load(var, player)
	local stored
	if player == nil then
		stored = var:get()
	else
		stored = var:get(player)
	end
	local t = {}
	if stored ~= nil then
		for k, v in pairs(stored) do
			local i = math.tointeger(tonumber(k))
			if i ~= nil then
				t[i] = v
			end
		end
	end
	return t
end

local function store(var, player, t)
	local out
	for k, v in pairs(t) do
		if v ~= 0 and v ~= "" then
			out = out or {}
			out[tostring(k)] = v
		end
	end
	if player == nil then
		var:set(out)
	else
		var:set(player, out)
	end
end

R.load = load
R.store = store

-- aget reads one element of a variable's array.
function R.aget(var, player, i, default)
	local v = load(var, player)[i]
	if v == nil then
		return default
	end
	return v
end

-- aset writes one element of a variable's array.
function R.aset(var, player, i, value)
	local t = load(var, player)
	t[i] = value
	store(var, player, t)
end

-- asetarray, adelete, asize and aimplode are setarray, deletearray,
-- getarraysize and implode on a variable's array.
function R.asetarray(var, player, start, ...)
	local t = load(var, player)
	R.setarray(t, start, ...)
	store(var, player, t)
end

function R.adelete(var, player, start, count)
	local t = load(var, player)
	R.deletearray(t, start, count)
	store(var, player, t)
end

function R.asize(var, player)
	return R.size(load(var, player))
end

function R.aimplode(var, player, sep)
	return R.implode(load(var, player), sep)
end

-- give_item and take_item are getitem and delitem with a computed
-- amount, and the storage and bound forms with their options. rAthena
-- does nothing for an amount of 0 or less, where wAthena's calls raise
-- an error.
function R.give_item(player, item, amount, options)
	if amount <= 0 then
		return true
	end
	return player:give_item(item, amount, options)
end

function R.take_item(player, item, amount, options)
	if amount <= 0 then
		return true
	end
	return player:take_item(item, amount, options)
end

-- add_currency changes a currency by an amount with a sign, as rAthena's
-- #CASHPOINTS += n with a computed n. It gives for an amount above 0 and
-- takes for one below 0. It returns false when the player has too
-- little to take, and then changes nothing.
function R.add_currency(player, name, n)
	if n > 0 then
		player:give_currency(name, n)
	elseif n < 0 then
		return player:take_currency(name, -n)
	end
	return true
end

-- set_currency sets a currency to an amount, as rAthena's
-- set #CASHPOINTS, n. It gives or takes the difference. An amount below
-- 0 is 0.
function R.set_currency(player, name, n)
	if n < 0 then
		n = 0
	end
	return R.add_currency(player, name, n - player:get_currency(name))
end

-- mail_items is the items of rAthena's mail command for
-- player:send_mail: the item IDs and the amounts, arrays from 0. It
-- takes at most 5 items, as MAIL_MAX_ITEM.
function R.mail_items(ids, amounts)
	local out = {}
	for i = 0, R.size(ids) - 1 do
		if #out == 5 then
			break
		end
		out[#out + 1] = { item = ids[i], amount = amounts[i] or 0 }
	end
	return out
end

-- Monsters of rAthena's monster command. alive counts the living
-- monsters of each NPC by event label, for mobcount. Its keys are weak,
-- so an NPC that ends leaves it.
local alive = setmetatable({}, { __mode = "k" })

-- own_map checks that a command works on the NPC's own map, the only
-- map where it can spawn monsters.
local function own_map(npc, what, map)
	if map ~= npc.map then
		error(what .. ": this NPC is on " .. npc.map .. " and works on its own map only, not on " .. tostring(map), 3)
	end
end

-- monster is rAthena's monster command on the NPC's own map. A cell of
-- 0 or less places each monster on a random cell, as rAthena does. With
-- a label, on_death runs when each monster dies: with the player that
-- killed it, or with nil. The death handler goes once all have died.
function R.monster(npc, map, x, y, mob, amount, label, on_death)
	own_map(npc, "monster", map)
	if amount <= 0 then
		return
	end
	local spec = { monster = mob, amount = amount }
	if x > 0 and y > 0 then
		spec.x, spec.y = x, y
	end
	local list = spawning.monsters(spec)
	if label == nil or #list == 0 then
		return
	end
	local counts = alive[npc]
	if counts == nil then
		counts = {}
		alive[npc] = counts
	end
	counts[label] = (counts[label] or 0) + #list
	local left = #list
	local handle
	handle = events.on("death", function(death)
		counts[label] = counts[label] - 1
		left = left - 1
		if left == 0 then
			handle:remove()
		end
		-- Only a player is attached to the event, as rAthena's RID.
		local killer = death.killer
		if killer ~= nil and killer.kind ~= "player" then
			killer = nil
		end
		on_death(killer)
	end, { victims = list })
end

-- mobcount is rAthena's mobcount for one event label of the NPC: its
-- monsters with that label that live.
function R.mobcount(npc, map, label)
	own_map(npc, "mobcount", map)
	local counts = alive[npc]
	return counts and counts[label] or 0
end

-- input is rAthena's input for a number: the player may enter any
-- number. One above max becomes max and returns 1, one below min
-- becomes min and returns 2, as buildin_input does.
function R.input(conversation, min, max)
	local n = conversation:ask_number(-2147483648, 2147483647)
	if n > max then
		return max, 1
	elseif n < min then
		return min, 2
	end
	return n, 0
end

-- party_field reads a field of a party handle, such as its name. Without
-- a party it returns the default, as rAthena's party reads return "null"
-- for a party that does not exist.
function R.party_field(party, field, default)
	if party == nil then
		return default
	end
	return party[field]
end

-- party_members is rAthena's getpartymember for names: every member's
-- name in an array from 0, and the count. Without a party the array is
-- empty and the count is 0.
function R.party_members(party)
	local out = {}
	if party == nil then
		return out, 0
	end
	local names = party:get_member_names()
	for i, name in ipairs(names) do
		out[i - 1] = name
	end
	return out, #names
end

-- guild_field reads a field of a guild handle, such as its name. Without
-- a guild it returns the default, as rAthena's guild reads return "null"
-- for a guild that does not exist.
R.guild_field = R.party_field

-- guild_skill is rAthena's getgdskilllv: the guild's level of a guild
-- skill, or -1 without a guild.
function R.guild_skill(guild, skill)
	if guild == nil then
		return -1
	end
	return guild:get_skill_level(skill)
end

-- guild_permission is rAthena's guild_has_permission for the player: true
-- when the player's guild position has every named permission, such as
-- "can_use_storage".
function R.guild_permission(player, ...)
	local guild = player.guild
	if guild == nil then
		return false
	end
	local position = guild:get_position(player)
	if position == nil then
		return false
	end
	for i = 1, select("#", ...) do
		if not position[select(i, ...)] then
			return false
		end
	end
	return true
end

-- guild_members is rAthena's getguildmember for names: every member's
-- name in an array from 0, and the count. Without a guild the array is
-- empty and the count is 0.
R.guild_members = R.party_members

-- internal is core's wa.internal module, or nil in any other mod. It is
-- not API, so the helpers that use it fall back to R.not_available.
local internal_loaded, internal
local function core_internal()
	if not internal_loaded then
		internal_loaded = true
		local ok, m = pcall(require, "wa.internal")
		if ok then
			internal = m
		end
	end
	return internal
end

-- guild_storage_codes are rAthena's GSTORAGE_ values of the reasons
-- player:open_guild_storage gives.
local guild_storage_codes = { already_open = 1, in_use = 2, no_guild = 3, no_storage = 4, no_permission = 5 }

-- guild_storage is rAthena's guildopenstorage. It opens the player's
-- guild storage and returns rAthena's GSTORAGE_ value, 0 when it opened.
function R.guild_storage(conversation, player)
	local ok, why = player:open_guild_storage()
	if ok then
		return 0
	end
	return guild_storage_codes[why] or 2
end

-- guild_storage_log is rAthena's guildopenstorage_log. It sends the player
-- the guild storage log. Outside core it says that the log is not
-- available.
function R.guild_storage_log(conversation, player)
	local m = core_internal()
	if m == nil then
		R.not_available(conversation, "The guild storage log")
		return
	end
	m.guild_storage_log(player)
end

-- checkquest is rAthena's quest_check for the attached player. kind is
-- 0 (HAVEQUEST), 1 (PLAYTIME) or 2 (HUNTING). timed says whether the
-- quest has a time limit, or is nil when the converter could not tell:
-- rAthena counts a quest without one as timed out, as its end time is
-- 0.
function R.checkquest(player, quest, kind, timed)
	local state = player:get_quest_state(quest)
	if state == "none" then
		return -1
	end
	if kind == 0 then
		return state == "complete" and 2 or 1
	end
	local expired = timed == false or player:quest_timed_out(quest)
	if kind == 1 then
		if expired then
			return 2
		end
		return state == "complete" and 1 or 0
	end
	if state == "complete" then
		return 0
	end
	local i = 1
	while true do
		local count, goal = player:get_quest_progress(quest, i)
		if count == nil then
			return 2
		end
		if count < goal then
			break
		end
		i = i + 1
	end
	return expired and 1 or 0
end

-- isbegin_quest is 0 without the quest, 1 for an active or inactive
-- quest, and 2 for a complete one.
function R.isbegin_quest(player, quest)
	local state = player:get_quest_state(quest)
	if state == "none" then
		return 0
	end
	return state == "complete" and 2 or 1
end

return R
