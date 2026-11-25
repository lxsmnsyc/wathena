# Outcome: security-fixes

## Built

None of the five fixes existed on batch/v0.2-parity. All were built. Decision record number taken: 0076 (docs/decisions/0076-security-and-operations-fixes.md). No review decision, proposal or migration number taken.

1. D1 (server/account/passkeys.go): a passkey reauth is refused when the key did not verify the user, with or without TOTP. The message says "Use your password instead" when TOTP is off.
2. D2: `admin.Handler.CheckOwners(ctx, dev)` (server/admin/admin.go) replaces `WarnMissingOwners`. `waserver` (server/cmd/waserver/main.go) returns its error, so it refuses to start while a `staff.owners` name has no account. With `--dev` it warns. Docs: deploy/README.md, server/README.md (two places), deploy/compose/waserver.yml.
3. D5 (scripts/release.sh): `check_tools` checks Go (at least the go.mod line), zig 0.16.0, Node major 26 and pnpm (`packageManager`, read inside client/), then runs `go mod verify`. It runs before every build and alone with `--check-tools`. scripts/check.sh has a new `tools` step, in `all` only. server/README.md "Release builds" describes it.
4. E1: `afterChange(ctx)` (server/admin/tools.go and server/adminworld/restore.go) gives the audit and sanction rows a context that outlives the request. Used for announce, plan restart, cancel restart, event start and stop, online mute and jail, and kick.
5. B3: `Job.runStart` (server/retention/retention.go) reads the cutoff base from the database through the new `DatabaseNow` query and `store.DatabaseNow`. `Job.Clock`, when set, still wins.
6. Documents: docs/security/review-2026-10.md has statuses for D1, D2 (hardened), D3 (deferred to `character-start-function`), D4 (accepted risk), D5, D6 (deferred to distributed-mode) and D7 (deferred to signed-releases). deploy/README.md "Secrets" has the D4 line.

## Proof

- base (batch/v0.2-parity, 126d0fe), test written first, each one failed:
  - D1: `go test -p 1 -parallel 2 -count=1 -run TestPasskeyReauthNeedsUserVerification ./server/account` gave "reauth without UV and without TOTP: 200".
  - D2: `TestCheckOwners` (server/admin) did not build: `CheckOwners` undefined. The old behavior only warned.
  - D5: `scripts/release.sh --check-tools` with a fake `zig` printing 0.15.0 first on PATH gave "unknown target --check-tools", exit 2.
  - E1: `TestAuditAfterEndedRequest` (server/admin) gave 503 for announce, plan and cancel, with no audit rows. `TestKickRowAfterEndedRequest` (server/adminworld) gave "context canceled".
  - B3: `TestJobClockSkew` (server/retention), process clock one hour ahead, 30-day periods, gave "the cutoff ... is less than 30 days ago (SQLSTATE P0001)".
- branch: the same commands pass. The fake zig 0.15.0 gives "zig 0.15.0, but the release needs 0.16.0", exit 1. A fake zig 0.16.0 gives "tools ok", exit 0. `TestJobClockSkew` also checks that a `Clock` an hour ahead is still refused (Clock wins).

## Deviations

- B3 needed a new sqlc query, outside the brief's paths: `DatabaseNow` in server/store/queries/retention.sql, the generated server/store/db/retention.sql.go (sqlc v1.31.1), and `DatabaseNow` in server/store/retention.go. sqlcheck.sh forbids raw queries, so there was no smaller way.
- Test hooks: server/retention/export_test.go (`SetWallClock`) and `wallClock` in retention.go.
- Review fixes: the D2 error and guide text give the real recovery (signup runs inside waserver); the retention log duration uses the process clock for both ends; release.sh prints its message when a tool crashes and reads Go versions such as `devel go1.28-x` and `1.25rc1`; stale text in check.sh, passkeys.go and decision 0005.
- E1: event start and stop now also wait for the scheduler under the `afterChange` context (`Scheduler.Do` queues the change, so it applied even when the request ended). `TestEventStartAfterEndedRequest` tests it with an ended request. Before: "context canceled"; after: success and two audit rows. Online mute and jail have no test with a request that ends mid-call (no hook in the zone's call), but `Zone.call` waits for the answer once the change started. Written in decision 0076 "Left".
- The two `afterChange` copies (server/admin, server/adminworld) stay: no shared package fits.
- Passkey reauth message changed when TOTP is off (small wording change in passkeys.go).
- The local zig is 0.17.1, so `scripts/release.sh --check-tools` fails on this machine, as designed.
- The review's test for D2 is at the `CheckOwners` level, not a process start.

## Stubs left

- None.

## Gotchas

- `scripts/check.sh all` now fails on a machine whose zig is not 0.16.0, or whose Node major or pnpm differ.
- pnpm prints another version outside client/ (it switches to `packageManager` inside it), so release.sh reads it inside client/.
- A retention run now needs one extra read of `now()` on the game pool before the lock. A database error there fails the run.

## Proposed learnings

- Rows that record a change a zone or world already applied use a context made with `context.WithoutCancel` and a timeout | an ended request must not turn an applied change into an error | server/admin, server/adminworld | no
- Retention cutoffs come from the database clock | the SQL functions check them by that clock | server/retention | yes

## Verification

- Targeted, one at a time, with `-p 1 -parallel 2` and a single postgres:18 container (colima): `./server/account`, `./server/admin`, `./server/adminworld`, `./server/retention` full packages, and `-run 'Retention|Owners|Role'` on `./server/cmd/waserver` and `./server/store`. Results in log.md.
- `go vet` on the touched packages: clean. `gofmt -l`: clean.
- `scripts/check.sh lint` and the commit: see log.md.
- `scripts/check.sh quick` and `all` were not run, as instructed.

## Size

- Estimated M, actual M.
