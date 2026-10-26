---
id: sieges-content
title: Sieges: convert the guild, guild2 and guild3 folders
status: todo
priority: must
type: feature
outline: true
milestone: v0.2.0
size: M
spike: false
gate: false
provides: []
depends_on:
  - on: sieges-war
    kind: hard
    why: castle scripts run on the real siege rules, not a stub
  - on: extract-folders
    kind: hard
    why: the siege scripts must be in the extract
---

# Sieges: convert the guild, guild2 and guild3 folders

## Goal

Make rAthena's castle and siege scripts convert and run: `guild`, `guild2`, `guild3` (1,267 renewal script NPCs), including `linkflag`, `returnflag`, `f_flag_woe_te` and `f_invest_status`.

## Context

See the frozen contract named in depends_on and its design proposal.

- Source: npc-scripts-renewal.md guild folders
