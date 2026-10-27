---
id: navigation
title: Client navigation
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
  - server/navi/**
  - client/src/viewer/navi*
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
  - client/src/viewer/dialog.ts
provides: []
depends_on:
  - on: proposal-small-systems
    kind: contract
    via: small-systems
---

# Client navigation

## Goal

Build navigation as proposal 0006 section 5 describes: a server route by breadth-first search over warp NPCs, the `NaviRoute` message, the optional `data/navi.yml` for airship, Kafra and scroll edges that core leaves empty, and the client's route display and `<NAVI>` dialog token. Convert `navigateto` and `F_Navi`.

## Context

See the frozen `small-systems` contract and proposal 0006, "Decisions in review". The gotchas for this node are in nodes/proposal-small-systems/outcome.md.

- Warp NPCs only in v0.2.0 (decision 7). A missing route sends the target alone.
- Split from marriage-and-navigation (decision 3).
- Source: proposal 0006 section 23

## Acceptance

- [ ] The `navigateto` and `F_Navi` rows convert in both modes.
- [ ] Route tests over the classic warp graph; the dialog renders `<NAVI>` links.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

M.
