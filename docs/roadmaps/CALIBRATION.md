# Calibration

One line per finished node: date, roadmap/node, estimated size against actual, tokens (worker, review and fix rounds), and what moved the estimate.

2026-11-05 v0.2-parity/npc-api-events est L actual M, tokens not measured: one session, PR #9 open to merge in 4 minutes.
2026-11-05 v0.2-parity/extract-folders est M actual M, tokens not measured: mostly moved files (+7152 -444), PR #10.
2026-11-05 v0.2-parity/npc-api-timers est M actual M, ~375k tokens: one session, PR #11.
2026-11-05 v0.2-parity/converter-names est M actual M, ~556k tokens: one session, PR #12.
2026-11-06 v0.2-parity/converter-commands est L actual L, ~546k tokens: one session; the review found countitem2 rentals and delitem2 not ending the script, fixed before merge (PR #13). Plain delitem had the same bug and got a separate fix PR.

## Checkpoint 2026-11-06 (v0.2-parity)

- 5 code nodes merged between 2026-11-05 10:40 and 2026-11-06 00:55 UTC: about 2 active days for 5 nodes, far above the stop rule of 1 node per 3 active days. No stop point applies.
- Measured nodes: M ~375k to ~556k tokens, L ~546k tokens, including the fresh review.
- The must line after converter-commands: 26 must nodes left, placed by wave 27 in sequential mode (`plan.py status --max-parallel 1`). At the measured pace of about 2.5 nodes per active day, that is about 11 active days, more for the L nodes of instances, sieges and companions.
- Check runs: `scripts/check.sh quick` on this 8 GB machine took 18 to 48 minutes and was often killed for memory; with one test binary and two parallel tests at a time (PR #15) the unit step takes 630 s uncached.
