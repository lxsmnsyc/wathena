---
id: world-gen-rules
title: World generation records and settings for mods
status: todo
priority: could
type: feature
outline: true
milestone: modding
size: M
spike: false
gate: false
added: 2026-10-27
provides: []
depends_on:
  - on: release-v0.2.0
    kind: hard
    why: modding follows the parity release
---

# World generation records and settings for mods

## Goal

The record and setting changes of review decision 62, each keeping today's behavior by default: the dungeon style field `spawn_biome`; quest template fields `repeat` and `time_limit` (decide core's default after measuring); `spawn_table` hooks at rotations and region changes (server/cmd/waserver/worldchange.go:252-275), not layers; `procedural.layout.rotating`, 1 by default; `rewards` of automatic events as a list. No schema change; worlds made before keep their stored quests and rotation.

## Context

Outline. Break into a full brief when `plan.py refine` lists it. Paths in nodes/review-world-gen/review.md ("Proposed nodes").

- Source: review decision 62 (C3, E5, F1, F6, I3)
