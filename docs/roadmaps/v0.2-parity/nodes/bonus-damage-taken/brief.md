---
id: bonus-damage-taken
title: Bonus sums for damage taken with three options
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: S
spike: false
verify: go test ./server/bonus ./tools/convert
gate: false
owns:
  - server/bonus/**
  - tools/convert/bonusmap.go
shared:
  - docs/generated/item-scripts-*.md
  - tools/convert/use.go
  - tools/convert/output.go
  - docs/generated/random-options-*.md
provides: []
depends_on: []
---

# Bonus sums for damage taken with three options

## Goal

Give `bonuses.add("damage_taken")` rAthena's sum for the option combinations that fail today, so the 10 `RACE_WEAPON_TOLERACE_*` random options and the `bSubEle` items with three options convert.

## Context

docs/generated/random-options-renewal.md lists the 10 failures; proposal 0002a 'Damage and damage taken' names the sums. Also fix the unknown monster IDs 1900, 2048 and 2050 of `bAddDamageClass`/`bAddDefMonster`, and the function arguments of `bHPRegenRate`/`bSPRegenRate` and `bAutoSpell` level 0.

- Source: random-options-renewal.md; item-scripts-*.md bonus failures

## Acceptance

- [ ] random-options-renewal.md shows 249 of 249; the bonus rows above drop in item-scripts-*.md.
- [ ] Equipment parity tests pass.

## Sizing

One bonus family, golden tests exist.
