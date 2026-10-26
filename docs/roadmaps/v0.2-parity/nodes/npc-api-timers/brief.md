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

Build timers on a player (`addtimer`, `deltimer`, `initnpctimer` attached to a player), `stopnpctimer` on another NPC, `schedules.yearly` for `OnDayMMDD`, and the rule for NPC timer labels due while a map sleeps, in the forms `review-npc-api` settles. Convert each.

## Context

Proposal 0002 step 40 left player timers; step 43 left `schedules.yearly` and the sleeping-map rule.

- Source: proposal 0002 next steps 40, 43; 0002b B10

## Acceptance

- [ ] Each call has tests and a converter mapping; the command rows drop in the reports.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

Known pattern (`wa.timers`), one area.
