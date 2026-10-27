---
id: review-world-gen
title: Review the open world generation and content questions
status: todo
priority: should
type: docs
outline: false
milestone: v0.2.0
size: M
spike: true
gate: true
owns:
  - docs/roadmaps/v0.2-parity/nodes/review-world-gen/**
shared:
  - docs/proposals/0002-lua-api.md
provides: []
depends_on: []
---

# Review the open world generation and content questions

## Goal

Settle the open review items of phase 7 (generator, layout, spawns, quests, procedural content, rotation, economy) in one review decision, and turn the accepted ones into build nodes.

## Context

These items came out of decisions 0040 to 0047. None blocks classic parity, so this review is `should`. It was item 1 of strategy.md 'Next'.

- Source: strategy.md 'Open review items' and 'Next' item 1; proposal 0002 next steps 17, 28-33

## Steps

- [ ] Decision 0040 (next step 28): `wa.testing` pick of a test world's template; mods' own tilesets and props; a floor's level band in Lua; NPC placed by tag with no match; dungeon IDs for 2 dungeons of one style.
- [ ] Decision 0041 (step 29): Lua read of graph distances or teleport prices; record field for server services on NPCs; `wa.testing` start on a mod's home map; band formula in small worlds; longer `@renametown` names; entered maps and memo points in the data export.
- [ ] Decision 0042 (step 30): monster level, race, element and tags in the spawn hook; Lua read of a map's stored spawns; dungeon style spawn biome; theme chance setting; flag `spawns.modify`.
- [ ] Decision 0043 (step 31): Lua list of a player's quests and read of a quest record; quest event options; quest calls for another character; `setquestinfo`.
- [ ] Decision 0044 (step 32): Lua read of a world's quests; a layer's read of map cells; places scope of fields; objects of a mod's own art; board quest limits and repeats; flag `world.generate`.
- [ ] Decision 0045 (step 33): hooks and layers on rotating floors and changed fields; Lua read of rotation date or season; forced changes across a restart and a cancel; new bands with a world change; more than one rotating dungeon; locale text and a banner for warnings; jail return points on removed maps.
- [ ] Decision 0046: the model player and targets; economy checks after `spawn_table` hooks; Lua read of a field's simulated rates.
- [ ] Decision 0047: `content.GuildLevel` as a `procedural.layout` setting.
- [ ] Proposal 0002 step 17: reward lists for automatic events; a map type in map data for `level_band`.

## Acceptance

- [ ] A review decision lists every item as accepted, refused or deferred.
- [ ] Accepted items have a proposed node.

## Sizing

No code; many items across 9 records.

## Notes for the worker

- Stop at the gate. Do not build.
