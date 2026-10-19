# Lua API reference

Generated from [server/modscript/luaapi](../../server/modscript/luaapi) with `go test ./server/modscript/luaapi -update`. Do not edit it by hand.

- The API is [proposal 0002](../proposals/0002-lua-api.md), version 1.0. A mod's manifest says `api: 1.0`.
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
| [`wa.companions`](#wacompanions) | Companions by name: core's, such as falcon, and the mods' records of data/companions.yml. |
| [`wa.mounts`](#wamounts) | Mounts by name: core's, such as peco_peco, and the mods' records of data/mounts.yml. |
| [`wa.monsters`](#wamonsters) | Monsters by name. |
| [`wa.text`](#watext) | Text for players, from the mod's locales/en.yml. Call the module with a key: text "greeter.hello". |
| [`wa.maps`](#wamaps) | Maps. |
| [`wa.world`](#waworld) | The world. |
| [`wa.world_events`](#waworld_events) | Starting and stopping world events of data/events.yml (review decision 31). A request takes effect within a second, on the scheduler's next check. |
| [`wa.channels`](#wachannels) | Chat channels, as rAthena's channel system. Core's channels are the records of data/channels.yml, such as #global and #trade. A channel is one for the whole world. |
| [`wa.parties`](#waparties) | Parties, as rAthena's party system (review decision 27). A party is one for the whole world. The write calls replace party_create, party_addmember, party_delmember, party_changeleader, party_changeoption and party_destroy, and skip the leader's rights and the invite, as those commands do. |
| [`wa.guilds`](#waguilds) | Guilds, as rAthena's guild system (review decision 28). A guild is one for the whole world. The write calls skip the position permissions and the invite, as rAthena's guild script commands. Castles, sieges and guardians come later. |
| [`wa.rules`](#warules) | The world's ruleset (strategy section 3.6, review decisions 27 and 32). Read-only. Its calls need no permission. An unknown name raises an error that lists the valid names. |
| [`wa.places`](#waplaces) | Named places of generated maps (review decision 42): cells with tags, such as blacksmith, square, gate, entrance, exit or boss_room, that the map generator records. Content finds places by tag, not by coordinates. A classic world has no places: find returns an empty list and nearest nil. |
| [`wa.towns`](#watowns) | The towns of the world layout (review decision 43): each with an ID such as t7, a display name, a map and a level band. A classic world has no towns: list returns an empty list, and nearest and get nil. |
| [`wa.generation`](#wageneration) | Generation layers (review decision 46): mod code that runs when wactl world new makes a procedural world, on the server only, and in the test worlds of wactl mod test. A layer adds named places, objects and bosses to each generated map. It cannot change terrain cells, which the client generates. Both calls need world.generate. In a running zone they record what they get, and nothing runs it. |
| [`wa.variables`](#wavariables) | Saved variables, declared at the top of a file with a scope and a type. They are stored under the mod's ID. A mod declares at most 256 variables, and declaring one again gives the same handle. A mod's values take at most 1 MiB as JSON on one player, and 1 MiB in its server variables. |
| [`wa.random`](#warandom) | Random numbers from the zone's secret stream. |
| [`wa.integers`](#waintegers) | Integer division that rounds as C does, toward zero, for scripts converted from rAthena. |
| [`wa.bonuses`](#wabonuses) | Item bonuses (proposal 0002a). Equip contexts only. |
| [`wa.races`](#waraces) | Monster races, from rAthena's RC_ names. |
| [`wa.monster_groups`](#wamonster_groups) | Monster groups, from rAthena's RC2_ names. |
| [`wa.elements`](#waelements) | Elements, from rAthena's ELE_ names. |
| [`wa.sizes`](#wasizes) | Sizes, from rAthena's Size_ names. |
| [`wa.monster_classes`](#wamonster_classes) | Monster classes, from rAthena's CLASS_ names. |
| [`wa.weapon_types`](#waweapon_types) | Weapon types, from rAthena's W_ names. |
| [`wa.statuses`](#wastatuses) | Status effects, from rAthena's SC_ names, and the statuses mods define. |
| [`wa.effects`](#waeffects) | Effects for entity:show_effect, from rAthena's EF_ names. |
| [`wa.headgear_effects`](#waheadgear_effects) | Effects for player:show_headgear_effect, from rAthena's HAT_EF_ names. |
| [`wa.emotions`](#waemotions) | Emotions for npc:show_emotion, from rAthena's ET_ names. |
| [`wa.jobs`](#wajobs) | Jobs, from rAthena's JOB_ names, and job tests. The job tests work with mod jobs too. |
| [`wa.testing`](#watesting) | Test cases. It exists only under wactl mod test, never in a running server. |

### wa.context

```lua
local context = require "wa.context"
```

The current instance, kind and mod. Calls outside a setup function or a handler raise an error.

#### `context:get_instance() → wa.ModInstance|wa.Npc|wa.EquippedItem|wa.ItemUse|wa.EventInstance|wa.SkillInstance|wa.PassiveSkillInstance|wa.Monster`

The instance the context owns: the mod, the NPC, the equipped copy, the used item, the event run, the skill, a passive skill of one owner, or the monster of a monster's AI script.

#### `context:get_kind() → wa.ContextKind`

The context's kind.

#### `context:get_mod() → string`

The ID of the context's mod.

#### `context:get_event() → wa.EventRun?`

The run of a typed world event, in the event context of its script (review decision 31), or nil in other contexts.

### wa.events

```lua
local events = require "wa.events"
```

Event handlers. The context owns them, and removes them when it ends.

#### `events.on(name, handler, options) → wa.EventHandle`

Registers a handler for an event.

Permission: `event.listen`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `wa.EventName` | The event's name. An unknown name, or an event of another context kind, raises an error. |
| `handler` | `function` | Runs each time the event fires, in registration order. |
| `options` | `wa.DeathOptions\|wa.StatusEventOptions\|wa.SkillUsedOptions\|wa.ChatOptions\|wa.TradeOptions\|nil` | Only the death, status, skill_used, chat and trade events take options: they narrow which events run the handler. |

### wa.hooks

```lua
local hooks = require "wa.hooks"
```

Hooks change a game result in a chain ordered by priority. Each hook calls next to get the rest of the chain's result. The innermost result is rAthena's value. Hooks only compute: calls that change the game raise an error in them. The context owns them.

#### `hooks.on(name, hook, options?) → wa.HookHandle`

Adds a hook. Higher priority runs first; equal priorities run in the order they were added. spawn_table needs spawns.modify in place of rules.override. It runs only when wactl world new makes a procedural world, and in the test worlds of wactl mod test.

Permission: `rules.override`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `wa.HookName` | The hook's name. An unknown name raises an error. |
| `hook` | `(fun(object: wa.Drop\|wa.Exp\|wa.StatusHook\|wa.SkillHook\|wa.MonsterTargetChoice\|wa.ItemRoll\|wa.ChatMessage\|wa.Trade, next: fun(object?: table): integer\|boolean\|string\|wa.Player\|wa.Monster\|wa.Npc\|nil, integer\|string\|nil): integer\|boolean\|string\|wa.Player\|wa.Monster\|wa.Npc\|nil, integer\|string\|nil)\|fun(map: wa.SpawnMap, spawns: wa.Spawn[], next: fun(spawns?: wa.Spawn[]): wa.Spawn[]): wa.Spawn[]` | Gets the object and next. drop_rate returns a rate per 10,000; experience returns base and job exp; status_chance returns a chance per 10,000; status_duration and cast_time return milliseconds; skill_damage returns the damage percentage; skill_cost returns SP; cast_allowed returns true or false; monster_target returns one of the candidates, or nil for none; item_roll returns the value, which the server clamps to the roll's range; chat returns the text, or nil to block the message; trade returns true, or false and a message to refuse the trade. spawn_table gets the map, a copy of its spawn list and next, and returns the list: next(spawns) runs the rest of the chain on a list, the hook's own by default. |
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
| `name` | `string` | A skill_db name, such as SM_BASH, or a mod skill's full name, such as example.chain-lightning:CHAIN_LIGHTNING. |

### wa.companions

```lua
local companions = require "wa.companions"
```

Companions by name: core's, such as falcon, and the mods' records of data/companions.yml.

#### `companions.id(name) → integer`

The companion's ID. An unknown name raises an error.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A core companion's name, such as falcon, or a mod companion's full name, such as example.familiars:Owl. |

### wa.mounts

```lua
local mounts = require "wa.mounts"
```

Mounts by name: core's, such as peco_peco, and the mods' records of data/mounts.yml.

#### `mounts.id(name) → integer`

The mount's ID. An unknown name raises an error.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A core mount's name, such as peco_peco, grand_peco, gryphon, dragon, warg or madogear, or a mod mount's full name. |

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

Shows a line to every player of a map, as mapannounce. Players see the mod's ID with a mod's line.

| Parameter | Type | Meaning |
|---|---|---|
| `map` | `string` | A map name, such as prontera. |
| `text` | `string` | At most 255 bytes. |

#### `maps.find_entities(query) → (wa.Player|wa.Monster|wa.Npc)[]`

The entities of an area of the context's map that pass every filter, sorted, and cut to the limit. Ties sort by kind, then by entity. A query with no area, two areas, or a field it does not know raises an error.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `query` | `wa.EntityQuery` | One area, filters, and an order. |

#### `maps.get_display_name(map) → string?`

The name players see for a map of the world layout (review decision 43), such as "North Valheim Fields 2". It changes when staff rename a town. nil for another map, and for every map of a classic world until locale files exist.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `map` | `string` | A map ID, such as "gen:town:t7". |

### wa.world

```lua
local world = require "wa.world"
```

The world.

#### `world.announce(text)`

Shows a line to every player of the world, as announce. Players see the mod's ID with a mod's line.

Permission: `world.announce`.

| Parameter | Type | Meaning |
|---|---|---|
| `text` | `string` | At most 255 bytes. |

#### `world.get_shop(name) → wa.Shop?`

A shop's handle, or nil when there is none. world:get_shop(name), as review decision 30 writes it, works too.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A shop of data/npcs.yml: a full record name, or a plain name, which is the calling mod's record, then core's. |

### wa.world_events

```lua
local world_events = require "wa.world_events"
```

Starting and stopping world events of data/events.yml (review decision 31). A request takes effect within a second, on the scheduler's next check.

#### `world_events.start(name) → boolean`

Starts a run of the event now, for its duration, as if its trigger fired. Returns false when there is no such event, or it runs already. An event the config turned off does not exist.

Permission: `events.control`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | An event's full name, such as core:Field_Boss, or a plain name, which is the calling mod's event. |

#### `world_events.stop(name) → boolean`

Ends the event's run now, with the result "stopped". Its monsters and NPCs go. Returns false when there is no such event, or it does not run.

Permission: `events.control`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | An event's full name, or a plain name, which is the calling mod's event. |

### wa.channels

```lua
local channels = require "wa.channels"
```

Chat channels, as rAthena's channel system. Core's channels are the records of data/channels.yml, such as #global and #trade. A channel is one for the whole world.

#### `channels.create(spec) → wa.Channel`

Makes a channel that the context owns, and returns it. The channel ends when its owner ends, and its members leave it. The same mod's context in another zone, or after a reload, gets the same channel. A name that core or another mod has raises an error.

| Parameter | Type | Meaning |
|---|---|---|
| `spec` | `wa.ChannelSpec` |  |

#### `channels.get(name) → wa.Channel?`

A running channel: core's or a mod's. nil when none has the name.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A channel's name, with or without its #, such as trade. |

### wa.parties

```lua
local parties = require "wa.parties"
```

Parties, as rAthena's party system (review decision 27). A party is one for the whole world. The write calls replace party_create, party_addmember, party_delmember, party_changeleader, party_changeoption and party_destroy, and skip the leader's rights and the invite, as those commands do.

#### `parties.create(leader, name) → wa.Party?`

Makes a party with the player as its leader, as party_create, and returns it. Exp share starts as each, and the items as each. nil when the name is taken or the leader has a party. The party_join event follows, with the reason "script". A test case needs no permission for it.

Permission: `party.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `leader` | `wa.Player` |  |
| `name` | `string` | 1 to 24 characters. |

#### `parties.get(id) → wa.Party?`

A party by ID, or nil.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `id` | `integer` | A party's ID, such as player.party.id. |

#### `parties.find(name) → wa.Party?`

A party by name, or nil.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A party's name, without regard to case. |

### wa.guilds

```lua
local guilds = require "wa.guilds"
```

Guilds, as rAthena's guild system (review decision 28). A guild is one for the whole world. The write calls skip the position permissions and the invite, as rAthena's guild script commands. Castles, sieges and guardians come later.

#### `guilds.create(master, name) → wa.Guild?`

Makes a guild with the player as its master and returns it. It takes no Emperium, so the script decides the cost. nil when the name is taken or the master has a guild. The guild_join event follows, with the reason "script". A test case needs no permission for it.

Permission: `guild.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `master` | `wa.Player` |  |
| `name` | `string` | 1 to 24 characters. |

#### `guilds.get(id) → wa.Guild?`

A guild by ID, or nil.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `id` | `integer` | A guild's ID, such as player.guild.id. |

#### `guilds.find(name) → wa.Guild?`

A guild by name, or nil.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A guild's name, without regard to case. |

### wa.rules

```lua
local rules = require "wa.rules"
```

The world's ruleset (strategy section 3.6, review decisions 27 and 32). Read-only. Its calls need no permission. An unknown name raises an error that lists the valid names.

| Field | Type | Meaning |
|---|---|---|
| `limits` | `wa.Limits` | The limits that rAthena compiles in, as the ruleset sets them, such as rules.limits.party_members. Each is the limit's base. |

#### `rules.is(flag) → boolean`

Whether a ruleset flag is on: true at its renewal value or at on. A skill_rebalance flag is the flag the skill follows: its own, or the base.

| Parameter | Type | Meaning |
|---|---|---|
| `flag` | `string` | A ruleset flag, such as "cast_time", or "skill_rebalance.<skill>" for a rebalanced skill or a song, such as "skill_rebalance.MG_FIREBALL". |

#### `rules.get(name) → integer|boolean`

The value of a ruleset rule: a boolean for a rule that is yes or no, else an integer.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A ruleset rule, such as "basic_skill_check" or "party_share_level". |

#### `rules.get_limit(name) → integer, integer`

A limit's base, what a player or guild starts with, and its max, where expansions stop. rules.limits holds the base too.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A limit, such as "inventory_slots". |

### wa.places

```lua
local places = require "wa.places"
```

Named places of generated maps (review decision 42): cells with tags, such as blacksmith, square, gate, entrance, exit or boss_room, that the map generator records. Content finds places by tag, not by coordinates. A classic world has no places: find returns an empty list and nearest nil.

#### `places.find(scope, tag) → wa.Place[]`

Every place of the scope with the tag, in a fixed order: by map ID, then in the order the generator made them. Empty when none matches.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `scope` | `string` | Where to look: "town:any", "town:<id>" such as "town:t7", "dungeon:any", "dungeon:<id>" such as "dungeon:crypt", or "map:<id>" such as "map:gen:field:3:-1". Another form raises an error. |
| `tag` | `string` | A tag, such as "blacksmith" or "shop:tool": 1 to 64 lowercase letters, digits, _, -, . and :. |

#### `places.nearest(player, tag) → wa.Place?`

The place with the tag closest to the player: the fewest warps away on the map graph, then the shortest distance. On another map, the distance counts from where the player would arrive. Maps no warp leads to are left out. nil when no place qualifies.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `tag` | `string` | A tag, as for find. |

### wa.towns

```lua
local towns = require "wa.towns"
```

The towns of the world layout (review decision 43): each with an ID such as t7, a display name, a map and a level band. A classic world has no towns: list returns an empty list, and nearest and get nil.

#### `towns.list(options?) → wa.Town[]`

Every town by number, or with visited_by only the towns that player entered.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `options` | `wa.TownListOptions?` |  |

#### `towns.nearest(player) → wa.Town?`

The town the fewest warps from the player's map, ties to the lower number: the player's own town when it stands in one. nil when no town is reached.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `towns.get(id) → wa.Town?`

A town by its ID, or nil when the world has none with it.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `id` | `string` | A town's ID, such as "t7". Another form raises an error. |

### wa.generation

```lua
local generation = require "wa.generation"
```

Generation layers (review decision 46): mod code that runs when wactl world new makes a procedural world, on the server only, and in the test worlds of wactl mod test. A layer adds named places, objects and bosses to each generated map. It cannot change terrain cells, which the client generates. Both calls need world.generate. In a running zone they record what they get, and nothing runs it.

#### `generation.on(event, layer?)`

Adds a generation layer in a mod context. The mods' layers run in load order. A layer has no other source of randomness than its node: wa.random raises an error while it runs. A layer that fails is logged and left out.

Permission: `world.generate`.

| Parameter | Type | Meaning |
|---|---|---|
| `event` | `"map"` | The event: "map", once on each generated field, town and dungeon floor, in the layout's order. |
| `layer` | `fun(map: wa.GenerationMap, node: wa.GenerationNode): wa.GenerationResult?` | Gets the map and the layer's node, and returns nil or what to add. |

#### `generation.quest_template(record)`

Adds a quest template, which the world's quest boards and job guilds draw from as from the records. Call it in an entry script or a layer. A bad record or a second template of the name raises an error.

Permission: `world.generate`.

| Parameter | Type | Meaning |
|---|---|---|
| `record` | `table` | A record of data/quest_templates.yml with its name, such as { name = "shrine_hunt", kind = "hunt", title = "Hunt {count} {monster}", count = { 5, 10 } }. The name gets the mod's prefix. |

### wa.variables

```lua
local variables = require "wa.variables"
```

Saved variables, declared at the top of a file with a scope and a type. They are stored under the mod's ID. A mod declares at most 256 variables, and declaring one again gives the same handle. A mod's values take at most 1 MiB as JSON on one player, and 1 MiB in its server variables.

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

Status effects, from rAthena's SC_ names, and the statuses mods define.

Constants: [lua-constants.md](lua-constants.md#wastatuses).

#### `statuses.define(definition)`

Adds a status in a mod script. It gets a constant in wa.statuses named after it, such as statuses.glow. Its ID stays the same in every zone, and a saved status keeps its name. A name that a status of the game or of another mod has raises an error. A mod defines at most 256 statuses.

| Parameter | Type | Meaning |
|---|---|---|
| `definition` | `wa.StatusDefinition` |  |

### wa.effects

```lua
local effects = require "wa.effects"
```

Effects for entity:show_effect, from rAthena's EF_ names.

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

Jobs, from rAthena's JOB_ names, and job tests. The job tests work with mod jobs too.

Constants: [lua-constants.md](lua-constants.md#wajobs).

#### `jobs.id(name) → integer`

The job's ID. An unknown name raises an error.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A name of wa.jobs, such as knight, or a mod job's full name, such as example.jobs:Desperado. |

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

rAthena's BaseJob: a Lord Knight is a Knight. A job record's base_job.

| Parameter | Type | Meaning |
|---|---|---|
| `job` | `integer` |  |

#### `jobs.first_job(job) → integer`

rAthena's BaseClass: a Knight is a Swordman.

| Parameter | Type | Meaning |
|---|---|---|
| `job` | `integer` |  |

#### `jobs.next_jobs(job) → integer[]`

The jobs a job can change into, in job ID order: the jobs whose record lists it in changes_from. In core's records a Novice changes into a first class or a Super Novice, a High Novice into a transcendent first class, and a first class into its two second classes. Jobs that differ by sex, such as Bard and Dancer, are both listed. Other core jobs have none until third classes exist.

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
| `party` | `wa.Party?` | The player's party, or nil. |
| `guild` | `wa.Guild?` | The player's guild, or nil. |
| `sex` | `"male"\|"female"` |  |
| `companion` | `integer?` | The player's companion, from companions.id, or nil. |
| `mount` | `integer?` | The player's mount, from mounts.id, or nil. |
| `cart` | `integer?` | The look of the player's cart, from 1, or nil. |
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

#### `player:count_item(item, options?) → integer?`

How many of the item the player has, as countitem. With from, how many a storage or the cart holds, as storagecountitem and cartcountitem; nil when it cannot be read: no cart, or a storage window open.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |
| `options` | `wa.FromOptions?` |  |

#### `player:take_item(item, amount, options?) → boolean`

Takes items, as delitem, or with from out of a storage or the cart, as storagedelitem and cartdelitem. Returns false, and takes nothing, when there are too few.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |
| `amount` | `integer` | 1 or more. |
| `options` | `wa.FromOptions?` |  |

#### `player:open_storage(name?) → boolean`

Opens the account's storage or an extra storage, as openstorage and openstorage2. Returns false when the player has a storage open, or another character of the account has this one open. Scripts charge any fee.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string?` | An extra storage of data/storages.yml. A name without a mod prefix is the calling mod's record, then core's. A mod reaches only its own records and core's: another mod's name raises an error. Nothing opens the account's storage. |

#### `player:open_guild_storage() → boolean, string?`

Opens the guild storage, as guildopenstorage. Returns true, or false and why: "no_guild", "no_storage", "no_permission", "in_use" or "already_open".

#### `player:get_currency(currency) → integer`

How much of a currency of data/currencies.yml the player has, such as cash for rAthena's #CASHPOINTS. An account's currency is its account's.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `currency` | `string` | A currency of data/currencies.yml: a full name, such as example.arena:tokens, or a plain name, which is the calling mod's record, then core's, such as cash or kafra. |

#### `player:give_currency(currency, amount)`

Gives a currency. An amount over the currency's max is lost, as pc_getcash. A mod changes its own currencies freely. Another mod's, and core's cash and kafra, need currencies.modify.

| Parameter | Type | Meaning |
|---|---|---|
| `currency` | `string` | A currency of data/currencies.yml: a full name, such as example.arena:tokens, or a plain name, which is the calling mod's record, then core's, such as cash or kafra. |
| `amount` | `integer` | 1 to 2147483647. |

#### `player:take_currency(currency, amount) → boolean`

Takes a currency, as pc_paycash. Returns false, and takes nothing, when the player has too little. A mod changes its own currencies freely. Another mod's, and core's, need currencies.modify.

| Parameter | Type | Meaning |
|---|---|---|
| `currency` | `string` | A currency of data/currencies.yml: a full name, such as example.arena:tokens, or a plain name, which is the calling mod's record, then core's, such as cash or kafra. |
| `amount` | `integer` | 1 to 2147483647. |

#### `player:send_mail(mail)`

Sends a mail to a character, online or not, as rAthena's mail command. The items are made for the mail, and the zeny comes from nowhere: the player pays nothing. A mail to a character who is not online arrives after the call returns. Script mail is not returned to its sender when it expires; it is deleted after mail_delete_days, as int_mail.cpp does with mail of send_id 0.

Permission: `mail.send`.

| Parameter | Type | Meaning |
|---|---|---|
| `mail` | `wa.MailSpec` |  |

#### `player:open_mail() → boolean`

Opens the player's mailbox window, as openmail. Returns false when the player is trading, vending, running a buying store or has a storage open, as mail_openmail.

#### `player:autotrade() → boolean`

Closes the player's connection and keeps its vending shop or buying store open, as @autotrade. The store opens again after a restart, as autotrade_restore. The player leaves when its store closes or sells out, or when it logs in again. Returns false when the player has no store open.

#### `player:search_stores(search) → boolean`

Opens the store search window, as searchstores, over the vending shops and buying stores of every map. Results can filter by price, cards and the ranges of rolls and random options. Returns false when the window is open already.

| Parameter | Type | Meaning |
|---|---|---|
| `search` | `wa.StoreSearchSpec` |  |

#### `player:open_buying_store(store) → boolean`

Opens the buying store setup window, as buyingstore. The player then picks what to buy. Returns false when the player is vending, has a buying store open, or is trading, as buyingstore_setup.

| Parameter | Type | Meaning |
|---|---|---|
| `store` | `wa.BuyingStoreSpec` |  |

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

#### `player:find_nearby(query) → (wa.Player|wa.Monster|wa.Npc)[]`

maps.find_entities with around set to the entity.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `query` | `wa.EntityQuery` | A query without an area: it is around the entity, with radius. |

#### `player:exists() → boolean`

False once the entity is removed, has logged out, or has left the zone. It never raises an error.

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

#### `player:add_timed_bonuses(key, milliseconds, bonuses, options?) → boolean`

Gives the player bonuses for a time, as rAthena's bonus_script. They are saved across logout, unless ends_on has logout. The player's stats change before it returns. Returns true when the bonuses were added or extended, and false when on_repeat ignored them.

| Parameter | Type | Meaning |
|---|---|---|
| `key` | `string` | 1 to 64 bytes. The option on_repeat says what a second call with the same key does. |
| `milliseconds` | `integer` | 1 or more. |
| `bonuses` | `table` | A list of entries: a bonus name, its arguments, and its options, as in bonuses.add, such as { { "str", 5 } }. |
| `options` | `wa.TimedBonusOptions?` |  |

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

#### `player:change_job(job, options?) → boolean`

Changes the player's job, as jobchange with rAthena's pc_jobchange. Job level goes back to 1, and learned skills stay. A transcendent player stays in the line it had before its rebirth. Returns false, and changes nothing, when the job is not a valid next job (jobs.next_jobs) or the player lacks the base or job level that the job's changes_from asks. A male player who changes into a Dancer becomes a Bard, and the reverse. The job_change event follows a change.

| Parameter | Type | Meaning |
|---|---|---|
| `job` | `integer` | A job from wa.jobs or jobs.id. |
| `options` | `wa.ChangeJobOptions?` |  |

#### `player:rebirth() → boolean`

Makes a second class at base level 99 and job level 50 or more a High Novice at base and job level 1, with 100 status points, First Aid and Trick Dead, as rAthena's rebirth. Its second class picks its transcendent jobs. Returns false, and changes nothing, for other players.

#### `player:set_companion(companion?) → boolean`

Gives the player a companion in place of its current one, as setfalcon. Returns false, and changes nothing, when the player lacks the skills of the companion's requires, such as Falconry Mastery for a falcon.

| Parameter | Type | Meaning |
|---|---|---|
| `companion` | `integer?` | A companion from companions.id, or nil to remove it. |

#### `player:set_mount(mount?) → boolean`

Puts the player on a mount in place of its current one, as setriding. The mount's acts_as picks its rules: walk speed, the mounted ASPD rule and spear damage. Returns false, and changes nothing, when the player lacks the skills of the mount's requires, such as Riding for a Peco Peco.

| Parameter | Type | Meaning |
|---|---|---|
| `mount` | `integer?` | A mount from mounts.id, or nil to remove it. |

#### `player:set_cart(look?) → boolean`

Gives the player a cart of a look, as setcart. Returns false, and changes nothing, without Pushcart or for a look the client does not have.

| Parameter | Type | Meaning |
|---|---|---|
| `look` | `integer?` | A cart look from 1 to 5, or nil to remove the cart. |

#### `player:reset_skills()`

Lowers every skill learned with skill points to 0 and refunds the points, as resetskill. Quest skills stay, and so does Basic Skill outside the Novice jobs.

#### `player:reset_stats()`

Sets every stat to 1 and gives back the status points of the base level, as resetstatus.

#### `player:reset_skill(skill, options?) → boolean`

Lowers one skill, to 0 by default, and refunds its skill points. Returns false, and changes nothing, when another learned skill needs the lowered level and with_dependents is not set, when the skill is not above the level, or for a quest skill.

| Parameter | Type | Meaning |
|---|---|---|
| `skill` | `integer` | A skill from skills.id. |
| `options` | `wa.ResetSkillOptions?` |  |

#### `player:whisper(text, options?)`

Sends the player a whisper. It does not pass the chat filter or the chat hooks, and the player's block list does not stop it. The player sees the mod's ID with a mod's whisper, as a server line it cannot answer, so it cannot pass for a player's. Core's whispers are plain whispers.

| Parameter | Type | Meaning |
|---|---|---|
| `text` | `string` | 1 to 255 bytes. |
| `options` | `wa.WhisperOptions?` |  |

#### `player:is_blocking(other) → boolean`

Whether the player blocks the other player's name.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `other` | `wa.Player` |  |

#### `player:is_friend(other) → boolean`

Whether the other player is on the player's friend list. Only players add and remove friends.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `other` | `wa.Player` |  |

#### `player:get_friend_names() → string[]`

The names on the player's friend list, in the order they were added.

Permission: `game.read`.

#### `player:get_staff_role() → string?`

The player's staff role, such as "game_master", or nil. The owner role is "owner". A role counts only when the player logged in with a second factor, a TOTP code, a recovery code or a passkey, and the account has TOTP or a passkey on, as on the admin screens.

Permission: `game.read`.

#### `player:has_staff_permission(permission) → boolean`

Whether the player's staff role has a permission. The owner has every permission. It follows the two-factor rule of get_staff_role.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `permission` | `string` | A staff permission, such as "moderation.mute". An unknown name raises an error that lists the names. |

#### `player:has_visited(town) → boolean`

Whether the player's character entered the town, as the teleport service lists it (review decision 43).

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `town` | `wa.Town` |  |

#### `player:add_quest(quest) → boolean`

Adds the quest to the player's quest log as active, with its time limit from now, as setquest. Returns false, and changes nothing, when the log has the quest already, in any state. The quest_add event follows.

| Parameter | Type | Meaning |
|---|---|---|
| `quest` | `integer\|string` | A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error. |

#### `player:complete_quest(quest) → boolean`

Marks an active or inactive quest complete, as completequest. It leaves the quest window, and kills stop counting for it. Returns false when the log lacks the quest or holds it as complete. The quest_complete event follows.

| Parameter | Type | Meaning |
|---|---|---|
| `quest` | `integer\|string` | A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error. |

#### `player:remove_quest(quest) → boolean`

Takes the quest out of the log, in any state, as erasequest. Returns false when the log lacks it.

| Parameter | Type | Meaning |
|---|---|---|
| `quest` | `integer\|string` | A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error. |

#### `player:change_quest(from, to) → boolean`

Replaces a quest with another, which starts active with its time limit from now and no kills, as changequest. Returns false, and changes nothing, when the log lacks from, holds it as complete, or has to already. The quest_add event follows for to.

| Parameter | Type | Meaning |
|---|---|---|
| `from` | `integer\|string` | A quest the log holds as active or inactive. |
| `to` | `integer\|string` | A quest the log lacks. |

#### `player:get_quest_state(quest) → "none"|"active"|"inactive"|"complete"`

The quest's state in the player's log. A quest is inactive when the player marked it so in the quest window.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `quest` | `integer\|string` | A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error. |

#### `player:get_quest_progress(quest, objective) → integer?, integer?`

The kills counted for a hunt objective and the kills it asks for, as checkquest with HUNTING reads them. nil when the player lacks the quest or the quest has no such objective. Kills stop counting at the goal.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `quest` | `integer\|string` | A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error. |
| `objective` | `integer` | A hunt objective of the quest, from 1. |

#### `player:quest_timed_out(quest) → boolean`

Whether the quest's time limit passed, as checkquest with PLAYTIME. false for a quest without a time limit, and for a quest the player lacks.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `quest` | `integer\|string` | A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error. |

#### `player:mute(minutes, reason)`

Mutes the player, as rAthena's SC_NOCHAT: it cannot talk until the end. It replaces a mute that has not ended. The player sees the reason and the end time. The mute is saved.

Permission: `player.sanction`.

| Parameter | Type | Meaning |
|---|---|---|
| `minutes` | `integer` | 1 to 5,256,000 (ten years). |
| `reason` | `string` | 1 to 255 characters. The player sees it. |

#### `player:jail(minutes, reason)`

Jails the player, as rAthena's pc_jail: it goes to the jail map and cannot warp until the end, then goes back to where it was. It replaces a jail that has not ended. The player sees the reason and the end time. The jail is saved.

Permission: `player.sanction`.

| Parameter | Type | Meaning |
|---|---|---|
| `minutes` | `integer` | 1 to 5,256,000 (ten years). |
| `reason` | `string` | 1 to 255 characters. The player sees it. |

#### `player:unmute() → boolean`

Ends the player's mute. Returns false when it was not muted.

Permission: `player.sanction`.

#### `player:expand(limit, amount) → integer`

Raises one of the player's limits by amount, up to the limit's max, and returns the new value, as rAthena's inventory expansion. A negative amount lowers it, not below the base, and deletes nothing. inventory_slots and cart_slots save with the character, storage_slots with the account, whose characters share it. 0 changes nothing and returns the current value.

Permission: `player.expand_limits`.

| Parameter | Type | Meaning |
|---|---|---|
| `limit` | `wa.ExpandableLimit` | The limit to change. |
| `amount` | `integer` | -10,000 to 10,000. A negative amount lowers the limit. |

#### `player:reset_stat(stat, options?) → boolean`

Lowers one stat, to 1 by default, and refunds what raising it cost, by rAthena's cost table. Returns false when the stat is not above the value.

| Parameter | Type | Meaning |
|---|---|---|
| `stat` | `"str"\|"agi"\|"vit"\|"int"\|"dex"\|"luk"` |  |
| `options` | `wa.ResetStatOptions?` |  |

#### `player:start_status(status, options?) → boolean`

Starts a status, as sc_start, sc_start2 and sc_start4, with rAthena's resistance and rules. Returns true when it starts. The target's stats change before it returns, as in rAthena. The status_start event runs after the script's call returns. On a player it needs player.status.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` | A status from wa.statuses. |
| `options` | `wa.StatusOptions?` |  |

#### `player:end_status(status) → boolean`

Ends a status, as sc_end. Returns true when it was active. The target's stats change before it returns. The status_end event runs after the script's call returns. On a player it needs player.status.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` |  |

#### `player:has_status(status) → boolean`

Whether a status is active, as getstatus.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` |  |

#### `player:get_status(status) → wa.Status?`

An active status, or nil.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` |  |

#### `player:get_statuses() → wa.Status[]`

The active statuses, by status.

Permission: `game.read`.

### wa.Monster

A monster. A killed monster's handle reads while its death event runs, with is_alive false, and raises an error after it. Two handles to one monster are equal with ==. A monster that comes back after it died is a new monster with a new handle. Reading needs game.read. The methods control its AI, as rAthena's unit script commands. They raise an error once the monster is gone.

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
| `target` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | The unit its AI follows (target_id), or nil. |
| `state` | `wa.MonsterState` | Its skill state, in rAthena's words. |
| `master` | `wa.Player\|wa.Monster\|nil` | The unit that summoned it as a slave, or nil. |

#### `monster:show_effect(effect)`

Plays an effect on the monster for everyone near, as specialeffect.

| Parameter | Type | Meaning |
|---|---|---|
| `effect` | `integer` | An effect from wa.effects. |

#### `monster:find_nearby(query) → (wa.Player|wa.Monster|wa.Npc)[]`

maps.find_entities with around set to the entity.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `query` | `wa.EntityQuery` | A query without an area: it is around the entity, with radius. |

#### `monster:exists() → boolean`

False once the entity is removed, has logged out, or has left the zone. It never raises an error.

#### `monster:set_target(target) → boolean`

The monster takes the target and chases it to attack, as unitattack. Returns false when it cannot target it.

| Parameter | Type | Meaning |
|---|---|---|
| `target` | `wa.Player\|wa.Monster\|wa.Npc` |  |

#### `monster:clear_target()`

The monster drops its target and goes idle, as mob_unlocktarget.

#### `monster:walk_to(x, y) → boolean`

The monster walks to a cell, as unitwalk. Its AI waits until it gets there. Returns false when it cannot walk there.

| Parameter | Type | Meaning |
|---|---|---|
| `x` | `integer` |  |
| `y` | `integer` |  |

#### `monster:use_skill(skill, level, target) → boolean`

The monster uses a skill with the skill's own cast time, as unitskilluseid. No delay of its skills list applies. Returns false when the cast does not start.

| Parameter | Type | Meaning |
|---|---|---|
| `skill` | `integer` | From skills.id. |
| `level` | `integer` | 1 to 100. |
| `target` | `wa.Player\|wa.Monster\|wa.Npc` | The target, or its cell for a ground skill. The monster itself for a skill on itself. |

#### `monster:talk(text)`

The monster says a line to the players near it, after its name, as unittalk.

| Parameter | Type | Meaning |
|---|---|---|
| `text` | `string` | 1 to 255 bytes. |

#### `monster:stop()`

The monster stops walking and attacking, and drops its target, as unitstop.

#### `monster:start_status(status, options?) → boolean`

Starts a status, as sc_start, sc_start2 and sc_start4, with rAthena's resistance and rules. Returns true when it starts. The target's stats change before it returns, as in rAthena. The status_start event runs after the script's call returns. On a player it needs player.status.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` | A status from wa.statuses. |
| `options` | `wa.StatusOptions?` |  |

#### `monster:end_status(status) → boolean`

Ends a status, as sc_end. Returns true when it was active. The target's stats change before it returns. The status_end event runs after the script's call returns. On a player it needs player.status.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` |  |

#### `monster:has_status(status) → boolean`

Whether a status is active, as getstatus.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` |  |

#### `monster:get_status(status) → wa.Status?`

An active status, or nil.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` |  |

#### `monster:get_statuses() → wa.Status[]`

The active statuses, by status.

Permission: `game.read`.

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

#### `npc:show_quest_icon(player, icon?)`

Shows an icon over the NPC to one player, as showevent. It stays while the player is on the map.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `icon` | `wa.QuestIcon?` | nil removes the icon. |

#### `npc:show_effect(effect)`

Plays an effect on the NPC for everyone near, as specialeffect.

| Parameter | Type | Meaning |
|---|---|---|
| `effect` | `integer` | An effect from wa.effects. |

#### `npc:find_nearby(query) → (wa.Player|wa.Monster|wa.Npc)[]`

maps.find_entities with around set to the entity.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `query` | `wa.EntityQuery` | A query without an area: it is around the entity, with radius. |

#### `npc:exists() → boolean`

False once the entity is removed, has logged out, or has left the zone. It never raises an error.

#### `npc:start_status(status, options?) → boolean`

Starts a status, as sc_start, sc_start2 and sc_start4, with rAthena's resistance and rules. Returns true when it starts. The target's stats change before it returns, as in rAthena. The status_start event runs after the script's call returns. On a player it needs player.status.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` | A status from wa.statuses. |
| `options` | `wa.StatusOptions?` |  |

#### `npc:end_status(status) → boolean`

Ends a status, as sc_end. Returns true when it was active. The target's stats change before it returns. The status_end event runs after the script's call returns. On a player it needs player.status.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` |  |

#### `npc:has_status(status) → boolean`

Whether a status is active, as getstatus.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` |  |

#### `npc:get_status(status) → wa.Status?`

An active status, or nil.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `status` | `integer` |  |

#### `npc:get_statuses() → wa.Status[]`

The active statuses, by status.

Permission: `game.read`.

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
| `rolls` | `table<string, integer>` | The copy's rolled values by name, such as rolls.boss_damage. In a card's script, the card's own rolls. Read-only. |
| `cards` | `wa.CardRolls[]` | The cards in the copy's slots, in slot order, with their rolls. |
| `random_options` | `wa.RandomOption[]` | The copy's random options, in option order. |

#### `equippedItem:set_roll(name, value)`

Sets one rolled value. The copy's equip scripts start again after the call, so they read the new value.

Permission: `items.modify_rolls`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string` | A roll of the item, or of the card for a card's copy. |
| `value` | `integer` | Clamped to the roll's range. |

#### `equippedItem:reroll(name?)`

Rolls values again, as a random give: from the roll's distribution when the item_rolls flag is on, the default otherwise, through the item_roll hooks with source kind "script".

Permission: `items.modify_rolls`.

| Parameter | Type | Meaning |
|---|---|---|
| `name` | `string?` | One roll. Every roll without it. |

### wa.CardRolls

A card in an equipped copy's slots.

| Field | Type | Meaning |
|---|---|---|
| `item` | `integer` | The card's item ID. |
| `rolls` | `table<string, integer>` | The card's rolled values by name. |

### wa.RandomOption

A random option of a copy, as rAthena's random options.

| Field | Type | Meaning |
|---|---|---|
| `option` | `string` | The option's name in data/random_options.yml, such as max_hp. |
| `value` | `integer` |  |
| `param` | `integer` |  |

### wa.ItemRoll

The object of the item_roll hook: one roll of a new copy. The innermost result is the server's roll: from the roll's distribution when the item_rolls flag is on and the copy is rolled, the default otherwise.

| Field | Type | Meaning |
|---|---|---|
| `item` | `integer` | The item's ID. |
| `name` | `string` | The roll's name. |
| `min` | `integer` |  |
| `max` | `integer` |  |
| `default` | `integer` |  |
| `source` | `wa.ItemRollSource` | Where the copy comes from. |

### wa.ItemRollSource

Where a rolled copy comes from: a monster's drop, a script, or an NPC shop.

| Field | Type | Meaning |
|---|---|---|
| `kind` | `"drop"\|"script"\|"shop"` |  |
| `monster` | `wa.Monster?` | The killed monster, for a drop. |

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

### wa.EventRun

A run of a typed world event, from context:get_event() (review decision 31). Its monsters and participants are those on the context's map.

#### `eventRun:get_monsters() → wa.Monster[]`

The run's monsters that live on the context's map: the boss, the invasion's monsters, or the treasure chests, in the order they spawned.

Permission: `game.read`.

#### `eventRun:get_participants() → wa.Player[]`

The players on the context's map who did at least the rewards' min_share of the damage to the run's monsters there, 1% by default, sorted by name.

Permission: `game.read`.

#### `eventRun:finish(result)`

Ends the run before its time, within a second. An invasion that finishes "won" gives its rewards, as when its last wave dies. world_event_end carries the result.

| Parameter | Type | Meaning |
|---|---|---|
| `result` | `"won"\|"lost"` |  |

### wa.Place

A named place of a generated map, from wa.places (review decision 42). A place never changes while the server runs.

#### `place:get_map() → string`

The map's ID, such as "gen:town:t7".

#### `place:get_position() → integer, integer`

The place's cell, x and y.

#### `place:get_tags() → string[]`

The place's tags, its main tag first, such as { "gate", "gate:south" }.

#### `place:get_area() → string`

The place's area: its map ID without gen:, such as "town:t7", "dungeon:crypt:2" or "field:3:-1".

#### `place:get_band() → integer, integer`

The level band of the place's map, its lowest and highest level (review decision 43). A place of a map without a band raises an error.

Permission: `game.read`.

### wa.GenerationNode

A generation layer's node of the seed tree's secret branch, its own for each mod and map (strategy section 8.2). It works only while the layer runs, and gives the same values for the same world each time.

#### `generationNode:child(label) → wa.GenerationNode`

A child node. Each label gives its own values, and the order of calls does not matter.

| Parameter | Type | Meaning |
|---|---|---|
| `label` | `string` | 1 to 64 bytes, such as "shrines". |

#### `generationNode:stream() → wa.GenerationStream`

A stream of numbers from the node. Two streams of one node give the same numbers.

#### `generationNode:hash(x, y) → integer`

A number for a cell, the same for the same node and cell, from all 64 bits. It is the first 8 bytes of HMAC-SHA256 of the cell, keyed by the node, so players cannot recover the node from it (review decision 54).

| Parameter | Type | Meaning |
|---|---|---|
| `x` | `integer` |  |
| `y` | `integer` |  |

### wa.GenerationStream

A stream of numbers of a generation node, from the node's secret ChaCha8 stream (strategy section 8.2).

#### `generationStream:integer(minimum, maximum) → integer`

The next whole number from minimum to maximum.

| Parameter | Type | Meaning |
|---|---|---|
| `minimum` | `integer` |  |
| `maximum` | `integer` |  |

#### `generationStream:chance(numerator, denominator) → boolean`

True with a chance of numerator in denominator.

| Parameter | Type | Meaning |
|---|---|---|
| `numerator` | `integer` |  |
| `denominator` | `integer` | Above 0. |

### wa.Town

A town of the world layout, from wa.towns (review decision 43). Two handles of one town are equal.

#### `town:get_id() → string`

The town's ID, such as "t7", which places.find's scope "town:t7" names.

Permission: `game.read`.

#### `town:get_name() → string`

The town's display name, such as "Valheim". It changes when staff rename the town.

Permission: `game.read`.

#### `town:get_map() → string`

The town's map ID, such as "gen:town:t7".

Permission: `game.read`.

#### `town:get_band() → integer, integer`

The town's level band: its lowest and highest level. Bands rise with the warps from the home area.

Permission: `game.read`.

### wa.TownListOptions

The options of towns.list.

| Field | Type | Meaning |
|---|---|---|
| `visited_by` | `wa.Player?` | Only the towns this player entered. |

### wa.Shop

A shop of data/npcs.yml, from world.get_shop (review decision 30). Its changes hold for the whole world until the server stops: the shop's file sets it again at start. A stock the methods set is saved.

#### `shop:add_item(item, options?) → boolean`

Adds an item, or replaces the item of that ID, as npcshopadditem. Without a cost the price is the item's buy price. Returns false when a cost names an item or a currency that does not exist.

Permission: `shops.modify`.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |
| `options` | `wa.ShopItemOptions?` |  |

#### `shop:remove_item(item) → boolean`

Takes an item out of the shop, as npcshopdelitem. Returns false when the shop does not sell it.

Permission: `shops.modify`.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |

#### `shop:set_stock(item, stock) → boolean`

Sets an item's stock, which every player shares, and the amount a restock fills it to, as npcshopupdate for a marketshop. Returns false when the shop does not sell the item.

Permission: `shops.modify`.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |
| `stock` | `integer` | 0 to 2147483647, or -1 for no limit. |

### wa.Channel

A chat channel, from channels.create or channels.get.

#### `channel:send(text)`

Shows a line of the channel itself to its members, as rAthena's channelmes. It does not pass the chat filter.

| Parameter | Type | Meaning |
|---|---|---|
| `text` | `string` | 1 to 255 bytes. |

#### `channel:add(player) → boolean`

Puts the player in the channel, whatever its join rule. Returns false when it was already in it.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `channel:remove(player) → boolean`

Takes the player out of the channel, even one it may not leave. Returns false when it was not in it.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `channel:get_members() → string[]`

The members' character names, sorted. Members may be on any map.

Permission: `game.read`.

### wa.Party

A party, from player.party or wa.parties. Its leader and members are players of this map: a member on another map is nil or left out.

| Field | Type | Meaning |
|---|---|---|
| `id` | `integer` |  |
| `name` | `string` |  |
| `leader` | `wa.Player?` | The leader, or nil when it is offline or on another map. |
| `leader_name` | `string` | The leader's name, online or not. |
| `exp_share` | `"each"\|"even"` | Even shares the exp of a kill among the members on the map, as rAthena's even share. |
| `item_share` | `"each"\|"even"` | Even gives each picked up item to a member on the map. |
| `item_pickup` | `"each"\|"shared"` | Shared lets members pick up the items their party's top attackers may take. |

#### `party:get_members() → wa.Player[]`

The online members on this map, in the party's slot order.

Permission: `game.read`.

#### `party:get_member_names() → string[]`

Every member's name, online or not, in slot order.

Permission: `game.read`.

#### `party:add(player) → boolean`

Puts the player in the party, as party_addmember. Returns false when the player has a party, the party is full, or a character of the same account is in it. The party_join event follows, with the reason "script".

Permission: `party.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `party:remove(player) → boolean`

Takes the player out of the party, as party_delmember. The leader's removal breaks the party up, as in rAthena. Returns false when the player is not in the party.

Permission: `party.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `party:set_leader(player) → boolean`

Makes an online member the leader, as party_changeleader. Returns false when the player is not an online member, or leads already. The party_change event follows.

Permission: `party.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `party:set_options(options)`

Changes the options it names, as party_changeoption. Even exp share needs the online members' base levels within the ruleset's party_share_level, else exp share stays each. The party_change event follows a change.

Permission: `party.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `options` | `wa.PartyOptions` |  |

#### `party:disband()`

Breaks the party up, as party_destroy. Each member gets the party_leave event, with the reason "disbanded".

Permission: `party.manage`.

### wa.PartyOptions

A party's options, in party:set_options and the party_change event.

| Field | Type | Meaning |
|---|---|---|
| `exp_share` | `"each"\|"even"\|nil` |  |
| `item_share` | `"each"\|"even"\|nil` |  |
| `item_pickup` | `"each"\|"shared"\|nil` |  |

### wa.PartyMembership

The object of the party_join and party_leave events.

| Field | Type | Meaning |
|---|---|---|
| `party` | `wa.Party` | The party. After it broke up, its fields still read. |
| `player` | `wa.Player` | The player who joined or left. |
| `reason` | `"invited"\|"create"\|"left"\|"expelled"\|"disbanded"\|"script"` | Why: an accepted invite, a party the player made from the client, a leave, an expel, a break up, or a wa.parties call. |

### wa.PartyChange

The object of the party_change event.

| Field | Type | Meaning |
|---|---|---|
| `party` | `wa.Party` |  |
| `leader` | `wa.Player?` | The leader, or nil when it is on another map. |
| `options` | `wa.PartyOptions` | The party's options after the change. |

### wa.Guild

A guild, from player.guild or wa.guilds. Its master and members are players of this map: a member on another map is nil or left out.

| Field | Type | Meaning |
|---|---|---|
| `id` | `integer` |  |
| `name` | `string` |  |
| `master` | `wa.Player?` | The master, or nil when it is offline or on another map. |
| `master_name` | `string` | The master's name, online or not. |
| `level` | `integer` | 1 to 50, from exp_guild.yml. |
| `exp` | `integer` | The exp toward the next level. |
| `notice` | `wa.GuildNotice` |  |
| `member_count` | `integer` | Every member, online or not. |

#### `guild:get_members() → wa.Player[]`

The online members on this map, in the guild's slot order. The master is first.

Permission: `game.read`.

#### `guild:get_member_names() → string[]`

Every member's name, online or not, in slot order.

Permission: `game.read`.

#### `guild:get_position(player) → wa.GuildPosition?`

The member's position, or nil for a player who is not a member.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `guild:get_skill_level(skill) → integer`

The guild's level of a guild skill, as getgdskilllv.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `skill` | `integer` | A guild skill's ID, such as skills.id("GD_EXTENSION"). |

#### `guild:count_item(item) → integer`

How many of the item the guild storage holds, as guildstoragecountitem.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |

#### `guild:is_allied(other) → boolean`

Whether the guilds are allies.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `other` | `wa.Guild` |  |

#### `guild:is_enemy(other) → boolean`

Whether this guild has the other as an enemy. Enemies are on one side only.

Permission: `game.read`.

| Parameter | Type | Meaning |
|---|---|---|
| `other` | `wa.Guild` |  |

#### `guild:add(player) → boolean`

Puts the player in the guild with the last position. Returns false when the player has a guild or the guild is full. The guild_join event follows, with the reason "script".

Permission: `guild.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `guild:remove(player) → boolean`

Takes a member out of the guild. Returns false for a player who is not a member, and for the master. The guild_leave event follows, with the reason "script".

Permission: `guild.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `guild:set_master(player) → boolean`

Makes a member the master, as guildchangegm, without guild_leaderchange_delay. Returns false for a player who is not a member, or is the master. The guild_change event follows.

Permission: `guild.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `guild:set_notice(title, text) → boolean`

Sets the guild notice. Returns false when a part is too long. The guild_change event follows.

Permission: `guild.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `title` | `string` | At most 59 characters. |
| `text` | `string` | At most 119 characters. |

#### `guild:give_exp(amount)`

Adds exp to the guild, as guildgetexp, which may level it up. The guild_change event follows a level up.

Permission: `guild.manage`.

| Parameter | Type | Meaning |
|---|---|---|
| `amount` | `integer` | 1 to 2147483647. |

#### `guild:disband()`

Breaks the guild up, with its members. Its storage is lost, as in rAthena. Each member gets the guild_leave event, with the reason "disbanded".

Permission: `guild.manage`.

### wa.GuildNotice

A guild's notice.

| Field | Type | Meaning |
|---|---|---|
| `title` | `string` |  |
| `text` | `string` |  |

### wa.GuildPosition

A member's position, from guild:get_position.

| Field | Type | Meaning |
|---|---|---|
| `name` | `string` |  |
| `can_invite` | `boolean` |  |
| `can_expel` | `boolean` |  |
| `can_use_storage` | `boolean` |  |
| `exp_tax` | `integer` | The percent of base exp the member gives the guild. |

### wa.GuildMembership

The object of the guild_join and guild_leave events.

| Field | Type | Meaning |
|---|---|---|
| `guild` | `wa.Guild` | The guild. After it broke up, its fields still read. |
| `player` | `wa.Player` | The player who joined or left. |
| `reason` | `"invited"\|"create"\|"left"\|"expelled"\|"disbanded"\|"script"` | Why: an accepted invite, a guild the player made from the client, a leave, an expel, a break up, or a wa.guilds call. |

### wa.GuildChange

The object of the guild_change event.

| Field | Type | Meaning |
|---|---|---|
| `guild` | `wa.Guild` | The guild, whose fields hold the new values. |
| `change` | `"master"\|"level"\|"notice"\|"emblem"` | What changed. |

### wa.Limits

The limits of the world's ruleset, as rules.limits: each limit's base, what a player or guild starts with. A list over a limit that a ruleset lowered keeps its entries and cannot grow. The engine's ceiling bounds each limit's max.

| Field | Type | Meaning |
|---|---|---|
| `party_members` | `integer` | The most members a party holds, as MAX_PARTY. 12 by default, ceiling 256. |
| `friends` | `integer` | The most names a friend list holds, as MAX_FRIENDS. 40 by default, ceiling 1,000. |
| `block_list` | `integer` | The most names a block list holds, as MAX_IGNORE_LIST. 20 by default, ceiling 1,000. |
| `guild_members` | `integer` | The most members a guild holds before Guild Extension, which adds 6 per level up to the max, 76 by default as MAX_GUILD. 16 by default, ceiling 512. |
| `guild_positions` | `integer` | The positions of a guild, as MAX_GUILDPOSITION. 20 by default, ceiling 40. |
| `guild_allies` | `integer` | The most allied guilds of a guild, as max_guild_alliance. 3 by default, ceiling 10. |
| `guild_enemies` | `integer` | The most enemy guilds of a guild, as max_guild_alliance. 3 by default, ceiling 10. |
| `guild_storage` | `integer` | The slots of a guild storage without renewal's Guild Storage skill, as MAX_GUILD_STORAGE. The skill gives 100 and 100 per level, up to the max. 600 by default, ceiling 10,000. |
| `inventory_slots` | `integer` | The inventory slots of a new character, as INVENTORY_BASE_SIZE. 100 by default, with the max 200, MAX_INVENTORY. Ceiling 1,000. |
| `storage_slots` | `integer` | The slots of an account's storage, as MAX_STORAGE. 600 by default, ceiling 10,000. |
| `cart_slots` | `integer` | The slots of a cart, as MAX_CART. 100 by default, ceiling 1,000. |
| `mail_items` | `integer` | The most item stacks one mail holds, as MAIL_MAX_ITEM. 5 by default, ceiling 20. |
| `mailbox` | `integer` | The most mails a mailbox holds. 100 by default, ceiling 1,000. |

### wa.ExpShare

The share of the experience hook's object.

| Field | Type | Meaning |
|---|---|---|
| `base` | `integer` |  |
| `job` | `integer` |  |

### wa.ChannelSpec

The channel of channels.create.

| Field | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 19 letters, digits, _ and -, with or without the #. Players see it as #name. Not public, whisper, party or guild. |
| `display_name` | `string?` | What players see before each line, 1 to 24 characters, as rAthena's alias. [name] by default. |
| `color` | `integer?` | The lines' color, 0xRRGGBB. White by default. |
| `join` | `"open"\|"invite"\|nil` | "open", the default: any player may join. "invite": only channel:add puts players in. |

### wa.WhisperOptions

The options of player:whisper.

| Field | Type | Meaning |
|---|---|---|
| `from` | `string?` | The name the player sees, 1 to 24 characters. The NPC's name in an NPC context, else the mod's name. |

### wa.ChatMessage

The object of the chat event and the chat hook: a line a player says. The hook runs after the server's chat filter, and the event after the line went out.

| Field | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | The speaker. |
| `channel` | `string` | "public" for local chat, "whisper", "party", "guild", or a channel's name without its #, such as trade. |
| `text` | `string` | The line, after the chat filter. |
| `to` | `string?` | A whisper's target name. nil for other channels. |
| `map` | `string` | The speaker's map. |

### wa.ChatOptions

The options of a chat handler. Each one narrows the lines that run it.

| Field | Type | Meaning |
|---|---|---|
| `channel` | `string?` | "public", "whisper", "party", "guild" or a channel's name, with or without its #. |
| `map` | `string?` | A map name. |
| `player` | `wa.Player?` | One speaker. |

### wa.ChatTestOptions

The options of world:chat.

It exists only in tests that `wactl mod test` runs.

| Field | Type | Meaning |
|---|---|---|
| `channel` | `string?` | "public", the default, "whisper", or a channel's name. |
| `to` | `string?` | A whisper's target name. |

### wa.HeardLine

A chat line a test player got, from world:get_chat.

It exists only in tests that `wactl mod test` runs.

| Field | Type | Meaning |
|---|---|---|
| `channel` | `string` | "public", "whisper", "system", or a channel's name with its #. |
| `from` | `string` | The speaker's name, or empty for a system line or a channel's own line. |
| `text` | `string` |  |

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

#### `testWorld:equip(player, item, options?) → wa.EquippedItem`

Gives the player one copy of the item and puts it on. Returns the equipped copy, as player:get_equipped. Fails when the copy does not fit or cannot be equipped, such as by the player's job or level.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `item` | `integer` | An item from items.id. |
| `options` | `wa.EquipOptions?` |  |

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

#### `testWorld:chat(player, text, options?)`

The player says a line, as a client's chat: it passes the mute, the chat filter and the chat hooks, then the chat event runs.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `text` | `string` | 1 to 255 characters. |
| `options` | `wa.ChatTestOptions?` |  |

#### `testWorld:get_chat(player) → wa.HeardLine[]`

The chat lines the player got so far, oldest first: local chat, whispers, channel lines and system lines.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `testWorld:trade(a, b, offer_a?, offer_b?) → boolean, string?`

Runs a whole trade, as two clients would: a asks, b accepts, both offer, lock and confirm. Returns true when it completed, or false and why: the trade hook's message, or an outcome such as "restricted" or "bound". A refused offer ends the trade. Trading needs Basic Skill level 1, which a new player from world:new_player lacks: give it skills = { [skills.id("NV_BASIC")] = 9 }.

| Parameter | Type | Meaning |
|---|---|---|
| `a` | `wa.Player` |  |
| `b` | `wa.Player` | Steps next to a. |
| `offer_a` | `wa.TradeOffer?` |  |
| `offer_b` | `wa.TradeOffer?` |  |

#### `testWorld:close_storage(player)`

Closes the player's storage window, as the client's close button.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `testWorld:set_staff_role(player, role?)`

Gives the player's account a staff role, as if it logged in with two-factor login, so player:get_staff_role and player:has_staff_permission see it.

Provisional: not reviewed in proposal 0002 yet.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `role` | `string?` | A role of the default staff roles: admin, game_master or moderator. nil takes the role away. |

#### `testWorld:buy(player, shop, items) → boolean, string?`

Opens a shop for the player, as player:open_shop, and buys, as the shop window's Buy. Returns true when the purchase went through, or false and the outcome, such as "out_of_stock", "not_enough_items" or "not_enough_currency".

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |
| `shop` | `string` | A shop's full record name of data/npcs.yml, such as example.arena:Prize Desk. |
| `items` | `table<integer, integer>` | Item IDs and the amount of each to buy. |

#### `testWorld:plan_restart(minutes, reason)`

Plans a restart that many minutes from the test's time, as staff do on the admin screens: restart_planned runs with { at, reason } in the world's zone, and the lines its handlers send go out. Nothing restarts.

Provisional: not reviewed in proposal 0002 yet.

| Parameter | Type | Meaning |
|---|---|---|
| `minutes` | `integer` | 1 to 10080, as the admin screens allow. |
| `reason` | `string` | 1 to 255 characters. |

#### `testWorld:rotate(dungeon) → string`

Rotates a dungeon at once, as staff force a rotation: its floors take the date of its style's next rotation after the world's time, or after the date it rotated to before, with new layouts and spawns. The points on the old floors move to the nearest town's save spot, and dungeon_rotated runs with { dungeon, date } in the world's zone. Returns the date, such as "2026-10-23". A case that runs on one of the dungeon's floors cannot rotate it, since a test world runs one map.

Provisional: not reviewed in proposal 0002 yet.

| Parameter | Type | Meaning |
|---|---|---|
| `dungeon` | `string` | A dungeon of the test world, by its ID: its style's name without the mod prefix, such as crypt. Its style must have a rotation. |

#### `testWorld:change_region(x, y) → string, integer`

Changes the region of a field at once, as staff force a world change: its fields generate again with their next season and a new biome, with new spawns. The points on them move to the region's town's save spot, and region_changed runs with { region, season } in the world's zone. Returns the region, its town's ID such as "t1", and its new season. A case that runs on one of the region's fields cannot change it.

Provisional: not reviewed in proposal 0002 yet.

| Parameter | Type | Meaning |
|---|---|---|
| `x` | `integer` | A field's x on the grid. The test world's field is 0, 0. |
| `y` | `integer` | A field's y on the grid. |

#### `testWorld:get_mails(player) → wa.TestMail[]`

The player's mails, oldest first, after the mail sent so far arrived.

| Parameter | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | A player handle. |

#### `testWorld:vend(seller, buyer, lines) → boolean, string?`

Runs a whole vending sale, as two clients would: the seller puts each line's items in its cart and opens a shop, and the buyer buys. The shop closes after it. Returns true when the sale went through, or false and why: the vend hook's message, or an outcome such as "not_enough_zeny".

| Parameter | Type | Meaning |
|---|---|---|
| `seller` | `wa.Player` | Gets a cart and the Vending skill when it lacks them. |
| `buyer` | `wa.Player` | Steps next to the seller. |
| `lines` | `wa.TestVendLine[]` |  |

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

### wa.JobChange

The object of the job_change event.

| Field | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | The player that changed jobs. |
| `from` | `integer` | The job before the change. |
| `to` | `integer` | The new job. A male player who changes into a Dancer is a Bard here. |

### wa.QuestEvent

The object of the quest_add, quest_progress and quest_complete events.

| Field | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` | The player whose quest log changed. |
| `quest` | `string` | The quest record's full name, such as example.hunts:wolves, or a core quest's ID as text, such as "7104". |
| `id` | `integer` | The quest's ID. |
| `objective` | `integer?` | quest_progress: the hunt objective that counted a kill, from 1. |
| `count` | `integer?` | quest_progress: the objective's kills now. |
| `goal` | `integer?` | quest_progress: the kills the objective asks for. |

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

The object of the experience hook: the exp one player gets for a kill, after the shares, the even party split and the level penalty. With even exp share, each member on the map gets its own hook run.

| Field | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` |  |
| `monster` | `wa.Monster` | The killed monster. |
| `base` | `integer` |  |
| `job` | `integer` |  |
| `party` | `wa.Party?` | The player's party, or nil. |
| `share` | `wa.ExpShare` | The player's own share of the kill, before the even party split and the level penalty. 0 and 0 for a member who did not fight. |
| `guild` | `wa.Guild?` | The player's guild, or nil. |
| `guild_tax` | `integer` | The share of base that goes to the guild, by the exp tax of the player's position. The tax applies at the same rate to the base the hook returns. |

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
| `respawn_milliseconds` | `integer?` | With respawn, how long a dead monster takes to come back, 1000 to 268435455. 5000 by default, as a spawn line without a delay. |

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

The object of the skill_used event: a skill that took effect, after its cast time.

| Field | Type | Meaning |
|---|---|---|
| `caster` | `wa.Player\|wa.Monster\|wa.Npc` | The wearer, for an item's handler. caster.kind says which. |
| `skill` | `integer` |  |
| `level` | `integer` |  |
| `target` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | nil for a ground skill. |
| `x` | `integer` | The cell the skill took effect at. |
| `y` | `integer` |  |

### wa.SkillUsedOptions

The options of a skill_used handler. Each one narrows the skills that run it.

| Field | Type | Meaning |
|---|---|---|
| `skill` | `integer?` | A skill from skills.id. |
| `caster_kind` | `"player"\|"monster"\|"npc"\|nil` |  |
| `map` | `string?` | A map name. |

### wa.Cast

The object of the cast event: one cast of the skill, when it takes effect. It stays usable after the event, so timers of the skill's context can use it.

| Field | Type | Meaning |
|---|---|---|
| `caster` | `wa.Player\|wa.Monster\|wa.Npc` | caster.kind says which. |
| `skill` | `integer` |  |
| `level` | `integer` |  |
| `target` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | nil for a ground skill. |
| `map` | `string` |  |
| `x` | `integer` | The cell the skill takes effect at: the target's, or the ground cell. |
| `y` | `integer` |  |

#### `cast:deal_damage(target, options?) → wa.CastDamageResult?`

Runs rAthena's damage calculation of the skill on the target, with percent as the skill's damage percentage, after the skill_damage hooks. It applies cards and defense, and fires the damage event after the script's call. Returns nil, and does nothing, when the caster is gone or cannot act, as rAthena's skill_addtimerskill.

| Parameter | Type | Meaning |
|---|---|---|
| `target` | `wa.Player\|wa.Monster\|wa.Npc` |  |
| `options` | `wa.CastDamageOptions?` |  |

#### `cast:heal(target, amount) → integer?`

Heals the target with rAthena's heal bonuses, and returns the HP healed. Returns nil, and does nothing, when the caster is gone or cannot act.

| Parameter | Type | Meaning |
|---|---|---|
| `target` | `wa.Player\|wa.Monster\|wa.Npc` |  |
| `amount` | `integer` | 0 or more. |

#### `cast:run_default(options?)`

Runs core's own behavior of the skill. It raises an error for a skill that has none, such as a mod's skill.

| Parameter | Type | Meaning |
|---|---|---|
| `options` | `wa.RunDefaultOptions?` |  |

### wa.CastDamageOptions

The options of cast:deal_damage. Without one, the skill's data gives the value.

| Field | Type | Meaning |
|---|---|---|
| `percent` | `integer?` | The skill's damage percentage, 0 or more. 100 by default. |
| `hits` | `integer?` | The number of hits, 1 to 30. |
| `type` | `"weapon"\|"magic"\|"misc"\|nil` | The attack type. |
| `element` | `integer?` | An element from wa.elements, not all. |

### wa.CastDamageResult

What cast:deal_damage returns.

| Field | Type | Meaning |
|---|---|---|
| `amount` | `integer` | The damage dealt. |
| `is_critical` | `boolean` |  |
| `missed` | `boolean` |  |

### wa.RunDefaultOptions

The options of cast:run_default.

| Field | Type | Meaning |
|---|---|---|
| `target` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | Another target. The cast's target by default. |

### wa.SkillInstance

The instance of a skill's context: the skill, once per zone. Its cast handlers run for every cast.

| Field | Type | Meaning |
|---|---|---|
| `skill` | `integer` | The skill's ID. |

### wa.PassiveSkillInstance

The instance of a passive skill's context: one owner's skill, while it is learned. A new context starts when the level changes.

| Field | Type | Meaning |
|---|---|---|
| `skill` | `integer` | The skill's ID. |
| `level` | `integer` | The learned level. |
| `owner` | `wa.Player` | Who learned the skill. |

### wa.SkillHook

The object of the skill_damage, cast_time, skill_cost and cast_allowed hooks. The innermost result is rAthena's value.

| Field | Type | Meaning |
|---|---|---|
| `caster` | `wa.Player\|wa.Monster\|wa.Npc` |  |
| `target` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | nil for a ground skill, and for skill_cost. |
| `skill` | `integer` |  |
| `level` | `integer` |  |
| `percent` | `integer?` | skill_damage: the damage percentage. |
| `milliseconds` | `integer?` | cast_time: the cast time, after DEX and the cast bonuses. |
| `sp` | `integer?` | skill_cost: the SP cost. |
| `allowed` | `boolean?` | cast_allowed: whether rAthena lets the cast start. |

### wa.MonsterTargetChoice

The object of the monster_target hook: a monster's AI chooses a target. The innermost result is current, rAthena's choice.

| Field | Type | Meaning |
|---|---|---|
| `monster` | `wa.Monster` | The monster that chooses. |
| `candidates` | `(wa.Player\|wa.Monster\|wa.Npc)[]` | The units it may take. The hook returns one of them, or nil. |
| `current` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | rAthena's choice: the target the monster takes without hooks. |
| `reason` | `"aggressive"\|"assist"\|"hit"\|"change"` | Why it chooses: it looks for a target, a monster of its kind calls for help, a unit hit it, or it changes chase target. |

### wa.EntityQuery

A query of maps.find_entities. It has one area: around with radius, x and y with radius, rectangle, line, or cells. The other fields filter and sort.

| Field | Type | Meaning |
|---|---|---|
| `around` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | The area is the cells within radius of the entity. The entity is in its own area: exclude leaves it out. |
| `map` | `string?` | The context's own map, which is the default. Another map raises an error. |
| `x` | `integer?` | With y, the area is the cells within radius of the cell. |
| `y` | `integer?` |  |
| `radius` | `integer?` | 0 to 30 cells, with around or x and y. |
| `shape` | `"square"\|"circle"\|nil` | With around or x and y. "square" by default, as rAthena's ranges. |
| `rectangle` | `integer[][]?` | Two opposite corners, such as { { 100, 100 }, { 110, 105 } }. Sides of at most 61 cells. |
| `line` | `wa.QueryLine?` | The cells along a line. |
| `cells` | `integer[][]?` | A list of 1 to 1024 cells, such as { { 100, 100 }, { 101, 100 } }. |
| `kinds` | `("player"\|"monster"\|"npc")[]?` | Keeps these kinds. Every kind by default. |
| `alive` | `boolean?` | true, the default, keeps living entities; false keeps dead ones. |
| `enemies_of` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | Keeps the entities this one may attack, by rAthena's rules. |
| `allies_of` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | Keeps the entities this one may help, by rAthena's rules, with itself. |
| `line_of_sight` | `boolean?` | Keeps the entities that the center of the area sees: the entity of around, the cell, the middle of the rectangle, the start of the line, or the first cell. |
| `exclude` | `(wa.Player\|wa.Monster\|wa.Npc)[]?` | Leaves these out. |
| `sort` | `"nearest"\|"farthest"\|"lowest_hp"\|"random"\|nil` | Distances are from the center of the area. "random" draws from the zone's secret stream. |
| `limit` | `integer?` | 1 to 1000. 1000 by default. |

### wa.QueryLine

The line of a query.

| Field | Type | Meaning |
|---|---|---|
| `from` | `integer[]` | A cell, such as { 100, 100 }. |
| `to` | `integer[]` | A cell, at most 61 cells away on each axis. |
| `width` | `integer?` | 1 to 15 cells. 1 by default. |

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
| `roll` | `"random"\|"default"\|table<string, integer>\|nil` | How the copies' rolls are set: "random" rolls each value, "default" (the default) uses the defaults, and a table sets values, each clamped to its range. A table needs items.modify_rolls, except in a test case's own code. |
| `to` | `string?` | Where the copies go: "storage", "cart" or an extra storage's name, instead of the inventory. They follow the place's restrictions and bound rules. |
| `bound` | `wa.Bound?` | Binds the copies, as getitembound. A bound copy cannot be traded or dropped, and goes only into storage its owner shares. |

### wa.TestMail

A mail as world:get_mails shows it.

It exists only in tests that `wactl mod test` runs.

| Field | Type | Meaning |
|---|---|---|
| `from` | `string` |  |
| `title` | `string` |  |
| `zeny` | `integer` |  |
| `items` | `table<integer, integer>` | Item IDs and amounts. |
| `read` | `boolean` |  |
| `returned` | `boolean` | Set for a mail that came back to its sender. |

### wa.TestVendLine

One line of world:vend.

It exists only in tests that `wactl mod test` runs.

| Field | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item the seller holds. |
| `amount` | `integer` | How many go on sale. |
| `price` | `integer` | The price of one. |
| `buy` | `integer?` | How many the buyer buys. None by default. |

### wa.TradeOffer

One side of world:trade.

| Field | Type | Meaning |
|---|---|---|
| `items` | `table<integer, integer>?` | Item IDs and the amount of each to offer. |
| `zeny` | `integer?` |  |

### wa.FromOptions

Options of player:count_item and player:take_item.

| Field | Type | Meaning |
|---|---|---|
| `from` | `string` | "storage", "cart" or an extra storage's name of data/storages.yml. |

### wa.WorldEvent

The object of world_event_start and world_event_end (review decision 31).

| Field | Type | Meaning |
|---|---|---|
| `name` | `string` | The event's full name, such as core:Field_Boss. |
| `type` | `"boss"\|"invasion"\|"treasure_hunt"\|"script"` | The event's type: "script" for an event without one. |
| `result` | `"won"\|"lost"\|"stopped"\|nil` | How the run ended, at world_event_end. A boss is won when it dies, an invasion when its last wave dies, and a treasure hunt when every chest is opened. A run whose time ran out is lost. nil at world_event_start, and for a script event that ran its time. |

### wa.RestartPlan

The object of restart_planned (review decision 36): { at, reason } when staff plan a restart, and { cancelled = true } when they cancel it.

| Field | Type | Meaning |
|---|---|---|
| `at` | `integer?` | When the server restarts, in Unix seconds. nil when the restart was cancelled. |
| `reason` | `string?` | Why the server restarts, as staff wrote it. nil when the restart was cancelled. |
| `cancelled` | `boolean?` | true when staff cancelled the planned restart. |

### wa.DungeonRotation

The object of dungeon_rotated (review decision 47).

| Field | Type | Meaning |
|---|---|---|
| `dungeon` | `string` | The dungeon's ID, such as crypt. |
| `date` | `string` | The date its new floors carry in their map IDs, such as 2026-10-31: their map IDs are gen:dungeon:<dungeon>:<date>:<floor>. |

### wa.RegionChange

The object of region_changed (review decision 47).

| Field | Type | Meaning |
|---|---|---|
| `region` | `string` | The region, by its town's ID, such as t2: the fields whose nearest town is that town. |
| `season` | `integer` | The region's new season, from 1. |

### wa.GenerationMap

The map of a generation layer: a generated field, town or dungeon floor of a new procedural world (review decision 46). It is read-only.

| Field | Type | Meaning |
|---|---|---|
| `id` | `string` | The map ID, such as gen:field:3:1, gen:town:t2 or gen:dungeon:crypt:2. |
| `kind` | `"field"\|"town"\|"dungeon"` |  |
| `biome` | `string` | A field's biome, such as forest, or a town's or a floor's tileset, such as crypt. |
| `tileset` | `string` | The tileset whose props objects name. |
| `band` | `integer[]` | The level band: the lowest and the highest level. |
| `width` | `integer` | The map's width in cells. |
| `height` | `integer` | The map's height in cells. |
| `places` | `wa.GenerationPlace[]` | The map's generated places, in the generator's order. |

### wa.GenerationPlace

A named place of a generation layer's map, or one a layer returns: its tags, the main tag first, and its cell. The server moves a returned place to the nearest open ground within 8 cells, or leaves it out.

| Field | Type | Meaning |
|---|---|---|
| `tags` | `string[]` | 1 to 8 tags of 1 to 64 lowercase letters, digits, _, -, . and :. |
| `x` | `integer` |  |
| `y` | `integer` |  |

### wa.GenerationObject

An object a generation layer puts on its map. Clients draw it from the map's info. It does not block the cell.

| Field | Type | Meaning |
|---|---|---|
| `model` | `string` | A prop of the map's tileset, such as rock or well. |
| `x` | `integer` | A cell of the map. |
| `y` | `integer` |  |
| `turn` | `integer?` | 0 to 3 quarter turns. 0 by default. |

### wa.GenerationBoss

A boss a generation layer adds to its field's or dungeon floor's spawns, one monster. The server keeps it 30 cells from warps to towns.

| Field | Type | Meaning |
|---|---|---|
| `monster` | `integer` | A monster from monsters.id. |
| `role` | `"mini_boss"\|"field_boss"` |  |
| `x` | `integer?` | Its cell, with y: it appears within 3 cells of the nearest open ground. Left out, anywhere on the map. |
| `y` | `integer?` |  |
| `respawn` | `integer?` | Milliseconds before it comes back, 1,000 to 268,435,455. 45 minutes for a mini boss and 90 for a field boss by default. |

### wa.GenerationResult

What a generation layer adds to its map. Each list is optional.

| Field | Type | Meaning |
|---|---|---|
| `places` | `wa.GenerationPlace[]?` | At most 32. |
| `objects` | `wa.GenerationObject[]?` | At most 256. |
| `bosses` | `wa.GenerationBoss[]?` | At most 8. |

### wa.SpawnMap

The map of the spawn_table hook: a field or a dungeon floor of a new procedural world (review decision 44). It is read-only.

| Field | Type | Meaning |
|---|---|---|
| `id` | `string` | The map ID, such as gen:field:3:1 or gen:dungeon:crypt:2. |
| `kind` | `"field"\|"dungeon"` |  |
| `biome` | `string` | A field's biome, such as forest, or a floor's tileset, such as crypt. |
| `band` | `integer[]` | The level band: the lowest and the highest level. |
| `features` | `string[]` | The map's features, sorted: water and clearing on fields, room and boss_room on floors. |

### wa.Spawn

One spawn of the spawn_table hook's list. The server keeps aggressive monsters 15 cells from arrivals, and bosses 30 cells from warps to towns, whatever the hook returns.

| Field | Type | Meaning |
|---|---|---|
| `monster` | `integer` | A monster from monsters.id. |
| `count` | `integer` | 1 to 1,000 monsters. |
| `respawn` | `integer` | Milliseconds before a dead one comes back, 1,000 to 268,435,455. |
| `role` | `"common"\|"uncommon"\|"plant"\|"rare"\|"mini_boss"\|"field_boss"` | A boss monster that is common or uncommon is left out. |
| `area` | `wa.SpawnArea?` | Where the monsters appear, or nil for the whole map. |

### wa.SpawnArea

A spawn's area: within w and h cells of x and y.

| Field | Type | Meaning |
|---|---|---|
| `x` | `integer` | 0 to 1,024. |
| `y` | `integer` |  |
| `w` | `integer` |  |
| `h` | `integer` |  |

### wa.Trade

The object of the trade event and the trade hook: a trade between two players. The hook runs before the trade completes, and the event after.

| Field | Type | Meaning |
|---|---|---|
| `players` | `wa.Player[]` | The two players: the one who confirmed last first. |
| `items` | `wa.TradedItem[]` | Each offered item. |
| `zeny` | `integer[]` | The zeny each player gave, in the order of players. |

### wa.MailSpec

A mail of player:send_mail (review decision 30).

| Field | Type | Meaning |
|---|---|---|
| `to` | `string\|integer\|wa.Player\|nil` | The recipient: a character's name, a character ID, or a player handle. The player itself by default. |
| `sender` | `string?` | The name shown as the sender, at most 23 characters. By default the NPC's name in an NPC context, else the mod's name. |
| `title` | `string` | 1 to 39 bytes. |
| `body` | `string?` | At most 499 bytes. |
| `zeny` | `integer?` | 0 to 2147483647. |
| `items` | `wa.MailItem[]?` | At most mail_items attachments. The items are made for the mail. |

### wa.MailItem

One attachment of player:send_mail.

| Field | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |
| `amount` | `integer?` | 1 to 30000. 1 by default. |
| `roll` | `"random"\|"default"\|table<string, integer>\|nil` | As give_item's roll. A table needs items.modify_rolls, except in a test case's own code. |
| `bound` | `wa.Bound?` | Binds the copies, as give_item's bound. |

### wa.StoreSearchSpec

The store search of player:search_stores.

| Field | Type | Meaning |
|---|---|---|
| `uses` | `integer` | How many searches the window allows, 1 to 1000. |
| `effect` | `"show"\|"open"\|nil` | What a click on a result does: "show" (the default) shows the store's place, as SEARCHSTORE_EFFECT_NORMAL; "open" opens the store, as SEARCHSTORE_EFFECT_REMOTE, when it is on the player's map. |

### wa.BuyingStoreSpec

The buying store of player:open_buying_store.

| Field | Type | Meaning |
|---|---|---|
| `slots` | `integer` | How many items the store may buy, 1 to 5, as MAX_BUYINGSTORE_SLOTS. |

### wa.ShopItemOptions

Options of shop:add_item.

| Field | Type | Meaning |
|---|---|---|
| `cost` | `wa.ShopCost?` | The cost of one. Without it, the item's buy price in zeny. |
| `stock` | `integer?` | How many the shop has for every player, 1 to 2147483647. No limit by default. |
| `restock` | `string?` | When the stock fills again: "daily", "weekly", or a time such as "6h", "30m" or "2d". It needs a stock. |

### wa.ShopCost

A cost: zeny, items and currencies, each optional, at most 5 items and currencies together.

| Field | Type | Meaning |
|---|---|---|
| `zeny` | `integer?` | 0 to 2147483647. Discount lowers it when the shop allows it. |
| `items` | `wa.ShopCostItem[]?` | Items the purchase takes, for one bought. |
| `currency` | `table<string, integer>?` | Currencies of data/currencies.yml and the amount of each, for one bought, such as { cash = 100 }. |

### wa.ShopCostItem

An item a cost takes.

| Field | Type | Meaning |
|---|---|---|
| `item` | `integer` | An item from items.id. |
| `amount` | `integer?` | 1 to 30000. 1 by default. |

### wa.ShopSale

The object of the shop_buy and shop_sell events: a purchase from a shop, or a sale to it.

| Field | Type | Meaning |
|---|---|---|
| `player` | `wa.Player` |  |
| `shop` | `string` | The shop's full record name. |
| `items` | `wa.SoldItem[]` | Each line. |
| `zeny` | `integer` | The zeny the player paid, or got for a sale. |

### wa.StoreSale

The object of the vend and buying_store_sale events and hooks. The hook runs before the sale, and the event after.

| Field | Type | Meaning |
|---|---|---|
| `seller` | `wa.Player` | The player who gives the items: the vendor, or the player who sells to a buying store. |
| `buyer` | `wa.Player` | The player who pays: the vendor's customer, or the buying store's owner. |
| `items` | `wa.SoldItem[]` | Each line, with its price. |
| `zeny` | `integer` | The zeny the buyer pays, before the vending tax. |

### wa.Mail

The object of the mail event and hook. The hook runs before a player's mail goes out, and the event after any mail, a script's too.

| Field | Type | Meaning |
|---|---|---|
| `from` | `string` | The sender's name. |
| `to` | `string` | The recipient's name. |
| `items` | `wa.SoldItem[]` | The attachments. |
| `zeny` | `integer` | The attached zeny. |

### wa.SoldItem

One line of a sale or a mail.

| Field | Type | Meaning |
|---|---|---|
| `item` | `integer` | The item's ID. |
| `amount` | `integer` |  |
| `price` | `integer?` | The price of one, for vend and buying_store_sale. |

### wa.TradedItem

One item of a trade.

| Field | Type | Meaning |
|---|---|---|
| `from` | `wa.Player` | The player who gave it. |
| `item` | `integer` | The item's ID. |
| `amount` | `integer` |  |

### wa.TradeOptions

The options of a trade handler.

| Field | Type | Meaning |
|---|---|---|
| `player` | `wa.Player?` | Only trades of this player. |

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

### wa.StatusOptions

The options of entity:start_status.

| Field | Type | Meaning |
|---|---|---|
| `milliseconds` | `integer\|"forever"\|nil` | The duration, 1 to 2,147,483,647, or forever. Without it, the duration rAthena's sc_start looks up for the level, from the status's skill. |
| `level` | `integer?` | rAthena's val1. 1 by default. |
| `values` | `integer[]?` | val2 to val4, for the statuses that read them. |
| `chance` | `integer?` | Per 10,000, before resistance. 10000 by default. |
| `source` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | Who caused the status: its stats and level count for resistance, and events and get_status show it. Without it, the target's own stats count, as rAthena's sc_start. |
| `cannot_be_avoided` | `boolean?` | SCSTART_NOAVOID: no resistance, no chance roll, and no boss immunity. |
| `keep_duration` | `boolean?` | SCSTART_NOTICKDEF: resistance does not shorten the duration. |
| `keep_chance` | `boolean?` | SCSTART_NORATEDEF: resistance does not lower the chance. |
| `hide_icon` | `boolean?` | SCSTART_NOICON: the status shows no icon. |

### wa.Status

An active status, as entity:get_status returns it.

| Field | Type | Meaning |
|---|---|---|
| `status` | `integer` |  |
| `level` | `integer` | val1. |
| `values` | `integer[]` | val2 to val4. |
| `remaining_milliseconds` | `integer\|"forever"` |  |
| `source` | `wa.Player\|wa.Monster\|wa.Npc\|nil` | Who caused it, or nil. |

### wa.StatusEvent

The object of status_start, status_end and status_tick.

| Field | Type | Meaning |
|---|---|---|
| `target` | `wa.Player\|wa.Monster\|wa.Npc` | target.kind says which. |
| `status` | `integer` |  |
| `level` | `integer` |  |
| `source` | `wa.Player\|wa.Monster\|wa.Npc\|nil` |  |
| `milliseconds` | `integer\|"forever"` | The duration at status_start, and the time left at status_end and status_tick. |
| `reason` | `string?` | Why it started: "skill", "item", "bonus", "script" or "login". Why it ended: "expired", "removed", "replaced", "death", "dispell", "clearance", "logout" or "damage". nil for status_tick. |

### wa.StatusEventOptions

The options of a status event handler. Each one narrows the events that run it.

| Field | Type | Meaning |
|---|---|---|
| `status` | `integer?` | A status from wa.statuses. |
| `target_kind` | `"player"\|"monster"\|"npc"\|nil` |  |
| `map` | `string?` | A map name. |

### wa.StatusHook

The object of the status_chance and status_duration hooks: a status that starts. The innermost result is rAthena's value after resistance.

| Field | Type | Meaning |
|---|---|---|
| `target` | `wa.Player\|wa.Monster\|wa.Npc` |  |
| `status` | `integer` |  |
| `level` | `integer` |  |
| `source` | `wa.Player\|wa.Monster\|wa.Npc\|nil` |  |
| `chance` | `integer?` | status_chance: the chance, per 10,000. |
| `milliseconds` | `integer?` | status_duration: the duration. |

### wa.StatusDefinition

The status of statuses.define.

| Field | Type | Meaning |
|---|---|---|
| `name` | `string` | 1 to 48 lower case letters, digits and _, starting with a letter. The constant's name in wa.statuses. |
| `icon` | `integer?` | The status icon (EFST_ ID) players see. None by default. |
| `kind` | `"buff"\|"debuff"\|nil` | "buff" by default. |
| `ends_on` | `string[]?` | What ends the status besides its time: death, dispell, clearance, logout, banishing_buster, refresh, lux_anima, magic_gear_change and damage. Nothing by default. |
| `bonuses` | `(fun(level: integer): table)?` | Returns bonus entries for a level, as add_timed_bonuses takes them. They apply to players while the status lasts. |
| `tick_milliseconds` | `integer?` | From 100: status_tick fires at this interval, and once more when the status runs out, as rAthena's statuses that tick. |

### wa.ChangeJobOptions

Options of player:change_job.

| Field | Type | Meaning |
|---|---|---|
| `force` | `boolean?` | Skips the job change rules. It needs the permission player.change_job. |

### wa.ResetSkillOptions

Options of player:reset_skill.

| Field | Type | Meaning |
|---|---|---|
| `to_level` | `integer?` | The level to lower the skill to. 0 by default. |
| `with_dependents` | `boolean?` | Also lowers to 0, and refunds, the skills that need a higher level of it. Quest skills among them stay. |

### wa.ResetStatOptions

Options of player:reset_stat.

| Field | Type | Meaning |
|---|---|---|
| `to` | `integer?` | The value to lower the stat to. 1 by default. |

### wa.TimedBonusOptions

Options of player:add_timed_bonuses.

| Field | Type | Meaning |
|---|---|---|
| `ends_on` | `string[]?` | What ends the bonuses besides their time: death, dispell, clearance, logout, banishing_buster, refresh, lux_anima, magic_gear_change and damage. |
| `on_repeat` | `"keep"\|"extend"\|"add"\|"replace"\|nil` | What happens when the player has timed bonuses with the same key. "keep", the default and rAthena's: the new ones are ignored. "extend", rAthena's BSF_FORCE_REPLACE: the first copy keeps its values and takes the new duration when it ends later. "add", rAthena's BSF_FORCE_DUPLICATE: both copies apply. "replace": every copy ends and the new one starts, with its values and duration. |
| `icon` | `integer?` | A status icon (EFST_ ID) to show while they last. |
| `kind` | `"buff"\|"debuff"\|nil` |  |

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
| `sex` | `"male"\|"female"\|nil` | Male by default. |
| `skills` | `integer[][]?` | Learned skills, a list of { skill, level }. |
| `stats` | `table?` | Raised stats, such as { str = 50, dex = 30 }. The others are 1. |
| `skill_points` | `integer?` | Skill points left to spend. 0 by default. |
| `status_points` | `integer?` | Status points left to spend. Those of the base level by default. |

### wa.EquipOptions

Options of world:equip.

It exists only in tests that `wactl mod test` runs.

| Field | Type | Meaning |
|---|---|---|
| `roll` | `"random"\|"default"\|table<string, integer>\|nil` | How the copy's rolls are set, as the roll option of player:give_item. A table needs no permission here. |

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
| `job_change` | mod, event | `change: wa.JobChange` | The player changed jobs: player:change_job, player:rebirth, or a converted jobchange. It fires after the stats and skills of the new job are set. |
| `click` | npc | `player: wa.Player, conversation: wa.Conversation` | A player talks to the NPC. |
| `touch` | npc | `player: wa.Player` | A player stepped into the NPC's touch area, the record's touch field, as rAthena's OnTouch. It fires once each time the player comes in: on a step, or when it arrives on a cell of the area. Hidden and dead players fire it too. It has no conversation. |
| `untouch` | npc | `player: wa.Player` | A player who fired touch left the NPC's touch area: a step out of it, a warp, or a logout. A player who left the map can only be compared: reading it raises an error. |
| `use` | item | `player: wa.Player` | The item is used. |
| `damage` | item | `hit: wa.Damage` | A hit the wearer deals or takes: each normal attack that lands. Skill hits come with skills (phase 4 step 3). |
| `skill_used` | item, mod, event | `use: wa.SkillUse` | A skill took effect, after its cast time. An interrupted cast does not fire it. An item's handler hears the wearer's skills. A mod or event context hears every skill of its map: options narrow it. |
| `cast` | skill | `cast: wa.Cast` | The skill takes effect. A skill whose script has a cast handler runs it instead of core's behavior of the skill. |
| `unequip` | item | `player: wa.Player` | The copy is unequipped, before its context ends. |
| `status_start` | mod, event | `event: wa.StatusEvent` | A status started on an entity of the context's map, or started again. Options narrow it. |
| `status_end` | mod, event | `event: wa.StatusEvent` | A status ended. Options narrow it. |
| `status_tick` | mod, event | `event: wa.StatusEvent` | A status that ticks did its tick, such as poison taking HP, or a mod status with tick_milliseconds. Options narrow it. |
| `chat` | mod, npc, event | `message: wa.ChatMessage` | A player on the context's map said a line: local chat, a whisper, a party line, or a channel line. It runs after the line went out. Options narrow it. Whispers, party and guild lines reach only a mod with rules.override, which the chat hook needs too. |
| `party_join` | mod, event | `join: wa.PartyMembership` | A player of the context's map joined a party: an accepted invite, or a wa.parties call. It does not fire for a player who makes a party itself. |
| `party_leave` | mod, event | `leave: wa.PartyMembership` | A player of the context's map left its party. A member who is offline gets no event. |
| `party_change` | mod, event | `change: wa.PartyChange` | A party's leader or options changed, by a change made on the context's map. |
| `guild_join` | mod, event | `join: wa.GuildMembership` | A player of the context's map joined a guild: an accepted invite, or a wa.guilds call, which also fires it for the master of a new guild. |
| `guild_leave` | mod, event | `leave: wa.GuildMembership` | A player of the context's map left its guild. A member who is offline gets no event. |
| `guild_change` | mod, event | `change: wa.GuildChange` | A guild got a new master, a level, a notice or an emblem, by a change made on the context's map. A new emblem from the account pages runs it on the map of the master, or of the first online member. |
| `trade` | mod, npc, event | `trade: wa.Trade` | Two players on the context's map completed a trade. Options narrow it. |
| `shop_buy` | mod, npc, event | `sale: wa.ShopSale` | A player on the context's map bought from a shop of data/npcs.yml. |
| `shop_sell` | mod, npc, event | `sale: wa.ShopSale` | A player on the context's map sold items to a shop. |
| `vend` | mod, npc, event | `sale: wa.StoreSale` | A player on the context's map bought from a vending shop. |
| `buying_store_sale` | mod, npc, event | `sale: wa.StoreSale` | A player on the context's map sold items to a buying store. |
| `mail` | mod, npc, event | `mail: wa.Mail` | A mail went out from the context's map: a player's, or a script's player:send_mail. |
| `world_event_start` | mod, npc, event | `event: wa.WorldEvent` | A run of a world event started, on any map. Every zone's handlers run. |
| `world_event_end` | mod, npc, event | `event: wa.WorldEvent` | A run of a world event ended, on any map, with its result. Every zone's handlers run. |
| `restart_planned` | mod, npc, event | `restart: wa.RestartPlan` | Staff planned a server restart from the admin screens, or cancelled it. Every zone's handlers run. At the restart time the server saves every character, disconnects every player and stops, so a handler can end a mod's event in time. |
| `dungeon_rotated` | mod, npc, event | `rotation: wa.DungeonRotation` | A rotating dungeon got new floors, on schedule or because staff forced it. Every zone's handlers run, after the players and points of the old floors moved to the nearest town. |
| `region_changed` | mod, npc, event | `change: wa.RegionChange` | A region's fields generated again with a new biome at the end of a season, or because staff forced it. Every zone's handlers run, after the players and points of its fields moved to the region's town. |
| `quest_add` | mod, npc, event | `event: wa.QuestEvent` | A quest joined a player's log on the context's map: player:add_quest, or the new quest of player:change_quest. |
| `quest_progress` | mod, npc, event | `event: wa.QuestEvent` | A kill raised a hunt objective's count, once per objective. Party members near the kill count it too, as rAthena. |
| `quest_complete` | mod, npc, event | `event: wa.QuestEvent` | player:complete_quest marked a quest complete. |

## Named types

### wa.ContextKind

A context's kind.

`mod`, `npc`, `item`, `skill`, `monster`, `event`

### wa.EventName

An event name of wa.events.

`login`, `logout`, `map_enter`, `death`, `base_level_up`, `job_level_up`, `job_change`, `click`, `touch`, `untouch`, `use`, `damage`, `skill_used`, `cast`, `unequip`, `status_start`, `status_end`, `status_tick`, `chat`, `party_join`, `party_leave`, `party_change`, `guild_join`, `guild_leave`, `guild_change`, `trade`, `shop_buy`, `shop_sell`, `vend`, `buying_store_sale`, `mail`, `world_event_start`, `world_event_end`, `restart_planned`, `dungeon_rotated`, `region_changed`, `quest_add`, `quest_progress`, `quest_complete`

### wa.BonusName

A bonus name of wa.bonuses (proposal 0002a).

`str`, `agi`, `vit`, `int`, `dex`, `luk`, `all_stats`, `pow`, `sta`, `wis`, `spl`, `con`, `crt`, `all_trait_stats`, `patk`, `smatk`, `res`, `mres`, `hplus`, `crate`, `max_hp`, `max_sp`, `max_ap`, `natural_hp_recovery`, `natural_sp_recovery`, `hp_gain_over_time`, `sp_gain_over_time`, `hp_loss_over_time`, `sp_loss_over_time`, `no_hp_recovery`, `no_sp_recovery`, `atk`, `weapon_atk`, `refine_atk`, `matk`, `hidden_matk`, `weapon_matk`, `def`, `status_def`, `mdef`, `status_mdef`, `attack_range`, `hit`, `flee`, `perfect_dodge`, `crit`, `crit_chance_taken`, `perfect_hit_chance`, `double_attack_chance`, `move_speed`, `aspd`, `variable_cast_time`, `fixed_cast_time`, `after_cast_delay`, `cooldown`, `sp_cost`, `uninterruptible_cast`, `uninterruptible_cast_everywhere`, `always_endure`, `damage`, `damage_taken`, `crit_damage`, `non_crit_damage`, `crit_damage_taken`, `skill_ratio`, `ignore_size_penalty`, `immunity`, `damage_taken_cap`, `damage_reflected`, `reflected_damage_taken`, `ignore_def`, `ignore_mdef`, `ignore_res`, `ignore_mres`, `def_as_damage`, `attack_element`, `defense_element`, `cause_status`, `coma`, `block_healing`, `cast`, `drain_hp`, `drain_sp`, `remove_hp`, `remove_sp`, `deal_max_hp`, `deal_max_sp`, `set_def`, `set_mdef`, `break_weapon`, `break_armor`, `transform_monster`, `reflect_magic`, `gain_zeny_on_kill`, `status_resistance`, `skill`, `knockback`, `no_gemstone_cost`, `no_magic_gear_fuel_cost`, `hp_per_hit`, `sp_per_hit`, `hp_per_kill`, `sp_per_kill`, `experience`, `drop_rate`, `extra_drop`, `steal_chance`, `healing_done`, `healing_received`, `item_hp_recovery`, `item_sp_recovery`, `full_recovery_on_revive`, `unbreakable_weapon`, `unbreakable_armor`, `unbreakable_helm`, `unbreakable_shield`, `unbreakable_garment`, `unbreakable_shoes`, `equipment_break_chance`, `unstrippable_weapon`, `unstrippable_armor`, `unstrippable_helm`, `unstrippable_shield`, `splash_range`, `no_knockback`, `see_hidden`, `hidden_from_detectors`, `max_weight`

### wa.EquipSlot

An equipment slot.

`head_top`, `head_middle`, `head_low`, `armor`, `left_hand`, `right_hand`, `garment`, `shoes`, `left_accessory`, `right_accessory`, `ammo`, `costume_head_top`, `costume_head_middle`, `costume_head_low`, `costume_garment`, `shadow_armor`, `shadow_weapon`, `shadow_shield`, `shadow_shoes`, `shadow_right_accessory`, `shadow_left_accessory`

### wa.HookName

A hook name of wa.hooks.

`drop_rate`, `experience`, `status_chance`, `status_duration`, `skill_damage`, `cast_time`, `skill_cost`, `cast_allowed`, `monster_target`, `item_roll`, `chat`, `trade`, `vend`, `buying_store_sale`, `mail`, `spawn_table`

### wa.Bound

Who a bound copy is bound to.

`account`, `character`, `party`, `guild`

### wa.ExpandableLimit

A limit that one player holds, which player:expand changes.

`inventory_slots`, `storage_slots`, `cart_slots`

### wa.QuestIcon

An icon of npc:show_quest_icon, as rAthena's QTYPE_ values 0 to 6 and 8: ! and ? marks for quests, jobs and events, a warg, and a warg's face.

`quest`, `quest_question`, `job`, `job_question`, `event`, `event_question`, `warg`, `warg_face`

### wa.MonsterState

A monster's skill state, in the words of rAthena's mob_skill_db.

`idle`, `walk`, `loot`, `dead`, `attack`, `angry`, `chase`, `follow`

### wa.Weekday

A day of the week.

`sunday`, `monday`, `tuesday`, `wednesday`, `thursday`, `friday`, `saturday`

