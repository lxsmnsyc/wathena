---
id: sieges-castles
title: Castles: data, ownership, investment and storage
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/castle/**
  - client/src/world/castle*
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
provides: []
depends_on:
  - on: proposal-sieges
    kind: contract
    via: sieges
---

# Castles: data, ownership, investment and storage

## Goal

Build castles as proposal 0004 describes: castle records, ownership saved in the database, economy and defense investment, castle storage, `getcastledata`, `setcastledata`, `getcastlename`, `flagemblem`, and the castle window in the client.

## Context

See the frozen contract named in depends_on and its design proposal.

- Source: proposal 0004 (to write)

## Acceptance

- [ ] Castle data persists across restarts (e2e with PostgreSQL).
- [ ] The castle commands convert.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

Known patterns (guild storage, records).
