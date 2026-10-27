---
id: npc-behavior-spike
title: Spike: how to prove NPC scripts behave as in rAthena
status: todo
priority: must
type: spike
outline: false
milestone: v0.2.0
size: M
spike: true
gate: true
owns:
  - docs/roadmaps/v0.2-parity/nodes/npc-behavior-spike/**
provides: []
depends_on:
  - on: extract-sync
    kind: hard
    why: the spike extends the fork's recorder, which must be in a known state
---

# Spike: how to prove NPC scripts behave as in rAthena

## Goal

Decide how to measure that converted NPC scripts behave as they do in rAthena, not only that they convert. Answer: what to record in rAthena (dialog lines, menus, items and zeny given, variables set, warps, events fired) for a scripted player session; how to replay the same inputs in wAthena; how to compare; and how many scripts can be covered automatically. Write the answer and a sample of 3 scripts recorded and compared by hand.

## Context

The maintainer made behavior parity a v0.2.0 requirement on 2026-10-26. Today docs/generated reports only whether a script converts. The fork's recorder (src/map/wathena_recorder.cpp in ~/Documents/GitHub/rathena) already records formula results; this would extend it to script runs. The e2e tests (TestQuestFromCoreNPC, the kRO run of decision 0052) play scripts by hand.

- Source: maintainer answer 2026-10-26; premortem item 4

## Acceptance

- [ ] The outcome names the recording format, the replay driver, the comparison rules and the expected coverage, and shows 3 scripts compared.
- [ ] The outcome proposes the build node's scope and size.

## Sizing

Novel; time-boxed answer, not a build.

## Notes for the worker

- Stop at the gate: the maintainer picks the approach.
- Do not build the harness here.
