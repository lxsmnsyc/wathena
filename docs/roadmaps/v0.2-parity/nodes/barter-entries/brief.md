---
id: barter-entries
title: Barter shops that sell one item twice
status: todo
priority: should
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
added: 2026-11-05
owns:
  - docs/roadmaps/v0.2-parity/nodes/barter-entries/**
shared:
  - schema/proto/**
  - schema/gen/**
  - client/src/net/**
  - server/shop/**
  - server/store/migrations/**
  - server/store/queries/**
  - server/store/db/**
  - tools/convert/npcshop*.go
  - docs/generated/npc-scripts-*.md
provides: []
depends_on:
  - on: converter-commands
    kind: hard
    why: it converts the rest of the barter lines and records why these stay refused (decision 0072)
---

# Barter shops that sell one item twice

## Goal

Convert the barter lines that sell an item a second time: 5 items in rAthena's `barter_refine_1` and `barter_refine_2`. Write a design proposal first, since this changes the protocol: `ShopBuyLine` names an entry instead of an item, and `shop_stock` is keyed by shop and entry instead of shop and item (a migration).

## Context

converter-commands (decision 0072) converted refined barter costs and 6 cost lines. These 5 items stay refused because two entries of one barter share an item ID. The maintainer chose a `should` node on 2026-11-05: v0.2.0 can ship with them refused.

- Source: decision 0072 "Details to review"; nodes/converter-commands/outcome.md

## Acceptance

- [ ] A design proposal (docs/proposals/README.md) covers the protocol and store changes, approved before code.
- [ ] The 5 items convert with tests; the barter refusal row drops to 0 in both reports.
- [ ] Existing saved shop stock migrates without loss.

## Sizing

M: one proposal, one protocol field, one migration, converter and client changes.
