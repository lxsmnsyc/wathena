# 0019: Ruleset Flags

- Status: proposed.
- Date: 2026-10-04.
- Code: [server/rules](../../server/rules), [server/gamedata](../../server/gamedata) (`ruleset.go`), [server/battle](../../server/battle), [server/skill](../../server/skill), [server/status](../../server/status), [server/statuschange](../../server/statuschange), [server/progression](../../server/progression) (`penalty.go`), [server/zone](../../server/zone), [tools/modkit](../../tools/modkit), [mods/examples/classic-casting](../../mods/examples/classic-casting).
- Builds on strategy section 3.6, [decision 0009](0009-stats-as-totals-of-modifiers.md), [decision 0010](0010-status-effects.md), [decision 0011](0011-skills.md) and [decision 0014](0014-rolled-items.md).

## Decision

- `server/rules` holds a world's ruleset flags as the game code reads them: one field per flag, `true` for the renewal value or on, and `Skills` for the `skill_rebalance.<skill>` flags by skill ID. `rules.Base` gives a core ruleset. `Skill(id)` gives a skill's flag, or the base when the ruleset does not set it.
- `gamedata.RulesOf` turns the loader's ruleset into rules. `LoadMods` then applies them:
  - The job tables get `aspd`, `size_table`, and the flags that player stats read (`ApplyRuleset`).
  - The exp and drop tables get the level penalty flags.
  - The skill database gets the flags that change skill data, before mods' overlays.
  - Every zone gets the rules in `zone.Mods.Rules`. A zone without them follows the core ruleset of its base.
- Each place where rAthena checks `RENEWAL`, `RENEWAL_CAST`, `RENEWAL_EXP`, `RENEWAL_DROP`, `RENEWAL_LVDMG`, `RENEWAL_ASPD` or `RENEWAL_STAT` reads the flag of its feature. The base stays for what section 3.6 puts in the base: the ATK, MATK and DEF models, HIT, FLEE and critical, card order, refining, classes, equipment rules, monster stats and the data sets.
- The battle code takes the rules with the attack: `battle.AttackRules` for normal attacks, and `battle.Skill.Rules` with `battle.SkillAttackRules` for skills. The base picks the damage pipeline. The flags pick the parts each pipeline reads.

## What each flag changes

| Flag | Where | What it changes |
|---|---|---|
| `cast_time` | `skill.CastTimeFor`, `MonsterCastTime`, `status` cast bonuses, the zone | `skill_castfix` with DEX, or `skill_vfcastfix` with fixed and variable cast time. Chase Walk and Charge Attack's distance rule. `bCastrate` and the fixed cast bonuses of `pc_bonus`. On a pre-renewal base, a skill without `FixedCastTime` gets `default_fixed_castrate`, 20%. |
| `skill_cooldown` | skill data | Every skill takes the cooldown of the flag's mode's `skill_db`. A skill that mode does not have keeps its own. |
| `stat_point_cost` | `status` | `PC_STATUS_POINT_COST` of `RENEWAL_STAT`. The two formulas agree below 100, so it only matters for stats above 99. |
| `level_penalty_exp`, `level_penalty_drop` | `progression` | The exp and drop penalty by level gap. A pre-renewal base takes renewal's `level_penalty.yml`. Off gives 100%. |
| `level_damage_mod` | `skill` | `RE_LVL_DMOD`: the renewal versions of skills scale with base level above 99. |
| `element_table` | `battle`, the zone | `battle_attr_fix` with its table and its status bonuses. |
| `status_resist` | `statuschange` | `status_get_sc_def`: chances, durations and caps. |
| `heal_formula` | `skill.CalcHeal`, the zone | `skill_calc_heal`, and the `RENEWAL` parts of `pc_itemheal` (Apple of Idun, Asura Strike). |
| `boss_protection` | the zone | Ankle Snare and Skid Trap do not catch bosses, and Ankle Snare holds a boss a fifth as long with the pre-renewal value. Blade Stop holds a boss for 2 seconds, or not at all. |
| `ensemble_songs` | `statuschange`, `status`, `skill`, the zone, skill data | Songs as renewal's self and party buffs or pre-renewal's ground units, ensemble fatigue, Longing for Freedom, the dance SP timers, the song statuses' values and stats, Siegfried's resistance, the lessons' SP, and the SP cuts of Adaptation and Ring of Nibelungen. The songs and their skills (`rules.SongSkills`) take their `skill_db` records from the flag's mode. |
| `busy_restrictions` | the zone | On: an item, a skill or an NPC click refused while talking to an NPC sends the busy message (`MSI_BUSY`), and a player casting a skill below the third class skills cannot click an NPC. Off: these fail with no message, as pre-renewal. |
| `fourth_job_traits` | `status` | Off: no trait stats from jobs, equipment or skills, and no P.ATK, S.MATK, RES, MRES, H.PLUS or C.RATE. |
| `skill_rebalance.<skill>` | `skill`, `status`, `statuschange`, `battle`, the zone, skill data | The skill's own code: its `SkillImpl` ratio and effects, its `skill.cpp` and `unit.cpp` parts, the start values and stat effects of its own status, and the checks other code makes for that status. The skill also takes its `skill_db` record from the flag's mode. The damage steps that the battle pipeline shares stay with the base, as section 3.6's "Base ATK model for damage skills" says. |
| `homunculus` | nothing yet | There is no homunculus system. |

`aspd`, `size_table` and `item_rolls` work as before, now through the same rules.

## The skills with a skill_rebalance flag

`rules.RebalancedSkills` lists the 94 skills whose `RENEWAL` branches are ported. They come from rAthena's `#ifdef RENEWAL` blocks in `skill.cpp`, `battle.cpp`, `status.cpp`, `unit.cpp`, `pc.cpp` and `src/map/skills/`:

- Swordman line: `SM_MAGNUM`, `KN_TWOHANDQUICKEN`, `KN_BRANDISHSPEAR`, `KN_BOWLINGBASH`, `KN_CHARGEATK`, `LK_SPIRALPIERCE`, `LK_CONCENTRATION`, `CR_AUTOGUARD`, `CR_SHIELDBOOMERANG`, `CR_HOLYCROSS`, `CR_SHRINK`, `CR_SPEARQUICKEN`, `PA_PRESSURE`, `PA_SHIELDCHAIN`.
- Mage line: `MG_SAFETYWALL`, `MG_FIREBALL`, `MG_THUNDERSTORM`, `MG_ENERGYCOAT`, `WZ_METEOR`, `WZ_VERMILION`, `WZ_FROSTNOVA`, `WZ_STORMGUST`, `WZ_EARTHSPIKE`, `WZ_HEAVENDRIVE`, `WZ_SIGHTBLASTER`, `HW_NAPALMVULCAN`, `HW_GRAVITATION`, `HW_MAGICPOWER`, `SA_MAGICROD`, `SA_SPELLBREAKER`, `SA_AUTOSPELL`, `SA_FLAMELAUNCHER`, `SA_FROSTWEAPON`, `SA_LIGHTNINGLOADER`, `SA_SEISMICWEAPON`, `SA_VOLCANO`, `SA_DELUGE`, `SA_VIOLENTGALE`, `PF_SOULCHANGE`.
- Archer line, without songs: `AC_SHOWER`, `HT_SKIDTRAP`, `HT_LANDMINE`, `HT_BLASTMINE`, `HT_CLAYMORETRAP`, `HT_BLITZBEAT`, `HT_PHANTASMIC`, `SN_FALCONASSAULT`, `SN_SHARPSHOOTING`, `BA_MUSICALSTRIKE`, `DC_THROWARROW`, `BA_PANGVOICE`, `DC_WINKCHARM`, `CG_ARROWVULCAN`, `CG_TAROTCARD`.
- Acolyte line: `AL_ANGELUS`, `AL_BLESSING`, `PR_IMPOSITIO`, `PR_SUFFRAGIUM`, `PR_TURNUNDEAD`, `PR_MAGNUS`, `PR_REDEMPTIO`, `HP_ASSUMPTIO`, `HP_BASILICA`, `MO_TRIPLEATTACK`, `MO_INVESTIGATE`, `MO_FINGEROFFENSIVE`, `MO_STEELBODY`, `MO_EXPLOSIONSPIRITS`, `MO_EXTREMITYFIST`, `MO_CHAINCOMBO`, `MO_COMBOFINISH`, `MO_BALKYOUNG`, `CH_PALMSTRIKE`, `CH_TIGERFIST`, `CH_CHAINCRUSH`.
- Merchant line: `MC_MAMMONITE`, `MC_LOUD`, `BS_ADRENALINE`, `BS_OVERTHRUST`, `WS_OVERTHRUSTMAX`, `WS_CARTTERMINATION`, `AM_DEMONSTRATION`, `AM_ACIDTERROR`, `CR_ACIDDEMONSTRATION`.
- Thief line: `TF_STEAL`, `AS_SONICBLOW`, `AS_SPLASHER`, `AS_VENOMKNIFE`, `ASC_EDP`, `ASC_BREAKER`, `ASC_METEORASSAULT`, `RG_BACKSTAP`, `RG_RAID`, `RG_CLOSECONFINE`.

Rules for the flag:

- An unknown skill is a load error.
- A song or ensemble is a load error: `ensemble_songs` covers them.
- A skill that is not in the list loads with a warning. Only its `skill_db` record changes.

## Why the songs and rebalanced skills take their records

The two modes' `skill_db` records differ for these skills in shape, not only in numbers. Pre-renewal's Whistle and Basilica have ground units, and renewal's have none. Code of one mode with the record of the other would cast nothing. So the flag that picks a skill's code also picks its record. The cooldowns stay with `skill_cooldown`.

## Why status values follow the skill's flag

A skill's status carries the skill's effect: Suffragium's cast reduction, Assumptio's DEF or damage cut, Magnum Break's fire bonus. When only the skill's code changed, a pre-renewal Basilica would start a status that renewal's stat code reads as a damage bonus. So `status`, `statuschange` and `battle_calc_damage` read the skill's flag for that status. The stat formulas around them stay with the base.

## Other choices

- `skill_cooldown: off` means pre-renewal's cooldowns, not none. rAthena's pre-renewal `skill_db` has cooldowns for 175 skills, such as Provoke, and `core.ruleset.pre_renewal` must keep them.
- `stat_point_cost` is the cost formula only. The status points per level of `statpoint.yml` stay with the base's data.
- Settings of `conf/battle` whose defaults depend on the mode but have no flag, such as `traps_setting` and `alchemist_summon_setting`, stay with the base. So does the monsters' cast sensor for instant casts in `unit_skilluse_id2`.
- `status_change_start`'s delay rule and `skill_toggle_magicpower`'s placement in the cast follow the base and `HW_MAGICPOWER`'s flag.
- `busy_restrictions` changes behavior on a renewal base: renewal now refuses an NPC click during a cast and sends the busy message, as rAthena. Before this step the zone did neither.
- `zone.Mods.Rules` carries the rules, next to `ItemRolls`, since a ruleset is a mod. Golden zones without mods follow their base.

## Tests

- Each flag has tests at both values on both bases where the loader allows it. `TestEveryFlagBothValues` in `server/mod` fails for a flag with no behavior test in its list, unless it names a reason (`homunculus`). Expected values come from the other mode's recording or from rAthena's formulas:
  - `cast_time`: pre-renewal's cast time on renewal's skill data gives the pre-renewal recording for the 2,607 cases whose skill has the same cast time in both modes. At 150 DEX every spell casts at once and there is no fixed cast. Renewal's cast time on pre-renewal's data leaves 20% of the cast time at full DEX and INT.
  - `skill_cooldown`: each base with each value gives the cooldowns of that value's recording, 6,075 to 6,096 cases each.
  - `element_table`: every cell of both `elements.json` on both bases.
  - `status_resist`: both modes' `status_effects.json` match in full, 2,640 of 2,640, with either base's status database.
  - `level_penalty_exp`, `level_penalty_drop`: `level_penalty.json` on both bases, 642 cases, and 100% when off.
  - `heal_formula`: both recordings' Heal amounts on both bases, and a cast of Heal in a zone.
  - `stat_point_cost`, `fourth_job_traits`, `level_damage_mod`, `boss_protection`, `ensemble_songs`, `busy_restrictions` and `skill_rebalance`: rAthena's formulas and behavior in hand tests.
- `rules/rulestest.Mixes` makes 8 mixed rulesets per base from a fixed seed. Each sets every flag at random and flips about a quarter of the rebalanced skills. Each package runs its recordings for the mixes whose flags keep the recording's values, through the mix's code paths:
  - `battle`: `mob_damage.json` and `pc_damage.json`.
  - `skill`: `skill_casting.json` by part, and the forced minimum of `skill_damage.json`, 6,781 cases.
  - `status`: `pc_status.json`.
  - `statuschange`: `status_effects.json`.
  - `progression`: `level_penalty.json`, for every mix.
  - `zone`: `skill_status.json` by part, for the first two mixes of each base that run cases.
  - `gamedata`: each mix loads through the mod loader into the same rules, and its stats, damage, cast times, exp and drops equal those of the flags they read.
- The core rulesets match every recording as before.

## Sample mod

[examples/classic-casting](../../mods/examples/classic-casting) extends `core.ruleset.renewal` with pre-renewal cast time, element table, cooldowns and Mystical Amplification. Its Lua test checks that fire hurts water armor a third as much as earth armor. `TestClassicCasting` checks the rules, an instant cast at 150 DEX, no damage to a water 3 monster, and Angelus's cooldown, against `core.ruleset.renewal`. `wactl mod test` loads a ruleset mod as the world's ruleset, on its own base.

## Left

- `homunculus`: no homunculus system exists. Its rAthena sites are `SC_HOMUN_TIME`, homunculus ASPD and stats, the exp share of `getexp`, and `AM_CALLHOMUN`'s items.
- `boss_protection`'s endure: bosses that damage does not stop (`status_isendure`). The zone does not stop units on hits yet.
- `busy_restrictions`'s cart change: there is no request to change the cart's look yet.
- The party exp share of `RENEWAL_EXP` waits for parties (phase 5 step 4).
- A rebalanced skill whose other version needs a status the base's `status.yml` lacks loses that part, such as `SC_RAID` for `RG_RAID` on a pre-renewal base. Longing for Freedom exists only in pre-renewal's data, so a renewal base with pre-renewal songs cannot learn it.
- The `RENEWAL` sites of skills that are not ported, such as the Ninja, Gunslinger and third class skills, wait for those skills.
- Scripts cannot read the flags: `rules.is` of section 3.6 needs Lua API, which this step does not add.
