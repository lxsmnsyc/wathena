# 0013: Monster AI and Monster Skills

- Status: proposed.
- Date: 2026-10-03.
- Code: [server/mob](../../server/mob) (data, stats, AI modes and states, skills lists), [server/zone](../../server/zone) (`mobai.go`, `mobskills.go`, `skills_npc*.go`, `monsterscripts.go`), [server/gamedata](../../server/gamedata) (`gamedata.go`, `world.go`), [server/modscript](../../server/modscript) (`monsters.go`), [tools/convert](../../tools/convert) (`monster.go`), [mods/examples/wolf-pack](../../mods/examples/wolf-pack).
- Builds on [decision 0011](0011-skills.md), [decision 0012](0012-second-classes-and-job-changes.md), and [proposal 0002](../proposals/0002-lua-api.md), review decision 22.

## Decision

- Monster stats come from data, as rAthena's `status_calc_mob`. `mob.Calc` ports `status_calc_misc` for monsters in both modes. A monster record has the base stats and `attack`; hit, flee, defense from stats, magic attack and base attack are computed. The Emperium gets `status_calc_misc` twice, as rAthena does.
- The data of every monster is the extract `monsters.json`, which keeps rAthena's raw values. `mob_status.json` stays as the parity check.
- The package `mob` holds the rules that need no map: the AI types as modes, the AI states and their words, the skills list with its conditions and targets, and the stats. The zone holds the AI that needs the map, in files named after rAthena's code:
  - `mobai.go` ports `mob_ai_sub_hard`, `mob_ai_sub_lazy`, the target search, chase, assist, loot, target change, retaliation, slaves, random walk and `unit_walktobl`.
  - `mobskills.go` ports `mobskill_use`, `mobskill_event`, the conditions, the targets, chat lines, dead skills and the cast sensor.
  - `skills_npc.go` and the other `skills_npc_*.go` files port the `NPC_` skills that only monsters use. Monsters also use player skills through the skill framework of decision 0011.
- The AI runs on rAthena's timers. A monster within a player's view plus 4 cells runs the hard AI every 100 ms. Other monsters run the lazy AI every second. The settings are rAthena's shipped `conf/battle` values, with `monster_ai` 0.
- A hit reaches the AI after the damage shows and the hit's walk delay passes, as rAthena's `mob_attacked` timer. Retaliation, rude attacks and the `hit_melee`, `hit_ranged` and `hit_by_skill` conditions all start there.
- `ai` in a monster record is rAthena's AI type, 1 to 27. `modes` adds to the modes of the AI type, so a record can be read as "AI type 4, plus can loot".
- The skills list keeps rAthena's rows one to one, with readable names. The converter writes it to core's `data/monsters.yml`. Two parts of a row have no field in review decision 22, so they are dropped and counted in `docs/generated/monster-skills-<mode>.md`: the mode change of `NPC_EMOTION` and `NPC_EMOTION_ON`, and the second emotion.
- The Lua API is review decision 22: monster handle fields and methods, the `monster_target` hook, and monster AI scripts.
  - The hook runs where rAthena picks a target: the aggressive search, an assist, a hit, and a target change. `current` is rAthena's choice. Without hooks the choice is rAthena's.
  - A monster's AI script runs once for each spawned monster, in a context of kind `"monster"`. The context ends when the monster dies, despawns, or its mod reloads.
  - `walk_to` sets rAthena's `force_walk`: the AI waits until the walk ends.
  - `use_skill` casts with the skill's own cast time, as `unitskilluseid`. No delay of the skills list applies.

## Why AI types become modes

| | A field per behavior | AI type plus modes (chosen) |
|---|---|---|
| Converting `mob_db.yml` | Each type expands to many fields. | One number, as rAthena. |
| Reading a record | Long. | Short, with the type's table in the docs. |
| A mod that wants one change | Sets one field. | Adds one mode to `modes`. |
| Parity | Same. | Same. |

## Other choices

- Parity runs on a walled 400 by 400 grid with an open area around the recorded cell. Teleports fail there as on `prt_fild08`, and path search stays fast.
- Monster skill parity checks the chosen entry, its target and cast time in every case, and the effects of every used skill. The effects that do not match are listed per skill in the test log.
- Lines of `mob_chat_db.yml` are sent as the monster's chat, after its name. The line's color is not sent; the protocol has no field for it.
- A class change (`NPC_METAMORPHOSIS`, `NPC_TRANSFORMATION`) gives full HP and a new damage log, as `monster_class_change_full_recover`. The client sees the monster leave and come back as the new kind.
- MVP rewards follow `mob_dead`: MVP exp and the MVP drops go to the player that did the most damage, with the MVP effect.

## Left

- MVP tombs, monster level ups, guardians, and the warp chase of monsters that follow a player through a warp.
- The mode change and second emotion of `NPC_EMOTION`, which need a field in the skills list. This needs review.
- `NPC_` skills without a port. `docs/generated/monster-skills-<mode>.md` lists the rows per skill.
- AI for summoned monsters beyond rAthena's defaults, and NPC AI hooks, which review decision 22 leaves for later.
