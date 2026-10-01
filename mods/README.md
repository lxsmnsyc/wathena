# Making a mod

A mod adds or changes game content without changes to the server (strategy section 6). This guide covers what the mod loader supports today, in phase 3 step 2. The scripting API is [proposal 0002](../docs/proposals/0002-lua-api.md). It is experimental and can change in any release until API 1.0.

- [core](core) is the base game as a mod. It is built into the server and has no files yet.
- [examples/greeter](examples/greeter) is a small sample: an item, an NPC, and a script that greets players.

Server owners install mods in the folder that `mods.dir` names. [server/README.md](../server/README.md), "Mods", covers the server side.

## Folders

A mod is a folder, or a `.zip` file with the same files at its root or in its one top folder.

```text
greeter/
  mod.yaml              the manifest
  data/
    items.yml           item overlay
    npcs.yml            NPC placements
  scripts/
    main.lua            an entry script that mod.yaml lists
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
| `scripts` | no | Entry scripts that run once per zone, in the mod's context. Paths are under `scripts/`. This field is provisional. |

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
| `player.give_items`, `player.give_zeny`, `player.give_exp`, `player.warp`, `player.status`, `events.schedule` | Needs the owner's approval. |
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
  heal: { command: itemheal, hp: [20, 30], sp: [0, 0] }
- name: Red_Potion
  weight: 50
```

- Names are AegisNames, such as `Red_Potion`.
- Fields are those of the item extract: `type`, `weight` (in tenths), `stackable`, `stack_limit`, `unique_id`, `no_consume`, `no_drop`, `delay_ms`, `delay_status`, `min_level`, `max_level`, `gender`, `job_restricted`, and `heal`.
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

- A name without a prefix is the mod's own NPC: `Greeter#prontera` in `example.greeter` is `example.greeter:Greeter#prontera`. Core has no NPCs yet.
- Players see the name up to `#`, or `display_name` when it is set.
- `map`, `x`, `y` and `sprite` are required. `sprite` is an NPC sprite name, such as `4_F_KAFRA1`, or its number.
- `direction` is `south` (the default), `southwest`, `west`, `northwest`, `north`, `northeast`, `east` or `southeast`.
- `script` names the NPC's entry script. It must exist. Clicking NPCs comes in phase 3 step 4, so the script does not run yet.

Kinds not overlaid yet: monsters, skills, jobs, spawns, quests, achievements, and events.

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
| `monster_killed` | `player`, `monster` | The player killed a monster. `monster` is a read-only table of what it was: `id`, `name`, `level`, `x`, `y`. |
| `level_up` | `player`, `kind` | The player gained a level. `kind` is `"base"` or `"job"`. |

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
| `player:send_message(text)` | None. Provisional. |
| `player:give_item(item, amount)` | `player.give_items`. Returns `false` when the items do not fit. |
| `player:count_item(item)` | `game.read`. |

### Items

`items.id(name)` from `wa.items` returns an item's ID by name, and raises an error for an unknown name. It needs `game.read`.

## Provisional parts

These are not in proposal 0002, or go further than it. They can change:

- The `scripts` field of mod.yaml, for a mod's own entry scripts.
- `api: experimental`.
- `player:send_message(text)`, which shows a line in the player's chat log, sent as `CHAT_CHANNEL_SYSTEM`.
- The fields of a mod context's instance: `id` and `map`.
- The `monster` argument of `monster_killed`, a read-only table instead of a handle, since the monster is gone.
- The `kind` argument of `level_up`: `"base"` or `"job"`.
- The NPC fields `direction`, `display_name`, and `sprite` names.
- The player fields `max_sp`, `character_id` and `account_id`.

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
