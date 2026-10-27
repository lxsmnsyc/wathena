---
id: sieges-content
title: Sieges: convert and run rAthena's castle scripts
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
    why: the guild folders and mapflag lines must be in the extract
---

# Sieges: convert and run rAthena's castle scripts

## Goal

Make rAthena's castle and siege scripts convert and run, including `linkflag`, `returnflag`, `f_flag_woe_te`, `f_invest_status`, the controllers and the SE stones and barricades: 1,267 renewal script NPC lines. Folders: `npc/guild`, `npc/guild2`, renewal `npc/re/guild3` and `npc/re/guild` (`invest_main`, `mission_main`), `npc/re/quests/woe_te`, and the castle warps. An e2e of one FE, one SE and one TE castle, from the controller's schedule on the dev clock to a capture and the next day's treasure.

## Context

Proposal 0004 is accepted and the `sieges` contract is frozen. Read the proposal's section for this node and the contract first. Each node writes its own decision record. Shares tools/convert, docs/generated, reference extracts, e2e and server/zone for fixes the scripts reveal. If the e2e checks flag emblems in the client, it also needs sieges-ui.

- Source: docs/proposals/0004-sieges.md (section 15, 'Who builds what'); nodes/proposal-sieges/outcome.md
