---
id: character-start-function
title: Character start values in the database
status: todo
priority: later
type: feature
outline: true
milestone: later
size: M
spike: false
gate: false
added: 2026-10-27
provides: []
depends_on:
  - on: release-v0.2.0
    kind: hard
    why: deferred after the parity release by review decision 61
---

# Character start values in the database

## Goal

A SECURITY DEFINER `create_character` function that sets a new character's start values from a start config in the database and logs the start items, so `INSERT` on `characters` and `inventory` can be taken from the account role (security review 2026-10 known issue, review decision 61 D3). First step: design proposal 0007.

## Context

Outline. Break into a full brief when `plan.py refine` lists it.

- Source: review decision 61; docs/security/review-2026-10.md; nodes/review-ops-security/review.md D3
