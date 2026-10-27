---
id: review-ops-security
title: Review the open operations and security questions
status: active
priority: must
type: docs
outline: false
milestone: v0.2.0
size: S
spike: true
gate: true
owns:
  - docs/roadmaps/v0.2-parity/nodes/review-ops-security/**
provides: []
depends_on: []
---

# Review the open operations and security questions

## Goal

Settle the open review items of backups, retention, load tests, logins and the security review, so `security-fixes` and the release node have a fixed list.

## Context

Items from decisions 0048-0050, 0054 and the security review (proposal 0002 step 37, docs/security/review-2026-10.md).

- Source: strategy.md 'Open review items' (0048-0050, security); docs/security/review-2026-10.md known issues

## Steps

- [ ] Decision 0048 (step 34): skip a backup the scheduled verify already checked; a time of day (`--at`) for the schedule.
- [ ] Decision 0049 (step 35): is `world.read` right for `GET /retention`; other Prometheus measurements; server clock ahead of the database clock.
- [ ] Decision 0050 (step 36): document 500 players; the 1,000-players-in-one-town slowdown (stays in `distributed-mode`); bot placement without a database URL; a run on a dedicated machine.
- [ ] Security review (step 37): passkey reauth without user verification refreshing the 10-minute window; when to remove `staff.owners` by name; which known issues v0.2.0 fixes (account role start values, secret file modes, `release.sh` tool checks, Compose database TLS, unsigned backups).
- [ ] Decision 0054: the admin API reporting an error for a change that happened.
- [ ] Proposal 0002 step 19: permission names that do nothing yet.

## Acceptance

- [ ] A review decision lists every item as accepted, refused or deferred, and names the items `security-fixes` builds.

## Sizing

Few items, mostly yes or no.

## Notes for the worker

- Stop at the gate.
