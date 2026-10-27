---
id: mercenaries
title: Mercenaries: contracts, faith and skills
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/companion/merc*.go
  - server/companion/core/data/mercenaries.yml
  - server/skill/merc*.go
  - server/zone/merc*.go
  - client/src/viewer/merc*
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
  - docs/generated/item-scripts-*.md
  - docs/generated/npc-*.md
  - tools/convert/**
  - server/skill/**
  - server/battle/testdata/**
  - reference/rathena/**
provides: []
depends_on:
  - on: companion-units
    kind: hard
    why: mercenaries are followers on the companion-units base
  - on: extract-sync
    kind: hard
    why: mercenary goldens need new recordings from the fork's recorder
---

# Mercenaries: contracts, faith and skills

## Goal

Build mercenaries as proposal 0005 section 4 and the mercenary parts of sections 5 to 16 describe: `data/mercenaries.yml` from a new `mercenaries.json` extract, hiring, the contract timer (online time only), faith and calls per guild in `character_mercenary_guilds`, kill counts and the faith buff, the 41 mercenary skills with goldens, and the window. Convert `mercenary_create`, `mercenary_sc_start`, `mercenary_heal`, `getmercinfo`, `mercenary_get_faith`, `mercenary_set_faith`, `mercenary_get_calls`, `mercenary_set_calls`, `mercenary_delete`.

## Context

See the frozen contract `companions` and proposal 0005, "Decisions in review" (7, 14, 18). The gotchas for this node are in nodes/proposal-companions/outcome.md.

- Migration: the next free number, planned 00047 `character_mercenary_guilds`. The mercenary unit state uses companion-units' `character_followers`.
- If the 41 skills make the node L, split them into `mercenary-skills` (decision 18) and tell the orchestrator.
- Source: proposal 0005 section 17

## Acceptance

- [ ] The 41 skills are recorded from rAthena and match the goldens.
- [ ] The mercenary item and NPC rows convert in both modes; an e2e test hires one, fights, reaches 50 kills, and loses faith on its death (PostgreSQL).
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

M; L if the skills take most of the work (see above).
