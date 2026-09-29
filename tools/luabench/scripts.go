package main

import (
	"fmt"
	"strings"
)

// Dialect selects the Lua version a script is written for.
type Dialect int

const (
	Lua54 Dialect = iota
	// Lua51 is for gopher-lua. It has no // or & operators, so the
	// scripts use math.floor and a host function, as the converter would.
	Lua51
)

// prelude defines the constants the workloads use.
const prelude = `
bLuk = 5
bFlee2 = 50
bBaseAtk = 17
BF_WEAPON = 1
BF_MAGIC = 2
`

// workloads is the Lua 5.4 source of each workload.
// Each global function takes one integer and returns one integer.
const workloads54 = `
-- Poring Card: a static bonus, two host calls.
function static_equip(_)
  wa_add(bLuk, 2)
  wa_add(bFlee2, 1)
  return 0
end

-- A computed bonus that reads one player field.
function computed(_)
  local lv = pc_get("base_level")
  return lv > 70 and (lv - 70) // 10 * 10 or 0
end

-- A proc on being hit, with a flag test and a chance roll.
function hit_taken(flag)
  if flag & (BF_WEAPON | BF_MAGIC) ~= 0 and wa_chance(3, 1000) then
    wa_add(bFlee2, 30)
    return 1
  end
  return 0
end

-- Pure Lua work: table fill, sort, integer math, and string building.
function compute(n)
  local t = {}
  for i = 1, n do t[i] = (i * 7919) % 1000 end
  table.sort(t)
  local s = 0
  for i = 1, n do s = s + t[i] // 3 end
  local parts = {}
  for i = 1, 20 do parts[#parts + 1] = string.format("%d", t[i]) end
  return s + #table.concat(parts, ",")
end
`

const workloads51 = `
function static_equip(_)
  wa_add(bLuk, 2)
  wa_add(bFlee2, 1)
  return 0
end

function computed(_)
  local lv = pc_get("base_level")
  return lv > 70 and math.floor((lv - 70) / 10) * 10 or 0
end

function hit_taken(flag)
  if bit_and(flag, bit_or(BF_WEAPON, BF_MAGIC)) ~= 0 and wa_chance(3, 1000) then
    wa_add(bFlee2, 30)
    return 1
  end
  return 0
end

function compute(n)
  local t = {}
  for i = 1, n do t[i] = (i * 7919) % 1000 end
  table.sort(t)
  local s = 0
  for i = 1, n do s = s + math.floor(t[i] / 3) end
  local parts = {}
  for i = 1, 20 do parts[#parts + 1] = string.format("%d", t[i]) end
  return s + #table.concat(parts, ",")
end
`

// npc is the Healer dialog from strategy section 5.2. mes and menu yield
// the coroutine, and Go resumes it with the player's answer.
// npc_start and npc_step return 0 when the dialog ends.
const npc = `
local co
local function mes(_) coroutine.yield(1) end
local function menu(_, _) return coroutine.yield(2) end
local function healer()
  mes("[Healer]")
  mes("Do you want to be healed for 500 zeny?")
  if menu("Yes", "No") == 1 then
    local zeny = pc_get("zeny")
    if zeny < 500 then
      mes("You do not have enough zeny.")
      return
    end
    pc_set("zeny", zeny - 500)
    mes("Done.")
  end
end
local function status(ok, w)
  if not ok then error(w) end
  if coroutine.status(co) == "dead" then return 0 end
  return w
end
function npc_start(_)
  co = coroutine.create(healer)
  return status(coroutine.resume(co))
end
function npc_step(answer)
  return status(coroutine.resume(co, answer))
end
`

// spin and hog test the sandbox limits.
const sandbox = `
function spin(_)
  while true do end
end
function hog(_)
  local t = {}
  for i = 1, 1e9 do t[i] = {i} end
  return 0
end
`

// Script returns the workload source for a dialect.
func Script(d Dialect) string {
	w := workloads54
	if d == Lua51 {
		w = workloads51
	}
	return prelude + w + npc + sandbox
}

// ItemScripts returns n item scripts in the event-driven form from strategy
// section 5.3. Each is loaded as its own chunk, as the server loads item files.
func ItemScripts(n int) []string {
	stats := []string{"bLuk", "bFlee2", "bBaseAtk"}
	out := make([]string, n)
	for i := range out {
		var b strings.Builder
		fmt.Fprintf(&b, "items = items or {}\nitems[%d] = {\n", 4000+i)
		b.WriteString("  on_equip = function(pc, it, mods)\n")
		for j := range 1 + i%3 {
			fmt.Fprintf(&b, "    wa_add(%s, %d)\n", stats[(i+j)%3], 1+(i*7+j)%10)
		}
		b.WriteString("  end,\n")
		if i%4 == 0 {
			fmt.Fprintf(&b, "  on_use = function(pc, it)\n    if pc_get(\"base_level\") > %d then wa_add(bLuk, 1) end\n  end,\n", i%99)
		}
		b.WriteString("}\n")
		out[i] = b.String()
	}
	return out
}
