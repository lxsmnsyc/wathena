---
id: lua-cost-accounting
title: Byte costs for string work in the Lua glue
status: todo
priority: should
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
    why: the maintainer accepted the risk for v0.2.0 (review decision 60, H1)
---

# Byte costs for string work in the Lua glue

## Goal

Charge the Lua CPU budget by bytes for string compares, concatenation, `string.rep`, `find` and `gsub` in the C glue, with a new `lua.wasm`, so a mod that compares long strings in a loop can no longer hold its zone up to the 30-second backstop.

## Context

Outline. Review decision 60 (H1) accepted the risk for v0.2.0 and refused a thread CPU clock (wasi has none). Decision 0054 made the wall-clock limit a 30-second backstop.

- Source: nodes/review-npc-api/review.md, H1 and H2
