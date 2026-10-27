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
  - server/chat/**
  - server/rules/**
  - tools/convert/npc*.go
provides: []
depends_on:
  - on: proposal-small-systems
    kind: contract
    via: small-systems
---

# NPC and player chat rooms

## Goal

Build chat rooms as proposal 0006 section 2 and the room rows of sections 12 to 19 describe: zone-local rooms for players and NPCs, not saved, with rAthena's limits and triggers, the `CHAT_CHANNEL_ROOM` channel and the client window. Convert `waitingroom`, `delwaitingroom`, `warpwaitingpc`, `enablewaitingroomevent`, `disablewaitingroomevent`, `getwaitingroomstate` and `kickwaitingroomall`, and lift the `nochat` and `no_chat` refusals.

## Context

See the frozen `small-systems` contract and proposal 0006, "Decisions in review". The gotchas for this node are in nodes/proposal-small-systems/outcome.md.

- A member of a player room who walks leaves it (decision 10).
- Mod permission `chat_rooms.manage` (decision 14).
- Source: proposal 0006 section 23

## Acceptance

- [ ] The room command rows convert in npc-scripts-*.md in both modes.
- [ ] Tests cover limits, triggers, `warpwaitingpc` and kicks; an e2e test runs an NPC room that warps its members.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

M: one zone-local system with a window.
