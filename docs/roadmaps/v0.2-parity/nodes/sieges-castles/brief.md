---
id: sieges-castles
title: Castles: records, registry, ownership and badges
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
  - server/castle/**
  - server/mod/castles.go
  - server/store/migrations/00043_castles.sql
  - server/store/queries/castles.sql
  - server/store/castles.go
  - data/castles.yml
shared:
  - server/social/**
  - server/gamedata/**
  - server/mod/**
  - server/cmd/waserver/**
  - server/zone/**
  - server/modscript/**
  - server/store/db/**
  - schema/proto/wathena/v1/event.proto
  - schema/gen/**
  - tools/convert/npcguild.go
  - reference/rathena/extracts/**
  - reference/rathena/README.md
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
  - on: proposal-sieges
    kind: contract
    via: sieges
  - on: extract-sync
    kind: hard
    why: the castle extracts come from the fork's extract.py, which is outside this repository
---

# Castles: records, registry, ownership and badges

## Goal

Build castles as proposal 0004 describes: castle records in `data/castles.yml` (`mod.CastleKind`), converted from `castle_db.yml`, with the extracts `castles.json`, `map_flags.json` and `job_noenter.json`; the castle registry with migration 00043, saved on every write; a guild break runs `OnGuildBreak` and then clears the owner; `npc:set_guild`, guild badges on NPCs and monsters, and emblem updates; the castle half of `wa.castles` (reads, `set_owner`, `set`, `set_guardian`, `get_guardian`, the `castle_change` event, the `castles.manage` permission) and the castle converter rows.

## Context

Proposal 0004 is accepted and the `sieges` contract is frozen. Read the proposal's section for this node and the contract first. Each node writes its own decision record.

- Source: docs/proposals/0004-sieges.md (section 15, 'Who builds what'); nodes/proposal-sieges/outcome.md

## Acceptance

- [ ] Castle values persist across a restart (e2e with PostgreSQL).
- [ ] A guild break runs `OnGuildBreak` and clears the owner.
- [ ] The castle commands convert.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

Records, one table and the registry; known patterns from guild storage.
