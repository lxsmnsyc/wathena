---
id: instances-core
title: Instances: lifecycle, maps, variables and Lua API
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - server/instance/**
  - server/world/**
  - tools/convert/npcinstance*.go
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
  - on: proposal-instances
    kind: contract
    via: instances
  - on: npc-api-events
    kind: hard
    why: instance NPCs use the cross-zone handles and player reads that npc-api-events builds in server/zone
---

# Instances: lifecycle, maps, variables and Lua API

## Goal

Build instances as proposal 0003 describes: instance zones from base maps, lifecycle and timeouts, `'x` variables, `OnInstanceInit`, NPC duplication onto instance maps, the Lua API and the protocol. Convert the `instance_*` commands.

## Context

See the frozen `instances` contract.

- Source: proposal 0003 (to write)

## Acceptance

- [ ] An e2e test creates an instance for a party, enters it, runs an `OnInstanceInit` NPC, and destroys it on timeout.
- [ ] `instance_*` command rows drop in the reports.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: core zone change and new API; known zone patterns.
