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
  events.on("death", function(death) ... end, { victim_kind = "monster" })
  events.on("base_level_up", function(player) ... end)
  events.on("job_level_up", function(player) ... end)
end
```

- `death` gets one death object, with the victim and the killer as handles, whose `kind` field says what each is, like the handles in "Players and other entities". A dead monster is gone, but its handle stays readable while the event's handlers run, and its `is_alive` field is `false`. After the event, reading it raises an error, as for any handle to a removed entity. Options narrow which deaths run the handler (review decision 16).
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
    return drop.monster.id == PUMPKIN_PORING and rate * 2 or rate
  end, { priority = 10 })
end
```

Hooks need the `rules.override` permission. Hook names are checked the same way as event names. Review decision 16 lists the hooks.

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

Monster handles have `id`, `name`, `level`, `hp`, `max_hp`, `map`, `x`, `y`, and `is_alive`. Every entity handle has `kind`: `"player"`, `"monster"` or `"npc"` (review decision 17).

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
player:set_save_point("prontera", 156, 191)   -- savepoint
player:can_carry(RED_POTION, 3)   -- checkweight
player:mark_minimap(1, 150, 180, 0xFF0000)    -- viewpoint
player:open_shop("Tool Dealer#prt1")          -- callshop
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
- `conversation:close()` shows the Close button and waits for it, as rAthena's `close2`. The handler goes on after it, and later dialog calls raise an error. A handler that returns with lines on show leaves the Close button. A handler that returns without showing anything closes the window.
- `conversation:wait(milliseconds)` pauses, as rAthena's `sleep2`.
- `conversation:show_image(name, position)` shows an NPC illustration, as rAthena's `cutin`. The position is `"left"`, `"center"` or `"right"`. `conversation:hide_image()` hides it, and the client hides it when the window closes.
- `conversation:show_progress(seconds)` shows a progress bar over the player and waits until it is full, as rAthena's `progressbar`. A walk cancels it and ends the conversation.
- If the player leaves, the conversation's coroutine is closed. Its pending call raises a Lua error that the runtime does not log as a failure.

The NPC from `context:get_instance()` has `name`, `map`, `x` and `y`, and two methods:

- `npc:show_emotion(emotions.thanks)` shows an emotion over the NPC, as rAthena's `emotion`. `wa.emotions` holds the constants, named from rAthena's `ET_` names.
- `npc:say_aloud(text)` says a line of local chat as the NPC, as rAthena's `npctalk`.

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

Shops are data too:

```yaml
- name: Tool Dealer#prt1
  map: prt_in
  x: 126
  y: 76
  sprite: "53"
  shop:
    - {item: Red_Potion}             # the item's buy price
    - {item: Fly_Wing, price: 60}
```

A click on an NPC with a shop and no click handler opens the shop. A script opens a shop with `player:open_shop("Tool Dealer#prt1")`. Prices follow rAthena's rules: Discount lowers the buy price, Overcharge raises the sell price, and zeny over the maximum is lost.

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
- Procs are named after their effect. The chance (a whole number per 10,000) and the trigger are options: `bonuses.add("cause_status", statuses.stun, { chance = 300, on = "hit_taken" })`. `on` is `"attack"` (the default), `"hit_taken"`, or `"skill_used"`.
- The `when` option is a condition: a function, tracked like a computed bonus. The bonus applies while it returns true: `bonuses.add("unbreakable_weapon", { when = function() return equipped.refine >= 10 end })`. Every bonus takes it.
- `bonuses.add` returns a handle. `handle:remove()` removes that one bonus, for a mod that adds a bonus in one handler and removes it in another. The context still removes it when it ends, and a second `remove()` does nothing.
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
  schedules.weekly("saturday", "20:00", function() ... end)
  schedules.hourly(30, function() ... end)
  schedules.when(function() return is_quiet() end, function() ... end, { lasting_minutes = 60 })
end
```

Timers and schedules belong to the context, so they stop when it ends. World events that run without staff action are data, in `data/events.yml` (review decision 16).

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
| `wa.timers`, `wa.schedules` | Timers and schedules owned by the context | None. World events in `data/events.yml` need `events.schedule` |
| `wa.variables` | Typed saved variables | `storage.mod` |
| `wa.storage` | Mod tables and key-value storage | `storage.mod` |
| `wa.bonuses` | Item bonuses | Item contexts only |
| `wa.items`, `wa.monsters`, `wa.skills`, `wa.companions`, `wa.mounts`, `wa.maps` | Record lookups by name, and map queries | `game.read` |
| `wa.spawning` | Monsters and NPCs owned by the context | `npc.create` |
| `wa.channels` | Chat channels: `channels.create` makes one owned by the context, and `channels.get` finds one (review decision 26) | None for `create` and the channel's `send`, `add` and `remove`; `game.read` for `get` and `get_members` |
| `wa.random` | `random.integer(minimum, maximum)` and `random.chance(numerator, denominator)` from the zone's secret stream | None |
| `wa.integers` | `divide_truncated` and `remainder_truncated` with C rounding, for converted scripts | None |
| `wa.text` | Translated text for the player's language, and `text.item_link(item)` | None |
| `wa.maps`, `wa.world` | `maps.announce(map, text)` and `world.announce(text)` | None for `maps.announce`; `world.announce` for `world.announce` |
| `wa.world_events` | `world_events.start(name)` and `world_events.stop(name)` (review decision 31) | `events.control` |
| `wa.emotions` | Constants of `npc:show_emotion` | None |
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
| `monster "prt_fild08",0,0,"--ja--",1002,10,"NPC::OnKill";` | `local list = spawning.monsters { monster = PORING, amount = 10 }` and `events.on("death", handler, { victims = list })` |
| `rand(45, 65)` | `random.integer(45, 65)` |

### Conversations

| rAthena | wAthena |
|---|---|
| `mes "...";` | `conversation:say("...")` |
| `next;` | `conversation:wait_for_next()` |
| `select("A:B:C")` | `conversation:choose("A", "B", "C")` |
| `menu "A",L_A,"B",L_B;` | `conversation:choose` with an `if`, or a table of functions |
| `input .@n;` | `local amount = conversation:ask_number(0, 100)` |
| `close;` / `end;` | `return`: the Close button stays while lines show |
| `close2;` | `conversation:close()`, and the handler goes on |
| `cutin "kafra_01",2;` / `cutin "",255;` | `conversation:show_image("kafra_01", "right")` / `conversation:hide_image()` |
| `emotion ET_THANKS;` | `npc:show_emotion(emotions.thanks)` |
| `npctalk "Hello";` | `npc:say_aloud("Hello")` |
| `mesitemlink(501)` | `text.item_link(RED_POTION)` |
| `progressbar "ffff00",3;` | `conversation:show_progress(3)` |
| `viewpoint 1,150,180,1,0xFF0000;` | `player:mark_minimap(1, 150, 180, 0xFF0000)` |
| `savepoint "prontera",156,191;` | `player:set_save_point("prontera", 156, 191)` |
| `checkweight(501,3)` | `player:can_carry(RED_POTION, 3)` |
| `getitemname(501)` | `items.name(RED_POTION)` |
| `announce "Hi",bc_all;` / `mapannounce "prontera","Hi",bc_all;` | `world.announce("Hi")` / `maps.announce("prontera", "Hi")` |
| `callshop "Tool Dealer#prt1",1;` | `player:open_shop("Tool Dealer#prt1")` |

### Events

| rAthena | wAthena |
|---|---|
| `OnInit:` | The setup function itself |
| `OnTouch:` | `events.on("touch", handler)` |
| `OnPCLoginEvent:` | `events.on("login", handler)` in a mod script |
| `OnClock0000:` | `schedules.daily("00:00", handler)` |
| `OnTimer5000:` with `initnpctimer` | `timers.after(5000, handler)` |
| `OnMyMobDead:` named in `monster` | `events.on("death", handler, { victims = list })`, owned by the context |
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
7. `monster_killed` (replaced by `death` in review decision 16) passes a monster handle, not a table of what the monster was. The handle reads while the event runs, with `is_alive` false, and raises an error after it, like any handle to a removed entity. Mods use one kind of object for monsters.
8. `level_up` with a kind argument is split into `base_level_up` and `job_level_up`, each `function(player)`. A handler listens to the level it needs, without a check on a string.
9. Item scripts (phase 3 step 3) add these, for the commands rAthena's item scripts use most:

   | rAthena | wAthena |
   |---|---|
   | `getgroupitem(IG_BlueBox)` (item boxes) | `player:give_item_from_group(items.group "BLUEBOX")` |
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
    - **Procs.** No name starts with `chance_to_`. The name is the effect (`cause_status`, `cast`, `coma`, `drain_hp`, `break_weapon`, and others). The chance goes in the `chance` option, with `on` (`"attack"`, `"hit_taken"`, or `"skill_used"`; named `when` in this round and renamed in decision 12), `against`, and `from`.
    - **Percent.** No name ends in `_percent`. Flat and percent share a name, with `{ percent = true }`. A name that rAthena has only as a percent requires it, and a name that rAthena has only as flat refuses it.
    - **Variants.** Per-skill and per-item variants are the options `skill`, `item`, and `item_group` on the shared name, such as `bonuses.add("sp_cost", -10, { percent = true, skill = BASH })`. A separate name stays only where the meaning differs.
    - **Argument order.** What the bonus produces or applies to, then the amount, then other numbers, then the options table. The chance is always an option.
    - **Conversion.** The converter maps every rAthena bonus form to a call in 0002a.

12. Item scripts (phase 3 step 3), review of what the converter needed:
    - **`on` and `when`.** The trigger of a proc is the option `on`: `{ chance = 300, on = "hit_taken" }`. The option `when` is a condition, a function tracked like a computed bonus: it reruns only when a value it read changes, and switches the bonus on or off. Every bonus takes it: flags, granted skills, one-value bonuses, amounts, and highest or lowest stacking. The converter writes `when` for every bonus that an rAthena script adds under a condition.
    - **Bonus handles.** `bonuses.add` returns a handle with `remove()`, which removes that one bonus. The handle belongs to the context, which removes the bonus anyway when it ends. A second `remove()` does nothing. Bonuses added in `timers.with_bonuses` have handles too: `remove()` ends one early, and the timer ends the rest.
    - **Computed levels and chances.** The level of `"skill"` and `"cast"` and the `chance` option can be functions. A granted skill at level 0 grants nothing.
    - **`"unequip"`** is an item event: `function(player)`, before the copy's context ends. Converted `UnEquipScript`s run on it.
    - **`wa.headgear_effects`** holds the constants of `player:show_headgear_effect`, which have IDs of their own.
    - **Readable constant names** come from rAthena's by a fixed rule, with a short list of spelled-out names ([0002a](0002a-bonus-names.md), "Constants used in arguments").
    - **Job tests.** `jobs.is_baby`, `jobs.is_third`, `jobs.is_fourth`, `jobs.base_job` (rAthena's `BaseJob`: a Lord Knight is a Knight) and `jobs.first_job` (`BaseClass`: a Knight is a Swordman), next to `jobs.is_transcendent`.
    - **Equipped copies.** `player:get_equipped(slot)` returns the copy in a slot as a handle, or nil. Equipped handles have `item`, `refine`, `enchant_grade` (0 for none, 1 to 4 for D to A), `wearer`, `weapon_type`, `weapon_level` and `armor_level`.
    - **Trait stats.** Player handles have `base_pow` to `base_crt`, next to `base_str` to `base_luk`.
    - **Item data.** Item records take `script` (the entry script), `bonuses` (static bonuses as data), and `unsupported` (what the item needs that the server does not have; a usable item with it is refused).
    - **Heals** (`player:item_heal`, `player:heal_percent`) need no approved permission.
    - **`player:start_status` and `player:add_timed_bonuses`** raise "not supported yet" until phase 4. Both work since phase 4 step 2 (review decision 19).
    - **Item groups.** `player:give_item_from_group` takes a group from `items.group`, such as `items.group "BLUEBOX"`, the group's name in item_group_db.

13. NPCs (phase 3 step 4). The user approved these, for the rAthena commands that NPC scripts use most:

    | rAthena | wAthena |
    |---|---|
    | `cutin` | `conversation:show_image(name, position)`, with `"left"`, `"center"` or `"right"`, and `conversation:hide_image()` to clear it |
    | `close2` | `conversation:close()`, which does not end the handler |
    | `emotion` | `npc:show_emotion(emotions.thanks)`, with the constants in `wa.emotions` |
    | `npctalk` | `npc:say_aloud(text)` |
    | `mesitemlink` | `text.item_link(item)`, which `say` shows as the item's name |
    | `progressbar` | `conversation:show_progress(seconds)` |
    | `viewpoint` | `player:mark_minimap(id, x, y, color)` |
    | `savepoint` | `player:set_save_point(map, x, y)`. Each character keeps its save point in PostgreSQL. Butterfly Wings and death take the player there, as rAthena's `pc_respawn`. |
    | `checkweight` | `player:can_carry(item, amount)` |
    | `getitemname` | `items.name(item)`: the item's name from rAthena's item database. Translated names come with player languages, from locale files. |
    | `announce`, `mapannounce` | `world.announce(text)`, `maps.announce(map, text)`. waserver also counts down with `world.announce` before it stops. |
    | `callshop` and shop NPCs | A `shop` list of `{item, price}` in `data/npcs.yml`, and `player:open_shop(shop)`. Purchases and sales follow `npc_buylist` and `npc_selllist`, with Discount and Overcharge. |

    `hide_image` is the way to clear an illustration that this decision left open.

14. NPCs, review of what phase 3 step 4 added. The user accepted these as they are:

    - `player:unmark_minimap(id)` removes a minimap mark, as `viewpoint` with action 2.
    - `player.sex` is `"male"` or `"female"`, for rAthena's `Sex`.
    - `variable:add(player, amount)`, and `variable:add(amount)` for server variables, change an integer in one step and return the new value. `x += n` converts to it, so a change made in between is not lost.
    - `variable:set(player, nil)` resets a variable to its default: 0, false, "", or nil for a table. A table is stored when its keys are all strings, or 1 to n.
    - `player:open_shop(name)` takes a record name: the mod's own shop name, or `mod:Name` for another mod's shop.
    - `player:set_save_point` needs the permission `player.warp`, and returns false when the map does not run or the cell is not walkable.
    - `text "key"` reads the mod's `locales/en.yml` only, until players have a language.
    - `data/npcs.yml`: `discount: false` turns Discount off for a shop, a record with a `shop` and no `map` is only opened by scripts, a shop item without a `price` costs its buy price, and `display_name` may be empty.
    - `data/items.yml`: `name`, `buy`, `sell` and `no_sell` set an item's name and prices.

    The user changed these:

    - `player:take_item(item, 0)` raises an error: the amount must be at least 1, so a script's bug shows. rAthena's `getitem` and `delitem` do nothing for an amount of 0 or less. The converter leaves out a call with a fixed amount of 0 or less, and sends a computed amount through `R.give_item` or `R.take_item`, which check it. The coverage reports count both.
    - `world.announce` needs the permission `world.announce`, which the owner approves. `maps.announce` needs none. Both take at most 255 bytes. The built-in mods hold every permission, so converted NPCs keep their announcements. waserver's countdown before it stops is server code, and needs no permission.

15. Mod tests (phase 3 step 5). The user approved `wa.testing` in this shape:

    - Test files `tests/*_test.lua` follow the entry script rule: they return a function that registers cases.
    - `testing.case(name, function(world))`. Each case gets a fresh headless world with the mod and its dependencies loaded, a fixed random seed, and no database.
    - `world:new_player { map, x, y, job, base_level, job_level, zeny, items = { {item, amount} } }` returns a normal player handle.
    - `world:talk(player, npc_name, answers)` clicks the NPC and answers each step in order: a number picks a menu option, a string fills text input, and an integer fills number input when asked. Next is pressed without an answer. It returns everything the NPC said as text. It fails the test when the conversation asks for more answers than given.
    - `world:use_item(player, item)`, `world:advance(milliseconds)` on the zone clock, where timers fire, `world:spawn_monster { monster, map, x, y }`, and `world:kill(player, monster)`.
    - `testing.expect(value)` with `.to_be(x)`, `.to_equal(table)` (deep), `.to_contain(text_or_value)`, `.to_be_true()`, `.to_be_false()`, `.to_be_nil()`, and `.to_fail_with(text)` on a function that should raise an error containing the text.
    - `wactl mod test` runs every test file of a mod, prints each failure with its file and line, and exits non-zero on any failure. `wa.testing` exists only under `wactl mod test`, never in a running server.

    Step 5 built it with these details, which are provisional until reviewed:

    - A test world runs one map: the map of its first `new_player` or `spawn_monster`, or `prontera` when `advance` comes first. `map` may be left out after that. Naming another map fails, and warps to other maps do not happen, as in a server where that map does not run.
    - The zone clock starts at 0 and moves only with `world:advance`, with waits and progress bars in `world:talk`, and with the item use interval in `world:use_item`.
    - `world:talk` also fails when answers are left over after the conversation ends, and when an answer does not fit its step. The NPC is named by its full name, its name in the mod, or the name players see when one NPC of the map has it. The text is the dialog lines, one per line.
    - `world:use_item` fails when the player has no such item, or the zone refuses the use, with the reason, such as "not supported".
    - `world:new_player` makes a character named `Tester 1`, `Tester 2` and so on, with full HP and SP, at a walkable cell. Without `--assets`, every cell is walkable.
    - `world:spawn_monster` takes a monster ID from `monsters.id`. The monster does not come back after it dies. `world:kill` gives the player the monster's exp and drops, as a last hit would.
    - `.to_contain` on a table compares elements deeply.
    - Test code runs as the mod: its calls need the mod's permissions, and the variables it declares are the mod's. Calls that need a context, such as `events.on`, fail in a case.
    - A case also fails when the server logs a warning that names the mod, such as a handler that failed.
    - The API reference lists `wa.ItemUse`, the instance of a use context: `{ item }`, which the runtime already returned.

16. World events, schedules, spawning, death, and hooks (phase 3 step 6). The user approved these:

    - **World events as data.** A mod's `data/events.yml` lists events. Each has a `trigger`, `duration_minutes`, a `script` (an entry script), and an optional `map`. It needs the permission `events.schedule`.

      ```yaml
      - name: example.bosses:Baphomet_Night
        trigger: { weekly: { day: saturday, time: "20:00" } }
        duration_minutes: 60
        script: scripts/events/baphomet_night.lua
      ```

      - Triggers: `daily: "HH:MM"`, `weekly: { day, time }`, `monthly: { day, time }`, `hourly: { minute }`, `random: { chance_per_hour }` (out of 10,000, drawn from a secret stream), and `condition`.
      - Each run of an event is one context of kind `"event"`. What it creates, such as monsters, timers, handlers, schedules and hooks, goes when the run ends.
      - Times are in the server's time zone, an IANA name in the config. A time that does not exist runs at the next valid minute. A time that happens twice runs once.
    - **`wa.schedules`**, owned by the context: `schedules.daily("00:00", handler)`, `schedules.weekly("saturday", "20:00", handler)`, `schedules.hourly(30, handler)`, and `schedules.when(condition, handler, { lasting_minutes = 60 })`, whose handler runs once the condition has held that long.
    - **`wa.spawning.monsters { map, monster, amount, x, y, area, respawn }`** returns monster handles. Without `x` and `y`, each monster lands on a random walkable cell. `area` spreads them within that many cells. `respawn = false` is the default: a dead monster stays dead. `respawn = true` is a normal spawn while the context lives. The context owns the monsters. It needs `npc.create`.
    - **`events.on("death", handler, options)`** replaces `monster_killed`. The handler gets a death object: `victim` (a handle, readable during the event, with `is_alive` false for a monster), `victim_kind` (`"player"` or `"monster"`), `killer` (a handle or nil), `map`, `x`, `y`, and `skill` (or nil). The options `victim`, `victims`, `victim_kind`, `monster` and `map` narrow it. It fires for deaths on the context's map, players included.
    - **`wa.hooks`**: `hooks.on("drop_rate", function(drop, next) ... end, { priority = n })`, with `drop` = { `player`, `monster`, `item`, `map`, `rate` per 10,000 }, and `hooks.on("experience", function(exp, next) ... end, { priority = n })`, with `exp` = { `player`, `monster`, `base`, `job` }. Higher priority runs first. `next(object)` returns the rest of the chain's result. The innermost result is rAthena's value. They need `rules.override`.

    Step 6 decided these details:

    - **Which zone runs an event.** The zone of the event's `map`. Without `map`, the zone of the world's start map. A context spawns monsters on its own map only, so an event that spawns sets `map`.
    - **Restarts.** Each run is recorded in PostgreSQL (`world_event_runs`) before it starts, so a restart never runs the same time twice. Runs whose whole window passed while the server was stopped are skipped. A run whose window is still open at the start, and that has not run, starts at once and ends at its usual time: a server that starts at 22:00 has the night that began at 18:00. A run cut short by a stop does not start again.
    - **One run at a time.** A time that comes while the event's last run still lasts is skipped.
    - **Random triggers** draw once at the start of each hour, from the world's secret stream `events`.
    - **Condition triggers** name a function of a helper module, which `require` finds: `condition: { module: example.bosses.conditions, function: town_is_quiet, lasting_minutes: 60 }`. The module returns a table that holds the function. It runs once a minute in the zone of the event's map. It only reads the game: a call that changes it raises an error. The event starts once the function has returned true for `lasting_minutes`, and after the run ends it must hold that long again.
    - **`schedules.when` polls.** Its condition runs once a minute, at the start of each minute of the server clock, not when a value it reads changes. Tracking reads is not feasible: player counts, server variables and monster counts have no change notifications. The handler runs once for each time the condition holds that long.
    - **Missed schedules.** A schedule whose time passed while its zone slept or the server was stopped runs once, or not at all after a stop. A sleeping zone wakes for its schedules.
    - **Hooks only compute.** A hook, like a computed bonus, cannot change the game: such calls raise an error. A hook that fails is logged and left out of the chain. To pass a changed object, a hook passes a table with the same fields to `next`. `drop_rate` results are kept to 0 to 10,000, and `experience` results below 0 count as 0. Hooks run where the kill happens: a hook of an event context changes kills on its map.
    - **Spawns from mods** come from scripts: a mod script calls `spawning.monsters { respawn = true }`. There is no `data/spawns.yml` yet. Core's spawns are still rAthena's spawn lines.
    - **Owners can move events.** The config's `mods.events` changes an event's trigger or duration, or turns it off, by its full name.
    - **Events in contexts.** Event contexts can listen to the events of mod contexts: `login`, `logout`, `map_enter`, `death`, `base_level_up` and `job_level_up`. NPC contexts can listen to `death`.
    - **Monster records.** `data/monsters.yml` adds and changes monsters. A new monster, named with the mod's ID, starts from the stats of the monster its `sprite` names and shows that sprite. IDs come from the mod range, from 100,000. The fields set what combat reads directly (`hp`, `attack`, `defense`, `hit`, `flee`), since the server has no monster status calculation yet. `class: boss` adds rAthena's boss modes.
    - **Handles compare.** Two handles to one player or monster are equal with `==`, so `death.victim == boss` works.

    The details that step 6 left provisional were reviewed in decision 17.

17. Review of step 6 (review decision 16). The user accepted these as they are:

    - `schedules.daily`, `weekly`, `hourly` and `when` return a schedule handle with `cancel()`. `hooks.on` returns a hook handle with `remove()`. A second call does nothing.
    - `context:get_instance()` of an event context is `{ name, map }`: the event's full name and the map of its zone.
    - Event contexts listen to the events of mod contexts: `login`, `logout`, `map_enter`, `death`, `base_level_up` and `job_level_up`. NPC contexts listen to `death`.
    - `display_name` sets the name players see of an item or a monster, as for NPCs. `name` is the record's key. It replaces the item `name` field of decision 14, which could not be written.
    - The fields of `data/monsters.yml` set what combat reads directly: `display_name`, `sprite`, `level`, `hp`, `sp`, `attack`, `magic_attack`, `defense`, `magic_defense`, `hit`, `flee`, `speed`, `attack_range`, `attack_delay`, `attack_motion`, `damage_motion`, `element`, `element_level`, `race`, `size`, `class`, `modes`, `base_exp`, `job_exp` and `drops`. This holds until phase 4 has monster stats: then the server computes the combat values from the base stats, as rAthena's `status_calc_mob`, and these fields change.
    - The config's `time_zone` (UTC by default) and `mods.events`.
    - The `wa.testing` calls `world:set_time(time, time_zone)`, `world:get_time()`, `world:get_monsters(monster)`, `world:get_announcements()` and `world:count_floor_items(item)`, and world events in test worlds.
    - `world:kill` takes any living monster handle. After a kill, the monster's handle raises an error, as after any death event.

    The user changed these:

    - **Respawn time.** `respawn = true` makes a monster come back as a spawn line's monster does in rAthena. rAthena keeps the delay on the spawn line, not in the monster data (`npc_parse_mob`, `mob_setdelayspawn`):
      - The line's first delay is the base time. Without one it is 5,000 ms, and a delay below `mob_respawn_time` (1,000 ms) becomes 5,000 ms.
      - The second delay adds a random variance, which rAthena keeps only for `boss_monster` lines by default (`mob_spawn_variance: 1`).
      - `boss_spawn_delay`, `plant_spawn_delay` and `mob_spawn_delay` scale the time for status immune monsters, plant monsters and the rest. They are 100% by default.
      - A monster never comes back faster than 1 second.

      Since monster data has no respawn time, `spawning.monsters` takes `respawn_milliseconds`, from 1,000, and uses 5,000 ms without it, as a spawn line without a delay. `respawn_milliseconds` is provisional. There is no variance option, as for a normal spawn line.
    - **Kind.** Every entity handle has a read-only `kind` field: `"player"`, `"monster"` or `"npc"`, and later kinds such as pets. The death object has no `victim_kind` field any more: scripts read `death.victim.kind` and `death.killer.kind` (the killer may be nil). The option `victim_kind` of `events.on("death", ...)` stays. The damage object's `source` and `target` have `kind` too.

18. Entity queries and the skill event (planned for phase 4 step 3). The user approved these on 2026-10-02:
    - **`maps.find_entities(query)`** returns a list of entity handles. It needs `game.read`. The name matches `maps.find_player`. The query has one area, filters, and an order:
      - The area is a radius around an entity (`around`) or a cell (`map`, `x`, `y`), a `rectangle` of two corners, a `line` from one cell to another with a `width`, or a list of `cells`.
      - `shape` is `"square"` by default, as rAthena's range, or `"circle"`.
      - The filters are `kinds`, `alive` (true by default), `enemies_of` or `allies_of` an entity with rAthena's attack rules, `line_of_sight` from the center of the area, and `exclude`, a list of handles.
      - `sort` is `"nearest"`, `"farthest"`, `"lowest_hp"` or `"random"`, and `limit` caps the count. Ties sort by entity ID. `"random"` uses the zone's secret stream.

      ```lua
      local next_target = maps.find_entities {
        around = current, radius = 5, enemies_of = caster,
        exclude = hit, sort = "nearest", limit = 1,
      }[1]
      ```
    - **`entity:find_nearby(query)`** is the same query with `around` set to the entity. Every entity handle has it. The query table stays open to new areas, filters and orders, which later versions add without changing old calls.
    - **The `skill_used` event** opens to mod and event contexts, which the user asked for as a cast event. It keeps the item event's name, so one name means one thing everywhere. Its object has `caster`, `skill`, `level`, `target`, `x` and `y`. Options narrow it: `skill`, `caster_kind` and `map`. It fires when the skill takes effect, after the cast time. A cast that is interrupted does not fire it.
    - A sample mod adds a chain lightning skill with these queries, once the skill definition API exists. Its shape gets its own review in step 3.

19. Status effects (phase 4 step 2). The user approved these on 2026-10-02:
    - **Every entity handle** has the status methods: players, monsters, NPCs, and later kinds.
    - **`entity:start_status(status, options)`** replaces `sc_start`, `sc_start2` and `sc_start4`. It returns true when the status starts. Options:
      - `milliseconds`, or `"forever"` for rAthena's `INFINITE_TICK`;
      - `level` (val1), and `values`, a list of val2 to val4 for the statuses that read them;
      - `chance`, per 10,000, always by default;
      - `source`, the entity that caused it, for resistance and kill credit;
      - `cannot_be_avoided`, `keep_duration`, `keep_chance` and `hide_icon`, for rAthena's `SCSTART_NOAVOID`, `SCSTART_NOTICKDEF`, `SCSTART_NORATEDEF` and `SCSTART_NOICON`.
    - **Reading and ending.** `entity:end_status(status)` replaces `sc_end` and returns true if the status was active. `entity:has_status(status)` replaces `getstatus`. `entity:get_status(status)` returns an object with `status`, `level`, `values`, `remaining_milliseconds` and `source`, or nil. `entity:get_statuses()` returns a list of them.
    - **Events** for mod and event contexts, with the options `status`, `target_kind` and `map`. The object has `target`, `status`, `level`, `source`, `milliseconds` and `reason`:
      - `status_start`. `reason` is `"skill"`, `"item"`, `"bonus"` (a proc such as `cause_status`), `"script"`, or `"login"` for a status restored from a save.
      - `status_end`. `reason` is `"expired"`, `"removed"` (by `end_status` or a skill), `"replaced"`, `"death"`, `"dispell"`, `"clearance"`, `"logout"` or `"damage"`.
      - `status_tick`, for each tick of a status that ticks, such as poison. Ticks of mod statuses run here, not in a function of the definition.
    - **Hooks** `status_chance` (per 10,000) and `status_duration` (milliseconds). The object has `target`, `status`, `level` and `source`. Without hooks the results are rAthena's.
    - **`statuses.define`** adds a status in a mod script: `name`, `icon`, `kind` (`"buff"` or `"debuff"`), `ends_on`, `bonuses` (a function of the level that returns bonus entries, as `add_timed_bonuses`), and `tick_milliseconds`. The status gets a constant in `statuses` named after it.

    The user changed these after the build, on 2026-10-02 ([decision 0010](../decisions/0010-status-effects.md), "Changes after the build"):

    - **Stats change inside the call.** `start_status`, `end_status`, `add_timed_bonuses` and the other calls that change a player's stats compute them before they return, as rAthena's `sc_start` and `sc_end` do. A host call may run Lua again only for this. The events still run after the script's call, so a `status_start` handler that starts a status cannot loop inside one call. rAthena has no events, so this changes no rAthena behavior.
    - **Repeated timed bonuses follow rAthena.** The option `on_repeat` replaces `stack`. It is named `on_repeat` because `repeat` is a Lua keyword. Copies match by key:

      ```lua
      player:add_timed_bonuses(key, ms, bonuses, {
        on_repeat = "keep",  -- the default, rAthena's: the first copy stays, the new one is ignored
        -- "extend": BSF_FORCE_REPLACE, the old values with the longer duration
        -- "add": BSF_FORCE_DUPLICATE, both copies apply
        -- "replace": for mods, the new values and the new duration
      })
      ```

      The method returns true when the bonuses were added or extended, and false when they were ignored. The converter maps `bonus_script`'s flags 0x400 and 0x800 to `"extend"` and `"add"`.

20. Skills (phase 4 step 3). The user approved these on 2026-10-02:
    - **Skill data** is a mod's `data/skills.yml`, with the fields of rAthena's `skill_db.yml` under readable names: `name`, `display_name`, `icon`, `max_level`, `type` (`weapon`, `magic` or `misc`), `target` (`passive`, `enemy`, `ground`, `self` or `support`), `element`, `range`, `cast_time`, `fixed_cast_time`, `after_cast_delay`, `cooldown`, `requirements` (such as `sp`, `weapons` and `items`), and `script`. A number is one value, or a list with one value per level. Core's skills use the same records, so overlays change them. Skill IDs come from the mod range.
    - **`icon`** names an existing skill whose icon and animation the new skill shows, until the client data tables of phase 6.
    - **Skill trees** are `data/skill_trees.yml`: `job`, `skill`, `max_level` and `requires`, a list of skills and levels. New jobs wait for step 7.
    - **The `cast` event** runs in the skill's context when the skill takes effect. The cast object has `caster`, `skill`, `level`, `target` (nil for ground skills), `map`, `x` and `y`, and these methods:
      - `cast:deal_damage(target, options)` runs rAthena's damage calculation with `percent` as the skill's damage percentage. The options `hits`, `type` and `element` change the data's values. It applies cards and defense, fires the `damage` event, and returns an object with `amount`, `is_critical` and `missed`.
      - `cast:heal(target, amount)` heals with rAthena's heal bonuses.
      - `cast:run_default(options)` runs core's own behavior of the skill, with `target` to change the target. It works only for skills that have one.
      - The cast object stays usable after the event, so timers can use it. When the caster is gone or cannot act, `deal_damage` does nothing and returns nil, as rAthena's `skill_addtimerskill`.
    - **Passive skills** run their script while the skill is learned, and again when its level changes. `context:get_instance()` gives `level` and `owner`.
    - **Replacing a core skill's behavior.** A mod can set `script` on a core skill. It needs `rules.override`.
    - **Hooks** `skill_damage` (the damage percentage), `cast_time`, `skill_cost` and `cast_allowed`. Without hooks the results are rAthena's.
    - **`entity:show_effect(effect)`** on every entity handle, not only players.
    - **`entity:exists()`** returns false once the entity is removed, has logged out, or has left the zone. It never raises an error.
    - A sample mod adds chain lightning with these calls and `maps.find_entities` (review decision 18), with a delay between bounces.

21. Job changes and resets (phase 4 step 4). The user approved these on 2026-10-03:
    - `player:change_job(job, { force })` replaces `jobchange`. It returns false when the job is not a valid next job for the player. `force = true` skips that check and needs the `player.change_job` permission. Job level goes back to 1 and learned skills stay, as rAthena does.
    - `player:rebirth()` makes the player a High Novice at base and job level 1, with rAthena's transcendent rules.
    - `jobs.next_jobs(job)` returns the jobs a job can change into.
    - `player:reset_skills()` and `player:reset_stats()` replace `resetskill` and `resetstatus` and refund the points.
    - `player:reset_skill(skill, { to_level, with_dependents })` lowers one skill, to 0 by default, and refunds its points. It returns false and changes nothing when another learned skill needs the lowered level, unless `with_dependents = true`, which lowers those skills too and refunds them.
    - `player:reset_stat(stat, { to })` lowers one stat, to 1 by default, and refunds what raising it cost by rAthena's cost table.
    - Quest skills, which cost no skill points, are not refunded or lowered by these calls, as rAthena's `resetskill`.

22. Monster stats, monster skills and monster AI (phase 4 step 5). The user approved these on 2026-10-03:
    - **Monster stats.** `data/monsters.yml` adds `str`, `agi`, `vit`, `int`, `dex`, `luk`, `magic_attack`, `resistance`, `magic_resistance`, `skill_range`, `chase_range`, `mvp_exp`, `mvp_drops`, `damage_taken`, and `ai`, rAthena's AI type number from 01 to 27, read as a list of modes. `hit` and `flee` are removed. The server computes them from the stats, as rAthena's `status_calc_mob`. There are no override fields. A mod raises them with stats or bonuses.
    - **Monster skills** are a `skills` list on a monster record, converted from rAthena's `mob_skill_db`. Each entry has `skill`, `level`, `state`, `rate` (per 10,000), `cast_time`, `delay`, `cancelable`, `target`, `condition`, `condition_value`, `summons`, `emotion` and `chat`.
      - `state` keeps rAthena's words: `any`, `idle`, `walk`, `loot`, `dead`, `attack`, `angry`, `chase`, `follow` and `anytarget`.
      - `target` is `target`, `random_target`, `self`, `friend`, `master`, or `around1` to `around8`.
      - `condition` uses readable names: `always`, `hp_below`, `hp_between`, `friend_hp_below`, `friend_hp_between`, `has_status`, `lacks_status`, `friend_has_status`, `friend_lacks_status`, `attackers_above`, `attackers_at_least`, `slaves_below`, `slaves_at_most`, `hit_melee`, `hit_ranged`, `hit_by_skill`, `after_skill`, `targeted_by_cast`, `target_casting`, `hit_while_stuck`, `master_hp_below`, `master_attacked`, `spawned`, `summoned_by_alchemist`, `monsters_nearby_above`, `hit_by_ground_skill` and `damage_above`. They map one to one to rAthena's conditions, from `always` to `trickcasting`.
    - **Monster handles** get the read-only fields `target` (a handle or nil), `state`, and `master` (a handle or nil, for summoned monsters). They get the methods `set_target(entity)`, `clear_target()`, `walk_to(x, y)`, `use_skill(skill, level, target)`, `talk(text)` and `stop()`, which replace `unitattack`, `unitwalk`, `unitskilluseid`, `unittalk` and `unitstop` for monsters. NPCs do not get them yet.
    - **Hook `monster_target`**: `hooks.on("monster_target", function(choice, next) ... end)`, with `choice` = { `monster`, `candidates`, `current`, `reason` }, where `reason` is `"aggressive"`, `"assist"`, `"hit"` or `"change"`. It returns a handle or nil, and needs `rules.override`. Without hooks the choice is rAthena's. Other AI hooks wait for strategy section 6.8.
    - **Monster AI scripts.** A monster record's `script` field names an entry script. Its setup function runs once for each spawned monster. `context:get_instance()` returns the monster. The context ends when the monster dies or is removed.
    - **Tests.** `world:advance` runs monster AI. Tests read `monster.target` and `monster.state`. There are no new `wa.testing` calls.
    - Later: an option to make a spawned monster follow a master, AI for summoned monsters beyond rAthena's defaults, and NPC AI hooks.

23. Rolled item values and random options (phase 4 step 6). The user approved these on 2026-10-03:
    - **Roll data.** An item in `data/items.yml` has `rolls`, a map from a roll name to `min`, `max`, `default`, `distribution` and `hidden`. `distribution` is `uniform`, `triangular` (the default) or `weighted`. A `weighted` roll has `weights`, a list of `{ value, weight }`, where `value` is one number or a `[min, max]` range. `hidden: true` shows the value only after identification. An item has at most 8 rolls, and values are integers.
    - **Static bonuses** take a roll in place of a number: `bonuses: [[luk, { roll: luck }]]`.
    - **Reading.** Equipped handles have a read-only `rolls` table, such as `equipped.rolls.boss_damage`, and `cards`, a list of `{ item, rolls }` for the cards in the slots.
    - **Giving.** `player:give_item(item, amount, { roll = ... })`. `"random"` rolls each value. `"default"` uses the defaults and is the default for scripts and shops. A table such as `{ boss_damage = 31 }` sets the values, needs `items.modify_rolls`, and clamps each value to its range. Monster drops roll. With the `item_rolls` flag off, every roll uses its default, as rAthena.
    - **Hook `item_roll`**: `hooks.on("item_roll", function(roll, next) ... end)`, with `roll` = { `item`, `name`, `min`, `max`, `default`, `source` }, where `source` is `{ kind = "drop", monster }`, `{ kind = "script" }` or `{ kind = "shop" }`. It returns the value, which the server clamps to the range. It needs `rules.override`. It replaces the Lua function form of strategy section 5.4.
    - **Rerolling.** `equipped:set_roll(name, value)` and `equipped:reroll(name)`, where `reroll()` without a name rerolls every value. Both need `items.modify_rolls`. Only equipped copies have them until inventory copies have handles.
    - **Random options.** rAthena rolls them too, so they roll whatever the `item_rolls` flag says. `data/random_options.yml` has `name`, `display_name` and `bonus`. `data/random_option_groups.yml` has the slots, chances and value ranges of `item_randomopt_group`. rAthena's names become readable names, such as `max_hp` for `RDMOPT_VAR_MAXHPAMOUNT`. They are stored with the rolls. `equipped.random_options` is a list of `{ option, value, param }`. Monster drops with a `random_option_group` roll them, as rAthena.
    - **Tests.** In `wa.testing`, `roll = { ... }` needs no permission, and drops from `world:kill` roll from a seeded random stream.
    - Later: rolls on crafting and quest rewards, roll searches in vending, and handles for inventory copies.

24. Job definitions and the details before 1.0 (phase 4 step 7). The user approved these on 2026-10-03:
    - **Job records** are `data/jobs.yml`, which core's jobs use too. Fields: `name`, `display_name`, `look` (an existing job whose sprites the job shows, until the client data tables of phase 6), `tier` (`novice`, `first`, `second`, `third`, `fourth` or `expanded`), `transcendent`, `baby`, `base_job`, `changes_from` (a list of `{ job, base_level, job_level }`), `inherits` (skill tree inheritance, rAthena's `Inherit`), `equips_as`, `max_base_level`, `max_job_level`, `max_stats`, `max_weight`, `hp` and `sp` (`{ factor, increase }`, or `hp_table` and `sp_table` with one value per level), `aspd` (base ASPD by weapon type), `bonus_stats` (stats by job level), and `exp_table` (an existing job's table, or `base_exp` and `job_exp` lists). A field that is left out comes from the `look` job. Job IDs come from the mod range. Skill trees stay in `data/skill_trees.yml`.
    - An item's `jobs` field also takes a list of job names, such as `[knight, example.jobs:Desperado]`. rAthena's bit masks still work.
    - `jobs.id "example.jobs:Desperado"` finds a job, as `skills.id`. The job calls and tests work with mod jobs.
    - **Event `job_change`**: `events.on("job_change", function(change) ... end)`, with `change` = { `player`, `from`, `to` }.
    - The client shows the `look` job's sprites and the `display_name`.
    - After the phase 4 exit check, manifests move from `api: experimental` to `api: 1.0`.
    - **Monster skill entries** get `modes`, applied through SC_MODECHANGE as rAthena's `NPC_EMOTION`, and `emotion` takes a list of two. `chat_color` is a hex color, sent with the chat line.
    - **`wa.testing`** gets `world:equip(player, item, { roll })`, which returns the equipped handle. The `new_player` fields `sex`, `skills`, `stats`, `skill_points` and `status_points` are final, and so is `respawn_milliseconds`.
    - **Rolls** take an optional `display_name`.
    - The details listed under next steps 6 to 9, and the step 6 details (`multiply` and `divide` in `{ roll }`, lists in a random option's `bonus`, `random_option_group` on drops, card handles with their own rolls, the `item_roll` hook for default values, core option IDs, and hidden rolls that start unidentified), are accepted as built.
    - Mounts, falcons and carts wait for a general design, since mods may add their own companions such as familiars.

25. Companions, mounts and carts (after phase 4 step 7). The user approved these on 2026-10-03:
    - **Companions** are `data/companions.yml`: `name`, `display_name`, `look` (an existing companion sprite, such as `falcon`), `acts_as` (a core companion whose skills work with it, such as `falcon`), `requires` (a list of `{ skill, level }`) and `bonuses`. A player has at most one companion. A skill that needs a falcon accepts any companion that acts as one. A skill's `requirements` can name a companion: `{ companion = <name> }`. Core's companion is `falcon`, and later `warg`.
    - **Mounts** are `data/mounts.yml`: `name`, `display_name`, `look` (such as `peco_peco` or `dragon`), `acts_as` (a core mount whose rules apply: speed, the mounted ASPD rule, and spear damage), `requires`, `speed` and `bonuses`.
    - **Carts** have a look number and the player's cart storage.
    - `player:set_companion(companion)`, `player:set_mount(mount)` and `player:set_cart(look)`, each with `nil` to remove. They return false when the player does not meet `requires`. A cart needs Pushcart, as rAthena. The fields `player.companion`, `player.mount` and `player.cart` are read-only. `companions.id` and `mounts.id` find records.
    - A job change removes a companion or mount the new job does not meet `requires` for, as `pc_jobchange`.
    - The converter maps `setfalcon`, `setriding`, `setcart` and `checkfalcon` to these calls.
    - Later: companions that fight on their own, companion events, and more than one companion at a time.

26. Chat and moderation (phase 5 step 2). The user approved these on 2026-10-03:
    - **Event `chat`**: `events.on("chat", function(message) ... end, options)`, with `message` = { `player`, `channel`, `text`, `to`, `map` }. `channel` is `"public"`, `"whisper"`, `"party"`, `"guild"` or a channel name. The options `channel`, `map` and `player` narrow it. It is a world event (strategy section 6.8).
    - **Hook `chat`**: `hooks.on("chat", function(message, next) ... end)` returns the text, or nil to block the message. It runs after the server's chat filter and needs `rules.override`.
    - `player:whisper(text, { from })` sends a whisper. `from` is the name the player sees, by default the NPC's or the mod's name. `player:send_message` stays for system messages.
    - `player:is_blocking(other)`.
    - `player:mute(minutes, reason)`, `player:jail(minutes, reason)` and `player:unmute()` need the new permission `player.sanction`, which is flagged for review. Bans are a staff tool, not Lua API.
    - **`wa.channels`**: `channels.create { name, display_name, color, join }`, with `join` = `"open"` or `"invite"`, makes a channel owned by the context. `channels.get(name)` finds one. A channel has `send(text)`, `add(player)`, `remove(player)` and `get_members()`. Core's channels, such as `#main` and `#trade`, are records in `data/channels.yml`.
    - The word lists of the chat filter and the reserved names are `data/chat_filter.yml`, with replace or reject per word. Chat rate limits, and lower limits in a new character's first hour, are server settings.
    - Reports and their review come with the staff tools, not the Lua API.

27. Friends, parties and limits (phase 5 step 4). The user approved these on 2026-10-03:
    - **Party handles.** `player.party` is a party or nil. Read-only fields: `id`, `name`, `leader` (a handle, or nil when the leader is offline), `leader_name`, `exp_share` (`"each"` or `"even"`), `item_share` (`"each"` or `"even"`) and `item_pickup` (`"each"` or `"shared"`). `party:get_members()` returns handles of online members. `party:get_member_names()` returns every member's name.
    - **`wa.parties`**: `parties.create(leader, name)` returns the party, or nil when the name is taken or the leader has a party. `party:add(player)`, `party:remove(player)`, `party:set_leader(player)`, `party:set_options { exp_share, item_share, item_pickup }` and `party:disband()` replace `party_create`, `party_addmember`, `party_delmember`, `party_changeleader`, `party_changeoption` and `party_destroy`. They need the new permission `party.manage`. `parties.get(id)` and `parties.find(name)` find a party. Reading needs `game.read`.
    - **Events** `party_join` and `party_leave`, with { `party`, `player`, `reason` }, where `reason` is `"invited"`, `"left"`, `"expelled"`, `"disbanded"` or `"script"`. `party_change`, with { `party`, `leader`, `options` }.
    - The `experience` hook's object gains `party` (or nil) and `share`, the player's share before rAthena's split.
    - **Friends**: `player:is_friend(other)` and `player:get_friend_names()`. Only players add and remove friends.
    - **Limits.** A ruleset mod's `limits` block sets limits that rAthena compiles in, with rAthena's values as defaults, such as `party_members` (12), `friends` (40) and `block_list` (20). rAthena's `battle_athena.conf` values, such as `party_share_level` (15), are ruleset rules. Each limit has a hard maximum, such as 64 party members, 200 friends, 100 blocked names, 8 card slots and 2,000 storage slots. A ruleset outside the bounds fails to load. Lowering a limit deletes nothing: an over-full list keeps its entries and cannot grow, and cards over the slot limit stay but give no bonus. The client gets the limits at login. Lua reads them as `rules.limits.<name>`, read-only. Step 4 adds the party, friend and block list limits, and later steps add the guild, storage, cart, inventory and card slot limits.
    - Test worlds call `parties.create` without the permission.

28. Guilds (phase 5 step 5). The user approved these on 2026-10-04:
    - **Guild handles.** `player.guild` is a guild or nil. Read-only fields: `id`, `name`, `master` (a handle, or nil when the master is offline or on another map), `master_name`, `level`, `exp`, `notice` (`{ title, text }`) and `member_count`. `guild:get_members()` returns handles of online members. `guild:get_member_names()` returns every member's name. `guild:get_position(player)` returns `{ name, can_invite, can_expel, can_use_storage, exp_tax }`. `guild:get_skill_level(skill)`, `guild:is_allied(other)` and `guild:is_enemy(other)`.
    - **`wa.guilds`**: `guilds.create(master, name)` needs no Emperium, so the script decides the cost. It returns the guild, or nil when the name is taken or the master has a guild. `guild:add(player)`, `guild:remove(player)`, `guild:set_master(player)`, `guild:set_notice(title, text)`, `guild:give_exp(amount)` and `guild:disband()` need the new permission `guild.manage`. `guilds.get(id)` and `guilds.find(name)` find a guild. Reading needs `game.read`.
    - **Events** `guild_join` and `guild_leave`, with { `guild`, `player`, `reason` }, and `guild_change` for a new master, a level up, a notice or an emblem.
    - The `experience` hook's object gains `guild` and `guild_tax`, the share that goes to the guild.
    - **Limits**, with rAthena's defaults and hard maximums: `guild_members` before Guild Extension (16, at most 200), `guild_positions` (20, at most 40), `guild_allies` (3, at most 10), `guild_enemies` (3, at most 10) and `guild_storage` (500, at most 2,000).
    - Castles, sieges and guardians are tier 2 and come later.

29. Storage, cart, trade and limit ranges (phase 5 step 6). The user approved these on 2026-10-04:
    - **Storage.** `player:open_storage(name)` opens the account's main storage without a name, and replaces `openstorage` and `openstorage2`. `player:open_guild_storage()` replaces the core-only `wa.internal` call of step 5. Opening needs no permission; scripts charge any fee. Extra storages are records in `data/storages.yml`: `name`, `display_name`, `slots`, and whether the account or the character owns it.
    - **Storage and cart items from scripts.** `player:count_item(item, { from })`, `player:give_item(item, amount, { to })` and `player:take_item(item, amount, { from })` take `"storage"`, `"cart"` or a storage name, with the permissions the calls already need. Without the option they act on the inventory, as before. `guild:count_item(item)` reads guild storage.
    - **Item restrictions** in `data/items.yml`: `restrictions`, a list of `no_drop`, `no_trade`, `no_sell`, `no_cart`, `no_storage`, `no_guild_storage` and `no_mail`, from rAthena's `item_trade` table.
    - **Bound items.** `give_item(..., { bound = "account" | "character" | "party" | "guild" })`. A bound item cannot be traded or dropped, and goes only into storage its owner shares.
    - **Trade.** Event `trade`, with { `players`, `items`, `zeny` }, after a trade completes, filtered by `player`. Hook `trade` returns true, or false and a message, to refuse a trade before it completes, and needs `rules.override`.
    - **Limit ranges.** Each limit in a ruleset has `base`, what a player or guild starts with, and `max`, where expansions such as items, Guild Extension or scripts stop: `inventory_slots: { base: 100, max: 200 }`. The engine has a fixed ceiling per limit that no ruleset may exceed; a `max` above it fails the load. This applies to the limits of steps 4 and 5 too. A call that expands a limit is new API for a later review.
    - **New limits** with rAthena's values: `inventory_slots` (base 100, max 200), `storage_slots` (600), `cart_slots` (100), and `guild_storage` changes to rAthena's 600. The cart weight limit, 8,000, is a ruleset rule.
    - **Engine ceilings** come from a benchmark at the ceiling (message size, save time and tick cost). Storage and cart lists are sent in pages, so the 64 KiB message limit does not cap them. Starting values to check: inventory 1,000, storage 10,000, cart 1,000, party 256, guild 512, friends 1,000, block list 1,000 and card slots 16.

30. Mail, vending, buying stores, shops and currencies (phase 5 step 7). The user approved these on 2026-10-04:
    - **Mail (RODEX).** `player:send_mail({ to, sender, title, body, zeny, items })` sends mail from a script, to a character name or ID, online or not, as rAthena's `mail` command. `items` is a list of { `item`, `amount`, `roll`, `bound` }. It needs a new permission, `mail.send`. `player:open_mail()` replaces `openmail`. Mail between players follows RODEX: items and zeny per mail, the fee, the `no_mail` restriction, and expiry with unread items returned to the sender. Each of these is a ruleset rule with rAthena's value. Event `mail` with { `from`, `to`, `items`, `zeny` } after a mail is sent. Hook `mail` refuses a mail with a message and needs `rules.override`. New limits `mail_items` (base 5, ceiling 20) and `mailbox` (base 100, ceiling 1,000).
    - **Vending.** Shops open with the Vending skill, as rAthena. Event `vend` with { `seller`, `buyer`, `items`, `zeny` } after a sale. Hook `vend` refuses a sale with a message and needs `rules.override`. The tax is a ruleset rule from `vending_tax` and `vending_tax_min`. `player:autotrade()` closes the connection and keeps the shop open. Autotrade shops are restored after a restart, as `autotrade_restore`. `player:search_stores({ uses, effect })` replaces `searchstores`, and results can filter by random option rolls.
    - **Buying stores.** `player:open_buying_store({ slots })` replaces `buyingstore`. Event and hook `buying_store_sale` work as `vend`.
    - **One shop type.** A `shop` in `data/npcs.yml` has `buys_from_players`, `discount` and `items`. Each item has an optional `cost` that mixes `zeny`, `items` (a list of { `item`, `amount` }) and `currency` (a map of currency name to amount), with the item's buy price in zeny when there is no cost. `stock` limits an item for every player, and `restock` refills it (`daily`, `weekly`, or a time such as `"6h"`). The fields combine freely. rAthena's `shop`, `itemshop`, `pointshop`, `cashshop`, `marketshop` and barter shops all convert to this type.
    - **Currencies.** Records in `data/currencies.yml`: `name`, `display_name`, `owner` (account or character) and `max`. Core defines `cash` and `kafra`. `player:get_currency(name)`, `player:give_currency(name, n)` and `player:take_currency(name, n)`, which returns false when there is not enough. A mod changes its own currencies freely. Another mod's currency is named `mod:name` and needs a new permission, `currencies.modify`. Moves go through transfers and the logs. The converter turns a `pointshop` variable into a currency and maps that variable's reads and writes to these calls.
    - **Shop changes.** `world:get_shop(name)` returns a handle with `add_item(item, { cost, stock, restock })`, `remove_item(item)` and `set_stock(item, n)`. They replace `npcshopadditem`, `npcshopdelitem`, `npcshopitem` and `npcshopupdate`, and need a new permission, `shops.modify`. Events `shop_buy` and `shop_sell` with { `player`, `shop`, `items`, `zeny` }.
    - Later: real-money cash purchases, auctions (rAthena removed them), a stock limit per player, and costs priced by a Lua function.

31. Built-in automatic events (phase 5 step 8). The user approved these on 2026-10-05:
    - **Typed events.** An event in `data/events.yml` takes `type`: `boss`, `invasion` or `treasure_hunt`. An event without `type` is a script event, as before. A typed event may still have a `script`.
    - **Boss spawn.** `boss` has `monsters` (one is picked), `level_band` (fields whose spawns fall in that range) or `maps`, and `announce`. The boss leaves when the duration ends.
    - **Invasion.** `invasion` has `town` and `waves`. Each wave has `monsters` and `start`, which is `cleared` or a time such as `"120s"`. Monsters enter from the town's edges and attack players. The raid is won when the last wave dies and lost when time runs out. Waves and the result are announced.
    - **Treasure hunt.** `treasure_hunt` has `chests`, `level_band` or `maps`, `chest` (a monster record whose drops are the loot) and `hint_npcs`, which tell the map and direction of a chest that is still there.
    - **Rewards.** `rewards` is `{ to, items, zeny, exp, currency }`. `to` is `killer`, `participants` or `everyone_on_map`. Participants are players who did at least `min_share` of the damage (default 1%), and each gets the full reward. Rewards that do not fit the inventory are mailed.
    - **Lua.** A typed event's script gets the run with `context:get_event()`: `run:get_monsters()`, `run:get_participants()` and `run:finish(result)` with `"won"` or `"lost"`. World events `world_event_start` and `world_event_end` carry { `name`, `type`, `result` }. `world_events.start(name)` and `world_events.stop(name)` need a new permission, `events.control`.
    - **Core events.** Core ships one sample event of each type, turned off by default. The server owner turns them on in the config.

## Version

The API is version 1.0, and a mod's manifest says `api: 1.0` or a range that includes it, such as `">=1.0 <2.0"` (strategy section 6.12). Manifests said `api: experimental` through phases 3 and 4, since phase 4 changed parts of the API, such as monster stats, and moved to `api: 1.0` after the phase 4 exit check.

## Next steps

1. Build phase 3 step 2, the mod loader, with this context model. Done.
2. Build the modules in the order the phase 3 steps need them: `wa.context`, `wa.events`, and `wa.timers` with the loader, `wa.bonuses` with item scripts, `wa.variables` with NPCs, and `wa.hooks`, `wa.spawning` and `wa.schedules` with world events. Done for these steps. `wa.storage` and `wa.exports` come later.
3. Review the API that step 4 added. Done: review decision 14.
4. Review the provisional details of `wa.testing` that step 5 added (review decision 15).
5. Review the provisional details that step 6 added. Done: review decision 17. `respawn_milliseconds` stays provisional.
6. Build the status API of phase 4 step 2 (review decision 19). Done on 2026-10-02 ([decision 0010](../decisions/0010-status-effects.md)). Details to review:
   - The objects of the `status_chance` and `status_duration` hooks also carry rAthena's value, as `chance` and `milliseconds`, as the `drop_rate` object carries `rate`.
   - The events of a script call's starts and ends run after the call. Stats change inside the call since the change of 2026-10-02 in review decision 19.
   - `tick_milliseconds` ticks once more when the status runs out, as rAthena's statuses that tick.
7. Build the entity queries and the skill API of phase 4 step 3 (review decisions 18 and 20). Done on 2026-10-02 ([decision 0011](../decisions/0011-skills.md)). Details to review:
   - An active skill's `context:get_instance()` is `{ skill }`. A passive skill's is `{ skill, level, owner }`.
   - `cast:heal` returns the HP healed, or nil when the caster is gone.
   - `alive = false` finds dead entities only.
   - A new mod skill has 1 hit, can be interrupted, and starts no status, unless its data says otherwise.
   - Query limits: a radius of at most 30, a rectangle side and a line of at most 61 cells, a line width of 1 to 15, at most 1,024 cells, and a limit of 1 to 1,000.
   - The skill hooks work in mod, NPC and event contexts, not in skill contexts.
   - Ties of an entity query sort by the zone's entity ID, the order the zone added the entities in.
   - `cast:run_default` runs core's behavior once the script's call returns.
8. Build the job change API of phase 4 step 4 (review decision 21). Done on 2026-10-03 ([decision 0012](../decisions/0012-second-classes-and-job-changes.md)). Details to review:
   - Without `force`, `player:change_job` also keeps a transcendent player in the line of its second class before the rebirth, as the transcendent job NPCs check with `ADVJOB`. `force` reaches any job of the job tables.
   - A male player who changes into a Dancer becomes a Bard, and the reverse, as `pc_jobchange`.
   - `player:rebirth` needs a second class at base level 99 and job level 50, as the rebirth NPC. It returns false for other players.
   - `player:reset_skill` returns false for a quest skill, and `with_dependents` leaves quest skills among the dependents as they are.
   - `wa.testing`'s `new_player` spec gained `sex`, `skills`, `stats`, `skill_points` and `status_points`, so mod tests can set up a job change. These fields are provisional.
9. Build the monster API of phase 4 step 5 (review decision 22). Done on 2026-10-03 ([decision 0013](../decisions/0013-monster-ai.md)). Details to review:
   - `attack` in `data/monsters.yml` is one number, rAthena's `Attack`. Pre-renewal also takes `[minimum, maximum]`, rAthena's `Attack` and `Attack2`. `magic_attack` is renewal only, where it is rAthena's `Attack2`.
   - `modes` now adds to the modes of `ai`. Before, it replaced the monster's modes.
   - The skills list has no field for two parts of rAthena's rows, so they are dropped: the mode change of `NPC_EMOTION` and `NPC_EMOTION_ON` (441 renewal rows), and the second emotion of `NPC_EMOTION` (71 renewal rows). A field for them needs review.
   - The color of a `mob_chat_db.yml` line is dropped. `chat` holds only the text.
   - `target_casting` is rAthena's `trickcasting`, and `spawned` is `onspawn`.
   - The `monster_target` hook's object has `current`, rAthena's choice, as the other hooks carry rAthena's value. It works in mod, NPC and event contexts.
   - The monster methods need no permission. Reading the fields needs `game.read`, as other handle fields.
   - `walk_to` keeps the AI waiting until the walk ends, as rAthena's `force_walk`. `use_skill` takes a level from 1 to 100 and skips the delays of the skills list.
   - A monster AI script's context also ends when its mod reloads. Monsters spawned after the reload run the new script.
10. Build the job definitions and the details before 1.0 of phase 4 step 7 (review decision 24). Done on 2026-10-03 ([decision 0015](../decisions/0015-job-definitions.md)). The details of next steps 6 to 9 are accepted as built, and the API is 1.0. The parts that step 5 dropped now have fields: `modes`, a list of two for `emotion`, and `chat_color`. The user accepted these details as built on 2026-10-03, except that a character whose mod job was removed must still load:
   - A new job's `display_name` defaults to its name without the mod prefix, as for new monsters, and not to its look's name.
   - `changes_from` levels are optional. Core's records have none, since rAthena's `jobchange` checks none, so `player:change_job` without `force` still accepts core's next jobs at any level. The Job Master checks rAthena's levels.
   - A new job with no `changes_from` takes its look's, so its look's earlier job may change into it. `changes_from: []` leaves it to scripts with `force`.
   - On core's jobs, `tier`, `transcendent`, `baby`, `base_job` and `inherits` cannot change. `look` changes only the sprites.
   - A mod job's class is its look's class with its own tier, transcendent and baby bits. A second job without second job bits gets the 2-1 bit.
   - An item's `jobs` list with a core job's name sets rAthena's `Jobs` bits of its class, so `knight` covers Lord Knight and Rune Knight unless `classes` leaves them out. A mod job's name lets that job wear the item whatever its class.
   - `aspd` merges by weapon type. `hp` and `hp_table` replace each other, and `hp_table` must cover every base level.
   - `jobs.id` needs `game.read`, as `skills.id`.
   - The `job_change` event also fires for `player:rebirth`, with `to` the High Novice.
   - `chat_color` is a number written 0xRRGGBB, from 0x000001, since 0 is the channel's own color in the protocol. Its default is rAthena's red.
   - `emotion` shows both emotions at the cast's end, in order, on any skill. `modes: []` on `NPC_EMOTION` ends the mode change, as a row whose mode equals the monster's.
   - `world:equip` leaves the given copy in the inventory when it cannot be equipped.
   - A roll's `display_name` replaces the label from its bonus. The unit stays.
   - `api: experimental` is refused with a message, and a range such as `^1.0` must include 1.0.
11. Build companions, mounts and carts (review decision 25), and load a character whose mod job was removed as a Novice. Done on 2026-10-03 ([decision 0016](../decisions/0016-companions-and-mounts.md)). Details to review:
   - Records of mods are named with the mod's ID, as jobs and skills, such as `example.familiars:Owl`. Core's are plain names. Mod records get IDs from 10,000, and core's keep the IDs of their files, from 1.
   - The looks a record may name are the sprites rAthena's option bits show: companions `falcon` and `warg`, mounts `peco_peco`, `grand_peco`, `gryphon`, `dragon`, `dragon_brown`, `dragon_gray`, `dragon_blue`, `dragon_red`, `warg` and `madogear`.
   - A core record acts as itself. A new record without `acts_as` follows no core rules: it has a look and bonuses only.
   - `speed` is the walk speed bonus in percent, 0 to 50, in place of the rules' bonus. The rules give 25 for a Peco Peco or a dragon.
   - `requires` entries take `level` as optional, 1 by default.
   - A record's `bonuses` cannot read rolls.
   - `companions.id` and `mounts.id` need `game.read`, as `jobs.id`. The set calls need no permission.
   - `player.cart` is the cart's look, or nil. `player:set_cart` returns false for a look above 5.
   - `player:reset_skills` removes a companion or mount whose skills it resets, as `resetskill`, and `player:reset_skill` one whose `requires` the player no longer meets.
   - The converter maps `checkfalcon`, `checkriding` and `checkcart` to tests of `player.companion`, `player.mount` and `player.cart` against nil. `checkriding` is then true on any mount.
   - Not built, since the decision has no API for them: `setdragon`, `setmadogear`, `checkdragon`, `checkwug`, `ismounting` and the warg companion. Each needs a review item before it converts.
12. Build chat and moderation (review decision 26). Done on 2026-10-03 ([decision 0018](../decisions/0018-chat-and-moderation.md)). Details to review:
   - Core's channels are rAthena's defaults, `#global`, `#support`, `#trade` and `#map`, not `#main`. A mod changes them by name in `data/channels.yml`, and cannot add channel records: it makes its own with `channels.create`.
   - The fields of a channel record: `display_name`, `color`, `join`, `autojoin`, `can_leave`, `can_chat`, `delay_milliseconds` and `kind` (`world` or `map`). The fields of a chat filter word: `action` (`replace`, `reject` or `reserve`) and `match` (`word` or `part`).
   - The `chat` event and hook's `channel` is the channel's name without its `#`, and `public`, `whisper`, `party` and `guild` are not channel names. `to` is the whisper target's name, not a handle, since the target may be on another map. The event's `channel` option takes a name with or without its `#`.
   - The `chat` event runs in mod, NPC and event contexts, after the line went out. A line a hook stopped runs no event. A hook cannot tell the speaker, since hooks only compute.
   - A channel from `channels.create` is one for the world, shared by the same mod's contexts in every zone and after a reload, and ends with its last owner. The channel handle has only the methods of the decision, without fields such as `name`.
   - `channels.create`, `channel:send`, `channel:add` and `channel:remove` need no permission. `channels.get`, `channel:get_members` and `player:is_blocking` need `game.read`. `channel:add` and `channel:remove` skip the join and leave rules, and return false when nothing changed.
   - `channel:get_members()` returns names, sorted. `player:is_blocking` takes a player handle.
   - `player:whisper` skips the filter, the hooks and the block list, and is not logged. Its default `from` is the NPC's display name in an NPC context, else the manifest's `name`, cut to 24 characters.
   - Minutes of `mute` and `jail` are 1 to 5,256,000. A new mute or jail replaces one that has not ended. There is no `unjail`. `unmute` returns false when the player was not muted.
   - `wa.testing` gained `world:chat(player, text, { channel, to })` and `world:get_chat(player)`, provisional, so mod tests can use the chat event and hook.
   - The converter maps `channel_chat`, `channel_join`, `channel_kick` and `channel_create` in their plain forms. Needed for the rest: deleting a channel, channel options, passwords and bans, a line in another color, the map channel of one map, and joining or kicking another character.
13. Build friends, parties and limits (review decision 27). Done on 2026-10-04 ([decision 0020](../decisions/0020-parties-and-friends.md)). Details to review:
   - The limits are `provides.ruleset.limits` and the rules `provides.ruleset.rules` of a ruleset mod's manifest. The rules are `party_share_level`, `party_even_share_bonus`, `party_item_share_type`, `change_party_leader_samemap`, `block_account_in_same_party` and `friend_auto_add`. Lua reads only the limits, from a new module `wa.rules`, as `rules.limits`. Reading the rules and flags from Lua (`rules.is` of strategy section 3.6) needs more API.
   - The `experience` hook runs once for each player who gets exp. With even share it runs for each member on the map, and `share` is a table `{ base, job }`: the member's own share of the kill before the split and the level penalty, `{ 0, 0 }` for a member who did not fight.
   - `party_join` and `party_leave` run on the map of the player who joined or left. A member who is offline gets no event, since the event's `player` is a handle. A player who makes a party itself gets no `party_join`: the reasons have no value for it. `parties.create` fires `party_join` with `"script"`.
   - `party_change` runs on the map where the change was made, and its `leader` is nil when the leader is on another map. Its `options` is a table of the three options.
   - `party.leader` is nil when the leader is offline or on another map, and `get_members` returns the online members on the context's map, since player handles belong to one map.
   - `party:add`, `remove` and `set_leader` return booleans. `set_options` and `disband` return nothing. `set_options` changes only the options it names. `parties.get` returns nil for an ID of 0 or less.
   - A party handle stays readable after the party broke up, for the last 1,024 parties, so a `party_leave` handler can read its name. Its methods then raise an error.
   - In `wactl mod test`, a test case's own code needs no permission for every `wa.parties` call, not only `parties.create`, so a test can set up members.
   - `party.manage` needs the owner's approval and is not flagged.
   - The converter maps `party_create`, `party_addmember`, `party_delmember`, `party_changeleader`, `party_changeoption`, `party_destroy`, `getpartymember` (names and count), `getpartyname`, `getpartyleader` (name), `is_party_leader`, `getcharid(1)` and `strcharinfo(1)` for the attached player. Needed for the rest: the result codes of the party commands, `$@party_create_id`, character and account IDs of members, the leader's job, map and level, `warpparty`, and the commands for another character.
14. Build guilds (review decision 28). Done on 2026-10-04 ([decision 0021](../decisions/0021-guilds.md)). Details to review:
   - `guild_change` gets `{ guild, change }`, where `change` is `"master"`, `"level"`, `"notice"` or `"emblem"`; the guild's fields hold the new values. It runs on the map where the change was made, and a new emblem from the account pages runs it on the map of the master, or of the first online member.
   - `guilds.create` fires `guild_join` with `"script"` for the master. A player who makes a guild from the client gets no `guild_join`.
   - `guild:get_position` returns nil for a player who is not a member. `guild:remove` returns false for the master: the master hands the guild over first.
   - `guild:set_master` skips `guild_leaderchange_delay`, as `guildchangegm`. `guild:set_notice` returns false for a title over 59 characters or a text over 119. `guild:give_exp` takes 1 to 2147483647.
   - `guild:disband` breaks a guild up with its members and loses its storage, which players cannot do while members remain.
   - `guild_tax` is the share of `base` before the hook; the tax applies at the same rate to the base the hook returns.
   - The guild rules are ruleset rules: `guild_emperium_check`, `guild_exp_limit`, `emergency_call`, `guild_skill_relog_delay`, `guild_leaderchange_delay` and `guild_alliance_onlygm`.
   - `guild_storage`'s decided default is 500. rAthena's `MAX_GUILD_STORAGE` is 600, which Guild Storage level 5 gives, so the default caps level 5 at 500. 600 would match rAthena.
   - Core's converted scripts open the guild storage through `wa.internal`, which only the core mod may require. It is not API. A public call, such as `player:open_guild_storage()`, needs review.
   - The converter maps `getcharid(2)`, `strcharinfo(2)`, `getguildname`, `getguildmaster`, `getgdskilllv`, `is_guild_leader`, `guild_has_permission`, `getguildmember` (names and count), `guildgetexp`, `guildopenstorage` and `guildopenstorage_log`. Needed for the rest: raising a guild skill (`guildskill`), a master change by name (`guildchangegm`), reading the guild storage (`guildstoragecountitem`), character IDs (`getguildmasterid`), warping a guild (`warpguild`, `maprespawnguildid`), counting a guild's members on a map (`getmapguildusers`), the guild's emblem and castles (`getguildinfo`), and castles and sieges. `getguildalliance` maps once a helper returns rAthena's relation codes over `is_allied` and `is_enemy`.
15. Build storage, cart, trade and limit ranges (review decision 29). Done on 2026-10-04 ([decision 0023](../decisions/0023-storage-cart-trade.md)). The details of next step 14 about `guild_storage` and `wa.internal` are settled: the default is rAthena's 600, and `player:open_guild_storage()` replaced the `wa.internal` call. Details to review:
   - `player:open_storage(name)` takes a record's full name, or a plain name, which is the calling mod's record first, then core's. It returns false when the player has a storage open, or another character of the account has this one open. Two characters of one account can be online at once in wAthena; they share the storage, one opener at a time.
   - `player:open_guild_storage()` returns true, or false and a reason: `"no_guild"`, `"no_storage"`, `"no_permission"`, `"in_use"` or `"already_open"`. The second value lets the converter give back rAthena's `GSTORAGE_` codes.
   - `count_item` with `from` returns nil where rAthena returns -1: no cart, or a storage window open (`storage_flag`). `take_item` with `from` takes nothing when there are too few, as `storagedelitem`.
   - `give_item` with `to` follows the place's restrictions and bound rules, and returns false when they keep the item out. rAthena has no command that gives into a storage.
   - `data/storages.yml` records have `display_name` (1 to 24 characters, the name without the prefix by default), `slots` (1 to the `storage_slots` ceiling) and `owner` (`account` by default, or `character`). Mod records get IDs from 10,000. Core has no records.
   - The `trade` object is `{ players, items, zeny }`: `players` holds the player who confirmed last first, `items` lists `{ from, item, amount }`, and `zeny` the zeny each player gave in the order of `players`. The event and the hook run in mod, NPC and event contexts. The hook's message reaches both players, cut to 255 bytes.
   - A player who blocks the requester refuses a trade request at once, as party and guild invites. rAthena does not check its ignore list here.
   - `rules.limits.<name>` stays the base. Reading a limit's max, and a call that expands a limit, such as rAthena's `expandinventory`, need API.
   - `wa.testing` gained `world:trade(a, b, offer_a, offer_b)` and `world:close_storage(player)`, provisional.
   - The item record field is `restrictions`, a list. `no_drop` and `no_sell` stay as fields and follow the list. rAthena's `trade_partner` and `no_auction` are left out until marriage and auctions, and `Override` until group levels.
   - The converter maps `openstorage`, `openstorage2` for storage 0 in mode `STOR_MODE_ALL`, `storagecountitem`, `cartcountitem`, `guildstoragecountitem`, `storagedelitem`, `cartdelitem`, `getitembound` and `getitembound2` without refine, attribute or cards, and `basicskillcheck()` as rAthena's default of yes, since Lua cannot read rules. Core's Kafras open storage with rAthena's fee. Needed for the rest: listing the bound items of an inventory (`countbound`), taking HP or SP (a negative `percentheal`, which keeps the Niflheim branch of `F_KafStor` and every Kafra partly converted), get-only or put-only storage (`openstorage2` modes), reading a rule (`basicskillcheck`), taking items from guild storage (`guildstoragedelitem`), random options in `give_item` (`getitembound3` and `getitembound4`), the exact-copy `2` forms of the count and delete commands, and commands for another account or character.
16. Build mail, vending, buying stores, shops and currencies (review decision 30). Done on 2026-10-04 ([decision 0024](../decisions/0024-mail-vending-shops.md)). Details to review:
   - `player:send_mail`'s `to` is a name, a character ID or a player handle, and the player itself when left out. Its `sender` defaults to the NPC's name in an NPC context, else the mod's name, as `player:whisper`'s `from`. The call returns nothing: a mail to a character who is not online is found after the call returns. Script mail skips the `mail` hook, and the `mail` event runs for it.
   - `player:send_mail` makes its items and zeny; the player pays nothing. Its `items` take `roll` as `give_item` does, with `items.modify_rolls` for a table, and `bound`.
   - `player:search_stores`'s `effect` is `"show"` (the default) or `"open"`, for rAthena's 0 and 1. `uses` is 1 to 1000. A store opens from a search only on the searcher's map.
   - `player:open_buying_store`'s `slots` is 1 to 5. The buying store flag has no field in `data/items.yml`: mod items cannot be bought by buying stores until it gets one.
   - `wa.world`'s functions are called with a dot, so the decision's `world:get_shop(name)` is `world.get_shop(name)`; both work. `get_shop` needs `game.read`. A plain name is the calling mod's shop, then core's.
   - `shop:add_item` takes `{ cost, stock, restock }`, with `cost` as in `data/npcs.yml`, and returns false for a cost that names an item or a currency that does not exist. `shop:set_stock(item, -1)` takes the limit away. A changed stock saves; an added item does not.
   - The events `shop_buy`, `shop_sell`, `vend`, `buying_store_sale` and `mail` take no options. They run in mod, NPC and event contexts. `vend` and `buying_store_sale` have `{ seller, buyer, items, zeny }`, where a buying store's seller is the player who sells to it, and each item has `price`. `mail` has names in `from` and `to`, not handles, as `chat`'s `to`.
   - `get_currency` needs `game.read`. Core's `cash` and `kafra` count as another mod's currencies, so changing them needs `currencies.modify`.
   - The shop table's `buys_from_players` and `discount` default to true, as rAthena's `shop` line. Discount lowers only the zeny part of a cost. A list form shop may repeat an item; a table may not.
   - `wa.testing` gained `world:buy(player, shop, items)`, `world:get_mails(player)` and `world:vend(seller, buyer, lines)`, provisional.
   - The converter maps `mail`, `openmail`, `searchstores`, `buyingstore`, `npcshopadditem`, `npcshopdelitem`, `npcshopupdate`, `callshop` of every shop kind, and the reads and writes of `#CASHPOINTS`, `#KAFRAPOINTS` and pointshop variables. Needed for the rest: listing a shop's items (`npcshopitem`), a script that takes over a shop's sales (`npcshopattach` with `OnBuyItem` and `OnSellItem`), a barter cost of a refined item or of 6 items, Kafra points that pay part of a cash shop price, the refine, bound, card and option arrays of `mail`, and `searchstores` for another map.
17. Build automatic events (review decision 31). Done on 2026-10-05 ([decision 0025](../decisions/0025-automatic-events.md)). Details to review:
   - A typed event's fields sit beside `type` in its record, such as `monsters` and `level_band` for a boss, not in a mapping under the type's name.
   - `level_band` is `[lowest, highest]`. It picks fields, maps whose ID has `_fild`, by the middle level of their spawns without bosses. A map type in map data would let it pick other maps.
   - `announce` is true or false, true by default. A wave's `monsters` is a list of `{ monster, amount }`, and `start` takes `cleared` or a time in `s`, `m` or `h`. `hint_npcs` is a list of `{ map, x, y, sprite, display_name }`, with `4_M_MANAGER` and "Treasure Hunter" by default.
   - `rewards` is one mapping, with `min_share` in percent, from above 0 to 100, and `exp` as `{ base, job }`. Each kind of reward needs the permission a script needs to give it. A list of rewards, such as one for the killer and one for the participants, needs review.
   - Rewards go only to players on the map of the boss, the chest or the town. An invasion's `killer` is the player who killed its last invader.
   - A typed run that a stop cut short goes on after the restart while its window is open, from its saved progress. A script event's run still does not.
   - Core's samples are `core:Field_Boss`, `core:Prontera_Invasion` and `core:Treasure_Hunt`, off until `mods.events.<name>.enabled` is true. `enabled` is a new field of `mods.events`.
   - `context:get_event()` returns nil outside a typed event's script. The run handle has the three methods and no fields.
   - `run:get_monsters()` and `run:get_participants()` need `game.read` and read the context's map only. `run:finish` takes effect within a second, and an invasion that finishes won gives its rewards.
   - `world_event_start` and `world_event_end` run in mod, NPC and event contexts. `type` is `"script"` for an event without one, and `result` is `"won"`, `"lost"`, `"stopped"` or nil.
   - `world_events.start` and `stop` return whether the request was taken. A plain name is the calling mod's event. An event the config turned off does not exist for them.
   - The announcements and the hint NPCs' text are English in Go, with map IDs, until core has locale text.
