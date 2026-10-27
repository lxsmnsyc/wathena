---
id: ui-windows
title: Refine, stylist and bank windows
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
  - server/stylist/**
  - server/bank/**
  - client/src/viewer/refine*
  - client/src/viewer/stylist*
  - client/src/viewer/bank*
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
  - server/status/**
  - server/currency/**
  - server/transfer/**
  - server/item/**
  - server/rules/**
  - reference/rathena/**
  - docs/generated/item-scripts-*.md
  - tools/convert/use.go
provides: []
depends_on:
  - on: proposal-small-systems
    kind: contract
    via: small-systems
  - on: extract-sync
    kind: hard
    why: the extended refine extract needs the fork's extractor
---

# Refine, stylist and bank windows

## Goal

Build the refine window (proposal 0006 section 8), the stylist (section 6) and the zeny bank (section 7). Refine: the extract gains Normal, HD and Enriched costs, `blessing` and the broadcast flags; `server/refine` replaces the Normal-only reads and decision 0064's HD and Enriched error goes; pre-renewal HD refuses. Stylist: renewal data, charges in rAthena's order, accessories by mail. Bank: core account currency `bank` with no migration, `bank_state_enforce` 1, staff `players.bank`. Convert `refineui`, `openstylist` and `openbank`; `feature.refineui`, `feature.stylist` and `feature.banking` become rule reads (decision 13).

## Context

See the frozen `small-systems` contract and proposal 0006, "Decisions in review". The gotchas for this node are in nodes/proposal-small-systems/outcome.md.

- Scope cut by decision 4: enchant, enchant grade and reform are item-windows. Kafra points in the cash shop moved to tier3-systems (decision 2).
- Source: proposal 0006 section 23

## Acceptance

- [ ] The three command rows convert; refine NPCs take the window branch in both modes.
- [ ] Refine costs and results for all three types match `refine_ui` recordings; stylist and bank have tests; the bank moves zeny with an audit row (PostgreSQL).
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: three windows with client and server.
