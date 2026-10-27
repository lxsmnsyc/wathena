# Log: review-ops-security

- 2026-10-27: Read GOAL.md, CLAUDE.md, the brief, the briefs of security-fixes, security-review-v0.2 and release-v0.2.0, review-npc-api's review.md and outcome.md, and review decision 60.
- Read decisions 0048, 0049, 0050 and 0054, docs/security/review-2026-10.md, proposal 0002 steps 19 and 34 to 37 (and review decision 54's settled items), docs/releases/v0.1.0.md.
- Finding: strategy.md has no "Open review items" section any more (the roadmap replaced it). The items are in the decisions' "Review items" lists and in proposal 0002's steps.
- Finding: step 19's permission names all do something now. Every name of the catalog has a route or a GM command (server/admin/admin.go, server/zone/gmcommands.go). Settled.
- Finding: the scheduled verify checks the newest backup every interval, also when it is the same file (tools/wactl/backup.go:1220). The backup schedule drifts only by the run's start delay (backup.go:702).
- Finding: `GET /retention` shows no personal data and a fixed error text (server/admin/retention.go:36). `world.read` stays.
- Finding: the retention job takes its cutoff from the server clock (server/retention/retention.go:258) and the SQL functions compare with the database clock (migration 00037).
- Finding: the passkey reauth refuses an unverified passkey only when TOTP is on (server/account/passkeys.go:545-549).
- Finding: `staff.owners` lists more than one owner; `world.owner_account_id` holds one. A listed name that is deleted is refused by the account service (server/account/deletion.go:120). The only window left is a name with no account at start.
- Finding: Announce, PlanRestart and CancelRestart ignore their context. After the zone applied a change, the handlers and `adminworld` write the audit row with the request's context (admin/tools.go:90 and :124, adminworld/tools.go:445 and moderation.go:241 and :310). A timeout there answers an error for a change that happened, and a retry repeats it.
- Finding: v0.1.0 was tagged after migration 00040, so every world made by `wactl world new` since has an owner ID. Only dev worlds from before 2026-10-19 lack one.
- Wrote review.md and outcome.md. Read-only: grep and file reads. No build, no tests, no Docker.
