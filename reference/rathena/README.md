# rAthena Reference Data

This folder holds data recorded from rAthena. wAthena's parity tests compare against it.

## Loading

Go code loads these files with the `internal/reference` package. Its tests check that every file decodes and that each mode's files match that mode.

## Source

- rAthena data: commit `e98500617` ("6th rebalance - Meister - Part 1", #9997). The `wathena-reference` branch of the fork is based on it and changes no data. The recordings carry it in `rathena_git_hash`.
- Everything reproduces from fork commit `f99a928d1` (checked on 2026-10-27; castles and map flags added on 2026-10-31). The fork's `wathena-reference` branch holds the whole recorder (`src/map/wathena_recorder.cpp`, `tools/wathena/record.sh`) and every extract function of `tools/wathena/extract.py`. Its commits, oldest first: `575d83531` extract script, `3f5534394` inventory script, `83616e3c3` exp tables and drops, `84dbe9b2f` job stats, `e9dceb60e` item weights and heals, `27ad000bc` warps, `f4641e37b` script constants, item scripts and groups, `003599e1c` NPCs, shops and item prices, `eac0a7e75` equipment, refine and monster groups, `2eb1327f4` status, `e92917ccc` skills, skill trees and casting, `784fce3ec` second class recordings and job change NPCs, `ecf13b511` monsters, `8e1c56558` random options, `5e50b7ad9` mounts, `50df331ca` party sharing, `f30328d04` guilds, `4214647ba` item trade, `3e45adbd3` barter shops and shop settings, `60bed4ccf` quests, `28488df04` refine costs, `39de3053d` part files for quest, warp and event NPCs, `f99a928d1` castles and GvG map flags.
- `python3 tools/wathena/extract.py` at `f99a928d1` writes every file in `extracts/` with the same content as this folder. Only `rathena_git_hash` differs: in `extracts/` it is the fork commit that last wrote the file, and a run writes `f99a928d1`. The files were not copied again.
- The recordings (`renewal/`, `pre-renewal/`) need `record.sh` and a built map server. The 2026-10-27 check did not rebuild them.
- The fork's branch has no upstream and is not on its remote yet.
- The recorder and scripts stay in the rAthena fork. This repository keeps only their output, so it does not need a C++ toolchain.

## Folders

| Folder | Content | Produced by |
|---|---|---|
| `renewal/`, `pre-renewal/` | Formula results for each mode, listed below. | `tools/wathena/record.sh` |
| `inventory/` | Skills, script commands, RENEWAL sites, and feature inventory gaps. | `tools/wathena/inventory.py` |
| `extracts/` | Monster spawns, spawn tags, minimum client tables, progression data (exp tables, status points, level penalty, and monster drops), job stats (weight, HP and SP tables, base ASPD, job bonus stats, and map job IDs), and items (type, weight, stacking, use and drop limits, and the heal of scripts that are only `itemheal` or `percentheal`), per mode. `items.json` lists the heal scripts it could not read in `unparsed_heal_scripts`. `warps.json` lists warp NPCs, described below. `script_constants.json`, `item_scripts.json` and `item_groups.json` are what the item script converter reads, described below. `equipment.json`, `refine_db.json` and `monster_groups.json` are what equipment reads, described below. `status.json` is what status effects read, described below. `skills.json` and `skill_trees.json` are what skills read, described below. `monsters.json` is what monster stats, skills and AI read, described below. `random_options.json` is what random options read, described below. `guilds.json` is what guilds read, described below. `quests.json` is what quests read, described below. `castles.json` and `map_flags.json` are what castles and GvG maps read, described below. `item_trade.json` holds the Trade flags of each item that sets one, and `shops.json` the barter shops, the items for buying stores, and the vending, store search and mail settings. | `tools/wathena/extract.py` |

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
| `skill_damage.json` | Every damaging skill of the first, second and transcendent job trees, at every level, against three monsters, with the ammunition a skill needs equipped and the learned skills of each player. Heal against two undead monsters. |
| `skill_casting.json` | For each first, second and transcendent job and three stat builds, every skill of its tree that no earlier job recorded, at every level: cast time, after-cast delay, cooldown, requirements, and heals. |
| `mob_ai.json` | Single decisions of the hard monster AI for every AI type: target search, hits, target changes, assists, loot, chase and cast sensor. |
| `mob_skill_use.json` | For every monster with skills, which entry of its skill list it uses in each state and condition, and what the skill does. |
| `skill_status.json` | For each second and transcendent class skill that starts a status, at every level: the statuses `skill_castend_nodamage_id` starts with their values and timers, the target's stats before and after, and a normal attack against a Golem before and after. |
| `item_bonuses.json` | Changes from each equipment item and card, in stats, damage dealt, and damage taken. |
| `refine.json` | Stats at every refine level for one armor and one weapon of each weapon level. |
| `status_effects.json` | Chance, duration, and whether ten common ailments start, for many sources and targets. |
| `level_penalty.json` | Renewal exp and drop modifiers by level difference. Empty in pre-renewal. |
| `party_share.json` | Exp sharing among attackers and parties when a monster dies, and party item pick-up rights and distribution. |
| `guild.json` | Guild exp tax, the guild skills Battle Orders, Regeneration, Restore and Emergency Call, and guild skill blocks at login. |
| `random_option_rolls.json` | rAthena's random option groups applied with every draw at its lowest, and statistics of seeded applications, and the options of each monster drop with a group. |

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

## NPCs

The converter also reads two extracts per mode for NPCs ([decision 0008](../../docs/decisions/0008-converted-npcs.md)):

- `npcs.json`: script NPCs, functions, shops and their duplicates, in the order `npc/<mode>/scripts_main.conf` loads them, with the names, map checks and duplicate rules of `warps.json`. Each NPC is one line of the file.
- Script NPCs are kept from the folders the first release needs: `kafras`, `merchants`, `cities`, `airports`, `guides`, `jobs` and `other`, below `npc/`, `npc/re/` and `npc/pre-re/`. Shops and functions are kept from every folder.
- The script NPCs of the `quests`, `warps` and `events` folders are kept too, in part files: `npcs-<folder>-<n>.json`, each below 1.9 MB of rows (review decision 55). `npcs.json` lists them in `parts`, in the order to read them after it, and counts their NPCs in `part_summary`. A duplicate goes in the file of its source, so each file holds the sources of its duplicates. The rows of `npcs.json` did not change.
- `left_out` counts the script NPCs of the other folders, such as `instances` and `guild`.
- Shops have their items, with a price below 0 in the line replaced by the item's buy price, as `npc_parse_shop`. `discount` says whether Discount lowers the prices (`npc_shop_discount`). Item and point shops name their currency, and market shops give a stock.
- `item_prices.json`: the buy and sell price of every item, with `ItemDatabase::loadingFinished`'s rules: a missing Buy is Sell * 2, a missing Sell is Buy / 2, and a Sell above what Discount and Overcharge allow becomes 1. `no_sell` is `Trade.NoSell`.

| Mode | NPC lines | Shops | `npcs.json` | `item_prices.json` |
|---|---:|---:|---:|---:|
| renewal | 6,870 | 358 | 5.3 MB | 2.0 MB |
| pre-renewal | 4,190 | 185 | 3.2 MB | 0.4 MB |

The part files, from rAthena fork commit `28488df04` (`PART_FOLDERS` is in the fork since `39de3053d`):

| Mode | `quests` | `warps` | `events` | Files | Size |
|---|---:|---:|---:|---:|---:|
| renewal | 6,162 | 250 | 150 | 11 | 16.3 MB |
| pre-renewal | 2,942 | 81 | 150 | 7 | 8.6 MB |

Each count is of script NPC lines, duplicates too. The largest file is 1.9 MB. `npcs.json` itself was already above 2 MB, at 8.3 MB and 4.8 MB, and only its header changed.

`PART_FOLDERS` and the part files are in `build_npcs` of `tools/wathena/extract.py`, with `extract.py npcs`, which writes `warps.json`, `npcs.json`, its part files and `item_prices.json` alone:

```text
python3 tools/wathena/extract.py npcs
```

Two runs give the same files. `warps.json` and `item_prices.json` come out the same but for `rathena_git_hash`, so they were not copied again.

They were first extracted at rAthena fork commit `003599e1c`. The `jobs` folder came at commit `784fce3ec`.

## Equipment

Equipment reads three extracts per mode ([decision 0009](../../docs/decisions/0009-stats-as-totals-of-modifiers.md)):

- `equipment.json`: the equipment fields of every weapon, armor, ammunition, shadow gear and card in `item_db_*.yml`, with `ItemDatabase::parseBodyNode`'s defaults and caps and `loadingFinished`'s fixes: type and subtype, location (`EQP_` bits), ATK, MATK, DEF, range, card slots, weapon and armor level, level range, refineable, view, `class_base` from `Jobs` through `itemdb_jobid2mapid`, `class_upper` from `Classes`, sex after `defaultGender`, and the `item_noequip.txt` restriction. Type names are read without case, as rAthena reads them through script constants.
- `refine_db.json`: `refine.yml` as `RefineDatabase` loads it, without chances: the bonus and random bonus of each refine level, by group and item level, and the Normal refine costs (since `28488df04`).
- `monster_groups.json`: each monster's `RaceGroups` as `RC2_` values.

| Mode | Equipment items | Refine levels | Monsters with race groups |
|---|---:|---:|---:|
| renewal | 18,504 | 160 | 454 |
| pre-renewal | 2,539 | 70 | 93 |

They come from `build_equipment`, `build_refine` and `build_monster_groups` in the fork's `tools/wathena/extract.py`, added at rAthena fork commit `eac0a7e75`.

## Status effects

Status effects read one extract per mode ([decision 0010](../../docs/decisions/0010-status-effects.md)):

- `status.json`: `status.yml` as `StatusDatabase` loads it, by `SC_` value: the name and constant, the icon (`EFST_` value, -1 for `EFST_BLANK`), the `DurationLookup` skill with its `Duration1` and `Duration2` at levels 1 to 13 from `skill_db.yml`, states, calc flags, opt1 to opt3, options, flags, `MinRate`, `MinDuration`, `Fail`, `EndOnStart`, `EndReturn`, `EndOnEnd`, and the script. `calc_flags` is `[All]` for a status with a script, as `loadingFinished` sets it.

| Mode | Statuses | With a script |
|---|---:|---:|
| renewal | 1,018 | 168 |
| pre-renewal | 699 | 67 |

It comes from `build_status` in the fork's `tools/wathena/extract.py`, added at rAthena fork commit `2eb1327f4`.

## Skills

Skills read two extracts per mode ([decision 0011](../../docs/decisions/0011-skills.md)):

- `skills.json`: `skill_db.yml` as `SkillDatabase` loads it, by skill ID: the name, description, maximum level, type, target, damage flags, flags, and every value per level as an array of 13 (range, hit count, element, splash, active instances, knockback, cast and delay times, durations, cooldown, fixed cast time, and the requirements), with the unit and the status. `nocast` holds `skill_nocast_db.txt`'s map types.
- `skill_trees.json`: `skill_tree.yml` with `Inherit` merged into each job, as `SkillTreeDatabase` loads it: each skill's maximum level, the skills it needs, and the base and job levels it needs.

| Mode | Skills | Trees | Tree entries |
|---|---:|---:|---:|
| renewal | 1,635 | 175 | 8,122 |
| pre-renewal | 1,228 | 77 | 2,121 |

They come from `build_skills` in the fork's `tools/wathena/extract.py`, added at rAthena fork commit `e92917ccc`, which also adds the recorder fix below and `record_skill_casting`.

`pc_damage.json` and `skill_damage.json` were recorded again with the recorder fix for ranged attacks: the recorder now sets `arrow_atk` for a bow or gun with ammunition equipped, as `battle_calc_attack` callers do, and for skills that need ammunition. `skill_casting.json` is new. The other formula files did not change.

## Second class recordings

`skill_damage.json` and `skill_casting.json` were recorded again for second and transcendent classes, and `skill_status.json` is new ([decision 0012](../../docs/decisions/0012-second-classes-and-job-changes.md)):

- The sample players are now the first, second and transcendent jobs. Dancers and Gypsies are female. `players` lists each player's learned skills, so the tests learn the same ones.
- Skills that need ammunition use arrows. A player with a cart has the cart weight of `cart_weight`, which Cart Termination reads.
- A few skills record extra cases, such as Charge Attack at each distance.

| Mode | `skill_damage.json` | `skill_casting.json` | `skill_status.json` |
|---|---:|---:|---:|
| renewal | 129 skills, 1.2 MB | 6,078 cases, 2.5 MB | 622 cases, 1.2 MB |
| pre-renewal | 129 skills, 1.2 MB | 6,096 cases, 2.5 MB | 512 cases, 0.9 MB |

The recorder changes are in the fork since commit `784fce3ec`: the recorder, `record_skill_status`, and `WATHENA_RECORD_ONLY` to record only some files, with the `jobs` folder in `npcs.json`. The recordings come from:

```text
WATHENA_RECORD_ONLY=skill_damage,skill_casting,skill_status bash tools/wathena/record.sh renewal pre-renewal
python3 tools/wathena/extract.py
```

## Monsters

Monster stats, skills and AI read one extract and two recordings per mode ([decision 0013](../../docs/decisions/0013-monster-ai.md)):

- `extracts/<mode>/monsters.json`: `mob_db.yml` as `MobDatabase` loads it, before `status_calc_misc`, with the AI type, the modes, the `mob_skill_db.txt` rows of each monster in file order, and `mob_chat_db.yml`. The file's `note` lists the details.
- `mob_ai.json`: single calls of `mob_ai_sub_hard`, `mob_set_attacked_id` and `unit_skilluse_id` with a probe monster of each AI type on an open part of `prt_fild08`. The file's `note` lists the cases and results.
- `mob_skill_use.json`: for every monster with skills, every entry in each case of its condition: which entry `mobskill_use` or `mobskill_event` picks, its target and cast time, and what the skill did to the player and the monster.

| Mode | Monsters | With skills | Skill rows | `mob_ai.json` | `mob_skill_use.json` |
|---|---:|---:|---:|---:|---:|
| renewal | 2,675 | 1,941 | 14,514 | 2,814 cases | 22,189 cases, 18,232 used |
| pre-renewal | 1,004 | 703 | 5,494 | 2,814 cases | 8,594 cases, 7,213 used |

They come from rAthena fork commit `ecf13b511`, which adds `build_monsters` to `tools/wathena/extract.py`, `record_mob_ai` and `record_mob_skill_use` to the recorder, and two small hooks in `mob.cpp` for the recorder. With it:

```text
WATHENA_RECORD_ONLY=mob_ai,mob_skill_use bash tools/wathena/record.sh renewal pre-renewal
python3 tools/wathena/extract.py
```

`mob_status.json` stays. The parity test of monster stats compares it with the stats that `server/mob` computes from `monsters.json`.

## Random options

Random options read one extract and one recording per mode ([decision 0014](../../docs/decisions/0014-rolled-items.md)):

- `extracts/<mode>/random_options.json`: `item_randomopt_db.yml` and `item_randomopt_group.yml` with their imports for the mode, as `RandomOptionDatabase` and `RandomOptionGroupDatabase` load them. Each option has its ID, name and script. Each group has its slots in order, `max_random` and `random`, and each entry its option, `min`, `max`, `param` and `chance` per 10,000, with rAthena's defaults and fixes (a `MinValue` above `MaxValue` makes `MaxValue` one more). `drops` lists every monster drop with a `RandomOptionGroup`, by its index in the monster's drops as `MobDatabase` keeps them.
- `random_option_rolls.json`: for each group, the options of `s_random_opt_group::apply` with `rnd_force_min`, and statistics of 100,000 calls with a seeded generator: how many copies got 0 to 5 options, and for each option index, each option's count, lowest and highest value, and sum. `drops` has the options of `mob_setdropitem_option` with `rnd_force_min` for each monster drop with a group. The values come from `std::uniform_int_distribution`, so the statistics depend on the C++ library: these were recorded with the builder image's (Alpine, musl).

| Mode | Options | Groups | Drops with a group | Seed |
|---|---:|---:|---:|---:|
| renewal | 249 | 108 | 179 | 20240501 |
| pre-renewal | 0 | 0 | 0 | 20240501 |

Pre-renewal has no random options: rAthena imports `db/re/item_randomopt_*.yml` in renewal only.

They come from rAthena fork commit `8e1c56558`, which adds `build_random_options` to `tools/wathena/extract.py` and `record_random_options` to the recorder. With it:

```text
WATHENA_RECORD_ONLY=random_option_rolls bash tools/wathena/record.sh renewal pre-renewal
python3 tools/wathena/extract.py
```

## Mounted players

`mount_status.json` holds player stats and damage on each mount, one file per mode:

- Players are level 99 at their job's maximum job level, with the `melee` build and every tree skill learned.
- Each job is recorded without a mount (`none`) and on each mount its skills allow: `riding` (`KN_RIDING`), `dragon` (`RK_DRAGONTRAINING`), `wug_rider` (`RA_WUGRIDER`) and `madogear` (`NC_MADOLICENCE`).
- Each mount is tested with no weapon (`weapon` 0) and the lowest-ID 1H spear, 2H spear and 1H sword the job can equip.
- Each case has `option`, `max_weight`, `battle_status`, `atkmods` (the right hand's size modifiers for small, medium and large) and `skills`.
- `damage` is a normal attack against Orc Warrior (1023, medium), with `forced_min` and `rolls` as in `pc_damage.json`.
- The jobs are Knight, Crusader, Lord Knight, Paladin, Baby Knight and Baby Crusader. Renewal also has Rune Knight, Royal Guard, Ranger and Mechanic.

| Mode | Cases | none | riding | dragon | wug_rider | madogear |
|---|---:|---:|---:|---:|---:|---:|
| renewal | 74 | 35 | 32 | 4 | 1 | 2 |
| pre-renewal | 48 | 24 | 24 | 0 | 0 | 0 |

They come from rAthena fork commit `5e50b7ad9`, which adds `record_mount_status` to the recorder. With it:

```text
WATHENA_RECORD_ONLY=mount_status bash tools/wathena/record.sh renewal pre-renewal
```

## Party sharing

`party_share.json` holds party exp sharing and party loot, one file per mode:

- `exp_cases` kill a monster through rAthena (`mob_dead` and `party_exp_share`) and list every `pc_gainexp` call in `gained`, with the base and job exp passed in, before `pc_calcexp`.
- Exp cases cover no party, parties with each take and even share, two parties, members that did no damage, dead members, members on another map, 1 to 5 members, and `party_even_share_bonus` at 0 and 10. Each case runs against Poring (1002), Orc Warrior (1023) and Raydric (1163), with player base levels from 1 to 99, so the renewal level penalty differs per member.
- `loot_cases` drop one Jellopy with `map_addflooritem` and take it with `pc_takeitem`. They cover the party item options 0 to 3, `party_item_share_type` 0 to 3, the first, second and third looters and other players, pick-up times between the protection ticks, and members that are dead, on another map, or have a full inventory.
- For random picks, `candidates` lists the players in rAthena's order and `drawn` lists each index `rnd_value` drew, so a test can replay the pick. `itemc_before` and `itemc_after` are the round robin counter.
- The header holds the `battle_config` values the cases use, at rAthena's defaults. The file's `note` explains every field.

| Mode | Exp cases | Loot cases |
|---|---:|---:|
| renewal | 69 | 108 |
| pre-renewal | 69 | 108 |

They come from rAthena fork commit `50df331ca`, which adds `record_party_share` to the recorder. With it:

```text
WATHENA_RECORD_ONLY=party_share bash tools/wathena/record.sh renewal pre-renewal
```

## Guilds

Guilds read one extract and one recording per mode:

- `extracts/<mode>/guilds.json`: `exp_guild.yml` and `guild_skill_tree.yml` with their imports for the mode, as `GuildExpDatabase` and `GuildSkillTreeDatabase` load them. It also has the guild constants rAthena compiles in the mode (`MAX_GUILD`, `MAX_GUILDPOSITION`, `MAX_GUILDALLIANCE`, `MAX_GUILDEXPULSION`, `MAX_GUILDLEVEL`, `MAX_GUILDSKILL`, `MAX_GUILD_STORAGE`, `GUILD_PERM_*`), the guild storage size, and the guild battle settings with their defaults and limits.
- `guild.json`: cases that run rAthena's own code. The file's `note` explains every field.

`guild.json` has four kinds of cases:

- `tax_cases` give exp through `pc_gainexp` to a member whose position has a tax rate of 0, 1, 5, 10, 33, 50, 99 or 100, with `exp_flag` 0, 1 and 2, for base exp from 1 to 2147483647. Each case has the tax `guild_payexp` took and the exp the player got. A few cases add a Battle Manual, to show that the tax comes before `pc_calcexp`.
- `kill_cases` kill Orc Warrior (1023) and Raydric (1163) through `mob_dead`, with taxed members alone and in parties with even share and each take. They list every `pc_gainexp` call, every tax, and the exp each player got, so the order of party share, level penalty and tax shows.
- `skill_cases` cast Battle Orders, Regeneration (levels 1 to 3), Restore and Emergency Call through `unit_skilluse_id`, on a map with and without `MF_GVG`. The guild has members in range, at the edge of the range, out of range, dead, on another map, and with calls turned off, plus a player of another guild and a player of an allied guild. Each case has the check results, the statuses, HP and SP and stats of every player, the caster's skill blocks, and where Emergency Call put each player. Some cases cast as a member who is not the master, cast a second skill right after the first, or change `emergency_call`.
- `relog_cases` show the master's skill blocks after `guild_recv_info` and `guild_member_joined`. Pre-renewal blocks the four skills for `guild_skill_relog_delay` when the first guild info arrives with the master online, and when the master logs in. Renewal does not.

Points to know:

- Renewal blocks only the cast skill for its `Cooldown`. Pre-renewal blocks all four skills for the cast skill's `Duration2`.
- In pre-renewal, `GD_GUILD_STORAGE` is not a guild skill (`MAX_GUILDSKILL` is 15), so with `OFFICIAL_GUILD_STORAGE` at `PACKETVER` 20211103 no guild can open its storage. `guilds.json` shows this in `storage`.
- Times of timers and blocks come from the real clock, so they can be 1 or 2 ms more than the setting.
- `guild_gm_changed` is not recorded. It blocks the new master's skills only for a player with a client connection, which recorder players do not have.

| Mode | Tax cases | Kill cases | Skill cases | Relog cases | Guild levels | Guild skills |
|---|---:|---:|---:|---:|---:|---:|
| renewal | 225 | 10 | 17 | 4 | 49 | 19 |
| pre-renewal | 225 | 10 | 17 | 4 | 49 | 15 |

They come from rAthena fork commit `f30328d04`, which adds `record_guild` to the recorder, `build_guilds` to `tools/wathena/extract.py`, and small hooks in `guild.cpp`, `pc.cpp` and `clif.cpp` for the recorder. With it:

```text
WATHENA_RECORD_ONLY=guild bash tools/wathena/record.sh renewal pre-renewal
python3 tools/wathena/extract.py guilds
```

`extract.py guilds` writes only `guilds.json`, so the other extracts stay as they are.

## Quests

`extracts/<mode>/quests.json` is `quest_db.yml` with its imports for the mode, as `QuestDatabase::parseBodyNode` loads it ([decision 0043](../../docs/decisions/0043-quests.md)). The converter makes core's `data/quests.yml` from it.

- Each quest has its ID and title, its time limit, its targets in order, and its item drops. The file's `note` explains every field.
- A `TimeLimit` with "+" is `{seconds}`, as `solve_time` adds it without daylight saving. One without is `{at}`, the time of day in seconds, with `days` or `weekday`, as `split_exact_quest_time` reads it.
- A target with `Mob` names a monster and reads only `Count`. A target without it keeps the level, race, size and element filters, the map, the map's display name, and the `MapMobTargets` that are true. A map the map server does not load counts everywhere, as `map_mapindex2mapid` gives -1.
- Monsters and items are resolved by AegisName, as rAthena does. A drop of an item that does not stack has a count of 1. An entry that `parseBodyNode` refuses is left out and listed in `skipped`.

| Mode | Quests | With a time limit | With targets | With item drops |
|---|---:|---:|---:|---:|
| renewal | 4,821 | 872 | 687 | 52 |
| pre-renewal | 2,879 | 231 | 349 | 0 |

They come from rAthena fork commit `60bed4ccf`, which adds `build_quests` to `tools/wathena/extract.py`. With it:

```text
python3 tools/wathena/extract.py quests
```

`extract.py quests` writes only `quests.json`, so the other extracts stay as they are.

## Castles

`extracts/<mode>/castles.json` is `castle_db.yml` of the mode, as `CastleDatabase::parseBodyNode` and `loadingFinished` load it ([proposal 0004](../../docs/proposals/0004-sieges.md), section 1). The converter makes core's `data/castles.yml` from it.

- Each castle has its ID, map, name, the NPC that hears `OnGuildBreak`, its edition (`fe`, `se` or `te` for rAthena's `Type`) and its client ID. `teleport` is set only when `loadingFinished` keeps `WarpEnabled`, with `WarpCost` (100 by default) and `WarpCostSiege` (100,000 by default).
- Pre-renewal's `castle_db.yml` enables no teleport, so no pre-renewal castle has one.

`extracts/<mode>/map_flags.json` holds the `gvg`, `gvg_noparty`, `gvg_castle`, `gvg_dungeon`, `gvg_te`, `gvg_te_castle` and `restricted` mapflag lines of the NPC files the mode loads, as `npc_parse_mapflag` reads them. A `restricted` map lists its zones. gvg-rules and instances-core read it.

| Mode | Castles | fe | se | te | With a teleport | Maps with flags | With `restricted` |
|---|---:|---:|---:|---:|---:|---:|---:|
| renewal | 44 | 24 | 10 | 10 | 30 | 249 | 187 |
| pre-renewal | 34 | 24 | 10 | 0 | 0 | 121 | 69 |

They come from rAthena fork commit `f99a928d1`, which adds `build_castles` and `build_map_flags` to `tools/wathena/extract.py`. With it:

```text
python3 tools/wathena/extract.py castles map_flags
```

That writes only these two files, so the other extracts stay as they are.

## Spawns

`extracts/<mode>/spawns.json` holds every map's spawn lines from `npc/<mode>/mobs`, with each map's walkable cells. Classic worlds spawn from it, with each line's `boss` flag and `respawn_variance_ms` (review decision 55). `tools/spawntags` reads the renewal file to derive core's spawn tags for procedural worlds: each monster's biome weights by map family, its role, and the densities per walkable cell ([decision 0042](../../docs/decisions/0042-spawns.md)). The map families and their biomes are wAthena's, in `tools/spawntags/families.go`, so the extract's own `biome` and `monster_tags` fields are not read.

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
