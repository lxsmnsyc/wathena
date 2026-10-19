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
| `wa.generation` | Generation layers and quest templates of a new procedural world (review decision 46) | `world.generate` |
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

32. Phase 5 review and rule reading (2026-10-12). The user approved these on 2026-10-12:
    - **Details accepted as built.** The details to review of next steps 11 to 17 are accepted as built, except as this decision changes them. So are the behavior notes of phase 5 step 3 in [decision 0019](../decisions/0019-ruleset-flags.md).
    - **Reading rules.** `wa.rules` gains `rules.is(flag)`, true when a ruleset flag is on, `rules.get(name)`, the value of a ruleset rule, and `rules.get_limit(name)`, which returns a limit's base and max. `rules.limits.<name>` stays the base. They need no permission.
    - **Expanding a limit.** `player:expand(limit, amount)` raises one of the player's limits, such as `inventory_slots`, up to the limit's max, and returns the new value. A negative amount lowers it, not below the base. It needs a new permission, `player.expand_limits`. The value saves with the character, or with the account for account storage.
    - **Joins on create.** A player who makes a party or a guild gets `party_join` or `guild_join` with the reason `"create"`.
    - **Block lists and trades.** A new ruleset rule, `block_list_refuses_trade`, on by default, controls whether a block list refuses trade requests.
    - **Test helpers.** The provisional `wa.testing` helpers are stable: `world:chat`, `world:get_chat`, `world:trade`, `world:close_storage`, `world:buy`, `world:get_mails`, `world:vend`, and those of review decision 15.
    - **Converter commands.** The script commands that need new API, listed in next steps 11 to 16, wait for one converter review, ordered by how many core scripts each blocks.
    - **Script events** still do not resume after a restart.

33. Staff roles, the admin API and the GM audit log (phase 6 step 1). The user approved these on 2026-10-12:
    - **Roles.** The config's `staff.owners` lists account names with the `owner` role, which has every permission and comes only from the config. `staff.roles` maps role names to permission lists, with defaults `admin`, `game_master` and `moderator`. Owners and admins give the other roles to accounts from the admin screens, and the database keeps them. Permission names come from a fixed list in the server, and the config is checked against it. `*` matches a group, such as `moderation.*`.
    - **Two-factor login.** A role counts only when the account has TOTP or a passkey turned on and the session logged in with it. Role changes, item restores and bans also need a re-login within 10 minutes (`/account/reauth`).
    - **Admin API.** JSON at `/admin/api/v1/`, with the account session cookie and a CSRF token, and a permission check on every call. Messages are protobuf under `schema/proto/wathena/admin/v1`, encoded as JSON. Step 1 has `GET /me`, `GET /staff`, `POST /staff`, `DELETE /staff/{account}` (`staff.assign`) and `GET /audit` (`audit.read`).
    - **GM audit log.** Each staff action writes a row with the time, account, role, action, target, arguments, reason and a review mark. The game's database roles can only insert rows. Reviews are kept in a separate table. Actions that create items or zeny need a reason and are marked for review. Rows are kept 2 years; the deletion job comes with phase 8.
    - **Admin screens.** At `/admin` in the web client, as a separate bundle that loads only after `/me` returns a role. Step 1 has the staff list and the audit log viewer.
    - **Lua.** `player:get_staff_role()` returns the role name or nil, and `player:has_staff_permission(name)` returns a boolean. Both need `game.read` and follow the two-factor rule.

34. Player search, item history, restores and audit reviews (phase 6 step 2). The user approved these on 2026-10-13. No Lua API changes:
    - **Search.** `GET /players` finds accounts and characters by account or character name (prefix, any case), account or character ID, item serial (the current holder), or IP range in CIDR form against login records of the last 90 days. The IP search needs `players.read_ip`. Results show the account, its characters, online status and position, the last login, and sanctions in effect.
    - **Details.** `GET /characters/{id}` shows job, levels, stats, zeny, currencies, position, party, guild, inventory, equipment, cart, account and extra storages, and mail with items. Online players are read from the game, offline players from the database.
    - **History.** `GET /items/{serial}/history` lists every move of a serial and follows splits and merges. `GET /characters/{id}/logs` lists a character's item and zeny log by date. Both need `items.history`.
    - **Restore.** `POST /restore` takes a character, items or zeny, and a reason, needs a login within 10 minutes and `items.restore`, and is marked for review. A serial that still exists moves back from its holder through the transfers. A serial that is gone is made again as a copy, which needs `items.create`. Restored items arrive by mail.
    - **Reviews.** `GET /audit?needs_review=true` and `POST /audit/{id}/review` with `ok` or `flag` and a note, with `audit.review`. No one reviews their own action.
    - **Lookups in the audit log.** Player detail views and IP searches write audit rows. Name searches do not.
    - **Default roles.** `players.read` for every default role, `items.history` for admin and game_master, and `players.read_ip`, `items.restore`, `items.create` and `audit.review` for admin.

35. Moderation and GM commands (phase 6 step 3). The user approved these on 2026-10-13. No Lua API changes:
    - **Reports.** `GET /reports` with filters for status, target and kind, `GET /reports/{id}` with the reported lines, the chat around them, and the target's earlier reports and sanctions, and `POST /reports/{id}/close` with an outcome (`no_action`, `warned` or `sanctioned`, linking the sanction) and a note. They need `moderation.reports`. Players can also report guild names and guild emblems from the guild window. After a report closes, its lines follow the normal chat log limit again.
    - **Sanctions.** `POST /sanctions` with a target, a kind, a duration and a reason: `mute`, `jail` and `kick` act on a character, and the new `ban` acts on an account. `DELETE /sanctions/{id}` lifts one early with a reason. `GET /sanctions?target=` lists a target's sanctions. A ban kicks the account's characters and refuses its logins, showing the reason and end time. A kick shows its reason. Permissions are `moderation.mute`, `moderation.jail`, `moderation.kick` and `moderation.ban`. Bans need a login within 10 minutes. A permanent ban needs `moderation.ban_permanent`, which only the default `admin` role has.
    - **GM commands** typed in chat: `@warp <map> [x y]` and `@jumpto <name>` (`commands.warp`), `@recall <name>` (`commands.recall`), `@hide` (`commands.hide`; invisible to players and ignored by monsters, as rAthena), `@kick <name> <reason>`, `@mute` and `@jail <name> <minutes> <reason>` (the moderation permissions), and `@item <item> <amount> <reason>` (`items.create`, marked for review). They follow the two-factor rule, write audit rows, never show in chat or the chat log, and answer "Unknown command" without the permission.
    - **Screens.** The report queue with chat context, sanction buttons on the player page, a target's sanction history, and the list of active bans.
    - Later: IP bans, and `@` commands from mods.

36. World, events and mods tools (phase 6 step 4). The user approved these on 2026-10-13:
    - **World view.** `GET /world` with `world.read`: each zone's map, players, monsters, and tick times over the last minute (p50, p99, maximum, ticks over budget); the process's CPU, memory, goroutines, uptime and version; and the players online. The screen refreshes every 5 seconds.
    - **Announcements.** `POST /announce` with `{ text, map }`, for the world or one map, with `world.announce` and an audit row.
    - **Planned restarts.** `POST /restart` with a time, a reason and a message, and `DELETE /restart` to cancel, with `world.restart` and a login within 10 minutes. Players get announcements 30, 15, 10, 5 and 1 minute before and 30 seconds before. New logins are refused in the last minute. At zero the server saves everyone, disconnects players with the reason and exits, and the process manager starts it again.
    - **Events.** `GET /events` lists core's and mods' events with whether each is on, its trigger, its next run, the runs in progress with their time left, and the latest results. `POST /events/{name}/start` and `/stop`, and `PATCH /events/{name}` for whether it is on, its trigger and its duration, with `events.control`. Changes are saved in the database over the config, show who made them, and can be reset to the config.
    - **Mods.** `GET /mods` with `mods.read`: version, signature status, granted and pending permissions, and Lua error counts per zone with the last error and its time. `POST /mods/{id}/disable` with a zone or all zones and a reason, and `/enable`, with `mods.disable`. A disabled mod's scripts stop in that zone (event handlers, hooks, timers and NPC clicks), and its data stays. It stays off until turned on again, across restarts. The core mod cannot be turned off.
    - **Lua.** A world event `restart_planned` with `{ at, reason }` when a restart is planned, and with `{ cancelled = true }` when it is cancelled.

37. The account page (phase 6 step 5). The user approved these on 2026-10-14. No Lua API changes:
    - **Block lists.** Each character's block list, with unblock. Blocks stay per character.
    - **Reports and sanctions.** The reports the account's characters filed, with date, target, reason and status. A closed report shows only "action taken" or "no action". The sanctions on the account and its characters, active and past, with reason and end time.
    - **Data export.** A JSON file with the account (name, email, creation date, whether two-factor is on, no secrets), login records of the last 90 days with IP addresses, each character's stats, skills, inventory, equipment, cart, storage, mail and currencies, the characters' chat lines still kept and their item and zeny logs, the reports filed and the sanctions. It needs a login within 10 minutes, is limited to once a day, and is built on request and downloaded at once.
    - **Account deletion.** It needs a login within 10 minutes and the account name typed to confirm. After a 14-day wait, a job deletes the email, password, two-factor settings, sessions, login records, the characters and their chat lines. Logs other players need keep the character ID without the account link. During the wait the characters cannot be played, and a login shows the date and a cancel button. A request is refused while one of the account's characters leads a guild with other members. A banned account may ask for deletion.
    - **Privacy notice.** `/account/privacy` with the owner's contact details and text from the config, and the retention periods in effect.

38. Mod assets (phase 6 step 6). The user approved these on 2026-10-14. No Lua API changes:
    - **Sources.** A mod ships art sources in `art/`: the pack format of assets-v1 ("Pack format for original art", with `pack.yaml` naming the license), and loose RO files (`.spr`, `.act`, `.bmp`, `.tga`, `.wav`, `.rsm`, `.str`, and maps as `.gat`, `.gnd` and `.rsw`).
    - **Builds.** `wactl mod pack` builds `art/` into an asset build with its own manifest inside the package. `wactl mod build-art` builds it for a folder mod. The server checks every file when it loads a mod (hashes, decoding and the asset limits) and converts nothing.
    - **Names.** Mod art is named `mod/<mod id>/<namespace>/<record name>`, such as `mod/example.lantern/icon/item/Paper_Lantern`, since mod IDs differ per server. The server tells the client which art each mod ID uses.
    - **Record fields.** Items: `icon`, `image`, `drop_sprite` and `view_sprite`. Monsters: `sprite` and `sounds`. NPCs: `sprite`. Jobs: `look` with body sprites per sex. Skills: `icon` and `effects`. Companions and mounts: `look`. Maps: `map/<name>` with music and minimap. A field names the mod's art or core's. Missing art falls back to a placeholder, and the client logs it once.
    - **Client data tables.** A mod may ship `client/*.yml` tables that override matching entries of core's. The server merges and serves one set. `wactl client-data validate` checks them against the schema.
    - **Serving.** A mod's build is served at `/assets/mods/<mod id>/<version>/`, cached by content hash. The client loads core's manifest, then each active mod's. Each mod's build is limited to 200 MB by default, set in the config.
    - **Tooling.** `wactl mod validate` reports art names a record refers to that the mod does not provide. `docs/client-data.md` is the guide. A sample mod has original art for an item icon, a monster sprite and an NPC.

39. Touch input (phase 6 step 7). The user approved these on 2026-10-15. No Lua API or protocol changes:
    - **Gestures**, chosen by pointer type, so mouse and touch work on one device: a tap does what a left click does; holding on the ground keeps walking toward the finger; a long press of half a second on a unit opens the right-click menu; a two-finger pinch zooms; a two-finger twist or drag turns the camera.
    - **Virtual joystick.** A joystick in a lower corner walks the player in the direction it is pushed, for as long as it is held. It shows when touch is used and can be hidden in the options. Tap to walk still works.
    - **On-screen controls** when touch is used: a larger hotbar for the F1 to F9 slots, buttons for the windows that keyboard shortcuts open, a chat button, and a cancel button while choosing a skill target.
    - **Small screens.** Below 600 pixels wide, windows open as full-width panels. Buttons and item slots are at least 44 pixels. Windows move by their title bar with any pointer.
    - **Item drags** start with a long press, in every place items move.
    - **Browser behavior.** No double-tap zoom, pull-to-refresh or text selection in the game view. Chat stays visible above the on-screen keyboard. Landscape and portrait both work. The account pages and admin screens are usable on a phone.
    - **Performance.** 30 FPS on mid-range phones, with the rendering resolution capped at twice the CSS size.
    - **Checks.** Phone and tablet emulation in `pnpm check:browsers` with a scripted touch run, and a test by the user on a real phone against the dev server on the local network.

40. The phase 6 exit check (phase 6 step 8). The user approved these on 2026-10-15. No Lua API or protocol changes:
    - **Mod text and art in the client.** `/client-data.json` gains the names and descriptions of mod items and the display names of mod monsters, skills and jobs, and its format version becomes 2. The client draws skill icons and effects, monster sounds, worn item sprites and companion art that the server already sends.
    - **The exit check.** A scripted browser run with three players, on desktop and on a phone profile, in `scripts/check.sh browsers`: walking, fighting, loot, levels, a job change, equipment, skills and NPCs; every tier 1 system of strategy section 11.3; an admin run that finds a player, reads their item history, mutes them and sees the mute in the game; and a mod run where a sample mod's item, monster, skill and job show their own names and art.
    - **Step 3 changes.** A player's chat line that starts with `@` goes out as normal chat when the player has no permission for any command, as in rAthena. Lifting a permanent ban needs `moderation.ban_permanent`.
    - **Then** one review of next steps 19 to 25.

41. Phase 6 review (2026-10-15). The user approved these on 2026-10-15:
    - **Details accepted as built.** The details to review of next steps 18 to 26 are accepted as built, except as this decision changes them.
    - **`accounts.staff`.** The column goes. `wactl world new --admin` prints the `staff.owners` line to add to the config.
    - **`@hide`** lasts across logins, and a hidden staff member's local chat shows to others, as rAthena.
    - **Bans** also end the account's web sessions.
    - **Test helper.** `wa.testing` gains `world:plan_restart(minutes, reason)`, which fires `restart_planned`.
    - **Data export.** Mod variables of the account and its characters join the export. New fields keep format version 1.
    - **Art.** A protocol field carries companion and mount art, so the client draws them. Weapons and shields get worn sprites through `view_sprite`, as headgear and garments do.
    - **Later.** Signature checks with trusted keys (phase 9), Ogg and MP3 sounds, pack map folders and zones running mods' maps, the inventory expansion voucher window, and the converter review of review decision 32.

42. Dungeons, towns and named places (phase 7 step 3). The user approved these on 2026-10-16:
    - **Dungeons.** Caves by cellular automata, and crypts, towers and ruins by rooms joined with a minimum spanning tree plus a few loops (strategy section 8.5). Each floor is a map `gen:dungeon:<id>:<floor>`. The exit to the next floor is in the room farthest from the entrance, and the last floor has a boss room. Each floor carries a level band one step above the floor before it. Styles are records in `data/dungeon_styles.yml`: method, floors, floor size, room sizes, tileset and level step.
    - **Towns.** A flattened plateau, main roads from each gate to a central square, side roads, plots, buildings, walls, gates, decorations and service NPC spots. Templates are records in `data/town_templates.yml`. A building lists its model, footprint and the named places inside it, such as `blacksmith`, `inn`, `storage` or `shop:tool`. Towns have IDs; names come with step 4.
    - **Named places.** Every generated map records its places with a tag and a cell: town services, `square`, `gate`, `entrance`, `exit`, `boss_room` and others.
    - **`wa.places`.** `places.find(scope, tag)` returns every match in a fixed order. `places.nearest(player, tag)` returns the closest by the map graph, then distance. A place has `get_map()`, `get_position()` (x, y), `get_tags()` and `get_area()` (such as `"town:t7"`, `"dungeon:crypt:2"` or `"field:3:-1"`). Scopes: `"town:any"`, `"town:<id>"`, `"dungeon:any"`, `"dungeon:<id>"` and `"map:<id>"`. It needs `game.read`. Classic worlds return no places.
    - **NPCs at places.** A record in `data/npcs.yml` may give `place: { scope, tag }` in place of `map`, `x` and `y`. It appears at every matching place.
    - **Mods** add town templates and dungeon styles as data records. Lua generation layers wait for step 6.
    - **Dev world.** `waserver --dev --towns <n> --dungeons <style>:<floors>` for walking towns and dungeons in the browser.

43. World layout (phase 7 step 4). The user approved these on 2026-10-16:
    - **Layout at world creation.** `wactl world new --mode procedural` builds the map graph from the public tree: by default a 6 x 4 grid of fields, 5 towns and 6 dungeons, set in the config. Fields link through portals on shared borders. Each town links to 2 to 4 fields. Dungeon entrances sit in fields, and floors link in order. Every map must be reachable from the home area, or the layout retries from a derived node, a bounded number of times. Level bands rise with graph distance from the home area, and dungeon floors build on their entrance field's band. The database keeps each map's ID, kind, display name, band and links.
    - **Names.** Town names come from naming styles, records in `data/naming_styles.yml` with mod records, one per biome. Names are 4 to 12 letters, unique in the world, pass the blocked-word filter, and retry with `name(1)`, `name(2)` and so on. Other maps take names from the nearest town and a template, such as "North Valheim Fields 2" or "Crypt of Skarvik", in English until locale files exist. `@renametown <town> <name>` changes a display name and needs a new permission, `world.rename`.
    - **Home area.** The first generated town by default. A mod may supply a home map in the map folder format of strategy section 8.12 (`map.yaml`, `cells.bin`, `materials.bin`, `objects.yaml`, `places.yaml`) tagged `home`, which the generator links to the nearest town. `wactl map validate` runs the generator's checks on a map folder. New characters start in the home area.
    - **Warps and points.** Portals come from the graph, and arrival puts the player on a walkable cell next to the linked portal. Save and memo points store map IDs, and removable maps get the `nomemo` flag. Each character's visited towns are recorded. A teleport NPC placed by tag at `teleport` in each town lists visited towns, with a price that rises with graph distance. `@go` lists towns by display name, and `@warp` accepts a display name.
    - **World map.** The client's world map window draws the part of the graph the character has discovered.
    - **Lua.** `wa.towns` with `towns.list()`, `towns.list({ visited_by = player })`, `towns.nearest(player)` and `towns.get(id)`; a town handle with `get_id()`, `get_name()`, `get_map()` and `get_band()` (lowest, highest); `player:has_visited(town)`; `maps.get_display_name(map)` in `wa.maps`; and `place:get_band()` on place handles. They need `game.read`. Classic worlds have no towns.

44. Spawns and level bands (phase 7 step 5). The user approved these on 2026-10-16:
    - **Bands.** The home area is levels 1 to 10, and each warp away from home adds `procedural.layout.band_step` levels (8 by default), capped at the level cap. Every band from 1 to the transcendent level cap of 99 needs at least 3 fields, or the layout retries. The default grid becomes 6 x 5 fields.
    - **Spawn tags.** A tool derives biome weights from rAthena's field spawn files by map family, committed as core data. Level, race, element, size and boss class come from monster data. Monster records may set `spawn: { biomes, role, themes }`. Roles are `common`, `uncommon`, `plant`, `rare`, `mini_boss` and `field_boss`, with the shares and respawn times of strategy section 8.11. Boss monsters are never `common` or `uncommon`. Themes are records in `data/spawn_themes.yml`, with mod records.
    - **Selection** follows section 8.11: filter, an optional theme, 5 to 8 species (at least 2 common, at most 3 of one element, at most 2 aggressive below level 30, at most 1 rare), counts from rAthena's density per walkable cell, placement (plants near features, no aggressive monster within 15 cells of a portal arrival), and bosses (a mini boss on a field next to a town at `spawns.town_field_boss_chance`, 50% by default; 1 or 2 field bosses per region elsewhere; none within 30 cells of a portal to a town). Dungeons use the same steps with their own biomes and deeper bands per floor.
    - **Secret branch at world creation.** `wactl world new` computes each map's spawn list from the map's secret node and stores it. The game server reads stored lists and never sees a secret key. A new generator version does not change an existing world's spawns.
    - **Hook `spawn_table`**, run at world creation only: `hooks.on("spawn_table", function(map, spawns, next) ... end)` with `map` = { `id`, `kind`, `biome`, `band`, `features` } and `spawns` a list of { `monster`, `count`, `respawn`, `role`, `area` }. It needs a new permission, `spawns.modify`.
    - **Settings.** `spawns.town_field_boss_chance`, `spawns.town_field_boss_role` and the density, rAthena's by default.
    - Later: better drop rolls in higher bands (step 8), density scaling by player count, and bosses placed by mods through `wa.generation` (step 6).

45. The quest system (phase 7 step 6a). The user approved these on 2026-10-17. Quests are a tier 1 system (strategy section 11.3) that phases 5 and 6 did not build:
    - **Records.** `data/quests.yml` from rAthena's `quest_db`: title, time limit, hunt objectives (monster and count, with the level, race and size filters of newer `quest_db`) and item objectives. Mods add records.
    - **Lua.** `player:add_quest(quest)` (`setquest`), `player:get_quest_state(quest)` returning `"none"`, `"active"`, `"inactive"` or `"complete"`, `player:complete_quest(quest)` (`completequest`), `player:remove_quest(quest)` (`erasequest`), `player:change_quest(from, to)` (`changequest`), `player:get_quest_progress(quest, objective)` (`checkquest` with `HUNTING`), `player:quest_timed_out(quest)` (`checkquest` with `PLAYTIME`), and `npc:show_quest_icon(player, icon)` (`showevent`). Events `quest_add`, `quest_progress` and `quest_complete`.
    - **Behavior.** Kills update hunt counts as rAthena does. Quest state is saved and joins the data export.
    - **Client.** A quest log window with objectives and timers, and quest icons over NPCs.
    - **Converter.** `setquest`, `completequest`, `erasequest`, `changequest`, `checkquest`, `isbegin_quest` and `showevent`.
46. Procedural content (phase 7 step 6b). The user approved these on 2026-10-17:
    - **Services.** Core ships service NPCs for procedural worlds, placed by tag: Kafra storage, shops stocked by band, a refiner, and an inn with the save point. They load only in procedural worlds.
    - **Quest templates.** Records in `data/quest_templates.yml` for hunt, collect and delivery quests. Each town's quests are chosen at world creation from its nearby fields' stored spawns, with the secret branch. A quest board NPC in each town offers them.
    - **Job guilds.** One per class family, placed by tag in towns. A job test is a chain of quest templates: hunt, collect, and a talk with the guild master. The trial dungeon of strategy section 11.4 waits for instanced maps.
    - **Rebirth.** An NPC in the largest town of the highest band.
    - **`wa.generation`.** Run at world creation on the server only. `generation.on("map", function(map, node) ... end)` receives a node with `child(label)`, `stream()` and `hash(x, y)`, and returns `{ places, objects, bosses }`. Layers cannot change terrain cells, which the client generates. `generation.quest_template(record)` adds a quest template. It needs a new permission, `world.generate`.

47. Rotation and world changes (phase 7 step 7). The user approved these on 2026-10-17:
    - **Rotating dungeons.** A dungeon style may set `rotation: { every: daily | weekly, at, weekday }`. Core marks one dungeon per world as rotating. Map IDs carry the date, such as `gen:dungeon:crypt:2026-10-31:2`, and the layout comes from the public node `dungeon(name, date)`. The next rotation is built before it starts. At the hour, the entrance portal switches to the new floors and the old floors are removed. Rotating floors have `nomemo`.
    - **Secrets for rotation.** `wactl world new` gives the game role one derived key per rotating dungeon, `child(secret, "rotation", name)`, and one for seasons, `child(secret, "season")`. They reveal only those maps' future spawns and loot. The threat model records the risk.
    - **World changes.** At the end of a season, a region's fields are generated again from `field(x, y, "season", n)` with a new biome. Map IDs and level bands stay. Off by default; `procedural.seasons.days` turns it on. Staff can trigger it.
    - **Removing or replacing a map**, as strategy section 8.10: warnings at 10, 5 and 1 minutes and 30 seconds; players still there move to the nearest town by the map graph; save and memo points on the map move to that town's save spot; portals to the map update in the same step.
    - **Lua.** World events `dungeon_rotated` with { `dungeon`, `date` } and `region_changed` with { `region`, `season` }. `wa.testing` gains `world:rotate(dungeon)` and `world:change_region(x, y)`.
    - **Admin.** The events tool lists rotations and seasons, and staff can force one with `events.control` and an audit row.

48. Encrypted backups (phase 8 step 2). The user approved these on 2026-10-18:
    - **Encryption.** Backups are encrypted with age (`filippo.io/age`). The server holds only the recipient, the public key. The owner keeps the identity, the private key, offline. Owners can also decrypt with the standard `age` command.
    - **Commands.** `wactl world backup-key new --out identity.txt` writes an identity and prints its recipient. `wactl world backup --recipient age1...` or `--recipients-file` encrypts the `pg_dump` stream, so no plain copy touches disk. A backup without a recipient is refused unless `--plain` is given. `wactl world restore --identity` decrypts and restores. Older plain backups still restore.
    - **Manifest.** Each backup has a plain `world-<UTC time>.json` next to it: release version, migration version, time, size, the SHA-256 of the encrypted file, and row counts of accounts, characters, item serials and the zeny total. It holds no secrets.
    - **Schedule.** `wactl world backup --dir /backups --every 24h --keep-daily 7 --keep-weekly 4` runs as a long-lived process, prunes old backups and logs each run. Compose gets a `backup` service with a backups volume and the recipient from a secret. The defaults are daily, keeping 7 daily and 4 weekly backups.
    - **Restore tests.** `wactl world verify-backup --in F --identity I` checks the hash against the manifest, decrypts, restores into a temporary database, checks the migrations are current or older, compares the row counts, and drops the temporary database. With `--every` it tests the latest backup on a schedule. By default it runs on the owner's machine against copied backups. An opt-in `backup-verify` Compose service can run it on the server, and the setup guide says that the server then holds the identity, so a stolen server can decrypt backups.
    - **Out of scope.** Copying backups to other machines (owners use `rclone` or `rsync`), an admin screen for backups, and point-in-time recovery.
    - **No Lua API.**

49. Retention jobs and the Compose config (phase 8 step 3). The user approved these on 2026-10-18:
    - **Config.** One `retention` block in `waserver.yml` with `login_ip_days` (90), `chat_days` (30), `item_log_days` (180) and `audit_days` (730). Each value is 1 to 3650 days. A value above the default of strategy section 7.10 is allowed, but logs a warning at start and shows one on the admin status page. `chat.log_days` still works, with a warning.
    - **Item and zeny logs.** A month partition is dropped once the whole month is older than the cutoff, so rows are kept at least 180 days and at most about 211. Rows in the default partition are deleted by date. The drop runs through a `SECURITY DEFINER` function, as `ensure_log_partitions` does, so the game role still cannot drop tables.
    - **Staff audit log.** Rows older than `audit_days` are deleted with their reviews. Rows that wait for a review are kept until they are reviewed.
    - **One daily job** in `waserver`, at start and every 24 hours. It logs the rows it deleted and exports metrics for the counts and the last run. The chat and login cleanups move into it.
    - **Privacy page.** It shows the periods in effect, drops "comes in a later version", and adds that backups keep deleted data for up to 4 weeks with the default schedule, and that item logs may be kept up to 31 days longer.
    - **Admin.** A read-only `GET /admin/api/v1/retention` lists the periods, the last run and the rows deleted. The status page shows it with any warnings.
    - **Compose config.** `maps` and `start_map` move into a `classic:` block of `waserver.yml` that procedural worlds ignore, so the default Compose config runs in both world modes.
    - **No Lua API.**

50. Load tests (phase 8 step 4). The user approved these on 2026-10-18:
    - **Setup.** Release binaries from `scripts/release.sh`, PostgreSQL 18 in Docker, and a procedural world from `wactl world new`, all on the development machine (an Apple M2 with 8 GB). Bots run on the same machine at `nice 15` with `GOMAXPROCS=4`. Classic mode gets one Town run on the kRO maps.
    - **Scenarios.** The five of phase 2 (Town, Crowd, Spread, Fight, Town with PostgreSQL saves), and two procedural ones: bots fighting on generated fields of several bands, and bots walking the generated map graph through portals.
    - **Steps.** 250, 500, 750, 1,000 bots and up, with 3 minutes of measurement each. The documented capacity is the highest count at which every scenario except Crowd meets section 3.8: tick p99 under 10 ms, a tick maximum of 25 ms with rare spikes reported, and under 256 KB per player. Crowd is a stress test. Its results are reported but do not set the capacity.
    - **Soak.** One 30-minute run at the documented count checks that memory and tick times stay flat.
    - **Results.** New tables in strategy section 3.8, the capacity and minimum hardware lines of section 4, and "Measure your own hardware" in the deploy guide. `wabots` ships in the release.
    - **Fixes.** Small slowdowns found are fixed in this step. Large ones are reported for a decision.
    - **No Lua API.**

51. Security review (phase 8 step 5). The user approved these on 2026-10-18:
    - **Capacity.** The docs keep 250 players, as measured in decision 0050. The send cost of many viewers in one zone moves to phase 10 item 5.
    - **Method.** The project reviews itself against docs/security/threat-model.md, in eight areas: protocol and zone, accounts, staff and admin, mods and the Lua sandbox, web, database rights, procedural secrets, and deployment and release. Each area gets its own read-only reviewer. A second pass proves or drops each finding, capped at 15 checking agents, with small findings of one area checked together. Reviewers and checkers run on Sonnet. Stage 2 starts only after the user checks the usage of stage 1.
    - **Tools.** The existing `gosec` and `govulncheck`, plus longer fuzz runs, `pnpm audit` and `trivy` on the images. All are free.
    - **Output.** docs/security/review-2026-10.md with each finding's severity, how to reproduce it, and its fix or accepted risk. The threat model's statuses are updated.
    - **Fixes.** Critical and high findings are fixed before the release, each with a test. Medium and low findings are fixed when small, or listed as known issues in the release notes.
    - **SECURITY.md** gains a supported versions table for the first release.
    - **No Lua API,** unless a finding needs a sandbox change, which goes to the user first.

52. Security fixes that add to the Lua API (phase 8 step 5). The user approved these on 2026-10-18:
    - **`data.core_override`.** A new permission. A data overlay that changes an economy field of a record another mod owns, such as an item's `buy` or `sell`, a monster's drops or exp, or a quest's drops, needs it. `wactl mod install` lists the core records such a mod changes.
    - **Hook `next()`.** A `next` function stops working once its chain call returns. Calling a kept `next` raises an error.
    - **Per-mod Lua budgets.** Each mod gets its own memory limit and a CPU budget per tick, on top of the per-call limits. A manifest may ask for a lower or higher budget within limits the server owner sets. A mod over its budget has its calls refused for the rest of the tick and logs a warning.
    - **Enforcement of existing permissions.** `player.status` is enforced. Whispers and announcements from mods are marked as coming from the mod, and reading private chat needs its permission. Mods reach only their own storages, unless a permission says otherwise.
    - All of these only add to API 1.0 or enforce rules it already states.

53. Details of the mod security fixes (phase 8 step 5). The user approved these on 2026-10-18:
    - **Private chat.** The `chat` event gives whispers, party and guild lines only to mods with `rules.override`, which the chat hook already needs. There is no separate permission.
    - **Budget names.** The manifest field `budget` with `memory_mb` and `cpu_per_tick`, and the config keys `mods.budget` and `mods.max_budget`. Defaults: 16 MB and 2,000,000 units per tick, with at most 64 MB and 10,000,000 units.
    - **Over budget.** Event handlers, hooks and timers of the mod are skipped for the rest of the tick, and timers wait for the next tick. Scripts that players trigger, such as item, skill and NPC scripts, still run.
    - **Sandbox output.** `tostring` shows `table: #N` instead of an address, and `string.format` refuses `%p`.
    - **Price rule.** An item record that breaks rAthena's buy and sell rule (buy / 124 below sell / 75) is a load error.
    - **Mod whispers.** They carry the mod's ID and show in the client as server lines with a `[Mod <id>]` prefix, with no reply tab.

54. Security review decisions (phase 8 step 5). The user approved these on 2026-10-19:
    - **`node:hash` on secret nodes** uses HMAC-SHA256 keyed by the node, so it cannot be inverted. Public nodes keep their hash, so the client and the server still agree. Generation runs once at world creation, so existing worlds keep their stored content. Only mods that hash secret nodes in new worlds see different values.
    - **`/gen/maps` stays public.** Map names, bands and places are visible in play and are public by decision 0044. Spawns and loot are not in it.
    - **Owners by account ID.** `wactl world new` records the first admin account's ID as the owner. `staff.owners` by name still works for existing worlds, with a startup warning.
    - **Passkeys for staff** count as a second factor only when the passkey verified its user (PIN or biometric).

55. Classic mode for the first release (phase 8 step 6). The user approved these on 2026-10-19:
    - **More NPC folders.** The extract adds rAthena's `quests`, `warps` and `events` folders. What converts loads, and what does not is refused and listed. Instances, guild castles and battlegrounds stay in phase 10.
    - **OnTouch.** An NPC record may set `touch: { x_span, y_span }`, the area around the NPC, as rAthena's `OnTouch` and `OnTouch_` areas. A player who steps into it fires the NPC's `touch` event with the player; a player who leaves it fires `untouch`. The converter maps `OnTouch` and `OnTouch_` labels to them. This adds to API 1.0.
    - **Bosses.** Spawns keep rAthena's boss flag and respawn variance, and MVPs are announced as rAthena does.
    - **Kafras** convert fully, including cart rental through the existing cart API.
    - **End-to-end check.** A full kRO asset build with `maps: all` and `mode: renewal`. A bot warps from Prontera to a field, fights spawns, uses a Kafra, buys from a shop and changes job at a converted NPC. A Chromium browser check covers the same world.
    - **Release notes** list what does not convert, generated from the converter's reports.
    - **Renewal only.** Classic ships with `mode: renewal`. Pre-renewal classic is marked experimental until `core.ruleset.pre_renewal` (phase 10 item 1).

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
18. Build review decision 32: rule reading, limit expansion, joins on create, the block list trade rule, and stable test helpers. Done on 2026-10-12 ([decision 0027](../decisions/0027-rule-reading-and-limits.md)). Details to review:
   - `rules.is` is true at a flag's renewal value or at `on`. `skill_rebalance.<skill>` takes the skills of `rules.RebalancedSkills` and the songs, and is the flag the skill follows, its own or the base. The base itself has no flag, so `checkre(0)` stays the converter's mode.
   - `rules.get` returns a boolean for a yes or no rule, else an integer. `rules.get_limit` returns two values, the base and the max.
   - An unknown flag, rule or limit raises an error that lists the valid names.
   - The limits `player:expand` takes are `inventory_slots`, `storage_slots` and `cart_slots`. `friends`, `block_list`, `mailbox`, the guild limits and extra storages are not expandable. The amount is -10,000 to 10,000.
   - `player:expand(limit, 0)` returns the value without a change, but needs `player.expand_limits` like any call. A read of the value without the permission needs API.
   - `player.expand_limits` needs the owner's approval and is not flagged.
   - `inventory_slots` saves in `characters.inventory_size`. `storage_slots` saves with the account's storage and `cart_slots` with the cart, in `storage_sizes`. A storage or cart at the base has no row. Each read clamps the saved value to the range, so a ruleset change applies at once.
   - The cart's size changes and saves while the player has no cart.
   - The client gets `PlayerLimits` at login and after each change. It sizes the inventory, the account's storage and the cart from it.
   - rAthena's inventory expansion vouchers (25791, 25792 and 25793) work through a client window, not item scripts, and the reference fork has no `expandinventory` or `getinventorysize` command. None converts. A window for the vouchers needs review.
   - The converter turns `basicskillcheck` into `rules.get("basic_skill_check")`, and `checkre(1)` to `checkre(5)` into `rules.is` with `cast_time`, `level_penalty_drop`, `level_penalty_exp`, `level_damage_mod` and `aspd`. `getbattleflag` does not convert: its names are conf settings that are not ruleset rules.
   - `party_join` and `guild_join` with `"create"` come after the client's created result.
   - `block_list_refuses_trade` is yes by default. No matches rAthena.
   - A runtime without a ruleset, such as the converter's test runtimes, reads rAthena's renewal rules.
   - The `wa.testing` helpers of phase 5 are stable. The definitions keep the `Provisional` field for later API.
19. Build review decision 33: staff roles, the admin API, the GM audit log, the first admin screens, `player:get_staff_role()` and `player:has_staff_permission(name)`. Done on 2026-10-12 ([decision 0028](../decisions/0028-staff-roles-and-audit.md)). Details to review:
   - The permission list has step 1's `staff.assign` and `audit.read`, and names for steps 2 to 4 that do nothing yet: `audit.review`, `players.read`, `items.history`, `items.restore`, `moderation.reports`, `moderation.mute`, `moderation.kick`, `moderation.jail`, `moderation.ban`, `commands.warp`, `commands.hide`, `commands.item`, `world.read`, `world.announce`, `world.restart`, `events.control`, `mods.read` and `mods.disable`.
   - The default roles: `admin` has every group, `game_master` has `audit.read`, `players.read`, `items.history`, `items.restore`, `moderation.*`, `commands.*`, `world.read`, `world.announce` and `events.control`, and `moderator` has `players.read`, `moderation.reports`, `moderation.mute`, `moderation.kick` and `moderation.jail`.
   - A `staff.roles` list replaces the default roles. It does not add to them.
   - `*` alone is not a pattern. Only `group.*` is.
   - A caller gives or takes only roles whose permissions it holds all of. No one changes their own role or an owner's. With the default roles, admins can make other admins.
   - A recovery code counts as a second factor for the two-factor rule, as a TOTP code does.
   - Giving the role an account already has changes nothing and writes no audit row. A reason is optional for role changes.
   - The account role reads the audit log for the admin API, and adds rows. It cannot change or delete them. The game role only adds rows.
   - `GET /me` needs a session and no permission. It returns the role the account has even when it does not count, so the screens can say two-factor login is missing.
   - The admin API's JSON uses protobuf's lowerCamelCase names, and int64 values are strings, as protobuf's JSON form has them.
   - `GET /audit` filters by the staff account's name, the action, the target, `since` and `until` dates, and `pending` or `reviewed`. Pages go by `before_id`, 50 rows by default and 100 at most.
   - The admin screens are at `/admin` of the server, not inside the game page. The game page has no link to them.
   - `player:has_staff_permission` takes a full name only, and raises an error for an unknown one.
   - A game session keeps the two-factor answer of its ticket. Turning TOTP off during play applies at the next login.
   - `wa.testing` gained `world:set_staff_role(player, role)`, provisional, for the default roles. The owner role is not offered there.
   - `wactl world new --admin` still marks its account with `accounts.staff`, which gives no role. The owner names that account in `staff.owners`. Whether to drop the column, or make that account an owner, needs review.
20. Build review decision 34: player search, player details, item history, the character logs, restores, audit reviews and their admin screens. No Lua API changes. Done on 2026-10-13 ([decision 0029](../decisions/0029-player-search-and-item-history.md)). Details to review:
   - Login records are a new table, `account_logins`, kept 90 days, since sessions are deleted when they expire or go idle. The migration copies the sessions of the last 90 days into it.
   - `GET /players` pages by account ID with an opaque `cursor`, 20 accounts by default and 50 at most. A name search matches a prefix of an account name or of a character name. An ID search matches an account ID or a character ID. An IP search takes one address or a CIDR range of any size.
   - The search shows the logins of an IP search, at most 20 per account. It shows no IP address otherwise, not even with `players.read_ip`.
   - `GET /characters/{id}/logs` lists the lines of the character's inventory, cart and vending shop, not those of its account's storage, which other characters of the account share.
   - The item history follows ancestors back and descendants forward, and not the ancestors of descendants. It stops at 100 serials or 2,000 lines.
   - A restore's mail comes from "Staff", with the title "Restored items", and puts each serial in a slot of its own, so no stack merges. It ignores the mailbox limit, as a script's mail does, and has no sender, so it is deleted after `mail_delete_days` and never goes back.
   - A copy of a gone serial takes the item, refine, cards, rolls, options and bond of the serial's last log line, and the line's amount unless the request names one.
   - Zeny in a restore is always made, so it needs `items.create`, even with `items.restore`.
   - A restore of items held by players on two maps is refused. The staff member restores them in separate calls.
   - A restore from a storage or a guild storage that someone has open waits and tries again, up to 8 times, then answers `conflict`.
   - The holder's trade closes, a worn item comes off, and a vending shop loses the restored stack's lines, or closes with none left.
   - A review needs a row marked for review, and a flag needs a note. A review needs no recent login. Reviews write an `audit.review` row.
   - The default `game_master` no longer has `items.restore`, as review decision 34 lists. Configs that list it keep it.
   - With `--dev` and a database, `POST /debug/wathena/restore` runs restores for the duplication tests.
21. Build review decision 35: the report queue, sanctions with account bans, GM commands and their admin screens. No Lua API changes. Done on 2026-10-13 ([decision 0030](../decisions/0030-moderation-and-gm-commands.md)). Details to review:
   - The catalog drops `commands.item`, since `@item` needs `items.create`. A config that lists `commands.item` no longer starts. It gains `commands.recall` and `moderation.ban_permanent`.
   - The default `game_master` lists its five moderation permissions in place of `moderation.*`, so it does not get `moderation.ban_permanent`.
   - Kicks and bans are rows of `sanctions`, with mutes and jails. A permanent ban ends at `'infinity'`. A kick is a row that ends when it starts, for the history.
   - `GET /sanctions` needs any permission of the moderation group. `?character=` lists the character's sanctions and its account's bans.
   - Mutes and jails last 1 to 525,600 minutes, and bans 1 to 5,256,000 minutes or 0 for permanent. Lifting a ban needs `moderation.ban` and a recent login, not `moderation.ban_permanent`.
   - No one sanctions their own account or lifts its sanctions.
   - The chat around a report is the lines the reporter saw, and the target's own lines from 5 minutes before to 5 minutes after the report. The detail shows the target's 20 newest earlier reports by name and 20 newest sanctions.
   - A sanctioned report links a sanction of the reported character or its account. Linking is checked when the report closes.
   - A ban closes the account's game connections and refuses logins and tickets. It does not end the account's web sessions. The login page shows the ban only after the right password.
   - Every line that starts with `@` is a command, in any chat tab. A player's `@hello` answers "Unknown command." and is not said. rAthena sends such a line as chat for players without a group level.
   - Commands run before the mute check, so a muted staff member can use them.
   - `@kick` reads the longest run of words that names an online player as the name. `@mute` and `@jail` read the words before the minutes. Double quotes work for both.
   - `@mute`, `@jail` and `@kick` need the game's database, since characters without one have no ID. `@item` takes an ID or an Aegis name, and makes at most 100 of an item that does not stack.
   - `@hide` hides from other staff too, as rAthena. It ends at logout, where rAthena keeps it. A hidden staff member's local chat does not show to others, where rAthena shows it.
   - A dead or jailed staff member cannot `@warp` or `@jumpto`, and a dead or jailed player cannot be recalled.
   - Events a player has not seen when it changes maps now go out before the map change, so a command's answer arrives.
   - With `--dev`, `POST /debug/wathena/staff` gives a role to an online character, and `POST /debug/wathena/sanction` gives a sanction, for the end-to-end tests.
22. Build review decision 36: the world view, announcements, planned restarts, the events tool, the mods tool, their admin screens, and the world event `restart_planned`. Done on 2026-10-13 ([decision 0031](../decisions/0031-world-events-and-mods-tools.md)). Details to review:
   - `restart_planned` runs in mod, NPC and event contexts, in every zone. `at` is in Unix seconds. A cancel sends `{ cancelled = true }` alone. A restart of the server before the time drops the plan without a cancel event.
   - `wa.testing` has no way to plan a restart, so a mod test cannot fire `restart_planned`. A test helper, such as `world:plan_restart(minutes, reason)`, needs review.
   - A mod that staff turn off is paused, not ended: its contexts, handlers, timers and hooks stay and run again once it is on, and its Lua values stay. Timers that come due meanwhile are skipped. Monsters it spawned stay with their AI contexts, bonuses it gave stay, and its world event runs go on in the scheduler. Conversations with its NPCs close at once.
   - A mod that is off when a zone starts runs no entry script or NPC setup there until it is turned on. Equipment worn while it is off gives its script's bonuses after it is worn again, and a monster spawned meanwhile has no AI script until it spawns again.
   - A usable item whose script belongs to a mod that is off is refused, and not used up.
   - Core's ruleset mods cannot be turned off either, as core cannot.
   - Turning a mod on in one zone while it is off everywhere answers `conflict`. Off everywhere replaces its zone rows.
   - The signature status is `builtin`, `folder`, `unsigned`, or `unverified` for a `.zip` with a `.minisig` file next to it. The server has no trusted keys, so it checks no signature.
   - A loaded mod has no pending permissions, since the loader refuses to start with one. Mods off in `mods.disable` do not show.
   - Error counts start at zero at each start, and keep the last error's last 500 bytes.
   - Staff may start an event that is off. Turning an event off lets a run that lasts go on until its end. `world_events.start` and `.stop` still see only the events that are on.
   - A change keeps the fields of the change before it. A reset of an event without a change answers `conflict`. The latest 5 results of each event are kept in memory, since the start.
   - The planned restart shows in `GET /world`. There is no `GET /restart`.
   - A restart is planned 1 minute to 7 days ahead. The message is optional and the announcements say the reason without one. A cancel is announced. When the clock passes several marks at once, only the last one is announced.
   - New logins in the last minute get `DISCONNECT_REASON_RESTART` before their ticket is used, and `POST /account/ticket` answers 503 `restarting`. The new reason is 11.
   - At the time, waserver exits with status 75 (`EX_TEMPFAIL`), and skips the shutdown countdown.
   - The world view lists every zone, sleeping ones too, and at most 1,000 players. CPU is measured since the start or the metrics' last reset. Reading it writes no audit row.
   - Announcements go out in the default announcement color. Restart announcements are yellow, as the shutdown countdown's.
   - With `--dev`, `POST /debug/wathena/announce` and `POST` and `DELETE /debug/wathena/restart` serve the end-to-end tests.
23. Build review decision 37: the account page's block lists, reports and sanctions, the data export, account deletion and the privacy notice. No Lua API changes. Done on 2026-10-14 ([decision 0032](../decisions/0032-account-page.md)). Details to review:
   - The pages are `/account/safety` (block lists, reports, sanctions), `/account/data` (the export and deletion) and `/account/privacy`, with a menu on every account page.
   - The export is refused while a character of the account plays or is on autotrade, and reads offline characters from the database. The refusal does not use up the day's export.
   - The export's `email` is always `null`, since the account service stores no email. Mod variables are not in it, since review decision 37 does not list them.
   - A log in the export holds at most 100,000 lines, the oldest, and names itself in `truncated`.
   - The export's format is `wathena-account-export` version 1. A field that goes, changes name or changes meaning raises the version. New fields keep it.
   - "Once a day" is 24 hours after the last export, not a calendar day.
   - The typed account name is compared in any case.
   - A cancel needs no recent login.
   - An account that `staff.owners` names cannot ask for deletion, since a new account of the same name would become an owner.
   - A character that becomes a guild master with members during the 14 days hands the guild to the member in the lowest slot when the job runs, as `guildchangegm`. A master alone breaks its guild up, and the guild storage's items go with it.
   - A party the deleted character led breaks up, as the char server does when a leader leaves.
   - Mails from the deleted characters stay, with the sender's name and no sender ID, so they never return. Mails to them go with their items.
   - Sanctions go with the account. The staff audit log keeps a row of each.
   - Reports keep the character IDs and names of the reporter and the target. The deleted character's reported lines go with its chat lines.
   - The item and zeny log lines of the account's storages name storage 0 in place of the account.
   - The job runs at start and every hour, on the server clock, at most 100 accounts a run. An account with a character that has a session waits for the next run, and the character is disconnected.
   - An autotrade store saved before a restart does not open while its account is set to be deleted.
   - The new disconnect reason is `DISCONNECT_REASON_ACCOUNT_DELETION`, 12.
   - The account page's changes have their own rate limit, `Limits.Account`: 10 at once, then 1 every 6 seconds per address.
   - With `--dev` and a database, `POST /debug/wathena/deletions` runs the deletion job for the end-to-end tests.
24. Build review decision 38: mod art sources and builds, the server's checks and serving of mods' builds, the client data with each record's art, the art fields of records, mods' client data tables, and the client's loading of mods' builds. No Lua API changes. Done on 2026-10-14 ([decision 0033](../decisions/0033-mod-assets.md)). Details to review:
   - Names under `mod/<mod id>/` keep upper case after the namespace, as `mod/example.lantern/icon/item/Paper_Lantern` in review decision 38 does. assets-v1 gained the rule without a new contract version, since readers never checked names. A pack in a server's own build keeps lower case names.
   - A field value with a `/` is an asset name. The values the fields took before still work: a monster's or NPC's sprite name, a job's or skill's name, a core look.
   - A mod's own record without a field uses the art named after it, when its build has it: `icon/item/<name>`, `image/item/<name>` and `sprite/item/<name>` for items, `sprite/body/<name>/<m or f>` for jobs, and `icon/skill/<name>` for skills. Monsters and NPCs name their sprite, since the field is required.
   - A new monster whose `sprite` is art starts from a Poring's stats. A job's `look` with art is a table with `job`, the job a new job starts from, and `m` and `f`. A companion's or mount's `look` with art needs `acts_as`, whose option bit shows it. Monster `sounds` keys are `attack`, `damage`, `die` and `move`, and skill `effects` keys `caster`, `effect` and `hit`.
   - NPC sprites of art get sprite IDs from 1,000,000 at each start, in the order of the names, sent as `Look.body`.
   - The server tells the client the art at `GET /client-data.json`, format version 1, which the client loads before it connects. The protocol does not change, and Welcome has no new field. The tables are in it only when a mod has any.
   - The size limit counts a build's distinct files before compression. `mods.max_asset_mb` sets it, 200 by default. A broken build stops the start. A mod with `art/` and no build starts with placeholders and a warning.
   - A mod's build may name only its own art. A mod changes how core's records look with their fields, not by replacing core's names.
   - `wactl mod pack` builds the art with a fixed build time and stores the built files in the `.zip` without compression. A build in the mod's folder is not packed.
   - Mod tables are `client/jobs.yml`, `weapons.yml`, `effects.yml` and `skills.yml`. An entry replaces core's whole entry with the same key: `look` for looks, `id` for the rest. A table is at most 4 MiB, and YAML aliases are refused.
   - `wactl mod validate` checks names under `mod/` against the sources in `art/`, or the built `assets/` of a `.zip` file. Names of core's art are not checked. `wactl assets check --mod` checks a mod's build with the server's rules.
   - Pack sprites: an indexed PNG's index 0 is transparent, a sprite without a frame file is one still frame in 8 directions, and the frame file's limits are 4,096 frames, 64 actions and 256 frames per direction.
   - glTF models must fit the profile already. Ogg and MP3 sounds and pack map folders are rejected for now. Loose RO maps build, but zones do not run mods' maps yet, and the map fields of review decision 38, music and minimap, have no record yet.
   - The client draws mod item icons, dropped items, monster and NPC sprites, and job bodies. Skill icons and effects, monster sounds, view sprites and companions of art are sent, and the client does not draw them yet. Mod items show as `Item <id>`, since the client data has no item names.
25. Build review decision 39: touch input, with gestures by pointer type, the joystick, on-screen controls, small screen panels, long-press item drags, browser behavior, phone layouts of the account pages and admin screens, and the checks. No Lua API or protocol changes. Done on 2026-10-15 ([decision 0034](../decisions/0034-touch-input.md)). Details to review:
   - A pen acts as a mouse. Only a finger has the gestures and the joystick.
   - A long press on a monster, an NPC or an item does nothing, as a right click there does nothing. While a skill waits for its target, a long press anywhere cancels it, as a right click does.
   - A press on the ground starts a hold, which walks toward the finger, as moving the finger does.
   - Two fingers: the distance zooms, a twist and a sideways move turn the camera, and an upward move tilts it.
   - The joystick aims 4 cells ahead, and closer, or 45 degrees to a side, when a wall is in the way. A path more than 2 steps longer than the distance counts as blocked. Walks go out at most every 250 ms and never twice to one cell. A release sends one walk to the cell one step past the next cell, so the player stops within two cells.
   - The joystick shows at 128 px in the lower left corner, and the option is saved in the browser, not on the account.
   - The window buttons fold into a Menu button below 600 px wide. The Options button and window are new, with one setting.
   - Below 600 px wide, panels open under the top bar and above the joystick and hotkey bar. While a trade, storage, cart, mail or store setup is open, the inventory is a bottom sheet over the joystick. A panel's bar moves it up and down only, and the sheet does not move.
   - Every window has a title bar with its name, also with a mouse on a desktop. A press on a window brings it to the front.
   - A long press lifted in place does the right click of the row, such as storing or offering the item. Two taps within 450 ms do a double click.
   - The chat of touch screens opens with the Chat button over the game view, with a Send button. Send on an empty line, or the focus leaving the chat, closes it. While closed, it shows its latest lines and lets touches through.
   - Text selection is off in touch mode on the whole page except text fields, not only in the game view.
   - The equipment window and the hotkey bar take no item drops, as with the mouse. An item is equipped by a double tap.
   - The dev server's `WATHENA_GAME` forwards `/ws`, `/client-data.json` and `/emblems`, and `WATHENA_HTTPS_CERT` and `WATHENA_HTTPS_KEY` serve HTTPS, since `crypto.subtle` needs a secure page.
   - `check:browsers` runs the touch checks in Chromium only, through the Chrome DevTools Protocol, and prints frame times without failing on them.
26. Build review decision 40: mod text and art in the client with `/client-data.json` format version 2, the two moderation changes of step 3, and the phase 6 exit check. No Lua API changes. Done on 2026-10-15 ([decision 0035](../decisions/0035-phase-6-exit-check.md)). Details to review:
   - Item records gain `description`, text of up to 4,096 bytes with RO's color codes. Only the client reads it.
   - Version 2 sends names for the records a mod adds, and for core records whose `display_name` a mod set. Items also send their card slots, for labels such as "Lantern Shield [1]".
   - A job's body sprites moved under `body`, and a mod's job has `look`, the job it looks like. The version changed for this move.
   - Looks sent a mod's job as the job it looks like, so step 6's job bodies never showed. A look now sends a mod's job, and a job whose record names body sprites, by its own ID. Rules such as ASPD still read the look job.
   - Looks carry view IDs, not items, so an item with a `view_sprite` gets a view of its own from 1,000,000 at each start, in item ID order, as NPC sprites of art do. Headgear and garments draw it. Weapons and shields keep their type's sprites.
   - Companions and mounts of art are not drawn: a look carries only the option bit of the core record they act as. A protocol field would let the client draw them.
   - The client shows the name of a unit under the mouse, so monsters' names show in the page, and a window that opens comes to the front, which a shop opened from the store search on a phone needs.
   - A staff role counts for `@` commands only when it has the permission of at least one command. A staff member who logged in without a second factor says `@` lines as chat.
   - The exit check runs two servers on one database, since `--dev` has no account pages and accounts have no `--start-job`. `tools/testdb` makes and drops the database.
   - The check's phone renders at one device pixel per CSS pixel to keep SwiftShader fast. The touch check keeps the full ratio.
   - The e2e fixture gains the Training Dummy, worth a million exp, placed by `!trainer`, and "Guild storage" at the Kafra. The Lantern Ghost no longer loots, so drops stay for players.
   - A second job learns its own skills only once the first job's points are spent, as rAthena's `player_skillup_limit`, so the check spends the Merchant's points before Lantern Flare shows.
27. Build review decision 41: the six changes of the phase 6 review, with one Lua API change, `world:plan_restart` in `wa.testing`. Done on 2026-10-15 ([decision 0036](../decisions/0036-phase-6-review-changes.md)). Details to review:
   - Migration 00028 drops `accounts.staff`. `wactl world new` prints, after its line, a `staff:` block with `owners: [<name>]` to add to `waserver.yml`. The account has no role until the config names it.
   - `@hide` is saved as rAthena's `OPTION_INVISIBLE` bit, 0x40, in `characters.options`. A login comes back hidden only while its role has `commands.hide` and it used a second factor, as `pc_authok` checks `pc_can_use_command`. Any other login comes back visible, and its next save clears the bit. A hidden login gets no message and writes no audit row, as in rAthena.
   - A hidden staff member's local chat reaches every player within 9 cells, other staff too, with `from` 0 and the speaker's name. Block lists still hide it. The client shows the line in the log without a bubble.
   - A ban ends the account's sessions on the account pages, which the admin screens share, deletes its logins that wait for a code and its unused game tickets, in one transaction of the account role, after it closes the game connections. A failure is logged, and the ban stays. With `--dev` there are no account pages, so nothing more ends. Lifting a ban gives nothing back.
   - `world:plan_restart(minutes, reason)` is provisional. The minutes are 1 to 10,080 and the reason 1 to 255 characters, as the admin screens allow. `at` is the test world's time plus the minutes. It runs the handlers in the world's one zone and steps the world twice, so the lines they send go out, as `world:chat` does. It makes no announcements and has no cancel, and a world without a map yet starts on `prontera`, as `world:advance` does.
   - The export has `variables` at the top for the account and in each character, each `{ mod, name, value }` with the value as the mod saved it, sorted by mod and name. A variable at its default is not saved, so it is not in the file. Variables of a mod that was removed are still in it. The no-secrets test now checks field names only, since a variable's name is data.
   - Looks gain `companion` and `mount`, the record IDs, sent for every record, with or without art, and `weapon_view`, the right hand's view of art. A left hand weapon shows no art. Shields already sent their view, so their art draws by it.
   - A player on a mount of art shows its job's body on foot: the server leaves the mount's option bits out of the body job, and the client too. The mount's sprite is drawn at the player's cell behind the body, with idle and walk actions. A companion of art takes the falcon's place beside the player. A look of art still needs `acts_as`, as review decision 38 says.
   - The lantern sample adds the Lantern Shield, one card slot, sold by the Lamplighter, and the Lantern Wisp, a companion that acts as a falcon without `requires`. The Lantern Master gives it to a Lantern Keeper, and a second click sends it home. The exit check counts the blue the shield adds on Ann, and the glow the wisp adds in a 120 pixel box, since the wisp sits half a cell to the side.
28. Build review decision 42: dungeons, towns and named places (phase 7 step 3), with `wa.places`, NPC records with `place`, dungeon styles and town templates as records, and the dev world. Done on 2026-10-16 ([decision 0040](../decisions/0040-dungeons-towns-places.md), [generator-v3](../specs/generator-v3.md)). Details to review:
   - `places.find` and `places.nearest` need `game.read`, which every mod has. A bad scope or tag raises an error. A tag is 1 to 64 lowercase letters, digits, `_`, `-`, `.` and `:`.
   - A place has several tags, its main tag first: a gate is `gate` and `gate:south`, a portal `portal` and `portal:<side>`. A building's tags are one place each, on the street at its door, 2 cells apart.
   - `find` returns places by map ID, then in the generator's order. `nearest` counts warps on the graph of the server's warps, and on another map the distance from the arrival cell of the first warp on the shortest way. It leaves out maps no warp reaches, and returns nil, not an error, when nothing qualifies.
   - A place handle is cached per place, so two finds of one place give the same handle and `==` holds. Its fields are read-only, as for every handle.
   - A town's ID is `t<n>` and its map ID `gen:town:t<n>`, so `get_area()` and the scope `town:t7` name it alike. Strategy section 8.10 wrote `gen:town:7`.
   - A dungeon's ID is its style's name without the mod prefix until the map graph names dungeons. Two dungeons of one style need a new ID scheme then.
   - An NPC record with `place` takes no `map`, `x` or `y`. It stands at every match with its record's other fields, and its shop is one shop. Without a match, or in a classic world, it does not appear, and the server logs it at info level.
   - Records: `data/dungeon_styles.yml` with `method`, `floors`, `floor_size`, `room_size`, `tileset` and `level_step`, and `data/town_templates.yml` with `size`, `gates`, `walls`, `tileset`, `buildings` (`model`, `footprint`, `count`, `places`) and `decorations`. `room_size` may be left out for caves. A model has one footprint in every template, and may not be named as a prop of its tileset.
   - A dungeon floor's level band is server data in the map's JSON, not in Lua.
   - Test worlds of a `wactl` built with `-tags procedural` hold a field, a town per template and a dungeon per style, from a fixed secret, and `world:new_player` accepts their map IDs. No `wa.testing` call changed. A case finds its town with `wa.places`.
   - Review items: a `wa.testing` way to pick a test world's template, mods' own tilesets and props, a floor's level band in Lua, and whether an NPC placed by tag without a match is a warning.
29. Build review decision 43: the world layout (phase 7 step 4), with `wa.towns`, the handle `wa.Town`, `player:has_visited`, `maps.get_display_name` and `place:get_band`, naming styles as records, home maps in the map folder format, the teleport service, `@go`, `@renametown` and the world map. Done on 2026-10-16 ([decision 0041](../decisions/0041-world-layout.md), [generator-v3](../specs/generator-v3.md), "World layout", [map-folder-v1](../specs/map-folder-v1.md)). Details to review:
   - Every call needs `game.read`. A town handle is cached per town, so two handles of one town are equal, and its fields are read-only.
   - `towns.list()` returns towns by number. Its only option is `visited_by`, a player; another option raises an error. `towns.nearest(player)` counts warps on the layout's graph, ties to the lower number, and returns nil when no town is reached. `towns.get(id)` takes `t<n>` only: a map ID or a display name raises an error, and an unknown town returns nil.
   - `town:get_band()` and `place:get_band()` return the lowest and highest level. A place of a map without a band raises an error; in a layout every map has one.
   - `player:has_visited(town)` takes a town handle, not a map ID. A player has visited a map once it entered it, also through a warp or at login; the set is saved.
   - `maps.get_display_name(map)` takes a map ID and returns nil for any map outside the layout, so classic maps have none until locale files exist.
   - Names change with `@renametown`: `get_name` and `get_display_name` read the current name.
   - In `wactl mod test`, the test world's layout is the dev layout: its home area is the field, so its towns' bands start above level 1, and it has no home map.
   - Naming styles are records of `data/naming_styles.yml` with `biome`, `syllables` and `patterns`. The last record of a biome, in load order, is its style. Records are read when a world is made, not at each start.
   - Home maps are map folders under a mod's `maps/<id>/`, tagged `home`, with a `portal` place and an optional `start` place. Their map ID is `gen:home:<id>`.
   - The teleport service is the server's: no Lua API reads graph distances or prices, and NPC records cannot set a service.
   - Review items: a Lua way to read graph distances or teleport prices, a record field for server services on NPCs, a `wa.testing` way to start on a mod's home map, the band formula in small worlds, longer town names for `@renametown`, and entered maps and memo points in the data export.
30. Build review decision 44: spawns and level bands (phase 7 step 5), with the hook `spawn_table`, the permission `spawns.modify`, monster records with `spawn`, spawn themes as records, and the bands of 8 levels per warp. Done on 2026-10-16 ([decision 0042](../decisions/0042-spawns.md), [generator-v3](../specs/generator-v3.md), "World layout" and "Spawns"). Details to review:
   - The hook's function is `function(map, spawns, next)`, and `next(spawns)` takes the list alone and returns the rest of the chain's list. Left out, it runs on the hook's own list. The innermost list is the one the world drew.
   - `map` is read-only: `id`, `kind` (`"field"` or `"dungeon"`), `biome`, `band` as `{ lowest, highest }`, and `features` (`water`, `clearing`, `room`, `boss_room`). A floor's biome is its style's tileset.
   - Each spawn is `{ monster, count, respawn, role, area }`: `monster` from `monsters.id`, `count` 1 to 1,000, `respawn` in milliseconds from 1,000, and `area` `{ x, y, w, h }`, within `w` and `h` cells of `x` and `y`, or nil for the whole map. A list holds at most 64. The hook gets a copy, so changing it changes only what it passes on or returns.
   - A list with a bad entry, or an unknown monster, fails the hook, which the chain leaves out. The server keeps its own rules over the result: the clear cells around arrivals and warps to towns follow each line's monster and role, and a boss as `common` or `uncommon` is left out with a warning.
   - `spawn_table` is a mod context's hook. It runs only when `wactl world new` makes a procedural world, in a Lua state that ran the mods' entry scripts on no map, and in the test worlds of `wactl mod test` when a case first runs on a generated field or floor. `hooks.on("spawn_table")` without `spawns.modify` raises `hooks.on("spawn_table") needs the permission spawns.modify`.
   - `spawns.modify` needs the owner's approval and is not flagged.
   - A monster record's `spawn` replaces the tags field by field. A role left out is `uncommon`, or `mini_boss` or `field_boss` for a boss. A boss as `common` or `uncommon` is an error. Biome and theme names are 1 to 64 lowercase letters, digits and `_`, and weights 1 to 1,000.
   - Spawn themes are records of `data/spawn_themes.yml` with `biomes`, `races`, `elements` and `boost` (100 to 1,000, 300 by default). A monster names a theme in `spawn.themes` with or without the mod prefix. A map draws a theme with a chance of 1 in 3.
   - `procedural.layout.band_step` is 1 to `band_width`. The coverage check counts the bands of `band_width` levels from 1 to 99, and a grid with fewer than 3 fields per such band skips it.
   - Review items: a way for the hook to read a monster's level, race, element or tags; a Lua way to read a map's stored spawns; a dungeon style field for its spawn biome; the theme chance as a setting; and whether `spawns.modify` should be flagged.
31. Build review decision 45: the quest system (phase 7 step 6a), with quests as records of `data/quests.yml`, the player's quest calls, `npc:show_quest_icon`, the events `quest_add`, `quest_progress` and `quest_complete`, the quest window, and the converter's quest commands. Done on 2026-10-17 ([decision 0043](../decisions/0043-quests.md)). Details to review:
   - A quest argument is an ID, such as `7104`, or a record name: a full name, or a plain name, the calling mod's record first, then core's. Core's records are named by their rAthena IDs as text, such as `"7104"`, since `quest_db.yml` has no other name. An unknown quest raises an error.
   - The changes return a boolean: false when rAthena logs an error and changes nothing. `complete_quest` takes an active or an inactive quest. `remove_quest` takes a quest in any state.
   - `get_quest_progress(quest, objective)` takes the objective from 1 and returns two values, the kills and the goal. It returns nil, nil without the quest or the objective. An objective below 1 raises an error.
   - `quest_timed_out` is false for a quest without a time limit. rAthena's `PLAYTIME` counts such a quest as timed out, since its end time is 0; the converter keeps rAthena's numbers through `R.checkquest`, which knows each quest's limit.
   - The reads need `game.read`. The changes need no permission, also on another mod's quest or a core quest.
   - The event objects are `{ player, quest, id }`, where `quest` is the record's full name; `quest_progress` adds `objective`, `count` and `goal`, once per objective a kill raised. The events run in mod, NPC and event contexts of the player's map, without options. `quest_add` also fires for the new quest of `change_quest`. Nothing fires for `remove_quest` or for the client's active toggle.
   - The icon names are rAthena's `QTYPE_` values 0 to 6 and 8: `quest`, `quest_question`, `job`, `job_question`, `event`, `event_question`, `warg` and `warg_face`. `QTYPE_DAILYQUEST` (7) shows nothing at PACKETVER 20211103, so it has no name. An icon is per player and per map, and comes again when the NPC comes into view.
   - The decision's item objectives are rAthena's quest `Drops`: items each kill gives while the quest is in the log, straight to the inventory. A drop that does not fit is lost, and the player is told.
   - Record fields: `title`, `time_limit` (a duration, a number of seconds, or `{ at, days }` or `{ at, weekday }`), `targets` (at most 3, with `monster` and `count`, or `count` with `min_level`, `max_level`, `race`, `size`, `element`, `map`, `map_name` and `monsters`), and `drops` (`monster`, `item`, `count`, `rate`). rAthena's element filter, map filter and monster list are kept, beside the level, race and size filters the decision names.
   - A party member counts a kill when it is on the map within 14 cells of the monster, dead or not, as `quest_update_objective_sub`. A killer with a party that stands farther away does not count it, as in rAthena.
   - A log entry whose record is gone, such as a removed mod's quest, stays saved and hidden, and comes back with the mod. rAthena drops such entries at login.
   - Review items: a Lua way to list a player's quests or to read a quest record (title, objectives, limit); a minimap mark for `showevent`'s color; `questinfo` and `setquestinfo`, which 32 core scripts use and which do not convert; `open_quest_ui`; options for the quest events; and quest calls for another character.
32. Build review decision 46: procedural content (phase 7 step 6b), with core's service NPCs placed by tag, quest templates as records of `data/quest_templates.yml`, quest boards, job guilds, the rebirth NPC, `wa.generation` and the permission `world.generate`. Done on 2026-10-17 ([decision 0044](../decisions/0044-procedural-content.md)). Details to review:
   - `generation.on(event, layer)` takes the event `"map"` alone; another raises an error. It needs a mod context, and returns nothing: a layer cannot be removed. Layers run in the mods' load order, each mod's in the order they came, once on every generated field, town and dungeon floor, in the layout's order. A mod's home map gets none.
   - `map` is read-only: `id`, `kind` (`"field"`, `"town"` or `"dungeon"`), `biome` (a field's biome, or a town's or floor's tileset), `tileset`, `band` as `{ lowest, highest }`, `width`, `height`, and `places`, each `{ tags, x, y }`. It has no cells.
   - The node is `child(secret, "generation").child(mod).child(map ID)`. `node:child(label)` takes 1 to 64 bytes. `node:stream()` returns a stream with `integer(minimum, maximum)` and `chance(numerator, denominator)` as `wa.random`'s, from the node's ChaCha8 stream; two streams of one node give the same numbers. `node:hash(x, y)` is the node's hash of the cell, all 64 bits as a Lua integer. Nodes and streams work only while their layer runs. `wa.random` raises an error while a layer runs.
   - A layer returns nil or `{ places, objects, bosses }`, each optional: at most 32 places `{ tags, x, y }` with 1 to 8 tags of `wa.places`'s characters; at most 256 objects `{ model, x, y, turn }`, a prop of the map's tileset, 0 to 3 quarter turns; at most 8 bosses `{ monster, role, x, y, respawn }`, a monster ID as `mini_boss` or `field_boss`, with both or neither of x and y, and `respawn` in milliseconds, 45 or 90 minutes by default. Coordinates are 0 to 1,024. A bad shape fails the layer.
   - The server's rules: a place moves to the nearest open ground within 8 cells, or is left out; an object must be a prop of the tileset and on the map; a boss must spawn, stands within 3 cells of the nearest open ground to its cell, spawns on fields and floors only, joins the list after the `spawn_table` hooks while it has fewer than 64 lines, and keeps 30 cells from warps to towns. What breaks a rule is left out with a note, which `wactl world new` prints.
   - `generation.quest_template(record)` takes a table with `name` and the fields of `data/quest_templates.yml`; the name gets the mod's prefix, and a name added twice raises an error. A bad record raises an error. It works in an entry script, while the file loads, and in a layer. Templates join after the records, so a world draws from both.
   - In a running zone both calls record what they get, and nothing runs it. In `wactl mod test`, the layers run once when the test world is made, and the test world's quests come from its spawn lists before the hooks.
   - `world.generate` needs the owner's approval and is not flagged.
   - Review items: a public exp reward call, a Lua way to read a world's quests, player fields for skill points and weight, a layer's read of map cells, a places scope of fields, objects of a mod's own art, board quest limits and repeats, and whether `world.generate` should be flagged.
33. Build review decision 47: rotation and world changes (phase 7 step 7), with the world events `dungeon_rotated` and `region_changed`, and `world:rotate(dungeon)` and `world:change_region(x, y)` in `wa.testing`. Done on 2026-10-17 ([decision 0045](../decisions/0045-rotation.md), [generator-v3](../specs/generator-v3.md), "Rotations and seasons"). Details to review:
   - `dungeon_rotated` gets `{ dungeon, date }`: the dungeon's ID, such as `crypt`, and the date of its new floors, such as `"2026-10-31"`, as text. `region_changed` gets `{ region, season }`: the region's town ID, such as `"t2"`, and its new season, an integer from 1. The objects are read-only.
   - Both run in mod, NPC and event contexts of every zone, without options, once per change, after the players and the points of the old maps moved and the new maps run. They run for scheduled changes, for staff's forced ones, and for `wa.testing`'s. They do not run for the changes a start makes for a time the server was stopped, since no zone ran then, nor for a world's first date.
   - A region is the fields whose nearest town by warps is one town, ties to the lower number, named by the town's ID, as the field bosses' regions of decision 0042. A town with no field has no region.
   - A rotating floor's map ID is `gen:dungeon:<id>:<date>:<floor>`. `places.find("dungeon:<id>", tag)` finds dated floors, and `get_area()` reads `dungeon:<id>:<date>:<floor>`.
   - `world:rotate(dungeon)` takes the dungeon's ID in the test world, its style's name without the mod prefix, and returns the new date. The date is that of the style's next rotation after the world's time, or after the date it rotated to before, so two calls give two days of a daily style. A style without a rotation, a dungeon the test world lacks, or a case that runs on one of the dungeon's floors raises an error. The new floors get spawns from the test world's rotation key.
   - `world:change_region(x, y)` takes a field's grid coordinates, `0, 0` in the test world, and returns two values: the region's town ID and its new season. A field the test world lacks, or a case that runs on one of the region's fields, raises an error.
   - Both are provisional. They change the test world's own copy of the generated maps, so a change stays in its case. They give the world its default map first when the case has none yet, as `world:plan_restart` does.
   - Test worlds do not rotate by schedule, and their floors keep the stored IDs until `world:rotate`.
   - Review items: the `spawn_table` hooks and generation layers on rotating floors and changed fields, a Lua way to read a rotating dungeon's date or a region's season, forced changes that last across a restart and a cancel for them, new level bands with a world change, more than one rotating dungeon per world, and locale text and a banner for the warnings.
34. Build review decision 48: encrypted backups (phase 8 step 2), with `wactl world backup-key new`, `wactl world backup` with age recipients, manifests and `--every`, `wactl world verify-backup`, `wactl world restore --identity`, and the Compose services `backup` and `backup-verify`. No Lua API changes. Done on 2026-10-18 ([decision 0048](../decisions/0048-encrypted-backups.md)). Details to review:
   - `filippo.io/age` v1.3.2. Recipients are what age's parser takes: X25519 (`age1...`) and post-quantum hybrid (`age1pq1...`). `backup-key new` makes X25519 keys in `age-keygen`'s format.
   - Names use the UTC time as `20261018T030000Z`: `world-<time>.dump.age`, `.dump` with `--plain`, and `world-<time>.json`. `--out FILE` puts the manifest at the name without `.dump.age` or `.dump`, with `.json`. `--out -` writes none.
   - The manifest's counts: rows of `accounts` and `characters`, item serials as the rows of `inventory`, `storage_items`, `guild_storage` and `mail_items`, and zeny as that of `characters` and `mails`. `format: 1` names these definitions. The counts and `pg_dump --snapshot` share one exported snapshot, so they agree on a live server.
   - Pruning keeps the newest backup of each of the newest `--keep-daily` UTC days that have one, and of each of the newest `--keep-weekly` ISO weeks (Monday to Sunday, UTC) that have one, as `restic forget`. The weekly backup of a past week is its Sunday backup with daily backups. With the defaults, a folder holds 10 backups.
   - The first scheduled backup is due one interval after the newest in the folder. A failed one is tried again after an hour. `--keep-daily` is at least 1. Without `--every`, pruning runs only when a keep flag is given.
   - `verify-backup` uses its own role with only `CREATEDB` (`wathena_verify` in Compose, from `WATHENA_VERIFY_DATABASE_URL`), not `CREATEDB` on the migrator. It restores with `--no-owner --no-acl`, so the world's roles need not exist where it runs. It needs `createdb` and `dropdb`. The `verify-init` service makes the role, only with the verify profile.
   - `secrets.sh` never makes the backup key. It makes `backup_recipients` with comments. Without a recipient, the Compose backup service runs with `--wait-for-recipient`: it makes no backup, logs a warning at start and every hour, and `up --wait` passes. It reads the recipient at start, so the owner recreates it after adding one. The command line without the flag still refuses.
   - Each scheduled run removes hidden temporary files of wactl's pattern, `.world-<time>.<extension>.<random>.tmp`, older than 24 hours.
   - Every `wactl` command reads the embedded reference data, so a release `wactl world new` makes a procedural world outside the repository. The Compose smoke test makes a procedural world again.
   - Review items: skipping a backup the scheduled verify already checked, and a time of day for the schedule.
35. Build review decision 49: retention jobs and the Compose config (phase 8 step 3), with the `retention` block of `waserver.yml`, one daily retention job in `waserver`, the functions `drop_old_log_partitions` and `delete_old_staff_audit`, `GET /admin/api/v1/retention`, the periods on the privacy page, and the `classic:` block for `maps` and `start_map`. No Lua API changes. Done on 2026-10-18 ([decision 0049](../decisions/0049-retention.md)). Details to review:
   - An explicit 0 in the `retention` block is an error, as any value outside 1 to 3650. A key left out takes the default. `chat.log_days` with the same value as `retention.chat_days` is allowed, with the warning.
   - The job takes the session advisory lock `(1 << 61) | 1` on a connection of the game pool for the whole run, so the deletes can run in batches. Bit 61 marks the server's jobs, as bit 62 marks account storages. No other daily job had a guard against two processes.
   - Each kind runs even when another fails. A run without errors counts as the last success. The last run is kept in `retention_status`. The counts since the start are per process.
   - A month partition is one named `<log>_YYYY_MM`, as `ensure_log_partitions` names them. It is dropped when its month ends at or before the cutoff, in UTC. The rows are counted before the drop. A drop waits at most 10 seconds for a lock.
   - Both functions refuse a cutoff later than 30 days ago (migration 00037). The game role can now drop log data older than 30 days through its function.
   - A flagged review counts as a review, so an old flagged row goes.
   - `GET /retention` needs `world.read`, the World screen's permission. It writes no audit row.
   - The metrics are `wathena_retention_deleted_rows_total{kind}` and `wathena_retention_last_success_timestamp_seconds`, at `/debug/wathena/metrics/prometheus` on the debug address. The server's other measurements stay JSON.
   - `--map` and `--maps` win over the `classic` block in both modes. A procedural world still reads the deprecated top-level keys.
   - The privacy page shows 730 days as "2 years", and its backups row says "Up to 4 weeks".
   - Later changes, approved on 2026-10-18 (decision 0049, "Later changes"):
     - Migration 00037 sets the floor of `drop_old_log_partitions` and `delete_old_staff_audit` to 30 days. `retention.item_log_days` and `retention.audit_days` are 30 to 3650 days. `login_ip_days` and `chat_days` stay 1 to 3650.
     - The admin IP search and the data export look back `login_ip_days`, not a fixed 90 days.
     - The table `retention_status` keeps the last run, its counts by kind and its errors. The job writes it as the game role. `GET /retention` reads it, so it shows a run of another process, and a run from before a restart.
   - Review items: whether `world.read` is the right permission for `GET /retention`.
36. Build review decision 50: load tests (phase 8 step 4), with the scenarios `fields` and `graph` in `tools/botclient`, bot placement through the database and routes to portals in `wabots`, `wabots` in the release, the results in strategy section 3.8, and "Measure your own hardware" in the deploy guide. No Lua API changes. Done on 2026-10-18 ([decision 0050](../decisions/0050-load-tests.md)). Details to review:
   - The phase 2 scenarios run in the procedural world's start town and its lowest field. Town uses new characters each step, which start at the start cell. Town with PostgreSQL uses the characters of an earlier run, spread over the town. Every procedural run loads and saves characters in PostgreSQL.
   - `--place fields` puts each bot on one of `--fields` fields (8 by default), spread from the lowest band to the highest, as the economy simulator's model player of its band, with 50 potions, and its save point on its field. `--place towns` spreads the bots over the towns as the model player of the highest band. The update raises `save_version`.
   - `wabots` reads generated maps from `/gen/maps/` and the world's links from `world_links`, and needs `--database` or `WATHENA_DATABASE_URL` for the procedural scenarios. The game role's rights are enough.
   - A travelling bot walks a breadth-first route to a random portal more than 12 cells away, in legs of up to 24 cells, and picks another after three walks without moving.
   - The p99 budget counts as met when fewer than 1% of a zone's ticks are over 10 ms, since the histogram rounds p99 up to its bucket.
   - The documented capacity is 250: every run of every scenario but Crowd met the budgets there. At 500, 3 of 10 runs had 1.06% to 1.17% of their ticks over 10 ms, while the zone thread's CPU time p99 stayed under 6 ms.
   - The classic run used Prontera's map files from the kRO client, since the full asset build did not fit on the disk.
   - The zone fix: a sleeping zone clears its block index, a player that leaves its zone resets its IDs for what it saw, and the tick clears its hits and removals before it cuts them.
   - Review items: whether to document 500 players, the slowdown of 1,000 players in one town, and a way to place bots without a database URL.
37. Build review decisions 51 to 53: the security review (phase 8 step 5), with the fixes of its findings, connection limits per address, and the Lua API changes of decision 52. Done on 2026-10-18 ([review 2026-10](../security/review-2026-10.md)). Details to review:
   - Eight areas, each with its own read-only reviewer on Sonnet, then checking agents that proved the high and medium findings, most with a failing test. 6 high, 19 medium and 39 low findings, some found twice. Commits 88149d2, 9fd99ca and eaf4077 fixed every high and medium finding but one, and a follow-up change fixed most low ones.
   - Decisions 52 and 53 are built as approved: `data.core_override`, hook `next()` that stops working, per-mod budgets with `budget` and `mods.budget`, `player.status` enforced, mod whispers marked with `from_mod`, private chat only for `rules.override`, `tostring` without addresses, and the price rule. No other Lua API change was made.
   - The gateway limits each address to 32 game connections, and 20 new ones at once, then one every 3 seconds, which limits `Hello` too. The address comes through `trusted_proxies`, through the new package `server/clientaddr`, which the account service uses too. Loopback is not limited. `connections.*` in `waserver.yml` changes the numbers.
   - The debug address answers only a loopback `Host` and no request from another page. `--dev` refuses `trusted_proxies` and a public URL on another host.
   - The e2e bots led to two rate changes: guild skill ups count as actions, and only a store search query counts as a search.
   - Known issues for the release notes: a passkey without user verification counts as a second factor, a stale session can commit after a second login, `/gen/maps` lists every map's places and names, `node:hash` on a secret node can be inverted, the account role can make characters with any values, backups are not signed, database traffic in Compose is not encrypted, secret files are mode 644 in their 0700 folder, and `release.sh` does not check its tools.
   - Review items:
     - A Lua API change for `node:hash` on secret nodes: an HMAC-SHA256 hash, or no `node:hash` in generation layers. Either changes the output of mods that use it.
     - Whether `/gen/maps` should list places and names only for maps a player visited, which changes the protocol and the bots.
     - Owners by account ID or a claim code instead of by name.
     - A session factor for a passkey that verified its user, which needs a migration.
   - Review decision 54 settled these review items on 2026-10-19, and they are built:
     - `node:hash` on a generation layer's node is rng-v1's new secret hash, HMAC-SHA256 keyed by the node (`Node.SecretHash`). Public nodes keep their hash. TinyGo builds leave the secret hash out, so `gen.wasm` is byte-identical and no client golden changed. No other Lua API change was made.
     - `/gen/maps` stays public, as an accepted risk by design.
     - Migration 00040 adds `world.owner_account_id`, which `wactl world new` fills with the first account's ID. The owner role follows it. `staff.owners` by name still works, with a warning at start.
     - Migration 00041 adds the session factor `passkey_verified`. A staff role counts only for a session with `totp`, `recovery_code` or `passkey_verified`.
     - Three known issues are gone from the release notes: the passkey without user verification, `/gen/maps`, which is now an accepted risk, and `node:hash` on a secret node.
     - Review items: a reauth with a passkey without user verification still refreshes the 10-minute window on an account without TOTP, as a password does. `staff.owners` by name is not yet removed.
