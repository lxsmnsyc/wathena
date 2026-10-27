---
id: item-windows
title: Item enchant, enchant grade and reform windows
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
  - server/itemwindow/**
  - client/src/viewer/enchant*
  - client/src/viewer/reform*
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
  - server/item/**
  - server/status/**
  - server/character/**
  - reference/rathena/**
  - docs/generated/item-scripts-*.md
  - tools/convert/use.go
provides: []
depends_on:
  - on: proposal-small-systems
    kind: contract
    via: small-systems
---

# Item enchant, enchant grade and reform windows

## Goal

Build proposal 0006 sections 9 to 11, renewal only: `item.Slot.EnchantGrade` and `item.Extra.Enchants`, `enchant_grade` and `enchants` on the four item tables (planned migration 00050), the enchant grade window first, then the item enchant window (random, perfect, upgrade, reset), then the item reform window, from new `enchantgrade`, `item_enchant` and `item_reform` extracts. Convert `enchantgradeui`, `item_enchant` and `item_reform` (130 items).

## Context

See the frozen `small-systems` contract and proposal 0006, "Decisions in review". The gotchas for this node are in nodes/proposal-small-systems/outcome.md.

- Split from ui-windows (decision 4). Stays `must`; it is the first node to become `should` if the release is late (decision 11).
- The grade bonus to refine ATK and DEF is renewal only.
- Source: proposal 0006 section 23

## Acceptance

- [ ] The three command rows convert in renewal; the item rows drop in item-scripts-renewal.md.
- [ ] Grade, enchant and reform results match `enchantgrade_ui`, `enchant_ui` and `item_reform` recordings; saves keep the new fields (PostgreSQL).
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: three windows on new item fields.
