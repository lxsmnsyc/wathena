# Outcome: review-ops-security

## Built

- Accepted: the maintainer accepted all 12 recommendations on 2026-10-27; recorded as review decision 61 in docs/proposals/0002-lua-api.md.

- review.md: 19 entries by source decision, a summary table, 12 questions for the maintainer with recommended answers, the list security-fixes builds, proposed brief changes and "Draft review decision 61". Review decision number taken: none yet (the orchestrator records 61 after approval). Decision record number taken: none.
- Counts: 5 accept (B3, D1, D2 hardening, D5, E1), 7 defer (A2, B2, C2, C4, D3, D6, D7), 4 refuse (A1, C1, C3, D4; D2 refuses the removal), 3 keep or settled (B1, E2, F1).
- security-fixes builds: D1 passkey reauth needs user verification; D2 `waserver` refuses a `staff.owners` name with no account (not with `--dev`); D5 `release.sh` version checks and `--check-tools`; E1 audit and sanction rows after a zone change use a context that outlives the request; B3 retention cutoffs from the database clock; the review-2026-10.md statuses and one deploy guide line.
- Proposed new node: `character-start-function` (later, size M, proposal 0007 first) for the account role's start values.

## Deviations

- strategy.md has no "Open review items" section any more. The items were taken from the decisions' "Review items" lists and proposal 0002's steps.
- Added E1's concrete cases, E2 (settled by review decision 60) and F1 (step 19, settled), which the brief lists only as one line each.
- Step 37's other review items (`node:hash`, `/gen/maps`, owners by ID, passkey factor) were already settled by review decision 54 and are not entries.

## Stubs left

- None. Nothing was built.

## Gotchas for security-fixes

- Its owned paths do not cover E1, B3 and D2. It needs `server/admin/**`, `server/adminworld/**`, `server/retention/**`, `server/cmd/waserver/**` and `docs/decisions/**`.
- docs/security/review-2026-10.md matches security-review-v0.2's glob `docs/security/review-2026-*.md` too. The nodes run in sequence, so the order is security-fixes first.
- D1: change the reauth branch only (`finishPasskeyLogin`, `c.kind == "reauth"`). A passkey login without UV still starts a pending TOTP login when TOTP is on.
- D2: `MissingOwners` already exists in server/admin/admin.go. Compare names case-insensitively, as it does. With `--dev` keep the warning.
- D5: Node's major version has no pin in the repo besides the docs (scripts/check.sh:80, "Node 26"). Check the major only. Go's version is the `go` line of go.mod.
- E1: Announce, PlanRestart and CancelRestart ignore their context. Only the audit writes after them need the fresh context. Use `context.WithoutCancel` with a timeout, as server/adminworld/restore.go:279.
- B3: `Job.Clock` is a test hook. Keep it winning over the database read.
- Review decision 61 is not yet in proposal 0002. Cite it only after the orchestrator records it.

## Verification

- Read-only review: grep and file reads. No build, tests, Docker or scripts/check.sh were run.

## Size

- Estimated S, actual S to M.
