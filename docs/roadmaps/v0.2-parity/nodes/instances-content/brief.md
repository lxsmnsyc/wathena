---
id: instances-content
title: Instances: convert and run the memorial dungeons
status: todo
priority: must
type: feature
outline: true
milestone: v0.2.0
size: L
spike: false
gate: false
provides: []
depends_on:
  - on: instances-core
    kind: hard
    why: the dungeons run on the real instance lifecycle, not a stub
  - on: extract-folders
    kind: hard
    why: the instance scripts must be in the extract
---

# Instances: convert and run the memorial dungeons

## Goal

Make rAthena's memorial dungeon scripts convert and run: work through the `instances` folder by dungeon, fix converter and API gaps each one shows, and add an e2e run of at least two dungeons.

## Context

2,860 renewal and 296 pre-renewal script NPCs, plus 10 NPCs on instance maps. This node will likely split by dungeon group once `instances-core` lands; the curator refines it then.

Review decision 60 (docs/proposals/0002-lua-api.md, 2026-10-27; details in nodes/review-npc-api/review.md): `monster:use_skill` takes `{ cast_milliseconds, can_cancel, emotion }` for `areamobuseskill` (B7).

- Source: npc-scripts-renewal.md instances folder
