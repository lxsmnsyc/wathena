---
id: world-gen-fixes
title: World generation fixes from review decision 62
status: todo
priority: should
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
added: 2026-10-27
owns:
  - server/gamedata/**
  - server/worldevent/**
  - server/mod/permission.go
  - server/account/export.go
  - server/adminworld/account.go
  - server/zone/social_test.go
  - tools/convert/core/scripts/procedural/**
shared:
  - server/modscript/**
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
  - docs/decisions/**
provides: []
depends_on:
  - on: review-world-gen
    kind: hard
    why: the review decides what this node builds
---

# World generation fixes from review decision 62

## Goal

The small v0.2.0 items of review decision 62 (docs/proposals/0002-lua-api.md):

1. I4: `gamedata.IsField` (server/gamedata/world.go:720) reads generated field IDs (`gen:field:x:y`), so typed events with `level_band` pick fields in procedural worlds. Test with a procedural world and a `level_band` boss event.
2. C5, E6: `spawns.modify` and `world.generate` become flagged permissions (server/mod/permission.go); permission docs and `TestPermissions`.
3. I1: remove `Provisional` from `set_staff_role`, `plan_restart`, `rotate` and `change_region` (spec.go:1247-1255); regenerate the Lua docs.
4. B6: `entered_maps` and `memo_points` in `ExportCharacter`, format still version 1; docs in decision 0032.
5. E7: core's procedural scripts call the public `skill_points`, `weight` and `give_exp`; remove them from `wa.internal`.
6. C1, F8: tests that `monsters.get` works in a `spawn_table` hook and a layer, and that a freed jailed player whose return map is gone goes to the save point.
7. G2: one line in the mod guide for `wactl sim economy --config`.

## Context

Details per item in nodes/review-world-gen/review.md and its outcome.md. Record what is built in a decision record; security-fixes also takes one, so take the next free number when the node starts.

- Source: review decision 62

## Acceptance

- [ ] Each step has the test named above; the Lua docs are regenerated.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

M: seven small items across the generator, permissions and export.
