# Outcome: world-gen-fixes

## Built

None of the seven items existed on the base (99a808a). All seven are built on branch `fix/world-gen-fixes`. Decision record: docs/decisions/0077-world-generation-fixes.md (decision number taken: 0077; no review decision, migration or proposal number taken).

1. I4: `gamedata.IsField` (server/gamedata/world.go) is also true for IDs that `gen.ParseFieldID` reads, so `FieldLevels` has a procedural world's fields and `level_band` events pick them. Tests: `TestFieldLevelsGenerated` and `TestLevelBandPicksGeneratedFields` (a `level_band` range through `worldevent.MapsOf` picks the generated fields, keyed as waserver does).
2. C5, E6: `spawns.modify` and `world.generate` are `Flagged` (server/mod/permission.go); mods/README.md says they are logged at each start. Test: `TestSpawnsAndGenerationFlagged` (server/mod/mod_test.go).
3. I1: `Provisional` is gone from `set_staff_role`, `plan_restart`, `rotate` and `change_region` (spec.go). docs/generated/lua-api.md and docs/generated/lua/wa/types.lua regenerated with `go test ./server/modscript/luaapi -run TestGeneratedFiles -update`.
4. B6: `entered_maps` and `memo_points` (`account.ExportMemo`: `slot`, `map`, `x`, `y`) in `ExportCharacter`, set in server/adminworld/account.go; version stays 1; decision 0032 lists them. `store.ReadCharacter` now loads the maps. Test: `TestAccountPageExport` extended.
5. E7: core's guild, rebirth and quest scripts use `player.skill_points`, `player.weight` and `player:give_exp`. `give_exp`, `skill_points` and `weight` are out of `wa.internal`, with their Go handlers, `CoreWorldHost` methods and zone code. Test: `TestInternalHasNoPublicCalls`.
6. C1, F8: tests only. `TestMonstersGetInHooksAndLayers` (server/zone/spawntable_test.go) and `TestJailReturnMapRemoved` (server/zone/social_test.go). Both pass on the base, so no code changed for them.
7. G2: one line in mods/README.md under the spawn_table hook for `wactl sim economy --config waserver.yml`.

## Proof

- base (99a808a), I4: `go test ./server/gamedata -run TestFieldLevelsGenerated -p 1 -parallel 2` failed: `FieldLevels map[prt_fild08:12], want map[gen:field:-2:3:40 gen:field:0:0:5 prt_fild08:12]`.
- base, C5/E6: `go test ./server/mod -run TestSpawnsAndGenerationFlagged` failed: both permissions `Flagged:false`, no warning in the notes.
- base, I1: `go test ./server/modscript/luaapi -run TestGeneratedFiles` failed after the spec change (docs stale); it passes after `-update`. The flag affects docs only.
- base, B6: `go test ./server/adminworld -run TestAccountPageExport` (PostgreSQL 18 container) failed: `entered maps [], memo points []`.
- base, E7: `go test ./server/modscript -run TestInternalHasNoPublicCalls` failed: `messages = "1:function: #1 function: #2 function: #3"`.
- base, C1 and F8: the tests passed on the base; they are kept as tests only. The problems did not show.
- branch: each command above passes. Full package runs pass for server/gamedata, server/mod, server/modscript, server/account, server/adminworld, server/zone and tools/convert.

## Deviations

- Docs fixed after review: server/README.md, decisions 0042, 0044 and 0045 now point to 0077.
- Files outside the owned and shared paths, each kept minimal: server/mod/mod_test.go (new test), mods/README.md (permission rows and the G2 line), server/store/staffviews.go (one `loadMaps` call, needed for B6), server/zone/coreworld.go and server/zone/refine.go (removed the host methods of the three internal calls), server/adminworld/account_test.go and server/account/export.go (named in the brief's item 4).
- Behavior change in E7: the quest reward now goes through `player:give_exp`, which applies `quest_exp_rate` as rAthena's `getexp` does. The old internal call did not. The quest text (`Q.describe`, used by the board and the reward line) shows the scaled exp, read with `rules.get("quest_exp_rate")`. The script skips the call when both amounts are 0, since the public call refuses that.
- The brief asked for `TestPermissions`; a new test was added next to it and `TestPermissions` is unchanged.

## Stubs left

- None.

## Gotchas

- `TestProceduralContent` (tools/modkit) has the build tag `procedural`; run it with `-tags procedural`. It covers core's guild, rebirth and quest board scripts.
- `store.ReadCharacter` did not load entered maps and memo points before this node; any staff view that wants them reads them now.
- `store.ReadCharacter` loads maps and memos for every caller, the staff detail view too. A separate loader for the export needs new plumbing for two small queries, so it stays.
- `refine_info` and `refine` stay in `wa.internal` on purpose (the refiner keeps rAthena's rules).

## Proposed learnings

- A new field in the data export goes in both the export struct and the store view that builds it | `ReadCharacter` silently left it empty | server/store, server/adminworld | yes (the extended `TestAccountPageExport`)

## Verification

- `go vet` on server/gamedata, server/mod, server/modscript/..., server/account, server/adminworld, server/store, server/zone, and `-tags procedural` on tools/modkit: clean.
- `scripts/check.sh lint`: passed (0 issues, both passes).
- `go test -count=1 -p 1 -parallel 2` on server/gamedata, server/mod, server/modscript, server/modscript/luaapi, server/account, server/adminworld, server/zone, tools/convert: pass. `go test -tags procedural ./tools/modkit -run TestProceduralContent`: pass.
- The PostgreSQL container `wa-worldgenfixes-pg` was removed after the tests. `scripts/check.sh quick` and `all` were not run (the orchestrator runs them).

## Size

- Estimated M, actual M.
