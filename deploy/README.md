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
| `wactl` | `wathena/wactl` | Not started by `up`. For `docker compose run`: making the world, building assets, backups and restores. |

The two wAthena images come from the repository's `Dockerfile`:

- `wathena/waserver` is distroless (no shell), with one static binary: `waserver` with the web client and the reference data built in. 12 MB compressed, 48 MB on disk. The build compiles Lua with `zig cc` and links the binary against musl, so it stays static. zig is downloaded in the build and checked against a pinned SHA-256.
- `wathena/wactl` is `wactl` on the `postgres:18` image, for `pg_dump` and `pg_restore`. 170 MB compressed, mostly the PostgreSQL image, which the stack pulls anyway.

You need Docker Engine with Compose 2.20 or later. For about 500 players, section 3.9 suggests 4 CPU cores, 8 GB of memory and 50 GB of SSD. Building assets from a full game client takes 4 GB of memory.

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
   - For a test without game files, use `--source placeholder`, and set `start_map: placeholder/map` and `mode: none` in `waserver.yml`.
5. World. Make the world and the first account. The password is read from standard input, so it stays out of the shell history and process lists:

   ```text
   read -rs PASSWORD
   printf '%s\n' "$PASSWORD" | docker compose run --rm -T wactl world new \
     --name "My World" --ruleset renewal --admin Admin --admin-password-file -
   ```

   `--ruleset` must match `mode` in `waserver.yml`. The command applies the migrations first, and makes the world's secret seed.

   To give the account the owner role, put its name in `staff.owners` in `waserver.yml`, as the command prints. The role counts once the account turns on TOTP or a passkey and logs in with it. The admin screens are then at `https://WATHENA_DOMAIN/admin` (server/README.md, "Staff and admin tools").
6. Start: `docker compose up -d --wait`. It waits until every service is healthy. Open `https://WATHENA_DOMAIN/`, sign up, make a character and press Play.

`waserver` refuses to start while migrations are pending or before step 5. Its log then says why: `docker compose logs waserver`.

### Settings

- `.env` sets the domain, the host ports, the images, where the assets are, and the edge network. `.env.example` explains each one.
- `waserver.yml` sets the game: the maps, the start map, the rAthena mode and the captcha. server/README.md, "All-in-one", lists every key.
- Fill in `privacy.contact` and `privacy.text` in `waserver.yml`. Players read them on `/account/privacy`, with the retention periods in effect. Players can download their data and delete their accounts on `/account/data`. You are responsible for following the laws that apply to you, such as the GDPR.
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

- Compose secrets on one machine are bind-mounted files. Anyone with root or Docker access on the host can read them.
- `secrets/` is only open to its owner. The files in it are readable by others, so the containers' users can read their mounted copies.
- Back up `secrets/`, and above all `account_secret_key`, apart from the database backups.
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

`wactl world backup` runs `pg_dump` as `wathena_migrator`, which owns every table. The backup holds password hashes, sealed TOTP secrets and the world seed. Keep it as private as the secrets.

```text
(umask 077 && docker compose run --rm -T wactl world backup --out - > wathena-$(date +%F).dump)
```

To restore into a new, empty database:

```text
docker compose down
docker volume rm wathena_pgdata
docker compose run --rm -T wactl world restore --in - < wathena-2026-10-01.dump
docker compose up -d --wait
```

- `world restore` refuses a database that has any migration applied.
- It runs `pg_restore` in one transaction, so a failed restore leaves the database empty.
- Keep the `account_secret_key` that was in use when the backup was made, or players with TOTP must use a recovery code.
- A backup from an older release gets its new migrations from `migrate` on `up`.
- `docker compose down -v` deletes every volume: the database, the assets and Caddy's certificates. Use plain `down` unless you mean that.

## Without Docker

`waserver` and `wactl` are single binaries. To run them on a host:

1. Run `postgres/roles.sql` once as a PostgreSQL superuser, and set each role's password with `\password` in psql.
2. Make the world as the migrator, which also applies the migrations: `WATHENA_MIGRATE_DATABASE_URL=... wactl world new --admin Admin --admin-password-file -`.
3. Make the key that encrypts TOTP secrets, once, and keep it with the other secrets: `openssl rand -base64 32`.
4. Start `waserver all` with the secrets in the environment or in files, never on the command line. See "Environment".
5. Put it behind a TLS proxy. Pass `--public-url https://...`, so cookies are `Secure` and passkeys work on its domain, and name the proxy with `--trusted-proxies`.

`wactl migrate up` applies the migrations of a new release. `wactl world backup` and `restore` need `pg_dump` and `pg_restore` 18 on the PATH.

## Database roles

| Role | Used by | Rights |
|---|---|---|
| `wathena_migrator` | `wactl migrate`, `wactl world` | Owns the database and the schema. Alone makes the world and its first account. |
| `wathena_game` | The game side of `waserver`, through `WATHENA_DATABASE_URL` | Reads and writes characters and inventories, reads and deletes game tickets, and reads the world without its seed. No DDL, and no access to passwords or login sessions. Deletes an account only through `delete_account`, once its player's deletion request is due (decision 0032). |
| `wathena_account` | The account service, through `WATHENA_ACCOUNT_DATABASE_URL` | Reads and writes passwords, login sessions, game tickets, TOTP secrets, recovery codes and passkeys, and makes accounts and characters. No DDL, and cannot change or delete characters. |

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

## Changing the key

1. Move the current key to `WATHENA_ACCOUNT_OLD_SECRET_KEYS`, and set a new `WATHENA_ACCOUNT_SECRET_KEY`. In the Compose stack, add a secret file for the old key, give it to `waserver` as `WATHENA_ACCOUNT_OLD_SECRET_KEYS_FILE`, and write the new key into `secrets/account_secret_key`.
2. Each secret is sealed again under the new key when its player next logs in with a code.
3. Keep the old key while any row still uses it. A secret whose key is lost cannot be opened, and its player must use a recovery code or a passkey and set TOTP up again.

Back up the key apart from the database. A database backup without the key does not reveal TOTP secrets.
