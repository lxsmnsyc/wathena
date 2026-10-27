---
id: release-v0.2.0
title: Release v0.2.0: parity with rAthena
status: todo
priority: must
type: release
outline: false
milestone: v0.2.0
size: M
spike: false
gate: true
exclusive:
  - loadtest
owns:
  - docs/releases/v0.2.0.md
  - SECURITY.md
  - schema/protocol.go
provides: []
depends_on:
  - on: security-review-v0.2
    kind: hard
    why: must be in the release
  - on: converter-names
    kind: hard
    why: must be in the release
  - on: converter-commands
    kind: hard
    why: must be in the release
  - on: npc-api-timers
    kind: hard
    why: must be in the release
  - on: bonus-damage-taken
    kind: hard
    why: must be in the release
  - on: item-skill-use
    kind: hard
    why: must be in the release
  - on: window-skills
    kind: hard
    why: must be in the release
  - on: monster-gaps
    kind: hard
    why: must be in the release
  - on: pre-renewal-parity
    kind: hard
    why: must be in the release
  - on: homunculus
    kind: hard
    why: must be in the release
  - on: marriage
    kind: hard
    why: must be in the release
  - on: navigation
    kind: hard
    why: must be in the release
  - on: item-windows
    kind: hard
    why: must be in the release
  - on: achievements
    kind: hard
    why: must be in the release
  - on: sieges-content
    kind: hard
    why: must be in the release
  - on: instances-content
    kind: hard
    why: must be in the release
  - on: check-hygiene
    kind: hard
    why: must be in the release
  - on: npc-behavior-harness
    kind: hard
    why: must be in the release
---

# Release v0.2.0: parity with rAthena

## Goal

Tag v0.2.0 once every must node is done: regenerate every coverage report, check the parity numbers against section 14.2a, raise `schema.Version`, run the full checks and load tests, write docs/releases/v0.2.0.md with the coverage and known issues, and mark classic mode supported.

## Context

Section 14.2a: every rAthena system in scope runs in both rulesets, and every NPC and item script converts except what a decision refuses. In scope for v0.2.0: everything up to transcendent classes, instances, sieges, pets, homunculus, mercenaries and the small systems. Battlegrounds, third classes and tier 3 systems are v0.3.0.

- Source: strategy.md section 14.2a

## Steps

- [ ] Regenerate reports; list every remaining unconverted script with its reason.
- [ ] Raise `schema.Version` (decisions 0063-0065).
- [ ] `scripts/check.sh all`, `e2e`, `economy`, load tests at the documented count.
- [ ] Release notes, SECURITY.md supported versions, deploy guide.
- [ ] Tag on the commit that passed every check.

## Acceptance

- [ ] Every in-scope report row is converted or has a refusal reason.
- [ ] The behavior parity report shows every driven script matching rAthena, or each difference with a reason.
- [ ] `scripts/check.sh all` passes on the tagged commit.

## Sizing

Checklist; the reports decide whether it ships.
