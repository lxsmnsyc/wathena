# Making a mod

A mod adds or changes game content without changes to the server (strategy section 6). This guide covers what the mod loader and tools support today, in phase 3 step 6. The scripting API is [proposal 0002](../docs/proposals/0002-lua-api.md). It is experimental and can change in any release until API 1.0.

- [core](core) is the base game as a mod. It is built into the server. Its files are rAthena's item scripts, NPCs and shops, which the server converts when it starts ("Item scripts" and "Core's NPCs" below).
- [examples/greeter](examples/greeter) is a small sample: an item, an NPC with a conversation and saved variables, a shop, a script that greets players, and tests.
- [examples/halloween](examples/halloween) is the phase 3 exit check: a hat, a witch who gives it once, Pumpkin Porings with drops, a night that doubles their drops through a `drop_rate` hook, and the Pumpkin King, a boss that a world event brings every evening.
- [examples/verve](examples/verve) adds a status effect: `statuses.define`, a `status_tick` handler, and a tonic that starts the status, with tests.
- `wactl mod new` makes a new mod that passes `wactl mod validate` and `wactl mod test` ("Tools" below).

Server owners install mods in the folder that `mods.dir` names. [server/README.md](../server/README.md), "Mods", covers the server side.

## Folders

A mod is a folder, or a `.zip` file with the same files at its root or in its one top folder.

```text
greeter/
  mod.yaml              the manifest
  data/
    items.yml           item overlay
    monsters.yml        monster overlay
    npcs.yml            NPC placements and shops
    events.yml          world events
  locales/
    en.yml              text for players, read with wa.text
  scripts/
    main.lua            an entry script that mod.yaml lists
    npc/greeter.lua     an NPC's entry script, which npcs.yml names
    lib/messages.lua    a helper module
  tests/
    greeter_test.lua    tests that wactl mod test runs
  .luarc.json           editor settings for the Lua Language Server
```

Names that start with `.`, such as `.luarc.json`, `.wathena` and `.git`, are not part of the mod. The server ignores them, and `wactl mod pack` leaves them out.

## The manifest

```yaml
id: example.greeter
name: Greeter
version: 0.1.0
api: experimental
description: Greets players when they log in.
authors:
  - name: Example Author
license: MIT
depends:
  - id: core
    version: ">=0.1"
load_after:
  - example.seasonal-lib
supports:
  rulesets:
    base: [renewal, pre_renewal]
    flags:
      skill_cooldown: on
permissions:
  - player.give_items
scripts:
  - scripts/main.lua
```

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | Lowercase words of letters, digits, `_` and `-`, separated by dots. `core`, `core.*`, `wa` and `wa.*` are reserved. |
| `name` | yes | The name people see. |
| `version` | yes | A semantic version, such as `1.2.0`. |
| `api` | yes | `experimental` until API 1.0. A range such as `">=1.0 <2.0"` parses, and is refused until the server has API 1.0. |
| `description`, `authors`, `license` | no | Shown to server owners. Each author needs a `name`. |
| `depends` | no | Mods that must be installed and load first. `version` is a range: `>=1.0 <2.0`, `^1.2`, `~1.2`, `1.2.3`, and alternatives with `||`. |
| `load_after` | no | Mods to load after when they are installed. A missing one is ignored. |
| `supports.rulesets` | no | The ruleset bases the mod works with, and values flags must have. A flag can list several values. Without it, the mod loads on any ruleset. |
| `provides.ruleset` | no | Makes the mod a ruleset mod (below). |
| `permissions` | no | What the mod needs (below). |
| `scripts` | no | Entry scripts that run once per zone, in the mod's context. Paths are under `scripts/`. |

Unknown fields are errors, so a typo does not pass. Every error names the file and the field.

## Load order

Mods load after `core`, after their `depends`, and after the mods in `load_after` that are installed. Among mods that could load next, the lowest ID goes first, so the order is the same on every start. A cycle is an error that shows it, such as `example.a -> example.b -> example.a`.

## Rulesets

A world has one ruleset: a base, `renewal` or `pre_renewal`, and a value for each flag of strategy section 3.6. `core.ruleset.renewal` and `core.ruleset.pre_renewal` set every flag to their base's value.

A ruleset mod starts from another ruleset mod and changes flags:

```yaml
id: example.classic-plus
name: Classic Plus
version: 1.0.0
api: experimental
provides:
  ruleset:
    extends: core.ruleset.pre_renewal
    flags:
      cast_time: renewal
      level_penalty_drop: on
```

- A ruleset mod can extend another ruleset mod. Flags apply from the core ruleset up.
- `fourth_job_traits: on` needs the renewal base.
- A content mod whose `supports.rulesets` does not match is refused, with the reason.

## Permissions

| Permission | Default |
|---|---|
| `game.read`, `npc.create`, `event.listen`, `storage.mod` | Granted to every mod. |
| `player.give_items`, `player.give_zeny`, `player.give_exp`, `player.warp`, `player.status`, `world.announce`, `events.schedule` | Needs the owner's approval. |
| `rules.override`, `admin.commands`, `net.http`, `items.modify_rolls` | Needs approval, and is logged as a warning at each start. |

- A permission that needs approval must be in `permissions`, and the owner approves it in the server config under `mods.approve`.
- A mod with a permission that is not approved does not load, and the server does not start. The error names the permissions.
- Calls check permissions when they run. A call without its permission raises an error that names it, such as `player:give_item needs the permission player.give_items`.

## Data overlays

Files in `data/` add records or change fields of existing ones. Each file is a list of records with a `name`.

- A new record is named with the mod's ID: `example.greeter:Welcome_Cookie`.
- A name without a prefix is a core record. A field set there changes the core record. A core name that does not exist is an error, so a typo does not make a new record.
- To change another mod's record, write its full name, and load after that mod.
- A mod cannot delete a record.
- When two mods set the same field, the later mod wins and the server logs a warning. With `mods.strict`, it is an error.
- Fields are changed whole: setting `heal` replaces all of `heal`.

### Items: data/items.yml

```yaml
- name: example.greeter:Welcome_Cookie
  type: Healing
  weight: 10
  stackable: true
  script: scripts/items/welcome_cookie.lua
- name: Red_Potion
  weight: 50
- name: Poring_Card
  bonuses: [[luk, 3], [perfect_dodge, 1]]
```

- Names are AegisNames, such as `Red_Potion`.
- Fields are those of the item extract: `type`, `weight` (in tenths), `stackable`, `stack_limit`, `unique_id`, `no_consume`, `no_drop`, `delay_ms`, `delay_status`, `min_level`, `max_level`, `gender`, and `job_restricted`, and `display_name`, `buy`, `sell` and `no_sell`: the name `items.name` gives, the prices of NPC shops in zeny, and whether shops buy the item. `name` is the record's key, so the name players see is `display_name`, as for NPCs.
- `script` is the item's entry script, a path under the mod's `scripts/`. It belongs to the mod that sets it.
- `bonuses` are static bonuses of an equipment item ("Static bonuses" below).
- Equipment has the fields of the equipment extract, as rAthena's item_db names them:
  - `location`: the positions the item is worn in, as a list such as `[head_top, head_middle]`. The names are the slot names of `player:get_equipped`, and `both_hand` and `both_accessory`. A card's location is where it can be put.
  - `subtype`: the weapon type of a weapon, or the ammunition type of ammunition, as rAthena's `W_` and `AMMO_` numbers.
  - `atk`, `matk` (renewal only), `def`, `range`, `slots` (up to 16; the world uses at most 4, rAthena's `MAX_SLOTS`), `weapon_level`, `armor_level` and `refineable`.
  - `view`: the sprite the item shows on a character, as item_db's `View`.
  - `jobs` and `classes`: who may wear it, as three decimal strings of rAthena's job bit masks and the `ITEMJ_` bits. Leave them out for everyone.
  - `no_equip`: the map types where its script does not run, as `item_noequip.txt`: 1 normal maps, 2 PvP, 4 GvG, 8 battlegrounds.
- `unsupported` says what the item's script needs that the server does not have yet. A usable item with it is refused, and the player sees the text. Core sets it on converted items.
- `id` and `aegis_name` cannot be set.
- A new item is an `Etc` item until `type` says otherwise.

### NPCs: data/npcs.yml

```yaml
- name: Greeter#prontera
  map: prontera
  x: 158
  y: 200
  direction: south
  sprite: 4_F_KAFRA1
```

- A name without a prefix is the mod's own NPC: `Greeter#prontera` in `example.greeter` is `example.greeter:Greeter#prontera`. Core's NPCs are named `core:<rAthena's unique name>`, such as `core:kaf_prontera`.
- Players see the name up to `#`, or `display_name` when it is set. An empty `display_name` shows no name.
- `map`, `x`, `y` and `sprite` are required. `sprite` is an NPC sprite name, such as `4_F_KAFRA1`, or its number.
- `direction` is `south` (the default), `southwest`, `west`, `northwest`, `north`, `northeast`, `east` or `southeast`.
- `script` names the NPC's entry script. It must exist. Its setup function runs once for each NPC that names it ("NPC scripts" below).
- `shop` makes the NPC a shop: a list of `{item, price}`. A price left out, or -1, is the item's `buy` price. A click on a shop without a click handler opens the shop.
- `discount: false` keeps Discount and Compulsion Discount from lowering the shop's prices, as rAthena's `shop` line with `no`. It is on by default, as in rAthena.
- A record with `shop` and no `map` is a shop that only scripts open, with `player:open_shop`.

```yaml
- name: Tool Dealer#prt1
  map: prt_in
  x: 126
  y: 76
  sprite: "53"
  shop:
    - {item: Red_Potion}
    - {item: Fly_Wing, price: 60}
- name: Cookie Stand
  shop:
    - {item: "example.greeter:Welcome_Cookie", price: 25}
```

### Monsters: data/monsters.yml

```yaml
- name: example.halloween:PUMPKIN_PORING
  sprite: PORING
  display_name: Pumpkin Poring
  level: 12
  hp: 260
  attack: [18, 26]
  base_exp: 90
  job_exp: 70
  element: ghost
  drops:
    - {item: Pumpkin, rate: 2000}
    - {item: "example.halloween:Pumpkin_Hat", rate: 50}
- name: PORING
  hp: 80
```

- Names are AegisNames in upper case, such as `PORING`. A new monster is named with the mod's ID.
- `sprite` names the monster whose sprite players see. A new monster needs it, and starts from that monster's stats, with no exp and no drops.
- The server has no monster status calculation yet, so the fields set what combat reads. Phase 4 adds monster stats: the server then computes these values from the base stats, as rAthena does, and the fields change. The fields are: `hp`, `sp`, `level`, `attack` and `magic_attack` as `[minimum, maximum]`, `defense`, `magic_defense`, `hit`, `flee`, `speed` (milliseconds per cell), `attack_range`, `attack_delay`, `attack_motion` and `damage_motion`.
- `element` (from `wa.elements`, with `element_level` 1 to 4), `race` (from `wa.races`) and `size` (from `wa.sizes`) are names.
- `class` is `normal`, `boss`, `guardian`, `battlefield` or `event`. As in rAthena, `boss` adds the modes `detector`, `status_immune` and `knockback_immune`.
- `modes` is a list of rAthena's modes in lower case, such as `can_move`, `can_attack`, `aggressive` and `mvp`. It replaces the monster's modes.
- `base_exp` and `job_exp` are scaled by the exp rates. `drops` is a list of `{item, rate}`, with rates per 10,000, at most 20. The drop rate settings apply to them as to core's.
- `display_name` is the name players see. `id` cannot be set.
- `monsters.id "example.halloween:PUMPKIN_PORING"` finds a mod's monster. Monsters appear on maps through scripts (`spawning.monsters`, below). There is no `data/spawns.yml` yet.

### World events: data/events.yml

```yaml
- name: Pumpkin_King
  trigger: { daily: "20:00" }
  duration_minutes: 60
  script: scripts/events/pumpkin_king.lua
  map: prt_fild08
```

The world event scheduler starts events without staff action (strategy section 8.8). A mod that adds or changes an event needs the permission `events.schedule`.

| Field | Meaning |
|---|---|
| `trigger` | What starts the event (below). |
| `duration_minutes` | How long a run lasts: 1 to 10,080 minutes. |
| `script` | The entry script of a run. Its setup function runs in a new event context. |
| `map` | The map whose zone runs the event. Without it, the world's start map. |

| Trigger | Starts the event |
|---|---|
| `daily: "20:00"` | Every day at that time. |
| `weekly: { day: saturday, time: "20:00" }` | Every week. |
| `monthly: { day: 1, time: "00:00" }` | Every month. A month without the day, such as the 31st, has no run. |
| `hourly: { minute: 30 }` | Every hour. |
| `random: { chance_per_hour: 500 }` | At the start of each hour, with a chance per 10,000 from a secret stream. |
| `condition: { module: example.bosses.conditions, function: town_is_quiet, lasting_minutes: 60 }` | Once the function has returned true for that long. |

Rules:

- Times are in the server's time zone, `time_zone` in the config (server/README.md). On a day clocks change, a time that does not exist runs at the next valid minute, and a time that happens twice runs once.
- Each run is one context of kind `"event"`. `context:get_instance()` is `{ name, map }`. What the run makes goes when it ends: monsters, handlers, timers, schedules and hooks.
- An event has one run at a time. A time that comes while a run lasts is skipped.
- Each run is recorded before it starts, so a restart never runs the same time twice. Runs whose window passed while the server was stopped are skipped. A run whose window is still open when the server starts begins at once and ends at its usual time. A run cut short by a stop does not start again.
- A condition is a function of a helper module that the mod can `require`. The module returns a table that holds it. It runs once a minute in the zone of the event's map, and only reads the game: a call that changes it raises an error. After a run ends, it must hold for `lasting_minutes` again.
- A sleeping zone keeps its clock still, so the timers of a run on a map without players wait. The run still ends on time.
- The server owner can move or turn off an event in the config, under `mods.events` (server/README.md, "World events").

```lua
-- scripts/events/pumpkin_king.lua
local events = require "wa.events"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"
local world = require "wa.world"

local PUMPKIN_KING = monsters.id "example.halloween:PUMPKIN_KING"

return function()
  local king = spawning.monsters { monster = PUMPKIN_KING }[1]
  world.announce("The Pumpkin King has risen on the Prontera fields!")
  events.on("death", function(death)
    world.announce(death.killer.name .. " defeated the Pumpkin King!")
  end, { victim = king })
end
```

Kinds not overlaid yet: skills, jobs, spawns, quests and achievements.

## Text: locales/en.yml

Text for players lives in `locales/en.yml`, a mapping of keys to text. Nested keys are joined with dots:

```yaml
greeter:
  hello: Hello! Welcome to Prontera.
```

`text "greeter.hello"` from `wa.text` reads it. Players have no language setting yet, so the English file is the only one read. A key the file lacks raises an error.

## IDs

The server gives each new record an ID in the mod range of its kind the first time it loads it. Items start at 2,000,000 and monsters at 100,000. Skills start at 20,000, jobs at 10,000, and quests and achievements at 1,000,000, for when those kinds get overlays.

- An ID never changes, and never goes to another name, even when the mod is removed. The item comes back with its ID when the mod is installed again.
- Scripts find IDs by name: `items.id "example.greeter:Welcome_Cookie"`.

## Scripts

Scripts are Lua 5.4 in a sandbox (internal/script/README.md). There are no globals from the server. Every part of the API comes from `require`.

### Entry scripts and helper modules

- An entry script is a file that `scripts` in mod.yaml lists. It must return a setup function. A file that returns anything else fails to load, with the file name in the error.
- Code outside the setup function runs once per zone, when the file loads. It can `require` modules and look up IDs. Calls that need a context, such as `events.on`, fail there.
- The setup function runs once per zone, in the mod's context. Locals inside it belong to that zone's instance.
- A helper module is any other file the mod loads with `require`. It can return any value. It loads once per zone.

### require names

| Name | Loads |
|---|---|
| `wa.events` | An API module. |
| `example.greeter.lib.messages` | `scripts/lib/messages.lua` of `example.greeter`. |
| `example.seasonal-lib.util` | `scripts/util.lua` of `example.seasonal-lib`, only when it is in `depends`. |

- A name is a mod ID, a dot, and the path under `scripts/` with dots for folders and no `.lua`.
- When several mod IDs match, the longest wins.
- A mod can require its own files and the files of mods in its `depends`. Any other mod's file is an error that names the missing dependency.
- Folder and file names under `scripts/` use letters, digits, `_` and `-`.
- A file of a dependency runs in that dependency's environment. The value it returns is shared with every mod that requires it.

### Isolation

- Each mod has its own global table in each zone. One mod cannot read or change another mod's globals.
- The standard libraries (`string`, `table`, `math`, `utf8`, `coroutine`) are copies per mod. Changing one changes only the mod's own copy.
- API modules, player handles, and event and timer handles are read-only. Writing to them, `rawset` on them, or `setmetatable` on them raises an error.
- `load` runs code in the mod's own environment unless it is given another table.

### The context

The runtime sets a hidden context before it calls a setup function, a handler, or a timer. It owns what they create. When it ends, its handlers and timers go with it. A mod's context ends when the mod is reloaded, or when its handlers fail 10 times in a minute in that zone.

```lua
local context = require "wa.context"

return function()
  context:get_kind()     -- "mod"
  context:get_mod()      -- "example.greeter"
  context:get_instance() -- a read-only table: id (the mod ID) and map (the zone's map)
end
```

### Events

```lua
local events = require "wa.events"

return function()
  local handle = events.on("login", function(player) end)
  handle:remove()
end
```

| Event | Handler arguments | When |
|---|---|---|
| `login` | `player` | A player logs in, in the zone it enters. |
| `logout` | `player` | A player logs out, before it leaves. |
| `map_enter` | `player` | A player enters the zone: at login and after a warp. |
| `death` | `death` | A player or a monster died on the context's map ("Death" below). Mod, NPC and event contexts. |
| `base_level_up` | `player` | The player gained a base level. |
| `job_level_up` | `player` | The player gained a job level. |
| `click` | `player`, `conversation` | NPC contexts only: a player talks to the NPC ("NPC scripts" below). |
| `status_start`, `status_end`, `status_tick` | `event` | A status started, ended or ticked on the context's map ("Status effects" below). Mod and event contexts. |

Event contexts can listen to the events of mod contexts.

`touch`, for NPCs whose area a player walks into, is a name that registers and does not fire yet.

- `events.on` checks the name. An unknown name, or an event of another context kind, such as `click`, raises an error during setup.
- Handlers run in registration order. A failing handler is logged and does not stop the others.
- A handler that runs over the CPU or memory limit stops with an error. The zone goes on.

### Timers

```lua
local timers = require "wa.timers"

return function()
  local once = timers.after(5000, function() end)
  local repeating = timers.every(60000, function() end)
  once:cancel()
end
```

- Times are in milliseconds of the zone clock. Timers run on the zone's ticks, every 50 ms.
- A zone without players sleeps, and its clock stops. Timers wait while it sleeps.
- A repeating timer that falls behind skips the runs it missed.
- A context can hold 10,000 handlers and timers.

### Players

Handlers get players as read-only handles. Each access looks the player up again. A handle to a player who left the zone raises an error.

| Field | Meaning |
|---|---|
| `kind` | `"player"`. Every entity handle has `kind`. |
| `name`, `job`, `base_level`, `job_level` | The character's name, job ID and levels. |
| `hp`, `max_hp`, `sp`, `max_sp`, `zeny` | Current and maximum HP and SP, and zeny. |
| `map`, `x`, `y` | The zone's map name, such as `prontera`, and the cell. |
| `character_id`, `account_id` | Database IDs, 0 without a database. |
| `party` | `nil`. Parties do not exist yet. |

| Method | Permission |
|---|---|
| `player:send_message(text)` | None. Shows a line in the player's chat log, sent as `CHAT_CHANNEL_SYSTEM`. |
| `player:give_item(item, amount)` | `player.give_items`. Returns `false` when the items do not fit. |
| `player:count_item(item)` | `game.read`. |

### NPC scripts

An NPC's entry script runs its setup function once for each placed NPC that names it, in an NPC context. The NPC is `context:get_instance()`. Locals of the setup function belong to that NPC, as rAthena's `.variables`. A zone starts its NPCs' scripts when the first player enters it.

```lua
local events = require "wa.events"
local emotions = require "wa.emotions"
local context = require "wa.context"

return function()
  local npc = context:get_instance()   -- name, map, x and y
  events.on("click", function(player, conversation)
    conversation:show_image("kafra_01", "right")
    npc:show_emotion(emotions.thanks)
    conversation:say("[" .. npc.name .. "]", "Hello!")
    conversation:wait_for_next()
    if conversation:choose("Save", "Cancel") == 1 then
      player:set_save_point("prontera", 156, 191)
    end
  end)
end
```

A click starts a conversation: the NPC's click handlers run in a thread of their own, which pauses while it waits for the player.

| Call | Meaning |
|---|---|
| `conversation:say(...)` | Shows lines: strings or numbers, up to 64, each at most 1,024 bytes. RO color codes, such as `^FF0000`, color them. |
| `conversation:wait_for_next()` | Waits for the Next button. The next lines replace the shown ones. |
| `conversation:choose(...)` | Shows options and returns the chosen one, from 1. An empty option is hidden and keeps its number. |
| `conversation:ask_number(minimum, maximum)` | Waits for a number in the range, from -2147483648 to 2147483647. |
| `conversation:ask_text(maximum_length)` | Waits for text of 1 to 255 characters at most. It is cleaned as chat is. |
| `conversation:close()` | Shows the Close button and waits for it, as rAthena's `close2`. The handler goes on after it, and later dialog calls raise an error. |
| `conversation:wait(milliseconds)` | Waits up to 10 minutes, as `sleep2`. |
| `conversation:show_image(name, position)` | Shows `image/illust/<name>`, as `cutin`, at `"left"`, `"center"` or `"right"`. |
| `conversation:hide_image()` | Hides it. The client also hides it when the window closes. |
| `conversation:show_progress(seconds)` | Shows a progress bar over the player and waits until it is full, up to 600 seconds. A walk cancels it and ends the conversation. |
| `npc:show_emotion(emotion)` | Shows an emotion over the NPC, from `wa.emotions`, such as `emotions.thanks`. |
| `npc:say_aloud(text)` | Says a line of local chat as the NPC, as `npctalk`. Players within 9 cells hear it. |

Rules, as rAthena's:

- A player clicks NPCs within 15 cells. A dead player, or one already talking, clicks nothing.
- While a player talks, it cannot walk, attack, pick up, use or drop items.
- A step times out after 60 seconds, or 180 seconds for input, and the conversation ends.
- A handler that returns with lines on show leaves the Close button. One that returns without showing anything closes the window.
- When the player leaves, dies, changes map, or the conversation times out, the waiting call raises the error "the conversation was closed". A handler may catch it with `pcall`, but cannot talk any more, and the player is gone when it left. The runtime does not log it as a failure.
- A warp asked for in a conversation happens when the conversation pauses or ends, and ends it.
- A reload ends the mod's NPC contexts, and starts them again with the new files. Conversations that run finish on the old version.

### Saved variables: wa.variables

```lua
local variables = require "wa.variables"

local visits = variables.character_integer "visits"     -- saved with the character
local met = variables.account_boolean "met"             -- shared by the account's characters
local busy = variables.character_temporary_integer "busy" -- until logout
local greetings = variables.server_integer "greetings"  -- the whole world, saved soon after a change
local notes = variables.character_table "notes"         -- plain data

return function()
  events.on("click", function(player, conversation)
    local n = visits:add(player, 1)
    met:set(player, true)
    greetings:add(1)
    notes:set(player, { last = n, seen = { 1, 2, 3 } })
  end)
end
```

- Scopes: `character`, `character_temporary`, `account`, `server`, and `instance`, which raises an error until instances exist. Types: `integer`, `boolean`, `string`, and `table`.
- A declaration names a variable of the running mod. Two mods can use the same name. One mod declares a name with one type.
- `get(player)` and `set(player, value)`, or `get()` and `set(value)` for a server variable. `add(player, amount)` changes an integer in one step and returns the new value.
- Defaults: 0, false, "", and nil for a table. Setting nil, or the default, removes the stored value.
- A string is at most 4,096 bytes. A table holds numbers, booleans, strings and tables, with keys that are all strings, or 1 to n, and is at most 64 KB as JSON. `get` returns a copy.
- Names are 1 to 64 letters, digits and `_`, not starting with a digit.
- Character and account variables are saved with the character, in the same transaction. Two characters of one account that change the same account variable at once: the last save wins.
- Without a database, variables live in memory until the server stops.
- Declaring needs `storage.mod`, which every mod has.

### Shops, save points and announcements

| Call | Permission | Meaning |
|---|---|---|
| `player:open_shop(name)` | None | Opens a shop of `data/npcs.yml` by record name, as `callshop`. A name without a mod ID is the running mod's own shop. `mod:Name` opens another mod's shop. |
| `player:set_save_point(map, x, y)` | `player.warp` | Sets the save point, as `savepoint`. Butterfly Wings and death take the player there. Returns false when the map does not run or the cell is not walkable. |
| `player:can_carry(item, amount)` | `game.read` | `checkweight`: whether the items fit, by weight and slots. |
| `player:pay_zeny(amount)` | None | Takes zeny. Returns false, and takes nothing, when the player has too little. |
| `player:give_zeny(amount)` | `player.give_zeny` | Gives zeny. Zeny over 2,147,483,647 is lost. |
| `player:take_item(item, amount)` | None | `delitem`. Returns false, and takes nothing, when the player has too few. The amount must be 1 or more, or the call raises an error. |
| `player:heal_fully()` | None | Heals all HP and SP. |
| `player:mark_minimap(id, x, y, color)` | None | Marks a cell on the minimap, as `viewpoint`. A mark with the same ID replaces it. |
| `player:unmark_minimap(id)` | None | Removes a mark. |
| `items.name(item)` | `game.read` | The item's name from rAthena's item database. |
| `text.item_link(item)` | None | A link `say` shows as the item's name, as `mesitemlink`. |
| `maps.announce(map, text)` | None | A line for every player of a map, as `mapannounce`. |
| `world.announce(text)` | `world.announce` | A line for every player of the world, as `announce`. |

Player handles also have `sex`: `"male"` or `"female"`.

Shops follow rAthena's `npc_buylist` and `npc_selllist`: a purchase checks the zeny, the weight and the free slots, and a sale checks every slot, before anything changes. Then zeny and items change in one step. Discount and Compulsion Discount lower what the player pays, and Overcharge raises what the player gets.

### Monsters

Monster handles, from `death` and `spawning.monsters`, are read-only. They work like player handles. Every entity handle has a `kind` field: `"player"`, `"monster"` or `"npc"`, so `death.victim.kind` and `death.killer.kind` say what died and what killed it.

| Field | Meaning |
|---|---|
| `kind` | `"monster"`. |
| `id`, `name`, `level` | The monster's ID, such as 1002, its name and level. |
| `hp`, `max_hp` | Current and maximum HP. |
| `map`, `x`, `y` | The zone's map name and the cell. |
| `is_alive` | `false` for a killed monster. |

- A killed monster is gone from the map, but its handle stays readable until the death event's handlers have run.
- After that, reading the handle raises an error, like a handle to a player who left. A timer that the handler starts cannot read it.
- Two handles to one monster are equal with `==`. A monster that comes back after it died is a new monster, with a new handle.
- Reading needs `game.read`.

### Death

```lua
local events = require "wa.events"

return function()
  events.on("death", function(death)
    if death.killer then
      death.killer:send_message("You defeated " .. death.victim.name .. ".")
    end
  end, { victim_kind = "monster" })
end
```

The death object has `victim` (a player or monster handle), `killer` (a handle, or nil when nothing killed it), `map`, `x`, `y`, and `skill` (nil for a normal attack). It fires for deaths on the context's map. Options narrow it:

| Option | The handler runs for |
|---|---|
| `victim` | One player or monster handle. |
| `victims` | Any handle of a list, such as what `spawning.monsters` returned. |
| `victim_kind` | `"player"` or `"monster"`. |
| `monster` | Monsters of one ID, from `monsters.id`. |
| `map` | One map. A context sees its own map's deaths only. |

### Spawning monsters

```lua
local spawning = require "wa.spawning"

return function()
  -- 15 Pumpkin Porings that come back after they die, while the mod runs.
  spawning.monsters { monster = PUMPKIN_PORING, amount = 15, respawn = true }
  -- 3 near a cell, that stay dead.
  local guards = spawning.monsters { monster = PORING, amount = 3, x = 150, y = 180, area = 5 }
end
```

- `spawning.monsters { map, monster, amount, x, y, area, respawn }` places monsters on the context's map and returns their handles. `map` may be left out. Another map raises an error, so a mod script, which runs in every zone, checks `context:get_instance().map` first.
- Without `x` and `y`, each monster lands on a random walkable cell. `area` spreads them within that many cells of `x`, `y`.
- `amount` is 1 to 1,000. By default a dead monster stays dead. `respawn = true` makes it come back as a spawn line's monster does in rAthena: after `respawn_milliseconds` (1,000 or more), or after 5,000 ms without it, rAthena's delay for a spawn line that gives none. Monster data has no respawn time, so the delay belongs to the call. `respawn_milliseconds` is provisional.
- The context owns the monsters: they go when it ends, such as when an event's run ends or the mod reloads.
- It needs `npc.create`, which every mod has.

### Schedules

```lua
local schedules = require "wa.schedules"

return function()
  schedules.daily("00:00", function() end)
  schedules.weekly("saturday", "20:00", function() end)
  local s = schedules.hourly(30, function() end)
  s:cancel()
  schedules.when(function() return #players_here() == 0 end, function() end, { lasting_minutes = 60 })
end
```

- Schedules follow the server clock, in the server's time zone. A time that does not exist on a day clocks change runs at the next valid minute. A time that happens twice runs once.
- A mod script runs in every zone, so its schedules run in every zone. Check `context:get_instance().map` to run on one map.
- A sleeping zone wakes for its schedules. A time that passed more than once, such as while the server was stopped, runs at most once.
- `schedules.when` runs its condition once a minute, at the start of each minute. The handler runs once the condition has held for `lasting_minutes`, once for each time it holds. The condition only reads the game: a call that changes it raises an error.
- The context owns its schedules. Each returns a handle with `cancel()`. They need no permission.

### Hooks

```lua
local hooks = require "wa.hooks"

return function()
  hooks.on("drop_rate", function(drop, next)
    local rate = next(drop)
    if drop.monster.id == PUMPKIN_PORING then
      return rate * 2
    end
    return rate
  end, { priority = 10 })
end
```

| Hook | Object | Returns |
|---|---|---|
| `drop_rate` | `drop`: `player` (the killer, or nil), `monster`, `item`, `map`, `rate` (rAthena's rate, per 10,000) | The rate, kept to 0 to 10,000. |
| `experience` | `exp`: `player`, `monster`, `base`, `job` (the exp the player gets, after the shares and the level penalty) | `base, job`. Below 0 counts as 0. |
| `status_chance` | `start`: `target`, `status`, `level`, `source`, `chance` (rAthena's chance after resistance, per 10,000) | The chance, kept to 0 to 10,000. |
| `status_duration` | `start`: `target`, `status`, `level`, `source`, `milliseconds` (rAthena's duration after resistance) | The duration, kept to 1 to 2,147,483,647. |

- Hooks run in a chain. Higher priority runs first; equal priorities run in the order they were added. `next(object)` returns the rest of the chain's result. The innermost result is rAthena's value.
- To pass a changed object, pass a table with the same fields to `next`.
- A hook only computes: a call that changes the game raises an error in it. A hook that fails is logged and left out.
- Hooks run in the zone of the kill. A hook of a mod script runs in every zone. A hook of an event run changes kills on the event's map, until the run ends.
- `hooks.on` needs `rules.override`, which the server owner approves and the server logs at each start. It returns a handle with `remove()`.

### Status effects

Every entity handle has the status methods of proposal 0002, review decision 19: players, monsters and NPCs.

```lua
local statuses = require "wa.statuses"

player:start_status(statuses.blessing, { milliseconds = 60000, level = 10 })
if player:has_status(statuses.poison) then
  player:end_status(statuses.poison)
end
local s = player:get_status(statuses.blessing)
-- s.status, s.level, s.values (val2 to val4), s.remaining_milliseconds, s.source
```

`start_status` options:

| Option | Default | Meaning |
|---|---|---|
| `milliseconds` | The duration rAthena's `sc_start` looks up for the level | 1 to 2,147,483,647, or `"forever"` (`INFINITE_TICK`). |
| `level` | 1 | val1. |
| `values` | `{}` | val2 to val4, for the statuses that read them. |
| `chance` | 10000 | Per 10,000, before resistance. |
| `source` | None | The entity that caused it. Its stats and level count for resistance. Without it, the target's own count, as `sc_start`. |
| `cannot_be_avoided` | false | `SCSTART_NOAVOID`: no resistance, no roll, no boss immunity. |
| `keep_duration` | false | `SCSTART_NOTICKDEF`: resistance does not shorten it. |
| `keep_chance` | false | `SCSTART_NORATEDEF`: resistance does not lower the chance. |
| `hide_icon` | false | `SCSTART_NOICON`. |

- `start_status` returns true when the status starts. rAthena's rules apply: a weaker copy of an active status does nothing and returns true, and a status that fails its roll returns false.
- `end_status` returns true when the status was active. `get_statuses()` lists every active status, by status.
- A player's stats change once an item's use script returns, or else at the end of the server tick.
- Converted item scripts set `keep_duration`, as rAthena's `sc_start` does in item scripts. Converted NPC scripts set `cannot_be_avoided`.

Events of mod and event contexts, with the options `status`, `target_kind` and `map`:

```lua
events.on("status_end", function(event)
  -- event.target, event.status, event.level, event.source,
  -- event.milliseconds (the time left), event.reason
end, { status = statuses.poison, target_kind = "player" })
```

- `status_start`: `reason` is `"skill"`, `"item"`, `"bonus"` (a proc such as `cause_status`), `"script"`, or `"login"` for a status restored from a save. `milliseconds` is the duration.
- `status_end`: `reason` is `"expired"`, `"removed"`, `"replaced"`, `"death"`, `"dispell"`, `"clearance"`, `"logout"` or `"damage"`.
- `status_tick`: each tick of a status that ticks, such as poison, and each tick of a mod status. It has no `reason`.
- The events of statuses that a script call starts or ends run after that call returns.

A mod script defines new statuses with `statuses.define` in its setup function:

```lua
statuses.define {
  name = "verve",
  icon = 241, -- a status icon (EFST_ ID) the client has
  kind = "buff",
  ends_on = { "death", "dispell" },
  bonuses = function(level)
    return { { "max_hp", 100 * level } }
  end,
  tick_milliseconds = 5000,
}
-- statuses.verve is now its ID.
```

- The name is 1 to 48 lower case letters, digits and `_`. A name that a status of the game or of another mod has raises an error.
- `bonuses` returns bonus entries, as `add_timed_bonuses` takes them. They apply to players while the status lasts. A status restored at login runs it again.
- With `tick_milliseconds`, `status_tick` fires at that interval, and once more when the status runs out.
- Mod statuses get IDs from 100,000. A saved status keeps its name, so it comes back when its ID changes.

The sample mod in [examples/verve](examples/verve) defines a status, listens to its ticks, and has an item that starts it.

### Timed bonuses

```lua
player:add_timed_bonuses("tonic", 60000, { { "str", 5 }, { "max_hp", 100 } },
  { ends_on = { "death" }, icon = 241, kind = "buff" })
```

- The bonuses last the time given. A second call with the same key replaces them, unless `stack` is set.
- `ends_on` lists what also ends them: `death`, `dispell`, `clearance`, `logout`, `banishing_buster`, `refresh`, `lux_anima`, `magic_gear_change` and `damage`.
- They are saved across logout, unless `ends_on` has `logout`.
- Converted `bonus_script` lines key the bonuses by the script's text.

### Items

`items.id(name)` from `wa.items` returns an item's ID by name, and raises an error for an unknown name. It needs `game.read`.

## Item scripts

An item's `script` field names its entry script. The setup function runs in an item context (proposal 0002, "Item scripts"):

- A usable item runs its setup on each use, then its `use` handlers with the player. The context ends after the use.
- An equipment item or card runs its setup when a copy is equipped. Its bonuses, handlers and timers last until the copy is unequipped. A copy that is still worn when its player leaves the map ends without `unequip`, and starts again where the player arrives.
- The wearer's stats are computed again from every worn item when equipment, levels or raised stats change, and when a timed bonus starts or ends. Computed bonuses and `when` conditions run at each calculation (server/README.md, "Equipment").

```lua
-- A usable item.
local events = require "wa.events"
local items = require "wa.items"

local BLUE_BOX = items.group "BLUEBOX"

return function()
  events.on("use", function(player)
    player:item_heal { hp = { 45, 65 } }
    player:give_item_from_group(BLUE_BOX)
  end)
end
```

```lua
-- An equipment item.
local bonuses = require "wa.bonuses"
local context = require "wa.context"
local races = require "wa.races"

return function()
  local equipped = context:get_instance()
  local player = equipped.wearer
  bonuses.add("str", 2)
  bonuses.add("damage", 10, { kind = "physical", against = { race = races.demi_human } })
  bonuses.add("atk", function() return equipped.refine * 5 end)
end
```

### When a use happens

- A used item is spent before its script runs, as in rAthena. A `DelayConsume` item, such as a Fly Wing, is spent after its script, and only when the script did not fail.
- Warps that a script asks for happen after the `use` event, once the item is spent.
- An item with `unsupported` set, or with no script, is refused before anything is spent.

### Item events

| Event | Handler arguments | When |
|---|---|---|
| `use` | `player` | The item is used. |
| `damage` | `hit` | A hit the wearer deals or takes. `hit` has `source`, `target`, `kind`, `range`, `skill`, `element`, `is_critical`, `amount` and `wearer_is_source` (0002a, "Item events"). `source.kind` and `target.kind` say whether each is a player or a monster. |
| `skill_used` | `use` | The wearer uses a skill. `use` has `skill`, `level`, `target`, `x` and `y`. |
| `unequip` | `player` | The copy is unequipped, before its context ends. Converted `UnEquipScript`s run on it. |

`damage` fires for each normal attack that lands, and `unequip` when a player takes the copy off. `skill_used` fires in tests until skills exist (phase 4 step 3).

### Bonuses

`wa.bonuses` follows proposal 0002a. `bonuses.add(name, ..., options)` checks the name, the arguments and the options when it is called.

- The amount can be a function: a computed bonus. It runs again when a value it read changes, and cannot change the game: calls such as `player:give_item` or `random.integer` fail inside it.
- A skill level, as in `bonuses.add("skill", BASH, function() ... end)`, and the `chance` option can be functions too. A granted skill at level 0 grants nothing.
- The trigger of a proc is the `on` option: `bonuses.add("cause_status", statuses.stun, { chance = 300, on = "hit_taken" })`.
- The `when` option is a condition. The bonus applies while the function returns true. It is tracked like a computed bonus, and every bonus takes it:

  ```lua
  bonuses.add("unbreakable_weapon", { when = function() return equipped.refine >= 10 end })
  bonuses.add("skill", TELEPORT, 1, { when = function() return player.base_level >= 99 end })
  ```

  Converted scripts use `when` for every bonus that rAthena adds under a condition.
- `bonuses.add` returns a handle. `handle:remove()` removes that one bonus, for a bonus added in one handler and removed in another:

  ```lua
  local granted
  events.on("base_level_up", function(player)
    if player.base_level >= 99 and not granted then
      granted = bonuses.add("skill", TELEPORT, 1)
    end
  end)
  ```

  The context still removes the bonus when it ends. A second `remove()` does nothing.
- `timers.with_bonuses(ms, fn, { restart = timer })` adds the bonuses `fn` adds for `ms` milliseconds. Their handles work too: `remove()` ends one early.
- Bonuses only exist in the context of an equipped copy. `bonuses.add` in a use handler raises an error.

### Static bonuses

Equipment whose bonuses never change needs no Lua. Its `bonuses` field lists them. Each entry is the bonus name, then the arguments of `bonuses.add`, with constants and records written by name:

```yaml
- name: example.greeter:Lucky_Ring
  type: Armor
  bonuses:
    - [luk, 3]
    - [damage, 10, {kind: physical, against: {race: demi_human}}]
    - [cause_status, stun, {chance: 300}]
    - [cast, MG_FIREBOLT, 3, {chance: 500, on: hit_taken}]
```

The server checks every entry at start with the rules of `bonuses.add`. `when` is a function, so static bonuses cannot have it. [Decision 0007](../docs/decisions/0007-converted-item-scripts.md) explains the format.

### Item API

| Call | Permission | Meaning |
|---|---|---|
| `player:item_heal { hp = n, sp = n }` | None | rAthena's `itemheal`, with its bonuses from VIT, INT and skills. An amount can be `{ min, max }`, drawn as `rand`, HP first. |
| `player:heal_percent { hp = n, sp = n }` | None | rAthena's `percentheal`. |
| `player:give_item(item, amount, { rent_seconds = s })` | `player.give_items` | rAthena's `getitem`, and `rentitem` with `rent_seconds`. Rentals do not expire yet. |
| `player:give_item_from_group(group)` | `player.give_items` | rAthena's `getgroupitem`: one use of an item group from `items.group "BLUEBOX"`, with rAthena's group rules. |
| `player:warp(map, x, y)` | `player.warp` | Moves the player after the event. Returns false when the map does not run. |
| `player:warp_to_random_cell()` | `player.warp` | A Fly Wing: a random walkable cell of the map, as `pc_randomwarp`. |
| `player:warp_to_save_point()` | `player.warp` | A Butterfly Wing: the save point, or the start map's start cell without one. |
| `player:show_effect(effect)` | None | rAthena's `specialeffect2`: everyone near sees it. |
| `player:show_headgear_effect(effect)` | None | rAthena's `hateffect`. It shows nothing yet: the protocol has no message for it. |
| `player:get_skill_level(skill)` | `game.read` | rAthena's `getskilllv`. |
| `player:get_equipped(slot)` | `game.read` | The copy in a slot, such as `right_hand`, as an equipped handle, or nil. |
| `player:start_status(status, options)` | None | rAthena's `sc_start`, `sc_start2` and `sc_start4` ("Status effects" below). |
| `player:add_timed_bonuses(key, ms, bonuses, options)` | None | rAthena's `bonus_script` ("Timed bonuses" below). |

Heals need no approved permission.

Equipped handles, from `context:get_instance()` in an equip script or from `player:get_equipped`, have `item`, `refine`, `enchant_grade` (0 for none, 1 to 4 for D to A), `wearer`, `weapon_type`, `weapon_level` and `armor_level`.

Player handles gained `base_str` to `base_luk` (allocated points), and `job_class` (rAthena's `eaclass()`). `base_pow` to `base_crt` are 0 until trait stats exist.

Modules:

| Module | Gives |
|---|---|
| `wa.random` | `random.integer(min, max)` and `random.chance(numerator, denominator)`, from the zone's secret stream. |
| `wa.integers` | `divide_truncated` and `remainder_truncated`, which round as C does. |
| `wa.skills`, `wa.monsters` | `skills.id "SM_BASH"`, `monsters.id "PORING"`. |
| `wa.races`, `wa.elements`, `wa.sizes`, `wa.monster_classes`, `wa.monster_groups`, `wa.weapon_types`, `wa.statuses`, `wa.effects`, `wa.jobs` | Constants. |
| `wa.headgear_effects` | The constants of `show_headgear_effect`. |
| `wa.emotions` | The constants of `npc:show_emotion`, from rAthena's `ET_` names, such as `emotions.thanks`. |
| `wa.variables`, `wa.text`, `wa.maps`, `wa.world` | Saved variables, text, and announcements ("NPC scripts"). |
| `wa.schedules`, `wa.spawning`, `wa.hooks` | Schedules, monsters and hooks ("Schedules", "Spawning monsters", "Hooks"). |

Constant names come from rAthena's by a fixed rule (0002a, "Constants used in arguments", lists the spelled-out names): the prefix goes, the rest is in lower case, and a few names are spelled out. So `RC_DemiHuman` is `races.demi_human`, `SC_STUN` is `statuses.stun`, `EF_HEAL` is `effects.heal`, and `W_2HSWORD` is `weapon_types.two_handed_sword`. A name that starts with a digit gets an `n`: `statuses.n2011rwc_scroll`.

`wa.jobs` also has job tests: `jobs.is_transcendent`, `jobs.is_baby`, `jobs.is_third`, `jobs.is_fourth`, `jobs.base_job` (rAthena's `BaseJob`: a Lord Knight is a Knight) and `jobs.first_job` (rAthena's `BaseClass`: a Knight is a Swordman). Each takes a job, such as `player.job`.

### Core's item scripts

`waserver` converts rAthena's item scripts when it starts, from the mode's `item_scripts.json` in `reference/rathena/extracts` ([tools/convert](../tools/convert/README.md)):

- `data/items.yml` sets `script`, `bonuses` or `unsupported` on every item with a script.
- `scripts/items/<AegisName>.lua` is the entry script of each item that needs Lua. A name that is not a valid file name uses `item_<id>`.
- Each file starts with the item's rAthena scripts as a comment.

The files are not in the repository ([decision 0007](../docs/decisions/0007-converted-item-scripts.md)). To read them:

```text
go run ./tools/wactl convert items --mode renewal --out /tmp/core-items
```

To change a core item, set its `script` or `bonuses` in a mod's `data/items.yml`. The coverage report of each mode is in [docs/generated](../docs/generated).

A zone loads an item's script the first time a player uses it there, and keeps at most 256 loaded.

## Core's NPCs

`waserver` converts rAthena's NPC scripts and shops when it starts, from the mode's `npcs.json` and `item_prices.json` in `reference/rathena/extracts` ([decision 0008](../docs/decisions/0008-converted-npcs.md)):

- `data/npcs.yml` places every NPC that converts and every zeny shop.
- `scripts/npcs/<name>.lua` is the entry script of a script NPC. Its duplicates name the same file.
- `scripts/functions/<name>.lua` is an rAthena function, which scripts load when they first call it.
- `scripts/lib/rathena.lua` has helpers the converted scripts share: arrays from 0, menus written as `"A:B"`, and `end` from inside a function.

To read them:

```text
go run ./tools/wactl convert npcs --mode renewal --out /tmp/core-npcs
```

To change a core NPC, set its fields in a mod's `data/npcs.yml` with its full name, such as `core:kaf_prontera`. Kafra storage and pushcart rental say they are not available yet. The coverage report of each mode is in [docs/generated](../docs/generated).

## Tools

`wactl` has the mod commands (strategy section 6.9). They carry the reference data, so they work outside the repository. Build `wactl` with `go build ./tools/wactl`, or run it with `go run ./tools/wactl` in the repository.

| Command | What it does |
|---|---|
| `wactl mod new <id> [--dir <folder>]` | Makes a mod from a template: a manifest, an NPC with a conversation that uses saved variables, an item overlay, a test, `locales/en.yml`, a README, and the editor definitions. It passes `validate` and `test` at once. |
| `wactl mod validate [path] [--mods <folder>] [--show]` | Checks the mod: the manifest, the data files and their IDs, the rulesets it loads on, its permissions, the Lua syntax of every file, that entry scripts return a function, that required modules exist, and that test files register cases. `--show` prints the final value of every record the mod adds or changes, and which mod set each field. |
| `wactl mod test [path] [--mods <folder>] [--run <text>] [--assets <build>]` | Runs the mod's tests ("Testing a mod" below). |
| `wactl dev [path] --assets <build> [--mods <folder>]` | Runs a local server with the mod, and reloads it when its files change. |
| `wactl mod pack [path] [--out <folder>]` | Validates the mod, then writes `<id>-<version>.zip` and `<id>-<version>.zip.sha256` into `dist`. |
| `wactl mod install <git URL \| .zip \| folder> --mods-dir <folder>` | Checks a mod and installs it for a server ("Installing a mod" below). |
| `wactl mod types [path]` | Writes the editor definitions into `.wathena/types` again, and `.luarc.json` when it is missing. |

- `path` is the mod's folder, or the working folder when it is left out. `validate` and `test` also take a `.zip` file.
- `--mods` names a folder of other mods, where the mod's dependencies are, as `mods.dir` does for the server.
- The commands check against renewal data. `--mode pre-renewal` checks against pre-renewal data.
- Problems are printed with their file and line, and the command exits with status 1.

```text
$ wactl mod new example.demo
made example.demo in example.demo:
  README.md
  data/items.yml
  data/npcs.yml
  locales/en.yml
  mod.yaml
  scripts/npc/guide.lua
  tests/guide_test.lua
  .luarc.json
  .wathena/types/ (26 files)
next: cd example.demo && wactl mod validate && wactl mod test
$ cd example.demo
$ wactl mod validate
example.demo 0.1.0 (.)
loads on: core.ruleset.renewal, core.ruleset.pre_renewal
checked: 2 Lua files, 2 test cases
permissions:
  player.give_items    needs the owner's approval: give items to players
nothing is approved. To approve, review the mod, then add to the server config:
  mods:
    approve:
      example.demo: [player.give_items]
ok
```

### Testing a mod

Test files are `tests/*_test.lua`. Like an entry script, a test file returns a function. That function registers cases with `testing.case` from `wa.testing` ([proposal 0002](../docs/proposals/0002-lua-api.md), review decision 15).

```lua
local testing = require "wa.testing"
local items = require "wa.items"
local variables = require "wa.variables"

local BADGE = items.id "example.demo:Guide_Badge"
local visits = variables.character_integer "visits"

return function()
  testing.case("the guide gives one badge", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 183 }
    local said = world:talk(player, "Guide#prontera", { 1 })
    testing.expect(said).to_contain("Here is your badge.")
    testing.expect(player:count_item(BADGE)).to_be(1)
    testing.expect(visits:get(player)).to_be(1)
  end)
end
```

- Each case runs in a fresh headless world: the real zone code without a network, with the mod, its dependencies and core loaded, core's converted NPCs and item scripts, a fixed random seed, and no database. A case takes a few milliseconds.
- A world runs one map: the map of its first `new_player` or `spawn_monster`, or `prontera` when `advance` comes first. Warps to other maps do not happen.
- Without `--assets`, every cell is walkable. With an asset build, the map's real cells are used.
- Test code runs as the mod: its calls need the mod's permissions, and the variables it declares are the mod's, so a test reads what the mod's NPCs saved.
- `wa.testing` exists only in `wactl mod test`. A server never has it, and never loads `tests/`.

| Call | What it does |
|---|---|
| `testing.case(name, function(world))` | Registers a case. |
| `world:new_player { map, x, y, job, base_level, job_level, zeny, items = { {item, amount} } }` | Adds a character at a walkable cell, with full HP and SP, and returns its player handle. Only `x` and `y` are required. |
| `world:talk(player, npc_name, answers)` | Clicks the NPC and answers each step in order: a number picks a menu option, a string fills text input, and an integer fills number input. Next is pressed without an answer, and waits and progress bars run on the clock. Returns the NPC's lines, one per line. Fails when the conversation asks for more answers than given, when one does not fit, or when some are left over. |
| `world:use_item(player, item)` | Uses one of the player's items. Fails when the player has none, or the use is refused. |
| `world:advance(milliseconds)` | Moves the zone clock. Timers fire. The clock starts at 0 and moves only with this call, conversation waits, and the item use interval. |
| `world:spawn_monster { monster, map, x, y }` | Places a monster from `monsters.id` on a cell and returns its handle. It does not come back after it dies. |
| `world:kill(player, monster)` | The player kills a living monster: `death` fires, and the player gets its exp and drops. After it, the monster's handle raises an error. |
| `world:set_time(time, time_zone)` | Sets the server time and time zone the world starts at, such as `"2026-10-31 19:58"` and `"Asia/Manila"`. Call it before the world has a map. Without it, the world starts at 2026-01-05 12:00 UTC. |
| `world:get_time()` | The server time, as `"2026-10-31 20:00"`. |
| `world:get_monsters(monster)` | Handles of the living monsters of the map, of one ID or all, in the order they spawned. |
| `world:get_announcements()` | Every line announced to the world or the map so far. |
| `world:count_floor_items(item)` | How many of an item lie on the map. |

The last five, and world events in test worlds, were added in phase 3 step 6 (proposal 0002, review decisions 16 and 17). A test world runs the world events of its map, and of no map, on its clock: `world:advance` past an event's time starts it, and past its end ends it.
| `testing.expect(value)` | Checks a value with `.to_be(x)`, `.to_equal(table)` (deep), `.to_contain(text_or_value)`, `.to_be_true()`, `.to_be_false()`, `.to_be_nil()`, or `.to_fail_with(text)` for a function that should raise an error containing the text. |

A failed check stops the case and prints the file and line of the check. A case also fails when the server logs a warning that names the mod, such as a handler that failed. `wactl mod test` exits with status 1 when any case fails, so CI can run it.

```text
$ wactl mod test
tests/guide_test.lua
  ok   the guide gives one badge (7 ms)
  FAIL the guide counts visits (5 ms)
       tests/guide_test.lua:28: expected "[Guide]\nWelcome to Prontera!\nYou have visited me 2 times.\nCome back any time." to contain "You have visited me 3 times."
2 cases: 1 passed, 1 failed (1.6 s)
```

### A local server: wactl dev

`wactl dev` runs `waserver` on loopback addresses with the mod and watches its files:

```text
$ wactl dev --assets <build>
wactl dev: ready at http://127.0.0.1:8300; editing . reloads it
wactl dev: changed scripts/npc/guide.lua
wactl dev: reloaded example.demo in 1 zones
wactl dev: changed data/items.yml
wactl dev: the manifest or data changed: restarting the server
wactl dev: ready at http://127.0.0.1:8300; editing . reloads it
```

- A change to a Lua file or `locales/en.yml` reloads the mod's scripts in every zone, through the server's reload endpoint. A Lua file that does not compile is reported and not reloaded.
- A change to `mod.yaml` or a data file restarts the server.
- The server runs with `--dev`, `--shutdown-countdown 0`, and the mods of `--mods`. It approves what the mods request, for development only.
- `--server` names the `waserver` binary. By default it is the one next to `wactl`, then the one on `PATH`. `--addr` and `--debug-addr` set the addresses, `127.0.0.1:8300` and `127.0.0.1:8301` by default, and `--map` the start map.
- Ctrl-C stops the server and `wactl dev`.

The reload endpoint also works without `wactl dev`: run `waserver --dev --debug-addr 127.0.0.1:8301 --mods <folder>`, and after a change:

```text
curl -X POST 'http://127.0.0.1:8301/debug/wathena/mods/reload?mod=example.greeter'
```

### Packing and installing

`wactl mod pack` validates the mod, then writes `dist/<id>-<version>.zip`, with the files in one top folder, and `dist/<id>-<version>.zip.sha256` in `sha256sum`'s format. The same files give the same archive.

`wactl mod install <source> --mods-dir <folder>` installs a mod into a server's mods folder:

- A Git URL is cloned. `--ref` checks out a tag, branch or commit, and the commit is printed.
- A `.zip` file is checked against `--sha256`, or against `<file>.sha256` when it exists. With `--public-key`, `<file>.minisig` must be a matching [minisign](https://jedisct1.github.io/minisign/) signature. Authors sign with `minisign -Sm <file>`.
- A folder is copied as it is.
- The mod must pass `wactl mod validate` against core and the mods already installed.
- The permissions it requests are shown, with those that need approval. Nothing is approved: the owner reviews them and adds the printed lines to the server config.
- A mod with the same ID already installed is an error, unless `--replace` is given.

```text
$ wactl mod install dist/example.demo-0.1.0.zip --mods-dir /srv/mods
checksum ok: SHA-256 fcf439cc...c69a (from dist/example.demo-0.1.0.zip.sha256)
example.demo 0.1.0: Demo
permissions:
  player.give_items    needs the owner's approval: give items to players
nothing is approved. To approve, review the mod, then add to the server config:
  mods:
    approve:
      example.demo: [player.give_items]
installed /srv/mods/example.demo-0.1.0.zip
```

### Editor support

The Lua Language Server gives completion and type warnings for every `wa.*` module, handle, event name and bonus name.

- `wactl mod new` writes the definitions into `.wathena/types` and a `.luarc.json` that names them. `wactl mod types` writes them again after a `wactl` update.
- The definitions come from [server/modscript/luaapi](../server/modscript/luaapi), which tests check against the runtime. The same source makes the [API reference](../docs/generated/lua-api.md) and the [constants](../docs/generated/lua-constants.md).
- In this repository, `docs/generated/lua` holds them, and `examples/greeter/.luarc.json` names that folder.
