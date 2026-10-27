# 0005: Pets, Homunculi and Mercenaries

Pets, homunculi and mercenaries are units that a player owns, that follow the player from map to map, and that the server saves with the character. This proposal designs the shared base they run on, then each system: pets (taming, eggs, hunger, intimacy, bonuses, evolution), homunculi (creation, skills, levels, evolution, mutation, intimacy, hunger, vaporize, death and revive) and mercenaries (contracts, faith, skills, kill counts). It covers the data records, the Lua API, the converter, the protocol, the database rows, the client windows, the ruleset rules, the admin tools and the tests. It adds to API 1.0 as proposal 0002's review decisions do.

Words used here:

- **Follower**: a pet, a homunculus or a mercenary while it exists as a unit on a map. The word is new, because decision 0016 already calls the falcon and warg records "companions" and keeps `data/companions.yml`. Those records are not followers and do not change.
- **Egg**: the item a pet hatches from. **Intimacy** and **hunger**: the two values of a pet or a homunculus. **Vaporize**: a homunculus put away (rAthena's `HOM_ST_REST`). **Morph**: the state of a level 99 evolved homunculus waiting to mutate (`HOM_ST_MORPH`). **Contract**: a mercenary's hired time. **Faith** (the client calls it loyalty) and **calls**: a character's counters for each mercenary guild.
- **Homunculus S**: the five mutated homunculi (Eira, Bayeri, Sera, Dieter, Eleanor), renewal only.

## Problem

- Nothing here exists. `server/companion` holds option-bit records only (decision 0016): no entity, no AI, no owner link, no saved state. `EntityKind` already has `PET`, `HOMUNCULUS` and `MERCENARY` (event.proto:290), and nothing sends them.
- The converter refuses 68 `pet`, 1 `bpet`, 12 `getpetinfo`, 59 `mercenary_create`, 3 `mercenary_sc_start`, 2 `mercenary_heal`, 1 `addhomintimacy` and 1 `homevolution` item rows in renewal (55, 39, 3, 2, 1 and 1 of the first six in pre-renewal), and `gethominfo`, `hommutate`, `morphembryo`, `checkhomcall`, `getmercinfo` and `mercenary_get_faith` in NPC scripts (0002b category C). Equipment scripts of 12 items read `getpetinfo` and need pet changes to recalculate the wearer.
- `AM_CALLHOMUN`, `AM_REST` and `AM_RESURRECTHOMUN` are skill IDs in server/skill/alchemist.go with no behavior. The skills of the units (16 homunculus skills, 41 mercenary skills, 42 Homunculus S skills in renewal) exist as `skill_db` records and as skill tree data, and nothing casts them.
- The ruleset flag `homunculus` exists and does nothing (decision 0019, "Left").
- The zone's monster AI (server/zone/mobai.go) is tied to `entity.monster`: its walk, chase and attack state live in the `monster` struct. A summon (`summonThink`) borrows it. Followers must not be monsters: scripts count and kill monsters (`maps.count_monsters`, `killmonsterall`), and rAthena's `BL_MOB` checks do not include pets, homunculi or mercenaries.
- Loot: `server/zone/loot.go:213` puts an egg a monster drops into the first looter's inventory as a plain item, with no pet behind it.

What rAthena does:

- **Pets** (`pet.cpp`, `pet_db.yml`). A taming item (`TameItem`) runs `catchpet`; the client shows the roulette and the player clicks a monster; the catch rate is `CaptureRate + (100 - HP%) * CaptureRate / 100` of 10,000 (`pet_legacy_formula` off), with checks for the item, boss mode, distance 5, a free slot, hiding statuses and the map flag `nopetcapture` (`pet_catch_process_end`). Success kills the monster and makes an egg with a pet row: class, level, egg ID, intimacy `IntimacyStart` (250), hunger 100, name, equip, autofeed, `incubate` 1. The egg item carries the pet ID in `card[1..2]` and the intimacy grade in `card[3]`. Eggs do not stack. A monster whose drop is an egg item, or an NPC shop that sells one, makes the egg through `pet_create_egg`. The player hatches an egg from the egg list (`CZ_SELECT_PETEGG`; the `bpet` command opens the list). A player has one pet.
  - Hunger falls by `Fullness` every `HungryDelay` seconds (60). At 0 it stops falling, intimacy drops by `IntimacyHungry` (-5) each tick, and the tick is 20 s. Feeding (`FoodItem`, from the menu) adds `HungerIncrease` (20), up to 100. Over 90 hunger it costs `IntimacyOverfed` (-100); otherwise it adds `IntimacyFed`, halved above 75 hunger (`pet_food`). Owner death costs `IntimacyOwnerDie` (-20) unless the map has `noexppenalty`. Intimacy 0 deletes the egg and the pet runs away. Grades: awkward below 100, shy below 250, neutral below 750, cordial below 910, loyal to 1000.
  - The menu: info, feed, perform, return to egg, unequip. The accessory (`EquipItem`, type `petarmor`) is worn once; unequipping destroys it when `pet_unequip_destroy` is on. A rename is allowed once unless `pet_rename`. Autofeed (`AllowAutoFeed`, `feature.pet_autofeed`) feeds when hunger is at or under 89.
  - The pet follows the owner (`pet_ai_sub_hard`, every 100 ms): within 3 cells of the owner, at half the owner's speed when far, teleporting to the owner after 3 s beyond 14 cells. It does not attack by default (`pet_attack_support`, `pet_damage_support` off). It cannot be targeted.
  - Bonuses: the record's `Script` runs in every `status_calc_pc` while a pet exists, with no equip check, and reads `getpetinfo(PETINFO_INTIMATE)`. Of 107 renewal records, 93 have a script, all of the same shape: tiers by intimacy (loyal, cordial, neutral, shy or awkward), with `bonus`, `bonus2`, `bonus3`, `bonus5`, and in 9 records `petautobonus`. `SupportScript` (`petloot`, `petskill*`, `petrecovery`, `petheal`) runs only with `pet_status_support`, off by default, and no record has one.
  - Evolution (renewal's `Evolution:` list, `feature.petevolution`): intimacy loyal, no accessory worn, the listed items in the inventory. It swaps the egg's item and the pet's class, sets intimacy to the new record's start, and keeps the name if renamed.
  - 107 pets in renewal's db, 57 in pre-renewal's. Both modes use `pet_status_support` 0, `pet_equip_required` 1 and `pet_support_min_friendly` 900.
- **Homunculi** (`homunculus.cpp`, `homunculus_db.yml`, `exp_homun.yml`). `AM_CALLHOMUN` makes a random one of the 8 base classes (6001 to 6008) the first time, costs Germination Breed (and Seed of Life in renewal), and calls or recalls it later. Renewal also starts `SC_HOMUN_TIME` for 30 minutes, and the homunculus vaporizes when it ends. A new one has level 1, hunger 32, intimacy 2,100 of 100,000 (the client shows tenths of a percent), and base stats ten times the record's. `AM_REST` vaporizes it (HP at least 80%, 20 s cooldown in renewal). `AM_RESURRECTHOMUN` revives a dead one with `20 * skill level` percent of its HP. The owner's death vaporizes it at 80% HP or more (`homunculus_auto_vapor`). The homunculus's own death keeps the unit saved with HP 0. It has no intimacy penalty for dying.
  - Levels (`hom_levelup`): `exp_homun.yml`; growth of each of HP, SP and six stats is a random value between the record's minimum and maximum, stats truncated to whole tenths; one skill point every third level; renewal adds level/10 to each stat as the "stat weight". Max level 99 (S: 275 in the shipped conf). Exp: in renewal 10 percent (`homunculus_exp_gain`) of each owner's share of every kill that the owner took part in; in pre-renewal the full share, only for kills the homunculus's own damage entry made.
  - Hunger falls by 1 each `HungryDelay` (60 s); at 10 or less every 20 s; at 0 intimacy drops by 100 and the homunculus is deleted when intimacy reaches 0. Feeding (`Food`) adds 10 hunger and changes intimacy by hunger: from 91, -50; from 76, -5; from 26, +75; from 11, +100; else +50. Autofeed at hunger 30 or less.
  - Evolution (`homevolution`, from an item): needs intimacy 91,100 (`homunculus_evo_intimacy_need`) and an active homunculus; swaps to the evolved class (6009 to 6016), adds the record's evolution growth (HP and SP flat, stats times 10), and resets intimacy to 1,000. Mutation (`hommutate`, in `npc/re/quests/homun_s.txt`): a morph-state level 99 evolved homunculus plus a Strange Embryo (`morphembryo` makes both) becomes an S class (6048 to 6052) and keeps `prev_class`; its growth and skills use the previous record below `homunculus_S_growth_level`. `homshuffle` redoes a homunculus's growth.
  - Skills (`hom_calc_skilltree`): the record's tree with base level, intimacy, evolution and prerequisite conditions; skill points raise them. 16 skills for the four types (Lif: Healing Hands, Urgent Escape, Brain Surgery, Mental Charge; Amistr: Castling, Amistr Bulwark, Adamantium Skin, Blood Lust; Filir: Moonlight, Flitting, Accelerated Flight, S.B.R.44; Vanilmirth: Caprice, Chaotic Blessings, Instruction Change, Bio Explosion) and 42 for the S types (renewal's `MH_*`). S.B.R.44's damage is the intimacy; Bio Explosion needs 45,000 intimacy (`hvan_explosion_intimate`).
  - Settings: `hom_setting` 0x3D: support skills cannot target it except the owner's, mobs go for it first, it copies its owner's walk speed, it shows LUK-based critical, and its MATK min equals its max.
  - The kRO client runs the homunculus AI: the server never moves or attacks for it, and only does what `CZ_REQUEST_MOVE`-type commands ask. Nothing in rAthena says when a homunculus attacks or casts, so no recording can.
- **Mercenaries** (`mercenary.cpp`, `mercenary_db.yml`). An item (`mercenary_create <class>,<ms>`; 59 scrolls) makes one for the contract time, usually 30 minutes; the guild NPCs sell the scrolls for zeny, base level and faith (`npc/other/mercenary_rent.txt`, 332 lines). Classes 6017 to 6046 are the 30 guild mercenaries (10 grades each of archer, lancer, swordsman); 14 more records (IDs from 1191) and, in renewal, 20 more are used by events and instances. Pre-renewal's own file has no records; the shared file's 44 apply to both modes. A character has faith and calls for each of the three guilds, capped at 32,767 and 2,147,483,647. A call adds 1 call when the mercenary is first created.
  - The contract timer runs only while the owner is online and the rest is saved. When it ends: +1 faith. When the mercenary dies: -1 faith and it is gone. When the owner dies: it is gone, no faith change. Every 50 kills: +1 faith and a random 5 minute buff (flee, attack, HP, SP or hit) of level 1 to 5 (`mercenary_kills`). It cannot attack the Emperium. It has stats and skills from the record (`Skills:`, 41 distinct skills: `MS_*`, `ML_*`, `MA_*`, `MER_*`), a cooldown per skill, no exp, and drops only reach the owner with `mercenary_autoloot`.
  - Like the homunculus, its AI is the client's. It teleports to its owner after 3 s beyond 15 cells.

What the scripts use: `catchpet` and `bpet` in item scripts; `getpetinfo(PETINFO_EGGID)` and `(PETINFO_INTIMATE)` in equipment; `mercenary_create`, `mercenary_heal` and `mercenary_sc_start` in item scripts; `mercenary_get_faith`, `mercenary_set_faith`, `getmercinfo` in the guild and Archbishop scripts; `gethominfo`, `morphembryo`, `hommutate`, `checkhomcall` in `npc/re/quests/homun_s.txt` (11 uses) and `homevolution` and `addhomintimacy` in item scripts.

## Change

### 1. The follower base

A follower is a zone entity with a new field `entity.follower`. It is not a monster: `entity.monster` stays nil, so monster counts, labels, kills and drops never see it, and the monster-only parts of `die`, `isEnemy` and the entity set do not run for it. Its kind is `ENTITY_KIND_PET`, `ENTITY_KIND_HOMUNCULUS` or `ENTITY_KIND_MERCENARY`.

```go
// server/zone/follower.go (companion-units)
type follower struct {
    kind   companion.FollowerKind // pet, homunculus, mercenary
    owner  *Player
    state  companion.FollowerState // *PetState, *HomunculusState, *MercenaryState
    walkState                      // shared with monster
    ai     followerAI              // order, stance, target, timers
    farAt  int64                   // when the owner was first out of range
    sp     int64                   // a homunculus's and a mercenary's SP
}
```

**1.1 Owner link and map changes.** Zones run one map each and a player moves between zones by a handoff (warp.go). A follower travels in the owner's `character.Character`: `depart` removes the follower entity from the old zone, the snapshot carries its state, and `arrive` in the new zone makes the entity beside the player. So a follower is never alone in a zone, and it needs no handoff of its own. On a same-zone teleport the follower is placed beside the owner. A follower beyond 14 cells (pet, homunculus) or 15 cells (mercenary) from its owner for 3 seconds is placed next to the owner (`unit_teleport_timer`). A dead owner's follower stays where it is.

**1.2 Shared walk and attack state.** `walkToBL`, `mobUpdateChase`, `mobStepped`, `stopWalking`, `unitAttack` and `unitUnattackable` read `entity.monster`. companion-units moves the walking fields of `monster` (`chaseTo`, `chaseRange`, `walked`, `walkFails`, `attackContinue`, `attackNow`, `forceWalk`, `nextWalk`) into `walkState`, embedded in both `monster` and `follower`, and adds `entity.walker()`. The change is mechanical: field access through the embedding keeps the old names, and `mob_ai.json` goldens must still pass without edits. The monster's state machine (`m.ai.SetState`) and skills stay in `monster`.

**1.3 Status.** A follower's `e.status` is a `battle.Status`, made by its kind:

- Pet: a copy of its monster's status with mode `canmove` only (plus `canattack` when `pet_attack_support` or `pet_damage_support`), then `status_calc_misc` for `BL_PET` (rAthena recalculates it once).
- Homunculus: from the saved stats (tenths), the record's element, race and size (the evolved size after evolution), ASPD from `baseASPD`, and the skill passives (`HAMI_SKIN`, `HVAN_INSTRUCT`, `HLIF_BRAIN`, and the S passives) of `status_calc_homunculus_`. The flag `homunculus` picks renewal's or pre-renewal's DEF, MDEF, ASPD, ATK and stat weight.
- Mercenary: the record's stats, `status_calc_misc` for `BL_MER` (renewal's HIT, FLEE and DEF2 differ from monsters'), and the owner's walk speed.

`mob.calcMisc` is for monsters only. The node adds `battle.CalcMisc(unit, ...)` with `UnitMonster`, `UnitPet`, `UnitHomunculus`, `UnitMercenary`, ported from `status_calc_misc`. Its golden is `status_calc_misc` through the recorder's new `follower_status` files (section 16). Statuses (`entity.sc`) work as for monsters. `enable_baseatk`, `enable_critical` and `enable_perfect_flee` follow rAthena's per-unit defaults.

**1.4 Targeting.** `isEnemy` gains the follower rules (battle_check_target):

- A pet is never an enemy of anything and is no one's target. It has no HP loss and cannot be hit by area skills (`queryKind` leaves it out of enemy queries).
- A homunculus or mercenary is on its owner's side (the owner's ally and party member for support skills), and an enemy of what is an enemy of its owner. Monsters pick it as a target as they pick players. With `hom_setting` bit 4, monsters prefer it until it is attacked (`mob.cpp:1341`). With bit 1, support skills of anyone but the owner cannot target it. A mercenary cannot attack the Emperium. On PvP and GvG maps a follower is an enemy of an enemy player's party (when sieges add player targeting; the function takes the owner's side, so no change is needed here).
- A follower's skill, damage and kill credit go to its owner for the purposes of `battle_get_master` (guild, party and exp checks).

**1.5 AI.** rAthena runs no follower AI on the server, so wAthena specifies one. The rules are the base's; each system adds its own. The AI thinks every 100 ms while the owner is on the map.

- **Orders** from the owner, one at a time: move to a cell, attack a unit, use a skill (on a unit, a cell or self), come to me, hold. A new order replaces the old. An order ends when done, when the target is gone, or when the owner gives another.
- **Without an order**, a follower follows: it stays within 3 cells of its owner (a cell next to the owner, `unit_calc_pos`), walking at the owner's speed (at half speed from far, as the pet AI). It never goes beyond its chase range from the owner (12 cells).
- **Fighting** (homunculus and mercenary): it attacks what attacks its owner, then what the owner attacks, then an aggressive monster in its view range (the same candidate rules as `enemyCandidates`). It drops the target when it dies, goes out of the chase range, or the owner goes beyond 14 cells. It uses `unitAttack` and the normal attack path, so ASPD, range and hit rules are the monster path's.
- **Skills**: each skill of a unit has a default use rule from its kind in `skill_db` (`TargetType`, damage flags): a damage skill is tried on the current target each attack round with the record's `use.chance` (default 25 percent); a support skill on the owner or itself when HP is below `use.hp` (default 40 percent); a buff when it is not active and a fight starts. A record's skill may set `use: { when, chance, hp, sp }`. An SP or cooldown failure skips the skill silently. Core records name no `use` block except where a skill needs a condition (Castling when the owner is hit by more than one enemy, Bio Explosion never, since it kills the homunculus). These rules are not in rAthena; the system nodes write each skill's rule in their decision record.
- **Hold** keeps it in place and attacking only what comes into its attack range.

The AI is a pure function of the zone's state and its own random stream (`z.rand`), so tests replay it.

**1.6 Protocol appearance and damage.** A follower appears with `EntityAppear` of its kind, its name, `look.body` (the pet's monster ID, or the class ID), `walk_ms`, `hp_permille`, and options. `Damage`, `Move`, `Stop`, `SkillCast` and `Vanish` work as for other entities. The owner also gets the typed info events of section 8.

**1.7 Skills of followers.** `castendDamage(src, target, ...)` and the other dispatch functions take `src *entity`. A follower casts through them with `src.follower` set: SP comes from `follower.sp`, costs and cooldowns from a per-follower table (`follower.cooldowns`), and the skill's range, cast time and splash from `skill.Def` as for monsters. Skills that read `src.player` must treat a follower as its owner for ownership (guild, party, devotion) and as itself for position, status and SP. companion-units adds the helpers `z.ownerOf(e)` and `e.sourcePlayer()`; each skill that needs more is the system node's work.

**1.8 Exp and the damage log.** `progression.DamageLog` gains a source flag (`normal`, `homunculus`, `pet`), as `MDLF_*`. A follower's damage is logged under its owner with the flag. `KillWith` returns shares for the owner and, for a homunculus, its own exp (section 3). `pet_attack_exp_to_master` and `pet_attack_exp_rate` are rules.

**1.9 Kind registry.** `server/companion` gets `FollowerKind` (an interface the three systems implement): `Spawn`, `Save`, `Load`, `Think`, `OnOwnerDeath`, `OnOwnerLogout`, `Info`. A mod cannot add a kind in v0.2.0; mods add records of the three kinds.

### 2. Pets

**2.1 Egg and pet state.** An egg is an item of type `petegg` with a new `item.Extra.PetID`. The pet's state lives in the `pets` table (section 9) and in the zone as `PetState`: `id`, `record`, `name`, `renamed`, `level`, `hunger`, `intimacy`, `accessory` (item), `autofeed`, `hatched`. The egg item and the pet row are made together and removed together: a pet row with no egg item is deleted by the retention job after 7 days. Eggs do not stack, and a hatched egg cannot be traded, stored in a guild storage, mailed or vended, as `itemdb_ishatched_egg` and `CARD0_PET` checks do. An unhatched egg can.

**2.2 Taming.** The converted taming item runs `player:start_taming(lure, mode)`; the server sends `PetCatchStart`; the client enters a target mode and the player clicks a monster; the client sends `PetCatch { target }`; the server runs `pet_catch_process_end` (checks, rate, `pet_catch_rate`, `pet_legacy_formula`) and answers `PetCatchResult`. `mode` is `normal`, `universal_no_boss` or `universal_all`, as `e_pet_catch_flag`. The lure item is consumed by the item as for rAthena, whatever the outcome (rAthena's FIXME about delay-consume items is not ported). A success removes the monster as `status_kill` does (no drops, no exp) and puts a new egg in the inventory. The map flag `nopetcapture` is a new `maps.set_flag` name. A monster without a pet record, or whose record the lure does not match, fails.

**2.3 Hatching and returning.** `PetHatch { slot }` hatches the egg in that inventory slot; `PetCommand { RETURN }`, a map with `pet_disable_in_gvg`, logout and death of the egg's item return it. Hatching needs no pet to be out already. A hatched pet is `incubate = false` in the row, and its egg item is marked hatched, as rAthena's `attribute` 1. Returning drops the pet's loot (section 2.6) and sets `hatched = false`.

**2.4 Hunger and intimacy.** Timers per rAthena (above), on the zone's clock. `pet_hungry_delay_rate`, `pet_friendly_rate`, `pet_support_min_friendly` and the autofeed rate are rules. A pet at intimacy 0 is removed with its egg and the owner is told. Intimacy changes recalculate the owner (`status_calc_pc`) and update the egg's displayed grade.

**2.5 Bonuses.** A pet record's `bonuses` is a list of tiers:

```yaml
bonuses:
  - {intimacy: cordial, bonuses: [[luk, 2], [critical, 1]]}
  - {intimacy: loyal, bonuses: [[luk, 3], [critical, 1]]}
```

The server applies the highest tier whose threshold the pet's intimacy meets (a tier is not cumulative, as the scripts are `if / else if` chains). `intimacy` is a grade name (`awkward` 1, `shy` 100, `neutral` 250, `cordial` 750, `loyal` 910) or a number. A tier may carry `procs` (the `petautobonus` rows: `{rate, duration_ms, attack_type, bonuses, on_proc}`) with the same shape as an item's `autobonus`. Bonuses apply while the pet is hatched, with no accessory check, as `status_calc_pc` does. They apply through `status.Calc` like a companion record's `bonuses`, and the pet's change recalculates the player.

**2.6 Pet support.** `support` is a record block for `SupportScript`: `loot: {max_items, max_weight}`, `skill_attack`, `skill_support`, `skill_bonus`, `recovery`, `heal`. It runs only with the rule `pet_status_support` (default 0) and needs the accessory with `pet_equip_required`. No core record has a block, so pets built in v0.2.0 implement the block and its tests but ship it off. Pet loot picks up ground items that the owner may take (as `pet_ai_sub_hard_lootsearch`) and drops them at the owner's cell on return.

**2.7 Evolution.** `PetEvolve { target }`, with `feature.petevolution` as the rule `pet_evolution` (default 1). Checks and effects as above. The evolution list is data (`evolution: [{target, items: [{item, amount}]}]`).

**2.8 Items.** An accessory is a `petarmor` item. The existing equip command, on a `petarmor` item, puts it on the hatched pet if the record accepts it and none is worn. Food is consumed by `PetCommand { FEED }`. `getpetinfo` reads `player.pet`.

### 3. Homunculi

**3.1 State.** `HomunculusState`: `class` (record), `prev_class`, `name`, `renamed`, `level`, `exp`, `hp`, `sp`, `max_hp`, `max_sp`, `str`, `agi`, `vit`, `int`, `dex`, `luk` (in tenths), `skill_points`, `skills` (id to level), `hunger`, `intimacy` (0 to 100,000), `autofeed`, `state` (`active`, `vaporized`, `morph`). One per character. The zone keeps it while the owner is online; it is saved with the character.

**3.2 Records.** `data/homunculi.yml` (section 5). The growth numbers, skill trees and evolution links come from `homunculus_db.yml`; the exp table from `exp_homun.yml`.

**3.3 Creation, call and vaporize.** `AM_CALLHOMUN` with no homunculus makes a random one of the 8 base classes and calls it; with one vaporized it recalls it; with one active it fails. `AM_REST` vaporizes (HP at least 80%), `AM_RESURRECTHOMUN` revives at `20 * level` percent. Skill costs and cooldowns come from the mode's `skill_db`. These three skills are built in server/skill/homun*.go and need the unit; they are Alchemist skills with the Alchemist skill tree entries already in the extract. A call, a revive and a login under the flag `homunculus` set renewal's 30 minute `SC_HOMUN_TIME` (the status exists in `status.yml`); its end vaporizes the homunculus. A homunculus out of the owner's sight gets no special handling.

**3.4 Levels and growth.** The exp table is the extract. `hom_levelup` takes the growth ranges from the record (the previous record for an S below `homunculus_S_growth_level`), draws from the zone's random stream, truncates stats to tenths, and gives a skill point every third level. The homunculus is healed fully on a level up. Renewal adds `level / 10` to each of the six stats as stat weight (flag `homunculus`).

**3.5 Exp.** With flag `homunculus` renewal: a homunculus receives `homunculus_exp_gain` percent of the owner's base exp share of each kill the owner took part in, whether or not the homunculus fought. Pre-renewal: the full share of kills its own damage entry took part in. Neither gives job exp. The owner's share is unchanged.

**3.6 Hunger and intimacy.** As above, with the starving rate and delay as rules. Intimacy 0 (from hunger 0 with intimacy at most 100, or from overfeeding) deletes the homunculus. A homunculus takes no intimacy loss for dying. `addhomintimacy` and `gethominfo` use the state.

**3.7 Skills.** The skill window (section 10) lists the tree entries that meet their conditions; a skill point raises one up to the record's max. The skill code is in `server/skill/homun*.go` and `server/zone/skills_homun*.go`. Each skill's formulas come from `skill.cpp` and have goldens (section 16). Intimacy-bound skills (`HFLI_SBR44`, `HVAN_EXPLOSION`) read the unit's intimacy.

**3.8 Evolution.** `homevolution` and the item `Stone_Of_Intelligence_` convert to `homunculus:evolve()`. It needs `homunculus_evo_intimacy_need`, an active unit and an unevolved class. Intimacy is reset to `homunculus_evo_intimacy_reset`.

**3.9 Mutation and Homunculus S.** `morphembryo`, `hommutate` and the S records, skills and growth rules: they are a separate node (section 17): `homunculus-s`. They need the S classes' data, the 42 `MH_*` skills with goldens and `npc/re/quests/homun_s.txt`. Renewal only: pre-renewal's data has no S class. The data and the state field `prev_class` are in the base node so saves do not change later.

**3.10 Death and revive.** At 0 HP the unit stays saved, leaves the map, and its hunger timer stops (as `hom_dead`). `AM_RESURRECTHOMUN` brings it back at its owner's cell. The owner's death vaporizes an active unit at 80% HP or more (`homunculus_auto_vapor`); under 80 it stays on the map and follows when the owner revives.

**3.11 Orders and AI.** Section 1.5. A homunculus's skill rules per skill are in `data/homunculi.yml`.

### 4. Mercenaries

**4.1 State.** `MercenaryState`: `class`, `hp`, `sp`, `remaining_ms` (contract), `kills`. One per character, saved in `character_followers` (section 9). Faith and calls per guild are `character_mercenary_guilds` (section 9).

**4.2 Hiring.** `mercenary_create` converts to `player:hire_mercenary(class, ms)`: it returns false when the player has one or the class is unknown, as rAthena; no zeny or faith check (the script does that). It adds 1 call to the guild. The contract timer runs on the zone clock only while the owner is online; the rest is saved.

**4.3 Faith, calls and kills.** `mercenaries.get_faith(guild)`, `set_faith(guild, delta)`, `get_calls`, `set_calls` as the commands (guild is `archer`, `spear` or `sword`). Faith is 0 to 32,767. A contract end gives +1, a death -1, every 50 kills +1 and a random buff. `getmercinfo` types 0 to 8 read the unit: ID, class, name, faith, calls, kills, remaining ms, level, game ID.

**4.4 Death and dismissal.** The mercenary's death, the end of its contract, the owner's death and `MercenaryCommand { DISMISS }` remove it with the faith changes above, and send the client's message. A mercenary has no revive.

**4.5 Skills and stats.** From the record. The 41 skills are `server/skill/merc*.go` (many share a player skill's code through the `src` entity). `mercenary_sc_start` and `mercenary_heal` convert to `mercenary:add_status` and `mercenary:heal`.

**4.6 Loot.** `mercenary_autoloot` and `mer_idle_no_share` are rules (default 0). A mercenary's kill credits its owner for drops only with autoloot, as rAthena.

### 5. Records

Three data kinds (`mod.PetKind`, `mod.HomunculusKind`, `mod.MercenaryKind`; `CoreNames` true). Core's files are converted from the extracts and live in `server/companion/core/data/`.

`data/pets.yml`, keyed by `monster` (the pet is its monster's record), `Economy: ["capture"]`:

```yaml
- name: PORING            # core: the monster's name
  monster: PORING         # fixed
  lure: Unripe_Apple
  egg: Poring_Egg
  accessory: Backpack     # omitted: none
  food: Apple_Juice
  fullness: 3
  hungry_delay: 60        # seconds
  hunger_increase: 20
  intimacy: {start: 250, fed: 50, overfed: -100, hungry: -5, owner_die: -20}
  capture: 2000           # per 10,000
  special_performance: true
  attack_rate: 0
  retaliate_rate: 0
  change_target_rate: 0
  autofeed: false
  bonuses: [...]          # section 2.5
  evolution: [{target: MASTERING, items: [{item: Leaf_Of_Yggdrasil, amount: 10}]}]
  support: {...}          # section 2.6
```

`data/homunculi.yml`: `name`, `display_name`, `id` (fixed; rAthena's 6001 to 6016 and 6048 to 6052, mods from 10,000), `evolution` (the evolved record's name), `food`, `hungry_delay` (ms), `race`, `element`, `size`, `evolution_size`, `aspd`, `stats` (for each of hp, sp, str, agi, vit, int, dex, luk: `base`, `growth: [min, max]`, `evolution_growth: [min, max]`), `skills` (a list of `{skill, max_level, level, intimacy, evolved, requires: [{skill, level}], use}`), `sprite`. An evolved class shares its base's record, as rAthena's `evo_class`.

`data/mercenaries.yml`: `name`, `display_name`, `id` (fixed), `guild` (`archer`, `spear`, `sword` or none), `level`, `hp`, `sp`, `attack`, `attack2`, `defense`, `magic_defense`, the six stats, `attack_range`, `skill_range`, `chase_range`, `size`, `race`, `element`, `element_level`, `walk_speed`, `attack_delay`, `attack_motion`, `damage_motion`, `skills` (`{skill, max_level, use}`), `sprite`.

A mod record's fields are checked at load; a record that fails is left out with a warning. A homunculus or mercenary record's `skills` must name known skills. Pet and homunculus records name items and monsters by record name.

### 6. Lua API

Reads need `game.read`. Changes need a new permission, `companions.manage` (approval needed, not flagged): "create, hatch, call, evolve and remove players' pets, homunculi and mercenaries, and change their values". Core holds it. The builder `player:start_taming` and the field reads need no more.

New modules and objects (all handles; two handles of one thing are equal):

| Call | Returns | rAthena | Permission |
|---|---|---|---|
| `pets.get(name)` | `wa.PetInfo?` | `pet_db` | `game.read` |
| `pets.id(name)` | integer | | `game.read` |
| `homunculi.get(name)`, `homunculi.id(name)` | `wa.HomunculusInfo?`, integer | `homunculus_db` | `game.read` |
| `mercenaries.get(name)`, `mercenaries.id(name)` | `wa.MercenaryInfo?`, integer | `mercenary_db` | `game.read` |
| `player:start_taming(lure?, mode?)` | boolean | `catchpet`, `pet` | `companions.manage` |
| `player:open_egg_list()` | boolean | `bpet` | none |
| `player:create_pet_egg(pet)` | boolean | `pet_create_egg` | `companions.manage` |
| `player:create_homunculus(class?)` | `wa.Homunculus?` | `hom_create_request` | `companions.manage` |
| `player:hire_mercenary(class, ms)` | `wa.Mercenary?` | `mercenary_create` | `companions.manage` |
| `mercenaries.get_faith(player, guild)`, `set_faith(player, guild, delta)`, `get_calls`, `set_calls` | integer | `mercenary_*_faith`, `*_calls` | `game.read` and `companions.manage` |

New fields: `player.pet` (`wa.Pet?`), `player.homunculus` (`wa.Homunculus?`), `player.mercenary` (`wa.Mercenary?`). They are read-only, as every field.

`wa.Pet`: fields `id`, `info` (`wa.PetInfo`), `name`, `renamed`, `level`, `hunger`, `intimacy`, `grade`, `accessory` (`wa.Item?`), `autofeed`, `egg` (item ID), `game_id`. Methods (need `companions.manage`): `set_intimacy(n)`, `add_intimacy(n)`, `set_hunger(n)`, `feed()`, `rename(name)`, `return_to_egg()`, `evolve(target)`, `set_accessory(item?)`. `feed` and `rename` return false on the rAthena failures.

`wa.Homunculus`: fields `id`, `info`, `class`, `name`, `renamed`, `level`, `exp`, `hp`, `sp`, `max_hp`, `max_sp`, the six stats, `skill_points`, `hunger`, `intimacy`, `grade`, `state` (`"active"`, `"vaporized"`, `"morph"`, `"dead"`), `autofeed`, `game_id`. Methods: `call()`, `vaporize()`, `revive(percent)`, `evolve()`, `mutate(class?)`, `morph()`, `shuffle()`, `add_intimacy(n)`, `set_hunger(n)`, `feed()`, `add_exp(n)`, `learn_skill(skill)`, `skill_level(skill)`, `delete()`. `evolve` needs the intimacy rule and returns false otherwise. `mutate` and `morph` are the `homunculus-s` node's.

`wa.Mercenary`: fields `id`, `info`, `class`, `name`, `level`, `guild`, `faith`, `calls`, `kills`, `remaining_ms`, `hp`, `sp`, `game_id`. Methods: `heal(hp, sp)`, `add_status(status, ms, value)`, `dismiss(reason)` (`"contract"`, `"killed"`, `"owner_dead"`, `"dismissed"`).

Events (kinds `mod`, `npc`, `event`): `pet_tame` `{ player, monster, pet }`, `pet_hatch`, `pet_return`, `pet_feed`, `pet_intimacy` (`{ pet, old, new }`), `pet_evolve`, `pet_lost`; `homunculus_create`, `homunculus_call`, `homunculus_vaporize`, `homunculus_level`, `homunculus_evolve`, `homunculus_lost`; `mercenary_hire`, `mercenary_end` (`{ mercenary, reason }`), `mercenary_kill`. A hook `pet_catch_rate` (needs `rules.override`, returns the rate per 10,000). `wa.testing`: `new_player` options `pet`, `homunculus`, `mercenary`, and `world:tick_followers(ms)`.

### 7. Converter

| Source | Becomes | Rows |
|---|---|---|
| `pet` item command | `player:start_taming(item, mode)` | 68 renewal, 55 pre-renewal |
| `bpet` | `player:open_egg_list()` | 1, 1 |
| `getpetinfo(PETINFO_*)` | `R.petinfo(player, type)`, reading `player.pet` (nil gives 0 and "null") | 12 equipment rows |
| `mercenary_create` | `player:hire_mercenary` | 59, 39 |
| `mercenary_sc_start`, `mercenary_heal` | `mercenary:add_status`, `mercenary:heal` | 3, 2 |
| `homevolution` | `homunculus:evolve()` | 1, 1 |
| `addhomintimacy` | `homunculus:add_intimacy` | 1 renewal |
| `gethominfo`, `checkhomcall` | `R.hominfo`, `R.homcall` | NPC rows |
| `getmercinfo`, `mercenary_get_faith`, `mercenary_set_faith`, `mercenary_get_calls`, `mercenary_set_calls`, `mercenary_delete` | `R.mercinfo` and the module calls | NPC rows |
| `morphembryo`, `hommutate`, `homshuffle` | the `homunculus-s` methods | NPC rows |
| `pet_db.yml` | `data/pets.yml` | 107, 57 |
| `homunculus_db.yml`, `exp_homun.yml` | `data/homunculi.yml` and the exp table | 8 and 13 |
| `mercenary_db.yml` | `data/mercenaries.yml` | 64 and 44 |

`PETINFO_*`, `PET_INTIMATE_*` and the mercenary guild constants go in `R`'s constants. A pet record's `Script` converts by the tier pattern: `.@i = getpetinfo(PETINFO_INTIMATE);` then `if / else if` on `PET_INTIMATE_*` of `bonus` rows, and `petautobonus` as a proc. A script that does not match is refused with its reason in the pet report (a new section of the item reports, or `docs/generated/pets-<mode>.md`). Equipment scripts that read `getpetinfo` convert with a dependency: an equip context that reads `player.pet` is recalculated on every pet intimacy or class change. The converter's equipment path must allow `player.pet` reads in an equip context (it forbids `rand`, for the same reason).

NPC scripts: `npc/other/mercenary_rent.txt` and the renewal copy, `npc/quests/monstertamers.txt`, `npc/merchants/wander_pet_food.txt` and `npc/re/quests/homun_s.txt` convert with the commands above. Their conversion is checked by the existing NPC reports. The NPC behavior harness (npc-behavior-harness) covers `mercenary_rent.txt` and `homun_s.txt` once it exists.

### 8. Protocol

Proposal 0001's rules: new fields and messages only, so the protocol version stays 1. Numbers are the next free at build time (event.proto has 114; proposal 0003 takes 115 and 116 and proposal 0004 takes 117 to 120; the numbers below start at 121 and move with merge order).

```proto
// event.proto, in Event.body
PetInfo pet_info = 121;
PetEggList pet_egg_list = 122;
PetCatch pet_catch = 123;
HomunculusInfo homunculus_info = 124;
MercenaryInfo mercenary_info = 125;
FollowerSkills follower_skills = 126;
FollowerResult follower_result = 127;

// PetInfo is the owner's pet window, as ZC_PROPERTY_PET. It is sent at
// hatching and whenever hunger, intimacy, name or accessory change.
message PetInfo {
  uint32 id = 1;        // the entity's ID on this connection
  string name = 2;
  bool renamed = 3;
  uint32 level = 4;
  uint32 hunger = 5;    // 0 to 100
  uint32 intimacy = 6;  // 0 to 1,000
  uint32 grade = 7;     // 0 awkward ... 4 loyal
  uint32 accessory = 8; // item ID, 0 for none
  uint32 monster = 9;   // the pet's monster ID
  bool autofeed = 10;
  bool can_autofeed = 11;
  repeated PetEvolution evolutions = 12;
}
message PetEvolution { uint32 target = 1; repeated PetEvolutionItem items = 2; bool ready = 3; }
message PetEvolutionItem { uint32 item = 1; uint32 amount = 2; uint32 have = 3; }

// PetEggList lists the eggs the player may hatch (ZC_PETEGG_LIST).
message PetEggList { repeated PetEgg eggs = 1; }
message PetEgg { uint32 slot = 1; uint32 item = 2; string name = 3; uint32 grade = 4; }

// PetCatch is the taming mode (ZC_START_CAPTURE, ZC_TRYCAPTURE_MONSTER).
message PetCatch {
  PetCatchStage stage = 1;
  uint32 lure = 2;
}
enum PetCatchStage {
  PET_CATCH_STAGE_UNSPECIFIED = 0;
  PET_CATCH_STAGE_START = 1;
  PET_CATCH_STAGE_SUCCESS = 2;
  PET_CATCH_STAGE_FAIL = 3;
}

// HomunculusInfo is the homunculus window (ZC_PROPERTY_HOMUN), sent
// when the unit appears and on each change.
message HomunculusInfo {
  uint32 id = 1;        // entity ID, 0 while vaporized or dead
  string name = 2;
  bool renamed = 3;
  HomunculusState state = 4;
  uint32 class_id = 5;
  uint32 level = 6;
  uint32 hunger = 7;
  uint32 intimacy = 8;  // 0 to 1,000 (the unit's value / 100)
  uint32 grade = 9;
  uint64 hp = 10; uint64 max_hp = 11; uint32 sp = 12; uint32 max_sp = 13;
  uint64 exp = 14; uint64 next_exp = 15;
  uint32 skill_points = 16;
  uint32 atk = 17; uint32 matk = 18; uint32 hit = 19; uint32 crit = 20;
  uint32 def = 21; uint32 mdef = 22; uint32 flee = 23; uint32 aspd = 24;
  uint32 range = 25;
  bool autofeed = 26;
  bool can_autofeed = 27;
}
enum HomunculusState {
  HOMUNCULUS_STATE_UNSPECIFIED = 0;
  HOMUNCULUS_STATE_ACTIVE = 1;
  HOMUNCULUS_STATE_VAPORIZED = 2;
  HOMUNCULUS_STATE_MORPH = 3;
  HOMUNCULUS_STATE_DEAD = 4;
}

// MercenaryInfo is the mercenary window (ZC_MER_INIT, ZC_MER_PAR_CHANGE).
message MercenaryInfo {
  uint32 id = 1;
  string name = 2;
  uint32 class_id = 3;
  uint32 level = 4;
  uint64 hp = 5; uint64 max_hp = 6; uint32 sp = 7; uint32 max_sp = 8;
  uint32 faith = 9;
  uint32 calls = 10;
  uint32 kills = 11;
  uint64 remaining_ms = 12;
  uint32 atk = 13; uint32 matk = 14; uint32 hit = 15; uint32 crit = 16;
  uint32 def = 17; uint32 mdef = 18; uint32 flee = 19; uint32 aspd = 20;
}

// FollowerSkills is a homunculus's or mercenary's skill list. It reuses
// SkillInfo. skill_points is the homunculus's, 0 for a mercenary.
message FollowerSkills {
  EntityKind kind = 1;
  repeated SkillInfo skills = 2;
  uint32 skill_points = 3;
}

// FollowerResult answers a follower command.
message FollowerResult {
  EntityKind kind = 1;
  FollowerAction action = 2;
  FollowerOutcome outcome = 3;
}
enum FollowerAction {
  FOLLOWER_ACTION_UNSPECIFIED = 0;
  FOLLOWER_ACTION_FEED = 1;
  FOLLOWER_ACTION_RENAME = 2;
  FOLLOWER_ACTION_EQUIP = 3;
  FOLLOWER_ACTION_HATCH = 4;
  FOLLOWER_ACTION_EVOLVE = 5;
  FOLLOWER_ACTION_LEARN_SKILL = 6;
  FOLLOWER_ACTION_DISMISS = 7;
}
enum FollowerOutcome {
  FOLLOWER_OUTCOME_UNSPECIFIED = 0;
  FOLLOWER_OUTCOME_OK = 1;
  FOLLOWER_OUTCOME_NO_FOOD = 2;
  FOLLOWER_OUTCOME_NAME_REFUSED = 3;
  FOLLOWER_OUTCOME_NOT_ALLOWED = 4;  // a rule or state refuses it
  FOLLOWER_OUTCOME_NO_MATERIAL = 5;
  FOLLOWER_OUTCOME_NO_POINTS = 6;
}

// command.proto, in Command.body
PetCommand pet_command = <next>;
PetHatch pet_hatch = <next>;
PetCatchTarget pet_catch_target = <next>;
PetEvolve pet_evolve = <next>;
HomunculusCommand homunculus_command = <next>;
MercenaryCommand mercenary_command = <next>;
FollowerOrder follower_order = <next>;
FollowerLearnSkill follower_learn_skill = <next>;

message PetCommand { PetAction action = 1; string name = 2; bool flag = 3; }
enum PetAction { PET_ACTION_UNSPECIFIED = 0; PET_ACTION_FEED = 1; PET_ACTION_PERFORM = 2;
  PET_ACTION_RETURN = 3; PET_ACTION_UNEQUIP = 4; PET_ACTION_RENAME = 5; PET_ACTION_AUTOFEED = 6; }
message PetHatch { uint32 slot = 1; }
message PetCatchTarget { uint32 entity = 1; }   // 0 cancels the taming mode
message PetEvolve { uint32 target = 1; }
message HomunculusCommand { HomunculusAction action = 1; string name = 2; bool flag = 3; }
enum HomunculusAction { HOMUNCULUS_ACTION_UNSPECIFIED = 0; HOMUNCULUS_ACTION_FEED = 1;
  HOMUNCULUS_ACTION_DELETE = 2; HOMUNCULUS_ACTION_RENAME = 3; HOMUNCULUS_ACTION_AUTOFEED = 4; }
message MercenaryCommand { bool dismiss = 1; }
message FollowerOrder {
  EntityKind kind = 1;
  oneof order {
    Cell move = 2;
    uint32 attack = 3;
    UseSkill skill = 4;
    bool come = 5;
    bool hold = 6;
  }
}
message FollowerLearnSkill { uint32 skill_id = 1; }
```

- A homunculus's skill use by the owner, and `AM_CALLHOMUN`, `AM_REST` and `AM_RESURRECTHOMUN`, use the existing `UseSkill`. The equip command handles the pet accessory. Entity appearance, movement, damage and chat emotions use the existing events.
- `EntityAppear.hp_permille` for a homunculus stays inexact (the owner reads exact HP from `HomunculusInfo`), as the field's comment says.
- Clients older than this change show no followers (they skip unknown events) and send no follower commands.

### 9. Database

Numbers are taken in merge order. Proposal 0004 reserved 00043 for `castles`; this proposal names 00044 onward.

- **00044_followers.sql** (companion-units): `character_followers (character_id bigint FK characters ON DELETE CASCADE, kind smallint CHECK (1..3), record text, name text, hp bigint, sp bigint, state jsonb CHECK (octet_length(state::text) <= 4096), PRIMARY KEY (character_id, kind))`. Game role: SELECT, INSERT, UPDATE, DELETE. Mercenaries use it with `state` holding `remaining_ms` and `kills`. A kind that has its own table does not use it.
- **00045_pets.sql** (pets): `pets (id bigint PK generated always as identity, owner_id bigint FK characters ON DELETE SET NULL, record text, name text CHECK (char_length <= 24), renamed boolean, level smallint, hunger smallint CHECK (0..100), intimacy smallint CHECK (0..1000), accessory integer, autofeed boolean, hatched boolean, created_at, updated_at)`; `ALTER TABLE inventory, storage_items, guild_storage, mail_items ADD COLUMN pet_id bigint REFERENCES pets (id) ON DELETE SET NULL`, with an index on `pet_id`. Game role: SELECT, INSERT, UPDATE, DELETE on `pets`. A hatched pet's row is written with its owner's save. The retention job deletes pets with no item and no owner after 7 days.
- **00046_homunculi.sql** (homunculus): `character_homunculi (character_id PK FK CASCADE, record text, prev_record text, name, renamed, level, exp bigint, hp, sp, max_hp, max_sp, str, agi, vit, int, dex, luk integer (tenths), skill_points, hunger, intimacy integer CHECK (0..100000), autofeed, state smallint CHECK (0..3))` and `character_homunculus_skills (character_id, skill text, level smallint, PRIMARY KEY (character_id, skill))`. Skills are stored by name so IDs of mod skills may change.
- **00047_mercenary_guilds.sql** (mercenaries): `character_mercenary_guilds (character_id FK CASCADE, guild smallint CHECK (0..2), faith integer CHECK (0..32767), calls integer CHECK (>= 0), PRIMARY KEY (character_id, guild))`.
- Skill cooldowns, statuses and positions of followers are not saved (players' are not either).
- Saves: a follower's row writes in the character's save transaction (`zone.snapshot` copies it: `Character.Pet`, `.Homunculus`, `.Mercenary`, `.MercenaryGuilds`). The store reads them in the character load.
- Item copies: an egg's `Extra.PetID` follows the item through the transfer layer, the logs (`item_log` gains the pet ID in its detail), mail and storage. Deleting a character deletes its homunculus and followers; its pets lose their owner (`SET NULL`) and are reaped.

### 10. Client windows

In `client/src/viewer` (not `client/src/world`, the world generator). New files, each with a unit test beside it as the folder does:

- `follower.ts` (companion-units): the store of the three followers' info, entity drawing by kind (pet by its monster sprite, homunculus and mercenary by class, with the placeholder sprite until the GRF importer names them, as decision 0016 left for companions), the order menu on a follower (move here, attack, come, hold; hotkey for come), and the chat emotions the server sends.
- `pet.ts`, `petEggs.ts`, `petCatch.ts` (pets): the pet window (name, level, hunger and intimacy bars with the grade name, accessory, buttons feed, perform, return to egg, unequip, rename, autofeed checkbox, evolution list with items held and a button), the egg list dialog, and the taming mode (cursor and a hint; a click on a monster sends `PetCatchTarget`, escape cancels).
- `homun.ts`, `homunSkills.ts` (homunculus): the status window as the client shows it (stats, HP, SP, exp, hunger, intimacy), its skill window (the `skills.ts` list with skill points and learn buttons), the menu (feed, delete after a confirmation, rename, autofeed), and the vaporized and dead state text.
- `merc.ts` (mercenaries): the info window (name, level, HP, SP, faith, calls, kills, time left), its skill list without learning, and the dismiss button with a confirmation.
- Touch controls reuse the order menu on a long press.

All text goes through the client's locale tables. Windows follow `windows.ts` conventions (one open at a time per kind, keyboard and touch).

### 11. Restart behavior

- Pets: the egg item and the `pets` row persist. A hatched pet comes back with its owner at login (`hatched` is true), with its saved hunger and intimacy. The hunger timer restarts at login: no hunger passes while the owner is offline, as rAthena. A server stop or crash loses at most the autosave interval.
- Homunculus: `state` persists. An active one spawns at login with the saved HP and SP, and renewal's 30 minute status starts again from login. A vaporized, morph or dead unit stays so.
- Mercenary: the saved remaining contract time resumes at login. The clock runs only while the owner is online, so a contract never ends while the owner is offline.
- A character whose follower record is gone (a removed mod) loads without it, with a warning that names the record. Its egg stays an item. A homunculus of a removed class stays saved and vaporized until the class returns.
- World restart and zone stop save followers with their owners. Instances and maps removed at runtime handle followers as their owners' players (`forward`).

### 12. Limits and ruleset rules

One of each follower per player, as rAthena. No follower count limit beyond that. Rules (`server/rules/limits.go`, with rAthena's shipped conf values; booleans are 0 or 1):

- Pets: `pet_catch_rate` 100, `pet_legacy_formula` 0, `pet_distance_check` 5, `pet_hide_check` 1, `pet_rename` 0, `pet_friendly_rate` 100, `pet_hungry_delay_rate` 100, `pet_equip_required` 1, `pet_unequip_destroy` 1, `pet_attack_support` 0, `pet_damage_support` 0, `pet_support_min_friendly` 900, `pet_status_support` 0, `pet_support_rate` 100, `pet_attack_exp_to_master` 0, `pet_attack_exp_rate` 100, `pet_lv_rate` 0, `pet_max_stats` 99, `pet_max_atk1` 500, `pet_max_atk2` 1000, `pet_disable_in_gvg` 0, `pet_ignore_infinite_def` 1, `pet_master_dead` 0, `pet_walk_speed` 1, `pet_evolution` 1, `pet_autofeed` 1, `pet_autofeed_rate` 89.
- Homunculi: `hom_setting` 0x3D, `homunculus_friendly_rate` 100, `hom_rename` 0, `homunculus_evo_intimacy_need` 91100, `homunculus_evo_intimacy_reset` 1000, `hvan_explosion_intimate` 45000, `homunculus_auto_vapor` 80, `homunculus_max_level` 99, `homunculus_s_max_level` 275, `homunculus_s_growth_level` 99, `homunculus_exp_gain` 10, `homunculus_starving_rate` 10, `homunculus_starving_delay` 20000, `homunculus_autoloot` 0, `homunculus_autofeed` 1, `homunculus_autofeed_rate` 30, `hom_delay_reset_vaporize` 1.
- Mercenaries: `mercenary_autoloot` 0, `mer_idle_no_share` 0, `mercenary_kill_faith` 50 (kills for +1 faith and the buff, rAthena's constant).
- Follower AI: `follower_follow_range` 3, `follower_chase_range` 12, `follower_teleport_delay` 3000 (wAthena's, from rAthena's constants, so servers can tune them).
- Flag `homunculus` gets its effects (3.3, 3.4, 3.5, section 1.3). It sits beside the other flags in `mod.flagValues` (already there).
- Map flag `nopetcapture` for `maps.set_flag` (needs `maps.control`) and the map flag extract (instances-core and sieges-castles already read `map_flags.json`; pets adds this line).
- Rules apply to both modes with the same defaults.

### 13. Admin and observability

- `GET /players/{id}` (`players.read`) shows the player's followers: pet record, intimacy, hunger; homunculus class, level, state, intimacy; mercenary class, faith, time left.
- Staff permission `players.companions` ("restore, create and remove players' pets, homunculi and mercenaries"), held by `admin` and `game_master`: `POST /players/{id}/followers` with `{ kind, record, reason }`, `DELETE /players/{id}/followers/{kind}`. Audit actions `followers.create` and `followers.remove`, with the record and the reason. These need the owner offline or online: an online owner's zone applies it, an offline one is changed in the rows.
- GM commands (permission `commands.companions`, new, held by `admin` and `game_master`, writing the same audit rows): `@makepet`, `@petfriendly`, `@pethungry`, `@makehomun`, `@homlevel`, `@homfriendly`, `@homhungry`, `@homevolution`, `@mercenary`.
- Metrics: gauges of followers by kind and zone, counters of tamings (by outcome), hatches, homunculus creations, mercenary hires and ends (by reason), and the follower AI's tick work in the zone's tick histogram. Info logs for losses: a pet that ran away, a homunculus deleted by hunger or overfeeding, a mercenary killed. Owner and record in each.

### 14. Procedural worlds

- The system, the records and the AI do not depend on the world mode. Taming items and eggs drop from monsters through the drop tables as usual.
- Core's guild NPCs and quest NPCs are converted classic NPCs and load in classic worlds only.
- A procedural world has no mercenary guild, no taming quests and no homunculus quest NPC until templates add them (strategy section 11.2 names the mercenary guild NPC in towns). That is new content and needs its own node after v0.2.0; mods can place such NPCs with the API.

### 15. Performance

Followers add up to three entities per player. The cost that matters is the Crowd case of decision 0050 (view events) and the AI's target search. The base measures it:

- A `wabots` scenario `companions`: 500 bots each with a pet, a homunculus and a mercenary, walking and fighting on a field and standing in a town. It records the zone tick p99, the send phase, the bytes per player per second, and the live heap.
- The AI runs for a follower only when its owner is on the map. Target searches use the monster path's `enemyCandidates` and run when the follower has no target, at most once a second for an idle follower. Pets never search.
- If 500 bots with three followers each exceed section 3.8's budget, decision 0055's shared view events come before the documented capacity, and the release notes state the measured count with followers.

### 16. Tests

Unit and golden tests:

- companion-units: `walkState` extraction (all `mob_ai.json` goldens unchanged); owner link and map change (follower appears beside the owner after a handoff and a teleport, and is placed back after 3 s beyond range); `isEnemy` rules for the three kinds; AI orders and follow; `battle.CalcMisc` for each unit kind; the damage log flag and exp shares; `character_followers` save, load and a removed record (PostgreSQL); the test kind in `wactl mod test`.
- pets: record loading (core's 107 and 57, a mod record); the catch rate and checks (distance, hide, boss, free slot, `nopetcapture`); egg creation by taming, drop and shop; hatch, return, hunger timeline, feed branches, intimacy and the removal at 0, owner death, rename, accessory, autofeed, evolution; bonus tiers and procs against `item_bonuses`-style equality; `getpetinfo` equipment recalculation; `pets` rows and reaping (PostgreSQL); converter golden files for the item rows.
- homunculus: record loading (8 and 13); creation, call, vaporize, revive; growth ranges, the exp table, skill points, the stat weight under each flag setting; hunger and intimacy branches; evolution; the skill tree conditions; every `HLIF_*`, `HAMI_*`, `HFLI_*`, `HVAN_*` skill against recordings; `hom_setting` bits; the exp share in both modes; saves (PostgreSQL); converter golden files.
- homunculus-s: mutation checks, S growth and previous-record growth, the 42 `MH_*` skills against recordings, `homun_s.txt` conversion.
- mercenaries: record loading (64 and 44); hiring, the contract timer online only, faith and calls, kills every 50, death and owner death, the Emperium rule; the 41 skills against recordings; saves and the faith rows (PostgreSQL); converter golden files.
- client: unit tests for each new viewer file; browser checks of the pet window, egg list, taming mode, homunculus windows and the mercenary window (the existing viewer browser run).

Golden against rAthena. The fork's recorder (`src/map/wathena_recorder.cpp`) gains, per mode:

- `follower_status`: pet stats at pet level scaling off and on, homunculus stats for every class at levels 1, 50, 99 with each skill passive, mercenary stats for every record, with the battle status (HIT, FLEE, DEF, MDEF, ASPD, ATK, MATK). Needs hooks that build a `pet_data`, `homun_data` and `s_mercenary_data` without the char server (`hom_alloc`, `pet_data_init` and `mercenary_recv_data` take rows from `intif`).
- `pet_life`: feed and hunger tick sequences for a sample of records (intimacy and hunger after each step).
- `homunculus_life`: `hom_levelup` growth with `rnd_force_min` and `rnd_force_max`, evolution, `hom_food` and `hom_hungry` sequences, `hom_shuffle`.
- `follower_skill`: each homunculus and mercenary skill at every level, as `skill_damage.json` does (forced minimum damage, statuses and heals, the caster a follower of the first class that has the skill).
- `mercenary_life`: faith, calls and kills transitions.
- Extracts: `pets.json`, `homunculi.json` (with the exp table), `mercenaries.json` (shared and mode files merged), and `nopetcapture` in `map_flags.json`, from `extract.py`.

The recorder cannot record the follower AI, since rAthena has none. The AI is tested with unit tests of wAthena's rules.

e2e (`e2e/`, a classic world with kRO assets, PostgreSQL, headless bots), acceptance of each node:

1. companion-units: a test follower follows its owner across a warp and a same-map teleport, and is saved and restored by a restart.
2. pets: a bot with a lure tames a Poring in a few attempts (seeded), hatches the egg, feeds it, equips the backpack, gets the loyal bonus, returns it to the egg, restarts, evolves a loyal pet with the items.
3. homunculus: a bot casts Call Homunculus, levels it by kills, feeds it, learns Healing Hands, vaporizes and recalls it, evolves it, restarts.
4. mercenaries: a bot hires a mercenary with the item, fights with it, its kills reach 50, it dies, faith falls by one.

### 17. Who builds what

- **companion-units** (sections 1, 5's loader and kinds, 6's handles and permission, 8's shared messages, 9's 00044, 10's `follower.ts`, 15, and the test kind): `walkState`, `follower`, `battle.CalcMisc`, orders and the AI base, the damage log flag, `character_followers`, `companions.manage`, `FollowerOrder`, `FollowerResult`, the `companions` wabots scenario. Needs more owned paths than its brief names: `server/character/**`, `server/progression/**`, `server/battle/**`, `server/mob/**` (the `calcMisc` move), `server/store/*.go` (character load and save), `server/mod/**` (kinds, permission), `server/gamedata/**` (loading), `server/rules/**` (rules), `server/admin/**`, `server/staff/**` and `schema/proto/wathena/admin/**` (the staff routes), `client/src/viewer/follower*`, `tools/wabots/**`, `tools/botclient/**`. Size M to L. It has no unit of its own: its e2e uses a test kind registered in tests.
- **pets**: sections 2, the pet parts of 5, 6, 7, 8, 9 (00045), 10, 12, 13, 16. Needs `server/item/**` (`Extra.PetID`), `server/progression/**` (egg drop), `server/zone/loot.go`, `server/zone/itemactions.go`, `server/mod/**`, `server/gamedata/**`, `server/rules/**`, `server/character/**`, `server/store/*.go`, `reference/rathena/extracts/**`, and the fork's `extract.py`. Owned: `server/companion/pet*.go`, `server/companion/core/data/pets.yml`, `server/zone/pet*.go`, `client/src/viewer/pet*`.
- **mercenaries**: sections 4 and the mercenary parts of 5 to 13, 16 (the recorder's `mercenary_life` and `follower_skill` for mercenary skills). Its brief lacks the skills and the goldens: add owned `server/skill/merc*.go`, `server/zone/merc*.go`, `server/companion/core/data/mercenaries.yml`, and shared `server/battle/testdata/**`, `reference/rathena/**`, `server/skill/**`, `server/character/**`, `server/store/*.go`, `server/mod/**`, `server/gamedata/**`. Depends on extract-sync (soft: the recorder's new files come from the fork) in addition to companion-units. Size M (L if the 41 skills need most of the work; see the split in question 8).
- **homunculus**: sections 3 (without 3.9), the homunculus parts of 5 to 13 and 16: the 16 regular skills and the three Alchemist skills, creation, levels, evolution, hunger, the exp share and the flag. Needs `server/progression/**`, `server/status/**`, `server/statuschange/**`, `server/rules/**`, `server/character/**`, `server/store/*.go`, `server/mod/**`, `server/gamedata/**`, `reference/rathena/extracts/**`. Owned: `server/companion/homun*.go`, `server/skill/homun*.go`, `server/zone/homun*.go`, `server/companion/core/data/homunculi.yml`, `client/src/viewer/homun*`. Size L, as its brief says.
- **homunculus-s** (new node): section 3.9: `morphembryo`, `hommutate`, `homshuffle`, the five S records and growth rules, the 42 `MH_*` skills with goldens, the S state in the window, and `npc/re/quests/homun_s.txt` conversion. Owned: `server/companion/homun_s*.go`, `server/skill/homun_s*.go`. Depends on homunculus. Priority: should (v0.2.0 ships without Homunculus S if needed, as strategy section 11.2 calls it tier 3); renewal only.
- The three system nodes do not need each other. They can run in any order after companion-units; the sequential rule picks pets, mercenaries, homunculus, homunculus-s, because pets are smallest and homunculus-s is optional.

## Breaks

- Players can have followers. No existing world has any, so nothing else changes until a player tames, calls or hires one.
- `monster` loses the walking fields to an embedded struct. Code and tests that build `monster` literals with those fields change; `mob_ai.json` and the AI tests must pass unchanged.
- `isEnemy` and `queryKind` take followers into account. Pets are never targets and never enemies.
- The ruleset flag `homunculus` stops being inert.
- Item `Extra` gains `PetID`; four item tables gain `pet_id` (migration 00045). Eggs the server made before this change (plain `petegg` items from drops) have no pet: they become eggs with a pet row at the first load, using the record's start values. That is what rAthena's `pet_create_egg` would have made.
- `maps.set_flag` gains `nopetcapture`.
- Mods that ship companion records are not affected.

## Alternatives

- **Followers as monsters** (a monster with a master, like Alchemist summons). It reuses all of mobai.go and needs no `walkState` change. But every monster count, label and kill script sees them, `die` runs the drop and exp path, and `isEnemy`'s summon rules are a large pile of special cases. rAthena's `BL_PET`, `BL_HOM` and `BL_MER` are separate types for this reason. Not chosen.
- **A full second AI implementation** for followers, with no shared walking code. Smaller diff to mobai.go, but it copies path following, chase and attack state machines (about 600 lines) and would drift. Not chosen; the embedding is mechanical.
- **Client-side AI as the kRO client has it.** The server would only validate commands. The web client would need a Lua or TS AI per unit with a trust problem (a hostile client can make a homunculus attack anything in range at any rate). The server AI keeps one source of truth and works for headless clients and bots. Not chosen.
- **Pet state in the egg item's row.** No `pets` table and no orphans, and trades move it for free. But four item tables get a dozen columns, and every hunger change would rewrite an item row. Not chosen; the `pets` table with `pet_id` on item rows keeps items light.
- **One generic table for all followers.** One migration, but the homunculus's 25 fields and its skills belong in typed columns that staff can query. Chosen only for the small mercenary state and the test kind.
- **Pet bonuses as Lua.** The converter could emit a function per pet. 93 of 93 renewal scripts and 48 of 48 pre-renewal ones are tier chains of `bonus` rows, so tiers as data stay readable and mods can edit them, and a record that does not fit is refused with a reason. Not chosen.
- **Save skill cooldowns and statuses of followers.** rAthena saves cooldowns. wAthena saves no player cooldowns either, so a follower's would be the only saved ones. Not chosen.
- **Build Homunculus S inside homunculus.** One node, but it adds 42 skills with goldens, an NPC quest and a separate priority to an L node. Not chosen.
- **Elementals.** Strategy section 11.2 lists them and they need third classes. Out of scope; the follower base has room for a fourth kind.

## Questions for review

1. Followers are entities of their own (`entity.follower`), not monsters. Recommended: yes.
2. The monster's walking state moves into an embedded `walkState` shared with followers, as a mechanical change guarded by the existing `mob_ai.json` goldens. Recommended: yes.
3. The server runs the follower AI with wAthena's own rules (orders, follow, defend the owner, default skill rules), because rAthena's AI is the kRO client's and cannot be recorded. Recommended: yes, and each system's decision record lists its skill use rules.
4. Pets are never targets and never enemies, as rAthena. Recommended: yes.
5. Pets, homunculi and mercenaries never save skill cooldowns or statuses, as players' do not. Recommended: yes.
6. Pet state is a `pets` table referenced by `pet_id` on item rows, with a retention reaper for orphans, and an egg made before the change is repaired at load. Recommended: yes.
7. The small mercenary state and the base's test kind use a generic `character_followers` table (00044); pets (00045), homunculi (00046) and mercenary guild counters (00047) use typed tables. Recommended: yes.
8. Homunculus S (mutation, five S classes, 42 `MH_*` skills, `homun_s.txt`) is a separate node `homunculus-s`, priority should, renewal only. Recommended: yes. If the maintainer wants it in v0.2.0, the node moves to must.
9. Pet bonuses are data tiers by intimacy with procs, converted from the `Script` pattern, and a script that does not match is refused with a reason. Recommended: yes.
10. `SupportScript` (`petloot`, `petskill*`, `petrecovery`, `petheal`) is built as a record block behind `pet_status_support` (default 0), though no core record uses it. Recommended: yes, since mods can use it, but as the last item of the pets node and cut first if the node runs long.
11. One new mod permission, `companions.manage` (approval, not flagged), covers creating, hatching, calling, hiring, evolving and removing followers and changing their values. Recommended: yes.
12. Staff get `players.companions` (create and remove a player's followers, audited, `admin` and `game_master`) and GM commands under `commands.companions`. Recommended: yes; the endpoints and commands go to companion-units so each system node only registers its kind.
13. `homunculus_s_max_level` is 275 (the shipped conf), not the code default 150. Recommended: 275, with a note in the homunculus-s decision record.
14. Mercenary contract time counts only while the owner is online, as rAthena. Recommended: yes.
15. The mercenary guild NPC, taming quests and the homunculus quest are classic content only; procedural towns get them from a later template node. Recommended: yes.
16. companion-units measures the `companions` scenario, and if 500 bots with three followers each are over budget, decision 0055 comes before the documented capacity. Recommended: yes.
17. The follower AI tunables (`follower_follow_range`, `follower_chase_range`, `follower_teleport_delay`) are ruleset rules with rAthena's constants. Recommended: yes.
18. The work splits into companion-units, pets, mercenaries, homunculus and homunculus-s, with the owned path changes of section 17. Recommended: yes; mercenaries' skills (41, with goldens) stay in mercenaries unless the node's estimate grows, then they split into `mercenary-skills`.
