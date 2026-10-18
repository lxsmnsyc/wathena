# 0049: Retention Jobs and the Compose Config

- Status: proposed.
- Date: 2026-10-18.
- Code: [server/retention](../../server/retention) (`retention.go`, `retention_test.go`), [server/store](../../server/store) (`retention.go`, `retention_test.go`, `chat.go`, `migrations/00036_retention.sql`, `queries/retention.sql`, `queries/accounts.sql`), [server/cmd/waserver](../../server/cmd/waserver) (`config.go`, `fields.go`, `main.go`, `retention.go`, `retention_test.go`), [server/account](../../server/account) (`privacy.go`), [server/admin](../../server/admin) (`retention.go`, `retention_test.go`, `admin.go`), [schema/proto/wathena/admin/v1/admin.proto](../../schema/proto/wathena/admin/v1/admin.proto), [client/src/admin](../../client/src/admin) (`api.ts`, `world.ts`), [deploy/compose/waserver.yml](../../deploy/compose/waserver.yml), [scripts/compose-smoke.sh](../../scripts/compose-smoke.sh).
- Phase 8 step 3 (strategy section 15, "Phase 8"), strategy section 7.10, and review decision 49 of [proposal 0002](../proposals/0002-lua-api.md).
- Lua API: none. Protocol: none. Admin API: `GET /admin/api/v1/retention`. Settings: the `retention` and `classic` blocks of `waserver.yml`. Migration: 00036.

## Context

Section 7.10 sets how long the server keeps data: login IP addresses 90 days, chat logs 30 days, item and zeny logs 180 days, and GM audit logs 2 years. Before this step:

- The account service deleted login records older than 90 days in its 10-minute cleanup. The period was fixed.
- The chat log deleted old lines every 12 hours, in each process, with `chat.log_days`.
- Nothing deleted item and zeny log rows or audit rows. The privacy page said the jobs come in a later version.
- `deploy/compose/waserver.yml` had `maps: all` at the top level. A procedural world refuses `maps: all`, so the default Compose config did not run the default world mode, and the smoke test wrote its own config.

Review decision 49 set the design: one config block, one daily job, a `SECURITY DEFINER` function for the log partitions, audit rows that wait for review kept, the privacy page and an admin call, and a `classic:` block for the maps.

## Decision

### Config

```yaml
retention:
  login_ip_days: 90
  chat_days: 30
  item_log_days: 180
  audit_days: 730
```

- Each value is 1 to 3650 days. A key left out takes the default of section 7.10. An explicit 0 is an error.
- A value above its default is allowed. It logs a warning at start, and the admin World screen shows the warning.
- `chat.log_days` still works. It logs a deprecation warning at start and sets the chat period. With `retention.chat_days` set to another value, the config is an error. With the same value, it is allowed, with the warning.
- `chat.log_days` had a default of 30. It now has none, so the server can tell whether the file set it.

### The job

`server/retention` runs one job in `waserver`, at start and every 24 hours. Each run:

1. Takes the advisory lock `pg_try_advisory_lock((1 << 61) | 1)` on a connection of the game pool, held for the run. When another session holds it, the run logs "the retention job runs in another process" and does nothing.
2. Deletes login records older than `login_ip_days`, as the account role, 10,000 rows per statement.
3. Deletes chat lines older than `chat_days`, as the game role, 10,000 rows per statement, except the lines that an open report holds. This is the query the chat log used before.
4. Runs `drop_old_log_partitions` with the cutoff of `item_log_days`, as the game role.
5. Runs `delete_old_staff_audit` with the cutoff of `audit_days`, as the account role.
6. Lets the lock go. A connection whose unlock fails is closed, which ends the session and its lock.

Each step runs even when one before failed. The run logs the rows it deleted by kind (`login_ips`, `chat`, `item_log`, `zeny_log`, `audit`), the month partitions it dropped, and the time it took. With errors, it logs them at warning level, and the job logs the run at error level. A cutoff is the run's start less the period.

The lock follows `LockAccountStorages`, the one advisory lock the store had: bit 62 marks account storages, and bit 61 now marks the server's jobs. No other daily job had a guard against two processes. A session lock was chosen over a transaction lock, so the deletes can run in batches and in their own transactions.

With `--dev` there is no account pool, so steps 2 and 5 are left out. Without a database there is no job.

The account service's cleanup no longer deletes login records, and the chat log no longer deletes lines. `ChatLogOptions.Retention` is gone.

### Item and zeny logs

`drop_old_log_partitions(cutoff)`:

- Refuses a cutoff later than a day ago.
- For each of `item_log` and `zeny_log`, finds the partitions named `<log>_YYYY_MM`, the names that `ensure_log_partitions` makes. Other partitions are left.
- Drops each partition whose month ends at or before the cutoff, in UTC. It counts the rows first, for the log.
- Deletes the rows of `<log>_default` from before the cutoff.
- Returns one row per log: the partitions dropped and the rows that went.
- Has `lock_timeout = 10s`. A drop waits for the commits that write to the log. When the wait is longer, the run fails for the logs, and the next run tries again.

So with 180 days a line is kept at least 180 days and at most about 211 days. The privacy page says lines may be kept up to 31 days longer.

### Staff audit log

`delete_old_staff_audit(cutoff)`:

- Refuses a cutoff later than a day ago.
- Deletes the rows of `staff_audit` from before the cutoff, except rows with `needs_review` and no row in `staff_audit_reviews`.
- Reviews go with their rows: the foreign key cascades.
- Returns how many rows went.

"Needs review" is the column `needs_review`, set for actions such as restores that made items or zeny. A review is a row of `staff_audit_reviews`, with the verdict ok or flag. A flagged review counts as a review, so its row goes when it is old.

### Database rights

- Both functions are `SECURITY DEFINER` with `search_path = pg_catalog, public`, owned by the migrator, as `ensure_log_partitions` and `delete_account` are.
- `REVOKE ALL ... FROM PUBLIC` on both. `wathena_game` gets `EXECUTE` on `drop_old_log_partitions`, and `wathena_account` on `delete_old_staff_audit`, when the roles exist.
- No role gets new table rights. The game role still cannot drop tables or delete log rows, and the account role still cannot delete audit rows. The tests check both.
- The game role can still drop log data older than a day through the function. Before, it could not delete log rows at all. A compromised game process could already write false log rows, so this was accepted.

### Metrics

With `debug_addr`, `GET /debug/wathena/metrics/prometheus` serves, in the Prometheus text format:

- `wathena_retention_deleted_rows_total{kind}`: a counter of the rows deleted since the process started, by kind.
- `wathena_retention_last_success_timestamp_seconds`: a gauge of the start of the last run without errors, as Unix seconds, or 0 before one.

The names start with `wathena_`, as the debug paths `/debug/wathena/...` do. The server had no Prometheus output before, and has no client library. The other measurements stay JSON at `/debug/wathena/metrics`.

### Admin API

`GET /admin/api/v1/retention` needs `world.read`, with the same session, role, two-factor and rate limit checks as every call. It is read-only and writes no audit row. It answers `RetentionStatus`:

- `kinds`: for each kind, `days`, `defaultDays`, `lastDeleted` (the last run) and `deletedSinceStart`.
- `lastRun`, `lastSuccess`, `nextRun`, `lastError`, `skipped` (another process held the lock), `partitionsDropped`.
- `warnings`: one line per period above its default.

`world.read` was chosen because the World screen is the status page and needs it. `game_master` and `admin` have it by default. `audit.read` was the other choice, but the call shows more than the audit log, and holds no personal data.

The status is per process and starts empty. A process whose runs are skipped shows `skipped`, and its counts stay 0.

The World screen of the web client shows a Retention section below the players: the warnings, the last run and the next run, and a table of each kind with its period, its default, and the rows deleted. It loads once when the screen opens.

### Privacy page

- The rows show the periods in effect: login records, chat lines, item and zeny logs, and the staff audit log. 730 days shows as "2 years".
- "comes in a later version" is gone.
- Item and zeny logs: "Old lines are deleted a month at a time, so a line may be kept up to 31 days longer."
- Staff audit log: "A row that waits for a staff review is kept until it is reviewed."
- A new row, Backups, "Up to 4 weeks": "Backups are encrypted copies of the data above. With the default backup schedule, data deleted from the server stays in backups for up to 4 weeks."
- `account.Privacy` has `LoginIPDays`, `ChatLogDays`, `ItemLogDays` and `AuditDays`, each the default when 0.

### The classic block

```yaml
classic:
  start_map: prontera
  maps: all
```

- A classic world takes its start map and maps from the `classic` block. A procedural world ignores the block.
- `--map` and `--maps` win over the block, in both modes, as before.
- `start_map` and `maps` at the top level still work, each with a deprecation warning at start. A procedural world reads them as before. Set in both places with different values, they are an error.
- `deploy/compose/waserver.yml` moves its maps into the block, and gains a commented `retention` block. So the default file runs a procedural world and a classic one.
- The smoke test now runs the default `waserver.yml`, and sets `WATHENA_CONFIG=./waserver.yml` so a value in the shell does not change it.

## Tests and checks

- `server/store` with PostgreSQL:
  - `TestDropOldLogs`: partitions for March and April 2026, rows in both, and rows of 2025 and 2030 in the default partitions. A cutoff one second before the end of March drops nothing and deletes the 2025 rows. A cutoff at the end of March drops March and keeps April. A cutoff one second before the end of April keeps April. The game role cannot drop a partition, delete log rows or drop the default partition. A cutoff an hour ago is refused, and the account role cannot run the function.
  - `TestDeleteOldAudit`: old rows without review, old reviewed rows, an old row that waits for review, and new rows. The first two go with their reviews, the waiting row stays until a review is added. The account role cannot delete audit rows itself, the game role cannot run the function, and a cutoff of now is refused.
  - `TestDeleteOldLogins` and `TestRetentionLock`: a second lock fails while the first holds, and works after the unlock.
  - `TestChatLogAndReports`: the lines of an open report stay, and go once it closes.
- `server/retention` with PostgreSQL, `TestJob`: a run while another session holds the lock skips and deletes nothing. A run then deletes each kind: an old login record, two old chat lines, a default-partition zeny row and an old audit row, and keeps the line an open report holds. The status, the totals across two runs and the Prometheus lines. A run without the account pool leaves the audit log.
- `server/retention`, `TestWarnings`.
- `server/cmd/waserver`:
  - `TestRetentionConfig`: the defaults, set values, the warning above a default, values out of range, an unknown key, `chat.log_days` alone, with the same and with another `retention.chat_days`, out of range, and above the default.
  - `TestClassicConfig`: the block, the flags over it, the top-level keys with their warnings, and both places with different and the same values.
  - `TestComposeConfig`: `deploy/compose/waserver.yml` loads without warnings, gives a procedural world no maps, and gives a classic world `prontera` and `all`.
- `server/admin` with PostgreSQL, `TestRetentionCall`: a game master reads it, a moderator and a player are refused, no session answers `login`, an owner without a two-factor login answers `two_factor`, a POST is not found, and without a job it answers `unavailable`.
- `server/account`, `TestPrivacyPage`: the configured periods, the defaults, "2 years", the backups row, the 31 days note and the review note, and no "later version".
- `client/src/admin/world.test.ts`: the rows and the run lines.

## Lua API

None.

## Left

- The status is in memory per process. A second process shows no counts, and a restart forgets them. A table of runs would fix both.
- The IP search, the account page's recent logins and the data export still look back 90 days at most. With `login_ip_days` above 90, the export leaves out the older records it keeps.
- The other measurements in the Prometheus format.
- The smoke test was not run for this step, since the disk was too full to build the images (2026-10-18).

## Review items

- Whether `world.read` is the right permission for `GET /retention`.
- Whether the data export and the IP search should follow `login_ip_days`.
- Whether to keep a table of runs, so every process and a restart show the last run.
