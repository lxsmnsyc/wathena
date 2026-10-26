---
id: npc-api-timers
title: NPC API: player timers, yearly schedules and NPC timer gaps
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - tools/convert/npctimer.go
  - server/schedule/**
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
    why: the sleeping-map timer rule is a review item
---

# NPC API: player timers, yearly schedules and NPC timer gaps

## Goal

Build the timer API that review decision 60 accepted, and convert each: `timers.wait(milliseconds)`, 0 to 10 minutes, in NPC handlers that run without a conversation, used by `R.sleep` (A4); `{ clock = "zone" | "server" }` on `timers.after` and `timers.every`, where a server clock timer wakes a sleeping zone and converted NPC timers use it (C1); `player:add_timer(milliseconds, label)` and `player:remove_timer(label)` for `addtimer`/`deltimer`, moving with the player and ending at logout (C2); NPC timers attached to a player (C3) and timer commands on another NPC (C4) over `fire_event`.

## Context

Proposal 0002 step 40 left player timers; step 43 left `schedules.yearly` and the sleeping-map rule.

Review decision 60 (docs/proposals/0002-lua-api.md, 2026-10-27; details in nodes/review-npc-api/review.md). `schedules.yearly` is deferred (no rAthena script uses it). A wait in a handler of a server clock timer follows the server clock.

- Source: proposal 0002 next steps 40, 43; 0002b B10

## Acceptance

- [ ] Each call has tests and a converter mapping; the command rows drop in the reports.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

Known pattern (`wa.timers`), one area.
