---
id: ui-windows
title: Stylist, bank, refine, enchant and item reform windows
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - server/refine/**
  - client/src/world/windows/**
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
  - docs/generated/item-scripts-*.md
  - tools/convert/use.go
  - tools/convert/output.go
provides: []
depends_on:
  - on: proposal-small-systems
    kind: contract
    via: small-systems
  - on: extract-sync
    kind: hard
    why: the HD and Enriched refine costs need the extractor
---

# Stylist, bank, refine, enchant and item reform windows

## Goal

Build the stylist, the zeny bank, and the refine, enchant, enchant grade and item reform windows as proposal 0006 describes, and convert `openstylist`, `openbank`, `refineui`, `item_enchant`, `item_reform`, `enchantgradeui`. Include the HD and Enriched refine costs the extract lacks (decision 0064).

## Context

See the frozen `small-systems` contract.

Review decision 60 (docs/proposals/0002-lua-api.md, 2026-10-27; details in nodes/review-npc-api/review.md): Kafra points paying part of a cash shop price is deferred to this node as a product call (G8); no script blocks on it, so it may stay unbuilt.

- Source: proposal 0006 (to write)

## Acceptance

- [ ] The command rows drop in the reports; tests cover each system.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: four windows with client and server; split per window if it grows.
