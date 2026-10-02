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
-- so two handles to one entity are equal with ==.
local function handle_type(name, methods, index, cached)
	handle_methods[name] = methods
	local ids = setmetatable({}, { __mode = "k" })
	local cache = cached and setmetatable({}, { __mode = "v" }) or nil
	local mt = {
		__index = function(self, key)
			local m = methods[key]
			if m ~= nil then
				return m
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

new_player, player_ids = handle_type("player", {
	send_message = function(self, text)
		return (call(H.player_send_message, player_ids[self], text))
	end,
	give_item = function(self, item, amount, options)
		local rent = 0
		if options ~= nil then
			if type(options) ~= "table" then
				error("player:give_item: the options must be a table, such as { rent_seconds = 3600 }", 2)
			end
			for k, v in pairs(options) do
				if k ~= "rent_seconds" or math_type(v) ~= "integer" then
					error("player:give_item: the only option is rent_seconds, a whole number of seconds", 2)
				end
				rent = v
			end
		end
		return (call(H.player_give_item, player_ids[self], item, amount, rent))
	end,
	count_item = function(self, item)
		return (call(H.player_count_item, player_ids[self], item))
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
	start_status = function(self, status, options)
		return (call(H.player_start_status, player_ids[self], status, options))
	end,
	add_timed_bonuses = function(self, key, milliseconds, list, options)
		return (call(H.player_add_timed_bonuses, player_ids[self], key, milliseconds))
	end,
	pay_zeny = function(self, amount)
		return (call(H.player_pay_zeny, player_ids[self], amount))
	end,
	give_zeny = function(self, amount)
		return (call(H.player_give_zeny, player_ids[self], amount))
	end,
	take_item = function(self, item, amount)
		return (call(H.player_take_item, player_ids[self], item, amount))
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
}, function(id, key)
	-- Level 3 is the script line that read the field.
	local ok, value = pcall(H.player_field, id, key)
	if not ok then
		error(value, 3)
	end
	return value
end, true)

-- Equipped copies are handles too: the context instance of an equip
-- script, and what player:get_equipped returns.
new_equipped = handle_type("equipped item", {}, function(id, key)
	local ok, value = pcall(H.equipped_field, id, key)
	if not ok then
		error(value, 3)
	end
	if key == "wearer" then
		return new_player(value)
	end
	return value
end)

-- Monsters are handles too. A killed monster's handle stays readable
-- while its death event runs, and fails after.
local new_monster, monster_ids = handle_type("monster", {}, function(id, key)
	local ok, value = pcall(H.monster_field, id, key)
	if not ok then
		error(value, 3)
	end
	return value
end, true)

-- handle_key names a player or monster handle for Go, as "<kind>:<id>",
-- or returns nil for anything else.
local function handle_key(h)
	local id = player_ids[h]
	if id ~= nil then
		return "player:" .. id
	end
	id = monster_ids[h]
	if id ~= nil then
		return "monster:" .. id
	end
	return nil
end

-- NPCs are handles too: the instance of an NPC context.
local npc_ids
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
end)

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

api["wa.context"] = readonly("wa.context", {
	get_instance = function()
		local fields = call(H.context_instance)
		if fields.equipped ~= nil then
			return new_equipped(fields.equipped)
		end
		if fields.npc ~= nil then
			return new_npc(fields.npc)
		end
		return readonly("instance", fields)
	end,
	get_kind = function()
		return (call(H.context_kind))
	end,
	get_mod = function()
		return (call(H.context_mod))
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

api["wa.events"] = readonly("wa.events", {
	on = function(name, handler, options)
		check(type(name) == "string", "events.on: the event name must be a string")
		check(type(handler) == "function", "events.on: the handler must be a function")
		if options ~= nil and name == "death" then
			options = death_options(options)
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
})

api["wa.world"] = readonly("wa.world", {
	announce = function(text)
		check(type(text) == "string", "world.announce: the text must be a string")
		return (call(H.announce, nil, text))
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
-- job tests.
local job_tests = {}
for _, test in ipairs({ "is_transcendent", "is_baby", "is_third", "is_fourth", "base_job", "first_job" }) do
	job_tests[test] = function(job)
		check(math_type(job) == "integer", "jobs." .. test .. ": want a job, such as player.job")
		return (call(H.jobs_test, test, job))
	end
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
	api["wa." .. module] = readonly("wa." .. module, impl)
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

local function require_from(mod_id, name)
	if type(name) ~= "string" then
		error("require: the module name must be a string", 3)
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
			-- source, target and wearer fields are handles.
			local fields = {}
			for k, v in pairs(value) do
				fields[k] = v
			end
			for _, k in ipairs({ "source", "target" }) do
				local who = value[k .. "_kind"]
				if who == "player" then
					fields[k] = new_player(value[k])
				elseif who == "monster" then
					fields[k] = new_monster(value[k])
				end
				fields[k .. "_kind"] = nil
			end
			value = readonly(kind == "hit" and "damage" or "skill use", fields)
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
			fields.killer_kind = nil
			value = readonly("death", fields)
		end
		args[#args + 1] = value
	end
	return fn(unpack(args, 1, n // 2))
end

-- hook_object makes the read-only object of a hook from Go's fields:
-- player and monster become handles.
local function hook_object(kind, fields)
	local o = {}
	for k, v in pairs(fields) do
		o[k] = v
	end
	if fields.player ~= nil then
		o.player = new_player(fields.player)
	end
	if fields.monster ~= nil then
		o.monster = new_monster(fields.monster)
	end
	return readonly(kind == "drop_rate" and "drop" or "exp", o)
end

-- hook_result checks what a hook returned: an integer rate for
-- drop_rate, and two integers, base and job, for experience.
local function hook_result(kind, a, b)
	if kind == "drop_rate" then
		if math_type(a) ~= "integer" then
			error("the drop_rate hook must return an integer rate, per 10,000", 0)
		end
		return a
	end
	if math_type(a) ~= "integer" or math_type(b) ~= "integer" then
		error("the experience hook must return two integers: base and job exp", 0)
	end
	return a, b
end

-- innermost is the end of a chain: the object's own value, which is
-- rAthena's unless a hook passed a changed object.
local function innermost(kind, o)
	if kind == "drop_rate" then
		return hook_result(kind, o.rate)
	end
	return hook_result(kind, o.base, o.job)
end

-- run_hooks runs a chain of n hooks. Each runs in its own context, which
-- Go sets with H.hook_enter. A hook that fails is logged, and the chain
-- goes on as if it were not there.
function R.run_hooks(kind, fields, n, ...)
	local fns = table.pack(...)
	local function at(i, o)
		if i > n then
			return innermost(kind, o)
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
			ok, a, b = pcall(hook_result, kind, a, b)
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
