---
id: marriage-and-navigation
title: Marriage and client navigation
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/social/marriage*.go
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
  - server/store/migrations/**
  - server/store/queries/**
  - server/store/db/**
provides: []
depends_on:
  - on: proposal-small-systems
    kind: contract
    via: small-systems
---

# Marriage and client navigation

## Goal

Build marriage (`getpartnerid`, `divorce`, the wedding skills) and client navigation (`navigateto`, `F_Navi`) as proposal 0006 describes.

## Context

See the frozen `small-systems` contract.

- Source: proposal 0006 (to write)

## Acceptance

- [ ] The command rows drop in the reports; tests cover each system.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

One system per node.
