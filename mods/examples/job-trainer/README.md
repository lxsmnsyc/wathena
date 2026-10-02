# Job Trainer

A sample mod for the job change and reset calls of proposal 0002, review decision 21 (mods/README.md, "Job changes and resets").

- The trainer in Prontera lowers one skill of the player's first class with `player:reset_skill`. When other learned skills need it, it asks, and lowers them too with `with_dependents = true`. It lowers one stat to 1 with `player:reset_stat`. Each lesson costs 500 zeny, which comes back when nothing changes.
- Its tests also walk core's Job Master: a Novice to a Swordman, a Swordman to a Knight, a female Archer to a Dancer, and a Knight's rebirth into the Lord Knight line with `player:change_job` and `jobs.next_jobs`.

Run `wactl mod test mods/examples/job-trainer`.
