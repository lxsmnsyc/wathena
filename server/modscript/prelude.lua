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
local function readonly(name, impl)
	local proxy = setmetatable({}, {
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

-- handle makes a read-only object with methods and an ID kept outside it.
local function handle_type(name, methods, index)
	local ids = setmetatable({}, { __mode = "k" })
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
		local h = setmetatable({}, mt)
		ids[h] = id
		protected[h] = true
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
}, function(id, key)
	-- Level 3 is the script line that read the field.
	local ok, value = pcall(H.player_field, id, key)
	if not ok then
		error(value, 3)
	end
	return value
end)

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
-- while its monster_killed event runs, and fails after.
local new_monster = handle_type("monster", {}, function(id, key)
	local ok, value = pcall(H.monster_field, id, key)
	if not ok then
		error(value, 3)
	end
	return value
end)

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
		return readonly("instance", fields)
	end,
	get_kind = function()
		return (call(H.context_kind))
	end,
	get_mod = function()
		return (call(H.context_mod))
	end,
})

api["wa.events"] = readonly("wa.events", {
	on = function(name, handler)
		check(type(name) == "string", "events.on: the event name must be a string")
		check(type(handler) == "function", "events.on: the handler must be a function")
		return new_event_handle(call(H.events_on, name, handler))
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
})

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
		end
		args[#args + 1] = value
	end
	return fn(unpack(args, 1, n // 2))
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

-- is_protected reports whether a value is a read-only table, for tests.
function R.is_protected(t)
	return protected[t] == true
end

return R
