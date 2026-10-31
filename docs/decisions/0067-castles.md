# 0067: Castles: Records, Registry, Ownership and Badges

- Status: built. Details to review.
- Date: 2026-10-31.
- Code: [server/castle](../../server/castle) (`castle.go`), [server/mod](../../server/mod) (`castles.go`, `ids.go`, `permission.go`), [server/gamedata](../../server/gamedata) (`castles.go`, `mods.go`), [server/store](../../server/store) (`castles.go`, `queries/castles.sql`, `migrations/00043_castles.sql`), [server/social](../../server/social) (`social.go`, `guild.go`), [server/zone](../../server/zone) (`castles.go`, `npcworld.go`, `npcs.go`, `scripts.go`, `zone.go`, `guild.go`), [server/modscript](../../server/modscript) (`castles.go`, `present.go`, `runtime.go`, `prelude.lua`, `luaapi/spec.go`), [server/cmd/waserver](../../server/cmd/waserver) (`castles.go`, `main.go`), [tools/convert](../../tools/convert) (`castles.go`, `reference.go`, `npcguild.go`, `npcexpr.go`, `rathena.lua`), [reference/rathena](../../reference/rathena) (`extracts/<mode>/castles.json`, `extracts/<mode>/map_flags.json`, `embed.go`, `README.md`).
- Docs: [lua-api.md](../generated/lua-api.md), [reference/rathena/README.md](../../reference/rathena/README.md).
- Builds the sieges-castles part of [proposal 0004](../proposals/0004-sieges.md) (section 15): section 1, section 2, section 3, section 7's NPC badges, the castle part of section 8 and the castle rows of section 9.
- Lua API: adds to API 1.0. Protocol: none (existing fields carry NPC badges). Settings: none. Data: `data/castles.yml`. Store: migration 00043. Permission: `castles.manage`, approval, not flagged.

## Context

- rAthena's castles are `castle_db.yml` records with a saved state (`guild_castle`) that scripts read and write with `getcastledata` and `setcastledata`. A guild break runs each owned castle's `OnGuildBreak` and clears the owner. Flags show the owner's emblem through `flagemblem`.
- wAthena had no castle records, no castle state, and refused every castle command.

## Decision

### Records

- `data/castles.yml`, kind `mod.CastleKind`: `id` and `map` are fixed; `teleport` is an economy field. Core's records are converted from the new `castles.json` extract: 44 in renewal, 34 in pre-renewal. A core record's name is rAthena's castle name, such as `Kriemhild`, as other core records have plain names. Mods' records get IDs from 10,000 (`mod.CastleIDs`, which `mod_ids` takes from migration 00043).
- Loading leaves out, with a warning, a `te` castle outside the renewal ruleset and a second castle on one map (server/gamedata), and a castle whose map does not run or whose teleport cell is not walkable (waserver). Procedural worlds have none of core's castle maps, so they load no core castle.
- The rAthena fork's `extract.py` writes `castles.json` and `map_flags.json` (fork commit `f99a928d1`). `map_flags.json` is for gvg-rules and instances-core; nothing reads it yet.

### Registry and saves

- `castle.Registry` is the world's one registry. Each write sets memory and queues an upsert through the `SocialStore`'s queue, so castle rows save in order with guild writes. `owner_id` saves as NULL when the guild row is gone.
- Owner changes log at info level with their source (`script`, `staff`, `guild_break`) and go to each zone's `castle.Feed`. A zone drains its feed at the top of its script loop and runs `castle_change`. The feed wakes a sleeping zone.
- A guild break calls `Registry.GuildBroken` from the social registry, with its lock held. For each owned castle it sends `OnGuildBreak` to the record's NPC through `NPCDirectory.FireEvent`, then clears the owner. The lock order is social, then castle, then the NPC directory; none calls back.

### Lua

- `wa.castles`: `get`, `list`, `of`. `wa.Castle` fields and `set_owner`, `set`, `set_guardian`, `get_guardian`. `castle_change` for mod, NPC and event contexts. Changes need `castles.manage`, and `npcs.control` too on another mod's castle, core's included.
- `npc:set_guild(guild)` and `npc.guild`: the NPC's `EntityAppear.guild` and `EntityGuild` carry the badge. A guild's new emblem or break up updates every NPC showing it, through the same feed.
- A runtime without a registry has no castles: `castles.get` returns nil. Tests and procedural worlds run converted castle code without errors.

### Converter

- `getcastledata`, `setcastledata`, `getcastlename` and `flagemblem` convert to `R.castle_data`, `R.set_castle_data`, `R.castle_name` and `R.flag_emblem`. `flagemblem` is refused in a function, as it changes the running NPC.
- `CD_ENABLED_GUARDIAN00` to `07` (10 to 17) come from the converter: rAthena makes them in a loop, so the constants extract does not have them.
- `R`'s `map_runs` treats `""` as a map that does not run. Okolnir's gift NPC, which now converts, builds an empty name.

## Breaks

- `wa.Npc` gains `guild` and `set_guild`. `PresentHost` gains `SetNPCGuild`.
- `events.on`'s list of mod events gains `castle_change`.
- Scripts that use the four castle commands now convert. The coverage reports change.

## Alternatives

- **A store of its own for castles.** Saves would not be ordered with guild deletes. The social store's queue orders them.
- **A non-blocking channel per zone for castle_change**, as `restart_planned`. A busy zone would drop events. The feed keeps them.
- **Plain names with a `core:` prefix**, as the proposal's examples. Other core records are plain, and `castles.get` finds both forms.

## Details to review

- `teleport` is an economy field as a whole: changing another mod's teleport cell, not only its costs, needs `data.core_override`. The overlay checks top-level fields only.
- `job_noenter.json` and `data/jobs.yml`'s `no_enter` are not built. Only `maps.can_enter` reads them, which sieges-war builds.
- `R.set_castle_data` saves a negative value as 0, since the table refuses negatives; rAthena keeps it. No rAthena script writes one. An owner whose guild does not exist saves as none.
- `npc.guild` of an NPC of another map is nil.
- `wa.testing`'s `new_player` `guild` option, `monster.castle`, `monster.guild` and guardian badges are left to sieges-war, with guardians.

## How to check it worked

- `go test ./server/castle ./server/gamedata -run 'Castle|Find|Writes|GuildBroken'`: records of both modes, mods' records and refusals, registry writes, the feed and guild breaks.
- `go test ./server/store -run TestCastlesSaved` with `WATHENA_TEST_DATABASE_URL`: every field survives a restart, a guild break clears the owner in memory and in the database, and the game role cannot delete rows.
- `go test ./server/modscript -run Castle` and `go test ./server/zone -run TestCastleFlags`.
- `go test ./tools/convert -run 'TestCastleCommandsConvert|TestRunConvertedNPCs|TestNPCReportsCurrent'`.
