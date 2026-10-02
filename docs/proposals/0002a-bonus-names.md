# 0002a: Item Bonus Names

- Status: accepted.
- Part of [0002: Lua Scripting API](0002-lua-api.md).

## Purpose

Item scripts add bonuses by name, such as `bonuses.add("luk", 2)`. This document:

- gives a name to every rAthena bonus type;
- defines the options table that every bonus shares;
- defines the `"damage"` and `"skill_used"` item events.

The converter maps every rAthena bonus form to a `bonuses.add` call in this document. The generated API reference uses the same table.

Sources:

- `doc/item_bonus.txt` for the documented forms.
- `src/map/pc.cpp` (`pc_bonus` to `pc_bonus5`) for the exact behavior, units, and signs.
- `src/map/status.cpp` (`status_calc_pc`) for the order in which item scripts run.
- `src/map/script_constants.hpp` for the full list of `b...` constants.
- `db/re/item_db_*.yml` for the use counts. A count is the number of times the form appears in script text, including bonuses inside `autobonus` and combo scripts. rAthena matches bonus names without case, so `bHealpower2` counts as `bHealPower2`.

Counts:

- rAthena exports 242 `b...` constants. One of them, `bFame`, is not a bonus. That leaves 241 bonus types.
- wAthena has 127 bonus names, plus the `skill` bonus that grants a skill.
- 33 rAthena types map to `"damage"` (17) or `"damage_taken"` (16).
- 54 of the 242 constants have no use in renewal item data. The renewal data has 60,805 bonus calls in total.

## Naming rules

1. **Stats use their familiar abbreviations.** Everywhere in the API (bonus names, player fields, examples), these stats keep the short form players know from the status window:

   | Group | Names |
   |---|---|
   | Base stats | `str`, `agi`, `vit`, `int`, `dex`, `luk` |
   | Status window | `atk`, `matk`, `def`, `mdef`, `hit`, `flee`, `crit`, `aspd`, `hp`, `sp`, `ap` |
   | Trait stats | `pow`, `sta`, `wis`, `spl`, `con`, `crt`, `patk`, `smatk`, `res`, `mres`, `hplus`, `crate` |

   - They appear alone (`"luk"`, `player.str`) or inside longer names (`"weapon_atk"`, `player.base_str`).
   - Every other word is written in full: `perfect_dodge`, `move_speed`, `attack_range`, `zeny`.
   - Every name in this document is a valid Lua identifier, and none is a reserved word. `int`, `def`, `con`, and `res` are not reserved, so `player.int` and `bonuses.add("int", 1)` both work.
2. **One name per quantity.** Variants of one quantity share its name, and options say which variant is meant:
   - flat or percent: `{ percent = true }` (rule 4);
   - all skills or one skill: `{ skill = ... }`;
   - all items or one item: `{ item = ... }` or `{ item_group = ... }`;
   - all targets or one kind of target: `{ against = ... }` or `{ from = ... }`;
   - the trigger of a proc: `{ on = ... }`.

   A separate name is kept only where the meaning differs. The table says why in each case.
3. **Damage dealt and damage taken by percent are two bonuses**, `"damage"` and `"damage_taken"`. See "Damage and damage taken".
4. **Flat and percent share a name.** `{ percent = true }` makes the amount a percent of the quantity: `bonuses.add("max_hp", 10, { percent = true })`.
   - Each name allows one or both modes. The "Modes" column of the table lists them.
   - A name that rAthena has only as a percent requires `percent = true`. Leaving it out raises an error.
   - A name that rAthena has only as a flat value refuses `percent = true`.
   - Some bonuses always scale a result: damage, healing, experience, drop rate, drain, and reflection. Their amount is always a percent of that result, as the table says. They take no `percent` option.
5. **Chances are whole numbers per 10,000,** in the `chance` option. 10000 is 100%, and 1 is 0.01%. rAthena uses three units, so the table shows the conversion in each call:
   - per 10,000: `chance = n`;
   - per mille: `chance = n * 10`;
   - percent: `chance = n * 100`.
6. **Procs are named after their effect.** The chance and the trigger are options: `bonuses.add("cause_status", statuses.stun, { chance = 300, on = "hit_taken" })`. See "Procs".
7. **Arguments have a fixed order.** The converter reorders rAthena's arguments to fit:
   1. what the bonus produces or applies to, if the name needs it: a status for `cause_status`, a skill and level for `cast` and `skill`, an element for `attack_element`;
   2. the amount;
   3. other numbers, such as the interval of `hp_gain_over_time`;
   4. the options table.
8. **The value is the change to the named quantity.** A positive value raises the quantity, and a negative value lowers it. `"damage_taken"` with `-15` means 15% less damage taken. This flips the sign of rAthena's reduction bonuses. The calls in the table show the flip as `-n`, and the "Sign" column marks the 22 flipped types.
9. **Stacking is an option.** See "Stacking".
10. **Other conventions.**
    - Times are milliseconds. Distances are cells. Weight is in tenths, as in wAthena's item weights (a Red Potion weighs 70).
    - Flag bonuses take no amount and are named after the state they give: `bonuses.add("no_knockback")`.
    - An unknown option, an option the name does not accept, or a missing required option raises an error in `bonuses.add`.

## Options table

The last argument of `bonuses.add` can be a table. Each row of the table lists which options its name accepts.

| Option | Values | Meaning |
|---|---|---|
| `percent` | `true` | The amount is a percent of the quantity (rule 4). |
| `skill` | A skill | The skill the bonus applies to. On `"damage"` and `"damage_taken"` it is the skill of the hit. On a proc with `on = "skill_used"`, it is the skill that triggers the proc. |
| `item` | An item | The item the bonus applies to. |
| `item_group` | An item group | The item group the bonus applies to. |
| `kind` | `"physical"`, `"magic"`, or `"misc"` | The kind of hit. Omitted means every kind. rAthena's `BF_WEAPON`, `BF_MAGIC`, `BF_MISC`, and the `ATF_` equivalents. |
| `range` | `"melee"` or `"ranged"` | The range of a physical hit. Omitted means both. It needs `kind = "physical"`. rAthena's `BF_SHORT`, `BF_LONG`, `ATF_SHORT`, `ATF_LONG`. |
| `attack` | `"normal"` or `"skill"` | Normal attacks or skills. Omitted means both. rAthena's `BF_NORMAL`, `BF_SKILL`. |
| `element` | An element | The element of the hit. |
| `weapon_type` | A weapon type | The wearer's equipped weapon. |
| `against` | A table with one key | The target. See the keys below. |
| `from` | A table with one key | The source. See the keys below. |
| `chance` | Integer per 10,000 | The chance of a proc. Required on every proc. |
| `on` | `"attack"` (default), `"hit_taken"`, or `"skill_used"` | The trigger of a proc. |
| `target` | `"self"` or `"enemy"` | Who gets a proc's effect. rAthena's `ATF_SELF`, `ATF_TARGET`, and autospell `i & 1`. |
| `duration` | Milliseconds | How long a proc's effect lasts. |
| `random_level` | `true` | `cast` picks a level from 1 to the given level. rAthena's autospell `i & 2`. |
| `stacking` | `"sum"`, `"highest"`, `"lowest"`, `"last"`, or `"priority"` | See "Stacking". |
| `priority` | Integer | Used with `stacking = "priority"`. |
| `when` | A function | A condition. The bonus applies while it returns true. Every bonus takes it. See "Conditions". |

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

`against` names the target of the wearer's action: a hit, a skill, or a kill. `from` names the other side when the wearer is hit, and the monster that gives experience, drops, or a reward for a kill.

## Damage and damage taken

`"damage"` changes damage the wearer deals by a percent. `"damage_taken"` changes damage the wearer takes by a percent. Both take the amount, then an options table.

```lua
bonuses.add("damage", 10, { kind = "physical", against = { race = races.demi_human } })
bonuses.add("damage_taken", -15, { from = { size = sizes.all } })
```

### Groups

rAthena keeps one sum per bonus type, such as one sum per race for `bAddRace`. It applies each sum as its own factor, at a fixed step of the damage formula. wAthena keeps the same sums, so results match rAthena:

1. Each accepted combination of options belongs to one rAthena sum, listed in the two tables below.
2. Values in the same sum add up. A specific key and its `all` value add into the same sum, as in rAthena: `races.all` and `races.demi_human` both count against a Demi-Human.
3. Each sum is applied as its own factor, at the step where rAthena applies it.
4. A combination that is not in the tables raises an error in `bonuses.add`, since it has no rAthena sum. For example, `damage_taken` with `kind = "magic"` and `from = { race = ... }` fails.
5. rAthena has no sum for damage of every kind against a target. On `"damage"`, an omitted `kind` with an `against` key adds the value to the physical sum and to the magic sum.

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

The `element` option is the element of the hit. The `element` key of `against` or `from` is the other entity's own element. So `bSubEle` (hits of element e) is `{ element = e }`, and `bSubDefEle` (attackers whose body has element e) is `{ from = { element = e } }`.

### Bonuses that stay separate

These change damage too, but they are not a percent of damage dealt or taken in one of the sums above:

| Names | Why they stay separate |
|---|---|
| `crit_damage`, `non_crit_damage`, `crit_damage_taken` | They depend on whether the hit is critical, which rAthena checks at the critical step, not in the sums. |
| `skill_ratio` | It adds percentage points to a skill's ratio. It is not a percent of the damage. |
| `immunity` | It blocks part of a hit before every other reduction. With `kind = "magic"` it also blocks heals and support skills. |
| `damage_taken_cap` | It caps one hit at a percent of max HP. |
| `damage_reflected`, `reflect_magic`, `reflected_damage_taken` | They act on reflected damage, which has no kind or source in rAthena's sums. |
| `ignore_def`, `ignore_mdef`, `ignore_res`, `ignore_mres`, `def_as_damage`, `set_def`, `set_mdef` | They change the target's defense, not the damage. |
| `drain_hp`, `drain_sp`, `hp_per_hit`, `sp_per_hit` | They heal the wearer. |
| `remove_hp`, `remove_sp`, `deal_max_hp`, `deal_max_sp` | They remove HP or SP by a percent of the target's pool, not of the damage. |
| `ignore_size_penalty` | A flag that turns off the weapon size table. |

## Procs

A proc is a bonus whose effect happens with a chance, on a trigger. Its name is the effect. The `chance` option is required.

```lua
bonuses.add("cause_status", statuses.stun, { chance = 300 })
bonuses.add("cause_status", statuses.stun, { chance = 300, on = "hit_taken" })
bonuses.add("cast", FIREBOLT, 3, { chance = 500, on = "hit_taken" })
bonuses.add("coma", { chance = 10, against = { race = races.demi_human } })
bonuses.add("drain_hp", 5, { chance = 100 })
bonuses.add("break_weapon", { chance = 10 })
```

`on` values:

| `on` | The proc rolls | Target options |
|---|---|---|
| `"attack"` (default) | When the wearer's hit lands. With `skill`, only hits of that skill. | `against` |
| `"hit_taken"` | When a hit lands on the wearer. | `from` |
| `"skill_used"` | When the wearer uses the skill in `skill`, also for skills that deal no damage. | `against` |

Each proc row in the table lists the `on` values and options it accepts, following rAthena's forms. rAthena has no proc that rolls on a kill. Rewards for a kill are separate bonuses (`hp_per_kill`, `sp_per_kill`, `gain_zeny_on_kill`).

## Conditions

The `when` option is a function. The bonus applies while it returns true:

```lua
bonuses.add("unbreakable_weapon", { when = function() return equipped.refine >= 10 end })
bonuses.add("skill", TELEPORT, 1, { when = function() return player.base_level >= 99 end })
bonuses.add("atk", 20, { percent = true, when = function() return jobs.base_job(player.job) == jobs.crusader end })
```

- It follows the rules of computed bonuses: it reruns only when a value it read changes, and it cannot change the game.
- It works on every bonus: flags, granted skills, one-value bonuses, amounts, and highest or lowest stacking.
- It is not part of a set's key, so a bonus with a condition stacks with the same bonus without one.
- Static bonuses in item data have no `when`: a function cannot be data.

The converter writes `when` for every bonus that an rAthena script adds under a condition.

## Removing a bonus

`bonuses.add` returns a handle with a `remove` method:

```lua
local granted
events.on("base_level_up", function(player)
  if player.base_level >= 99 and not granted then
    granted = bonuses.add("skill", TELEPORT, 1)
  end
end)
```

- `remove()` removes that one bonus. A bonus that has already ended stays ended, and a second `remove()` does nothing.
- The bonus still belongs to its context, which removes it when it ends.
- Static, computed and conditional bonuses all return a handle.
- A bonus added in `timers.with_bonuses` returns a handle too. `remove()` ends it early. The timer ends the others.
- The converter does not use handles. It writes `when` for conditions.

## Stacking

The `stacking` option says how several bonuses of one name combine.

| Value | Rule | Allowed on |
|---|---|---|
| `"sum"` | All values add up. | Default for every bonus with an amount or a chance, except the six one-value bonuses. |
| `"highest"` | Only the highest value of the set counts. | Every bonus with an amount or a chance, except the six one-value bonuses. |
| `"lowest"` | Only the lowest value of the set counts. | The same as `"highest"`. |
| `"last"` | The bonus that comes last in rAthena's order wins. | Default for the six one-value bonuses. |
| `"priority"` | The bonus with the highest `priority` wins. | The six one-value bonuses. |

Flag bonuses take no `stacking` option.

**Sets.** Bonuses form a set when they have the same name, the same arguments before the amount, the same options other than `stacking`, `priority`, `chance`, and `when`, and a `stacking` value of the same group (`"sum"`, `"highest"`, and `"lowest"` form one group; `"last"` and `"priority"` the other).

**`"highest"` and `"lowest"`.**

1. Within a set, the bonuses with `"highest"` give one value: the largest. The bonuses with `"lowest"` give the smallest. The value compared is the chance on a proc, and the amount on any other bonus. "Highest" means the largest number, so of -10 and -20 it keeps -10.
2. That value is added to the sum of the set's `"sum"` bonuses.
3. On a proc, the kept bonus brings its other values with it. For `gain_zeny_on_kill`, the bonus with the highest chance brings its maximum zeny, as in rAthena.

**The one-value bonuses.** Six bonuses keep a single value, because rAthena overwrites them instead of adding: `attack_element`, `defense_element`, `set_def`, `set_mdef`, `block_healing`, and `transform_monster`. Within a set:

1. Each bonus has a priority: its `priority` option with `"priority"`, or 0 with `"last"`.
2. The bonus with the highest priority wins.
3. Among equal priorities, the bonus that comes last in rAthena's order wins.

**rAthena's order** is the order in which `status_calc_pc` runs item scripts. It does not depend on when items were equipped:

1. Equipment scripts, by slot: left accessory, right accessory, shoes, garment, lower head, middle head, upper head, armor, left hand, right hand, the four costume slots, ammo, and the six shadow slots.
2. Combo scripts.
3. Card scripts, by the slot of the item that holds the card, in the same order.
4. Timed bonuses (`timers.with_bonuses` and `player:add_timed_bonuses`), in the order they started.

Within one script, a later `bonuses.add` comes after an earlier one.

rAthena's seven types that keep only one value of a sum map to these options:

| rAthena | wAthena |
|---|---|
| `bonus bSpeedRate,n` | `"move_speed", n, { percent = true, stacking = "highest" }` |
| `bonus bDoubleRate,n` | `"double_attack_chance", n * 100, { stacking = "highest" }` |
| `bonus bSplashRange,n` | `"splash_range", n, { stacking = "highest" }` |
| `bonus bPerfectHitRate,n` | `"perfect_hit_chance", n * 100, { stacking = "highest" }` |
| `bonus2 bGetZenyNum,x,n` | `"gain_zeny_on_kill", x, { chance = n * 100, stacking = "highest" }` |
| `bonus bFixedCastrate,n` | `"fixed_cast_time", n, { percent = true, stacking = "lowest" }`. rAthena keeps the lowest value, which is the largest reduction. |
| `bonus bAbsorbDmgMaxHP2,n` | `"damage_taken_cap", n, { stacking = "highest" }` |

Mods can use `"highest"` and `"lowest"` on any bonus with an amount or a chance.

## Item events

An item context has three events: `"use"` (usable items), `"damage"`, and `"skill_used"`.

### damage

`"damage"` fires for every hit where the wearer is the source or the target.

```lua
events.on("damage", function(hit) ... end)
```

| Field | Meaning |
|---|---|
| `source` | Handle of the attacker. `source.kind` is `"player"` or `"monster"` (0002, review decision 17). |
| `target` | Handle of the one hit. `target.kind` says the same. |
| `kind` | `"physical"`, `"magic"`, or `"misc"`: the kind of the hit, not of a handle. |
| `range` | `"melee"` or `"ranged"`. |
| `skill` | The skill, or `nil` for a normal attack. |
| `element` | The element of the hit. |
| `is_critical` | `true` for a critical hit. |
| `amount` | The damage, after every reduction. It can be 0. |
| `wearer_is_source` | `true` when the wearer dealt the hit, `false` when the wearer took it. |

- The event fires once per hit, after its damage is computed. A miss does not fire it.
- The event name `"damage"` and the bonus name `"damage"` are in separate lists, so they do not clash.

### skill_used

`"skill_used"` fires every time the wearer uses a skill, also for skills that deal no damage, such as buffs. It fires once per use, when the skill takes effect, before its hits.

```lua
events.on("skill_used", function(use) ... end)
```

| Field | Meaning |
|---|---|
| `skill` | The skill. |
| `level` | The level used. |
| `target` | Handle of the target, or `nil` for a ground skill. |
| `x`, `y` | The target cell. |

Both event objects are read-only. A handler cannot change the damage or the skill. That is the job of hooks.

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

| rAthena | wAthena | Uses in renewal data |
|---|---|---|
| `autobonus` | `"damage"` with `hit.wearer_is_source` | 422 |
| `autobonus2` | `"damage"` with `not hit.wearer_is_source` | 52 |
| `autobonus3` with skill sk | `"skill_used"` with `use.skill == sk` | 158 |

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
    { "natural_hp_recovery", random.integer(11, 33), { percent = true } },
  }, { ends_on = { "death" } })
end)
```

- Each entry is a bonus name followed by its arguments, as in `bonuses.add`.
- The key (`"hp_recovery_food"`) replaces an older copy with the same key. This is rAthena's default. rAthena's "add a duplicate" flag (2048) is the option `stack = true`.
- `ends_on` lists the removal flags by name: `"death"`, `"dispell"`, `"clearance"`, `"logout"`, `"banishing_buster"`, `"refresh"`, `"lux_anima"`, `"magic_gear_change"`, `"damage"`.
- The options `icon` and `kind` (`"buff"` or `"debuff"`) cover rAthena's icon and type.

## Constants used in arguments

Constant names come from rAthena's by a fixed rule:

- The prefix is removed, such as `RC_`, `SC_`, `EF_`, `HAT_EF_` or `JOB_`.
- The rest is written in lower case. Underscores stay.
- A name that would start with a digit gets an `n`: `SC_2011RWC_SCROLL` is `statuses.n2011rwc_scroll`.
- These names are spelled out:

  | rAthena | wAthena |
  |---|---|
  | `RC_DEMIHUMAN` | `races.demi_human` |
  | `W_1HSWORD`, `W_2HSWORD` | `weapon_types.one_handed_sword`, `two_handed_sword` |
  | `W_1HSPEAR`, `W_2HSPEAR` | `weapon_types.one_handed_spear`, `two_handed_spear` |
  | `W_1HAXE`, `W_2HAXE` | `weapon_types.one_handed_axe`, `two_handed_axe` |
  | `W_2HMACE`, `W_2HSTAFF` | `weapon_types.two_handed_mace`, `two_handed_staff` |
  | `W_DOUBLE_DD`, `W_DOUBLE_SS`, `W_DOUBLE_AA` | `weapon_types.dual_daggers`, `dual_swords`, `dual_axes` |
  | `W_DOUBLE_DS`, `W_DOUBLE_DA`, `W_DOUBLE_SA` | `weapon_types.dagger_and_sword`, `dagger_and_axe`, `sword_and_axe` |

The modules and their rAthena prefixes: `races` (`RC_`), `monster_groups` (`RC2_`), `elements` (`ELE_`), `sizes` (`Size_`), `monster_classes` (`CLASS_`), `weapon_types` (`W_`), `statuses` (`SC_`), `effects` (`EF_`), `headgear_effects` (`HAT_EF_`), and `jobs` (`JOB_`).

Arguments use the constant modules from proposal 0002 (`races`, `monster_classes`, `sizes`, `elements`, `statuses`, `jobs`) and record lookups (`items.id`, `skills.id`, `monsters.id`). This document adds:

- `wa.weapon_types` for rAthena's `W_...`, such as `weapon_types.bow`.
- `wa.monster_groups` for rAthena's `RC2_...` "monster race", such as `monster_groups.goblin`.
- `items.group "Potion"` for item groups, by the group's name in the item group data (rAthena's `IG_Potion`).

## Table

Columns:

- **rAthena**: the bonus command and its arguments. Several forms of one type are listed together.
- **wAthena**: the arguments of `bonuses.add`, with the conversion written in. `sk`, `r`, `e`, and the other letters are rAthena's arguments, converted to wAthena constants.
- **Modes**: for names with an amount, whether the amount is `flat`, `percent`, or both (rule 4). A flag has none.
- **Meaning**: one line.
- **Uses**: count in renewal item data, per form.
- **Sign**: "flipped" if the converter negates the value.

### Base stats

| rAthena | wAthena | Modes | Meaning | Uses |
|---|---|---|---|---|
| `bonus bStr,n` | `"str", n` | flat | STR + n. | 507 |
| `bonus bAgi,n` | `"agi", n` | flat | AGI + n. | 442 |
| `bonus bVit,n` | `"vit", n` | flat | VIT + n. | 389 |
| `bonus bInt,n` | `"int", n` | flat | INT + n. | 780 |
| `bonus bDex,n` | `"dex", n` | flat | DEX + n. | 643 |
| `bonus bLuk,n` | `"luk", n` | flat | LUK + n. | 332 |
| `bonus bAllStats,n` | `"all_stats", n` | flat | All six base stats + n. | 525 |
| `bonus bAgiVit,n` | `"agi", n` and `"vit", n` | | AGI and VIT + n. | 5 |
| `bonus bAgiDexStr,n` | `"str", n`, `"agi", n`, and `"dex", n` | | STR, AGI, and DEX + n. | 0 |

### Trait stats (fourth jobs)

| rAthena | wAthena | Modes | Meaning | Uses |
|---|---|---|---|---|
| `bonus bPow,n` | `"pow", n` | flat | POW + n. | 407 |
| `bonus bSta,n` | `"sta", n` | flat | STA + n. | 42 |
| `bonus bWis,n` | `"wis", n` | flat | WIS + n. | 33 |
| `bonus bSpl,n` | `"spl", n` | flat | SPL + n. | 306 |
| `bonus bCon,n` | `"con", n` | flat | CON + n. | 281 |
| `bonus bCrt,n` | `"crt", n` | flat | CRT + n. | 74 |
| `bonus bAllTraitStats,n` | `"all_trait_stats", n` | flat | All six trait stats + n. | 18 |
| `bonus bPatk,n`<br>`bonus bPAtkRate,n` | `"patk", n`<br>`"patk", n, { percent = true }` | flat, percent | P.ATK + n, or + n%. | 1203<br>0 |
| `bonus bSmatk,n`<br>`bonus bSMatkRate,n` | `"smatk", n`<br>`"smatk", n, { percent = true }` | flat, percent | S.MATK + n, or + n%. | 752<br>0 |
| `bonus bRes,n`<br>`bonus bResRate,n` | `"res", n`<br>`"res", n, { percent = true }` | flat, percent | RES + n, or + n%. | 217<br>0 |
| `bonus bMres,n`<br>`bonus bMResRate,n` | `"mres", n`<br>`"mres", n, { percent = true }` | flat, percent | MRES + n, or + n%. | 151<br>0 |
| `bonus bHplus,n`<br>`bonus bHPlusRate,n` | `"hplus", n`<br>`"hplus", n, { percent = true }` | flat, percent | H.PLUS + n, or + n%. | 21<br>0 |
| `bonus bCrate,n`<br>`bonus bCRateRate,n` | `"crate", n`<br>`"crate", n, { percent = true }` | flat, percent | C.RATE + n, or + n%. | 125<br>0 |

### HP, SP, and AP

| rAthena | wAthena | Modes | Meaning | Uses |
|---|---|---|---|---|
| `bonus bMaxHP,n`<br>`bonus bMaxHPrate,n` | `"max_hp", n`<br>`"max_hp", n, { percent = true }` | flat, percent | Max HP + n, or + n%. | 1565<br>1579 |
| `bonus bMaxSP,n`<br>`bonus bMaxSPrate,n` | `"max_sp", n`<br>`"max_sp", n, { percent = true }` | flat, percent | Max SP + n, or + n%. | 581<br>1042 |
| `bonus bMaxAP,n`<br>`bonus bMaxAPrate,n` | `"max_ap", n`<br>`"max_ap", n, { percent = true }` | flat, percent | Max AP + n, or + n%. | 0<br>0 |
| `bonus bHPrecovRate,n` | `"natural_hp_recovery", n, { percent = true }` | percent | Natural HP recovery + n%. | 106 |
| `bonus bSPrecovRate,n` | `"natural_sp_recovery", n, { percent = true }` | percent | Natural SP recovery + n%. | 138 |
| `bonus2 bHPRegenRate,n,t`<br>`bonus2 bRegenPercentHP,n,t` | `"hp_gain_over_time", n, t`<br>`"hp_gain_over_time", n, t, { percent = true }` | flat, percent (of max HP) | Gain n HP, or n% of max HP, every t ms. | 100<br>4 |
| `bonus2 bSPRegenRate,n,t`<br>`bonus2 bRegenPercentSP,n,t` | `"sp_gain_over_time", n, t`<br>`"sp_gain_over_time", n, t, { percent = true }` | flat, percent (of max SP) | Gain n SP, or n% of max SP, every t ms. | 132<br>6 |
| `bonus2 bHPLossRate,n,t` | `"hp_loss_over_time", n, t` | flat | Lose n HP every t ms. A separate name: rAthena keeps losses in their own list, apart from gains. | 48 |
| `bonus2 bSPLossRate,n,t` | `"sp_loss_over_time", n, t` | flat | Lose n SP every t ms. Separate for the same reason. | 27 |
| `bonus bNoRegen,1`<br>`bonus bNoRegen,2` | `"no_hp_recovery"`<br>`"no_sp_recovery"` | | Stops natural HP or SP recovery. | 17 |

### ATK and DEF

| rAthena | wAthena | Modes | Meaning | Uses |
|---|---|---|---|---|
| `bonus bBaseAtk,n`<br>`bonus bAtkRate,n`<br>`bonus2 bWeaponAtk,w,n` | `"atk", n`<br>`"atk", n, { percent = true }`<br>`"atk", n, { weapon_type = w }` | flat, percent | Equipment ATK + n (the "ATK + n" of renewal items), ATK + n%, or ATK + n while a weapon of type w is equipped. | 2623<br>1619<br>0 |
| `bonus bAtk,n`<br>`bonus bWeaponAtkRate,n` | `"weapon_atk", n`<br>`"weapon_atk", n, { percent = true }` | flat, percent | Weapon ATK + n, or + n%. rAthena marks the flat form unofficial. | 1<br>7 |
| `bonus bAtk2,n` | `"refine_atk", n` | flat | ATK that counts like refine ATK + n. | 1 |
| `bonus bMatk,n`<br>`bonus bMatkRate,n` | `"matk", n`<br>`"matk", n, { percent = true }` | flat, percent | MATK + n, or + n%. | 1767<br>1495 |
| `bonus bMatk2,n` | `"hidden_matk", n` | flat | MATK + n, not shown in the status window. A separate name: the status window shows `matk` but not this. | 0 |
| `bonus bWeaponMatkRate,n` | `"weapon_matk", n, { percent = true }` | percent | Weapon MATK + n%. | 0 |
| `bonus bDef,n`<br>`bonus bDefRate,n` | `"def", n`<br>`"def", n, { percent = true }` | flat, percent | Equipment DEF + n, or + n%. | 641<br>9 |
| `bonus bDef2,n`<br>`bonus bDef2Rate,n` | `"status_def", n`<br>`"status_def", n, { percent = true }` | flat, percent | DEF from stats (VIT) + n, or + n%. | 0<br>5 |
| `bonus bMdef,n`<br>`bonus bMdefRate,n` | `"mdef", n`<br>`"mdef", n, { percent = true }` | flat, percent | Equipment MDEF + n, or + n%. | 1346<br>5 |
| `bonus bMdef2,n`<br>`bonus bMdef2Rate,n` | `"status_mdef", n`<br>`"status_mdef", n, { percent = true }` | flat, percent | MDEF from stats (INT) + n, or + n%. | 0<br>0 |
| `bonus bAtkRange,n` | `"attack_range", n` | flat | Attack range + n cells. | 1 |

### HIT, FLEE, and CRIT

| rAthena | wAthena | Modes | Meaning | Uses | Sign |
|---|---|---|---|---|---|
| `bonus bHit,n`<br>`bonus bHitRate,n` | `"hit", n`<br>`"hit", n, { percent = true }` | flat, percent | HIT + n, or + n%. | 531<br>1 | |
| `bonus bFlee,n`<br>`bonus bFleeRate,n` | `"flee", n`<br>`"flee", n, { percent = true }` | flat, percent | FLEE + n, or + n%. | 426<br>1 | |
| `bonus bFlee2,n`<br>`bonus bFlee2Rate,n` | `"perfect_dodge", n`<br>`"perfect_dodge", n, { percent = true }` | flat, percent | Perfect Dodge + n, or + n%. | 155<br>0 | |
| `bonus bCritical,n`<br>`bonus bCriticalRate,n`<br>`bonus bCriticalLong,n`<br>`bonus2 bCriticalAddRace,r,n` | `"crit", n`<br>`"crit", n, { percent = true }`<br>`"crit", n, { kind = "physical", range = "ranged", attack = "normal" }`<br>`"crit", n, { against = { race = r } }` | flat, percent | CRIT + n or + n%; CRIT + n for ranged normal attacks (not shown in the status window); CRIT + n against race r. | 757<br>4<br>14<br>41 | |
| `bonus bCriticalDef,n` | `"crit_chance_taken", -n` | percent | The chance of being hit by a critical changes by n%. A relative change, so it is a percent, not a chance per 10,000. | 0 | flipped |
| `bonus bPerfectHitRate,n`<br>`bonus bPerfectHitAddRate,n` | `"perfect_hit_chance", n * 100, { stacking = "highest" }`<br>`"perfect_hit_chance", n * 100` | per 10,000 | Chance that an attack always hits. | 1<br>193 | |
| `bonus bDoubleRate,n`<br>`bonus bDoubleAddRate,n` | `"double_attack_chance", n * 100, { stacking = "highest" }`<br>`"double_attack_chance", n * 100` | per 10,000 | Chance of a double attack, with any weapon. | 0<br>0 | |

### Speed, cast, and delay

| rAthena | wAthena | Modes | Meaning | Uses | Sign |
|---|---|---|---|---|---|
| `bonus bSpeedRate,n`<br>`bonus bSpeedAddRate,n` | `"move_speed", n, { percent = true, stacking = "highest" }`<br>`"move_speed", n, { percent = true }` | percent | Move speed + n%. The first form does not stack with speed potions. | 73<br>0 | |
| `bonus bAspd,n`<br>`bonus bAspdRate,n` | `"aspd", n`<br>`"aspd", n, { percent = true }` | flat, percent | ASPD + n, or + n%. | 296<br>1291 | |
| `bonus bVariableCast,t`<br>`bonus2 bSkillVariableCast,sk,t`<br>`bonus bVariableCastrate,n`<br>`bonus2 bVariableCastrate,sk,n` | `"variable_cast_time", t`<br>`"variable_cast_time", t, { skill = sk }`<br>`"variable_cast_time", n, { percent = true }`<br>`"variable_cast_time", n, { percent = true, skill = sk }` | flat (ms), percent | Variable cast time of all skills or of skill sk + t ms, or + n%. | 1<br>30<br>1209<br>229 | |
| `bonus bCastrate,n`<br>`bonus2 bCastrate,sk,n` | `"variable_cast_time", n, { percent = true }`<br>`"variable_cast_time", n, { percent = true, skill = sk }` | | The same as `bVariableCastrate` in renewal. | 1<br>0 | |
| `bonus bFixedCast,t`<br>`bonus2 bSkillFixedCast,sk,t`<br>`bonus bFixedCastrate,n`<br>`bonus2 bFixedCastrate,sk,n` | `"fixed_cast_time", t`<br>`"fixed_cast_time", t, { skill = sk }`<br>`"fixed_cast_time", n, { percent = true, stacking = "lowest" }`<br>`"fixed_cast_time", n, { percent = true, skill = sk }` | flat (ms), percent | Fixed cast time of all skills or of skill sk + t ms, or + n%. | 501<br>28<br>67<br>61 | |
| `bonus bDelayrate,n`<br>`bonus2 bSkillDelay,sk,t` | `"after_cast_delay", n, { percent = true }`<br>`"after_cast_delay", t, { skill = sk }` | percent for all skills, flat (ms) with `skill` | After-cast delay of all skills + n%, or of skill sk + t ms. | 1227<br>0 | |
| `bonus2 bSkillCooldown,sk,t` | `"cooldown", t, { skill = sk }` | flat (ms); `skill` required | Cooldown of skill sk + t. | 618 | |
| `bonus bUseSPrate,n`<br>`bonus2 bSkillUseSP,sk,n`<br>`bonus2 bSkillUseSPrate,sk,n` | `"sp_cost", n, { percent = true }`<br>`"sp_cost", -n, { skill = sk }`<br>`"sp_cost", -n, { percent = true, skill = sk }` | percent for all skills; flat and percent with `skill` | SP cost of all skills + n%, or of skill sk + n SP or + n%. | 269<br>269<br>75 | flipped (both skill forms) |
| `bonus bNoCastCancel` | `"uninterruptible_cast"` | | Casting is not interrupted by hits, except in guild wars. | 54 | |
| `bonus bNoCastCancel2` | `"uninterruptible_cast_everywhere"` | | Casting is not interrupted by hits, also in guild wars. A separate name: the guild war rule differs, and both can be worn. | 3 | |
| `bonus bNoWalkDelay` | `"always_endure"` | | Hits never stop the wearer (permanent Endure). | 11 | |

### Damage dealt

All amounts are percents of damage dealt.

| rAthena | wAthena | Meaning | Uses |
|---|---|---|---|
| `bonus2 bAddRace,r,x` | `"damage", x, { kind = "physical", against = { race = r } }` | Physical damage + x% against race r. | 1980 |
| `bonus2 bMagicAddRace,r,x` | `"damage", x, { kind = "magic", against = { race = r } }` | Magic damage + x% against race r. | 1046 |
| `bonus2 bAddClass,c,x` | `"damage", x, { kind = "physical", against = { class = c } }` | Physical damage + x% against class c. | 384 |
| `bonus2 bMagicAddClass,c,x` | `"damage", x, { kind = "magic", against = { class = c } }` | Magic damage + x% against class c. | 91 |
| `bonus2 bAddSize,s,x` | `"damage", x, { kind = "physical", against = { size = s } }` | Physical damage + x% against size s. | 867 |
| `bonus2 bMagicAddSize,s,x` | `"damage", x, { kind = "magic", against = { size = s } }` | Magic damage + x% against size s. | 487 |
| `bonus2 bAddEle,e,x`<br>`bonus3 bAddEle,e,x,bf` | `"damage", x, { kind = "physical", against = { element = e } }`<br>The same, with `kind`, `range`, and `attack` from bf | Physical damage + x% against targets of element e. | 1154<br>0 |
| `bonus2 bMagicAddEle,e,x` | `"damage", x, { kind = "magic", against = { element = e } }` | Magic damage + x% against targets of element e. | 866 |
| `bonus2 bAddRace2,mr,x` | `"damage", x, { kind = "physical", against = { monster_group = mr } }` | Physical damage + x% against monster group mr. | 59 |
| `bonus2 bMagicAddRace2,mr,x` | `"damage", x, { kind = "magic", against = { monster_group = mr } }` | Magic damage + x% against monster group mr. | 46 |
| `bonus2 bAddDamageClass,mid,x` | `"damage", x, { kind = "physical", against = { monster = mid } }` | Physical damage + x% against monster mid. For a job, `against = { job = ... }`. | 60 |
| `bonus2 bAddMagicDamageClass,mid,x` | `"damage", x, { kind = "magic", against = { monster = mid } }` | Magic damage + x% against monster mid. | 24 |
| `bonus2 bMagicAtkEle,e,x` | `"damage", x, { kind = "magic", element = e }` | Damage of the wearer's element e magic + x%. | 2531 |
| `bonus bShortAtkRate,n` | `"damage", n, { kind = "physical", range = "melee" }` | Melee physical damage + n%. | 951 |
| `bonus bLongAtkRate,n` | `"damage", n, { kind = "physical", range = "ranged" }` | Ranged physical damage + n%. | 1637 |
| `bonus2 bWeaponDamageRate,w,n` | `"damage", n, { kind = "physical", weapon_type = w }` | Physical damage + n% while a weapon of type w is equipped. | 20 |
| `bonus2 bSkillAtk,sk,n` | `"damage", n, { skill = sk }` | Damage of skill sk + n%. | 7893 |
| `bonus bCritAtkRate,n` | `"crit_damage", n` | Critical damage + n%. | 732 |
| `bonus bNonCritAtkRate,n` | `"non_crit_damage", n` | Non-critical damage + n%. | 79 |
| `bonus bSkillRatio,n` | `"skill_ratio", n` | Adds n percentage points to the damage ratio of every attack and skill. | 0 |
| `bonus bNoSizeFix` | `"ignore_size_penalty"` | Weapon size penalties do not apply. | 29 |

### Ignoring and changing defense

`ignore_def` and `ignore_mdef` have two modes. Without an amount, they ignore all of the target's DEF or MDEF. With an amount and `percent = true`, they ignore that percent.

| rAthena | wAthena | Modes | Meaning | Uses |
|---|---|---|---|---|
| `bonus bIgnoreDefRace,r`<br>`bonus bIgnoreDefEle,e`<br>`bonus bIgnoreDefClass,c` | `"ignore_def", { against = { race = r } }`<br>`"ignore_def", { against = { element = e } }`<br>`"ignore_def", { against = { class = c } }` | full | Ignore DEF of race r, element e, or class c. | 47<br>0<br>20 |
| `bonus2 bIgnoreDefRaceRate,r,n`<br>`bonus2 bIgnoreDefClassRate,c,n` | `"ignore_def", n, { percent = true, against = { race = r } }`<br>`"ignore_def", n, { percent = true, against = { class = c } }` | percent | Ignore n% of DEF of race r or class c. | 656<br>32 |
| `bonus bIgnoreMdefRace,r`<br>`bonus bIgnoreMdefEle,e` | `"ignore_mdef", { against = { race = r } }`<br>`"ignore_mdef", { against = { element = e } }` | full | Ignore MDEF of race r or element e. | 1<br>0 |
| `bonus2 bIgnoreMdefRaceRate,r,n`<br>`bonus2 bIgnoreMdefClassRate,c,n`<br>`bonus2 bIgnoreMdefRace2Rate,mr,n` | `"ignore_mdef", n, { percent = true, against = { race = r } }`<br>`"ignore_mdef", n, { percent = true, against = { class = c } }`<br>`"ignore_mdef", n, { percent = true, against = { monster_group = mr } }` | percent | Ignore n% of MDEF of race r, class c, or monster group mr. | 308<br>58<br>1 |
| `bonus2 bIgnoreResRaceRate,r,n` | `"ignore_res", n, { percent = true, against = { race = r } }` | percent | Ignore n% of RES of race r. | 18 |
| `bonus2 bIgnoreMResRaceRate,r,n` | `"ignore_mres", n, { percent = true, against = { race = r } }` | percent | Ignore n% of MRES of race r. | 18 |
| `bonus bDefRatioAtkRace,r`<br>`bonus bDefRatioAtkEle,e`<br>`bonus bDefRatioAtkClass,c` | `"def_as_damage", { against = { race = r } }`<br>`"def_as_damage", { against = { element = e } }`<br>`"def_as_damage", { against = { class = c } }` | | Ignore DEF and deal more damage the higher the DEF of race r, element e, or class c. | 0<br>0<br>14 |
| `bonus4 bSetDefRace,r,n,t,y` | `"set_def", y, { chance = n, against = { race = r }, attack = "normal", duration = t }` | flat | Normal attacks may set the DEF of race r to y for t ms. Proc; one-value bonus. | 5 |
| `bonus4 bSetMDefRace,r,n,t,y` | `"set_mdef", y, { chance = n, against = { race = r }, attack = "normal", duration = t }` | flat | The same for MDEF. | 5 |

### Damage taken

All amounts are percents of damage taken. A negative value means less damage.

| rAthena | wAthena | Meaning | Uses | Sign |
|---|---|---|---|---|
| `bonus2 bSubRace,r,x`<br>`bonus3 bSubRace,r,x,bf` | `"damage_taken", -x, { from = { race = r } }`<br>The same, with `kind`, `range`, and `attack` from bf | Damage taken from race r changes. | 1631<br>1 | flipped |
| `bonus2 bSubClass,c,x` | `"damage_taken", -x, { from = { class = c } }` | Damage taken from class c changes. | 281 | flipped |
| `bonus2 bSubSize,s,x` | `"damage_taken", -x, { from = { size = s } }` | Damage taken from size s changes. | 147 | flipped |
| `bonus2 bWeaponSubSize,s,x` | `"damage_taken", -x, { kind = "physical", from = { size = s } }` | Physical damage taken from size s changes. | 0 | flipped |
| `bonus2 bMagicSubSize,s,x` | `"damage_taken", -x, { kind = "magic", from = { size = s } }` | Magic damage taken from size s changes. | 2 | flipped |
| `bonus2 bSubEle,e,x`<br>`bonus3 bSubEle,e,x,bf` | `"damage_taken", -x, { element = e }`<br>The same, with `kind`, `range`, and `attack` from bf | Damage taken from hits of element e changes. | 931<br>12 | flipped |
| `bonus2 bSubDefEle,e,x` | `"damage_taken", -x, { kind = "physical", from = { element = e } }` | Physical damage taken from attackers whose own element is e changes. | 8 | flipped |
| `bonus2 bMagicSubDefEle,e,x` | `"damage_taken", -x, { kind = "magic", from = { element = e } }` | The same for magic damage. | 3 | flipped |
| `bonus2 bSubRace2,mr,x` | `"damage_taken", -x, { from = { monster_group = mr } }` | Damage taken from monster group mr changes. | 30 | flipped |
| `bonus2 bAddDefMonster,mid,x` | `"damage_taken", -x, { kind = "physical", from = { monster = mid } }` | Physical damage taken from monster mid changes. For a job, `from = { job = ... }`. | 33 | flipped |
| `bonus2 bAddMdefMonster,mid,x` | `"damage_taken", -x, { kind = "magic", from = { monster = mid } }` | Magic damage taken from monster mid changes. | 3 | flipped |
| `bonus bNearAtkDef,n` | `"damage_taken", -n, { kind = "physical", range = "melee" }` | Melee physical damage taken changes. | 11 | flipped |
| `bonus bLongAtkDef,n` | `"damage_taken", -n, { kind = "physical", range = "ranged" }` | Ranged physical damage taken changes. | 83 | flipped |
| `bonus bMagicAtkDef,n` | `"damage_taken", -n, { kind = "magic" }` | Magic damage taken changes. | 2 | flipped |
| `bonus bMiscAtkDef,n` | `"damage_taken", -n, { kind = "misc" }` | Misc damage taken (traps, falcon) changes. | 0 | flipped |
| `bonus2 bSubSkill,sk,n` | `"damage_taken", -n, { skill = sk }` | Damage taken from skill sk changes. | 24 | flipped |
| `bonus bCritDefRate,n` | `"crit_damage_taken", -n` | Critical damage taken changes by n%. | 0 | flipped |
| `bonus bNoWeaponDamage,n`<br>`bonus bNoMagicDamage,n`<br>`bonus bNoMiscDamage,n` | `"immunity", n, { kind = "physical" }`<br>`"immunity", n, { kind = "magic" }`<br>`"immunity", n, { kind = "misc" }` | Blocks n% of hits of that kind, before other reductions. Magic immunity also blocks heals and support skills. `kind` is required. | 0<br>3<br>0 | |
| `bonus bAbsorbDmgMaxHP2,n`<br>`bonus bAbsorbDmgMaxHP,n` | `"damage_taken_cap", n, { stacking = "highest" }`<br>The same | A hit above n% of max HP is reduced to n% of max HP. The second form is rAthena's old, unofficial cap. | 1<br>0 | |
| `bonus bShortWeaponDamageReturn,n`<br>`bonus bLongWeaponDamageReturn,n` | `"damage_reflected", n, { kind = "physical", range = "melee" }`<br>`"damage_reflected", n, { kind = "physical", range = "ranged" }` | Reflects n% of melee or ranged physical damage taken. `range` is required. | 45<br>0 | |
| `bonus bMagicDamageReturn,n` | `"reflect_magic", { chance = n * 100, on = "hit_taken" }` | Proc: may reflect a targeted spell back to its caster. | 11 | |
| `bonus bReduceDamageReturn,n` | `"reflected_damage_taken", -n` | Damage taken from reflection changes by n%. | 39 | flipped |

### Elements

| rAthena | wAthena | Meaning | Uses |
|---|---|---|---|
| `bonus bAtkEle,e` | `"attack_element", e` | Attacks have element e. One-value bonus. | 266 |
| `bonus bDefEle,e` | `"defense_element", e` | The wearer's body has element e. One-value bonus. | 60 |

### Status effects and other procs

Every row here is a proc: `chance` is required (rule 5).

| rAthena | wAthena | Meaning | Uses |
|---|---|---|---|
| `bonus2 bAddEff,eff,n`<br>`bonus3 bAddEff,eff,n,atf`<br>`bonus4 bAddEff,eff,n,atf,t` | `"cause_status", eff, { chance = n }`<br>The same, with `target`, `kind`, and `range` from atf<br>The same, with `duration = t` | Attacks may cause status eff. | 163<br>50<br>2 |
| `bonus2 bAddEff2,eff,n` | `"cause_status", eff, { chance = n, target = "self" }` | Attacks may cause status eff on the wearer. | 25 |
| `bonus2 bAddEffWhenHit,eff,n`<br>`bonus3 bAddEffWhenHit,eff,n,atf`<br>`bonus4 bAddEffWhenHit,eff,n,atf,t` | `"cause_status", eff, { chance = n, on = "hit_taken", kind = "physical" }`<br>The same, with `target`, `kind`, and `range` from atf<br>The same, with `duration = t` | Being hit may cause status eff on the attacker. | 61<br>23<br>0 |
| `bonus3 bAddEffOnSkill,sk,eff,n`<br>`bonus4 bAddEffOnSkill,sk,eff,n,atf`<br>`bonus5 bAddEffOnSkill,sk,eff,n,atf,t` | `"cause_status", eff, { chance = n, skill = sk }`<br>The same, with `target` from atf<br>The same, with `duration = t` | Hits of skill sk may cause status eff. | 24<br>0<br>0 |
| `bonus2 bResEff,eff,n` | `"status_resistance", eff, n` | The chance to resist status eff rises by n per 10,000. Not a proc: an amount, with no `chance` option. | 379 |
| `bonus2 bComaRace,r,n`<br>`bonus2 bComaClass,c,n` | `"coma", { chance = n, against = { race = r } }`<br>`"coma", { chance = n, against = { class = c } }` | Any attack on race r or class c may cause Coma. | 10<br>3 |
| `bonus2 bWeaponComaRace,r,n`<br>`bonus2 bWeaponComaClass,c,n`<br>`bonus2 bWeaponComaEle,e,n` | `"coma", { chance = n, attack = "normal", against = { race = r } }`<br>The same, with `against = { class = c }`<br>The same, with `against = { element = e }` | Normal attacks on race r, class c, or element e may cause Coma. | 0<br>0<br>0 |
| `bonus3 bStateNoRecoverRace,r,x,t` | `"block_healing", { chance = x, attack = "normal", against = { race = r }, duration = t }` | Normal attacks on race r may stop the target's healing for t ms. One-value bonus. | 2 |
| `bonus3 bAutoSpell,sk,y,n`<br>`bonus4 bAutoSpell,sk,y,n,i`<br>`bonus5 bAutoSpell,sk,y,n,bf,i` | `"cast", sk, y, { chance = n * 10 }`<br>The same, with `target` and `random_level` from i<br>The same, with `kind`, `range`, and `attack` from bf | Attacks may cast skill sk at level y. | 375<br>25<br>90 |
| `bonus3 bAutoSpellWhenHit,sk,y,n`<br>`bonus4 bAutoSpellWhenHit,sk,y,n,i`<br>`bonus5 bAutoSpellWhenHit,sk,y,n,bf,i` | `"cast", sk, y, { chance = n * 10, on = "hit_taken" }`<br>The same, with `target` and `random_level` from i<br>The same, with `kind`, `range`, and `attack` from bf | Being hit by a direct attack may cast skill sk at level y. | 145<br>35<br>60 |
| `bonus4 bAutoSpellOnSkill,sk,x,y,n`<br>`bonus5 bAutoSpellOnSkill,sk,x,y,n,i` | `"cast", x, y, { chance = n * 10, on = "skill_used", skill = sk }`<br>The same, with `target` and `random_level` from i | Using skill sk may cast skill x at level y. | 71<br>10 |
| `bonus2 bHPDrainRate,x,n` | `"drain_hp", n, { chance = x * 10 }` | Physical hits may heal n% of the damage dealt as HP. | 92 |
| `bonus2 bSPDrainRate,x,n` | `"drain_sp", n, { chance = x * 10 }` | The same, as SP. | 66 |
| `bonus2 bHPVanishRate,x,n`<br>`bonus3 bHPVanishRate,x,n,bf` | `"remove_hp", n, { chance = x * 10, attack = "normal" }`<br>`"remove_hp", n, { chance = x * 10 }`, with `kind`, `range`, and `attack` from bf | Hits may remove n% of the target's current HP. | 1<br>0 |
| `bonus2 bSPVanishRate,x,n`<br>`bonus3 bSPVanishRate,x,n,bf` | `"remove_sp", n, { chance = x * 10, attack = "normal" }`<br>`"remove_sp", n, { chance = x * 10 }`, with `kind`, `range`, and `attack` from bf | The same for SP. | 1<br>5 |
| `bonus3 bHPVanishRaceRate,r,x,n` | `"deal_max_hp", n, { chance = x * 10, kind = "physical", against = { race = r } }` | Physical hits on race r may deal n% of the target's max HP instead of their damage. | 2 |
| `bonus3 bSPVanishRaceRate,r,x,n` | `"deal_max_sp", n, { chance = x * 10, kind = "physical", against = { race = r } }` | The same, as SP damage. | 5 |
| `bonus bBreakWeaponRate,n` | `"break_weapon", { chance = n }` | Hits may break the target's weapon. | 12 |
| `bonus bBreakArmorRate,n` | `"break_armor", { chance = n }` | Hits may break the target's armor. | 17 |
| `bonus bClassChange,n` | `"transform_monster", { chance = n, kind = "physical" }` | Physical hits may turn the monster into a random other monster. One-value bonus. | 2 |

The set-defense and reflect-magic procs are in "Ignoring and changing defense" and "Damage taken".

### Skills

| rAthena | wAthena | Meaning | Uses |
|---|---|---|---|
| `skill sk,lv` (a command) | `"skill", sk, lv` | Grants skill sk at level lv while equipped. | 633 |
| `bonus2 bAddSkillBlow,sk,n` | `"knockback", n, { skill = sk }` | Skill sk knocks the target back n more cells. `skill` is required. | 8 |
| `bonus bNoGemStone` | `"no_gemstone_cost"` | Skills need no gemstones. Hocus Pocus and Ganbantein keep their cost. | 16 |
| `bonus bNoMadoFuel` | `"no_magic_gear_fuel_cost"` | Skills need no Magic Gear Fuel. | 9 |

Skill damage, damage taken from a skill, cast time, delay, cooldown, SP cost, and healing take the `skill` option on their own names.

### HP and SP from hits and kills

| rAthena | wAthena | Meaning | Uses |
|---|---|---|---|
| `bonus bHPDrainValue,n`<br>`bonus2 bHPDrainValueRace,r,n`<br>`bonus2 bHPDrainValueClass,c,n` | `"hp_per_hit", n`<br>`"hp_per_hit", n, { against = { race = r } }`<br>`"hp_per_hit", n, { against = { class = c } }` | Each normal attack heals n HP. A separate name from `drain_hp`: a fixed amount on every normal attack, with no chance. | 1<br>0<br>0 |
| `bonus bSPDrainValue,n`<br>`bonus2 bSPDrainValueRace,r,n`<br>`bonus2 bSPDrainValueClass,c,n` | `"sp_per_hit", n`<br>`"sp_per_hit", n, { against = { race = r } }`<br>`"sp_per_hit", n, { against = { class = c } }` | Each normal attack restores n SP. | 39<br>5<br>0 |
| `bonus bHPGainValue,n`<br>`bonus bLongHPGainValue,n`<br>`bonus bMagicHPGainValue,n` | `"hp_per_kill", n, { kind = "physical", range = "melee" }`<br>`"hp_per_kill", n, { kind = "physical", range = "ranged" }`<br>`"hp_per_kill", n, { kind = "magic" }` | A kill by that kind of hit heals n HP. `kind` is required. | 68<br>3<br>38 |
| `bonus bSPGainValue,n`<br>`bonus2 bSPGainRace,r,n`<br>`bonus bLongSPGainValue,n`<br>`bonus bMagicSPGainValue,n` | `"sp_per_kill", n, { kind = "physical", range = "melee" }`<br>`"sp_per_kill", n, { kind = "physical", range = "melee", against = { race = r } }`<br>`"sp_per_kill", n, { kind = "physical", range = "ranged" }`<br>`"sp_per_kill", n, { kind = "magic" }` | A kill by that kind of hit restores n SP. `kind` is required. | 90<br>28<br>19<br>49 |

### Drops, zeny, and experience

| rAthena | wAthena | Meaning | Uses |
|---|---|---|---|
| `bonus2 bExpAddRace,r,x`<br>`bonus2 bExpAddClass,c,x` | `"experience", x, { from = { race = r } }`<br>`"experience", x, { from = { class = c } }` | Experience from race r or class c + x%. | 295<br>12 |
| `bonus2 bDropAddRace,r,x`<br>`bonus2 bDropAddClass,c,x` | `"drop_rate", x, { from = { race = r } }`<br>`"drop_rate", x, { from = { class = c } }` | Drop rates of race r or class c + x%. | 55<br>2 |
| `bonus2 bAddMonsterDropItem,iid,n`<br>`bonus3 bAddMonsterDropItem,iid,r,n`<br>`bonus3 bAddClassDropItem,iid,c,n`<br>`bonus3 bAddMonsterIdDropItem,iid,mid,n` | `"extra_drop", { item = iid, chance = n }`<br>`"extra_drop", { item = iid, chance = n, from = { race = r } }`<br>`"extra_drop", { item = iid, chance = n, from = { class = c } }`<br>`"extra_drop", { item = iid, chance = n, from = { monster = mid } }` | A kill may drop item iid. A negative chance means -n × level / 10 + 1, as in rAthena. | 215<br>77<br>0<br>62 |
| `bonus2 bAddMonsterDropItemGroup,ig,n`<br>`bonus3 bAddMonsterDropItemGroup,ig,r,n`<br>`bonus3 bAddClassDropItemGroup,ig,c,n` | `"extra_drop", { item_group = ig, chance = n }`<br>`"extra_drop", { item_group = ig, chance = n, from = { race = r } }`<br>`"extra_drop", { item_group = ig, chance = n, from = { class = c } }` | A kill may drop an item from group ig. `extra_drop` needs exactly one of `item` and `item_group`. | 8<br>0<br>0 |
| `bonus2 bGetZenyNum,x,n`<br>`bonus2 bAddGetZenyNum,x,n` | `"gain_zeny_on_kill", x, { chance = n * 100, stacking = "highest" }`<br>`"gain_zeny_on_kill", x, { chance = n * 100 }` | A kill may give 1 to x zeny. A negative x means x × monster level. | 3<br>0 |
| `bonus bAddStealRate,n` | `"steal_chance", n` | Steal success chance + n per 10,000. An amount, not a proc. | 0 |

### Healing

| rAthena | wAthena | Meaning | Uses |
|---|---|---|---|
| `bonus bHealPower,n`<br>`bonus2 bSkillHeal,sk,n` | `"healing_done", n`<br>`"healing_done", n, { skill = sk }` | Healing from the wearer's heal skills, or from skill sk, + n%. | 359<br>7 |
| `bonus bHealPower2,n`<br>`bonus2 bSkillHeal2,sk,n` | `"healing_received", n`<br>`"healing_received", n, { skill = sk }` | Healing the wearer receives from skills, or from skill sk, + n%. A separate name from `healing_done`: the wearer is the target, not the caster. | 80<br>2 |
| `bonus bAddItemHealRate,n`<br>`bonus2 bAddItemHealRate,iid,n`<br>`bonus2 bAddItemGroupHealRate,ig,n` | `"item_hp_recovery", n`<br>`"item_hp_recovery", n, { item = iid }`<br>`"item_hp_recovery", n, { item_group = ig }` | HP from healing items, from item iid, or from group ig + n%. | 63<br>73<br>18 |
| `bonus bAddItemSPHealRate,n`<br>`bonus2 bAddItemSPHealRate,iid,n`<br>`bonus2 bAddItemGroupSPHealRate,ig,n` | `"item_sp_recovery", n`<br>`"item_sp_recovery", n, { item = iid }`<br>`"item_sp_recovery", n, { item_group = ig }` | SP from healing items, from item iid, or from group ig + n%. | 0<br>1<br>0 |
| `bonus bRestartFullRecover` | `"full_recovery_on_revive"` | Revival restores full HP and SP. | 27 |

### Equipment protection

| rAthena | wAthena | Meaning | Uses | Sign |
|---|---|---|---|---|
| `bonus bUnbreakableWeapon` | `"unbreakable_weapon"` | The weapon cannot break. | 597 | |
| `bonus bUnbreakableArmor` | `"unbreakable_armor"` | The armor cannot break. | 105 | |
| `bonus bUnbreakableHelm` | `"unbreakable_helm"` | The helm cannot break. | 149 | |
| `bonus bUnbreakableShield` | `"unbreakable_shield"` | The shield cannot break. | 21 | |
| `bonus bUnbreakableGarment` | `"unbreakable_garment"` | The garment cannot break. | 17 | |
| `bonus bUnbreakableShoes` | `"unbreakable_shoes"` | The shoes cannot break. | 11 | |
| `bonus bUnbreakable,n` | `"equipment_break_chance", -n` | The chance that any equipped item breaks changes by n%. A relative change, so it is a percent, not a chance per 10,000. | 0 | flipped |
| `bonus bUnstripableWeapon` | `"unstrippable_weapon"` | Strip skills cannot remove the weapon. | 0 | |
| `bonus bUnstripableArmor`<br>`bonus bUnstripable` | `"unstrippable_armor"`<br>The same | Strip skills cannot remove the armor. rAthena's documentation says `bUnstripable` covers all equipment, but `pc_bonus` protects only the armor. | 0<br>0 | |
| `bonus bUnstripableHelm` | `"unstrippable_helm"` | Strip skills cannot remove the helm. | 0 | |
| `bonus bUnstripableShield` | `"unstrippable_shield"` | Strip skills cannot remove the shield. | 0 | |

### Other

| rAthena | wAthena | Modes | Meaning | Uses |
|---|---|---|---|---|
| `bonus bSplashRange,n`<br>`bonus bSplashAddRange,n` | `"splash_range", n, { stacking = "highest" }`<br>`"splash_range", n` | flat | Normal attacks hit an area of radius n (1 is 3×3). | 52<br>0 |
| `bonus bNoKnockback` | `"no_knockback"` | | Skills cannot knock the wearer back. | 52 |
| `bonus bIntravision` | `"see_hidden"` | | The wearer sees hidden and cloaked characters. | 12 |
| `bonus bPerfectHide` | `"hidden_from_detectors"` | | While hidden or cloaked, monsters that detect hidden characters do not see the wearer. | 0 |
| `bonus bAddMaxWeight,n` | `"max_weight", n` | flat | Weight limit + n tenths. | 0 |
| `bFame` | none | | Not a bonus. It is a value `readparam` reads, and `pc_bonus` rejects it. | 0 |

## Names

The 127 bonus names. The `skill` bonus, which grants a skill, comes in addition.

- Stats: `str`, `agi`, `vit`, `int`, `dex`, `luk`, `all_stats`, `pow`, `sta`, `wis`, `spl`, `con`, `crt`, `all_trait_stats`, `patk`, `smatk`, `res`, `mres`, `hplus`, `crate`.
- HP, SP, and AP: `max_hp`, `max_sp`, `max_ap`, `natural_hp_recovery`, `natural_sp_recovery`, `hp_gain_over_time`, `sp_gain_over_time`, `hp_loss_over_time`, `sp_loss_over_time`, `no_hp_recovery`, `no_sp_recovery`.
- ATK and DEF: `atk`, `weapon_atk`, `refine_atk`, `matk`, `hidden_matk`, `weapon_matk`, `def`, `status_def`, `mdef`, `status_mdef`, `attack_range`.
- HIT, FLEE, and CRIT: `hit`, `flee`, `perfect_dodge`, `crit`, `crit_chance_taken`, `perfect_hit_chance`, `double_attack_chance`.
- Speed, cast, and delay: `move_speed`, `aspd`, `variable_cast_time`, `fixed_cast_time`, `after_cast_delay`, `cooldown`, `sp_cost`, `uninterruptible_cast`, `uninterruptible_cast_everywhere`, `always_endure`.
- Damage: `damage`, `damage_taken`, `crit_damage`, `non_crit_damage`, `crit_damage_taken`, `skill_ratio`, `ignore_size_penalty`, `immunity`, `damage_taken_cap`, `damage_reflected`, `reflected_damage_taken`.
- Defense: `ignore_def`, `ignore_mdef`, `ignore_res`, `ignore_mres`, `def_as_damage`.
- Elements: `attack_element`, `defense_element`.
- Procs: `cause_status`, `coma`, `block_healing`, `cast`, `drain_hp`, `drain_sp`, `remove_hp`, `remove_sp`, `deal_max_hp`, `deal_max_sp`, `set_def`, `set_mdef`, `break_weapon`, `break_armor`, `transform_monster`, `reflect_magic`, `gain_zeny_on_kill`.
- Status: `status_resistance`.
- Skills: `knockback`, `no_gemstone_cost`, `no_magic_gear_fuel_cost`.
- HP and SP from hits and kills: `hp_per_hit`, `sp_per_hit`, `hp_per_kill`, `sp_per_kill`.
- Drops and experience: `experience`, `drop_rate`, `extra_drop`, `steal_chance`.
- Healing: `healing_done`, `healing_received`, `item_hp_recovery`, `item_sp_recovery`, `full_recovery_on_revive`.
- Equipment: `unbreakable_weapon`, `unbreakable_armor`, `unbreakable_helm`, `unbreakable_shield`, `unbreakable_garment`, `unbreakable_shoes`, `equipment_break_chance`, `unstrippable_weapon`, `unstrippable_armor`, `unstrippable_helm`, `unstrippable_shield`.
- Other: `splash_range`, `no_knockback`, `see_hidden`, `hidden_from_detectors`, `max_weight`.

Types with no use in renewal data are kept. Pre-renewal data and other servers' items use some of them, and mods may want them.

## Review decisions

First round:

1. Stats use the abbreviations of rule 1 everywhere in the API, including trait stats. So `bBaseAtk` is `"atk"` and `bAtk` is `"weapon_atk"`.
2. Damage dealt and taken by percent are the `"damage"` and `"damage_taken"` bonuses with an options table, summed in rAthena's groups.
3. Reductions flip their sign. A positive value raises the named quantity.
4. Chances are whole numbers per 10,000.
5. Trigger flags go in the options table.
6. rAthena's "monster race" is `monster_group`, with `wa.monster_groups`.
7. The draft's hard names are accepted, with the abbreviations of rule 1: `status_def`, `misc`, `def_as_damage`, `remove_hp`, `always_endure`, and `see_hidden`.
8. Unused types are kept.
9. The damage object has `source` and `target`, and the options have `against` and `from`.
10. Approved API additions: the `"damage"` event (its `kind` field replaces `is_weapon`), `player:add_timed_bonuses`, the `restart` option of `timers.with_bonuses`, `wa.weapon_types`, `wa.monster_groups`, and `items.group`.

Second round:

11. A `"skill_used"` item event fires for every skill use, including skills that deal no damage. `autobonus3` maps onto it.
12. Bonuses where rAthena keeps the last value use `stacking = "last"` by default, which follows rAthena's order. A mod can use `{ stacking = "priority", priority = n }`. `"highest"` and `"lowest"` stay.
13. No name starts with `chance_to_`. The name is the effect, and the chance goes in the `chance` option, with `on`, `against`, and `from`.
14. No name ends in `_percent`. Flat and percent share a name, with `{ percent = true }`. Names that rAthena has only as a percent require it; names that rAthena has only as flat refuse it.
15. Per-skill and per-item variants are options (`skill`, `item`, `item_group`) on the shared name. Separate names stay only where the meaning differs, and the table says why.
16. Arguments have a fixed order: what the bonus produces or applies to, then the amount, then other numbers, then the options table. The chance is always an option.
17. The converter maps every rAthena bonus form to a call in this document.

## Open points

1. **rAthena's order is not equip time.** `status_calc_pc` runs item scripts in a fixed order: equipment by slot, then combos, then cards. So `"last"` follows rAthena without depending on when items were equipped. Strategy 5.3 rule 4 still holds. The only order wAthena adds is that of timed bonuses, by the time they started.
2. **Misses.** rAthena's autobonus runs from `skill_additional_effect`, after a hit. This document says `"damage"` does not fire on a miss. The parity tests should confirm it.
3. **Two percents that are not chances.** `crit_chance_taken` and `equipment_break_chance` change a chance by a relative percent. They stay percents, not per 10,000.
4. **The `job` key.** rAthena reads the monster ID of `bAddDamageClass` and similar as a job ID when the other side is a player. wAthena splits this into the `monster` and `job` keys. The converter writes `monster`, since renewal data uses monster IDs there.
5. **`skill_used` timing.** This document fires `"skill_used"` once per use, before its hits, where rAthena runs `skill_onskillusage`. The parity tests should confirm the order against `"damage"`.
