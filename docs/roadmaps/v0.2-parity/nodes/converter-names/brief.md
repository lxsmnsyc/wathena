---
id: converter-names
title: Convert variable names, getarg, labels and expressions rAthena's parser takes
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
verify: go test ./tools/convert
gate: false
owns:
  - tools/convert/npcscript.go
  - tools/convert/npclua.go
  - tools/convert/ast.go
  - tools/convert/expr.go
  - tools/convert/npc.go
shared:
  - tools/convert/rathena.lua
  - tools/convert/npcexpr.go
  - docs/generated/npc-scripts-*.md
  - docs/generated/classic-coverage.md
  - tools/convert/output.go
  - server/modscript/variables.go
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
provides: []
depends_on:
  - on: review-npc-api
    kind: hard
    why: the review settles the naming and order rules for digit-leading names and assignments
---

# Convert variable names, getarg, labels and expressions rAthena's parser takes

## Goal

Close the converter's parse and naming gaps: `wa.variables` names may start with a digit (F1, an API amendment in server/modscript/variables.go:78); an assignment used as a value becomes an inline function that assigns and returns (F2); computed `getd`/`setd` names keep `.@` variables in a table, make `.` variables NPC variables, and declare other scopes at run time (F3); an NPC with a touch area and no touch label clicks on touch, as rAthena (E2); plus the rows that need no decision: other variable names rAthena accepts (221 renewal and 47 pre-renewal NPCs), `getarg` outside a function (64), labels inside a block (6), and `sscanf`.

## Context

Proposal 0002 next step 40 lists these as left. Variables are kept by name at run time for computed names; decide how they count toward limits (decision 0062 says core has no count limit).

Review decision 60 (docs/proposals/0002-lua-api.md, 2026-10-27; details in nodes/review-npc-api/review.md). E2: rAthena skips the click while the player talks (`sd->npc_id == 0`); wAthena queues touches while talking, so the converted handler must not click for a touch that waited. Check whether a player read of 'is talking' exists before adding API.

- Source: proposal 0002 next step 40; npc-scripts-renewal.md command table

## Acceptance

- [ ] The rows above drop to 0 in docs/generated/npc-scripts-renewal.md and npc-scripts-pre-renewal.md, or each remaining row has a reason.
- [ ] New converter tests cover each form.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

Converter only, one module family; known pattern.

## Notes for the worker

- The digit-leading name rule and the assignment order rule are decisions; write them in the outcome for the curator.
