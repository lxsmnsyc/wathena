---
id: sieges-war
title: War of Emperium: siege state, guardians, admin and the bot siege
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
added: 2026-10-27
owns:
  - server/castle/siege*.go
  - server/castle/guardian*.go
  - tools/convert/npcsiege*.go
  - e2e/siege_test.go
shared:
  - server/castle/**
  - server/zone/**
  - server/world/**
  - server/modscript/**
  - server/mod/**
  - server/gamedata/**
  - server/mob/**
  - server/admin/**
  - server/adminworld/**
  - server/staff/**
  - server/cmd/waserver/**
  - schema/proto/wathena/admin/**
  - schema/gen/**
  - tools/convert/npcguild.go
  - tools/convert/npcgroup3.go
  - tools/wabots/**
  - tools/botclient/**
  - e2e/**
  - docs/generated/**
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
  - on: gvg-rules
    kind: hard
    why: guardians, the Emperium and the e2e fight need the battle rules
  - on: sieges-castles
    kind: hard
    why: ownership and guardian bits live in its registry
  - on: npc-api-timers
    kind: hard
    why: `npc:wake()` ends the waits of `timers.wait`
---

# War of Emperium: siege state, guardians, admin and the bot siege

## Goal

Build the siege itself as proposal 0004 describes: siege state per edition; `OnAgitInit` at boot so castle zones and flag towns start; `OnAgitStart*`/`OnAgitEnd*` labels plus the typed events `siege_start`, `siege_end`, `castles_ready`; restart behavior inside a siege window; guardians (`spawn_guardian`, Guard Up stats, ownership, installed bits) and the Emperium's castle data; `maps.respawn_guild`, `guild:warp`, `maps.can_enter(map, job)`, `guilds.relation` and `npc:wake()`; the siege converter rows; admin endpoints `POST /sieges/{edition}/start|end` and `POST /castles/{id}/owner`, staff permissions `world.sieges` (admin and game_master) and `world.castles` (admin), audit rows and GM commands; the e2e bot siege and the `siege` wabots scenario.

## Context

Proposal 0004 is accepted and the `sieges` contract is frozen. Read the proposal's section for this node and the contract first. Each node writes its own decision record.

- Source: docs/proposals/0004-sieges.md (section 15, 'Who builds what'); nodes/proposal-sieges/outcome.md

## Acceptance

- [ ] An e2e bot siege: two guilds, the Emperium breaks, ownership changes, guardians respawn.
- [ ] The `siege` wabots scenario reports tick times; if 100 against 100 in one castle is over budget, decision 0055 comes before a documented siege capacity.
- [ ] Staff endpoints refuse without the permission and write audit rows.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L; would be M without the performance scenario.
