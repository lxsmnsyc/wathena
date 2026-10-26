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
  - client/src/world/pet*
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
    why: pets are companion units
---

# Pets: taming, eggs, hunger, intimacy and bonuses

## Goal

Build pets as proposal 0005 describes, with the client's pet window, and convert the `pet`, `bpet` and `getpetinfo` item scripts.

## Context

See the frozen contract named in depends_on and its design proposal.

- Source: proposal 0005 (to write)

## Acceptance

- [ ] Taming, hatching, feeding and returning to egg work in an e2e test.
- [ ] The pet item rows drop in item-scripts-*.md.

## Sizing

Known pattern on the companion model.
