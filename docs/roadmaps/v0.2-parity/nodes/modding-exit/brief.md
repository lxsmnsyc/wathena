---
id: modding-exit
title: Modding exit check: five outside mods and a forum script
status: todo
priority: should
type: feature
outline: true
milestone: modding
size: M
spike: false
gate: false
provides: []
depends_on:
  - on: mod-tooling
    kind: hard
    why: outline ordering from phase 9
  - on: mod-index
    kind: hard
    why: outline ordering from phase 9
  - on: api-reference-site
    kind: hard
    why: outline ordering from phase 9
---

# Modding exit check: five outside mods and a forum script

## Goal

Phase 9 exit: five mods by other people install from Git URLs and pass `wactl mod validate` and `wactl mod test`; a popular rAthena forum script converts with documented fixes.

## Context

Outline. Break into a full brief when `plan.py refine` lists it.

- Source: strategy.md section 15, phase 9
