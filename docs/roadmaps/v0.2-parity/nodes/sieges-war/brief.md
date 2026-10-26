---
id: sieges-war
title: War of Emperium: schedule, GvG rules, guardians and the Emperium
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - server/siege/**
  - server/battle/gvg*.go
  - tools/convert/npcsiege*.go
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
  - schema/proto/**
  - schema/gen/**
  - client/src/net/**
  - server/zone/**
  - server/modscript/**
  - server/store/migrations/**
  - server/store/queries/**
  - server/store/db/**
  - server/battle/**
provides: []
depends_on:
  - on: sieges-castles
    kind: hard
    why: ownership changes need castle data in place
  - on: npc-api-timers
    kind: hard
    why: `npc:wake()` ends the waits of `timers.wait`, which npc-api-timers builds
---

# War of Emperium: schedule, GvG rules, guardians and the Emperium

## Goal

Build the siege itself: schedule and `agitcheck`, GvG and TE map flags with their battle rules, guardians (`guardian`, `guardianinfo`), the Emperium and `OnGuildBreak`, `maprespawnguildid`, `warpguild`, `maps.can_enter`, and `jobcanentermap`.

## Context

Battle rules under GvG flags change damage, skills and knockback; check against rAthena recordings where they exist.

Review decision 60 (docs/proposals/0002-lua-api.md, 2026-10-27; details in nodes/review-npc-api/review.md): `npc:wake()` ends the waits of an NPC's handlers, as `awake` (A3), and `maps.can_enter(map, job)` is `jobcanentermap` (D4).

- Source: proposal 0004 (to write); decision 0065

## Acceptance

- [ ] A bot siege test: two guilds, the Emperium breaks, ownership changes, guardians respawn.
- [ ] The siege commands convert.

## Sizing

L: battle rule changes have high blast radius.
