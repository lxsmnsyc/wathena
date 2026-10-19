# 0051: Classic Mode NPCs, Touch Areas and Bosses

- Status: proposed.
- Date: 2026-10-19.
- Code: [tools/convert](../../tools/convert) (`npc.go`, `npcscript.go`, `npclua.go`, `npcexpr.go`, `npcreport.go`, `parse.go`, `ast.go`, `render.go`, `reference.go`, `rathena.lua`, `npc_touch_test.go`, `npc_runtime_test.go`), [server/zone](../../server/zone) (`touch.go`, `zone.go`, `scripts.go`, `monster.go`, `combat.go`, `loot.go`, `touch_test.go`, `kafra_test.go`, `mobai_test.go`), [server/modscript](../../server/modscript) (`npcs.go`, `runtime.go`, `luaapi/spec.go`), [server/mod](../../server/mod) (`npcs.go`), [server/gamedata](../../server/gamedata) (`gamedata.go`, `mods.go`, `spawns_boss_test.go`), [server/cmd/waserver](../../server/cmd/waserver) (`main.go`), [reference/rathena](../../reference/rathena) (`embed.go`, the part files of `npcs.json`).
- Docs: [mods/README.md](../../mods/README.md) ("NPCs: data/npcs.yml", "Touch areas", "Core's NPCs"), [tools/convert/README.md](../../tools/convert/README.md), [server/README.md](../../server/README.md) ("Spawns", "NPCs"), [reference/rathena/README.md](../../reference/rathena/README.md) ("NPCs"), [deploy/README.md](../../deploy/README.md) ("Settings"), the coverage reports in [docs/generated](../generated).
- Phase 8 step 6, part A (strategy section 15, "Phase 8"), and review decision 55 of [proposal 0002](../proposals/0002-lua-api.md).
- Lua API: the NPC record field `touch`, and the events `touch` and `untouch`, as decision 55 approved. No other change. Protocol: none. Settings: none.
- Part B: [decision 0052](0052-classic-mode-part-b.md) builds review decision 56: touch handlers with a conversation, negative `player:heal_percent`, and gzipped extracts in the release binaries. It also has the full kRO run, the browser check and the release notes list. What this record says of touch dialogs and Niflheim's `percentheal` was true until then.

## Context

The first release ships classic mode with `mode: renewal` (decision 55). Classic worlds placed the NPCs of seven rAthena folders, ran no `OnTouch` labels, said that Kafra pushcart rental was not available, and dropped the boss flag and respawn variance of spawn lines. Part A builds the server side of decision 55. Part B, the full kRO run, the browser check and the release notes, comes next.

## Decision

### More NPC folders

- `tools/wathena/extract.py` in the rAthena fork keeps the script NPCs of the `quests`, `warps` and `events` folders, below `npc/`, `npc/re/` and `npc/pre-re/`. Instances, guild castles and battlegrounds stay out, for phase 10.
- Their scripts would grow `npcs.json` from 8.3 MB to about 24 MB in renewal. They go to part files instead: `npcs-<folder>-<n>.json`, each with at most 1.9 MB of rows. `npcs.json` lists them in `parts`. Its rows did not change, only its header.
- A duplicate goes in the file of its source. The converter reads the part files after `npcs.json`, so the record names of the NPCs already converted stay the same.
- `extract.py npcs` writes `warps.json`, `npcs.json`, its part files and `item_prices.json` alone. Two runs give the same bytes.
- `waserver` and `wactl` embed the part files with the other extracts.

| Mode | Part files | Size | `quests` | `warps` | `events` |
|---|---:|---:|---:|---:|---:|
| renewal | 11 | 16.3 MB | 6,162 | 250 | 150 |
| pre-renewal | 7 | 8.6 MB | 2,942 | 81 | 150 |

The counts are script NPC lines, duplicates too. Three of the renewal `quests` lines are duplicates in `cities` files whose source is a quest NPC, which `npcs.json` left out before.

### What loads

What converts loads, and what does not is refused and listed in the coverage report, `docs/generated/npc-scripts-<mode>.md`. The report has a new table by folder and one for touch areas. Script NPC lines by folder, duplicates too, before and after part A:

| Mode | Folder | Lines before | Placed before | Lines after | Placed after |
|---|---|---:|---:|---:|---:|
| renewal | airports | 63 | 52 | 63 | 53 |
| renewal | cities | 944 | 676 | 947 | 680 |
| renewal | events | 0 | 0 | 150 | 110 |
| renewal | guides | 95 | 11 | 95 | 11 |
| renewal | jobs | 1,804 | 399 | 1,804 | 428 |
| renewal | kafras | 97 | 87 | 97 | 87 |
| renewal | merchants | 690 | 218 | 690 | 223 |
| renewal | other | 3,466 | 207 | 3,466 | 565 |
| renewal | quests | 463 | 431 | 6,622 | 2,044 |
| renewal | warps | 0 | 0 | 250 | 100 |
| pre-renewal | airports | 59 | 48 | 59 | 49 |
| pre-renewal | cities | 679 | 533 | 682 | 536 |
| pre-renewal | events | 0 | 0 | 150 | 110 |
| pre-renewal | guides | 43 | 43 | 43 | 43 |
| pre-renewal | jobs | 1,027 | 122 | 1,027 | 317 |
| pre-renewal | kafras | 71 | 62 | 71 | 62 |
| pre-renewal | merchants | 101 | 52 | 101 | 52 |
| pre-renewal | other | 3,052 | 171 | 3,052 | 529 |
| pre-renewal | quests | 0 | 0 | 2,939 | 850 |
| pre-renewal | warps | 0 | 0 | 81 | 72 |

- A line counts in the folder of its own file. The `quests` lines before part A were duplicates in quest files of NPCs from other folders. renewal also has `instances` (671 lines, 4 placed) and `guild3` (10, none), which did not change.
- In all, renewal places 4,587 NPCs of 15,337 lines, up from 2,367 of 8,775, and pre-renewal 2,805 of 8,390, up from 1,216 of 5,217. The counts include shops.
- `other` and `jobs` gained most from touch areas and the parser fixes below: their warp portals and the job tests with `if (a) || (b)` and statements before a `case`.
- The main reasons the rest does not convert, in the report's command table: dialogs in touch labels, `donpcevent`, `OnEnable` and `OnDisable`, the cloak and enable commands, `questinfo`, and functions that do not convert, such as `F_time_limit_recorder` of the Verus warps.

The parser now reads three things as rAthena does, which many quest scripts need:

- Statements before the first `case` of a `switch` run after the value is read and before the cases are compared. rAthena compiles a switch as the value, the statements in order, and a test before each case.
- The condition of `if`, `while` and `switch` is the whole expression from its `(`, as `parse_expr` reads it, so `if (a) || (b)` is one condition.
- Digits before a letter or `_` start a word, as `is_number` says, such as the sprite constant `4_M_CACTUSMAN20`.

Item scripts parse the same. A `switch` with statements before its first case does not convert in an item script.

Other fixes the new scripts showed:

- `R.checkquest` and `R.isbegin_quest` read a computed quest ID that `data/quests.yml` lacks as a quest the player does not have. rAthena's `quest_check` looks only in the player's log.
- A display name with a space before a `?`, such as `Adventurer ?`, is quoted in `data/npcs.yml`. YAML read it as a mapping key.
- `TestRunConvertedNPCs` runs each map's NPCs in a Lua state of their own, as each zone does. In one state for all maps, core went past the 256 variables a mod may declare in one zone. The largest map stays under it.
- `TestParseNPCScripts` allows `set` on a reference that is not a variable, such as `set getvariableofnpc(.x, "NPC"), 1;`, which the parser does not read yet, and three other scripts, as before. Those three are script errors that rAthena rejects too, such as a colon for a semicolon.

### Touch areas

The record field and the events of decision 55:

- `touch: {x_span, y_span}` on an NPC record, each 0 to 100 cells. It needs a `script`.
- `touch` runs with the player when it steps into the area, once until it leaves. `untouch` runs when it leaves: a step out, a warp, or a logout. They run only in the NPC's own context. A player who left the map can only be compared.

The zone follows `npc_touch_area_allnpc`:

- It checks a player's cell after each step, and once on the cell it arrives on, after its first frame, as `unit_walktoxy_timer` and `clif_parse_LoadEndAck` do. A teleport within the map counts as an arrival.
- A step onto a warp that takes the player checks no touch area. rAthena sorts warps first and stops at a warp that moves the player.
- Leaving is checked before entering, as `npc_touchnext_areanpc` runs before `npc_touch_areanpc`. The areas come in the order the NPCs were placed.
- Hidden and dead players fire `touch`: `npc_ontouch2_event`, rAthena's `OnTouch`, checks neither.
- The events run with the zone's script events, after the walks. A `touch` whose player left the area or the zone by then does not run, as `npc_event_sub` checks the area again.
- They run as a click does, so a mod over its CPU budget still runs them (review decision 53).

The converter:

- `OnTouch` becomes `events.on("touch", ...)`.
- `OnTouch_` becomes `R.touch_one(...)`, which keeps rAthena's `touching_id`: one player at a time, not a hiding or dead player (`pc_ishiding`, through `player:has_status`), and the next player in the area who can when that one leaves. rAthena picks that player by its place in the map's blocks. `R.touch_one` picks the one that came in first.
- `OnTouch_` wins over `OnTouch`, as `npc_touch_areanpc` runs `OnTouch` only for an NPC without `OnTouch_`.
- The line's span becomes the record's `touch` field. A line with a span below 0 gets none. A line without a sprite (-1) that has a touch area gets `HIDDEN_NPC`, which shows nothing.
- A touch has no conversation. A touch label with a dialog, or that calls a function with one, does not convert, and the report says so.

Coverage, from the reports:

| Mode | Scripts with a touch label | Converted with a touch area | Lines with a touch label | Lines with a touch area | Warp sprite lines | Placed | With a touch area |
|---|---:|---:|---:|---:|---:|---:|---:|
| renewal | 1,265 | 46 | 4,935 | 447 | 2,089 | 143 | 54 |
| pre-renewal | 682 | 28 | 3,974 | 588 | 1,392 | 108 | 27 |

- A dialog blocks 288 touch scripts in renewal and 188 in pre-renewal. The others fail first on other commands.
- rAthena clicks an NPC that has a touch area and no touch label when a player steps into it. wAthena does not, since a touch has no conversation. Such NPCs convert as click NPCs.
- rAthena queues a touch event while the player talks to another NPC. wAthena runs it at once, since it has no dialog.
- rAthena runs `OnTouch_` when a player in the area stops hiding (`SCF_ONTOUCH`). wAthena does not: no event tells the NPC.

### Bosses

Classic spawns follow `npc_parse_mob` and `mob_setdelayspawn`:

- A spawn keeps the line's `boss` flag and its variance. `mob_spawn_variance` is 1, so only `boss_monster` lines keep it.
- A line's delay below `mob_respawn_time`, 1 second, or no delay is 5 seconds. A delay or variance above `0xfffffff` skips the line.
- Each death adds a random part of the variance, `rnd() % delay2`, and the delay is at least 1 second. `boss_spawn_delay`, `plant_spawn_delay` and `mob_spawn_delay` are 100, which changes nothing.
- A `boss_monster` line with a span of 0 or 1, which `npc_parse_mob` makes 1, places its monster on its cell without a search, as `mob_spawn`. A cell that cannot be walked on places it anywhere, as `npc_parse_mob` does.
- A monster that changes class keeps the flag and the variance of its spawn.

What rAthena announces when an MVP dies (`mob_dead`):

- `clif_mvp_effect`: the MVP effect over the MVP player, to the players in its area. wAthena already sent it.
- `clif_mvp_item`: the prize, to the MVP player. wAthena now sends the client's line, "Congratulations! You are the MVP! Your reward item is" and the item's name.
- `clif_mvp_exp` sends nothing at `PACKETVER` 20211103 with `mvp_exp_reward_message: no`.
- `rare_drop_announce` is 0, so no prize goes to the world. rAthena has no world announcement of an MVP's spawn or death by default.
- The MVP tomb (`mvp_tomb_enabled`) is not built.

### Kafras

- Before: `F_KafCart` was in the converter's `unavailableFunctions`, so the menu said "Pushcart rental is not available yet". `F_KafStor` and `F_KafEnd` had one stub each, a `percentheal` with a negative amount. So every city Kafra converted partly.
- Now: `F_KafCart` converts whole, with `BaseClass`, `checkcart()`, `getskilllv`, the Free Ticket for the Cart Service, the fee, Special Reserve Points and `setcart`, which is `player:set_cart`.
- The `percentheal` stubs stay. They are in the Niflheim branches only (`F_KafStor` with `getarg(2) == 1` and `F_KafEnd` with type 1), and need a way to take HP and SP, which the API does not have. So the report still counts 54 renewal Kafras as partly converted: every service works but Niflheim's HP and SP loss.

### Pre-renewal classic

`waserver` warns at start when a classic world runs with `mode: pre-renewal`: it is experimental until `core.ruleset.pre_renewal` matches rAthena's pre-renewal (phase 10 item 1). The deploy guide says so.

## Consequences

- The release binaries carry 24.9 MB more of reference data in the part files.
- `waserver` converts more NPCs at start: core's whole conversion takes about 1.6 s in renewal and 0.6 s in pre-renewal on the Apple M2, up from 1.0 s and 0.25 s.
- Part B can run the full kRO world with the new NPCs and take the release notes from the reports.

## Review items

Review decision 56 settled the first three items, and [decision 0052](0052-classic-mode-part-b.md) built them. The NPC-to-NPC commands of the last item stay in phase 10.

- A conversation for touch handlers, so touch labels with a dialog convert. It is a Lua API addition beyond decision 55: `touch` would get `(player, conversation)` and could start a conversation as a click does.
- A way to take HP and SP, such as `player:heal_percent` with negative amounts, as `percentheal` does, for Niflheim's Kafra. It is a Lua API addition.
- Whether to keep the part files as JSON, or to compress them: gzip makes the renewal files 3.3 MB, about a fifth, but the diffs of a new extract would not be readable.
- Whether a converted NPC with a touch area and no touch label should open its dialog on touch, as rAthena clicks it, once touch has a conversation.
- `set` on `getvariableofnpc` and the NPC events of other NPCs (`donpcevent`, `OnEnable`, `OnDisable`, `enablenpc`, `cloakoffnpc`), which block most of the rest of the quest NPCs.
