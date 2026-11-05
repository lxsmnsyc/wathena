# Outcome: extract-folders

## Built

- rAthena fork commit `b6294c1f6` (`wathena-reference` branch): `PART_FOLDERS` in `tools/wathena/extract.py` adds `instances`, `guild`, `guild2`, `guild3`, `battleground`, `custom` and `mobs`.
- New part files in reference/rathena/extracts: 8 in renewal and 5 in pre-renewal, `npcs-<folder>-<n>.json`, each below 1.9 MB. `left_out` is empty in both modes.
- tools/convert `countOnlyFolders`: script NPCs whose source is in `instances`, the guild folders or `battleground` convert for the reports but are not placed.
- classic-coverage.md's "Systems not built" table counts every guild folder.
- Decision record: docs/decisions/0069-npc-extract-folders.md. Reports regenerated. reference/rathena/README.md "Source" and "NPCs" updated.

## Deviations

- The brief's "mapflag lines" were already extracted: `map_flags.json` since fork commit `f99a928d1` (decision 0067). Nothing was added for them.
- `countOnlyFolders` is new. The brief only said battleground NPCs are for counting. Placing the guild and instance NPCs that convert made the converted-NPC run fail: `Investment_total#fund*` fails in `OnInit` on `agitcheck`, and the guild Kafras read guilds the test host does not have.

## Stubs left

- `countOnlyFolders`: instances-content removes `instances`, sieges-content removes `guild`, `guild2` and `guild3`, and battlegrounds (v0.3.0) removes `battleground`.

## Gotchas

- Report rows for the guild and instance folders now appear under the `siege folder` and `instance folder` reasons. The first command that blocks one of those scripts is listed only after the folder rule is lifted.
- `custom` and `mobs` NPCs are placed: renewal places 116 of 130 `custom` lines and 12 of 13 `mobs` lines.

## Proposed learnings

- Use bash with nullglob, not zsh, for copy loops over optional globs | a zsh glob with no match aborts the whole command | scripts and one-off copies | no

## Verification

- `python3 tools/wathena/extract.py npcs` in the fork: the rows of `npcs.json` and the part files of `quests`, `warps` and `events` are the same as before, apart from `rathena_git_hash`.
- `go test -count=1 ./tools/convert`: ok.
- `go test -count=1 -run '^TestClassicWorldProceduralBuild$' ./e2e/`: ok (30 s).
- Per-PR check: see the PR.

## Size

- Estimated M, actual M.
