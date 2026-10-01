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
new_player, player_ids = handle_type("player", {
	send_message = function(self, text)
		return (call(H.player_send_message, player_ids[self], text))
	end,
	give_item = function(self, item, amount)
		return (call(H.player_give_item, player_ids[self], item, amount))
	end,
	count_item = function(self, item)
		return (call(H.player_count_item, player_ids[self], item))
	end,
}, function(id, key)
	-- Level 3 is the script line that read the field.
	local ok, value = pcall(H.player_field, id, key)
	if not ok then
		error(value, 3)
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
})

api["wa.items"] = readonly("wa.items", {
	id = function(name)
		check(type(name) == "string", "items.id: the item name must be a string")
		return (call(H.items_id, name))
	end,
})

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
