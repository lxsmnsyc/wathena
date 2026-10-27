---
id: world-gen-api
title: World generation reads for mods
status: todo
priority: could
type: feature
outline: true
milestone: modding
size: L
spike: false
gate: false
added: 2026-10-27
provides: []
depends_on:
  - on: release-v0.2.0
    kind: hard
    why: modding follows the parity release
---

# World generation reads for mods

## Goal

The mod-facing reads of review decision 62, all needing `game.read`: `towns.get_distance(a, b)`, `towns.get_teleport_price(from, to)`, a `spawn` field (tags) in `monsters.get`'s record, `player:get_quests(state?)`, `quests.get(quest)`, `world.get_quests(options?)`, `map:is_walkable(x, y)` in layers only, the places scope `field:any`, `world.get_rotation(dungeon)` and `town:get_season()`. Each call gets a `wactl mod test` test and a line in the generated reference.

## Context

Outline. Break into a full brief when `plan.py refine` lists it. Forms and paths in nodes/review-world-gen/review.md ("Proposed nodes").

- Source: review decision 62 (B1, C1, D1, E1, E2, E3, F2)
