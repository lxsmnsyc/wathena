# Log: security-fixes

Branch fix/security-fixes from batch/v0.2-parity. Decision record 0076. PostgreSQL: colima + one postgres:18 container, WATHENA_TEST_DATABASE_URL.

## 1. D1 passkey reauth

- Not there: passkeys.go refused a reauth without UV only when TOTP was on.
- Test first: `go test -p 1 -parallel 2 -count=1 -run TestPasskeyReauthNeedsUserVerification ./server/account`. FAIL before: "reauth without UV and without TOTP: 200".
- Fix: `if !verified` in the reauth branch. Test passes; TestPasskeys still passes.

## 2. D2 owners without account

- Not there: `WarnMissingOwners` only warned.
- Test first: `TestCheckOwners` in server/admin/admin_test.go. Before: build failure, `CheckOwners` undefined (the check had no failing form). After: `go test -p 1 -parallel 2 -count=1 -run 'TestCheckOwners|TestMissingOwners' ./server/admin` passes.
- Fix: `Handler.CheckOwners(ctx, dev)` replaces `WarnMissingOwners`; main.go returns its error. Docs: deploy/README.md, server/README.md (2 places), deploy/compose/waserver.yml.

## 3. D5 release.sh tool checks

- Not there: release.sh only checked that go, zig and pnpm were on PATH.
- Test first: a fake `zig` (prints 0.15.0) first on PATH, `scripts/release.sh --check-tools`. Before: "unknown target --check-tools", exit 2 (the flag did not exist). After: "zig 0.15.0, but the release needs 0.16.0", exit 1; a fake 0.16.0 gives "tools ok", exit 0. The real zig here is 0.17.1 and fails the check, as it should.
- pnpm is read inside client/, where it switches to the packageManager version (outside it prints 12.14.0).
- Fix: `check_tools` in scripts/release.sh, run on every build and by `--check-tools`; new `tools` step in scripts/check.sh, in `all` only.

## 4. E1 audit rows after a zone change

- Not there: the audit and sanction rows used the request context.
- Tests first: `TestAuditAfterEndedRequest` (server/admin/tools_test.go: announce, plan and cancel a restart; the request ends inside the fake world's call) and `TestKickRowAfterEndedRequest` (server/adminworld/moderation_test.go: the kicker ends the request). Before: announce, plan and cancel gave 503 with no rows; the kick gave "context canceled". After: both pass (`-p 1 -parallel 2 -run ...`), with TestRestartCalls, TestWorldCalls, TestSanctions, TestCommandSanctions and TestEventOverridesSurviveRestart.
- Fix: `afterChange(ctx)` in server/admin/tools.go and server/adminworld/restore.go (`context.WithoutCancel` plus a 10 s timeout) for the rows of announce, plan restart, cancel restart, event start and stop, online mute and jail, and kick.
- No test for event start/stop and online mute/jail: `Scheduler.Do` and `StaffSanction` select on the request context against the zone's answer, so a hang-up inside them is a race, not a fixed order. The fix at those rows is the same call as the tested ones.

## 5. B3 retention clock

- Not there: `RunOnce` took `start` from the process clock.
- Test first: `TestJobClockSkew` (server/retention/retention_test.go) with the process clock one hour ahead (hook `SetWallClock` in export_test.go) and 30-day periods. Before: "the cutoff ... is less than 30 days ago (SQLSTATE P0001)" for the logs and the audit. After: passes, and a `Clock` set on the job still wins (the same test checks that a Clock an hour ahead is refused).
- Fix: `runStart` reads `now()` from the database unless `Clock` is set. Needed a new sqlc query `DatabaseNow` (server/store/queries/retention.sql, generated server/store/db/retention.sql.go) and `store.DatabaseNow`: outside the brief's paths, recorded under Deviations. sqlc v1.31.1 made the generated file.

## 6. Documents and checks

- Statuses in docs/security/review-2026-10.md, the D4 line in deploy/README.md "Secrets", decision 0076.
- Tests, one at a time, `-p 1 -parallel 2`: full packages server/account, server/admin, server/adminworld, server/retention pass; `-run 'Retention|Owners|Role'` on server/cmd/waserver and server/store pass.
- `go vet` on the touched packages, `gofmt -l`, `scripts/check.sh lint`: clean. Test container removed after the run.

## 7. Review fixes

- D2 text: error, deploy/README.md and review-2026-10.md say the real recovery and that the window exists only while the server runs.
- B3: duration logged from the process clock (`began`), not from the database start.
- E1: `TestEventStartAfterEndedRequest` first: "start with an ended request: context canceled". Fix: `Scheduler.Do` under `afterChange`; the test passes. Mute and jail: `Zone.call` waits for the zone once the change started, so no window; no test.
- D5: guarded each tool call with `|| have=""`; a fake zig that exits 3 now prints "zig unknown, but the release needs 0.16.0", exit 1. Go version parsing takes the numbers of `devel go1.28-abc` (1.28) and `go 1.25rc1` (1.25).
- Stale text: scripts/check.sh header, passkeys.go notes and doc comment, decision 0005.
