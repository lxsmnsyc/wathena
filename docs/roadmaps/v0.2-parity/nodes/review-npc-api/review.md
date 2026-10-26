# Review: open NPC API and converter items

Review of the items in brief.md "Steps", checked against the code at b9a32ba. Counts come from docs/generated/npc-scripts-renewal.md and npc-scripts-pre-renewal.md (script NPCs that are not duplicates). Those reports leave out the battleground, custom, guild, guild2, guild3, instances and mobs folders, so where a command lives only there, the count is of files in the rAthena fork's `npc/` tree (`grep -rlw`, test folder left out), marked "files".

States: **built** (the code does it), **settled** (a later decision or node already decides it), **open**.
Recommendations: **keep** (accept as built), **accept** (with the form to build), **defer** (with a milestone), **refuse**.

## Summary

| # | Item | Recommendation | Node |
|---|---|---|---|
| A1 | `fire_event` / `fire_npc_event` results for unstarted zones | keep | none |
| A2 | A broadcast starts every unstarted zone | accept: start only zones whose source names the label | npc-api-events |
| A3 | `awake` | accept `npc:wake()`, build with sieges | sieges-war |
| A4 | Waits in an event without a player (`R.sleep`) | accept `timers.wait(milliseconds)` | npc-api-timers |
| A5 | `OnInterIfInitOnce` | accept: converter, once per server start | npc-api-events |
| A6 | `unloadnpc` of an NPC with duplicates | accept: converter removes each record | npc-api-events |
| B1 | Forms of the review decision 59 calls | keep | none |
| B2 | `maps.set_cells`' `send` | keep | none |
| B3 | `OnNPCKillEvent` | defer: later | none |
| B4 | `killedrid`, `killedgid` | accept `event.monster_id`, `event.monster_game_id` | npc-api-events |
| B5 | `attachrid` without force | defer: later | none |
| B6 | Reads of another map's player (items, variables) | defer: later | none |
| B7 | `areamobuseskill` options | accept `use_skill` options, build with instances | instances-content |
| B8 | `unitexists` | accept: converter over `maps.find_monster` | npc-api-events |
| B9 | Monster emotion | accept `monster:show_emotion(emotion)` | npc-api-events |
| B10 | Statuses on a monster of another map | defer: later | none |
| B11 | `unittalk` of another game ID with `bc_self` | accept `to = "self"` | npc-api-events |
| B12 | Directory lag, 2 s logout wait, nil remote results, `OnPCLoadMapEvent` | keep | none |
| C1 | NPC timer labels due while a map sleeps | accept `{ clock = "server" }` on timers | npc-api-timers |
| C2 | Player timers (`addtimer`, `deltimer`) | accept `player:add_timer`, `player:remove_timer` | npc-api-timers |
| C3 | NPC timer attached to a player | accept: converter | npc-api-timers |
| C4 | Timer commands on another NPC | accept: converter | npc-api-timers |
| C5 | `schedules.yearly` | defer: modding | none |
| C6 | `getnpctimer(0)`, `startnpctimer` resume | built (0064) | none |
| D1 | `schema.Version` for v0.2.0 | settled | release-v0.2.0 |
| D2 | Staff levels by role | keep; a per-role level defers to later | none |
| D3 | API names changed from 0002b | keep | none |
| D4 | `maps.can_enter` | accept `maps.can_enter(map, job)` | sieges-war |
| D5 | Read a limit without `player.expand_limits` (step 18) | defer: later | none |
| D6 | Decision 0063 leftovers: `to` on `remove_effect` | accept | npc-api-events |
| E1 | `R.touch_one` with a dialog and a waiting player | accept `npc:fire_touch(player)` | npc-api-events |
| E2 | Click fallback for touch areas without a touch label | accept: converter | converter-names |
| E3 | `jobname` | accept `jobs.name(job)` | converter-commands |
| E4 | `percentheal -100` | accept: -100 kills, as rAthena | item-effects |
| E5 | Answer range of NPCs without a view | accept: no range check for `sprite: none` | npc-api-events |
| E6 | Item skills in the client's skill list | accept: `SkillInfo.granted_level` | item-skill-use |
| E7 | `get_skill_level` with item and temporary levels | accept: read `pc_checkskill`'s level | item-skill-use |
| F1 | Variable names that start with a digit | accept: `wa.variables` takes them | converter-names |
| F2 | Assignment used as a value | accept: inline function rule | converter-names |
| F3 | `getd` / `setd` of computed names | accept: tables and run-time declarations | converter-names |
| F4 | `strnpcinfo(0)` | built (0062, 0064) | none |
| G1 | Chat channel commands (step 12) | defer: later | none |
| G2 | Party result codes, `$@party_create_id` (step 13) | defer: later | none |
| G3 | Guild calls (step 14) | defer: later; siege ones to sieges-war | sieges-war |
| G4 | `countbound`, `openstorage2` modes, `getitembound3/4`, `guildstoragedelitem` (step 15) | defer: later | none |
| G5 | `delitem2` exact copies | accept `take_item`'s `refine`, `cards` | converter-commands |
| G6 | `npcshopitem`, `npcshopattach` (step 16) | defer: later | none |
| G7 | Barter of refined items and more than 5 kinds | accept: data fields | converter-commands |
| G8 | Kafra points paying part of a cash shop price | defer to the windows node | ui-windows |
| G9 | `mail` arrays, `searchstores` on another map | defer: later | none |
| G10 | Computed `callshop` | accept: converter | converter-commands |
| G11 | `freeloop` | accept: converts to nothing | converter-commands |
| G12 | `isequippedcnt` | accept `player:count_equipped(item)` | converter-commands |
| G13 | `getequiprandomoption` | accept: converter over `random_options` | converter-commands |
| G14 | `mesitemlink` without brackets, with a name or arguments | accept: converter | converter-commands |
| G15 | `getunitdata`, `getunittype`, `setunitdata` of game IDs | accept: converter over handle fields | npc-api-events, converter-commands |
| G16 | `$@partymember*` outside the function | accept: converter | converter-commands |
| G17 | `checkweight` lists, computed monster labels, `areamonster` rectangle, `rentitem` | built (0064, 0065) | none |
| G18 | Dragon colors, Madogear, warg (step 11) | defer to third classes | third-classes |
| H1 | Long string compares hold a zone up to 30 s | defer: modding, new node | lua-cost-accounting (new) |
| H2 | CPU time limit on the Lua thread | refuse | none |

Totals: 35 accept, 15 defer, 1 refuse, 10 already built or settled (keep).

## A. Events between NPCs

### A1. `fire_event` and `fire_npc_event` results for unstarted zones

- Source: decisions/0062-npc-api-group-1.md:113, :62; proposal 0002:1625.
- State: built as documented. `npc:fire_event` returns true for an NPC whose zone has not started (server/modscript/luaapi/spec.go:738). `world.fire_npc_event` counts only started zones (spec.go:275).
- Evidence of need: no rAthena script reads `donpcevent`'s result (`grep -E '(if|set|=)\s*\(?\s*donpcevent'` finds only a comment).
- Recommendation: keep. A2 makes the unstarted case closer for free.

### A2. A broadcast starts every unstarted zone

- Source: decisions/0062-npc-api-group-1.md:115; proposal 0002:1626.
- State: open. Player events already scan an unstarted zone's NPC sources for the event name before they start it (server/zone/playerworld.go:545 `mentions`). `world.fire_npc_event` does not.
- Options:
  1. Keep: a floating NPC's `OnInit` that sends `::OnX` starts most towns at boot. Cost: none now; memory and start time per Lua state (decision 0056).
  2. Reuse `mentions` for `npc_event` labels: start a zone only when one of its NPC sources names the label (case-insensitive). Cost: small, in server/zone. No API change.
  3. Start every zone at boot. Refused by decision 0062.
- Unblocks: no script; it is a cost fix.
- Recommendation: accept option 2. `fire_event` then returns true for an unstarted zone only when its source names the label, and `fire_npc_event` still counts started zones only. Node: npc-api-events.

### A3. `awake`

- Source: decisions/0062-npc-api-group-1.md:118; 0002b B3 table.
- State: open. No converter mapping.
- Users: 1 official file, guild2/agit_main_se.txt:601 (sieges), plus 5 custom files. 0 rows in the extract's reports.
- Options:
  1. `npc:wake()`: ends the waits of the NPC's handlers at once, as `awake` ends `sleep` timers. Needs A4, since a wait without a player must exist first. Cost: small once A4 exists.
  2. Refuse. Breaks the siege script.
- Recommendation: accept option 1 with this form. Build with the siege that needs it. Node: sieges-war.

### A4. Waits in an event without a player (`R.sleep`)

- Source: decisions/0063-npc-api-group-2.md:138; proposal 0002:1635.
- State: open. `R.sleep(nil, ms)` raises "a wait in an event without a player" (tools/convert/rathena.lua:41). The converter writes `R.sleep` for every `sleep` and `sleep2` in an event label (tools/convert/npcexpr.go:1116). The failure shows only at run time, so the reports do not count it. `sleep2` is in 184 renewal scripts and `sleep` in 19; the labels started by `OnInit`, clock and timer labels or `donpcevent` run without a player.
- Options:
  1. Keep: the script stops, with an error in the log. Cost: none; behavior differs from rAthena.
  2. `timers.wait(milliseconds)`: the runtime runs `npc_event`, timer and schedule handlers of NPC contexts in a Lua thread, so a handler can wait on the zone clock without a conversation. `R.sleep(nil, ms)` calls it. Cost: medium, in server/modscript (handlers become resumable threads).
  3. Split each label into `timers.after` continuations in the converter. Cost: high; loops around `sleep` do not split.
- Recommendation: accept option 2. Form: `timers.wait(milliseconds)`, 0 to 10 minutes, as `conversation:wait`. It works in a handler of an NPC context that runs without a conversation, and raises elsewhere. With a conversation, `conversation:wait` stays the call. With C1, the wait follows the timer's clock. Node: npc-api-timers.

### A5. `OnInterIfInitOnce`

- Source: proposal 0002:1616.
- State: open. The label does not convert ("event oninterifinitonce", 1 renewal script).
- Users: re/quests/quests_14_3_bis.txt:114, which only fires `donpcevent` when a `$` variable is set. The script that also needed `setbattleflag` (events/nguild/nguild_ev_agit.txt) is a siege event.
- Recommendation: accept. The converter runs the label as `OnInit` once per server start, guarded by a temporary server boolean, so a mod reload does not run it again, as rAthena runs it when the map server first reaches the character server. No API change. Node: npc-api-events.

### A6. `unloadnpc` of an NPC with duplicates (added)

- Source: decisions/0062-npc-api-group-1.md:117.
- State: open. "an NPC with duplicates, which rAthena unloads with it": 5 renewal, 1 pre-renewal.
- Recommendation: accept. The converter knows each duplicate's record, so it removes each one with `world.get_npc(name):remove()`. No API change. Node: npc-api-events.

## B. Players and monsters across zones (decision 0066)

### B1. Forms of the calls review decision 59 named

- Source: decisions/0066-npc-api-follow-ups.md:113.
- State: built: `npc:set_sprite`'s `keep` and `to`, `npc:stop`'s `send_cell`, `player:play_sound`'s `to = "area"`, `npc:move_to(x, y, direction)`, `npc:walk_to`'s `label`, `world.play_music`, `maps.play_music`'s `rectangle`, `equipped:destroy`'s `refine_failure`, `monster.game_id`, `maps.find_monster`, `party:warp`'s targets and options, `"random"` for `maps.warp_players` (spec.go:253, 591, 684, 745-756, 784, 915, 1165-1189).
- Recommendation: keep.

### B2. `maps.set_cells`' `send`

- Source: decisions/0066-npc-api-follow-ups.md:114.
- State: built (spec.go:1182, `wa.SetCellsOptions.send`). The converted `setwall` and `delwall` use it.
- Recommendation: keep.

### B3. `OnNPCKillEvent`

- Source: decisions/0066-npc-api-follow-ups.md:115.
- State: open. The converter refuses the label with a reason.
- Users: 0 official files; 2 custom (custom/quests/hunting_missions.txt, questboard.txt).
- Options: an option on the `death` event that hears monster deaths by players on every map in NPC contexts; or refuse.
- Recommendation: defer to later. No in-scope script needs it.

### B4. `killedrid` and `killedgid`

- Source: decisions/0066-npc-api-follow-ups.md:115; decisions/0064-npc-api-group-3.md:146.
- State: open. `wa.NpcEvent` has `name`, `player` and `conversation` (spec.go:1090).
- Users: in `OnMyMobDead` labels, where rAthena's `mob_dead` sets them on the killer before the label runs: re/mobs/dungeons/tur_dun.txt:74 (mobs folder), re/instances/CharlestonCrisis.txt:2187, re/instances/TempleOfDemonGod.txt:213. other/poring_war.txt uses it in `OnPCKillEvent`, which stays refused with PvP.
- Options:
  1. Fields on the label's event: `event.monster_id` (the dead monster's ID, as `monsters.id`) and `event.monster_game_id`. Cost: small; the label's event already carries the death's counts (decision 0066).
  2. A player field that keeps the last kill, as rAthena's param. Cost: more state, and it lags across zones.
- Recommendation: accept option 1. The converter maps `killedrid` and `killedgid` inside a monster label's code to them, and refuses them elsewhere. Node: npc-api-events.

### B5. `attachrid` without force

- Source: decisions/0066-npc-api-follow-ups.md:115.
- State: open. `attachrid` converts with rAthena's default `force = true` (script.cpp 13515).
- Users: no official script passes a second argument; only custom/woe_controller.txt does, with `true`.
- Recommendation: defer to later.

### B6. Reads of another map's player (items, variables)

- Source: decisions/0066-npc-api-follow-ups.md:115, :117.
- State: open. A remote handle reads the directory's fields; other reads raise (decision 0066).
- Options: copy inventories and variables into the directory each step (memory and work per player per step); an asynchronous read with a callback (new API shape for converted code); keep the error.
- Unblocks: unknown; the failures are at run time. Instance scripts, the main users of `attachrid`, act on players of the instance map, which is the same zone.
- Recommendation: defer to later. Revisit if the behavior harness (npc-behavior-harness) finds a script that reads a remote player.

### B7. `areamobuseskill` options

- Source: decisions/0066-npc-api-follow-ups.md:115; tools/convert/README.md:136.
- State: open. The converter refuses it for its cast time, emotion and target type (tools/convert/npcmaps.go:707).
- Users: 2 files, both instances (instances/SealedShrine.txt, re/instances/SaraMemory.txt).
- Recommendation: accept this form, built with the dungeons: `monster:use_skill(skill, level, target, { cast_milliseconds, can_cancel, emotion })`. The converter resolves rAthena's target type to the target handle. Node: instances-content.

### B8. `unitexists`

- Source: decisions/0066-npc-api-follow-ups.md:115; proposal 0002:1614.
- State: open. 2 renewal scripts; 13 more files in instances.
- Recommendation: accept, converter only. A monster's game ID maps to `maps.find_monster(id) ~= nil`, an NPC's of `getnpcid` to its handle's `exists`, and the attached player's to true. No API change. Node: npc-api-events.

### B9. Monster emotion

- Source: decisions/0066-npc-api-follow-ups.md:115.
- State: open. `emotion` with a game ID other than the player's or an NPC's: 2 renewal scripts.
- Recommendation: accept `monster:show_emotion(emotion)`, as `player:show_emotion`, local and remote (a message, as the other remote monster calls). Node: npc-api-events.

### B10. Statuses on a monster of another map

- Source: decisions/0066-npc-api-follow-ups.md:121.
- State: open. `sc_start` on a remote monster raises.
- Users: instance scripts, whose monsters are on the instance map.
- Recommendation: defer to later.

### B11. `unittalk` of another game ID with `bc_self`

- Source: decisions/0066-npc-api-follow-ups.md:115 (via npc-api-events brief); report row "unittalk", 7 renewal scripts.
- State: open. The converter refuses a flag with another game ID (tools/convert/npcpresent.go:438). rAthena refuses `bc_self` for a unit that is not a player (script.cpp 20229), so only players need it.
- Recommendation: accept `"self"` as a value of `player:say_aloud`'s `to`: the line shows to that player alone. It works on a remote handle, since it is a plain value. Node: npc-api-events.

### B12. Directory lag, the 2-second logout wait, nil remote results, `OnPCLoadMapEvent`

- Source: decisions/0066-npc-api-follow-ups.md:116, :118, :119, :120.
- State: built as documented. Synchronous reads were refused for deadlocks (decisions 0065 and 0066).
- Recommendation: keep. `OnPCLoadMapEvent` stays converted and idle until a map has `loadevent`; no map has it.

## C. Timers and the clock

### C1. NPC timer labels due while a map sleeps

- Source: decisions/0064-npc-api-group-3.md:100, :130; proposal 0002:1609, :1646.
- State: built as catch-up: the tick follows the server clock, and labels due while the map slept run when it wakes. A sleeping zone already wakes for schedules (server/zone/zone.go:909).
- Options:
  1. Keep catch-up. Announcements and server variable changes of an empty map wait for its next player.
  2. `timers.after` and `timers.every` take `{ clock = "server" }`: the timer follows the server clock and wakes a sleeping zone, as schedules do. `R.npc_timer` uses it. Cost: medium, in server/modscript and server/zone; a woken zone runs only the timer.
  3. Wake zones for every NPC context's timer. Changes `wa.timers` for mods too.
- Unblocks: no new script; it fixes behavior of timer loops started without a player (`initnpctimer` is in 471 renewal scripts).
- Recommendation: accept option 2. Form: `wa.TimerOptions { clock = "zone" | "server" }`, `"zone"` by default. Node: npc-api-timers.

### C2. Player timers (`addtimer`, `deltimer`)

- Source: proposal 0002:1610.
- State: open.
- Users: 1 official file, re/instances/GhostPalace.txt; 3 custom. The jobs files only mention it in comments.
- Recommendation: accept. Form: `player:add_timer(milliseconds, label)` and `player:remove_timer(label)`. `label` is `"OnX"` of the context's NPC or `"<NPC>::OnX"`. The timer belongs to the player: it moves with the player between zones and ends at logout. When it fires, the label runs as `npc:fire_event(label, { player })`. Node: npc-api-timers.

### C3. NPC timer attached to a player

- Source: proposal 0002:1610; report "initnpctimer: a timer attached to the player", 2 renewal, 2 pre-renewal; `attachnpctimer` in warps/dungeons/juperos.txt.
- Recommendation: accept, converter only: `R.npc_timer` keeps the attached player and runs its labels through `fire_event` with the player. Node: npc-api-timers.

### C4. Timer commands on another NPC

- Source: proposal 0002 0002b table; report rows `initnpctimer` 2, `stopnpctimer` 1, `setnpctimer` 1 renewal, `setnpctimer` 1 pre-renewal.
- Recommendation: accept, converter only: an NPC that uses `R.npc_timer` gets hidden `npc_event` handlers for start, stop and set, and another NPC fires them. They run after the current step, as every event between NPCs (review decision 58). No API change. Node: npc-api-timers.

### C5. `schedules.yearly`

- Source: decisions/0064-npc-api-group-3.md:142; 0002b line 224.
- State: open. No script in the fork uses `OnDayMMDD` (grep finds 0 files).
- Recommendation: defer to the modding milestone. npc-api-timers can drop it.

### C6. `getnpctimer(0)` and `startnpctimer` resume

- Source: proposal 0002:1610.
- State: built by decision 0064 (`R.npc_timer` keeps the tick on the server clock).
- Recommendation: none needed.

## D. Staff, protocol and maps

### D1. `schema.Version` for v0.2.0

- Source: decisions/0063-npc-api-group-2.md:140, 0064:159, 0065:121.
- State: settled. schema/protocol.go:7 is 1, and release-v0.2.0's brief raises it (nodes/release-v0.2.0/brief.md:85).
- Recommendation: none needed.

### D2. Staff levels by role

- Source: decisions/0064-npc-api-group-3.md:154.
- State: built: 99 for owner and admin, 10 for other staff, 0 for players (spec.go:521).
- Evidence: official scripts compare `getgmlevel()` with 80, 90 and 99, or test it as a boolean. 99 and 10 give rAthena's answer for its default groups.
- Options: keep; or a `level` field per role in `staff.roles` (config, not Lua API).
- Recommendation: keep for v0.2.0. Defer the per-role level to later.

### D3. API names changed from proposal 0002b

- Source: decisions/0064-npc-api-group-3.md:135-139.
- State: built: `equipped:add_refine` (the field `refine` would hide `equipped:refine`), `items.refine_cost`'s refine argument, `clock.format`'s time argument, the block `equip` in place of `party`.
- Recommendation: keep.

### D4. `maps.can_enter`

- Source: decisions/0065-npc-api-group-4.md:139; decisions/0064-npc-api-group-3.md:141.
- State: open. `jobcanentermap` refused in 3 renewal scripts (needs WoE:TE map flags). Already in sieges-war's goal (nodes/sieges-war/brief.md:47).
- Recommendation: accept this form: `maps.can_enter(map, job)` returns a boolean, as `jobcanentermap` and `pc_job_can_entermap` with job_noenter_map.txt. `game.read`. The converter passes `player.job` when the command has no job. Node: sieges-war.

### D5. Read a limit without `player.expand_limits` (proposal step 18)

- Source: proposal 0002:1294 (GOAL coverage maps it here).
- State: open. `player:expand(limit, 0)` needs the permission.
- Users: none; the fork has no `getinventorysize`.
- Recommendation: defer to later.

### D6. Decision 0063 leftovers (added)

- Source: decisions/0063-npc-api-group-2.md:139.
- State: decision 0066 built the direction, the walk label, `to` for `player:play_sound`, sounds and music of part of a map, and music for every map. Left: `to` for `remove_effect` (`removespecialeffect` with `SELF`, 1 renewal script), a size for `setnpcdisplay` and a title for players (their report rows convert fully), and `unittalk` of an NPC (no failing row).
- Recommendation: accept `npc:remove_effect(effect, { to })` and `player:remove_effect(effect, { to })`, with `to` as `show_effect`'s. Drop the rest: no script fails on them. Node: npc-api-events.

## E. Touch areas, conversations and skills (decisions 0052, 0053)

### E1. `R.touch_one` with a dialog and a waiting player

- Source: decisions/0052-classic-mode-part-b.md:27, :162; proposal 0002:1587.
- State: open. When the player of a dialog leaves, `R.touch_one` waits for the next player who enters (tools/convert/rathena.lua:1825). rAthena runs `OnTouch_` for the next player already in the area (`npc_touchnext_areanpc`).
- Options:
  1. Keep. A player standing in a quest notice's area gets nothing until it steps out and in.
  2. `npc:fire_touch(player)`: runs the NPC's `touch` handlers for a player who stands in its touch area, in a conversation, waiting while the player talks, as a step in would. Returns false when the player is not in the area. `R.touch_one` calls it for the next player. Cost: small, in server/zone and server/modscript.
  3. A general "start a conversation with a player" call. Larger, and opens dialogs a player did not ask for.
- Unblocks: no new placement; fixes behavior of `OnTouch_` dialogs (627 renewal scripts use `OnTouch_`).
- Recommendation: accept option 2 with this form. Node: npc-api-events.

### E2. Click fallback for touch areas without a touch label

- Source: decisions/0052-classic-mode-part-b.md:163; proposal 0002:1588.
- State: open. rAthena clicks a script NPC whose area a player enters when it has no `OnTouch` label and the player is not talking (npc.cpp 1943-1949). The converter gives a touch area only to NPCs with a touch label.
- Users: about 500 script lines that are not duplicates and about 1,600 duplicate lines in the fork's tree have a span and no `OnTouch` (a heuristic count, both modes and all folders), for example warp NPCs that ask before they move the player.
- Recommendation: accept, converter only: such an NPC gets its `touch` field and a `touch` handler that runs its click handler. rAthena skips the click while the player talks, and wAthena queues touches; the handler returns at once when the touch waited (see Gotchas). Node: converter-names (it is placement and parsing work in tools/convert; no API).

### E3. `jobname`

- Source: decisions/0053-skill-command.md:105.
- State: open. 2 renewal scripts: the Thief Guide (re/jobs/1-1/thief.txt:77), which blocks the Thief job change, and re/jobs/3-2/sura.txt.
- Recommendation: accept `jobs.name(job)`: the job's name as players see it (its record's `display_name`, rAthena's job names for core), as `items.name`. `game.read`. Node: converter-commands.

### E4. `percentheal -100`

- Source: decisions/0052-classic-mode-part-b.md:48, :165; review decision 56.
- State: built as review decision 56 says: HP never goes below 1 (spec.go:565).
- Users: 8 official NPC files (okolnir, quests_13_1, quests_ayothaya, eye_of_hellion, megingard_seal, divorce, umbala, jawaii) and the Poison Bottle (db/re and db/pre-re item_db_usable.yml).
- Options:
  1. Keep 1 HP. Differs from rAthena; the behavior harness will flag it.
  2. -100 kills, as rAthena's `status_percent_change` sets its kill flag at -100. Other negative values still stop at 1 HP. Cost: small, in `item.PercentLoss` and the zone. Changes review decision 56's rule.
- Recommendation: accept option 2, since the goal is 1:1 parity. Node: item-effects (the Poison Bottle is its visible case).

### E5. Answer range of NPCs without a view

- Source: decisions/0052-classic-mode-part-b.md:166.
- State: open, in a new form. Since decision 0062, view -1 converts to `sprite: none`, not `HIDDEN_NPC` (tools/convert/npc.go:575). rAthena lets a player answer a class-less NPC from anywhere (`npc_checknear`, npc.cpp 2155); `HIDDEN_NPC` (111, 139) keeps the 15-cell check. wAthena checks 15 cells for every answer (server/zone/npcs.go:344).
- Users: 29 lines in the fork have a class-less or hidden NPC with a span over 15 cells; the dialog ones matter.
- Recommendation: accept: an answer to an NPC with `sprite: none` skips the range check. A rule of `sprite: none`, written in the API docs; no call changes. Node: npc-api-events.

### E6. Item skills in the client's skill list

- Source: decisions/0053-skill-command.md:106.
- State: open for item skills. rAthena's `skill` command with a temporary kind sends no skill list (pc.cpp 5466), and the next `status_calc_pc` clears the level, so temporary levels match already. Item skills show in rAthena after `status_calc_pc`. wAthena's `SkillInfo.level` is the learned level only (schema/proto/wathena/v1/event.proto:629).
- Recommendation: accept a protocol field `SkillInfo.granted_level`, the level that equipment gives above the learned one. The client shows the higher level and lets the player cast it; `upgradable` stays on the learned level. Node: item-skill-use.

### E7. `get_skill_level` with item and temporary levels

- Source: decisions/0053-skill-command.md:107.
- State: open. The host reads learned levels (server/zone/itemscripts.go:244-250), while the zone's own `skillLevel` counts temporary, item and Plagiarism levels (server/zone/skills.go:44).
- Users: `getskilllv` is in 131 renewal and 98 pre-renewal scripts.
- Recommendation: accept: `player:get_skill_level` returns the zone's `skillLevel`, as `getskilllv` reads `pc_checkskill`. No form change; the doc changes. Node: item-skill-use, with E6.

## F. Converter: names and expressions (proposal step 40)

### F1. Variable names that start with a digit

- Source: proposal 0002:1615.
- State: open. Every "is not a valid name" in the reports is a name that starts with a digit once its prefix is gone, such as `$@3rdgc_room02` (tools/convert/npclua.go:615). It is the first blocker of 27 renewal scripts (decision 0065:113).
- Options:
  1. `wa.variables` accepts a leading digit: 1 to 64 letters, digits and `_` (server/modscript/variables.go:78). Names are strings, so Lua identifiers do not matter; the converter already makes Lua locals with `luaName`. Cost: one rule and its doc. An API amendment.
  2. The converter adds a prefix. Cost: none in the API; a stored name that differs from rAthena's, and a collision if a script also has the prefixed name.
- Recommendation: accept option 1. Node: converter-names (it must take server/modscript/variables.go and spec.go as shared paths; see Gotchas).

### F2. Assignment used as a value

- Source: proposal 0002:1615.
- State: open: "assignment" and "assignment in an expression", 8 renewal and 5 pre-renewal scripts.
- Recommendation: accept this rule: the converter writes the assignment as an inline function that sets the variable and returns its new value, `(function() x = e; return x end)()`, in the place of the expression. Lua keeps left-to-right order, so side effects run where rAthena runs them, and a Lua local is set as an upvalue. Node: converter-names.

### F3. `getd` and `setd` of computed names

- Source: proposal 0002:1613.
- State: open. `getd` converts in 20 of 36 renewal scripts, `setd` in 15 of 27. Reasons: a computed `.@` or `.` name (Lua locals), a name with no fixed start, a fixed start that does not fix the scope.
- Recommendation: accept these rules, converter only:
  - A function that uses a computed `.@` name keeps all its `.@` variables in one Lua table for the call.
  - A script that uses a computed `.` name makes all its `.` variables NPC variables, as `getvariableofnpc` does now.
  - A name with no fixed start goes through a run-time helper that reads the prefix and declares the variable then. A declaration repeated in a handler is allowed (variables.go:89), and core has no count limit (decision 0062). A mod that converts its own scripts keeps the 256 limit, and computed names count toward it.
  - Node: converter-names.

### F4. `strnpcinfo(0)`

- Source: proposal 0002:1617.
- State: built: `npc.full_name` (decision 0062) with the `#` part kept in `display_name` (decision 0064). The `strnpcinfo` row converts in 264 of 264 renewal scripts.
- Recommendation: none needed.

## G. Converter: commands (proposal steps 11 to 16 and 40)

Most of the "needed for the rest" lists of steps 12 to 16 have no user: the fork's `npc/` tree has 0 files with `countbound`, `openstorage2`, `npcshopitem`, `guildskill`, `guildchangegm`, `getmapguildusers`, `getguildinfo`, `guildstoragedelitem`, `getitembound3`, `getitembound4`, `$@party_create_id`, `party_create`, `party_addmember` or any `channel_*` command. Those are API completeness for mods, not parity.

### G1. Chat channel commands (step 12)

- Source: proposal 0002:1229.
- Recommendation: defer to later: deleting a channel, options, passwords, bans, colors, one map's channel, other characters. 0 files.

### G2. Party (step 13)

- Source: proposal 0002:1240.
- State: member IDs built (`party:get_member_list`, spec.go:909), the leader's name built (`party.leader_name`, spec.go:901; the 3 `getpartyleader` uses in instances read the name), `warpparty` built (decisions 0065, 0066), commands for another character built (decision 0066).
- Recommendation: defer result codes and `$@party_create_id` to later. 0 files.

### G3. Guild (step 14)

- Source: proposal 0002:1251.
- State: open. Users: `getguildmasterid` 1 custom file; `requestguildinfo` re/guild3/agit_main_te.txt; `maprespawnguildid` siege files; `getguildalliance` re/warps/guildcastles.txt (castle warps).
- Recommendation: defer `guildskill`, `guildchangegm`, `guildstoragecountitem` reads beyond the built ones, `getguildmasterid`, `getmapguildusers` and `getguildinfo` to later (0 official files). The siege ones (`warpguild`, `maprespawnguildid`, `requestguildinfo`, `getguildalliance`, castles) go to sieges-war, as its brief says.

### G4. Storage and items (step 15)

- Source: proposal 0002:1263.
- State: taking HP built (decision 0052), `basicskillcheck` built (decision 0027), commands for another character built (decision 0066).
- Recommendation: defer `countbound`, `openstorage2` modes, `guildstoragedelitem` and `getitembound3/4` to later. 0 files.

### G5. `delitem2` exact copies

- Source: proposal 0002:1263 ("the exact-copy 2 forms").
- State: `countitem2` converts (`wa.FromOptions` has `refine` and `cards`, spec.go:1596). `delitem2` does not: 1 renewal, 1 pre-renewal (quests/the_sign_quest.txt).
- Recommendation: accept: `player:take_item` honors the `refine` and `cards` of `wa.FromOptions`, which it already takes, as `delitem2`. Node: converter-commands.

### G6. `npcshopitem`, `npcshopattach` (step 16)

- Source: proposal 0002:1275.
- Users: 0 files for `npcshopitem`; `npcshopattach` with `OnBuyItem` only in 2 custom files.
- Recommendation: defer to later.

### G7. Barter of refined items and more than 5 kinds

- Source: proposal 0002:1275; npc-scripts-renewal.md:11, :25 (barter lines: 6 partly, 5 not).
- Recommendation: accept: a barter item in `data/npcs.yml` takes `refine` on the item it sells and on each cost entry, and any number of cost kinds; the same item may be sold twice with different costs. A data format change, noted as an amendment. Node: converter-commands.

### G8. Kafra points paying part of a cash shop price

- Source: proposal 0002:1275; npc-scripts-renewal.md:23.
- State: open. A cash shop's price is cash points only.
- Options: a shop field and a buy request that carries the points (protocol and window work); or keep cash only.
- Recommendation: defer to ui-windows, which owns shop windows. It is a product call: no script blocks on it.

### G9. `mail` arrays and `searchstores` on another map

- Source: proposal 0002:1275; tools/convert/README.md:70.
- Users: `mail` with arrays only in custom/woe_controller.txt; no failing `searchstores` row.
- Recommendation: defer to later.

### G10. Computed `callshop`

- Source: proposal 0002:1614; report "callshop: a computed shop", 5 renewal.
- Recommendation: accept, converter only: `R.callshop(name)` finds the converted shop record by name at run time and calls `player:open_shop`. Node: converter-commands.

### G11. `freeloop`

- Source: proposal 0002:1612.
- Users: 3 renewal scripts; 11 files in all.
- Options: convert to nothing and keep the per-call CPU budget; lift the budget for core while a script has `freeloop` on (new API); refuse.
- Recommendation: accept the first: `freeloop` converts to a comment. The loops it guards walk inventories and item lists, far below 10,000,000 units per call. Node: converter-commands.

### G12. `isequippedcnt`

- Source: proposal 0002:1612.
- Users: 4 renewal scripts.
- Recommendation: accept `player:count_equipped(item)`: how many worn copies, and cards in worn copies, are the item, as `isequippedcnt`. `game.read`. Node: converter-commands.

### G13. `getequiprandomoption`

- Source: proposal 0002:1612.
- Users: 1 renewal script (re/merchants/enchan_illusion_dungeons.txt:364).
- Recommendation: accept, converter only: `equipped.random_options` (spec.go:775) has the option, value and param; `rathena.lua` maps option names to rAthena's `RDMOPT_` IDs for `ROA_ID`. Node: converter-commands.

### G14. `mesitemlink` without brackets, with a name, or with arguments

- Source: proposal 0002:1614; report: 5 and 4 renewal.
- Recommendation: accept, converter only, over `items.name` and the given name. Node: converter-commands.

### G15. `getunitdata`, `getunittype`, `setunitdata` of game IDs

- Source: proposal 0002:1614; report "setunitdata: a type other than UNPC_CLASS, UNPC_SEX and the UNPC_ looks", 5 renewal.
- Users: `getunitdata` in 25 files, 24 of them instances and the mobs folder.
- Recommendation: accept, converter only: map the `UMOB_` and `UNPC_` types that monster and NPC handles read or set (`hp`, `max_hp`, `x`, `y`, `map`, speed, modes) and refuse the rest with a reason. `getunittype` maps to `kind`. Nodes: npc-api-events for `getunitdata` and `getunittype` (it owns npcmaps.go), converter-commands for `setunitdata`, as its brief lists it.

### G16. `$@partymember*` outside the function

- Source: proposal 0002:1614; report "getpartymember: into an array of the script's own", 1 renewal, 1 pre-renewal.
- Recommendation: accept, converter only, over `party:get_member_list`. Node: converter-commands.

### G17. Built since step 40

- `checkweight` with several items (the row converts in 902 of 902 renewal scripts), `monster` and `mobcount` with computed labels and the `areamonster` rectangle (decision 0065), `rentitem` (decision 0064), handles by `getnpcid` (decision 0063). None needed.

### G18. Dragon colors, Madogear suit, warg (step 11)

- Source: proposal 0002:1217; decisions/0064-npc-api-group-3.md:147-148.
- State: `setdragon`, `setmadogear`, `checkdragon`, `checkwug`, `ismounting` convert for one record each. The colors other than green, the suit and the warg need third-class data.
- Recommendation: defer to third-classes (v0.3.0), as GOAL.md maps it.

## H. Lua limits (decision 0054)

### H1. Long string compares hold a zone up to 30 s

- Source: decisions/0054-restores-logins-and-lua-time-limits.md:111; proposal 0002:1601.
- State: open, accepted risk. The CPU budget counts instructions, and one compare of two long strings is one instruction that reads both strings. The wall clock backstop is 30 s.
- Options:
  1. Keep the risk. Mods need the owner's approval, and core's scripts are reviewed. Cost: none.
  2. Charge the CPU budget by bytes for string compares, concatenation, `string.rep`, `find` and `gsub` in the C glue. Deterministic, and closes the class. Cost: a C glue change and a new `lua.wasm`.
  3. Lower the backstop. Brings back the stalls decision 0054 fixed.
- Recommendation: keep the risk for v0.2.0 and defer option 2 to the modding milestone, before third-party mods spread through the mod index. New node: `lua-cost-accounting` (modding, size M, depends on nothing; touches internal/script and the Lua glue).

### H2. CPU time limit on the Lua thread

- Source: decisions/0054-restores-logins-and-lua-time-limits.md:84, :106.
- State: not built. wasi has no thread clock.
- Recommendation: refuse. H1 option 2 covers the case it was for.

## Draft review decision 60

60. NPC API and converter review (review-npc-api, decisions 0052 to 0054 and 0062 to 0066, and next steps 11 to 16 and 40). The user approved these on <date>:
    - **Kept as built:** `fire_event` and `fire_npc_event` results; the forms of review decision 59's calls; `maps.set_cells`' `send`; the one-step lag of another map's player, the 2-second logout wait and nil results of remote calls; staff levels 99 and 10; `equipped:add_refine`, `items.refine_cost`'s refine, `clock.format`'s time and the block `equip`. `schema.Version` is raised by the release.
    - **Events between NPCs.** A broadcast starts a zone that has not started its NPC scripts only when one of its NPC sources names the label, as player events do. `npc:wake()` ends the waits of an NPC's handlers, as `awake`; it comes with sieges. `OnInterIfInitOnce` converts as `OnInit` run once per server start. `unloadnpc` of an NPC with duplicates converts to removing each record.
    - **Waits without a player.** `timers.wait(milliseconds)`, 0 to 10 minutes, in a handler of an NPC context that runs without a conversation. `R.sleep` uses it.
    - **Timers.** `timers.after` and `timers.every` take `{ clock = "zone" | "server" }`; a server clock timer wakes a sleeping zone, as schedules do, and converted NPC timers use it. `player:add_timer(milliseconds, label)` and `player:remove_timer(label)` are `addtimer` and `deltimer`: the timer moves with the player and ends at logout. NPC timers attached to a player and timer commands on another NPC convert over `fire_event`.
    - **Monsters and players across maps.** A monster label's event has `monster_id` and `monster_game_id`, for `killedrid` and `killedgid`. `monster:show_emotion(emotion)`. `"self"` as `player:say_aloud`'s `to`. `unitexists` converts over `maps.find_monster`. `monster:use_skill` takes `{ cast_milliseconds, can_cancel, emotion }` for `areamobuseskill`, with the instances. `npc:remove_effect` and `player:remove_effect` take `{ to }`.
    - **Maps.** `maps.can_enter(map, job)` is `jobcanentermap`, with sieges.
    - **Touch areas.** `npc:fire_touch(player)` runs an NPC's `touch` handlers for a player standing in its area, as `npc_touchnext_areanpc`. An NPC with a touch area and no touch label clicks on touch, as rAthena. An NPC with `sprite: none` takes answers from any distance, as `npc_checknear` for a class-less NPC.
    - **Parity fixes.** `player:heal_percent` with -100 kills, as `percentheal`; this replaces review decision 56's rule for -100. `player:get_skill_level` counts item and temporary levels, as `getskilllv`. `SkillInfo.granted_level` shows item skills in the skill list.
    - **Converter rules.** `wa.variables` names may start with a digit. An assignment used as a value becomes an inline function that assigns and returns. Computed `getd` and `setd` names keep `.@` variables in a table, make `.` variables NPC variables, and declare other scopes when the script runs. `freeloop` converts to nothing.
    - **More calls.** `jobs.name(job)` for `jobname`. `player:count_equipped(item)` for `isequippedcnt`. `player:take_item` honors `refine` and `cards`, for `delitem2`. Barter items take refines and any number of cost kinds. Computed `callshop`, `getequiprandomoption`, `mesitemlink`'s forms, `getunitdata`, `getunittype`, `setunitdata` and `$@partymember*` convert over existing calls.
    - **Deferred:** `OnNPCKillEvent`, `attachrid` without force, reads of another map's player, statuses on a monster of another map, `schedules.yearly`, a level per staff role, reading a limit without permission, chat channel commands, party result codes, guild calls without sieges, `countbound`, `openstorage2` modes, `getitembound3/4`, `npcshopitem`, `npcshopattach`, `mail` arrays and `searchstores` on another map to later; Kafra points in cash shops to the windows node; dragon colors, the Madogear suit and the warg to third classes; byte costs for string work in the Lua glue to the modding milestone.
    - **Refused:** a CPU time limit on the Lua thread, since wasi has no thread clock.
    - All of these add to API 1.0, except `heal_percent` at -100, which changes review decision 56.
