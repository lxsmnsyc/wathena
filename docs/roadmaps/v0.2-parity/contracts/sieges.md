---
name: sieges
version: 1
status: frozen
defined_by: proposal-sieges
---

# Contract: sieges

## Purpose

The interface between the sieges design and the nodes that build on it: gvg-rules, sieges-castles, sieges-war, sieges-ui and sieges-content. Frozen 2026-10-27, when the maintainer accepted design proposal 0004 with every recommendation ("Decisions in review" 1 to 19).

Source: [docs/proposals/0004-sieges.md](../../../proposals/0004-sieges.md). Section numbers below are the proposal's.

## Interface

Data (section 1):

- `data/castles.yml`, kind `mod.CastleKind` (`CoreNames`, `LocalNames`, fixed `id` and `map`, economy fields `teleport.cost` and `teleport.siege_cost`). Fields: `name`, `display_name`, `id` (rAthena's; mods from 10,000), `map`, `edition` (`fe`, `se`, `te`), `npc` (hears `OnGuildBreak`), `client_id`, `teleport: {x, y, cost, siege_cost}`. Core's file is converted from `extracts/<mode>/castles.json` (44 renewal, 34 pre-renewal).
- Extract files: `castles.json`, `map_flags.json` (gvg family and `restricted`), `job_noenter.json`. Core's `data/jobs.yml` gains `no_enter` (`normal`, `pvp`, `gvg`, `battleground`, `gvg_te`, `zone_1` to `zone_8`).
- Ruleset rules (section 5): `gvg_short_attack_damage_rate` 80, `gvg_long_attack_damage_rate` 80, `gvg_weapon_attack_damage_rate` 60, `gvg_magic_attack_damage_rate` 60, `gvg_misc_attack_damage_rate` 60, `gvg_flee_penalty` 20, `guild_max_castles` 0, `guild_leaderchange_woe` 0, `gvg_traps_target_all` 1; `emergency_call` gains its siege bit.

Database (section 2): migration 00043, table `castles` (`castle` text PK, `owner_id` bigint FK guilds ON DELETE SET NULL, `economy`, `defense`, `invested_economy`, `invested_defense`, `next_time`, `pay_time`, `create_time` integers, `kafra` boolean, `guardians` smallint bits 0 to 7, `updated_at`). Game role: SELECT, INSERT, UPDATE.

Map flags (section 5, gvg-rules): `maps.set_flag` and `get_flag` accept `gvg`, `gvg_noparty`, `gvg_castle`, `gvg_dungeon`, `gvg_te`, `gvg_te_castle`, `restricted`; setting them needs `maps.control`. Zone predicates `gvg` (map_flag_gvg), `gvgGround` (map_flag_gvg2), `vs`, `mapType` replace `Zone.GvG`.

Lua (section 8), module `wa.castles`:

- `castles.get(name_or_map) -> wa.Castle?`, `castles.list { edition, owner } -> wa.Castle[]`, `castles.of(guild) -> wa.Castle[]`, `castles.siege_running(edition?) -> boolean` (`game.read`); `castles.start_siege(edition) -> boolean`, `castles.end_siege(edition) -> boolean` (`castles.manage`).
- `wa.Castle` fields: `id`, `name`, `display_name`, `map`, `edition`, `npc`, `owner`, `owner_id`, `economy`, `defense`, `invested_economy`, `invested_defense`, `next_time`, `pay_time`, `create_time`, `kafra`, `teleport`.
- `wa.Castle` methods: `set_owner(guild?) -> boolean`, `set(values)`, `set_guardian(index, installed)`, `spawn_guardian { monster, x, y, display_name, label, index } -> wa.Monster?` (`castles.manage`; spawn also `npc.create`); `get_guardian(index) -> wa.Guardian` (`{ index, installed, monster }`), `is_siege() -> boolean` (`game.read`).
- New fields: `monster.castle`, `monster.guild`, `npc.guild`. New methods: `npc:set_guild(guild?)`, `npc:wake()`.
- Other modules: `maps.can_enter(map, job?) -> boolean`, `maps.respawn_guild(map, guild_id, { members, others, monsters }) -> integer`, `maps.count_players`'s `guild` option, `guild:warp(map, x, y, { target }) -> integer`, `guilds.relation(a, b) -> integer`.
- Events: `siege_start`, `siege_end` (argument `wa.Siege` `{ edition }`, option `edition`), `castles_ready` (no argument), `castle_change` (`{ castle, previous_owner_id }`); kinds `mod`, `npc`, `event`.
- NPC event labels the world sends: `OnAgitInit`, `OnAgitInit2`, `OnAgitInit3` (at world start), `OnAgitStart*`, `OnAgitEnd*` (each siege change), `OnGuildBreak` (to the record's `npc`).
- Mod permission `castles.manage` (approval, not flagged).
- `wa.testing`: `world:start_siege`, `world:end_siege`, `new_player`'s `guild` option.

Converter helpers in `tools/convert/rathena.lua` (section 9): `R.castle_data`, `R.set_castle_data`, `R.guardian`, `R.guardian_info`; `CD_` constants in the converter's table.

Protocol (section 10): events `MapRules` (`gvg`, `gvg_ground`, `castle`, `siege`, `no_party`), `GuildCastles` (`CastleEntry`: `id`, `name`, `map`, `client_id`, `teleport_cost`), `CastleInfo` (`id`, `economy`, `defense`), `CastleTeleportResult` (`id`, `CastleTeleportOutcome`); commands `CastleInfoRequest`, `CastleTeleport`. `EntityAppear.guild` and `EntityGuild` carry NPC and monster badges.

Admin (section 11): `WorldView.castles` (`CastleView`) and `WorldView.sieges` (`SiegeView`); `POST /sieges/{edition}/start`, `/end` (staff `world.sieges`, `admin` and `game_master`); `POST /castles/{id}/owner` (staff `world.castles`, `admin` only); audit actions `sieges.start`, `sieges.end`, `castles.owner`; GM commands `@agitstart`, `@agitend` and the 2 and 3 forms.

## Behavior

- One castle registry for the world; each write updates memory and queues an upsert. Owner changes move or remove guardians and fire `castle_change`; nothing else (scripts warp, kill and change flags).
- Guild break: `OnGuildBreak` on the record's `npc`, then the owner is cleared.
- Siege state is three booleans in memory, never saved. A start or end updates GvG state and `MapRules`, sends the labels as NPC events (starting zones that name them), then the typed events.
- At world start castles load before zones; then the `OnAgitInit` labels go out, which start castle zones and flag towns.
- The schedule is the converted controllers; staff can start and end sieges.
- Battle rules of section 5 hold on GvG ground or GvG maps as listed: player targeting on `vs` maps, damage rates, Emperium rules, flee penalty, nocast, noequip, no revival, no knockback, movement skills, cast cancel, respawn after 1 s, guild action blocks, Emergency Call bits.
- Guardians belong to the castle owner, get castle stats with Guard Up, and clear their installed bit at death. Guardian HP is not saved. The Emperium on a castle map gets castle data with slot 8.
- No castle storage. Core castles in classic worlds only.
- Respawn is 1 s on GvG ground, 3 s elsewhere. `skill_nocast_db` and `item_noequip` apply on every map.
- If 100 against 100 in one castle is over the tick budget, decision 0055 comes before the documented siege capacity.

## Stub

None. Nodes build on sieges-castles' real registry.

## Contract tests

Named in proposal 0004 section 14:

- sieges-castles: record loading, registry writes and saves (PostgreSQL), restart, guild break, `castle_change`, flag emblems, `CastleTeleport`, converter goldens for the castle rows.
- gvg-rules: targeting, predicates, damage rates, nocast and noequip on every map, `restricted` zones, movement and revival rules, respawn, guild blocks, Emergency Call, Emperium hit rules, `record_gvg` goldens against rAthena in both modes.
- sieges-war: siege start and end, `OnAgitInit` at boot, guardian ownership and stats, `maps.respawn_guild`, `guild:warp`, `maps.can_enter`, `guilds.relation`, `npc:wake()`, admin endpoints and audit rows, converter goldens for siege rows.
- sieges-ui: castle messages and commands, the client castle window, teleport, flag emblems and attacks on enemy players.
- e2e (sieges-war): two guilds of bots; staff owner set with audit row; siege start with `MapRules`; Emperium break changes owner, flags and guardians; guardian install; siege end; restart keeps owner and guardian.
- Performance (sieges-war): the `siege` wabots scenario at 50 to 300 bots in one castle and 20 castles of 50.

## Changes

- v1: initial draft, filled from proposal 0004.
- v1: frozen 2026-10-27 with proposal 0004.
