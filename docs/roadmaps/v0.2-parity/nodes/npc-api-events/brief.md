---
id: npc-api-events
title: NPC API: kill events, attached players and handles across maps
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - tools/convert/npcplayers.go
  - tools/convert/npcmaps.go
  - tools/convert/npcworld.go
shared:
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
  - tools/convert/rathena.lua
  - tools/convert/npcexpr.go
  - docs/generated/npc-scripts-*.md
  - docs/generated/classic-coverage.md
  - tools/convert/output.go
  - server/zone/**
  - server/modscript/**
provides: []
depends_on:
  - on: review-npc-api
    kind: hard
    why: the review fixes the call forms before they enter the public API
---

# NPC API: kill events, attached players and handles across maps

## Goal

Build the NPC API that review decision 59 left as still wanted, in the forms `review-npc-api` settles: a death event for monsters without a label (`OnNPCKillEvent`), `killedrid`, `attachrid` without force, reads of a player of another map, `unitexists`, `unittalk` for another game ID, `areamobuseskill` options, statuses on a monster of another map, and `awake`. Convert each.

## Context

Decision 0066 built the online player directory and remote handles; these extend it. Public Lua API changes need an amendment to proposal 0002 (docs/proposals/README.md).

- Source: decision 0066 'Details to review'; proposal 0002 next steps 41, 45; 0002b B1, B3, B8

## Acceptance

- [ ] Each accepted call exists in the API spec with tests, and the converter maps its rAthena command.
- [ ] The command rows in npc-scripts-*.md drop, and the reports are regenerated.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: blast radius high (zone messaging, spec.go); uncertainty medium.

## Notes for the worker

- Remote reads lag one step; keep that rule from decision 0066.
- Regenerate docs/generated/lua with the `mods` step.
