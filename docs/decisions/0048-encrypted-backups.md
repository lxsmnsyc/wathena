# 0048: Encrypted Backups

- Status: proposed.
- Date: 2026-10-18.
- Code: [tools/wactl](../../tools/wactl) (`backup.go`, `backup_test.go`, `backup_db_test.go`, `world.go`, `main.go`), [server/store](../../server/store) (`backup.go`, `migrate.go`, `queries/backup.sql`, `db/backup.sql.go`), [deploy/compose](../../deploy/compose) (`docker-compose.yml`, `secrets.sh`, `init-db.sh`, `.env.example`), [Dockerfile](../../Dockerfile), [scripts/compose-smoke.sh](../../scripts/compose-smoke.sh), `go.mod`.
- Phase 8 step 2 (strategy section 15, "Phase 8"), strategy section 7.8, and review decision 48 of [proposal 0002](../proposals/0002-lua-api.md).
- Lua API: none. Protocol: none. Settings: none in `waserver.yml`. New variables in `.env`: see "Compose".

## Context

Section 7.8 asks for encrypted backups with restore tests on a schedule. Before this step:

- `wactl world backup` wrote a plain `pg_dump` file. It holds password hashes, sealed TOTP secrets and the world seed.
- Nothing made backups on a schedule, and nothing checked that a backup restores.

Review decision 48 set the design: age encryption with the identity kept offline, a manifest beside each backup, a long-lived schedule with pruning, and `verify-backup` for restore tests.

## Decision

### Encryption

- Backups are encrypted with age, from `filippo.io/age` v1.3.2 (2026-08-29).
- The server holds only the recipient, the public key. The owner keeps the identity, the private key, offline.
- `pg_dump` writes into age's encrypting writer, which writes into the file. No plain copy touches disk.
- Owners can decrypt with the standard `age` command, since the files are plain age files.
- Recipients are what age's own parser takes: X25519 recipients (`age1...`) and post-quantum hybrid ones (`age1pq1...`). `backup-key new` makes X25519 keys, as `age-keygen` does by default.

### Commands

```text
wactl world backup-key new --out FILE
wactl world backup (--out FILE | --out - | --dir DIR) (--recipient age1... | --recipients-file FILE | --plain)
                   [--every 24h] [--keep-daily 7] [--keep-weekly 4] [--database URL]
wactl world restore --in FILE|- [--identity FILE] [--database URL]
wactl world verify-backup (--in FILE [--manifest FILE] | --dir DIR) [--identity FILE] [--every 24h] [--database URL]
```

`backup-key new`:

- Writes a new X25519 identity in `age-keygen`'s format, with `# created:` and `# public key:` lines, mode 0600.
- Refuses a file that exists.
- Prints the recipient on standard output, and a reminder to keep the identity offline on standard error.

`backup`:

- `--recipient` and `--recipients-file` can repeat. A recipients file has one recipient per line, with `#` for comments.
- Without a recipient the command is refused, unless `--plain` is given. `--plain` with a recipient is refused too.
- `--out FILE` makes the file and a manifest beside it: the name without `.dump.age` or `.dump`, with `.json`.
- `--out -` writes the backup to standard output, without a manifest.
- `--dir DIR` makes `world-<UTC time>.dump.age`, or `.dump` with `--plain`, and `world-<UTC time>.json`. The time is like `20261018T030000Z`, so names sort by time. The folder is made with mode 0700 when it is missing.
- `--every` needs `--dir`, and is at least one minute. Without `--every`, `--keep-daily` and `--keep-weekly` prune only when given.
- It connects as the migrator role, as before, since that role owns every table.

`restore`:

- Reads the first bytes. An age file, binary or armored, needs `--identity`. Other input is taken as a plain backup, so older plain backups still restore.
- An identity of another key is refused with "it was made for another recipient".
- The empty-database rule and the single transaction stay.
- When decryption fails in the middle, such as on a changed byte, wactl kills `pg_restore` before it can commit. Its transaction rolls back and the database stays empty.

`verify-backup` checks, in this order:

1. The manifest's format is 1.
2. The file's size and SHA-256 are the manifest's.
3. The file is encrypted when the manifest says so, and the identity opens it.
4. It restores into a new database `wathena_verify_<random hex>`, with `--single-transaction --no-owner --no-acl`, so the world's roles need not exist where it runs.
5. The restored migration is the manifest's, and not newer than this wactl's newest migration.
6. The counts in the restored copy are the manifest's.

It drops the temporary database at the end, also on failure and on SIGTERM, with its own one-minute limit. Any mismatch exits with status 1 and names each difference. `--dir` checks the newest backup that has a manifest.

### Manifest

```json
{
  "format": 1,
  "file": "world-20261018T030000Z.dump.age",
  "encrypted": true,
  "release": "v0.1.0",
  "migration": 35,
  "time": "2026-10-18T03:00:00Z",
  "size": 183204,
  "sha256": "9f2c...",
  "counts": { "accounts": 2, "characters": 3, "item_serials": 6, "zeny": 600 }
}
```

- `release` is `internal/buildinfo`'s version of wactl.
- `size` and `sha256` are of the file as written, the encrypted bytes for an age backup.
- `counts` come from the sqlc query `WorldCounts`:
  - `accounts`: rows of `accounts`.
  - `characters`: rows of `characters`.
  - `item_serials`: rows of `inventory`, `storage_items` (storages and carts), `guild_storage` and `mail_items`.
  - `zeny`: the zeny of `characters` plus the zeny of `mails`.
- It holds no secrets: no URL, no recipient and no identity.
- `format` names the fields and the count definitions. A change to either needs a new format.

The counts and the dump come from one snapshot:

- wactl opens a repeatable read, read-only transaction, runs `pg_export_snapshot()`, and counts in it.
- `pg_dump --snapshot` dumps the same snapshot, while the transaction stays open.
- So the counts match the dump while players play. Without this, verify would see differences on any live server.
- The migration version is read just before, from goose's table.

### Files that look finished only when they are

- Each file is written to a hidden temporary name in the same folder, `.world-<UTC time>.dump.age.<random>.tmp`, with mode 0600.
- The file is synced, then given its name with a hard link and the temporary name removed. A link never replaces a file. Where hard links do not work, a rename follows a check that the name is free.
- The folder is synced after, where the system supports it.
- On any error, the temporary file is removed and no final file is made.
- The manifest is written the same way, after the backup file. A crash between the two leaves a backup without a manifest, which `verify-backup --dir` skips.
- A crash or SIGKILL during a backup leaves the hidden temporary file. It never matches the backup names, so nothing reads or prunes it. The owner can delete it.

### Schedule

`backup --dir D --every 24h`:

- The first run is due one interval after the newest backup in the folder, or at once when there is none. A restart of the container does not make an extra backup.
- After each backup it prunes the folder, and logs the backup's file, size, time taken, migration and counts, and each file it removed.
- A failed run is logged and tried again after an hour, or after the interval when that is shorter.
- SIGTERM or Ctrl+C stops it with status 0. A backup in progress is stopped, its `pg_dump` killed, and its temporary file removed.
- Logs are `log/slog` text lines on standard error.

`verify-backup --dir D --every 24h` checks at once, then once per interval. When the folder has no backup yet, it looks again after an hour, or the interval when shorter. A failed check is logged at error level, and the loop goes on.

### Pruning

Pruning keeps:

- Daily: for each of the newest `--keep-daily` UTC days that have a backup, the newest backup of that day.
- Weekly: for each of the newest `--keep-weekly` ISO weeks (Monday to Sunday, UTC) that have a backup, the newest backup of that week.

The rules:

- The weekly backup of a week is its newest backup. For a past week with daily backups, that is its Sunday backup. For the current week, it is today's backup, which is also a daily one.
- A backup may count as daily and weekly. Every other backup is removed, with its manifest.
- Days and weeks without a backup do not count, so a stopped server does not lose its old backups to the calendar.
- `--keep-daily` is 1 to 10,000, so the newest backup always stays. `--keep-weekly` is 0 to 10,000.
- With the defaults of 7 and 4 and a daily schedule, the folder holds 10 backups: the last 7 days, and the Sunday backups of the 3 weeks before.
- Only regular files that match `world-<UTC time>.dump.age`, `.dump` or `.json` are touched. Other files and folders stay, the hidden temporary files too.
- A manifest whose backup file is gone is removed.

This is the rule of `restic forget --keep-daily --keep-weekly`, which many owners know.

### Database rights of verify-backup

`verify-backup` uses its own role, not the migrator:

- It needs `CREATEDB` to make and drop its temporary database, and nothing else.
- The migrator role does not get `CREATEDB`. The backup service keeps the migrator's rights, and the verify service gets none of them.
- In Compose the role is `wathena_verify`: `LOGIN CREATEDB NOSUPERUSER NOCREATEROLE NOREPLICATION NOBYPASSRLS`. `db-init` makes it when it is missing and sets its password on every run, so stacks made before this step get it too.
- It connects to the `postgres` database. It cannot connect to `wathena`, which revokes `CONNECT` from `PUBLIC` and grants it to the game and account roles only.
- It owns what it restores, since the restore has `--no-owner --no-acl`, and drops it at the end.
- Its URL comes from `--database`, or `WATHENA_VERIFY_DATABASE_URL`, or `WATHENA_VERIFY_DATABASE_URL_FILE`.
- On the owner's own machine, any role with `CREATEDB` works, such as the local superuser.

A role with `CREATEDB` can fill the disk with databases. On the server it runs only with the opt-in verify profile, and the database network has no route out.

### Compose

- `backup` (always on): the tools image with `world backup --dir=/backups --recipients-file=/run/secrets/backup_recipients --every --keep-daily --keep-weekly`, the `backups` volume, and the secrets `migrate_database_url` and `backup_recipients`. It starts after `migrate` and restarts unless stopped.
- `backup-verify` (profile `verify`): `world verify-backup --dir=/backups --identity=/run/secrets/backup_identity --every`, the `backups` volume read-only, and the secrets `verify_database_url` and `backup_identity`. Its environment has only the verify URL.
- `.env` variables: `WATHENA_BACKUP_EVERY` (24h), `WATHENA_BACKUP_KEEP_DAILY` (7), `WATHENA_BACKUP_KEEP_WEEKLY` (4), `WATHENA_VERIFY_EVERY` (24h), and `WATHENA_BACKUP_RECIPIENTS` and `WATHENA_BACKUP_IDENTITY`, which name other files for the two secrets.
- The tools image has `/backups` owned by user 65532 with mode 0700, so the new volume is too.
- `secrets.sh` makes `verify_password` and `verify_database_url` in its style. It never makes the backup key, since the identity must not be made on the server. It makes `backup_recipients` with comments that say how to fill it, and prints a note while it has no recipient.
- Until the owner adds a recipient, the backup service exits with an error and Docker restarts it, so `docker compose up --wait` fails and the logs say why. The setup guide makes the key in the first start, before `up`.
- `secrets/backup_identity` is needed only for the verify profile, and the owner copies it there on purpose.

The smoke test makes a key in a throwaway container, gives the stack its recipient and identity through the two variables, waits for the backup service's first backup, checks that it is an age file with a manifest, verifies it with `backup-verify`, restores it with the identity into an empty database, and starts the stack again.

## Tests and checks

- `tools/wactl` unit tests:
  - `TestBackupEncryptionRoundTrip`: no plain bytes in the file, mode 0600, the manifest's size and hash, two recipients, armored files, and plain backups with and without an identity.
  - `TestBackupNeedsRecipient`: no recipient, `--plain`, `--plain` with recipients, bad recipients, a file with comments only, an identity given as a recipient, and the command's refusal.
  - `TestBackupWrongIdentity`, `TestBackupManifestMismatch`, `TestBackupTampered` (a byte changed in the body and at the end, the end cut, the last chunk cut).
  - `TestKeepRule`: a table over dates, with daily runs across weeks, a new week, several backups a day, gaps, and an ISO week across the new year.
  - `TestPruneDir`: only wactl's names are removed. `TestWriteAtomic`: only the hidden temporary file exists during a write, nothing stays after a failure, and an existing file is never replaced.
  - `TestBackupNames`, `TestBackupKeyNew`, `TestRestoredProblems`, and `TestBackupLoop`: a run, pruning, logs, and a clean stop.
- `tools/wactl` with PostgreSQL:
  - `TestBackupVerifyRestoreWithDatabase`: an encrypted backup of a world with accounts, characters, items and zeny, its manifest, `verify-backup --dir` and no temporary database left, a refused verify without the identity, refused restores with another identity, without an identity, and of a changed file, each leaving the database empty, a restore with the same counts, the empty-database rule, and a plain backup that verifies and restores.
  - `TestVerifyBackupMismatchWithDatabase`: one character more, less zeny, another migration and another hash in the manifest each fail, with no temporary database left.
- The tests with PostgreSQL need `pg_dump`, `pg_restore`, `createdb` and `dropdb` on the PATH, and skip without them.
- `scripts/compose-smoke.sh`: as in "Compose". It now makes the world with `--mode classic`:
  - Since decision 0047 the tools image's default mode is procedural, and `wactl world new` there failed with "reference: go.mod not found above the working directory", since the image has no reference data. The placeholder pack has no game maps, so a classic world fits the test.
  - On this machine (2026-10-18), Colima's port forwarding is broken, so the script's HTTPS checks from the host could not connect. A copy of the script without the HTTPS checks and the build passed: the backup service made an encrypted backup with a manifest, `backup-verify` verified it, and it restored with the identity.

## Lua API

None.

## Left

- Copying backups to other machines. Owners use `rclone` or `rsync` (out of scope in review decision 48).
- An admin screen for backups, and point-in-time recovery (out of scope).
- Removing hidden temporary files that a crash left.
- A fixed time of day for the schedule. The first backup runs at start, and the next ones one interval after the newest.
- A procedural world from `wactl world new` in the tools image, which needs the reference data in the image or in the binary.

## Review items

- Whether the backup service should wait without a recipient instead of failing, so `up --wait` passes on a stack without backups.
- Whether `db-init` should make `wathena_verify` only when the verify profile is used.
- Whether the scheduled verify should skip a backup it already checked.
- A `--at` option for the time of day of the schedule.
