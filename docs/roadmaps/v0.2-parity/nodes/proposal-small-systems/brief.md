---
id: proposal-small-systems
title: Design proposal: chat rooms, marriage, achievements and windows
status: todo
priority: must
type: contract
outline: false
milestone: v0.2.0
size: M
spike: false
gate: true
owns:
  - docs/proposals/0006-small-systems.md
  - docs/roadmaps/v0.2-parity/contracts/small-systems.md
provides:
  - small-systems
depends_on: []
---

# Design proposal: chat rooms, marriage, achievements and windows

## Goal

Write design proposal 0006 for the small systems rAthena scripts call: NPC and player chat rooms (`waitingroom`, `warpwaitingpc`, room events), marriage (`getpartnerid`, `divorce`, wedding skills), achievements, client navigation (`navigateto`), the stylist, the zeny bank, and the refine, enchant, enchant grade and item reform windows. Freeze the `small-systems` contract.

## Context

0002b category C. Counts from the reports: navigateto 73, waitingroom 33, getpartnerid 22, achievementinfo 14, refineui 9, openstylist 7, item_reform 130 delayed items. Reputation and Laphine are tier 3 (v0.3.0).

- Source: 0002b category C

## Acceptance

- [ ] docs/proposals/0006-small-systems.md is accepted; contracts/small-systems.md is frozen.

## Sizing

Many small designs.

## Notes for the worker

- Stop at the gate.
