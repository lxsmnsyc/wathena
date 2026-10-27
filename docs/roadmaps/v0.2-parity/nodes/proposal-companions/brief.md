---
id: proposal-companions
title: Design proposal: pets, homunculus and mercenaries
status: done
priority: must
type: contract
outline: false
milestone: v0.2.0
size: M
spike: false
gate: true
owns:
  - docs/proposals/0005-companions.md
  - docs/roadmaps/v0.2-parity/contracts/companions.md
provides:
  - companions
depends_on: []
---

# Design proposal: pets, homunculus and mercenaries

## Goal

Write design proposal 0005 for the three companion systems on top of decision 0016's companion model: pets (taming, eggs, hunger, intimacy, loot, pet bonuses), homunculus (creation, skills, evolution, mutation, intimacy, AI), and mercenaries (contracts, faith, skills). Lua API, protocol, database. Freeze the `companions` contract.

## Context

Decision 0016 and server/companion cover only option-bit records (falcon, peco, dragon, warg, Madogear): no entity, AI, owner link or persistence. Pet egg drops are in server/zone/loot.go:213. Pets, homunculus and mercenaries are units that need a shared base in server/zone (entities, monster AI in mobai.go, companions.go) and server/modscript/companions.go; the proposal designs that base. 0002b category C. Item scripts blocked: `pet` 68, `mercenary_create` 59, homunculus items.

- Source: 0002b category C; strategy.md section 11.2; decision 0016

## Acceptance

- [ ] docs/proposals/0005-companions.md is accepted.
- [ ] contracts/companions.md is frozen.

## Sizing

Three systems in one design; they share the companion model.

## Notes for the worker

- Stop at the gate.
