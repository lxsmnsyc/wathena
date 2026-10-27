---
id: sieges-ui
title: Sieges: castle window, teleport and admin screens
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
added: 2026-10-27
owns:
  - client/src/viewer/castle*.ts
  - client/src/admin/castles*.ts
  - server/zone/castleteleport*.go
shared:
  - client/src/viewer/**
  - client/src/admin/**
  - client/src/net/**
  - schema/proto/**
  - schema/gen/**
  - server/zone/**
  - server/castle/**
  - server/gateway/**
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
  - tools/convert/rathena.lua
  - tools/convert/npcexpr.go
  - tools/convert/output.go
  - docs/generated/npc-scripts-*.md
  - docs/generated/classic-coverage.md
provides: []
depends_on:
  - on: sieges-war
    kind: hard
    why: siege state sets the teleport cost, and the admin screen calls its endpoints
  - on: gvg-rules
    kind: hard
    why: `MapRules` and player targeting come from it
---

# Sieges: castle window, teleport and admin screens

## Goal

Build what players and staff see of sieges: on the server, `GuildCastles`, `CastleInfo`, `CastleInfoRequest` and `CastleTeleport` with its result, costed from the siege state; in the client, the castle list in the guild window, the teleport button, and the economy and defense view; emblems drawn on flag NPCs and guardians; the attack cursor and click-to-attack on enemy players while `MapRules.gvg` is set; the admin World screen's castles and sieges, with start, end and owner forms.

## Context

Proposal 0004 is accepted and the `sieges` contract is frozen. Read the proposal's section for this node and the contract first. Each node writes its own decision record.

- Source: docs/proposals/0004-sieges.md (section 15, 'Who builds what'); nodes/proposal-sieges/outcome.md

## Acceptance

- [ ] The client lists a guild's castles, teleports to one, and shows emblems on flags (client tests or a browser check).
- [ ] The admin screen starts and ends a siege and sets an owner.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

Client and admin screens on known patterns.
