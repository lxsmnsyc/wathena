---
id: companion-units
title: Companion units: owner link, follow AI and persistence
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/companion/**
shared:
  - server/zone/**
  - server/modscript/**
  - schema/proto/**
  - schema/gen/**
  - client/src/net/**
  - server/store/migrations/**
  - server/store/queries/**
  - server/store/db/**
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
provides: []
depends_on:
  - on: proposal-companions
    kind: contract
    via: companions
---

# Companion units: owner link, follow AI and persistence

## Goal

Build the shared base for pets, homunculus and mercenaries as proposal 0005 describes: a unit owned by a player, that follows, fights through monster AI, is saved with the character, and appears to other players. No system on top yet.

## Context

See the frozen contract named in depends_on and its design proposal.

- Source: proposal 0005 (to write); adversarial review finding 1

## Acceptance

- [ ] A test unit follows its owner across a map change and is saved and restored with the character (e2e with PostgreSQL).
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

One base in the zone; known AI patterns from monsters.
