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
| `extracts/` | Monster spawns, spawn tags, minimum client tables, progression data (exp tables, status points, level penalty, and monster drops), job stats (weight, HP and SP tables, base ASPD, job bonus stats, and map job IDs), and items (type, weight, stacking, use and drop limits, and the heal of scripts that are only `itemheal` or `percentheal`), per mode. `items.json` lists the heal scripts it could not read in `unparsed_heal_scripts`. `warps.json` lists warp NPCs, described below. `script_constants.json`, `item_scripts.json` and `item_groups.json` are what the item script converter reads, described below. | `tools/wathena/extract.py` |

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

## Warps

`extracts/<mode>/warps.json` lists every `warp` and `warp2` NPC rAthena loads in the mode, and the duplicates of them, with their map, cell, touch span (`xs`, `ys`), target map and cell, kind and state. The extract follows rAthena's own loading:

- It reads the NPC files in the order `npc/<mode>/scripts_main.conf` lists them, with its `npc:`, `delnpc:` and `import:` lines, as `map_reloadnpc_sub`. Pre-renewal removes the Brasilis files this way.
- It skips lines on maps that are not in `db/map_index.txt`, `conf/maps_athena.conf` and the map cache, and lines whose cell is outside the map (`npc_parsesrcfile`). It skips warps whose target map is not in the map index (`npc_parse_warp`).
- It gives names as `npc_parsename`: the part after `::`, cut to 24 characters, and a new name when the name is taken. `duplicate(<name>)` copies the first NPC that took the name (`npc_parse_duplicate`), with the span of the duplicate line.
- The list keeps the load order, which decides which warp wins where areas overlap (`npc_touch_area_allnpc`).
- `state` is `enabled` unless the line sets `warp(DISABLED)`, `warp(HIDDEN)` or `warp(CLOAKED)`. Only enabled warps show and work, so `visible` is true for them.
- Script NPCs with a warp sprite (`WARPNPC` or `HIDDEN_WARP_NPC`) run an `OnTouch` script. They need the script engine, so the summary only counts them.

| Mode | Warps | Maps with warps | Script NPCs with a warp sprite |
|---|---:|---:|---:|
| renewal | 4,138 | 674 | 1,710 |
| pre-renewal | 3,085 | 488 | 621 |

## Item scripts

wAthena's converter (tools/convert) reads three extracts per mode:

- `script_constants.json`: every constant and parameter that `src/map/script_constants.hpp` exports, with its value, and the skill names of `skill_db.yml`. The extractor computes the values from the enums, `#define`s and constants of rAthena's headers, with `RENEWAL` or `PRERE` as in the mode and `PACKETVER` 20211103, as `record.sh` builds. Its job IDs and NPC sprite IDs match those of `client_tables.json`.
- `item_scripts.json`: `Script`, `EquipScript` and `UnEquipScript` of every item that has one.
- `item_groups.json`: `item_group_db.yml` as `ItemGroupDatabase` loads it, with `Clear` applied and entries without a rate skipped. rAthena keeps subgroups and entries in unordered maps, whose order depends on the C++ library. The file sorts them by subgroup number and index.

They were first extracted at rAthena fork commit `f4641e37b`, from the same rAthena data as the other files.

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

Then copy `generated/wathena/*` into this folder. `warps.json` was first extracted at rAthena fork commit `27ad000bc`, from the same rAthena data as the other files.
