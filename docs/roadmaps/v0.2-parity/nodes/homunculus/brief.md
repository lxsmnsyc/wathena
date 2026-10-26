---
id: homunculus
title: Homunculus: creation, skills, evolution and AI
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: L
spike: false
gate: false
owns:
  - server/companion/homun*.go
  - server/skill/homun*.go
  - client/src/viewer/homun*
shared:
  - server/modscript/luaapi/spec.go
  - server/modscript/prelude.lua
  - docs/generated/lua-api.md
  - docs/generated/lua/**
  - docs/proposals/0002-lua-api.md
  - docs/generated/item-scripts-*.md
  - tools/convert/use.go
  - tools/convert/output.go
  - schema/proto/**
  - schema/gen/**
  - client/src/net/**
  - server/zone/**
  - server/modscript/**
  - server/store/migrations/**
  - server/store/queries/**
  - server/store/db/**
  - server/battle/testdata/**
  - reference/rathena/**
provides: []
depends_on:
  - on: companion-units
    kind: hard
    why: homunculi are companion units
  - on: extract-sync
    kind: hard
    why: homunculus goldens need new recordings from the fork's recorder
---

# Homunculus: creation, skills, evolution and AI

## Goal

Build homunculus as proposal 0005 describes: Alchemist's Call Homunculus and related skills, the four homunculus types, their skills with goldens, evolution and mutation, intimacy, and the client window. Convert `gethominfo`, `hommutate`, `morphembryo`, `checkhomcall`, `addhomintimacy`, `homevolution`.

## Context

See the frozen contract named in depends_on and its design proposal.

- Source: proposal 0005 (to write)

## Acceptance

- [ ] Homunculus skills are recorded from rAthena with the fork's recorder and their damage matches the goldens.
- [ ] An e2e test calls, levels, feeds and evolves a homunculus.

## Sizing

L: new skill set plus AI; needs recordings.
