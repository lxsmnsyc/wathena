---
id: pre-renewal-parity
title: Pre-renewal ruleset parity and classic mode support
status: todo
priority: must
type: feature
outline: false
milestone: v0.2.0
size: M
spike: false
verify: go test ./server/battle ./tools/convert
gate: false
owns:
  - server/battle/prerenewal.go
  - tools/convert/equipment_test.go
  - tools/convert/testdata/equipment_parity_pre-renewal.md
  - tools/convert/classicnotes.go
provides: []
depends_on: []
---

# Pre-renewal ruleset parity and classic mode support

## Goal

Finish `core.ruleset.pre_renewal` to rAthena's pre-renewal: compare equipment damage in pre-renewal (the parity table says 'not compared' because the test predates `server/battle/prerenewal.go`), fix the differences, add a pre-renewal classic coverage report, and drop the 'experimental' warning once the numbers match.

## Context

tools/convert/equipment_test.go line ~560 and testdata/equipment_parity_pre-renewal.md. The warning is in server/cmd/waserver/main.go lines ~1183-1190. Strategy phase 10 item 1.

- Source: strategy.md phase 10 item 1; parity survey sections 3-4

## Acceptance

- [ ] equipment_parity_pre-renewal.md compares damage for every converted item and card.
- [ ] docs/generated has a pre-renewal classic coverage report.
- [ ] The warning is gone, or stays with the list of what still differs.

## Sizing

Harness exists; the unknown is how many damage differences appear.
