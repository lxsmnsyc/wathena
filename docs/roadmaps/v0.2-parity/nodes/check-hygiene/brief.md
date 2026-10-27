---
id: check-hygiene
title: Check script hygiene: leaked volumes and tool pins
status: active
priority: must
type: chore
outline: false
milestone: v0.2.0
size: S
spike: false
gate: false
owns:
  - scripts/check.sh
  - tools/gosec/**
provides: []
depends_on: []
---

# Check script hygiene: leaked volumes and tool pins

## Goal

Stop `scripts/check.sh` from leaking an unnamed Docker volume per run (`docker rm -f` without `-v`, line 150), and drop the `tools/gosec` module once a gosec release reads Go 1.27.2's export data.

## Context

strategy.md 'Known issues'. The disk filled before; leaked volumes add to it, and every later node runs the checks, so this goes first.

- Source: strategy.md 'Known issues'

## Acceptance

- [ ] Two runs of `scripts/check.sh test` leave no new unnamed volume (`docker volume ls -qf dangling=true`).

## Sizing

One line, plus a version check.
