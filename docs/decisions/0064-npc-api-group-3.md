# 0064: NPC API Group 3: Player Calls, Items and the Clock

- Status: built. Details to review.
- Date: 2026-10-21.
- Code: [server/modscript](../../server/modscript) (`playerapi.go`, `itemapi.go`, `clock.go`, `host.go`, `jobs.go`, `rules.go`, `runtime.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone](../../server/zone) (`playeractions.go`, `itemactions.go`, `rentals.go`, `zone.go`, `scripts.go`, `equipment.go`, `npcpresent.go`, `npcworld.go`, `npcs.go`, `touch.go`, `social.go`, `query.go`, `giveskill.go`, `jobchange.go`, `loot.go`, `statuses.go`, `itemscripts.go`, `inventory.go`, `transfers.go`, `storage.go`, `trade.go`, `vending.go`, `mail.go`, `persist.go`), [server/item](../../server/item) (`roll.go`, `inventory.go`), [server/store](../../server/store) (migration `00042_item_attributes.sql`, `queries`, `store.go`, `storage.go`, `guild.go`, `market.go`, `statuses.go`), [server/character](../../server/character), [server/mod](../../server/mod) (`npcs.go`), [server/status](../../server/status) (`equip.go`), [server/progression](../../server/progression), [schema](../../schema) (`event.proto`), [client/src](../../client/src) (`viewer/online.ts`, `viewer/main.ts`, `viewer/inventory.ts`, `audio/audio.ts`), [tools/convert](../../tools/convert) (`npcgroup3.go`, `npctimer.go`, `npcpresent.go`, `npclua.go`, `npcworld.go`, `npcscript.go`, `npcexpr.go`, `npcstorage.go`, `npc.go`, `rathena.lua`).
- Docs: [mods/README.md](../../mods/README.md), [tools/convert/README.md](../../tools/convert/README.md), [server/README.md](../../server/README.md), [client/README.md](../../client/README.md), [lua-api.md](../generated/lua-api.md).
- Builds review decision 58 of [proposal 0002](../proposals/0002-lua-api.md), areas B7, B9 and B10 of [proposal 0002b](../proposals/0002b-npc-api.md). [Decision 0062](0062-npc-api-group-1.md) built B1 to B4, and [decision 0063](0063-npc-api-group-2.md) B5 and B6. Area B8 comes later.
- Lua API: adds to API 1.0. Protocol: new fields and an event, listed below. Settings: none. Data: an NPC record's `display_name` may hold a `#` part. Store: migration 00042. Permission: none new. `player.give_exp`, which the catalog had, now has a call.

## Context

- rAthena's scripts read and change players, give items with refines, cards and rentals, refine, break and repair equipment, and read the time and the server's rates. The converter could not map these commands: `getexp`, `MaxWeight`, `setpcblock`, `disable_items` and `gettime` alone blocked hundreds of renewal scripts (proposal 0002b).
- Review decision 58 approved the B batch with its calls. This record builds areas B7, B9 and B10, and fixes differences from rAthena that decision 0063 left, as the next release needs 1:1 parity.

## Decision

### B7: player reads and actions

| Call | rAthena |
|---|---|
| `player.weight`, `player.max_weight`, `player.skill_points` | `Weight`, `MaxWeight`, `SkillPoint` |
| `player.look` | `getlook` (script.cpp 16377) |
| `player.staff_level` | `getgmlevel` (10469) |
| `player:give_exp(base, job, { raw })` | `getexp` (11142), `getexp2` (24713) |
| `player:heal(hp, sp)` | `heal` (6007) |
| `player:set_hp(amount)` | `Hp = n`, `pc_setparam` (pc.cpp 10503) |
| `player:set_blocked(action, blocked)`, `player:is_blocked(action)` | `setpcblock`, `getpcblock` (18964, 18979) |
| `conversation:lock_items()`, `conversation:unlock_items()` | `disable_items`, `enable_items` (8828, 8820) |
| `player:unequip_all()` | `nude` (15715) |
| `player:push(direction, cells)` | `pushpc` (22604) |
| `player:transform(monster, milliseconds)` | `transform` (23777) |
| `player:open_book(book, page)` | `readbook` (20929) |
| `player:get_save_point()` | `getsavepoint` (16412) |
| `player:get_inventory()` | `getinventorylist` (15029) |
| `player:set_look { ... }` | `setlook` (6860), `pc_changelook` (pc.cpp 11127) |
| `player:has_option(option)` | `checkoption` (10526) |
| `player:change_job(job, { keep_line })` | `jobchange <job>,Upper` (6099) |
| `player:give_skill(skill, level, "permanent_grant")` | `SKILL_PERM_GRANT` (pc.cpp 5435) |
| `jobs.class_flags(job)`, `jobs.from_class(flags, sex?)` | `eaclass` (12835), `roclass` (12853) |

- `give_exp` without `raw` multiplies each amount by `quest_exp_rate` as a double, capped to 0 and `MAX_EXP` (script.cpp 11168), then pays the guild's tax, as `pc_gainexp` with flag 1. With `raw` it gives each positive amount alone without the tax, as flag 2, and a negative amount takes exp down to 0, as `pc_lostexp` (pc.cpp 8556). Both 0, or a negative amount without `raw`, raise an error, as the commands fail. A dead player gets nothing, as `pc_gainexp` returns for `pc_isdead` (pc.cpp 8471). `pc_lostexp` takes from the dead too.
- `heal` is `status_heal` with flag 1: nothing for the dead, Frenzy does not stop it, and a negative amount is passive damage that may kill (status.cpp 1812, 1819).
- `set_hp` caps to 1 and the maximum, so it never kills (pc.cpp 10503). rAthena sets the HP of a dead player without reviving it; wAthena leaves the dead alone.
- The blocks are rAthena's `PCBLOCK_` flags (script.hpp 2221): `move` (`unit_can_move`), `attack`, `skill`, `use_item` (clif.cpp 12103), `chat` and typed GM `commands` (clif.cpp 10573, atcommand.cpp 12041), `immune` (no one's enemy, battle.cpp 8010), `npc_click` (clicks, touch areas, events with the player, and a walking NPC's touch, npc.cpp 2232, 1970, 1782, unit.cpp 511), `equip` (clif.cpp 12141, 12189), `emotion` and `sit_stand`, and `npc` and `all`. wAthena has no sitting and no player emotions yet, so those two block nothing. `is_blocked` of `npc` or `all` is true when every flag is set.
- A conversation's items follow `item_enabled_npc`: locked in renewal and unlocked in pre-renewal (battle.cpp 8685). Locked, a player who talks cannot use, equip or unequip items, or use skills, and is told it is busy. Unlocked, it may, and its skills are those on itself alone (clif.cpp 12922).
- `transform` starts `SC_MONSTER_TRANSFORM` with the monster in val1. The player's look shows the monster until the status ends. A monster that does not exist, or the Emperium, raises an error. 0 milliseconds does nothing.
- `set_look` caps hair style, hair color and cloth color to `rules.get`'s `max_hair_style`, `max_hair_color` and `max_cloth_color`, and saves them. The headgear views go over the equipment's until the player leaves the map, as `setlook_head_*`.
- `staff_level` is 99 for the owner and admin roles, as rAthena's Admin group, 10 for any other staff role, as its Law Enforcement group, and 0 for a player.
- `keep_line` changes into the job of the player's line, as `pc_jobid2mapid(job) | JOBL_UPPER` or `JOBL_BABY`, then `pc_mapid2jobid` by sex.
- `permanent_grant` is `permanent`, and a skill reset keeps the skill, as `SKILL_FLAG_PERM_GRANTED` (pc.cpp 9514). The flag is saved.

### B9: items and equipment

- `player:give_item`'s options `refine`, `cards`, `broken`, `identified` and `signed_by` are `getitem2` and `getnameditem`. Refine, broken and identified apply to weapons, armor and shadow gear alone, as `getitem2` turns other items into identified, unrefined and unbroken copies (script.cpp 7853). Refine is capped to `MAX_REFINE`.
- `rent_seconds` gives rentals that expire, as `rentitem` (7922):
  - The end is in Unix seconds of the server clock, as `expire_time`. A rental never stacks (pc.cpp 6019).
  - The zone checks its players' inventories once a second and at login, as `pc_inventory_rentals` (pc.cpp 1200, 14807). An expired rental comes off and is deleted, and the client is told, as `clif_rental_expired`.
  - A rental cannot be dropped (pc.cpp 11381), traded (trade.cpp 393), mailed (mail.cpp 270), sold to a buying store (buyingstore.cpp 393), vended (vending.cpp 346), or put in the guild storage (storage.cpp 744). It goes into the player's storage and cart, where it does not expire, as rAthena checks the inventory alone.
  - An item group's entry with a duration gives a rental too (`pc_get_itemgroup_sub`).
- `player:count_item`'s options are `countitem2` and `rentalcountitem` (7224, 7427). Without `rentals`, `count_item` counts only copies that are not rentals, as `countitem`. `refine` and `cards` count identified copies that are not broken, with exactly those cards.
- `maps.drop_item(map, x, y, item, amount, options)` is `makeitem` and `makeitem2`: one pile on a free cell next to the cell, as `map_addflooritem` with `DIR_MAX`, with no loot rights, in the map's zone.
- `items.get(item)` is `getiteminfo` and `getitemslots`.
- `items.can_refine`, `items.refine_chance` and `items.refine_cost` read `refine.yml`, as `getequipisenableref`, `getequippercentrefinery` and `getequiprefinecost`. `item` is an item ID with the refine it has, or a worn copy, whose refine and rental they read. Weapons by weapon level, armor by armor level, and shadow gear at level 1, as `calculate_refine_info` (status.cpp 465).
- `equipped:add_refine(amount)` is `successrefitem` and `downrefitem`: the copy comes off, changes refine within 0 and `MAX_REFINE`, shows again and goes back on, and the wearer shows the refine's effect. At `MAX_REFINE`, `successrefitem` changes nothing.
- `equipped:destroy()` is `delequip`, and `failedrefitem`'s deletion.
- `player:get_broken_items()`, `player:repair_item(n)` and `player:repair_all()` are `getbrokenid`, `repair` and `repairall`. Repairs cost nothing, as the commands.
- `player:run_item_script(item)` is `consumeitem`: the item's use script runs, with no check and nothing spent.
- `player:merge_items()` joins the stacks of one item with the same copy data, as `mergeitem2`, through moves the item log records.

### B10: clock and server settings

- `wa.clock`: `clock.now()` is `gettimetick(2)`, `clock.milliseconds()` the server clock in Unix milliseconds, `clock.get(field)` `gettime`'s `DT_` fields in the server's time zone, and `clock.format(format, seconds?)` `gettimestr`'s `strftime` in the C locale. No permission. `world:set_time` sets the clock in tests.
- `rules.get` reads the server settings `base_exp_rate`, `job_exp_rate`, `quest_exp_rate`, `max_hair_style`, `max_hair_color` and `max_cloth_color`, as `getbattleflag`. The exp rates come from the progression tables, and the look limits are rAthena's conf/battle/client.conf values, 42, 8 and 4.

### Store

- Migration 00042 saves `broken`, a rental's `expires_at`, and `signed_by` and `signed_name`, on `inventory` and `storage_items`, and `broken` and the signer on `guild_storage` and `mail_items`. Before it, a broken item was mended at logout. It saves `character_skills.granted` too.
- Down deletes the rentals of the inventory and the storages, so they do not become permanent.

### Protocol and client

- New: the event `BookOpen`, the `InventoryItem` and `InventoryAdd` fields `broken`, `expires_at` and `signed_by`, and `InventoryRemoveReason.EXPIRED`. `Look.cloth_color` is now sent, and a transformed player's look has the monster in `body` with `npc_view` `MONSTER`.
- The client draws a transformed player as its monster, a cloth color as the body's palette, and the item lines "Named for", "Broken" and "Rental until". It says when a rental ended. It does not show books: the asset build has no book texts.
- A sound with `repeat` plays again and again until the map changes. rAthena notes its client plays it once (clif.cpp 9690); wAthena's honours it.

### Group 2 differences fixed

| Difference (decision 0063) | Now |
|---|---|
| Quest icons shown to players already on the map | As `pc_show_questinfo` (pc.cpp 15396): a player's icons are set up at its map entry, as `pc_show_questinfo_reinit` (clif.cpp 11187), and an NPC new to the map's list shows to no player already there until it enters again. `npc:add_quest_icon` sends nothing at once. |
| Converted duplicates kept their own NPC variables | A script that duplicates run keeps every `.x` as an NPC variable of its source, which they share, as `npc_parse_duplicate` copies the script (npc.cpp 4596). `getvariableofnpc` of a duplicate reaches its source's (script.cpp 20521). |
| `full_name` lost the `#` part of a `::` name | The converter keeps the `#` part in `display_name`, such as `Guard#east`. Players see the part before `#`, and `full_name` and `hidden_name` read the rest, as `strnpcinfo` reads `nd->name` (npc.cpp 3668). `npc:set_display_name("A#b")` sets both, as `setnpcdisplay`. |
| A walking NPC's touch area moved with it | rAthena moves it too: `map_moveblock` unsets and sets the NPC's `CELL_NPC` cells (map.cpp 555, 632), and a walking NPC touches the players on each step (unit.cpp 700). Decision 0063 read it wrong; a test now checks it, and `movenpc` fires no touch, as `npc_movenpc` (npc.cpp 5046). |
| `unitwalk` started at once | `R.unitwalk` starts the walk after 50 ms and refuses a second forced walk until the first reaches its cell (script.cpp 20015, 20032). `unitstopwalk` and `npcstop` end the forced walk. |
| The client played repeated sounds once | It loops them. |

### NPC timers

- `R.npc_timer` keeps the tick by the server clock: `getnpctimer(0)` converts, as `npc_gettimerevent_tick` (npc.cpp 1704), and `startnpctimer` resumes from the tick where `stopnpctimer` stopped, as `npc_timerevent_start` (npc.cpp 1542).
- The tick runs while the map has no players, as rAthena's. The labels still wait on `timers.after` and the zone clock: a label that came due while the map slept runs when the map wakes, at the delay left before it slept, and the labels after it that came due run with it, in order.

### Converter

- Every B7, B9 and B10 command converts onto the calls above, for the attached player, with the limits tools/convert/README.md lists. `jobchange <job>,Upper` and `SKILL_PERM_GRANT` convert now.
- `getbattleflag` converts the exp rates, the look limits, `feature.barter` as 1, and `feature.bgqueue`, `feature.refineui`, `feature.achievement` and `feature.stylist` as 0, for systems that do not exist.

## Coverage

Script NPC lines of `docs/generated/npc-scripts-<mode>.md`:

| Mode | Lines | Before | After |
|---|---:|---:|---:|
| renewal, not duplicates | 7,067 | 5,371 (76.0%) | 6,213 (87.9%) |
| renewal, with duplicates | 14,865 | 10,986 (73.9%) | 12,168 (81.9%) |
| pre-renewal, not duplicates | 3,754 | 3,032 (80.8%) | 3,342 (89.0%) |
| pre-renewal, with duplicates | 8,205 | 6,985 (85.1%) | 7,327 (89.3%) |

## Breaks

- `player:count_item(item)` leaves out rentals, as `countitem`.
- `player:give_item(item, amount, { rent_seconds })` gives real rentals. With `to` or `bound` it raises an error, as `rentitem` gives to the inventory, unbound.
- A pre-renewal conversation lets the player use and equip items, and use skills on itself, as `item_enabled_npc`. Before, every conversation refused them.
- `npc:add_quest_icon` no longer shows a new NPC's icons to the players already on the map.
- `player.look`'s cloth color reaches the client, which draws the body's palette.
- A converted script with duplicates shares its `.x` with them, and reads them from the world service.

## Alternatives

- **One call `player:refine`.** The proposal names `equipped:refine(amount)`, but `equipped.refine` is the copy's refine field, and a method of the same name would hide it. Built as `equipped:add_refine(amount)`.
- **Timers on the server clock.** NPC labels would run while the map sleeps, as rAthena's, if the zone woke for the timers of NPC contexts, as it does for schedules. That changes `wa.timers`, whose timers follow the zone clock. Not built: the tick follows the server clock, and the labels catch up.
- **Merge in a client window.** `mergeitem` opens a window where the player picks the stacks. The client has none, and wAthena stacks equal copies when they are added, so `merge_items` joins the stacks it can, as `mergeitem2`.

## Details to review

- API beyond decision 58 and 0002b, built because the proposal's form could not work as written:
  - `equipped:add_refine` in place of `equipped:refine` (Alternatives).
  - `items.refine_cost(item, refine?, kind?)` takes the refine, which the cost depends on, and the refine reads take a worn copy too.
  - `clock.format`'s second argument, `gettimestr`'s time.
  - `set_blocked`'s action `equip` in place of the proposal's `party`: rAthena has `PCBLOCK_EQUIP` and no party flag.
- Not built, as they need API that decision 58 does not name, or systems that do not exist:
  - `maps.can_enter` (`jobcanentermap`): `job_noenter_map.txt` restricts third jobs on WoE:TE maps, which need map flags that come with sieges (area C). The table lists it under B10, and section B8 under maps.
  - `schedules.yearly`: proposal 0002b says it can wait, and no script uses `OnDayMMDD`.
  - The HD and Enriched refine costs and the Enriched chance: the refine extract holds the Normal cost alone. The rAthena fork's extractor needs to record them. The calls raise an error for them.
  - `failedrefitem`'s failure effect: `equipped:destroy()` shows none, and an option would be new API.
  - `active_transform` and `transform` with a status: an option of `player:transform`.
  - `killedrid`: the death events of every map (decision 0062's details).
  - A warg companion: no core record shows `OPTION_WUG`, so `player:has_option("warg")` is false. rAthena has no script command that gives one.
  - The Madogear suit and the dragon colors other than green: core's mounts have one record each.
- Group 2 differences that cannot match rAthena without new API:
  - `classchange` sends a class to the area or one player and keeps nothing (script.cpp 15163, clif.cpp 1476). `npc:set_sprite` keeps it, so players who come later see it. A call that sends a view without keeping it is new API.
  - `unitstopwalk`'s default `USW_NONE` sends no fixpos (unit.cpp 1733). `npc:stop()` sends the cell. An option is new API.
  - `soundeffectall` without a map, with an attached player, plays around the player, from it (script.cpp 15315). `player:play_sound` plays to the player alone; `to` for it is new API, as decision 0063 listed. Without a player it plays from the NPC, which matches.
  - `unitwalk` gives 1 when it starts the walk, where rAthena gives `unit_can_reach_pos`'s answer; that needs a reachability read.
- `staff_level` maps roles to rAthena's group levels: 99 and 10. Roles are the config's, without levels.
- `jobs.from_class`'s sex is male without one, where `roclass` takes the attached player's. The converter passes `player.sex`.
- A rental expires within a second of its end, as the zone checks once a second. rAthena's timer fires at the end, at most an hour apart.
- A signed copy keeps no mark in its cards: `getequipcardid(slot, 0)` gives the first card, where rAthena gives 254.
- The client shows no books, and no minimap marks, as decision 0063 noted.
- The protocol changed since v0.1.0. `schema.Version` stays 1 until the next release raises it.

## How to check it worked

- `go test ./server/zone -run 'TestGiveExpRules|TestHealAndSetHP|TestBlockedActions|TestItemsDuringTalk|TestRentalsExpire|TestGiveItemCopies|TestClockCalls|TestQuestInfoNewNPC|TestFullNameHiddenPart|TestRefineCalls|TestMovingTouchArea|TestTransformLookAndPush|TestPlayerReadsAndItemCalls'`.
- `go test ./server/modscript -run 'TestStrftime|TestClockFields'` and `go test ./server/mod -run TestNPCHiddenName`.
- With `WATHENA_TEST_DATABASE_URL`: `go test ./server/store -run 'TestSaveAndLoadEveryField|TestConstraints|TestMigrationsFromEmpty'`.
- `go test ./tools/convert -run 'TestGroup3Convert|TestDuplicateVariablesConvert|TestScriptDisplayName|TestRunNPCTimerTick|TestRunConvertedNPCs'`.
- `pnpm vitest run src/viewer/online.test.ts src/viewer/inventory.test.ts` in `client/`.
