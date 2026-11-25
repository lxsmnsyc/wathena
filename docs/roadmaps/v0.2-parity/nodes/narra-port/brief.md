---
id: narra-port
title: Port the server from Go to Narra
status: todo
priority: later
type: refactor
outline: true
milestone: later
size: XL
spike: false
gate: false
added: 2026-11-25
provides: []
depends_on:
  - on: narra-study
    kind: hard
    why: starts only if the study recommends it and the maintainer approves
---

# Port the server from Go to Narra

## Goal

Port the server from Go to Narra, in the order and on the conditions narra-study sets. Behavior stays the same: the parity reports, golden tests and e2e tests pass on the ported server.

## Context

Outline. Break into full briefs when `plan.py refine` lists it, after narra-study's decision.

- Source: maintainer, 2026-11-25
