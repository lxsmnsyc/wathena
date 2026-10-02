# Making a mod

A mod adds or changes game content without changes to the server (strategy section 6). This guide covers what the mod loader supports today, in phase 3 step 4. The scripting API is [proposal 0002](../docs/proposals/0002-lua-api.md). It is experimental and can change in any release until API 1.0.

- [core](core) is the base game as a mod. It is built into the server. Its files are rAthena's item scripts, NPCs and shops, which the server converts when it starts ("Item scripts" and "Core's NPCs" below).
- [examples/greeter](examples/greeter) is a small sample: an item, an NPC with a conversation and saved variables, a shop, and a script that greets players.

Server owners install mods in the folder that `mods.dir` names. [server/README.md](../server/README.md), "Mods", covers the server side.

## Folders

A mod is a folder, or a `.zip` file with the same files at its root or in its one top folder.

```text
greeter/
  mod.yaml              the manifest
  data/
    items.yml           item overlay
    npcs.yml            NPC placements and shops
  locales/
    en.yml              text for players, read with wa.text
  scripts/
    main.lua            an entry script that mod.yaml lists
    npc/greeter.lua     an NPC's entry script, which npcs.yml names
    lib/messages.lua    a helper module
```

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
- Fields are those of the item extract: `type`, `weight` (in tenths), `stackable`, `stack_limit`, `unique_id`, `no_consume`, `no_drop`, `delay_ms`, `delay_status`, `min_level`, `max_level`, `gender`, and `job_restricted`, and `name`, `buy`, `sell` and `no_sell`: the name `items.name` gives, the prices of NPC shops in zeny, and whether shops buy the item.
- `script` is the item's entry script, a path under the mod's `scripts/`. It belongs to the mod that sets it.
- `bonuses` are static bonuses of an equipment item ("Static bonuses" below).
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

Kinds not overlaid yet: monsters, skills, jobs, spawns, quests, achievements, and events.

## Text: locales/en.yml

Text for players lives in `locales/en.yml`, a mapping of keys to text. Nested keys are joined with dots:

```yaml
greeter:
  hello: Hello! Welcome to Prontera.
```

`text "greeter.hello"` from `wa.text` reads it. Players have no language setting yet, so the English file is the only one read. A key the file lacks raises an error.

## IDs

The server gives each new record an ID in the mod range of its kind the first time it loads it. Items start at 2,000,000. Monsters start at 100,000, skills at 20,000, jobs at 10,000, and quests and achievements at 1,000,000, for when those kinds get overlays.

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
| `monster_killed` | `player`, `monster` | The player killed a monster. `monster` is a monster handle. |
| `base_level_up` | `player` | The player gained a base level. |
| `job_level_up` | `player` | The player gained a job level. |
| `click` | `player`, `conversation` | NPC contexts only: a player talks to the NPC ("NPC scripts" below). |

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

`monster_killed` gives the handler a read-only monster handle. It works like a player handle.

| Field | Meaning |
|---|---|
| `id`, `name`, `level` | The monster's ID, such as 1002, its name and level. |
| `hp`, `max_hp` | Current and maximum HP. |
| `map`, `x`, `y` | The zone's map name and the cell. |
| `is_alive` | `false` for a killed monster. |

- The killed monster is gone from the map, but its handle stays readable until the event's handlers have run.
- After that, reading the handle raises an error, like a handle to a player who left. A timer that the handler starts cannot read it.
- Reading needs `game.read`.

### Items

`items.id(name)` from `wa.items` returns an item's ID by name, and raises an error for an unknown name. It needs `game.read`.

## Item scripts

An item's `script` field names its entry script. The setup function runs in an item context (proposal 0002, "Item scripts"):

- A usable item runs its setup on each use, then its `use` handlers with the player. The context ends after the use.
- An equipment item or card runs its setup when a copy is equipped. Its bonuses, handlers and timers last until the copy is unequipped. Equipment comes with phase 4, so today only tests equip items.

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
| `damage` | `hit` | A hit the wearer deals or takes. `hit` has `source`, `target`, `kind`, `range`, `skill`, `element`, `is_critical`, `amount` and `wearer_is_source` (0002a, "Item events"). |
| `skill_used` | `use` | The wearer uses a skill. `use` has `skill`, `level`, `target`, `x` and `y`. |
| `unequip` | `player` | The copy is unequipped, before its context ends. Converted `UnEquipScript`s run on it. |

`damage`, `skill_used` and `unequip` fire in tests until phase 4.

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
| `player:start_status(...)`, `player:add_timed_bonuses(...)` | | Raise an error until status effects and timed bonuses exist (phase 4). |

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

## Testing a mod

Until `wactl mod test` exists (phase 3 step 5), run a local server with the folder that holds the mod. The loader looks one level down: `mods/examples` finds `mods/examples/greeter`.

```text
waserver --assets <build> --map prontera --dev --debug-addr 127.0.0.1:8301 --mods mods/examples
```

After changing a script, reload it without a restart:

```text
curl -X POST 'http://127.0.0.1:8301/debug/wathena/mods/reload?mod=example.greeter'
```

A reload reads the Lua files again. Changes to mod.yaml and data files need a restart.
