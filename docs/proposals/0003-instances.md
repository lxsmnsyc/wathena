# 0003: Instances

Instances are rAthena's memorial dungeons: a private copy of one or more maps, made for a character, a party or a guild, with its own monsters, NPCs and variables, and an end. This proposal covers the records, the lifecycle, the zones, the variables, the NPC copies, the Lua API `wa.instances`, the converter, the protocol and the admin tools. It adds to API 1.0 as proposal 0002's review decisions do.

In this file "instance" means a memorial dungeon instance. Proposal 0002's "instance of a script" (the object of `context:get_instance()`) is called a script instance here.

## Problem

- `variables.instance_*` refuses with "instances do not exist yet" (server/modscript/variables.go:69). `'x` variables do not convert (tools/convert/npclua.go:582, npcbuiltins.go:644).
- NPCs on instance maps (map names with `@`, such as `1@tower`) do not convert at all (tools/convert/npc.go:553). 0002b category C counts 1,392 renewal script NPCs on instance maps, 736 uses of `'x`, 304 `OnInstanceInit` labels, 259 scripts with `instance_npcname`, 185 with `instance_mapname`, and 55 with `instance_create`. The extract leaves most of the `instances` folder out today (extract-folders adds it: 2,860 renewal and 296 pre-renewal NPCs).
- The world runs one zone per map name (server/world/world.go). There is no way to run two copies of one map, and no owner, timer or end for a group of maps.
- The client has no memorial dungeon window, no timers and no destroy notice.

What rAthena does (src/map/instance.cpp, script.cpp 21640 to 22420, map.cpp 2821 to 2990, npc.cpp 4680):

- `db/re/instance_db.yml` has 78 records in renewal and 4 in pre-renewal: `Id`, `Name`, `TimeLimit` (seconds, 3,600 by default, 0 for none), `IdleTimeOut` (300 by default, 0 for none), `NoNpc`, `NoMapFlag`, `Destroyable` (true by default), `Enter` (map, x, y), and `AdditionalMaps`. 20 records have more than one map; Endless Tower has 6.
- `instance_create(name{,mode{,owner}})` makes one instance per owner. Modes are `IM_NONE` (owned by the NPC), `IM_CHAR`, `IM_PARTY` (the default), `IM_GUILD` and `IM_CLAN`. It returns the instance ID, or -1 (unknown record), -2 (no owner), -3 (the owner has one) or -4 (no free ID). The time limit starts at creation. Creation joins a queue (`instance_wait`); the maps are made at once when the queue is empty.
- Each map is a copy: `map_addinstancemap` copies the cells and, unless `NoMapFlag`, the map flags, and names the copy `%04u#%0*u` from the source map ID and the instance ID. Spawn lines are not copied: monsters come from scripts.
- Unless `NoNpc`, every NPC of each source map is duplicated onto the copy as `dup_<instance>_<npc id>` (`npc_duplicate4instance`). Warps whose target is a map of the instance are pointed at the copy. Then `OnInstanceInit` runs on every copy, map by map. `OnInstanceDestroy` runs before the maps go.
- The idle timer starts when the maps are made and when the last player leaves; it stops when a player enters. At either timer, or on request, `instance_destroy` runs `OnInstanceDestroy`, sends every player on the maps to their save point, removes NPCs, monsters, floor items and skill units, drops the `'x` variables, and tells the owner why (`IN_DESTROY_LIVE_TIMEOUT`, `IN_DESTROY_ENTER_TIMEOUT`, `IN_DESTROY_USER_REQUEST`).
- `'x` variables belong to the instance of the NPC that runs the script (`script_instancegetid` with `IM_NONE`). `getinstancevar` and `setinstancevar` name an instance.
- A party member who leaves the party while on its instance goes to the save point. A party that breaks up destroys its instance. `conf/battle/instance.conf` blocks leaving, leader changes, invites and expulsions while a party or guild has an instance (all yes by default).
- `feature.instance_allow_reconnect` (yes in renewal, no in pre-renewal) saves the source map, the cell and the instance ID at logout, and puts the player back if the instance still runs and the player still belongs to its owner (`pc_lastpoint_special`). Otherwise the player goes to the save point.
- Instances are not saved. `do_final_instance` destroys them all at shutdown, and IDs start at 1 again.

What the scripts use (npc/re/instances and npc/instances, 72,439 lines): `instance_npcname` 7,977 times, `instance_mapname` 531, `instance_id` 162, `instance_create` 54, `instance_enter` 48, `instance_announce` 33, `instance_warpall` 26, `instance_live_info` 14 (only `ILI_NAME`), `OnInstanceInit` 307, `OnInstanceDestroy` 0, `getinstancevar` and `setinstancevar` 0. Every `instance_create` uses the default party mode. Around these: `monster` and `areamonster` with labels on `'map_name$` (3,890), `donpcevent instance_npcname(...)+"::OnX"` (1,164), `enablenpc`, `disablenpc`, `hideonnpc` with `instance_npcname`, NPC timers (`initnpctimer` 477), `mobcount`, `killmonster`, `npctalk`, `mapannounce`, `duplicate` (2,145) and quests as lockouts (`setquest`, `checkquest`). Faceworm's Nest is typical: an NPC in `dali` calls `instance_create` for the leader, a second NPC calls `instance_enter`, and a chain of hidden NPCs on `1@face` spawn waves on `'map_name$ = instance_mapname("1@face")` and show the next NPCs as each wave dies.

## Change

### 1. Instance records

A new data kind, `data/instances.yml` (`mod.InstanceKind`: `CoreNames`, `LocalNames`). Core's file is converted from `instance_db.yml` of the mode, as core's `data/quests.yml` is from `quest_db.yml`. A core record's name is rAthena's `Name`, such as `Endless Tower`, since scripts create instances by that name. A mod's records are `<mod>:<name>`.

```yaml
- name: Endless Tower
  id: 1                      # rAthena's Id, for instance_info(IIT_ID); mods' records get IDs from 100,000
  time_limit_seconds: 14400  # 0: no limit. Default 3600.
  idle_timeout_seconds: 300  # 0: no idle timeout. Default 300.
  enter: {map: 1@tower, x: 50, y: 355}
  maps: [2@tower, 3@tower, 4@tower, 5@tower, 6@tower]
  copy_npcs: true            # rAthena's NoNpc, inverted
  copy_map_flags: true       # rAthena's NoMapFlag, inverted
  destroyable: true          # the client's destroy button may end it
```

- `id` is fixed: a mod cannot change it on a record it did not add. Every other field can be changed by any mod, such as a longer `time_limit_seconds` for Endless Tower. None is an economy field.
- At load, each map must exist in the asset build (or in the world layout, see section 12), `enter` must be a walkable cell of `enter.map`, a record has at most 16 maps, and a map appears once per record. A record that fails is left out with a warning that names it, as other data kinds do.
- The maps of every loaded record are **base maps**. A base map does not run as a zone of its own. Its cells are loaded once, at the first creation, and shared by every copy. Its NPC records and warps are templates for the copies. A warp, `@warp` or `player:warp` to a base map by name is refused. rAthena keeps base maps running; nothing reaches them but staff, and their NPCs' `OnInit` labels run once. wAthena runs nothing there. A script that names a base map NPC by its plain name, not through `instance_npcname`, finds nothing.
- The extract adds `instances.json` (from `instance_db.yml`) and the map flags of base maps for both modes. The fork's `tools/wathena/extract.py` writes them.

### 2. Lifecycle

An instance has a state: `pending`, `running` or `ended`.

**Create.** `instances.create(name, owner)`:

1. Fails with `"unknown"` for a record that is not loaded (rAthena -1), `"no_owner"` when the owner is nil but the mode needs one, or the party or guild does not exist (-2), `"exists"` when the owner has an instance already, pending or running (-3), and `"full"` when the world has `instances.max_running` instances or the queue has `instances.max_queued` (-4).
2. Records the instance as `pending`, with a new ID, and puts it in the creation queue. The time limit starts now, as rAthena's `instance_startkeeptimer` at creation.
3. Tells the owner's online members its place in the queue (`InstanceStatus`, section 9).

**Build.** One goroutine of the instance registry builds the queue's instances one at a time, in order, so a burst of creations does not make every zone wait. For each map, the enter map first and then `maps` in order:

1. The base map's grid, loaded on first use and kept.
2. A new zone (`zone.New`) with the shared grid, no spawn lines, the copied warps and the copied NPCs (section 5), and the settings that `newZone` in server/cmd/waserver/main.go gives every zone. `World.Add` puts it in the world.
3. The zone starts the setup functions of its copied NPCs at once, not at the first player's arrival as other zones do (`startNPCs`), and reports back.

When every map's NPCs have started, the registry sends `instance_start` to every copied NPC, map by map in the same order, and waits for each zone to run them before the next. The instance becomes `running`, its idle timer starts, and the owner's members get `InstanceStatus`. A build that fails (a zone does not start within 10 seconds) ends the instance with the reason `create_failed`, as rAthena's `IN_CREATE_FAIL`.

**Enter.** `instance:enter(player, x, y)` (`instance_enter`):

- Refuses with `"no_member"` when the player is not in the owning party or guild (`IE_NOMEMBER`), `"not_running"` while the instance is pending or ended (`IE_NOINSTANCE`), and `"other"` when the player is not the owner of a character instance or the move fails (`IE_OTHER`).
- Without x and y, the player arrives at the record's `enter` cell. Otherwise at x, y of the enter map.
- It is a warp, handed to the instance zone as any warp is.

The only ways onto an instance map are `instance:enter`, a warp or script move from another map of the same instance, a reconnect (below), and staff commands. A move from anywhere else to an instance map ID is refused.

**Idle timeout.** The registry counts the players on the instance's maps from each zone's shared player count (`mapShared.players`). When the count goes to 0 the idle timer starts, with `idle_timeout_seconds`; when a player arrives it stops. Owners get `InstanceStatus` with the new idle end, or none.

**Time limit.** `time_limit_seconds` after creation the instance ends with `time_limit`.

**Destroy.** An instance ends for one of these reasons: `time_limit`, `idle_timeout`, `owner_request` (the client's destroy button, section 9), `script` (`instance:destroy()`), `staff` (the admin screens), `owner_gone` (its party broke up, its guild broke up, or its owner character was deleted) and `create_failed`. Steps, in order:

1. The state becomes `ended`. New enters, creates for the same instance and timers stop.
2. Every zone of the instance runs the `instance_end` handlers of its NPC and mod contexts, with the reason, and gets at most 5 seconds.
3. `World.Remove` takes each zone out, so warps and logins no longer find it.
4. Each zone moves its players to their own save points, as `map_instancemap_leave`'s `pc_setpos_savepoint`. A player who arrives after this, such as one on a warp between two maps of the instance, goes on to its save point. This needs a variant of `Zone.Evacuate` (server/zone/worldchange.go) that sends each player to its own save point instead of one cell.
5. The `'x` variables of the instance go.
6. Owners' online members get `InstanceEnded` with the reason.
7. Each zone is retired (`World.Retire`) 30 seconds later, as rotation's `retireAfter`, which saves and drops anyone still there.

This reuses the rotation procedure of decision 0045 (Remove, Evacuate, Retire after a delay) without the countdown warnings, since rAthena sends none: the client's window shows the time left all along.

**Who belongs.** A player belongs to an instance when it is the owner character (character mode), a member of the owning party (party mode) or guild (guild mode), or always (mode `none`).

- A party member who leaves the party, or is expelled, while on the party's instance goes to the save point (party.cpp 838). The same for guilds (guild.cpp 1292).
- A party or guild that breaks up ends its instance with `owner_gone` (party.cpp 862).
- The ruleset rules `instance_block_leave`, `instance_block_leader_change`, `instance_block_invite` and `instance_block_expulsion` (all on by default, as `conf/battle/instance.conf`) refuse those party and guild actions while the party or guild has a pending or running instance. The client gets the refusal as each action's usual failure result.

**Logout and login.** The ruleset rule `instance_allow_reconnect` (on in renewal, off in pre-renewal, as `feature.instance_allow_reconnect`):

- On, a player who logs out on an instance map is saved on that map ID and cell, such as `instance:42:1@tower`. At login, `World.JoinCharacter` puts it back when instance 42 runs, has that map, and the player belongs to it. Otherwise the player enters at its save point.
- Off, the player is saved at its save point, as rAthena's `pc_makesavestatus`.
- An instance map is never a save point or a memo point: `player:set_save_point` on it returns false, and `/memo` refuses, as rAthena's "You cannot create a memo in an instance".

**Restart.** Instances are lost when the server stops. `World.Run`'s stop saves each player in an instance zone on the instance map, as a logout does, and at the next start those maps do not run, so the players enter at their save points. Why:

- An instance is mostly memory that has no saved form: its monsters, the state of every copied NPC's Lua locals and timers, cells changed by `set_cells`, floor items and `'x` variables. Saving it would mean saving Lua states, which no part of wAthena does.
- rAthena loses them too, and its scripts expect it: lockout quests (`setquest`) are set at entry, so a lost instance costs the party a run, not items.
- A planned restart warns first (`restart_planned`), and the admin screens show the running instances (section 10), so staff can wait.

**IDs.** Instance IDs are positive integers from a counter in memory that starts at 1 at each start, as rAthena's `instance_count`. After a restart an ID can be used again. Reconnect checks the owner and the map, as rAthena does, so a player saved in an old instance 42 enters a new instance 42 only if it belongs to it and it has the same map.

### 3. Instance maps and zones

- **Map ID.** `instance:<id>:<base map>`, such as `instance:42:1@tower`, as generated maps use `gen:` (decision 0040). It fits the 64 characters of `characters.map`. Scripts, `player.map`, `npc.map`, `strcharinfo(3)` and the database see it. It is the key of the zone in the world.
- **Asset.** The zone gets a new field, `Asset`, the base map's asset name, such as `map/1@tower`. `Welcome.map` and `MapChange.map` send `Asset`, so the client loads the base map and needs no change to show it. For every other zone `Asset` is `Map`.
- **Zone lookups.** `mapZone` (server/zone/maps.go) adds `map/` to plain names; names with a `:` prefix, `instance:` and `gen:`, are looked up as they are.
- **Cells.** The copy starts from the base map's shared grid. A `set_cells` change copies it for that zone only (`ensureOwnGrid`), so instances never change each other's cells.
- **Map flags.** With `copy_map_flags`, the copy starts with the base map's flags from the extract. Flags wAthena builds (only `nomemo` today) work; the rest stay refused as now. Instance maps are `nomemo` always. Base maps carry `noteleport`, `nowarpto`, `nobranch`, `partylock`, `restricted 6` and `monster_noteleport` in rAthena (113 to 115 maps each); building their effects is content work (section 13).
- **Spawns.** None. Monsters come from the copied NPCs' scripts, as in rAthena.
- **Sleep.** An instance zone sleeps without players, as every zone does. The idle timeout and the time limit run on the registry's server clock, not the zone clock. Converted NPC timers use the server clock (review decision 60), so they wake the zone as they do now.
- **Limits.** In the server config, `instances` block:
  - `max_running`: instances pending or running in the world, 100 by default until instances-core measures the cost below.
  - `max_queued`: instances waiting to be built, 50 by default.
  - One instance per owner, pending or running, as rAthena. A character may own one character instance and belong to a party and a guild that each own one.
  - At most 16 maps per record (section 1).
- **Budget.** Strategy section 3.8: a 50 ms tick with tick work p99 under 10 ms per zone, and under 256 KB per online player. An instance map is a zone like any other, so its own ticks have the same budget, and they are small: few players and the monsters its NPCs spawn. The new costs are memory per instance map and the build:
  - The grid is shared. Entities, the block index and the outbox are the zone's usual cost.
  - Each instance map makes its own Lua state at build, since its NPCs start at once. The prelude (3,298 lines) and `rathena.lua` are compiled in each state (decision 0056, not built). This is the largest cost, and nothing measures it yet.
  - instances-core measures, with the kRO assets: the build time and the live heap of one Faceworm's Nest (1 map) and one Endless Tower (6 maps), and the tick work of every other zone while 20 builds run back to back. Targets: under 8 MB of live heap per instance map, a build under 500 ms of the registry goroutine's wall time per map, and no other zone's p99 over 10 ms during builds. It sets `max_running`'s default from the heap number, so the default uses at most 1 GB, and records the numbers in its decision record. If the Lua state cost is over the target, decision 0056's steps come before raising the limit.

### 4. Ownership

| Mode | rAthena | Owner handle | Belongs |
|---|---|---|---|
| `character` | `IM_CHAR` | `wa.Player` | The owner character |
| `party` | `IM_PARTY` | `wa.Party` | Its members |
| `guild` | `IM_GUILD` | `wa.Guild` | Its members |
| `none` | `IM_NONE` | nil | Everyone |

Clans do not exist in wAthena: there is no clan system, and `getcharid(5)` is refused already. `IM_CLAN` and `instance_check_clan` are refused with "clans do not exist". The owner of a `none` instance in rAthena is the creating NPC; wAthena records the creating context's mod and NPC for the admin screens only.

The owner is kept by ID: character ID, party ID or guild ID, which are stable while the server runs. The registry asks the social registry (server/social) for members at each check, so a party's new member belongs at once.

### 5. NPC copies

With `copy_npcs`, each instance map gets a copy of every NPC record of its base map, from every mod, in load order:

- The copy's record name is the base record's full name, a colon, and the instance ID, such as `core:Chaos#01:42`. Record names cannot hold a colon after the mod ID (`validLocalName`), so a copy's name never collides with a record. The world's NPC directory lists it under that name, and `world.get_npc` finds it.
- `name`, `full_name` and `hidden_name` are the base record's, as rAthena's duplicate keeps `name` and changes only `exname`. `unique_name` is the copy's, such as `Chaos#01:42`, as `strnpcinfo(3)` returns `dup_42_<id>`.
- `npc.map` is the instance map ID, and `npc.instance` is the instance (section 7).
- **NPC variables** (`.x`, `variables.npc_*`) of a copy are keyed by the base record's name. Every copy in every instance, and the base record, share them, as rAthena's duplicates share their source's `script_code->local`. Per-instance state is in `'x` variables, as rAthena's scripts already do.
- Each copy runs its script's setup function, so its Lua locals are its own.
- A copied warp whose target is a map of the same instance targets that instance's copy (`npc_duplicate4instance`'s warp case). A warp to another map keeps its target.
- NPCs without a map and NPCs on other maps are not copied.
- Records added by `npcs.control` calls at run time are not copied; only records of data files are.

`OnInstanceInit` is the `instance_start` event. `instance_npcname(name)` is `instance:get_npc(name)` (a handle) or `instance:get_npc_name(name)` (the copy's record name, for the converter's strings). `instance_mapname(map)` is `instance:get_map(map)`.

### 6. `'x` variables

- Declared as other scopes: `variables.instance_integer "map_name"`, `instance_boolean`, `instance_string` and `instance_table`. The declaration no longer refuses.
- Reads and writes take the instance: `map_name:get(instance)`, `map_name:set(instance, value)`, `map_name:add(instance, amount)`. As with `variables.npc_*`, the scope's owner is an argument, not hidden in the context.
- Values live in the instance registry, in memory, shared by every zone of the instance, as server temporary and NPC variables live in `variables.Temporary` (review decision 58). They are never saved. They start at their defaults when the instance is created and go when it ends; a call with an ended instance raises an error.
- Limits per mod and per instance: 256 values and 1 MiB, as NPC variables; core is exempt from the count (review decision 59).
- `getinstancevar` and `setinstancevar` convert to the same calls with `instances.get(id)`.

### 7. Lua API: `wa.instances`

The module needs `game.read` for reads. Its changes need a new permission, `instances.manage` (approval needed, not flagged): "create, enter and end instances of the mod's own records, and of core's". Core holds it, as the built-in mods hold every permission. A mod that ends or enters an instance of another mod's record needs `instances.manage` and `npcs.control`.

Functions:

| Call | Returns | rAthena | Permission |
|---|---|---|---|
| `instances.create(name, owner, options?)` | `wa.Instance?`, `string?` | `instance_create` | `instances.manage` |
| `instances.get(id)` | `wa.Instance?` | `getinstancevar`'s ID | `game.read` |
| `instances.of(owner)` | `wa.Instance?` | `instance_id(IM_CHAR/IM_PARTY/IM_GUILD)` | `game.read` |
| `instances.current()` | `wa.Instance?` | `instance_id()`, `instance_id(IM_NONE)` | `game.read` |
| `instances.list(options?)` | `wa.Instance[]` | `instance_list` | `game.read` |
| `instances.get_record(name)` | `wa.InstanceRecord?` | `instance_info` | `game.read` |
| `instances.check_members(group, options?)` | `boolean` | `instance_check_party`, `instance_check_guild` | `game.read` |

- `create`: `name` is a record of `data/instances.yml`, a full name or a plain name (the calling mod's, then core's). `owner` is a `wa.Player` (character mode), a `wa.Party`, a `wa.Guild`, or nil (mode `none`). On failure it returns nil and the reason: `"unknown"`, `"no_owner"`, `"exists"` or `"full"`. The instance it returns is `pending`.
- `of(owner)`: the pending or running instance a player owns (character mode), or a party or guild owns. `of(player)` is the player's character instance only; `of(player.party)` is the party's.
- `current()`: the instance of the context's map: an NPC copy's, or a mod or event context's in an instance zone. nil elsewhere.
- `list { base_map, mode }`: running and pending instances, by ID. `base_map` keeps those with that base map, `mode` those of one mode.
- `check_members(group, { count, min_level, max_level })`: true when the party or guild has at least `count` online members (1 by default), not autotrading, and every one of them has a base level within `min_level` (1) and `max_level` (the ruleset's maximum), as `instance_check_party` and `instance_check_guild`.

`wa.Instance` (handle; two handles of one instance are equal; after the instance ends its fields still read and its methods raise an error):

| Field | Type | |
|---|---|---|
| `id` | integer | |
| `name` | string | The record's full name, such as `core:Endless Tower`. `instance_live_info(ILI_NAME)` is the plain name for core's records. |
| `state` | `"pending"\|"running"\|"ended"` | |
| `mode` | `"character"\|"party"\|"guild"\|"none"` | |
| `owner_id` | integer | Character, party or guild ID; 0 for `none`. |
| `maps` | string[] | The instance map IDs, the enter map first. Empty while pending. |
| `created_at` | integer | Unix seconds. |
| `ends_at` | integer? | Unix seconds of the time limit, nil without one. |
| `idle_ends_at` | integer? | Unix seconds of the idle timeout, nil while players are inside or without one. |

| Method | Returns | rAthena | Permission |
|---|---|---|---|
| `get_map(base_map)` | `string?` | `instance_mapname` | `game.read` |
| `get_npc(name)` | `wa.Npc?` | `instance_npcname` | `game.read` |
| `get_npc_name(name)` | `string?` | `instance_npcname` | `game.read` |
| `enter(player, x?, y?)` | `boolean`, `string?` | `instance_enter` | `instances.manage` |
| `warp_players(base_map, x, y, options?)` | `integer` | `instance_warpall` | `instances.manage` |
| `announce(text, options?)` | | `instance_announce` | none |
| `count_players()` | `integer` | `getmapusers` over its maps | `game.read` |
| `is_member(player)` | `boolean` | the checks of `instance_enter` | `game.read` |
| `destroy()` | | `instance_destroy` | `instances.manage` |

- `get_map("1@tower")` returns `instance:42:1@tower`, or nil when the base map is not one of the instance's.
- `get_npc(name)`: name is a base record's name, a full name or a plain name, as `world.get_npc` takes. It returns the copy's handle, or nil.
- `enter` returns false and `"no_member"`, `"not_running"` or `"other"` (section 2).
- `warp_players(base_map, x, y, { alive_only })` moves every player on any map of the instance who belongs to it to the cell of the instance's copy of `base_map`, as `instance_warpall` with `IWA_NOTDEAD` for `alive_only`. Returns how many moved.
- `announce(text, options)` shows the line to every player on the instance's maps, with `maps.announce`'s options (color and the font fields).
- `destroy()` ends the instance with the reason `script`. A record with `destroyable: false` can still be ended by a script, as rAthena.

`wa.InstanceRecord`: `name`, `id`, `time_limit_seconds`, `idle_timeout_seconds`, `enter_map`, `enter_x`, `enter_y`, `maps` (the additional maps), `copy_npcs`, `copy_map_flags`, `destroyable`.

New fields on existing handles: `npc.instance` and `player.instance`, the `wa.Instance?` of the entity's map. `context:get_instance()` of a mod context in an instance zone gains `instance` too.

Events (`events.on`):

| Event | Kinds | Argument | rAthena |
|---|---|---|---|
| `instance_start` | `npc` | `wa.Instance` | `OnInstanceInit` |
| `instance_end` | `npc`, `mod` | `wa.InstanceEnd` | `OnInstanceDestroy` |

- `instance_start` fires once on each NPC copy, after every copy of every map of the instance started, in the order of section 2. It runs with no player.
- `instance_end` fires in each zone of the instance, before players move out. `wa.InstanceEnd` has `instance` and `reason`: `"time_limit"`, `"idle_timeout"`, `"owner_request"`, `"script"`, `"staff"`, `"owner_gone"` or `"create_failed"`. Mod and event contexts on other maps do not hear it.

Variables: `variables.instance_integer`, `instance_boolean`, `instance_string`, `instance_table` (section 6).

`wa.testing`: `world:advance` moves the registry's timers too, and `world:get_instances()` lists the test world's instances. A test world runs every map an instance makes (an exception to "a test world runs one map").

Example, Faceworm's Nest's two entrance NPCs written for wAthena:

```lua
local events = require "wa.events"
local instances = require "wa.instances"

local NEST = "Faceworm's Nest"

return function()
  events.on("click", function(player, conversation)
    local party = player.party
    if not party then
      return conversation:say("[Magic Scholar]", "Come back with a party.")
    end
    if party.leader ~= player then
      return conversation:say("[Magic Scholar]", "Only your party leader can open the passage.")
    end
    local nest, reason = instances.create(NEST, party)
    if not nest then
      return conversation:say("[Magic Scholar]", reason == "exists" and "Your party has a passage open." or "The passage would not open.")
    end
    conversation:say("[Magic Scholar]", "Please wait for a minute.")
  end)
end
```

```lua
-- An NPC on 1@face: hidden at the start of each instance.
local events = require "wa.events"
local variables = require "wa.variables"
local context = require "wa.context"

local map_name = variables.instance_string "map_name"

return function()
  local npc = context:get_instance()
  events.on("instance_start", function(instance)
    map_name:set(instance, instance:get_map("1@face"))
    npc:set_hidden(true)
  end)
end
```

### 8. Converter

| rAthena | Lua |
|---|---|
| `instance_create(name{,mode{,owner}})` | `R.instance_create(name, mode, owner)`: the instance ID, or -1 to -4 as rAthena, since scripts test `< 0` and `== -3`. Mode `IM_CLAN` is refused. |
| `instance_destroy {id}` | `R.instance(id):destroy()`; without an ID, `instances.current()`. |
| `instance_enter(name{,x,y{,char_id{,id}}})` | `R.instance_enter(player, name, x, y, id)`: `IE_OK` 0, `IE_NOMEMBER` 1, `IE_NOINSTANCE` 2, `IE_OTHER` 3. Without an ID, the player's party instance, as rAthena. |
| `instance_npcname(name{,id})` | Folded into the call that uses it when the converter sees it: `donpcevent instance_npcname("X")+"::OnY"` is `instance:get_npc("X"):fire_event("OnY")`, and `enablenpc`, `disablenpc`, `hideonnpc`, `hideoffnpc`, `cloakonnpc`, monster labels and `getnpcid` the same. Otherwise `R.instance_npc_name(name, id)`, a string that `world.get_npc` resolves. |
| `instance_mapname(map{,id})` | `R.instance_map(map, id)`: the map ID, or `""`. |
| `instance_id({mode})` | `R.instance_id(mode)`: the ID, or 0. |
| `instance_warpall map,x,y{,id{,flag}}` | `instance:warp_players(map, x, y, { alive_only = flag & IWA_NOTDEAD ~= 0 })` |
| `instance_announce id,text,flag{,...}` | `instance:announce(text, options)` with the color and font arguments as `mapannounce` converts them; id 0 is `instances.current()`. |
| `instance_check_party(party{,count,min,max})` | `instances.check_members(parties.get(party), { count, min_level, max_level })`, false for no party. |
| `instance_check_guild(...)` | The same with `guilds.get`. |
| `instance_check_clan` | Refused: clans do not exist. |
| `instance_info(name, type{,index})` | `instances.get_record(name)` and its field: `IIT_ID`, `IIT_TIME_LIMIT`, `IIT_IDLE_TIMEOUT`, `IIT_ENTER_MAP`, `IIT_ENTER_X`, `IIT_ENTER_Y`, `IIT_MAPCOUNT`, `IIT_MAP` (index from 0). Unknown name: -1, as rAthena. |
| `instance_live_info(type{,id})` | `ILI_NAME`: the record's plain name; `ILI_MODE`: the mode as `IM_` number; `ILI_OWNER`: `owner_id`. No instance: `""` or -1. |
| `instance_list(map{,mode})` | `instances.list { base_map, mode }` into `.@instance_list`, returning the count. |
| `getinstancevar(var, id)`, `setinstancevar(var, value, id)` | `var:get(instances.get(id))`, `var:set(instances.get(id), value)` |
| `'x`, `'x$`, `setarray 'x` | `variables.instance_*` with `R.instance()` (the context's instance); arrays as `instance_table`. |
| `OnInstanceInit` | `events.on("instance_start", ...)` |
| `OnInstanceDestroy` | `events.on("instance_end", ...)` |
| NPC lines on a base map | Placed records on that map, as now, which load as templates (section 1). |

- `R.instance()` raises an error in a script that runs outside an instance, as rAthena reports "instance not found", so a stray `'x` shows in the logs.
- The converter writes core's `data/instances.yml` from `extracts/<mode>/instances.json`.
- `IM_` constants (`IM_NONE` 0 to `IM_CLAN` 4), `IE_`, `IIT_`, `ILI_` and `IWA_` constants stay in the converter's table and `rathena.lua`, not in a `wa.*` module (proposal 0002, "Constants and IDs").
- The coverage reports count each of these rows as converted. The `instance` feature row (NPC on an instance map) and `variables (instance variable 'x)` drop to zero, except scripts that fail for other reasons.

### 9. Protocol

Proposal 0001's rules: new fields and messages only, so the protocol version stays 1. Sketches; field numbers are the next free ones at build time.

```proto
// event.proto, in Event.body
InstanceStatus instance_status = 115;
InstanceEnded instance_ended = 116;

// InstanceStatus is the state of an instance the player belongs to, as
// rAthena's clif_instance_create, clif_instance_changewait and
// clif_instance_status. The server sends it on each change, at login,
// and when the player joins the owning party or guild.
message InstanceStatus {
  uint64 id = 1;
  // name is the record's name players see.
  string name = 2;
  InstanceState state = 3;
  // queue_position is the place in the creation queue, from 1, while
  // the state is INSTANCE_STATE_PENDING.
  uint32 queue_position = 4;
  // ends_at_ms is the time limit in Unix milliseconds, 0 for none.
  uint64 ends_at_ms = 5;
  // idle_ends_at_ms is when the idle timeout ends the instance, 0
  // while players are inside or without a timeout.
  uint64 idle_ends_at_ms = 6;
  // can_destroy is set when this player may press the destroy button:
  // the record is destroyable and the player is the owner character,
  // the party leader or the guild master.
  bool can_destroy = 7;
}

enum InstanceState {
  INSTANCE_STATE_UNSPECIFIED = 0;
  INSTANCE_STATE_PENDING = 1;
  INSTANCE_STATE_RUNNING = 2;
}

// InstanceEnded says an instance the player belonged to ended, as
// clif_instance_changestatus. The client closes its window and shows
// the reason.
message InstanceEnded {
  uint64 id = 1;
  string name = 2;
  InstanceEndReason reason = 3;
}

enum InstanceEndReason {
  INSTANCE_END_REASON_UNSPECIFIED = 0;
  INSTANCE_END_REASON_TIME_LIMIT = 1;
  INSTANCE_END_REASON_IDLE_TIMEOUT = 2;
  INSTANCE_END_REASON_OWNER_REQUEST = 3;
  INSTANCE_END_REASON_SCRIPT = 4;
  INSTANCE_END_REASON_STAFF = 5;
  INSTANCE_END_REASON_OWNER_GONE = 6;
  INSTANCE_END_REASON_CREATE_FAILED = 7;
}

// command.proto, in Command.body
InstanceDestroy instance_destroy = <next>;

// InstanceDestroy is the window's destroy button, as
// instance_destroy_command. It names the instance; the server checks
// the player may end it and ignores the command otherwise.
message InstanceDestroy {
  uint64 id = 1;
}
```

- A player can belong to up to three instances (its own, its party's and its guild's). The client keeps one window per ID. rAthena's client shows one; showing the one of the current map first is the client's choice.
- `Welcome.map` and `MapChange.map` carry the zone's `Asset` (section 3). No other message changes.
- The client's memorial dungeon window shows the name, the queue position, the two countdowns and the destroy button, from these messages alone.

### 10. Admin and observability

- `GET /world` (admin.proto `WorldView`) gains `repeated InstanceView instances = 7`, every pending and running instance: `id`, `name`, `mode`, `owner_name` (character, party or guild name; empty for none), `state`, `maps`, `players`, `created_at`, `ends_at`. `ZoneView` gains `uint64 instance = 10`, set for an instance map's zone. The World screen shows the count of running and pending instances against `max_running`, and the list. It needs `world.read`.
- `POST /instances/{id}/destroy` ends an instance with the reason `staff`. It needs a new staff permission, `world.instances` ("end instances"), which `admin` has through `world.*`. It writes a GM audit row with the action `instances.destroy`, the instance's ID, record and owner, and the staff reason.
- Logs: one info line at create, running and end, with the ID, the record, the owner and the reason.
- Metrics: instance zones register their zone metrics as other zones do, labeled by their map ID, and are removed at retire, as rotation does (`reg.RemoveZone`). The registry adds two gauges, running and pending instances, and a counter of ends by reason.

### 11. Database

No new tables and no migration.

- Instances are not saved (section 2), so nothing about them needs a row.
- A character saved on an instance map stores the map ID in `characters.map`, a text column of at most 64 characters, which `instance:<id>:<base>` fits.
- Lockouts are quests (`setquest` with a time limit), which are saved already.
- Staff ends go to the existing GM audit table.

### 12. Procedural worlds

The instance system runs in both world modes: the registry, zones, variables and API do not depend on the mode.

- Core's converted records name classic base maps (`1@tower`), and their NPCs are classic converted NPCs, so they load only in classic worlds, as rAthena's fixed instances are "classic mode only" (strategy section 11's inventory and section 11.4).
- A mod may add records in a procedural world whose maps are asset maps it ships (after mod-assets-gaps) or generated dungeon floors. A record whose map does not exist in the world is left out with a warning.
- Strategy section 11.4's "generated instance dungeons with floor counts and a final boss", and the job guild's trial dungeon, are content on top of this system and are not part of v0.2.0. A generated base map would be a floor of the world layout that the layout does not link to any other map.

Recommendation: build the system mode-independent, load core's records in classic worlds only, and leave generated instance dungeons to a later node.

### 13. Tests

Unit and golden tests (instances-core):

- Record loading: defaults (3,600 and 300 seconds), a mod overriding `time_limit_seconds`, a record with an unknown map left out with a warning, `id` refused on another mod's record.
- Create results: unknown, no owner, exists (pending and running), full; one per owner.
- Timers on a fake clock: the time limit from creation; the idle timer starting at build and when the last player leaves, stopping on entry, across two maps of one instance.
- Destroy: players to their own save points, a player on a warp between two maps forwarded, `'x` values gone, zones out of the world and retired.
- NPC copies: names, `unique_name`, NPC variables shared with the base record, warps between maps of the instance pointed at the copies.
- `instance_start` order across maps; `instance_end` with each reason.
- Reconnect on and off; a reused ID after a restart refused for a non-member.
- Party and guild hooks: leave moves to the save point, break up ends the instance, the four block rules.
- Converter golden files for every row of section 8, and the coverage report rows.

e2e (`e2e/`, a classic world with the kRO assets and PostgreSQL), the acceptance test of instances-core:

1. Two players log in; one makes a party and invites the other.
2. The leader talks to a test mod's entrance NPC, which calls `instances.create` for a test record with one map, `time_limit_seconds: 120` and `idle_timeout_seconds: 20`. Both clients get `InstanceStatus` pending, then running.
3. Both enter through a second NPC. Each client gets `MapChange` with the base map's asset. An NPC copy's `instance_start` handler spawned 3 monsters with a label and set an `'x` string; the players see the monsters, and a third NPC reads the string back in a conversation.
4. A player who is not in the party is refused with `no_member`.
5. One member leaves the party and lands at its save point.
6. The other walks out through the copied warp; after 20 seconds of the dev clock both clients get `InstanceEnded` with `IDLE_TIMEOUT`, and the zone is gone from `GET /world`.
7. A second run: create, enter, staff ends it through `POST /instances/{id}/destroy`; the player lands at its save point and the audit log has the row.
8. A third run: log out inside with reconnect on, log in, back on the instance map; restart the server, log in, at the save point.

instances-content adds an e2e run of two converted dungeons (Faceworm's Nest and one with several maps, such as Sealed Catacomb), driven by headless bots, to the boss and back out.

## Breaks

- Nothing that runs today stops working. NPCs on `@` maps did not convert, `variables.instance_*` refused, and the protocol and API only gain messages, fields and calls.
- `--maps all` today runs base maps such as `1@tower` as plain zones, with no NPCs. After this change, maps that a loaded instance record names do not run as zones. A player saved on one of them (only staff could get there) enters at the save point.
- `Welcome.map` and `MapChange.map` were the zone's map name, which was also its asset name. They are now the zone's asset; for every zone but instance zones the value is the same.
- `wa.Npc`, `wa.Player` and the mod context's instance object gain a field. Mods that iterate a handle's fields do not exist (handles are userdata).
- Mods that want instances request `instances.manage`, which the owner approves.
- Clients older than this change ignore the new events, as proposal 0001 allows, and show no window.

## Alternatives

- **Save instances across restarts.** Would need saved Lua states, monsters, cells and floor items. rAthena does not, scripts do not expect it, and lockout quests already limit the loss. Not chosen.
- **One zone per instance, holding all its maps.** Fewer goroutines and one Lua state per instance, but a zone owns one map everywhere in the code (grid, block index, warps, shared map state). Every map lookup would need a second key. Not chosen; the Lua state cost is better handled by decision 0056.
- **Copy rAthena's map names (`0001#00000042`).** Opaque in logs and admin screens, and they hide the base map. Scripts never parse them. Not chosen.
- **Hidden instance in the context for `'x`.** `'x` would read the context's instance without an argument, as rAthena. Proposal 0002 keeps owners of a scope explicit (`variables.npc_*` takes the NPC), and an NPC outside an instance would fail only at run time either way. Not chosen; the converter supplies `R.instance()`.
- **Keep base maps running.** rAthena keeps them, and their NPCs' `OnInit` runs there once. Nothing should reach them, and running them costs a zone and a Lua state each. Not chosen.
- **Per-instance NPC variables for copies.** Would make each instance's `.x` its own, which is cleaner but breaks the few scripts that count across instances with `.x`, as rAthena's duplicates share. Not chosen; listed as a question.
- **Build instances on the calling zone's goroutine.** Simple, but a build makes zones and Lua states, which would stall the caller's tick well past 10 ms. Not chosen.
- **Clans.** Not a system in wAthena; adding one is out of scope for v0.2.0.

## Questions for review

1. **Instances are lost on restart** (section 2). Recommendation: accept, as rAthena.
2. **IDs restart at 1 at each start**, and reconnect relies on the owner and map checks, as rAthena. The other choice is a counter that goes on across starts, from the database, which needs a migration. Recommendation: restart at 1.
3. **NPC variables of copies are shared** with the base record and every other instance, as rAthena's duplicates. Recommendation: share, for parity; per-instance state is `'x`.
4. **Base maps do not run as zones**, and warps to them by name are refused. Recommendation: yes.
5. **The module name `wa.instances`** next to `context:get_instance()`, which means a script instance. Recommendation: keep `wa.instances`, rAthena's word for the system, and have the API reference call the other "script instance".
6. **`instances.max_running` defaults to 100** until instances-core measures the heap per instance map, then to what fits in 1 GB. Recommendation: yes, with the measurement in instances-core's decision record.
7. **One build at a time**, with the queue position sent to the client, as rAthena's queue. Recommendation: yes.
8. **Mod permission `instances.manage`**, approval needed, not flagged. Recommendation: yes; ending another mod's instance needs `npcs.control` too.
9. **Staff permission `world.instances`**, held by `admin`. Recommendation: give it to `game_master` too, since ending a stuck instance is support work.
10. **The four block rules and `instance_allow_reconnect` as ruleset rules**, with rAthena's defaults per mode. Recommendation: yes.
11. **Mode `none`** (anyone may enter), unused by rAthena's scripts. Recommendation: support it; it costs one branch.
12. **Clans**: `IM_CLAN` and `instance_check_clan` refused. Recommendation: yes.
13. **Map flags of base maps** (`noteleport`, `nowarpto`, `nobranch`, `partylock`, `restricted`, `monster_noteleport`): copied but not enforced until their effects are built. Recommendation: instances-content builds `noteleport`, `nobranch` and `partylock`, which the dungeons depend on; the rest follow the map flag work.
14. **No countdown warnings before an end**, unlike rotation's 10, 5 and 1 minutes, since the window shows the time left. Recommendation: none, as rAthena.
