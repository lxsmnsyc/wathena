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

Build and convert the 'needed for the rest' commands of proposal 0002 steps 12 to 16 that `review-npc-api` accepts: chat channel options, party result codes and member IDs, `$@party_create_id`, guild calls (`guildskill`, `guildchangegm`, `getguildmasterid`, `getmapguildusers`, `getguildinfo`, `guildstoragedelitem`, `requestguildinfo`), `countbound`, `openstorage2` modes, random options in `give_item`, exact-copy forms, `npcshopitem`, `npcshopattach` with `OnBuyItem`/`OnSellItem`, barter forms, computed `callshop`, `checkweight` with several items, `mesitemlink` without brackets, `isequippedcnt`, `getequiprandomoption`, `setunitdata`, `strnpcinfo(0)`, and the buying-store flag in `data/items.yml`.

## Context

Each was refused by the converter with a reason in docs/generated/npc-scripts-*.md. Commands that need sieges (`warpguild`, `maprespawnguildid`, castles in `getguildinfo`) go to `sieges-war`.

- Source: proposal 0002 next steps 12-16, 40; npc-scripts-renewal.md command table

## Steps

- [ ] Party and chat
- [ ] Guild (non-siege)
- [ ] Storage and items
- [ ] Shops and barter
- [ ] Unit data and NPC info

## Acceptance

- [ ] Each accepted command converts with a test; refused ones keep a reason in the report.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: many small commands across server modules and the converter; split by step if it grows past one review.

## Notes for the worker

- If this runs long, ship it as two PRs: party/chat/guild, then storage/items/shops.
- Guild commands that need castles or sieges (`warpguild`, `maprespawnguildid`, castles in `getguildinfo`, `getcastledata`) belong to the siege nodes; this node converts the rest of the guild commands.
