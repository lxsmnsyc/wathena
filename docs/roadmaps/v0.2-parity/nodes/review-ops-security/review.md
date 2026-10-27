# Review: open operations and security questions

Draft for the maintainer, 2026-10-27. Not yet approved.

Review of the items in brief.md "Steps", checked against the code at b81d3c3. The sources are decisions 0048, 0049, 0050 and 0054, docs/security/review-2026-10.md, proposal 0002 next steps 19 and 34 to 37, and docs/releases/v0.1.0.md. strategy.md no longer has an "Open review items" section (the roadmap replaced it), so the items come from the decisions' "Review items" lists and the proposal's steps.

States: **built** (the code does it), **settled** (a later decision or the code already decides it), **open**.
Recommendations: **keep** (accept as built), **accept** (with the exact change), **defer** (with a milestone), **refuse**.

## Summary

| # | Item | Recommendation | Node |
|---|---|---|---|
| A1 | Skip a backup the scheduled verify already checked | refuse | none |
| A2 | `--at` time of day for the backup schedule | defer: later; document the workaround | release-v0.2.0 (docs) |
| B1 | `world.read` for `GET /retention` | keep | none |
| B2 | Other Prometheus measurements | defer: later | none |
| B3 | Server clock ahead of the database clock | accept: cutoffs from the database clock | security-fixes |
| C1 | Document 500 players | refuse; the release documents what its own run meets | release-v0.2.0 |
| C2 | 1,000 players in one town; 500 in view | defer: later | distributed-mode |
| C3 | Bot placement without a database URL | refuse | none |
| C4 | A run on a dedicated machine | defer: later | distributed-mode |
| D1 | Passkey reauth without user verification | accept: refuse it always | security-fixes |
| D2 | Remove `staff.owners` by name | refuse removal; accept: refuse to start with a name that has no account | security-fixes |
| D3 | Account role start values | defer: later, proposal first | character-start-function (new) |
| D4 | Secret file modes | refuse: accepted risk | none |
| D5 | `release.sh` tool checks | accept | security-fixes |
| D6 | Compose database TLS | defer: later | distributed-mode |
| D7 | Unsigned backups | defer: modding | signed-releases |
| E1 | Admin API reports an error for a change that happened | accept | security-fixes |
| E2 | Lua time limits (0054) | settled by review decision 60 | none |
| F1 | Permission names that do nothing yet (step 19) | settled | none |

Totals by row: 5 accept (B3, D1, D2 hardening, D5, E1), 7 defer (A2, B2, C2, C4, D3, D6, D7), 4 refuse (A1, C1, C3, D4; D2 refuses the removal), 3 keep or settled (B1, E2, F1).

## A. Decision 0048: encrypted backups (step 34)

### A1. Skip a backup the scheduled verify already checked

- Source: decisions/0048-encrypted-backups.md:243.
- State: open. `verifier.loop` checks the newest backup at start and then every interval (tools/wactl/backup.go:1220, :1233 `latestVerifiable`). It keeps no record of the file it checked last.
- Facts: with the default schedule both run every 24 hours, so each check normally sees a new backup. A repeat only happens when the backup service stopped, failed, or has no recipient. A repeat costs one restore into a temporary database.
- Options:
  1. Keep: a repeat check reads the same file again. Cost: one restore a day while backups are off. A repeat also finds a damaged file (disk errors on the volume), which a one-time check cannot.
  2. Remember the last checked file and its modification time, and log "already verified" instead of restoring. Cost: small, in tools/wactl; loses the bit-rot check.
- Recommendation: refuse option 2. Keep the repeat. When backups stop, a daily error from the failing backup service is the signal; the verify log adds the file's age so the owner sees it (docs only, deploy/README.md "Backups"). No code change.

### A2. `--at`, a time of day for the schedule

- Source: decisions/0048-encrypted-backups.md:244.
- State: open. The first run is due one interval after the newest backup in the folder, or at once (backup.go:640 `backupLoop`). The next run is the last backup's start plus the interval (backup.go:702), so the time of day moves only by the start delay of each run.
- Workaround that works now: run one manual backup at the wanted hour (`docker compose run --rm backup world backup --dir=/backups ...`, deploy/README.md "The backup service"), then start or recreate the service. The schedule keeps that hour.
- Options:
  1. `--at HH:MM` (UTC) with `--every 24h`: the next run is the next such time at least one interval after the last backup. Cost: small, in tools/wactl, plus `WATHENA_BACKUP_AT` in `.env` and the verify schedule (it must run after the backup).
  2. Document the workaround and wait for a request.
- The backup is one read-only snapshot (`pg_export_snapshot`), so its load on a live world is small.
- Recommendation: defer to later, and take option 2 now. release-v0.2.0 adds the workaround to the deploy guide's "Backups".

## B. Decision 0049: retention (step 35)

### B1. `world.read` for `GET /retention`

- Source: decisions/0049-retention.md:201; proposal 0002 step 35.
- State: built and checked. The call is registered with `world.read` (server/admin/admin.go:142). It shows the periods, the run times, the counts per kind and, since review 2026-10, a fixed error text (server/admin/retention.go:36). It holds no personal data and writes no audit row.
- Options: `world.read` (the World screen's permission, which `game_master` and `admin` have); `audit.read` (the log's permission, which carries more weight than the data warrants); a new `retention.read` (a new name in the catalog for one read).
- Recommendation: keep `world.read`. The screen that shows it needs that permission anyway.

### B2. Other Prometheus measurements

- Source: decisions/0049-retention.md:7 (the metrics); GOAL.md maps strategy phase "Left" lines to the inbox.
- State: open. `/debug/wathena/metrics/prometheus` serves two retention series only (server/retention/retention.go:351). Tick work, GC pauses, CPU, memory and bytes per connection are JSON at `/debug/wathena/metrics` (server/metrics). go.mod has no Prometheus client; the text is written by hand.
- Options:
  1. Write the zone and process measurements in the same text format. Cost: medium; names, units and histogram buckets need a decision.
  2. Keep JSON for them. Owners who run Grafana can convert with a small script.
- Recommendation: defer to later. No script or v0.2.0 node needs it.

### B3. Server clock ahead of the database clock

- Source: decisions/0049-retention.md:197.
- State: open, known. The job computes each cutoff from its own clock (server/retention/retention.go:258, `start.Add(-days)`). The SQL functions refuse a cutoff less than 30 days before the database's `now()` (migration 00037). With `item_log_days` or `audit_days` at 30 and a server clock ahead, the run fails and logs the error. The next run fails too until the clocks agree.
- Options:
  1. Keep: the error is logged, and the setting is the minimum. Cost: none; the failure repeats daily and the World screen shows "The last run failed".
  2. The job reads `SELECT now()` once at the start of each run and takes every cutoff from it. Cost: small, in server/retention. The rows' own times come from the database's `now()` defaults, so this also matches them better.
  3. Raise the minimum period to 31 days. Cost: changes the documented range and the privacy page.
- Recommendation: accept option 2. Keep `Job.Clock` for tests; it replaces the database read only when set. Test: a job whose clock is 1 hour ahead and a period of 30 days runs without error. Node: security-fixes (it owns the area for this review; no schema change).

## C. Decision 0050: load tests (step 36)

### C1. Document 500 players

- Source: decisions/0050-load-tests.md:130, :62-63; docs/releases/v0.1.0.md:67-71.
- State: open. v0.1.0 documents 250 and claims nothing above. At 500, three of ten runs had 1.06% to 1.17% of ticks over 10 ms; the zone thread's CPU time p99 stayed under 6 ms. The machine, an 8 GB Mac shared with the desktop, caused the long ticks (waits for memory, swap use).
- v0.2.0 adds instances, sieges, companions and chat rooms. Each adds work per tick, so the v0.1.0 numbers do not carry over.
- Options:
  1. Document 500 now from the v0.1.0 runs. Cost: a claim the project's own rule does not support (the rule is every run of every scenario but Crowd under 1%).
  2. Keep the rule and let the release run decide. release-v0.2.0 runs the scenarios from 250 in steps of 250 on the release commit, repeats each step near the limit at least three times, and documents the highest count at which every run met the rule. It states the machine.
  3. Wait for a bigger machine. Cost: blocks the release on hardware the project does not have.
- Recommendation: refuse option 1, accept option 2. The decision 0050 rule stays as written. The release notes state 250 or whatever the release run meets, never more.

### C2. 1,000 players in one town, and 500 in view

- Source: decisions/0050-load-tests.md:101-103, :125; strategy.md section 3.7 and phase 10 item 5.
- State: open, reported with a profile and not fixed. At 1,000 in one town, the zone thread's CPU time p99 is 14.7 ms. 30% of the samples are in `viewEvents`. Crowd breaks the budgets from 250 players in view and drops connections at 1,000. Section 3.7's target of 500 in view is not met.
- The brief says this stays in `distributed-mode`, whose goal already names "the send cost of 1,000 players in one town".
- Recommendation: defer to later, in distributed-mode. The v0.2.0 notes list it as a known issue next to the capacity line, as v0.1.0 does ("Crowd breaks the budgets from 250 players in view"). No new work.

### C3. Bot placement without a database URL

- Source: decisions/0050-load-tests.md:132.
- State: open. `wabots` places bots through the database for `fields`, `graph` and `Travel` (tools/wabots/main.go:176-180). It reads `--database` or `WATHENA_DATABASE_URL`, and the game role's rights are enough.
- The tests use a throwaway world in a database of its own (deploy/README.md "Measure your own hardware", step 1: the owner makes it with `createdb`). The owner has the URL already.
- Options:
  1. Keep.
  2. A development call on the debug address (`--dev` only) that places a bot. Cost: new server surface on the debug address, a new write path for levels and items outside the game rules, and a second code path to keep equal to `placer`.
  3. Place through GM commands of a staff bot. Cost: no command sets a save point, stats or `save_version`.
- Recommendation: refuse options 2 and 3. Keep option 1. The load test needs a test world, and the owner of one holds its database URL.

### C4. A run on a dedicated machine

- Source: decisions/0050-load-tests.md:123; strategy.md phase 8.
- State: open. The project owns one machine, shared, with 13 GB of disk free. The load test needs the machine alone for step 2 of the guide.
- Options:
  1. Run the release's load test on the same machine, stop other work during it, and say so.
  2. Hire a machine for the run. Cost: money; the project runs no servers of its own (strategy section 2.2).
- Recommendation: defer a dedicated run to distributed-mode, whose goal is the 7,500-bot test. v0.2.0 takes option 1. The maintainer decides if a rented hour is acceptable (question 4).

## D. Security review (step 37)

Item states come from docs/security/review-2026-10.md and the code. The brief names seven items.

### D1. Passkey reauth without user verification

- Source: review-2026-10.md:485 (accepted risk); proposal 0002 step 37 review items.
- State: open. A passkey reauth without the user-verification flag is refused only when TOTP is on (server/account/passkeys.go:545-549). On an account without TOTP, it refreshes the 10-minute window, as a password does. A login with such a passkey never counts as a staff factor (`passkey_verified`, migration 00041).
- What the window gates: `items.create`, sanctions, restores and bans (staff.NeedsRecentLogin, server/staff/staff.go:382-387) and account deletion. A stolen session cookie cannot reauth either way; the attacker needs the authenticator. The gap is an authenticator held without its PIN or fingerprint.
- Options:
  1. Keep the accepted risk.
  2. Refuse any reauth with a passkey that did not verify the user. The password (and TOTP when on) stays as the other way. Cost: one condition removed in `finishPasskeyLogin`; owners of keys that never verify (some old security keys) reauth with the password.
- Recommendation: accept option 2. Change `if !verified && on` to `if !verified` for the reauth ceremony, with the existing "verify" message reworded to "Use your password instead" when TOTP is off. Update `TestPasskeys` and remove the accepted risk from review-2026-10.md. Node: security-fixes (server/account).

### D2. When to remove `staff.owners` by name

- Source: review-2026-10.md:448-455; proposal 0002 step 37 ("not yet removed"); deploy/README.md:68.
- State: open. The owner by ID is built (migration 00040; `wactl world new` records it; the role follows the ID). `staff.owners` still works and logs a warning at start (server/cmd/waserver/main.go:524-525).
- Facts:
  - v0.1.0 was tagged after migration 00040, so every world made with `wactl world new` since 2026-10-19 has an owner ID. No binary was published. Only the maintainer's older dev worlds lack one.
  - `staff.owners` can name several owners. The world row holds one ID. Removing the list removes the second owner.
  - Deleting a listed account is refused (server/account/deletion.go:120). The window left is a listed name with no account at start. The server warns (server/admin/admin.go:187-199) and starts.
  - No command sets the owner ID on a world made before 00040.
- Options:
  1. Remove the list in v0.2.0. Cost: loses a second owner and breaks old dev worlds; needs a `wactl world set-owner` and a replacement for more owners (a schema change, so a proposal).
  2. Keep the list, and make the one open window impossible: `waserver` refuses to start when a name has no account (a warning with `--dev`). Cost: small; an owner lists a name only after the account exists, which the guide already says.
  3. Keep as is.
- Recommendation: refuse removal for v0.2.0 and accept option 2. Revisit removal with v0.3.0 when a second-owner mechanism exists (not planned). Change: `admin.Handler.MissingOwners` returns the names; `waserver` returns an error for a non-empty list unless `--dev`. Update deploy/README.md, server/README.md and waserver.yml's comment. Test: start with a missing name fails, with `--dev` it warns. Node: security-fixes (needs server/cmd/waserver and server/admin in its owned paths).

### D3. Account role start values

- Source: review-2026-10.md:505-508 (known issue); docs/releases/v0.1.0.md:95-97.
- State: open, known. `wathena_account` has `INSERT` on `characters` and `inventory` (migration 00002:89-90), so a compromised account service can make characters with any zeny, level or items and no item log line. The start values come from the server config (`NewCharacter`, server/account/service.go:101), which the database cannot read.
- Fix needs: the start config stored in the database (the world row), a SECURITY DEFINER `create_character(account, name, look)` that sets the start values and logs the start items, and `INSERT` taken away from the account role. That is a schema change and a change to the account service's write path: it needs a design proposal first (GOAL.md constraints).
- Options:
  1. Do it in v0.2.0. Cost: a proposal (0007), a migration, tests, and every node that adds character columns or starting items (pets, homunculus, marriage) must follow it. Risk to the release.
  2. Defer; keep it in the release notes. The account role is the web service; its compromise already allows account takeover and mail abuse.
  3. A cheaper mitigation in v0.2.0: a trigger that rejects inserts by `wathena_account` with a level above 1 or zeny above the world's start value. Cost: the start items stay unchecked; hardcoded values drift from the config.
- Recommendation: defer (option 2). New node `character-start-function`, milestone later, depends on release-v0.2.0, first step a proposal 0007. security-review-v0.2 checks that no v0.2.0 migration widens the account role's rights (see "Changes to briefs").

### D4. Secret file modes

- Source: review-2026-10.md:630-635 (known issue); docs/releases/v0.1.0.md:96.
- State: partly fixed. `secrets.sh` refuses to run unless `secrets/` is a folder of the user with mode 0700, and then writes the files 644 (deploy/compose/secrets.sh:15-24, :75). Compose without Swarm ignores a secret's owner and mode, so the containers' users (65532, `postgres`) can read the files only if the files are world-readable.
- Options:
  1. Keep the 0700 folder and the 644 files. Cost: none. A copy that loses the folder's mode exposes the files; `secrets.sh` catches that the next time it runs, and the guide tells owners to keep the mode.
  2. A root init service copies each secret into a tmpfs or volume with the owner and mode of its service, as `caddy-init` does for certificates. Cost: medium; a new root-run service, a second copy of every secret on disk, and a rewrite of every `secrets:` entry. It protects only against another user of the host who can already enter a folder they should not.
  3. Mode 640 with a shared group id. Cost: the PostgreSQL entrypoint drops extra groups, so it fails (review 2026-10).
- Recommendation: refuse. The 0700 folder is the control and it is checked. Change the review's status to "accepted risk" and add one line to the deploy guide's "Secrets" that says `secrets.sh` rechecks the folder on every run, so owners run it after copying the stack. Node: security-fixes (status and the line in deploy/README.md).

### D5. `release.sh` tool checks

- Source: review-2026-10.md:639-644 (known issue); docs/releases/v0.1.0.md:97.
- State: open. The script checks only that `go`, `zig` and `pnpm` are on `PATH` (scripts/release.sh:64-69). Versions are pinned elsewhere: Go `1.27.2` in go.mod:3, zig 0.16.0 (Dockerfile and scripts/check.sh:81), Node 26 and pnpm 12.7.0 (client/package.json:35 `packageManager`; Dockerfile checks pnpm's SHA-256). The client install already uses `--frozen-lockfile`, whose lockfile has integrity hashes, so a registry cannot change a package.
- What a check can add: a wrong zig or Go changes the binary's bytes silently, which breaks the reproducible build the script's header promises.
- Options:
  1. Keep.
  2. Check versions before building: `go env GOVERSION` against go.mod's `go` line, `zig version` equal to 0.16.0, `node --version` major 26, `pnpm --version` equal to `packageManager`. Run `go mod verify`. Exit 1 with the wanted and found versions. A `--check-tools` flag runs only these checks, so `scripts/check.sh all` can run it.
  3. Also hash the tools' binaries. Cost: owners install tools in many ways; the hashes would fail legitimate setups.
- Recommendation: accept option 2. The header comment loses "Nothing is signed" only if signing arrives (it does not). Test: `--check-tools` passes on the maintainer's machine, and a test with a fake `zig` that prints another version on `PATH` fails with the message (a shell check in `scripts/check.sh all`). Node: security-fixes (scripts/release.sh).

### D6. Compose database TLS

- Source: review-2026-10.md:646-649 (known issue); deploy/README.md:147.
- State: open, known. Services connect with `sslmode=disable` (deploy/compose/secrets.sh:49). The `db` network is `internal: true`, so only the stack's containers are on it. `init-db.sh` sends role passwords in `ALTER ROLE` statements, with statement logging turned off first (deploy/README.md:122).
- Options:
  1. Self-signed certificate made by `secrets.sh`, `ssl=on` in PostgreSQL, `sslmode=verify-full` in the URLs. Cost: PostgreSQL refuses a private key that other users can read, so the key file needs the init-copy of D4 option 2, plus certificate rotation and a CA file in each client container.
  2. Keep for the single-host stack; owners with a database on another host set `sslmode=require` themselves (server/README.md:173 already says `disable` is for the same machine).
  3. Do it with the distributed mode, where database traffic crosses hosts and TLS stops being optional.
- Recommendation: defer to later, in distributed-mode (add the item to its brief). The v0.2.0 notes keep the known issue.

### D7. Unsigned backups

- Source: review-2026-10.md:651-654 (known issue); docs/releases/v0.1.0.md:100; the signed-releases brief ("signed backups if the ops review asks").
- State: open, known. age encrypts to a public recipient, so anyone who can write the backups folder or a copy can encrypt a forged backup with a matching manifest.
- Options:
  1. Sign the manifest with an Ed25519 key. Where the private key lives decides what it protects: on the server, a forged file in a copied folder is caught, but a compromised server signs anything; with the identity (offline), the owner signs after `verify-backup`, which adds a manual step.
  2. Keep the documented rule: restore only copies you made yourself, kept where others cannot write.
- Recommendation: defer to the modding milestone, in signed-releases (which signs mod packages and so has the key handling). Add a step to its brief: "signed backup manifests, with the signing key choice as a question". The v0.2.0 notes keep the known issue.

## E. Decision 0054: restores, logins and Lua time limits

### E1. The admin API reports an error for a change that happened

- Source: decisions/0054-restores-logins-and-lua-time-limits.md:112.
- State: open. `Zone.call` waits for a call it started (server/zone/worldcalls.go:219), and `adminworld.Restore` reads its answer with a fresh context (server/adminworld/restore.go:279). The other admin calls apply the change first and then write the audit row or the sanction row with the request's context, which ends after 10 seconds (`requestTimeout`, server/admin/admin.go:62). Found cases:
  - `announce`: the announcement is sent, then `writeAudit(ctx)` (server/admin/tools.go:78 and :90).
  - `planRestart` and `cancelRestart`: the plan is made, then `writeAudit(ctx)` (tools.go:119 and :124, :147 and :152).
  - Event start and stop: `Scheduler.Do`, then `Store.WriteAudit(ctx)` (server/adminworld/tools.go:431 and :445).
  - Mute and jail of an online character: the zone applies it, then `AddCharacterSanction(ctx)` writes the row and the audit (server/adminworld/moderation.go:232 and :241).
  - Kick: the player is disconnected, then `AddCharacterSanction(ctx)` (moderation.go:300 and :310).
  - Mod switch is safe: the store write comes first and a zone error is only logged (tools.go:675 and :698). `ban` writes first and kicks after.
- Effect: on a slow database the API answers `unavailable`, the change stands, and the audit row is missing (an unaudited staff action). The staff member retries, and an announcement is sent twice, a restart plan is replaced, a mute is extended.
- Options:
  1. Keep. Cost: a missing audit row and a duplicate on a slow database.
  2. Write the row first, then apply the change. Cost: a row for a change that did not happen when the zone refuses; the audit log's meaning changes.
  3. After the zone applied the change, write the audit and sanction rows with `context.WithoutCancel(ctx)` and a fresh timeout, as `Restore` does, and answer with the result. If the write fails, answer `unavailable` with the message "The change was made, but its audit row could not be written" and log the full error.
- Recommendation: accept option 3. Apply it to the five cases above. Test with a fake store that cancels the request's context after the zone call: the row is still written and the answer is success. Node: security-fixes, with `server/admin/**` and `server/adminworld/**` added to its owned paths.

### E2. Lua time limits

- Source: decisions/0054-restores-logins-and-lua-time-limits.md:111 (long string compares hold a zone up to 30 s); proposal 0002 step 39.
- State: settled by review decision 60 (review-npc-api H1 and H2): byte costs for string work go to `lua-cost-accounting` in the modding milestone, and the thread CPU limit is refused. Not reopened here.

## F. Proposal 0002 step 19

### F1. Permission names that do nothing yet

- Source: proposal 0002 step 19 (line 1321); decisions/0028-staff-roles-and-audit.md:13-16.
- State: settled. Step 19 listed 18 names that did nothing yet. Every name of the catalog (server/staff/staff.go:50-73, 24 names) is now used by an admin route or a GM command:
  - Admin routes (server/admin/admin.go:128-153): `audit.read`, `audit.review`, `items.restore`, `world.read`, `world.restart`, `events.control`, `mods.read`, `mods.disable`; players, moderation and `players.read_ip` in server/admin/players.go and moderation.go.
  - GM commands (server/zone/gmcommands.go:94-97 and the `@warp`, `@item`, `@mute` family): `commands.warp`, `commands.recall`, `commands.hide`, `world.rename`, `items.create`.
  - `commands.item` was replaced by `items.create`; the catalog has no `commands.item`.
- Leftover: the `Step` field and its comment in staff.go ("Later steps reserve theirs") are history. No change is needed, and the step 19 text in the proposal is a record of the time.
- Recommendation: settled; nothing to build.

## Questions for the maintainer

1. Passkey reauth (D1): refuse a passkey reauth without user verification in every case, so a key without a PIN or fingerprint cannot refresh the window? Recommended: yes. Cost is that such keys reauth with the password.
2. `staff.owners` (D2): keep the list for more than one owner and make `waserver` refuse to start when a listed name has no account (a warning with `--dev`)? Recommended: yes, and do not remove the list in v0.2.0.
3. Secret file modes (D4): accept the risk as it stands (folder mode 0700 checked by `secrets.sh`, files 644)? Recommended: yes. The fix is a root service that copies every secret, which adds more than it protects.
4. A dedicated machine (C4): is a rented hour for the release's load test acceptable, or does the release run on the shared Mac and say so? Recommended: the shared Mac, stated in the notes. Use the rented hour for distributed-mode.
5. Capacity (C1): the release documents the highest count at which every run of every scenario but Crowd meets the rule of decision 0050, found from 250 upward in steps of 250, never above what its own runs show? Recommended: yes.
6. Account role start values (D3): defer to a later node with its own proposal (0007), and list it as a known issue in v0.2.0? Recommended: yes.
7. Compose database TLS (D6) and unsigned backups (D7): defer to distributed-mode and to signed-releases, and list both as known issues in v0.2.0? Recommended: yes.
8. Retention clock (B3): take the cutoffs from the database clock, built in security-fixes? Recommended: yes.
9. Admin audit rows (E1): write the audit and sanction rows after a zone change with a context that outlives the request, built in security-fixes with `server/admin` and `server/adminworld` added to its paths? Recommended: yes.
10. `--at` (A2) and other Prometheus measurements (B2): leave both for later, with the `--at` workaround in the deploy guide? Recommended: yes.
11. Scheduled verify (A1): keep the repeat check of an unchanged backup? Recommended: yes (it finds a damaged file).
12. Bot placement (C3): keep the database URL requirement for `wabots`? Recommended: yes.

## Items security-fixes builds

The exact list, in the order to build (each with its test):

1. D1: a passkey reauth always needs user verification (server/account/passkeys.go). Test: `TestPasskeys` reauth without UV on an account without TOTP is refused.
2. D2: `waserver` refuses to start when a `staff.owners` name has no account, except with `--dev` (server/cmd/waserver/main.go, `admin.MissingOwners`). Docs: deploy/README.md, server/README.md, deploy/compose/waserver.yml. Test: a start with a missing name fails.
3. D5: `scripts/release.sh` checks Go, zig, Node and pnpm versions and runs `go mod verify`, with `--check-tools`; `scripts/check.sh all` runs the flag. Test: a fake `zig` of another version fails the check.
4. E1: the audit and sanction rows after a zone change use a context that outlives the request, for `announce`, `planRestart`, `cancelRestart`, event start and stop, online mute and jail, and kick (server/admin/tools.go, server/adminworld/tools.go, moderation.go). Test: a store whose context cancels after the zone call still gets the row, and the answer is success.
5. B3: the retention job takes its cutoffs from the database clock (server/retention/retention.go). Test: a job with a clock 1 hour ahead and a 30-day period runs.
6. Documents: docs/security/review-2026-10.md statuses for D1, D2 (hardened), D4 (accepted risk), D5, and the deferred D3, D6 and D7 with their nodes; one line in deploy/README.md "Secrets" (D4).

Not built: D3, D4, D6 and D7.

## Changes to briefs and new nodes

These are proposals for the orchestrator. This node edits no other brief.

- **security-fixes.**
  - Goal: replace the list with D1, D2 (hardening), D5, E1 and B3. Drop "secret file modes" (refused) and "Compose database TLS" (deferred). Title can become "Security and operations fixes for v0.2.0".
  - `owns`: add `server/admin/**`, `server/adminworld/**`, `server/retention/**`, `server/cmd/waserver/**`, `docs/decisions/**` for its record (next number 0067). Keep `server/account/**`, `deploy/**`, `scripts/release.sh`, `docs/security/review-2026-10.md`.
  - Acceptance: each item above has a test or a check named in this review.
  - Size stays M.
- **security-review-v0.2.** Add to the Goal: check that no migration of the v0.2.0 nodes grants `wathena_account` more than `SELECT, INSERT` on `characters` and `INSERT` on `inventory` (D3 stays a known issue); list the accepted risks and known issues (D3, D4, D6, D7) in the review's summary for the release notes; check E1's rule in the new admin endpoints (instances, sieges): any state change through a zone call writes its audit row after the change with a context that outlives the request.
- **release-v0.2.0.** Add Steps: (a) the known issues in docs/releases/v0.2.0.md: account role start values, secret file modes (accepted), Compose database TLS, unsigned backups, Crowd and 1,000 players in one town; (b) the capacity rule of C1, run from 250 upward in steps of 250 with three runs near the limit, on the stated machine; (c) the deploy guide gets the `--at` workaround (A2) and the note that repeated verify of an unchanged backup is on purpose (A1).
- **signed-releases** (modding). Add a Step: signed backup manifests (D7), with where the signing key lives as an open question.
- **distributed-mode** (later). Add to the Goal or Context: database TLS between hosts (D6), and a dedicated-machine load run (C4).
- **New node `character-start-function`** (type feature, milestone later, priority later, size M, depends on release-v0.2.0). Goal: a SECURITY DEFINER `create_character` that sets the start values from a start config in the database and logs the start items, and `INSERT` taken from the account role (D3). First step: design proposal 0007.
- **Inbox, no node:** other Prometheus measurements (B2), `--at` (A2).

## Draft review decision 61

61. Operations and security review (review-ops-security, decisions 0048, 0049, 0050 and 0054, the security review of 2026-10, and next steps 19 and 34 to 37). The user approved these on (date):
    - **Kept as built:** `world.read` for `GET /retention`; the scheduled verify checks the newest backup each interval, also an unchanged one; bot placement needs a database URL; step 19's permission names are all in use.
    - **Built in security-fixes:**
      - A passkey reauth needs user verification in every case.
      - `waserver` refuses to start when a `staff.owners` name has no account, except with `--dev`. `staff.owners` stays for more than one owner.
      - `scripts/release.sh` checks the versions of Go, zig, Node and pnpm and runs `go mod verify`, with a `--check-tools` flag that `scripts/check.sh all` runs.
      - After a zone applies an admin change, its audit and sanction rows are written with a context that outlives the request.
      - The retention job takes its cutoffs from the database clock.
    - **Capacity:** the release documents the highest count at which every run of every scenario but Crowd meets decision 0050's rule, found from 250 in steps of 250, on the machine it names. It claims no more.
    - **Deferred:** `--at` and other Prometheus measurements to later; 1,000 players in one town, a dedicated machine for the load test and database TLS to distributed-mode; unsigned backups to signed-releases; the account role's start values to a new node `character-start-function`, with proposal 0007 first.
    - **Accepted risk:** the secret files stay mode 644 in their 0700 folder.
    - **Refused:** removing `staff.owners`; documenting 500 players from the v0.1.0 runs; placing bots without a database URL.
    - No Lua API or protocol change.
