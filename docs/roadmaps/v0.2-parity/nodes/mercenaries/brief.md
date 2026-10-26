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
  - client/src/viewer/merc*
shared:
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
  - docs/generated/item-scripts-*.md
  - tools/convert/use.go
  - tools/convert/output.go
  - schema/proto/**
  - schema/gen/**
  - client/src/net/**
  - server/zone/**
  - server/modscript/**
  - server/store/migrations/**
  - server/store/queries/**
  - server/store/db/**
provides: []
depends_on:
  - on: companion-units
    kind: hard
    why: mercenaries are companion units
---

# Mercenaries: contracts, faith and skills

## Goal

Build mercenaries as proposal 0005 describes, and convert `mercenary_create`, `mercenary_sc_start`, `getmercinfo` and `mercenary_get_faith`.

## Context

See the frozen contract named in depends_on and its design proposal.

- Source: proposal 0005 (to write)

## Acceptance

- [ ] The mercenary item rows drop in item-scripts-*.md; an e2e test hires and fights with one.

## Sizing

Smaller than homunculus; reuses its AI.
