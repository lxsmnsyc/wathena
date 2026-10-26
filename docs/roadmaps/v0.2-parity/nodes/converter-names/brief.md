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
shared:
  - tools/convert/rathena.lua
  - tools/convert/npcexpr.go
  - docs/generated/npc-scripts-*.md
  - docs/generated/classic-coverage.md
  - tools/convert/output.go
provides: []
depends_on:
  - on: review-npc-api
    kind: hard
    why: the review settles the naming and order rules for digit-leading names and assignments
---

# Convert variable names, getarg, labels and expressions rAthena's parser takes

## Goal

Close the converter's parse and naming gaps that need no new Lua API: variable names rAthena accepts but the converter refuses (221 renewal and 47 pre-renewal NPCs, for example `$@3rdgc_room02`), names that start with a digit, `getarg` outside a function (64), an assignment used as a value (8), labels inside a block (6), `getd`/`setd` of computed names, and `sscanf`.

## Context

Proposal 0002 next step 40 lists these as left. Variables are kept by name at run time for computed names; decide how they count toward limits (decision 0062 says core has no count limit).

- Source: proposal 0002 next step 40; npc-scripts-renewal.md command table

## Acceptance

- [ ] The rows above drop to 0 in docs/generated/npc-scripts-renewal.md and npc-scripts-pre-renewal.md, or each remaining row has a reason.
- [ ] New converter tests cover each form.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

Converter only, one module family; known pattern.

## Notes for the worker

- The digit-leading name rule and the assignment order rule are decisions; write them in the outcome for the curator.
