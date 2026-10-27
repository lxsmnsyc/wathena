---
id: marriage
title: Marriage and the wedding skills
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/social/marriage*.go
  - server/skill/wedding*.go
  - server/zone/skills_wedding.go
  - client/src/viewer/marriage*
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
  - server/character/**
  - server/skill/**
  - server/staff/**
  - server/admin/**
  - docs/generated/item-scripts-*.md
provides: []
depends_on:
  - on: proposal-small-systems
    kind: contract
    via: small-systems
---

# Marriage and the wedding skills

## Goal

Build marriage as proposal 0006 section 3 describes: `character_partners` as two tied rows (planned migration 00048, numbers taken in merge order), `player:marry` and `player:divorce` with ring removal online and offline, the five wedding skills from the rings (WE_MALE, WE_FEMALE, WE_CALLPARTNER, WE_CALLALLFAMILY, WE_ONEFOREVER), staff `players.marriage` and the window. Convert `getpartnerid`, `marriage`, `divorce` and `wedding_effect`.

## Context

See the frozen `small-systems` contract and proposal 0006, "Decisions in review". The gotchas for this node are in nodes/proposal-small-systems/outcome.md.

- No adoption, baby skills or family columns: they are v0.3.0 (decision 1, tier3-systems).
- Split from marriage-and-navigation (decision 3).
- Source: proposal 0006 section 23

## Acceptance

- [ ] The marriage rows convert in both modes; the wedding skills have tests.
- [ ] An e2e test marries two players, uses Call Partner, divorces one while the other is offline, and checks the rings are gone (PostgreSQL).
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

M.
