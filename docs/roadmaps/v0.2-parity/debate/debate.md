---
goal: Tag v0.2.0 with 1:1 rAthena parity up to transcendent classes plus instances, sieges, companions and small systems, at a cost the sequential 8 GB machine and the maintainer's time can sustain
date: 2026-11-05
rounds: 2 of 2
status: approved
roadmap: docs/roadmaps/v0.2-parity
supersedes: none
decision: none
---

# Debate: v0.2.0 scope re-evaluation

The full ledger is in [ledger.json](ledger.json), and the raw measurements are in [evidence/](evidence/).

## Settled structure

The roadmap stays mostly as it is. Two nodes change, the order changes, and a checkpoint is added.

1. Park item-windows to v0.2.x. A new decision record refuses its 3 command rows and 130 items with a reason, so the acceptance bar ("converted or refused with a reason") still holds. (O1, P1)
2. Merge npc-behavior-spike into npc-behavior-harness. The result is one L node with two phases and a checkpoint in log.md between them. The scope is fixed:
   - The harness replays only the scripts the driver reaches with default inputs: next, the first menu choice, close, and at most 2 menu paths per script.
   - It compares dialog lines, items and zeny, variables, warps and events.
   - The report lists every script as compared, undriven (with the reason) or different (with the explanation). "No unexplained difference" applies to compared scripts. Undriven scripts are counted per folder.
   - Stop rule: if the 3-script sample in phase 1 needs more than about 400 lines of C++ in the recorder, the node falls back to comparing dialog lines and warps only. (O2 as amended by P2)
3. Order the must line so that v0.2.0 can be tagged at three points:
   - first the acceptance-critical nodes: converters, extract-folders, npc-api-timers, window-skills, monster-gaps, pre-renewal-parity and the item effect nodes
   - then instances, sieges and companions
   - then the small systems: chat-rooms, marriage, ui-windows, achievements and navigation
   - last the harness, the security review and the release

   When a stop point is reached and the date matters, the remaining small-system nodes may become `should` and ship in v0.2.x, with their rows refused by a decision record. Each such move needs the maintainer's decision at that point. (P5)
4. Checkpoint: after 5 more merged code nodes, record the actuals and recompute the length of the must line. If the pace is below 1 node per 3 active days, apply the `should` rule from step 3. (P3)
5. Checks stay at the current practice: targeted tests, one `check.sh quick queries` run per PR, and `check.sh all` only on the tag. The harness replays rAthena recordings stored as files, so the C++ fork is built once. (P6, J8)

Net change: the must line goes from 33 nodes to 31 (one L parked, one M merged away).

## Budget

- device: one build at a time, at least 10 GB free. Today 13 GiB is free (J4), so run `docker builder prune` / `docker volume prune` before each check. One rAthena fork build, for the harness. (J4, J8, P6)
- time: no date is set (J5). About 31 active work days at the 10-31 pace of one node per active day; about 10 weeks at 3 active days a week. These numbers are estimates until the step 4 checkpoint. (P3, O3)
- tokens: no actuals are recorded (J3). Start recording `spent:` in each brief from the next node, so the checkpoint has numbers to use.

## Rejected

- O4, replace the full check per L node with quick checks: this is already the practice since 2026-10-31 (J8).
- O5, merge converter-names into converter-commands: most nodes touch the same hot files anyway, and an M+L merge makes an XL pull request.
- O6, park achievements and navigation: navigateto appears in 73 script rows and achievement commands in 15, all unconverted and none refused (J9). Parking them would fail the acceptance bar. (P4)
- O7, security fixes have no node: security-review-v0.2 fixes high and critical findings itself (brief, line 57).

## Scope changes

- park item-windows: becomes `priority: later` with a v0.2.x note. The release node drops its dependency on item-windows. A decision record refuses the rows. (O1, P1)
- merge npc-behavior-spike into npc-behavior-harness: the spike becomes `status: dropped`, with the reason "merged into npc-behavior-harness phase 1". The harness brief takes the spike's acceptance and the P2 scope. (O2, P2)
- reorder the must line as in step 3, by changing priority and dependencies only. No node is dropped. (P5)

## Proofs

- none. No high open claim could be settled by a small build.

## Prior art

- none outside the repo. The repo has no earlier debates. The project's own record (J2, J8) was used.

## Open risks

- O3, the schedule length is unknown: there are only 2 code nodes of actuals. Handled by the step 4 checkpoint.
- The 400-line C++ stop rule in P2 is an estimate. Phase 1 of the harness measures it.
- The security review comes late (O7): findings in early systems surface only at wave 30 or later. Accepted as low risk, because the node includes the fixes.

## Inferred choices

- No release date (J5): from GOAL.md and strategy.md 14.2a. If v0.2.0 has a date, step 3's stop points apply sooner.
- "1:1 parity" means scripts convert and run, or are refused by a decision. It does not mean every window exists at the tag. (P5): from GOAL.md Acceptance and the `should` priority definition. The Outcome names enchant windows, so parking item-windows needs your approval.
- Strictly sequential work (J6): from GOAL.md Constraints.

## Answers from the user

- none asked

## Overruled

- none

## Cost of this debate

- 2 of 2 rounds, about 102k tokens for the agents, plus the judge's work
