---
name: instances
version: 1
status: frozen
defined_by: proposal-instances
---

# Contract: instances

## Purpose

The interface between the instances design and the nodes that build on it. Frozen 2026-10-27, when the maintainer accepted design proposal 0003.

Source: [docs/proposals/0003-instances.md](../../../proposals/0003-instances.md). Section numbers below are the proposal's.

## Interface

Data (section 1):

- `data/instances.yml`, kind `mod.InstanceKind` (`CoreNames`, `LocalNames`). Fields: `name`, `id` (fixed), `time_limit_seconds` (3600; 0 none), `idle_timeout_seconds` (300; 0 none), `enter: {map, x, y}`, `maps` (at most 16 in all), `copy_npcs` (true), `copy_map_flags` (true), `destroyable` (true). Core's file is converted from `extracts/<mode>/instances.json`.
- Server config block `instances`: `max_running` (100 until measured), `max_queued` (50).
- Ruleset rules: `instance_allow_reconnect` (renewal 1, pre-renewal 0), `instance_block_leave`, `instance_block_leader_change`, `instance_block_invite`, `instance_block_expulsion` (all 1).

Map IDs (section 3): `instance:<id>:<base map>`, such as `instance:42:1@tower`. Zone field `Asset` holds the base map's asset name. NPC copy record names: `<base full name>:<id>`, such as `core:Chaos#01:42`.

Lua (section 7), module `wa.instances`:

- `instances.create(name, owner, options?) -> wa.Instance?, string?` (reasons `"unknown"`, `"no_owner"`, `"exists"`, `"full"`), permission `instances.manage`.
- `instances.get(id)`, `instances.of(owner)`, `instances.current()` -> `wa.Instance?`; `instances.list { base_map, mode } -> wa.Instance[]`; `instances.get_record(name) -> wa.InstanceRecord?`; `instances.check_members(group, { count, min_level, max_level }) -> boolean`. All `game.read`.
- `wa.Instance` fields: `id`, `name`, `state` (`"pending"|"running"|"ended"`), `mode` (`"character"|"party"|"guild"|"none"`), `owner_id`, `maps`, `created_at`, `ends_at`, `idle_ends_at`.
- `wa.Instance` methods: `get_map(base_map) -> string?`, `get_npc(name) -> wa.Npc?`, `get_npc_name(name) -> string?`, `count_players() -> integer`, `is_member(player) -> boolean` (`game.read`); `enter(player, x?, y?) -> boolean, string?` (reasons `"no_member"`, `"not_running"`, `"other"`), `warp_players(base_map, x, y, { alive_only }) -> integer`, `destroy()` (`instances.manage`); `announce(text, options?)` (none).
- `wa.InstanceRecord`: `name`, `id`, `time_limit_seconds`, `idle_timeout_seconds`, `enter_map`, `enter_x`, `enter_y`, `maps`, `copy_npcs`, `copy_map_flags`, `destroyable`.
- `wa.InstanceEnd`: `instance`, `reason` (`"time_limit"`, `"idle_timeout"`, `"owner_request"`, `"script"`, `"staff"`, `"owner_gone"`, `"create_failed"`).
- New fields: `npc.instance`, `player.instance`, and `instance` on a mod context's `context:get_instance()` in an instance zone.
- Events: `instance_start` (kind `npc`, argument `wa.Instance`), `instance_end` (kinds `npc`, `mod`, argument `wa.InstanceEnd`).
- Variables: `variables.instance_integer`, `instance_boolean`, `instance_string`, `instance_table`; `get(instance)`, `set(instance, value)`, `add(instance, amount)`.
- Mod permission `instances.manage` (approval, not flagged).
- `wa.testing`: `world:get_instances()`; `world:advance` moves instance timers.

Converter helpers in `tools/convert/rathena.lua` (section 8): `R.instance_create`, `R.instance`, `R.instance_enter`, `R.instance_npc_name`, `R.instance_map`, `R.instance_id`; events `OnInstanceInit` -> `instance_start`, `OnInstanceDestroy` -> `instance_end`.

Protocol (section 9): events `InstanceStatus` (`id`, `name`, `state`, `queue_position`, `ends_at_ms`, `idle_ends_at_ms`, `can_destroy`), `InstanceEnded` (`id`, `name`, `reason`), enums `InstanceState`, `InstanceEndReason`; command `InstanceDestroy` (`id`). `Welcome.map` and `MapChange.map` carry the zone's `Asset`.

Admin (section 10): `WorldView.instances` (`InstanceView`: `id`, `name`, `mode`, `owner_name`, `state`, `maps`, `players`, `created_at`, `ends_at`), `ZoneView.instance`; `POST /instances/{id}/destroy`; staff permission `world.instances` (held by `admin` and `game_master`); audit action `instances.destroy`.

Database (section 11): none. No migration.

## Behavior

- One pending or running instance per owner. Create returns a pending instance; one registry goroutine builds the queue in order; the time limit starts at create; the idle timer starts when the build ends and whenever the instance's maps have no players, and stops when one arrives.
- Each instance map is its own zone from the base map's shared grid, with no spawn lines, copied NPCs and warps (warps inside the instance retargeted). Base maps of loaded records do not run as zones; moves to them and to instance maps from outside the instance are refused, except staff commands.
- Copied NPCs start at build. `instance_start` runs on every copy after all copies of all maps started, map by map, enter map first.
- NPC variables of copies are keyed by the base record name (shared, as rAthena). `'x` values live in the registry, unsaved, and go at the end.
- End: state `ended`, `instance_end` in every zone (5 s each), `World.Remove`, players to their own save points (late arrivals forwarded), `'x` dropped, `InstanceEnded` to members, `World.Retire` after 30 s.
- Party or guild leave while on its instance: to the save point. Break up: end with `owner_gone`. The block rules refuse leave, leader change, invite and expulsion while an instance is pending or running.
- Reconnect on: saved on the instance map ID; login enters it when it runs, has the map and the player belongs, else the save point. Off: saved at the save point. Instance maps are never save or memo points.
- Instances are lost on restart. IDs start at 1 at each start.
- Clans do not exist: `IM_CLAN` and `instance_check_clan` refuse.
- Base map flags are copied; only `nomemo` has an effect until instances-content builds `noteleport`, `nobranch` and `partylock`.
- `max_running` is 100 until instances-core measures the heap per instance map, then what fits in 1 GB.

## Stub

None. Nodes that depend on this contract build on instances-core's real code, not a stub.

## Contract tests

Named in proposal 0003 section 13, owned by instances-core:

- Unit and golden: record loading, create results, timers on a fake clock, destroy, NPC copies, event order, reconnect, party and guild hooks, converter golden files for each row of section 8.
- e2e: party create and enter with an `instance_start` NPC that spawns labeled monsters and sets an `'x` value, a non-member refused, a member who leaves the party moved out, idle timeout end with `InstanceEnded`, staff end with an audit row, reconnect and restart.

## Changes

- v1: initial draft, filled from proposal 0003.
- v1: frozen 2026-10-27 with proposal 0003.
