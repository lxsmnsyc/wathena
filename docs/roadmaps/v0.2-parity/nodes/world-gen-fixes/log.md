# Log: world-gen-fixes

Base: batch/v0.2-parity at 99a808a. Branch fix/world-gen-fixes. Decision record number: 0077 (highest in docs/decisions was 0076).

## Already there check

None of items 1 to 7 existed on the base (checked by grep: IsField had no gen check; spawns.modify and world.generate not Flagged; four Provisional flags present; export had no entered_maps; internal.* still used).

## 1 I4

- Test: server/gamedata/fieldlevels_test.go TestFieldLevelsGenerated.
- Base: `go test ./server/gamedata -run TestFieldLevelsGenerated -p 1 -parallel 2` failed: `FieldLevels map[prt_fild08:12], want map[gen:field:-2:3:40 gen:field:0:0:5 prt_fild08:12]`.
- Fix: IsField also true when gen.ParseFieldID reads the ID.

## 2 C5, E6

- Test: server/mod/mod_test.go TestSpawnsAndGenerationFlagged (new; TestPermissions untouched, its grants and notes checks stay green).
- Base: `go test ./server/mod -run TestSpawnsAndGenerationFlagged -p 1 -parallel 2` failed: both permissions `Flagged:false`, no warning in notes.
- Fix: `Flagged: true` in server/mod/permission.go; mods/README.md permission table says "logged as a warning at each start".

## 3 I1

- No behavior bug: the flag only changes the docs. Proof is the generated-files test: after removing `Provisional` from the four helpers in spec.go, `go test ./server/modscript/luaapi -run TestGeneratedFiles` failed (stale docs/generated). Then `-update` regenerated lua-api.md and lua/*; the test passes.

## 4 B6

- Test: TestAccountPageExport in server/adminworld/account_test.go, extended with two entered maps and two memo points (PostgreSQL 18 container wa-worldgenfixes-pg, WATHENA_TEST_DATABASE_URL).
- Base: `go test ./server/adminworld -run TestAccountPageExport -p 1 -parallel 2` failed: `entered maps [], memo points []`.
- Fix: `EnteredMaps` and `MemoPoints` (`ExportMemo`) in ExportCharacter; set in adminworld exportCharacter; both `[]` when empty. Decision 0032 lists them. Version stays 1. After: adminworld and account TestExport* pass (TestExportHasNoSecrets included).

## 5 E7

- Test: server/modscript/internal_test.go TestInternalHasNoPublicCalls (core mod reads `wa.internal`).
- Base: failed: `messages = "1:function: #1 function: #2 function: #3"`, want nil for give_exp, skill_points, weight.
- Fix: removed the three from prelude.lua `core_internal`, their Go handlers (modscript/coreworld.go, runtime.go), the CoreWorldHost methods and their zone implementations (zone/coreworld.go, zone/refine.go). rebirth.lua and guild.lua read `player.weight` and `player.skill_points`; quests.lua calls `player:give_exp` when either exp is above 0 (the public call refuses 0, 0). Core holds every permission (mod/load.go builtin grants), so `player.give_exp` is there.
- Behavior note: public `give_exp` scales by quest_exp_rate (rAthena getexp); the old internal call did not.
- Checked: `go test -tags procedural ./tools/modkit -run TestProceduralContent -p 1 -parallel 2` passes (guild, rebirth and quest board cases). Note the test needs the `procedural` build tag.

## 6 C1, F8 (tests only)

- server/zone/spawntable_test.go TestMonstersGetInHooksAndLayers: passes on the base (no code change needed). The hook sets a count from `monsters.get(1002).level` and the layer a boss x from it, so a nil result fails the test.
- server/zone/social_test.go TestJailReturnMapRemoved: passes on the base; the jail's return map is set to a map that does not run, save point 12,13.

## 7 G2

- mods/README.md: one line under the spawn_table hook for `wactl sim economy --config waserver.yml`.

## Decision

- docs/decisions/0077-world-generation-fixes.md.

## Store fix found late (item 4)

- The full adminworld package run failed TestAccountPageExport (`entered maps [], memo points []`): `store.ReadCharacter` (staff and export view) did not call `loadMaps`. Added the call in server/store/staffviews.go (outside the owned paths; deviation). adminworld and the store staff tests pass after it.

## Review fixes

- I4: TestLevelBandPicksGeneratedFields in server/gamedata (worldevent.MapsOf, levels keyed `map/`+ID as main.go:682-686 does). A worldevent-package test would cycle (gamedata imports worldevent), so it lives in gamedata. Passes.
- Docs: server/README.md list, decisions 0042, 0044, 0045 point to 0077.
- Quest text: `Q.describe` scales by `rules.get("quest_exp_rate")`; board listing and reward line use it. TestProceduralContent (-tags procedural) passes.
- ReadCharacter keeps loading maps for all callers; left, see outcome.
