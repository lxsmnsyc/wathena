---
id: converter-commands
title: Convert the remaining shop, item, party, guild and storage commands
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - tools/convert/npcparty.go
  - tools/convert/npcguild.go
  - tools/convert/npcshop*.go
  - tools/convert/npcstorage*.go
  - server/social/**
  - server/shop/**
  - server/storage/**
  - server/chat/**
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
    why: the review accepts or refuses each command
  - on: converter-names
    kind: hard
    why: the command converters reuse the naming and order rules converter-names sets in the same files
---

# Convert the remaining shop, item, party, guild and storage commands

## Goal

Build and convert the commands review decision 60 accepted: `jobs.name(job)` for `jobname` (E3); `player:take_item` honors `refine` and `cards` for `delitem2` (G5); barter items with refines and any number of cost kinds (G7); computed `callshop` (G10); `freeloop` converts to nothing (G11); `player:count_equipped(item)` for `isequippedcnt` (G12); `getequiprandomoption` (G13); `mesitemlink`'s forms (G14); `setunitdata` (G15); `$@partymember*` outside the function (G16).

## Context

Each was refused by the converter with a reason in docs/generated/npc-scripts-*.md. Commands that need sieges (`warpguild`, `maprespawnguildid`, castles in `getguildinfo`) go to `sieges-war`.

Review decision 60 (docs/proposals/0002-lua-api.md, 2026-10-27; details in nodes/review-npc-api/review.md). Deferred by that decision (no rAthena script uses them): chat channel commands, party result codes and `$@party_create_id`, guild calls without sieges, `countbound`, `openstorage2` modes, `getitembound3/4`, `guildstoragedelitem`, `npcshopitem`, `npcshopattach`, `mail` arrays, `searchstores` on another map. Siege guild calls go to sieges-war.

- Source: proposal 0002 next steps 12-16, 40; npc-scripts-renewal.md command table

## Steps

- [ ] Items and equipment: E3, G5, G12, G13, G14
- [ ] Shops and barter: G7, G10
- [ ] Unit data and party: G11, G15, G16

## Acceptance

- [ ] Each accepted command converts with a test; refused ones keep a reason in the report.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: many small commands across server modules and the converter; split by step if it grows past one review.

## Notes for the worker

- If this runs long, ship it as two PRs: party/chat/guild, then storage/items/shops.
- Guild commands that need castles or sieges (`warpguild`, `maprespawnguildid`, castles in `getguildinfo`, `getcastledata`) belong to the siege nodes; this node converts the rest of the guild commands.
