# 0012: Second Class Skills and Job Changes

- Status: proposed.
- Date: 2026-10-03.
- Code: [server/skill](../../server/skill) (one file per job), [server/zone](../../server/zone) (`skills_<job>.go`, `jobchange.go`, `skilltimers.go`, `breakequip.go`), [server/statuschange](../../server/statuschange), [server/status](../../server/status), [server/modscript](../../server/modscript) (`jobs.go`), [server/store](../../server/store) (migration 00012), [tools/convert](../../tools/convert) (`core.go`, `core/scripts/npcs/job_master.lua`), [mods/examples/job-trainer](../../mods/examples/job-trainer).
- Builds on [decision 0011](0011-skills.md), [decision 0010](0010-status-effects.md), and [proposal 0002](../proposals/0002-lua-api.md), review decision 21.

## Decision

- Second and transcendent class skills follow decision 0011. Each job has its formulas in `skill/<job>.go` and its behaviors in `zone/skills_<job>.go`. Systems that several jobs share live once, in the files of the zone that rAthena's own code maps to:
  - `skilltimers.go` ports `skill_addtimerskill` and `skill_timerskill`. A skill's behavior can handle its own timer. Without one, the timer runs `skill_attack`.
  - `breakequip.go` ports `skill_break_equip`.
  - Riding, the falcon and the cart are the option bits of `status.option`, saved with the character. `setOptions` ports `pc_setoption` for them.
- A status recalculates only what its flags name, as `status_calc_bl`. Of what the server computes, only ASPD can differ from a full `status_calc_pc`: renewal's ASPD reads Musical Lesson and Plagiarism only while a status is active. So the zone keeps the old ASPD when a status without `SCB_ASPD`, `SCB_AGI`, `SCB_DEX` or `SCB_BASE` changes the stats.
- Job changes are the calls of review decision 21. They port `pc_jobchange`, `pc_resetlvl`, `pc_resetskill` and `pc_resetstate`, and the rules of the rebirth NPC (`npc/jobs/valkyrie.txt`).
  - `jobs.next_jobs` lists the jobs a job can change into: a Novice into a first class or a Super Novice, a first class into its two second classes, and a High Novice into a transcendent first class.
  - Without `force`, `player:change_job` allows only those jobs, and a transcendent player stays in the line it had before its rebirth. `force` needs the `player.change_job` permission.
  - A male Dancer becomes a Bard, and a female Bard a Dancer, as `pc_jobchange`.
  - `player:reset_skill` lowers one skill. When other skills need it, it changes nothing unless `with_dependents` is set; then the dependents go to 0 first.
  - `player:reset_stat` refunds the points of rAthena's cost table (`pc_need_status_point`).
  - Quest skills stay in every reset, as `pc_resetskill` without `RESET_SKILL_ALL`.
- Migration 00012 saves three values that rAthena keeps on the character: `job_change_level` (`change_level_2nd`), which the skill tree reads; `rebirth_job`, the second class before a rebirth, which picks the transcendent line (rAthena reads it from the `ADVJOB` variable of the job NPCs); and `options`, the saved option bits.
- rAthena's job change quests convert with `tools/convert` like other NPCs: `jobchange` becomes `player:change_job` with `force`, and `resetlvl`, `resetskill` and `resetstatus` become the reset calls. Many quests use commands that do not convert yet, so core also places a Job Master in Prontera. It follows rAthena's level rules and uses only the public API.

## Why force needs a permission

| | `change_job` without a permission | `force` with `player.change_job` |
|---|---|---|
| A quest NPC that follows the job tree | Works. | Not needed. |
| A converted rAthena quest, which calls `jobchange` with any job | Fails for jobs outside the tree. | Works once the owner approves it. |
| A mod that hands out any job | Cannot. | Needs the owner's approval, as `world.announce` does. |

## Other choices

- Skills that need a window, a homunculus, or a family fail before they start, as in decision 0011: Repair Weapon, Weapon Refine, Hindsight (for players), Abracadabra, Create Elemental Converter, Prepare Potion, Call Homunculus, Vaporize, Resurrect Homunculus and Call Baby.
- Party skills reach the caster alone. Ensembles, Devotion and Redemptio need a party member, so they fail, as rAthena does for a player without a party. Their effects are ported for when parties come.
- Plant summons (Bio Cannibalize, Sphere Mine) stand still until monster AI exists. They keep rAthena's master, timer and kill rules.
- A broken item comes off and cannot be worn again in the session. The broken flag is not saved yet.
- Three skills check their recordings in their own golden tests, since the shared harness cannot reproduce one detail of each: Spiral Pierce's weapon script, a Shield Chain hit count that is a statistical outlier, and the Golem in Eternal Chaos's area.

## Left

- Parties, for party skills, ensembles, Devotion, Redemptio's party resurrection and the Blacksmith buffs.
- Homunculi, cart items, forging and brewing windows, Abracadabra's skill list, Hindsight's menu, and sitting.
- Monster AI for summons, and monster skills.
- Protocol fields for spirit sphere counts, Blade Stop, Sense's monster window, Ice Wall cells, text units (Graffiti, Talkie Box) and broken items.
- Lua API to give a Peco Peco, a falcon or a cart from a script, which needs a review first.
- The client's effects for second class skills.
