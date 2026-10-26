---
id: item-skill-use
title: Items that cast skills: itemskill, unitskilluseid and delayed use
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/skill/itemskill*.go
shared:
  - docs/generated/item-scripts-*.md
  - tools/convert/use.go
  - tools/convert/output.go
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
  - server/zone/**
  - server/modscript/**
provides: []
depends_on:
  - on: item-effects
    kind: hard
    why: both edit the use-item converter in tools/convert/use.go; run in sequence
---

# Items that cast skills: itemskill, unitskilluseid and delayed use

## Goal

Make items that cast skills work: `itemskill` (110 delayed-use items in renewal, 61 in pre-renewal, plus usable items), `unitskilluseid` (18), and the skill names the converter does not know (`DC_FORTUNEKISS2`, `BA_POEMBRAGI2`). Delayed-use items become usable once their skill casts.

## Context

Delayed use is 2.4% full in renewal and 17.1% in pre-renewal, almost all `itemskill`. Laphine and item reform are other nodes.

- Source: item-scripts-*.md 'Delayed use'

## Acceptance

- [ ] Pre-renewal delayed use reaches its non-`callfunc` total; renewal `itemskill` rows drop to 0.
- [ ] Tests cast an item skill in a zone.

## Sizing

One mechanism (skill cast from an item) with many users.
