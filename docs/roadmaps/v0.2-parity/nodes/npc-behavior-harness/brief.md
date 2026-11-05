---
id: npc-behavior-harness
title: NPC behavior parity harness
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - docs/roadmaps/v0.2-parity/nodes/npc-behavior-harness/**
provides: []
depends_on:
  - on: extract-sync
    kind: hard
    why: the harness extends the fork's recorder, which must be in a known state
---

# NPC behavior parity harness

## Goal

Prove that converted NPC scripts behave as they do in rAthena, not only that they convert. Record scripted sessions of rAthena NPCs with the fork's recorder, replay the same inputs against wAthena's converted scripts, compare, and report behavior parity per folder next to the conversion numbers in docs/generated.

## Context

The maintainer made behavior parity a v0.2.0 requirement on 2026-10-26. The roadmap debate of 2026-11-05 (debate/debate.md, O2 and P2) merged npc-behavior-spike into this node and fixed the scope below. The fork's recorder (src/map/wathena_recorder.cpp in ~/Documents/GitHub/rathena) already records formula results. The e2e tests (TestQuestFromCoreNPC, the kRO run of decision 0052) play scripts by hand.

Scope:

- Drive only the scripts the driver reaches with default inputs: next, the first menu choice, close, and at most 2 menu paths per script.
- Compare dialog lines, items and zeny given, variables set, warps and events fired.
- The report lists every script as compared, undriven (with the reason) or different (with the explanation), and counts undriven scripts per folder.
- Store the rAthena recordings as files in the repo, so the C++ fork is built once and replay does not need it.

- Source: maintainer answer 2026-10-26; premortem item 4; debate/debate.md

## Steps

- [ ] Phase 1: extend the recorder to script runs, record and compare 3 scripts by hand, and write the recording format, replay driver and comparison rules in log.md as a checkpoint.
- [ ] Stop rule: if phase 1 shows the recorder extension needs more than about 400 lines of C++, compare dialog lines and warps only, and note it in the outcome.
- [ ] Phase 2: build the replay driver and the comparison, run them over every script in scope, and write the report.

## Acceptance

- [ ] The outcome names the recording format, the replay driver, the comparison rules and the measured size of the recorder extension.
- [ ] The behavior parity report lists every converted script as compared, undriven or different, each undriven or different script with a reason.
- [ ] No compared script differs from rAthena without an explanation.

## Sizing

L: one recorder extension in the fork, one replay driver and a report generator. Phase 1 is the old spike, M.
