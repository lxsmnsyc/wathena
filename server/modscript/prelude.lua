-- The mod runtime's Lua side. It runs once per zone state, with the real
-- globals, before any mod code. It builds:
--   - one environment table per mod, so mods cannot read or change each
--     other's globals,
--   - read-only API modules, which every mod shares,
--   - require, which finds wa.* modules and mod files,
--   - read-only handles for players and monsters.
-- Mod code never sees the real globals or the host functions in H.
-- mods/README.md describes the rules for mod authors.

local H = ...

local type, error, select, tostring, tonumber = type, error, select, tostring, tonumber
local setmetatable, getmetatable = setmetatable, getmetatable
local rawget, rawset, rawequal, rawlen = rawget, rawset, rawequal, rawlen
local next, pairs, ipairs, load, pcall, xpcall, assert = next, pairs, ipairs, load, pcall, xpcall, assert
local collectgarbage = collectgarbage
local unpack = table.unpack
local math_type = math.type
local libraries = { string = string, table = table, math = math, utf8 = utf8, coroutine = coroutine }
local version = _VERSION

-- protected holds every read-only table, so the rawset that mods get
-- refuses them.
local protected = setmetatable({}, { __mode = "k" })

-- call runs a host function, and raises its error at the script's line:
-- level 3 is the code that called the API function that called call.
-- Callers wrap call in parentheses, as (call(...)), which is not a tail
-- call, so the API function's own level stays on the stack.
local function call(fn, ...)
	local ok, a, b = pcall(fn, ...)
	if not ok then
		error(a, 3)
	end
	return a, b
end

local function readonly_error(name)
	return function(_, key)
		error(name .. " is read-only: " .. tostring(key) .. " cannot be set", 2)
	end
end

-- readonly returns a read-only view of impl. Writing raises an error, and
-- pairs walks impl without handing impl out.
local function readonly(name, impl, callable)
	local proxy = setmetatable({}, {
		__call = callable,
		__index = impl,
		__newindex = readonly_error(name),
		__pairs = function()
			local key
			return function()
				local value
				key, value = next(impl, key)
				return key, value
			end
		end,
		__len = function()
			return #impl
		end,
		__tostring = function()
			return name
		end,
		__metatable = false,
	})
	protected[proxy] = true
	return proxy
end

-- handle_methods holds each handle type's methods, by type name, for the
-- test that checks the runtime against the API definitions.
local handle_methods = {}

-- handle makes a read-only object with methods and an ID kept outside it.
-- With cached set, one ID gives the same object while a script holds it,
-- so two handles to one entity are equal with ==. An entity handle has
-- the read-only field kind, such as "player".
local function handle_type(name, methods, index, cached, kind)
	handle_methods[name] = methods
	local ids = setmetatable({}, { __mode = "k" })
	local cache = cached and setmetatable({}, { __mode = "v" }) or nil
	local mt = {
		__index = function(self, key)
			local m = methods[key]
			if m ~= nil then
				return m
			end
			if kind ~= nil and key == "kind" then
				return kind
			end
			if index ~= nil then
				return (index(ids[self], key))
			end
			return nil
		end,
		__newindex = function(_, key)
			error(name .. "." .. tostring(key) .. " is read-only; change it with a method", 2)
		end,
		__tostring = function(self)
			return name .. " " .. tostring(ids[self])
		end,
		__metatable = false,
	}
	local function new(id)
		if cache ~= nil then
			local h = cache[id]
			if h ~= nil then
				return h
			end
		end
		local h = setmetatable({}, mt)
		ids[h] = id
		protected[h] = true
		if cache ~= nil then
			cache[id] = h
		end
		return h
	end
	return new, ids
end

-- Players are handles: each access looks the player up again, and fails
-- once the player has left.
local player_ids
local new_player
local new_equipped

-- heal_amount reads one amount of item_heal or heal_percent: a number,
-- or { min, max } that the server draws as rAthena's rand does.
local function heal_amount(spec, key, what)
	local v = spec[key]
	if v == nil then
		return 0, 0
	end
	if math_type(v) == "integer" then
		return v, v
	end
	if type(v) == "table" and math_type(v[1]) == "integer" and math_type(v[2]) == "integer" and v[1] <= v[2] then
		return v[1], v[2]
	end
	error(what .. ": " .. key .. " must be an integer or { minimum, maximum }", 3)
end

local function heal(self, spec, percent, what)
	if type(spec) ~= "table" then
		error(what .. ": want a table such as { hp = 50 }", 3)
	end
	for k in pairs(spec) do
		if k ~= "hp" and k ~= "sp" then
			error(what .. ": unknown field " .. tostring(k) .. "; it takes hp and sp", 3)
		end
	end
	local hp1, hp2 = heal_amount(spec, "hp", what)
	local sp1, sp2 = heal_amount(spec, "sp", what)
	return (call(H.player_heal, player_ids[self], percent, hp1, hp2, sp1, sp2))
end

-- Party and guild handles are made after the player handle, which reads
-- them.
local new_party, party_ids
local new_guild, guild_ids

new_player, player_ids = handle_type("player", {
	send_message = function(self, text)
		return (call(H.player_send_message, player_ids[self], text))
	end,
	give_item = function(self, item, amount, options)
		local rent, roll, to, bound = 0, nil, nil, nil
		if options ~= nil then
			if type(options) ~= "table" then
				error("player:give_item: the options must be a table, such as { rent_seconds = 3600 }", 2)
			end
			for k, v in pairs(options) do
				if k == "rent_seconds" then
					if math_type(v) ~= "integer" then
						error("player:give_item: rent_seconds is a whole number of seconds", 2)
					end
					rent = v
				elseif k == "roll" then
					if v ~= "random" and v ~= "default" and type(v) ~= "table" then
						error("player:give_item: roll is \"random\", \"default\" or a table of values, such as { boss_damage = 31 }", 2)
					end
					roll = v
				elseif k == "to" then
					if type(v) ~= "string" then
						error("player:give_item: to is \"storage\", \"cart\" or a storage's name", 2)
					end
					to = v
				elseif k == "bound" then
					if type(v) ~= "string" then
						error("player:give_item: bound is \"account\", \"character\", \"party\" or \"guild\"", 2)
					end
					bound = v
				else
					error("player:give_item: the options are rent_seconds, roll, to and bound", 2)
				end
			end
		end
		return (call(H.player_give_item, player_ids[self], item, amount, rent, roll, to, bound))
	end,
	count_item = function(self, item, options)
		local from = nil
		if options ~= nil then
			if type(options) ~= "table" then
				error("player:count_item: the options must be a table, such as { from = \"storage\" }", 2)
			end
			for k, v in pairs(options) do
				if k ~= "from" then
					error("player:count_item: the option is from", 2)
				end
				from = v
			end
			if from == nil then
				error("player:count_item: from is \"storage\", \"cart\" or a storage's name", 2)
			end
		end
		return (call(H.player_count_item, player_ids[self], item, from))
	end,
	open_storage = function(self, name)
		return (call(H.player_open_storage, player_ids[self], name))
	end,
	open_guild_storage = function(self)
		return call(H.player_open_guild_storage, player_ids[self])
	end,
	give_item_from_group = function(self, group)
		return (call(H.player_give_group, player_ids[self], group))
	end,
	item_heal = function(self, spec)
		return heal(self, spec, false, "player:item_heal")
	end,
	heal_percent = function(self, spec)
		return heal(self, spec, true, "player:heal_percent")
	end,
	warp = function(self, map, x, y)
		return (call(H.player_warp, player_ids[self], "map", map, x, y))
	end,
	warp_to_random_cell = function(self)
		return (call(H.player_warp, player_ids[self], "random"))
	end,
	warp_to_save_point = function(self)
		return (call(H.player_warp, player_ids[self], "save_point"))
	end,
	show_effect = function(self, effect)
		return (call(H.player_show_effect, player_ids[self], effect, false))
	end,
	show_headgear_effect = function(self, effect)
		return (call(H.player_show_effect, player_ids[self], effect, true))
	end,
	get_skill_level = function(self, skill)
		return (call(H.player_skill_level, player_ids[self], skill))
	end,
	get_equipped = function(self, slot)
		local id = call(H.player_equipped, player_ids[self], slot)
		if id == nil then
			return nil
		end
		return new_equipped(id)
	end,
	add_timed_bonuses = function(self, key, milliseconds, list, options)
		if type(key) ~= "string" then
			error("player:add_timed_bonuses: the key must be a string", 2)
		end
		if math_type(milliseconds) ~= "integer" then
			error("player:add_timed_bonuses: milliseconds must be an integer", 2)
		end
		if type(list) ~= "table" then
			error("player:add_timed_bonuses: the bonuses must be a list of entries, such as { { \"str\", 5 } }", 2)
		end
		if options ~= nil and type(options) ~= "table" then
			error("player:add_timed_bonuses: the options must be a table", 2)
		end
		return (call(H.player_add_timed_bonuses, player_ids[self], key, milliseconds, list, options))
	end,
	pay_zeny = function(self, amount)
		return (call(H.player_pay_zeny, player_ids[self], amount))
	end,
	change_job = function(self, job, options)
		if options ~= nil and type(options) ~= "table" then
			error("player:change_job: the options must be a table", 2)
		end
		local force = options ~= nil and options.force or false
		return (call(H.player_change_job, player_ids[self], job, force))
	end,
	rebirth = function(self)
		return (call(H.player_rebirth, player_ids[self]))
	end,
	set_companion = function(self, companion)
		return (call(H.player_set_companion, player_ids[self], "companion", companion))
	end,
	set_mount = function(self, mount)
		return (call(H.player_set_companion, player_ids[self], "mount", mount))
	end,
	set_cart = function(self, look)
		return (call(H.player_set_cart, player_ids[self], look))
	end,
	reset_skills = function(self)
		call(H.player_reset, player_ids[self], "skills")
	end,
	reset_stats = function(self)
		call(H.player_reset, player_ids[self], "stats")
	end,
	reset_skill = function(self, skill, options)
		if options ~= nil and type(options) ~= "table" then
			error("player:reset_skill: the options must be a table", 2)
		end
		options = options or {}
		return (call(H.player_reset_skill, player_ids[self], skill, options.to_level, options.with_dependents or false))
	end,
	reset_stat = function(self, stat, options)
		if options ~= nil and type(options) ~= "table" then
			error("player:reset_stat: the options must be a table", 2)
		end
		options = options or {}
		return (call(H.player_reset_stat, player_ids[self], stat, options.to))
	end,
	give_zeny = function(self, amount)
		return (call(H.player_give_zeny, player_ids[self], amount))
	end,
	take_item = function(self, item, amount, options)
		local from = nil
		if options ~= nil then
			if type(options) ~= "table" then
				error("player:take_item: the options must be a table, such as { from = \"cart\" }", 2)
			end
			for k, v in pairs(options) do
				if k ~= "from" then
					error("player:take_item: the option is from", 2)
				end
				from = v
			end
			if from == nil then
				error("player:take_item: from is \"storage\", \"cart\" or a storage's name", 2)
			end
		end
		return (call(H.player_take_item, player_ids[self], item, amount, from))
	end,
	heal_fully = function(self)
		return (call(H.player_heal, player_ids[self], true, 100, 100, 100, 100))
	end,
	set_save_point = function(self, map, x, y)
		return (call(H.player_set_save_point, player_ids[self], map, x, y))
	end,
	can_carry = function(self, item, amount)
		return (call(H.player_can_carry, player_ids[self], item, amount))
	end,
	mark_minimap = function(self, id, x, y, color)
		return (call(H.player_mark_minimap, player_ids[self], id, x, y, color))
	end,
	unmark_minimap = function(self, id)
		return (call(H.player_mark_minimap, player_ids[self], id))
	end,
	open_shop = function(self, shop)
		return (call(H.player_open_shop, player_ids[self], shop))
	end,
	-- Currencies, mail, vending, buying stores and store search (review
	-- decision 30).
	get_currency = function(self, name)
		return (call(H.player_get_currency, player_ids[self], name))
	end,
	give_currency = function(self, name, n)
		call(H.player_change_currency, player_ids[self], name, n, false)
	end,
	take_currency = function(self, name, n)
		return (call(H.player_change_currency, player_ids[self], name, n, true))
	end,
	send_mail = function(self, spec)
		if type(spec) ~= "table" then
			error("player:send_mail: want a table, such as { to = \"Name\", title = \"Hello\" }", 2)
		end
		local fields = { to = true, sender = true, title = true, body = true, zeny = true, items = true }
		for k in pairs(spec) do
			if not fields[k] then
				error("player:send_mail: " .. tostring(k) .. " is not a field; a mail has to, sender, title, body, zeny and items", 2)
			end
		end
		local to = spec.to
		if type(to) == "table" then
			to = player_ids[to]
			if to == nil then
				error("player:send_mail: to is a character's name, a character ID or a player handle", 2)
			end
		end
		if spec.items ~= nil and type(spec.items) ~= "table" then
			error("player:send_mail: items is a list of { item, amount, roll, bound }", 2)
		end
		call(H.player_send_mail, player_ids[self], to, spec.sender, spec.title, spec.body, spec.zeny, spec.items)
	end,
	open_mail = function(self)
		return (call(H.player_market_call, player_ids[self], "open_mail"))
	end,
	autotrade = function(self)
		return (call(H.player_market_call, player_ids[self], "autotrade"))
	end,
	search_stores = function(self, spec)
		if type(spec) ~= "table" then
			error("player:search_stores: want a table, such as { uses = 10, effect = \"show\" }", 2)
		end
		for k in pairs(spec) do
			if k ~= "uses" and k ~= "effect" then
				error("player:search_stores: " .. tostring(k) .. " is not a field; the fields are uses and effect", 2)
			end
		end
		return (call(H.player_market_call, player_ids[self], "search_stores", spec.uses, spec.effect or "show"))
	end,
	open_buying_store = function(self, spec)
		if type(spec) ~= "table" then
			error("player:open_buying_store: want a table, such as { slots = 5 }", 2)
		end
		for k in pairs(spec) do
			if k ~= "slots" then
				error("player:open_buying_store: " .. tostring(k) .. " is not a field; the field is slots", 2)
			end
		end
		return (call(H.player_market_call, player_ids[self], "open_buying_store", spec.slots))
	end,
	whisper = function(self, text, options)
		if type(text) ~= "string" then
			error("player:whisper: the text must be a string", 2)
		end
		local from = nil
		if options ~= nil then
			if type(options) ~= "table" then
				error("player:whisper: the options must be a table, such as { from = \"Guide\" }", 2)
			end
			for k, v in pairs(options) do
				if k ~= "from" then
					error("player:whisper: the only option is from", 2)
				end
				if type(v) ~= "string" then
					error("player:whisper: from must be a string", 2)
				end
				from = v
			end
		end
		return (call(H.player_whisper, player_ids[self], text, from))
	end,
	is_blocking = function(self, other)
		local id = player_ids[other]
		if id == nil then
			error("player:is_blocking: want a player handle", 2)
		end
		return (call(H.player_is_blocking, player_ids[self], id))
	end,
	mute = function(self, minutes, reason)
		if math_type(minutes) ~= "integer" then
			error("player:mute: minutes must be an integer", 2)
		end
		if type(reason) ~= "string" then
			error("player:mute: the reason must be a string", 2)
		end
		call(H.player_sanction, player_ids[self], "mute", minutes, reason)
	end,
	jail = function(self, minutes, reason)
		if math_type(minutes) ~= "integer" then
			error("player:jail: minutes must be an integer", 2)
		end
		if type(reason) ~= "string" then
			error("player:jail: the reason must be a string", 2)
		end
		call(H.player_sanction, player_ids[self], "jail", minutes, reason)
	end,
	unmute = function(self)
		return (call(H.player_sanction, player_ids[self], "unmute"))
	end,
	expand = function(self, limit, amount)
		if type(limit) ~= "string" then
			error("player:expand: the limit must be a string", 2)
		end
		if math_type(amount) ~= "integer" then
			error("player:expand: the amount must be an integer", 2)
		end
		return (call(H.player_expand, player_ids[self], limit, amount))
	end,
	is_friend = function(self, other)
		local id = player_ids[other]
		if id == nil then
			error("player:is_friend: want a player handle", 2)
		end
		return (call(H.player_friend, player_ids[self], "is_friend", id))
	end,
	get_friend_names = function(self)
		return (call(H.player_friend, player_ids[self], "get_friend_names"))
	end,
	get_staff_role = function(self)
		return (call(H.player_staff, player_ids[self], "role"))
	end,
	has_staff_permission = function(self, name)
		if type(name) ~= "string" then
			error("player:has_staff_permission: the permission must be a string, such as \"moderation.mute\"", 2)
		end
		return (call(H.player_staff, player_ids[self], "has", name))
	end,
}, function(id, key)
	-- Level 3 is the script line that read the field.
	local ok, value = pcall(H.player_field, id, key)
	if not ok then
		error(value, 3)
	end
	if key == "party" and value ~= nil then
		return new_party(value)
	end
	if key == "guild" and value ~= nil then
		return new_guild(value)
	end
	return value
end, true, "player")

-- Equipped copies are handles too: the context instance of an equip
-- script, and what player:get_equipped returns.
local equipped_ids
new_equipped, equipped_ids = handle_type("equipped item", {
	set_roll = function(self, name, value)
		call(H.equipped_set_roll, equipped_ids[self], name, value)
	end,
	reroll = function(self, name)
		call(H.equipped_reroll, equipped_ids[self], name)
	end,
}, function(id, key)
	local ok, value = pcall(H.equipped_field, id, key)
	if not ok then
		error(value, 3)
	end
	if key == "wearer" then
		return new_player(value)
	end
	if key == "rolls" then
		return readonly("rolls", value)
	end
	if key == "cards" or key == "random_options" then
		-- A list of read-only entries: { item, rolls } or
		-- { option, value, param }.
		for i, entry in ipairs(value) do
			if entry.rolls ~= nil then
				entry.rolls = readonly("rolls", entry.rolls)
			end
			value[i] = readonly(key == "cards" and "card" or "random option", entry)
		end
		return readonly(key, value)
	end
	return value
end)

-- Monsters are handles too. A killed monster's handle stays readable
-- while its death event runs, and fails after. target and master are
-- handles, which key_handle makes below; the methods come below too.
local key_handle
local new_monster, monster_ids = handle_type("monster", {}, function(id, key)
	local ok, value = pcall(H.monster_field, id, key)
	if not ok then
		error(value, 3)
	end
	if (key == "target" or key == "master") and value ~= nil then
		return key_handle(value)
	end
	return value
end, true, "monster")

-- npc_ids holds the IDs of NPC handles, which come below.
local npc_ids
-- new_cast makes the cast object of a cast event, and find_entities runs
-- an entity query, below.
local new_cast, find_entities

-- handle_key names a player, monster or NPC handle for Go, as
-- "<kind>:<id>", or returns nil for anything else.
local function handle_key(h)
	local id = player_ids[h]
	if id ~= nil then
		return "player:" .. id
	end
	id = monster_ids[h]
	if id ~= nil then
		return "monster:" .. id
	end
	if npc_ids ~= nil then
		id = npc_ids[h]
		if id ~= nil then
			return "npc:" .. id
		end
	end
	return nil
end

-- NPCs are handles too: the instance of an NPC context.
local new_npc
new_npc, npc_ids = handle_type("npc", {
	show_emotion = function(self, emotion)
		return (call(H.npc_emotion, npc_ids[self], emotion))
	end,
	say_aloud = function(self, text)
		return (call(H.npc_say, npc_ids[self], text))
	end,
}, function(id, key)
	local ok, value = pcall(H.npc_field, id, key)
	if not ok then
		error(value, 3)
	end
	return value
end, false, "npc")

-- entity_handle makes the handle of an entity of a kind.
local function entity_handle(kind, id)
	if kind == "player" then
		return new_player(id)
	elseif kind == "monster" then
		return new_monster(id)
	elseif kind == "npc" then
		return new_npc(id)
	end
	return nil
end

-- key_handle makes the handle of a handle key, "<kind>:<id>".
key_handle = function(key)
	local kind, id = string.match(key, "^(%a+):(%d+)$")
	if kind == nil then
		return nil
	end
	return entity_handle(kind, math.tointeger(tonumber(id)))
end

-- The methods of monster handles (proposal 0002, review decision 22).
local monster_methods = handle_methods["monster"]
monster_methods.set_target = function(self, target)
	local key = handle_key(target)
	if key == nil then
		error("monster:set_target: the target must be a player, monster or NPC handle", 2)
	end
	return (call(H.monster_set_target, monster_ids[self], key))
end
monster_methods.clear_target = function(self)
	call(H.monster_clear_target, monster_ids[self])
end
monster_methods.walk_to = function(self, x, y)
	return (call(H.monster_walk_to, monster_ids[self], x, y))
end
monster_methods.use_skill = function(self, skill, level, target)
	local key = handle_key(target)
	if key == nil then
		error("monster:use_skill: the target must be a player, monster or NPC handle; use the monster itself for a skill on itself", 2)
	end
	return (call(H.monster_use_skill, monster_ids[self], skill, level, key))
end
monster_methods.talk = function(self, text)
	call(H.monster_talk, monster_ids[self], text)
end
monster_methods.stop = function(self)
	call(H.monster_stop, monster_ids[self])
end

-- with_handles copies Go's fields, and makes handles of target, source,
-- caster and owner with their kinds.
local function with_handles(fields)
	local o = {}
	for k, v in pairs(fields) do
		o[k] = v
	end
	for _, k in ipairs({ "target", "source", "caster", "owner" }) do
		if fields[k .. "_kind"] ~= nil then
			o[k] = entity_handle(fields[k .. "_kind"], fields[k])
			o[k .. "_kind"] = nil
		end
	end
	return o
end

-- Conversations: the methods that wait for the player pause the
-- conversation's thread. Go resumes it with "ok" and the answer, or with
-- "closed" when the conversation ends without the script, such as when
-- the player leaves. A closed conversation raises CLOSED, which the
-- runtime does not log as a failure.
local CLOSED = "the conversation was closed"
local conversation_ids
local new_conversation

local function answer(status, value)
	if status == "closed" then
		error(CLOSED, 0)
	end
	return value
end

local function strings_of(what, ...)
	local n = select("#", ...)
	for i = 1, n do
		local v = select(i, ...)
		if type(v) ~= "string" and math_type(v) == nil then
			error(what .. ": argument " .. i .. " must be a string", 3)
		end
	end
end

new_conversation, conversation_ids = handle_type("conversation", {
	say = function(self, ...)
		strings_of("conversation:say", ...)
		return (call(H.conversation_say, conversation_ids[self], ...))
	end,
	wait_for_next = function(self)
		return (answer(call(H.conversation_next, conversation_ids[self])))
	end,
	choose = function(self, ...)
		strings_of("conversation:choose", ...)
		return (answer(call(H.conversation_choose, conversation_ids[self], ...)))
	end,
	ask_number = function(self, minimum, maximum)
		return (answer(call(H.conversation_ask_number, conversation_ids[self], minimum, maximum)))
	end,
	ask_text = function(self, maximum_length)
		return (answer(call(H.conversation_ask_text, conversation_ids[self], maximum_length)))
	end,
	close = function(self)
		return (answer(call(H.conversation_close, conversation_ids[self])))
	end,
	wait = function(self, milliseconds)
		return (answer(call(H.conversation_wait, conversation_ids[self], milliseconds)))
	end,
	show_image = function(self, name, position)
		return (call(H.conversation_image, conversation_ids[self], name, position))
	end,
	hide_image = function(self)
		return (call(H.conversation_image, conversation_ids[self], "", "center"))
	end,
	show_progress = function(self, seconds)
		return (answer(call(H.conversation_progress, conversation_ids[self], seconds)))
	end,
})

local event_ids
local new_event_handle
new_event_handle, event_ids = handle_type("event handle", {
	remove = function(self)
		return (call(H.events_remove, event_ids[self]))
	end,
})

local timer_ids
local new_timer
new_timer, timer_ids = handle_type("timer", {
	cancel = function(self)
		return (call(H.timers_cancel, timer_ids[self]))
	end,
})

local function check(cond, message)
	if not cond then
		error(message, 3)
	end
end

local api = {}

-- The run of a typed world event (proposal 0002, review decision 31),
-- from context:get_event(). Its monsters and participants are those on
-- the context's map.
local event_run_ids
local new_event_run
new_event_run, event_run_ids = handle_type("event run", {
	get_monsters = function(self)
		local out = {}
		for i, id in ipairs(call(H.event_run_call, "get_monsters", event_run_ids[self])) do
			out[i] = new_monster(id)
		end
		return out
	end,
	get_participants = function(self)
		local out = {}
		for i, id in ipairs(call(H.event_run_call, "get_participants", event_run_ids[self])) do
			out[i] = new_player(id)
		end
		return out
	end,
	finish = function(self, result)
		check(result == "won" or result == "lost", "run:finish: the result is \"won\" or \"lost\"")
		call(H.event_run_call, "finish", event_run_ids[self], result)
	end,
}, nil, true)

api["wa.context"] = readonly("wa.context", {
	get_instance = function()
		local fields = call(H.context_instance)
		if fields.equipped ~= nil then
			return new_equipped(fields.equipped)
		end
		if fields.npc ~= nil then
			return new_npc(fields.npc)
		end
		if fields.monster ~= nil then
			-- A monster's AI script: the instance is the monster.
			return new_monster(fields.monster)
		end
		if fields.owner_kind ~= nil then
			-- A passive skill's instance: owner is a handle.
			return readonly("instance", with_handles(fields))
		end
		return readonly("instance", fields)
	end,
	get_kind = function()
		return (call(H.context_kind))
	end,
	get_mod = function()
		return (call(H.context_mod))
	end,
	get_event = function()
		local id = call(H.context_event)
		if id == nil then
			return nil
		end
		return new_event_run(id)
	end,
})

-- Starting and stopping world events (review decision 31).
api["wa.world_events"] = readonly("wa.world_events", {
	start = function(name)
		check(type(name) == "string", "world_events.start: want an event's name of data/events.yml")
		return (call(H.world_events_request, name, true))
	end,
	stop = function(name)
		check(type(name) == "string", "world_events.stop: want an event's name of data/events.yml")
		return (call(H.world_events_request, name, false))
	end,
})

-- death_options turns the options of a death handler into plain data:
-- victim and victims become one list of handle keys.
local function death_options(options)
	check(type(options) == "table", "events.on(\"death\"): the options must be a table")
	local out = {}
	local victims
	for k, v in pairs(options) do
		if k == "victim" then
			local key = handle_key(v)
			check(key ~= nil, "events.on(\"death\"): victim must be a player or monster handle")
			victims = victims or {}
			victims[#victims + 1] = key
		elseif k == "victims" then
			check(type(v) == "table", "events.on(\"death\"): victims must be a list of handles")
			victims = victims or {}
			for _, h in ipairs(v) do
				local key = handle_key(h)
				check(key ~= nil, "events.on(\"death\"): victims must be a list of player or monster handles")
				victims[#victims + 1] = key
			end
		else
			out[k] = v
		end
	end
	out.victims = victims
	return out
end

-- chat_options turns the options of a chat handler into plain data: the
-- player handle becomes its ID.
local function chat_options(options, name)
	name = name or "chat"
	check(type(options) == "table", "events.on(\"" .. name .. "\"): the options must be a table")
	local out = {}
	for k, v in pairs(options) do
		if k == "player" then
			local id = player_ids[v]
			check(id ~= nil, "events.on(\"" .. name .. "\"): player must be a player handle")
			out.player = id
		else
			out[k] = v
		end
	end
	return out
end

api["wa.events"] = readonly("wa.events", {
	on = function(name, handler, options)
		check(type(name) == "string", "events.on: the event name must be a string")
		check(type(handler) == "function", "events.on: the handler must be a function")
		if options ~= nil and name == "death" then
			options = death_options(options)
		elseif options ~= nil and name == "chat" then
			options = chat_options(options)
		elseif options ~= nil and name == "trade" then
			options = chat_options(options, "trade")
		end
		return new_event_handle(call(H.events_on, name, handler, options))
	end,
})

-- Hooks (proposal 0002, "Hooks"): a chain ordered by priority. Each hook
-- gets the object and next, which runs the rest of the chain.
local hook_ids
local new_hook
new_hook, hook_ids = handle_type("hook handle", {
	remove = function(self)
		return (call(H.hooks_remove, hook_ids[self]))
	end,
})

api["wa.hooks"] = readonly("wa.hooks", {
	on = function(name, fn, options)
		check(type(name) == "string", "hooks.on: the hook name must be a string")
		check(type(fn) == "function", "hooks.on: the hook must be a function(object, next)")
		local priority = 0
		if options ~= nil then
			check(type(options) == "table", "hooks.on: the options must be a table, such as { priority = 10 }")
			for k, v in pairs(options) do
				check(k == "priority", "hooks.on: the only option is priority")
				check(math_type(v) == "integer", "hooks.on: priority must be an integer")
				priority = v
			end
		end
		return new_hook(call(H.hooks_on, name, fn, priority))
	end,
})

-- Schedules on the server clock, in the server's time zone.
local schedule_ids
local new_schedule
new_schedule, schedule_ids = handle_type("schedule", {
	cancel = function(self)
		return (call(H.schedules_cancel, schedule_ids[self]))
	end,
})

api["wa.schedules"] = readonly("wa.schedules", {
	daily = function(time, handler)
		check(type(time) == "string", "schedules.daily: the time must be a string, such as \"20:00\"")
		check(type(handler) == "function", "schedules.daily: the handler must be a function")
		return new_schedule(call(H.schedules_add, "daily", handler, time))
	end,
	weekly = function(day, time, handler)
		check(type(day) == "string", "schedules.weekly: the day must be a string, such as \"saturday\"")
		check(type(time) == "string", "schedules.weekly: the time must be a string, such as \"20:00\"")
		check(type(handler) == "function", "schedules.weekly: the handler must be a function")
		return new_schedule(call(H.schedules_add, "weekly", handler, day, time))
	end,
	hourly = function(minute, handler)
		check(math_type(minute) == "integer", "schedules.hourly: the minute must be an integer from 0 to 59")
		check(type(handler) == "function", "schedules.hourly: the handler must be a function")
		return new_schedule(call(H.schedules_add, "hourly", handler, minute))
	end,
	when = function(condition, handler, options)
		check(type(condition) == "function", "schedules.when: the condition must be a function that returns true or false")
		check(type(handler) == "function", "schedules.when: the handler must be a function")
		local lasting = 0
		if options ~= nil then
			check(type(options) == "table", "schedules.when: the options must be a table, such as { lasting_minutes = 60 }")
			for k, v in pairs(options) do
				check(k == "lasting_minutes", "schedules.when: the only option is lasting_minutes")
				check(math_type(v) == "integer", "schedules.when: lasting_minutes must be an integer")
				lasting = v
			end
		end
		return new_schedule(call(H.schedules_when, condition, handler, lasting))
	end,
})

api["wa.spawning"] = readonly("wa.spawning", {
	monsters = function(spec)
		check(type(spec) == "table", "spawning.monsters: want a table such as { monster = PORING, amount = 5 }")
		local list = call(H.spawning_monsters, spec)
		local out = {}
		for i, id in ipairs(list) do
			out[i] = new_monster(id)
		end
		return out
	end,
})

api["wa.timers"] = readonly("wa.timers", {
	after = function(milliseconds, fn)
		check(math_type(milliseconds) == "integer", "timers.after: milliseconds must be an integer")
		check(type(fn) == "function", "timers.after: the callback must be a function")
		return new_timer(call(H.timers_start, milliseconds, fn, false))
	end,
	every = function(milliseconds, fn)
		check(math_type(milliseconds) == "integer", "timers.every: milliseconds must be an integer")
		check(type(fn) == "function", "timers.every: the callback must be a function")
		return new_timer(call(H.timers_start, milliseconds, fn, true))
	end,
	-- with_bonuses runs fn now: the bonuses it adds end after the time.
	-- { restart = timer } cancels that timer first, if it still runs.
	with_bonuses = function(milliseconds, fn, options)
		check(math_type(milliseconds) == "integer", "timers.with_bonuses: milliseconds must be an integer")
		check(type(fn) == "function", "timers.with_bonuses: the bonuses must be a function")
		local restart
		if options ~= nil then
			check(type(options) == "table", "timers.with_bonuses: the options must be a table")
			for k, v in pairs(options) do
				check(k == "restart", "timers.with_bonuses: the only option is restart")
				restart = v
			end
		end
		local restart_id = nil
		if restart ~= nil then
			restart_id = timer_ids[restart]
			check(restart_id ~= nil, "timers.with_bonuses: restart must be a timer")
		end
		local id = call(H.timers_bonus_start, milliseconds, restart_id)
		local ok, err = pcall(fn)
		call(H.timers_bonus_done, id, ok)
		if not ok then
			error(err, 2)
		end
		return new_timer(id)
	end,
})

api["wa.items"] = readonly("wa.items", {
	id = function(name)
		check(type(name) == "string", "items.id: the item name must be a string")
		return (call(H.items_id, name))
	end,
	group = function(name)
		check(type(name) == "string", "items.group: the group name must be a string")
		return (call(H.items_group, name))
	end,
	name = function(item)
		check(math_type(item) == "integer", "items.name: want an item from items.id")
		return (call(H.items_name, item))
	end,
})

-- Text for players (strategy section 6.13). text "key" reads the mod's
-- locales/en.yml.
api["wa.text"] = readonly("wa.text", {
	item_link = function(item)
		check(math_type(item) == "integer", "text.item_link: want an item from items.id")
		return (call(H.text_item_link, item))
	end,
}, function(_, key)
	check(type(key) == "string", "text: the key must be a string")
	return (call(H.text, key))
end)

api["wa.maps"] = readonly("wa.maps", {
	announce = function(map, text)
		check(type(map) == "string", "maps.announce: the map must be a name, such as prontera")
		check(type(text) == "string", "maps.announce: the text must be a string")
		return (call(H.announce, map, text))
	end,
	find_entities = function(query)
		return find_entities("maps.find_entities", query)
	end,
})

-- Shops of data/npcs.yml (review decision 30). A shop handle names the
-- shop's record.
local shop_names
local new_shop
new_shop, shop_names = handle_type("shop", {
	add_item = function(self, item, options)
		check(options == nil or type(options) == "table", "shop:add_item: the options are a table of cost, stock and restock")
		return (call(H.shop_call, shop_names[self], "add_item", item, options))
	end,
	remove_item = function(self, item)
		return (call(H.shop_call, shop_names[self], "remove_item", item))
	end,
	set_stock = function(self, item, n)
		return (call(H.shop_call, shop_names[self], "set_stock", item, n))
	end,
}, nil, true)

local world_module
local function get_shop(a, b)
	-- world.get_shop(name), or world:get_shop(name) as review decision 30
	-- writes it.
	local name = a
	if a == world_module then
		name = b
	end
	check(type(name) == "string", "world.get_shop: want a shop's name of data/npcs.yml")
	local full = call(H.world_get_shop, name)
	if full == nil then
		return nil
	end
	return new_shop(full)
end

world_module = readonly("wa.world", {
	announce = function(text)
		check(type(text) == "string", "world.announce: the text must be a string")
		return (call(H.announce, nil, text))
	end,
	get_shop = get_shop,
})
api["wa.world"] = world_module

-- Chat channels (proposal 0002, review decision 26). A channel is one
-- for the whole world: every zone's handle of it is the same channel.
local channel_ids
local new_channel
new_channel, channel_ids = handle_type("channel", {
	send = function(self, text)
		check(type(text) == "string", "channel:send: the text must be a string")
		call(H.channel_call, "send", channel_ids[self], text)
	end,
	add = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "channel:add: want a player handle")
		return (call(H.channel_call, "add", channel_ids[self], id))
	end,
	remove = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "channel:remove: want a player handle")
		return (call(H.channel_call, "remove", channel_ids[self], id))
	end,
	get_members = function(self)
		return (call(H.channel_call, "get_members", channel_ids[self]))
	end,
}, nil, true)

api["wa.channels"] = readonly("wa.channels", {
	create = function(spec)
		check(type(spec) == "table", "channels.create: want a table such as { name = \"lounge\", join = \"open\" }")
		return new_channel(call(H.channels_create, spec))
	end,
	get = function(name)
		check(type(name) == "string", "channels.get: the name must be a string, such as \"trade\"")
		local id = call(H.channels_get, name)
		if id == nil then
			return nil
		end
		return new_channel(id)
	end,
})

-- Parties (proposal 0002, review decision 27). A party is one for the
-- whole world. Its leader and members are handles of players in this
-- zone, so a member on another map is nil or left out.
new_party, party_ids = handle_type("party", {
	get_members = function(self)
		local out = {}
		for i, id in ipairs(call(H.party_call, "get_members", party_ids[self])) do
			out[i] = new_player(id)
		end
		return out
	end,
	get_member_names = function(self)
		return (call(H.party_call, "get_member_names", party_ids[self]))
	end,
	add = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "party:add: want a player handle")
		return (call(H.party_call, "add", party_ids[self], id))
	end,
	remove = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "party:remove: want a player handle")
		return (call(H.party_call, "remove", party_ids[self], id))
	end,
	set_leader = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "party:set_leader: want a player handle")
		return (call(H.party_call, "set_leader", party_ids[self], id))
	end,
	set_options = function(self, options)
		check(type(options) == "table", "party:set_options: want a table such as { exp_share = \"even\" }")
		call(H.party_call, "set_options", party_ids[self], options)
	end,
	disband = function(self)
		call(H.party_call, "disband", party_ids[self])
	end,
}, function(id, key)
	local ok, value = pcall(H.party_field, id, key)
	if not ok then
		error(value, 3)
	end
	if key == "leader" and value ~= nil then
		return new_player(value)
	end
	return value
end, true)

api["wa.parties"] = readonly("wa.parties", {
	create = function(leader, name)
		local id = player_ids[leader]
		check(id ~= nil, "parties.create: want a player handle as the leader")
		check(type(name) == "string", "parties.create: the name must be a string")
		local party = call(H.parties_create, id, name)
		if party == nil then
			return nil
		end
		return new_party(party)
	end,
	get = function(id)
		check(math_type(id) == "integer", "parties.get: the ID must be an integer")
		local party = call(H.parties_find, "get", id)
		if party == nil then
			return nil
		end
		return new_party(party)
	end,
	find = function(name)
		check(type(name) == "string", "parties.find: the name must be a string")
		local party = call(H.parties_find, "find", name)
		if party == nil then
			return nil
		end
		return new_party(party)
	end,
})

-- Guilds (proposal 0002, review decision 28). A guild is one for the
-- whole world. Its master and members are handles of players in this
-- zone, so a member on another map is nil or left out.
new_guild, guild_ids = handle_type("guild", {
	get_members = function(self)
		local out = {}
		for i, id in ipairs(call(H.guild_call, "get_members", guild_ids[self])) do
			out[i] = new_player(id)
		end
		return out
	end,
	get_member_names = function(self)
		return (call(H.guild_call, "get_member_names", guild_ids[self]))
	end,
	get_position = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "guild:get_position: want a player handle")
		local p = call(H.guild_call, "get_position", guild_ids[self], id)
		if p == nil then
			return nil
		end
		return readonly("guild position", p)
	end,
	get_skill_level = function(self, skill)
		check(math_type(skill) == "integer", "guild:get_skill_level: want a skill ID, such as skills.id(\"GD_EXTENSION\")")
		return (call(H.guild_call, "get_skill_level", guild_ids[self], skill))
	end,
	count_item = function(self, item)
		check(math_type(item) == "integer", "guild:count_item: want an item ID from items.id")
		return (call(H.guild_call, "count_item", guild_ids[self], item))
	end,
	is_allied = function(self, other)
		local id = guild_ids[other]
		check(id ~= nil, "guild:is_allied: want a guild handle")
		return (call(H.guild_call, "is_allied", guild_ids[self], id))
	end,
	is_enemy = function(self, other)
		local id = guild_ids[other]
		check(id ~= nil, "guild:is_enemy: want a guild handle")
		return (call(H.guild_call, "is_enemy", guild_ids[self], id))
	end,
	add = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "guild:add: want a player handle")
		return (call(H.guild_call, "add", guild_ids[self], id))
	end,
	remove = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "guild:remove: want a player handle")
		return (call(H.guild_call, "remove", guild_ids[self], id))
	end,
	set_master = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "guild:set_master: want a player handle")
		return (call(H.guild_call, "set_master", guild_ids[self], id))
	end,
	set_notice = function(self, title, text)
		check(type(title) == "string" and type(text) == "string", "guild:set_notice: want a title and a text")
		return (call(H.guild_call, "set_notice", guild_ids[self], title, text))
	end,
	give_exp = function(self, amount)
		check(math_type(amount) == "integer", "guild:give_exp: the amount must be an integer")
		call(H.guild_call, "give_exp", guild_ids[self], amount)
	end,
	disband = function(self)
		call(H.guild_call, "disband", guild_ids[self])
	end,
}, function(id, key)
	local ok, value = pcall(H.guild_field, id, key)
	if not ok then
		error(value, 3)
	end
	if key == "master" and value ~= nil then
		return new_player(value)
	end
	if key == "notice" then
		return readonly("guild notice", value)
	end
	return value
end, true)

api["wa.guilds"] = readonly("wa.guilds", {
	create = function(master, name)
		local id = player_ids[master]
		check(id ~= nil, "guilds.create: want a player handle as the master")
		check(type(name) == "string", "guilds.create: the name must be a string")
		local guild = call(H.guilds_create, id, name)
		if guild == nil then
			return nil
		end
		return new_guild(guild)
	end,
	get = function(id)
		check(math_type(id) == "integer", "guilds.get: the ID must be an integer")
		local guild = call(H.guilds_find, "get", id)
		if guild == nil then
			return nil
		end
		return new_guild(guild)
	end,
	find = function(name)
		check(type(name) == "string", "guilds.find: the name must be a string")
		local guild = call(H.guilds_find, "find", name)
		if guild == nil then
			return nil
		end
		return new_guild(guild)
	end,
})

-- The world's ruleset (proposal 0002, review decisions 27 and 32): its
-- limits, flags and rules, read-only.
api["wa.rules"] = readonly("wa.rules", {
	limits = readonly("rules.limits", H.rules_limits()),
	is = function(flag)
		check(type(flag) == "string", "rules.is: the flag must be a string")
		return (call(H.rules_read, "is", flag))
	end,
	get = function(name)
		check(type(name) == "string", "rules.get: the name must be a string")
		return (call(H.rules_read, "get", name))
	end,
	get_limit = function(name)
		check(type(name) == "string", "rules.get_limit: the name must be a string")
		return call(H.rules_read, "limit", name)
	end,
})

-- Saved variables (proposal 0002, "Variables"). A declaration names the
-- scope and the type. Go checks the name and keeps the declaration.
local variable_ids
local new_variable
new_variable, variable_ids = handle_type("variable", {
	get = function(self, player)
		return (call(H.variable_get, variable_ids[self], player_ids[player]))
	end,
	set = function(self, player, value)
		return (call(H.variable_set, variable_ids[self], player_ids[player], value))
	end,
	add = function(self, player, amount)
		return (call(H.variable_add, variable_ids[self], player_ids[player], amount))
	end,
})

local server_variable_ids
local new_server_variable
new_server_variable, server_variable_ids = handle_type("server variable", {
	get = function(self)
		return (call(H.variable_get, server_variable_ids[self]))
	end,
	set = function(self, value)
		return (call(H.variable_set, server_variable_ids[self], nil, value))
	end,
	add = function(self, amount)
		return (call(H.variable_add, server_variable_ids[self], nil, amount))
	end,
})

local variable_api = {}
for _, scope in ipairs({ "character", "character_temporary", "account", "server", "instance" }) do
	for _, kind in ipairs({ "integer", "boolean", "string", "table" }) do
		local what = "variables." .. scope .. "_" .. kind
		variable_api[scope .. "_" .. kind] = function(name)
			check(type(name) == "string", what .. ": the name must be a string")
			local id = call(H.variables_declare, scope, kind, name)
			if scope == "server" then
				return new_server_variable(id)
			end
			return new_variable(id)
		end
	end
end
api["wa.variables"] = readonly("wa.variables", variable_api)

api["wa.skills"] = readonly("wa.skills", {
	id = function(name)
		check(type(name) == "string", "skills.id: the skill name must be a string")
		return (call(H.skills_id, name))
	end,
})

for _, module in ipairs({ "companions", "mounts" }) do
	api["wa." .. module] = readonly("wa." .. module, {
		id = function(name)
			check(type(name) == "string", module .. ".id: the name must be a string, such as \"falcon\"")
			return (call(H.companions_id, module, name))
		end,
	})
end

api["wa.monsters"] = readonly("wa.monsters", {
	id = function(name)
		check(type(name) == "string", "monsters.id: the monster name must be a string")
		return (call(H.monsters_id, name))
	end,
})

api["wa.random"] = readonly("wa.random", {
	-- integer returns a whole number from minimum to maximum.
	integer = function(minimum, maximum)
		check(math_type(minimum) == "integer" and math_type(maximum) == "integer",
			"random.integer: minimum and maximum must be integers")
		check(minimum <= maximum, "random.integer: minimum is above maximum")
		return (call(H.random_integer, minimum, maximum))
	end,
	-- chance is true with a chance of numerator in denominator.
	chance = function(numerator, denominator)
		check(math_type(numerator) == "integer" and math_type(denominator) == "integer",
			"random.chance: numerator and denominator must be integers")
		check(denominator > 0, "random.chance: the denominator must be above 0")
		return (call(H.random_chance, numerator, denominator))
	end,
})

-- divide_truncated and remainder_truncated round as C does, toward zero,
-- for scripts converted from rAthena (strategy section 5.5).
local function divide_truncated(a, b)
	check(math_type(a) == "integer" and math_type(b) == "integer", "integers.divide_truncated: want two integers")
	check(b ~= 0, "integers.divide_truncated: division by zero")
	local q = a // b
	if a % b ~= 0 and (a < 0) ~= (b < 0) then
		q = q + 1
	end
	return q
end

api["wa.integers"] = readonly("wa.integers", {
	divide_truncated = divide_truncated,
	remainder_truncated = function(a, b)
		check(math_type(a) == "integer" and math_type(b) == "integer", "integers.remainder_truncated: want two integers")
		check(b ~= 0, "integers.remainder_truncated: division by zero")
		return a - divide_truncated(a, b) * b
	end,
})

-- Bonus handles: bonuses.add returns one, with a remove method.
local bonus_ids
local new_bonus
new_bonus, bonus_ids = handle_type("bonus", {
	remove = function(self)
		return (call(H.bonuses_remove, bonus_ids[self]))
	end,
})

-- add_bonus is bonuses.add. Numbers and functions go to Go as separate
-- arguments, so Go can keep the functions. The chance and when options,
-- when they are functions, go on their own.
local function add_bonus(name, ...)
	local n = select("#", ...)
	local options
	if n > 0 and type(select(n, ...)) == "table" then
		options = select(n, ...)
		n = n - 1
	end
	local values = {}
	for i = 1, n do
		local v = select(i, ...)
		if math_type(v) ~= "integer" and type(v) ~= "function" then
			error("bonuses.add(" .. tostring(name) .. "): argument " .. i .. " must be an integer or a function", 3)
		end
		values[i] = v
	end
	local plain, chance, condition
	if options ~= nil then
		plain = {}
		for k, v in pairs(options) do
			if type(k) ~= "string" then
				error("bonuses.add(" .. tostring(name) .. "): option names are strings", 3)
			end
			if k == "chance" and type(v) == "function" then
				chance = v
			elseif k == "when" then
				if type(v) ~= "function" then
					error("bonuses.add(" .. tostring(name) .. "): when must be a function that returns true or false", 3)
				end
				condition = v
			elseif type(v) == "function" then
				error("bonuses.add(" .. tostring(name) .. "): only the amount, a level, the chance and when can be functions", 3)
			else
				plain[k] = v
			end
		end
	end
	return new_bonus(call(H.bonuses_add, name, plain, chance, condition, n, unpack(values, 1, n)))
end

api["wa.bonuses"] = readonly("wa.bonuses", {
	add = function(name, ...)
		check(type(name) == "string", "bonuses.add: the bonus name must be a string")
		return add_bonus(name, ...)
	end,
})

-- Constant modules, such as wa.races, come from Go. wa.jobs also has the
-- job tests, and wa.statuses has define.
local statuses_impl

-- status_bonus_fns are the bonuses functions of mod statuses, by status.
local status_bonus_fns = {}
local job_tests = {}
for _, test in ipairs({ "is_transcendent", "is_baby", "is_third", "is_fourth", "base_job", "first_job" }) do
	job_tests[test] = function(job)
		check(math_type(job) == "integer", "jobs." .. test .. ": want a job, such as player.job")
		return (call(H.jobs_test, test, job))
	end
end
job_tests.id = function(name)
	check(type(name) == "string", "jobs.id: want a job's name, such as \"knight\" or \"example.jobs:Desperado\"")
	return (call(H.jobs_id, name))
end
job_tests.next_jobs = function(job)
	check(math_type(job) == "integer", "jobs.next_jobs: want a job, such as player.job")
	return (call(H.jobs_next, job))
end
for module, values in pairs(H.constants()) do
	local impl = {}
	for k, v in pairs(values) do
		impl[k] = v
	end
	if module == "jobs" then
		for k, fn in pairs(job_tests) do
			impl[k] = fn
		end
	end
	if module == "statuses" then
		statuses_impl = impl
	end
	api["wa." .. module] = readonly("wa." .. module, impl)
end
if statuses_impl == nil then
	statuses_impl = {}
	api["wa.statuses"] = readonly("wa.statuses", statuses_impl)
end

local define_fields = { name = true, icon = true, kind = true, ends_on = true, bonuses = true, tick_milliseconds = true }

-- statuses.define adds a status in a mod script, with a constant in
-- statuses named after it. Its bonuses function runs in Lua when one of
-- its statuses starts.
statuses_impl.define = function(spec)
	check(type(spec) == "table", "statuses.define: want a table such as { name = \"glow\", kind = \"buff\" }")
	for k in pairs(spec) do
		check(define_fields[k], "statuses.define: " .. tostring(k) .. " is not a field; the fields are name, icon, kind, ends_on, bonuses and tick_milliseconds")
	end
	check(type(spec.name) == "string", "statuses.define: name must be a string")
	check(spec.icon == nil or math_type(spec.icon) == "integer", "statuses.define: icon must be a status icon ID")
	check(spec.kind == nil or type(spec.kind) == "string", "statuses.define: kind is \"buff\" or \"debuff\"")
	check(spec.ends_on == nil or type(spec.ends_on) == "table", "statuses.define: ends_on must be a list of names")
	check(spec.bonuses == nil or type(spec.bonuses) == "function", "statuses.define: bonuses must be a function of the level that returns bonus entries")
	check(spec.tick_milliseconds == nil or math_type(spec.tick_milliseconds) == "integer", "statuses.define: tick_milliseconds must be an integer")
	local existing = rawget(statuses_impl, spec.name)
	local id = call(H.statuses_define, spec.name, spec.icon, spec.kind, spec.ends_on, spec.tick_milliseconds)
	check(existing == nil or existing == id, "statuses.define: statuses." .. spec.name .. " is a status already")
	status_bonus_fns[id] = spec.bonuses
	statuses_impl[spec.name] = id
end

-- wa.testing: test cases for wactl mod test (mods/README.md, "Testing a
-- mod"). The module exists only when Go set up the testing host
-- functions, which a running server never does.
--
-- A test file returns a function that registers cases with
-- testing.case. Go runs each case in a thread of a fresh world. World
-- methods pause the thread with H.testing_world, and Go resumes it with
-- "ok" and the results, or "error" and a message.

-- registering is the list of cases while a test file registers them.
local registering = nil
-- test_cases are the cases of the test file that loaded last.
local test_cases = {}

local function world_call(op, ...)
	local results = table.pack(pcall(H.testing_world, op, ...))
	if not results[1] then
		error(results[2], 3)
	end
	if results[2] == "error" then
		error(results[3], 3)
	end
	return unpack(results, 3, results.n)
end

local world_ids
local new_world
new_world, world_ids = handle_type("test world", {
	new_player = function(self, spec)
		check(type(spec) == "table", "world:new_player: want a table such as { map = \"prontera\", x = 150, y = 150 }")
		return new_player(world_call("new_player", spec))
	end,
	talk = function(self, player, npc_name, answers)
		local id = player_ids[player]
		check(id ~= nil, "world:talk: want a player from world:new_player")
		check(type(npc_name) == "string", "world:talk: the NPC's name must be a string")
		check(answers == nil or type(answers) == "table", "world:talk: the answers must be a list, such as { 1, \"Bob\" }")
		return (world_call("talk", id, npc_name, answers or {}))
	end,
	use_item = function(self, player, item)
		local id = player_ids[player]
		check(id ~= nil, "world:use_item: want a player from world:new_player")
		check(math_type(item) == "integer", "world:use_item: want an item from items.id")
		return (world_call("use_item", id, item))
	end,
	equip = function(self, player, item, options)
		local id = player_ids[player]
		check(id ~= nil, "world:equip: want a player from world:new_player")
		check(math_type(item) == "integer", "world:equip: want an item from items.id")
		local roll = nil
		if options ~= nil then
			check(type(options) == "table", "world:equip: the options must be a table, such as { roll = \"random\" }")
			for k, v in pairs(options) do
				check(k == "roll", "world:equip: the only option is roll")
				check(v == "random" or v == "default" or type(v) == "table",
					"world:equip: roll is \"random\", \"default\" or a table of values, such as { boss_damage = 31 }")
				roll = v
			end
		end
		return new_equipped(world_call("equip", id, item, roll))
	end,
	advance = function(self, milliseconds)
		check(math_type(milliseconds) == "integer" and milliseconds >= 0, "world:advance: milliseconds must be an integer of 0 or more")
		return (world_call("advance", milliseconds))
	end,
	spawn_monster = function(self, spec)
		check(type(spec) == "table", "world:spawn_monster: want a table such as { monster = PORING, map = \"prontera\", x = 150, y = 150 }")
		return new_monster(world_call("spawn_monster", spec))
	end,
	kill = function(self, player, monster)
		local id = player_ids[player]
		check(id ~= nil, "world:kill: want a player from world:new_player")
		local m = monster_ids[monster]
		check(m ~= nil, "world:kill: want a monster handle")
		return (world_call("kill", id, m))
	end,
	set_time = function(self, time, time_zone)
		check(type(time) == "string", "world:set_time: want a time such as \"2026-10-31 19:58\"")
		check(time_zone == nil or type(time_zone) == "string", "world:set_time: the time zone must be an IANA name, such as \"Asia/Manila\"")
		return (world_call("set_time", time, time_zone))
	end,
	get_time = function(self)
		return (world_call("get_time"))
	end,
	get_monsters = function(self, monster)
		check(monster == nil or math_type(monster) == "integer", "world:get_monsters: want a monster from monsters.id, or nothing")
		local list = world_call("get_monsters", monster)
		local out = {}
		for i, mid in ipairs(list) do
			out[i] = new_monster(mid)
		end
		return out
	end,
	get_announcements = function(self)
		return (world_call("get_announcements"))
	end,
	close_storage = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "world:close_storage: want a player from world:new_player")
		world_call("close_storage", id)
	end,
	set_staff_role = function(self, player, role)
		local id = player_ids[player]
		check(id ~= nil, "world:set_staff_role: want a player from world:new_player")
		check(role == nil or type(role) == "string", "world:set_staff_role: the role is a string, or nil to take it away")
		world_call("set_staff_role", id, role or "")
	end,
	buy = function(self, player, shop, items)
		local id = player_ids[player]
		check(id ~= nil, "world:buy: want a player from world:new_player")
		check(type(shop) == "string", "world:buy: the shop is a shop's full name of data/npcs.yml")
		check(type(items) == "table", "world:buy: items is a table such as { [501] = 2 }")
		return world_call("buy", id, shop, items)
	end,
	plan_restart = function(self, minutes, reason)
		check(math_type(minutes) == "integer", "world:plan_restart: the minutes are an integer from 1 to 10080")
		check(type(reason) == "string", "world:plan_restart: the reason is a string that players see")
		world_call("plan_restart", minutes, reason)
	end,
	get_mails = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "world:get_mails: want a player from world:new_player")
		return (world_call("get_mails", id))
	end,
	vend = function(self, seller, buyer, lines)
		local is, ib = player_ids[seller], player_ids[buyer]
		check(is ~= nil and ib ~= nil, "world:vend: want two players from world:new_player")
		check(type(lines) == "table", "world:vend: lines is a list of { item, amount, price, buy }")
		return world_call("vend", is, ib, lines)
	end,
	trade = function(self, a, b, offer_a, offer_b)
		local ia, ib = player_ids[a], player_ids[b]
		check(ia ~= nil and ib ~= nil, "world:trade: want two players from world:new_player")
		for _, o in ipairs({ offer_a or {}, offer_b or {} }) do
			check(type(o) == "table", "world:trade: an offer is a table, such as { items = { [909] = 2 }, zeny = 100 }")
			for k in pairs(o) do
				check(k == "items" or k == "zeny", "world:trade: an offer has items and zeny")
			end
		end
		return world_call("trade", ia, ib, offer_a, offer_b)
	end,
	chat = function(self, player, text, options)
		local id = player_ids[player]
		check(id ~= nil, "world:chat: want a player from world:new_player")
		check(type(text) == "string", "world:chat: the text must be a string")
		local channel, to = "public", ""
		if options ~= nil then
			check(type(options) == "table", "world:chat: the options must be a table, such as { channel = \"whisper\", to = \"Bob\" }")
			for k, v in pairs(options) do
				check(k == "channel" or k == "to", "world:chat: the options are channel and to")
				check(type(v) == "string", "world:chat: " .. k .. " must be a string")
				if k == "channel" then
					channel = v
				else
					to = v
				end
			end
		end
		return (world_call("chat", id, text, channel, to))
	end,
	get_chat = function(self, player)
		local id = player_ids[player]
		check(id ~= nil, "world:get_chat: want a player from world:new_player")
		local list = world_call("get_chat", id)
		for i, line in ipairs(list) do
			list[i] = readonly("heard line", line)
		end
		return list
	end,
	count_floor_items = function(self, item)
		check(math_type(item) == "integer", "world:count_floor_items: want an item from items.id")
		return (world_call("count_floor_items", item))
	end,
})

-- describe shows a value in a failure message.
local function describe(v, depth)
	depth = depth or 0
	if type(v) == "string" then
		return (string.format("%q", v):gsub("\\\n", "\\n"))
	end
	if type(v) == "table" and getmetatable(v) == nil and depth < 3 then
		local keys = {}
		for k in pairs(v) do
			keys[#keys + 1] = k
		end
		table.sort(keys, function(a, b)
			return tostring(a) < tostring(b)
		end)
		local parts = {}
		local n = rawlen(v)
		for i = 1, n do
			parts[#parts + 1] = describe(v[i], depth + 1)
		end
		for _, k in ipairs(keys) do
			if math_type(k) ~= "integer" or k < 1 or k > n then
				parts[#parts + 1] = tostring(k) .. " = " .. describe(v[k], depth + 1)
			end
			if #parts > 20 then
				parts[#parts + 1] = "..."
				break
			end
		end
		return "{ " .. table.concat(parts, ", ") .. " }"
	end
	return tostring(v)
end

-- deep_equal compares plain tables by their contents, and other values
-- with ==.
local function deep_equal(a, b, depth)
	if a == b then
		return true
	end
	if type(a) ~= "table" or type(b) ~= "table" or getmetatable(a) ~= nil or getmetatable(b) ~= nil or depth > 32 then
		return false
	end
	for k, v in pairs(a) do
		if not deep_equal(v, b[k], depth + 1) then
			return false
		end
	end
	for k in pairs(b) do
		if a[k] == nil then
			return false
		end
	end
	return true
end

local function fail(message)
	error(message, 3)
end

local function expect(value)
	return readonly("expectation", {
		to_be = function(expected)
			if value ~= expected then
				fail("expected " .. describe(expected) .. ", got " .. describe(value))
			end
		end,
		to_equal = function(expected)
			if not deep_equal(value, expected, 0) then
				fail("expected " .. describe(expected) .. ", got " .. describe(value))
			end
		end,
		to_contain = function(part)
			if type(value) == "string" then
				if type(part) ~= "string" or not value:find(part, 1, true) then
					fail("expected " .. describe(value) .. " to contain " .. describe(part))
				end
				return
			end
			if type(value) == "table" then
				for _, v in pairs(value) do
					if deep_equal(v, part, 0) then
						return
					end
				end
			end
			fail("expected " .. describe(value) .. " to contain " .. describe(part))
		end,
		to_be_true = function()
			if value ~= true then
				fail("expected true, got " .. describe(value))
			end
		end,
		to_be_false = function()
			if value ~= false then
				fail("expected false, got " .. describe(value))
			end
		end,
		to_be_nil = function()
			if value ~= nil then
				fail("expected nil, got " .. describe(value))
			end
		end,
		to_fail_with = function(text)
			if type(value) ~= "function" then
				fail("to_fail_with: expect(...) needs a function that should fail")
			end
			local ok, err = pcall(value)
			if ok then
				fail("expected an error containing " .. describe(text) .. ", and the function returned")
			end
			if not tostring(err):find(text, 1, true) then
				fail("expected an error containing " .. describe(text) .. ", got " .. describe(tostring(err)))
			end
		end,
	})
end

if H.testing_world ~= nil then
	api["wa.testing"] = readonly("wa.testing", {
		case = function(name, fn)
			check(registering ~= nil, "testing.case: register cases inside the function that the test file returns")
			check(type(name) == "string" and name ~= "", "testing.case: the name must be a string")
			check(type(fn) == "function", "testing.case: the case must be a function(world)")
			for _, c in ipairs(registering) do
				check(c.name ~= name, "testing.case: there are two cases named " .. describe(name))
			end
			registering[#registering + 1] = { name = name, fn = fn }
		end,
		expect = expect,
	})
end

-- Environments and require.

local envs = {}
local loaded = {}
local LOADING = {}

local base_names = {
	"assert", "error", "ipairs", "next", "pairs", "pcall", "xpcall", "rawequal", "rawlen", "rawget",
	"select", "tonumber", "tostring", "type", "getmetatable", "setmetatable", "collectgarbage",
}
local base = {
	assert = assert, error = error, ipairs = ipairs, next = next, pairs = pairs, pcall = pcall,
	xpcall = xpcall, rawequal = rawequal, rawlen = rawlen, rawget = rawget, select = select,
	tonumber = tonumber, tostring = tostring, type = type, getmetatable = getmetatable,
	setmetatable = setmetatable, collectgarbage = collectgarbage,
}

local function copy(t)
	local out = {}
	for k, v in pairs(t) do
		out[k] = v
	end
	return out
end

local load_file

-- core_internal is what core's converted NPC scripts use and the API has
-- no call for yet, such as the guild storage log. Only the core mod may
-- require it, as wa.internal, and Go checks the caller again. It is not
-- API: it may change without notice.
local core_internal = readonly("wa.internal", {
	guild_storage_log = function(player)
		local id = player_ids[player]
		check(id ~= nil, "guild_storage_log: want a player handle")
		call(H.core_guild_storage_log, id)
	end,
})

local function require_from(mod_id, name)
	if type(name) ~= "string" then
		error("require: the module name must be a string", 3)
	end
	if name == "wa.internal" and mod_id == "core" then
		return core_internal
	end
	if name:sub(1, 3) == "wa." then
		local m = api[name]
		if m == nil and name == "wa.testing" then
			error("require: wa.testing exists only in tests that wactl mod test runs", 3)
		end
		if m == nil then
			error("require: there is no API module " .. name, 3)
		end
		return m
	end
	local ok, owner, path, source = pcall(H.resolve, mod_id, name)
	if not ok then
		error(owner, 3)
	end
	if loaded[owner .. "/" .. path] == LOADING then
		error("require: " .. name .. " requires itself while it loads", 3)
	end
	return (load_file(owner, path, source))
end

local function environment(mod_id)
	local env = envs[mod_id]
	if env ~= nil then
		return env
	end
	env = {}
	for _, k in ipairs(base_names) do
		env[k] = base[k]
	end
	for k, lib in pairs(libraries) do
		env[k] = copy(lib)
	end
	env._VERSION = version
	env._G = env
	env.rawset = function(t, k, v)
		if protected[t] then
			error("rawset: the table is read-only", 2)
		end
		return rawset(t, k, v)
	end
	env.load = function(chunk, name, mode, chunk_env)
		if chunk_env == nil then
			chunk_env = env
		end
		return load(chunk, name, mode, chunk_env)
	end
	env.require = function(name)
		return (require_from(mod_id, name))
	end
	envs[mod_id] = env
	return env
end

-- load_file runs one file of a mod in the mod's environment, once per
-- state, and returns what it returned.
load_file = function(owner, path, source)
	local key = owner .. "/" .. path
	local v = loaded[key]
	if v == LOADING then
		error("require: " .. key .. " requires itself while it loads", 0)
	end
	if v ~= nil then
		return v
	end
	loaded[key] = LOADING
	local fn, err = load(source, "@" .. key, "t", environment(owner))
	if fn == nil then
		loaded[key] = nil
		error(err, 0)
	end
	local ok, result = pcall(fn)
	if not ok then
		loaded[key] = nil
		error(result, 0)
	end
	if result == nil then
		result = true
	end
	loaded[key] = result
	return result
end

local R = {}

-- load_entry loads an entry script and returns its setup function.
function R.load_entry(mod_id, path, source)
	local v = load_file(mod_id, path, source)
	if type(v) ~= "function" then
		error(mod_id .. "/" .. path .. " must return a setup function, and it returned " .. (v == true and "nil" or type(v)), 0)
	end
	return v
end

-- invoke calls a handler. Arguments come in pairs: a kind and a value.
-- "player" makes a player handle, "monster" a monster handle, and
-- "value" passes the value as it is.
-- trade_object and sale_object are set below, with the hook objects.
local trade_object
local sale_object

function R.invoke(fn, ...)
	local n = select("#", ...)
	local args = {}
	for i = 1, n, 2 do
		local kind, value = select(i, ...)
		if kind == "player" then
			value = new_player(value)
		elseif kind == "monster" then
			value = new_monster(value)
		elseif kind == "hit" or kind == "skill_use" then
			-- The damage and skill_used objects: read-only tables whose
			-- source, target and caster fields are handles.
			value = readonly(kind == "hit" and "damage" or "skill use", with_handles(value))
		elseif kind == "cast" then
			-- The cast object of a skill's cast event.
			value = new_cast(value)
		elseif kind == "death" then
			-- The death object: victim and killer are handles.
			local fields = {}
			for k, v in pairs(value) do
				fields[k] = v
			end
			fields.victim = value.victim_kind == "player" and new_player(value.victim) or new_monster(value.victim)
			if value.killer ~= nil then
				fields.killer = value.killer_kind == "player" and new_player(value.killer) or new_monster(value.killer)
			end
			fields.victim_kind, fields.killer_kind = nil, nil
			value = readonly("death", fields)
		elseif kind == "status" then
			-- The status event object: target and source are handles.
			value = readonly("status event", with_handles(value))
		elseif kind == "job_change" then
			-- The job_change object: player is a handle.
			value = readonly("job change", { player = new_player(value.player), from = value.from, to = value.to })
		elseif kind == "party_event" then
			-- The party_join, party_leave and party_change objects: party,
			-- player and leader are handles.
			local fields = { party = new_party(value.party), reason = value.reason }
			if value.player ~= nil then
				fields.player = new_player(value.player)
			end
			if value.leader ~= nil then
				fields.leader = new_player(value.leader)
			end
			if value.options ~= nil then
				fields.options = readonly("party options", value.options)
			end
			value = readonly("party event", fields)
		elseif kind == "guild_event" then
			-- The guild_join, guild_leave and guild_change objects: guild
			-- and player are handles.
			local fields = { guild = new_guild(value.guild), reason = value.reason, change = value.change }
			if value.player ~= nil then
				fields.player = new_player(value.player)
			end
			value = readonly("guild event", fields)
		elseif kind == "chat" then
			-- The chat message: player is a handle.
			value = readonly("chat message", {
				player = new_player(value.player), channel = value.channel, text = value.text, to = value.to, map = value.map,
			})
		elseif kind == "world_event" then
			-- world_event_start and world_event_end: { name, type, result }.
			value = readonly("world event", value)
		elseif kind == "restart" then
			-- restart_planned: { at, reason } or { cancelled = true }.
			value = readonly("restart", value)
		elseif kind == "trade" then
			value = trade_object(value)
		elseif kind == "sale" then
			value = sale_object(value)
		end
		args[#args + 1] = value
	end
	return fn(unpack(args, 1, n // 2))
end

-- trade_object makes the object of the trade event and hook: players
-- and each item's from are handles (review decision 29).
trade_object = function(fields)
	local players = { new_player(fields.players[1]), new_player(fields.players[2]) }
	local items = {}
	for i, it in ipairs(fields.items) do
		local from = players[1]
		if it.from == fields.players[2] then
			from = players[2]
		end
		items[i] = readonly("traded item", { from = from, item = it.item, amount = it.amount })
	end
	return readonly("trade", {
		players = readonly("trade players", players),
		items = readonly("traded items", items),
		zeny = readonly("trade zeny", { fields.zeny[1], fields.zeny[2] }),
	})
end

-- sale_object makes the object of the shop_buy, shop_sell, vend,
-- buying_store_sale and mail events and hooks (review decision 30):
-- player, seller and buyer are handles; a mail's from and to are names.
sale_object = function(fields)
	local items = {}
	for i, it in ipairs(fields.items) do
		items[i] = readonly("sold item", { item = it.item, amount = it.amount, price = it.price })
	end
	local o = { items = readonly("sold items", items), zeny = fields.zeny }
	local name = fields.event
	if name == "shop_buy" or name == "shop_sell" then
		o.player, o.shop = new_player(fields.player), fields.shop
	elseif name == "vend" or name == "buying_store_sale" then
		o.seller, o.buyer = new_player(fields.seller), new_player(fields.buyer)
	else
		o.from, o.to = fields.from, fields.to
	end
	return readonly(name == "mail" and "mail" or "sale", o)
end

local hook_objects = {
	drop_rate = "drop", experience = "exp", status_chance = "status hook", status_duration = "status hook",
	skill_damage = "skill damage hook", cast_time = "cast time hook", skill_cost = "skill cost hook", cast_allowed = "cast allowed hook",
	monster_target = "monster target choice", item_roll = "item roll", chat = "chat message",
}

-- hook_values names the field of each hook's object that holds rAthena's
-- value, and hook_messages what a hook must return.
local hook_values = {
	drop_rate = "rate", status_chance = "chance", status_duration = "milliseconds",
	skill_damage = "percent", cast_time = "milliseconds", skill_cost = "sp", cast_allowed = "allowed",
	chat = "text",
}
local hook_messages = {
	drop_rate = "the drop_rate hook must return an integer rate, per 10,000",
	status_chance = "the status_chance hook must return an integer chance, per 10,000",
	status_duration = "the status_duration hook must return an integer number of milliseconds",
	skill_damage = "the skill_damage hook must return an integer damage percentage",
	cast_time = "the cast_time hook must return an integer number of milliseconds",
	skill_cost = "the skill_cost hook must return an integer SP cost",
	item_roll = "the item_roll hook must return an integer value",
}

-- hook_object makes the read-only object of a hook from Go's fields:
-- player, monster, target and source become handles.
local function hook_object(kind, fields)
	if kind == "trade" then
		return trade_object(fields)
	end
	if kind == "vend" or kind == "buying_store_sale" or kind == "mail" then
		return sale_object(fields)
	end
	if kind == "item_roll" then
		-- The server's roll is not a field: the innermost next returns
		-- it. The source's monster is a handle.
		local src = { kind = fields.source.kind }
		if fields.source.monster ~= nil then
			src.monster = new_monster(fields.source.monster)
		end
		return readonly(hook_objects[kind], {
			item = fields.item, name = fields.name, min = fields.min, max = fields.max,
			default = fields.default, source = readonly("roll source", src),
		})
	end
	local o = with_handles(fields)
	if fields.player ~= nil then
		o.player = new_player(fields.player)
	end
	if fields.monster ~= nil then
		o.monster = new_monster(fields.monster)
	end
	if kind == "experience" then
		-- The player's party, and its own share before the party split.
		if fields.party ~= nil then
			o.party = new_party(fields.party)
		end
		if fields.share ~= nil then
			o.share = readonly("exp share", { base = fields.share.base, job = fields.share.job })
		end
		-- The player's guild, and the share of base exp it takes.
		if fields.guild ~= nil then
			o.guild = new_guild(fields.guild)
		end
	end
	if kind == "monster_target" then
		-- The candidates and rAthena's choice are handle keys.
		local list = {}
		for i, key in ipairs(fields.candidates) do
			list[i] = key_handle(key)
		end
		o.candidates = readonly("candidates", list)
		if fields.current ~= nil then
			o.current = key_handle(fields.current)
		end
	end
	return readonly(hook_objects[kind], o)
end

-- hook_result checks what a hook returned: two integers, base and job,
-- for experience, true or false for cast_allowed, and an integer for
-- the others.
local function hook_result(kind, a, b, o)
	if kind == "monster_target" then
		-- nil, or one of the candidates.
		if a == nil then
			return nil
		end
		local key = handle_key(a)
		if key ~= nil and type(o) == "table" and type(o.candidates) == "table" then
			for _, c in ipairs(o.candidates) do
				if handle_key(c) == key then
					return a
				end
			end
		end
		error("the monster_target hook must return one of the candidates, or nil", 0)
	end
	if kind == "experience" then
		if math_type(a) ~= "integer" or math_type(b) ~= "integer" then
			error("the experience hook must return two integers: base and job exp", 0)
		end
		return a, b
	end
	if kind == "cast_allowed" then
		if type(a) ~= "boolean" then
			error("the cast_allowed hook must return true or false", 0)
		end
		return a
	end
	if kind == "trade" or kind == "vend" or kind == "buying_store_sale" or kind == "mail" then
		-- true, or false and a message.
		if type(a) ~= "boolean" or (b ~= nil and type(b) ~= "string") then
			error("the " .. kind .. " hook must return true, or false and a message", 0)
		end
		if a then
			return true
		end
		return false, b
	end
	if kind == "chat" then
		-- The text, or nil to block the message.
		if a ~= nil and type(a) ~= "string" then
			error("the chat hook must return the text, or nil to block the message", 0)
		end
		return a
	end
	if math_type(a) ~= "integer" then
		error(hook_messages[kind], 0)
	end
	return a
end

-- innermost is the end of a chain: the object's own value, which is
-- rAthena's unless a hook passed a changed object.
local function innermost(kind, o, fields)
	if kind == "item_roll" then
		return hook_result(kind, fields.value)
	end
	if kind == "experience" then
		return hook_result(kind, o.base, o.job)
	end
	if kind == "monster_target" then
		return hook_result(kind, o.current, nil, o)
	end
	if kind == "trade" or kind == "vend" or kind == "buying_store_sale" or kind == "mail" then
		return true
	end
	return hook_result(kind, o[hook_values[kind]])
end

-- run_chain runs a chain of n hooks. Each runs in its own context, which
-- Go sets with H.hook_enter. A hook that fails is logged, and the chain
-- goes on as if it were not there.
local function run_chain(kind, fields, n, fns)
	local function at(i, o)
		if i > n then
			return innermost(kind, o, fields)
		end
		H.hook_enter(i)
		local called, inner_a, inner_b = false, nil, nil
		local next_fn = function(next_o)
			if next_o == nil then
				next_o = o
			end
			if type(next_o) ~= "table" then
				error("next: want the object, or a table with the same fields", 2)
			end
			inner_a, inner_b = at(i + 1, next_o)
			called = true
			H.hook_enter(i)
			return inner_a, inner_b
		end
		local ok, a, b = pcall(fns[i], o, next_fn)
		if ok then
			ok, a, b = pcall(hook_result, kind, a, b, o)
		end
		if not ok then
			H.hook_failed(i, tostring(a))
			if called then
				return inner_a, inner_b
			end
			return at(i + 1, o)
		end
		return a, b
	end
	return at(1, hook_object(kind, fields))
end

function R.run_hooks(kind, fields, n, ...)
	if kind == "monster_target" then
		-- Go gets the chosen handle as its key, or nil.
		local h = run_chain(kind, fields, n, table.pack(...))
		if h == nil then
			return nil
		end
		return handle_key(h)
	end
	return run_chain(kind, fields, n, table.pack(...))
end

-- Status effects (proposal 0002, review decision 19): the status methods
-- of every entity handle. A host function cannot call into Lua, so a
-- start runs the bonuses function of a mod status and the hooks here,
-- and passes their results to Go.

-- lua_hooks runs the chain of a status hook for a start that a script
-- asked for. It returns nil when no hook runs.
local function lua_hooks(kind, fields)
	local got = table.pack(H.hook_chain_begin(kind))
	local n = got[1]
	if n == 0 then
		H.hook_chain_end()
		return nil
	end
	local ok, a = pcall(run_chain, kind, fields, n, table.pack(unpack(got, 2, got.n)))
	H.hook_chain_end()
	if not ok then
		error(a, 0)
	end
	return a
end

local status_option_names = {
	milliseconds = true, level = true, values = true, chance = true, source = true,
	cannot_be_avoided = true, keep_duration = true, keep_chance = true, hide_icon = true,
}

-- status_options copies the options of start_status as plain data: the
-- source handle becomes "<kind>:<id>".
local function status_options(options)
	if options == nil then
		return nil
	end
	if type(options) ~= "table" then
		error("start_status: the options must be a table, such as { milliseconds = 60000, level = 10 }", 4)
	end
	local out = {}
	for k, v in pairs(options) do
		if not status_option_names[k] then
			error("start_status: " .. tostring(k) .. " is not an option; the options are milliseconds, level, values, chance, source, cannot_be_avoided, keep_duration, keep_chance and hide_icon", 4)
		end
		if k == "source" then
			local key = handle_key(v)
			if key == nil then
				error("start_status: source must be a player, monster or NPC handle", 4)
			end
			out.source = key
		elseif k == "values" then
			if type(v) ~= "table" then
				error("start_status: values must be a list of up to 3 integers: val2 to val4", 4)
			end
			local list = {}
			for i, x in ipairs(v) do
				list[i] = x
			end
			out.values = list
		else
			out[k] = v
		end
	end
	return out
end

local function start_status(kind, id, status, options)
	if math_type(status) ~= "integer" then
		error("start_status: want a status from wa.statuses", 3)
	end
	local opts = status_options(options)
	local level = (opts ~= nil and math_type(opts.level) == "integer") and opts.level or 1
	local entries = nil
	local fn = status_bonus_fns[status]
	if fn ~= nil then
		H.compute_begin()
		local ok, r = pcall(fn, level)
		H.compute_end()
		if not ok then
			error(r, 3)
		end
		entries = r
	end
	local chance, duration
	local preview = call(H.entity_status, "preview", kind, id, status, opts)
	if preview ~= nil then
		local fields = { target = id, target_kind = kind, status = status, level = level, chance = preview.chance }
		if opts ~= nil and opts.source ~= nil then
			local skind, sid = opts.source:match("^(%a+):(%-?%d+)$")
			fields.source, fields.source_kind = math.tointeger(tonumber(sid)), skind
		end
		chance = lua_hooks("status_chance", fields)
		fields.chance, fields.milliseconds = nil, preview.milliseconds
		duration = lua_hooks("status_duration", fields)
	end
	return (call(H.entity_status, "start", kind, id, status, opts, entries, chance, duration))
end

-- status_object makes the read-only object of get_status from Go's
-- fields.
local function status_object(fields)
	local o = with_handles(fields)
	o.values = readonly("values", fields.values)
	return readonly("status", o)
end

-- add_status_methods gives a handle type the status methods.
local function add_status_methods(kind, ids)
	local methods = handle_methods[kind]
	methods.start_status = function(self, status, options)
		return start_status(kind, ids()[self], status, options)
	end
	methods.end_status = function(self, status)
		return (call(H.entity_status, "end", kind, ids()[self], status))
	end
	methods.has_status = function(self, status)
		return (call(H.entity_status, "has", kind, ids()[self], status))
	end
	methods.get_status = function(self, status)
		local list = call(H.entity_status, "get", kind, ids()[self], status)
		if list[1] == nil then
			return nil
		end
		return status_object(list[1])
	end
	methods.get_statuses = function(self)
		local list = call(H.entity_status, "list", kind, ids()[self], nil)
		local out = {}
		for i, fields in ipairs(list) do
			out[i] = status_object(fields)
		end
		return out
	end
end
add_status_methods("player", function()
	return player_ids
end)
add_status_methods("monster", function()
	return monster_ids
end)
add_status_methods("npc", function()
	return npc_ids
end)

-- Entity queries and skills (proposal 0002, review decisions 18 and 20).

-- handle_parts returns the kind and ID of an entity handle, or nil.
local function handle_parts(h)
	local key = handle_key(h)
	if key == nil then
		return nil
	end
	local kind, id = key:match("^(%a+):(%-?%d+)$")
	return kind, math.tointeger(tonumber(id))
end

-- find_entities runs a query: handles become "<kind>:<id>" keys, and Go
-- checks the rest. Callers tail call it, so level 2 is the script.
find_entities = function(what, query)
	if type(query) ~= "table" then
		error(what .. ": want a query table, such as { around = player, radius = 5 }", 2)
	end
	local q = {}
	for k, v in pairs(query) do
		if k == "around" or k == "enemies_of" or k == "allies_of" then
			q[k] = handle_key(v) or false
		elseif k == "exclude" and type(v) == "table" then
			local list = {}
			for i, h in ipairs(v) do
				list[i] = handle_key(h) or false
			end
			q[k] = list
		else
			q[k] = v
		end
	end
	local list = call(H.maps_find_entities, what, q)
	local out = {}
	for i, e in ipairs(list) do
		out[i] = entity_handle(e.kind, e.id)
	end
	return out
end

-- area_fields are the query fields that give an area, which find_nearby
-- sets itself.
local area_fields = { around = true, map = true, x = true, y = true, rectangle = true, line = true, cells = true }

-- add_entity_methods gives a handle type find_nearby, exists, and
-- show_effect when it has none.
local function add_entity_methods(kind, ids)
	local methods = handle_methods[kind]
	methods.find_nearby = function(self, query)
		local what = kind .. ":find_nearby"
		if type(query) ~= "table" then
			error(what .. ": want a query table, such as { radius = 5, kinds = { \"monster\" } }", 2)
		end
		local q = {}
		for k, v in pairs(query) do
			if area_fields[k] then
				error(what .. ": the area is around the " .. kind .. ", so the query cannot have " .. tostring(k), 2)
			end
			q[k] = v
		end
		q.around = self
		return find_entities(what, q)
	end
	methods.exists = function(self)
		local id = ids()[self]
		if id == nil then
			return false
		end
		local ok, v = pcall(H.entity_exists, kind, id)
		return ok and v == true
	end
	if methods.show_effect == nil then
		methods.show_effect = function(self, effect)
			return (call(H.entity_show_effect, kind, ids()[self], effect))
		end
	end
end
add_entity_methods("player", function()
	return player_ids
end)
add_entity_methods("monster", function()
	return monster_ids
end)
add_entity_methods("npc", function()
	return npc_ids
end)

-- The cast object of a skill's cast event. It keeps the cast's fields,
-- and passes them to Go with each method, so it works after the event,
-- such as in a timer.
local cast_data
local cast_fields = { skill = true, level = true, map = true, x = true, y = true }
local cast_options = { percent = true, hits = true, type = true, element = true }

local function cast_of(self, what)
	local fields = cast_data[self]
	if fields == nil then
		error("not a cast: call the method with a colon, such as cast:" .. what .. "(target)", 3)
	end
	return fields
end

new_cast, cast_data = handle_type("cast", {
	deal_damage = function(self, target, options)
		local fields = cast_of(self, "deal_damage")
		local tkind, tid = handle_parts(target)
		check(tkind ~= nil, "cast:deal_damage: the target must be a player, monster or NPC handle")
		local percent, hits, attack, element = 100, nil, nil, nil
		if options ~= nil then
			check(type(options) == "table", "cast:deal_damage: the options must be a table, such as { percent = 150 }")
			for k, v in pairs(options) do
				check(cast_options[k], "cast:deal_damage: " .. tostring(k) .. " is not an option; the options are percent, hits, type and element")
				if k == "percent" then
					percent = v
				elseif k == "hits" then
					hits = v
				elseif k == "type" then
					attack = v
				else
					element = v
				end
			end
		end
		check(math_type(percent) == "integer" and percent >= 0, "cast:deal_damage: percent must be a whole number of 0 or more")
		local hooked = lua_hooks("skill_damage", {
			caster = fields.caster, caster_kind = fields.caster_kind, target = tid, target_kind = tkind,
			skill = fields.skill, level = fields.level, percent = percent,
		})
		if hooked ~= nil then
			percent = hooked < 0 and 0 or hooked
		end
		local result = call(H.cast_deal_damage, fields, tkind .. ":" .. tid, percent, hits, attack, element)
		if result == nil then
			return nil
		end
		return readonly("damage result", result)
	end,
	heal = function(self, target, amount)
		local fields = cast_of(self, "heal")
		local key = handle_key(target)
		check(key ~= nil, "cast:heal: the target must be a player, monster or NPC handle")
		check(math_type(amount) == "integer" and amount >= 0, "cast:heal: the amount must be a whole number of 0 or more")
		return (call(H.cast_heal, fields, key, amount))
	end,
	run_default = function(self, options)
		local fields = cast_of(self, "run_default")
		local key = nil
		if options ~= nil then
			check(type(options) == "table", "cast:run_default: the options must be a table, such as { target = monster }")
			for k, v in pairs(options) do
				check(k == "target", "cast:run_default: the only option is target")
				key = handle_key(v)
				check(key ~= nil, "cast:run_default: target must be a player, monster or NPC handle")
			end
		end
		return (call(H.cast_run_default, fields, key))
	end,
}, function(fields, key)
	if key == "caster" then
		return entity_handle(fields.caster_kind, fields.caster)
	end
	if key == "target" then
		if fields.target_kind == nil then
			return nil
		end
		return entity_handle(fields.target_kind, fields.target)
	end
	if cast_fields[key] then
		return fields[key]
	end
	return nil
end)

-- status_bonuses runs the bonuses function of a mod status, for a start
-- that Go makes, such as at login.
function R.status_bonuses(status, level)
	local fn = status_bonus_fns[status]
	if fn == nil then
		return nil
	end
	return fn(level)
end

-- check_condition runs the condition of a world event: a function of a
-- module that mod_id can require.
function R.check_condition(mod_id, module, name)
	local m = require_from(mod_id, module)
	if type(m) ~= "table" or type(m[name]) ~= "function" then
		error("the condition " .. module .. "." .. name .. " is not a function: the module must return a table with it", 0)
	end
	return m[name]() and true or false
end

-- converse runs the click handlers of an NPC in a conversation's
-- thread, one after the other.
function R.converse(player_id, conversation_id, ...)
	local p = new_player(player_id)
	local c = new_conversation(conversation_id)
	for i = 1, select("#", ...) do
		(select(i, ...))(p, c)
	end
end

-- closed_message is the error a closed conversation raises.
function R.closed_message()
	return CLOSED
end

-- drop_mod forgets a mod's environment and loaded files, before a reload.
-- Other mods that required its files keep what they got.
function R.drop_mod(mod_id)
	envs[mod_id] = nil
	local prefix = mod_id .. "/"
	for key in pairs(loaded) do
		if key:sub(1, #prefix) == prefix then
			loaded[key] = nil
		end
	end
end

-- drop_file forgets one loaded file, so the next load runs it again and
-- the old one can be collected once nothing uses it.
function R.drop_file(mod_id, path)
	loaded[mod_id .. "/" .. path] = nil
end

-- reset_loading clears files left half loaded by a call that a limit
-- stopped, which pcall cannot catch.
function R.reset_loading()
	for key, v in pairs(loaded) do
		if v == LOADING then
			loaded[key] = nil
		end
	end
end

-- load_tests loads a test file of wactl mod test in its mod's
-- environment, and runs the function it returns, which registers cases.
-- It returns the names of the cases.
function R.load_tests(mod_id, path, source)
	local fn, err = load(source, "@" .. path, "t", environment(mod_id))
	if fn == nil then
		error(err, 0)
	end
	local register = fn()
	if type(register) ~= "function" then
		error(path .. " must return a function that registers cases with testing.case, and it returned " .. type(register), 0)
	end
	registering = {}
	local ok, e = pcall(register)
	local cases = registering
	registering = nil
	if not ok then
		error(e, 0)
	end
	test_cases = cases
	local names = {}
	for i, c in ipairs(cases) do
		names[i] = c.name
	end
	return names
end

-- run_case runs one case of the test file that loaded last, with a new
-- world handle. It runs in a thread that world methods pause.
function R.run_case(i)
	test_cases[i].fn(new_world(i))
end

-- describe lists the API as the runtime has it, for the test that checks
-- it against the API definitions: each module's keys and each handle
-- type's methods.
function R.describe()
	local modules, handles = {}, {}
	for name, m in pairs(api) do
		local keys = {}
		for k in pairs(m) do
			keys[#keys + 1] = k
		end
		modules[name] = keys
	end
	for name, methods in pairs(handle_methods) do
		local keys = {}
		for k in pairs(methods) do
			keys[#keys + 1] = k
		end
		handles[name] = keys
	end
	local expectation = {}
	for k in pairs(expect(nil)) do
		expectation[#expectation + 1] = k
	end
	handles.expectation = expectation
	return { modules = modules, handles = handles }
end

-- is_protected reports whether a value is a read-only table, for tests.
function R.is_protected(t)
	return protected[t] == true
end

return R
