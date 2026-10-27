---
id: mod-assets-gaps
title: Mod asset gaps: sounds, map folders and mods' maps
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
  - on: release-v0.2.0
    kind: hard
    why: modding follows the parity release
---

# Mod asset gaps: sounds, map folders and mods' maps

## Goal

Review decision 41's leftovers: Ogg and MP3 sounds, pack map folders, zones that run mods' maps, and map record fields (music, minimap). From review decision 62: test worlds run a mod's map folders, home maps included, and `world:new_player { map }` takes `gen:home:<id>` (B3); records that add props to a tileset for layers' objects, with the mod's own art (E4).

## Context

Outline. Break into a full brief when `plan.py refine` lists it.

- Source: strategy.md section 15, phase 9
