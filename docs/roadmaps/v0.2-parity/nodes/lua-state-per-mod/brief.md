---
id: lua-state-per-mod
title: One Lua state per mod and zone crash recovery
status: todo
priority: later
type: feature
outline: true
milestone: later
size: L
spike: false
gate: false
provides: []
depends_on:
  - on: release-v0.2.0
    kind: hard
    why: follows the parity release
---

# One Lua state per mod and zone crash recovery

## Goal

Section 17 open questions: per-mod Lua states for memory limits (threat model finding 1), and crash recovery numbers for zone processes (finding 3).

## Context

Outline. Break into full briefs when `plan.py refine` lists it.

- Source: strategy.md section 15, phase 10; section 17
