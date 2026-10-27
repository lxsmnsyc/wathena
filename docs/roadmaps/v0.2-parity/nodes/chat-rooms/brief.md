---
id: chat-rooms
title: NPC and player chat rooms
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/chatroom/**
  - client/src/viewer/chatroom*
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
  - on: proposal-small-systems
    kind: contract
    via: small-systems
---

# NPC and player chat rooms

## Goal

Build chat rooms for players and NPCs as proposal 0006 describes, with the client window, and convert `waitingroom`, `warpwaitingpc`, `enablewaitingroomevent`, `disablewaitingroomevent` and `getwaitingroomstate`.

## Context

See the frozen `small-systems` contract.

- Source: proposal 0006 (to write)

## Acceptance

- [ ] The command rows drop in the reports; tests cover each system.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

One system per node.
