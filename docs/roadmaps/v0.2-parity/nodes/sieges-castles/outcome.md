# Outcome: sieges-castles

## Built

- Castle records: `data/castles.yml`, `mod.CastleKind` (server/mod/castles.go), loaded by server/gamedata/castles.go into `gamedata.Mods.Castles`. Core's file is converted from `extracts/<mode>/castles.json` (tools/convert/castles.go): 44 renewal, 34 pre-renewal. Mods' castles get IDs from 10,000 (`mod.CastleIDs`).
- Extracts `castles.json` and `map_flags.json` for both modes, from rAthena fork commit `f99a928d1` (reference/rathena/README.md, "Castles").
- `castle.Registry` (server/castle): state per castle, `SetOwner`, `Set`, `SetGuardian`, `GuildBroken`, `BadgeChanged`, per-zone `Feed`s. Migration 00043 (`castles`), queries `LoadCastles` and `UpsertCastle`, `store.LoadCastles` and `SocialStore.SaveCastle`.
- Guild break: `social.Registry.Castles` hook; each owned castle's NPC gets `OnGuildBreak` through `zone.NPCDirectory.FireEvent`, then the owner clears.
- Lua: `wa.castles` (`get`, `list`, `of`), `wa.Castle` (fields, `set_owner`, `set`, `set_guardian`, `get_guardian`), `castle_change`, permission `castles.manage`, `npc:set_guild` and `npc.guild`. NPC badges go out in `EntityAppear.guild` and `EntityGuild`.
- Converter: `getcastledata`, `setcastledata`, `getcastlename`, `flagemblem`; `R.castle_data`, `R.set_castle_data`, `R.castle_name`, `R.flag_emblem`; `CD_ENABLED_GUARDIAN00` to `07`.
- Decision record: docs/decisions/0067-castles.md.

## Deviations

- Paths outside the brief's owns and shared: tools/convert/castles.go, tools/convert/reference.go (the core conversion reads the new extract), reference/rathena/embed.go, server/castle tests, server/cmd/waserver/castles.go (owns lists server/cmd/waserver/** as shared, so in scope), server/modscript/castles.go and present.go (server/modscript/** shared). `data/castles.yml` is converted in memory, as every core data file; there is no file in the repository.
- Core castle names are plain (`Kriemhild`), as other core records, not `core:Kriemhild` as the proposal's examples write.
- `teleport` is an economy field as a whole, since overlays check top-level fields: a mod changing another mod's teleport cell needs `data.core_override` too.
- `job_noenter.json` and `jobs.yml`'s `no_enter` are not built: only `maps.can_enter` (sieges-war) reads them. The contract lists them under data.
- `R.map_runs` now treats `""` as a map that does not run (tools/convert/rathena.lua). A converted castle-using script needs it; it is a bug on main that no converted script reached before.

## Stubs left

- None. `monster.castle`, `monster.guild`, guardian badges and `wa.testing`'s `guild` option belong to sieges-war with guardians.

## Gotchas

- The castle registry's lock is taken inside the social registry's lock (guild break, emblem). Never call the social registry from inside the castle registry or from a `Feed` wake.
- A zone's `castle_change` handlers run at the top of its script loop, after the zone drains its feed. Zones in tests built without `Run` must subscribe themselves (`z.castleFeed = reg.Subscribe(nil)`).
- A runtime without a registry has no castles; `castles.get` returns nil. wactl mod test worlds have none unless a node adds them.
- The constants extract lacks the loop-made `CD_ENABLED_GUARDIAN` constants; other loop-made constants may be missing too.
- `map_flags.json` lists `restricted` zones as rAthena's zone numbers, not bits.

## Proposed learnings

- Read constants made in a loop from the converter's table | script_constants.hpp builds some names in loops that the extractor's parser does not see | tools/convert | yes
- Save world registries through the SocialStore's queue when their rows reference guilds | keeps writes ordered with guild deletes | server/store | no

## Verification

- `go test ./server/castle ./server/gamedata ./server/social ./server/modscript/... ./server/zone ./server/cmd/waserver ./tools/convert`: ok.
- `go test ./server/store -run 'TestCastlesSaved|TestGuildsSaved|TestModIDs|TestMigrationsFromEmpty'` with postgres:18: ok.
- `scripts/check.sh quick queries`: see the PR.

## Size

- Estimated M, actual L. The extracts, the converter, the Lua API and NPC badges each touched a different layer.
