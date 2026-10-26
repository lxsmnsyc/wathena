---
id: window-skills
title: Skills that fail before they start: crafting, carts and windows
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - server/skill/**
  - client/src/viewer/**
shared:
  - schema/proto/**
  - schema/gen/**
  - client/src/net/**
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
provides: []
depends_on: []
---

# Skills that fail before they start: crafting, carts and windows

## Goal

Port the skills up to transcendent classes that fail before they start because they open a window or craft: Warp Portal, Item Appraisal, Arrow Crafting, Change Cart, Decorate Cart, Open Buying Store, Repair Weapon, Weapon Refine, Hindsight, Abracadabra, Create Elemental Converter, Prepare Potion, and the item scripts `cooking` and `produce`. Also party skills that reach only the caster, and ensembles that need a same-party partner.

## Context

The parity survey found these skills refused at start. They need client windows; the protocol needs proposal 0001 amendments.

- Source: parity survey; strategy.md section 11.1

## Steps

- [ ] Crafting family: Arrow Crafting, Prepare Potion, Elemental Converter, cooking, produce, Weapon Refine, Repair Weapon.
- [ ] Windows: Warp Portal (memo list), Item Appraisal, Change Cart, Decorate Cart, Open Buying Store.
- [ ] Others: Hindsight, Abracadabra, party reach, ensembles.

## Acceptance

- [ ] Each skill casts in a zone test and matches the skill goldens where rAthena recorded them.
- [ ] The client shows each window.

## Sizing

L: protocol plus client plus server; known patterns. Ship as one PR per step if the diff grows.
