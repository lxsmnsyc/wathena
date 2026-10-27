---
id: security-fixes
title: Security and operations fixes for v0.2.0
status: todo
priority: must
type: fix
outline: false
milestone: v0.2.0
size: M
spike: false
gate: false
owns:
  - server/account/**
  - server/admin/**
  - server/adminworld/**
  - server/retention/**
  - server/cmd/waserver/**
  - deploy/**
  - scripts/release.sh
  - docs/security/review-2026-10.md
shared:
  - scripts/check.sh
  - server/README.md
  - docs/decisions/**
provides: []
depends_on:
  - on: review-ops-security
    kind: hard
    why: the review picks which issues v0.2.0 fixes
---

# Security and operations fixes for v0.2.0

## Goal

Build what review decision 61 (docs/proposals/0002-lua-api.md) assigns to v0.2.0, in this order:

1. D1: a passkey reauth needs user verification in every case (server/account/passkeys.go:546 refuses it only when TOTP is on).
2. D2: `waserver` refuses to start when a `staff.owners` name has no account, except with `--dev` (a warning). `staff.owners` stays.
3. D5: `scripts/release.sh` checks Go (go.mod), zig 0.16.0, Node major 26 and pnpm (`packageManager`) and runs `go mod verify`, with a `--check-tools` flag that `scripts/check.sh all` runs.
4. E1: after a zone applies an admin change (announce, restart plan and cancel, event start and stop, online mute and jail, kick), its audit and sanction rows are written with a context that outlives the request.
5. B3: the retention job takes its cutoffs from the database clock.
6. Statuses in docs/security/review-2026-10.md for D1, D2, D4 (accepted risk), D5 and the deferred D3, D6, D7; one line in deploy/README.md "Secrets" for D4.

## Context

The tests and files for each item are in nodes/review-ops-security/review.md ("Items security-fixes builds") and its outcome.md. Not built here: account role start values (character-start-function), secret file modes (accepted risk), Compose database TLS (distributed-mode), unsigned backups (signed-releases). Record what is built in a decision record (next free number).

- Source: review decision 61; docs/security/review-2026-10.md

## Acceptance

- [ ] Each of items 1 to 5 has the test or check review.md names.
- [ ] The review's statuses are updated.
- [ ] Fast check passes: `scripts/check.sh quick`

## Sizing

M: five small fixes in separate areas.
