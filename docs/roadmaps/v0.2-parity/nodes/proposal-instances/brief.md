---
id: proposal-instances
title: Design proposal: instances
status: done
priority: must
type: contract
outline: false
milestone: v0.2.0
size: M
spike: false
gate: true
owns:
  - docs/proposals/0003-instances.md
  - docs/roadmaps/v0.2-parity/contracts/instances.md
provides:
  - instances
depends_on: []
---

# Design proposal: instances

## Goal

Write design proposal 0003 for instances (memorial dungeons): instance lifecycle (create, enter, destroy, time limits, idle timeout), instance maps as copies of base maps with their own zone, `'x` variables, `OnInstanceInit`, NPCs on instance maps, party/guild/character ownership, the Lua API (`wa.instances`), the protocol messages, and the database rows. Freeze the `instances` contract.

## Context

`variables.instance_*` refuses with 'instances do not exist yet' (server/modscript/variables.go:69, test server/zone/variables_test.go:109). Zones, maps and their start and stop live in server/zone and server/world; NPC duplication in server/zone/npcs and server/modscript. 0002b category C lists the commands. Instances are the largest single blocker: about 3,500 renewal NPC lines. Zones run one goroutine per map; an instance is a new zone.

- Source: 0002b category C; npc-scripts-renewal.md instances folder

## Acceptance

- [ ] docs/proposals/0003-instances.md is accepted by the maintainer.
- [ ] contracts/instances.md is frozen and links to it.

## Sizing

Design across zone, Lua API, protocol and schema.

## Notes for the worker

- Stop at the gate for the maintainer's review.
