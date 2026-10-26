---
id: security-review-v0.2
title: Security review of the v0.2.0 systems
status: todo
priority: must
type: chore
outline: false
milestone: v0.2.0
size: M
spike: false
gate: true
exclusive:
  - fuzz
owns:
  - docs/security/review-2026-*.md
provides: []
depends_on:
  - on: instances-content
    kind: hard
    why: reviews the finished system
  - on: instances-ui
    kind: hard
    why: reviews the staff endpoint and client commands
  - on: sieges-ui
    kind: hard
    why: reviews the castle teleport and admin endpoints
  - on: sieges-content
    kind: hard
    why: reviews the finished system
  - on: mercenaries
    kind: hard
    why: reviews the finished companions
  - on: homunculus
    kind: hard
    why: reviews the finished companions
  - on: pets
    kind: hard
    why: reviews the finished companions
  - on: chat-rooms
    kind: hard
    why: reviews the finished system
  - on: ui-windows
    kind: hard
    why: the bank moves zeny
  - on: npc-api-events
    kind: hard
    why: reviews cross-zone calls
  - on: security-fixes
    kind: hard
    why: the review checks the fixes
---

# Security review of the v0.2.0 systems

## Goal

Review the new attack surface of v0.2.0 against section 7's threat model: instances, sieges, companions, chat rooms, the bank and the new NPC API across zones. Fix high and critical findings. Run the hour-long fuzz tests (`scripts/fuzz.sh 1h`) that v0.1.0 left.

## Context

Same method as the 2026-10 review (read-only reviewers per area, then tests).

- Source: strategy.md phase 8 exit criteria; security review 'Tools'

## Acceptance

- [ ] No open high or critical finding.
- [ ] `scripts/fuzz.sh 1h` ran with no failure, recorded in the review.

## Sizing

Review plus fixes; finding count unknown.
