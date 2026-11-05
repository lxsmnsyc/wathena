---
id: instances
title: Instances: memorial dungeons from lifecycle to client window
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
  - docs/roadmaps/v0.2-parity/nodes/instances/**
provides: []
depends_on:
  - on: proposal-instances
    kind: hard
    why: proposal 0003 is the design every child builds
---

# Instances: memorial dungeons from lifecycle to client window

## Goal

Ship instances as one feature on `feat/instances`: the server lifecycle and Lua API (instances-core), the converted and running memorial dungeon scripts (instances-content), and the client window, admin screen and staff endpoint (instances-ui). The story pull request into main is reviewed as a whole.

## Context

A story since the roadmap review of 2026-11-05: none of the children is useful on main alone (the lifecycle without dungeons, or a window with nothing to list), and no node outside the story depends on one child.

- Source: docs/proposals/0003; debate/debate.md (run order group 2)

## Acceptance

- [ ] Every child is done on `feat/instances`.
- [ ] The story pull request lists each child with its outcome and passes one fresh review.

## Sizing

The sum of its children: L, L and M.
