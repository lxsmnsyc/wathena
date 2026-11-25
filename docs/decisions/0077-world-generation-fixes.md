# 0077: World generation fixes for v0.2.0

- Status: built.
- Date: 2026-11-25.
- Code: [server/gamedata](../../server/gamedata) (`world.go`), [server/mod](../../server/mod) (`permission.go`), [server/modscript](../../server/modscript) (`prelude.lua`, `coreworld.go`, `runtime.go`, `luaapi/spec.go`), [server/account](../../server/account) (`export.go`), [server/adminworld](../../server/adminworld) (`account.go`), [server/zone](../../server/zone) (`coreworld.go`, `refine.go`), [tools/convert/core/scripts/procedural](../../tools/convert/core/scripts/procedural).
- Docs: [mods/README.md](../../mods/README.md), [docs/generated/lua-api.md](../generated/lua-api.md), [decision 0032](0032-account-page.md).
- Builds review decision 62 of proposal 0002 (docs/roadmaps/v0.2-parity/nodes/world-gen-fixes/brief.md).

## Context

The review of world generation (review-world-gen) accepted seven small items for v0.2.0.

## Decisions

- I4: `gamedata.IsField` is also true for IDs that `gen.ParseFieldID` reads, so `FieldLevels` has the fields of a procedural world and typed events with `level_band` pick them. Classic names (`_fild`) stay as they are. There is no general map type in map data.
- C5, E6: `spawns.modify` and `world.generate` are flagged: the loader logs a warning at each start for a mod that has them. Nothing else changes.
- I1: `set_staff_role`, `plan_restart`, `rotate` and `change_region` of `wa.testing` lose `Provisional`; the Lua docs are regenerated.
- B6: the data export has `entered_maps` and `memo_points` (`slot`, `map`, `x`, `y`) for each character. The format stays version 1, as decision 0032 says new fields do. Both are `[]` when empty. No migration: the game side reads `character.Character.Visited` and `Memos`.
- E7: core's procedural scripts call `player:give_exp`, `player.weight` and `player.skill_points`. `give_exp`, `skill_points` and `weight` leave `wa.internal`, with their Go handlers, `CoreWorldHost` methods and zone code. `refine_info` and `refine` stay: the refiner keeps rAthena's rules. The quest reward now follows `quest_exp_rate`, as rAthena's `getexp` does. The reward text on the quest board and after a quest shows the scaled exp (`Q.describe`, read with `rules.get`), before the guild's tax. The script skips the call when both exp amounts are 0, since the public call refuses that.
- C1, F8: tests only. `monsters.get` works in a `spawn_table` hook and in a generation layer (`TestMonstersGetInHooksAndLayers`), and a freed jailed player whose return map is gone goes to the save point (`TestJailReturnMapRemoved`). Both passed on the base.
- G2: the mod guide says to run `wactl sim economy --config` after hooks change the spawn lists.

## Tests

- `server/gamedata`: `TestFieldLevelsGenerated`, `TestLevelBandPicksGeneratedFields` (`worldevent.MapsOf` with levels keyed `map/` plus the field ID, as waserver keys them).
- `server/mod`: `TestSpawnsAndGenerationFlagged`.
- `server/modscript`: `TestInternalHasNoPublicCalls`; `server/modscript/luaapi`: `TestGeneratedFiles`.
- `server/adminworld`: `TestAccountPageExport` (entered maps and memo points).
- `server/zone`: `TestMonstersGetInHooksAndLayers`, `TestJailReturnMapRemoved`.
- `tools/modkit` with `-tags procedural`: `TestProceduralContent` (core's guild, rebirth and quest board scripts).

## Left

- Decisions 0042, 0044 and 0045 name the flags and the jail line as open. Each now points here.
- Decision 0045's "Left" line about jail return points on removed maps is stale: the code handled it, and the test now covers it.
