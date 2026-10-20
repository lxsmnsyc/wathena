-- Helpers for scripts converted from rAthena (tools/convert). Generated
-- NPC scripts and functions require this module as core.lib.rathena.
-- It gives what rAthena's script language has and the API does not
-- need: arrays that start at 0, menus written as "A:B", and the end of a
-- script from inside a function.

local clock = require "wa.clock"
local events = require "wa.events"
local items = require "wa.items"
local maps = require "wa.maps"
local mounts = require "wa.mounts"
local spawning = require "wa.spawning"
local timers = require "wa.timers"
local variables = require "wa.variables"
local world = require "wa.world"

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

-- sleep is sleep and sleep2 in an event label. rAthena's script sleeps
-- with a timer even without a player. Here the wait needs the
-- conversation of an event with a player.
function R.sleep(conversation, ms)
	if conversation == nil then
		R.unsupported("a wait in an event without a player")
	end
	conversation:wait(ms)
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
-- player (npc.cpp 1470 to 1735). labels are its OnTimer<n> labels as
-- { tick, function } by tick. elapsed is nd->u.scr.timer, the tick the
-- timer had at its last start, stop or label, and started is
-- nd->u.scr.timertick: when it ran from elapsed, in milliseconds of the
-- server clock (clock.milliseconds), or nil while the timer stands.
-- handle is the next label's timer, nd->u.scr.timerid.
--
-- The tick follows the server clock, so it runs on while the map has no
-- players, as rAthena's. The labels wait on timers.after, which follows
-- the zone clock: a label that came due while the map slept runs when
-- the map wakes, and the labels after it that came due run with it, in
-- order, as the tick catches up.
local NpcTimer = {}
NpcTimer.__index = NpcTimer

function R.npc_timer(labels)
	return setmetatable({ labels = labels, elapsed = 0 }, NpcTimer)
end

-- tick is npc_gettimerevent_tick: getnpctimer(0).
function NpcTimer:tick()
	if self.started == nil then
		return self.elapsed
	end
	return self.elapsed + (clock.milliseconds() - self.started)
end

-- schedule arranges label i when the tick reaches it. When it runs, the
-- tick is the label's, and the next label is arranged before the
-- label's function runs, as npc_timerevent does, so a stop there
-- cancels it. Labels that came due while the map slept run after it,
-- each at its own tick. The functions run without a player.
function NpcTimer:schedule(i)
	local label = self.labels[i]
	self.handle = timers.after(math.max(label[1] - self:tick(), 0), function()
		self.handle = nil
		local now = clock.milliseconds()
		-- The tick at least reached the label, which the zone clock's
		-- timer waited for.
		local reached = math.max(self:tick(), label[1])
		local j = i
		while true do
			local due = self.labels[j]
			-- npc_timerevent: timertick is now and timer the label's tick,
			-- less the time since the label came due.
			self.elapsed, self.started = due[1], now - (reached - due[1])
			local after = self.labels[j + 1]
			if after ~= nil then
				self:schedule(j + 1)
			end
			local handle = self.handle
			due[2](nil, nil)
			-- A stop, a set or an init in the label ends the catching up.
			if after == nil or self.handle ~= handle or handle == nil or after[1] > reached then
				return
			end
			handle:cancel()
			self.handle = nil
			j = j + 1
		end
	end)
end

-- start is npc_timerevent_start: nothing when the timer runs, and else
-- the first label after the tick, which resumes where a stop left it.
-- Without a label left, the tick still runs.
function NpcTimer:start()
	if self.handle ~= nil or self.started ~= nil then
		return
	end
	self.started = clock.milliseconds()
	for i, label in ipairs(self.labels) do
		if label[1] > self.elapsed then
			self:schedule(i)
			return
		end
	end
end

-- stop is npc_timerevent_stop: the tick keeps the time it ran.
function NpcTimer:stop()
	if self.handle ~= nil then
		self.handle:cancel()
		self.handle = nil
	end
	if self.started ~= nil then
		self.elapsed = self.elapsed + (clock.milliseconds() - self.started)
		self.started = nil
	end
end

-- set is npc_settimerevent_tick: a running timer stops, takes the tick,
-- and starts again.
function NpcTimer:set(tick)
	local running = self.handle ~= nil or self.started ~= nil
	if running then
		self:stop()
	end
	self.elapsed = tick
	if running then
		self:start()
	end
end

-- init is initnpctimer: the timer starts again from 0.
function NpcTimer:init()
	self.started = nil
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
	if scope == "server" or scope == "server_temporary" then
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

-- NPCs by name (review decision 58). A command on an NPC that does not
-- exist does nothing, as rAthena only logs an error.

-- npc_named is the handle of an NPC: a record name, or a handle.
local function npc_named(target)
	if type(target) == "string" then
		-- npc_name2id finds no NPC for an empty name.
		if target == "" then
			return nil
		end
		return world.get_npc(target)
	end
	return target
end

-- forced_walks are the walks unitwalk forced, by NPC: rAthena's
-- ud->state.force_walk and the cell it walks to (script.cpp 19996).
local forced_walks = {}

-- unitwalk is unitwalk on an NPC: refused while an earlier forced walk
-- has not reached its cell, and else started after a 50 ms timer, as
-- rAthena's unit_delay_walktoxy_timer. It gives 1, or 0 when refused.
-- force_walk ends on arrival, and with unitstopwalk and npcstop.
function R.unitwalk(target, x, y)
	local n = npc_named(target)
	if n == nil then
		return 0
	end
	local key = tostring(n)
	local f = forced_walks[key]
	if f ~= nil then
		if n.x ~= f[1] or n.y ~= f[2] then
			-- "already been forced to walk".
			return 0
		end
		forced_walks[key] = nil
	end
	forced_walks[key] = { x, y }
	timers.after(50, function()
		if not n:walk_to(x, y) then
			forced_walks[key] = nil
		end
	end)
	return 1
end

-- npc_stop is unitstopwalk and npcstop on an NPC: the walk stops, and so
-- does a forced walk.
function R.npc_stop(target)
	local n = npc_named(target)
	if n == nil then
		return
	end
	forced_walks[tostring(n)] = nil
	n:stop()
end

-- or_self is the NPC of npctalk's and classchange's name argument: the
-- NPC itself for an empty name.
function R.or_self(target, self)
	if target == "" then
		return self
	end
	return target
end

-- npc_view is enablenpc, disablenpc, hideonnpc and hideoffnpc, the
-- cloaks for everyone, and the presentation commands of review decision
-- 58, area B5: a method of the NPC's handle, and what it returns.
function R.npc_view(target, method, ...)
	local n = npc_named(target)
	if n ~= nil then
		return n[method](n, ...)
	end
end

-- say_aloud is npctalk of a line made when the script runs: an empty
-- line, which rAthena sends, is left out, as the API refuses it.
function R.say_aloud(target, text, options)
	text = R.text(text)
	local n = npc_named(target)
	if n ~= nil and text ~= "" then
		n:say_aloud(text, options)
	end
end

-- title is setunittitle's title made when the script runs: safestrncpy
-- cuts it to NAME_LENGTH - 1 bytes, at a character's start.
function R.title(text)
	text = R.text(text)
	if #text <= 23 then
		return text
	end
	local n = 23
	while n > 0 and (text:byte(n + 1) or 0) >= 0x80 and (text:byte(n + 1) or 0) < 0xC0 do
		n = n - 1
	end
	return text:sub(1, n)
end

-- npc_display is setnpcdisplay: the name players see and the class, or
-- nil for no change. It gives 1 for an NPC that does not exist, and 0
-- otherwise.
function R.npc_display(target, name, class)
	local n = npc_named(target)
	if n == nil then
		return 1
	end
	if name ~= nil then
		n:set_display_name(name)
	end
	if class ~= nil then
		n:set_sprite(class)
	end
	return 0
end

-- npc_cloak_for is the cloak of one player: cloakonnpcself and
-- cloakoffnpcself, or the cloaks with getcharid(0). Without an attached
-- player it does nothing, as script_rid2sd.
function R.npc_cloak_for(target, cloaked, player)
	local n = npc_named(target)
	if n ~= nil and player ~= nil then
		n:set_cloaked(cloaked, player)
	end
end

-- is_cloaked is isnpccloaked for the attached player: false for an NPC
-- that does not exist or without a player.
function R.is_cloaked(target, player)
	local n = npc_named(target)
	if n == nil or player == nil then
		return false
	end
	return n:is_cloaked(player)
end

-- fire is donpcevent "<NPC>::<label>": whether the NPC has the label.
function R.fire(target, label)
	local n = npc_named(target)
	if n == nil then
		return false
	end
	return n:fire_event(label)
end

-- doevent runs the label for the attached player, or does nothing
-- without one, as script_rid2sd.
function R.doevent(target, label, player)
	local n = npc_named(target)
	if n ~= nil and player ~= nil then
		n:fire_event(label, { player = player })
	end
end

-- donpcevent is donpcevent of an event name made when the script runs:
-- "<NPC>::<label>", or "::<label>" for every NPC with the label.
function R.donpcevent(text)
	local name, label = tostring(text):match("^(.*)::([%a_][%w_]*)$")
	if label == nil then
		return false
	end
	if name == "" then
		return world.fire_npc_event(label) > 0
	end
	return R.fire(name, label)
end

-- npc_event_of is the death event of a monster whose label is another
-- NPC's: the label runs for the killer, as mob_dead's npc_event, or
-- with no player.
function R.npc_event_of(target, label)
	return function(player)
		if player ~= nil then
			R.doevent(target, label, player)
		else
			R.fire(target, label)
		end
	end
end

-- npc_of is the NPC of getvariableofnpc, which ends the script for an
-- NPC that does not exist, as rAthena's does.
function R.npc_of(target)
	local n = npc_named(target)
	if n == nil then
		R.stop()
	end
	return n
end

-- player_field reads a field of a player handle, or the default without
-- a player.
function R.player_field(p, field, default)
	if p == nil then
		return default
	end
	return p[field]
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

-- mapannounce is mapannounce of a computed text. An empty text shows
-- nothing, so it is left out.
function R.mapannounce(map, text)
	if text ~= "" then
		maps.announce(map, text)
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
function R.party_members(party, field)
	local out = {}
	if party == nil then
		return out, 0
	end
	if field ~= nil then
		-- getpartymember types 1 and 2: character and account IDs (review
		-- decision 58).
		local list = party:get_member_list()
		for i, m in ipairs(list) do
			out[i - 1] = m[field]
		end
		return out, #list
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

-- The player, item and clock helpers of review decision 58, areas B7,
-- B9 and B10 (decision 0064).

-- pcblock_names are player:set_blocked's actions by PCBLOCK_ bit.
local pcblock_names = { "move", "attack", "skill", "use_item", "chat", "immune", "sit_stand", "commands", "npc_click", "emotion", "equip" }

-- set_pc_block is setpcblock with a computed flag: each PCBLOCK_ bit.
function R.set_pc_block(player, flag, on)
	for i, name in ipairs(pcblock_names) do
		if flag & (1 << (i - 1)) ~= 0 then
			player:set_blocked(name, on)
		end
	end
end

-- pc_block is getpcblock: the PCBLOCK_ bits the player has.
function R.pc_block(player)
	local flag = 0
	for i, name in ipairs(pcblock_names) do
		if player:is_blocked(name) then
			flag = flag | (1 << (i - 1))
		end
	end
	return flag
end

-- lock_items is disable_items and enable_items. An event without a
-- player has no conversation, and rAthena's flag then does nothing.
function R.lock_items(conversation, locked)
	if conversation == nil then
		return
	end
	if locked then
		conversation:lock_items()
	else
		conversation:unlock_items()
	end
end

-- pushpc is pushpc: a direction past 7 wraps, as buildin_pushpc's
-- dir %= DIR_MAX, and 0 cells does nothing.
function R.pushpc(player, dir, cells)
	if cells == 0 then
		return
	end
	player:push(dir % 8, math.max(math.min(cells, 25), -25))
end

-- item_info is getiteminfo by its type (script.hpp 2238): -1 for an
-- unknown item or type, and "" for the AegisName of an unknown item.
local iteminfo_fields = {
	[0] = "buy", "sell", "type", false, "gender", "locations", "weight", "attack", "defense", "range", "slots", "view",
	"min_level", "weapon_level", false, "max_level", "magic_attack", "id", "aegis_name", "armor_level", "subtype",
}
local item_types = { healing = 0, usable = 2, etc = 3, armor = 4, weapon = 5, card = 6, petegg = 7, petarmor = 8, ammo = 10,
	delayconsume = 11, shadowgear = 12, cash = 18 }
function R.item_info(item, typ)
	local info = items.get(item)
	if typ == 18 and info == nil then
		return ""
	end
	local field = iteminfo_fields[typ]
	if info == nil or not field then
		return -1
	end
	if field == "type" then
		return item_types[info.type] or -1
	end
	if field == "gender" then
		-- SEX_FEMALE 0, SEX_MALE 1, SEX_BOTH 2.
		return ({ female = 0, male = 1 })[info.gender] or 2
	end
	if field == "view" and (info.type == "weapon" or info.type == "ammo") then
		-- ITEMINFO_VIEW is the subtype of a weapon or ammunition.
		return info.subtype
	end
	return info[field]
end

-- cards is getitem2's four card arguments as the list of filled slots.
-- card[0] of a forged, named or pet item marks it, which the API does not
-- take.
function R.cards(...)
	local out = {}
	for i = 1, select("#", ...) do
		local c = select(i, ...)
		if i == 1 and (c == 0xFF or c == 0xFE or c == -0x100) then
			R.unsupported("a forged, named or pet item's mark in card 1")
		end
		if c ~= 0 then
			out[#out + 1] = c
		end
	end
	return out
end

-- broken_id is getbrokenid: the nth broken item, from 1, or 0.
function R.broken_id(player, n)
	return player:get_broken_items()[n] or 0
end

-- equip_can_refine is getequipisenableref: 1 when the item in the slot
-- may be refined, else 0.
function R.equip_can_refine(player, slot)
	local e = R.equipped(player, slot)
	if e ~= nil and items.can_refine(e) then
		return 1
	end
	return 0
end

-- equip_refine_chance is getequippercentrefinery: the chance in
-- percent, 0 without an item.
function R.equip_refine_chance(player, slot)
	local e = R.equipped(player, slot)
	if e == nil then
		return 0
	end
	return items.refine_chance(e)
end

-- equip_refine_cost is getequiprefinecost with REFINE_COST_NORMAL: the
-- material's item ID (info 0) or the zeny (info 1), and -1 without an
-- item or a cost.
function R.equip_refine_cost(player, slot, info)
	local e = R.equipped(player, slot)
	if e == nil then
		return -1
	end
	local zeny, material = items.refine_cost(e)
	if zeny == nil then
		return -1
	end
	if info == 0 then
		return material
	end
	return zeny
end

-- refitem is successrefitem and downrefitem: the new refine, or -1
-- without an item.
function R.refitem(player, slot, amount)
	local e = R.equipped(player, slot)
	if e == nil then
		return -1
	end
	if amount == 0 then
		return e.refine
	end
	return e:add_refine(math.max(math.min(amount, 20), -20))
end

-- failedrefitem destroys the item in the slot: 1, or 0 without one.
function R.failedrefitem(player, slot)
	local e = R.equipped(player, slot)
	if e == nil then
		return 0
	end
	e:destroy()
	return 1
end

-- delequip deletes the item in the slot and gives 1. Without one,
-- buildin_delequip ends the script with an error.
function R.delequip(player, slot)
	local e = R.equipped(player, slot)
	if e == nil then
		R.unsupported("delequip of an empty slot, which ends rAthena's script")
	end
	e:destroy()
	return 1
end

-- named_item is getnameditem: the item named for a player of the map,
-- by handle, name or character ID. 1, or 0 when the player is not here
-- or the item does not fit.
function R.named_item(player, item, who)
	if type(who) == "string" then
		who = maps.find_player(who)
	elseif math.type(who) == "integer" then
		who = maps.find_player_by_id(who)
	end
	if who == nil then
		return 0
	end
	return player:give_item(item, 1, { signed_by = who }) and 1 or 0
end

-- rentitem gives one rental. A time of 0 or less gives a rental that
-- ends at once, as rAthena deletes it as soon as pc_additem gave it.
function R.rentitem(player, item, seconds, options)
	if seconds <= 0 then
		return
	end
	options = options or {}
	options.rent_seconds = seconds
	player:give_item(item, 1, options)
end

-- makeitem drops items as makeitem: nothing for an amount below 1.
function R.makeitem(map, x, y, item, amount, options)
	if amount < 1 then
		return
	end
	maps.drop_item(map, x, y, item, math.min(amount, 30000), options)
end

-- setdragon is setdragon (script.cpp 22824): a Rune Knight's dragon on or
-- off. 1 when it changed, else 0.
function R.setdragon(player)
	if player:has_option("dragon") then
		return player:set_mount(nil) and 1 or 0
	end
	return player:set_mount(mounts.id("dragon")) and 1 or 0
end

-- setmadogear is setmadogear's MADO_ROBOT.
function R.setmadogear(player, on)
	if on then
		player:set_mount(mounts.id("madogear"))
	elseif player:has_option("madogear") then
		player:set_mount(nil)
	end
end

-- gettimetick is gettimetick: 2 is Unix seconds, 1 the seconds since
-- midnight, and the rest milliseconds.
function R.gettimetick(typ)
	if typ == 2 then
		return clock.now()
	elseif typ == 1 then
		return clock.get("hour") * 3600 + clock.get("minute") * 60 + clock.get("second")
	end
	return clock.milliseconds()
end

-- gettime is gettime with a computed type: -1 outside DT_.
local gettime_fields = { "second", "minute", "hour", "day_of_week", "day_of_month", "month", "year", "day_of_year", "date" }
function R.gettime(typ)
	local field = gettime_fields[typ]
	if field == nil then
		return -1
	end
	return clock.get(field)
end

-- gettimestr is gettimestr: strftime gets maxlen as its size, so at most
-- maxlen - 1 bytes of the text.
function R.gettimestr(format, maxlen, at)
	local text = clock.format(format, at)
	if maxlen < 1 then
		return ""
	end
	return text:sub(1, maxlen - 1)
end

-- inventory_list is getinventorylist: the @inventorylist_ arrays the
-- script reads, from 0, and @inventorylist_count.
local bound_values = { account = 1, guild = 2, party = 3, character = 4 }
function R.inventory_list(player, vars)
	local list = player:get_inventory()
	local arrays = {}
	for field in pairs(vars) do
		arrays[field] = {}
	end
	for i, e in ipairs(list) do
		local values = {
			id = e.item, idx = e.slot, amount = e.amount, equip = e.equip, refine = e.refine,
			identify = e.identified and 1 or 0, attribute = e.broken and 1 or 0, expire = e.expires,
			bound = bound_values[e.bound] or 0, enchantgrade = 0, tradable = e.tradable and 1 or 0, favorite = 0,
			card1 = e.cards[1] or 0, card2 = e.cards[2] or 0, card3 = e.cards[3] or 0, card4 = e.cards[4] or 0,
		}
		for field, t in pairs(arrays) do
			if field ~= "count" then
				t[i - 1] = values[field]
			end
		end
	end
	for field, var in pairs(vars) do
		if field == "count" then
			var:set(player, #list)
		else
			store(var, player, arrays[field])
		end
	end
end

return R
