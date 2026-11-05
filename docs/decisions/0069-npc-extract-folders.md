# 0069: NPC Extract Folders: Instances, Guilds, Battlegrounds, Custom and Mobs

- Status: built. Details to review.
- Date: 2026-11-05.
- Code: [tools/convert](../../tools/convert) (`npc.go`, `classicnotes.go`), [reference/rathena](../../reference/rathena) (`extracts/<mode>/npcs.json`, `extracts/<mode>/npcs-<folder>-<n>.json`, `embed_test.go`, `README.md`).
- Docs: [npc-scripts-renewal.md](../generated/npc-scripts-renewal.md), [npc-scripts-pre-renewal.md](../generated/npc-scripts-pre-renewal.md), [classic-coverage.md](../generated/classic-coverage.md).
- Builds roadmap node extract-folders (docs/roadmaps/v0.2-parity).
- Lua API: none. Protocol: none. Settings: none. Data: none. Store: none. Permission: none.

## Context

- The NPC extract kept the script NPCs of the folders the first release needed, plus `quests`, `warps` and `events` in part files (review decision 55). It counted the rest in `left_out`: 4,555 script NPC lines in renewal and 1,551 in pre-renewal. So the coverage reports did not show the real totals, and instances-content and sieges-content had no scripts to convert.

## Decision

### Extract

- The rAthena fork's `PART_FOLDERS` adds `instances`, `guild`, `guild2`, `guild3`, `battleground`, `custom` and `mobs` (fork commit `b6294c1f6`). Their script NPCs go to part files of at most 1.9 MB each, as review decision 55 set. A folder with no script NPCs in a mode, such as `guild3` and `custom` in pre-renewal, has no part file.
- `left_out` is now empty in both modes. The rows of `npcs.json` and the part files of `quests`, `warps` and `events` do not change. Only `npcs.json`'s header changes.
- The extracts grow from 89 MB to 94 MB: 8 new part files in renewal and 5 in pre-renewal. `reference/rathena` embeds them through its `npcs-*.json` patterns: 77 files, 98.5 MB of JSON, and 12.4 MB of gzip in the release builds (review decision 56). `embed_test.go` expects 77.

### Placement

- The converter converts every new script NPC, so the reports count it. It does not place a script NPC whose source is in `instances`, `guild`, `guild2`, `guild3` or `battleground` (`countOnlyFolders`). The report gives the reason (`instance folder`, `siege folder`, `battleground`).
- Reasons:
  - The instance and siege systems are not built yet. The instances and sieges stories place these NPCs.
  - Battlegrounds are a v0.3.0 system.
  - Some of these NPCs convert but fail when they run. For example, `Investment_total#fund*` fails in `OnInit` on `agitcheck`, and the guild Kafras read guilds that the converted-NPC test host does not have.
- The rule uses the folder of the NPC's source, so a duplicate follows its source. The 4 lines of the `instances` folder that were already placed, duplicates of NPCs in other folders, stay placed.
- `custom` and `mobs` are placed like every other folder: rAthena loads them, and their systems exist. Renewal places 116 of 130 `custom` lines and 12 of 13 `mobs` lines.
- classic-coverage.md's "Systems not built" table now counts the lines of `guild` and `guild2` in the extract too. Before, it added only `guild3`.

## Breaks

- None. Classic worlds place 128 more script NPC lines, all from `custom` and `mobs`. That takes renewal from 13,322 to 13,450 of 19,892 lines.

## Alternatives

- **Place every NPC that converts**, as rAthena loads them. This places 865 guild NPCs and 209 battleground NPCs before the systems that run them exist. Some of them would fail at start.
- **Keep the folders out of the extract** until instances-content and sieges-content. The reports would then keep showing totals that are too low.

## Details to review

- `countOnlyFolders` is temporary. instances-content removes `instances`, sieges-content removes the guild folders, and the battlegrounds node removes `battleground`.
- `custom` holds the scripts of rAthena's default `scripts_custom.conf`, which a server owner usually changes. wAthena places the default set.

## How to check it worked

- `go test ./tools/convert`: the reports are current, and `TestRunConvertedNPCs` starts every placed NPC.
- docs/generated/npc-scripts-*.md and classic-coverage.md list no folder as left out of the extract.
- `go test -run '^TestClassicWorldProceduralBuild$' ./e2e/` (it builds waserver with `-tags procedural` itself).
