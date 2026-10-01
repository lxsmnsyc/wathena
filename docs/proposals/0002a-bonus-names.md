# 0002a: Item Bonus Names

- Status: accepted.
- Part of [0002: Lua Scripting API](0002-lua-api.md).

## Purpose

Item scripts add bonuses by name, such as `bonuses.add("luk", 2)`. This document gives a name to every rAthena bonus type, and defines the `"damage"` and `"damage_taken"` bonuses and the `"damage"` event. The converter and the generated API reference use the same table.

Sources:

- `doc/item_bonus.txt` for the documented forms.
- `src/map/pc.cpp` (`pc_bonus` to `pc_bonus5`) for the exact behavior, units, and signs.
- `src/map/script_constants.hpp` for the full list of `b...` constants.
- `db/re/item_db_*.yml` for the use counts. A count is the number of times the form appears in script text, including bonuses inside `autobonus` and combo scripts. rAthena matches bonus names without case, so `bHealpower2` counts as `bHealPower2`.

Counts:

- rAthena exports 242 `b...` constants. One of them, `bFame`, is not a bonus. That leaves 241 bonus types.
- 33 types become the `"damage"` bonus (17 types) or the `"damage_taken"` bonus (16 types).
- 8 types have no wAthena name of their own. The converter rewrites them into other names.
- The other 200 types each map to a name of their own. Some share a name, with the `stacking` option or a different form.
- 54 of the 242 constants have no use in renewal item data. The renewal data has 60,805 bonus calls in total.

## Naming rules

1. **Stats use their familiar abbreviations.** Everywhere in the API (bonus names, player fields, examples), these stats keep the short form players know from the status window:

   | Group | Names |
   |---|---|
   | Base stats | `str`, `agi`, `vit`, `int`, `dex`, `luk` |
   | Status window | `atk`, `matk`, `def`, `mdef`, `hit`, `flee`, `crit`, `aspd`, `hp`, `sp`, `ap` |
   | Trait stats | `pow`, `sta`, `wis`, `spl`, `con`, `crt`, `patk`, `smatk`, `res`, `mres`, `hplus`, `crate` |

   - They appear alone (`"luk"`, `player.str`) or inside longer names (`"weapon_atk"`, `"aspd_percent"`, `player.base_str`).
   - Every other word is written in full: `perfect_dodge`, `move_speed_percent`, `attack_range`, `zeny`.
   - Every name in this document is a valid Lua identifier, and none is a reserved word. `int`, `def`, `con`, and `res` are not reserved, so `player.int` and `bonuses.add("int", 1)` both work.
2. **One name per quantity.** The `bonus2` to `bonus5` forms of one rAthena type share one name. Per-skill and per-item variants have their own names (`skill_cooldown`, `hp_recovery_from_item`).
3. **Damage dealt and damage taken by percent are two bonuses**, `"damage"` and `"damage_taken"`. An options table says which hits they apply to. See "Damage and damage taken".
4. **`_percent` marks a percent of a normally flat quantity.**
   - It is used for stats, HP, SP, attack, defense, hit, flee, crit, speed, cast time, delay, and SP cost. So `max_hp` adds points and `max_hp_percent` adds a percent.
   - Damage, healing, experience, and drop bonuses always scale a result. They never take the suffix, and their values are always percents.
   - The suffix follows the quantity it qualifies: `ignore_def_percent_against_race`.
5. **Chances are whole numbers per 10,000.** 10000 is 100%, and 1 is 0.01%. rAthena uses three units, so each row gives the rAthena unit and the conversion:
   - "per 10,000": the converter copies the value.
   - "per mille" (1000 is 100%): the converter multiplies by 10.
   - "percent": the converter multiplies by 100.
6. **Trigger flags go in the options table.** rAthena's `bf`, `atf`, and autospell `i` flags become fields of an optional last argument. See "Options table".
7. **Arguments have a fixed order.** The converter reorders rAthena's arguments to fit:
   1. What the bonus applies to or acts with: records and constants such as a status, skill, item, item group, race, class, element, size, monster, monster group, or weapon type.
   2. The chance, if the bonus has one.
   3. The amount.
   4. Other numbers, such as a skill level, a duration, an interval, or a DEF value, in rAthena's order.
   5. The options table.
8. **The value is the change to the named quantity.** A positive value raises the quantity, and a negative value lowers it. `"damage_taken"` with `-15` means 15% less damage taken. This flips the sign of rAthena's reduction bonuses. The "Sign" column marks the 22 flipped types, and the converter negates them.
9. **Non-stacking bonuses use the `stacking` option.** See "Stacking".
10. **Other conventions.**
    - Procs are `chance_to_<effect>`, such as `chance_to_cause_status`. A chance that is a property of an attack roll is `<thing>_chance`, such as `double_attack_chance`.
    - In names that stay separate, `_against_<kind>` is used when the bonus changes the wearer's actions on that target, and `_from_<kind>` when it changes what that kind gives the wearer (experience, drops, rewards for a kill).
    - Times are milliseconds. Distances are cells. Weight is in tenths, as in wAthena's item weights (a Red Potion weighs 70).
    - Flag bonuses take no value and are named after the state they give: `bonuses.add("no_knockback")`.

## Options table

The last argument of `bonuses.add` can be a table. A bonus accepts only the fields its row lists. An unknown field, or a field the bonus does not accept, raises an error in `bonuses.add`.

| Field | Values | Used by | rAthena |
|---|---|---|---|
| `kind` | `"physical"`, `"magic"`, or `"misc"`. Omitted means every kind. | `damage`, `damage_taken`, procs | `BF_WEAPON`, `BF_MAGIC`, `BF_MISC`, `ATF_WEAPON`, `ATF_MAGIC`, `ATF_MISC` |
| `range` | `"melee"` or `"ranged"`. Omitted means both. On `damage` and `damage_taken` it needs `kind = "physical"`. | `damage`, `damage_taken`, procs | `BF_SHORT`, `BF_LONG`, `ATF_SHORT`, `ATF_LONG` |
| `attack` | `"normal"` (normal attacks) or `"skill"`. Omitted means both. | `damage` and `damage_taken` (element and race groups), procs | `BF_NORMAL`, `BF_SKILL` |
| `element` | An element: the element of the hit. | `damage`, `damage_taken` | |
| `skill` | A skill: the skill of the hit. | `damage`, `damage_taken` | |
| `weapon_type` | A weapon type: the wearer's equipped weapon. | `damage` | |
| `against` | A table with one key: the target of the hit. | `damage` | |
| `from` | A table with one key: the source of the hit. | `damage_taken` | |
| `target` | `"self"` or `"enemy"`: who gets the effect. | procs | `ATF_SELF`, `ATF_TARGET`, autospell `i & 1` |
| `duration` | Milliseconds. | status procs | The `t` argument of `bonus4 bAddEff` and similar |
| `random_level` | `true` picks a level from 1 to the given level. | autospell procs | Autospell `i & 2` |
| `stacking` | `"highest"` or `"lowest"`. | Any bonus with an amount | See "Stacking" |

Keys of `against` and `from`:

| Key | Value | rAthena |
|---|---|---|
| `race` | A race from `wa.races`, including `races.all` | `RC_...` |
| `element` | An element from `wa.elements`, including `elements.all`. The other entity's own element. | `Ele_...` |
| `size` | A size from `wa.sizes`, including `sizes.all` | `Size_...` |
| `class` | A class from `wa.monster_classes`, including `classes.all` | `Class_...` |
| `monster_group` | A group from `wa.monster_groups` | `RC2_...` |
| `monster` | A monster, from `monsters.id` | A monster ID in `bAddDamageClass` and similar |
| `job` | A job from `wa.jobs`, when the other side is a player | The same argument. rAthena reads the number as a job ID when the other side is a player. |

Examples:

```lua
bonuses.add("chance_to_cause_status", statuses.stun, 500, { range = "melee", duration = 3000 })
bonuses.add("chance_to_cast", skills.id "MG_FIREBOLT", 50, 3, { kind = "magic", random_level = true })
bonuses.add("damage", 10, { kind = "physical", against = { race = races.demi_human } })
bonuses.add("damage_taken", -15, { from = { size = sizes.all } })
```

## Damage and damage taken

`"damage"` changes damage the wearer deals by a percent. `"damage_taken"` changes damage the wearer takes by a percent. Both take the amount, then an options table.

### Groups

rAthena keeps one sum per bonus type, such as one sum per race for `bAddRace`. It applies each sum as its own factor, at a fixed step of the damage formula. wAthena keeps the same sums, so results match rAthena:

1. Each accepted combination of options belongs to one rAthena sum, listed in the two tables below.
2. Values in the same sum add up. A specific key and its `all` value add into the same sum, as in rAthena: `races.all` and `races.demi_human` both count against a Demi-Human.
3. Each sum is applied as its own factor, at the step where rAthena applies it.
4. A combination that is not in the tables raises an error in `bonuses.add`, since it has no rAthena sum. For example, `damage_taken` with `kind = "magic"` and `from = { race = ... }` fails.
5. rAthena has no sum for damage of every kind against a target. On `"damage"`, an omitted `kind` with an `against` key adds the value to the physical sum and to the magic sum.
6. Bonuses are kept as data. A computed value (a function) is allowed, as for any bonus.

`"damage"` combinations:

| Options | rAthena sum |
|---|---|
| `kind = "physical"`, `against = { race }` | `bAddRace` |
| `kind = "magic"`, `against = { race }` | `bMagicAddRace` |
| `kind = "physical"`, `against = { class }` | `bAddClass` |
| `kind = "magic"`, `against = { class }` | `bMagicAddClass` |
| `kind = "physical"`, `against = { size }` | `bAddSize` |
| `kind = "magic"`, `against = { size }` | `bMagicAddSize` |
| `kind = "physical"`, `against = { element }` | `bonus2 bAddEle` |
| `kind`, `range`, and `attack` from a `bf` flag, `against = { element }` | `bonus3 bAddEle` (the list with trigger flags) |
| `kind = "magic"`, `against = { element }` | `bMagicAddEle` |
| `kind = "physical"`, `against = { monster_group }` | `bAddRace2` |
| `kind = "magic"`, `against = { monster_group }` | `bMagicAddRace2` |
| `kind = "physical"`, `against = { monster }` or `{ job }` | `bAddDamageClass` |
| `kind = "magic"`, `against = { monster }` or `{ job }` | `bAddMagicDamageClass` |
| `kind = "magic"`, `element` | `bMagicAtkEle` |
| `kind = "physical"`, `range = "melee"` | `bShortAtkRate` |
| `kind = "physical"`, `range = "ranged"` | `bLongAtkRate` |
| `kind = "physical"`, `weapon_type` | `bWeaponDamageRate` |
| `skill` | `bSkillAtk` |

`"damage_taken"` combinations:

| Options | rAthena sum |
|---|---|
| `from = { race }` | `bonus2 bSubRace` |
| `kind`, `range`, and `attack` from a `bf` flag, `from = { race }` | `bonus3 bSubRace` (the list with trigger flags) |
| `from = { class }` | `bSubClass` |
| `from = { size }` | `bSubSize` |
| `kind = "physical"`, `from = { size }` | `bWeaponSubSize` |
| `kind = "magic"`, `from = { size }` | `bMagicSubSize` |
| `element` | `bonus2 bSubEle` |
| `kind`, `range`, and `attack` from a `bf` flag, `element` | `bonus3 bSubEle` (the list with trigger flags) |
| `kind = "physical"`, `from = { element }` | `bSubDefEle` |
| `kind = "magic"`, `from = { element }` | `bMagicSubDefEle` |
| `from = { monster_group }` | `bSubRace2` |
| `kind = "physical"`, `from = { monster }` or `{ job }` | `bAddDefMonster` |
| `kind = "magic"`, `from = { monster }` or `{ job }` | `bAddMDefMonster` |
| `kind = "physical"`, `range = "melee"` | `bNearAtkDef` |
| `kind = "physical"`, `range = "ranged"` | `bLongAtkDef` |
| `kind = "magic"` | `bMagicAtkDef` |
| `kind = "misc"` | `bMiscAtkDef` |
| `skill` | `bSubSkill` |

The `element` field is the element of the hit. The `element` key of `against` or `from` is the other entity's own element. So `bSubEle` (hits of element e) is `{ element = e }`, and `bSubDefEle` (attackers whose body has element e) is `{ from = { element = e } }`.

### Bonuses that stay separate

These change damage too, but they are not a percent of damage dealt or taken in one of the sums above:

| Names | Why they stay separate |
|---|---|
| `crit_damage`, `non_crit_damage`, `crit_damage_taken` | They depend on whether the hit is critical, which rAthena checks at the critical step, not in the sums. |
| `skill_ratio` | It adds percentage points to a skill's ratio. It is not a percent of the damage. |
| `physical_immunity`, `magic_immunity`, `misc_immunity` | They block part of a hit before every other reduction. The magic one also blocks heals and support skills. |
| `damage_taken_cap` | It caps one hit at a percent of max HP. |
| `melee_damage_reflected`, `ranged_damage_reflected`, `chance_to_reflect_magic`, `reflected_damage_taken` | They act on reflected damage, which has no kind or source in rAthena's sums. |
| `ignore_def_...`, `ignore_mdef_...`, `ignore_res_...`, `ignore_mres_...`, `def_as_damage_...` | They change the target's defense, not the damage. |
| `chance_to_set_def_against_race`, `chance_to_set_mdef_against_race` | Procs that change the target's defense. |
| `chance_to_drain_hp`, `chance_to_drain_sp`, `hp_per_normal_attack...`, `sp_per_normal_attack...` | They heal the wearer. |
| `chance_to_remove_hp`, `chance_to_remove_sp`, `chance_to_deal_max_hp_percent_against_race`, `chance_to_deal_max_sp_percent_against_race` | Procs that remove HP or SP by a percent of the target's pool, not of the damage. |
| `ignore_size_penalty` | A flag that turns off the weapon size table. |

## Stacking

`{ stacking = "highest" }` or `{ stacking = "lowest" }` stops a bonus from adding up with others of its kind. The rule:

1. Bonuses with the same name, the same subjects (the arguments before the chance or amount), the same options other than `stacking`, and the same `stacking` value form a set.
2. From the set, only one bonus counts: the one with the highest value for `"highest"`, or the lowest for `"lowest"`. The value compared is the chance if the bonus has one, or else the amount. "Highest" means the largest number, so of -10 and -20 it keeps -10.
3. That one value is then added to the sum of the bonuses with the same name, subjects, and options that have no `stacking` option.
4. Flag bonuses take no `stacking` option, since they have no value.

rAthena has seven types that keep only one value. The converter writes the option for them:

| rAthena | wAthena |
|---|---|
| `bonus bSpeedRate,n` | `move_speed_percent` with `{ stacking = "highest" }` |
| `bonus bDoubleRate,n` | `double_attack_chance` with `{ stacking = "highest" }` |
| `bonus bSplashRange,n` | `splash_range` with `{ stacking = "highest" }` |
| `bonus bPerfectHitRate,n` | `perfect_hit_chance` with `{ stacking = "highest" }` |
| `bonus2 bGetZenyNum,x,n` | `chance_to_gain_zeny_on_kill` with `{ stacking = "highest" }`. The kept bonus brings its maximum zeny, as in rAthena. |
| `bonus bFixedCastrate,n` | `fixed_cast_time_percent` with `{ stacking = "lowest" }`. rAthena keeps the lowest value, which is the largest reduction. |
| `bonus bAbsorbDmgMaxHP2,n` | `damage_taken_cap` with `{ stacking = "highest" }` |

Mods can use the option on any bonus with an amount.

## The damage event

One event, `"damage"`, replaces `"attack"` and `"hit_taken"`. The `"skill_used"` event of the draft is not added. An item context gets the event for every hit where the wearer is the source or the target.

```lua
events.on("damage", function(hit) ... end)
```

The handler gets one damage object:

| Field | Meaning |
|---|---|
| `source` | Handle of the attacker. |
| `target` | Handle of the one hit. |
| `kind` | `"physical"`, `"magic"`, or `"misc"`. |
| `range` | `"melee"` or `"ranged"`. |
| `skill` | The skill, or `nil` for a normal attack. |
| `element` | The element of the hit. |
| `is_critical` | `true` for a critical hit. |
| `amount` | The damage, after every reduction. It can be 0. |
| `wearer_is_source` | `true` when the wearer dealt the hit, `false` when the wearer took it. |

- The event fires once per hit, after its damage is computed. A miss does not fire it.
- The object is read-only, like every other API object. A handler cannot change the damage. That is the job of hooks.
- The event name `"damage"` and the bonus name `"damage"` are in separate lists, so they do not clash.

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
  events.on("damage", function(hit)
    if hit.wearer_is_source and (hit.kind == "physical" or hit.kind == "misc")
        and random.chance(10, 1000) then
      active = timers.with_bonuses(10000, function() bonuses.add("all_stats", 10) end,
        { restart = active })
      player:show_effect(effects.fire_splash_hit)
    end
  end)
end
```

| rAthena | Condition in the `"damage"` handler | Uses in renewal data |
|---|---|---|
| `autobonus` | `hit.wearer_is_source` | 422 |
| `autobonus2` | `not hit.wearer_is_source` | 52 |
| `autobonus3` with skill sk | `hit.wearer_is_source and hit.skill == sk` | 158 |

- The `bf` flag becomes checks on `hit.kind`, `hit.range`, and `hit.skill`. `BF_NORMAL` is `hit.skill == nil`.
- `timers.with_bonuses(ms, fn, { restart = handle })` cancels the given timer, if it still runs, before it starts the new one. This keeps rAthena's restart behavior.

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

- Each entry is a bonus name followed by its arguments, as in `bonuses.add`.
- The key (`"hp_recovery_food"`) replaces an older copy with the same key. This is rAthena's default. rAthena's "add a duplicate" flag (2048) is the option `stack = true`.
- `ends_on` lists the removal flags by name: `"death"`, `"dispell"`, `"clearance"`, `"logout"`, `"banishing_buster"`, `"refresh"`, `"lux_anima"`, `"magic_gear_change"`, `"damage"`.
- The options `icon` and `kind` (`"buff"` or `"debuff"`) cover rAthena's icon and type.

## Constants used in arguments

Arguments use the constant modules from proposal 0002 (`races`, `monster_classes`, `sizes`, `elements`, `statuses`, `jobs`) and record lookups (`items.id`, `skills.id`, `monsters.id`). This document adds:

- `wa.weapon_types` for rAthena's `W_...`, such as `weapon_types.bow`.
- `wa.monster_groups` for rAthena's `RC2_...` "monster race", such as `monster_groups.goblin`.
- `items.group "Potion"` for item groups, by the group's name in the item group data (rAthena's `IG_Potion`).

## Table

Columns:

- **rAthena**: the bonus command and its arguments. Several forms of one type are listed together.
- **wAthena**: the name, or the `bonuses.add` call for damage bonuses.
- **Arguments**: in wAthena's order, with units and the conversion.
- **Meaning**: one line.
- **Uses**: count in renewal item data, per form.
- **Sign**: "flipped" if the converter negates the value.

### Base stats

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bStr,n` | `str` | points | STR + n. | 507 |
| `bonus bAgi,n` | `agi` | points | AGI + n. | 442 |
| `bonus bVit,n` | `vit` | points | VIT + n. | 389 |
| `bonus bInt,n` | `int` | points | INT + n. | 780 |
| `bonus bDex,n` | `dex` | points | DEX + n. | 643 |
| `bonus bLuk,n` | `luk` | points | LUK + n. | 332 |
| `bonus bAllStats,n` | `all_stats` | points | All six base stats + n. | 525 |
| `bonus bAgiVit,n` | none | | AGI and VIT + n. The converter writes `agi` and `vit`. | 5 |
| `bonus bAgiDexStr,n` | none | | STR, AGI, and DEX + n. The converter writes three bonuses. | 0 |

### Trait stats (fourth jobs)

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bPow,n` | `pow` | points | POW + n. | 407 |
| `bonus bSta,n` | `sta` | points | STA + n. | 42 |
| `bonus bWis,n` | `wis` | points | WIS + n. | 33 |
| `bonus bSpl,n` | `spl` | points | SPL + n. | 306 |
| `bonus bCon,n` | `con` | points | CON + n. | 281 |
| `bonus bCrt,n` | `crt` | points | CRT + n. | 74 |
| `bonus bAllTraitStats,n` | `all_trait_stats` | points | All six trait stats + n. | 18 |
| `bonus bPatk,n` | `patk` | points | P.ATK + n. | 1203 |
| `bonus bPAtkRate,n` | `patk_percent` | percent | P.ATK + n%. | 0 |
| `bonus bSmatk,n` | `smatk` | points | S.MATK + n. | 752 |
| `bonus bSMatkRate,n` | `smatk_percent` | percent | S.MATK + n%. | 0 |
| `bonus bRes,n` | `res` | points | RES + n. | 217 |
| `bonus bResRate,n` | `res_percent` | percent | RES + n%. | 0 |
| `bonus bMres,n` | `mres` | points | MRES + n. | 151 |
| `bonus bMResRate,n` | `mres_percent` | percent | MRES + n%. | 0 |
| `bonus bHplus,n` | `hplus` | points | H.PLUS + n. | 21 |
| `bonus bHPlusRate,n` | `hplus_percent` | percent | H.PLUS + n%. | 0 |
| `bonus bCrate,n` | `crate` | points | C.RATE + n. | 125 |
| `bonus bCRateRate,n` | `crate_percent` | percent | C.RATE + n%. | 0 |

### HP, SP, and AP

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bMaxHP,n` | `max_hp` | points | Max HP + n. | 1565 |
| `bonus bMaxHPrate,n` | `max_hp_percent` | percent | Max HP + n%. | 1579 |
| `bonus bMaxSP,n` | `max_sp` | points | Max SP + n. | 581 |
| `bonus bMaxSPrate,n` | `max_sp_percent` | percent | Max SP + n%. | 1042 |
| `bonus bMaxAP,n` | `max_ap` | points | Max AP + n. | 0 |
| `bonus bMaxAPrate,n` | `max_ap_percent` | percent | Max AP + n%. | 0 |
| `bonus bHPrecovRate,n` | `natural_hp_recovery_percent` | percent | Natural HP recovery + n%. | 106 |
| `bonus bSPrecovRate,n` | `natural_sp_recovery_percent` | percent | Natural SP recovery + n%. | 138 |
| `bonus2 bHPRegenRate,n,t` | `hp_gain_over_time` | HP, interval in ms | Gain n HP every t ms. | 100 |
| `bonus2 bHPLossRate,n,t` | `hp_loss_over_time` | HP, interval in ms | Lose n HP every t ms. | 48 |
| `bonus2 bSPRegenRate,n,t` | `sp_gain_over_time` | SP, interval in ms | Gain n SP every t ms. | 132 |
| `bonus2 bSPLossRate,n,t` | `sp_loss_over_time` | SP, interval in ms | Lose n SP every t ms. | 27 |
| `bonus2 bRegenPercentHP,n,t` | `hp_gain_percent_over_time` | percent of max HP, interval in ms | Gain n% of max HP every t ms. | 4 |
| `bonus2 bRegenPercentSP,n,t` | `sp_gain_percent_over_time` | percent of max SP, interval in ms | Gain n% of max SP every t ms. | 6 |
| `bonus bNoRegen,x` | `no_hp_recovery`, `no_sp_recovery` | flag | Stops natural HP (x = 1) or SP (x = 2) recovery. The converter picks the name from x. | 17 |
| `bonus bUseSPrate,n` | `sp_cost_percent` | percent | SP cost of all skills + n%. | 269 |

### ATK and DEF

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bBaseAtk,n` | `atk` | points | Equipment ATK + n. This is the "ATK + n" of renewal items. | 2623 |
| `bonus bAtk,n` | `weapon_atk` | points | Weapon ATK + n. rAthena marks it unofficial. | 1 |
| `bonus bAtk2,n` | `refine_atk` | points | ATK that counts like refine ATK + n. | 1 |
| `bonus bAtkRate,n` | `atk_percent` | percent | ATK + n%. | 1619 |
| `bonus bWeaponAtkRate,n` | `weapon_atk_percent` | percent | Weapon ATK + n%. | 7 |
| `bonus2 bWeaponAtk,w,n` | `atk_with_weapon_type` | weapon type, points | ATK + n while a weapon of type w is equipped. | 0 |
| `bonus bMatk,n` | `matk` | points | MATK + n. | 1767 |
| `bonus bMatk2,n` | `hidden_matk` | points | MATK + n, not shown in the status window. | 0 |
| `bonus bMatkRate,n` | `matk_percent` | percent | MATK + n%. | 1495 |
| `bonus bWeaponMatkRate,n` | `weapon_matk_percent` | percent | Weapon MATK + n%. | 0 |
| `bonus bDef,n` | `def` | points | Equipment DEF + n. | 641 |
| `bonus bDefRate,n` | `def_percent` | percent | Equipment DEF + n%. | 9 |
| `bonus bDef2,n` | `status_def` | points | DEF from stats (VIT) + n. | 0 |
| `bonus bDef2Rate,n` | `status_def_percent` | percent | DEF from stats + n%. | 5 |
| `bonus bMdef,n` | `mdef` | points | Equipment MDEF + n. | 1346 |
| `bonus bMdefRate,n` | `mdef_percent` | percent | Equipment MDEF + n%. | 5 |
| `bonus bMdef2,n` | `status_mdef` | points | MDEF from stats (INT) + n. | 0 |
| `bonus bMdef2Rate,n` | `status_mdef_percent` | percent | MDEF from stats + n%. | 0 |
| `bonus bAtkRange,n` | `attack_range` | cells | Attack range + n. | 1 |

### HIT, FLEE, and CRIT

| rAthena | wAthena | Arguments | Meaning | Uses | Sign |
|---|---|---|---|---|---|
| `bonus bHit,n` | `hit` | points | HIT + n. | 531 | |
| `bonus bHitRate,n` | `hit_percent` | percent | HIT + n%. | 1 | |
| `bonus bFlee,n` | `flee` | points | FLEE + n. | 426 | |
| `bonus bFleeRate,n` | `flee_percent` | percent | FLEE + n%. | 1 | |
| `bonus bFlee2,n` | `perfect_dodge` | points | Perfect Dodge + n. | 155 | |
| `bonus bFlee2Rate,n` | `perfect_dodge_percent` | percent | Perfect Dodge + n%. | 0 | |
| `bonus bCritical,n` | `crit` | points | CRIT + n. | 757 | |
| `bonus bCriticalRate,n` | `crit_percent` | percent | CRIT + n%. | 4 | |
| `bonus bCriticalLong,n` | `ranged_crit` | points | CRIT + n for ranged normal attacks only. Not shown in the status window. | 14 | |
| `bonus2 bCriticalAddRace,r,n` | `crit_against_race` | race, points | CRIT + n against race r. | 41 | |
| `bonus bCriticalDef,n` | `crit_chance_taken` | percent | The chance of being hit by a critical changes by n%. A relative change, not a chance, so it stays a percent. | 0 | flipped |
| `bonus bPerfectHitRate,n` | `perfect_hit_chance` with `{ stacking = "highest" }` | chance (rAthena percent, × 100) | An attack always hits with this chance. | 1 | |
| `bonus bPerfectHitAddRate,n` | `perfect_hit_chance` | chance (rAthena percent, × 100) | More chance that an attack always hits. | 193 | |
| `bonus bDoubleRate,n` | `double_attack_chance` with `{ stacking = "highest" }` | chance (rAthena percent, × 100) | Chance of a double attack, with any weapon. | 0 | |
| `bonus bDoubleAddRate,n` | `double_attack_chance` | chance (rAthena percent, × 100) | More chance of a double attack, with any weapon. | 0 | |

### Speed and cast

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bSpeedRate,n` | `move_speed_percent` with `{ stacking = "highest" }` | percent | Move speed + n%. It does not stack with speed potions. | 73 |
| `bonus bSpeedAddRate,n` | `move_speed_percent` | percent | Move speed + n%. | 0 |
| `bonus bAspd,n` | `aspd` | points | ASPD + n. | 296 |
| `bonus bAspdRate,n` | `aspd_percent` | percent | ASPD + n%. | 1291 |
| `bonus bVariableCastrate,n`<br>`bonus2 bVariableCastrate,sk,n` | `variable_cast_time_percent`<br>`skill_variable_cast_time_percent` | percent<br>skill, percent | Variable cast time + n%, for all skills or for skill sk. | 1209<br>229 |
| `bonus bCastrate,n`<br>`bonus2 bCastrate,sk,n` | none | | Same as `bVariableCastrate` in renewal. The converter writes the variable cast names. | 1<br>0 |
| `bonus bFixedCastrate,n`<br>`bonus2 bFixedCastrate,sk,n` | `fixed_cast_time_percent` with `{ stacking = "lowest" }`<br>`skill_fixed_cast_time_percent` | percent<br>skill, percent | Fixed cast time + n%, for all skills or for skill sk. | 67<br>61 |
| `bonus bVariableCast,t` | `variable_cast_time` | ms | Variable cast time of all skills + t. | 1 |
| `bonus2 bSkillVariableCast,sk,t` | `skill_variable_cast_time` | skill, ms | Variable cast time of skill sk + t. | 30 |
| `bonus bFixedCast,t` | `fixed_cast_time` | ms | Fixed cast time of all skills + t. | 501 |
| `bonus2 bSkillFixedCast,sk,t` | `skill_fixed_cast_time` | skill, ms | Fixed cast time of skill sk + t. | 28 |
| `bonus bDelayrate,n` | `after_cast_delay_percent` | percent | After-cast delay of all skills + n%. | 1227 |
| `bonus2 bSkillDelay,sk,t` | `skill_after_cast_delay` | skill, ms | After-cast delay of skill sk + t. | 0 |
| `bonus bNoCastCancel` | `uninterruptible_cast` | flag | Casting is not interrupted by hits, except in guild wars. | 54 |
| `bonus bNoCastCancel2` | `uninterruptible_cast_everywhere` | flag | Casting is not interrupted by hits, also in guild wars. | 3 |
| `bonus bNoWalkDelay` | `always_endure` | flag | Hits never stop the wearer (permanent Endure). | 11 |

### Damage dealt

All values are percents of damage dealt.

| rAthena | wAthena | Meaning | Uses |
|---|---|---|---|
| `bonus2 bAddRace,r,x` | `bonuses.add("damage", x, { kind = "physical", against = { race = r } })` | Physical damage + x% against race r. | 1980 |
| `bonus2 bMagicAddRace,r,x` | `bonuses.add("damage", x, { kind = "magic", against = { race = r } })` | Magic damage + x% against race r. | 1046 |
| `bonus2 bAddClass,c,x` | `bonuses.add("damage", x, { kind = "physical", against = { class = c } })` | Physical damage + x% against class c. | 384 |
| `bonus2 bMagicAddClass,c,x` | `bonuses.add("damage", x, { kind = "magic", against = { class = c } })` | Magic damage + x% against class c. | 91 |
| `bonus2 bAddSize,s,x` | `bonuses.add("damage", x, { kind = "physical", against = { size = s } })` | Physical damage + x% against size s. | 867 |
| `bonus2 bMagicAddSize,s,x` | `bonuses.add("damage", x, { kind = "magic", against = { size = s } })` | Magic damage + x% against size s. | 487 |
| `bonus2 bAddEle,e,x`<br>`bonus3 bAddEle,e,x,bf` | `bonuses.add("damage", x, { kind = "physical", against = { element = e } })`<br>The same, with `kind`, `range`, and `attack` from bf | Physical damage + x% against targets of element e. | 1154<br>0 |
| `bonus2 bMagicAddEle,e,x` | `bonuses.add("damage", x, { kind = "magic", against = { element = e } })` | Magic damage + x% against targets of element e. | 866 |
| `bonus2 bAddRace2,mr,x` | `bonuses.add("damage", x, { kind = "physical", against = { monster_group = mr } })` | Physical damage + x% against monster group mr. | 59 |
| `bonus2 bMagicAddRace2,mr,x` | `bonuses.add("damage", x, { kind = "magic", against = { monster_group = mr } })` | Magic damage + x% against monster group mr. | 46 |
| `bonus2 bAddDamageClass,mid,x` | `bonuses.add("damage", x, { kind = "physical", against = { monster = mid } })` | Physical damage + x% against monster mid. For a job, `against = { job = ... }`. | 60 |
| `bonus2 bAddMagicDamageClass,mid,x` | `bonuses.add("damage", x, { kind = "magic", against = { monster = mid } })` | Magic damage + x% against monster mid. | 24 |
| `bonus2 bMagicAtkEle,e,x` | `bonuses.add("damage", x, { kind = "magic", element = e })` | Damage of the wearer's element e magic + x%. | 2531 |
| `bonus bShortAtkRate,n` | `bonuses.add("damage", n, { kind = "physical", range = "melee" })` | Melee physical damage + n%. | 951 |
| `bonus bLongAtkRate,n` | `bonuses.add("damage", n, { kind = "physical", range = "ranged" })` | Ranged physical damage + n%. | 1637 |
| `bonus2 bWeaponDamageRate,w,n` | `bonuses.add("damage", n, { kind = "physical", weapon_type = w })` | Physical damage + n% while a weapon of type w is equipped. | 20 |
| `bonus2 bSkillAtk,sk,n` | `bonuses.add("damage", n, { skill = sk })` | Damage of skill sk + n%. | 7893 |
| `bonus bCritAtkRate,n` | `crit_damage` (percent) | Critical damage + n%. | 732 |
| `bonus bNonCritAtkRate,n` | `non_crit_damage` (percent) | Non-critical damage + n%. | 79 |
| `bonus bSkillRatio,n` | `skill_ratio` (percentage points) | Adds n to the damage ratio of every attack and skill. | 0 |
| `bonus bNoSizeFix` | `ignore_size_penalty` (flag) | Weapon size penalties do not apply. | 29 |

### Ignoring and changing defense

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bIgnoreDefRace,r` | `ignore_def_against_race` | race | Ignore DEF of race r. | 47 |
| `bonus bIgnoreDefEle,e` | `ignore_def_against_element` | element | Ignore DEF of element e. | 0 |
| `bonus bIgnoreDefClass,c` | `ignore_def_against_class` | class | Ignore DEF of class c. | 20 |
| `bonus bIgnoreMdefRace,r` | `ignore_mdef_against_race` | race | Ignore MDEF of race r. | 1 |
| `bonus bIgnoreMdefEle,e` | `ignore_mdef_against_element` | element | Ignore MDEF of element e. | 0 |
| `bonus2 bIgnoreDefRaceRate,r,n` | `ignore_def_percent_against_race` | race, percent | Ignore n% of DEF of race r. | 656 |
| `bonus2 bIgnoreDefClassRate,c,n` | `ignore_def_percent_against_class` | class, percent | Ignore n% of DEF of class c. | 32 |
| `bonus2 bIgnoreMdefRaceRate,r,n` | `ignore_mdef_percent_against_race` | race, percent | Ignore n% of MDEF of race r. | 308 |
| `bonus2 bIgnoreMdefClassRate,c,n` | `ignore_mdef_percent_against_class` | class, percent | Ignore n% of MDEF of class c. | 58 |
| `bonus2 bIgnoreMdefRace2Rate,mr,n` | `ignore_mdef_percent_against_monster_group` | monster group, percent | Ignore n% of MDEF of monster group mr. | 1 |
| `bonus2 bIgnoreResRaceRate,r,n` | `ignore_res_percent_against_race` | race, percent | Ignore n% of RES of race r. | 18 |
| `bonus2 bIgnoreMResRaceRate,r,n` | `ignore_mres_percent_against_race` | race, percent | Ignore n% of MRES of race r. | 18 |
| `bonus bDefRatioAtkRace,r` | `def_as_damage_against_race` | race | Against race r, ignore DEF and deal more damage the higher the DEF. | 0 |
| `bonus bDefRatioAtkEle,e` | `def_as_damage_against_element` | element | The same, against element e. | 0 |
| `bonus bDefRatioAtkClass,c` | `def_as_damage_against_class` | class | The same, against class c. | 14 |
| `bonus4 bSetDefRace,r,n,t,y` | `chance_to_set_def_against_race` | race, chance (rAthena per 10,000, copied), DEF value, duration in ms | Normal attacks have this chance to set the DEF of race r to y for t ms. | 5 |
| `bonus4 bSetMDefRace,r,n,t,y` | `chance_to_set_mdef_against_race` | race, chance (rAthena per 10,000, copied), MDEF value, duration in ms | The same for MDEF. | 5 |

### Damage taken

All values are percents of damage taken. A negative value means less damage.

| rAthena | wAthena | Meaning | Uses | Sign |
|---|---|---|---|---|
| `bonus2 bSubRace,r,x`<br>`bonus3 bSubRace,r,x,bf` | `bonuses.add("damage_taken", -x, { from = { race = r } })`<br>The same, with `kind`, `range`, and `attack` from bf | Damage taken from race r changes. | 1631<br>1 | flipped |
| `bonus2 bSubClass,c,x` | `bonuses.add("damage_taken", -x, { from = { class = c } })` | Damage taken from class c changes. | 281 | flipped |
| `bonus2 bSubSize,s,x` | `bonuses.add("damage_taken", -x, { from = { size = s } })` | Damage taken from size s changes. | 147 | flipped |
| `bonus2 bWeaponSubSize,s,x` | `bonuses.add("damage_taken", -x, { kind = "physical", from = { size = s } })` | Physical damage taken from size s changes. | 0 | flipped |
| `bonus2 bMagicSubSize,s,x` | `bonuses.add("damage_taken", -x, { kind = "magic", from = { size = s } })` | Magic damage taken from size s changes. | 2 | flipped |
| `bonus2 bSubEle,e,x`<br>`bonus3 bSubEle,e,x,bf` | `bonuses.add("damage_taken", -x, { element = e })`<br>The same, with `kind`, `range`, and `attack` from bf | Damage taken from hits of element e changes. | 931<br>12 | flipped |
| `bonus2 bSubDefEle,e,x` | `bonuses.add("damage_taken", -x, { kind = "physical", from = { element = e } })` | Physical damage taken from attackers whose own element is e changes. | 8 | flipped |
| `bonus2 bMagicSubDefEle,e,x` | `bonuses.add("damage_taken", -x, { kind = "magic", from = { element = e } })` | The same for magic damage. | 3 | flipped |
| `bonus2 bSubRace2,mr,x` | `bonuses.add("damage_taken", -x, { from = { monster_group = mr } })` | Damage taken from monster group mr changes. | 30 | flipped |
| `bonus2 bAddDefMonster,mid,x` | `bonuses.add("damage_taken", -x, { kind = "physical", from = { monster = mid } })` | Physical damage taken from monster mid changes. For a job, `from = { job = ... }`. | 33 | flipped |
| `bonus2 bAddMdefMonster,mid,x` | `bonuses.add("damage_taken", -x, { kind = "magic", from = { monster = mid } })` | Magic damage taken from monster mid changes. | 3 | flipped |
| `bonus bNearAtkDef,n` | `bonuses.add("damage_taken", -n, { kind = "physical", range = "melee" })` | Melee physical damage taken changes. | 11 | flipped |
| `bonus bLongAtkDef,n` | `bonuses.add("damage_taken", -n, { kind = "physical", range = "ranged" })` | Ranged physical damage taken changes. | 83 | flipped |
| `bonus bMagicAtkDef,n` | `bonuses.add("damage_taken", -n, { kind = "magic" })` | Magic damage taken changes. | 2 | flipped |
| `bonus bMiscAtkDef,n` | `bonuses.add("damage_taken", -n, { kind = "misc" })` | Misc damage taken (traps, falcon) changes. | 0 | flipped |
| `bonus2 bSubSkill,sk,n` | `bonuses.add("damage_taken", -n, { skill = sk })` | Damage taken from skill sk changes. | 24 | flipped |
| `bonus bCritDefRate,n` | `crit_damage_taken` (percent) | Critical damage taken changes by n%. | 0 | flipped |
| `bonus bNoWeaponDamage,n` | `physical_immunity` (percent) | Blocks n% of physical damage, before other reductions. | 0 | |
| `bonus bNoMagicDamage,n` | `magic_immunity` (percent) | Blocks n% of magic. This also blocks heals and support skills. | 3 | |
| `bonus bNoMiscDamage,n` | `misc_immunity` (percent) | Blocks n% of misc damage. | 0 | |
| `bonus bAbsorbDmgMaxHP2,n` | `damage_taken_cap` with `{ stacking = "highest" }` (percent of max HP) | A hit above n% of max HP is reduced to n% of max HP. | 1 | |
| `bonus bAbsorbDmgMaxHP,n` | none | Old rAthena form of the cap. Not official. The converter writes `damage_taken_cap`. | 0 | |
| `bonus bShortWeaponDamageReturn,n` | `melee_damage_reflected` (percent) | Reflects n% of melee damage taken. | 45 | |
| `bonus bLongWeaponDamageReturn,n` | `ranged_damage_reflected` (percent) | Reflects n% of ranged damage taken. | 0 | |
| `bonus bMagicDamageReturn,n` | `chance_to_reflect_magic` (chance; rAthena percent, × 100) | Chance to reflect a targeted spell back to its caster. | 11 | |
| `bonus bReduceDamageReturn,n` | `reflected_damage_taken` (percent) | Damage taken from reflection changes by n%. | 39 | flipped |

### Elements

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bAtkEle,e` | `attack_element` | element | Attacks have element e. | 266 |
| `bonus bDefEle,e` | `defense_element` | element | The wearer's body has element e. | 60 |

### Status effects

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus2 bAddEff,eff,n`<br>`bonus3 bAddEff,eff,n,atf`<br>`bonus4 bAddEff,eff,n,atf,t` | `chance_to_cause_status` | status, chance (rAthena per 10,000, copied), [options: `target`, `range`, `kind`, `duration`] | Attacks have this chance to cause status eff. | 163<br>50<br>2 |
| `bonus2 bAddEff2,eff,n` | none | | Status eff on the wearer when attacking. The converter writes `chance_to_cause_status` with `{ target = "self" }`. | 25 |
| `bonus2 bAddEffWhenHit,eff,n`<br>`bonus3 bAddEffWhenHit,eff,n,atf`<br>`bonus4 bAddEffWhenHit,eff,n,atf,t` | `chance_to_cause_status_when_hit` | status, chance (rAthena per 10,000, copied), [options] | Being hit by physical damage has this chance to cause status eff on the attacker. | 61<br>23<br>0 |
| `bonus3 bAddEffOnSkill,sk,eff,n`<br>`bonus4 bAddEffOnSkill,sk,eff,n,atf`<br>`bonus5 bAddEffOnSkill,sk,eff,n,atf,t` | `chance_to_cause_status_on_skill` | skill, status, chance (rAthena per 10,000, copied), [options] | Using skill sk has this chance to cause status eff. | 24<br>0<br>0 |
| `bonus2 bResEff,eff,n` | `status_resistance` | status, chance (rAthena per 10,000, copied) | The chance to resist status eff rises by n. | 379 |
| `bonus2 bComaRace,r,n` | `chance_to_cause_coma_against_race` | race, chance (rAthena per 10,000, copied), [options: `attack`] | Any attack on race r has this chance to cause Coma. | 10 |
| `bonus2 bComaClass,c,n` | `chance_to_cause_coma_against_class` | class, chance (rAthena per 10,000, copied), [options: `attack`] | Any attack on class c has this chance to cause Coma. | 3 |
| `bonus2 bWeaponComaRace,r,n` | none | | Coma on normal attacks only. The converter writes `chance_to_cause_coma_against_race` with `{ attack = "normal" }`. | 0 |
| `bonus2 bWeaponComaClass,c,n` | none | | The same for class: `chance_to_cause_coma_against_class` with `{ attack = "normal" }`. | 0 |
| `bonus2 bWeaponComaEle,e,n` | `chance_to_cause_coma_against_element` | element, chance (rAthena per 10,000, copied), [options: `attack`] | Attacks on element e have this chance to cause Coma. The converter adds `{ attack = "normal" }`. | 0 |
| `bonus3 bStateNoRecoverRace,r,x,t` | `chance_to_block_healing_against_race` | race, chance (rAthena per 10,000, copied), duration in ms | Normal attacks on race r have this chance to stop the target's healing for t ms. | 2 |

### Skills

| rAthena | wAthena | Arguments | Meaning | Uses | Sign |
|---|---|---|---|---|---|
| `skill sk,lv` (a command) | `skill` | skill, level | Grants skill sk at level lv while equipped. | 633 | |
| `bonus2 bSkillUseSP,sk,n` | `skill_sp_cost` | skill, SP | SP cost of skill sk changes by n. | 269 | flipped |
| `bonus2 bSkillUseSPrate,sk,n` | `skill_sp_cost_percent` | skill, percent | SP cost of skill sk changes by n%. | 75 | flipped |
| `bonus2 bSkillCooldown,sk,t` | `skill_cooldown` | skill, ms | Cooldown of skill sk + t. | 618 | |
| `bonus2 bAddSkillBlow,sk,n` | `skill_knockback` | skill, cells | Skill sk knocks the target back n cells. | 8 | |
| `bonus bNoGemStone` | `no_gemstone_cost` | flag | Skills need no gemstones. Hocus Pocus and Ganbantein keep their cost. | 16 | |
| `bonus bNoMadoFuel` | `no_magic_gear_fuel_cost` | flag | Skills need no Magic Gear Fuel. | 9 | |

Skill damage and damage taken from a skill are `"damage"` and `"damage_taken"` with the `skill` option. Per-skill cast time and delay are in "Speed and cast". Per-skill healing is in "Healing and rewards for a kill".

### Autospell and other procs

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus3 bAutoSpell,sk,y,n`<br>`bonus4 bAutoSpell,sk,y,n,i`<br>`bonus5 bAutoSpell,sk,y,n,bf,i` | `chance_to_cast` | skill, chance (rAthena per mille, × 10), level, [options] | Attacks have this chance to cast skill sk at level y. | 375<br>25<br>90 |
| `bonus3 bAutoSpellWhenHit,sk,y,n`<br>`bonus4 bAutoSpellWhenHit,sk,y,n,i`<br>`bonus5 bAutoSpellWhenHit,sk,y,n,bf,i` | `chance_to_cast_when_hit` | skill, chance (rAthena per mille, × 10), level, [options] | Being hit by a direct attack has this chance to cast skill sk at level y. | 145<br>35<br>60 |
| `bonus4 bAutoSpellOnSkill,sk,x,y,n`<br>`bonus5 bAutoSpellOnSkill,sk,x,y,n,i` | `chance_to_cast_on_skill` | trigger skill, skill, chance (rAthena per mille, × 10), level, [options] | Using skill sk has this chance to cast skill x at level y. | 71<br>10 |
| `bonus2 bHPDrainRate,x,n` | `chance_to_drain_hp` | chance (rAthena per mille, × 10), percent of damage | Physical attacks have this chance to heal n% of the damage dealt as HP. | 92 |
| `bonus2 bSPDrainRate,x,n` | `chance_to_drain_sp` | chance (rAthena per mille, × 10), percent of damage | The same, as SP. | 66 |
| `bonus bHPDrainValue,n` | `hp_per_normal_attack` | HP | Each normal attack heals n HP. | 1 |
| `bonus2 bHPDrainValueRace,r,n` | `hp_per_normal_attack_against_race` | race, HP | The same, against race r. | 0 |
| `bonus2 bHPDrainValueClass,c,n` | `hp_per_normal_attack_against_class` | class, HP | The same, against class c. | 0 |
| `bonus bSPDrainValue,n` | `sp_per_normal_attack` | SP | Each normal attack restores n SP. | 39 |
| `bonus2 bSPDrainValueRace,r,n` | `sp_per_normal_attack_against_race` | race, SP | The same, against race r. | 5 |
| `bonus2 bSPDrainValueClass,c,n` | `sp_per_normal_attack_against_class` | class, SP | The same, against class c. | 0 |
| `bonus2 bHPVanishRate,x,n`<br>`bonus3 bHPVanishRate,x,n,bf` | `chance_to_remove_hp` | chance (rAthena per mille, × 10), percent of the target's HP, [options] | Attacks have this chance to remove n% of the target's current HP. The two-argument form is `{ attack = "normal" }`. | 1<br>0 |
| `bonus2 bSPVanishRate,x,n`<br>`bonus3 bSPVanishRate,x,n,bf` | `chance_to_remove_sp` | chance (rAthena per mille, × 10), percent of the target's SP, [options] | The same for SP. | 1<br>5 |
| `bonus3 bHPVanishRaceRate,r,x,n` | `chance_to_deal_max_hp_percent_against_race` | race, chance (rAthena per mille, × 10), percent of max HP | Against race r, a physical hit has this chance to deal n% of the target's max HP instead of its damage. | 2 |
| `bonus3 bSPVanishRaceRate,r,x,n` | `chance_to_deal_max_sp_percent_against_race` | race, chance (rAthena per mille, × 10), percent of max SP | The same, as SP damage. | 5 |
| `bonus bBreakWeaponRate,n` | `chance_to_break_weapon` | chance (rAthena per 10,000, copied) | Attacks have this chance to break the target's weapon. | 12 |
| `bonus bBreakArmorRate,n` | `chance_to_break_armor` | chance (rAthena per 10,000, copied) | Attacks have this chance to break the target's armor. | 17 |
| `bonus bClassChange,n` | `chance_to_transform_monster` | chance (rAthena per 10,000, copied) | Physical attacks have this chance to turn the monster into a random other monster. | 2 |

### Drops, zeny, and experience

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus2 bExpAddRace,r,x` | `experience_from_race` | race, percent | Experience from race r + x%. | 295 |
| `bonus2 bExpAddClass,c,x` | `experience_from_class` | class, percent | Experience from class c + x%. | 12 |
| `bonus2 bDropAddRace,r,x` | `drop_rate_from_race` | race, percent | Drop rates of race r + x%. | 55 |
| `bonus2 bDropAddClass,c,x` | `drop_rate_from_class` | class, percent | Drop rates of class c + x%. | 2 |
| `bonus2 bAddMonsterDropItem,iid,n`<br>`bonus3 bAddMonsterDropItem,iid,r,n` | `extra_drop_from_race` | item, race, chance (rAthena per 10,000, copied) | Killing a monster of race r has this chance to drop item iid. The two-argument form uses `races.all`. A negative chance means -n × level / 10 + 1, as in rAthena. | 215<br>77 |
| `bonus3 bAddClassDropItem,iid,c,n` | `extra_drop_from_class` | item, class, chance (rAthena per 10,000, copied) | The same, for class c. | 0 |
| `bonus3 bAddMonsterIdDropItem,iid,mid,n` | `extra_drop_from_monster` | item, monster, chance (rAthena per 10,000, copied) | The same, for monster mid. | 62 |
| `bonus2 bAddMonsterDropItemGroup,ig,n`<br>`bonus3 bAddMonsterDropItemGroup,ig,r,n` | `extra_group_drop_from_race` | item group, race, chance (rAthena per 10,000, copied) | Killing a monster of race r has this chance to drop an item from group ig. | 8<br>0 |
| `bonus3 bAddClassDropItemGroup,ig,c,n` | `extra_group_drop_from_class` | item group, class, chance (rAthena per 10,000, copied) | The same, for class c. | 0 |
| `bonus2 bGetZenyNum,x,n` | `chance_to_gain_zeny_on_kill` with `{ stacking = "highest" }` | chance (rAthena percent, × 100), maximum zeny | A kill has this chance to give 1 to x zeny. A negative x means x × monster level. | 3 |
| `bonus2 bAddGetZenyNum,x,n` | `chance_to_gain_zeny_on_kill` | chance (rAthena percent, × 100), maximum zeny | The same, stacking. | 0 |
| `bonus bAddStealRate,n` | `steal_chance` | chance (rAthena per 10,000, copied) | Steal success chance + n. | 0 |

### Healing and rewards for a kill

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bHealPower,n` | `healing_done` | percent | Healing from the wearer's heal skills + n%. | 359 |
| `bonus bHealPower2,n` | `healing_received` | percent | Healing the wearer receives from skills + n%. | 80 |
| `bonus2 bSkillHeal,sk,n` | `skill_healing_done` | skill, percent | Healing from the wearer's skill sk + n%. | 7 |
| `bonus2 bSkillHeal2,sk,n` | `skill_healing_received` | skill, percent | Healing the wearer receives from skill sk + n%. | 2 |
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

| rAthena | wAthena | Arguments | Meaning | Uses | Sign |
|---|---|---|---|---|---|
| `bonus bUnbreakableWeapon` | `unbreakable_weapon` | flag | The weapon cannot break. | 597 | |
| `bonus bUnbreakableArmor` | `unbreakable_armor` | flag | The armor cannot break. | 105 | |
| `bonus bUnbreakableHelm` | `unbreakable_helm` | flag | The helm cannot break. | 149 | |
| `bonus bUnbreakableShield` | `unbreakable_shield` | flag | The shield cannot break. | 21 | |
| `bonus bUnbreakableGarment` | `unbreakable_garment` | flag | The garment cannot break. | 17 | |
| `bonus bUnbreakableShoes` | `unbreakable_shoes` | flag | The shoes cannot break. | 11 | |
| `bonus bUnbreakable,n` | `equipment_break_chance` | percent | The chance that any equipped item breaks changes by n%. A relative change, not a chance, so it stays a percent. | 0 | flipped |
| `bonus bUnstripableWeapon` | `unstrippable_weapon` | flag | Strip skills cannot remove the weapon. | 0 | |
| `bonus bUnstripableArmor` | `unstrippable_armor` | flag | Strip skills cannot remove the armor. | 0 | |
| `bonus bUnstripableHelm` | `unstrippable_helm` | flag | Strip skills cannot remove the helm. | 0 | |
| `bonus bUnstripableShield` | `unstrippable_shield` | flag | Strip skills cannot remove the shield. | 0 | |
| `bonus bUnstripable` | none | | The documentation says "all equipment", but `pc_bonus` protects only the armor. The converter writes `unstrippable_armor`, which keeps rAthena's behavior. | 0 | |

### Other

| rAthena | wAthena | Arguments | Meaning | Uses |
|---|---|---|---|---|
| `bonus bSplashRange,n` | `splash_range` with `{ stacking = "highest" }` | cells | Normal attacks hit an area of radius n (1 is 3×3). | 52 |
| `bonus bSplashAddRange,n` | `splash_range` | cells | Splash radius + n. | 0 |
| `bonus bNoKnockback` | `no_knockback` | flag | Skills cannot knock the wearer back. | 52 |
| `bonus bIntravision` | `see_hidden` | flag | The wearer sees hidden and cloaked characters. | 12 |
| `bonus bPerfectHide` | `hidden_from_detectors` | flag | While hidden or cloaked, monsters that detect hidden characters do not see the wearer. | 0 |
| `bonus bAddMaxWeight,n` | `max_weight` | tenths of weight | Weight limit + n. | 0 |
| `bFame` | none | | Not a bonus. It is a value `readparam` reads, and `pc_bonus` rejects it. | 0 |

## Types without a wAthena name

The converter rewrites each one:

| rAthena | Converter writes | Reason |
|---|---|---|
| `bAgiVit`, `bAgiDexStr` | One bonus per stat | Shortcuts. |
| `bCastrate` (both forms) | `variable_cast_time_percent`, `skill_variable_cast_time_percent` | The same as `bVariableCastrate` in renewal. |
| `bAddEff2` | `chance_to_cause_status` with `{ target = "self" }` | The same as `bAddEff` with `ATF_SELF`. |
| `bWeaponComaRace`, `bWeaponComaClass` | The coma names with `{ attack = "normal" }` | Only the trigger differs. |
| `bAbsorbDmgMaxHP` | `damage_taken_cap` | rAthena calls it legacy and not official. Unused in renewal data. |
| `bUnstripable` | `unstrippable_armor` | Its code protects only the armor. |
| `bFame` | Nothing | Not a bonus. |

Types with no use in renewal data are kept. Pre-renewal data and other servers' items use some of them, and mods may want them.

## Review decisions

The draft's questions were answered in review:

1. Stats use the abbreviations of rule 1 everywhere in the API, including trait stats. So `bBaseAtk` is `"atk"` and `bAtk` is `"weapon_atk"`.
2. Damage dealt and taken by percent are the `"damage"` and `"damage_taken"` bonuses with an options table, summed in rAthena's groups. This replaces the draft's per-kind and per-target names.
3. Reductions flip their sign. A positive value raises the named quantity.
4. `_percent` marks only percents of normally flat quantities.
5. Chances are whole numbers per 10,000.
6. Trigger flags go in the options table.
7. Per-skill and per-item variants keep separate names.
8. Non-stacking types use the normal name with `{ stacking = "highest" }`. There is no suffix.
9. Trait stats use their abbreviations.
10. rAthena's "monster race" is `monster_group`, with `wa.monster_groups`.
11. The draft's hard names are accepted, with the abbreviations of rule 1: `status_def`, `misc`, `def_as_damage_against_race`, `chance_to_remove_hp`, `always_endure`, and `see_hidden`.
12. Unused types are kept.
13. The question of `against` and `from` in names is answered by the damage design: the damage object has `source` and `target`, and the options have `against` and `from`.
14. Approved API additions: the single `"damage"` event (it replaces `"attack"`, `"hit_taken"`, and the draft's `"skill_used"`, and its `kind` field replaces `is_weapon`), `player:add_timed_bonuses`, the `restart` option of `timers.with_bonuses`, `wa.weapon_types`, `wa.monster_groups`, and `items.group`.
15. Arguments have a fixed order (rule 7). The converter reorders rAthena's arguments.

## Open points

These follow from the decisions, but their details were not reviewed:

1. **`stacking = "lowest"`.** Review approved `"highest"`. rAthena's `bFixedCastrate` keeps the lowest value, so the converter also needs `"lowest"`.
2. **autobonus3 on skills without damage.** rAthena runs `autobonus3` when the skill is used (`skill_onskillusage`), also for skills that deal no damage, such as buffs. The `"damage"` event does not fire for those. Converted items that trigger on a support skill lose the trigger, unless another event covers skill use.
3. **Misses.** rAthena's autobonus runs from `skill_additional_effect`, after a hit. This document says `"damage"` does not fire on a miss. The parity tests should confirm it.
4. **Bonuses where rAthena keeps the last value.** `attack_element`, `defense_element`, `chance_to_set_def_against_race`, `chance_to_set_mdef_against_race`, `chance_to_block_healing_against_race`, and `chance_to_transform_monster` keep the last one set, so the result depends on equip order. Strategy section 5.3 forbids that. They need a fixed rule, such as the item in the lowest equip slot.
5. **Two percents that are not chances.** `crit_chance_taken` and `equipment_break_chance` change a chance by a relative percent. They stay percents, not per 10,000.
6. **The `job` key.** rAthena reads the monster ID of `bAddDamageClass` and similar as a job ID when the other side is a player. wAthena splits this into the `monster` and `job` keys. The converter writes `monster`, since renewal data uses monster IDs there.
