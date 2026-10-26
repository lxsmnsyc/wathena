---
id: proposal-sieges
title: Design proposal: castles and sieges
status: todo
priority: must
type: contract
outline: false
milestone: v0.2.0
size: M
spike: false
gate: true
owns:
  - docs/proposals/0004-sieges.md
  - docs/roadmaps/v0.2-parity/contracts/sieges.md
provides:
  - sieges
depends_on: []
---

# Design proposal: castles and sieges

## Goal

Write design proposal 0004 for castles and War of Emperium (FE and TE): castle data and ownership, economy and defense investment, castle storage, the siege schedule (`OnAgitStart`, `OnAgitEnd`, `agitcheck` 1-3), GvG map flags and their battle rules, guardians, the Emperium and `OnGuildBreak`, emblem flags, walls, `maps.can_enter`, the Lua API (`wa.castles`), the protocol and the database rows. Freeze the `sieges` contract.

## Context

0002b category C lists the commands. Decision 0065 left `maps.can_enter` for WoE:TE flags (server/zone/maps.go:647). Step 44 built `setwall`/`delwall` over cells. There is no guild package: guild code is in server/zone/guild.go, guildscripts.go, skills_guild.go and server/modscript/guild.go. Battle rules are in server/battle and server/zone/combat.go; warps in server/zone/warp.go.

- Source: 0002b category C; proposal 0002 step 14; decision 0065

## Acceptance

- [ ] docs/proposals/0004-sieges.md is accepted.
- [ ] contracts/sieges.md is frozen.

## Sizing

Design across battle rules, guilds, Lua API, schema.

## Notes for the worker

- Stop at the gate.
