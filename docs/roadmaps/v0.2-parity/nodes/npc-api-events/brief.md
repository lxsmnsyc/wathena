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

Build the cross-map NPC API that review decision 60 accepted, and convert each: a broadcast starts an unstarted zone only when one of its NPC sources names the label (A2); `OnInterIfInitOnce` as `OnInit` once per server start (A5); `unloadnpc` of an NPC with duplicates removes each record (A6); `monster_id` and `monster_game_id` on a monster label's event for `killedrid`/`killedgid` (B4); `unitexists` over `maps.find_monster` (B8); `monster:show_emotion` (B9); `"self"` as `player:say_aloud`'s `to` (B11); `{ to }` on `npc:remove_effect` and `player:remove_effect` (D6); `npc:fire_touch(player)` (E1); NPCs with `sprite: none` take answers from any distance (E5); `getunitdata` and `getunittype` over existing calls (G15).

## Context

Decision 0066 built the online player directory and remote handles; these extend it. Public Lua API changes need an amendment to proposal 0002 (docs/proposals/README.md).

Review decision 60 (docs/proposals/0002-lua-api.md, 2026-10-27; details in nodes/review-npc-api/review.md). Deferred by that decision, not built here: `OnNPCKillEvent`, `attachrid` without force, reads of another map's player, statuses on a monster of another map. `npc:wake()` goes to sieges-war and `areamobuseskill` options to instances-content.

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
