-- Helpers for scripts converted from rAthena (tools/convert). Generated
-- NPC scripts and functions require this module as core.lib.rathena.
-- It gives what rAthena's script language has and the API does not
-- need: arrays that start at 0, menus written as "A:B", and the end of a
-- script from inside a function.

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
-- Kafra storage. It returns 1, as the functions it replaces do.
function R.not_available(conversation, what)
	conversation:say(what .. " is not available yet.")
	return 1
end

-- truthy is rAthena's test of a value: not 0 and not "".
function R.truthy(v)
	return v ~= nil and v ~= 0 and v ~= ""
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

return R
