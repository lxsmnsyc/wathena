# Outcome: review-world-gen

## Built

- review.md: 46 entries by source decision (0040 to 0047 and proposal 0002 steps 4, 5 and 17), a summary table, 15 questions for the maintainer with recommended answers, the proposed nodes and "Draft review decision 62". Review decision number taken: none (the orchestrator records 62 after approval). Decision record number taken: none.
- Counts: 19 accept, 13 settled, 7 refuse, 5 defer, 2 keep.
- Accepted work by node: world-gen-fixes 5 plus 3 checks (v0.2.0, should); world-gen-api 7 and world-gen-rules 5 (modding, could); mod-assets-gaps 2 (existing).
- Proposed new nodes: `world-gen-fixes`, `world-gen-api`, `world-gen-rules`.

## Deviations

- strategy.md has no "Open review items" section any more. The items come from the decisions' "Review items" lists and the proposal's steps.
- Added rows the brief does not list but the decisions do: D5, E7, G4, H2, and split F3 and F4.
- Two findings are bigger than the brief: typed events' `level_band` finds no field in a procedural world (I4), and rotations run no `spawn_table` hook (F1).

## Stubs left

- None. Nothing was built.

## Gotchas for the proposed nodes

- world-gen-fixes: `IsField` is read from the code, not run. Write the failing test first. Decision record 0067 is also named by security-fixes; take the next free number when the node starts.
- world-gen-fixes: the export is built in server/adminworld/account.go with `character.Character.Visited` and `Memos`; no migration. Core needs `player.give_exp` when its quest script calls it; check core's grants (server/mod/load.go).
- world-gen-fixes: the four `Provisional` flags are in spec.go:1247-1255; regenerate docs/generated/lua-api.md and docs/generated/lua.
- world-gen-api: `monsters.get` needs a `MapHost` (modscript/maps.go:195). The hook state is a zone with no map, so check it before building `spawn`.
- world-gen-api: layers run once per map at world creation, so `map:is_walkable` needs the cells passed to the layer state (server/content/layers.go, server/modscript/generation.go).
- world-gen-rules: `waserver` has no world secret, only derived keys (migration 00035). A hook at a rotation takes its stream from the change's key. `NewSpawnHooks` wants Mods without Testing.
- world-gen-rules: `State.Dates` and `world_rotations` are per dungeon already. Only `PickRotation` (layout/rotation.go:136) and tools/wactl/layout.go:203 pick one.
- world-gen-rules: a stored world keeps its quests, so `repeat` and `time_limit` apply to new worlds; the board script must treat a quest without the fields as `always`.
- Review decision 62 is not yet in proposal 0002. Cite it only after the orchestrator records it.

## Verification

- Read-only review: grep and file reads. No build, tests, Docker or scripts/check.sh were run.

## Size

- Estimated M, actual M.
