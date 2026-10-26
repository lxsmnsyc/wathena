---
id: security-fixes
title: Security known issues for v0.2.0
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
  - deploy/**
  - scripts/release.sh
  - docs/security/review-2026-10.md
provides: []
depends_on:
  - on: review-ops-security
    kind: hard
    why: the review picks which issues v0.2.0 fixes
---

# Security known issues for v0.2.0

## Goal

Fix the security known issues that `review-ops-security` assigns to v0.2.0, from: account role start values, secret file modes, `release.sh` tool and registry checks, Compose database TLS, the passkey reauth window, and removing `staff.owners` by name.

## Context

docs/security/review-2026-10.md 'Known issues'; docs/releases/v0.1.0.md.

- Source: docs/security/review-2026-10.md; proposal 0002 step 37

## Acceptance

- [ ] Each assigned issue has a test or a check; the review's statuses are updated.

## Sizing

Several small fixes in separate areas.
