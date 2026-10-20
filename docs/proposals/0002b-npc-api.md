# NPC side of the Lua API: what blocks full rAthena NPC conversion

Read-only analysis of 2026-10-20 for one API batch. Nothing in either repository was changed.

## Summary

- Scope: every script NPC that rAthena loads from `npc/re/scripts_main.conf` and `npc/pre-re/scripts_main.conf`, in every folder. That includes `instances`, `guild`, `guild2`, `guild3`, `battleground`, `custom` and `mobs`, which the extract leaves out. Scripts in `npc/custom` that `scripts_custom.conf` does not load are out of scope.
- Renewal has 9,219 script NPCs that are not duplicates, and 19,420 lines with duplicates. Pre-renewal has 4,342 and 9,756.
- The converter places 3,209 renewal scripts today (34.8%). With every A, B and E item done it would place 7,213 (78.2%). The other 2,006 stay blocked by systems that do not exist (C).
- Pre-renewal goes from 2,139 (49.3%) to 3,933 (90.6%). 409 stay blocked by C.
- With duplicates, renewal goes from 5,070 of 19,420 lines (26.1%) to 13,845 (71.3%). Pre-renewal goes from 3,233 of 9,756 (33.1%) to 8,245 (84.5%).
- Of the 2,006 renewal scripts left for C, 1,511 need only instances. Sieges, battlegrounds and client navigation hold most of the rest.
- Distinct commands and labels that fail somewhere: about 61 in A, 136 in B (with 609 renewal custom event labels counted as one row), 77 in C, 2 in D and 47 in E.
- First blocker today, renewal: B 4,094 scripts, C 897, A 649, E 369, D 1. Pre-renewal: B 1,700, A 298, C 150, E 55.
- Only two scripts hit a D item: `atcommand` (1) and `setbattleflag` (1). `query_sql`, the refusal the strategy names (section 7.2 and section 14.2a), appears only in `npc/custom` files that rAthena does not load.

## Method

- I ran `tools/wathena/extract.py`'s `build_warps` and `build_npcs` from the fork, with every folder kept, and wrote the result to the scratchpad (`ext/<mode>/npcs*.json`). The counts for the extract's own folders match `docs/generated/npc-scripts-renewal.md` exactly (for example cities 947 lines and 699 placed).
- I ran the converter from a copy of wAthena in the scratchpad with one change: a script records every part that fails and goes on, as functions already do. An unknown event label is recorded and its part is still converted. The normal run gives the first blocker. The full run gives every blocker. The report in docs/generated stops at the first one, so it hides most commands. For example `initnpctimer` blocks 825 renewal scripts and is not in the report at all.
- "Scripts" counts script NPCs that are not duplicates, whose own script fails on the item. "First" is the converter's first blocker today. "Sole" means nothing else in the script fails. A duplicate converts when its source converts and its own map is not an instance map.
- Parse failures (48 renewal scripts, 37 pre-renewal) hide every command after them. They count as E only.
- Category C is decided by the command, or by the folder: `instances` and instance maps (names with `@`), `guild`, `guild2`, `guild3`, and `battleground`. A script in those folders counts as C even when its own commands would convert.
- Coverage after "A, B and E" assumes each item converts every form the scripts use.
- Raw data: `inv-renewal.json` and `inv-pre-renewal.json` (every NPC with its first blocker and full list), `an3.json` (the classification per script), and the scripts in `tools/` of the scratchpad.

## Coverage estimate

| | Renewal | Pre-renewal |
|---|---:|---:|
| Script NPCs, not duplicates | 9,219 | 4,342 |
| Placed today | 3,209 (34.8%) | 2,139 (49.3%) |
| Placed after A, B and E | 7,213 (78.2%) | 3,933 (90.6%) |
| Left for C | 2,006 | 409 |
| Lines with duplicates | 19,420 | 9,756 |
| Placed today, with duplicates | 5,070 (26.1%) | 3,233 (33.1%) |
| After A, B and E, with duplicates | 13,845 (71.3%) | 8,245 (84.5%) |

What the 4,002 renewal and 1,794 pre-renewal scripts that are not placed and not C need (A is converter work, B new API, E converter syntax):

| Needs | Renewal | Pre-renewal |
|---|---:|---:|
| B only | 2,415 | 1,091 |
| A and B | 799 | 461 |
| E only | 302 | 37 |
| A only | 284 | 154 |
| B and E | 132 | 28 |
| A, B and E | 60 | 19 |
| A and E | 10 | 4 |
| D | 2 | 0 |

Scripts each B area unlocks, in the order that unlocks the most at each step (A and E done first):

| Step | Renewal gain | Renewal total | Step | Pre-renewal gain | Pre-renewal total |
|---|---:|---:|---|---:|---:|
| A and E | 596 | 596 | A and E | 195 | 195 |
| B5 presentation | 480 | 1,076 | B3 NPC events | 242 | 437 |
| B7 player | 556 | 1,632 | B2 NPC visibility | 416 | 853 |
| B2 NPC visibility | 463 | 2,095 | B8 maps and monsters | 221 | 1,074 |
| B3 NPC events | 791 | 2,886 | B7 player | 209 | 1,283 |
| B6 quest icons | 365 | 3,251 | B5 presentation | 231 | 1,514 |
| B8 maps and monsters | 348 | 3,599 | B4 variables | 194 | 1,708 |
| B4 variables | 229 | 3,828 | B10 clock and settings | 47 | 1,755 |
| B9 items | 89 | 3,917 | B9 items | 29 | 1,784 |
| B10 clock and settings | 65 | 3,982 | B1 lookups | 10 | 1,794 |
| B1 lookups | 20 | 4,002 | | | |

- B1 is small on its own, but every call on another NPC in B2, B3 and B5 needs `world.get_npc`, so B1 ships with them.
- Scripts that need each area (any combination), renewal: B2 1,305, B3 1,301, B5 1,128, B7 889, B8 435, B6 388, B4 237, B9 92, B10 65, B1 20. Pre-renewal: B3 991, B2 660, B8 301, B7 258, B5 254, B4 202, B10 47, B9 30, B1 10.
- Scripts that need only one B area, renewal: B5 480, B7 432, B3 319, B2 308, B6 80, B4 65, B10 30, B8 17, B9 11, B1 3.

What stays blocked by C (scripts not placed, not duplicates):

| Systems | Renewal | Pre-renewal |
|---|---:|---:|
| Instances only | 1,511 | 124 |
| Sieges (WoE castles) only | 137 | 98 |
| Battlegrounds only | 121 | 117 |
| Client navigation (`navigateto`) only | 79 | 0 |
| NPC chat rooms (`waitingroom`) only | 34 | 25 |
| Marriage (`getpartnerid`, `divorce`) only | 22 | 22 |
| Achievements only | 17 | 0 |
| Instances and navigation | 16 | 0 |
| Battlegrounds and chat rooms | 14 | 14 |
| Enchant, reform and refine windows | 11 | 0 |
| Mercenaries | 10 (+2 with instances) | 1 |
| Reputation | 10 (+3 combined) | 0 |
| Battlegrounds and sieges | 7 | 7 |
| Stylist window | 6 | 0 |
| Homunculus | 4 | 0 |
| Auction, bank | 1, 1 | 1, 0 |

- 1,494 of the 2,006 renewal C scripts, and 362 of the 409 pre-renewal ones, also need B items. B is a prerequisite for those systems too.

## B: proposed API, by area

Rules followed: full words, read-only fields, methods for changes, options tables, no globals, and rAthena's semantics (proposal 0002 decisions 3 to 5, and 0002a rule 1). Every name below is a proposal for the user's approval. Line numbers are `src/map/script.cpp` of the fork unless another file is named: the BUILDIN_DEF line and then the BUILDIN_FUNC line.

### B1. Lookups: NPCs and players by name or ID

Prerequisite for every call on another NPC. rAthena finds NPCs by unique name (`npc_name2id`) and players by name or ID.

- `world.get_npc(name) → wa.Npc?`: an NPC by record name. The mod's own name, or `mod:Name` for another mod's NPC. Permission `game.read`. It matches `world.get_shop`. Converts `getnpcid(0, "X")` (28004/8915), and the NPC name argument of `enablenpc`, `donpcevent`, `npctalk`, `emotion`, `specialeffect`, `cloakonnpc`, `setnpcdisplay`, `getvariableofnpc` and the timer commands.
- New read-only `wa.Npc` fields for `strnpcinfo` (28013/9248): `full_name` (type 0, the name with its `#` part), `hidden_name` (type 2, the part after `#`) and `unique_name` (type 3). `name` stays type 1 and `map` type 4.
- `maps.find_player(name) → wa.Player?`: an online player by name. Proposal 0002 already uses this name in "Comparison with rAthena", but it was never built. Add `maps.find_player_by_id(id)` for a character or account ID, as `getcharid(3)` values reach `unittalk`, `sc_start2` (28118/12401), `attachrid` (28150/13507) and `convertpcinfo` (28597/26690). Permission `game.read`.
- `party:get_member_list() → { name, character_id, account_id, online }[]` for `getpartymember` types 1 and 2 (28006/8972), as decision 27's next step 13 lists.
- Design question: wAthena runs a Lua state per map. Calls between NPCs cross zones. In the renewal scripts, NPC-to-NPC commands with a fixed target name go to an NPC on the same map 5,037 times, on another map 153 times, from an NPC without a map 1,222 times, and to an NPC without a map 170 times. A handle to an NPC of another zone needs its change calls queued to that zone. Reads such as `getvariableofnpc` across zones need a decision.

### B2. NPC visibility and state

rAthena: `enablenpc` and its seven aliases are one function (28109 to 28116, func 12339), which calls `npc_enable_target` (npc.cpp 1036).

- `npc:show()`: `enablenpc`. The NPC shows, takes clicks, and its touch area works again. A player who stands in the area gets the touch event (npc.cpp 1100). Proposal 0002 "Comparison with rAthena", Events table, already names `npc:show()`.
- `npc:hide()`: `disablenpc`. The NPC is gone for players: no view, no click, no touch, and player events of the NPC do not run (npc.cpp 1761, 1901, 5120). `fire_event` of B3 still runs it, as `donpcevent` does (`npc_event_do_sub`, npc.cpp 1284, runs the label with no check). Named in the same table.
- `npc:set_hidden(hidden)`: `hideonnpc` and `hideoffnpc`. Not shown and clicks refused (npc.cpp 2225), but the touch area still works (npc.cpp 1901 checks only disabled and cloaked).
- `npc:set_cloaked(cloaked, player?)`: `cloakonnpc`, `cloakoffnpc`, `cloakonnpcself`, `cloakoffnpcself`. Without a player, for everyone. With a player, for that player only (`sd->cloaked_npc`). A cloaked NPC has no touch for that player (npc.cpp 1005), and monsters do not touch it (npc.cpp 2007).
- `npc:is_cloaked(player?) → boolean`: `isnpccloaked` (28598/26738).
- `npc:remove()`: `unloadnpc` (28530/25658). The NPC is gone until the server restarts.
- Read-only field `npc.state`: `"shown"`, `"hidden"` or `"disabled"`.
- Data: `data/npcs.yml` takes `state: disabled | hidden | cloaked`, as rAthena's `script(DISABLED)`, `(HIDDEN)` and `(CLOAKED)` lines (160 renewal scripts start cloaked). It takes `sprite: none` for rAthena's view -1: no view and no click, events and touch only (116 renewal, 99 pre-renewal scripts). The converter now uses HIDDEN_NPC only with a touch area.
- Permission: none on the context's own NPCs. A new `npcs.control` on another mod's NPC.

### B3. NPC events between NPCs, and NPCs without a map

The largest blocker: 1,682 renewal scripts have custom event labels (609 distinct, such as `OnEnable` 792, `OnDisable` 445, `OnStart` 309, `OnMyMobDead` 225, `OnReset` 202), and 1,157 use `donpcevent`.

- Target side: `events.on("npc_event", handler, { name = "OnEnable" })` in an npc context. The handler gets `event` with `name`, `player` (or nil) and `conversation` (when a player is attached, as the touch event has). Names match rAthena's labels without case.
- `npc:fire_event(name, options?) → boolean`: `donpcevent "NPC::OnX"` (28085/11615) runs the label with no player. `{ player = p }` is `doevent` (28084/11598), which runs it for the player and queues it while the player talks. Returns whether a handler ran, as `donpcevent` returns 1 or 0. It runs on a disabled NPC, as `npc_event_do_id` (npc.cpp 1305) does.
- `world.fire_npc_event(name) → integer`: `donpcevent "::OnX"`, every NPC with the label (`npc_event_doall_sub`, npc.cpp 1255). Returns the count.
- `cmdothernpc` (11629) is `fire_event("OnCommand" .. command)`.
- Data: a script record may leave out `map`. It runs only through events, schedules and timers, as rAthena's floating NPCs (574 renewal and 451 pre-renewal scripts). Like a world event without a map (decision 16), it runs in the zone of the world's start map. Floating NPCs call other NPCs 1,222 times in renewal, so this needs the cross-zone answer of B1.
- Player events in npc contexts: `login`, `logout`, `map_enter`, `base_level_up` and `job_level_up` (today mod and event contexts only), for `OnPCLoginEvent`, `OnPCLogoutEvent`, `OnPCLoadMapEvent`, `OnPCBaseLvUpEvent` and `OnPCJobLvUpEvent` (script.cpp 263 to 270). `OnPCDieEvent`, `OnPCKillEvent` and `OnNPCKillEvent` hear deaths on every map, while the `death` event hears only the context's map. They need a `{ everywhere = true }` option, or a floating NPC hears every zone.
- New event `monster_touch` (npc contexts), with the monster: `OnTouchNPC` (script.cpp 275, npc.cpp 2000). 30 renewal scripts.
- A monster death that runs another NPC's label converts once `fire_event` exists. `awake` (28353/20436) wakes an NPC's sleeping scripts, one script.

### B4. Variables

- `variables.npc_integer(name)`, `npc_boolean`, `npc_string`, `npc_table`: rAthena's `.x` NPC variables, which other NPCs reach with `getvariableofnpc` (28354/20483). Handles have `get(npc)`, `set(npc, value)` and `add(npc, amount)`. Kept in memory per NPC, not saved, as rAthena. They also fix `set getvariableofnpc(...)`, which today stops `F_time_limit_recorder` from parsing (113 renewal scripts).
- `variables.server_temporary_integer`, `_boolean`, `_string`, `_table`: rAthena's `$@x`. One value for the world, not saved, lost at restart. Proposal 0002 "Comparison" maps `$@` to a local of one script, which does not hold when many NPCs, often on other maps, share it. 310 renewal and 237 pre-renewal scripts use `$@` variables.
- `getd` and `setd` (28296/18155, 28297/18016) need no API: the converter can declare a variable by a computed name at run time. But a mod declares at most 256 variables per runtime (`server/modscript/variables.go`, `MaxModVariables`). Converted core NPCs on one map can pass that once all of them load. Core needs a higher limit or no limit.

### B5. NPC presentation: speech, effects, sounds, looks and movement

- `npc:say_aloud(text, options?)`: `npctalk` (28218/16193) with its other arguments. `to` is `"area"` (the default), `"map"` (`bc_map`), `"world"` (`bc_all`) or a player handle (`bc_self`). `color` is 0xRRGGBB, white by default. On another NPC through B1. 800 renewal scripts.
- `player:say_aloud(text, options?)`: `unittalk` (28347/20216) on a player, with the same `to`. 207 renewal scripts, mostly `bc_self`.
- `player:show_emotion(emotion)`: `emotion` with a player's game ID (28162/13818). An NPC's ID goes through B1. 401 renewal and 119 pre-renewal scripts.
- `npc:show_effect(effect, options?)` and `player:show_effect(effect, options?)` gain `to`: `"area"` (the default), `"map"` or a player, for `specialeffect`'s send target (28208/15596) and `specialeffect2`. Plain `specialeffect` on the NPC itself needs only converter work (A). `npc:remove_effect(effect)` and `player:remove_effect(effect)`: `removespecialeffect` (28210/15652).
- `npc:set_title(text)`: `setunittitle` (28336/19944), 51 renewal scripts (the enchant NPCs).
- `npc:set_display_name(name)` and `npc:set_sprite(sprite)`: `setnpcdisplay` (28278/17869) and `classchange` (28191/15163).
- `npc:set_look(look)`, with `sprite`, `hair_style`, `hair_color`, `cloth_color`, `head_top`, `head_middle`, `head_bottom` and `sex`: `setunitdata` with `UNPC_*` (28339/19398). Uses: `UNPC_CLASS` 15, `UNPC_HAIRSTYLE` 15, `UNPC_HAIRCOLOR` 12, `UNPC_HEADTOP` 9, `UNPC_SEX` 7, `UNPC_CLOTHCOLOR` 7.
- `npc:move_to(x, y)`: `movenpc` (28216/16146). `npc:walk_to(x, y)`: `npcwalkto` (28224/16293) and `unitwalk` on an NPC (28340/19996). `npc:set_speed(milliseconds)`: `npcspeed` (28223/16260). `npc:stop()`: `npcstop` and `unitstopwalk` (28346/20183).
- `npc:show_progress(seconds, options?)`: `progressbar_npc` (28389/22546). A bar over the NPC, with `color`. The script waits.
- `npc:use_skill(skill, level, target, options?)`: `npcskill` (28455/23286). The NPC casts a support skill with `stat` and `npc_level`, as `monster:use_skill`.
- `entity:show_skill_effect(skill, level)`: `skilleffect` (28206/15519).
- Sounds: `player:play_sound(file, options?)` for `soundeffect` (28195/15281) with `repeat`, `npc:play_sound(file, options?)` for `soundeffectall` (28196/15308) with `to`, `player:play_music(file)` for `playbgm`, and `maps.play_music(map, file)` for `playbgmall`.
- `player:send_message(text, { color })`: `dispbottom`'s color (28250/15796).
- `player:show_digits(value, kind)`: `showdigit` (28394/22745).
- `maps.mark_minimap(map, id, x, y, color)` and `maps.unmark_minimap(map, id)`: `viewpointmap` (27986/7013).
- Data: an NPC record's `sprite` takes a monster or job view, as rAthena allows any class (112 renewal and 71 pre-renewal scripts, such as `Bonubonu#ama1` with 1323).

### B6. Quest icons

- `npc:add_quest_icon(icon, options?)`: `questinfo` (28467/20949). Each NPC keeps a list. For each player on the map, the first entry whose `condition` holds shows. `mark` is the minimap mark: `"yellow"`, `"green"` or `"purple"` (`QMARK_*`). `condition` is a function of the player that only reads, like a hook. rAthena parses a script string, which the converter compiles to Lua. 491 renewal scripts, 424 as their first blocker.
- rAthena shows the icons again on map entry (clif.cpp 11188), on item gain and loss (pc.cpp 6088 and 6125), on level and job changes (pc.cpp 8314, 8356, 11076) and on quest updates (quest.cpp 837). `player:refresh_quest_icons()` is `questinfo_refresh` (28475/21043).
- `wa.QuestIcon` gains `daily_quest`, `job_quest`, `click_me`, `event3` and `jumping_poring` (QTYPE_ values of PACKETVER 20170315 and later). In `npc/`: QTYPE_QUEST 331, QTYPE_DAILYQUEST 131, QTYPE_QUEST2 64, QTYPE_JOBQUEST 15, QTYPE_CLICKME 10, QTYPE_JUMPING_PORING 1.
- `npc:show_quest_icon(player, icon, { mark })`: the mark color of `showevent`, which the converter refuses today.
- `player:open_quest_log(quest?)`: `open_quest_ui` (28620).

### B7. Player reads and actions

- Fields: `player.weight` and `player.max_weight` in rAthena's tenths (`Weight`, `MaxWeight`: 343 renewal scripts, 235 as first blocker), `player.skill_points` (`SkillPoint`, 56), and `player.look` with `hair_style`, `hair_color`, `cloth_color`, `head_top`, `head_middle`, `head_bottom`, `weapon` and `shield` (`getlook`, 28221/16377).
- `player:give_exp(base, job, options?)`: `getexp` (28187/11142). It multiplies by `quest_exp_rate` and caps at the maximum, as rAthena. `{ raw = true }` is `getexp2` (28519/24713): no rate, and a negative value takes exp (`pc_lostexp`). New permission `player.give_exp`. 446 renewal scripts.
- `player:heal(hp, sp)`: `heal` (27987/6007), fixed amounts without item bonuses.
- `player:set_blocked(action, blocked)` and `player:is_blocked(action)`: `setpcblock` and `getpcblock` (28328/18964, 18979). Actions are rAthena's `PCBLOCK_` flags in words: `move`, `attack`, `skill`, `use_item`, `chat`, `immune`, `sit_stand`, `commands`, `npc_click`, `npc`, `emotion` and `party`. 157 renewal scripts.
- `conversation:lock_items()` and `conversation:unlock_items()`: `disable_items` and `enable_items` (27983/8828, 27982/8820). While locked, the player cannot use or equip items during the dialog. 118 renewal scripts.
- `player:unequip_all()`: `nude` (28212/15715).
- `player:push(direction, cells)`: `pushpc` (28390/22604).
- `player:transform(monster, milliseconds)`: `transform` (28494/23777).
- `player:open_book(book, page)`: `readbook` (28385/20929).
- `player:get_save_point() → map, x, y`: `getsavepoint` (28222/16412).
- `player:get_inventory()`: `getinventorylist` (28188/15029). Proposal 0002 "Comparison" names it, but it was not built.
- `player:set_look(look)`: `setlook` (27953/6860).
- `player:has_option(name)`: `checkoption` (28055/10526).
- `player:set_hp(amount)`: `Hp = n`.
- `player.staff_level`, or a level per staff role: `getgmlevel`. `F_GM_NPC` calls it and leaves 96 renewal NPCs partly converted.
- `jobs.class_flags(job)` and `jobs.from_class(flags, sex?)`: `eaclass(job)` (28361/12835) and `roclass` (28362/12853). `player.job_class` already covers `eaclass()` without an argument.
- `player:change_job(job, { force = true, keep_line = true })`: `jobchange <job>,Upper` for a transcendent or baby player. Today `Job_Change` stays partly converted for 23 NPCs.
- `player:give_skill(skill, level, "permanent_grant")`: `SKILL_PERM_GRANT`, which decision 57 left out (5 scripts).
- Mounts and companions that decision 25's next step 11 left for review: the dragon and mado gear mounts and the warg companion. Then `checkdragon`, `setdragon`, `checkmadogear`, `setmadogear`, `checkwug` and `ismounting` convert over `player.mount` and `player.companion`.

### B8. Maps, areas and monsters

- `spawning.monsters` options: any `map` (decision 16 limits a context to its own map), `display_name` (the name argument of `monster` and `areamonster`, 28078/11238 and 28080/11359), `size` and `ai`. `monster` on another map blocks 122 renewal and 108 pre-renewal scripts, and a name of its own 145 and 57.
- `monster:remove()` and `monster:kill()`: `killmonster` (28081/11480) kills a label's monsters with no killer and strips the label first (`buildin_killmonster_sub_strip`). `unitkill` (28342/20061). 435 renewal and 234 pre-renewal scripts.
- `maps.remove_monsters(map)`: `killmonsterall` (28082/11521), spawn-line monsters too, which come back.
- `maps.count_players(map, options?) → integer`: `getmapusers` (28103/12146), and `getareausers` (28104/12167) with `rectangle`. Any map.
- `maps.warp_players(map, to_map, x, y, options?)`: `mapwarp` (28213/14317) with a guild or party filter, and `areawarp` (27950/5695) with `rectangle` and a target area.
- `party:warp(map, x, y)`: `warpparty` (27951/5781) with its map checks (decision 27, next step 13).
- `maps.set_cells(map, x1, y1, x2, y2, kind, on)` and `maps.check_cell(map, x, y, kind)`: `setcell` (28371/20631) and `checkcell` (28370/20614).
- `maps.set_flag(map, flag, value)`: `setmapflag` and `removemapflag` (28156/13617, 28157/13683).
- `maps.find_free_cell(map, options)`: `getfreecell` (28372/20657).
- `maps.announce(map, text, { rectangle })`: `areaannounce` (28100/12050).
- `maps.find_entities` on another map, which raises an error today: `areapercentheal` (28409/5746) and `areamobuseskill` (28387/22447).
- `maps.can_enter(map, player)`: `jobcanentermap` (28528/25007).
- Monster methods for `setunitdata` with `UMOB_*`: `monster:set_hp(hp)` (`UMOB_HP`, 50 uses), `monster:set_damage_immune(immune)` (9), `monster:set_modes(...)` (9) and `monster:set_speed(milliseconds)`.
- `monster:use_skill_at(skill, level, x, y)`: `unitskillusepos` (28349/20305). `monster:warp_to(x, y)`: `unitwarp` (28343/20075).
- `monsters.get(monster)`: `getmonsterinfo` (28311/18711), with name, level, HP, SP, exp, attack, defense, stats, range, race, element, size and mode. `F_AvenBoard` leaves 42 renewal NPCs partly converted.

### B9. Items and equipment

- `GiveItemOptions` gains `refine`, `cards`, `broken`, `identified` and `signed_by`: `getitem2` (27968/7775) and `getnameditem` (27969/8085). `FromOptions` of `count_item` gains `refine`, `cards` and `rentals`: `countitem2` and `rentalcountitem` (28600/7427).
- `maps.drop_item(map, x, y, item, amount, options?)`: `makeitem` and `makeitem2` (27971/8165, 27972/8237).
- `items.get(item)`: `getiteminfo` (28281/14761) and `getitemslots` (28185/14743).
- `items.can_refine(item)`, `items.refine_chance(item, level, options?)` and `items.refine_cost(item, kind)`: `getequipisenableref` (28021/9539), `getequippercentrefinery` (28025/9684) and `getequiprefinecost` (28580/26037), from core's refine data.
- `equipped:refine(amount)` for `successrefitem` (28026/9728) and `downrefitem` (28028/9836) with a negative amount, and `equipped:destroy()` for `failedrefitem` (28027/9802) and `delequip` (28457/9885). A new permission, or `player.give_items`.
- `player:get_broken_items()`, `player:repair_item(n)` and `player:repair_all()`: `getbrokenid` (28017/9422), `repair` and `repairall` (28019/9481). `repairmain` leaves 15 NPCs partly converted.
- `player:run_item_script(item)`: `consumeitem` (28456/23351).
- `player:merge_items()`: `mergeitem` (28507/24112).

### B10. Clock and server settings

- `wa.clock`: `clock.now()` (Unix seconds, `gettimetick(2)`), `clock.milliseconds()` (`gettimetick(0)`), `clock.get(field)` in the server's time zone (`gettime`'s `DT_` fields), and `clock.format(format)` (`gettimestr`). Lines 28068 to 28070, funcs 10862, 10908 and 10929. The sandbox has no `os` library (internal/script/README.md), so Lua cannot read the time today. No permission. `world:set_time` sets it in tests.
- `rules.get` gains `base_exp_rate`, `job_exp_rate`, `quest_exp_rate`, `max_hair_style`, `max_hair_color` and `max_cloth_color`: `getbattleflag` (28306/17053). The next steps of review decision 32 say these are conf settings, not rules. `feature.barter` is a constant true, and `feature.bgqueue` and `feature.refineui` a constant false while those systems do not exist, which the converter can do alone. 75 renewal and 65 pre-renewal scripts.
- `schedules.yearly(month_day, time, handler)` for `OnDayMMDD`. No script in `npc/` uses it now, so it can wait.

## A: converter work onto API that exists

- NPC timers (`initnpctimer`, `startnpctimer`, `stopnpctimer`, `setnpctimer`, `getnpctimer`, `addtimer`, `deltimer`, and `OnTimer<n>` labels): an `R.npc_timer` helper over `timers.after` keeps rAthena's tick, its labels and the attached player (11710 to 11868). 891 renewal and 408 pre-renewal scripts. A timer of another NPC needs B3.
- Equipment reads: `isequipped` (216 renewal scripts, 173 with nothing else failing), `isequippedcnt`, `getequipid`, `getequipcardid`, `getequiprefinerycnt`, `getequipisequiped`, `getequipweaponlv`, `getequiparmorlv`, `getequipname` and `getequiprandomoption`, over `player:get_equipped(slot)` and its `item`, `refine`, `cards`, `weapon_level`, `armor_level` and `random_options`.
- `specialeffect` on the NPC itself with AREA: `npc:show_effect` (370 renewal scripts).
- Strings and arrays: `atoi`, `strtol`, `compare`, `charat`, `replacestr`, `sscanf`, `strtolower`, `strtoupper`, `explode`, `insertchar`, `sprintf`, `substr`, `countstr`, `inarray`, `countinarray`, `copyarray`, `pow`, `min` and `max` with arrays, and `distance`.
- Handles: `getunitdata` reads, `getmapxy`, `unitexists`, `getunittype` and `getnpcid(0)`. A handle of another NPC or player needs B1.
- `getd` and `setd` (runtime declaration, see B4 for the limit).
- `areamonster` on the NPC's own map, and `monster` and `mobcount` with a computed label of the NPC itself.
- `warp` to a computed map (an `R.warp` that checks `Random` and `SavePoint` when it runs), `callshop` of a computed name, `checkweight` with several items, `mesitemlink` with more arguments, `getskilllv` and `skill` with a computed skill, `Zeny = n`, `rentitem`, `getgroupitem`, `debugmes`, `freeloop`, and `$@partymember*` read outside the function.
- `OnInterIfInitOnce` runs as `OnInit`.
- Fix: `strnpcinfo(0)` converts to `npc.name`, which drops the `#` part. rAthena returns the full name. This converts today but is not rAthena's value, and it matters where scripts build `strnpcinfo(0) + "::OnX"`. It needs `npc.full_name` of B1.

## C: systems that do not exist

- Instances: `instance_create`, `instance_enter`, `instance_npcname`, `instance_mapname`, `instance_warpall`, `instance_announce`, `instance_check_party`, `instance_live_info`, `instance_id`, `instance_destroy`, `'x` variables, `OnInstanceInit`, and every NPC on an instance map. `variables.instance_*` is in the API already but refuses with "instances do not exist yet".
- Sieges: `getcastledata`, `setcastledata`, `getcastlename`, `flagemblem`, `agitcheck` 1 to 3, `gvgon`, `gvgoff`, `gvgoff3`, `guardian`, `guardianinfo`, `setwall`, `delwall`, `checkwall`, `maprespawnguildid`, `woetetimestart`, `woetimer`, and the `OnAgit*` and `OnGuildBreak` labels. Decision 28 says castles, sieges and guardians come later.
- Battlegrounds: `bg_*`, `waitingroom2bg`, and `getcharid(4)`. `getcharid(5)` needs clans.
- Smaller systems: NPC chat rooms (`waitingroom`, `warpwaitingpc`, `enablewaitingroomevent`, `disablewaitingroomevent`, `getwaitingroomstate`), client navigation (`navigateto`, and `F_Navi`), marriage (`getpartnerid`, `divorce`), achievements, reputation, mercenaries, homunculus, the stylist window, the bank, auctions (decision 30 says later), and the enchant grade, item enchant, item reform and refine windows.
- NPC chat rooms are the smallest of these. A proposal would be `npc:open_chat_room { title, limit, event, trigger, zeny, min_level, max_level }` with `chat_room:warp_members(map, x, y, count)`. It still needs chat rooms in the client and the server.

## D: refused by a decision

- `query_sql` and `query_logsql`: strategy section 7.2 ("Not provided") and section 14.2a, and section 6.6, "There is no permission for raw SQL". No script that rAthena loads uses them. Only `npc/custom/card_seller.txt`, `woe_controller.txt` and `quests/hunting_missions.txt`, which are not loaded, do.
- `setbattleflag` (1 renewal script, `quests_14_3_bis.txt` raises the exp rates by 10): `wa.rules` is read-only (review decision 32). There is no explicit refusal yet. The user decides.
- `atcommand` (1 renewal script, "Clothing Effect Removal"): GM commands are staff tools (review decision 35). There is no explicit refusal of the command for scripts yet. The user decides.

## E: converter parse bugs and syntax it does not support

- E1, parse: `set` on a reference that is not a variable: `set getvariableofnpc(...)` (also needs B4), `set getarg(0), x` (a reference argument), `set getelementofarray(getarg(0), i)` and `set getd(...)`. `F_time_limit_recorder` alone stops 113 renewal scripts (the Verus warps). Other forms that rAthena takes: a comma in a `for` header (`novice_vending_machine`), a nested ternary (`Baba Yaga#rus32`), and a stray `:` (`Strewn paper#e152a01`). 48 renewal scripts and 37 pre-renewal ones do not parse.
- E2, structure: `getarg` and `getargcount` in a `callsub` part that the main body also holds after an `end`, `callsub` and `goto` to a label in another event part, a label inside a block, local functions (`function Name; ... function Name { }`), assignment used as a value, a variable name that starts with a digit (`$2012_tegod_gloria`), and `setarray` or `getarraysize` on `getd(...)`.
- E3, functions called by name: rAthena calls a global function, or a local `function`, without `callfunc` (`parse_callfunc`, script.cpp 960 to 1000). The converter reads the name as an unknown command. Examples: `unknown_d` (54 renewal scripts), `F_InfoBoard` (36), `F_Navi` (35, also C navigation), `WeightBigQuest` (8), `PranksterDiag` (6), `check_sauce` and `checkburglary` (4 each), and local functions such as `Cell`, `Catwarp`, `JobName`, `kick_mob`, `Choice` and `RName`.

## Functions that convert partly

These functions convert, with parts that fail when a script reaches them. The NPCs that call them are placed but convert partly.

| Function | File | NPCs R/P | Parts that fail (category, area) |
|---|---|---:|---|
| linkflag | npc/guild2/agit_main_se.txt | 115/115 | getcastledata: castles, sieges and guardians are not in the API yet (C C:sieges); warp: to a computed map (A A-misc) |
| f_gm_npc | npc/other/gm_npcs.txt | 96/46 | getgmlevel (B B7) |
| f_okolnir | npc/quests/okolnir.txt | 34/34 | compare (A A-string) |
| job_change | npc/other/Global_Functions.txt | 23/23 | jobchange: a transcendent or baby flag (B B7) |
| f_avenboard | npc/re/other/adven_boards.txt | 42/0 | getmonsterinfo (B B8) |
| f_isequipidhack | npc/other/Global_Functions.txt | 33/1 | getequipid (A A-equip) |
| f_isequipcardhack | npc/other/Global_Functions.txt | 32/1 | getequipcardid (A A-equip) |
| f_tt | npc/other/turbo_track.txt | 16/16 | charat (A A-string); compare (A A-string) |
| f_isequiprefinehack | npc/other/Global_Functions.txt | 30/1 | getequiprefinerycnt (A A-equip) |
| repairmain | npc/merchants/refine.txt | 15/15 | getbrokenid (B B9); repair (B B9) |
| returnflag | npc/guild2/agit_main_se.txt | 13/13 | compare (A A-string); getcastledata: castles, sieges and guardians are not in the API yet (C C:sieges) |
| f_insertplural | npc/other/Global_Functions.txt | 15/6 | sprintf (A A-string) |
| f_ko_survival_dice | npc/re/jobs/2e/kagerou_oboro.txt | 21/0 | emotion: on a player or another NPC (B B5) |
| f_signseal | npc/quests/the_sign_quest.txt | 8/8 | f_sealfail (E E3); function: local functions (E E2) |
| f_mora_mist | npc/re/instances/HazyForest.txt | 14/0 | instance_mapname (C C:instances); mobcount: a computed event label (A A-spawn) |
| f_flag_woe_te | npc/re/guild3/agit_main_te.txt | 11/0 | getcastledata: castles, sieges and guardians are not in the API yet (C C:sieges); jobcanentermap (B B10) |
| f_insertcomma | npc/other/Global_Functions.txt | 10/0 | insertchar (A A-string) |
| f_tower_monster | npc/instances/EndlessTower.txt | 5/5 | areamonster (A A-spawn) |
| f_bardskillyhelle | npc/quests/skills/bard_skills.txt | 5/5 | disablenpc (B B2); donpcevent (B B3); emotion: on a player or another NPC (B B5); parameter MaxWeight (B B7) |
| mora_remains | npc/re/quests/quests_mora.txt | 10/0 | parameter MaxWeight (B B7) |
| f_malaya_jeepney | npc/re/cities/malaya.txt | 9/0 | getmapusers (B B8); setd (A A-var) |
| f_pvp_fsrs | npc/other/pvp.txt | 7/2 | getmapusers (B B8); strnpcinfo: other than the name and the map (B B1); warp: to a computed map (A A-misc) |
| f_partyrelay_exp | npc/quests/partyrelay.txt | 4/4 | getexp (B B7) |
| f_brilliant_soda | npc/re/quests/juno_monster_society.txt | 8/0 | warp: to a computed map (A A-misc) |
| f_invest_status | npc/re/guild/invest_main.txt | 7/0 | agitcheck: castles, sieges and guardians are not in the API yet (C C:sieges); gettime (B B10) |
| f_cooldown | npc/re/merchants/eden_market.txt | 6/0 | gettimetick (B B10) |
| f_mal_picture | npc/re/quests/quests_malangdo.txt | 6/0 | parameter MaxWeight (B B7) |
| f_izludechannel | npc/re/cities/izlude.txt | 5/0 | warp: to a computed map (A A-misc) |
| magic_book_essence | npc/re/quests/magic_books.txt | 5/0 | parameter MaxWeight (B B7) |
| f_clocktowergate | npc/cities/aldebaran.txt | 2/2 | warp: to a computed map (A A-misc) |
| jewel_13_2 | npc/quests/quests_13_2.txt | 2/2 | donpcevent (B B3) |
| f_cherno | npc/quests/quests_airship.txt | 2/2 | emotion: on a player or another NPC (B B5); specialeffect (B B5) |
| f_mos_1 | npc/quests/quests_moscovia.txt | 2/2 | emotion: on a player or another NPC (B B5) |
| f_geoborg_text | npc/re/quests/quests_16_1.txt | 4/0 | emotion: on a player or another NPC (B B5); npctalk: as another NPC (B B5) |
| f_ko_survival_rps | npc/re/jobs/2e/kagerou_oboro.txt | 3/0 | emotion: on a player or another NPC (B B5) |
| f_ko_survival_dice2 | npc/re/jobs/2e/kagerou_oboro.txt | 3/0 | emotion: on a player or another NPC (B B5) |
| f_mal_coin | npc/re/merchants/coin_exchange.txt | 3/0 | parameter MaxWeight (B B7) |
| f_mora_enchant | npc/re/merchants/enchan_mora.txt | 3/0 | getitem2 (B B9) |
| f_getarmortype | npc/other/Global_Functions.txt | 1/1 | getiteminfo (B B9) |
| f_getweapontype | npc/other/Global_Functions.txt | 1/1 | getiteminfo (B B9) |
| f_invest_abyss | npc/re/guild/invest_main.txt | 2/0 | insertchar (A A-string); setd (A A-var) |
| f_tower_warp | npc/instances/EndlessTower.txt | 1/1 | warp: to a computed map (A A-misc) |
| func_socket | npc/merchants/socket_enchant.txt | 1/1 | disable_items (B B7); getiteminfo (B B9); specialeffect (B B5) |
| func_socket2 | npc/merchants/socket_enchant2.txt | 1/1 | disable_items (B B7); getiteminfo (B B9); specialeffect (B B5) |
| func_bingoresult | npc/other/hugel_bingo.txt | 1/1 | enablenpc (B B2); variables: $@bingoresult is a temporary server variable (B B4) |
| f_getplural | npc/other/Global_Functions.txt | 1/1 | charat (A A-string); compare (A A-string); countstr (A A-string); strtoupper (A A-string) |
| f_kienna | npc/quests/skills/rogue_skills.txt | 1/1 | disablenpc (B B2); donpcevent (B B3); enablenpc (B B2); specialeffect (B B5) |
| f_terra_gloria_inn | npc/re/quests/quests_16_2.txt | 2/0 | npctalk: as another NPC (B B5) |
| f_terragloria_lady | npc/re/quests/quests_16_2.txt | 2/0 | cloakoffnpcself (B B2); cloakonnpcself (B B2); emotion: on a player or another NPC (B B5); npctalk: as another NPC (B B5) |
| f_malaya_nurse | npc/re/quests/quests_malaya.txt | 2/0 | getexp (B B7); parameter MaxWeight (B B7) |
| eldicastes_c | npc/re/quests/quests_dicastes.txt | 1/0 | disablenpc (B B2); emotion: on a player or another NPC (B B5); enablenpc (B B2) |

## Every command and label that does not convert

Columns: the scripts that fail on it, with the scripts where it is the first blocker today and those where it is the only blocker, for renewal and for pre-renewal. Then how many of those scripts are in C folders or on instance maps, the area, the mapping or proposal, and the BUILDIN_DEF and BUILDIN_FUNC lines of script.cpp. A detail in parentheses splits one command by reason. `event (custom labels)` is one row for all 609 renewal and 255 pre-renewal custom labels.

### Category A

| Command or label | Renewal scripts (first, sole) | Pre-renewal scripts (first, sole) | In C folders R/P | Area | Mapping or proposal | script.cpp def/func |
|---|---:|---:|---:|---|---|---|
| `isequipped` | 216 (183, 173) | 122 (111, 108) | 13/13 | A-equip | R.isequipped over player:get_equipped(slot).item and .cards | 28237/16777 |
| `getequipid` | 70 (12, 5) | 18 (10, 5) | 6/0 | A-equip | player:get_equipped(slot).item | 28014/9303 |
| `getequipcardid` | 37 (0, 0) | 1 (0, 0) | 3/0 | A-equip | get_equipped(slot).cards[n] | 28283/14932 |
| `getequiprefinerycnt` | 33 (0, 0) | 1 (0, 0) | 0/0 | A-equip | get_equipped(slot).refine | 28022/9568 |
| `getequipisequiped` | 25 (1, 0) | 1 (0, 0) | 0/0 | A-equip | get_equipped(slot) ~= nil | 28020/9511 |
| `getequipname` | 8 (0, 0) | 0 (0, 0) | 0/0 | A-equip | items.name(get_equipped(slot).item) with rAthena's format | 28016/9383 |
| `isequippedcnt` | 8 (3, 1) | 0 (0, 0) | 1/0 | A-equip | R helper over get_equipped | 28238/16722 |
| `getequipweaponlv` | 6 (1, 0) | 2 (1, 0) | 0/0 | A-equip | get_equipped(slot).weapon_level | 28023/9598 |
| `getequiparmorlv` | 4 (0, 0) | 1 (0, 0) | 0/0 | A-equip | get_equipped(slot).armor_level | 28024/9635 |
| `getequiprandomoption` | 1 (0, 0) | 0 (0, 0) | 0/0 | A-equip | get_equipped(slot).random_options | 28524/24867 |
| `getunitdata` | 65 (0, 0) | 0 (0, 0) | 58/0 | A-handle | fields of the monster or NPC handle (x, y, hp, max_hp, map); needs B1 for NPC IDs | 28338/19065 |
| `getmapxy` | 36 (9, 0) | 8 (4, 0) | 24/4 | A-handle | handle.map, .x, .y; another player or NPC by name needs B1 | 28226/16451 |
| `unitexists` | 24 (0, 0) | 0 (0, 0) | 21/0 | A-handle | handle:exists() (exists) | 28332/19015 |
| `getnpcid` | 22 (17, 0) | 0 (0, 0) | 4/0 | A-handle | the NPC handle (B1 world.get_npc for another NPC) | 28004/8915 |
| `getunittype` | 1 (0, 0) | 0 (0, 0) | 1/0 | A-handle | handle.kind (exists) | 28333/19032 |
| `specialeffect` | 370 (109, 51) | 166 (74, 45) | 115/24 | A-misc | specialeffect on the NPC itself: npc:show_effect (exists) | 28208/15596 |
| `warp` | 37 (16, 6) | 27 (12, 2) | 8/5 | A-misc | computed map | 27948/5628 |
| `mesitemlink` | 14 (5, 2) | 0 (0, 0) | 1/0 | A-misc | arguments | 28633/27451 |
| `freeloop` | 5 (2, 2) | 1 (0, 0) | 2/1 | A-misc | no-op: the CPU budget applies | 28451/22994 |
| `variables ($@partymember* read outside the function)` | 5 (0, 0) | 1 (0, 0) | 4/0 | A-misc | $@partymember* read outside the function |  |
| `callshop` | 6 (1, 1) | 0 (0, 0) | 0/0 | A-misc | a computed shop | 28298/18172 |
| `rentitem` | 4 (0, 0) | 0 (0, 0) | 2/0 | A-misc | give_item { rent_seconds } (exists; rentals do not expire yet) | 27966/7922 |
| `getgroupitem` | 4 (0, 0) | 0 (0, 0) | 0/0 | A-misc | player:give_item_from_group (exists) | 28500/23225 |
| `getskilllv` | 2 (1, 0) | 2 (1, 0) | 0/0 | A-misc | computed skill | 28049/10421 |
| `parameter Zeny` | 2 (1, 1) | 1 (1, 1) | 0/0 | A-misc | Zeny = n: R helper over pay_zeny/give_zeny |  |
| `checkweight` | 3 (0, 0) | 0 (0, 0) | 3/0 | A-misc | several items | 28000/7480 |
| `debugmes` | 2 (0, 0) | 1 (0, 0) | 2/1 | A-misc | no output, or a log line (logmes converts to a comment) | 28124/12634 |
| `skill` | 1 (0, 0) | 1 (0, 0) | 0/0 | A-misc | computed skill | 28046/10372 |
| `callshop` | 1 (0, 0) | 0 (0, 0) | 0/0 | A-misc | the shop barter_ill_frozen does not convert | 28298/18172 |
| `callshop` | 1 (1, 1) | 0 (0, 0) | 0/0 | A-misc | the shop barter_ill_teddy does not convert | 28298/18172 |
| `callshop` | 1 (1, 1) | 0 (0, 0) | 0/0 | A-misc | the shop barter_ill_luanda does not convert | 28298/18172 |
| `callshop` | 1 (1, 0) | 0 (0, 0) | 0/0 | A-misc | the shop barter_ill_moonlight does not convert | 28298/18172 |
| `callshop` | 1 (1, 0) | 0 (0, 0) | 0/0 | A-misc | Pet_Items is not a shop | 28298/18172 |
| `mesitemicon` | 1 (1, 1) | 0 (0, 0) | 0/0 | A-misc | text.item_link |  |
| `event oninterifinitonce` | 1 (0, 0) | 0 (0, 0) | 0/0 | A-misc | runs as OnInit |  |
| `areamonster` | 126 (0, 0) | 32 (0, 0) | 95/18 | A-spawn | spawning.monsters { area } on own map; other map or name need B8 | 28080/11359 |
| `mobcount (computed label)` | 51 (0, 0) | 28 (0, 0) | 40/21 | A-spawn | computed label | 28220/14378 |
| `monster (computed label of the NPC itself: R helper)` | 29 (0, 0) | 13 (0, 0) | 22/12 | A-spawn | computed label of the NPC itself: R helper | 28078/11238 |
| `atoi` | 107 (41, 4) | 13 (2, 1) | 54/9 | A-string | R.atoi (tonumber with rAthena's rules) | 28318/17913 |
| `compare` | 47 (15, 6) | 21 (4, 2) | 23/14 | A-string | string.find plain, lower case | 28279/17945 |
| `charat` | 14 (3, 1) | 8 (1, 0) | 6/1 | A-string | string.sub | 28262/17119 |
| `copyarray` | 10 (0, 0) | 0 (0, 0) | 3/0 | A-string | R.copyarray | 27959/6388 |
| `sscanf` | 9 (3, 0) | 0 (0, 0) | 5/0 | A-string | R.sscanf | 28274/17555 |
| `replacestr` | 8 (4, 0) | 1 (0, 0) | 3/0 | A-string | R.replacestr | 28276/17728 |
| `explode` | 6 (1, 0) | 1 (1, 0) | 2/0 | A-string | R.explode | 28271/17267 |
| `sprintf` | 6 (0, 0) | 0 (0, 0) | 0/0 | A-string | string.format with rAthena's specifiers | 28273/17415 |
| `strtolower` | 3 (1, 1) | 1 (0, 0) | 0/0 | A-string | string.lower | 28267/17224 |
| `substr` | 2 (0, 0) | 2 (0, 0) | 2/2 | A-string | string.sub | 28270/17242 |
| `inarray` | 4 (1, 1) | 0 (0, 0) | 2/0 | A-string | R.inarray | 27963/6673 |
| `insertchar` | 1 (1, 0) | 1 (1, 0) | 0/0 | A-string | string.sub concat | 28264/17153 |
| `max` | 1 (0, 0) | 0 (0, 0) | 1/0 | A-string | arguments | 28290/24592 |
| `countinarray` | 1 (0, 0) | 0 (0, 0) | 0/0 | A-string | R.countinarray | 27964/6751 |
| `min` | 1 (0, 0) | 0 (0, 0) | 1/0 | A-string | arguments | 28288/24592 |
| `pow` | 1 (0, 0) | 0 (0, 0) | 1/0 | A-string | Lua ^ with integer result | 28286/17975 |
| `distance` | 1 (0, 0) | 0 (0, 0) | 1/0 | A-string | R.distance (rAthena distance_xy) | 28287/17985 |
| `strtol` | 1 (0, 0) | 0 (0, 0) | 1/0 | A-string | R.strtol | 28319/17932 |
| `event ontimer<n>` | 891 (197, 0) | 408 (70, 0) | 407/79 | A-timer | OnTimer<n>: R.npc_timer helper |  |
| `initnpctimer` | 825 (0, 0) | 392 (1, 0) | 376/78 | A-timer | R.npc_timer helper over timers.after; another NPC's timer needs B3 | 28090/11710 |
| `stopnpctimer` | 817 (1, 0) | 385 (0, 0) | 382/78 | A-timer | R.npc_timer:stop(); another NPC's needs B3 | 28091/11789 |
| `startnpctimer` | 38 (0, 0) | 38 (0, 0) | 0/0 | A-timer | R.npc_timer:start(); another NPC's needs B3 | 28092/11751 |
| `setnpctimer` | 37 (0, 0) | 37 (0, 0) | 0/0 | A-timer | R.npc_timer:set(tick) | 28093/11868 |
| `getnpctimer` | 5 (0, 0) | 1 (0, 0) | 4/0 | A-timer | R.npc_timer:get(type) | 28094/11823 |
| `deltimer` | 1 (0, 0) | 0 (0, 0) | 1/0 | A-timer | cancel the R helper timer | 28088/11675 |
| `addtimer` | 1 (0, 0) | 0 (0, 0) | 1/0 | A-timer | timers.after with a player check; the label runs through B3 when it is another NPC's | 28087/11652 |
| `getd` | 50 (12, 8) | 16 (2, 0) | 23/6 | A-var | runtime variable declaration (variables.*(name) at run time; 256 per mod limit) | 28296/18155 |
| `setd` | 36 (3, 3) | 17 (2, 2) | 20/5 | A-var | as getd | 28297/18016 |

### Category B

| Command or label | Renewal scripts (first, sole) | Pre-renewal scripts (first, sole) | In C folders R/P | Area | Mapping or proposal | script.cpp def/func |
|---|---:|---:|---:|---|---|---|
| `strnpcinfo` | 33 (11, 2) | 18 (5, 2) | 15/10 | B1 | npc.unique_name / hidden part | 28013/9248 |
| `getpartymember` | 5 (0, 0) | 1 (0, 0) | 4/0 | B1 | party member IDs | 28006/8972 |
| `sc_start2` | 4 (4, 0) | 0 (0, 0) | 4/0 | B1 | player handle of another character (B1 maps.find_player) | 28118/12401 |
| `convertpcinfo` | 1 (0, 0) | 1 (0, 0) | 0/0 | B1 | maps.find_player(name or ID) then fields | 28597/26690 |
| `getcharid (another character)` | 1 (1, 0) | 1 (1, 0) | 0/0 | B1 | another character | 28003/8884 |
| `attachrid` | 1 (0, 0) | 0 (0, 0) | 1/0 | B1 | maps.find_player / world.get_player(account_id): the handle replaces the attached player (0002 'Who a call acts on') | 28150/13507 |
| `requestguildinfo` | 1 (0, 0) | 0 (0, 0) | 1/0 | B1 | guilds.get loads a guild from the character server | 28171/14095 |
| `getbattleflag` | 75 (38, 16) | 65 (30, 15) | 59/58 | B10 | rules.get for exp rates and look limits; feature.* flags map to constants (A) | 28306/17053 |
| `gettime` | 46 (25, 15) | 33 (18, 14) | 8/2 | B10 | clock.get(field) | 28069/10908 |
| `gettimetick` | 12 (4, 1) | 9 (3, 1) | 3/1 | B10 | clock.milliseconds() / clock.now() | 28068/10862 |
| `gettimestr` | 1 (0, 0) | 1 (0, 0) | 0/0 | B10 | clock.format(format) | 28070/10929 |
| `jobcanentermap` | 2 (1, 0) | 0 (0, 0) | 1/0 | B10 | maps.can_enter(map, player) | 28528/25007 |
| `disablenpc` | 1328 (182, 48) | 628 (95, 39) | 650/118 | B2 | npc:hide() on the NPC or on world.get_npc(name) (approved name in 0002 'Comparison', Events table) | 28110/12339 |
| `enablenpc` | 1082 (39, 9) | 537 (30, 6) | 491/95 | B2 | npc:show() (approved name in 0002 'Comparison') | 28109/12339 |
| `cloakoffnpcself` | 252 (78, 54) | 0 (0, 0) | 54/0 | B2 | npc:set_cloaked(false, player) | 28115/12339 |
| `cloakonnpcself` | 250 (19, 8) | 0 (0, 0) | 61/0 | B2 | npc:set_cloaked(true, player) | 28116/12339 |
| `sprite (npcs.yml sprite: none (an NPC without a view, no click, events only))` | 116 (11, 11) | 99 (3, 3) | 9/0 | B2 | npcs.yml sprite: none (an NPC without a view, no click, events only) |  |
| `hideonnpc` | 162 (32, 17) | 51 (19, 12) | 85/3 | B2 | npc:set_hidden(true) | 28112/12339 |
| `state` | 160 (25, 25) | 0 (0, 0) | 25/0 | B2 | npcs.yml state: hidden | cloaked | disabled, then the events of B3 show it |  |
| `cloakonnpc` | 139 (101, 33) | 0 (0, 0) | 25/0 | B2 | npc:set_cloaked(true, player?) | 28114/12339 |
| `hideoffnpc` | 73 (10, 3) | 32 (7, 2) | 22/0 | B2 | npc:set_hidden(false) | 28111/12339 |
| `cloakoffnpc` | 40 (10, 8) | 0 (0, 0) | 8/0 | B2 | npc:set_cloaked(false, player?) | 28113/12339 |
| `unloadnpc` | 1 (1, 0) | 1 (1, 0) | 0/0 | B2 | npc:remove() | 28530/25658 |
| `isnpccloaked` | 1 (0, 0) | 0 (0, 0) | 0/0 | B2 | npc:is_cloaked(player?) | 28598/26738 |
| `event (custom labels)` | 1682 (1417, 2) | 952 (819, 1) | 811/262 | B3 | named event label (donpcevent, doevent, monster and timer labels) |  |
| `donpcevent` | 1157 (165, 87) | 709 (144, 79) | 448/129 | B3 | other:fire_event(label) or world.fire_npc_event(label) for "::OnX" | 28085/11615 |
| `floating` | 574 (84, 84) | 451 (66, 66) | 109/87 | B3 | npcs.yml script record without map (runs only through events) |  |
| `event ontouchnpc` | 30 (27, 0) | 8 (6, 0) | 3/0 | B3 | event monster_touch |  |
| `doevent` | 11 (7, 7) | 0 (0, 0) | 4/0 | B3 | other:fire_event(label, player) | 28084/11598 |
| `mobcount (monsters of another NPC's label)` | 7 (0, 0) | 0 (0, 0) | 0/0 | B3 | monsters of another NPC's label | 28220/14378 |
| `monster (death of a monster runs another NPC's label)` | 2 (0, 0) | 3 (0, 0) | 0/0 | B3 | death of a monster runs another NPC's label | 28078/11238 |
| `event onpckillevent` | 1 (0, 0) | 1 (0, 0) | 0/0 | B3 | npc contexts hear death (killer is a player) |  |
| `event onpclogoutevent` | 1 (1, 0) | 1 (1, 0) | 0/0 | B3 | npc contexts hear logout |  |
| `awake` | 1 (0, 0) | 1 (0, 0) | 1/1 | B3 | run the sleeping instances of an NPC: npc:fire_event wake | 28353/20436 |
| `event onpcdieevent` | 1 (0, 0) | 1 (0, 0) | 0/0 | B3 | npc contexts hear death (player) |  |
| `variables (variables.server_temporary_*)` | 310 (86, 60) | 237 (78, 56) | 77/37 | B4 | variables.server_temporary_* |  |
| `getvariableofnpc` | 4 (4, 3) | 2 (2, 1) | 0/0 | B4 | variables.npc_integer/string(name):get(npc) and :set(npc, v) | 28354/20483 |
| `npctalk` | 800 (275, 169) | 0 (0, 0) | 268/0 | B5 | say_aloud as another NPC (B1) or with a target or color | 28218/16193 |
| `emotion` | 401 (161, 111) | 119 (56, 40) | 49/1 | B5 | player:show_emotion / other NPC | 28162/13818 |
| `specialeffect` | 207 (14, 6) | 20 (6, 3) | 109/0 | B5 | npc:show_effect(e) converts today for AREA (A); SELF and ALL_SAMEMAP need npc:show_effect(e, { to }) and another NPC needs B1 | 28208/15596 |
| `unittalk` | 207 (12, 3) | 0 (0, 0) | 107/0 | B5 | player:say_aloud(text, { to }); npc:say_aloud for NPC IDs | 28347/20216 |
| `sprite (NPC view of a monster or job sprite)` | 112 (59, 59) | 71 (48, 48) | 8/3 | B5 | NPC view of a monster or job sprite |  |
| `soundeffect` | 36 (19, 13) | 31 (17, 13) | 2/0 | B5 | player:play_sound(file, { repeat }) | 28195/15281 |
| `setunittitle` | 51 (51, 8) | 0 (0, 0) | 8/0 | B5 | npc:set_title(text) | 28336/19944 |
| `setnpcdisplay` | 44 (2, 0) | 4 (0, 0) | 11/0 | B5 | npc:set_display_name(name) and npc:set_sprite(sprite) | 28278/17869 |
| `npcskill` | 41 (16, 9) | 6 (3, 2) | 5/0 | B5 | npc:use_skill(skill, level, player, { stat, npc_level }) | 28455/23286 |
| `soundeffectall` | 18 (1, 1) | 18 (1, 1) | 0/0 | B5 | npc:play_sound(file, { to }) / maps.play_sound | 28196/15308 |
| `progressbar_npc` | 33 (0, 0) | 0 (0, 0) | 21/0 | B5 | npc:show_progress(seconds, { color }) (waits) | 28389/22546 |
| `classchange` | 19 (7, 5) | 0 (0, 0) | 2/0 | B5 | npc:set_sprite(sprite) | 28191/15163 |
| `playbgm` | 17 (11, 1) | 0 (0, 0) | 14/0 | B5 | player:play_music(file) |  |
| `npcspeed` | 13 (1, 0) | 0 (0, 0) | 9/0 | B5 | npc:set_speed(ms) | 28223/16260 |
| `removespecialeffect` | 8 (1, 0) | 0 (0, 0) | 4/0 | B5 | npc:remove_effect(e) | 28210/15652 |
| `viewpointmap` | 6 (1, 0) | 0 (0, 0) | 6/0 | B5 | maps.mark_minimap(map, id, x, y, color) / maps.unmark_minimap | 27986/7013 |
| `movenpc` | 2 (0, 0) | 2 (0, 0) | 0/0 | B5 | npc:move_to(x, y) | 28216/16146 |
| `showdigit` | 1 (0, 0) | 0 (0, 0) | 1/0 | B5 | player:show_digits(value, kind) | 28394/22745 |
| `npcwalkto` | 1 (0, 0) | 0 (0, 0) | 1/0 | B5 | npc:walk_to(x, y) | 28224/16293 |
| `skilleffect` | 1 (0, 0) | 0 (0, 0) | 0/0 | B5 | entity:show_skill_effect(skill, level) | 28206/15519 |
| `playbgmall` | 1 (0, 0) | 0 (0, 0) | 1/0 | B5 | maps.play_music(map, file) |  |
| `dispbottom` | 1 (0, 0) | 0 (0, 0) | 0/0 | B5 | color | 28250/15796 |
| `removespecialeffect2` | 1 (0, 0) | 0 (0, 0) | 0/0 | B5 | player:remove_effect(e) | 28211/15652 |
| `setunitdata` | 55 (8, 0) | 8 (0, 0) | 30/8 | B5/B8 | npc:set_look{...} for UNPC_*; monster:set_hp, set_damage_immune, set_modes for UMOB_* | 28339/19398 |
| `unitwalk` | 25 (0, 0) | 0 (0, 0) | 22/0 | B5/B8 | npc:walk_to / monster:walk_to (exists) / player: refused (see D) | 28340/19996 |
| `unitwarp` | 4 (0, 0) | 0 (0, 0) | 0/0 | B5/B8 | npc:move_to / monster:warp_to(map,x,y) / player:warp (exists) | 28343/20075 |
| `unitstopwalk` | 3 (0, 0) | 0 (0, 0) | 1/0 | B5/B8 | npc:stop() / monster:stop() (exists) | 28346/20183 |
| `questinfo` | 491 (424, 74) | 0 (0, 0) | 105/0 | B6 | npc:add_quest_icon(icon, { mark, condition = function(player) }) | 28467/20949 |
| `questinfo_refresh` | 11 (0, 0) | 0 (0, 0) | 0/0 | B6 | player:refresh_quest_icons() | 28475/21043 |
| `open_quest_ui` | 8 (1, 0) | 0 (0, 0) | 0/0 | B6 | player:open_quest_log(quest?) | 28620 |
| `checkstatusquest` | 1 (0, 0) | 0 (0, 0) | 0/0 | B6 | R helper over get_quest_state (A) once B6 exists |  |
| `getexp` | 446 (129, 93) | 97 (54, 43) | 55/1 | B7 | player:give_exp(base, job) with quest_exp_rate | 28187/11142 |
| `parameter MaxWeight` | 343 (235, 89) | 96 (79, 45) | 24/0 | B7 | player.max_weight |  |
| `setpcblock` | 157 (53, 3) | 0 (0, 0) | 52/0 | B7 | player:set_blocked(action, blocked) | 28328/18964 |
| `disable_items` | 118 (87, 16) | 18 (17, 9) | 10/0 | B7 | conversation:lock_items() (rAthena npc_item_flag) | 27983/8828 |
| `parameter SkillPoint` | 56 (39, 29) | 31 (31, 26) | 2/0 | B7 | player.skill_points (read; writes need player:give_skill_points) |  |
| `eaclass` | 42 (35, 21) | 21 (20, 17) | 1/0 | B7 | jobs.class_flags(job) (player.job_class exists for the attached player) | 28361/12835 |
| `ismounting` | 20 (5, 3) | 4 (3, 2) | 0/0 | B7 | mounts and companions of decision 25 next step 11: dragon, mado gear, warg | 28440/22857 |
| `nude` | 14 (5, 2) | 7 (5, 2) | 3/1 | B7 | player:unequip_all() | 28212/15715 |
| `roclass` | 12 (0, 0) | 0 (0, 0) | 0/0 | B7 | jobs.from_class(class_flags, sex) | 28362/12853 |
| `parameter Weight` | 7 (7, 6) | 3 (3, 3) | 1/0 | B7 | player.weight |  |
| `getexp2` | 8 (0, 0) | 0 (0, 0) | 2/0 | B7 | player:give_exp(base, job, { raw = true }), negative takes | 28519/24713 |
| `readbook` | 5 (4, 3) | 3 (3, 2) | 2/2 | B7 | player:open_book(book, page) | 28385/20929 |
| `transform` | 7 (3, 1) | 0 (0, 0) | 5/0 | B7 | player:transform(monster, milliseconds) | 28494/23777 |
| `setlook` | 4 (1, 1) | 3 (1, 1) | 0/0 | B7 | player:set_look{...} | 27953/6860 |
| `getlook` | 2 (2, 1) | 4 (4, 2) | 0/0 | B7 | player.look fields (hair_style, hair_color, cloth_color, head_top, ...) | 28221/16377 |
| `checkdragon` | 4 (2, 1) | 1 (1, 1) | 0/0 | B7 | player.mount == mounts.id 'dragon' once the dragon mount exists | 28438/22803 |
| `skill` | 3 (1, 1) | 2 (1, 1) | 0/0 | B7 | give_skill kind permanent_grant | 28046/10372 |
| `heal` | 4 (0, 0) | 1 (0, 0) | 3/1 | B7 | player:heal { hp, sp } (absolute amounts, as heal; item_heal adds bonuses) | 27987/6007 |
| `checkwug` | 2 (1, 1) | 1 (1, 1) | 0/0 | B7 | warg companion | 28063/10746 |
| `checkmadogear` | 2 (0, 0) | 1 (0, 0) | 0/0 | B7 | mado gear mount | 28064/10765 |
| `pushpc` | 2 (1, 0) | 1 (1, 0) | 2/1 | B7 | player:push(direction, cells) | 28390/22604 |
| `setdragon` | 3 (0, 0) | 0 (0, 0) | 0/0 | B7 | player:set_mount(dragon) | 28439/22824 |
| `parameter killedrid` | 3 (0, 0) | 0 (0, 0) | 2/0 | B7 | the death event's killer (needs B3 OnPCDieEvent) |  |
| `enable_items` | 2 (0, 0) | 0 (0, 0) | 0/0 | B7 | conversation:unlock_items() | 27982/8820 |
| `getinventorylist` | 1 (0, 0) | 1 (0, 0) | 0/0 | B7 | player:get_inventory() (named in 0002 'Comparison', not built) | 28188/15029 |
| `getsavepoint` | 1 (0, 0) | 1 (0, 0) | 0/0 | B7 | player:get_save_point() → map, x, y | 28222/16412 |
| `parameter Hp` | 1 (1, 0) | 0 (0, 0) | 1/0 | B7 | player:set_hp(n) (Hp = n) |  |
| `setmadogear` | 1 (0, 0) | 0 (0, 0) | 0/0 | B7 | mado gear mount | 28065/10785 |
| `checkoption` | 1 (1, 0) | 0 (0, 0) | 0/0 | B7 | player:has_option(name) (cart, falcon, riding, hide, ...) | 28055/10526 |
| `readparam` | 1 (0, 0) | 0 (0, 0) | 0/0 | B7 | player fields by name | 28002/8842 |
| `killmonster` | 435 (2, 2) | 234 (2, 2) | 158/37 | B8 | spawning: monsters:remove() / spawning.remove(label) for monsters of a label; the converter records the spawn list | 28081/11480 |
| `monster (spawning.monsters on another map)` | 122 (0, 0) | 108 (0, 0) | 0/0 | B8 | spawning.monsters on another map | 28078/11238 |
| `monster (spawning.monsters { display_name })` | 145 (11, 9) | 57 (2, 1) | 37/10 | B8 | spawning.monsters { display_name } | 28078/11238 |
| `mapwarp` | 94 (6, 4) | 58 (4, 4) | 27/16 | B8 | maps.warp_players(from, to, x, y, { guild/party filter }) | 28213/14317 |
| `killmonsterall` | 40 (1, 1) | 39 (1, 1) | 5/4 | B8 | maps.remove_monsters(map) (script-spawned and normal?) | 28082/11521 |
| `areawarp` | 35 (0, 0) | 32 (0, 0) | 13/10 | B8 | maps.warp_players with an area | 27950/5695 |
| `getmapusers` | 40 (2, 2) | 22 (2, 2) | 24/9 | B8 | maps.count_players(map) (any map, any zone) | 28103/12146 |
| `setcell` | 27 (0, 0) | 5 (0, 0) | 22/4 | B8 | maps.set_cells(map, x1, y1, x2, y2, kind, on) | 28371/20631 |
| `unitskilluseid` | 29 (0, 0) | 0 (0, 0) | 29/0 | B8 | monster:use_skill (exists); NPC caster needs npc:use_skill | 28348/20250 |
| `getmonsterinfo` | 19 (0, 0) | 1 (0, 0) | 2/0 | B8 | monsters.get(monster) fields (name, level, hp, race, element, size, exp, ...) | 28311/18711 |
| `areapercentheal` | 10 (0, 0) | 10 (0, 0) | 10/10 | B8 | maps.find_entities over a rectangle then player:heal_percent; cross-map needs B8 | 28409/5746 |
| `unitskillusepos` | 19 (0, 0) | 0 (0, 0) | 19/0 | B8 | monster:use_skill_at(skill, level, x, y) | 28349/20305 |
| `mobcount (count of a label's monsters on another map)` | 9 (0, 0) | 7 (0, 0) | 0/0 | B8 | count of a label's monsters on another map | 28220/14378 |
| `getareausers` | 3 (1, 0) | 2 (1, 0) | 0/0 | B8 | maps.count_players(map, { rectangle }) | 28104/12167 |
| `removemapflag` | 2 (0, 0) | 2 (0, 0) | 0/0 | B8 | maps.set_flag(map, flag, false) | 28157/13683 |
| `warpparty` | 3 (0, 0) | 0 (0, 0) | 3/0 | B8 | party:warp(map, x, y) with rAthena's map checks (decision 27 next step 13) | 27951/5781 |
| `setmapflag` | 1 (0, 0) | 1 (0, 0) | 0/0 | B8 | maps.set_flag(map, flag, value) | 28156/13617 |
| `unitkill` | 2 (1, 0) | 0 (0, 0) | 2/0 | B8 | monster:kill() | 28342/20061 |
| `areaannounce` | 1 (0, 0) | 1 (0, 0) | 0/0 | B8 | maps.announce(map, text, { rectangle }) | 28100/12050 |
| `smonster` | 1 (0, 0) | 1 (0, 0) | 0/0 | B8 | spawning with a monster's own size and AI (monster name variant) |  |
| `checkcell` | 2 (0, 0) | 0 (0, 0) | 0/0 | B8 | maps.check_cell(map, x, y, kind) | 28370/20614 |
| `getfreecell` | 1 (0, 0) | 0 (0, 0) | 0/0 | B8 | maps.find_free_cell(map, options) | 28372/20657 |
| `areamobuseskill` | 1 (0, 0) | 0 (0, 0) | 1/0 | B8 | maps.find_entities + monster:use_skill (A once cross-map is B8) | 28387/22447 |
| `makeitem` | 31 (2, 2) | 20 (2, 2) | 8/0 | B9 | maps.drop_item(map, x, y, item, amount) | 27971/8165 |
| `getitem2` | 38 (0, 0) | 1 (0, 0) | 4/0 | B9 | give_item options refine, cards, broken (GiveItemOptions) | 27968/7775 |
| `delequip` | 31 (0, 0) | 0 (0, 0) | 3/0 | B9 | equipped:remove() (deletes the equipped copy) | 28457/9885 |
| `getiteminfo` | 20 (0, 0) | 3 (0, 0) | 0/0 | B9 | items.get(item) fields (type, weight, slots, look, ...) | 28281/14761 |
| `rentalcountitem` | 11 (4, 2) | 0 (0, 0) | 7/0 | B9 | count_item { rentals = true } once rentals expire | 28600/7427 |
| `getequipisenableref` | 8 (0, 0) | 1 (0, 0) | 0/0 | B9 | items.can_refine(item) | 28021/9539 |
| `getequippercentrefinery` | 8 (0, 0) | 1 (0, 0) | 0/0 | B9 | items.refine_chance(item, refine, { enriched }) | 28025/9684 |
| `consumeitem` | 7 (1, 1) | 1 (1, 1) | 0/0 | B9 | player:run_item_script(item) | 28456/23351 |
| `getequiprefinecost` | 7 (0, 0) | 1 (0, 0) | 0/0 | B9 | items.refine_cost(item, kind)  | 28580/26037 |
| `getnameditem` | 4 (1, 1) | 3 (1, 1) | 0/0 | B9 | give_item option signed_by = player | 27969/8085 |
| `downrefitem` | 3 (0, 0) | 0 (0, 0) | 0/0 | B9 | equipped:refine(-amount) | 28028/9836 |
| `getbrokenid` | 2 (2, 0) | 1 (1, 0) | 0/0 | B9 | player:get_broken_items() | 28017/9422 |
| `makeitem2` | 3 (0, 0) | 0 (0, 0) | 3/0 | B9 | maps.drop_item with refine and cards | 27972/8237 |
| `failedrefitem` | 2 (0, 0) | 1 (0, 0) | 0/0 | B9 | equipped:break_refine() (destroys, as failedrefitem) | 28027/9802 |
| `repairall` | 2 (0, 0) | 1 (0, 0) | 0/0 | B9 | player:repair_all() | 28019/9481 |
| `successrefitem` | 2 (0, 0) | 0 (0, 0) | 0/0 | B9 | equipped:refine(amount) | 28026/9728 |
| `countitem2` | 1 (0, 0) | 1 (0, 0) | 0/0 | B9 | count_item options refine and cards | 27996/7224 |
| `getitemslots` | 1 (0, 0) | 0 (0, 0) | 0/0 | B9 | items.get(item).slots | 28185/14743 |
| `mergeitem` | 1 (1, 1) | 0 (0, 0) | 0/0 | B9 | player:merge_items() | 28507/24112 |

### Category C

| Command or label | Renewal scripts (first, sole) | Pre-renewal scripts (first, sole) | In C folders R/P | Area | Mapping or proposal | script.cpp def/func |
|---|---:|---:|---:|---|---|---|
| `achievementinfo` | 14 (4, 3) | 0 (0, 0) | 14/0 | C:achievements | achievements | 28573/25890 |
| `achievementexists` | 3 (0, 0) | 0 (0, 0) | 3/0 | C:achievements | achievements | 28577/25951 |
| `openauction` | 1 (1, 0) | 1 (1, 0) | 1/1 | C:auction | auctions (0002 decision 30: later) | 28369/20592 |
| `openbank` | 1 (1, 1) | 0 (0, 0) | 1/0 | C:bank | bank | 28621/27080 |
| `bg_monster` | 20 (0, 0) | 20 (0, 0) | 20/20 | C:battlegrounds |  | 28404/21431 |
| `getcharid (getcharid(4) battleground, (5) clan)` | 22 (8, 8) | 18 (8, 8) | 22/18 | C:battlegrounds | getcharid(4) battleground, (5) clan | 28003/8884 |
| `bg_warp` | 17 (0, 0) | 17 (0, 0) | 17/17 | C:battlegrounds |  | 28403/21416 |
| `waitingroom2bg` | 14 (0, 0) | 14 (0, 0) | 14/14 | C:battlegrounds | battleground teams | 28400/21211 |
| `bg_leave` | 12 (0, 0) | 12 (0, 0) | 12/12 | C:battlegrounds |  | 28406/21468 |
| `bg_updatescore` | 9 (0, 0) | 9 (0, 0) | 9/9 | C:battlegrounds |  | 28412/21517 |
| `bg_get_data` | 7 (0, 0) | 7 (0, 0) | 7/7 | C:battlegrounds |  | 28410/21535 |
| `bg_team_setxy` | 2 (0, 0) | 2 (0, 0) | 2/2 | C:battlegrounds |  | 28402/21403 |
| `waitingroom` | 47 (9, 8) | 38 (4, 3) | 47/38 | C:chat_rooms | NPC chat rooms | 28139/13089 |
| `warpwaitingpc` | 25 (0, 0) | 21 (0, 0) | 25/21 | C:chat_rooms | NPC chat rooms | 28149/13283 |
| `disablewaitingroomevent` | 24 (0, 0) | 21 (0, 0) | 24/21 | C:chat_rooms | NPC chat rooms | 28145/13208 |
| `enablewaitingroomevent` | 24 (0, 0) | 21 (0, 0) | 24/21 | C:chat_rooms | NPC chat rooms | 28144/13189 |
| `getwaitingroomstate` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:chat_rooms | NPC chat rooms | 28148/13237 |
| `item_enchant` | 4 (0, 0) | 0 (0, 0) | 4/0 | C:enchant_ui | item enchant window | 28631/27391 |
| `item_reform` | 4 (0, 0) | 0 (0, 0) | 4/0 | C:enchant_ui | item reform window | 28630/27334 |
| `enchantgradeui` | 2 (0, 0) | 0 (0, 0) | 2/0 | C:enchant_ui | enchant grade window | 28624 |
| `refineui` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:enchant_ui | refine window | 28581/26764 |
| `gethominfo` | 3 (0, 0) | 0 (0, 0) | 3/0 | C:homunculus | homunculus | 28253/15998 |
| `checkhomcall` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:homunculus | homunculus | 28360/12816 |
| `hommutate` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:homunculus | homunculus | 28357/12721 |
| `morphembryo` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:homunculus | homunculus | 28358/12760 |
| `instance` | 1392 (243, 243) | 98 (0, 0) | 1392/98 | C:instances | NPC on an instance map |  |
| `variables (instance variable 'x)` | 736 (262, 0) | 9 (2, 0) | 736/9 | C:instances | instance variable 'x |  |
| `event oninstanceinit` | 304 (233, 0) | 74 (74, 0) | 304/74 | C:instances | OnInstanceInit |  |
| `instance_npcname` | 259 (14, 0) | 2 (0, 0) | 259/2 | C:instances |  | 28424/21809 |
| `instance_mapname` | 185 (5, 0) | 64 (4, 0) | 185/64 | C:instances |  | 28425/21839 |
| `instance_enter` | 57 (14, 13) | 4 (1, 1) | 57/4 | C:instances |  | 28423/21783 |
| `instance_create` | 55 (23, 12) | 3 (0, 0) | 55/3 | C:instances |  | 28420/21702 |
| `instance_warpall` | 9 (0, 0) | 3 (0, 0) | 9/3 | C:instances |  | 28426/21939 |
| `instance_live_info` | 12 (3, 0) | 0 (0, 0) | 12/0 | C:instances |  | 28432/22299 |
| `instance_check_party` | 5 (4, 1) | 4 (3, 1) | 5/4 | C:instances |  | 28428/22018 |
| `instance_announce` | 9 (0, 0) | 0 (0, 0) | 9/0 | C:instances |  | 28427/21982 |
| `instance_id` | 2 (0, 0) | 0 (0, 0) | 2/0 | C:instances |  | 28422/21864 |
| `instance_destroy` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:instances |  | 28421/21757 |
| `getpartnerid` | 22 (11, 6) | 22 (11, 6) | 22/22 | C:marriage | marriage | 28179/14470 |
| `divorce` | 1 (0, 0) | 1 (0, 0) | 1/1 | C:marriage | marriage | 28177/14437 |
| `getmercinfo` | 11 (5, 0) | 0 (0, 0) | 11/0 | C:mercenaries | mercenaries | 28255/16053 |
| `mercenary_set_faith` | 1 (0, 0) | 1 (0, 0) | 1/1 | C:mercenaries | mercenaries | 28384/20891 |
| `mercenary_get_faith` | 1 (0, 0) | 1 (0, 0) | 1/1 | C:mercenaries | mercenaries | 28382/20864 |
| `mercenary_create` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:mercenaries | mercenaries | 28377/20729 |
| `navigateto` | 97 (3, 2) | 0 (0, 0) | 97/0 | C:navigation | client navigation (navi data) | 28516/24491 |
| `get_reputation_points` | 13 (1, 1) | 0 (0, 0) | 13/0 | C:reputation | reputation | 28628/27281 |
| `add_reputation_points` | 11 (0, 0) | 0 (0, 0) | 11/0 | C:reputation | reputation | 28629/27305 |
| `flagemblem` | 83 (1, 0) | 72 (0, 0) | 83/72 | C:sieges | castles | 28167/13994 |
| `getcastledata` | 53 (8, 0) | 41 (6, 0) | 53/41 | C:sieges | castles | 28169/14029 |
| `event onagitinit2` | 34 (34, 0) | 34 (34, 0) | 34/34 | C:sieges | WoE event |  |
| `setwall` | 7 (0, 0) | 7 (0, 0) | 7/7 | C:sieges | castle walls (also B8 setcell) | 28373/14633 |
| `setcastledata` | 9 (0, 0) | 5 (0, 0) | 9/5 | C:sieges | castles | 28170/14072 |
| `delwall` | 4 (0, 0) | 4 (0, 0) | 4/4 | C:sieges | castle walls | 28374/14654 |
| `checkwall` | 4 (0, 0) | 4 (0, 0) | 4/4 | C:sieges | castle walls | 28375/14666 |
| `getcastlename` | 6 (0, 0) | 2 (0, 0) | 6/2 | C:sieges | castles | 28168/14020 |
| `agitcheck2` | 3 (0, 0) | 3 (0, 0) | 3/3 | C:sieges | sieges | 28398/13975 |
| `maprespawnguildid` | 3 (0, 0) | 2 (0, 0) | 3/2 | C:sieges | guild warp off a castle | 28163/13875 |
| `event onguildbreak` | 3 (0, 0) | 2 (0, 0) | 3/2 | C:sieges | WoE event |  |
| `event onagitinit` | 2 (1, 0) | 2 (1, 0) | 2/2 | C:sieges | WoE event |  |
| `guardian` | 2 (0, 0) | 2 (0, 0) | 2/2 | C:sieges | guardians | 28198/14597 |
| `gvgoff` | 2 (0, 0) | 2 (0, 0) | 2/2 | C:sieges | sieges | 28161/13763 |
| `woetetimestart` | 3 (0, 0) | 0 (0, 0) | 3/0 | C:sieges | WoE TE |  |
| `agitcheck3` | 3 (0, 0) | 0 (0, 0) | 3/0 | C:sieges | sieges | 28537/13985 |
| `agitcheck` | 2 (0, 0) | 1 (0, 0) | 2/1 | C:sieges | sieges | 28166/13965 |
| `event onagitend` | 1 (0, 0) | 1 (0, 0) | 1/1 | C:sieges | WoE event |  |
| `event onagitstart2` | 1 (0, 0) | 1 (0, 0) | 1/1 | C:sieges | WoE event |  |
| `event onagitinit3` | 2 (2, 0) | 0 (0, 0) | 2/0 | C:sieges | WoE event |  |
| `event onagitstart` | 1 (1, 0) | 1 (1, 0) | 1/1 | C:sieges | WoE event |  |
| `event onagitbreak` | 1 (0, 0) | 1 (0, 0) | 1/1 | C:sieges | WoE event |  |
| `gvgon` | 1 (0, 0) | 1 (0, 0) | 1/1 | C:sieges | sieges | 28160/13746 |
| `event onagitend2` | 1 (0, 0) | 1 (0, 0) | 1/1 | C:sieges | WoE event |  |
| `guardianinfo` | 1 (0, 0) | 1 (0, 0) | 1/1 | C:sieges | guardians | 28199/14681 |
| `event onagitstart3` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:sieges | WoE event |  |
| `gvgoff3` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:sieges | sieges | 28539/13797 |
| `woetimer` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:sieges | WoE |  |
| `event onagitend3` | 1 (0, 0) | 0 (0, 0) | 1/0 | C:sieges | WoE event |  |
| `openstylist` | 6 (6, 6) | 0 (0, 0) | 6/0 | C:stylist | stylist window | 28610 |

### Category D

| Command or label | Renewal scripts (first, sole) | Pre-renewal scripts (first, sole) | In C folders R/P | Area | Mapping or proposal | script.cpp def/func |
|---|---:|---:|---:|---|---|---|
| `setbattleflag` | 1 (0, 0) | 0 (0, 0) | 0/0 | D | changing the server's battle config from a script: no API, rules are read-only (0002 decision 32) | 28305/17011 |
| `atcommand` | 1 (1, 1) | 0 (0, 0) | 0/0 | D | runs GM commands from a script: no GM command API for scripts (0002 decision 35 keeps GM commands staff-only) | 28214/15787 |

### Category E

| Command or label | Renewal scripts (first, sole) | Pre-renewal scripts (first, sole) | In C folders R/P | Area | Mapping or proposal | script.cpp def/func |
|---|---:|---:|---:|---|---|---|
| `callfunc F_time_limit_recorder` | 114 (113, 97) | 0 (0, 0) | 0/0 | E1 | parser | 27942/5404 |
| `parse (set on a non-variable, other syntax)` | 27 (27, 27) | 21 (21, 21) | 7/5 | E1 | parser |  |
| `getarg` | 166 (41, 19) | 44 (23, 15) | 37/1 | E2 | getarg in a callsub part that the main body also holds (unreachable after end) | 27944/5518 |
| `callsub` | 33 (1, 1) | 2 (0, 0) | 21/0 | E2 | callsub to a label of another event part | 27941/5465 |
| `assignment in an expression` | 29 (1, 1) | 5 (1, 1) | 23/2 | E2 | x = y used as a value |  |
| `function` | 20 (8, 0) | 11 (3, 0) | 4/0 | E2 | local functions (function name; ... function name { }) |  |
| `variables (variable name starting with a digit)` | 20 (7, 4) | 2 (1, 0) | 10/0 | E2 | variable name starting with a digit |  |
| `labels` | 16 (9, 1) | 6 (4, 0) | 1/0 | E2 | a label inside a block |  |
| `goto` | 13 (0, 0) | 6 (0, 0) | 1/0 | E2 | goto into another event part | 27940/5386 |
| `getargcount` | 18 (0, 0) | 0 (0, 0) | 5/0 | E2 | getargcount in a part reached by callsub but also written in the main body | 28446/22899 |
| `getarraysize` | 4 (2, 0) | 2 (1, 0) | 0/0 | E2 | getarraysize of getd(...) or of a computed reference | 27960/6482 |
| `setarray` | 2 (1, 0) | 2 (1, 0) | 2/2 | E2 | setarray on getd(...) | 27957/6285 |
| `set` | 1 (0, 0) | 1 (0, 0) | 0/0 | E2 | set on a computed reference | 27955/6201 |
| `unknown_d` | 54 (54, 54) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_infoboard` | 36 (36, 36) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_navi` | 35 (35, 17) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `kick_mob` | 15 (0, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `weightbigquest` | 8 (8, 6) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_queststatus` | 7 (0, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `cell` | 3 (0, 0) | 3 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `que_dic` | 6 (0, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `pranksterdiag` | 6 (6, 6) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `wallnotes` | 5 (5, 5) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `excavator_part2` | 5 (0, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `catwarp` | 2 (0, 0) | 2 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `checkburglary` | 4 (4, 4) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `check_sauce` | 4 (4, 4) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_kids_roegenburg_quest` | 3 (1, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `171_worker_talk` | 3 (0, 0) | 0 (0, 0) | 1/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `eclageinentrance` | 3 (3, 2) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_banquet_main_4_spica` | 2 (0, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_getnumsuffix` | 1 (0, 0) | 1 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `func_juphist` | 1 (0, 0) | 1 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `jobname` | 2 (2, 2) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) | 27946/6123 |
| `rname` | 1 (0, 0) | 1 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_rand` | 2 (0, 0) | 0 (0, 0) | 1/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_equip_menu` | 1 (0, 0) | 0 (0, 0) | 1/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `eldicastes_dp` | 1 (0, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `brief` | 1 (0, 0) | 0 (0, 0) | 1/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_enchant` | 1 (0, 0) | 0 (0, 0) | 1/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `mob_brief` | 1 (0, 0) | 0 (0, 0) | 1/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `equip_check` | 1 (0, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `effect_gen` | 1 (0, 0) | 0 (0, 0) | 1/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `choice` | 1 (0, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_ischarm` | 1 (0, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `module_check` | 1 (0, 0) | 0 (0, 0) | 0/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
| `f_insertcomma` | 1 (1, 0) | 0 (0, 0) | 1/0 | E3 | function called by name without callfunc (rAthena parse_callfunc, script.cpp 960) |  |
