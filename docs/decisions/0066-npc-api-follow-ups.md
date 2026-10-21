# 0066: NPC API Follow-Ups: Players Across Zones and Parity Fixes

- Status: built. Details to review.
- Date: 2026-10-21.
- Code: [server/zone](../../server/zone) (`playerworld.go`, `remotemonsters.go`, `npcworld.go`, `npcpresent.go`, `maps.go`, `entities.go`, `monster.go`, `scripts.go`, `zone.go`, `warp.go`, `itemscripts.go`, `itemactions.go`, `worldcalls.go`, `skillscripts.go`, `statuses.go`, `loot.go`), [server/modscript](../../server/modscript) (`remoteplayers.go`, `npcworld.go`, `present.go`, `maps.go`, `skills.go`, `itemapi.go`, `world.go`, `host.go`, `runtime.go`, `prelude.lua`, `luaapi/spec.go`), [server/world](../../server/world) (`world.go`), [server/mod](../../server/mod) (`permission.go`), [client/src](../../client/src) (`viewer/online.ts`, `viewer/player.ts`, `render/sprites.ts`, `render/characters.ts`), [tools/convert](../../tools/convert) (`npcplayers.go`, `npcworld.go`, `npcscript.go`, `npcpresent.go`, `npcmaps.go`, `npcexpr.go`, `npclua.go`, `classicnotes.go`, `rathena.lua`).
- Docs: [mods/README.md](../../mods/README.md), [tools/convert/README.md](../../tools/convert/README.md), [server/README.md](../../server/README.md), [client/README.md](../../client/README.md), [lua-api.md](../generated/lua-api.md).
- Builds review decision 59 of [proposal 0002](../proposals/0002-lua-api.md), the follow-ups of decisions [0062](0062-npc-api-group-1.md) to [0065](0065-npc-api-group-4.md), and fixes differences from rAthena that decision 0065 left.
- Lua API: adds to API 1.0. Protocol: none. Settings: none. Data: none. Store: none. Permission: `maps.control`, flagged.

## Context

- Each map runs in its own zone, and decision 0062 kept player handles in their zone. rAthena's `OnPCLoginEvent` and the other player labels run for the NPCs of every map, and `attachrid` and the commands that take a character ID act on a player of any map. The converter refused them.
- Decisions 0063 to 0065 listed calls that converted scripts need and that the API did not have, and differences from rAthena.
- Review decision 59 approved players across zones, more calls for parity, and `maps.control`, and kept group 4's calls.

## Decision

### Players across zones

- The world keeps a `PlayerDirectory`: each online player's zone, its ID there, its name, account and character IDs, its sex, and a copy of its fields. A zone lists a player when it joins and drops it when it leaves. Its zone writes the copy at the end of each step.
- `maps.find_player` and `maps.find_player_by_id` find a player of any map, as `map_nick2sd`, `map_id2sd` and `map_charid2sd`. A player of another zone is a handle from 2^42 of the calling zone, which names the character, so it follows the player from map to map. It acts as a local handle once the player is on the zone's map.
- Its fields come from the copy: `kind`, `name`, `map`, `x`, `y`, `character_id`, `account_id`, `sex`, `base_level`, `job_level`, `job`, `job_class`, `zeny`, `hp`, `max_hp`, `sp`, `max_sp`, `weight`, `max_weight`, `skill_points`, `party` and `guild`. Other fields raise an error.
- Its methods of `game.read` raise "the player is on another map". `exists` reads the directory. Every other method goes to the player's zone as a message, which runs it after the current step in the calling mod's context, or a context the runtime keeps for a mod without one, with the mod's permissions. The call returns nil. Its arguments must be plain values and tables. A player who went offline gets nothing, as rAthena's commands only log an error.
- The prelude wraps every player method, so new methods follow the same rule. A method's kind comes from the API definitions.

### Player events of every map

- `login`, `logout`, `map_enter`, `base_level_up`, `job_level_up`, and `death` of a player reach the NPC handlers of every map, as `npc_script_event` (npc.cpp 5824). Mod contexts still hear their own map. A disabled NPC hears none, as `npc_event_sub` (npc.cpp 1767).
- The directory counts each zone's NPC handlers of these events. A player's zone sends each event to the zones that have one. A zone that has not started its NPC scripts gets it when one of its NPC scripts names the event in its source, as `events.on("login", ...)` does, and the message starts its scripts.
- The handler gets a handle of the player. In `death`, a player killer is a handle too. A monster killer of another map is left out.
- A logout that other zones heard waits until each answers, up to 2 seconds, before the player's last save, as `npc_script_event` runs before `map_quit` (map.cpp 2266). The other zones' calls on the player run in the wait.

### More calls for parity

| Call | rAthena |
|---|---|
| `npc:set_sprite(sprite, { keep = false, to })` | `classchange` (script.cpp 15163): sent to the area or one player, not kept. A job's view sends nothing (clif.cpp 1477). |
| `npc:stop({ send_cell = false })` | `unitstopwalk`'s `USW_NONE` (script.cpp 20186): the NPC stands at once and nothing is sent (unit.cpp 1725, 1732). |
| `player:play_sound(file, { to = "area" })` | `soundeffectall` without a map, around the attached player (script.cpp 15315). |
| `npc:move_to(x, y, direction)` | `movenpc`'s direction, `dir % 8` (script.cpp 16161). |
| `npc:walk_to(x, y, { label })` | `unitwalk`'s label (script.cpp 20049): it runs with no player when a walk ends on its cell, and stays until one does (unit.cpp 650). |
| `world.play_music(file)`, `maps.play_music(map, file, { rectangle })` | `playBGMall` without a map, and with an area (script.cpp 15253). |
| `equipped:destroy({ refine_failure = true })` | `failedrefitem` (script.cpp 9802): the refine goes to 0, and the wearer shows `REFINE_FAILURE`. |
| `monster.game_id`, `maps.find_monster(id)` | A GID, `$@mobid` (script.cpp 11312), `map_id2md`. |
| `maps.find_entities { map = "other", ... }` | `areapercentheal`, `areamobuseskill` on another map. |
| `party:warp(target, x, y, { from, range_x, range_y, player })` | `warpparty`'s targets `Random`, `SavePoint`, `SavePointAll`, `Leader` and `RandomAll`, its from map and its range (script.cpp 5781). |
| `maps.warp_players(map, "random", ...)` | `areawarp` to `"Random"` (script.cpp 5670). |
| `"this"` | The converter reads it as the attached player's map, for `monster`, `areamonster` and `mobcount`. |

- A monster's game ID is its zone's serial in the high 32 bits and its serial in the zone in the low ones. Each zone shares its living monsters at the end of each step.
- A handle of a monster of another map reads `kind`, `id`, `name`, `level`, `hp`, `max_hp`, `map`, `x`, `y`, `is_alive` and `game_id` from that list. `kill`, `remove`, `set_hp`, `set_damage_immune`, `set_modes`, `set_speed`, `use_skill_at` and `warp_to` go to its zone as messages and return true. Its other methods raise an error.
- `spawning.monsters` on another map reserves the monsters' serials in the target zone at once and returns their handles. A monster that finds no cell leaves its handle without a monster. Each placed monster leaves the spawns that wait as it joins the counts, under one lock, so another zone never counts it twice.
- `maps.find_entities` on another map reads the players of the directory, the monsters the zone shares, and the NPCs of the NPC directory. Its area is a cell with a radius, a rectangle or cells, with no entity filters.
- `warpparty`'s range tries 10 cells and keeps the cell itself when the tries run out, even when the tenth found one, as its loop's count does. `RandomAll` tries 1,000 cells away from warps. `Leader` reads the leader's cell, and moves nobody when the leader is offline.
- The provisional marks of `maps.count_monsters`, `maps.get_flag` and their options are gone.

### maps.control

- A new permission, approved by the owner and flagged. `maps.remove_monsters` and `maps.set_cells` need it in place of `npc.create`. The built-in mods hold every permission.

### Differences from rAthena fixed

| Difference (decision 0065) | Now |
|---|---|
| Monster labels ran after the step, so every label of a `killmonster` saw the last count | Each label's event carries the counts of its map, and of its label, at the moment its monster died. `maps.count_monsters` reads them while the event runs, as rAthena runs each label inside `mob_dead` (mob.cpp 3600) and inside `killmonster`'s loop (script.cpp 11463). Two monsters that die in one step see 1 and 0, too. |
| A label of an NPC on another map ran with no player | It runs with the killer, or the player that dealt the most damage, on any map, as `npc_event` attaches a player of any map (mob.cpp 3604 to 3609). A player of another zone is a handle without a conversation. |
| Dead players stayed on map warps | `maps.warp_players` moves them, as `buildin_areawarp_sub` and `mapwarp` do not check `pc_isdead` (script.cpp 5659, 14340). A dead player arrives dead on another map and returns to its save point after the rest of its wait. `party:warp` skips them, as `warpparty` does (script.cpp 5870). |
| `setcell` sent its cells to the players | `maps.set_cells` sends nothing, as `setcell` and `map_setcell` (script.cpp 20631, map.cpp 3404). With `send`, it sends the cells to the map's players and to players who enter, as `map_iwall_set` and `map_iwall_get` do for `setwall` (map.cpp 3511, 3538). The converted `setwall` and `delwall` send them. |
| The client did not draw the baby and giant body effects | Effects 421 and 423 draw the entity at 0.75 and 1.25 of its size until it vanishes, with no particles. |

### Converter

- The player labels are handlers of the player events: `OnPCLoginEvent`, `OnPCLogoutEvent`, `OnPCBaseLvUpEvent`, `OnPCJobLvUpEvent`, `OnPCDieEvent`, and `OnPCLoadMapEvent` on maps with the `loadevent` flag (clif.cpp 11148), which no map has. They stay `npc_event` handlers too. A label that waits or shows a dialog does not convert, as the player events have no conversation.
- `OnPCKillEvent`, `OnNPCKillEvent`, `OnPCIdentifyEvent` and `OnPCStatCalcEvent` do not convert, with their reasons.
- `attachrid(<account ID>)` and `detachrid`. A command whose last argument names another character acts on its handle in a block: 42 commands, by `script_accid2sd`, `script_mapid2sd`, `script_charid2sd`, `script_nick2sd` and `map_id2bl` (tools/convert/README.md). `message` to another character too.
- `classchange`, `unitstopwalk` and `npcstop` flags, `soundeffectall` without a map and of a part of a map, `movenpc`'s direction, `unitwalk`'s label, `playBGMall` without a map and of an area, `failedrefitem`'s effect, `$@mobid`, `unitkill`, `unitwarp`, `unitskillusepos` and `setunitdata` with `UMOB_` on a monster's game ID, `unittalk` of another game ID, `warpparty`'s targets and range, `areawarp` and `mapwarp` to `"Random"`, `areapercentheal`, and `"this"`.

## Coverage

Script NPC lines of `docs/generated/npc-scripts-<mode>.md`:

| Mode | Lines | Before | After |
|---|---:|---:|---:|
| renewal, not duplicates | 7,067 | 6,620 (93.7%) | 6,623 (93.7%) |
| renewal, with duplicates | 14,865 | 12,900 (86.8%) | 12,905 (86.8%) |
| pre-renewal, not duplicates | 3,754 | 3,639 (96.9%) | 3,639 (96.9%) |
| pre-renewal, with duplicates | 8,205 | 7,907 (96.4%) | 7,907 (96.4%) |

- Most scripts that use these commands are in the instance, siege and battleground folders, which the extract leaves out, or have an earlier blocker. `poring_war.txt` still needs PvP map flags and `killedrid`.
- The first blockers of the renewal scripts that are not placed (`docs/generated/classic-coverage.md`): `navigateto` 73, `waitingroom` 33, variables named with a digit 27, `getpartnerid` 22, `questinfo` with a computed condition 16, `achievementinfo` 14, `add_reputation_points` 10, a computed `callshop` 10, instance maps 10, `getcastledata` 9, `getmercinfo` 9, `instance_create` 9, `mesitemlink` 9 and `refineui` 9.

## Breaks

- An NPC context's player event handlers hear the players of every map. Before, they heard their own map's.
- `maps.find_player` and `maps.find_player_by_id` find players of other maps.
- `maps.remove_monsters` and `maps.set_cells` need `maps.control`.
- `maps.set_cells` no longer sends the cells to the players without `send`.
- `spawning.monsters` on another map returns handles. Before, the list was empty.
- `maps.warp_players` moves dead players.
- A logout that NPCs of other maps heard ends up to 2 seconds later.
- Converted `classchange` no longer keeps the view, and `unitstopwalk` no longer sends the cell.

## Alternatives

- **Run another zone's handler in the player's zone.** An NPC's context and its locals live in its own zone's Lua state. Not possible without one state for the world.
- **Synchronous reads of a player of another zone.** A zone would wait on another, which can deadlock two zones that ask each other (decision 0065). Reads come from the copy of the last step, and reads of items and other state raise an error.
- **Start every zone's NPC scripts at boot**, so each knows its listeners. It makes a Lua state in every zone with NPCs (decision 0062). The zone that has not started is sent the events that its scripts' sources name.
- **Run labels inside the loop.** A label is Lua of an NPC context, which cannot run inside another script's call. The counts it reads are those of its monster's death instead.
- **Keep sending `setcell`'s cells.** Clients' paths would match the server's, but rAthena's do not, and the task is parity.

## Details to review

- API forms that decision 59 named and did not shape: `set_sprite`'s `keep` and `to`, `stop`'s `send_cell`, `play_sound`'s `to = "area"`, `move_to`'s third argument as rAthena's `DIR_`, `walk_to`'s `label`, `world.play_music`, `maps.play_music`'s `rectangle`, `destroy`'s `refine_failure`, `monster.game_id` and `maps.find_monster`, `party:warp`'s target names and options, and `"random"` for `warp_players`.
- API beyond decision 59, added because a fix the brief asked for needs it: `maps.set_cells`' `send` option, which keeps `setwall`'s cells sent while `setcell`'s are not.
- Wanted, not built, as it is new API: a death event option for monsters without a label (`OnNPCKillEvent`), `killedrid`, `attachrid` without force (whether the player talks), reads of another player's items and variables, `areamobuseskill`'s cast time, emotion and target, `unitexists`, a monster emotion, and statuses on a monster of another map.
- `OnPCLoadMapEvent` converts but never runs: `loadevent` is refused by `maps.set_flag` with the other flags.
- A handle of a player of another map has no conversation: a dialog command after `attachrid` of such a player still shows to the conversation's player, and a label event with a player of another map runs without one.
- The directory's copy is from the last step: `x` and `y` of another map's player lag up to one tick.
- A logout waits at most 2 seconds. A zone that does not answer in time loses its calls on the player.
- Remote calls return nil, so a script that tests `give_item`'s result reads failure.
- `sc_start` on a monster of another map raises an error: statuses stay on local handles.
- `unittalk` of a monster shows its name before the line, as `monster:talk` does. rAthena sends the line alone.
- A monster that kills itself in its own script's call, such as `kill` followed by `maps.find_monster` in the same call, is still found until the call returns, as deaths in a script's call wait for it (decision 0064).

## How to check it worked

- `go test -race ./server/world -run 'TestPlayersAcrossZones|TestMapsAcrossZones'`: an NPC of map a hears the login, map entry and logout of a player of map b, reads its fields, fails to read its items, finds it, and gives it zeny at login and logout, which the last save has. Monster labels of another map see 2, 1 and 0.
- `go test ./server/zone -run 'TestClassChangeNotKept|TestNPCMoveDirWalkLabelStop|TestSoundsAndMusicOfDecision59|TestMonsterGameIDs|TestWarpsOfDecision59|TestRefineFailureEffect|TestMonstersAndLabelsAcrossZones|TestMapsControlPermission|TestDeadPlayerWarpsToAnotherMap|TestSpawnLabels|TestCellsAndFlags'`.
- `go test ./tools/convert -run 'TestPlayersConvert|TestPresentConvert|TestMapsConvert|TestNPCWorldConvert|TestNPCEventsConvert|TestRunConvertedNPCs|TestNPCReportsCurrent'`.
- `pnpm vitest run src/viewer/online.test.ts` in `client/`.
