# 0002a: Item Bonus Names

- Status: draft for review.
- Part of [0002: Lua Scripting API](0002-lua-api.md).

## Purpose

Item scripts add bonuses by name, such as `bonuses.add("luck", 2)`. This document gives a readable name to every rAthena bonus type. The converter and the generated API reference use the same table.

Sources:

- `doc/item_bonus.txt` for the documented forms.
- `src/map/pc.cpp` (`pc_bonus` to `pc_bonus5`) for the exact behavior, units, and signs.
- `src/map/script_constants.hpp` for the full list of `b...` constants.
- `db/re/item_db_*.yml` for the use counts. A count is the number of times the form appears in script text, including bonuses inside `autobonus` and combo scripts. rAthena matches bonus names without case, so `bHealpower2` counts as `bHealPower2`.

rAthena exports 242 `b...` constants. One of them, `bFame`, is not a bonus. That leaves 241 bonus types, not the 248 that proposal 0002 says. 54 of the 242 have no use in renewal item data. The renewal data has 60,805 bonus calls in total.

## Naming rules

1. **Full words.** Stats are `strength`, `agility`, `vitality`, `intelligence`, `dexterity`, and `luck`. Short forms stay only where they are the game's own word: `hp`, `sp`, `ap`, and `zeny`.
2. **Client terms first.** A name uses the word the game client shows, such as `flee`, `perfect_dodge`, `critical`, `defense`, and `magic_defense`.
3. **One name per rAthena type.** The `bonus2` to `bonus5` forms of one type share one name. Extra rAthena arguments become optional trailing arguments or the options table (rule 11).
4. **Arguments keep rAthena's order.** The converter copies them in place. Flags move into the options table.
5. **`_percent` marks a percent of a flat quantity.** It is used when the quantity is normally a flat number: stats, HP, SP, attack, defense, hit, flee, critical, speed, cast time, delay, and SP cost. So `max_hp` adds points and `max_hp_percent` adds a percent. Damage, healing, experience, and drop bonuses always scale a result, so they never take the suffix, and their values are always percents. The suffix follows the quantity it qualifies: `ignore_defense_percent_against_race`.
6. **Damage dealt is `<kind>_damage_against_<target>`.** The target is `race`, `class`, `size`, `element`, `monster_group`, or `monster`.
7. **Damage taken is `<kind>_damage_taken_from_<source>`.**
8. **The damage kind is always explicit.**
   - No prefix means every kind: physical, magic, and misc.
   - `physical_` means weapon attacks: normal attacks and weapon skills. rAthena calls these `BF_WEAPON`.
   - `magic_` means magic attacks.
   - `misc_` means rAthena's `BF_MISC` attacks, such as traps and falcon attacks.
   - `melee` and `ranged` always mean physical attacks, as in rAthena (`BF_SHORT`, `BF_LONG`).
9. **The value is the change to the named quantity.** A positive value raises the quantity, and a negative value lowers it. So `damage_taken_from_size` with `-15` means 15% less damage taken, and `skill_sp_cost` with `-10` means the skill costs 10 SP less. This flips the sign of rAthena's reduction bonuses (`bSub...`, `...AtkDef`, `bSkillUseSP`, `bSkillUseSPrate`, `bCritDefRate`, `bCriticalDef`, `bReduceDamageReturn`, `bUnbreakable`). The converter negates them. The "Sign" column below marks every flipped type.
10. **Procs are `chance_to_<effect>`.** Examples: `chance_to_cause_status` and `chance_to_cast`. A chance that is a property of an attack roll, rather than a triggered effect, is `<thing>_chance`, as in `double_attack_chance`.
11. **Chances are percents.** A chance argument is a percent and may have decimals, in steps of 0.01. rAthena uses three units: percent, per mille (1000 = 100%), and per 10,000 (10000 = 100%). The arguments column gives the rAthena unit, so the converter knows the divisor.
12. **Times are milliseconds.** Distances are cells. Weight is in tenths, as in wAthena's item weights (a Red Potion weighs 70).
13. **`against` and `from`.** `_against_<kind>` is used when the bonus changes your actions on that target. `_from_<kind>` is used when the bonus changes what that kind does to you or gives you: damage taken, experience, drops, and rewards for a kill.
14. **Per-skill variants start with `skill_`** and take the skill first: `skill_damage`, `skill_cooldown`. Procs keep the `chance_to_` prefix and end in `_on_skill`.
15. **`_highest_only` marks rAthena's non-stacking forms.** Only the highest value of all equipped items applies. The stacking form has the plain name.
16. **Flags take no value.** Flag bonuses are named after the state they give: `bonuses.add("no_knockback")`.

### Options table

Some rAthena forms take trigger flags (`bf`, `atf`, or the autospell `i`). In wAthena these go into an optional table as the last argument:

```lua
bonuses.add("chance_to_cause_status", statuses.stun, 5, { range = "melee", duration = 3000 })
bonuses.add("chance_to_cast", skills.id "MG_FIREBOLT", 3, 5, { kinds = { "magic" }, random_level = true })
```

| Field | Values | rAthena |
|---|---|---|
| `range` | `"melee"` or `"ranged"`. Default: both. | `BF_SHORT`, `BF_LONG`, `ATF_SHORT`, `ATF_LONG` |
| `kinds` | A list of `"physical"`, `"magic"`, `"misc"`. Default: per bonus, usually `{ "physical" }`. | `BF_WEAPON`, `BF_MAGIC`, `BF_MISC`, `ATF_WEAPON`, `ATF_MAGIC`, `ATF_MISC`; `ATF_SKILL` is `{ "magic", "misc" }` |
| `from` | `"normal_attack"` or `"skill"`. Default: per bonus, as in rAthena. | `BF_NORMAL`, `BF_SKILL` |
| `target` | `"self"` or `"enemy"`. | `ATF_SELF`, `ATF_TARGET`, autospell `i & 1` |
| `duration` | Milliseconds. | The `t` argument of `bonus4 bAddEff` and similar |
| `random_level` | `true` picks a level from 1 to the given level. | Autospell `i & 2` |

### Constants used in arguments

Arguments use the constant modules from proposal 0002: `races`, `monster_classes`, `sizes`, `elements`, and `statuses`, plus record lookups (`items.id`, `skills.id`, `monsters.id`). This table needs three more:

- `wa.weapon_types` for rAthena's `W_...` (for example `weapon_types.bow`).
- `wa.monster_groups` for rAthena's `RC2_...` "monster race" (for example `monster_groups.goblin`).
- Item groups, either as `wa.item_groups` or as `items.group "IG_Potion"`.

## Table

Columns:

- **rAthena**: the bonus command and its arguments. Several forms of one type are listed together.
- **Name**: the new name.
- **Arguments**: in order, with units.
- **Meaning**: one line.
- **Uses**: count in renewal item data, per form.
- **Sign**: "flipped" if the converter negates the value (rule 9).

### Base stats

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bStr,n` | `strength` | points | STR + n. | 507 |
| `bonus bAgi,n` | `agility` | points | AGI + n. | 442 |
| `bonus bVit,n` | `vitality` | points | VIT + n. | 389 |
| `bonus bInt,n` | `intelligence` | points | INT + n. | 780 |
| `bonus bDex,n` | `dexterity` | points | DEX + n. | 643 |
| `bonus bLuk,n` | `luck` | points | LUK + n. | 332 |
| `bonus bAllStats,n` | `all_stats` | points | All six base stats + n. | 525 |
| `bonus bAgiVit,n` | none | | AGI and VIT + n. The converter writes `agility` and `vitality`. | 5 |
| `bonus bAgiDexStr,n` | none | | STR, AGI, and DEX + n. The converter writes three bonuses. | 0 |

### Trait stats (fourth jobs)

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bPow,n` | `power` | points | POW + n. | 407 |
| `bonus bSta,n` | `stamina` | points | STA + n. | 42 |
| `bonus bWis,n` | `wisdom` | points | WIS + n. | 33 |
| `bonus bSpl,n` | `spell` | points | SPL + n. | 306 |
| `bonus bCon,n` | `concentration` | points | CON + n. | 281 |
| `bonus bCrt,n` | `creative` | points | CRT + n. | 74 |
| `bonus bAllTraitStats,n` | `all_trait_stats` | points | All six trait stats + n. | 18 |
| `bonus bPatk,n` | `power_attack` | points | P.ATK + n. | 1203 |
| `bonus bPAtkRate,n` | `power_attack_percent` | percent | P.ATK + n%. | 0 |
| `bonus bSmatk,n` | `spell_magic_attack` | points | S.MATK + n. | 752 |
| `bonus bSMatkRate,n` | `spell_magic_attack_percent` | percent | S.MATK + n%. | 0 |
| `bonus bRes,n` | `resistance` | points | RES + n. | 217 |
| `bonus bResRate,n` | `resistance_percent` | percent | RES + n%. | 0 |
| `bonus bMres,n` | `magic_resistance` | points | MRES + n. | 151 |
| `bonus bMResRate,n` | `magic_resistance_percent` | percent | MRES + n%. | 0 |
| `bonus bHplus,n` | `healing_plus` | points | H.PLUS + n. | 21 |
| `bonus bHPlusRate,n` | `healing_plus_percent` | percent | H.PLUS + n%. | 0 |
| `bonus bCrate,n` | `critical_damage_rate` | points | C.RATE + n. | 125 |
| `bonus bCRateRate,n` | `critical_damage_rate_percent` | percent | C.RATE + n%. | 0 |

### HP, SP, and AP

| rAthena | Name | Arguments | Meaning | Uses | Sign |
|---|---|---|---|---|---|
| `bonus bMaxHP,n` | `max_hp` | points | Max HP + n. | 1565 | |
| `bonus bMaxHPrate,n` | `max_hp_percent` | percent | Max HP + n%. | 1579 | |
| `bonus bMaxSP,n` | `max_sp` | points | Max SP + n. | 581 | |
| `bonus bMaxSPrate,n` | `max_sp_percent` | percent | Max SP + n%. | 1042 | |
| `bonus bMaxAP,n` | `max_ap` | points | Max AP + n. | 0 | |
| `bonus bMaxAPrate,n` | `max_ap_percent` | percent | Max AP + n%. | 0 | |
| `bonus bHPrecovRate,n` | `natural_hp_recovery_percent` | percent | Natural HP recovery + n%. | 106 | |
| `bonus bSPrecovRate,n` | `natural_sp_recovery_percent` | percent | Natural SP recovery + n%. | 138 | |
| `bonus2 bHPRegenRate,n,t` | `hp_gain_over_time` | HP, interval in ms | Gain n HP every t ms. | 100 | |
| `bonus2 bHPLossRate,n,t` | `hp_loss_over_time` | HP, interval in ms | Lose n HP every t ms. | 48 | |
| `bonus2 bSPRegenRate,n,t` | `sp_gain_over_time` | SP, interval in ms | Gain n SP every t ms. | 132 | |
| `bonus2 bSPLossRate,n,t` | `sp_loss_over_time` | SP, interval in ms | Lose n SP every t ms. | 27 | |
| `bonus2 bRegenPercentHP,n,t` | `hp_gain_percent_over_time` | percent of max HP, interval in ms | Gain n% of max HP every t ms. | 4 | |
| `bonus2 bRegenPercentSP,n,t` | `sp_gain_percent_over_time` | percent of max SP, interval in ms | Gain n% of max SP every t ms. | 6 | |
| `bonus bNoRegen,x` | `no_hp_recovery`, `no_sp_recovery` | flag | Stops natural HP (x = 1) or SP (x = 2) recovery. The converter picks the name from x. | 17 | |
| `bonus bUseSPrate,n` | `sp_cost_percent` | percent | SP cost of all skills + n%. | 269 | |

### Attack and defense

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bBaseAtk,n` | `attack` (proposal 0002 says `base_attack`; see question 1) | points | Equipment ATK + n. This is the "ATK + n" of renewal items. | 2623 |
| `bonus bAtk,n` | `weapon_attack` (proposal 0002 says `attack`; see question 1) | points | Weapon ATK + n. rAthena marks it unofficial. | 1 |
| `bonus bAtk2,n` | `refine_attack` | points | ATK that counts like refine ATK + n. | 1 |
| `bonus bAtkRate,n` | `attack_percent` | percent | ATK + n%. | 1619 |
| `bonus bWeaponAtkRate,n` | `weapon_attack_percent` | percent | Weapon ATK + n%. | 7 |
| `bonus2 bWeaponAtk,w,n` | `attack_with_weapon_type` | weapon type, points | ATK + n while a weapon of type w is equipped. | 0 |
| `bonus bMatk,n` | `magic_attack` | points | MATK + n. | 1767 |
| `bonus bMatk2,n` | `hidden_magic_attack` | points | MATK + n, not shown in the status window. | 0 |
| `bonus bMatkRate,n` | `magic_attack_percent` | percent | MATK + n%. | 1495 |
| `bonus bWeaponMatkRate,n` | `weapon_magic_attack_percent` | percent | Weapon MATK + n%. | 0 |
| `bonus bDef,n` | `defense` | points | Equipment DEF + n. | 641 |
| `bonus bDefRate,n` | `defense_percent` | percent | Equipment DEF + n%. | 9 |
| `bonus bDef2,n` | `status_defense` | points | DEF from stats (VIT) + n. | 0 |
| `bonus bDef2Rate,n` | `status_defense_percent` | percent | DEF from stats + n%. | 5 |
| `bonus bMdef,n` | `magic_defense` | points | Equipment MDEF + n. | 1346 |
| `bonus bMdefRate,n` | `magic_defense_percent` | percent | Equipment MDEF + n%. | 5 |
| `bonus bMdef2,n` | `status_magic_defense` | points | MDEF from stats (INT) + n. | 0 |
| `bonus bMdef2Rate,n` | `status_magic_defense_percent` | percent | MDEF from stats + n%. | 0 |
| `bonus bAtkRange,n` | `attack_range` | cells | Attack range + n. | 1 |

### Hit, flee, and critical

| rAthena | Name | Arguments | Meaning | Uses | Sign |
|---|---|---|---|---|---|
| `bonus bHit,n` | `hit` | points | HIT + n. | 531 | |
| `bonus bHitRate,n` | `hit_percent` | percent | HIT + n%. | 1 | |
| `bonus bFlee,n` | `flee` | points | FLEE + n. | 426 | |
| `bonus bFleeRate,n` | `flee_percent` | percent | FLEE + n%. | 1 | |
| `bonus bFlee2,n` | `perfect_dodge` | points | Perfect Dodge + n. | 155 | |
| `bonus bFlee2Rate,n` | `perfect_dodge_percent` | percent | Perfect Dodge + n%. | 0 | |
| `bonus bCritical,n` | `critical` | points | CRIT + n. | 757 | |
| `bonus bCriticalRate,n` | `critical_percent` | percent | CRIT + n%. | 4 | |
| `bonus bCriticalLong,n` | `ranged_critical` | points | CRIT + n for ranged normal attacks only. Not shown in the status window. | 14 | |
| `bonus2 bCriticalAddRace,r,n` | `critical_against_race` | race, points | CRIT + n against race r. | 41 | |
| `bonus bCriticalDef,n` | `critical_chance_taken` | percent | Chance of being hit by a critical changes by n%. | 0 | flipped |
| `bonus bPerfectHitRate,n` | `perfect_hit_chance_highest_only` | percent | n% chance that an attack always hits. Only the highest applies. | 1 | |
| `bonus bPerfectHitAddRate,n` | `perfect_hit_chance` | percent | n% more chance that an attack always hits. | 193 | |
| `bonus bDoubleRate,n` | `double_attack_chance_highest_only` | percent | n% chance of a double attack, with any weapon. Only the highest applies. | 0 | |
| `bonus bDoubleAddRate,n` | `double_attack_chance` | percent | n% more chance of a double attack, with any weapon. | 0 | |

### Speed and cast

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bSpeedRate,n` | `move_speed_percent_highest_only` | percent | Move speed + n%. Only the highest applies, and it does not stack with speed potions. | 73 |
| `bonus bSpeedAddRate,n` | `move_speed_percent` | percent | Move speed + n%. | 0 |
| `bonus bAspd,n` | `attack_speed` | points | ASPD + n. | 296 |
| `bonus bAspdRate,n` | `attack_speed_percent` | percent | ASPD + n%. | 1291 |
| `bonus bVariableCastrate,n`<br>`bonus2 bVariableCastrate,sk,n` | `variable_cast_time_percent`<br>`skill_variable_cast_time_percent` | percent<br>skill, percent | Variable cast time + n%, for all skills or for skill sk. | 1209<br>229 |
| `bonus bCastrate,n`<br>`bonus2 bCastrate,sk,n` | none | | Same as `bVariableCastrate` in renewal. The converter writes the variable cast names. | 1<br>0 |
| `bonus bFixedCastrate,n`<br>`bonus2 bFixedCastrate,sk,n` | `fixed_cast_time_percent`<br>`skill_fixed_cast_time_percent` | percent<br>skill, percent | Fixed cast time + n%, for all skills or for skill sk. The all-skills form keeps only the lowest value. | 67<br>61 |
| `bonus bVariableCast,t` | `variable_cast_time` | ms | Variable cast time of all skills + t. | 1 |
| `bonus2 bSkillVariableCast,sk,t` | `skill_variable_cast_time` | skill, ms | Variable cast time of skill sk + t. | 30 |
| `bonus bFixedCast,t` | `fixed_cast_time` | ms | Fixed cast time of all skills + t. | 501 |
| `bonus2 bSkillFixedCast,sk,t` | `skill_fixed_cast_time` | skill, ms | Fixed cast time of skill sk + t. | 28 |
| `bonus bDelayrate,n` | `after_cast_delay_percent` | percent | After-cast delay of all skills + n%. | 1227 |
| `bonus2 bSkillDelay,sk,t` | `skill_after_cast_delay` | skill, ms | After-cast delay of skill sk + t. | 0 |
| `bonus bNoCastCancel` | `uninterruptible_cast` | flag | Casting is not interrupted by hits, except in guild wars. | 54 |
| `bonus bNoCastCancel2` | `uninterruptible_cast_everywhere` | flag | Casting is not interrupted by hits, also in guild wars. | 3 |
| `bonus bNoWalkDelay` | `always_endure` | flag | Hits never stop the player (permanent Endure). | 11 |

### Damage dealt, by target

All values are percents of damage dealt.

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus2 bAddRace,r,x` | `physical_damage_against_race` (proposal 0002 says `damage_against_race`; see question 2) | race, percent | Physical damage + x% against race r. | 1980 |
| `bonus2 bMagicAddRace,r,x` | `magic_damage_against_race` | race, percent | Magic damage + x% against race r. | 1046 |
| `bonus2 bAddClass,c,x` | `physical_damage_against_class` (proposal 0002 says `damage_against_class`; see question 2) | class, percent | Physical damage + x% against class c. | 384 |
| `bonus2 bMagicAddClass,c,x` | `magic_damage_against_class` | class, percent | Magic damage + x% against class c. | 91 |
| `bonus2 bAddSize,s,x` | `physical_damage_against_size` | size, percent | Physical damage + x% against size s. | 867 |
| `bonus2 bMagicAddSize,s,x` | `magic_damage_against_size` | size, percent | Magic damage + x% against size s. | 487 |
| `bonus2 bAddEle,e,x`<br>`bonus3 bAddEle,e,x,bf` | `physical_damage_against_element` | element, percent, [options] | Physical damage + x% against targets of element e. | 1154<br>0 |
| `bonus2 bMagicAddEle,e,x` | `magic_damage_against_element` | element, percent | Magic damage + x% against targets of element e. | 866 |
| `bonus2 bAddRace2,mr,x` | `physical_damage_against_monster_group` | monster group, percent | Physical damage + x% against monster group mr. | 59 |
| `bonus2 bMagicAddRace2,mr,x` | `magic_damage_against_monster_group` | monster group, percent | Magic damage + x% against monster group mr. | 46 |
| `bonus2 bAddDamageClass,mid,x` | `physical_damage_against_monster` | monster (job for players), percent | Physical damage + x% against monster mid. rAthena keeps at most 10. | 60 |
| `bonus2 bAddMagicDamageClass,mid,x` | `magic_damage_against_monster` | monster (job for players), percent | Magic damage + x% against monster mid. | 24 |
| `bonus2 bMagicAtkEle,e,x` | `magic_damage_with_element` | element, percent | Damage of your element e magic + x%. | 2531 |
| `bonus bShortAtkRate,n` | `melee_damage` | percent | Melee physical damage + n%. | 951 |
| `bonus bLongAtkRate,n` | `ranged_damage` | percent | Ranged physical damage + n%. | 1637 |
| `bonus bCritAtkRate,n` | `critical_damage` | percent | Critical damage + n%. | 732 |
| `bonus bNonCritAtkRate,n` | `non_critical_damage` | percent | Non-critical damage + n%. | 79 |
| `bonus2 bWeaponDamageRate,w,n` | `physical_damage_with_weapon_type` | weapon type, percent | Physical damage + n% while a weapon of type w is equipped. | 20 |
| `bonus bSkillRatio,n` | `skill_ratio` | percentage points | Adds n to the damage ratio of every attack and skill. | 0 |
| `bonus bNoSizeFix` | `ignore_size_penalty` | flag | Weapon size penalties do not apply. | 29 |

### Ignoring and changing defense

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bIgnoreDefRace,r` | `ignore_defense_against_race` | race | Ignore DEF of race r. | 47 |
| `bonus bIgnoreDefEle,e` | `ignore_defense_against_element` | element | Ignore DEF of element e. | 0 |
| `bonus bIgnoreDefClass,c` | `ignore_defense_against_class` | class | Ignore DEF of class c. | 20 |
| `bonus bIgnoreMdefRace,r` | `ignore_magic_defense_against_race` | race | Ignore MDEF of race r. | 1 |
| `bonus bIgnoreMdefEle,e` | `ignore_magic_defense_against_element` | element | Ignore MDEF of element e. | 0 |
| `bonus2 bIgnoreDefRaceRate,r,n` | `ignore_defense_percent_against_race` | race, percent | Ignore n% of DEF of race r. | 656 |
| `bonus2 bIgnoreDefClassRate,c,n` | `ignore_defense_percent_against_class` | class, percent | Ignore n% of DEF of class c. | 32 |
| `bonus2 bIgnoreMdefRaceRate,r,n` | `ignore_magic_defense_percent_against_race` | race, percent | Ignore n% of MDEF of race r. | 308 |
| `bonus2 bIgnoreMdefClassRate,c,n` | `ignore_magic_defense_percent_against_class` | class, percent | Ignore n% of MDEF of class c. | 58 |
| `bonus2 bIgnoreMdefRace2Rate,mr,n` | `ignore_magic_defense_percent_against_monster_group` | monster group, percent | Ignore n% of MDEF of monster group mr. | 1 |
| `bonus2 bIgnoreResRaceRate,r,n` | `ignore_resistance_percent_against_race` | race, percent | Ignore n% of RES of race r. | 18 |
| `bonus2 bIgnoreMResRaceRate,r,n` | `ignore_magic_resistance_percent_against_race` | race, percent | Ignore n% of MRES of race r. | 18 |
| `bonus bDefRatioAtkRace,r` | `defense_as_damage_against_race` | race | Against race r, ignore DEF and deal more damage the higher the DEF. | 0 |
| `bonus bDefRatioAtkEle,e` | `defense_as_damage_against_element` | element | The same, against element e. | 0 |
| `bonus bDefRatioAtkClass,c` | `defense_as_damage_against_class` | class | The same, against class c. | 14 |
| `bonus4 bSetDefRace,r,n,t,y` | `chance_to_set_defense_against_race` | race, chance (rAthena: per 10,000), duration in ms, DEF value | Normal attacks have an n chance to set the DEF of race r to y for t ms. | 5 |
| `bonus4 bSetMDefRace,r,n,t,y` | `chance_to_set_magic_defense_against_race` | race, chance (rAthena: per 10,000), duration in ms, MDEF value | The same for MDEF. | 5 |

### Damage taken, by source

All values are percents of damage taken. A negative value means less damage (rule 9).

| rAthena | Name | Arguments | Meaning | Uses | Sign |
|---|---|---|---|---|---|
| `bonus2 bSubRace,r,x`<br>`bonus3 bSubRace,r,x,bf` | `damage_taken_from_race` | race, percent, [options] | Damage taken from race r changes by x%. | 1631<br>1 | flipped |
| `bonus2 bSubClass,c,x` | `damage_taken_from_class` | class, percent | Damage taken from class c changes by x%. | 281 | flipped |
| `bonus2 bSubSize,s,x` | `damage_taken_from_size` (proposal 0002 says `damage_taken_from_size_percent`; see question 4) | size, percent | Damage taken from size s changes by x%. | 147 | flipped |
| `bonus2 bWeaponSubSize,s,x` | `physical_damage_taken_from_size` | size, percent | Physical damage taken from size s changes by x%. | 0 | flipped |
| `bonus2 bMagicSubSize,s,x` | `magic_damage_taken_from_size` | size, percent | Magic damage taken from size s changes by x%. | 2 | flipped |
| `bonus2 bSubEle,e,x`<br>`bonus3 bSubEle,e,x,bf` | `damage_taken_from_element` | element, percent, [options] | Damage taken from attacks of element e changes by x%. | 931<br>12 | flipped |
| `bonus2 bSubDefEle,e,x` | `physical_damage_taken_from_attacker_element` | element, percent | Physical damage taken from attackers whose own element is e changes by x%. | 8 | flipped |
| `bonus2 bMagicSubDefEle,e,x` | `magic_damage_taken_from_attacker_element` | element, percent | The same for magic damage. | 3 | flipped |
| `bonus2 bSubRace2,mr,x` | `damage_taken_from_monster_group` | monster group, percent | Damage taken from monster group mr changes by x%. | 30 | flipped |
| `bonus2 bAddDefMonster,mid,x` | `physical_damage_taken_from_monster` | monster (job for players), percent | Physical damage taken from monster mid changes by x%. | 33 | flipped |
| `bonus2 bAddMdefMonster,mid,x` | `magic_damage_taken_from_monster` | monster (job for players), percent | Magic damage taken from monster mid changes by x%. | 3 | flipped |
| `bonus bNearAtkDef,n` | `melee_damage_taken` | percent | Melee physical damage taken changes by n%. | 11 | flipped |
| `bonus bLongAtkDef,n` | `ranged_damage_taken` | percent | Ranged physical damage taken changes by n%. | 83 | flipped |
| `bonus bMagicAtkDef,n` | `magic_damage_taken` | percent | Magic damage taken changes by n%. | 2 | flipped |
| `bonus bMiscAtkDef,n` | `misc_damage_taken` | percent | Misc damage taken (traps, falcon) changes by n%. | 0 | flipped |
| `bonus bCritDefRate,n` | `critical_damage_taken` | percent | Critical damage taken changes by n%. | 0 | flipped |
| `bonus bNoWeaponDamage,n` | `physical_immunity` | percent | Blocks n% of physical damage. Applied before other reductions. | 0 | |
| `bonus bNoMagicDamage,n` | `magic_immunity` | percent | Blocks n% of magic. This also blocks heals and support skills. | 3 | |
| `bonus bNoMiscDamage,n` | `misc_immunity` | percent | Blocks n% of misc damage. | 0 | |
| `bonus bAbsorbDmgMaxHP2,n` | `damage_taken_cap` | percent of max HP | A hit above n% of max HP is reduced to n% of max HP. Only the highest applies. | 1 | |
| `bonus bAbsorbDmgMaxHP,n` | none | | Old rAthena form of the cap. Not official. The converter writes `damage_taken_cap`. | 0 | |
| `bonus bShortWeaponDamageReturn,n` | `melee_damage_reflected` | percent | Reflects n% of melee damage taken. | 45 | |
| `bonus bLongWeaponDamageReturn,n` | `ranged_damage_reflected` | percent | Reflects n% of ranged damage taken. | 0 | |
| `bonus bMagicDamageReturn,n` | `chance_to_reflect_magic` | percent | n% chance to reflect a targeted spell back to its caster. | 11 | |
| `bonus bReduceDamageReturn,n` | `reflected_damage_taken` | percent | Damage taken from reflection changes by n%. | 39 | flipped |

### Elements

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bAtkEle,e` | `attack_element` | element | Attacks have element e. | 266 |
| `bonus bDefEle,e` | `defense_element` | element | The player's body has element e. | 60 |

### Status effects

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus2 bAddEff,eff,n`<br>`bonus3 bAddEff,eff,n,atf`<br>`bonus4 bAddEff,eff,n,atf,t` | `chance_to_cause_status` | status, chance (rAthena: per 10,000), [options] | Attacks have an n chance to cause status eff. | 163<br>50<br>2 |
| `bonus2 bAddEff2,eff,n` | none | | Status eff on yourself when attacking. The converter writes `chance_to_cause_status` with `{ target = "self" }`. | 25 |
| `bonus2 bAddEffWhenHit,eff,n`<br>`bonus3 bAddEffWhenHit,eff,n,atf`<br>`bonus4 bAddEffWhenHit,eff,n,atf,t` | `chance_to_cause_status_when_hit` | status, chance (rAthena: per 10,000), [options] | Being hit by physical damage has an n chance to cause status eff on the attacker. | 61<br>23<br>0 |
| `bonus3 bAddEffOnSkill,sk,eff,n`<br>`bonus4 bAddEffOnSkill,sk,eff,n,atf`<br>`bonus5 bAddEffOnSkill,sk,eff,n,atf,t` | `chance_to_cause_status_on_skill` | skill, status, chance (rAthena: per 10,000), [options] | Using skill sk has an n chance to cause status eff. | 24<br>0<br>0 |
| `bonus2 bResEff,eff,n` | `status_resistance` | status, percent (rAthena: per 10,000) | Resistance to status eff + n. | 379 |
| `bonus2 bComaRace,r,n` | `chance_to_cause_coma_against_race` | race, chance (rAthena: per 10,000), [options] | Any attack on race r has an n chance to cause Coma. | 10 |
| `bonus2 bComaClass,c,n` | `chance_to_cause_coma_against_class` | class, chance (rAthena: per 10,000), [options] | Any attack on class c has an n chance to cause Coma. | 3 |
| `bonus2 bWeaponComaRace,r,n` | none | | Coma on normal attacks only. The converter writes `chance_to_cause_coma_against_race` with `{ from = "normal_attack" }`. | 0 |
| `bonus2 bWeaponComaClass,c,n` | none | | The same for class: `chance_to_cause_coma_against_class` with `{ from = "normal_attack" }`. | 0 |
| `bonus2 bWeaponComaEle,e,n` | `chance_to_cause_coma_against_element` | element, chance (rAthena: per 10,000), [options] | Normal attacks on element e have an n chance to cause Coma. The converter adds `{ from = "normal_attack" }`. | 0 |
| `bonus3 bStateNoRecoverRace,r,x,t` | `chance_to_block_healing_against_race` | race, chance (rAthena: per 10,000), duration in ms | Normal attacks on race r have an x chance to stop the target's healing for t ms. | 2 |

### Skills

| rAthena | Name | Arguments | Meaning | Uses | Sign |
|---|---|---|---|---|---|
| `skill sk,lv` (a command) | `skill` | skill, level | Grants skill sk at level lv while equipped. Fixed in proposal 0002. | 633 | |
| `bonus2 bSkillAtk,sk,n` | `skill_damage` | skill, percent | Damage of skill sk + n%. | 7893 | |
| `bonus2 bSubSkill,sk,n` | `skill_damage_taken` | skill, percent | Damage taken from skill sk changes by n%. | 24 | flipped |
| `bonus2 bSkillUseSP,sk,n` | `skill_sp_cost` | skill, SP | SP cost of skill sk changes by n. | 269 | flipped |
| `bonus2 bSkillUseSPrate,sk,n` | `skill_sp_cost_percent` | skill, percent | SP cost of skill sk changes by n%. | 75 | flipped |
| `bonus2 bSkillCooldown,sk,t` | `skill_cooldown` | skill, ms | Cooldown of skill sk + t. | 618 | |
| `bonus2 bAddSkillBlow,sk,n` | `skill_knockback` | skill, cells | Skill sk knocks the target back n cells. | 8 | |
| `bonus bNoGemStone` | `no_gemstone_cost` | flag | Skills need no gemstones. Hocus Pocus and Ganbantein keep their cost. | 16 | |
| `bonus bNoMadoFuel` | `no_magic_gear_fuel_cost` | flag | Skills need no Magic Gear Fuel. | 9 | |

The per-skill cast time and delay names are in "Speed and cast". The per-skill healing names are in "Healing".

### Autospell and other procs

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus3 bAutoSpell,sk,y,n`<br>`bonus4 bAutoSpell,sk,y,n,i`<br>`bonus5 bAutoSpell,sk,y,n,bf,i` | `chance_to_cast` | skill, level, chance (rAthena: per mille), [options] | Attacks have an n chance to cast skill sk at level y. | 375<br>25<br>90 |
| `bonus3 bAutoSpellWhenHit,sk,y,n`<br>`bonus4 bAutoSpellWhenHit,sk,y,n,i`<br>`bonus5 bAutoSpellWhenHit,sk,y,n,bf,i` | `chance_to_cast_when_hit` | skill, level, chance (rAthena: per mille), [options] | Being hit by a direct attack has an n chance to cast skill sk at level y. | 145<br>35<br>60 |
| `bonus4 bAutoSpellOnSkill,sk,x,y,n`<br>`bonus5 bAutoSpellOnSkill,sk,x,y,n,i` | `chance_to_cast_on_skill` | trigger skill, skill, level, chance (rAthena: per mille), [options] | Using skill sk has an n chance to cast skill x at level y. | 71<br>10 |
| `bonus2 bHPDrainRate,x,n` | `chance_to_drain_hp` | chance (rAthena: per mille), percent of damage | Physical attacks have an x chance to heal n% of the damage dealt as HP. | 92 |
| `bonus2 bSPDrainRate,x,n` | `chance_to_drain_sp` | chance (rAthena: per mille), percent of damage | The same, as SP. | 66 |
| `bonus bHPDrainValue,n` | `hp_per_normal_attack` | HP | Each normal attack heals n HP. | 1 |
| `bonus2 bHPDrainValueRace,r,n` | `hp_per_normal_attack_against_race` | race, HP | The same, against race r. | 0 |
| `bonus2 bHPDrainValueClass,c,n` | `hp_per_normal_attack_against_class` | class, HP | The same, against class c. | 0 |
| `bonus bSPDrainValue,n` | `sp_per_normal_attack` | SP | Each normal attack restores n SP. | 39 |
| `bonus2 bSPDrainValueRace,r,n` | `sp_per_normal_attack_against_race` | race, SP | The same, against race r. | 5 |
| `bonus2 bSPDrainValueClass,c,n` | `sp_per_normal_attack_against_class` | class, SP | The same, against class c. | 0 |
| `bonus2 bHPVanishRate,x,n`<br>`bonus3 bHPVanishRate,x,n,bf` | `chance_to_remove_hp` | chance (rAthena: per mille), percent of target's HP, [options] | Attacks have an x chance to remove n% of the target's current HP. The two-argument form is normal attacks only. | 1<br>0 |
| `bonus2 bSPVanishRate,x,n`<br>`bonus3 bSPVanishRate,x,n,bf` | `chance_to_remove_sp` | chance (rAthena: per mille), percent of target's SP, [options] | The same for SP. | 1<br>5 |
| `bonus3 bHPVanishRaceRate,r,x,n` | `chance_to_deal_max_hp_percent_against_race` | race, chance (rAthena: per mille), percent of max HP | Against race r, a physical hit has an x chance to deal n% of the target's max HP instead of its damage. | 2 |
| `bonus3 bSPVanishRaceRate,r,x,n` | `chance_to_deal_max_sp_percent_against_race` | race, chance (rAthena: per mille), percent of max SP | The same, as SP damage. | 5 |
| `bonus bBreakWeaponRate,n` | `chance_to_break_weapon` | chance (rAthena: per 10,000) | Attacks have an n chance to break the target's weapon. | 12 |
| `bonus bBreakArmorRate,n` | `chance_to_break_armor` | chance (rAthena: per 10,000) | Attacks have an n chance to break the target's armor. | 17 |
| `bonus bClassChange,n` | `chance_to_transform_monster` | chance (rAthena: per 10,000) | Physical attacks have an n chance to turn the monster into a random other monster. | 2 |

### Drops, zeny, and experience

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus2 bExpAddRace,r,x` | `experience_from_race` | race, percent | Experience from race r + x%. | 295 |
| `bonus2 bExpAddClass,c,x` | `experience_from_class` | class, percent | Experience from class c + x%. | 12 |
| `bonus2 bDropAddRace,r,x` | `drop_rate_from_race` | race, percent | Drop rates of race r + x%. | 55 |
| `bonus2 bDropAddClass,c,x` | `drop_rate_from_class` | class, percent | Drop rates of class c + x%. | 2 |
| `bonus2 bAddMonsterDropItem,iid,n`<br>`bonus3 bAddMonsterDropItem,iid,r,n` | `extra_drop_from_race` | item, race, chance (rAthena: per 10,000) | Killing a monster of race r has an n chance to drop item iid. The two-argument form uses `races.all`. A negative n means a chance of -n × level / 10 + 1. | 215<br>77 |
| `bonus3 bAddClassDropItem,iid,c,n` | `extra_drop_from_class` | item, class, chance (rAthena: per 10,000) | The same, for class c. | 0 |
| `bonus3 bAddMonsterIdDropItem,iid,mid,n` | `extra_drop_from_monster` | item, monster, chance (rAthena: per 10,000) | The same, for monster mid. | 62 |
| `bonus2 bAddMonsterDropItemGroup,ig,n`<br>`bonus3 bAddMonsterDropItemGroup,ig,r,n` | `extra_group_drop_from_race` | item group, race, chance (rAthena: per 10,000) | Killing a monster of race r has an n chance to drop an item from group ig. | 8<br>0 |
| `bonus3 bAddClassDropItemGroup,ig,c,n` | `extra_group_drop_from_class` | item group, class, chance (rAthena: per 10,000) | The same, for class c. | 0 |
| `bonus2 bGetZenyNum,x,n` | `chance_to_gain_zeny_on_kill_highest_only` | maximum zeny, chance (percent) | A kill has an n% chance to give 1 to x zeny. A negative x means x × monster level. Only the highest applies. | 3 |
| `bonus2 bAddGetZenyNum,x,n` | `chance_to_gain_zeny_on_kill` | maximum zeny, chance (percent) | The same, stacking. | 0 |
| `bonus bAddStealRate,n` | `steal_chance` | chance (rAthena: per 10,000) | Steal success chance + n. | 0 |

### Healing and rewards for a kill

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bHealPower,n` | `healing_done` | percent | Healing from your heal skills + n%. | 359 |
| `bonus bHealPower2,n` | `healing_received` | percent | Healing you receive from skills + n%. | 80 |
| `bonus2 bSkillHeal,sk,n` | `skill_healing_done` | skill, percent | Healing from your skill sk + n%. | 7 |
| `bonus2 bSkillHeal2,sk,n` | `skill_healing_received` | skill, percent | Healing you receive from skill sk + n%. | 2 |
| `bonus bAddItemHealRate,n`<br>`bonus2 bAddItemHealRate,iid,n` | `hp_recovery_from_items`<br>`hp_recovery_from_item` | percent<br>item, percent | HP from healing items + n%, for all items or for item iid. | 63<br>73 |
| `bonus2 bAddItemGroupHealRate,ig,n` | `hp_recovery_from_item_group` | item group, percent | HP from items of group ig + n%. | 18 |
| `bonus bAddItemSPHealRate,n`<br>`bonus2 bAddItemSPHealRate,iid,n` | `sp_recovery_from_items`<br>`sp_recovery_from_item` | percent<br>item, percent | SP from healing items + n%, for all items or for item iid. | 0<br>1 |
| `bonus2 bAddItemGroupSPHealRate,ig,n` | `sp_recovery_from_item_group` | item group, percent | SP from items of group ig + n%. | 0 |
| `bonus bHPGainValue,n` | `hp_per_melee_kill` | HP | A kill by a melee physical attack heals n HP. | 68 |
| `bonus bSPGainValue,n` | `sp_per_melee_kill` | SP | A kill by a melee physical attack restores n SP. | 90 |
| `bonus2 bSPGainRace,r,n` | `sp_per_melee_kill_from_race` | race, SP | The same, for a monster of race r. | 28 |
| `bonus bLongHPGainValue,n` | `hp_per_ranged_kill` | HP | A kill by a ranged physical attack heals n HP. | 3 |
| `bonus bLongSPGainValue,n` | `sp_per_ranged_kill` | SP | A kill by a ranged physical attack restores n SP. | 19 |
| `bonus bMagicHPGainValue,n` | `hp_per_magic_kill` | HP | A kill by magic heals n HP. | 38 |
| `bonus bMagicSPGainValue,n` | `sp_per_magic_kill` | SP | A kill by magic restores n SP. | 49 |
| `bonus bRestartFullRecover` | `full_recovery_on_revive` | flag | Revival restores full HP and SP. | 27 |

### Equipment protection

| rAthena | Name | Arguments | Meaning | Uses | Sign |
|---|---|---|---|---|---|
| `bonus bUnbreakableWeapon` | `unbreakable_weapon` | flag | The weapon cannot break. | 597 | |
| `bonus bUnbreakableArmor` | `unbreakable_armor` | flag | The armor cannot break. | 105 | |
| `bonus bUnbreakableHelm` | `unbreakable_helm` | flag | The helm cannot break. | 149 | |
| `bonus bUnbreakableShield` | `unbreakable_shield` | flag | The shield cannot break. | 21 | |
| `bonus bUnbreakableGarment` | `unbreakable_garment` | flag | The garment cannot break. | 17 | |
| `bonus bUnbreakableShoes` | `unbreakable_shoes` | flag | The shoes cannot break. | 11 | |
| `bonus bUnbreakable,n` | `equipment_break_chance` | percent | The chance that any equipped item breaks changes by n%. | 0 | flipped |
| `bonus bUnstripableWeapon` | `unstrippable_weapon` | flag | Strip skills cannot remove the weapon. | 0 | |
| `bonus bUnstripableArmor` | `unstrippable_armor` | flag | Strip skills cannot remove the armor. | 0 | |
| `bonus bUnstripableHelm` | `unstrippable_helm` | flag | Strip skills cannot remove the helm. | 0 | |
| `bonus bUnstripableShield` | `unstrippable_shield` | flag | Strip skills cannot remove the shield. | 0 | |
| `bonus bUnstripable` | none | | The documentation says "all equipment", but `pc_bonus` protects only the armor. The converter writes `unstrippable_armor`, which keeps rAthena's behavior. | 0 | |

### Other

| rAthena | Name | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bSplashRange,n` | `splash_range_highest_only` | cells | Normal attacks hit an area of radius n (1 is 3×3). Only the highest applies. | 52 |
| `bonus bSplashAddRange,n` | `splash_range` | cells | Splash radius + n. | 0 |
| `bonus bNoKnockback` | `no_knockback` | flag | Skills cannot knock the player back. | 52 |
| `bonus bIntravision` | `see_hidden` | flag | The player sees hidden and cloaked characters. | 12 |
| `bonus bPerfectHide` | `hidden_from_detectors` | flag | While hidden or cloaked, monsters that detect hidden characters do not see the player. | 0 |
| `bonus bAddMaxWeight,n` | `max_weight` | tenths of weight | Weight limit + n. | 0 |
| `bFame` | none | | Not a bonus. It is a value `readparam` reads, and `pc_bonus` rejects it. | 0 |

## Deprecated and unused types

Types with no wAthena name. The converter rewrites each one:

| rAthena | Converter writes | Reason |
|---|---|---|
| `bAgiVit`, `bAgiDexStr` | One bonus per stat | Shortcuts. |
| `bCastrate` (both forms) | `variable_cast_time_percent`, `skill_variable_cast_time_percent` | The same as `bVariableCastrate` in renewal. |
| `bAddEff2` | `chance_to_cause_status` with `{ target = "self" }` | The same as `bAddEff` with `ATF_SELF`. |
| `bWeaponComaRace`, `bWeaponComaClass` | The coma names with `{ from = "normal_attack" }` | Only the trigger differs. |
| `bAbsorbDmgMaxHP` | `damage_taken_cap` | rAthena calls it legacy and not official. Unused in renewal data. |
| `bUnstripable` | `unstrippable_armor` | Its code protects only the armor. |
| `bFame` | Nothing | Not a bonus. |

Types that are kept, but have no use in renewal data: `bMaxAP`, `bMaxAPrate`, `bDef2`, `bMdef2`, `bMdef2Rate`, `bFlee2Rate`, the six trait `...Rate` types, `bCriticalDef`, `bDoubleRate`, `bDoubleAddRate`, `bSpeedAddRate`, `bWeaponMatkRate`, `bWeaponAtk`, `bMatk2`, `bIgnoreDefEle`, `bIgnoreMdefEle`, `bDefRatioAtkEle`, `bDefRatioAtkRace`, `bMiscAtkDef`, `bNoWeaponDamage`, `bNoMiscDamage`, `bCritDefRate`, `bWeaponSubSize`, `bLongWeaponDamageReturn`, `bWeaponComaEle`, `bHPDrainValueRace`, `bHPDrainValueClass`, `bSPDrainValueClass`, `bAddClassDropItem`, `bAddClassDropItemGroup`, `bAddGetZenyNum`, `bAddStealRate`, `bSplashAddRange`, `bPerfectHide`, `bAddMaxWeight`, `bSkillDelay`, `bSkillRatio`, `bUnbreakable`, the four `bUnstripable...` slot types, and `bAddItemGroupSPHealRate`. They cost one table row each, pre-renewal data and other servers' items use some of them, and mods may want them. Question 12 asks whether to drop them instead.

## autobonus and bonus_script

These are not bonus types. They map to events, random chances, and timed bonuses.

### autobonus, autobonus2, autobonus3

rAthena:

```
autobonus "{ bonus bAllStats,10; }",10,10000,BF_WEAPON|BF_MISC,"{ specialeffect2 EF_FIRESPLASHHIT; }";
```

- The rate is per mille. The duration is in milliseconds.
- The flag uses the `bf` values, with the same defaults as `bAutoSpell`.
- When it triggers while active, rAthena restarts the timer. It does not stack.
- The second script runs once on trigger. It is used for effects.

wAthena:

```lua
return function()
  local player = context:get_instance().wearer
  local active
  events.on("attack", function(attack)
    if (attack.is_physical or attack.is_misc) and random.chance(10, 1000) then
      if active then active:cancel() end
      active = timers.with_bonuses(10000, function() bonuses.add("all_stats", 10) end)
      player:show_effect(effects.fire_splash_hit)
    end
  end)
end
```

| rAthena | Event | Count in renewal data |
|---|---|---|
| `autobonus` | `"attack"` | 422 |
| `autobonus2` | `"hit_taken"` | 52 |
| `autobonus3` (trigger skill) | `"skill_used"`, a new item event, with a check on `use.skill` | 158 |

- The event gets fields for the `bf` checks: `is_physical`, `is_magic`, `is_misc`, `is_melee`, `is_ranged`, and `is_skill`. Proposal 0002 uses `hit.is_weapon`; see question 14.
- `timers.with_bonuses` returns a handle. The converter writes the `cancel` line to keep rAthena's restart behavior. An option such as `timers.with_bonuses(ms, fn, { restart = active })` would be shorter; see question 14.

### bonus_script

rAthena:

```
bonus_script "{ bonus bHPRecovRate,rand(11,33); }",1800,1;
```

- It attaches bonuses to the player for a duration in seconds. It is used by consumables (17 uses in renewal data).
- It is saved at logout and restored at login.
- Its flags say what removes it: death, Dispell, Clearance, logout, damage, and others. It can show a status icon.
- A second use replaces or adds to the first one, depending on flags.

wAthena: a consumable's `use` context ends at once, so the bonuses need the player as owner, and they must be saved. A Lua function cannot be saved, so the bonuses are data, computed when the item is used:

```lua
events.on("use", function(player)
  player:add_timed_bonuses("hp_recovery_food", 1800000, {
    { "natural_hp_recovery_percent", random.integer(11, 33) },
  }, { ends_on = { "death" } })
end)
```

- The key (`"hp_recovery_food"`) replaces an older copy with the same key. This is rAthena's default. rAthena's "add a duplicate" flag (2048) would need an option.
- `ends_on` lists the removal flags by name: `"death"`, `"dispell"`, `"clearance"`, `"logout"`, `"banishing_buster"`, `"refresh"`, `"lux_anima"`, `"magic_gear_change"`, `"damage"`.
- Options `icon` and `kind` (`"buff"` or `"debuff"`) cover rAthena's icon and type.
- This is a new player method. It needs approval as an API addition; see question 14.

## Questions for review

1. **Attack names.** In renewal, `bBaseAtk` is the "ATK + n" on items (2623 uses), and `bAtk` is weapon ATK (1 use, marked unofficial). Proposal 0002 fixed `base_attack` for `bBaseAtk` and `attack` for `bAtk`. This draft uses `attack` and `weapon_attack` instead, so that `attack`, `attack_percent` (`bAtkRate`), `weapon_attack`, and `weapon_attack_percent` (`bWeaponAtkRate`) line up. Options:
   - a. `attack` and `weapon_attack` (this draft).
   - b. Keep `base_attack` and `attack`, as in 0002.
2. **Physical made explicit.** `bAddRace` and `bAddClass` add physical damage only. Magic has its own types. The draft names them `physical_damage_against_race` and `physical_damage_against_class`, which changes two fixed names. Options:
   - a. `physical_` prefix (this draft). No prefix then always means every damage kind.
   - b. Keep `damage_against_race`, and document that damage dealt with no prefix means physical. Damage taken with no prefix would still mean every kind.
3. **Sign of reductions.** rAthena's `bSubSize,Size_All,15` means 15% less damage taken. The draft flips it: `damage_taken_from_size` with `-15`, as in the 0002 example. Cast time, delay, and SP cost already use "negative means less" in rAthena, so one rule covers all. Options:
   - a. Flip (this draft). The converter negates 22 types.
   - b. Keep rAthena's sign, with names that say "reduction": `damage_reduction_from_size`.
4. **The `_percent` suffix.** The draft adds `_percent` only to percents of flat quantities, so damage bonuses have no suffix. This renames `damage_taken_from_size_percent` to `damage_taken_from_size`. Options:
   - a. This draft's rule.
   - b. `_percent` on every percent value. Names become long: `physical_damage_against_race_percent`.
5. **Chance units.** The draft takes every chance as a percent with decimals (0.01 steps). rAthena mixes percent, per mille, and per 10,000. Options:
   - a. Percent with decimals (this draft).
   - b. One integer unit, per 10,000, for all chances.
   - c. rAthena's unit per bonus.
6. **Trigger flags.** The draft moves `bf`, `atf`, and the autospell `i` into an options table (`range`, `kinds`, `from`, `target`, `duration`, `random_level`). Options:
   - a. Options table (this draft).
   - b. A separate name per combination, such as `chance_to_cast_on_melee`.
   - c. Positional flag constants, as in rAthena.
7. **Per-skill and per-item variants.** The draft gives them their own names (`skill_variable_cast_time_percent`, `hp_recovery_from_item`). Options:
   - a. Separate names (this draft). Each name has one argument list.
   - b. One name with an optional first argument, chosen by argument count.
8. **Non-stacking types.** Five rAthena types apply only the highest value (`bSpeedRate`, `bDoubleRate`, `bSplashRange`, `bPerfectHitRate`, `bGetZenyNum`). Options:
   - a. Keep them with `_highest_only` (this draft).
   - b. Another suffix, such as `_best`.
   - c. Drop them and convert to the stacking form. This changes the result when two items are worn.
9. **Trait stat names.** Draft: `power`, `stamina`, `wisdom`, `spell`, `concentration`, `creative`, `power_attack`, `spell_magic_attack`, `resistance`, `magic_resistance`, `healing_plus`, `critical_damage_rate`. Are these the words players know? The other option is the client's letters (`pow`, `p_attack`), which rule 1 forbids.
10. **Monster race (`RC2_...`).** rAthena calls these "monster race", but they are named groups such as Goblin, Orc, and Bio Lab. The draft calls them `monster_group` and adds `wa.monster_groups`. The other option is `monster_family`.
11. **Hard names.** Each has a weaker alternative:
    - `status_defense` for `bDef2` (or `soft_defense`, or `vitality_defense`).
    - `misc` for rAthena's third damage kind (or `other`, or `trap`).
    - `defense_as_damage_against_race` for `bDefRatioAtkRace` (or `pierce_defense_against_race`).
    - `chance_to_remove_hp` for `bHPVanishRate`, and `chance_to_deal_max_hp_percent_against_race` for `bHPVanishRaceRate`.
    - `always_endure` for `bNoWalkDelay` (or `no_flinch`).
    - `see_hidden` for `bIntravision`.
12. **Unused types.** 48 kept types have no use in renewal data. Options:
    - a. Keep them (this draft).
    - b. Drop them until a mod asks. This makes the converter fail on pre-renewal and custom items that use them.
13. **`against` and `from`.** The draft uses `against` for your actions and `from` for what a monster does to you or gives you, so `experience_from_race` and `damage_taken_from_race`, but `hp_per_normal_attack_against_race`. Is this clear enough, or should every target use `against`?
14. **API additions.** This draft needs:
    - a. A new item event, `"skill_used"`, for `autobonus3`.
    - b. Event fields `is_physical`, `is_misc`, `is_melee`, `is_ranged`, and `is_skill`. This renames 0002's `hit.is_weapon` to `is_physical`, to match the bonus names.
    - c. `player:add_timed_bonuses(key, ms, bonuses, options)` for `bonus_script`.
    - d. Possibly a `restart` option on `timers.with_bonuses`.
    - e. `wa.weapon_types`, `wa.monster_groups`, and item group lookups.
15. **Argument order.** The draft keeps rAthena's order, so the chance is not always in the same place (`chance_to_cause_status(status, chance)`, but `chance_to_drain_hp(chance, percent)`). Options:
    - a. rAthena's order (this draft). The conversion is a copy.
    - b. A fixed order, such as "chance last before options". The converter reorders.
