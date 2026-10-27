---
id: companion-units
title: Companion units: follower base, AI and persistence
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - server/companion/**
  - client/src/viewer/follower*
shared:
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
  - schema/proto/**
  - schema/gen/**
  - client/src/net/**
  - server/zone/**
  - server/modscript/**
  - server/store/migrations/**
  - server/store/queries/**
  - server/store/db/**
  - server/store/*.go
  - server/character/**
  - server/mod/**
  - server/gamedata/**
  - server/rules/**
  - server/progression/**
  - server/battle/**
  - server/mob/**
  - server/admin/**
  - server/staff/**
  - schema/proto/wathena/admin/**
  - tools/wabots/**
  - tools/botclient/**
provides: []
depends_on:
  - on: proposal-companions
    kind: contract
    via: companions
---

# Companion units: follower base, AI and persistence

## Goal

Build the follower base of proposal 0005 (sections 1, 5's loader and kinds, 6's handles and permission, 8's shared messages, 9's migration, 10's `follower.ts`, 13, 15): `walkState` shared with monsters, `entity.follower`, `battle.CalcMisc` per unit kind, orders and the AI base, the damage log source flag, `character_followers`, `companions.manage`, staff `players.companions` and `commands.companions`, `FollowerOrder` and `FollowerResult`, and the `companions` wabots scenario. No pet, homunculus or mercenary yet.

## Context

See the frozen contract `companions` and proposal 0005, "Decisions in review". The gotchas for this node are in nodes/proposal-companions/outcome.md.

- Migration: the next free number after 00043 (`castles`), planned 00044 `character_followers`. Numbers are taken in merge order.
- No unit of its own: tests register a test kind that is not shipped.
- Source: proposal 0005 section 17; adversarial review finding 1

## Acceptance

- [ ] `mob_ai.json` goldens and the monster AI tests pass unchanged after the `walkState` move.
- [ ] A test follower follows its owner across a map change and is saved and restored with the character (e2e with PostgreSQL).
- [ ] The `companions` scenario is measured at 500 bots with three followers each; if over budget, the outcome says decision 0055 comes first (decision 16).
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: the base touches zone, battle, mob, character saves, admin and the bot tools (decision 18).
