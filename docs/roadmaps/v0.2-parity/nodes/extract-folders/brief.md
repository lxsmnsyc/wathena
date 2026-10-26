---
id: extract-folders
title: Extract rAthena's instance, guild, battleground, custom and mobs NPC folders
status: todo
priority: must
type: chore
outline: false
milestone: v0.2.0
size: M
spike: false
verify: go test ./tools/convert
gate: false
owns:
  - reference/rathena/extracts/**
shared:
  - tools/convert/rathena.lua
  - tools/convert/npcexpr.go
  - docs/generated/npc-scripts-*.md
  - docs/generated/classic-coverage.md
  - tools/convert/output.go
provides: []
depends_on:
  - on: extract-sync
    kind: hard
    why: the extractor must be reproducible before new folders are added to it
---

# Extract rAthena's instance, guild, battleground, custom and mobs NPC folders

## Goal

Add the NPC folders the extract leaves out (`instances` in full, `guild`, `guild2`, `guild3`, renewal `re/guild` and `re/guild3`, the mapflag lines, `battleground`, `custom`, `mobs`) to `npcs.json`'s part files for both modes, so the coverage reports count every script rAthena loads. Nothing new has to convert yet; the reports must show the real totals.

## Context

The renewal reports show 4,555 script NPCs missing from the extract (instances 2,860, guild 534, guild2 478, guild3 245, battleground 295, custom 130, mobs 13) and pre-renewal 1,551. Part files stay below 2 MB each (decision 0051).

- Source: docs/generated/npc-scripts-renewal.md 'missing from the extract'; strategy.md phase 10 item 4

## Acceptance

- [ ] docs/generated/npc-scripts-*.md and classic-coverage.md list no folder as missing from the extract.
- [ ] `go test ./tools/convert` passes with the regenerated reports.
- [ ] Classic worlds still start; NPCs that do not convert are not placed.

## Sizing

Extract change plus report regeneration; mostly data.

## Notes for the worker

- Battleground NPCs are extracted for counting only; battlegrounds are a v0.3.0 system.
- Regenerate the reports once, at the end, with `wactl convert`.
