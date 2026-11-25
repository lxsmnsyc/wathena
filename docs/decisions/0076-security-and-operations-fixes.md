# 0076: Security and operations fixes for v0.2.0

- Status: built.
- Date: 2026-11-25.
- Code: [server/account](../../server/account) (`passkeys.go`), [server/admin](../../server/admin) (`admin.go`, `tools.go`), [server/adminworld](../../server/adminworld) (`moderation.go`, `restore.go`, `tools.go`), [server/retention](../../server/retention), [server/cmd/waserver](../../server/cmd/waserver) (`main.go`), [scripts/release.sh](../../scripts/release.sh), [scripts/check.sh](../../scripts/check.sh).
- Docs: [docs/security/review-2026-10.md](../security/review-2026-10.md), [deploy/README.md](../../deploy/README.md), [server/README.md](../../server/README.md).
- Builds review decision 61 of proposal 0002 (docs/roadmaps/v0.2-parity/nodes/security-fixes/brief.md).

## Context

The operations and security review (review-ops-security) chose five fixes for v0.2.0 and left four items for later.

## Decisions

- D1: a passkey reauth needs user verification in every case. Before, it was refused only when TOTP was on. A passkey login without it still starts a pending TOTP login when TOTP is on.
- D2: `waserver` refuses to start when a name of `staff.owners` has no account. Signup runs inside `waserver`, so the error and the deploy guide say to remove the name, start, sign up, and add the name back, or to start once with `--dev` on a loopback address. `admin.Handler.CheckOwners` replaces `WarnMissingOwners`. With `--dev` it logs a warning. `staff.owners` stays, since it can name more than one owner.
- D5: `scripts/release.sh` checks Go (at least the go.mod line), zig (exactly 0.16.0), Node (major 26) and pnpm (the `packageManager` of client/package.json, read inside client/) before a build, and runs `go mod verify`. `--check-tools` does only that. `scripts/check.sh all` runs it as the new `tools` step.
- E1: the audit and sanction rows of an announcement, a restart plan and cancel, an event start and stop, an online mute and jail, and a kick are written with `context.WithoutCancel` and a 10 second timeout. A request that ends while the zone applies the change no longer answers an error for a change that happened. An event start or stop also waits for the scheduler under that context, since `Scheduler.Do` queues the change before it waits and the change applies whether or not the request ends.
- B3: the retention job takes its cutoffs from the database clock (`DatabaseNow`, a new sqlc query), since the log functions refuse a cutoff by that clock and a process clock that runs ahead failed every run. `Job.Clock`, when set, still wins.
- D4 is an accepted risk: the secret files stay 644 in their 0700 folder, and `secrets.sh` checks the folder on every run. D3 (account role start values), D6 (Compose database TLS) and D7 (unsigned backups) stay known issues. They go to `character-start-function`, distributed-mode and signed-releases.

## Left

- No test for the online mute and jail with a request that ends in the middle of the call. It needs no code change: `Zone.call` waits for the zone's answer once the zone started the change, so the call answers success when the change applied and an error only when it did not start. The rows are written under the context that outlives the request.
- `afterChange` exists in server/admin and server/adminworld with different timeouts. The two packages share no package that fits it, so both stay.
