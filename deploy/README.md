# deploy

The all-in-one deployment of strategy section 3.9: one `waserver` process with PostgreSQL 18 next to it, and Caddy in front for TLS.

| Path | Content |
|---|---|
| `compose/` | The Docker Compose stack: `docker-compose.yml`, the `Caddyfile`, waserver's `waserver.yml`, `.env.example`, `secrets.sh` and the database setup script `init-db.sh`. |
| `docker/waserver.yml` | The default config inside the server image. |
| `postgres/roles.sql` | Makes the `wathena` database and its three roles. |
| `../Dockerfile` | Builds the server image and the tools image. |

## Docker Compose

The stack:

| Service | Image | Does |
|---|---|---|
| `postgres` | `postgres:18` | The database, on a network with no route out. |
| `db-init` | `postgres:18` | Runs on each `up`. Makes the database and roles from `roles.sql` on the first run, and sets the role passwords from the secrets on every run. |
| `migrate` | `wathena/wactl` | Runs on each `up`, after `db-init`. Applies pending migrations as `wathena_migrator`. |
| `waserver` | `wathena/waserver` | `waserver all`: the game at `/ws`, the account pages at `/account/`, the assets at `/assets/`, and the web client at `/`. Starts after `migrate` succeeds. |
| `caddy` | `caddy:2` | The only service with public ports. Gets certificates and proxies everything to `waserver`. |
| `backup` | `wathena/wactl` | Makes an encrypted backup once a day into the `backups` volume, and removes old ones. Without a recipient, it makes none and logs a warning every hour. See "Backups". |
| `verify-init` | `postgres:18` | Only with the `verify` profile. Makes the `wathena_verify` role and sets its password. |
| `backup-verify` | `wathena/wactl` | Only with the `verify` profile, after `verify-init`. Restores the newest backup into a temporary database once a day and checks it. It needs the backup identity on the server. See "Backups". |
| `wactl` | `wathena/wactl` | Not started by `up`. For `docker compose run`: making the world, building assets, backups and restores. |

The two wAthena images come from the repository's `Dockerfile`:

- `wathena/waserver` is distroless (no shell), with one static binary: `waserver` with the web client and the reference data built in. The binary is about 107 MB, 22 MB compressed (2026-10-18). The build compiles Lua with `zig cc` and links the binary against musl, so it stays static. zig is downloaded in the build and checked against a pinned SHA-256.
- `wathena/wactl` is `wactl` on the `postgres:18` image, for `pg_dump` and `pg_restore`. 170 MB compressed, mostly the PostgreSQL image, which the stack pulls anyway.
- Both binaries are built with the tag `procedural`, as `scripts/release.sh` builds them (docs/decisions/0047-guild-towns-and-release-build.md). They run procedural and classic worlds, and `wactl world new --mode` picks the mode. Both have the reference data built in, so they need no copy of the repository. `WATHENA_VERSION` in `.env` is the version they print with `version`; set it to the output of `git describe --tags --always --dirty` for a release.

You need Docker Engine with Compose 2.20 or later. The load tests of phase 8 measured 250 players on 4 CPU cores and 8 GB of memory (strategy section 3.9), with 50 GB of SSD suggested. For more players, measure your own hardware first (see "Measure your own hardware"). Building assets from a full game client takes 4 GB of memory.

### First start

Run these in `deploy/compose`.

1. Settings. Copy `.env.example` to `.env` and set `WATHENA_DOMAIN` to the server's public host name. Its DNS record must point at the machine, and ports 80 and 443 must be open, so Caddy can get a certificate. For a test on your own machine, see "Local mode".
2. Secrets. Run `./secrets.sh`. It writes random passwords, the database URLs and the TOTP key into `secrets/`, which Git ignores. Running it again keeps files that exist.
3. Images. Build both: `docker compose --profile tools build`.
4. Assets. Build them from your own game client into the `assets` volume. The repository ships no game files.

   ```text
   docker compose run --rm --no-deps -v /path/to/client:/client:ro \
     wactl assets build --source grf:/client --out /srv/assets
   ```

   - The client folder needs its `DATA.INI` and the GRF files it names. A kRO client takes about 21 minutes on 8 cores and gives 112,000 assets.
   - Run the same command again after a client update. Unchanged assets come from the build's cache.
   - To use a build you already have, set `WATHENA_ASSETS` in `.env` to its absolute path instead. The user 65532 must be able to read it.
   - For a test without game files, use `--source placeholder`. For a classic world, also set `start_map: placeholder/map` in the `classic` block and `mode: none` in `waserver.yml`.
5. World. Make the world and the first account. The password is read from standard input, so it stays out of the shell history and process lists:

   ```text
   read -rs PASSWORD
   printf '%s\n' "$PASSWORD" | docker compose run --rm -T wactl world new \
     --name "My World" --ruleset renewal --admin Admin --admin-password-file -
   ```

   `--ruleset` must match `mode` in `waserver.yml`. The command applies the migrations first, and makes the world's secret seed.

   To give the account the owner role, put its name in `staff.owners` in `waserver.yml`, as the command prints. The role counts once the account turns on TOTP or a passkey and logs in with it. The admin screens are then at `https://WATHENA_DOMAIN/admin` (server/README.md, "Staff and admin tools").
6. Backup key. On your own machine, run `wactl world backup-key new --out backup-identity.txt`, or `age-keygen -o backup-identity.txt`. Keep the identity file offline. Put the recipient it prints, `age1...`, on a line of its own in `secrets/backup_recipients` on the server. See "Backups".
7. Start: `docker compose up -d --wait`. It waits until every service is healthy. Open `https://WATHENA_DOMAIN/`, sign up, make a character and press Play.

`waserver` refuses to start while migrations are pending or before step 5. Its log then says why: `docker compose logs waserver`.

### Settings

- `.env` sets the domain, the host ports, the images, where the assets are, and the edge network. `.env.example` explains each one.
- `waserver.yml` sets the game: the rAthena mode, the captcha, and in its `classic` block the maps and the start map of a classic world. A procedural world runs its generated maps and ignores the `classic` block, so the file works in both world modes. server/README.md, "All-in-one", lists every key.
- `start_map` and `maps` at the top level of `waserver.yml` still work, with a warning at start. Move them into the `classic` block. A procedural world fails to start with `maps: all` at the top level.
- Fill in `privacy.contact` and `privacy.text` in `waserver.yml`. Players read them on `/account/privacy`, with the retention periods in effect. Players can download their data and delete their accounts on `/account/data`. You are responsible for following the laws that apply to you, such as the GDPR.
- The `retention` block of `waserver.yml` sets how many days the server keeps login records with IP addresses (90), chat lines (30), the item and zeny logs (180) and the GM audit log (730). Each is 1 to 3650. `waserver` deletes older data once a day, at start and every 24 hours. A period longer than the default logs a warning and shows one on the admin World screen. Shorter periods need no notice. `chat.log_days` still works, with a warning: move it to `retention.chat_days`. server/README.md, "Retention", has the details.
- Backups keep data that the retention job deleted since, for up to 4 weeks with the default schedule (see "Backups"). The privacy page says so.
- `docker-compose.yml` passes `--public-url` and `--trusted-proxies` from `.env`.
- Change a setting, then run `docker compose up -d --wait`.

### Local mode

For a test on your own machine, set `WATHENA_DOMAIN=localhost`. Caddy then signs a certificate with its own local authority instead of asking Let's Encrypt. If ports 80 and 443 are taken, use others, such as `WATHENA_HTTP_PORT=8080` and `WATHENA_HTTPS_PORT=8443`, and open `https://localhost:8443/`.

Browsers do not trust Caddy's local authority until you add it. Without it, the page shows a certificate warning and the client's Service Worker does not register. Copy it out and add it to your system's or browser's trust store:

```text
docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt caddy-root.crt
```

On a port other than 443, Caddy's redirect from plain HTTP goes to port 443. Open the https URL directly.

### Secrets

Secrets never go into `.env`, the Compose file, the config file, an image, a process's arguments or a log. `secrets.sh` makes them as files, and Compose mounts each file read-only at `/run/secrets/<name>` in the services that need it. The programs read `NAME_FILE`, the file that holds the secret `NAME`.

| File in `secrets/` | Used by | As |
|---|---|---|
| `postgres_password` | `postgres`, `db-init` | The `postgres` superuser's password. |
| `migrator_password`, `game_password`, `account_password` | `db-init` | The roles' passwords. `db-init` reads them inside psql, and turns statement logging off first. |
| `migrate_database_url` | `migrate`, `wactl` | `WATHENA_MIGRATE_DATABASE_URL_FILE` |
| `game_database_url` | `waserver` | `WATHENA_DATABASE_URL_FILE` |
| `account_database_url` | `waserver` | `WATHENA_ACCOUNT_DATABASE_URL_FILE` |
| `account_secret_key` | `waserver` | `WATHENA_ACCOUNT_SECRET_KEY_FILE`. Encrypts TOTP secrets. |
| `verify_password` | `verify-init` | The password of `wathena_verify`, the role of `backup-verify`. Only the `verify` profile uses it. |
| `verify_database_url` | `backup-verify` | `WATHENA_VERIFY_DATABASE_URL_FILE` |
| `backup_recipients` | `backup` | The recipients of the backups, which you add. Not secret, but kept here. `WATHENA_BACKUP_RECIPIENTS` in `.env` can name another file. |
| `backup_identity` | `backup-verify` | Only for the `verify` profile, which you copy here yourself. `secrets.sh` never makes it. `WATHENA_BACKUP_IDENTITY` can name another file. |

- Compose secrets on one machine are bind-mounted files. Anyone with root or Docker access on the host can read them.
- `secrets/` is only open to its owner. The files in it are readable by others, so the containers' users can read their mounted copies.
- Back up `secrets/`, and above all `account_secret_key`, apart from the database backups. Keep the backup identity apart from both.
- To change a password, delete its file, run `./secrets.sh`, then `docker compose up -d --wait --force-recreate`. `db-init` sets the new password.
- To change `account_secret_key`, see "Changing the key".

### Security of the stack

- `waserver` and `wactl` run as user 65532. `waserver`, `wactl`, `db-init` and Caddy have a read-only root filesystem, no Linux capabilities (Caddy keeps `NET_BIND_SERVICE`) and `no-new-privileges`. PostgreSQL has a read-only root filesystem too.
- Only Caddy publishes ports. The database network is internal.
- `waserver` believes `X-Forwarded-For` only from Caddy's fixed address on the edge network, for rate limits.
- Caddy answers `/healthz` and `/readyz` with 404. Docker reaches them inside the network.
- Every base image is pinned by digest.

### Updates and restarts

To update, pull the repository, build the images and start again:

```text
git pull
docker compose --profile tools build
docker compose up -d --wait
```

- `migrate` applies new migrations before `waserver` starts.
- On `docker compose stop`, `restart` or `down`, Docker sends SIGTERM to `waserver`. It stops taking players, tells each player "The server is restarting" (`DISCONNECT_REASON_SERVER_SHUTDOWN`), saves every character, and stops its zones. Compose gives it 30 seconds.
- `shutdown_countdown` in `waserver.yml`, 10 seconds by default and at most 10 minutes, is a countdown players see before a SIGTERM stops `waserver`. Compose's `stop_grace_period` of 30 seconds must stay longer than it plus the last saves.

### Planned restarts

Staff with `world.restart` plan a restart from the admin screens, with a time, a reason and a message (server/README.md, "Staff and admin tools", [decision 0031](../docs/decisions/0031-world-events-and-mods-tools.md)):

- Players get announcements 30, 15, 10, 5 and 1 minute before, and 30 seconds before.
- New logins are refused in the last minute.
- At the time, `waserver` saves every character, disconnects every player with the reason, writes what waits, and exits with status 75 (`EX_TEMPFAIL` of sysexits.h). The log's last line says `waserver: restarting as planned: <reason>`.
- The process manager must start it again:
  - Compose: `waserver` has `restart: unless-stopped`, so Docker starts it again after any exit, unless someone stopped it with `docker compose stop`. The stack needs no change. `docker compose ps` shows it `Up` again within seconds, and `docker compose logs waserver` shows both runs.
  - systemd: use `Restart=on-failure` or `Restart=always`. Both restart on status 75. With `Restart=on-failure`, a stop with status 0 does not restart, as you want for `systemctl stop`.
  - Another supervisor: restart on a non-zero exit status, or on 75 alone.
- The plan lives in memory. A restart before the time, such as a crash, a `docker compose restart` or an update, drops it. Plan it again afterwards.
- A planned restart starts the same container again, with the same image. To run a new image, use `docker compose up -d --wait` as above.

A systemd unit for `waserver` without Docker:

```ini
[Unit]
Description=wAthena server
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
User=wathena
ExecStart=/usr/local/bin/waserver all --config /etc/wathena/waserver.yml
EnvironmentFile=/etc/wathena/secrets.env
Restart=on-failure
RestartSec=2
# Long enough for --shutdown-countdown and the last saves.
TimeoutStopSec=90

[Install]
WantedBy=multi-user.target
```

### Backups

Backups are encrypted with [age](https://age-encryption.org) (docs/decisions/0048-encrypted-backups.md):

- The server holds only the recipient, the public key. You keep the identity, the private key, offline.
- The `backup` service makes a backup once a day into the `backups` volume, and keeps the newest backup of each of the last 7 days and 4 weeks.
- A backup holds password hashes, sealed TOTP secrets and the world seed. Without the identity, the files reveal nothing but their size.
- `wactl world verify-backup` restores a backup into a temporary database and checks it. Run it on your own machine, or on the server with the opt-in `verify` profile.
- Copying backups to another machine is up to you, with `rsync`, `rclone` or similar. A backup on the server alone is lost with the server.

#### The backup key

Make the key on your own machine, not on the server:

```text
wactl world backup-key new --out backup-identity.txt
```

- It prints the recipient, such as `age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p`, and writes the identity with mode 0600. It never overwrites a file.
- The standard `age-keygen -o backup-identity.txt` makes the same kind of key. Its `Public key:` line is the recipient.
- Put the recipient on a line of its own in `secrets/backup_recipients` on the server. More lines encrypt to more keys, such as one per owner. `secrets.sh` makes the file with comments that say this.
- Keep the identity offline, such as in a password manager or on two USB drives. Without it, no backup can be restored. Anyone with it can read every backup.
- Until the file has a recipient, the backup service makes no backups. It keeps running, so `docker compose up --wait` passes, and logs a warning at start and every hour. `docker compose logs backup` shows it.
- The service reads the file only when it starts. After you add a recipient, run `docker compose up -d --wait --force-recreate backup`. A plain restart may keep the old file, since Docker mounts the file and an editor may write a new one.
- To change the key, put the new recipient in the file and run the same command. Keep the old identity while you keep backups made for it.

#### The backup service

`backup` runs `wactl world backup --dir=/backups --every=24h --keep-daily=7 --keep-weekly=4` as the migrator role:

- The first backup runs when the service starts and the folder has none from the last 24 hours. Then one runs every 24 hours.
- Each backup is `world-<UTC time>.dump.age`, such as `world-20261018T030000Z.dump.age`, with a plain manifest `world-20261018T030000Z.json` beside it. The manifest has the release, the migration, the time, the size, the SHA-256 of the encrypted file, and the numbers of accounts, characters and item serials, and the zeny total. It holds no secrets.
- Files are written under a hidden temporary name and renamed when complete, so a file with a backup's name is always complete. A crash can leave a hidden temporary file, such as `.world-20261018T030000Z.dump.age.123456.tmp`. Each scheduled run removes such files older than 24 hours. It removes only names of this pattern.
- After each backup, old backups are removed. The service keeps the newest backup of each of the last 7 UTC days that have one, and the newest backup of each of the last 4 ISO weeks (Monday to Sunday, UTC) that have one. With daily backups, that is the last 7 days and the Sunday backups of the 3 weeks before. Files with other names in the folder stay.
- A failed backup is logged and tried again after an hour.
- `.env` sets `WATHENA_BACKUP_EVERY`, `WATHENA_BACKUP_KEEP_DAILY` and `WATHENA_BACKUP_KEEP_WEEKLY`.

To see the backups and the log, and to copy them out:

```text
docker compose logs backup
docker compose exec backup ls -l /backups
docker compose cp backup:/backups ./backups
```

To make a backup now, into the volume or into a file:

```text
docker compose run --rm backup world backup --dir=/backups --recipients-file=/run/secrets/backup_recipients
docker compose run --rm -T backup world backup --out - --recipients-file=/run/secrets/backup_recipients > world.dump.age
```

`--out -` writes no manifest. Without Docker, `wactl world backup` takes `--recipient age1...`, which can repeat, or `--recipients-file`. It refuses to run without one, unless `--plain` asks for an unencrypted backup.

#### Checking backups on your own machine

This is the default: the identity never goes to the server. Copy a backup and its manifest to a machine with PostgreSQL 18 and its client tools, then:

```text
wactl world verify-backup --in world-20261018T030000Z.dump.age --identity backup-identity.txt \
  --database postgres://me@localhost/postgres
```

- `--database`, or `WATHENA_VERIFY_DATABASE_URL`, names a role with `CREATEDB`, such as your local superuser, and a database it may connect to.
- It checks the file's size and SHA-256 against the manifest, decrypts it, restores it into a new database `wathena_verify_<random>`, checks that its migrations are the manifest's and not newer than this wactl's, and compares the counts. It drops the database at the end, also when a check fails.
- It exits with status 1 and names each difference on any mismatch.
- `--dir` checks the newest backup of a folder that has a manifest. `--every 24h` with `--dir` checks on a schedule.
- It needs `pg_restore`, `createdb` and `dropdb` 18 on the PATH.

#### Checking backups on the server

> **Warning.** The `backup-verify` service puts the identity on the server. Anyone who takes the server, or its disk, or gets root or Docker access to it, then has the backups and the key to decrypt them. Use it only if checks on the server are worth that to you. Otherwise check on your own machine, as above.

To turn it on:

1. Copy the identity to `secrets/backup_identity` on the server, and run `chmod 644 secrets/backup_identity`, as `secrets.sh` does for the other secrets. The `secrets/` folder stays open to its owner only.
2. Start the profile: `docker compose --profile verify up -d --wait`.

- It checks the newest backup at start and every 24 hours (`WATHENA_VERIFY_EVERY`). `docker compose logs backup-verify` shows each result.
- It connects as `wathena_verify`, a role that may make databases and has no rights on the world's database. `verify-init` makes it, only with the `verify` profile, and `secrets.sh` makes its password. A stack without the profile has no such role.
- The temporary database lives on the stack's PostgreSQL. Keep free disk for one more copy of the database.

To turn it off, run `docker compose rm -sf backup-verify` and delete `secrets/backup_identity`. The role stays. To remove it too, run `docker compose exec postgres psql -U postgres -c 'DROP ROLE wathena_verify'`. On a stack started before `verify-init` existed, `db-init` made the role on every `up`. The same command removes it there too.

#### Decrypting with age

The backups are standard age files. To get the plain `pg_dump` file:

```text
age --decrypt -i backup-identity.txt -o world.dump world-20261018T030000Z.dump.age
pg_restore --list world.dump
```

- The plain file has the secrets of the backup. Delete it when you are done.
- `age -d -i backup-identity.txt FILE | pg_restore ...` decrypts without a plain file on disk.

#### Restoring

To restore into a new, empty database, decrypt on your machine and stream the backup into the `wactl` service:

```text
docker compose down
docker volume rm wathena_pgdata
age -d -i backup-identity.txt world-20261018T030000Z.dump.age | docker compose run --rm -T wactl world restore --in -
docker compose up -d --wait
```

- Without Docker, `wactl world restore --in FILE --identity backup-identity.txt` decrypts and restores in one step. Plain backups, such as those of older releases, restore without `--identity`.
- `world restore` refuses a database that has any migration applied.
- It runs `pg_restore` in one transaction, so a failed restore leaves the database empty. A changed byte in an encrypted backup stops the restore before it commits.
- Keep the `account_secret_key` that was in use when the backup was made, or players with TOTP must use a recovery code.
- A backup from an older release gets its new migrations from `migrate` on `up`.
- `docker compose down -v` deletes every volume: the database, the backups, the assets and Caddy's certificates. Use plain `down` unless you mean that.

## Measure your own hardware

`wabots` ships with each release, next to `waserver` and `wactl`. It plays the load tests of strategy section 3.8 against a server, and checks the results against the budgets. Run it before you open a world, to learn how many players your machine holds.

Always use a test world in a database of its own, never your live world. The bots log in with development sessions, make characters, and change their levels, items and places.

1. Make the test world, as the migrator of a new database:

   ```text
   createdb wathena_loadtest
   WATHENA_MIGRATE_DATABASE_URL=postgres://.../wathena_loadtest \
     wactl world new --name "Load test" --admin Admin --admin-password-file -
   ```

2. Stop the live server, so the test measures the machine alone. Start `waserver` for the test world with development sessions and the debug address. Both need a loopback address:

   ```text
   WATHENA_DATABASE_URL=postgres://.../wathena_loadtest \
     waserver all --dev --addr 127.0.0.1:8300 --debug-addr 127.0.0.1:8301 --assets /path/to/build
   ```

3. Run the bots on the same machine, in their own process at a low priority, so their CPU does not count as the server's:

   ```text
   WATHENA_DATABASE_URL=postgres://.../wathena_loadtest GOMAXPROCS=4 nice -n 15 \
     wabots --scenario fields --bots 250 --metrics http://127.0.0.1:8301 --check
   ```

4. Run each scenario, and raise `--bots` by 250 while the budgets hold: `town`, `crowd`, `fight` (with `--place fields --fields 1` in a procedural world), `fields` and `graph`. `fields` and `graph` need the database URL, since they place the bots before they join. For a classic world, start `waserver` with `--map prontera` and give `wabots --assets` the same build. server/README.md, "Load tests and metrics", lists the scenarios and flags.
5. Read the report. `all budgets met` means every zone's tick work stayed under 10 ms at p99 and 25 ms at most, and every bot's traffic within its budgets. Each line `over budget:` names what broke. A p99 of 10.5 ms can still be within the budget: count the ticks over 10 ms, which must be fewer than 1% of the zone's ticks. Rare ticks over 25 ms come from the machine as often as from the server. `--check` exits with status 1 on any broken budget, and `--json` writes the report.
6. Repeat a step that is close to the budget. On a busy machine, results vary from run to run.
7. Drop the test database when you are done: `dropdb wathena_loadtest`.

Crowd puts every bot in view of every other, as at a town event. It is a stress test and breaks the budgets early on most machines.

Set `GOMEMLIMIT` for the live server to the memory it may use, such as `GOMEMLIMIT=2GiB`. Each online player needs about 70 to 120 KB of live heap, and about twice that in process memory with Go's default `GOGC`. An idle procedural world of the default size holds about 370 MB of live heap and 600 MB of process memory.

## Without Docker

`waserver` and `wactl` are single binaries. To run them on a host:

1. Run `postgres/roles.sql` once as a PostgreSQL superuser, and set each role's password with `\password` in psql.
2. Make the world as the migrator, which also applies the migrations: `WATHENA_MIGRATE_DATABASE_URL=... wactl world new --admin Admin --admin-password-file -`.
3. Make the key that encrypts TOTP secrets, once, and keep it with the other secrets: `openssl rand -base64 32`.
4. Start `waserver all` with the secrets in the environment or in files, never on the command line. See "Environment".
5. Put it behind a TLS proxy. Pass `--public-url https://...`, so cookies are `Secure` and passkeys work on its domain, and name the proxy with `--trusted-proxies`.

`wactl migrate up` applies the migrations of a new release. `wactl world backup` and `restore` need `pg_dump` and `pg_restore` 18 on the PATH, and `verify-backup` needs `pg_restore`, `createdb` and `dropdb`. For scheduled backups, run `wactl world backup --dir /var/backups/wathena --every 24h --recipients-file FILE` as a service, such as a systemd unit with `Restart=on-failure`.

## Database roles

| Role | Used by | Rights |
|---|---|---|
| `wathena_migrator` | `wactl migrate`, `wactl world` | Owns the database and the schema. Alone makes the world and its first account. |
| `wathena_game` | The game side of `waserver`, through `WATHENA_DATABASE_URL` | Reads and writes characters and inventories, reads and deletes game tickets, and reads the world without its seed. No DDL, and no access to passwords or login sessions. Deletes an account only through `delete_account`, once its player's deletion request is due (decision 0032). |
| `wathena_account` | The account service, through `WATHENA_ACCOUNT_DATABASE_URL` | Reads and writes passwords, login sessions, game tickets, TOTP secrets, recovery codes and passkeys, and makes accounts and characters. No DDL, and cannot change or delete characters. |
| `wathena_verify` | `wactl world verify-backup` in the `backup-verify` service, through `WATHENA_VERIFY_DATABASE_URL` | `CREATEDB` and nothing else: it makes, fills and drops its temporary databases from the `postgres` database. It cannot connect to `wathena`. In Compose, `verify-init` makes it, only with the `verify` profile. |

Without Docker, make the verify role only where you run `verify-backup`, as a superuser, and set its password with `\password wathena_verify`:

```text
CREATE ROLE wathena_verify LOGIN CREATEDB;
```

The migrations grant the table rights, and only to roles that exist when they run. A database made with an older `roles.sql`, before `wathena_account` existed, needs the role made and granted by hand before `wactl migrate up` applies migration 00002:

```text
CREATE ROLE wathena_account LOGIN;
GRANT CONNECT ON DATABASE wathena TO wathena_account;
GRANT USAGE ON SCHEMA public TO wathena_account;
```

## Environment

Each secret can also come from a file: `NAME_FILE` names the file that holds `NAME`. Setting both is an error.

| Variable | Use |
|---|---|
| `WATHENA_DATABASE_URL` | The game role's database URL. |
| `WATHENA_ACCOUNT_DATABASE_URL` | The account role's database URL. |
| `WATHENA_ACCOUNT_SECRET_KEY` | 32 random bytes in base64. Encrypts TOTP secrets in the database with AES-256-GCM. |
| `WATHENA_ACCOUNT_OLD_SECRET_KEYS` | Older keys, comma-separated, after a key change. They only open secrets sealed before the change. |
| `WATHENA_MIGRATE_DATABASE_URL` | The migrator role's URL, for `wactl migrate` and `wactl world`. |
| `WATHENA_VERIFY_DATABASE_URL` | The URL of a role with `CREATEDB`, for `wactl world verify-backup`. |

## Changing the key

1. Move the current key to `WATHENA_ACCOUNT_OLD_SECRET_KEYS`, and set a new `WATHENA_ACCOUNT_SECRET_KEY`. In the Compose stack, add a secret file for the old key, give it to `waserver` as `WATHENA_ACCOUNT_OLD_SECRET_KEYS_FILE`, and write the new key into `secrets/account_secret_key`.
2. Each secret is sealed again under the new key when its player next logs in with a code.
3. Keep the old key while any row still uses it. A secret whose key is lost cannot be opened, and its player must use a recovery code or a passkey and set TOTP up again.

Back up the key apart from the database. A database backup without the key does not reveal TOTP secrets.
