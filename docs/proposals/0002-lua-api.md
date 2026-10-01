# 0002: Lua Scripting API

- Status: accepted.
- Runtime: [internal/script](../../internal/script) ([decision 0002](../decisions/0002-lua-runtime-c-lua.md)).
- Replaces: strategy sections 5.1 to 5.3 and 6.8, where they differ from this proposal.

## Problem

Scripts for NPCs, items, skills, events, and mods need an API. Strategy section 5 drafted one with a global `wa` table, global constants with rAthena's names, and item files that return a table of handlers. These rules change that draft:

1. **No globals.** The API is not placed in the global table. Scripts get every part of it with `require`.
2. **Entry scripts return a function, not a table.** The runtime calls that function with a hidden context. Scripts register for events by calling an internal module, not by returning handlers.
3. **Readable names.** Every name in the API is written out in full words, except the stat abbreviations players know from the status window, such as `str`, `luk`, `atk`, and `aspd` ([0002a](0002a-bonus-names.md), rule 1). There are no obscure names, such as `bFlee2`, `SC_STUN`, or `EQI_HAND_R`.

The API should also fix the problems of rAthena's script language. They are listed in the comparison at the end.

## Change

### Entry scripts and helper modules

There are two kinds of Lua files in a mod:

- **Entry scripts** are the files wAthena loads itself: the scripts that NPCs, items, skills, monsters, and the mod manifest point to. They must return a setup function. The manifest lists a mod's own entry scripts in its `scripts` field, as paths under `scripts/`.
- **Helper modules** are any other files the mod loads with `require`, such as `scripts/lib/rewards.lua`. They can return any value: a table of functions, a single function, or data. They have no rules of their own.

An entry script:

```lua
-- Requires and constants run once, when the file loads.
local events = require "wa.events"
local items = require "wa.items"
local rewards = require "example.halloween.lib.rewards"   -- the mod's own helper

local RED_POTION = items.id "Red_Potion"   -- checked at load

-- The setup function runs once per instance, inside its context.
return function()
  events.on("use", function(player)
    player:item_heal { hp = { 45, 65 } }
  end)
end
```

Rules for entry scripts:

- The file must return a function. A file that returns anything else fails to load, with the file name in the error.
- Code outside the setup function runs once per Lua state, when the file loads. It may `require` modules and compute values. Calls that need a context, such as registering an event, fail there.
- The runtime calls the setup function once for each **instance** of the script. Each instance has its own context.
- Locals declared inside the setup function belong to that instance. They replace rAthena's NPC variables (`.name`).

Helper modules are loaded once per Lua state, like any Lua module. A helper that calls a context-dependent function, such as `events.on`, works when it is called from inside a setup function or a handler, and fails when it is called while its file loads.

### The hidden context

The context is the owner of everything a setup function creates. Scripts do not pass it around. The runtime sets it before it calls the setup function, and again before it calls any handler that the setup function registered.

| Script kind | One instance is | The context ends when |
|---|---|---|
| Mod script | The mod, once per zone | The mod is unloaded or reloaded |
| NPC | One placed NPC. A script placed 5 times has 5 instances. | The NPC is removed, or its file is reloaded |
| Item | One equipped copy, or one use of a usable item | The copy is unequipped, or the use ends |
| Skill | The skill definition, once per zone | The mod is unloaded |
| Monster AI | One spawned monster | The monster dies or is removed |
| Scheduled event | One run of the event | The run ends |

What the context does:

- **Ownership.** Event handlers, timers, bonuses, spawned monsters, and hooks belong to the context that made them. When the context ends, the runtime removes all of them at once. An unequipped item leaves no listener behind.
- **Permissions.** The context knows its mod, so each call is checked against the mod's approved permissions (strategy section 6.6).
- **Variables.** Saved variables are stored under the context's mod (strategy section 6.7).

Scripts read the context through `wa.context`:

```lua
local context = require "wa.context"

return function()
  local current = context:get_instance()
  -- current is the NPC, the equipped item, the monster, or the mod.
end
```

- `context:get_instance()` returns the instance object. Its fields depend on the kind: an equipped item has `item`, `wearer`, and `refine`, an NPC has `name`, `map`, `x`, and `y`, and a mod has `id` (the mod ID) and `map` (the zone's map).
- `context:get_kind()` returns the kind as a string: `"mod"`, `"npc"`, `"item"`, `"skill"`, `"monster"`, or `"event"`.
- `context:get_mod()` returns the mod's ID.
- Calling these outside a setup function or a handler raises an error.

What the context never does:

- **It never holds a player.** rAthena's attached player (RID) is the source of many script bugs. In this API, a player is always an argument to a handler, and every call that acts on a player takes it explicitly.

### Events

Scripts register handlers with `events.on`, which takes the event's name:

```lua
local events = require "wa.events"

return function()
  events.on("click", function(player, conversation) ... end)   -- NPC clicked
  events.on("touch", function(player) ... end)                 -- walked into the NPC's area
  events.on("login", function(player) ... end)                 -- mod context only
  events.on("monster_killed", function(player, monster) ... end)
  events.on("base_level_up", function(player) ... end)
  events.on("job_level_up", function(player) ... end)
end
```

- `monster_killed` gets a monster handle, like the handles in "Players and other entities". The monster is gone, but the handle stays readable while the event's handlers run, and its `is_alive` field is `false`. After the event, reading it raises an error, as for any handle to a removed entity.
- `base_level_up` and `job_level_up` are separate events. Each gets the player.

- `events.on` checks the name when it is called. An unknown name, or a name the context's kind cannot use, raises an error during setup, so a typo still fails before the event ever fires.
- Which events a context can register depends on its kind. `"click"` exists only for NPCs, and `"damage"` and `"skill_used"` only for equipped items.
- Many handlers can listen to the same event. They run in registration order, and a failing handler does not stop the others.
- A handler returns nothing. Changing a result is the job of hooks.
- `events.on` returns a handle with a `remove` method, for handlers that should stop before their context ends.

### Hooks

Hooks change a game result. They run in a chain ordered by priority. Each hook calls `next` to get the rest of the chain's result.

```lua
local hooks = require "wa.hooks"

return function()
  hooks.on("drop_rate", function(drop, next)
    local rate = next(drop)
    return drop.map.flags.event_zone and rate * 2 or rate
  end)
end
```

Hooks need the `rules.override` permission. Hook names are checked the same way as event names.

### Players and other entities

Entities are handles: userdata holding an entity ID. Each access looks the entity up again. A handle to a player who logged out raises a Lua error, and a conversation with that player closes.

**Reading** uses fields:

```lua
player.name        player.base_level    player.job_level    player.job
player.zeny        player.hp            player.max_hp       player.sp
player.max_sp      player.map           player.x            player.y
player.party       -- nil without a party
player.character_id                     player.account_id   -- database IDs
player.str        player.base_str      -- total, and allocated points only
-- The same for agi, vit, int, dex, and luk.
```

Monster handles have `id`, `name`, `level`, `hp`, `max_hp`, `map`, `x`, `y`, and `is_alive`.

HP, SP, zeny, and the stats keep the names players know from the game, such as `hp`, `str`, and `luk` ([0002a](0002a-bonus-names.md), rule 1).

**Changing** uses methods:

```lua
player:pay_zeny(500)              -- returns false if the player has too little
player:give_zeny(500)             -- needs player.give_zeny
player:give_item(RED_POTION, 3)   -- needs player.give_items; returns false if it does not fit
player:take_item(RED_POTION, 1)
player:count_item(RED_POTION)
player:warp("prontera", 156, 191)
player:heal_fully()
player:item_heal { hp = { 45, 65 } }   -- pc_itemheal, with its bonuses
player:show_effect(effect.heal)
player:send_message("Welcome!")   -- a line in the chat log, as CHAT_CHANNEL_SYSTEM
```

Fields are always read-only, on every object. Writing to a field raises an error. Every change is a method call, so it can report failure, check permissions, and run as one step.

Lists are returned as Lua arrays, never through magic variables:

```lua
for _, slot in ipairs(player:get_inventory()) do
  if slot.item == RED_POTION then ... end
end
```

### NPC conversations

The click handler gets a `conversation` object. Its methods pause the handler until the player answers. Each conversation runs in its own coroutine, through the runtime's pausing threads.

```lua
local events = require "wa.events"
local text = require "wa.text"

return function()
  events.on("click", function(player, conversation)
    conversation:say("[Healer]", text "healer.greeting")
    if conversation:choose(text "common.yes", text "common.no") == 1 then
      if not player:pay_zeny(500) then
        conversation:say(text "common.not_enough_zeny")
        return conversation:close()
      end
      player:heal_fully()
    end
    conversation:close()
  end)
end
```

- `conversation:say(...)` shows lines. `conversation:wait_for_next()` waits for the player to press next.
- `conversation:choose(...)` returns the 1-based index of the chosen option.
- `conversation:ask_number(minimum, maximum)` and `conversation:ask_text(maximum_length)` read input.
- `conversation:close()` ends the conversation. A handler that returns without closing closes it.
- If the player leaves, the conversation's coroutine is closed. Its pending call raises a Lua error that the runtime does not log as a failure.

NPC placement lives in data, not in the script:

```yaml
# data/npcs.yml
- name: Healer#prontera
  map: prontera
  x: 150
  y: 180
  direction: south
  display_name: Healer
  sprite: 4_F_KAFRA1
  script: scripts/npc/healer.lua
```

- A name without a mod ID is the mod's own NPC: `Healer#prontera` in `example.town` is `example.town:Healer#prontera`.
- Players see `display_name`, or the name up to `#` without it.
- `direction` is a named direction: `south` (the default), `southwest`, `west`, `northwest`, `north`, `northeast`, `east`, or `southeast`.
- `sprite` is an NPC sprite name, such as `4_F_KAFRA1`, or its number.

Placing the same script twice replaces rAthena's `duplicate`.

### Item scripts

An equip script's setup function runs when a copy is equipped. Bonuses it adds belong to that copy and disappear when it is unequipped.

```lua
local bonuses = require "wa.bonuses"
local classes = require "wa.monster_classes"
local context = require "wa.context"

return function()
  local equipped = context:get_instance()
  local player = equipped.wearer
  bonuses.add("luk", 2)
  bonuses.add("perfect_dodge", 1)
  -- Computed: reruns only when a value it read changes.
  bonuses.add("atk", function()
    local level = player.base_level
    return level > 70 and (level - 70) // 10 * 10 or 0
  end)
  bonuses.add("aspd", function() return 4 + equipped.refine end, { percent = true })
  bonuses.add("damage", equipped.rolls.boss_damage,
    { kind = "physical", against = { class = classes.boss } })
end
```

A proc on being hit:

```lua
local bonuses = require "wa.bonuses"
local events = require "wa.events"
local random = require "wa.random"
local timers = require "wa.timers"
local effects = require "wa.effects"
local sizes = require "wa.sizes"
local context = require "wa.context"

return function()
  local player = context:get_instance().wearer
  bonuses.add("damage_taken", -15, { from = { size = sizes.all } })
  events.on("damage", function(hit)
    if not hit.wearer_is_source and (hit.kind == "physical" or hit.kind == "magic") then
      if random.chance(3, 1000) then
        timers.with_bonuses(10000, function() bonuses.add("perfect_dodge", 30) end)
        player:show_effect(effects.wind)
      end
    end
  end)
end
```

- Bonus types are names, checked by `bonuses.add` like event names. rAthena's 241 bonus types each get a name, such as `bLuk` to `"luk"` and `bFlee2` to `"perfect_dodge"`. The full list is in [0002a](0002a-bonus-names.md).
- Damage dealt and taken by percent are two bonuses, `"damage"` and `"damage_taken"`. An options table says which hits they apply to: `kind`, `range`, `element`, `skill`, and `against` or `from` with a `race`, `element`, `size`, `class`, `monster_group`, `monster`, or `job`. Values are summed in rAthena's own groups, so results match rAthena.
- The last argument of `bonuses.add` can be an options table. Variants of one quantity share its name and differ by option:
  - `{ percent = true }` for a percent instead of a flat value: `bonuses.add("max_hp", 10, { percent = true })`;
  - `skill`, `item`, or `item_group` for one skill or item: `bonuses.add("sp_cost", -10, { percent = true, skill = BASH })`;
  - `against` or `from` for one kind of target or source.
- Procs are named after their effect. The chance (a whole number per 10,000) and the trigger are options: `bonuses.add("cause_status", statuses.stun, { chance = 300, when = "hit_taken" })`. `when` is `"attack"` (the default), `"hit_taken"`, or `"skill_used"`.
- The `stacking` option says how bonuses of one name combine: `"sum"` (the default), `"highest"`, or `"lowest"`. Six bonuses keep one value, such as `attack_element`: their default `"last"` follows rAthena's order, and `{ stacking = "priority", priority = n }` lets a mod choose.
- Arguments come in a fixed order: what the bonus produces or applies to, then the amount, then other numbers, then the options table.
- Equipped items get two events. `"damage"` fires for every hit the wearer deals or takes. Its object has `source`, `target`, `kind`, `range`, `skill`, `element`, `is_critical`, `amount`, and `wearer_is_source`. `"skill_used"` fires for every skill the wearer uses, also skills that deal no damage. Its object has `skill`, `level`, `target`, `x`, and `y`.
- `player:add_timed_bonuses(key, ms, bonuses, options)` gives a player bonuses for a time, saved across logout. It replaces rAthena's `bonus_script`.
- The rules from strategy section 5.3 stay: conditions read base stats only, computed functions have no side effects, results do not depend on equip order, and scripts with only constants become static data.

### Timers and schedules

```lua
local timers = require "wa.timers"
local schedules = require "wa.schedules"

return function()
  local timer = timers.after(5000, function() ... end)   -- once
  timers.every(60000, function() ... end)                -- repeating
  timer:cancel()
  schedules.daily("00:00", function() ... end)           -- server time zone
end
```

Timers belong to the context, so they stop when it ends.

`timers.with_bonuses(ms, function() ... end)` adds bonuses that end after `ms`. It returns a timer. The option `{ restart = timer }` cancels that timer first, if it still runs, so a proc that triggers again restarts its duration instead of stacking.

### Variables

Saved variables are declared once, at the top of the file, with a scope and a type. Reads and writes go through the declared object.

```lua
local events = require "wa.events"
local variables = require "wa.variables"

local quest_step = variables.character_integer "quest_step"          -- character, saved
local visited_inn = variables.account_boolean "visited_inn"          -- account, saved
local boss_kills = variables.server_integer "boss_kills"             -- server, saved
local busy = variables.character_temporary_boolean "busy"            -- character, until logout

return function()
  events.on("click", function(player, conversation)
    if quest_step:get(player) == 2 then
      quest_step:set(player, 3)
    end
  end)
end
```

- Every variable is stored under the context's mod. Two mods can use the same name.
- Types are `integer`, `boolean`, `string`, and `table` (plain data only, saved as `JSONB`).
- Per-instance state is a Lua local inside the setup function. Per-call state is a Lua local inside the handler.

### Constants and IDs

Constants live in their own read-only modules, with readable lowercase names:

```lua
local effects = require "wa.effects"     -- effects.heal, effects.wind
local races = require "wa.races"         -- races.demi_human, races.undead
local statuses = require "wa.statuses"   -- statuses.stun, statuses.blessing
local jobs = require "wa.jobs"           -- jobs.knight, jobs.high_priest
local sizes = require "wa.sizes"         -- sizes.small, sizes.all
local elements = require "wa.elements"   -- elements.fire, elements.holy
local classes = require "wa.monster_classes"   -- classes.boss, classes.normal
local weapon_types = require "wa.weapon_types"  -- weapon_types.bow, weapon_types.dagger
local monster_groups = require "wa.monster_groups"  -- monster_groups.goblin, monster_groups.orc
```

- Writing to a constants module raises an error.
- Game records are found by their database name when the file loads: `items.id "Red_Potion"`, `skills.id "SM_BASH"`, `monsters.id "PORING"`, and item groups with `items.group "Potion"`. An unknown name fails the load, with the file and line. Record names come from the game data and stay as they are.
- There is no module with rAthena's constant names. The generated API reference has a table from each rAthena name to its new name, and the converter uses the same table.

### Mod exports

A mod offers functions to other mods with `wa.exports`:

```lua
-- example.seasonal-lib: scripts/exports.lua (an entry script listed in the manifest)
local exports = require "wa.exports"

return function()
  exports.provide("current_season", function() return "winter" end)
end
```

```lua
-- Another mod, which lists example.seasonal-lib in depends.
local exports = require "wa.exports"
local current_season = exports.get("example.seasonal-lib", "current_season")
```

`exports.get` checks the dependency when the file loads. Inside one mod, files share code by requiring helper modules instead.

### Module list

| Module | Gives | Permission |
|---|---|---|
| `wa.context` | The current instance, kind, and mod | None |
| `wa.events` | Event registration | `event.listen` |
| `wa.hooks` | Result hooks | `rules.override` |
| `wa.timers`, `wa.schedules` | Timers and schedules owned by the context | None, `events.schedule` for world events |
| `wa.variables` | Typed saved variables | `storage.mod` |
| `wa.storage` | Mod tables and key-value storage | `storage.mod` |
| `wa.bonuses` | Item bonuses | Item contexts only |
| `wa.items`, `wa.monsters`, `wa.skills`, `wa.maps` | Record lookups by name, and map queries | `game.read` |
| `wa.spawning` | Monsters and NPCs owned by the context | `npc.create` |
| `wa.random` | `random.integer(minimum, maximum)` and `random.chance(numerator, denominator)` from the zone's secret stream | None |
| `wa.integers` | `divide_truncated` and `remainder_truncated` with C rounding, for converted scripts | None |
| `wa.text` | Translated text for the player's language | None |
| `wa.exports` | Functions shared between mods | None |
| `wa.effects`, `wa.races`, `wa.statuses`, `wa.jobs`, `wa.sizes`, `wa.elements`, `wa.monster_classes`, `wa.weapon_types`, `wa.monster_groups` | Constants | None |
| `wa.testing` | Test players and conversations, in `wactl mod test` only | None |

Methods on player handles check their own permissions, such as `player.give_items` for `player:give_item`. A mod without a permission can still `require` the module. The call fails with an error that names the missing permission.

## Comparison with rAthena

### Who a call acts on

| rAthena | wAthena |
|---|---|
| Commands act on the attached player (RID), set by the engine or by `attachrid`. | Handlers receive the player as an argument. Every call names it: `player:give_item(...)`. |
| `getitem 501,3,getcharid(3,"Bob");` | `bob:give_item(RED_POTION, 3)` with a handle from `maps.find_player "Bob"`. |
| `detachrid;` to run without a player. | Not needed. The context never holds a player. |

### Reading values

| rAthena | wAthena |
|---|---|
| `BaseLevel`, `Zeny`, `Hp` (magic parameters) | `player.base_level`, `player.zeny`, `player.hp` |
| `Zeny -= 500;` | `player:pay_zeny(500)`, which returns false if the player has too little |
| `readparam(bStr)` | `player.base_str` (allocated) or `player.str` (total) |
| `strcharinfo(0)`, `strcharinfo(1)` | `player.name`, `player.party and player.party.name` |
| `getcharid(0)`, `getcharid(3)` | `player.character_id`, `player.account_id` |
| `getequipid(EQI_HAND_R)` | `player:get_equipped("right_hand")` |
| `countitem(501)` | `player:count_item(RED_POTION)` |
| `getskilllv("SM_BASH")` | `player:get_skill_level(BASH)` with `BASH = skills.id "SM_BASH"` |
| `getinventorylist;` then `@inventorylist_id[i]` | `player:get_inventory()` returns an array |
| `getpartymember getcharid(1);` then `$@partymembername$[i]` | `player.party:get_members()` returns an array |

### Variables

| rAthena | wAthena |
|---|---|
| `quest` (character, saved) | `variables.character_integer "quest"` |
| `@quest` (character, temporary) | `variables.character_temporary_integer "quest"` |
| `#quest` / `##quest` (account) | `variables.account_integer "quest"` |
| `$kills` (server, saved) | `variables.server_integer "kills"` |
| `$@kills` (server, temporary) | A local inside a mod script's setup function |
| `.count` (NPC) | A local inside the setup function |
| `.@n` (this call) | A local inside the handler |
| `'room` (instance) | `variables.instance_integer "room"` |
| `name$` (string suffix) | The declared type: `variables.character_string "name"` |
| `setarray`, `getarraysize`, `cleararray` | Lua tables and `#` |

### Actions

| rAthena | wAthena |
|---|---|
| `getitem 501, 3;` | `player:give_item(RED_POTION, 3)` |
| `delitem 501, 1;` | `player:take_item(RED_POTION, 1)` |
| `warp "prontera", 156, 191;` | `player:warp("prontera", 156, 191)` |
| `percentheal 50, 0;` | `player:heal_percent { hp = 50 }` |
| `itemheal rand(45,65), 0;` | `player:item_heal { hp = { 45, 65 } }` |
| `sc_start SC_BLESSING, 60000, 10;` | `player:start_status(statuses.blessing, { milliseconds = 60000, level = 10 })` |
| `specialeffect EF_HEAL;` | `player:show_effect(effects.heal)`, or the same on the NPC from `context:get_instance()` |
| `monster "prt_fild08",0,0,"--ja--",1002,10,"NPC::OnKill";` | `spawning.monsters { map = "prt_fild08", monster = PORING, amount = 10, on_death = function(monster, killer) ... end }` |
| `rand(45, 65)` | `random.integer(45, 65)` |

### Conversations

| rAthena | wAthena |
|---|---|
| `mes "...";` | `conversation:say("...")` |
| `next;` | `conversation:wait_for_next()` |
| `select("A:B:C")` | `conversation:choose("A", "B", "C")` |
| `menu "A",L_A,"B",L_B;` | `conversation:choose` with an `if`, or a table of functions |
| `input .@n;` | `local amount = conversation:ask_number(0, 100)` |
| `close;` / `end;` | `conversation:close()` / `return` |

### Events

| rAthena | wAthena |
|---|---|
| `OnInit:` | The setup function itself |
| `OnTouch:` | `events.on("touch", handler)` |
| `OnPCLoginEvent:` | `events.on("login", handler)` in a mod script |
| `OnClock0000:` | `schedules.daily("00:00", handler)` |
| `OnTimer5000:` with `initnpctimer` | `timers.after(5000, handler)` |
| `OnMyMobDead:` named in `monster` | `on_death` in `spawning.monsters`, owned by the context |
| `donpcevent "Guard#1::OnAlarm";` | An export the guard provides, or a shared helper module |
| `enablenpc` / `disablenpc` | `npc:show()` / `npc:hide()` on the NPC from `context:get_instance()` |

### Item scripts

| rAthena | wAthena |
|---|---|
| `Script:` with `bonus bStr,5;` | Equip setup: `bonuses.add("str", 5)` |
| `bonus2 bAddRace,RC_DemiHuman,10;` | `bonuses.add("damage", 10, { kind = "physical", against = { race = races.demi_human } })` |
| `bonus2 bSubSize,Size_All,15;` | `bonuses.add("damage_taken", -15, { from = { size = sizes.all } })` |
| `if (getrefine() >= 7) bonus bBaseAtk,10;` | `bonuses.add("atk", function() return equipped.refine >= 7 and 10 or 0 end)` |
| `bonus bSpeedRate,25;` | `bonuses.add("move_speed", 25, { percent = true, stacking = "highest" })` |
| `bonus2 bSkillUseSPrate,"SM_BASH",10;` | `bonuses.add("sp_cost", -10, { percent = true, skill = BASH })` |
| `bonus2 bAddEff,Eff_Stun,300;` | `bonuses.add("cause_status", statuses.stun, { chance = 300 })` |
| `autobonus "{...}",50,5000,BF_WEAPON;` | `events.on("damage", ...)` with `hit.wearer_is_source`, `random.chance`, and `timers.with_bonuses` |
| `autobonus2` | The same `"damage"` event, with `not hit.wearer_is_source` |
| `autobonus3` | `events.on("skill_used", ...)` with a check on `use.skill` |
| `bonus_script "{...}",60;` | `player:add_timed_bonuses(key, 60000, { ... })` |
| `OnEquipScript:` / `OnUnequipScript:` | The equip setup function, and the context's end |
| Every equipped script reruns on each stat recalculation. | Bonuses are added once. Computed ones rerun only when a value they read changes. |

### Code structure

| rAthena | wAthena |
|---|---|
| `function script F_Heal {...}` (global) | A local function, or a helper module the mod requires |
| `callfunc "F_Heal", 50;` | `heal(50)`, or `require "example.mod.lib.healing"` |
| `getarg(0)` | Named Lua parameters |
| `callsub L_Sub, 1;` / `goto L_End;` | Local functions and `return` |
| `sleep2 1000;` | `timers.after(1000, handler)`, or `conversation:wait(1000)` |
| Errors are printed, and the script often carries on. | A Lua error stops that handler, is logged with the file and line, and closes any conversation. |

## Problems this fixes

- **Implicit player.** The context owns things but never holds a player.
- **Strings as references.** Records, events, bonus types, and exports are checked when the file loads or during setup.
- **Magic selectors.** `getcharid(3)` becomes `player.account_id`.
- **Results through globals.** Lists are returned as arrays.
- **Scope in the name.** Variables are declared with their scope and type.
- **Full reruns for items.** Bonuses are added once and removed with their owner.
- **Obscure names.** `bFlee2`, `SC_STUN`, and `EQI_HAND_R` become `"perfect_dodge"`, `statuses.stun`, and `"right_hand"`.
- **One flat namespace.** About 25 modules replace 672 commands and 10,700 global constants.

## Decisions taken in review

1. Entry scripts return a setup function. Helper modules a mod loads with `require` can return any value.
2. Scripts read the context with `wa.context` (`context:get_instance()`).
3. Events, hooks, and bonus types are registered by name with `events.on("name", handler)`. The name is checked when the handler is registered.
4. Names are readable full words, except the stat abbreviations of decision 10. rAthena's names exist only in the converter's table and the API reference.
5. Fields are read-only on every object, including the NPC or monster the context owns. Changes are methods, such as `npc:set_name("Night Merchant")` and `player:pay_zeny(300)`. Reasons:
   - A method can return failure. `player.zeny = player.zeny - 300` cannot say that the player has too little.
   - A method shows that a permission is needed. An assignment looks like plain arithmetic.
   - A method changes a value in one step. Reading a field and writing it back can undo a change made in between, such as a trade.
6. The mod loader (phase 3 step 2) added parts that this proposal did not name. They are accepted:
   - The manifest's `scripts` field, and `api: experimental` until API 1.0.
   - `player:send_message(text)`, sent as `CHAT_CHANNEL_SYSTEM`.
   - The mod context's instance fields `id` and `map`.
   - The NPC fields `direction` (a named direction), `display_name`, and `sprite`, and the mod ID prefix on NPC names.
   - The player fields `max_sp`, `character_id`, and `account_id`.
7. `monster_killed` passes a monster handle, not a table of what the monster was. The handle reads while the event runs, with `is_alive` false, and raises an error after it, like any handle to a removed entity. Mods use one kind of object for monsters.
8. `level_up` with a kind argument is split into `base_level_up` and `job_level_up`, each `function(player)`. A handler listens to the level it needs, without a check on a string.
9. Item scripts (phase 3 step 3) add these, for the commands rAthena's item scripts use most:

   | rAthena | wAthena |
   |---|---|
   | `getgroupitem` (item boxes) | `player:give_item_from_group("Old_Blue_Box")` |
   | `warp "Random",0,0;` (Fly Wing) | `player:warp_to_random_cell()` |
   | `warp "SavePoint",0,0;` (Butterfly Wing) | `player:warp_to_save_point()` |
   | `getenchantgrade()` | `equipped.enchant_grade`, next to `equipped.refine` |
   | `skill` (equipment that grants a skill) | `bonuses.add("skill", BASH, 3)` |
   | `eaclass()` | `player.job_class`, and tests such as `jobs.is_transcendent(player.job)` |
   | `hateffect` | `player:show_headgear_effect(effect)` |
   | `rentitem` | `player:give_item(item, 1, { rent_seconds = 3600 })` |

   The names of rAthena's bonus types are listed in [0002a](0002a-bonus-names.md).
10. Bonus names and item events ([0002a](0002a-bonus-names.md), accepted):
    - **Stat abbreviations.** Stats keep the short forms players know, everywhere in the API: `str`, `agi`, `vit`, `int`, `dex`, `luk`; `atk`, `matk`, `def`, `mdef`, `hit`, `flee`, `crit`, `aspd`, `hp`, `sp`, `ap`; and the trait stats `pow`, `sta`, `wis`, `spl`, `con`, `crt`, `patk`, `smatk`, `res`, `mres`, `hplus`, `crate`. So `"luk"`, `player.str`, `player.base_str`, `"atk"` (rAthena's `bBaseAtk`), and `"weapon_atk"` (`bAtk`). Other words stay full, such as `perfect_dodge`.
    - **One name per quantity.**
    - **Damage.** Damage dealt and taken by percent are the bonuses `"damage"` and `"damage_taken"`, with an options table (`kind`, `range`, `attack`, `element`, `skill`, `weapon_type`, and `against` or `from`). Values are summed in rAthena's own groups, so results match rAthena. 33 rAthena types map onto them.
    - **Signs.** A positive value raises the named quantity. Reductions such as `bSubRace` flip their sign.
    - **Chances** are whole numbers per 10,000.
    - **Options table.** Trigger flags go in the last argument. `{ stacking = "highest" }` replaces rAthena's non-stacking types, and any bonus with an amount can use it.
    - **Constants.** `wa.weapon_types`, `wa.monster_groups`, and `items.group`.
    - **The `"damage"` event** replaces `"attack"` and `"hit_taken"`. The damage object has `source`, `target`, `kind` (which replaces `is_weapon` and `is_magic`), `range`, `skill`, `element`, `is_critical`, `amount`, and `wearer_is_source`. `autobonus` and `autobonus2` convert to it.
    - **Timed bonuses.** `timers.with_bonuses` takes the option `{ restart = timer }`. `player:add_timed_bonuses(key, ms, bonuses, options)` replaces `bonus_script`.
11. Bonus names, second round ([0002a](0002a-bonus-names.md), review decisions 11 to 17):
    - **`"skill_used"`** is an item event, fired for every skill the wearer uses, also skills that deal no damage. `autobonus3` converts to it.
    - **One-value bonuses.** `attack_element`, `defense_element`, `set_def`, `set_mdef`, `block_healing`, and `transform_monster` keep one value. Their default, `stacking = "last"`, follows rAthena's order: equipment by slot, then combos, then cards, then timed bonuses. `{ stacking = "priority", priority = n }` lets a mod choose: the highest priority wins, and ties fall back to that order. `"highest"` and `"lowest"` stay for the other bonuses.
    - **Procs.** No name starts with `chance_to_`. The name is the effect (`cause_status`, `cast`, `coma`, `drain_hp`, `break_weapon`, and others). The chance goes in the `chance` option, with `when` (`"attack"`, `"hit_taken"`, or `"skill_used"`), `against`, and `from`.
    - **Percent.** No name ends in `_percent`. Flat and percent share a name, with `{ percent = true }`. A name that rAthena has only as a percent requires it, and a name that rAthena has only as flat refuses it.
    - **Variants.** Per-skill and per-item variants are the options `skill`, `item`, and `item_group` on the shared name, such as `bonuses.add("sp_cost", -10, { percent = true, skill = BASH })`. A separate name stays only where the meaning differs.
    - **Argument order.** What the bonus produces or applies to, then the amount, then other numbers, then the options table. The chance is always an option.
    - **Conversion.** The converter maps every rAthena bonus form to a call in 0002a.

## Version

A mod's manifest says `api: experimental` (strategy section 6.12) until the phase 3 exit check passes with a sample mod. It then becomes `api: 1.0`.

## Next steps

1. Build phase 3 step 2, the mod loader, with this context model. Done.
2. Build the modules in the order the phase 3 steps need them: `wa.context`, `wa.events`, and `wa.timers` with the loader, `wa.bonuses` with item scripts, and `wa.variables` with NPCs.
