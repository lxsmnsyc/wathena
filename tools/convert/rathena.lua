-- Helpers for scripts converted from rAthena (tools/convert). Generated
-- NPC scripts and functions require this module as core.lib.rathena.
-- It gives what rAthena's script language has and the API does not
-- need: arrays that start at 0, menus written as "A:B", and the end of a
-- script from inside a function.

local events = require "wa.events"
local items = require "wa.items"
local spawning = require "wa.spawning"
local timers = require "wa.timers"
local variables = require "wa.variables"

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

-- text is a value of no known type as a string, as rAthena's set of a
-- string variable: a number becomes its digits, and nothing "".
function R.text(v)
	if v == nil then
		return ""
	end
	return tostring(v)
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

-- NpcTimer is rAthena's NPC timer of one NPC, without an attached
-- player (npc.cpp 1431 to 1735). labels are its OnTimer<n> labels as
-- { tick, function } by tick. elapsed is nd->u.scr.timer, the tick the
-- timer had at its last start, stop or label. running is
-- nd->u.scr.timertick ~= 0, and handle the next label's timer,
-- nd->u.scr.timerid. A stop while running would add the time since then,
-- which scripts cannot read, so elapsed becomes unknown (nil) until init
-- or set. The converter calls start only right after init or set.
local NpcTimer = {}
NpcTimer.__index = NpcTimer

function R.npc_timer(labels)
	return setmetatable({ labels = labels, elapsed = 0, running = false }, NpcTimer)
end

-- schedule arranges label i after delay milliseconds. When it runs,
-- elapsed is its tick and the next label is arranged before the label's
-- function runs, as npc_timerevent does, so a stop there cancels it.
-- The function runs without a player.
function NpcTimer:schedule(i, delay)
	self.handle = timers.after(delay, function()
		self.handle = nil
		local label = self.labels[i]
		self.elapsed, self.running = label[1], true
		local after = self.labels[i + 1]
		if after ~= nil then
			self:schedule(i + 1, after[1] - label[1])
		end
		label[2](nil, nil)
	end)
end

-- start is npc_timerevent_start: nothing when the timer runs, and else
-- the first label after elapsed.
function NpcTimer:start()
	if self.handle ~= nil or self.running then
		return
	end
	if self.elapsed == nil then
		error("startnpctimer: resuming a stopped timer needs the server clock, which scripts cannot read yet", 2)
	end
	self.running = true
	for i, label in ipairs(self.labels) do
		if label[1] > self.elapsed then
			self:schedule(i, label[1] - self.elapsed)
			return
		end
	end
end

-- stop is npc_timerevent_stop.
function NpcTimer:stop()
	if self.handle == nil and not self.running then
		return
	end
	if self.handle ~= nil then
		self.handle:cancel()
		self.handle = nil
	end
	if self.running then
		self.elapsed, self.running = nil, false
	end
end

-- set is npc_settimerevent_tick: a running timer stops, takes the tick,
-- and starts again.
function NpcTimer:set(tick)
	local started = self.handle ~= nil or self.running
	if started then
		self:stop()
	end
	self.elapsed = tick
	if started then
		self:start()
	end
end

-- init is initnpctimer: the timer starts again from 0.
function NpcTimer:init()
	self.running = false
	self:set(0)
	self:start()
end

-- due is getnpctimer(1): 1 while a label is arranged, else 0.
function NpcTimer:due()
	return self.handle ~= nil and 1 or 0
end

-- Equipment reads by rAthena's EQI_ slot numbers (pc.cpp equip_bitmask).
local equip_slots = {
	[0] = "left_accessory", "right_accessory", "shoes", "garment", "head_low", "head_middle", "head_top",
	"armor", "left_hand", "right_hand", "costume_head_top", "costume_head_middle", "costume_head_low",
	"costume_garment", "ammo", "shadow_armor", "shadow_weapon", "shadow_shield", "shadow_shoes",
	"shadow_right_accessory", "shadow_left_accessory",
}

-- equipped is the copy in an EQI_ slot, or nil for an empty slot or a
-- number that is not a slot, as equip_index_check and pc_checkequip.
function R.equipped(player, slot)
	local name = equip_slots[slot]
	if name == nil then
		return nil
	end
	return player:get_equipped(name)
end

-- equip_field reads a field of the copy in an EQI_ slot, or returns none
-- for an empty slot: getequipid, getequiprefinerycnt, getequipweaponlv
-- and getequiparmorlv.
function R.equip_field(player, slot, field, none)
	local e = R.equipped(player, slot)
	if e == nil then
		return none
	end
	return e[field]
end

-- equip_name is getequipname: the item's name, or "".
function R.equip_name(player, slot)
	local e = R.equipped(player, slot)
	if e == nil then
		return ""
	end
	return items.name(e.item)
end

-- equip_card is getequipcardid: the card in card slot n from 0 to 3, or
-- 0.
function R.equip_card(player, slot, n)
	local e = R.equipped(player, slot)
	if e == nil or n < 0 or n >= 4 then
		return 0
	end
	local card = e.cards[n + 1]
	return card and card.item or 0
end

-- isequipped is 1 when the player wears every item, else 0. The
-- converter passes only items that are not cards.
function R.isequipped(player, ...)
	for i = 1, select("#", ...) do
		local id = select(i, ...)
		local found = false
		for slot = 0, #equip_slots do
			local e = player:get_equipped(equip_slots[slot])
			if e ~= nil and e.item == id then
				found = true
				break
			end
		end
		if not found then
			return 0
		end
	end
	return 1
end

-- int32 is a number truncated to 32 bits, as C's cast to int.
local function int32(n)
	n = n & 0xffffffff
	if n >= 0x80000000 then
		n = n - 0x100000000
	end
	return n
end

-- strtol64 is C's strtol on a 64-bit long: spaces, a sign, a 0x prefix
-- for base 16 or 0, and digits of the base. It stops at the first other
-- character and holds at the long's limits. A base other than 0 and 2
-- to 36 gives 0.
local function strtol64(s, base)
	local i, len = 1, #s
	while i <= len and s:find("^[ \t\n\v\f\r]", i) do
		i = i + 1
	end
	local negative = false
	local sign = s:sub(i, i)
	if sign == "+" or sign == "-" then
		negative = sign == "-"
		i = i + 1
	end
	if base ~= 0 and (base < 2 or base > 36) then
		return 0
	end
	local function digit(c)
		local b = c:byte()
		if b == nil then
			return nil
		elseif b >= 48 and b <= 57 then
			return b - 48
		elseif b >= 97 and b <= 122 then
			return b - 87
		elseif b >= 65 and b <= 90 then
			return b - 55
		end
		return nil
	end
	local prefixed = s:find("^0[xX]", i) ~= nil
	local after = s:sub(i + 2, i + 2)
	if (base == 0 or base == 16) and prefixed and digit(after) ~= nil and digit(after) < 16 then
		base, i = 16, i + 2
	elseif base == 0 then
		base = s:sub(i, i) == "0" and 8 or 10
	end
	local n, over = 0, false
	while i <= len do
		local d = digit(s:sub(i, i))
		if d == nil or d >= base then
			break
		end
		if not over then
			if n > (math.maxinteger - d) // base then
				over = true
			else
				n = n * base + d
			end
		end
		i = i + 1
	end
	if over then
		return negative and math.mininteger or math.maxinteger
	end
	return negative and -n or n
end

-- atoi is C's atoi, as buildin_atoi: glibc's (int) strtol(s, NULL, 10).
function R.atoi(s)
	return int32(strtol64(s, 10))
end

-- strtol is buildin_strtol: strtol held to the int32 limits.
function R.strtol(s, base)
	local n = strtol64(s, base)
	if n > 2147483647 then
		return 2147483647
	elseif n < -2147483648 then
		return -2147483648
	end
	return n
end

-- compare is buildin_compare: 1 when b is in a, without case (stristr),
-- else 0. An empty b is in every string.
function R.compare(a, b)
	return string.find(string.upper(a), string.upper(b), 1, true) and 1 or 0
end

-- charat is buildin_charat: the character at index i from 0, or "".
function R.charat(s, i)
	if i < 0 or i >= #s then
		return ""
	end
	return s:sub(i + 1, i + 1)
end

-- substr is buildin_substr: the characters from start to stop, both from
-- 0 and included, or "" when they are out of the string.
function R.substr(s, start, stop)
	if start >= 0 and stop >= 0 and stop < #s and start <= stop then
		return s:sub(start + 1, stop + 1)
	end
	return ""
end

-- insertchar is buildin_insertchar: c's first character put in at index
-- i. An index below 0, which C reads as a large size_t, or past the end
-- puts it at the end. An empty c ends the C string at i.
function R.insertchar(s, c, i)
	if i < 0 or i > #s then
		i = #s
	end
	if c == "" then
		return s:sub(1, i)
	end
	return s:sub(1, i) .. c:sub(1, 1) .. s:sub(i + 1)
end

-- delchar is buildin_delchar: s without the character at index i from
-- 0. An index out of the string gives s.
function R.delchar(s, i)
	if i < 0 or i > #s then
		return s
	end
	return s:sub(1, i) .. s:sub(i + 2)
end

-- sprintf is buildin_sprintf with a format read when the script runs.
-- Each % starts a part that runs to the next %, which C's sprintf gets
-- with one value. %% is %, and %n is left out. A conversion is %d or %i
-- of a number, or %s of a string, with the flags, width and precision
-- that string.format takes as C does. Others raise an error, as the
-- converter does not know what C would print. Without values the format
-- comes back as it is, and too few values give "".
function R.sprintf(format, ...)
	local n = select("#", ...)
	if format == "" or n == 0 then
		return format
	end
	local out, used, i = {}, 0, 1
	while i <= #format do
		local p = format:find("%", i, true)
		if p == nil then
			out[#out + 1] = format:sub(i)
			break
		end
		out[#out + 1] = format:sub(i, p - 1)
		local after = format:sub(p + 1, p + 1)
		if after == "%" then
			out[#out + 1] = "%"
			i = p + 2
		elseif after == "n" then
			i = p + 2
		else
			if used >= n then
				return ""
			end
			local spec = format:match("^%%[-+ 0]*%d?%d?%.?%d?%d?[di]", p)
			local text = false
			if spec == nil then
				spec = format:match("^%%%-*%d?%d?%.?%d?%d?s", p)
				text = true
			end
			if spec == nil then
				error("sprintf: a conversion other than %d, %i and %s in " .. format, 2)
			end
			used = used + 1
			local v = select(used, ...)
			if text and type(v) ~= "string" or not text and math.type(v) ~= "integer" then
				error("sprintf: " .. spec .. " of a value of another type", 2)
			end
			if not text then
				spec = spec:sub(1, -2) .. "d"
			end
			out[#out + 1] = string.format(spec, v)
			i = p + #spec
		end
	end
	return table.concat(out)
end

-- matches reports whether find is in s at index i from 1, with case or
-- without, as replacestr and countstr compare: a byte that differs
-- matches without case when toupper makes them equal.
local function matches(s, find, i, usecase)
	if i + #find - 1 > #s then
		return false
	end
	local part = s:sub(i, i + #find - 1)
	if part == find then
		return true
	end
	return not usecase and string.upper(part) == string.upper(find)
end

-- replacestr is buildin_replacestr: find replaced by replace from the
-- left, with case unless usecase is 0, count times or every time when
-- count is nil. An empty find, or a count of 0, ends the script.
function R.replacestr(s, find, replace, usecase, count)
	if find == "" or count == 0 then
		R.stop()
	end
	local out, i, n = {}, 1, 0
	while i <= #s do
		if count ~= nil and n == count then
			break
		end
		if matches(s, find, i, usecase == nil or usecase ~= 0) then
			out[#out + 1] = replace
			n = n + 1
			i = i + #find
		else
			out[#out + 1] = s:sub(i, i)
			i = i + 1
		end
	end
	out[#out + 1] = s:sub(i)
	return table.concat(out)
end

-- countstr is buildin_countstr: how many times find is in s from the
-- left, without overlaps, with case unless usecase is 0. An empty find
-- ends the script.
function R.countstr(s, find, usecase)
	if find == "" then
		R.stop()
	end
	local i, n = 1, 0
	while i <= #s do
		if matches(s, find, i, usecase == nil or usecase ~= 0) then
			n = n + 1
			i = i + #find
		else
			i = i + 1
		end
	end
	return n
end

-- pow is buildin_pow: (int32) pow(a, b). C's cast of a value out of
-- the int32 range gives INT_MIN on x86.
function R.pow(a, b)
	local r = (a + 0.0) ^ b
	if r ~= r or r >= 2147483648.0 or r < -2147483648.0 then
		return -2147483648
	end
	if r < 0 then
		return math.ceil(r)
	end
	return math.floor(r)
end

-- distance is buildin_distance: distance_xy without CIRCULAR_AREA, the
-- larger of the two differences.
function R.distance(x0, y0, x1, y1)
	return math.max(math.abs(x0 - x1), math.abs(y0 - y1))
end

-- inarray is buildin_inarray: the first index from 0 below the array's
-- size whose element, or the default for a missing one, equals v, or
-- -1.
function R.inarray(t, v, default)
	for i = 0, R.size(t) - 1 do
		local x = t[i]
		if x == nil then
			x = default
		end
		if x == v then
			return i
		end
	end
	return -1
end

-- copyarray is buildin_copyarray: count elements of from, from index
-- j, to to from index i. Copying an array over itself to a later index
-- goes from the end, so it does not read what it wrote.
function R.copyarray(to, i, from, j, count, default)
	if count <= 0 or to == from and i == j then
		return
	end
	local function copy(k)
		local v = from[j + k]
		if v == nil then
			v = default
		end
		to[i + k] = v
	end
	if to == from and i > j then
		for k = count - 1, 0, -1 do
			copy(k)
		end
	else
		for k = 0, count - 1 do
			copy(k)
		end
	end
end

-- explode is buildin_explode: the parts of s between the delimiter's
-- first character, into t from index i. An empty delimiter matches
-- nothing.
function R.explode(t, i, s, delimiter)
	local d = delimiter:sub(1, 1)
	local part = {}
	for k = 1, #s do
		local c = s:sub(k, k)
		if c == d then
			t[i] = table.concat(part)
			i = i + 1
			part = {}
		else
			part[#part + 1] = c
		end
	end
	t[i] = table.concat(part)
end

-- dynamic finds a variable of wa.variables by a name made when the
-- script runs, as getd and setd read it (script.cpp 18016 and 18155):
-- the prefix, which the converter checked, the name up to the first [
-- with $ for a string, and the number after [, or 0. It returns the
-- handle, the index for an array or nil, the owner, and the default.
-- rAthena's x and x[0] are one value: a name that a script keeps as an
-- array is read at index 0.
local function dynamic(scope, text, player)
	local name = text:match("^[^%[]*")
	local index = math.tointeger(tonumber(text:match("^[^%[]*%[[ \t\n\v\f\r]*([+-]?%d+)") or "0")) or 0
	local bare = name:gsub("^[$#@]+", "")
	local str = bare:sub(-1) == "$"
	if str then
		bare = bare:sub(1, -2)
	end
	if not bare:find("^[A-Za-z_][A-Za-z0-9_]*$") or #bare > 64 then
		error("getd: " .. text .. " is not a variable name", 3)
	end
	local declared = bare:lower() .. (str and "_text" or "")
	local owner = player
	if scope == "server" then
		owner = nil
	end
	local default = str and "" or 0
	if index == 0 then
		local ok, handle = pcall(variables[scope .. "_" .. (str and "string" or "integer")], declared)
		if ok then
			return handle, nil, owner, default
		end
	end
	return variables[scope .. "_table"](declared), index, owner, default
end

-- getd reads a variable by a name made when the script runs.
function R.getd(scope, text, player)
	local handle, index, owner, default = dynamic(scope, text, player)
	if index ~= nil then
		return R.aget(handle, owner, index, default)
	end
	local v
	if owner == nil then
		v = handle:get()
	else
		v = handle:get(owner)
	end
	if v == nil then
		return default
	end
	return v
end

-- setd writes a variable by a name made when the script runs. A string
-- variable takes the value as text; a number variable needs a number.
function R.setd(scope, text, player, value)
	local handle, index, owner, default = dynamic(scope, text, player)
	if type(default) == "string" then
		value = tostring(value)
	elseif math.type(value) ~= "integer" then
		error("setd: " .. text .. " takes a number", 2)
	end
	if index ~= nil then
		R.aset(handle, owner, index, value)
	elseif owner == nil then
		handle:set(value)
	else
		handle:set(owner, value)
	end
end

-- skill_level is getskilllv of a computed skill ID: 0 for an ID that is
-- not a skill, as pc_checkskill.
function R.skill_level(player, skill)
	if skill <= 0 then
		return 0
	end
	return player:get_skill_level(skill)
end

-- mapxy is getmapxy for a unit: 0 and its map and cell, or -1 without
-- the unit.
function R.mapxy(unit)
	if unit == nil then
		return -1, "", 0, 0
	end
	return 0, unit.map, unit.x, unit.y
end

-- warp is buildin_warp with a map read when it runs: "Random" warps to
-- a random cell, "SavePoint" and "Save" to the save point.
function R.warp(player, map, x, y)
	if map == "Random" then
		player:warp_to_random_cell()
	elseif map == "SavePoint" or map == "Save" then
		player:warp_to_save_point()
	else
		player:warp(map, x, y)
	end
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

-- quest_state is the player's state of a quest. A computed ID that
-- data/quests.yml lacks is "none": rAthena's quest_check looks only in
-- the player's log, where such a quest never is.
local function quest_state(player, quest)
	local ok, state = pcall(player.get_quest_state, player, quest)
	if ok then
		return state
	end
	if type(state) == "string" and state:find("there is no quest", 1, true) then
		return "none"
	end
	error(state, 0)
end

-- quest runs a quest command with a computed ID. A quest that
-- data/quests.yml lacks is one rAthena's quest_db lacks, where
-- quest_add, quest_delete, quest_update_status and quest_change only log
-- an error.
function R.quest(player, method, ...)
	local ok, err = pcall(player[method], player, ...)
	if not ok then
		if type(err) == "string" and err:find("there is no quest", 1, true) then
			return
		end
		error(err, 0)
	end
end

-- checkquest is rAthena's quest_check for the attached player. kind is
-- 0 (HAVEQUEST), 1 (PLAYTIME) or 2 (HUNTING). timed says whether the
-- quest has a time limit, or is nil when the converter could not tell:
-- rAthena counts a quest without one as timed out, as its end time is
-- 0.
function R.checkquest(player, quest, kind, timed)
	local state = quest_state(player, quest)
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
	local state = quest_state(player, quest)
	if state == "none" then
		return 0
	end
	return state == "complete" and 2 or 1
end

-- hiding_statuses are the statuses that set OPTION_HIDE, OPTION_CLOAK
-- or OPTION_CHASEWALK, which pc_ishiding reads: SC_HIDING,
-- SC_CLOAKING, SC_CHASEWALK, SC_CLOAKINGEXCEED, SC__INVISIBILITY and
-- SC_NEWMOON.
local hiding_statuses = { 24, 25, 124, 364, 450, 713 }

-- hiding is rAthena's pc_ishiding.
function R.hiding(player)
	for _, status in ipairs(hiding_statuses) do
		if player:has_status(status) then
			return true
		end
	end
	return false
end

-- touch_one runs fn as rAthena's OnTouch_: for one player in the NPC's
-- area at a time (nd->touching_id), with the player and the touch's
-- conversation. A player who is hiding or dead does not start it. When
-- the player it ran for leaves the area, it runs for the next player in
-- the area who can start it, as npc_touchnext_areanpc does. rAthena
-- picks that player by its place in the map's blocks; touch_one picks
-- the one that came in first.
--
-- dialog is set when fn shows a dialog. The next player then has no
-- conversation, since it did not touch the NPC just now, so touch_one
-- leaves the NPC free and runs fn for the next player who comes in.
-- rAthena would show the dialog to the player already in the area.
function R.touch_one(fn, dialog)
	local inside, touching = {}, nil
	local function can(player)
		return player.hp > 0 and not R.hiding(player)
	end
	events.on("touch", function(player, conversation)
		inside[#inside + 1] = player
		if touching == nil and can(player) then
			touching = player
			fn(player, conversation)
		end
	end)
	events.on("untouch", function(player)
		for i, p in ipairs(inside) do
			if p == player then
				table.remove(inside, i)
				break
			end
		end
		if touching ~= player then
			return
		end
		touching = nil
		if dialog then
			return
		end
		for _, p in ipairs(inside) do
			if can(p) then
				touching = p
				fn(p)
				return
			end
		end
	end)
end

return R
