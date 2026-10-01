# 0006: The Mod Loader, Mod IDs in PostgreSQL, and Module Names

- Status: accepted.
- Date: 2026-10-01.
- Code: [server/mod](../../server/mod), [server/modscript](../../server/modscript), [migration 00005](../../server/store/migrations/00005_mod_ids.sql), [mods/README.md](../../mods/README.md).
- Builds on [proposal 0002](../proposals/0002-lua-api.md) and [decision 0002](0002-lua-runtime-c-lua.md).

## Decision

- The IDs of records that mods add are kept in a PostgreSQL table, `mod_ids`, and not in an `ids.lock` file. Without a database they live in memory.
- A mod's script files are required as `<mod ID>.<path>`: `example.halloween.lib.rewards` is `scripts/lib/rewards.lua` of `example.halloween`.
- Per-mod environments, read-only API modules and `require` are built in Lua, in a prelude that runs once per zone state with the real globals. The C binding did not change.
- Any problem with a mod stops the server's start. That includes a permission that needs approval and is not approved.
- Mod events wait in a queue until the zone reaches a point where scripts may run.
- Mod timers follow the zone clock, and wait while the zone sleeps.

## IDs: a table, not ids.lock

Strategy section 6.5 said that assigned IDs are saved in `ids.lock` on each server. A table is better here:

| Need | `ids.lock` | `mod_ids` table |
|---|---|---|
| IDs match the inventories that hold them | Only if the file and the database are backed up and restored together. | Yes. `wactl world backup` and `restore` keep both in one dump. |
| Never reuse an ID | The file must not be edited or lost. | Rows are never changed or deleted. The game role has only `SELECT` and `INSERT`. |
| Two servers on one database | Each would keep its own file and give different IDs. | The unique `(kind, id)` key stops a clash, and the loser tries again. |
| Works in a read-only container | Needs a writable file. | Yes. |

- Each row has the kind, the full name, the ID, the mod that added it, and when.
- An ID is one more than the highest of its kind, and at least the start of the mod range. Rows are never deleted, so a new ID is always above every old one.
- A server without a database keeps characters in memory, so its IDs need not outlive it either. `mod.MemoryIDs` keeps them for the run.

## Module names

Proposal 0002 shows `require "example.halloween.lib.rewards"` for a mod's helper. The rule behind it:

- A name is a mod ID, a dot, and the file's path under `scripts/`, with dots for folders and without `.lua`.
- A mod may require its own files and the files of the mods in its `depends`. Requiring another mod's file is an error that names the missing dependency. This keeps every link between mods visible in manifests.
- Mod IDs contain dots, so a name can match more than one mod. The longest matching ID wins.
- `wa.*` names are API modules. `wa` and `wa.*` are reserved as mod IDs, as are `core` and `core.*`.
- A dependency's file runs in the dependency's own environment, since it is the dependency's code.

Paths with slashes, such as `require "example.halloween/lib/rewards"`, were the other choice. Dots match Lua's own convention for module names and the proposal's examples.

## Environments in a Lua prelude

The runtime's `require` (decision 0002) loads modules into the shared globals. Per-mod environments could be added in C, or built in Lua on top of the sandbox. They are built in Lua:

- `load` with an environment table already gives each chunk its own globals. Nothing new is needed in C, so `lua.wasm` did not need a rebuild.
- The prelude keeps the real globals, the host functions and the API modules' tables in its own locals. Mod code only gets an environment table with copies of the safe base functions and of the standard library tables.
- API modules and handles are empty tables with a protected metatable. Writing raises an error. `rawset` in a mod's environment refuses them. `pairs` walks them without handing out the table behind.
- The context is tracked in Go, which sets it before each call it makes. Host functions read it. A limit that stops a call cannot leave it set.

Tests in [server/modscript](../../server/modscript/runtime_test.go) check that a mod cannot read another's globals, change a shared library, or change an API module with assignment, `rawset` or `setmetatable`.

## A broken mod stops the start

Strategy section 6.6 lets the loader refuse a mod whose permissions are not approved, or load it with the permission off. Refusing was chosen, and the server does not start:

- A mod that runs without a permission it needs fails later, in front of players, for example when it should give a reward.
- The server owner sees the problem at once, with the permissions to review and the config key to approve them.
- The same goes for missing dependencies, version ranges, cycles, unsupported rulesets, and data overlay errors. A server that starts with part of its mods would change the game in ways the owner did not choose.

Script errors are different. A mod whose scripts fail to load in a zone is logged and left off in that zone, and a mod whose handlers fail too often is turned off there (strategy section 6.6). Scripts can fail at run time in ways a start cannot check.

## When scripts run in a zone

Events such as `monster_killed` happen while the zone walks its entity list. A handler that adds or removes entities there could break the walk. So:

- Events wait in a queue. The zone runs their handlers after the tick's actions, before it sends updates, and after each message it handles, such as a join.
- `logout` runs at once, before the player leaves, so its handlers can still read the player.
- Messages that handlers send to a player go out with the same tick's frame.

## Timers and sleeping zones

A zone with no players sleeps, and its clock stops (server/README.md, "Maps and warps"). Mod timers follow the zone clock: when the zone wakes, every timer moves later by the time it slept. A timer for a player in a zone always has players, so only timers in empty zones wait. World events that must run in empty zones belong to the event scheduler (phase 3 step 6).

## Rulesets

- `core.ruleset.renewal` and `core.ruleset.pre_renewal` are built-in mods next to `core`.
- The world's ruleset is the `mods.ruleset` config key, or the core ruleset of `--mode`. The world row's ruleset is the base, and the server refuses a ruleset with another base.
- Storing the ruleset mod itself in the world row, so it cannot change after `wactl world new`, waits for `wactl world new --ruleset` in phase 3 step 5.
- Flags are checked and decide which mods load. The game code reads the rAthena mode until the flags drive formulas in phase 4.

## Left for later

- Overlays for monsters, skills, jobs, spawns, quests, achievements, and events. The engine is generic: each needs a `Kind` and a function that applies records.
- `wa.text`, `wa.variables`, `wa.storage`, `wa.hooks`, `wa.exports`, and the rest of proposal 0002's modules.
- NPC clicks and NPC contexts (phase 3 step 4).
- Mod migrations, storage limits, and `wactl mod` commands (phase 3 step 5).
