# Desperado

A sample mod for job definitions of proposal 0002, review decision 24 (mods/README.md, "Jobs"). It adds a new job and a new skill, the phase 4 exit criterion.

- `data/jobs.yml` adds the Desperado, a second job that a Gunslinger at job level 40 may change into. It shows the Gunslinger's sprites. Its fields that the file leaves out come from the Gunslinger.
- `data/skills.yml` adds Quick Draw, two shots at one enemy, and `data/skill_trees.yml` lets Desperados learn it once Single Action is at level 3.
- `data/items.yml` adds the Desperado's Hat, whose `jobs` field names the new job.
- The Desperado Mentor in Prontera changes jobs with `player:change_job`, which follows the job's `changes_from`.
- The entry script listens to the `job_change` event and gives each new Desperado a hat.

Run `wactl mod test mods/examples/desperado`. `TestDesperado` in `tools/modkit` checks in Go what a test cannot read: the Desperado's skill list and a cast of Quick Draw.
