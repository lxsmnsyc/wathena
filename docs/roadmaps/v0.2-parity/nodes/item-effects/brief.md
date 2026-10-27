---
id: item-effects
title: Item scripts: autobonus effects, statuses, transforms and bonus scripts
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
verify: go test ./tools/convert ./server/item ./server/statuschange
gate: false
owns:
  - tools/convert/usestatus.go
  - tools/convert/item.go
  - tools/convert/equipitem.go
  - server/statuschange/**
  - server/item/**
shared:
  - docs/generated/item-scripts-*.md
  - tools/convert/use.go
  - tools/convert/output.go
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
provides: []
depends_on: []
---

# Item scripts: autobonus effects, statuses, transforms and bonus scripts

## Goal

Convert the item script effects that fail today: `autobonus`, `autobonus2` and `autobonus3` effect scripts with `sc_start` and `heal`, the `sc_start` statuses that refuse (21 renewal, 14 pre-renewal), `heal` in equipment (60), `transform` and `active_transform` (51), `bonus_script` (17), `showscript`, `getitempos`, `skilleffect`, `setfont`, `getexp2`. Also `heal_percent` at -100 kills (E4).

## Context

docs/generated/item-scripts-*.md list each with counts. `active_transform` needs options (decision 0064).

Review decision 60 (docs/proposals/0002-lua-api.md, 2026-10-27; details in nodes/review-npc-api/review.md): `player:heal_percent` with -100 kills, as `percentheal` (E4). This replaces review decision 56's rule; update the API doc at spec.go:565 and the 10 NPC files' and the Poison Bottle's expectations.

- Source: item-scripts-renewal.md, item-scripts-pre-renewal.md

## Acceptance

- [ ] The rows above drop in item-scripts-*.md; equipment parity stays green.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

Known patterns in the item converter and statuses.
