# 0043: Quests

- Status: proposed.
- Date: 2026-10-17.
- Code: [server/quest](../../server/quest) (`quest.go`, `log.go`), [server/gamedata/quests.go](../../server/gamedata/quests.go), [server/mod/quests.go](../../server/mod/quests.go), [server/character](../../server/character/character.go), [server/store](../../server/store) (`quests.go`, migration 00033, `queries/quests.sql`), [server/zone](../../server/zone) (`quests.go`, `combat.go`, `zone.go`, `persist.go`, `scripts.go`), [server/modscript](../../server/modscript) (`quests.go`, `prelude.lua`, `runtime.go`, `luaapi/spec.go`), [server/gateway/gateway.go](../../server/gateway/gateway.go), [server/account/export.go](../../server/account/export.go), [server/adminworld/account.go](../../server/adminworld/account.go), [schema/proto/wathena/v1](../../schema/proto/wathena/v1) (`event.proto`, `command.proto`), [client/src/viewer](../../client/src/viewer) (`quests.ts`, `online.ts`, `main.ts`), [client/scripts/quests-check.ts](../../client/scripts/quests-check.ts), [tools/convert](../../tools/convert) (`quests.go`, `npcquest.go`, `rathena.lua`), [tools/wactl/convert.go](../../tools/wactl/convert.go), [mods/examples/hunting-board](../../mods/examples/hunting-board), [e2e/quests_test.go](../../e2e/quests_test.go), and the rAthena fork's `build_quests` in `tools/wathena/extract.py` (fork commit `60bed4ccf`).
- Phase 7 step 6a (strategy section 15, "Phase 7 in full"), and proposal 0002, review decision 45.
- Lua API: the calls and events of review decision 45, and nothing else.
- Protocol: `QuestList`, `QuestUpdate`, `QuestRemove`, `QuestIcon` and the command `QuestActive`, recorded in proposal 0001.

## Context

Quests are a tier 1 system of strategy section 11.3 that phases 5 and 6 did not build. rAthena's job change quests and many town NPCs use `setquest`, `checkquest` and the other quest commands, so they did not convert. Review decision 45 fixed the shape: records from `quest_db`, the player's quest calls, three events, a quest window with icons over NPCs, and the converter's commands. The open questions were:

- How records are named, since `quest_db.yml` has IDs and titles but no names.
- What the decision's "item objectives" are.
- How to keep rAthena's `checkquest` numbers, which count a quest without a time limit as timed out, with an API that does not.
- Which kills count, and for whom.

## Decision

### Records

- A fork function, `build_quests`, reads `quest_db.yml` with its imports for each mode as `QuestDatabase::parseBodyNode` does, and writes `quests.json`: 4,821 quests in renewal and 2,879 in pre-renewal, none refused.
- `waserver` converts it into core's `data/quests.yml` when it starts, with the item and NPC scripts. A core record's name is its quest ID as text, such as `"7104"`, so converted scripts and mods name core's quests by rAthena's IDs. Mods add records named with their ID, with IDs from 1,000,000, and change core's by ID.
- Fields: `title`, `time_limit`, `targets` and `drops`. `server/quest` checks every field, and `server/gamedata` resolves monsters, items and the names of `wa.races`, `wa.sizes` and `wa.elements`.
- The decision's "item objectives" are rAthena's quest `Drops`: items each kill gives while the quest is in the log, straight to the inventory. rAthena's `quest_db` has no other item objective.
- rAthena's element filter, map filter, map name and monster list stay beside the level, race and size filters the decision names, since core's quests use them.

### Time limits

- A limit with "+" is a duration from the quest's start. One without ends at a time of day on the server's clock, some days later, or on a weekday, with `quest_time`'s rules for a time that has passed today.
- The end is an instant, saved as `timestamptz`. rAthena keeps seconds since 1970 in an unsigned 32-bit number.
- Nothing happens when a limit passes. Scripts check it, as in rAthena.

### The quest log

- `server/quest` holds the log rules of `quest_add`, `quest_change`, `quest_delete`, `quest_update_status` and `quest_check`. A log holds each quest once, in the order the character took it. rAthena keeps complete quests at the end of its array; wAthena keeps the order, and the window shows only active and inactive quests.
- An entry whose record is gone stays saved and hidden, and comes back with its mod, as mod variables and IDs do. rAthena drops such entries at login.
- The log saves with the character's save and commit, in `character_quests`, as a diff. Deleting a character or an account deletes its rows. The data export lists each character's quests with their name, title, state, end and counts, in format version 1.

### Kills

- `mob_dead` calls `quest_update_objective` on a monster's final death, after the rewards, whatever its rewards. wAthena does the same in `die`: a reborn monster does not count yet.
- Without a party, the killer counts the kill. With a party, every member on the map within `AREA_SIZE` (14 cells) of the monster counts it, dead or not, and a killer farther away does not, as `quest_update_objective_sub` and `map_foreachinallrange`.
- A quest that is not complete counts the kill for every objective it passes, up to the goal. Inactive quests count too, as in rAthena. Each of its drops rolls once per kill, with rAthena's `rnd_chance` out of 10,000, and goes to the inventory through transfers with the new action `quest`. A drop that does not fit is lost, and the player sees "You cannot carry more items.", as `clif_additem`'s failure.
- The map filter compares the player's map, as rAthena's `sd->m`.

### Lua

- The calls of review decision 45. A quest is an ID or a record name. An unknown quest raises an error.
- `quest_timed_out` is false for a quest without a limit, which is what a mod author expects. The converter keeps rAthena's numbers: `R.checkquest` takes whether the quest has a limit, which the converter knows from the mode's records, and counts a quest without one as timed out for `PLAYTIME` and `HUNTING`, as rAthena does. Pre-renewal's quests 7104, 7105, 7107 and 7110 have no limit and are checked with `PLAYTIME` in core's scripts.
- The events carry `{ player, quest, id }`, and `quest_progress` adds `objective`, `count` and `goal`. They run in mod, NPC and event contexts, so an NPC can update its icon when a kill counts.
- `npc:show_quest_icon(player, icon)` keeps the icon per player and per NPC in the zone, sends it when the player sees the NPC, and sends it again when the NPC comes back into view. A map change drops it.

### The client

- `QuestList` at login and after each map change, then `QuestUpdate` and `QuestRemove`, and the command `QuestActive`, as rAthena's quest packets.
- The quest window lists active quests, then inactive ones, with a timer that counts down from the time left the server sent, the objectives with their counts and level ranges, and a button that marks a quest active or inactive. Alt+U or the Quests button opens it. It works with the mouse and with touch.
- Icons draw as badges over the NPC's head: "!" or "?" in a color per kind. The asset pack has no quest icon art.

### The converter

- `setquest`, `completequest`, `erasequest`, `changequest`, `checkquest`, `isbegin_quest` and `showevent` convert. A fixed ID the mode lacks converts to what rAthena does with it. `showevent` with a minimap mark color, with a computed icon, and every command for another character do not convert.
- Script NPCs that convert fully, of the extract's folders:

| Mode | Kind | Before | After |
|---|---|---:|---:|
| renewal | script | 927 | 1,023 |
| renewal | script (duplicate) | 903 | 945 |
| renewal | job change NPCs | 80 | 175 |
| pre-renewal | script | 754 | 790 |
| pre-renewal | script (duplicate) | 175 | 175 |
| pre-renewal | job change NPCs | 63 | 99 |

- Renewal places 1,130 script NPCs, up from 1,039, and pre-renewal 851, up from 813. `TestRunConvertedNPCs` starts and clicks every one of them without an error.

## Tests and checks

- `server/quest`: `TestLimitEnd` for each kind of limit, `TestParseLimitErrors`, `TestBuild`, `TestTargetCounts` for each filter, `TestLog` for the log rules and rAthena's `quest_check`, and `TestKillDrops`.
- `server/store`: `TestCharacterQuests` saves, changes, reorders and removes quests, and reads them in the staff view. `TestDeleteAccountEveryTable` checks that a deleted account's quests go and another account's stay.
- `server/zone`: `TestQuestLog` (the host calls, the client's events, the active toggle, a time limit on the server clock, and the snapshot), `TestQuestKills` (counts up to the goal, drops, and the filters), `TestQuestPartyKills` (members in range, a dead member, a member out of range, a killer out of range, and a kill without a party), and `TestQuestIcons`.
- `server/modscript`: `TestQuestCalls`, `TestQuestReadsNeedGameRead` and `TestQuestEvents`.
- `server/adminworld`: `TestAccountPageExport` checks the quests in the export, with a quest whose record is gone.
- `server/cmd/waserver`: `TestLoadSampleMod` loads core's quests and the Hunting Board's.
- `tools/convert`: `TestQuestsConvert` builds every record of both modes as the server does, `TestQuestCommandsConvert` and `TestRunQuestCommands` check the converted commands and rAthena's numbers, `TestQuestReportsCurrent` checks the reports, and `TestNPCGolden` keeps Dancer Kim.
- `wactl mod test`: the Hunting Board's 6 cases take the quest, kill Porings with their drops, collect the reward, count the kills left, and let the hour run out in Prontera, and take the quest and count a kill on a generated town's square with a `wactl` built with `-tags procedural`.
- e2e, with PostgreSQL: `TestQuestFromCoreNPC`. A character at Dancer Kim's step of the Wanderer job change takes her converted quest, kills a Dummy, logs out, comes back with its count and end saved, kills a second Dummy, and turns it in. The fixture moves Dancer Kim to the start map and makes quest 2221 count 2 Dummies.
- The browser check `quests`: a desktop page with the mouse sees the Hunt Master's "!" icon, takes the quest, reads it in the quest window with its timer and objective, kills three Dummies, sees "Dummy 3/3" and the "?" icon, marks the quest inactive and active again, and turns it in. A phone page with touch sees the icon, takes the quest, opens the window through the menu, and marks the quest inactive.

## Lua API

The calls and events of review decision 45, and nothing else. Proposal 0002, next step 31, has the details to review.

## Left

- `questinfo` and `setquestinfo`, which 32 renewal scripts use, and `open_quest_ui`. They need API.
- Quest art in the client: the icons are placeholders.
- Quest templates for procedural worlds (step 6b).
- Achievements, which share the kill counting.

## Review items

These need a decision before they are built, since they would be new API or change approved behavior:

- A Lua way to list a player's quests, or to read a quest record's title, objectives and limit.
- A minimap mark for `showevent`'s color.
- `questinfo`, `setquestinfo` and `open_quest_ui`.
- Options for the quest events, such as a quest's name.
- Quest calls for another character, which `setquest` and the other commands allow with a character ID.
