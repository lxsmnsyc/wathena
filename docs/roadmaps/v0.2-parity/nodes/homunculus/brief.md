---
id: homunculus
title: Homunculus: creation, skills, evolution and AI
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - server/companion/homun*.go
  - server/companion/core/data/homunculi.yml
  - server/skill/homun*.go
  - server/zone/homun*.go
  - client/src/viewer/homun*
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
  - server/progression/**
  - server/status/**
  - server/statuschange/**
  - server/skill/**
  - server/battle/testdata/**
  - reference/rathena/**
provides: []
depends_on:
  - on: companion-units
    kind: hard
    why: homunculi are followers on the companion-units base
  - on: extract-sync
    kind: hard
    why: homunculus goldens need new recordings from the fork's recorder
---

# Homunculus: creation, skills, evolution and AI

## Goal

Build homunculi as proposal 0005 section 3 (without 3.9) and the homunculus parts of sections 5 to 16 describe: `data/homunculi.yml` from a new `homunculi.json` extract, the 8 base classes and their evolved forms, the three Alchemist skills and the 16 regular homunculus skills with goldens, levels and growth, exp share, hunger, intimacy, evolution, vaporize, death and revive, the `homunculus` ruleset flag, and the window. Convert `gethominfo`, `checkhomcall`, `addhomintimacy`, `homevolution`.

## Context

See the frozen contract `companions` and proposal 0005, "Decisions in review" (3, 5, 8, 13). The gotchas for this node are in nodes/proposal-companions/outcome.md.

- No Homunculus S (decision 8): mutation needs a Geneticist, a v0.3.0 class. Keep the S records and `prev_class` in data and saves; `mutate`, `morph`, `shuffle`, `morphembryo`, `hommutate` and `homshuffle` are refused with a reason. Do not add `homunculus_s_max_level` or `homunculus_s_growth_level`.
- Migration: the next free number, planned 00046 `character_homunculi` and `character_homunculus_skills`.
- The fork's recorder adds `homunculus_life` and `follower_skill`; the fork's branch is local only (GOAL open question).
- Source: proposal 0005 section 17

## Acceptance

- [ ] The 19 skills are recorded from rAthena with the fork's recorder and match the goldens in both modes.
- [ ] An e2e test calls, levels, feeds, vaporizes, recalls and evolves a homunculus across a restart (PostgreSQL).
- [ ] The homunculus item and NPC rows convert; the mutation rows are refused with a reason.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: a skill set with recordings plus the growth and AI rules.
