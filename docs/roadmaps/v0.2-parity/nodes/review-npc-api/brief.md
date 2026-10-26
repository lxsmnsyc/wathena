---
id: review-npc-api
title: Review the open NPC API and converter questions
status: active
priority: must
type: docs
outline: false
milestone: v0.2.0
size: M
spike: true
gate: true
owns:
  - docs/roadmaps/v0.2-parity/nodes/review-npc-api/**
shared:
  - docs/proposals/0002-lua-api.md
provides: []
depends_on: []
---

# Review the open NPC API and converter questions

## Goal

Settle the open review items of the NPC API and the converter in one review decision, so the build nodes that follow have fixed API forms. The output is a review decision in docs/proposals/0002-lua-api.md and a list of accepted, refused and deferred items.

## Context

Each item below was left open by a decision record. Many are small. Some were settled by a later decision; check the later decision first and mark those settled.

- Source: strategy.md 'Open review items' (decisions 0062-0066, converter review of review decision 32); proposal 0002 next steps 11-16, 38-45

## Steps

- [ ] Decision 0062: `fire_event` and `fire_npc_event` return values for zones that have not started their scripts; broadcasts that start every unstarted zone; `awake`.
- [ ] Decision 0063: waits in an event without a player (`R.sleep`); the protocol version for v0.2.0 (`schema.Version`).
- [ ] Decision 0064: the API names changed from proposal 0002b; staff levels by role; NPC timer labels due while a map sleeps.
- [ ] Decision 0065: `maps.can_enter` (moves to the siege nodes).
- [ ] Decision 0066: forms of the calls review decision 59 named; `maps.set_cells`' `send`; still wanted: `OnNPCKillEvent`, `killedrid`, `attachrid` without force, reads of another map's player, `areamobuseskill` options, `unitexists`, statuses on a monster of another map; the one-step directory lag; the 2-second logout wait; nil results of remote calls.
- [ ] Decisions 0052 and 0053: `R.touch_one` with a dialog and a waiting player; click fallback for touch areas without a touch label; `jobname`; `percentheal -100`; the `HIDDEN_NPC` 15-cell check; temporary and item skills in the client's skill list and in `get_skill_level`.
- [ ] Proposal 0002 next steps 11 to 16 and 40: the converter's 'needed for the rest' lists (party result codes, member IDs, guild calls, `countbound`, `openstorage2` modes, `npcshopitem`, `npcshopattach`, barter forms, `freeloop`, `isequippedcnt`, `getequiprandomoption`, computed `getd`/`setd` names, digit-leading variable names, assignment as a value, `OnInterIfInitOnce`, `strnpcinfo(0)`).
- [ ] Decision 0054: a mod comparing long strings can hold its zone up to 30 s; CPU time limit on the Lua thread.
- [ ] Write the review decision, then propose the build nodes it implies (the orchestrator adds them to the roadmap).

## Acceptance

- [ ] A review decision lists every item above as accepted (with the API form), refused (with the reason) or deferred (with a milestone).
- [ ] Every accepted item is covered by `npc-api-events`, `npc-api-timers`, `converter-names` or `converter-commands`, or by a new node the decision proposes.

## Sizing

Many small questions, no code; M for the reading across 8 decision records.

## Notes for the worker

- The maintainer decides. The worker prepares the options and a recommendation per item, then stops at the gate.
- Do not build anything in this node.
