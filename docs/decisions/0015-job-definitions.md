# 0015: Job Definitions

- Status: proposed.
- Date: 2026-10-03.
- Code: [server/gamedata](../../server/gamedata) (`jobs.go`), [server/status](../../server/status) (`jobinfo.go`), [server/zone](../../server/zone) (`jobchange.go`), [server/item](../../server/item) (`wear.go`), [server/skill](../../server/skill) (`tree.go`), [server/modscript](../../server/modscript) (`jobs.id`, the `job_change` event), [mods/examples/desperado](../../mods/examples/desperado).
- Builds on [decision 0012](0012-second-classes-and-job-changes.md), [decision 0011](0011-skills.md), and [proposal 0002](../proposals/0002-lua-api.md), review decision 24.

## Decision

- Jobs are records of `data/jobs.yml`, with the fields of review decision 24. Core's jobs are records too. The server converts them when it starts, from four extracts per mode:
  - `job_stats.json`: rAthena's `job_stats.yml`, `job_aspd.yml`, `job_basepoints.yml` and `job_exp.yml` merged per job. It was already extracted, so the rAthena fork needed no change.
  - `progression.json`: the exp tables.
  - `skill_trees.json`: the `Inherit` lists.
  - `script_constants.json`: the names of `wa.jobs`.
- The job tables, the exp tables, `jobs.next_jobs`, `player:change_job` and the skill trees of new jobs read the records. Nothing reads `job_stats.json` directly once the mods load.
- Core's records keep rAthena's map job IDs (`pc_jobid2mapid`). The tier, `transcendent`, `baby` and `base_job` of a core record follow from them. `changes_from` holds the rules that decision 0012 coded in Go: a Novice to a first class or the Super Novice, a High Novice to a transcendent first class, and a first class to its two second classes.
- A mod job gets an ID from 10,000 and a map job ID made from its look's class:
  - The same tier as the look keeps the look's class. `expanded` keeps it too.
  - `novice`, `first`, `second`, `third` and `fourth` keep the look's first job line and set the bits of that tier. A second job without second job bits gets the 2-1 bit.
  - `transcendent` and `baby` set their bits.
- So rAthena's rules that read the class treat a mod job as that class, such as the HP formula of its line, dual wielding, and the default maximum stats. `pc_mapid2jobid` keeps core's jobs only: a mod job keeps its own job bonuses, weight, maximum stats and skill tree, and a change into it keeps its ID, without the Bard and Dancer swap.
- A field that a new job leaves out comes from its look, as review decision 24 says. `display_name` is the exception: it defaults to the record's name without the mod prefix, as for new monsters, since the look's name would name another job.
- `changes_from` entries take optional `base_level` and `job_level`, which `player:change_job` checks without `force`. Core's records have no levels, since rAthena's `jobchange` checks none and core's Job Master checks rAthena's levels itself.
- On core's jobs, `tier`, `transcendent`, `baby`, `base_job` and `inherits` cannot change: they come from rAthena's class and trees. `look` on a core job changes only its sprites.
- A new job's skill tree starts with the trees of its `inherits`, in order, as `SkillTreeDatabase` merges `Inherit`. Entries of `data/skill_trees.yml` for a job reach the new jobs that inherit it.
- An item's `jobs` field also takes job names. A core job's name sets the bits of rAthena's `Jobs` for its class, as `itemdb_jobid2mapid`, so `knight` covers Lord Knight and Rune Knight unless `classes` leaves them out. A mod job's name lets that job wear the item whatever its class. `equips_as` makes the job and class checks read another job's class.
- The protocol gained `JobName`, the record's `display_name`, sent after login and after each change. The look's body is the look job's.
- The `job_change` event fires after `pc_jobchange`'s work, with `{ player, from, to }`, in mod and event contexts.

## Other parts of review decision 24

- Monster skill entries got `modes`, a list of two for `emotion`, and `chat_color` as 0xRRGGBB, rAthena's red by default. `NPC_EMOTION` and `NPC_EMOTION_ON` change the mode through `SC_MODECHANGE`, as `status_calc_mode`. Every `mob_skill_db` row now converts in full: 14,514 in renewal and 5,494 in pre-renewal.
- `wa.testing` gained `world:equip(player, item, { roll })`.
- Rolls take an optional `display_name`, which the client shows in place of the words of the bonus.
- Manifests declare `api: 1.0`. `api: experimental` is refused with a message, and a range must include 1.0.

## Other choices

- Core's records hold the extract's HP and SP rows as they are, with the formula for the levels between them, so the tables match rAthena exactly. A mod's `hp_table` must cover every base level.
- `aspd` merges by weapon type: a type the record leaves out keeps the look's value. With the `aspd` flag away from the base, a mod job reads its look's table of the other mode.
- The exp tables must cover `max_base_level` and `max_job_level`, so a job never reaches a level whose exp is 0.

## Left

- Mounts, falcons and carts for new jobs, which wait for a general design of companions.
- Mod job names in the account pages' character list, which still shows the job ID.
- A character whose mod job was removed fails to load. A fallback to its look job needs a review.
- A skill-up limit for a mod job: while it has spent too few points, it sees the tree of its class's earlier core job, as rAthena's normalization does for core jobs.
