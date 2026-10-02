# Lua API reference

Generated from [server/modscript/luaapi](../../server/modscript/luaapi) with `go test ./server/modscript/luaapi -update`. Do not edit it by hand.

- The API is [proposal 0002](../proposals/0002-lua-api.md). It is experimental until API 1.0.
- Tests check these definitions against the runtime, so this page lists what the server has today.
- Editors get the same definitions from `wactl mod types` ([mods/README.md](../../mods/README.md), "Tools").
- The constants are in [lua-constants.md](lua-constants.md).

## Modules

| Module | Gives |
|---|---|
| [`wa.context`](#wacontext) | The current instance, kind and mod. Calls outside a setup function or a handler raise an error. |
| [`wa.events`](#waevents) | Event handlers. The context owns them, and removes them when it ends. |
| [`wa.hooks`](#wahooks) | Hooks change a game result in a chain ordered by priority. Each hook calls next to get the rest of the chain's result. The innermost result is rAthena's value. Hooks only compute: calls that change the game raise an error in them. The context owns them. |
| [`wa.schedules`](#waschedules) | Schedules on the server clock, in the server's time zone (time_zone in the config). A time that does not exist on a day clocks change runs at the next valid minute, and a time that happens twice runs once. A time that passed while the zone slept or the server was stopped runs once, or not at all. The context owns them. |
| [`wa.spawning`](#waspawning) | Monsters that the context owns. They go when it ends. |
| [`wa.timers`](#watimers) | Timers on the zone clock. The context owns them, and cancels them when it ends. |
| [`wa.items`](#waitems) | Items by name. |
| [`wa.skills`](#waskills) | Skills by name. |
| [`wa.monsters`](#wamonsters) | Monsters by name. |
| [`wa.text`](#watext) | Text for players, from the mod's locales/en.yml. Call the module with a key: text "greeter.hello". |
| [`wa.maps`](#wamaps) | Maps. |
| [`wa.world`](#waworld) | The world. |
| [`wa.variables`](#wavariables) | Saved variables, declared at the top of a file with a scope and a type. They are stored under the mod's ID. |
| [`wa.random`](#warandom) | Random numbers from the zone's secret stream. |
| [`wa.integers`](#waintegers) | Integer division that rounds as C does, toward zero, for scripts converted from rAthena. |
| [`wa.bonuses`](#wabonuses) | Item bonuses (proposal 0002a). Equip contexts only. |
| [`wa.races`](#waraces) | Monster races, from rAthena's RC_ names. |
| [`wa.monster_groups`](#wamonster_groups) | Monster groups, from rAthena's RC2_ names. |
| [`wa.elements`](#waelements) | Elements, from rAthena's ELE_ names. |
| [`wa.sizes`](#wasizes) | Sizes, from rAthena's Size_ names. |
| [`wa.monster_classes`](#wamonster_classes) | Monster classes, from rAthena's CLASS_ names. |
| [`wa.weapon_types`](#waweapon_types) | Weapon types, from rAthena's W_ names. |
| [`wa.statuses`](#wastatuses) | Status effects, from rAthena's SC_ names. |
| [`wa.effects`](#waeffects) | Effects for player:show_effect, from rAthena's EF_ names. |
| [`wa.headgear_effects`](#waheadgear_effects) | Effects for player:show_headgear_effect, from rAthena's HAT_EF_ names. |
| [`wa.emotions`](#waemotions) | Emotions for npc:show_emotion, from rAthena's ET_ names. |
| [`wa.jobs`](#wajobs) | Jobs, from rAthena's JOB_ names, and job tests. |
| [`wa.testing`](#watesting) | Test cases. It exists only under wactl mod test, never in a running server. |

### wa.context

```lua
local context = require "wa.context"
```

The current instance, kind and mod. Calls outside a setup function or a handler raise an error.

#### `context:get_instance() → wa.ModInstance|wa.Npc|wa.EquippedItem|wa.ItemUse|wa.EventInstance`

The instance the context owns: the mod, the NPC, the equipped copy, the used item, or the event run.

#### `context:get_kind() → wa.ContextKind`

The context's kind.

#### `context:get_mod() → string`

The ID of the context's mod.

### wa.events

```lua
local events = require "wa.events"
```

Event handlers. The context owns them, and removes them when it ends.

#### `events.on(name, handler, options?) → wa.EventHandle`

Registers a handler for an event.

Permission: `event.listen`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `wa.EventName` | The event's name. An unknown name, or an event of another context kind, raises an error. |
| `handler` | `function` | Runs each time the event fires, in registration order. |
| `options` | `wa.DeathOptions?` | Only the death event takes options: they narrow which deaths run the handler. |

### wa.hooks

```lua
local hooks = require "wa.hooks"
```

Hooks change a game result in a chain ordered by priority. Each hook calls next to get the rest of the chain's result. The innermost result is rAthena's value. Hooks only compute: calls that change the game raise an error in them. The context owns them.

#### `hooks.on(name, hook?, options?) → wa.HookHandle`

Adds a hook. Higher priority runs first; equal priorities run in the order they were added.

Permission: `rules.override`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `wa.HookName` | The hook's name. An unknown name raises an error. |
| `hook` | `fun(object: wa.Drop\|wa.Exp, next: fun(object?: table): integer, integer?): integer, integer?` | Gets the object and next. drop_rate returns a rate per 10,000; experience returns base and job exp. |
| `options` | `wa.HookOptions?` |  |

### wa.schedules

```lua
local schedules = require "wa.schedules"
```

Schedules on the server clock, in the server's time zone (time_zone in the config). A time that does not exist on a day clocks change runs at the next valid minute, and a time that happens twice runs once. A time that passed while the zone slept or the server was stopped runs once, or not at all. The context owns them.

#### `schedules.daily(time, handler) → wa.Schedule`

Runs the handler every day at the time.

| Parameter | Type | Meaning |
|---|---|---|
| `time` | `string` | HH:MM, such as "20:00". |
| `handler` | `fun()` |  |

#### `schedules.weekly(day, time, handler) → wa.Schedule`

Runs the handler every week on the day at the time.

| Parameter | Type | Meaning |
|---|---|---|
| `day` | `wa.Weekday` |  |
| `time` | `string` | HH:MM. |
| `handler` | `fun()` |  |

#### `schedules.hourly(minute, handler) → wa.Schedule`

Runs the handler every hour at the minute.

| Parameter | Type | Meaning |
|---|---|---|
| `minute` | `integer` | 0 to 59. |
| `handler` | `fun()` |  |

#### `schedules.when(condition, handler, options?) → wa.Schedule`

Runs the handler once the condition has held for lasting_minutes. The condition runs once a minute, at the start of each minute of the server clock. The handler runs once for each time the condition holds.

| Parameter | Type | Meaning |
|---|---|---|
| `condition` | `fun(): boolean` | Reads the game and returns true or false. Calls that change the game raise an error in it. |
| `handler` | `fun()` |  |
| `options` | `wa.WhenOptions?` |  |

### wa.spawning

```lua
local spawning = require "wa.spawning"
```

Monsters that the context owns. They go when it ends.

#### `spawning.monsters(spec) → wa.Monster[]`

Places monsters on the context's map and returns their handles. Without x and y, each lands on a random walkable cell.

Permission: `npc.create`.

| Parameter | Type | Meaning |
|---|---|---|
| `spec` | `wa.SpawnSpec` |  |

### wa.timers

```lua
local timers = require "wa.timers"
```

Timers on the zone clock. The context owns them, and cancels them when it ends.

#### `timers.after(milliseconds, callback) → wa.Timer`

Runs the callback once, after the time.

| Parameter | Type | Meaning |
|---|---|---|
| `milliseconds` | `integer` | 0 or more. |
| `callback` | `fun()` |  |

#### `timers.every(milliseconds, callback) → wa.Timer`

Runs the callback every time the interval passes. A timer that falls behind skips the runs it missed.

| Parameter | Type | Meaning |
|---|---|---|
| `milliseconds` | `integer` | Above 0. |
| `callback` | `fun()` |  |

#### `timers.with_bonuses(milliseconds, add, options?) → wa.Timer`

Runs the function now. The bonuses it adds end after the time. Item contexts only.

| Parameter | Type | Meaning |
|---|---|---|
| `milliseconds` | `integer` |  |
| `add` | `fun()` | Adds bonuses with bonuses.add. |
| `options` | `wa.WithBonusesOptions?` |  |

### wa.items

```lua
local items = require "wa.items"
```

Items by name.

#### `items.id(name) → integer`

The item's ID. An unknown name raises an error.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | An AegisName, such as Red_Potion, or a mod item's full name. |

#### `items.group(name) → integer`

An item group, for player:give_item_from_group.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | The group's name in item_group_db, such as BLUEBOX. |

#### `items.name(item) → string`

The item's name from rAthena's item database.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |

### wa.skills

```lua
local skills = require "wa.skills"
```

Skills by name.

#### `skills.id(name) → integer`

The skill's ID. An unknown name raises an error.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A skill_db name, such as SM_BASH. |

### wa.monsters

```lua
local monsters = require "wa.monsters"
```

Monsters by name.

#### `monsters.id(name) → integer`

The monster's ID. An unknown name raises an error.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | An AegisName, such as PORING. |

### wa.text

```lua
local text = require "wa.text"
```

Text for players, from the mod's locales/en.yml. Call the module with a key: text "greeter.hello".

#### `text(key) → string`

The text of a key. A missing key raises an error.

#### `text.item_link(item) → string`

A link that conversation:say shows as the item's name, as mesitemlink.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |

### wa.maps

```lua
local maps = require "wa.maps"
```

Maps.

#### `maps.announce(map, text)`

Shows a line to every player of a map, as mapannounce.

| Parameter | Type | Meaning |
|---|---|---|
| `map` | `string` | A map name, such as prontera. |
| `text` | `string` | At most 255 bytes. |

### wa.world

```lua
local world = require "wa.world"
```

The world.

#### `world.announce(text)`

Shows a line to every player of the world, as announce.

Permission: `world.announce`.

| Parameter | Type | Meaning |
|---|---|---|
| `text` | `string` | At most 255 bytes. |

### wa.variables

```lua
local variables = require "wa.variables"
```

Saved variables, declared at the top of a file with a scope and a type. They are stored under the mod's ID.

#### `variables.character_integer(name) → wa.IntegerVariable`

Declares a integer variable. Saved with the character.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.character_boolean(name) → wa.BooleanVariable`

Declares a boolean variable. Saved with the character.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.character_string(name) → wa.StringVariable`

Declares a string variable. Saved with the character.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.character_table(name) → wa.TableVariable`

Declares a table variable. Saved with the character.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.character_temporary_integer(name) → wa.IntegerVariable`

Declares a integer variable. Kept until the character logs out.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.character_temporary_boolean(name) → wa.BooleanVariable`

Declares a boolean variable. Kept until the character logs out.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.character_temporary_string(name) → wa.StringVariable`

Declares a string variable. Kept until the character logs out.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.character_temporary_table(name) → wa.TableVariable`

Declares a table variable. Kept until the character logs out.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.account_integer(name) → wa.IntegerVariable`

Declares a integer variable. Shared by the account's characters.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.account_boolean(name) → wa.BooleanVariable`

Declares a boolean variable. Shared by the account's characters.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.account_string(name) → wa.StringVariable`

Declares a string variable. Shared by the account's characters.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.account_table(name) → wa.TableVariable`

Declares a table variable. Shared by the account's characters.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.server_integer(name) → wa.ServerIntegerVariable`

Declares a integer variable. One value for the whole world, saved soon after a change.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.server_boolean(name) → wa.ServerBooleanVariable`

Declares a boolean variable. One value for the whole world, saved soon after a change.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.server_string(name) → wa.ServerStringVariable`

Declares a string variable. One value for the whole world, saved soon after a change.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.server_table(name) → wa.ServerTableVariable`

Declares a table variable. One value for the whole world, saved soon after a change.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.instance_integer(name) → wa.IntegerVariable`

Declares a integer variable. Raises an error until instances exist.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.instance_boolean(name) → wa.BooleanVariable`

Declares a boolean variable. Raises an error until instances exist.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.instance_string(name) → wa.StringVariable`

Declares a string variable. Raises an error until instances exist.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

#### `variables.instance_table(name) → wa.TableVariable`

Declares a table variable. Raises an error until instances exist.

Permission: `storage.mod`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 64 letters, digits and _, not starting with a digit. |

### wa.random

```lua
local random = require "wa.random"
```

Random numbers from the zone's secret stream.

#### `random.integer(minimum, maximum) → integer`

A whole number from minimum to maximum.

| Parameter | Type | Meaning |
|---|---|---|
| `minimum` | `integer` |  |
| `maximum` | `integer` |  |

#### `random.chance(numerator, denominator) → boolean`

True with a chance of numerator in denominator.

| Parameter | Type | Meaning |
|---|---|---|
| `numerator` | `integer` |  |
| `denominator` | `integer` | Above 0. |

### wa.integers

```lua
local integers = require "wa.integers"
```

Integer division that rounds as C does, toward zero, for scripts converted from rAthena.

#### `integers.divide_truncated(a, b) → integer`

a / b, rounded toward zero.

| Parameter | Type | Meaning |
|---|---|---|
| `a` | `integer` |  |
| `b` | `integer` | Not 0. |

#### `integers.remainder_truncated(a, b) → integer`

The remainder of divide_truncated.

| Parameter | Type | Meaning |
|---|---|---|
| `a` | `integer` |  |
| `b` | `integer` | Not 0. |

### wa.bonuses

```lua
local bonuses = require "wa.bonuses"
```

Item bonuses (proposal 0002a). Equip contexts only.

#### `bonuses.add(name, ...) → wa.BonusHandle`

Adds a bonus to the equipped copy. It ends when the copy is unequipped.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `wa.BonusName` | The bonus name. It is checked when the call runs. |
| `...` | `integer\|(fun(): integer)\|wa.BonusOptions` | What the bonus applies to, the amount, other numbers, then the options table. A number can be a function: a computed value. |

### wa.races

```lua
local races = require "wa.races"
```

Monster races, from rAthena's RC_ names.

Constants: [lua-constants.md](lua-constants.md#waraces).

### wa.monster_groups

```lua
local monster_groups = require "wa.monster_groups"
```

Monster groups, from rAthena's RC2_ names.

Constants: [lua-constants.md](lua-constants.md#wamonster_groups).

### wa.elements

```lua
local elements = require "wa.elements"
```

Elements, from rAthena's ELE_ names.

Constants: [lua-constants.md](lua-constants.md#waelements).

### wa.sizes

```lua
local sizes = require "wa.sizes"
```

Sizes, from rAthena's Size_ names.

Constants: [lua-constants.md](lua-constants.md#wasizes).

### wa.monster_classes

```lua
local monster_classes = require "wa.monster_classes"
```

Monster classes, from rAthena's CLASS_ names.

Constants: [lua-constants.md](lua-constants.md#wamonster_classes).

### wa.weapon_types

```lua
local weapon_types = require "wa.weapon_types"
```

Weapon types, from rAthena's W_ names.

Constants: [lua-constants.md](lua-constants.md#waweapon_types).

### wa.statuses

```lua
local statuses = require "wa.statuses"
```

Status effects, from rAthena's SC_ names.

Constants: [lua-constants.md](lua-constants.md#wastatuses).

### wa.effects

```lua
local effects = require "wa.effects"
```

Effects for player:show_effect, from rAthena's EF_ names.

Constants: [lua-constants.md](lua-constants.md#waeffects).

### wa.headgear_effects

```lua
local headgear_effects = require "wa.headgear_effects"
```

Effects for player:show_headgear_effect, from rAthena's HAT_EF_ names.

Constants: [lua-constants.md](lua-constants.md#waheadgear_effects).

### wa.emotions

```lua
local emotions = require "wa.emotions"
```

Emotions for npc:show_emotion, from rAthena's ET_ names.

Constants: [lua-constants.md](lua-constants.md#waemotions).

### wa.jobs

```lua
local jobs = require "wa.jobs"
```

Jobs, from rAthena's JOB_ names, and job tests.

Constants: [lua-constants.md](lua-constants.md#wajobs).

#### `jobs.is_transcendent(job) → boolean`

Whether the job is a transcendent job.

| Parameter | Type | Meaning |
|---|---|---|
| `job` | `integer` | A job, such as player.job. |

#### `jobs.is_baby(job) → boolean`

Whether the job is a baby job.

| Parameter | Type | Meaning |
|---|---|---|
| `job` | `integer` |  |

#### `jobs.is_third(job) → boolean`

Whether the job is a third job.

| Parameter | Type | Meaning |
|---|---|---|
| `job` | `integer` |  |

#### `jobs.is_fourth(job) → boolean`

Whether the job is a fourth job.

| Parameter | Type | Meaning |
|---|---|---|
| `job` | `integer` |  |

#### `jobs.base_job(job) → integer`

rAthena's BaseJob: a Lord Knight is a Knight.

| Parameter | Type | Meaning |
|---|---|---|
| `job` | `integer` |  |

#### `jobs.first_job(job) → integer`

rAthena's BaseClass: a Knight is a Swordman.

| Parameter | Type | Meaning |
|---|---|---|
| `job` | `integer` |  |

### wa.testing

```lua
local testing = require "wa.testing"
```

Test cases. It exists only under wactl mod test, never in a running server.

It exists only in tests that `wactl mod test` runs.

#### `testing.case(name, run)`

Registers a case. Call it inside the function that the test file returns.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | Unique in the file. |
| `run` | `fun(world: wa.TestWorld)` | Runs in a fresh world: the mod and its dependencies loaded, a fixed random seed, and no database. |

#### `testing.expect(value) → wa.Expectation`

Checks a value. A failed check fails the case at the line that called it.

| Parameter | Type | Meaning |
|---|---|---|
| `value` | `any` |  |

## Handles and tables

Handles are read-only. Writing a field raises an error. Changes are method calls.

### wa.Player

A player. Each access looks the player up again. A handle to a player who left the map raises an error. Reading needs game.read.

| Field | Type | Meaning |
|---|---|---|
| `kind` | `"player"` | The entity's kind. |
| `name` | `string` | The character's name. |
| `job` | `integer` | The job, such as jobs.knight. |
| `job_class` | `integer` | rAthena's eaclass(): the job's class flags. |
| `base_level` | `integer` |  |
| `job_level` | `integer` |  |
| `zeny` | `integer` |  |
| `hp` | `integer` |  |
| `max_hp` | `integer` |  |
| `sp` | `integer` |  |
| `max_sp` | `integer` |  |
| `map` | `string` | The map's name, such as prontera. |
| `x` | `integer` |  |
| `y` | `integer` |  |
| `character_id` | `integer` | The database ID, 0 without a database. |
| `account_id` | `integer` | The database ID, 0 without a database. |
| `party` | `nil` | nil: parties do not exist yet. |
| `sex` | `"male"\|"female"` |  |
| `base_str` | `integer` | Allocated points. |
| `base_agi` | `integer` | Allocated points. |
| `base_vit` | `integer` | Allocated points. |
| `base_int` | `integer` | Allocated points. |
| `base_dex` | `integer` | Allocated points. |
| `base_luk` | `integer` | Allocated points. |
| `base_pow` | `integer` | Allocated points. 0 until trait stats exist. |
| `base_sta` | `integer` | Allocated points. 0 until trait stats exist. |
| `base_wis` | `integer` | Allocated points. 0 until trait stats exist. |
| `base_spl` | `integer` | Allocated points. 0 until trait stats exist. |
| `base_con` | `integer` | Allocated points. 0 until trait stats exist. |
| `base_crt` | `integer` | Allocated points. 0 until trait stats exist. |

#### `player:send_message(text)`

Shows a line in the player's chat log, as CHAT_CHANNEL_SYSTEM.

| Parameter | Type | Meaning |
|---|---|---|
| `text` | `string` | 1 to 255 bytes. |

#### `player:give_item(item, amount?, options?) → boolean`

Gives items, as getitem. Returns false when they do not fit.

Permission: `player.give_items`.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |
| `amount` | `integer?` | 1 to 30000. 1 by default. |
| `options` | `wa.GiveItemOptions?` |  |

#### `player:give_item_from_group(group)`

Gives one use of an item group, as getgroupitem.

Permission: `player.give_items`.

| Parameter | Type | Meaning |
|---|---|---|
| `group` | `integer` | A group from items.group. |

#### `player:count_item(item) → integer`

How many of the item the player has, as countitem.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |

#### `player:take_item(item, amount) → boolean`

Takes items, as delitem. Returns false, and takes nothing, when the player has too few.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |
| `amount` | `integer` | 1 or more. |

#### `player:can_carry(item, amount) → boolean`

Whether the items fit, by weight and slots, as checkweight.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |
| `amount` | `integer` |  |

#### `player:pay_zeny(amount) → boolean`

Takes zeny. Returns false, and takes nothing, when the player has too little.

| Parameter | Type | Meaning |
|---|---|---|
| `amount` | `integer` |  |

#### `player:give_zeny(amount)`

Gives zeny. Zeny over 2,147,483,647 is lost.

Permission: `player.give_zeny`.

| Parameter | Type | Meaning |
|---|---|---|
| `amount` | `integer` |  |

#### `player:item_heal(heal)`

Heals as itemheal, with its bonuses from VIT, INT and skills.

| Parameter | Type | Meaning |
|---|---|---|
| `heal` | `wa.HealSpec` |  |

#### `player:heal_percent(heal)`

Heals as percentheal.

| Parameter | Type | Meaning |
|---|---|---|
| `heal` | `wa.HealSpec` | Amounts in percent. |

#### `player:heal_fully()`

Heals all HP and SP.

#### `player:warp(map, x, y) → boolean`

Moves the player after the event. Returns false when the map does not run.

Permission: `player.warp`.

| Parameter | Type | Meaning |
|---|---|---|
| `map` | `string` |  |
| `x` | `integer` |  |
| `y` | `integer` |  |

#### `player:warp_to_random_cell() → boolean`

A Fly Wing: a random walkable cell of the map.

Permission: `player.warp`.

#### `player:warp_to_save_point() → boolean`

A Butterfly Wing: the save point.

Permission: `player.warp`.

#### `player:set_save_point(map, x, y) → boolean`

Sets the save point, as savepoint. Returns false when the map does not run or the cell is not walkable.

Permission: `player.warp`.

| Parameter | Type | Meaning |
|---|---|---|
| `map` | `string` |  |
| `x` | `integer` |  |
| `y` | `integer` |  |

#### `player:show_effect(effect)`

Plays an effect on the player for everyone near, as specialeffect2.

| Parameter | Type | Meaning |
|---|---|---|
| `effect` | `integer` | An effect from wa.effects. |

#### `player:show_headgear_effect(effect)`

rAthena's hateffect. It shows nothing yet.

| Parameter | Type | Meaning |
|---|---|---|
| `effect` | `integer` | An effect from wa.headgear_effects. |

#### `player:get_skill_level(skill) → integer`

The level of a skill the player has, or 0, as getskilllv.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `skill` | `integer` | A skill from skills.id. |

#### `player:get_equipped(slot) → wa.EquippedItem?`

The copy in a slot, or nil.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `slot` | `wa.EquipSlot` |  |

#### `player:start_status(status, options?)`

Raises an error until status effects exist (phase 4).

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` | A status from wa.statuses. |
| `options` | `table?` |  |

#### `player:add_timed_bonuses(key, milliseconds, bonuses, options?)`

Raises an error until timed bonuses exist (phase 4).

| Parameter | Type | Meaning |
|---|---|---|
| `key` | `string` |  |
| `milliseconds` | `integer` |  |
| `bonuses` | `table` |  |
| `options` | `table?` |  |

#### `player:mark_minimap(id, x, y, color)`

Marks a cell on the minimap, as viewpoint. A mark with the same ID replaces it.

| Parameter | Type | Meaning |
|---|---|---|
| `id` | `integer` |  |
| `x` | `integer` |  |
| `y` | `integer` |  |
| `color` | `integer` | 0xRRGGBB. |

#### `player:unmark_minimap(id)`

Removes a minimap mark.

| Parameter | Type | Meaning |
|---|---|---|
| `id` | `integer` |  |

#### `player:open_shop(shop)`

Opens a shop, as callshop.

| Parameter | Type | Meaning |
|---|---|---|
| `shop` | `string` | A shop record of data/npcs.yml: the mod's own name, or mod:Name. |

### wa.Monster

A monster. A killed monster's handle reads while its death event runs, with is_alive false, and raises an error after it. Two handles to one monster are equal with ==. A monster that comes back after it died is a new monster with a new handle. Reading needs game.read.

| Field | Type | Meaning |
|---|---|---|
| `kind` | `"monster"` | The entity's kind. |
| `id` | `integer` | The monster's ID, such as 1002. |
| `name` | `string` |  |
| `level` | `integer` |  |
| `hp` | `integer` |  |
| `max_hp` | `integer` |  |
| `map` | `string` |  |
| `x` | `integer` |  |
| `y` | `integer` |  |
| `is_alive` | `boolean` |  |

### wa.Npc

An NPC: the instance of an NPC context.

| Field | Type | Meaning |
|---|---|---|
| `kind` | `"npc"` | The entity's kind. |
| `name` | `string` | The name players see. |
| `map` | `string` |  |
| `x` | `integer` |  |
| `y` | `integer` |  |

#### `npc:show_emotion(emotion)`

Shows an emotion over the NPC, as emotion.

| Parameter | Type | Meaning |
|---|---|---|
| `emotion` | `integer` | An emotion from wa.emotions. |

#### `npc:say_aloud(text)`

Says a line of local chat as the NPC, as npctalk. Players within 9 cells hear it.

| Parameter | Type | Meaning |
|---|---|---|
| `text` | `string` |  |

### wa.EquippedItem

An equipped copy: the instance of an equip context, or what player:get_equipped returns.

| Field | Type | Meaning |
|---|---|---|
| `item` | `integer` | The item's ID. |
| `refine` | `integer` |  |
| `enchant_grade` | `integer` | 0 for none, 1 to 4 for D to A. |
| `wearer` | `wa.Player` |  |
| `weapon_type` | `integer` |  |
| `weapon_level` | `integer` |  |
| `armor_level` | `integer` |  |

### wa.Conversation

A player talking to an NPC. Methods that wait pause the handler until the player answers. When the player leaves, the waiting call raises "the conversation was closed".

#### `conversation:say(...)`

Shows lines.

| Parameter | Type | Meaning |
|---|---|---|
| `...` | `string\|integer` | Up to 64 lines, each at most 1,024 bytes. |

#### `conversation:wait_for_next()`

Waits for the Next button. The next lines replace the shown ones.

#### `conversation:choose(...) → integer`

Shows options and returns the chosen one, from 1.

| Parameter | Type | Meaning |
|---|---|---|
| `...` | `string` | The options. An empty option is hidden and keeps its number. |

#### `conversation:ask_number(minimum, maximum) → integer`

Waits for a number in the range.

| Parameter | Type | Meaning |
|---|---|---|
| `minimum` | `integer` |  |
| `maximum` | `integer` |  |

#### `conversation:ask_text(maximum_length) → string`

Waits for text.

| Parameter | Type | Meaning |
|---|---|---|
| `maximum_length` | `integer` | Up to 255. |

#### `conversation:close()`

Shows the Close button and waits for it, as close2. The handler goes on after it, and later dialog calls raise an error.

#### `conversation:wait(milliseconds)`

Waits, as sleep2.

| Parameter | Type | Meaning |
|---|---|---|
| `milliseconds` | `integer` | Up to 10 minutes. |

#### `conversation:show_image(name, position)`

Shows an NPC image, as cutin.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | An image of image/illust. |
| `position` | `"left"\|"center"\|"right"` |  |

#### `conversation:hide_image()`

Hides the NPC image.

#### `conversation:show_progress(seconds)`

Shows a progress bar and waits until it is full. A walk cancels it and ends the conversation.

| Parameter | Type | Meaning |
|---|---|---|
| `seconds` | `integer` | Up to 600. |

### wa.EventHandle

What events.on returns.

#### `eventHandle:remove()`

Removes the handler. A second call does nothing.

### wa.Timer

What wa.timers returns.

#### `timer:cancel()`

Cancels the timer. A timer of timers.with_bonuses ends its bonuses.

### wa.BonusHandle

What bonuses.add returns.

#### `bonusHandle:remove()`

Removes the bonus. A second call does nothing.

### wa.TestWorld

The world of one test case. It runs one map: the map of its first new_player or spawn_monster, or prontera when advance comes first.

It exists only in tests that `wactl mod test` runs.

#### `testWorld:new_player(spec) → wa.Player`

Adds a character at a cell, with full HP and SP.

| Parameter | Type | Meaning |
|---|---|---|
| `spec` | `wa.NewPlayerSpec` |  |

#### `testWorld:talk(player, npc_name, answers?) → string`

Clicks the NPC and answers each step in order. Next is pressed without an answer. Returns everything the NPC said, one line per line. Fails when the conversation asks for more answers than given, or ends with answers left.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `npc_name` | `string` | The NPC's full name, its name in the mod, or the name players see when one NPC of the map has it. |
| `answers` | `(integer\|string)[]?` | A number picks a menu option, a string fills text input, and an integer fills number input. |

#### `testWorld:use_item(player, item)`

Uses one of the player's items. It waits for the item use interval first. Fails when the player has no such item, or the use is refused.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `item` | `integer` | An item from items.id. |

#### `testWorld:advance(milliseconds)`

Moves the zone clock forward. Timers fire and conversations that wait go on.

| Parameter | Type | Meaning |
|---|---|---|
| `milliseconds` | `integer` |  |

#### `testWorld:spawn_monster(spec) → wa.Monster`

Places a monster on a cell. It does not come back after it dies.

| Parameter | Type | Meaning |
|---|---|---|
| `spec` | `wa.SpawnMonsterSpec` |  |

#### `testWorld:kill(player, monster)`

The player kills the monster at once: the death event fires, and the player gets its exp and drops.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `monster` | `wa.Monster` | A living monster: from spawn_monster, get_monsters or spawning.monsters. |

#### `testWorld:set_time(time, time_zone?)`

Sets the server time and time zone the world starts at. Call it before the world has a map. Without it, the world starts at 2026-01-05 12:00 UTC.

| Parameter | Type | Meaning |
|---|---|---|
| `time` | `string` | Such as "2026-10-31 19:58". |
| `time_zone` | `string?` | An IANA name, such as Asia/Manila. UTC by default. |

#### `testWorld:get_time() → string`

The server time in the world's time zone, as "2026-10-31 20:00".

#### `testWorld:get_monsters(monster?) → wa.Monster[]`

The living monsters of the map, in the order they spawned.

| Parameter | Type | Meaning |
|---|---|---|
| `monster` | `integer?` | From monsters.id. Every monster without it. |

#### `testWorld:get_announcements() → string[]`

Every line announced to the world or the map so far, in order.

#### `testWorld:count_floor_items(item) → integer`

How many of the item lie on the map, with drops about to fall.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |

### wa.Expectation

A check on a value, from testing.expect. Its functions are called with a dot: testing.expect(n).to_be(3).

It exists only in tests that `wactl mod test` runs.

#### `expectation.to_be(expected)`

The value equals the expected one, with ==.

| Parameter | Type | Meaning |
|---|---|---|
| `expected` | `any` |  |

#### `expectation.to_equal(expected)`

The value is a table with the same contents, compared deeply.

| Parameter | Type | Meaning |
|---|---|---|
| `expected` | `table` |  |

#### `expectation.to_contain(part)`

A string value contains the text, or a table value has an element equal to the value.

| Parameter | Type | Meaning |
|---|---|---|
| `part` | `any` |  |

#### `expectation.to_be_true()`

The value is true.

#### `expectation.to_be_false()`

The value is false.

#### `expectation.to_be_nil()`

The value is nil.

#### `expectation.to_fail_with(text)`

The value is a function that raises an error containing the text.

| Parameter | Type | Meaning |
|---|---|---|
| `text` | `string` |  |

### wa.ModInstance

The instance of a mod context.

| Field | Type | Meaning |
|---|---|---|
| `id` | `string` | The mod's ID. |
| `map` | `string` | The zone's map. |

### wa.ItemUse

The instance of a use context.

| Field | Type | Meaning |
|---|---|---|
| `item` | `integer` | The used item's ID. |

### wa.EventInstance

The instance of an event context: one run of a world event.

| Field | Type | Meaning |
|---|---|---|
| `name` | `string` | The event's full name, such as example.halloween:Pumpkin_King. |
| `map` | `string` | The map of the zone that runs it. |

### wa.Death

The object of the death event.

| Field | Type | Meaning |
|---|---|---|
| `victim` | `wa.Player\|wa.Monster` | Readable while the event runs, with is_alive false for a monster. victim.kind says which. |
| `killer` | `wa.Player\|wa.Monster\|nil` | nil when nothing killed the unit. killer.kind says which. |
| `map` | `string` |  |
| `x` | `integer` |  |
| `y` | `integer` |  |
| `skill` | `integer?` | nil for a normal attack. |

### wa.DeathOptions

The options of a death handler. Each one narrows the deaths that run it.

| Field | Type | Meaning |
|---|---|---|
| `victim` | `wa.Player\|wa.Monster\|nil` | One unit. |
| `victims` | `(wa.Player\|wa.Monster)[]?` | Any of these units. |
| `victim_kind` | `"player"\|"monster"\|nil` |  |
| `monster` | `integer?` | A monster from monsters.id. |
| `map` | `string?` | A map name. |

### wa.Drop

The object of the drop_rate hook: one drop of a killed monster.

| Field | Type | Meaning |
|---|---|---|
| `player` | `wa.Player?` | The killer, or nil. |
| `monster` | `wa.Monster` | The killed monster. |
| `item` | `integer` |  |
| `map` | `string` |  |
| `rate` | `integer` | rAthena's rate, per 10,000. |

### wa.Exp

The object of the experience hook: the exp one player gets for a kill, after the shares and the level penalty.

| Field | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` |  |
| `monster` | `wa.Monster` | The killed monster. |
| `base` | `integer` |  |
| `job` | `integer` |  |

### wa.HookOptions

The options of hooks.on.

| Field | Type | Meaning |
|---|---|---|
| `priority` | `integer?` | Higher runs first. 0 by default. |

### wa.HookHandle

What hooks.on returns.

#### `hookHandle:remove()`

Removes the hook. A second call does nothing.

### wa.Schedule

What wa.schedules returns.

#### `schedule:cancel()`

Stops the schedule. A second call does nothing.

### wa.WhenOptions

The options of schedules.when.

| Field | Type | Meaning |
|---|---|---|
| `lasting_minutes` | `integer?` | How long the condition must hold, 0 to 10080. 0 by default. |

### wa.SpawnSpec

The monsters of spawning.monsters.

| Field | Type | Meaning |
|---|---|---|
| `monster` | `integer` | From monsters.id. |
| `map` | `string?` | The context's own map, which is the default. Another map raises an error. |
| `amount` | `integer?` | 1 to 1000. 1 by default. |
| `x` | `integer?` | With y, the cell. Without them, a random walkable cell for each monster. |
| `y` | `integer?` |  |
| `area` | `integer?` | Spreads the monsters within this many cells of x and y. |
| `respawn` | `boolean?` | true: a dead monster comes back while the context lives, as a spawn line's monster. false by default: dead stays dead. |
| `respawn_milliseconds` | `integer?` | With respawn, how long a dead monster takes to come back, 1000 to 268435455. 5000 by default, as a spawn line without a delay. Provisional. |

### wa.Damage

The object of the damage event (0002a, "Item events").

| Field | Type | Meaning |
|---|---|---|
| `source` | `wa.Player\|wa.Monster` | source.kind says which. |
| `target` | `wa.Player\|wa.Monster` | target.kind says which. |
| `kind` | `"physical"\|"magic"\|"misc"` |  |
| `range` | `"melee"\|"ranged"\|nil` |  |
| `skill` | `integer?` | nil for a normal attack. |
| `element` | `integer` |  |
| `is_critical` | `boolean` |  |
| `amount` | `integer` |  |
| `wearer_is_source` | `boolean` |  |

### wa.SkillUse

The object of the skill_used event.

| Field | Type | Meaning |
|---|---|---|
| `skill` | `integer` |  |
| `level` | `integer` |  |
| `target` | `wa.Player?` | nil for a ground skill. |
| `x` | `integer` |  |
| `y` | `integer` |  |

### wa.HealSpec

The amounts of player:item_heal and player:heal_percent. An amount is a number, or { minimum, maximum }.

| Field | Type | Meaning |
|---|---|---|
| `hp` | `integer\|integer[]?` |  |
| `sp` | `integer\|integer[]?` |  |

### wa.GiveItemOptions

Options of player:give_item.

| Field | Type | Meaning |
|---|---|---|
| `rent_seconds` | `integer?` | Gives rentals, as rentitem. Rentals do not expire yet. |

### wa.WithBonusesOptions

Options of timers.with_bonuses.

| Field | Type | Meaning |
|---|---|---|
| `restart` | `wa.Timer?` | Cancels this timer first, if it still runs. |

### wa.BonusTarget

The against and from options of a bonus.

| Field | Type | Meaning |
|---|---|---|
| `race` | `integer?` | From wa.races. |
| `element` | `integer?` | From wa.elements. |
| `size` | `integer?` | From wa.sizes. |
| `class` | `integer?` | From wa.monster_classes. |
| `monster_group` | `integer?` | From wa.monster_groups. |
| `monster` | `integer?` | From monsters.id. |
| `job` | `integer?` | From wa.jobs. |

### wa.BonusOptions

The options table of bonuses.add (0002a). Each name takes the options 0002a lists for it.

| Field | Type | Meaning |
|---|---|---|
| `percent` | `boolean?` |  |
| `skill` | `integer?` |  |
| `item` | `integer?` |  |
| `item_group` | `integer?` |  |
| `kind` | `"physical"\|"magic"\|"misc"\|nil` |  |
| `range` | `"melee"\|"ranged"\|nil` |  |
| `attack` | `"normal"\|"skill"\|nil` |  |
| `element` | `integer?` |  |
| `weapon_type` | `integer?` |  |
| `against` | `wa.BonusTarget?` |  |
| `from` | `wa.BonusTarget?` |  |
| `chance` | `integer\|(fun(): integer)\|nil` | Per 10,000. |
| `on` | `"attack"\|"hit_taken"\|"skill_used"\|nil` |  |
| `target` | `"self"\|"enemy"\|nil` |  |
| `duration` | `integer?` |  |
| `random_level` | `boolean?` |  |
| `stacking` | `"sum"\|"highest"\|"lowest"\|"last"\|"priority"\|nil` |  |
| `priority` | `integer?` |  |
| `when` | `(fun(): boolean)?` | A condition, tracked like a computed bonus. |

### wa.NewPlayerSpec

The character of world:new_player.

It exists only in tests that `wactl mod test` runs.

| Field | Type | Meaning |
|---|---|---|
| `map` | `string?` | The map, such as prontera. The world's map by default; a world runs one map. |
| `x` | `integer` |  |
| `y` | `integer` |  |
| `job` | `integer?` | From wa.jobs. A novice by default. |
| `base_level` | `integer?` | 1 by default. |
| `job_level` | `integer?` | 1 by default. |
| `zeny` | `integer?` | 0 by default. |
| `items` | `integer[][]?` | A list of { item, amount }. |

### wa.SpawnMonsterSpec

The monster of world:spawn_monster.

It exists only in tests that `wactl mod test` runs.

| Field | Type | Meaning |
|---|---|---|
| `monster` | `integer` | From monsters.id. |
| `map` | `string?` | The world's map by default. |
| `x` | `integer` |  |
| `y` | `integer` |  |

### wa.IntegerVariable

A declared integer variable of a player.

#### `integerVariable:get(player) → integer`

The value, or the default: 0, false, "", or nil for a table.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `integerVariable:set(player, value)`

Changes the value.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `value` | `integer` | nil or the default removes the stored value. |

#### `integerVariable:add(player, amount) → integer`

Adds to the value in one step, and returns the new value.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `amount` | `integer` |  |

### wa.ServerIntegerVariable

A declared integer server variable.

#### `serverIntegerVariable:get() → integer`

The value, or the default.

#### `serverIntegerVariable:set(value)`

Changes the value.

| Parameter | Type | Meaning |
|---|---|---|
| `value` | `integer` |  |

#### `serverIntegerVariable:add(amount) → integer`

Adds to the value in one step, and returns the new value.

| Parameter | Type | Meaning |
|---|---|---|
| `amount` | `integer` |  |

### wa.BooleanVariable

A declared boolean variable of a player.

#### `booleanVariable:get(player) → boolean`

The value, or the default: 0, false, "", or nil for a table.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `booleanVariable:set(player, value)`

Changes the value.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `value` | `boolean` | nil or the default removes the stored value. |

### wa.ServerBooleanVariable

A declared boolean server variable.

#### `serverBooleanVariable:get() → boolean`

The value, or the default.

#### `serverBooleanVariable:set(value)`

Changes the value.

| Parameter | Type | Meaning |
|---|---|---|
| `value` | `boolean` |  |

### wa.StringVariable

A declared string variable of a player.

#### `stringVariable:get(player) → string`

The value, or the default: 0, false, "", or nil for a table.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `stringVariable:set(player, value)`

Changes the value.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `value` | `string` | nil or the default removes the stored value. |

### wa.ServerStringVariable

A declared string server variable.

#### `serverStringVariable:get() → string`

The value, or the default.

#### `serverStringVariable:set(value)`

Changes the value.

| Parameter | Type | Meaning |
|---|---|---|
| `value` | `string` |  |

### wa.TableVariable

A declared table variable of a player.

#### `tableVariable:get(player) → table?`

The value, or the default: 0, false, "", or nil for a table.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `tableVariable:set(player, value?)`

Changes the value.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `value` | `table?` | nil or the default removes the stored value. |

### wa.ServerTableVariable

A declared table server variable.

#### `serverTableVariable:get() → table?`

The value, or the default.

#### `serverTableVariable:set(value?)`

Changes the value.

| Parameter | Type | Meaning |
|---|---|---|
| `value` | `table?` |  |

## Events

| Event | Contexts | Handler arguments | When |
|---|---|---|---|
| `login` | mod, event | `player: wa.Player` | A player logs in, in the zone it enters. |
| `logout` | mod, event | `player: wa.Player` | A player logs out, before it leaves. |
| `map_enter` | mod, event | `player: wa.Player` | A player enters the zone: at login and after a warp. |
| `death` | mod, npc, event | `death: wa.Death` | A player or a monster died on the context's map. Options narrow it. |
| `base_level_up` | mod, event | `player: wa.Player` | The player gained a base level. |
| `job_level_up` | mod, event | `player: wa.Player` | The player gained a job level. |
| `click` | npc | `player: wa.Player, conversation: wa.Conversation` | A player talks to the NPC. |
| `touch` | npc | `player: wa.Player` | A player walks into the NPC's area. It does not fire yet. |
| `use` | item | `player: wa.Player` | The item is used. |
| `damage` | item | `hit: wa.Damage` | A hit the wearer deals or takes: each normal attack that lands. Skill hits come with skills (phase 4 step 3). |
| `skill_used` | item | `use: wa.SkillUse` | The wearer uses a skill. It fires in tests until skills exist (phase 4 step 3). |
| `unequip` | item | `player: wa.Player` | The copy is unequipped, before its context ends. |

## Named types

### wa.ContextKind

A context's kind.

`mod`, `npc`, `item`, `skill`, `monster`, `event`

### wa.EventName

An event name of wa.events.

`login`, `logout`, `map_enter`, `death`, `base_level_up`, `job_level_up`, `click`, `touch`, `use`, `damage`, `skill_used`, `unequip`

### wa.BonusName

A bonus name of wa.bonuses (proposal 0002a).

`str`, `agi`, `vit`, `int`, `dex`, `luk`, `all_stats`, `pow`, `sta`, `wis`, `spl`, `con`, `crt`, `all_trait_stats`, `patk`, `smatk`, `res`, `mres`, `hplus`, `crate`, `max_hp`, `max_sp`, `max_ap`, `natural_hp_recovery`, `natural_sp_recovery`, `hp_gain_over_time`, `sp_gain_over_time`, `hp_loss_over_time`, `sp_loss_over_time`, `no_hp_recovery`, `no_sp_recovery`, `atk`, `weapon_atk`, `refine_atk`, `matk`, `hidden_matk`, `weapon_matk`, `def`, `status_def`, `mdef`, `status_mdef`, `attack_range`, `hit`, `flee`, `perfect_dodge`, `crit`, `crit_chance_taken`, `perfect_hit_chance`, `double_attack_chance`, `move_speed`, `aspd`, `variable_cast_time`, `fixed_cast_time`, `after_cast_delay`, `cooldown`, `sp_cost`, `uninterruptible_cast`, `uninterruptible_cast_everywhere`, `always_endure`, `damage`, `damage_taken`, `crit_damage`, `non_crit_damage`, `crit_damage_taken`, `skill_ratio`, `ignore_size_penalty`, `immunity`, `damage_taken_cap`, `damage_reflected`, `reflected_damage_taken`, `ignore_def`, `ignore_mdef`, `ignore_res`, `ignore_mres`, `def_as_damage`, `attack_element`, `defense_element`, `cause_status`, `coma`, `block_healing`, `cast`, `drain_hp`, `drain_sp`, `remove_hp`, `remove_sp`, `deal_max_hp`, `deal_max_sp`, `set_def`, `set_mdef`, `break_weapon`, `break_armor`, `transform_monster`, `reflect_magic`, `gain_zeny_on_kill`, `status_resistance`, `skill`, `knockback`, `no_gemstone_cost`, `no_magic_gear_fuel_cost`, `hp_per_hit`, `sp_per_hit`, `hp_per_kill`, `sp_per_kill`, `experience`, `drop_rate`, `extra_drop`, `steal_chance`, `healing_done`, `healing_received`, `item_hp_recovery`, `item_sp_recovery`, `full_recovery_on_revive`, `unbreakable_weapon`, `unbreakable_armor`, `unbreakable_helm`, `unbreakable_shield`, `unbreakable_garment`, `unbreakable_shoes`, `equipment_break_chance`, `unstrippable_weapon`, `unstrippable_armor`, `unstrippable_helm`, `unstrippable_shield`, `splash_range`, `no_knockback`, `see_hidden`, `hidden_from_detectors`, `max_weight`

### wa.EquipSlot

An equipment slot.

`head_top`, `head_middle`, `head_low`, `armor`, `left_hand`, `right_hand`, `garment`, `shoes`, `left_accessory`, `right_accessory`, `ammo`, `costume_head_top`, `costume_head_middle`, `costume_head_low`, `costume_garment`, `shadow_armor`, `shadow_weapon`, `shadow_shield`, `shadow_shoes`, `shadow_right_accessory`, `shadow_left_accessory`

### wa.HookName

A hook name of wa.hooks.

`drop_rate`, `experience`

### wa.Weekday

A day of the week.

`sunday`, `monday`, `tuesday`, `wednesday`, `thursday`, `friday`, `saturday`

