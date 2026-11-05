---
id: sieges
title: Sieges: War of Emperium with castle scripts and windows
status: todo
priority: must
type: story
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
added: 2026-11-05
owns:
  - docs/roadmaps/v0.2-parity/nodes/sieges/**
provides: []
depends_on:
  - on: proposal-sieges
    kind: hard
    why: proposal 0004 is the design every child builds
  - on: sieges-castles
    kind: hard
    why: castle records and ownership are on main and every child uses them
---

# Sieges: War of Emperium with castle scripts and windows

## Goal

Ship War of Emperium as one feature on `feat/sieges`: siege state, guardians, admin and the bot siege (sieges-war), rAthena's castle scripts converted and running (sieges-content), and the castle window, teleport and admin screens (sieges-ui). The story pull request into main is reviewed as a whole.

## Context

A story since the roadmap review of 2026-11-05: a siege is not playable until the castle scripts and the window exist. gvg-rules stays a top-level node, because its map rules (`skill_nocast_db`, `item_noequip` on every map) are useful on main by themselves. sieges-castles is done and on main.

- Source: docs/proposals/0004; debate/debate.md (run order group 2)

## Acceptance

- [ ] Every child is done on `feat/sieges`.
- [ ] The story pull request lists each child with its outcome and passes one fresh review.

## Sizing

The sum of its children: L, M and M.
