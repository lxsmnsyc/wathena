# 0065: NPC API Group 4: Maps, Areas and Monsters

- Status: built. Details to review.
- Date: 2026-10-21.
- Code: [server/modscript](../../server/modscript) (`maps.go`, `world.go`, `runtime.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone](../../server/zone) (`maps.go`, `monster.go`, `entities.go`, `combat.go`, `query.go`, `mobai.go`, `zone.go`, `vending.go`, `worldmap.go`, `worldcalls.go`, `skills_alchemist.go`), [schema](../../schema) (`event.proto`), [client/src/viewer](../../client/src/viewer) (`online.ts`), [tools/convert](../../tools/convert) (`npcmaps.go`, `npcexpr.go`, `rathena.lua`).
- Docs: [mods/README.md](../../mods/README.md), [tools/convert/README.md](../../tools/convert/README.md), [server/README.md](../../server/README.md), [lua-api.md](../generated/lua-api.md).
- Builds review decision 58 of [proposal 0002](../proposals/0002-lua-api.md), area B8 of [proposal 0002b](../proposals/0002b-npc-api.md), the last group. [Decision 0062](0062-npc-api-group-1.md) built B1 to B4, [decision 0063](0063-npc-api-group-2.md) B5 and B6, and [decision 0064](0064-npc-api-group-3.md) B7, B9 and B10.
- Lua API: adds to API 1.0. Protocol: the event `MapCells`. Settings: none. Data: none. Store: none. Permission: none new.

## Context

- rAthena's scripts spawn monsters on any map with names and event labels, kill them by label, count them, move the players of a map or an area, change cells and map flags, and read monster records. The converter could not map these commands: `killmonster`, `monster` on another map or with a name of its own, `mapwarp` and `getmapusers` blocked hundreds of renewal scripts (proposal 0002b).
- Decision 0062 left `mobcount` of another NPC's label for this group, and decision 0040's converter work left `areamonster`'s rectangle and its failure rule.
- Each map runs in its own zone, so a call on another map crosses zones, as decision 0062's NPC calls do.

## Decision

### B8: maps, areas and monsters

| Call | rAthena |
|---|---|
| `spawning.monsters { map, display_name, size, ai, label, rectangle }` | `monster`, `areamonster` (script.cpp 11238, 11359) |
| `monster:kill()`, `monster:remove()` | `unitkill` (20061), `killmonster`'s kill (11480) |
| `maps.remove_monsters(map, { label, scripted, run_labels })` | `killmonster`, `killmonsterall` (11480, 11521) |
| `maps.count_monsters(map, { label })` | `mobcount` (14378) |
| `maps.count_players(map, { rectangle })` | `getmapusers`, `getareausers` (12146, 12167) |
| `maps.warp_players(map, to_map, x, y, { rectangle, to_rectangle, party, guild })` | `mapwarp`, `areawarp` (14317, 5695) |
| `party:warp(map, x, y)` | `warpparty` (5781) |
| `maps.set_cells(map, x1, y1, x2, y2, kind, on)`, `maps.check_cell(map, x, y, kind)` | `setcell`, `checkcell` (20631, 20614) |
| `maps.set_flag(map, flag, value)`, `maps.get_flag(map, flag)` | `setmapflag`, `removemapflag`, `getmapflag` (13617, 13683, 13584) |
| `maps.find_free_cell(map, { x, y, range_x, range_y, reachable_from })` | `getfreecell` (20657) |
| `maps.announce(map, text, { rectangle })` | `areaannounce` (12050) |
| `monsters.get(monster)` | `getmonsterinfo` (18711) |
| `monster:set_hp`, `set_damage_immune`, `set_modes`, `set_speed` | `setunitdata`'s `UMOB_HP`, `UMOB_DMGIMMUNE`, `UMOB_MODE`, `UMOB_SPEED` (19398) |
| `monster:use_skill_at(skill, level, x, y)`, `monster:warp_to(x, y)` | `unitskillusepos` (20305), `unitwarp` (20075) |

### Spawns

- Without x and y, each monster lands on a random cell as `mob_once_spawn_sub` finds it: `map_search_freecell` with the whole map, 50 tries, at least 15 cells from the edges (map.cpp `map_search_freecell`). A failure places no monster, where rAthena places it on the invalid cell.
- `rectangle` is `mob_once_spawn_area` (mob.cpp 732): up to the rectangle's cells times 3 tries, at most 1000, for a walkable cell. A failure uses the last good cell, and the first failure ends the spawn. A cell outside the map, or on its first row or column, is a random one.
- `display_name` is the name players see, cut to 23 bytes as `NAME_LENGTH`. `"--en--"` and `"--ja--"` are the monster's own (`mob_parse_dataset`). `""` shows no name, as rAthena.
- `size` is `special_state.size`: players get `EF_BABYBODY2` or `EF_GIANTBODY2` when the monster comes into sight (clif.cpp 1737, 5118). The stats stay, as `mob_size_influence` is off by default.
- `ai` is `special_state.ai` without a master. Such a monster fights the monsters without an AI, and those fight it unless its AI is `AI_WAVEMODE` (battle.cpp 8101). Players may hit it. It gives exp, as it has no master, and drops nothing unless it is a sphere, as `alchemist_summon_reward` 1 (mob.cpp 3089, 3260). Monsters without an AI look for it as a target.
- On another map, the spawn is a message to the map's zone, which places the monsters after the current step. They belong to no context, so `respawn` there raises an error, and the list is empty.

### Labels

- A label is rAthena's event label of monster: `"OnMyMobDead"` for the NPC of the context, or `"<NPC>::<label>"` of another NPC, found as `world.get_npc` finds it. Its key is the NPC's full record name and the label as written, as `buildin_killmonster_sub` compares the label exactly. An NPC that does not exist keeps the name as given, and its monsters run nothing.
- Another mod's NPC needs `npcs.control`, as `npc:fire_event`.
- A death runs the label, as `mob_dead`'s `npc_event` (mob.cpp 3600): with the killer, or the player that dealt the most damage (`first_sd`), when the NPC runs in the monsters' zone. Otherwise it runs with no player: a player handle acts in its own zone (decision 0062).
- The label runs after the current step, as an event between NPCs. A script's kill without labels sets the monster's `npc_killmonster`, and its label does not run.
- Each zone counts its living monsters, and those of each label, as the entity set adds and removes them. Other zones read the counts under a lock. A spawn on another map counts its monsters there at once, until the target places them, so `mobcount` after `monster` sees them, as rAthena.

### Kills

- `monster:kill()` and `monster:remove()` are `status_kill`: the monster dies with no exp and no drops (mob_dead's type 3), its slaves die, and the `death` event runs. `kill` runs the label, as `unitkill`. `remove` does not, as `killmonster`'s default.
- `maps.remove_monsters` without options is `killmonsterall`: every monster, and those of spawn lines come back. `label` is `killmonster` of a label, and `scripted` its `"All"`: every monster that no spawn line placed (`!md->spawn`). `run_labels` is type 1.

### Players and warps

- `maps.count_players` is `map->users`, or the players of a rectangle (`map_foreachinallarea`). Another zone's count is from the end of its last step.
- `maps.warp_players` moves the players of the map, of a rectangle, of a party or of a guild, as `mapwarp`'s types and `areawarp`. `to_rectangle` is `buildin_areawarp_sub`'s second area: up to its cells times 3 tries, at most 1000, for a walkable cell of the target, then the last try. The warp ends a talk and places a cell outside the map at random, as `pc_setpos` (`arrivalCell`).
- `party:warp` moves every online member that lives, on any map, as `warpparty`'s `WARPPARTY_RANDOMALLAREA` without a range. Each member's zone moves it after the current step. The map flags that refuse warps (`noreturn`, `nowarp`) and `pc_job_can_entermap` do not exist yet.

### Cells

- `set_cells` takes `walkable`, `shootable`, `no_vending` and `no_buying_store`. It refuses `water` (the server's map data has no water cells), `npc` (touch areas read their NPCs, not a cell flag), `basilica` (wAthena's Basilica is a status of its caster, and battlegrounds do not exist), `land_protector` and `maelstrom` (the skills do not exist), `no_chat` (chat rooms do not exist) and `ice_wall` (wAthena's Ice Wall is a skill unit).
- `walkable` and `shootable` copy the zone's grid, as Ice Wall does, and change it. The map's players get a `MapCells` event at once, and a player who enters gets every cell that scripts changed, as `clif_changemapcell` and `map_iwall_get` do for walls (map.cpp 3511, 3538). rAthena's `setcell` sends nothing; wAthena sends it so the client's paths match the server's.
- `no_vending` and `no_buying_store` refuse to open a shop on the cell, with msg 204, as clif.cpp 14259 and buyingstore.cpp 154.
- `check_cell` follows `map_getcellp`: a cell outside the map, or on its last row or column, is `no_pass` and `no_reach` and nothing else. `stack` is false, as `CELL_NOSTACK` is off by default. `npc` reads the touch areas of the context's own map.
- Another zone's cells come from a copy that its `set_cells` publishes. Ice Wall's changes after the last copy are not in it.

### Map flags

- `nomemo` is the one map flag wAthena has. A script's value overrides the world layout's for Warp Portal's memo.
- Every other flag of rAthena's `e_mapflag` raises an error that says why: PvP is not ported, sieges, battlegrounds, chat rooms, the bank, pets and day and night do not exist, the client draws no weather, or the flag's effect is not built yet. `get_flag` gives false for them.

### Free cells

- `find_free_cell` is `map_search_freecell` with flag 1: 50 tries within the ranges of x and y, never x and y themselves, or anywhere at least 15 cells from the edges for a range of -1. With no range, x and y when walkable. `reachable_from` is flag 2's `unit_can_reach_pos` with a direct walk. Flag 4 does nothing at the default `no_spawn_on_player` of 0, so it is not an option.

### Monster records and methods

- `monsters.get` reads the record's status: name, level, HP, SP, exp after the server's rates (rAthena applies them when it loads mob_db), attack, magic attack, defense, resistance, stats, speed, ranges, size, race, element, damage taken and modes by name.
- `set_hp` is `status_set_hp`: 0 changes nothing, and the HP is at most the maximum, so it never kills (status.cpp 1299).
- `set_damage_immune` is `ud.immune_attack`: no unit targets the monster (battle.cpp 7928).
- `set_modes` replaces the base modes, and `set_speed` the walk speed. Both copy the monster's status first, as `base_status`.
- `use_skill_at` casts a ground skill on a cell with its own cast time. Another skill returns false.
- `warp_to` is `unit_warp` on the monster's map: a cell that cannot be walked on is one within 4 cells of it.

### Protocol and client

- New event `MapCells`: packed cells with x, y and the walkable and shootable flags, at most 4096 to an event. The client changes its map's cells for the visit, keeps the water bit, and gives the pathfinder the new cells.

### Converter

- Every B8 command converts onto the calls above (tools/convert/README.md, `npcmaps.go`). A command on a map that does not run does nothing, as rAthena's, through `R.on_map`.
- `monster` and `areamonster` take their name, size and AI. A label of the NPC itself, another NPC's label, a label with a fixed NPC and a computed end, and a label made when the script runs all convert. A script that duplicates run names its own NPC by the source's record, as rAthena reads the name.
- `setwall`, `delwall` and `checkwall` work over `maps.set_cells`, with the walls by name in a temporary server table, as rAthena's `iwall_db`.
- `getmonsterinfo`'s `MOB_MODE` gives rAthena's bits.

## Coverage

Script NPC lines of `docs/generated/npc-scripts-<mode>.md`:

| Mode | Lines | Before | After |
|---|---:|---:|---:|
| renewal, not duplicates | 7,067 | 6,213 (87.9%) | 6,620 (93.7%) |
| renewal, with duplicates | 14,865 | 12,168 (81.9%) | 12,900 (86.8%) |
| pre-renewal, not duplicates | 3,754 | 3,342 (89.0%) | 3,639 (96.9%) |
| pre-renewal, with duplicates | 8,205 | 7,327 (89.3%) | 7,907 (96.4%) |

The remaining renewal scripts that are not duplicates, by their first blocker (`docs/generated/classic-coverage.md`): `navigateto` 73, `waitingroom` 33, variables named with a digit 27, `getpartnerid` 22, `questinfo` with a computed condition 16, `achievementinfo` 14, `add_reputation_points` 10, a computed `callshop` 10, instance maps 10, `getcastledata` 9, `getmercinfo` 9, `instance_create` 9 and `refineui` 9. The extract still leaves out the instance, siege and battleground folders: 3,531, 1,267 and 295 script NPC lines.

## Breaks

- `spawning.monsters` on another map spawns there. Before, it raised an error.
- `spawning.monsters` without x and y uses `map_search_freecell`'s rule: no cell within 15 of the edges, and 50 tries. Before, any walkable cell, with 1000 tries.
- Converted monsters of event labels run their labels through `npc_event` handlers in the zone. Before, `R.monster` ran the label's function through a `death` handler of the spawning NPC's own map.
- `npc_event_of` and the old `R.mobcount` of `rathena.lua` are gone.
- The protocol gains `MapCells`. `schema.Version` stays 1 until the next release raises it.

## Alternatives

- **Labels in the converter.** The converter kept its own counts and death handlers. That works on the NPC's own map only: a death on another map happens in another zone's Lua state. Built in the zone, so every zone counts and fires them.
- **Monster handles across zones.** `$@mobid` and the unit commands on monsters need a handle of a monster of another map, as decision 0062's remote NPC handles. Not built: decision 58 does not name it, and the unit commands that read `$@mobid` are mostly in instances.
- **Synchronous reads of another zone.** A call to another zone that waits for an answer can deadlock two zones that ask each other. The zones share counts, cells and flags under a lock, and other zones read the last step's state.

## Details to review

- API that decision 58 and proposal 0002b do not name, or name differently, built because the user's brief or the commands needed it. `maps.count_monsters` and `maps.get_flag` are marked provisional in the API reference:
  - `maps.count_monsters(map, { label })` for `mobcount`. 0002b names the need ("count of a label's monsters on another map") but no call.
  - `maps.get_flag(map, flag)` for `getmapflag`. 0002b names `set_flag` alone.
  - `spawning.monsters`' `label` and `rectangle`, and `maps.remove_monsters`' options `label`, `scripted` and `run_labels`. 0002b names `spawning.remove(label)` for killmonster in its table, and `maps.remove_monsters(map)` in B8.
  - `maps.warp_players`' `to_rectangle` for areawarp's target area, which 0002b names as "a target area".
  - `maps.find_free_cell`'s `x`, `y`, `range_x`, `range_y` and `reachable_from`.
  - Size and AI names: `"normal"`, `"small"` and `"big"`, and rAthena's `AI_` names in lower case.
- Wanted, not built, as it is new API: `party:warp` from one map with a range (`warpparty`'s from map, `rx` and `ry`) and its targets `Random`, `SavePoint`, `SavePointAll`, `Leader` and `RandomAll`; `maps.warp_players` to `Random` (`areawarp`); monster handles by game ID for `$@mobid` and the unit commands on monsters; `maps.find_entities` on another map (`areapercentheal`, `areamobuseskill`); and `"this"` as a map.
- Not built: `maps.can_enter` (`jobcanentermap`), which needs the WoE:TE map flags that come with sieges. Every map flag but `nomemo`, with its reason.
- Differences from rAthena:
  - Labels run after the step that killed the monsters. rAthena runs each one inside `killmonster`'s loop, so a label that reads `mobcount` sees the monsters that the loop has not killed yet.
  - A label of an NPC in another zone runs with no player.
  - Another zone's counts and cells are those of its last step. A kill on another map shows in its counts after that zone runs it.
  - Dead players stay where they are on `maps.warp_players` and `party:warp`. rAthena moves them.
  - `setcell` sends the cells to the players. rAthena's sends nothing.
  - A random cell that no try finds places no monster.
  - The client does not draw the baby and giant body effects yet.
  - A converted monster on a cell that units cannot walk on lands on a random cell. rAthena places it on the cell.
  - `killmonster` and `mobcount` with an empty computed label kill and count nothing, where rAthena matches the monsters without a label.
- The protocol changed since v0.1.0, as decision 0064 notes.

## How to check it worked

- `go test ./server/zone -run 'TestSpawnLabels|TestSpawnRectangle|TestCellsAndFlags|TestFreeCellsAndCounts|TestMonsterMethodsB8|TestScriptAIEnemies'`.
- `go test -race ./server/world -run TestMapsAcrossZones`: a floating NPC spawns monsters with its label on another zone's map, counts them before they spawn, changes that map's cells and flag, and kills them with their labels, which run in its own zone and see the other map's counts, cells and flag.
- `go test ./server/modscript -run TestSpawningAndDeath`, and `go test ./server/modscript/luaapi`.
- `go test ./tools/convert -run 'TestMapsConvert|TestNPCEventsConvert|TestRunNPCEvents|TestRunConvertedNPCs|TestNPCReportsCurrent'`.
- `pnpm vitest run src/viewer/online.test.ts` in `client/`.
