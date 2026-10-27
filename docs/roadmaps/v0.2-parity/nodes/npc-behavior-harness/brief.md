---
id: npc-behavior-harness
title: NPC behavior parity harness
status: todo
priority: must
type: feature
outline: true
milestone: v0.2.0
size: L
spike: false
gate: false
provides: []
depends_on:
  - on: npc-behavior-spike
    kind: hard
    why: the spike chooses the approach
---

# NPC behavior parity harness

## Goal

Build the harness the spike chose: record scripted sessions of rAthena NPCs with the fork's recorder, replay them against wAthena's converted scripts, compare, and report behavior parity per folder next to the conversion numbers in docs/generated. Run it over every converted script it can drive.

## Context

Read the spike's outcome first; it fixes the scope.

- Source: maintainer answer 2026-10-26
