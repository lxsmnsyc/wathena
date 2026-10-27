---
id: pets
title: Pets: taming, eggs, hunger, intimacy and bonuses
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/companion/pet*.go
  - server/companion/core/data/pets.yml
  - server/zone/pet*.go
  - client/src/viewer/pet*
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
  - server/item/**
  - server/progression/**
  - server/zone/loot.go
  - server/zone/itemactions.go
  - reference/rathena/extracts/**
provides: []
depends_on:
  - on: companion-units
    kind: hard
    why: pets are followers on the companion-units base
---

# Pets: taming, eggs, hunger, intimacy and bonuses

## Goal

Build pets as proposal 0005 section 2 and the pet parts of sections 5 to 16 describe: `data/pets.yml` from a new `pets.json` extract, taming, eggs with `Extra.PetID` and the `pets` table, hatch and return, hunger, intimacy, feeding, evolution, bonus tiers by intimacy, the pet window, and the `nopetcapture` map flag. Convert `pet`, `bpet` and `getpetinfo` (including the 12 equipment items, which need an equip-context rule for `player.pet`).

## Context

See the frozen contract `companions` and proposal 0005, "Decisions in review" (4, 6, 9, 10). The gotchas for this node are in nodes/proposal-companions/outcome.md.

- Migration: the next free number, planned 00045 `pets` and `pet_id` on the four item tables.
- `SupportScript` behind `pet_status_support` is the last item, cut first if the node runs long (decision 10).
- The extract needs the fork's `tools/wathena/extract.py` (~/Documents/GitHub/rathena, branch wathena-reference).
- Source: proposal 0005 section 17

## Acceptance

- [ ] Taming, hatching, feeding, returning to egg, a restart and evolution work in an e2e test (PostgreSQL).
- [ ] The `pet`, `bpet` and `getpetinfo` rows convert in item-scripts-*.md in both modes.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

M: known pattern on the follower base; bonuses are data.
