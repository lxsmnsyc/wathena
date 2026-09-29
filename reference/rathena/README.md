# rAthena Reference Data

This folder holds data recorded from rAthena. wAthena's parity tests compare against it.

## Loading

Go code loads these files with the `internal/reference` package. Its tests check that every file decodes and that each mode's files match that mode.

## Source

- rAthena commit: `e98500617` (full hash in each file's `rathena_git_hash` field).
- Recorded with the `wathena-reference` branch of a rAthena fork, which adds a recorder to `map-server-generator` and two scripts.
- The recorder and scripts stay in the rAthena fork. This repository keeps only their output, so it does not need a C++ toolchain.

## Folders

| Folder | Content | Produced by |
|---|---|---|
| `renewal/`, `pre-renewal/` | Formula results for each mode, listed below. | `tools/wathena/record.sh` |
| `inventory/` | Skills, script commands, RENEWAL sites, and feature inventory gaps. | `tools/wathena/inventory.py` |
| `extracts/` | Monster spawns, spawn tags, and minimum client tables, per mode. | `tools/wathena/extract.py` |

## Formula files

Each file starts with `system`, `mode`, `rathena_git_hash`, `data_path`, and `circular_area`.

| File | Content |
|---|---|
| `pathfinding.json` | Walk paths and line-of-sight checks on six maps, with a hash of each map's walkable cells. |
| `distance.json` | Distance and range check results for every offset up to 20 cells. |
| `elements.json` | The element damage table for levels 1 to 4. |
| `mob_status.json` | Final stats for every monster. |
| `mob_damage.json` | Normal attacks between 400 pairs of monsters. |
| `pc_status.json` | Base and final stats for 39 jobs, 3 level pairs, and 6 stat builds, with and without skills. |
| `pc_damage.json` | Normal attacks by level 99 players with every usable weapon type against five monsters. |
| `skill_damage.json` | Every damaging skill in the sample job trees, at every level, against three monsters. |
| `item_bonuses.json` | Changes from each equipment item and card, in stats, damage dealt, and damage taken. |
| `refine.json` | Stats at every refine level for one armor and one weapon of each weapon level. |
| `status_effects.json` | Chance, duration, and whether ten common ailments start, for many sources and targets. |
| `level_penalty.json` | Renewal exp and drop modifiers by level difference. Empty in pre-renewal. |

## How random rolls are recorded

rAthena's random number output depends on the C++ standard library, so wAthena cannot reproduce the exact rolls. The recorder uses three methods instead:

- **`forced_min`.** Every random roll returns its lowest value and every chance succeeds. The result is exact. Because every chance succeeds, critical hits and similar effects always happen in this mode.
- **`rolls`.** Many attacks with a seeded generator. Only statistics are recorded: hits, criticals, minimum, maximum, and total damage.
- **Forced value search.** For status effects, the recorder forces the roll to each value and searches for the exact chance.

## Regenerating

In the rAthena fork, on the `wathena-reference` branch:

```text
tools/wathena/record.sh
tools/wathena/inventory.py
tools/wathena/extract.py
```

Then copy `generated/wathena/*` into this folder.
