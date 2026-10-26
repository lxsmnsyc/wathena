---
id: monster-gaps
title: Monster port gaps: MVP tombs, level ups, warps and missing effects
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
verify: go test ./server/mob ./server/zone -run 'Mob'
gate: false
owns:
  - server/mob/**
  - server/zone/mob*.go
provides: []
depends_on: []
---

# Monster port gaps: MVP tombs, level ups, warps and missing effects

## Goal

Close the monster gaps server/README.md lists as left: MVP tombs, monster level ups, monsters that follow a player through a warp, the 79 renewal and 8 pre-renewal monster skill effects that use player skills (for example `ITM_TOMAHAWK`, `GS_DUST`), and the unported `NPC_` skills. Guardians go with sieges.

## Context

server/README.md lines 904-918 give the effect parity numbers.

- Source: server/README.md 'Left' (monsters)

## Acceptance

- [ ] Mob skill effect parity reaches 18,232 of 18,232 and 7,213 of 7,213, or the README lists each exception with a reason.
- [ ] Tests cover tombs, level ups and warp following.

## Sizing

Known patterns, parity harness exists.
