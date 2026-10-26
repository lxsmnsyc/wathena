# 0004: Castles and Sieges

Castles are rAthena's guild castles: maps a guild owns, with an economy, a defense, guardians, a Kafra, a treasure room and flags that show the owner's emblem. A siege is rAthena's War of Emperium (WoE): at set hours the castle maps become guild versus guild (GvG) maps, and the guild whose member breaks a castle's Emperium takes the castle. This proposal covers the castle records, the saved castle state, the siege editions and their events, the GvG map flags and every battle rule they change, guardians and the Emperium, flag emblems, the Lua API `wa.castles`, the converter, the protocol, the admin tools and the database rows. It adds to API 1.0 as proposal 0002's review decisions do.

Words used here:

- **Edition**: rAthena's WoE type. `fe` is First Edition (WoE 1, `agitstart`), `se` is Second Edition (WoE 2, `agitstart2`), `te` is Training Edition (WoE:TE, `agitstart3`, renewal only).
- **Siege**: an edition that runs, rAthena's `agit_flag`, `agit2_flag` or `agit3_flag`.
- **GvG map**: a map where rAthena's `map_flag_gvg` holds. **GvG ground**: a map where `map_flag_gvg2` holds, whether a siege runs or not. Section 5 defines both.

## Problem

- The converter refuses every castle and siege command (tools/convert/npcguild.go:188), and `jobcanentermap` (tools/convert/npcgroup3.go:370). 0002b category C lists 21 commands and 14 labels. The extract leaves out the `guild`, `guild2` and `guild3` folders: 1,267 renewal script NPC lines (decision 0065).
- `maps.set_flag` refuses `gvg`, `gvg_noparty`, `gvg_castle`, `gvg_dungeon`, `gvg_te` and `gvg_te_castle` with "sieges do not exist" (server/zone/maps.go:1028). Only `nomemo` has an effect.
- There is no player versus player combat at all. `isEnemy` (server/zone/query.go:183) makes players enemies of monsters only, and `PvP is not ported`.
- `Zone.GvG` (server/zone/zone.go:574) is a test switch that only the guild skills read. A dozen places note a GvG rule that waits for sieges: the guild skills and Emergency Call (skills_guild.go:42), revival (skills_priest.go:87), Endure, Devotion and Chase Walk (statuschange), the flee penalty (status/changes.go:361), the Emperium's damage (battle/weapon.go:871), castle bonuses of guardians (mob/stats.go:44), movement skills (skills_knight.go:325), cast cancel bonuses (cast.go:414).
- `skill.Def.NoCast` (skill_nocast_db's bits) is loaded and never checked. `status.Equipment.MapType` is always `item.MapNormal` (status/equip.go:250), so `item_noequip` never applies.
- There is no castle record, no saved castle state, no guardian and no siege schedule. The client has no castle window, does not draw emblems on flags, and has no way to attack another player.

What rAthena does:

- **Records.** `db/re/castle_db.yml` has 44 castles, `db/pre-re/castle_db.yml` 34 (`CastleDatabase::parseBodyNode`, guild.cpp 275 to 476): `Id`, `Map`, `Name` (such as "Kriemhild"), `Npc` (the NPC whose `OnGuildBreak` runs), `Type` (`First_Edition`, `Second_Edition`, `Third_Edition`), `ClientId`, `WarpEnabled`, `WarpX`, `WarpY`, `WarpCost` (100) and `WarpCostSiege` (100,000). 20 FE castles (aldeg, gefg, payg, prtg), 4 novice castles typed FE without a client ID (`nguild_*`), 10 SE castles (schg, arug), and in renewal 10 TE castles (te_aldecas1 to 5, te_prtcas01 to 05).
- **State.** `struct guild_castle` (mmo.hpp 780 to 807) and the table `guild_castle` (sql-files/main.sql 500 to 521): `guild_id`, `economy`, `defense`, `triggerE` and `triggerD` (the day's investments), `nextTime`, `payTime`, `createTime` (unused), `visibleC` (the Kafra is hired) and `visibleG0` to `visibleG7` (each of 8 guardians is installed). Scripts read and write them with `getcastledata` and `setcastledata` and the `CD_` indexes 1 to 17 (mmo.hpp 810 to 824). Each write saves at once (`guild_castledatasave`, guild.cpp 2403; int_guild.cpp 1822). Guardian HP is never saved: `guardianinfo` reads the live monster.
- **Load.** At the first char server connection the map server loads every castle (`guild_castle_map_init`), then runs `OnAgitInit`, `OnAgitInit2` and `OnAgitInit3` on every NPC once the owners' guild data has arrived (`guild_castledataloadack`, guild.cpp 2496 to 2540).
- **Sieges.** `agitstart`, `agitstart2` and `agitstart3` set the edition's flag and run `OnAgitStart`, `OnAgitStart2` or `OnAgitStart3` on every NPC (`guild_agit_start`, guild.cpp 2545 to 2630); the ends likewise. `agitcheck` 1 to 3 read the flags. The engine does nothing per castle: scripts do the rest.
- **Schedule.** Scripts, not the engine. `npc/guild/agit_controller.txt` runs FE on Tuesday and Thursday 21:00 to 23:00 and Saturday 16:00 to 18:00 through `OnClock` labels that fall into `OnAgitInit` and test `gettime`. `npc/guild2/agit_start_se.txt` runs SE Tuesday and Thursday 18:00 to 21:00 and Saturday 21:00 to 23:00. `npc/re/guild3/agit_start_te.txt` runs TE on Sunday 20:00 to 21:00 (`OnSun2000`, `OnSun2100`). The `OnAgitInit` paths start a siege when the server boots inside its window.
- **A castle in a siege** (`Gld_Agit_Manager`, npc/guild/agit_main.txt 60 to 315): `OnAgitStart` runs `maprespawnguildid <map>,<owner>,6` and `gvgon <map>`, then spawns the Emperium (monster 1288) with the label `Agit#<map>::OnAgitBreak`. The label is the capture: economy and defense lose 5, `setcastledata <map>,CD_GUILD_ID,<breaker's guild>`, the investments and Kafra reset, the guardians go unless the new owner has Guardian Research (10002), everyone else is warped out, the flags change, and the Emperium comes back while the siege runs. `OnAgitEnd` runs `gvgoff`. SE castles add guardian stones, barricades with `setwall` and repair minigames; TE castles add mission spawners and the job restriction. `OnAgitBreak` is not an engine event: it is the Emperium's monster label.
- **Guild break.** `guild_broken` runs `<Npc>::OnGuildBreak` for each castle the guild owns, then sets its owner to 0, which frees its guardians (`castle_guild_broken_sub`, guild.cpp 2145 to 2159).
- **Guardians.** `guardian(map, x, y, name, mob{, event}{, index})` spawns a monster owned by the castle's guild (`mob_spawn_guardian`, mob.cpp 835 to 936). With an index of 0 to 7 it is one of the castle's 8 guardians; without one it is a temporary guardian, such as an SE barricade. A guardian that dies clears its installed bit (mob.cpp 2966). Owner changes move guardians to the new guild or free them (`mob_guardian_guildchange`, mob.cpp 3690). Castle defense and Guardian Up raise their stats (status.cpp 2896 to 2919).
- **Investment.** Entirely in scripts: the steward's cost tables (economy 5,000 to 955,000 zeny by level band, defense 10,000 to 1,910,000, four times the cost for a second investment in a day, at most 2 a day, a cap of 100), and `OnClock0001` in the treasure NPC, which adds the day's investments, rolls Permanent Development (10014), resets the counters and spawns `economy / 5 + 4` chests. Renewal's `npc/re/guild/invest_main.txt` adds the Hall of Abyss funds.
- **Castle storage.** None. `storage_type` has no castle kind; the castle Kafra opens guild storage, and treasure chests are monsters.
- **The client.** `ZC_GUILD_AGIT_INFO` lists the owned castles, `ZC_REQ_ACK_AGIT_INVESTMENT` shows a castle's economy and defense to its owner, and `CZ_REQ_MOVE_GUILD_AGIT` teleports a member to the castle for `WarpCost`, or `WarpCostSiege` during the castle's siege (clif.cpp 3366 to 3508). The map property packet marks GvG maps so the client shows the attack cursor on enemies (clif.cpp 6884 to 6919, 10852). A flag NPC shows its guild's emblem through its unit packet's guild fields (clif.cpp 1046 to 1090).

What the scripts use (FE, SE, TE): `getcastledata` 121, 81, 41; `setcastledata` 18, 15, 16; `flagemblem` 40, 32, 12; `getcastlename` 4, 2, 4; `guardian` 2, 6, 2; `guardianinfo` 3, 0, 3; `maprespawnguildid` 2 each; `setwall`, `delwall`, `checkwall` in SE only; `jobcanentermap` and `requestguildinfo` in TE only; `awake` once (agit_main_se.txt 601). `warpguild`, `getmapguildusers`, `getguildinfo`, `guildskill` and `woetimer` are not used in these folders. Labels: `OnAgitInit` 3, `OnAgitInit2` 34 (mostly flags), `OnAgitInit3` 4, `OnAgitStart`, `OnAgitEnd` and their 2 and 3 variants once or twice each, `OnGuildBreak` 4, `OnRecvCastle<code>` 99 (broadcast by the castles' own `donpcevent`), `OnClock0001` 3. Around them: `duplicate` 1,000, `strnpcinfo` 634 lines, `donpcevent` 80, `monster` 115, `getd`/`setd`, `mapannounce`, `announce` with `bc_woe`, `killmonster`, `enablenpc`/`disablenpc`, `getgdskilllv`, `is_guild_leader`, `getguildname`, `getguildmaster`, NPC timers (SE barricades), `setunitdata UMOB_DMGIMMUNE` and `specialeffect`. Global `::Label` events match without regard to case (npc.cpp 1295), so `OnRecvCastlear01` and `OnRecvCastleAr01` are the same label.

## Change

### 1. Castle records

A new data kind, `data/castles.yml` (`mod.CastleKind`: `CoreNames`, `LocalNames`, fixed `id`). Core's file is converted from `castle_db.yml` of the mode. A core record's name is rAthena's `Name`, such as `Kriemhild`; a mod's records are `<mod>:<name>`.

```yaml
- name: Kriemhild
  id: 15                     # rAthena's Id; mods' records get IDs from 10,000
  map: prtg_cas01
  edition: fe                # fe, se or te
  npc: "Agit#prtg_cas01"     # the NPC that hears OnGuildBreak
  client_id: 1               # the client's castle list ID; 0 for none
  teleport: {x: 112, y: 183, cost: 100, siege_cost: 100000}   # omitted: no teleport
```

- `id` and `map` are fixed: a mod cannot change them on a record it did not add. A mod may change `name`'s display (`display_name`, the name without the mod prefix by default), `npc`, `client_id` and `teleport`. `teleport.cost` and `teleport.siege_cost` are economy fields (`Economy` of the kind), so changing core's needs `data.core_override`.
- At load, `map` must run in the world, a map has at most one castle, `teleport` must be a walkable cell inside the map, and `edition: te` needs a renewal ruleset. A record that fails is left out with a warning that names it.
- The extract adds `castles.json` (from `castle_db.yml`) and `map_flags.json` (the `gvg`, `gvg_noparty`, `gvg_castle`, `gvg_dungeon`, `gvg_te`, `gvg_te_castle` and `restricted` lines of `npc/mapflag` and `npc/re/mapflag`) for both modes. The fork's `tools/wathena/extract.py` writes them. instances-core reads base map flags from the same file.
- A castle's map gets `gvg_castle` (fe, se) or `gvg_te_castle` (te) from its record, whether or not the flag file lists it.

### 2. Castle state

The world keeps one castle registry (`server/castle`), as it keeps the social registry: every zone reads and writes it under its lock, and it saves each change through a queued store that never blocks the caller, as `social.Store`.

| Field | rAthena | Type | Notes |
|---|---|---|---|
| `owner_id` | `CD_GUILD_ID` (1) | guild ID or 0 | |
| `economy` | `CD_CURRENT_ECONOMY` (2) | 0 to 2,147,483,647 | Scripts cap it at 100. |
| `defense` | `CD_CURRENT_DEFENSE` (3) | same | |
| `invested_economy` | `CD_INVESTED_ECONOMY` (4), `triggerE` | same | The day's investments. |
| `invested_defense` | `CD_INVESTED_DEFENSE` (5), `triggerD` | same | |
| `next_time`, `pay_time`, `create_time` | 6, 7, 8 | same | Unused by rAthena's scripts; kept so `getcastledata` reads what `setcastledata` wrote. |
| `kafra` | `CD_ENABLED_KAFRA` (9), `visibleC` | boolean | |
| `guardians` | `CD_ENABLED_GUARDIAN00` to `07` (10 to 17), `visibleG0` to `7` | 8 booleans | Installed, not alive. |

What is saved: every field above, per castle. What is not saved: guardian HP (rAthena does not save it either), the live guardian and Emperium monsters, treasure chests (rAthena's header says they are lost on a crash), the flag NPCs' emblems (they follow the owner at `castles_ready`), SE barricade and stone state (in `$agit_*` server variables, which convert as server variables and are saved as those are), and whether a siege runs (section 4).

**Writes.** A write sets the value in memory and queues an upsert of the castle's row, as rAthena saves each `setcastledata`. Side effects, as `guild_castledatasave` (guild.cpp 2403 to 2461):

- Owner change: each living guardian of the castle takes the new owner, its emblem and Guardian Up level, or is removed when the castle has no owner (`mob_guardian_guildchange`). The Emperium's guild follows too. The `castle_change` Lua event fires (section 8). Nothing else happens: scripts warp players, kill the Emperium and change flags, as in rAthena.
- Defense change: each living guardian's stats are worked out again (`status_calc_mob`).
- A guardian's installed bit: set by `castle:set_guardian`, cleared by the guardian's death (mob.cpp 2966).

**Guild break.** When a guild breaks up (`social.Registry.guildBreak`), the registry runs, for each castle the guild owns, the `OnGuildBreak` NPC event on the record's `npc` (with no player), then sets the owner to 0, as `castle_guild_broken_sub`. The database's foreign key sets `owner_id` to 0 as well, as int_guild.cpp 1436 deletes the row's owner.

**Migration 00043, `castles`:**

```sql
-- Castles (proposal 0004). One row per castle record, keyed by the
-- record's full name, as world events key their rows. A row is written
-- whenever a field changes, as rAthena's guild_castledatasave.
CREATE TABLE castles (
    castle text PRIMARY KEY CHECK (char_length(castle) BETWEEN 1 AND 160),
    owner_id bigint REFERENCES guilds (id) ON DELETE SET NULL,
    economy integer NOT NULL DEFAULT 0 CHECK (economy >= 0),
    defense integer NOT NULL DEFAULT 0 CHECK (defense >= 0),
    invested_economy integer NOT NULL DEFAULT 0 CHECK (invested_economy >= 0),
    invested_defense integer NOT NULL DEFAULT 0 CHECK (invested_defense >= 0),
    next_time integer NOT NULL DEFAULT 0 CHECK (next_time >= 0),
    pay_time integer NOT NULL DEFAULT 0 CHECK (pay_time >= 0),
    create_time integer NOT NULL DEFAULT 0 CHECK (create_time >= 0),
    kafra boolean NOT NULL DEFAULT false,
    -- guardians holds the installed bits of guardians 0 to 7.
    guardians smallint NOT NULL DEFAULT 0 CHECK (guardians BETWEEN 0 AND 255),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX castles_owner ON castles (owner_id);
```

The game role gets `SELECT, INSERT, UPDATE` (no delete: a castle record a mod removed keeps its row, so putting the mod back restores the castle). `owner_id` NULL reads as 0. Values are `integer` since rAthena's columns are `int(11) unsigned` and scripts never pass 2^31.

### 3. Investment rules

Investment stays in scripts, as rAthena: the steward's cost tables, the limit of 2 investments a day, the cap of 100, the daily `OnClock0001` that moves `invested_*` into `economy` and `defense` and spawns chests, and Permanent Development's roll. wAthena guarantees only what the scripts need from the engine:

- The fields read back what was written, and survive restarts (section 2).
- `OnClock0001` runs in a castle zone even when no player is there: castle zones start their NPCs at world start (section 4), and server clock schedules wake a sleeping zone (review decision 60).
- Treasure chests are monsters with labels (decision 0065). A treasure room's `$@<map>_treasure` bits are `$@` variables, lost on restart as rAthena's.
- Renewal's `invest_main.txt` (Hall of Abyss funds, `F_Invest_Status`) uses only `$` variables, `gettime` and items: it converts with the folder (sieges-content).

**Castle storage.** None: rAthena has none. The castle Kafra opens the guild storage through `F_CanOpenStorage`, which converts today.

### 4. Sieges

The registry holds three booleans, one per edition, in memory.

- `castles.start_siege(edition)` (`agitstart`, `agitstart2`, `agitstart3`): returns false when that edition runs. Otherwise it sets it, logs an info line, and sends the start to every zone (below).
- `castles.end_siege(edition)`: the reverse.
- `castles.siege_running(edition)` (`agitcheck` 1 to 3), and `castles.siege_running()` with no edition (`is_agit_start`, any edition).

**What a start or an end sends**, in this order, after the current step:

1. Every zone updates its GvG state (section 5): a castle map of that edition becomes a GvG map. Players on it get `MapRules` (section 10). An end runs `unit_stopattack` for every unit of maps that stop being GvG maps, as `map_setmapflag` does for `gvg` off.
2. The NPC event `OnAgitStart`, `OnAgitStart2` or `OnAgitStart3` (ends: `OnAgitEnd`, `OnAgitEnd2`, `OnAgitEnd3`) goes to every NPC with a handler of the label, as `world.fire_npc_event` does, with no player. Review decision 60's rule starts a zone whose NPC sources name the label.
3. The Lua event `siege_start` or `siege_end` (section 8) runs in mod and event contexts of every zone.

**Load and `castles_ready`.** At start, the world loads every castle row before any zone runs; a record without a row starts at zeros. When all zones are up, the world sends `OnAgitInit`, `OnAgitInit2` and `OnAgitInit3` as NPC events with no player, as `guild_castledataloadack` does once the owners' guild data arrived (guilds are always loaded in wAthena). By review decision 60's rule this starts every zone whose NPC sources name one of the labels: the castle maps, the guild fields and the towns with flags. That is the point: guardians respawn from their installed bits, unowned castles spawn their monsters, flags take their owner's emblem, and the controllers' `OnAgitInit` paths start a siege the server booted into. A zone that starts its NPCs later (a script reload) runs the labels for its NPCs as it starts them, as npc.cpp 6133 runs them on reload. The Lua event `castles_ready` runs the same way for mod and event contexts.

**Schedule.** The schedule is rAthena's: the converted controllers `Agit_Event`, `Agit2_Event` and `woe_TE_contoller`, NPCs without a map that start with the world (decision 0062). Their `OnClock` labels convert to `schedules.daily` (tools/convert/npcscript.go 238), and their `gettime` checks to `clock` reads. A server owner who wants other hours replaces the controller record in a mod's `data/npcs.yml`, as with any NPC, or turns it off and starts sieges from the admin screens or a schedule of their own. There is no schedule setting in the server config (question 2).

**Restart during a siege.** Siege state is not saved, as rAthena's `agit_flag`. After a restart:

- Castle owners and values are as last written, since each write saves. A capture that set the owner is kept; the steps of its label after the restart point are lost (a respawned Emperium, an announcement), as in a rAthena crash.
- The controllers' `OnAgitInit` path starts the siege again when the clock is inside its window, and `OnAgitStart` runs on every castle as at a normal start: players are warped out by `maprespawnguildid`, the Emperium spawns.
- Guardians respawn at full HP from their installed bits.
- A planned restart warns players first (`restart_planned`), and the admin World screen shows running sieges (section 11), so staff can wait for a siege's end.

**Teleport to a castle.** `CastleTeleport` (section 10) warps a member of the owning guild to the record's `teleport` cell for `cost` zeny, or `siege_cost` while the castle's edition runs, as `clif_parse_guild_castle_teleport_request`: refused without a teleport cell, for a guild that does not own it, from a castle map, a `noteleport` or a `nowarp` map (both flags not built yet, so only the castle map check applies), or without the zeny.

### 5. GvG map flags and battle rules

**Flags.** `maps.set_flag` and `maps.get_flag` accept `gvg`, `gvg_noparty`, `gvg_castle`, `gvg_dungeon`, `gvg_te`, `gvg_te_castle` and `restricted` (with a zone number), with the effects below. They start from `map_flags.json` and the castle records (section 1). Setting one of them needs `maps.control`, since it makes players of a map able to kill each other (question 8). rAthena's exclusions hold: `gvg` or `gvg_te` on clears `pvp` and `battleground`; `gvg_castle` and `gvg_te_castle` exclude each other (map.cpp 4767 to 4838). `gvgon` and `gvgoff` are `set_flag(map, "gvg", ...)`; `gvgon3` and `gvgoff3` are `set_flag(map, "gvg_te", ...)`.

**Predicates**, as map.hpp 932 to 1089, worked out by each zone from its flags and the siege state and kept as fields, so hot paths read a field:

| Zone field | rAthena | True when |
|---|---|---|
| `gvg` | `map_flag_gvg` | `gvg`, `gvg_te`, `gvg_castle` while FE or SE runs, `gvg_te_castle` while TE runs |
| `gvgGround` | `map_flag_gvg2` | any of `gvg`, `gvg_te`, `gvg_castle`, `gvg_te_castle`, siege or not |
| `vs` | `map_flag_vs` | `gvg` above, or `gvg_dungeon` (PvP and battlegrounds not ported) |
| `mapType` | the restriction bits | 1 when not `vs2`, 4 on `gvg`/`gvg_castle` (`gvg2_no_te`), 16 on `gvg_te`/`gvg_te_castle` (`gvg2_te`), and the `restricted` zone bit (zone n is `1 << (n + 4)`, map.cpp 4872) |

`Zone.GvG` goes; its readers use these fields. Most combat rules use `gvgGround`, so they hold on castle maps outside sieges too, as in rAthena. Targeting and the flee penalty use `gvg` and `vs`.

**Rules**, each with its rAthena source. Defaults are ruleset rules (`provides.ruleset.rules`) with rAthena's values from conf/battle/guild.conf, skill.conf and misc.conf:

Targeting:

- On a `vs` map players are enemies of each other, except members of the same party (unless the map is `gvg` and `gvg_noparty`), and members of the same or an allied guild; an alliance beats an enemy relation (battle_check_target, battle.cpp 8143 to 8207). Outside `vs` maps players stay friends, as now.
- Guardians and the Emperium belong to the castle's owner: they are friends of its members and allies, enemies of everyone else on `gvg` maps, and cannot be targeted and do not attack outside `gvg` maps when the castle has an owner (battle.cpp 8024, 8098, 8151). A guildless player cannot target the Emperium on a `gvg` map (8089).
- The Emperium takes hits only from a player whose guild has Official Guild Approval (GD_APPROVAL, 10000), and from monsters that are not clones; a skill hits it only with `INF2_TARGETEMPERIUM` (renewal: `HW_GRAVITATION`, `KO_MAKIBISHI`; pre-renewal adds `MO_TRIPLEATTACK`); a target with `MD_SKILLIMMUNE` takes no skill hit (`battle_can_hit_gvg_target`, battle.cpp 2145 to 2172).
- Rule `guild_max_castles` (0, no limit): a guild that owns that many castles cannot hit guardians or the Emperium (battle.cpp 2160).

Damage (`battle_calc_gvg_damage`, battle.cpp 2183 to 2209), on `gvgGround`, after `battle_calc_damage` and before the result:

- Normal attacks: short range times `gvg_short_attack_damage_rate` (80), long range times `gvg_long_attack_damage_rate` (80). Skills: weapon times `gvg_weapon_attack_damage_rate` (60), magic `gvg_magic_attack_damage_rate` (60), misc `gvg_misc_attack_damage_rate` (60). At least 1. Skills with `INF2_IGNOREGVGREDUCTION` are left alone (renewal: `NJ_ZENYNAGE`, `GN_FIRE_EXPANSION_ACID`; pre-renewal adds `PA_PRESSURE`, `HW_GRAVITATION`).
- It applies to weapon (`battle_calc_attack_gvg_bg`, 5102 to 5154, which renewal skips for `NJ_ISSEN` and `GN_FIRE_EXPANSION_ACID`), magic (6268), misc (6688) and reflected damage, with the source's map (6871). `calcAttackGvgBg` (server/battle/weapon.go:939) gets the GvG branch; the battle package takes a small `Ground` value with the flags and rates, not a zone.
- The Emperium: in renewal its `Ignore*` modes give infinite defense, 1 per hit (battle.cpp 2901); `HW_GRAVITATION` deals 400 (6421); splash and drain skip it (7069); Heal, Highness Heal and the potion skills fail on it; Sanctuary and Apple of Idun do not heal it in renewal; Estimation, Soul Link skills and Phantom Thrust cannot target it (skill.cpp 4805). The weapon path is battle.cpp 4981 to 4997, which `calcAttackPlant`'s note (weapon.go:871) waits for.
- Sonic Blow with the Assassin spirit gives +25% on `gvgGround`, +100% elsewhere (battle.cpp 4578). `skill_damage_db`'s GvG bit (2797). `SC_GVG_GOLEM` and `SC_GVG_GIANT` already exist as statuses.

Statuses and stats:

- `gvg_flee_penalty` (20): players on `gvg` maps lose 20% of their flee (status.cpp 7617). Entering and leaving recalculates flee (clif.cpp 11065).
- Endure's hit count is not lowered by non-players, PCs get no infinite Endure, Chase Walk costs 5 times the SP, and Devotion passes only Auto Guard, Defender and Reflect Shield, on `gvgGround` (status.cpp 1588, 10858, 11264, 11472). `status_disabled.txt`'s `SC_ENDURE,28` ends Endure on GvG and TE maps.
- Owners on their own `gvg_castle` (not TE) regenerate twice as often (pc.cpp 7127, status.cpp 15541).

Skills and items:

- `skill_nocast_db` (`skill.Def.NoCast`, loaded today, checked nowhere) is checked against the zone's `mapType`, as skill.cpp 853 to 863: renewal's GvG list (Teleport, Warp Portal, Ice Wall, Back Slide, Intimidate, Call Partner and the family calls, Assumptio, Basilica, Moonlit Water Mill, Cultivation, Kirikage, Kaizel), its TE list, and zones 4 (SE castles) and 8 (TE dungeons). Item skills skip it (skill.cpp 850).
- `item_noequip` applies: `status.Equipment.MapType` becomes the zone's `mapType`, so Anodyne, Assumptio and Greed scrolls stop on GvG maps, and TE gear works only on TE maps.
- No revival on `gvgGround`: Resurrection, Redemptio, Kaizel's and Ultimate Sacrifice's auto revive fail (skills/acolyte/resurrection.cpp 28, status.cpp 1722, 1740). Dead Branches and the `IG_MF_NOTELEPORT` group (Fly Wings) cannot be used on `gvgGround` (pc.cpp 6314 to 6321).
- Battle Orders, Regeneration, Restore work only on `gvgGround` (skill.cpp 8643). Emergency Call follows `emergency_call` (11) with the real siege bit: 1 outside sieges, 2 during any siege, 4 off GvG ground, 8 on it, 16 refuses `nowarpto` maps but castles (skill.cpp 956 to 966). `guildCastBegin` (skills_guild.go:46) treats every moment as outside sieges today. Guild auras follow `guild_aura` (31) once they exist.
- `GC_DARKILLUSION` fails on `gvgGround` (950); `SR_CURSEDCIRCLE` fails near the Emperium or guardian stones (8853). Third class skills are v0.3.0; their rules come with them.
- Traps: `gvg_traps_target_all` (1) lets traps and Quagmire hit everyone on `vs` maps (skill.cpp 5836).

Knockback and movement:

- Offensive knockback is ignored on `gvgGround` (`unit_blown_immune`, unit.cpp 1377): `blownImmune` (server/zone/skills_hunter.go:215) gets the map. Trap stops still apply (`skill_trap_type` 0).
- Skills that move their caster with `skill_check_unit_movepos` flag 5 do not move it on `gvgGround` (skill.cpp 14662; server/zone/skills_knight.go:325, skills_npc_buff.go:334).
- `bNoCastCancel` does not work on `gvgGround`; `bNoCastCancel2` does (unit.cpp 3427, server/zone/cast.go:414).

Death and respawn:

- No exp or zeny death penalty on `gvgGround` (pc.cpp 10014). wAthena has no death penalty yet, so there is nothing to turn off; the rule is written down for when one comes.
- On `gvgGround` a dead player returns to its save point after 1 second (pc.cpp 10143), with full HP and SP. Elsewhere wAthena keeps its 3 seconds (`reviveMs`, server/zone/combat.go:19).

Guild actions:

- No invite, leave or expulsion of a player on `gvgGround` (guild.cpp 978, 1177, 1217); no alliance change during any siege (1869, 1990, 2026); with `guild_leaderchange_woe` (off), no master change during a siege; no master change on `gvg_castle` maps (atcommand.cpp 6882). The client gets each action's usual failure result.

Not built, since the systems do not exist: duels (`duel_allow_gvg`), pets (`pet_disable_in_gvg`), monster transformation (`mon_trans_disable_in_gvg`), AP (`loose_ap_on_map`), battlegrounds and PvP maps (the `vs` parts of them).

### 6. Guardians and the Emperium

Both are monsters with castle data, as rAthena's `guardian_data`:

- **Guardian.** `castle:spawn_guardian(spec)` places one. With `index` 0 to 7 it is the castle's guardian of that slot: it is refused while that slot's guardian lives (mob.cpp 875), and its death clears the slot's installed bit. Without `index` it is temporary, such as an SE barricade or guardian stone. It belongs to the castle's owner at spawn, shows the owner's badge with the castle's name as its position (clif.cpp 10042), and its stats get the castle bonus below. A castle without an owner spawns it with a warning, as rAthena.
- **Emperium.** Monster 1288 placed on a castle map by any spawn (`spawning.monsters`, so `monster`) gets the castle data with slot 8, as `mob_once_spawn` (mob.cpp 697). It is not in the guardian slots: an owner change updates its guild but does not remove it.
- **Stats** (`status_calc_mob` flag 4, status.cpp 2896 to 2919), for a guardian whose guild has Guardian Up (GD_GUARDUP, 10003) and for the Emperium: on castles with an rAthena ID under 24 or for the Emperium, renewal adds `50 × (defense / 5)` max HP and pre-renewal `1000 × defense`, and DEF and MDEF gain `(defense + 2) / 3`; guardians also gain `1000 × defense` max HP, `2 × guardup + 8` ATK and lose `2 × guardup + 3` ASPD rate. HP is set to the new max. Without Guardian Up a guardian gets no castle bonus. This replaces `mob.Calc`'s note (server/mob/stats.go:44).
- **`guardianinfo`.** `castle:get_guardian(index)` returns `{ index, installed, monster }`, `monster` being the living guardian's handle or nil. `guardianinfo`'s types convert over it: 0 installed, 1 max HP, 2 HP, -1 for a slot without a living guardian, as script.cpp 14674.
- **Kills.** `maprespawnguildid` with flag 4 kills every monster of the map but guardians, the Emperium and clones (script.cpp 13852). A script's kill of the Emperium skips the top damage dealer (mob.cpp 3548), so its label runs with no player.
- **Guild changes.** A guardian whose guild breaks is removed; one whose castle changes owner takes the new guild (`mob_guardian_guildchange`). A new emblem updates guardians in sight (`guild_emblem_changed`, guild.cpp 1609).

### 7. Emblems on flags and units

- `npc:set_guild(guild)` (`flagemblem`) shows a guild's badge on an NPC: name and emblem version, as the unit packet's guild fields (clif.cpp 1046 to 1090). nil removes it. It needs no permission on the context's own NPC, and `npcs.control` on another mod's. `npc.guild` reads it.
- `EntityAppear.guild` is sent for NPCs and monsters with a guild, not only players. `EntityGuild` goes to players in sight on each change. A guild's new emblem sends `EntityGuild` for every NPC and guardian showing that guild: the registry keeps, per guild, the zones that show it, as rAthena's guild flag cache.
- The client draws the emblem on NPCs with the guild flag sprite (722, `GUILD_FLAG`) from `/emblems/<guild>/<version>.png`, as it does over players (decision 0021).

### 8. Lua API: `wa.castles`

Reads need `game.read`. Changes need a new permission, `castles.manage` (approval needed, not flagged): "change castle owners and values, place guardians, and start and end sieges". Core holds it. A mod that changes a castle of another mod's record needs `castles.manage` and `npcs.control`.

Functions:

| Call | Returns | rAthena | Permission |
|---|---|---|---|
| `castles.get(name)` | `wa.Castle?` | the map argument of the castle commands | `game.read` |
| `castles.list(options?)` | `wa.Castle[]` | `castle_db` | `game.read` |
| `castles.of(guild)` | `wa.Castle[]` | `ZC_GUILD_AGIT_INFO`'s list | `game.read` |
| `castles.siege_running(edition?)` | `boolean` | `agitcheck`, `agitcheck2`, `agitcheck3`, `is_agit_start` | `game.read` |
| `castles.start_siege(edition)` | `boolean` | `agitstart`, `agitstart2`, `agitstart3` | `castles.manage` |
| `castles.end_siege(edition)` | `boolean` | `agitend`, `agitend2`, `agitend3` | `castles.manage` |

- `get(name)`: a record's full name, a plain name (the calling mod's, then core's), or a castle's map, such as `prtg_cas01`, since rAthena's scripts name castles by map.
- `list { edition, owner }`: by record ID. `owner` is a `wa.Guild`, or `false` for castles without one.
- `edition` is `"fe"`, `"se"` or `"te"`; another value raises an error. `start_siege` and `end_siege` return false when nothing changed.

`wa.Castle` (handle; two handles of one castle are equal):

| Field | Type | |
|---|---|---|
| `id` | integer | |
| `name` | string | The record's full name, such as `core:Kriemhild`. |
| `display_name` | string | `getcastlename`, such as `Kriemhild`. |
| `map` | string | |
| `edition` | `"fe"\|"se"\|"te"` | |
| `npc` | string | The record name of the NPC that hears `OnGuildBreak`. |
| `owner` | `wa.Guild?` | |
| `owner_id` | integer | 0 for none. |
| `economy`, `defense`, `invested_economy`, `invested_defense` | integer | |
| `next_time`, `pay_time`, `create_time` | integer | |
| `kafra` | boolean | |
| `teleport` | `{ x, y, cost, siege_cost }?` | |

| Method | Returns | rAthena | Permission |
|---|---|---|---|
| `set_owner(guild)` | `boolean` | `setcastledata CD_GUILD_ID` | `castles.manage` |
| `set(values)` | | `setcastledata` 2 to 9 | `castles.manage` |
| `set_guardian(index, installed)` | | `setcastledata` 10 to 17 | `castles.manage` |
| `get_guardian(index)` | `wa.Guardian` | `getcastledata` 10 to 17, `guardianinfo` | `game.read` |
| `spawn_guardian(spec)` | `wa.Monster?` | `guardian` | `castles.manage`, `npc.create` |
| `is_siege()` | `boolean` | `agitcheck` of the castle's edition | `game.read` |

- `set_owner(guild)`: a `wa.Guild` or nil. Returns false when the owner is already that guild. Section 2's side effects follow.
- `set { economy, defense, invested_economy, invested_defense, next_time, pay_time, create_time, kafra }`: changes the named fields in one write. A negative value raises an error.
- `get_guardian(index)`: index 0 to 7. `wa.Guardian` is `{ index, installed, monster }`.
- `spawn_guardian { monster, x, y, display_name, label, index }`: `monster` is a monster ID; `x` and `y` of 0 pick a free cell; `label` is a monster label as `spawning.monsters` takes (decision 0065). It returns nil when the slot's guardian lives or the castle's map does not run. On another map the castle's zone places it after the current step, as `spawning.monsters` does.

New fields on existing handles: `monster.castle` (`wa.Castle?`, for guardians and the Emperium) and `monster.guild` (`wa.Guild?`, the castle's owner); `npc.guild` (`wa.Guild?`). New methods: `npc:set_guild(guild)` (section 7); `npc:wake()` (below).

Other calls, in their modules:

| Call | Returns | rAthena | Permission |
|---|---|---|---|
| `maps.can_enter(map, job?)` | `boolean` | `jobcanentermap` | `game.read` |
| `maps.respawn_guild(map, guild_id, options)` | `integer` | `maprespawnguildid` | `player.warp`; `maps.control` with `monsters` |
| `guild:warp(map, x, y, options?)` | `integer` | `warpguild` | `player.warp` |
| `maps.count_players(map, { guild })` | `integer?` | `getmapguildusers` | `game.read` |
| `guilds.relation(a, b)` | `integer` | `getguildalliance` | `game.read` |
| `npc:wake()` | | `awake` | none on its own NPC; `npcs.control` on another mod's |

- `maps.can_enter(map, job)`: false when the job's restriction bits meet the map's `mapType`, as `pc_job_can_entermap` (pc.cpp 15470). `job` defaults to the context's player's job. Restrictions come from `job_noenter_map.txt`, converted into a field of core's `data/jobs.yml`, `no_enter` (a list of `normal`, `pvp`, `gvg`, `battleground`, `gvg_te`, `zone_1` to `zone_8`). Renewal gives 92 jobs (every third, fourth and expanded class) `gvg_te` and `zone_8`; pre-renewal none. `GroupLevelBypass` is 100 and wAthena's staff levels are 99 at most, so no one bypasses it. The same check refuses warp NPCs, script warps, Warp Portal, Emergency Call's members and `guild:warp`'s target (npc.cpp 1932, skill.cpp 4516, 6497, script.cpp 5900, 5994). It also closes decision 0065's note on `party:warp`.
- `maps.respawn_guild(map, guild_id, { members, others, monsters })`: `members` warps the guild's members on the map to their save points, `others` everyone else (guildless players too), `monsters` kills every monster but guardians, the Emperium and clones. Returns how many players moved. Flags 1, 2 and 4.
- `guild:warp(map, x, y, { target })`: moves every online member, on any map, as `warpguild`: `target` `"random"` (a random cell of `map`), `"save_point_all"` (each member's own save point), `"save_point"` (the context player's save point), or nil (map, x, y). Returns how many moved. It follows `party:warp`'s rules (decision 0065).
- `maps.count_players`'s new `guild` option (a guild ID) counts only that guild's members.
- `guilds.relation(a, b)`: rAthena's codes, -2 when `a` does not exist, -1 when `b` does not exist, 0 for none, 1 allied or the same guild, 2 enemies (script.cpp 25038). TE's castle warp uses it.
- `npc:wake()` (review decision 60): every `timers.wait` of the NPC's handlers returns at once, as `awake`. SE's steward uses it (agit_main_se.txt 601). It needs npc-api-timers' `timers.wait`.

Events (`events.on`):

| Event | Kinds | Argument | rAthena |
|---|---|---|---|
| `siege_start` | `mod`, `npc`, `event` | `wa.Siege` | `OnAgitStart`, `OnAgitStart2`, `OnAgitStart3` |
| `siege_end` | `mod`, `npc`, `event` | `wa.Siege` | `OnAgitEnd`, `OnAgitEnd2`, `OnAgitEnd3` |
| `castles_ready` | `mod`, `npc`, `event` | none | `OnAgitInit`, `OnAgitInit2`, `OnAgitInit3` |
| `castle_change` | `mod`, `npc`, `event` | `wa.CastleChange` | none (scripts broadcast `OnRecvCastle`) |

- `wa.Siege` is `{ edition }`. The option `{ edition = "se" }` keeps one edition.
- `castles_ready` runs once per context, after its setup, once castles are loaded: at world start, or when the context starts later.
- `castle_change` is `{ castle, previous_owner_id }`, after an owner change, in every zone.
- Converted scripts do not use these events: they keep rAthena's labels, which the world sends as NPC events (section 4), so a label's case and review decision 60's zone start rule work as in rAthena. New Lua uses the events (question 1).

`wa.testing`: `world:start_siege(edition)`, `world:end_siege(edition)`, and `new_player`'s `guild` option makes a test guild. A test world runs the castle's map.

Example, a castle's manager written for wAthena:

```lua
local castles = require "wa.castles"
local events = require "wa.events"
local maps = require "wa.maps"
local spawning = require "wa.spawning"
local context = require "wa.context"

local EMPERIUM = 1288

return function()
  local npc = context:get_instance()
  local castle = castles.get(npc.map)

  local function spawn_emperium()
    if maps.count_monsters(castle.map, { label = "OnBreak" }) == 0 then
      spawning.monsters { map = castle.map, x = 158, y = 192, monster = EMPERIUM, label = "OnBreak" }
    end
  end

  events.on("siege_start", function()
    maps.respawn_guild(castle.map, castle.owner_id, { others = true, monsters = true })
    spawn_emperium()
  end, { edition = castle.edition })

  events.on("npc_event", function(event)
    local guild = event.player and event.player.guild
    if not guild then return spawn_emperium() end
    castle:set { economy = math.max(castle.economy - 5, 0), defense = math.max(castle.defense - 5, 0), kafra = false }
    castle:set_owner(guild)
    maps.respawn_guild(castle.map, guild.id, { others = true, monsters = true })
    if castle:is_siege() then spawn_emperium() end
  end, { label = "OnBreak" })
end
```

### 9. Converter

| rAthena | Lua |
|---|---|
| `getcastledata(map, index)` | `R.castle_data(map, index)`: the field of section 2's table, the installed bit for 10 to 17, 0 for an unknown castle or index (script.cpp 14029). |
| `setcastledata map, index, value` | `R.set_castle_data(map, index, value)`: `set_owner(guilds.get(value))` for 1, `set` for 2 to 9, `set_guardian` for 10 to 17. |
| `getcastlename(map)` | `castles.get(map).display_name`, or `""`. |
| `flagemblem guild_id` | `npc:set_guild(guilds.get(id))`; 0 is nil, a negative ID does nothing. |
| `agitcheck()`, `agitcheck2()`, `agitcheck3()` | `castles.siege_running("fe")` and the others, as 1 or 0. |
| `agitstart`, `agitend` and the 2 and 3 forms | `castles.start_siege`, `castles.end_siege`. |
| `gvgon map`, `gvgoff map` | `maps.set_flag(map, "gvg", true/false)` |
| `gvgon3 map`, `gvgoff3 map` | `maps.set_flag(map, "gvg_te", true/false)` |
| `guardian(map, x, y, name, mob{, event}{, index})` | `R.guardian(...)` over `castle:spawn_guardian`: the game ID, or 0. |
| `guardianinfo(map, index, type)` | `R.guardian_info(map, index, type)` over `get_guardian`. |
| `maprespawnguildid map, guild, flag` | `maps.respawn_guild(map, guild, { members = flag & 1 ~= 0, others = flag & 2 ~= 0, monsters = flag & 4 ~= 0 })` |
| `warpguild map, x, y, guild` | `guilds.get(guild):warp(...)`, with `"Random"`, `"SavePointAll"` and `"SavePoint"` as targets. |
| `getmapguildusers(map, guild)` | `maps.count_players(map, { guild = guild })`, -1 for a map that does not run. |
| `getguildalliance(a, b)` | `guilds.relation(a, b)` |
| `requestguildinfo guild{, event}` | Guilds are always loaded: the event runs at once with `world.fire_npc_event`, after the current step. |
| `jobcanentermap(map{, job})` | `maps.can_enter(map, job)`, as 1 or 0. |
| `awake npc` | `world.get_npc(npc):wake()` |
| `setwall`, `delwall`, `checkwall` | As built in decision 0065. |
| `announce ..., bc_woe` | A world announce: wAthena has no per-player setting for siege announcements. |
| `OnAgitInit`, `OnAgitInit2`, `OnAgitInit3`, `OnAgitStart*`, `OnAgitEnd*`, `OnGuildBreak` | `npc_event` handlers of the label, as other named labels (section 4). |
| `OnAgitBreak`, `OnStartArena`, `OnGuardianDied`, `OnTreasureDied*` | Monster labels, as built (decision 0065). |
| `woetetimestart` | `WoeTETimeStart` is a script function of `Global_Functions.txt` over `getvariableofnpc`; it converts as one. |

- `CD_` constants stay in the converter's table and `rathena.lua`, as `IM_` and the others (proposal 0002, "Constants and IDs").
- The converter writes core's `data/castles.yml` from `extracts/<mode>/castles.json`, and the `no_enter` field of core's `data/jobs.yml` from `job_noenter.json`.
- The coverage reports count each row as converted. 0002b's C:sieges rows drop to zero, except scripts that fail for other reasons. sieges-content converts the folders and owns what is left.

### 10. Protocol

Proposal 0001's rules: new fields and messages only, so the protocol version stays 1. Field numbers are the next free ones at build time (event.proto has 114 now; proposal 0003 takes 115 and 116).

```proto
// event.proto, in Event.body
MapRules map_rules = 117;
GuildCastles guild_castles = 118;
CastleInfo castle_info = 119;
CastleTeleportResult castle_teleport_result = 120;

// MapRules is what the map's flags allow, as clif_map_property. The
// server sends it after Welcome and MapChange, and when a flag or a
// siege changes it. The client shows the attack cursor on players who
// are enemies, and allows attacking them, only when gvg is set.
message MapRules {
  // gvg is rAthena's map_flag_gvg: guilds fight here now.
  bool gvg = 1;
  // gvg_ground is map_flag_gvg2: GvG rules hold here, siege or not.
  bool gvg_ground = 2;
  // castle is set on a castle's map; siege while its edition runs.
  bool castle = 3;
  bool siege = 4;
  // no_party: party members are enemies too (gvg_noparty).
  bool no_party = 5;
}

// GuildCastles lists the castles the player's guild owns, as
// ZC_GUILD_AGIT_INFO. Sent with GuildInfo and on each owner change.
message GuildCastles {
  repeated CastleEntry castles = 1;
}

message CastleEntry {
  uint32 id = 1;
  string name = 2;
  string map = 3;
  // client_id is the client's castle number, 0 for none.
  uint32 client_id = 4;
  // teleport_cost is the zeny a teleport costs now, unset without one.
  optional uint64 teleport_cost = 5;
}

// CastleInfo answers CastleInfoRequest for the owner's members, as
// ZC_REQ_ACK_AGIT_INVESTMENT.
message CastleInfo {
  uint32 id = 1;
  uint32 economy = 2;
  uint32 defense = 3;
}

message CastleTeleportResult {
  uint32 id = 1;
  CastleTeleportOutcome outcome = 2;
}

enum CastleTeleportOutcome {
  CASTLE_TELEPORT_OUTCOME_UNSPECIFIED = 0;
  CASTLE_TELEPORT_OUTCOME_OK = 1;
  CASTLE_TELEPORT_OUTCOME_NO_ZENY = 2;
  CASTLE_TELEPORT_OUTCOME_NOT_OWNER = 3;
  CASTLE_TELEPORT_OUTCOME_NOT_HERE = 4;   // from a castle or a map that refuses it
  CASTLE_TELEPORT_OUTCOME_NO_TELEPORT = 5;
}

// command.proto, in Command.body
CastleInfoRequest castle_info_request = <next>;
CastleTeleport castle_teleport = <next>;

message CastleInfoRequest { uint32 id = 1; }
message CastleTeleport { uint32 id = 1; }
```

- `EntityAppear.guild` and `EntityGuild` carry NPC and monster badges too (section 7). `GuildBadge.position` is the castle's name for a guardian.
- Attacks on players use the existing attack command; the server checks `isEnemy`, so an older client that sends one off a GvG map is refused as now.
- The client: a castle list in the guild window with the teleport button and the economy and defense view, emblems on flag NPCs, the attack cursor and click to attack on enemy players while `MapRules.gvg` is set. It needs no change to show siege announcements: they are announce lines.

### 11. Admin and observability

- `GET /world` (`WorldView`) gains `repeated CastleView castles = 8` (`id`, `name`, `map`, `edition`, `owner_id`, `owner_name`, `economy`, `defense`, `kafra`, `guardians` installed and alive, `players`) and `repeated SiegeView sieges = 9` (`edition`, `running`, `started_at`). The World screen shows the sieges with start and end buttons, and the castles. It needs `world.read`.
- `POST /sieges/{edition}/start` and `/end` start and end an edition, as the script calls. New staff permission `world.sieges` ("start and end sieges"), held by `admin` through `world.*` and by `game_master` by default.
- `POST /castles/{id}/owner` takes `{ guild, reason }` (guild 0 for none) and runs `set_owner`. It does not fire `OnGuildBreak` or warp anyone. New staff permission `world.castles` ("set castle owners"), held by `admin` only, since it moves the economy of a castle between guilds.
- Audit actions `sieges.start`, `sieges.end` and `castles.owner`, with the edition or castle, the old and new owner, and the staff reason. Script changes are not audited; owner changes are logged at info level with the castle, the old and new owner, and the source (`script`, `staff`, `guild_break`).
- Metrics: a gauge of running editions, a counter of owner changes by source, and the castle zones' usual zone metrics.
- GM commands: `@agitstart`, `@agitend` and their 2 and 3 forms need `world.sieges` and write the same audit rows.

### 12. Procedural worlds

- The castle system, the GvG flags and the battle rules do not depend on the world mode.
- Core's castle records name classic castle maps, and their NPCs are classic converted NPCs, so they load only in classic worlds, as the rest of rAthena's fixed content (strategy section 11.4).
- A procedural world has no castles unless a mod adds records on maps it ships (after mod-assets-gaps). A generated castle (a floor type, a generated Emperium room, a placement in the world layout) is new content and needs its own design; it is not part of v0.2.0.

Recommendation: build mode-independent, load core's castles in classic worlds only, and leave generated castles to a later node.

### 13. Performance

A siege is the Crowd case of decision 0050 with combat on top:

- Load: at a full FE siege, 20 castle zones run at once. Guilds crowd the Emperium room: 50 to 200 players in view of each other in one zone, casting area skills, each hit a `Damage` event for every viewer. Strategy section 3.8's budget is a tick work p99 under 10 ms per zone. Decision 0050 found the Crowd scenario over budget from 250 players in view and 1,000 players in one town at 14.7 ms p99 thread CPU, with 30% of the samples in `viewEvents`. Decision 0055's shared view events, the fix it names, are not built.
- New per-tick costs: `isEnemy` for player pairs on `vs` maps (a guild ID compare and an alliance lookup in the attacker's guild view, no registry lock), the GvG damage rate (a multiply), and guild badge events for guardians. None needs the social registry's lock in a hot path: zones read the castle owner from a value the registry publishes on change, as `mapShared` holds map flags.
- Boot: `OnAgitInit` starts every castle zone and the flag towns at world start, about 60 zones in a renewal classic world, each with its own Lua state (decision 0056 is not built). sieges-war measures the heap and the start time this adds.

What sieges-war measures, with a new `wabots` scenario `siege` (two guilds of bots on one castle map, attacking each other and the Emperium with normal attacks and a fixed set of area skills): 50, 100, 200 and 300 bots in one castle, and 20 castles of 50 bots at once. It records the zone tick work p99 and maximum, the send phase, the bytes per player per second against section 3.8's busy town budget (8 KB/s average, 32 KB/s peaks), and the live heap at boot with castles. The target is 100 against 100 in one castle within the tick budget. If that fails, decision 0055's steps come before v0.2.0's documented siege capacity, and the release notes state the measured count.

### 14. Tests

Unit and golden tests:

- sieges-castles: record loading (core's 44 and 34, a mod record, `id` and `map` refused on another mod's record, an unknown map left out); the registry's writes, side effects and saves against PostgreSQL; a restart keeps every field; guild break runs `OnGuildBreak` and clears the owner; `castle_change`; flag emblems and their updates on a new emblem; `CastleTeleport`'s outcomes; converter golden files for the castle rows.
- sieges-war: `isEnemy` on `vs` maps for parties, guilds, allies, enemies, guardians and the Emperium; the predicates under each flag and siege state; each damage rate and its exceptions; nocast, noequip and `restricted` zones; knockback, movement skills, revival, Dead Branch and Fly Wing refusals; respawn after 1 second; the guild action blocks; Emergency Call's bits during and outside sieges; guardian stats with and without Guardian Up; the Emperium's hit rules; `maps.respawn_guild`'s flags; `maps.can_enter`; `npc:wake()`; converter golden files for the siege rows.
- Golden against rAthena: the fork's recorder gains `record_gvg`, as `record_guild` (reference/rathena/README.md, "Guilds"): normal attacks and a set of weapon, magic and misc skills between players and on guardians and the Emperium, on a map with `MF_GVG` and with `MF_GVG_CASTLE` inside and outside a siege; guardian stats at defense 0, 50 and 100 with Guard Up 0 to 3; flee with the penalty; nocast and noequip refusals; knockback. Both modes.

e2e (`e2e/`, a classic world with the kRO assets, PostgreSQL, two guilds of headless bots), the acceptance test of sieges-war:

1. Guild A owns a castle through `POST /castles/{id}/owner`; the audit log has the row. Its flags show A's emblem to a bot in town.
2. Staff start FE through `POST /sieges/fe/start`. Bots on the castle map get `MapRules.gvg`; B's bots inside were warped out by `maprespawnguildid`.
3. B's bots walk in, fight A's bots and A's guardians, and break the Emperium. The castle's owner is B, in `GET /world` and in the database; A's bots are warped out; the flags show B's emblem; the guardians installed for A go (B has no Guardian Research).
4. B's master installs a guardian at the steward; it spawns and belongs to B.
5. Staff end the siege. `MapRules.gvg` clears. B's bots can no longer attack A's.
6. Restart the server: the castle is still B's, the guardian respawns at full HP, and `castle:get_guardian(0).installed` is true.

sieges-content adds an e2e run of converted castles of each edition (one FE, one SE with its stones and barricades, one TE with the job check), from the controller's schedule on the dev clock to a capture and the next day's treasure.

## Breaks

- Players can attack and kill each other on GvG maps. Nothing else changes: no map has a GvG flag until castle records or a script set one.
- `maps.set_flag` with a GvG flag worked as an error before; it now needs `maps.control` and has an effect. `maps.get_flag` returns true for flags that the extract or castle records set.
- `Zone.GvG` is removed; tests that set it set the map's flags.
- Items with `item_noequip` bits for normal maps (flag 1, such as Siege gear) stop working outside GvG maps, and skills in `skill_nocast_db` stop working on the maps it names. Both are rAthena's behavior that wAthena did not apply.
- Emergency Call's siege bits apply: with the default 11 it still works on GvG ground only.
- `EntityAppear.guild` and `EntityGuild` can carry an NPC or a monster. Clients older than this change show no emblem on flags and ignore the new events, as proposal 0001 allows.
- `wa.Npc` and `wa.Monster` gain fields; `wa.Guild` gains `warp`; `wa.guilds` gains `relation`; `maps.count_players` gains an option. Mods that want castles request `castles.manage`.
- Migration 00043 adds a table; existing rows are untouched.

## Alternatives

- **Schedule in the server config or `data/events.yml`.** Owners could change hours without a mod. But rAthena's controllers carry the schedule and the boot check together, every castle script tests `agitcheck`, and the converted controllers run as they are. A config schedule would run beside them or replace them with a second source of truth. Not chosen; staff start and end sieges from the admin screens, and a mod replaces the controller.
- **Typed events only, no labels.** The converter would map `OnAgitStart` to `siege_start`. That loses the case-insensitive label match and review decision 60's rule that starts a zone whose NPC sources name a label, which the castles need at boot. Not chosen for converted scripts; typed events stay for new Lua.
- **Save siege state.** A siege would resume exactly after a restart. rAthena does not, and the controllers restart a siege inside its window. Not chosen.
- **Save guardian HP.** rAthena has no column for it. Not chosen.
- **A castle storage.** rAthena has none. Not chosen.
- **Castles in the guild registry.** Fewer packages, but `server/social` would grow siege state and battle hooks. A separate registry with a hook from guild break is smaller.
- **Key rows by rAthena's ID.** IDs are fixed, but names match how world events key rows and survive a mod giving a castle a new ID by mistake. Either works; names chosen.
- **Start castle zones lazily.** Saves memory at boot, but `OnClock0001` treasure, guardian respawns and flags would not run in castles nobody visited, and sieges would start zones in the same tick as their first fight. Not chosen.
- **PvP first.** GvG needs player versus player targeting; PvP maps would use the same code. PvP is not ported and is not in v0.2.0's scope; this proposal builds only the `vs` parts GvG needs, so PvP can add its flags later.

## Questions for review

1. **Labels and events.** Converted scripts keep rAthena's labels (`OnAgitStart`, `OnAgitInit`, `OnGuildBreak` and the 2 and 3 forms), which the world sends as NPC events; new Lua uses `siege_start`, `siege_end`, `castles_ready` and `castle_change`. Recommendation: both, as proposed.
2. **Schedule.** rAthena's converted controllers hold the schedule; no config setting. Recommendation: yes; staff use the admin screens, owners who want other hours replace the controller in a mod.
3. **Restart.** Siege state is not saved; the controllers restart a siege inside its window; castle values are saved on each write. Recommendation: yes, as rAthena.
4. **Castle zones at boot.** `OnAgitInit` starts the castle zones and flag towns at world start (about 60 zones in renewal). Recommendation: yes; sieges-war measures the heap and the start time.
5. **Records.** `data/castles.yml` with fixed `id` and `map`, rows keyed by record name, mod IDs from 10,000, teleport costs as economy fields. Recommendation: yes.
6. **No castle storage, no saved guardian HP.** Recommendation: as rAthena.
7. **Owner on guild break.** Run `OnGuildBreak` on the record's NPC, then clear the owner; the foreign key clears it in the database too. Recommendation: yes.
8. **GvG flags need `maps.control`.** Setting `gvg` and the others from a script needs the flagged permission. Recommendation: yes; core holds it.
9. **Mod permission `castles.manage`** (approval, not flagged), and `npcs.control` too for another mod's castle. Recommendation: yes.
10. **Staff permissions.** `world.sieges` (start and end) for `admin` and `game_master`; `world.castles` (set an owner) for `admin` only. Recommendation: yes.
11. **Ruleset rules** with rAthena's defaults: the five GvG damage rates, `gvg_flee_penalty`, `guild_max_castles`, `guild_leaderchange_woe`, `gvg_traps_target_all`, and `emergency_call` with its siege bit. Recommendation: yes.
12. **Respawn.** 1 second on GvG ground, as rAthena; wAthena's 3 seconds elsewhere. Recommendation: yes.
13. **`restricted` zones.** Build their effect on nocast, noequip and `maps.can_enter` only, since SE castles (zone 4) and TE dungeons (zone 8) need it; the rest of the flag waits for the map flag work. Recommendation: yes.
14. **`skill_nocast_db` and `item_noequip` everywhere.** Turning them on applies them to normal maps too (flag 1). Recommendation: yes, as rAthena; sieges-war lists the skills and items that change.
15. **`bc_woe`** converts as a world announce. Recommendation: yes.
16. **Castle teleport and the castle window** are in v0.2.0 (renewal's `WarpEnabled` castles). Recommendation: yes; it is small and the data is there.
17. **Procedural worlds.** No core castles there; mods may add them; generated castles later. Recommendation: yes.
18. **Performance gate.** sieges-war measures the `siege` scenario; if 100 against 100 in one castle is over budget, decision 0055 comes before the documented siege capacity. Recommendation: yes.
19. **Node split.** sieges-war as briefed holds the battle rules, the siege, guardians, the converter, the admin tools and the e2e test, more than one L node. Recommendation: split the GvG rules (section 5 and its goldens) into their own node before sieges-war (see the outcome of proposal-sieges).
