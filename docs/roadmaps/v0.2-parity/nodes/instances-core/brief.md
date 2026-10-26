---
id: instances-core
title: Instances: lifecycle, maps, variables and Lua API
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - server/instance/**
  - tools/convert/npcinstance*.go
  - data/instances.yml
shared:
  - server/world/**
  - server/social/**
  - server/mod/**
  - server/gamedata/**
  - server/rules/**
  - server/cmd/waserver/**
  - server/character/**
  - reference/rathena/extracts/**
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
  - tools/convert/rathena.lua
  - tools/convert/npcexpr.go
  - docs/generated/npc-scripts-*.md
  - docs/generated/classic-coverage.md
  - tools/convert/output.go
  - schema/proto/**
  - schema/gen/**
  - client/src/net/**
  - server/zone/**
  - server/modscript/**
provides: []
depends_on:
  - on: proposal-instances
    kind: contract
    via: instances
  - on: npc-api-events
    kind: hard
    why: instance NPCs use the cross-zone handles and player reads that npc-api-events builds in server/zone
  - on: extract-sync
    kind: hard
    why: instances.json and the base map flags come from the fork's extract.py
---

# Instances: lifecycle, maps, variables and Lua API

## Goal

Build the server side of instances as proposal 0003 describes: `data/instances.yml` from rAthena's `instance_db.yml`, the registry and its build queue, instance zones from base maps (`instance:<id>:<base>`, `Zone.Asset`), lifecycle with idle timeout and time limit, ending through rotation's removal steps with each player sent to their save point, ownership and the party and guild hooks, `'x` variables, NPC copies and `instance_start`, the `wa.instances` module and handle, the ruleset rules, the protocol messages, and the converter rows of every `instance_*` command. Measure the Lua state cost per instance map and set `max_running`. No migration: instances are not saved. The client window, the admin screen and the staff endpoint are `instances-ui`.

## Context

See the frozen `instances` contract.

Code points the proposal found (nodes/proposal-instances/outcome.md): `startNPCs` is lazy, so instance zones start NPCs at build; `NPCDirectory.AddZone` keeps the first record of a name, so copies are named `<full name>:<id>` and `NPCKey` returns the base name; `mapZone` adds `map/`, so instance IDs need their own lookup; `MapChange.map` and `Welcome.map` must send `Zone.Asset`; `Zone.Evacuate` sends everyone to one cell, instances need each player's save point; `World.JoinCharacter` must send a character on a gone instance map to the save point, or back in when reconnect allows.

- Source: docs/proposals/0003-instances.md

## Acceptance

- [ ] An e2e test creates an instance for a party, enters it, runs an `OnInstanceInit` NPC, and destroys it on timeout.
- [ ] `instance_*` command rows drop in the reports.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

L: core zone change and new API; known zone patterns.
