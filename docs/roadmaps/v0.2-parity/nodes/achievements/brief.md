---
id: achievements
title: Achievements and titles
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/achievement/**
  - server/zone/achieve.go
  - client/src/viewer/achievements*
  - client/src/viewer/titles*
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
  - server/store/*.go
  - server/mod/**
  - server/gamedata/**
  - server/mail/**
  - server/character/**
  - reference/rathena/**
provides: []
depends_on:
  - on: proposal-small-systems
    kind: contract
    via: small-systems
---

# Achievements and titles

## Goal

Build achievements as proposal 0006 section 4 describes: `data/achievements.yml` and `data/titles.yml` converted from a new `achievements.json` extract (361 renewal, 359 pre-renewal records) with Lua conditions and rewards, `z.achieve` and the trigger call sites that exist, score and level computed at load, rewards by mail, titles, `character_achievements` and `characters.title` (planned migration 00049), staff `players.achievements` and the window. Convert `achievementinfo`, `achievementexists`, `achievementadd`, `achievementcomplete`, `achievementremove` and `achievementupdate`.

## Context

See the frozen `small-systems` contract and proposal 0006, "Decisions in review". The gotchas for this node are in nodes/proposal-small-systems/outcome.md.

- `z.achieve` may already exist as the contract's no-op stub, added by a node that ran first; replace its body.
- Title text is the achievement's name until `TitleTable` is extracted (decision 6). The 15 `Eat` and `Chatting` records never complete, as in rAthena.
- The extract and the `achievement_life` recordings need the fork (decision 12).
- Source: proposal 0006 section 23

## Acceptance

- [ ] The achievement rows convert in both modes; trigger and reward tests pass against `achievement_life` recordings.
- [ ] An e2e test completes an achievement, claims its reward by mail and sets the title across a restart (PostgreSQL).
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

M.
