# server

Go services: gateway, account, world, and zone. See docs/strategy.md, section 4.

| Package | Content |
|---|---|
| `zone` | One map: its players and monsters, local chat to every player within 9 cells of the speaker, their walks through `shared/path`, and what each player sees within 14 cells, in 50 ms ticks. Monsters wander as rAthena's `mob_randomwalk`, from the zone's secret random stream. Units attack a target at their attack delay, walking into range first. A monster that can attack strikes back. Dead monsters respawn after their respawn time, and dead players return to the start after 3 seconds. A kill gives exp to the players who fought the monster and drops items around it. Items lie on the ground for a minute, and players pick them up with the looter rights rAthena gives the top attackers. Each player has rAthena's inventory: 100 slots, stacks up to 30000, and a weight limit that pick-ups respect. Players use healing items, and drop items at their feet with no looter rights. HP and SP regenerate as rAthena's natural regen, which stops from 70% weight in renewal (50% in pre-renewal). From 90% weight, attacks stop. |
| `item` | The inventory, ported from rAthena's `pc_additem`, `pc_delitem` and `pc_inventoryblank`, the overweight levels of `pc_updateweightstatus`, and the heal of `pc_itemheal` and `pc_percentheal`, with the item data from `reference/rathena/extracts/<mode>/items.json`. |
| `battle` | Attacks, ported from rAthena's `battle.cpp` in renewal mode and checked against the recorded reference. |
| `status` | Player stats from job, levels, raised stats and skills, ported from rAthena's `status_calc_pc` for a player with no equipment, in both modes, and checked against every recorded player. Raising stats with status points, ported from `pc_statusup`, `pc_need_status_point` and `pc_maxparameter`, with the costs of both modes and the job limits from `conf/battle/player.conf`. |
| `progression` | Exp tables, level ups, status and skill points, the renewal level penalty, exp shares, drop rates and rolls, where drops land, and pick-up rights, ported from rAthena's `pc.cpp`, `mob.cpp` and `map.cpp` with the default `conf/battle` settings. |
| `character` | A saved character as plain data: what the zone loads and hands to the store. |
| `store` | PostgreSQL persistence: goose migrations in `migrations`, sqlc queries in `queries` generated into `db`, a pool of save workers, and game ticket redemption. `storetest` makes throwaway databases for integration tests. See [decision 0003](../docs/decisions/0003-database-migrations-and-queries.md). |
| `account` | The account service under `/account/`: signup, login, logout, login sessions, account settings with TOTP codes, recovery codes and passkeys, the character list with character creation, Play, and one-time game tickets. Argon2id passwords, CSRF tokens, a strict Content Security Policy, rate limits and a captcha hook. `GameTickets` checks the ticket in `Hello` for the gateway. See decisions [0004](../docs/decisions/0004-account-service-and-game-tickets.md) and [0005](../docs/decisions/0005-two-factor-login-and-passkeys.md). |
| `gateway` | The WebSocket protocol from docs/proposals/0001-protocol.md: Hello first within 10 seconds, the session token checked through `Sessions` (game tickets, or names with `DevSessions`), trusted `Origin` checks, version and size checks, and commands to the zone. Chat lines are cleaned and checked (valid UTF-8, no control or bidirectional formatting characters, 1 to 255 code points) and rate limited per connection: a burst of 5, then 1 per second. |
| `cmd/waserver` | A server in one process: an asset build over HTTP, the account pages, and one zone over WebSocket, with characters in PostgreSQL. With `--dev`, names instead of accounts, and characters in PostgreSQL or in memory. |

## Development server

`--dev` takes any valid character name as the session token, instead of accounts. Anyone who reaches it can play any character, so it refuses to start unless `--addr` is a loopback address, such as the default `127.0.0.1:8300`. See "Running with accounts" for the real login.

```text
go run ./server/cmd/waserver --assets /path/to/build --map prontera --dev
```

It spawns the map's monsters from the spawns that `reference/rathena` recorded (`--mode renewal` or `pre-renewal`, or `none`), until the core mod has its own.

Players are novices with no equipment. With `--dev`, their sex and hair come from a hash of their name. With accounts, the player picks them on the character page. They start as a new rAthena character does: every stat at 1 and 48 status points to spend. `--build` picks other raised stats for testing, from the reference recorder's builds (STR, AGI, VIT, INT, DEX, LUK, before job bonuses), with the same 48 points:

| Build | Stats |
|---|---|
| `all_1` (default) | 1, 1, 1, 1, 1, 1 |
| `melee` | 90, 50, 30, 1, 40, 1 |
| `critical` | 70, 90, 1, 1, 40, 50 |
| `magic` | 1, 1, 30, 99, 80, 1 |
| `ranged` | 30, 40, 1, 1, 99, 30 |
| `tank` | 60, 1, 99, 1, 30, 1 |

The `all_1` novice can barely hurt a monster. `melee` is easier for testing drops and level-ups. Players start at base and job level 1, so there is no `--level`. Their stats come from the job tables in `reference/rathena/extracts/<mode>/job_stats.json` (renewal's with `--mode none`).

It serves the build at `http://127.0.0.1:8300/assets/` and the game at `ws://127.0.0.1:8300/ws`. With the web client's dev server:

```text
WATHENA_ASSETS=http://127.0.0.1:8300/assets pnpm dev
```

then open `http://localhost:5173/?server=ws://localhost:8300/ws&name=Alice` in two tabs with different names. Click the ground to walk, a monster to attack it, and an item to pick it up. Each tab sees the other walk and fight.

Players gain exp and levels from kills, with the exp tables, drops and level penalty from `reference/rathena/extracts/<mode>/progression.json`. Each base or job level up computes their stats again, and a base level up heals HP and SP fully. Players spend status points from the client's status window, and each raise computes their stats again. Local chat reaches every player within view, the speaker included; other channels are ignored for now. Without a database, characters and the inventory only live in memory. See "Running with PostgreSQL".

`--start-items` gives every new character items, for development only, such as `--start-items 501:5,512:10` for 5 Red Potions and 10 Apples. Without it, characters start with nothing and get items from drops. With a database, a character only gets them when it is made.

One character has one session. A second login with the same name, in any case, kicks the first, as rAthena does. The first page shows "Your character logged in somewhere else." The second continues once the first has saved.

## Running with PostgreSQL

`waserver` keeps characters in PostgreSQL 18 or later when it gets a database URL, from `--database` or the `WATHENA_DATABASE_URL` environment variable. Use the environment variable, so the password does not show in process lists. Accounts need a second URL, for the account role, in `WATHENA_ACCOUNT_DATABASE_URL`.

With a database:

- It refuses to start while migrations are pending.
- With `--dev`, a token that is a new name makes an account and a character of that name, marked as a dev account. Dev accounts have no password and cannot log in on the account pages.
- Characters load when they log in and are saved every 30 seconds when they changed, when they log out, and when the server stops (Ctrl+C or SIGTERM).
- The level, exp, status and skill points, raised stats, HP, SP, position, look, zeny and inventory are saved. Items keep their serial numbers.

A local database in Docker:

```text
docker run -d --name wathena-pg -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:18
docker exec -i wathena-pg psql -U postgres < deploy/postgres/roles.sql
docker exec -it wathena-pg psql -U postgres -c '\password wathena_migrator' -c '\password wathena_game'
```

`deploy/postgres/roles.sql` makes the `wathena` database and three roles:

- `wathena_migrator` owns the schema. Only `wactl migrate` uses it.
- `wathena_game` is what the game side of `waserver` uses. It reads and writes rows, and cannot change the schema, delete accounts and characters, or read passwords and login sessions. It only reads and deletes game tickets.
- `wathena_account` is what the account service uses. It alone reads password hashes, login sessions, TOTP secrets, recovery codes and passkeys. It makes accounts and characters, and cannot change or delete characters.

Set the passwords of all three with `\password` in psql, as the header of `roles.sql` shows. Apply the migrations as the migrator:

```text
export WATHENA_MIGRATE_DATABASE_URL='postgres://wathena_migrator:PASSWORD@127.0.0.1:5432/wathena?sslmode=disable'
go run ./tools/wactl migrate status
go run ./tools/wactl migrate up
```

Use `sslmode=disable` only for a database on the same machine.

### Running with accounts

Without `--dev`, `waserver` needs both database URLs. It serves the account pages under `/account/` and accepts only game tickets:

```text
export WATHENA_DATABASE_URL='postgres://wathena_game:PASSWORD@127.0.0.1:5432/wathena?sslmode=disable'
export WATHENA_ACCOUNT_DATABASE_URL='postgres://wathena_account:PASSWORD@127.0.0.1:5432/wathena?sslmode=disable'
export WATHENA_ACCOUNT_SECRET_KEY="$(openssl rand -base64 32)"
go run ./server/cmd/waserver --assets /path/to/build --map prontera \
  --account-url http://localhost:8300 --origins http://localhost:5173
```

With the web client's dev server running, open `http://localhost:5173/?server=ws://localhost:8300/ws`. The client sends you to the login page. Sign up, make a character, and press Play: the page returns to the client, which fetches a game ticket and connects. The Characters button, or the end of the game, goes back to the character list.

Use the same host name, such as `localhost`, for the client and the account service. The login cookie is `SameSite=Strict`, so the browser only sends it with the ticket request when both are on the same site. `localhost` and `127.0.0.1` are different sites.

`WATHENA_ACCOUNT_SECRET_KEY` encrypts TOTP secrets in the database. Keep the same key across restarts, or players with TOTP must use recovery codes. Without it, TOTP setup is off. `WATHENA_ACCOUNT_OLD_SECRET_KEYS` lists older keys after a key change. See [deploy/README.md](../deploy/README.md).

Two-factor login and passkeys:

- `/account/settings` turns TOTP on and off, makes new recovery codes, and adds and removes passkeys. The characters page links to it.
- Turning TOTP on, new recovery codes, and passkey changes need a login from the last 10 minutes. Otherwise the player confirms the password (and code) or a passkey first.
- Passkeys need `--account-url` with a domain name, such as `http://localhost:8300`. The browser only allows passkeys on https pages and on `localhost`.
- A logout, or a new login in the same browser, closes the game connection that its session opened, with `DISCONNECT_REASON_LOGGED_OUT`.

Flags:

| Flag | Use |
|---|---|
| `--origins` | Trusted web client origins, as `scheme://host[:port]`, comma-separated. They may fetch tickets and open the WebSocket, and Play may send players to them. Default `http://localhost:5173,http://127.0.0.1:5173`. |
| `--account-url` | The public URL of the account pages, such as `https://play.example.com`. With https, cookies are `Secure` with the `__Host-` prefix, and HSTS is sent. Its host is the passkeys' relying party ID. Empty takes the origin from each request, for development, and turns passkeys off. |
| `--play-url` | Where Play goes when the account pages were not opened from a client. Its origin must be in `--origins`. |
| `--trusted-proxies` | Proxy addresses or CIDR blocks whose `X-Forwarded-For` gives the client address, for rate limits. |
| `--captcha` | `none` (default), or `dev` for a checkbox that shows where a real captcha goes. A real provider is not built yet. |

Rate limits, session lifetimes and Argon2id parameters are in [decision 0004](../docs/decisions/0004-account-service-and-game-tickets.md). TOTP, recovery code, pending login and passkey parameters are in [decision 0005](../docs/decisions/0005-two-factor-login-and-passkeys.md). The rate limits live in the memory of the process.

### Changing the schema or queries

- Add a migration as the next numbered file in `server/store/migrations`, with `-- +goose Up` and `-- +goose Down` sections. Grant `wathena_game` only the rights it needs, as the first migration does.
- Add queries to `server/store/queries`. Game code only runs queries that sqlc generates. `.github/scripts/sqlcheck.sh` rejects `fmt.Sprintf` and raw query calls in database code.
- Generate the code, and commit it:

```text
cd server/store && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
```

sqlc needs a C compiler for its PostgreSQL parser. CI generates the code again and fails when it differs.

### Tests

The integration tests of `store` and `account` need a PostgreSQL 18 server and a role that can create databases and roles. Each test makes its own database and drops it afterwards, and acts as the service roles with `SET ROLE`. Without `WATHENA_TEST_DATABASE_URL`, they are skipped.

```text
docker run -d --name wathena-test-pg -e POSTGRES_PASSWORD=postgres -p 55432:5432 postgres:18
WATHENA_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' go test ./server/store/ ./server/account/
```

CI's `test` job runs them against a `postgres:18` service container.
