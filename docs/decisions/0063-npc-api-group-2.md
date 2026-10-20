# 0063: NPC API Group 2: Presentation and Quest Icons

- Status: built. Details to review.
- Date: 2026-10-21.
- Code: [server/modscript](../../server/modscript) (`present.go`, `npcs.go`, `quests.go`, `runtime.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone](../../server/zone) (`npcpresent.go`, `npcworld.go`, `npcs.go`, `quests.go`, `scripts.go`, `zone.go`, `cast.go`, `inventory.go`, `loot.go`, `jobchange.go`), [server/mob](../../server/mob) (`stats.go`), [server/mod](../../server/mod) (`npcs.go`), [schema](../../schema) (`event.proto`), [client/src/viewer](../../client/src/viewer) (`online.ts`, `main.ts`, `digits.ts`, `quests.ts`, `chat.ts`, `viewer.css`), [tools/convert](../../tools/convert) (`npcpresent.go`, `npcexpr.go`, `npclua.go`, `npcscript.go`, `npcquest.go`, `npc.go`, `rathena.lua`).
- Docs: [mods/README.md](../../mods/README.md), [tools/convert/README.md](../../tools/convert/README.md), [server/README.md](../../server/README.md), [client/README.md](../../client/README.md), [lua-api.md](../generated/lua-api.md).
- Builds review decision 58 of [proposal 0002](../proposals/0002-lua-api.md), areas B5 and B6 of [proposal 0002b](../proposals/0002b-npc-api.md). [Decision 0062](0062-npc-api-group-1.md) built B1 to B4. Areas B7 to B10 come later.
- Lua API: adds to API 1.0. Protocol: new events and fields, listed below. Settings: none. Data: an NPC record's `sprite` takes a monster's or a job's number. Permission: none new. Another mod's NPC needs `npcs.control`, as in decision 0062.

## Context

- rAthena's NPCs speak to the map or one player, show effects and emotions on other units, change their titles, names and views, walk, and show quest icons by conditions. The converter could not map these commands, and `questinfo` alone blocked 424 renewal scripts as their first blocker (proposal 0002b).
- Review decision 58 approved the B batch with its calls. This record builds areas B5 and B6, and keeps decision 0062's design of calls across zones.

## Decision

### Who gets what

Each call follows the send target that rAthena's `clif_send` uses:

- `"area"` is every player who sees the entity, as `AREA`. A player's view is `AREA_SIZE`, so these are the same players.
- `"map"` is every player of the entity's zone, as `ALL_SAMEMAP`. An effect still needs the entity in sight, as the client draws it on the entity.
- `"world"` is every player of every zone, as `ALL_CLIENT`.
- A player handle is that player, as `SELF`.
- `player:say_aloud` without `to` is local chat, as `unittalk`'s `clif_disp_overhead_`: the players within 9 cells who see the player (`AREA_CHAT_WOC`), and the player itself.

### B5: presentation

| Call | rAthena |
|---|---|
| `npc:say_aloud(text, { to, color })` | `npctalk` (script.cpp 16193) |
| `player:say_aloud(text, { to })` | `unittalk` (20216) |
| `player:show_emotion(emotion)` | `emotion` with a game ID (13818) |
| `npc:show_effect(effect, { to })`, `player:show_effect(effect, { to })` | `specialeffect`, `specialeffect2` (15596, 15634) |
| `npc:remove_effect(effect)`, `player:remove_effect(effect)` | `removespecialeffect` (15652) |
| `entity:show_skill_effect(skill, level)` | `skilleffect` (15519) |
| `npc:set_title(text)` | `setunittitle` (19944) |
| `npc:set_display_name(name)`, `npc:set_sprite(sprite)` | `setnpcdisplay` (17869), `classchange` (15163) |
| `npc:set_look { ... }` | `setunitdata` with `UNPC_` (19398) |
| `npc:move_to(x, y)`, `npc:walk_to(x, y)`, `npc:set_speed(ms)`, `npc:stop()` | `movenpc`, `npcwalkto`, `unitwalk`, `npcspeed`, `npcstop`, `unitstopwalk` |
| `npc:show_progress(seconds, { color })` | `progressbar_npc` (22546) |
| `npc:use_skill(skill, level, target, { stat, npc_level })` | `npcskill` (23286) |
| `npc:play_sound`, `player:play_sound`, `maps.play_sound` | `soundeffectall`, `soundeffect` (15308, 15281) |
| `player:play_music(file)`, `maps.play_music(map, file)` | `playBGM`, `playBGMall` (15226, 15253) |
| `player:send_message(text, { color })` | `dispbottom` (15796) |
| `player:show_digits(value, kind)` | `showdigit` (22745) |
| `maps.mark_minimap`, `maps.unmark_minimap` | `viewpointmap` (7013) |

- `npctalk`'s default color is the chat's own. A color of 0 cannot be sent, as the protocol reads 0 as the channel's color.
- A title is at most 23 bytes, as `NAME_LENGTH`. A name is 1 to 50 bytes.
- A change of the view, the name or the title makes the NPC appear again to the players who see it, as `unit_refresh` and `clif_name_area`.
- The view's kind follows `status_set_viewdata`: a job (`pcdb_checkid`, now `mod.IsJobView`), then a monster (`mobdb_checkid`), then an NPC sprite. A job view shows a character with the look of `UNPC_HAIRSTYLE` and the others. A class sets `sprite: none` NPCs visible.
- `npc:move_to` is `npc_movenpc`: the cell is capped to the map, the players who saw the NPC get a vanish (`clif_outsight`), and those near the new cell see it come (`clif_insight`).
- NPCs walk at `DEFAULT_NPC_WALK_SPEED`, 200 ms a cell. `npcspeed` takes 20 to 1000, and `status_get_speed` reads it at each step. A walk uses `CELL_CHKNOPASS` cells. `npc:walk_to` returns false outside the map or without a path.
- `npc:stop()` is `npcstop`'s default `USW_FIXPOS | USW_MOVE_FULL_CELL | USW_FORCE_STOP`: past half of its step, the NPC finishes the cell, then the players who see it get its cell.
- A walking NPC with a touch area runs the touch areas of the zone's players after each step, as `unit_walktoxy_ontouch`. A disabled NPC touches nobody.
- `npc:show_progress` waits in the conversation, as `progressbar_npc`'s sleep, and runs only in a conversation's handler. The bar shows to the players who see the NPC. The NPC keeps the bar's end, so a player who comes into view gets the time left.
- `npc:use_skill` gives the NPC `status_calc_npc_`'s status: 1 HP and SP, neutral, demi-human, small, range 1, every stat at `stat`, its level at `npc_level`, and `status_calc_misc`'s values (`mob.NPCStatus`). It casts with `unit_skilluse_id2`'s cast time of 0 and `ignore_range`. The values stay for the next cast, as rAthena keeps them on the NPC. The NPC must be on the context's map.
- `skilleffect` shows by the skill's cast type, as `script_skill_effect`: a ground skill on the unit's cell, a support skill as used on itself, and an attack as a hit of 0.
- Sounds and music are asset names. A sound of `soundeffect` and of a map comes from the player itself, as `soundeffect_sub`. An NPC's sound to its area comes from the NPC, as `clif_soundeffect` with the NPC.
- `showdigit`'s types 1 and 2 send the negated absolute value, and type 3 the absolute value, at most 99, as rAthena does.

### B6: quest icons

- `npc:add_quest_icon(icon, { mark, condition })` is `questinfo`. The runtime keeps each NPC's list in the order added. The zone lists the NPCs with icons in the order they got their first, as the map's `qi_npc`.
- A condition is a function of the player. It runs as a hook: a call that changes the game raises an error. An error counts as false and is logged, as `achievement_check_condition` gives false.
- The check is `pc_show_questinfo`: for each NPC, the first icon whose condition holds, and none when none holds. The zone keeps what each player was shown, as `qi_display`, and sends only changes.
- A player's icons are checked at the end of the step after: map entry (clif.cpp 11188), a change of the inventory (pc.cpp 6088 and 6125), a level up and a job change (pc.cpp 8314, 8356 and 11076), `setquest`, `completequest`, `erasequest` and `changequest`, whether or not the log changed (script.cpp 21073), a kill counted for quests (quest.cpp 837), and `player:refresh_quest_icons()`.
- A new icon checks every player of the map. rAthena runs `questinfo` at `OnInit`, before players come. Here NPC scripts start with the first player, so the players already on the map need the check.
- `npc:show_quest_icon(player, icon, { mark })` is `showevent`. It turns `daily_quest`, `job_quest` and `jumping_poring` into no icon, as `showevent` turns them into `QTYPE_NONE`, and still sends the mark.
- `wa.QuestIcon` gains `click_me`, `daily_quest`, `event3`, `job_quest` and `jumping_poring`. `warg` and `click_me` are both 6, and `warg_face` and `event3` both 8.
- `player:open_quest_log(quest?)` is `open_quest_ui`.

### Calls across zones

- A change, line, effect or sound of an NPC of another zone goes to the NPC's zone as a message, which runs after the current step, in the order sent, as decision 0062's view changes do.
- These messages need no NPC scripts, so they do not start the zone's NPC scripts.
- A line to the world, and music, sounds and marks of a map, go to the other zones as messages too. The world lists its zones (`zone.ZoneLister`).
- A line of a remote NPC to one player of this zone goes to the player at once, with the NPC's name and no speaker.
- `npc:walk_to` of a remote NPC returns true. Its zone checks the walk.
- The world's directory now follows an NPC's name and cell, under its lock, so a remote handle reads them.
- `npc:use_skill`, `npc:show_progress` and `npc:add_quest_icon` raise "the NPC is on another map".

### Protocol

- New events: `EffectRemove`, `NpcProgressBar`, `SoundPlay`, `MusicPlay`, `DigitsShow` with `DigitsKind`, and `QuestLogOpen`.
- `EntityAppear.title`, `Look.npc_view` with `NpcView`, and `QuestIcon.mark`.
- `ChatMessage.color` now also colors NPC lines with a color and server lines of `dispbottom`.

### Client

- Monster and job views of NPCs, the title under the hovered NPC's name, NPC progress bars, effect removal, sounds and music, the digital clock (`digits.ts`), colored server lines, the new quest icons, and the quest log on demand.
- The client has no minimap, so the marks of quest icons and of `viewpointmap` are not drawn.

### Converter

- Every command of B5 and B6 converts, with the limits tools/convert/README.md lists.
- A game ID converts when it is the attached player's (`getcharid(3)`, `playerattached()`, `getattachedrid()`) or an NPC's of `getnpcid(0)` or `getnpcid(0, <name>)`. A variable whose every assignment is a `getnpcid` form stands for that NPC.
- `questinfo`'s condition string parses as rAthena script and becomes a function of the player.
- A line whose view is a monster, by number or name, or a job is placed with the number.
- `sleep` in an event label converts to `R.sleep`, which stops the script without a conversation. These waits failed before at run time with a less clear error.

## Coverage

Script NPC lines of `docs/generated/npc-scripts-<mode>.md`:

| Mode | Lines | Before | After |
|---|---:|---:|---:|
| renewal, not duplicates | 7,067 | 4,557 (64.5%) | 5,371 (76.0%) |
| renewal, with duplicates | 14,865 | 9,617 (64.7%) | 10,986 (73.9%) |
| pre-renewal, not duplicates | 3,754 | 2,827 (75.3%) | 3,032 (80.8%) |
| pre-renewal, with duplicates | 8,205 | 6,460 (78.7%) | 6,985 (85.1%) |

## Breaks

- `npc:say_aloud(text)` now reaches every player who sees the NPC, as `npctalk`'s `AREA`, in place of 9 cells.
- `player:send_message(text, x)` with a second argument that is not a table raises an error. A call such as `player:send_message(s:gsub(...))` passes `gsub`'s count, and needs parentheses.
- `QuestHost.ShowQuestIcon` takes a mark.
- A converted `sleep` in an event label is `R.sleep(conversation, ms)`.

## Alternatives

- **Send a look change for NPCs.** rAthena sends `clif_changelook` for `UNPC_` looks, and the client changes the look in place. The client's `LookChange` only handles players. An appearance again needs no client change and gives the same view.
- **Guess the view's kind in the client.** The RO client draws a class by its ID range. The server knows the monster data and the jobs, so it sends the kind.
- **Check quest icons at once.** rAthena runs `pc_show_questinfo` inside `pc_additem` and the others. Here a condition is Lua, and the change may come during a script's call, so the check waits for the end of the step.
- **Refuse waits in event labels.** 421 converted renewal NPCs have an event label that uses the conversation, and most run with a player. `R.sleep` keeps them and stops only the run without one.

## Details to review

- `classchange` sends a class to the players near and keeps nothing (`clif_class_change`). `npc:set_sprite` keeps it, so players who come later see it too.
- A walking NPC's touch area moves with it. rAthena keeps the `CELL_NPC` flags of the first cell, so its walking touch works only where the old and the new areas meet.
- `unitstopwalk`'s default `USW_NONE` stops without a fixpos packet in rAthena. Both convert to `npc:stop()`, which sends the cell.
- `unitwalk` starts after a 50 ms timer and refuses a second forced walk in rAthena. `npc:walk_to` starts at once.
- `soundeffectall` without a map is centered on the attached player in rAthena, and on the NPC here.
- `playBGM`'s and `soundeffect`'s repeat is sent, and the client plays once, as rAthena notes the repeat does not work.
- `npc:show_progress` does not block the player's actions beyond the conversation's own rules. rAthena sets `WIP_DISABLE_ALL` and blocks moves, attacks and skills, which a conversation also blocks.
- `questinfo` shows new icons to players already on the map. rAthena's `pc_show_questinfo` skips a player whose `qi_display` is shorter than the map's list until its next map entry.
- A wait in an event without a player stops the script, through `R.sleep`. rAthena's script sleeps on a timer.
- API that converted scripts would use and that 0002b does not name, reported and not added: a direction for `npc:move_to` (`movenpc`'s 4th argument), an event at the end of `npc:walk_to` (`unitwalk`'s label), `to` for `remove_effect` (`removespecialeffect` with `SELF`) and for `player:play_sound`, sounds and music on a part of a map, `maps.play_music` for every map (`playBGMall` without a map), a size for `setnpcdisplay`, `unittalk` of an NPC with chat range, and a title for players.
- The protocol changed since v0.1.0. `schema.Version` stays 1 until the next release raises it.

## How to check it worked

- `go test ./server/zone -run 'TestSayAloudTargets|TestPresentTargets|TestNPCLooks|TestNPCWalk|TestNPCStopEarly|TestNPCProgressBar|TestNPCUseSkill|TestQuestInfo|TestShowQuestIconMark|TestPresentChecks|TestPresentAcrossZones'`: who hears each line and sees each effect, sound, music, mark and clock, the looks, walks and stops, the bar, the skill, the quest icons and their refresh, the errors, and the calls on another zone's NPC.
- `go test -race ./server/world -run TestNPCPresentAcrossZones`: a floating NPC renames, titles, moves and makes an NPC of another zone speak, in order before an event, and reads its new name and cell.
- `go test ./tools/convert -run 'TestPresentConvert|TestNPCViewSprites|TestRunConvertedNPCs'`.
- `pnpm vitest run src/viewer/online.test.ts src/viewer/digits.test.ts src/viewer/quests.test.ts` in `client/`.
