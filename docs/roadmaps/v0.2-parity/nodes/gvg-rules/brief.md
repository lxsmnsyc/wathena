---
id: gvg-rules
title: GvG map flags and battle rules
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
added: 2026-10-27
owns:
  - server/battle/gvg*.go
  - server/zone/gvg*.go
shared:
  - server/zone/**
  - server/battle/**
  - server/status/**
  - server/statuschange/**
  - server/skill/**
  - server/mob/**
  - server/social/**
  - server/rules/**
  - server/mod/**
  - server/modscript/**
  - schema/proto/wathena/v1/event.proto
  - schema/gen/**
  - client/src/net/**
  - reference/rathena/extracts/**
  - reference/rathena/README.md
  - server/battle/testdata/**
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
  - tools/convert/rathena.lua
  - tools/convert/npcexpr.go
  - tools/convert/output.go
  - docs/generated/npc-scripts-*.md
  - docs/generated/classic-coverage.md
provides: []
depends_on:
  - on: sieges-castles
    kind: hard
    why: castle records set the castle flags, and map_flags.json comes from its extract
  - on: proposal-sieges
    kind: contract
    via: sieges
---

# GvG map flags and battle rules

## Goal

Build the whole of proposal 0004 section 5: GvG flags that take effect and need `maps.control`; zone fields `gvg`, `gvgGround`, `vs` and `mapType` in place of `Zone.GvG`, and player targeting on `vs` maps; damage rates, Emperium hit rules, flee penalty; `skill_nocast_db` and `item_noequip` checked on every map, normal maps included, as rAthena; `restricted` zones, revival, knockback, movement skills, cast cancel, the 1 s respawn on GvG ground, guild action blocks and Emergency Call's siege bit; the ruleset rules with rAthena's defaults; `MapRules` sent by the server; `record_gvg` goldens in both modes from the fork's recorder.

## Context

Proposal 0004 is accepted and the `sieges` contract is frozen. Read the proposal's section for this node and the contract first. Each node writes its own decision record. There is no player-versus-player combat in the code today, and the no-cast table is loaded but never checked.

- Source: docs/proposals/0004-sieges.md (section 15, 'Who builds what'); nodes/proposal-sieges/outcome.md

## Acceptance

- [ ] The `record_gvg` goldens match in both modes.
- [ ] No-cast and no-equip apply on every map; the decision record lists what changed on normal maps.
- [ ] Player targeting works on `vs` maps; siege state is set directly in tests (sieges-war wires the real setter).
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: battle rules touch shared core code; goldens exist as a pattern.
