# server

Go services: gateway, account, world, and zone. See docs/strategy.md, section 4.

| Package | Content |
|---|---|
| `zone` | One map: its players and monsters, local chat to every player within 9 cells of the speaker, their walks through `shared/path`, and what each player sees within 14 cells, in 50 ms ticks. Monsters wander as rAthena's `mob_randomwalk`, from the zone's secret random stream. Units attack a target at their attack delay, walking into range first. Monsters run rAthena's AI and use their skills (see "Monster AI and monster skills"). Dead monsters respawn after their respawn time, and dead players return to the start after 3 seconds. A kill gives exp to the players who fought the monster and drops items around it. Items lie on the ground for a minute, and players pick them up with the looter rights rAthena gives the top attackers. Each player has rAthena's inventory: 100 slots, stacks up to 30000, and a weight limit that pick-ups respect. Players use items, whose scripts run in Lua (see "Item scripts"), and drop items at their feet with no looter rights. HP and SP regenerate as rAthena's natural regen, which stops from 70% weight in renewal (50% in pre-renewal). From 90% weight, attacks stop. Warps send a player who steps into their area to their target, as rAthena's `npc_touch_areanpc` and `pc_setpos`: to another map as a message to that map's zone, or within the map. A zone without players sleeps. Each tick finds what a player sees through an index of 8 × 8 cell blocks, as rAthena's `BLOCK_SIZE`, and hands its messages to a sender goroutine in one batch (see "Load tests and metrics"). |
| `metrics` | Measurements for the budgets in strategy section 3.8: tick work per zone, garbage collector pauses, CPU, memory, goroutines, and bytes per connection. A zone over its tick budget for 10 ticks in a row logs a warning and can write a CPU profile. See "Load tests and metrics". |
| `world` | Several zones in one process, one goroutine per map. It finds zones by map name, puts a joining character in the zone of its saved map, and waits at shutdown for every zone and every player between zones. `LoadWarps` reads `reference/rathena/extracts/<mode>/warps.json`. |
| `item` | The inventory, ported from rAthena's `pc_additem`, `pc_delitem` and `pc_inventoryblank`, the overweight levels of `pc_updateweightstatus`, the heal of `pc_itemheal` and `pc_percentheal`, and the item groups of `getgroupitem`, with the item data from `reference/rathena/extracts/<mode>/items.json`, `equipment.json` and `item_groups.json`. Equipping, ported from `pc_equippoint`, `pc_isequip`, `pc_equipitem`, `pc_unequipitem`, `pc_setequipindex`, `pc_calcweapontype` and `pc_insert_card` (see "Equipment"). Rolled values and random options, with `s_random_opt_group::apply` (see "Rolled item values"). |
| `battle` | Attacks, ported from rAthena's `battle.cpp` in renewal mode and checked against the recorded reference, with the equipment bonuses of both sides (`battle_calc_cardfix`, `battle_absorb_damage` and the player target's defense). |
| `status` | Player stats from job, levels, raised stats, skills and equipment, ported from rAthena's `status_calc_pc` and `pc_bonus`, in both modes, and checked against every recorded player, every equipment item and card of `item_bonuses.json`, and every level of `refine.json`. Raising stats with status points, ported from `pc_statusup`, `pc_need_status_point` and `pc_maxparameter`, with the costs of both modes and the job limits from `conf/battle/player.conf`. |
| `statuschange` | Status effects, ported from rAthena's `status_change_start`, `status_change_end`, `status_change_clear`, `status_change_timer` and `status_get_sc_def`, with the data of `status.yml` from `reference/rathena/extracts/<mode>/status.json`, and checked against every case of `status_effects.json` (see "Status effects"). |
| `progression` | Exp tables, level ups, status and skill points, the renewal level penalty, exp shares, drop rates and rolls, where drops land, and pick-up rights, ported from rAthena's `pc.cpp`, `mob.cpp` and `map.cpp` with the default `conf/battle` settings. |
| `companion` | Companions, mounts and carts: the records of `data/companions.yml` and `data/mounts.yml`, core's own files, the core rules a record acts as, and the option bits that show each look (see "Companions, mounts and carts"). |
| `rules` | A world's ruleset flags as the code reads them, the skills whose `skill_rebalance` flags change something, and `rulestest`, the sampled mixed rulesets of the tests. See "Rulesets". |
| `character` | A saved character as plain data: what the zone loads and hands to the store. |
| `transfer` | Moves of items and zeny between owners, checked as one step and logged with serials: inventories, storages, carts and the guild storage now, and the holders that later steps add (mail and vending). See "Transfers and item logs". |
| `storage` | The storages and carts of the characters online: accounts' storages, the extra storages of `data/storages.yml` and carts, loaded at login and let go after the last commit, one opener at a time. See "Storage, cart and trade". |
| `emblem` | Guild emblem uploads: the size, format and dimension checks, and the PNG the server keeps. See "Guilds". |
| `store` | PostgreSQL persistence: goose migrations in `migrations`, sqlc queries in `queries` generated into `db`, a pool of save workers, transfer commits with the item and zeny logs, and game ticket redemption. `storetest` makes throwaway databases for integration tests. See [decision 0003](../docs/decisions/0003-database-migrations-and-queries.md). |
| `account` | The account service under `/account/`: signup, login, logout, login sessions, account settings with TOTP codes, recovery codes and passkeys, the character list with character creation, Play, and one-time game tickets. Argon2id passwords, CSRF tokens, a strict Content Security Policy, rate limits and a captcha hook. `GameTickets` checks the ticket in `Hello` for the gateway. See decisions [0004](../docs/decisions/0004-account-service-and-game-tickets.md) and [0005](../docs/decisions/0005-two-factor-login-and-passkeys.md). |
| `gateway` | The WebSocket protocol from docs/proposals/0001-protocol.md: Hello first within 10 seconds, the session token checked through `Sessions` (game tickets, or names with `DevSessions`), trusted `Origin` checks, version and size checks, and commands to the zone the player is in, through its `zone.Session`, which stays the same across map changes. Chat lines are cleaned and checked (valid UTF-8, no control or bidirectional formatting characters, 1 to 255 code points) and rate limited per connection: a burst of 5, then 1 per second. |
| `cmd/waserver` | The all-in-one server: the web client, an asset build over HTTP, the account pages, and a world of zones over WebSocket, one per map, with characters in PostgreSQL. With `--dev`, names instead of accounts, and characters in PostgreSQL or in memory. See "All-in-one". |

## Building

`waserver` runs Lua through cgo ([internal/script](../internal/script/README.md)).

- `go build ./server/cmd/waserver` and `go test ./...` need a C compiler. Go uses the system one: clang on macOS, gcc on Linux.
- Without a C compiler, or with `CGO_ENABLED=0`, the same code runs Lua compiled to WebAssembly in wazero. It is slower, and meant for development.
- Release builds use `zig cc`: static musl binaries for Linux, and binaries for Windows and macOS. The Dockerfile and CI's `lua` and `lua-macos` jobs have the commands. Static Linux builds need `-Wl,-z,stack-size=8388608`, since Lua needs more C stack than musl's default.
- At startup, `waserver` runs a self-test of the Lua runtime and refuses to start when it fails.

## Development server

`--dev` takes any valid character name as the session token, instead of accounts. Anyone who reaches it can play any character, so it refuses to start unless `--addr` is a loopback address, such as the default `127.0.0.1:8300`. See "Running with accounts" for the real login.

```text
go run ./server/cmd/waserver --assets /path/to/build --map prontera --dev
go run ./server/cmd/waserver --assets /path/to/build --map prontera --dev \
  --maps prontera,prt_fild08,prt_in,prt_fild05,prt_fild06,prt_fild07,prt_church,izlude
go run ./server/cmd/waserver --assets /path/to/build --map prontera --dev --maps all
```

It spawns each map's monsters from the spawns that `reference/rathena` recorded (`--mode renewal` or `pre-renewal`, or `none`), until the core mod has its own.

Players start as novices with no equipment. With `--dev`, their sex and hair come from a hash of their name. With accounts, the player picks them on the character page. They start as a new rAthena character does: every stat at 1 and 48 status points to spend. `--build` picks other raised stats for testing, from the reference recorder's builds (STR, AGI, VIT, INT, DEX, LUK, before job bonuses), with the same 48 points:

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

## Maps and warps

`--maps` lists the maps to run, comma-separated, or `all` for every map in the asset build. Without it, only `--map` runs. `--map` is where new characters start, and where characters enter when their saved map does not run. It must be one of the maps.

Each map runs in its own zone goroutine, which owns all of the map's state (strategy section 4.1):

- Zones share no mutable state. They read the same data tables, which never change after startup.
- Each zone has its own secret random stream from the world's root, under the label `zone:<map ID>` (docs/specs/rng-v1.md).
- The warps come from `reference/rathena/extracts/<mode>/warps.json`, in rAthena's load order. Only enabled warps run.

A warp works as in rAthena:

- A player who steps onto a cell within `xs` and `ys` of a warp goes to its target (`npc_touch_areanpc`). The walk stops there. When warp areas overlap, the warp loaded first wins.
- Dead players do not warp. A warp to a map that does not run does nothing, and the walk goes on, as `pc_setpos` without a map server.
- A target cell outside the map, or 0,0, becomes a random walkable cell away from warps. A target cell inside the map is used even when it is not walkable, as `pc_setpos` does. The player can walk off it.
- A player who arrives on a warp area is warped again once its first frame is sent, as `clif_parse_LoadEndAck`. After 10 warps in a row, `count_rewarp` stops it.
- A warp within the same map moves the player there. Others see it go out of sight.

A map change is a message between zones:

1. The old zone copies the character, with its target map and cell, and its item waits. It removes the player (others see `VANISH_REASON_OUT_OF_SIGHT`) and sends the copy to the new zone's inbox from a new goroutine, so two zones never wait on each other.
2. The session, which the gateway holds, points at no zone while the player moves. Commands sent then are dropped.
3. The new zone places the player, saves the character at once (the map change checkpoint of strategy section 3.8), and points the session at itself. Its first frame to the player starts with `MapChange`.
4. A logout while the player moves ends the session at the new zone, with one final save. A player moving to a zone that stopped gets its final save at shutdown.

The character is in one zone at a time, so its items and exp are never copied. Chat and visibility stay within a map.

A zone without players sleeps: its ticker stops and its monsters stand still. rAthena runs full monster AI only near players, and monsters no player has seen stand still (`mob_ai_sub_lazy`). A zone wakes when a player joins or arrives, and its monsters pick new wander times then.

Measured with the kRO asset build of 988 maps, in renewal, on an Apple M2 with 8 cores:

| Setup | Startup | Heap after startup | CPU while idle |
|---|---:|---:|---:|
| `--maps prontera` | 9 ms | 11 MB | 0 |
| 8 maps around Prontera, 1,415 monsters | 28 ms | 12 MB | 0 |
| `--maps all`: 988 maps, 59,374 monsters, 3,855 warps | 2.1 s | 186 MB | 0 while asleep |
| `--maps all` with every zone kept awake | 2.1 s | 186 MB | 59% of one core |

- An idle map costs about 180 KB of heap, mostly its cells and monsters.
- One tick of a map with no players takes 10 µs with 60 monsters (the average over all maps) and 18 µs with 215 (the average field), from `BenchmarkIdleTick` in `server/zone`. Kept awake at 20 ticks a second, all 988 maps cost 0.6 CPU seconds a second, which sleeping saves.
- Startup reads only each map's cells (`wmap.DecodeCells`), several maps at once.

Players gain exp and levels from kills, with the exp tables, drops and level penalty from `reference/rathena/extracts/<mode>/progression.json`. Each base or job level up computes their stats again, and a base level up heals HP and SP fully. Players spend status points from the client's status window, and each raise computes their stats again. Local chat reaches every player within view, the speaker included; other channels are ignored for now. Without a database, characters and the inventory only live in memory, until the server stops. A name that logs in again plays its character as it was when it left. See "Running with PostgreSQL".

`--start-items` gives every new character items, for development only, such as `--start-items 501:5,512:10` for 5 Red Potions and 10 Apples. Without it, characters start with nothing and get items from drops. With a database, a character only gets them when it is made.

One character has one session. A second login with the same name, in any case, kicks the first, as rAthena does. The first page shows "Your character logged in somewhere else." The second continues once the first has saved.

## Running with PostgreSQL

`waserver` keeps characters in PostgreSQL 18 or later when it gets a database URL, from `--database` or the `WATHENA_DATABASE_URL` environment variable. Use the environment variable, so the password does not show in process lists. Accounts need a second URL, for the account role, in `WATHENA_ACCOUNT_DATABASE_URL`.

With a database:

- It refuses to start while migrations are pending.
- With `--dev`, a token that is a new name makes an account and a character of that name, marked as a dev account. Dev accounts have no password and cannot log in on the account pages.
- Characters load when they log in and are saved every 30 seconds when they changed, when they log out, and when the server stops (Ctrl+C or SIGTERM).
- Characters are also saved when they arrive on another map, at their new cell.
- The level, exp, status and skill points, raised stats, HP, SP, map and position, look, zeny and inventory are saved. Items keep their serial numbers, and their rolled values and random options (migration 00013, "Rolled item values").
- A move of items or zeny is saved with its log lines at once, in one transaction with the characters it changed (see "Transfers and item logs").

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

Without `--dev`, `waserver` also needs a world. `wactl world new` applies pending migrations, then makes the world's row with its secret seed and the first staff account:

```text
read -rs PASSWORD
printf '%s\n' "$PASSWORD" | go run ./tools/wactl world new --admin Admin --admin-password-file -
```

`--ruleset` is `renewal` by default, and must match `waserver`'s `--mode`. Staff accounts have no staff tools yet.

### Running with accounts

Without `--dev`, `waserver` needs both database URLs. It serves the account pages under `/account/` and accepts only game tickets:

```text
export WATHENA_DATABASE_URL='postgres://wathena_game:PASSWORD@127.0.0.1:5432/wathena?sslmode=disable'
export WATHENA_ACCOUNT_DATABASE_URL='postgres://wathena_account:PASSWORD@127.0.0.1:5432/wathena?sslmode=disable'
export WATHENA_ACCOUNT_SECRET_KEY="$(openssl rand -base64 32)"
go run ./server/cmd/waserver --assets /path/to/build --map prontera \
  --public-url http://localhost:8300 --origins http://localhost:5173
```

With the web client's dev server running, open `http://localhost:5173/?server=ws://localhost:8300/ws`. The client sends you to the login page. Sign up, make a character, and press Play: the page returns to the client, which fetches a game ticket and connects. The Characters button, or the end of the game, goes back to the character list.

Use the same host name, such as `localhost`, for the client and the account service. The login cookie is `SameSite=Strict`, so the browser only sends it with the ticket request when both are on the same site. `localhost` and `127.0.0.1` are different sites.

`WATHENA_ACCOUNT_SECRET_KEY` encrypts TOTP secrets in the database. Keep the same key across restarts, or players with TOTP must use recovery codes. Without it, TOTP setup is off. `WATHENA_ACCOUNT_OLD_SECRET_KEYS` lists older keys after a key change. See [deploy/README.md](../deploy/README.md).

Two-factor login and passkeys:

- `/account/settings` turns TOTP on and off, makes new recovery codes, and adds and removes passkeys. The characters page links to it.
- Turning TOTP on, new recovery codes, and passkey changes need a login from the last 10 minutes. Otherwise the player confirms the password (and code) or a passkey first.
- Passkeys need `--public-url` with a domain name, such as `http://localhost:8300`. The browser only allows passkeys on https pages and on `localhost`.
- A logout, or a new login in the same browser, closes the game connection that its session opened, with `DISCONNECT_REASON_LOGGED_OUT`.

Flags of the account service, all in "All-in-one" with their config keys:

- `--public-url` (or its old name `--account-url`): the public URL, such as `https://play.example.com`. With https, cookies are `Secure` with the `__Host-` prefix, and HSTS is sent. Its host is the passkeys' relying party ID. Empty takes the origin from each request, for development, and turns passkeys off.
- `--origins`: more trusted web client origins, as `scheme://host[:port]`, comma-separated. They may fetch tickets and open the WebSocket, and Play may send players to them. The public URL's origin is always trusted. Without `--origins` and without a public URL, the default is the Vite dev server, `http://localhost:5173,http://127.0.0.1:5173`.
- `--play-url`: where Play goes when the account pages were not opened from a client. Its origin must be trusted. Without it, Play opens the client that `waserver` serves at the public URL.
- `--trusted-proxies`: proxy addresses or CIDR blocks whose `X-Forwarded-For` gives the client address, for rate limits.
- `--captcha`: `none` (default), or `dev` for a checkbox that shows where a real captcha goes. A real provider is not built yet.

Rate limits, session lifetimes and Argon2id parameters are in [decision 0004](../docs/decisions/0004-account-service-and-game-tickets.md). TOTP, recovery code, pending login and passkey parameters are in [decision 0005](../docs/decisions/0005-two-factor-login-and-passkeys.md). The rate limits live in the memory of the process.

### Changing the schema or queries

- Add a migration as the next numbered file in `server/store/migrations`, with `-- +goose Up` and `-- +goose Down` sections. Grant `wathena_game` only the rights it needs, as the first migration does.
- Add queries to `server/store/queries`. Game code only runs queries that sqlc generates. `scripts/sqlcheck.sh` rejects `fmt.Sprintf` and raw query calls in database code.
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

## All-in-one

`waserver all` runs every service in one process (strategy section 3.9): the gateway and the world's zones at `/ws`, the account pages at `/account/`, the asset build at `/assets/`, and the web client at `/`. `waserver` with flags and no command does the same. deploy/README.md runs it with PostgreSQL and Caddy in Docker Compose.

```text
waserver all --config waserver.yml
waserver health [--url http://127.0.0.1:8300/readyz]
waserver version
```

Settings come from the defaults, then the YAML file that `--config` names, then the flags given on the command line. An unknown key in the file is an error. Secrets are refused in the file: they come from the environment (deploy/README.md, "Environment").

| Key | Flag | Default | Use |
|---|---|---|---|
| `listen` | `--addr` | `127.0.0.1:8300` | Address to listen on. |
| `public_url` | `--public-url`, `--account-url` | none | The public URL, the root of an origin. https, or http on a loopback host. Required when `listen` is not a loopback address. |
| `origins` | `--origins` | see "Running with accounts" | More trusted web client origins. |
| `play_url` | `--play-url` | the public URL | Where Play goes. |
| `trusted_proxies` | `--trusted-proxies` | none | Proxies whose `X-Forwarded-For` is believed. |
| `assets` | `--assets` | none, required | The asset build folder, with its `manifest.json`. |
| `client_dir` | `--client-dir` | the built-in client | A web client build to serve at `/` instead. |
| `start_map` | `--map` | `prontera` | Where new characters start. |
| `maps` | `--maps` | the start map | Maps to run, or `all`. See "Maps and warps". |
| `mode` | `--mode` | `renewal` | The rAthena mode of the reference data: `renewal`, `pre-renewal`, or `none` for no monsters. It must match the world's ruleset. |
| `captcha` | `--captcha` | `none` | `none` or `dev`. |
| `dev` | `--dev` | `false` | Development sessions instead of accounts. Needs a loopback `listen`. |
| `build` | `--build` | `all_1` | Raised stats of new characters, for testing. |
| `start_items` | `--start-items` | none | Items of new characters, for development. |
| `start_job` | `--start-job` | none | Job, base level and job level of new characters, as `job:base:job`, for development. Needs `dev`. |
| `seed` | `--seed` | none | Text that fixes the random streams of the zones and world events, so a test runs the same way each time. For tests only. Needs `dev`. |
| `debug_addr` | `--debug-addr` | none | Loopback address for measurements and profiles. |
| `profile_dir` | `--profile-dir` | none | Folder for CPU profiles of slow ticks. |
| `shutdown_countdown` | `--shutdown-countdown` | `10s` | The countdown players see before the server stops, up to `10m`. `0s` turns it off. See "NPCs". |
| `time_zone` | `--time-zone` | `UTC` | The server's time zone, as an IANA name such as `Asia/Manila`. Schedules and world events match their times there. See "World events". |
| `mods.dir` | `--mods` | none | The folder of installed mods. See "Mods". |
| `mods.ruleset` | `--ruleset` | the core ruleset of `mode` | The world's ruleset mod. |
| `mods.strict` | `--mods-strict` | `false` | Data overlay warnings become errors. |
| `mods.approve` | none | none | Approved permissions, by mod ID. |
| `mods.disable` | none | none | Installed mods that must not load. |
| `mods.events` | none | none | Changes to world events, by event name. See "World events". |
| `chat.burst`, `chat.refill` | none | `5`, `1s` | Chat rate limits: lines at once, and the time for one more. See "Chat and moderation". |
| `chat.new_character_burst`, `chat.new_character_refill`, `chat.new_character_period` | none | `3`, `3s`, `1h` | Lower limits for a character's first period after it was made. A period of `0s` turns them off. |
| `chat.log_days` | none | `30` | Days the chat log keeps lines, except lines of open reports. More than 30 logs a warning. |
| `jail.map`, `jail.cells` | none | `sec_pri`, `[[49, 75], [24, 75]]` | Where jailed players go, as rAthena's `MAP_JAIL` and `pc_jail`. A jail map that does not run logs a warning. |

`waserver` checks everything before it starts, and lists every problem by key. With a database it also refuses to start:

- while migrations are pending (`wactl migrate up`),
- before `wactl world new` made the world, except with `--dev`,
- when `mode` is not the world's ruleset, except `none`,
- when a mod cannot load (see "Mods").

The web client:

- A binary built with the `webclient` build tag embeds `client/dist`, which `pnpm build` makes first. The container image is built this way. Without the tag, `go build ./...` needs no Node.js, and `waserver` serves no client unless `client_dir` names one.
- `waserver` serves it from memory, compressed with Brotli and gzip at startup. Files under `app/` have hashed names and are cached for a year. `index.html` and `sw.js` are checked on each visit.
- Every client response has a strict Content Security Policy that allows only this origin and its WebSocket, and HSTS with an https public URL.
- A production build plays on `/ws` of its own origin. `?server=` names another server, and `?viewer` opens the map viewer.

The reference data that `waserver` reads at startup (`reference/rathena/extracts` and `mob_status.json` of both modes) is embedded in the binary by `reference/rathena/embed.go`, so it runs outside the repository. It adds 24 MB to the binary, about half of its 48 MB, and compresses to about 2 MB in the image. The client adds 0.9 MB.

Health checks:

- `/healthz` answers `ok` while the process serves HTTP.
- `/readyz` answers `ok` once the world runs and both databases answer, until shutdown starts. Otherwise it answers 503.
- `waserver health` fetches `/readyz` and exits with status 1 when it is not ready, for container images without curl.

On SIGTERM or Ctrl+C, `waserver` stops being ready, ends every game connection with `DISCONNECT_REASON_SERVER_SHUTDOWN`, saves every character, stops its zones, and gives the save workers 10 seconds to finish.

## Mods

`waserver` loads the mods in `mods.dir` at startup (strategy section 6, [decision 0006](../docs/decisions/0006-mod-loader.md)). [mods/README.md](../mods/README.md) is the guide for mod authors. `mods/examples/greeter` is a small sample, `mods/examples/halloween` adds an NPC, an item, monsters, a hook and world events, `mods/examples/verve` adds a status, `mods/examples/chain-lightning` adds a skill, and `mods/examples/familiars` adds a companion and a mount.

```yaml
mods:
  dir: /srv/mods
  ruleset: core.ruleset.renewal
  approve:
    example.halloween-event: [player.give_items]
  disable: [example.old-event]
```

What it does:

- Each folder in `mods.dir` with a `mod.yaml`, and each `.zip` file with one, is a mod. Other entries are skipped.
- The built-in `core` mod stands for the base game. `core.ruleset.renewal` and `core.ruleset.pre_renewal` are its ruleset mods.
- Mods load after their `depends`, their `load_after` that are installed, and `core`. Ties go by mod ID, so the order is the same on every start.
- The world's ruleset is `mods.ruleset`, or the core ruleset of `mode`. Ruleset mods that are not the world's ruleset, or what it extends, are skipped.
- Items from mods get IDs from 2,000,000 the first time they load, monsters from 100,000, skills from 20,000, and jobs, companions and mounts from 10,000. With a database the IDs are kept in the `mod_ids` table, and never change or go to another record. Without a database they live in memory, like the characters.
- Monster overlays change core's monsters everywhere they spawn, and mods' monsters appear through scripts.
- Skills and skill trees come from the extracts `skills.json` and `skill_trees.json` of the mode, with each mod's `data/skills.yml` and `data/skill_trees.yml` over them. Every zone shares the result.
- Each zone runs the mods' scripts in its own Lua state. A mod whose scripts fail is logged and left off in that zone. A mod whose handlers fail 10 times in a minute is turned off in that zone.
- A zone makes its Lua state at start when a mod has entry scripts, and otherwise when a player first uses an item there.

## Rulesets

A world's ruleset is a base and a value for each flag of strategy section 3.6. [Decision 0019](../docs/decisions/0019-ruleset-flags.md) lists what each flag changes, and [mods/README.md](../mods/README.md), "Rulesets", how a ruleset mod sets them.

- `gamedata.RulesOf` turns the loader's ruleset into `rules.Rules`: one value per flag, and the `skill_rebalance` flags by skill ID.
- `LoadMods` applies them. The job tables get `aspd`, `size_table`, `cast_time`, `stat_point_cost`, `fourth_job_traits` and `ensemble_songs`. The exp and drop tables get the level penalty flags, with renewal's table on a pre-renewal base. The skill data gets `skill_cooldown`'s cooldowns, and the records of the songs and the rebalanced skills from their flag's mode. Every zone gets the rules in `zone.Mods.Rules`.
- The code reads the flag of its feature where rAthena checks `RENEWAL`. The base still picks the data sets and the stat and damage models. The battle code takes the rules with each attack (`battle.AttackRules`, `battle.SkillAttackRules`).
- The log line `mods loaded` shows the ruleset and the flags it changes. A `skill_rebalance` flag for a skill whose versions the server does not tell apart is logged as a warning.
- `homunculus` changes nothing yet: homunculi do not exist.
- A ruleset also has limits and rules ([decision 0020](../docs/decisions/0020-parties-and-friends.md)): `rules.Limits` (`party_members`, `friends`, `block_list`) and `rules.Party` (`party_share_level` and the other party values of rAthena's conf files). `server/rules` lists their defaults and bounds. The gateway sends the limits in `Welcome`.
- Scripts read the flags, rules and limits with `rules.is`, `rules.get` and `rules.get_limit` (review decision 32). `Rules.Values` holds every rule's value for them. The converter turns `basicskillcheck` and `checkre(1)` to `checkre(5)` into these reads.

Tests: each flag has tests at both values on both bases (`TestEveryFlagBothValues` in `server/mod` names them). `rules/rulestest.Mixes` makes 8 mixed rulesets per base, and each package runs the recordings that apply to them (the `TestMixedRulesets` tests).

## Item scripts

`waserver` converts rAthena's item scripts for its `mode` when it starts, and logs `item and npc scripts converted` with the time it took, about 0.9 s for renewal ([decision 0007](../docs/decisions/0007-converted-item-scripts.md), mods/README.md, "Core's item scripts"). They are the files of the `core` mod.

- A used item runs its Lua `use` handler: heals, items, item boxes, effects, warps, status effects and timed bonuses.
- An item whose script needs what the server does not have yet, such as pets, is refused and kept. The player sees a chat line that says what it needs.
- Butterfly Wings and `player:warp_to_save_point()` go to the character's save point. A new character's save point is the start cell of the start map (`--map`).
- Rentals are given as normal items, since items do not expire yet.

To try them, give a new character some items:

```text
go run ./server/cmd/waserver --assets /path/to/build --map prt_fild08 --dev \
  --start-items 501:5,603:3,601:3,602:2,619:2
```

These are Red Potions, Old Blue Boxes, Fly Wings, Butterfly Wings, and Unripe Apples, which need pets and are refused.

Any of these stop the start, with every problem listed:

- A manifest with a missing or wrong field. The error names the file and the field.
- A missing dependency, a dependency at a version outside the range, or a load order cycle, which the error shows as `a -> b -> a`.
- A mod whose `supports.rulesets` does not match the world's ruleset.
- A permission that needs approval and is not in `mods.approve`. A mod either runs with every permission it asked for or not at all, so a player never meets half a mod. The error names the permissions to review.
- A data overlay error. With `mods.strict`, also two mods setting the same field.

Flagged permissions, such as `rules.override`, are logged as warnings at each start.

`mode` and the ruleset:

- `mode` picks the rAthena reference data: monsters, drops, items and tables. The world row pins it, and `waserver` refuses another.
- The ruleset's base must be the same as `mode`: `renewal` with `renewal`, `pre_renewal` with `pre-renewal`.
- The ruleset's flags decide which mods may load, and pick the game code of each feature ("Rulesets" below).

Reloading a mod's scripts, for development:

```text
curl -X POST 'http://127.0.0.1:8301/debug/wathena/mods/reload?mod=example.greeter'
```

- It needs `dev` and `debug_addr`. It is served on the debug address only.
- It reads the mod's Lua files again, ends the mod's context in every zone, which removes its handlers and timers, and runs the new entry scripts.
- It does not read `mod.yaml` or data files again. Those need a restart.
- Other mods that required the mod's helper modules keep the versions they loaded.
- A reload that fails in a zone leaves the mod off there until a reload succeeds.
- A zone that has no Lua state yet keeps the new files, and makes its state from them later.
- `wactl dev` runs a server with one mod and calls the endpoint when the mod's Lua files change (mods/README.md, "Tools").

Cost: each zone's Lua state holds about 50 KB for the sample mod. All 988 maps of a kRO build start in 2.1 seconds with it, against 2.0 without mods.

## Equipment

Players wear items from their inventory, as rAthena's `pc_equipitem` and `pc_unequipitem` ([decision 0009](../docs/decisions/0009-stats-as-totals-of-modifiers.md)).

- Positions are rAthena's `EQP_` bits: three headgear positions, armor, both hands, garment, shoes, two accessories, ammunition, four costume positions and six shadow gear positions.
- An item goes on when the player's job, class (`Jobs` and `Classes`), sex and base level allow it (`pc_isequip`). Ammunition needs a weapon that fires it: arrows a bow, an instrument or a whip, bullets a gun.
- What is in the way comes off first. A two-handed weapon takes both hands, so the shield comes off too. Accessories go right first, then left. Classes that dual wield (Assassins, Kagerou and Oboro, and anyone with Left Hand Mastery) put a second dagger, one-handed sword or one-handed axe in the left hand.
- Ammunition that does not fit a new weapon comes off, and so does the ammunition of a bow or gun that comes off (`ammo_unequip`).
- `InsertCard` puts a card into the first free card slot of an item that is not worn, as `pc_insert_card`: the card's location must match the item's, a shield card cannot go into a weapon, and a card for one accessory side only goes into that side. Items hold at most `MaxCardSlots` cards (rAthena's `MAX_SLOTS`, 4).
- Worn items cannot be used or sold. An item that leaves the inventory comes off first. A dropped item keeps its refine and cards on the ground.

Each worn slot has a handle that scripts read through `player:get_equipped` and the equipped item's fields. Its item and each card run their converted script in an equip context. A worn item that is still on when its player leaves the map ends quietly, and starts again where the player arrives. Taking it off runs its `unequip` handlers. Each normal attack that lands fires the `damage` event of the equipped items on both sides.

Stats come from `status.Calc` with every worn item, its refine, its cards and their bonuses, in rAthena's order:

- weapon ATK, MATK, range, weapon level and element, armor DEF, and refine bonuses from `refine_db.json`;
- static bonuses of item data, and the bonuses of Lua scripts whose `when` condition holds, with computed values run at that moment;
- the stacking options of 0002a, then a port of `pc_bonus` by bonus name.

The zone computes stats again after an equip or unequip, a level up, a raised stat, and a bonus that starts or ends. A calculation with ten items and 30 bonuses takes 5.4 µs (`BenchmarkCalcEquipped`).

The player's look follows its equipment: the weapon type, the shield's view, the views of its headgear and garment, and costumes over them. Viewers get `LookChange`.

Items with an `item_noequip.txt` restriction keep their DEF and ATK, but their scripts do not run: every map is a normal map until PvP and GvG maps exist.

To try it, give a new character a Knife, a Hat, a Cotton Shirt with a card slot and a Poring Card, and open the equipment window (Alt+Q) in the web client:

```text
go run ./server/cmd/waserver --assets /path/to/build --map prt_fild08 --dev --build melee \
  --start-items 1201:1,2220:1,2302:1,4001:1
```

Equipment is saved with the inventory: the `equip`, `refine` and `cards` columns of migration 00008. Cards are a list, and the database allows up to 16 cards and refine 255, so a ruleset can raise the limits without a migration.

The ruleset's `aspd` and `size_table` flags pick the ASPD formula and table, and the size table of knuckles and whips (strategy section 3.6). A flag set to the other mode loads that mode's job table. `cast_time` picks how cast bonuses count, `fourth_job_traits` whether trait stats count, and `stat_point_cost` the cost of raising a stat ("Rulesets" above).

Parity with rAthena is in `tools/convert/testdata/equipment_parity_<mode>.md`, made by `go test ./tools/convert -run TestEquipmentParity`:

| Mode | Equipment items | Cards | Refine levels |
|---|---|---|---|
| renewal | 9,993 of 9,993 compared, stats and damage | 1,502 of 1,502, stats and damage | 105 of 105, stats and damage |
| pre-renewal | 1,875 of 1,875, stats | 431 of 431, stats | 55 of 55, stats |

Compared items are those whose scripts convert fully: 176 renewal items and 43 cards, and 10 pre-renewal items and 21 cards, are left out. Pre-renewal damage is not compared, since `battle` has renewal's formula only.

## Status effects

Players, monsters and NPCs have status effects, as rAthena's `status_change` ([decision 0010](../docs/decisions/0010-status-effects.md)). The package `statuschange` ports `status_change_start`, `status_change_end`, `status_change_timer` and `status_get_sc_def`, with the data of rAthena's `status.yml` for the mode (the `status.json` extract).

- Starting a status checks immunities, the `Fail` list, boss resistance, and resistance from VIT, INT, LUK, level and `bResEff`, then rolls the chance on the zone's random stream. `start_status` options and `sc_start` flags skip parts of it, as rAthena's `SCSTART_` flags.
- Each status lasts its duration and ends with the reason `"expired"`. Poison, bleeding, stone and the other statuses that tick do their work on rAthena's intervals.
- Statuses change stats: the zone computes stats again when a status starts or ends, with the `status_calc_*` changes of rAthena and the converted status scripts (such as `bonus bStr` of SC_INCSTR). Monsters' stats follow their statuses too. A script that starts or ends a status reads the new stats in the same call, as in rAthena.
- The common ailments work in full: stone, freeze, stun, sleep, poison, deadly poison, curse, silence, confusion, blind and bleeding. A stunned, frozen, stoned or sleeping unit cannot walk, attack or use items, and its HP regen follows rAthena. Damage ends sleep, freeze and stone. Confusion walks to random cells.
- Death ends the statuses without `NoRemoveOnDead`, and logout ends those with `NoSave`.
- Procs: `cause_status` bonuses on equipment and timed bonuses roll on each normal attack that lands, with the target's resistance and rAthena's 100 ms delay. `on = "hit_taken"` rolls on the attacker.
- The player sees a status icon with the time left (`StatusIcon`). Everyone near sees the status options: stone, freeze, stun, poison, curse, hiding and the rest (`OptionChange`, and the `options` of `EntityAppear`).

Statuses are saved with the character, as `chrif_save_scdata`: every status without `NoSave`, with its values and the time to its next timer. Login restores them with the reason `"login"`. Timed bonuses (`player:add_timed_bonuses`, rAthena's `bonus_script`) are saved too, unless `ends_on` has `logout`. A second use of a key keeps the first copy by default, as `bonus_script` does, and the option `on_repeat` can extend it, add a copy, or replace it. Migration 00009 adds `character_statuses` and `character_timed_bonuses`, and migration 00010 drops their `stack` column.

To try them, give a new character STR Dishes, Green Potions and Concentration Potions:

```text
go run ./server/cmd/waserver --assets /path/to/build --map prt_fild08 --dev \
  --start-items 12041:3,506:3,645:3
```

Parity with rAthena's `status_effects.json` is in `server/statuschange/testdata/status_effects_parity.md`, made by `go test ./server/statuschange -run TestStatusEffectParity -update`:

| Mode | Cases | Chance | Duration | Starts | All match |
|---|---:|---:|---:|---:|---:|
| renewal | 2,640 | 2,640 | 2,640 | 2,640 | 2,640 |
| pre-renewal | 2,640 | 2,640 | 2,640 | 2,640 | 2,640 |

Left: the effects of statuses that only skills start. They start, save, end, and show their icon and option, but do nothing else until their skills exist.

## Skills

Players learn and cast skills, as rAthena's ([decision 0011](../docs/decisions/0011-skills.md)). The package `skill` holds the data of rAthena's `skill_db.yml` and `skill_tree.yml` for the mode (the `skills.json` and `skill_trees.json` extracts), with the overlays of mods, and the formulas of casts and damage. The zone runs the casts.

- **Skill points and the tree.** Each job level gives a skill point. The skill window lists the skills of the job's tree, as `pc_calc_skilltree`: a first class job sees the Novice tree until it spent 9 points there (`player_skillup_limit`), and quest skills stay hidden. `LearnSkill` raises a skill by one level when its required skills allow it, as `pc_skillup`. Learned skills are saved in `character_skills` (migration 00011). Passive skills change stats at once.
- **Casting** follows `unit_skilluse_id2` and `unit_skilluse_pos2`:
  - The cast starts after the checks of `skill_check_condition_castbegin`: SP, zeny, weapon, state, statuses and weight. A target out of range makes the player walk into range first. Range is circular for players, as rAthena's client distance.
  - Cast time follows `skill_castfix` in pre-renewal and `skill_vfcastfix` in renewal, with DEX, INT in renewal, Suffragium, Memorize and the cast bonuses of items. Everyone near sees the cast bar.
  - Only a hit of at least 1 damage stops a cast, when the skill's `CastCancel` allows it. Phen Card's bonus keeps the cast going.
  - When the cast ends, the target is checked (enemy, friend, line of sight), then HP, the weapon, ammunition and items. HP, SP and zeny are paid before the skill acts, and items after it, as `skill_consume_requirement`.
  - The after-cast delay (`skill_delayfix`), the cooldown and the walk delay start. A failed cast lets the player act again at once.
- **Damage** goes through the same code as normal attacks (`battle_calc_weapon_attack`, `battle_calc_magic_attack`, `battle_calc_misc_attack`), in both modes, with each skill's ratio and hit rate. Splash skills hit every enemy around their target, with Napalm Beat's split and Fire Ball's edge. Knockback follows `skill_blown`.
- **Ground skills** place units that viewers see: Safety Wall and Pneuma give their status to whoever stands on them and block melee or ranged hits; Fire Wall hits and pushes what stands on it every 20 ms; Thunder Storm hits once. A timer every 100 ms runs them, as `skill_unit_timer`. `ActiveInstance` limits how many a player may have up.
- **Statuses** that skills start work with their stat effects: Provoke, Endure, Auto Berserk, Magnum Break's fire bonus, Sight, Energy Coat, Stone Curse, Frost Diver's freeze, Ruwach, Increase and Decrease AGI, Signum Crucis, Angelus, Blessing, Improve Concentration, Crazy Uproar, Hiding, Envenom's poison, Sand Attack's blind and Trick Dead.
- **Procs**: `cause_status` bonuses with the `skill` option roll on hits of that skill, and the `cast` bonuses (autospells) cast on attack, when hit, and on the use of a skill, as `skill_additional_effect`, `skill_counter_additional_effect` and `skill_onskillusage`.
- **Entity queries** find units in an area for skills and scripts alike (`query.go`): a radius, a rectangle, a line or a list of cells, with filters and an order.
- **Mods** add skills with `data/skills.yml`, `data/skill_trees.yml` and a Lua script that runs when the skill takes effect, and change core skills the same way ([mods/README.md](../mods/README.md)). `mods/examples/chain-lightning` adds a Mage skill that jumps between enemies.

- **Second and transcendent classes** have their skills too, in both modes: every 2-1 and 2-2 class, the Bard and Dancer, their transcendent classes, and the Super Novice, whose skills are all first class skills. Each job's formulas are in `skill/<job>.go` and its behaviors in `zone/skills_<job>.go`. The systems they brought:
  - Peco Peco riding, the falcon and the cart, saved in the option bits (`pc_setriding`, `pc_setfalcon`, `pc_setcart`).
  - Spirit spheres, combos (`skill_combo`) and Blade Stop.
  - Timer skills (`skill_addtimerskill`), for Double Bolt, Palm Strike and the delayed hits of other skills.
  - Traps, songs, dances and ensembles, element fields, Land Protector and the other ground units, with their overlap rules (`skill_cell_overlap`, `skill_dance_overlap`).
  - Plagiarism, the strip skills, breaking equipment (`skill_break_equip`), thrown potions, plant summons, Guard, Shield Reflect and `pc_checkallowskill`.
  - Statuses that only recalculate the stats they flag, as `status_calc_bl`: a status without `SCB_ASPD` leaves ASPD as it was.

Skills that need something the server or the client does not have yet fail before they start, so they cost nothing:

- A window: Warp Portal, Item Appraisal, Vending, Arrow Crafting, Change Cart, Decorate Cart, Open Buying Store, Repair Weapon, Weapon Refine, Hindsight, Abracadabra, Create Elemental Converter and Prepare Potion.
- A homunculus: Call Homunculus, Vaporize and Resurrect Homunculus.
- Call Baby, which needs a family.

Teleport warps to a random cell at both levels, since it has no menu. Party skills reach only the caster, and Devotion and Redemptio fail, as rAthena does for a player without a party. Ensembles need a partner in the same party, so they fail before they start.

To try them, start new characters as a job with skill points, such as a Swordman at job level 40, then change to a Knight at the Job Master (see "Job changes"):

```text
go run ./server/cmd/waserver --assets /path/to/build --map prt_fild08 --dev --start-job 1:50:40
```

Parity with rAthena, from `go test ./server/skill` and `go test ./server/zone -run TestSkillStatusGolden`:

| Mode | `skill_damage.json` | `skill_casting.json` | `skill_status.json` |
|---|---:|---:|---:|
| renewal | 1,879 of 1,879 cases, 92 skills | 6,078 of 6,078 | 621 of 621 |
| pre-renewal | 1,864 of 1,864 cases, 91 skills | 6,096 of 6,096 | 512 of 512 |

- `skill_casting.json` compares cast time, after-cast delay, cooldown, requirements, and heals, for every first, second and transcendent class skill at every level.
- `skill_status.json` compares the statuses that a skill starts, with their values and timers, the target's stats before and after, and a normal attack before and after.
- Three skills have their own golden tests instead of the shared ones: Spiral Pierce (its recorded weapon has a script bonus), Shield Chain (one recorded hit count is a statistical outlier) and Eternal Chaos (the recorder's Golem stood in the song).
- The damage tests leave out the Gunslinger, Ninja, Taekwon and Soul Linker skills, which come later.

Left: parties for party skills, ensembles and Devotion, homunculi, the windows listed above, cart items, sitting (Tension Relax, Gangster's Paradise), monster AI for plant summons, and saving broken equipment.

## Job changes

Players change jobs through scripts, with the calls of proposal 0002, review decision 21 (`jobchange.go`, and [mods/README.md](../mods/README.md), "Job changes and resets"):

- `player:change_job` ports `pc_jobchange`: job level 1 and job exp 0, the transcendent status points, a base level cap with a stat reset, equipment the new job cannot wear comes off, and a companion, mount or cart goes when the new job does not meet its `requires`. A male Dancer becomes a Bard. Without `force`, only the next jobs of `jobs.next_jobs` are allowed, with the levels of the job's `changes_from`, and a transcendent player stays in its line. The `job_change` event follows.
- `player:rebirth` makes a second class at 99/50 a High Novice at 1/1 with 100 status points and the platinum skills, as the rebirth NPC (`npc/jobs/valkyrie.txt`).
- `player:reset_skills`, `player:reset_stats`, `player:reset_skill` and `player:reset_stat` refund skill and status points as `pc_resetskill` and `pc_resetstate`, and by rAthena's stat cost table. Quest skills stay.
- Migration 00012 saves the job level of the change to a second class (rAthena's `change_level_2nd`), which the skill tree reads, the second class before a rebirth, which picks the transcendent line, and the saved option bits of a cart, a falcon and a Peco Peco. A rider shows its mounted class, as `status_set_viewdata`.
- The progress stats carry `STAT_ID_JOB`, and `JobName` carries the job's `display_name` after login and after each change. The client shows the name with the levels.

### Job records

Jobs are records ([decision 0015](../docs/decisions/0015-job-definitions.md), and [mods/README.md](../mods/README.md), "Jobs"):

- `gamedata.CoreJobs` converts each mode's `job_stats.json`, the exp tables of `progression.json` and the `Inherit` lists of `skill_trees.json` into core's records, with the names of `wa.jobs`. `status.CoreJobData` adds what follows from the map job IDs: the tier, `base_job`, and `changes_from` from rAthena's job change rules.
- `gamedata.LoadMods` applies the mods' `data/jobs.yml` over them. The job tables (`status.Tables.WithJobs`), the exp tables (`progression.Tables.WithJobs`) and the skill trees of new jobs come from the records.
- A mod job gets an ID from 10,000 and a map job ID made from its look's class with its own tier, transcendent and baby bits. `pc_mapid2jobid` keeps core's jobs only, so a mod job keeps its own bonuses, weight and maximum stats, and its own skill tree.
- The body of a player's look is the look job's, and the client names the job from `JobName`.
- `TestCoreJobRecordsMatchRAthena` checks that the tables built from core's records match rAthena's for every job: 3,150 stat calculations of 175 jobs in renewal and 1,368 of 76 jobs in pre-renewal, with the maximum stats, exp tables and next jobs.

Core places a Job Master in Prontera, since rAthena's job change quests mostly do not convert. It follows rAthena's level rules: job level 10 with Basic Skill 9 for a first class (base level 45 too for a Super Novice), job level 40 for a second class, and 99/50 for a rebirth.

To try it, start a Swordman at job level 40 and talk to the Job Master at Prontera 151,191:

```text
go run ./server/cmd/waserver --assets /path/to/build --map prontera --dev --start-job 1:50:40
```

### Companions, mounts and carts

Players keep a companion, a mount and a cart, with the calls of proposal 0002, review decision 25 ([decision 0016](../docs/decisions/0016-companions-and-mounts.md), `companions.go`, and [mods/README.md](../mods/README.md), "Companions, mounts and carts"):

- Core's records are `server/companion/core/data/companions.yml` and `mounts.yml`, which the core mod carries. `gamedata.LoadMods` merges the mods' records over them. Mod records get IDs from 10,000.
- A record's `acts_as` picks the rules it follows, and its `look` the option bits that show it, as rAthena's `OPTION_FALCON`, `OPTION_RIDING`, `OPTION_DRAGON1` to `OPTION_DRAGON5`, `OPTION_WUGRIDER` and `OPTION_MADOGEAR`. `pc_isfalcon`, `pc_isriding` and the other checks read the rules, so a mod's record that acts as the falcon or the Peco Peco works with their skills.
- `status.Calc` ports the mounted rules of `status_calc_pc` and `status_calc_speed` for the Peco Peco, the dragon, the warg and the Madogear: walk speed, the ASPD rule with Cavalier Mastery or Dragon Training, the weight limit, spears against medium targets, Spear Mastery, and the Madogear's speed, weight and Mainframe Restructure DEF. A mount record's `speed` replaces the walk speed bonus. Record `bonuses` run after the timed bonuses.
- A warg rider uses only skills marked `AllowOnWarg` and does not attack, and a Madogear only skills marked `AllowOnMado`, as `skill_check_condition_castbegin` and `unit_attack`.
- Migration 00014 saves the record names beside the option bits of migration 00012. A character saved before it gets core's records from the bits. A name whose record is gone, such as one of a removed mod, is dropped with a warning when the character loads.
- `TestMountParity` checks the stats, weight limit, option bits and a normal attack of mounted players against `mount_status.json`: 74 of 74 cases in renewal (Knight and Crusader lines, babies, Rune Knight on a Peco Peco and a dragon, Royal Guard, Ranger on a warg, Mechanic on a Madogear) and 48 of 48 in pre-renewal.

## Monster AI and monster skills

Monsters think as rAthena's `mob.cpp` ([decision 0013](../docs/decisions/0013-monster-ai.md)). The package `mob` holds what needs no map: the data of `monsters.json`, the stats of `status_calc_mob`, the AI types and states, and the skills lists. The zone runs the AI:

- **Stats** come from the record: hit, flee, defense and magic defense from stats, attack ranges and magic attack, in both modes, as `status_calc_misc`.
- **The hard AI** (`mob_ai_sub_hard`) runs every 100 ms for each monster within a player's view plus 4 cells. Other monsters run the lazy AI every second: they walk at random and use idle skills.
- **Targets.** An aggressive monster looks for a target in its view range, and walks to it if a path is no longer than its chase range. A monster gives up when the target leaves its chase range, hides, or dies. Monsters with the `assist` mode join monsters of their kind that fight, looters pick up items, and the change-target modes, angry monsters and the cast sensor follow rAthena's rules.
- **Hits.** The AI learns of a hit once the damage shows and the hit's walk delay passes, as the `mob_attacked` timer. A monster that can attack strikes back.
- **Skills.** Every 1 s, or on an event (a hit, a cast on it, its spawn), a monster checks its skills list as `mobskill_use` and `mobskill_event`: state, condition, rate and delay, then the target. Casts go through the skill framework with the entry's cast time. Lines of `mob_chat_db.yml` show as the monster's chat, in the entry's `chat_color`.
- **`NPC_` skills** that only monsters use are in `skills_npc*.go`: summons and slaves, class changes, emotions, random moves, rebirth, and the attack, status and buff skills.
- **Emotions and modes.** An entry's `emotion` is one emotion or a list of two, shown in turn. The `modes` of an `NPC_EMOTION` or `NPC_EMOTION_ON` entry change the monster's mode through `SC_MODECHANGE`, as `status_calc_mode`: `NPC_EMOTION` sets the AI modes, or ends the change with `modes: []`, and `NPC_EMOTION_ON` adds modes.
- **MVPs.** The player that did the most damage gets the MVP exp and drops, with the MVP effect.

The `monster_target` hook, the methods of monster handles and the AI scripts of monster records are the Lua API of proposal 0002, review decision 22 ([mods/README.md](../mods/README.md), "Monster AI").

Parity with rAthena, from `go test ./server/mob` and `go test ./server/zone -run 'TestMobAIParity|TestMobSkillUseParity'`:

| Mode | `mob_status.json` | `mob_ai.json` | `mob_skill_use.json`, entry chosen | `mob_skill_use.json`, effects |
|---|---:|---:|---:|---:|
| renewal | 2,675 of 2,675 | 2,814 of 2,814 | 22,189 of 22,189 | 18,153 of 18,232 |
| pre-renewal | 1,004 of 1,004 | 2,814 of 2,814 | 8,594 of 8,594 | 7,205 of 7,213 |

- `mob_ai.json` checks single decisions of every AI type: target search, hits, target changes, assists, loot, chase and the cast sensor.
- The chosen entry includes its target and cast time. The effects (damage, statuses, moves, summons, class changes, mode changes) are checked in every case where the monster used a skill. The rest are a few player skills that monsters use, such as `ITM_TOMAHAWK` and `GS_DUST`.

To see it, start on `prt_fild03`, where Chocos (AI type 19) attack on sight and use their skills, and Yoyos (AI type 07) loot and help each other:

```text
go run ./server/cmd/waserver --assets /path/to/build --map prt_fild03 --dev
```

Left: MVP tombs, monster level ups, guardians, monsters that follow a player through a warp, and the `NPC_` skills that `docs/generated/monster-skills-<mode>.md` lists without a port.

## Rolled item values

Items can roll values when a copy is made, and rAthena's random options roll on the same storage ([decision 0014](../docs/decisions/0014-rolled-items.md), strategy section 5.4). The package `item` holds the rolls and their distributions (`roll.go`) and the port of `s_random_opt_group::apply` (`random.go`). The zone rolls them (`rolls.go`):

- **Drops.** Each item a monster drops, and each MVP prize, rolls its values from the zone's secret random stream, through the mods' `item_roll` hooks. A drop with a random option group rolls its options, as `mob_setdropitem_option`. Monsters that loot keep the rolls of what they pick up, and drop them when they die.
- **The `item_rolls` flag.** Off, the default of core's rulesets, every roll is its default, as rAthena. Random options roll either way.
- **Gives and shops.** `player:give_item` uses the defaults unless the script asks for `"random"` or sets values. NPC shops sell copies at their defaults. `player:give_item_from_group` rolls an equipment entry's random options, as `getgroupitem`.
- **Stats.** Static bonuses read the copy's rolls (`{ roll: name }`), and a card's bonuses the card's rolls. Random options apply after every card, as `status_calc_pc_sub`. `equipped:set_roll` and `equipped:reroll` start the copy's equip scripts again once the script's call returns.
- **Stacking.** A stack is the item plus its rolls and options. Unidentified copies with hidden rolls never stack. An item picked up from the ground gets a new serial, and its line in the item log names the serial of the stack it was dropped from (see "Transfers and item logs").
- **Messages.** `InventoryItem`, `InventoryAdd` and `ItemRollsChange` carry each roll with its label, range and unit, each card's rolls, and the random options with their display names. A hidden roll of an unidentified copy goes without its value. The web client shows them in the item's description, such as "LUK +2 (1 to 5)" and "MaxHP +51".
- **Storage.** Migration 00013 adds the `rolls`, `card_rolls` and `random_options` JSONB columns and `identified` to `inventory`. Options are saved by name. When a character enters a zone, values outside a roll's range are clamped, and rolls and options the data no longer has are dropped. The server logs each change as `item rolls fixed on load`.

Core's random options come from the extract `random_options.json` (reference/rathena/README.md, "Random options"). In renewal, 239 of 249 options and all 108 groups convert ([docs/generated/random-options-renewal.md](../docs/generated/random-options-renewal.md)). Pre-renewal has none, as rAthena.

Parity with rAthena's `random_option_rolls.json`, from `go test ./server/gamedata -run TestRandomOptionParity`:

| Mode | Groups | Options with every draw at its lowest | Seeded statistics (100,000 rolls per group) | Monster drops with a group |
|---|---:|---:|---:|---:|
| renewal | 108 | 108 of 108 groups | 3,363 of 3,363 option cells within 6 standard errors | 179 of 179 |
| pre-renewal | 0 | none | none | none |

The roll distributions are checked by `go test ./server/item -run TestRollDistributions`: 100,000 draws of each distribution stay in their range and pass a chi-square test.

To try it, load the sample mod [examples/lucky-clover](../mods/examples/lucky-clover) with a ruleset mod that turns `item_rolls` on ([mods/README.md](../mods/README.md), "Rulesets"), kill Clover Porings on `prt_fild09`, pick up a charm, and select it in the inventory window (Alt+E). Its description shows its rolls and its random option.

## Transfers and item logs

Every change to a player's items and zeny goes through the package `transfer` ([decision 0017](../docs/decisions/0017-e2e-and-transfers.md), strategy section 3.5):

- **Moves.** A transfer lists moves: an item from one holder to another, an item from an end into a holder (a pick-up, a purchase, a script's `give_item`, a skill's product), an item from a holder to an end (a drop, a sale, a script's `take_item`, a use), and zeny between wallets and ends. Holders are inventories, storages, carts and guild storages now, and mail and vending for the steps that add them.
- **Checks.** Every move runs first on copies of the holders: the slot must hold the serial the move names, the amount must be there, and the item must fit the weight and slot limits and the stack rules of strategy section 5.4. When one move fails, nothing changes.
- **Serials.** A whole stack that moves keeps its serial. Part of a stack gets a new serial. An amount that joins a stack takes that stack's serial. A new copy from an end gets a new serial. A ground item keeps the serial of the stack it was dropped from as its origin, and the copy picked up names it.
- **Messages.** The player sees each change in the order of the moves, with the same messages as before transfers existed.
- **Paths.** Drops, pick-ups, monster items given to the inventory (pet eggs, MVP prizes, Steal), NPC shops, `give_item`, `give_item_from_group`, `take_item`, `pay_zeny`, `give_zeny`, item use, the items and zeny that skills make or cost, storage and cart moves, trades, and bound items that go away with their bond. Card insertion does not use transfers yet.

With a database, the zone hands the characters a transfer changed, with its log lines, to the store after each event it handles. The store writes each such commit in one transaction:

- It locks the characters' rows in ID order, saves each character with its save version, and writes the lines to `item_log` and `zeny_log` (migration 00015).
- Commits are written one at a time, in the order they came. A drop's commit is written before the pick-up of the dropped item, so the database never holds an item in two inventories, even after a crash.
- A character's saves that came before its commit are dropped, since the commit's copy is newer. Its saves that came after wait for the commit.
- A commit that waits takes in the commits that come after it, so commits batch when the database is slow.

Each log line has the time, the action (`drop`, `pick_up`, `loot`, `buy`, `sell`, `give`, `take`, `use`, `skill`, `move`, `guild`, `storage`, `cart`, `trade` or `bound`), the owners it came from and went to, with the extra storage in `from_storage` and `to_storage`, the copy's `bound`, the serial it ended in, the serial it came from when that differs, the item, the amount, and the refine, cards, rolls and random options. Without a database, the lines go to the log at debug level.

Both logs are partitioned by month. `ensure_log_partitions` makes the partitions of this month and the next, and `waserver` calls it at start and every 12 hours. Dropping months older than the 180 days of strategy section 7.10 is left to a later step.

To find where a serial went, and where it came from:

```sql
SELECT at, action, from_kind, from_id, to_kind, to_id, amount, source
FROM item_log WHERE serial = '0192f0c4-...' OR source = '0192f0c4-...' ORDER BY at;
```

## Chat and moderation

Chat follows strategy section 7.10 and proposal 0002, review decision 26 ([decision 0018](../docs/decisions/0018-chat-and-moderation.md)). The package `chat` holds what every zone of the world shares: the chat filter, the channels, and the hub, which finds online players by name.

- **Local chat** reaches the players in range, as before.
- **Whispers** reach a player on any map of the process. The sender gets `WhisperResult`: sent, not online, or blocked, as rAthena's `clif_wis_end`. A whisper to `#name` speaks in that channel, and joins it first when anyone may join, as rAthena.
- **Channels** are core's records of `data/channels.yml` (rAthena's `#global`, `#support`, `#trade` and `#map`) and the channels mods make. `ChannelCommand` joins, leaves, lists channels and lists members. A channel's lines carry its name and color. `#map` members hear the members on their own map, and leave it on a map change.
- **Blocking.** `Block` adds a name to the character's block list, up to the ruleset's `block_list` limit (20 by default, rAthena's `MAX_IGNORE_LIST`), or removes it. A blocked name cannot whisper to the player, and the player does not see its local, channel or party lines. The list is saved with the character, in `character_blocks`. Party invites, friend requests and trade requests check it.
- **The chat filter** of `data/chat_filter.yml` replaces or rejects words in every line, and refuses new character names that have a word or a reserved name, on the account pages and for `--dev` logins.
- **Rate limits** are the `chat.*` settings, per connection. A character made less than `chat.new_character_period` ago has the lower limits. Lines over the rate are dropped, and the player gets `ChatRefused` with `TOO_FAST`, since the client does not know a new character's limits. A flood ends the connection, as before.
- **Mutes and jails** come from mods with `player.sanction` for now, and from the staff tools later. A muted player's lines come back as `ChatRefused` with a `SanctionNotice` that has the reason and the end time. A jailed player goes to the jail map, cannot use warps, scripts' warps or wings, comes back in jail after death, and goes back to where it was when the jail ends. Sanctions run on the server clock and are saved in `sanctions`, which keeps ended ones too.
- **Reports.** `Report` reports a player, or a line by its `line_id`. The report keeps the lines the reporter saw in the last 5 minutes. A player may send 10 reports an hour.
- **The chat log** keeps every line players say: local, whisper and channel. With a database, `chat_log` keeps lines for `chat.log_days` days, and `report_lines` ties a report to its lines, which stay while the report is open. A worker writes lines and reports in batches, in order, and deletes old lines every 12 hours. Without a database, the last 10,000 lines and 1,000 reports stay in memory.

With `--dev`, the debug address serves two more endpoints for development and the end-to-end tests:

- `GET /debug/wathena/reports` lists the newest reports with their lines.
- `POST /debug/wathena/clock?advance=90s` moves the server clock forward, which mutes, jails, schedules and world events in zones follow.

Migration 00016 adds `character_blocks`, `sanctions`, `chat_log`, `reports` and `report_lines`. To read the open reports:

```sql
SELECT r.id, r.reporter_name, r.target_name, r.reason, c.at, c.name, c.text
FROM reports r JOIN report_lines rl ON rl.report_id = r.id JOIN chat_log c ON c.id = rl.line_id
WHERE r.status = 'open' ORDER BY r.id, c.at;
```

## Parties and friends

Parties, friends and party booking follow proposal 0002, review decision 27 ([decision 0020](../docs/decisions/0020-parties-and-friends.md)). The package `social` holds one registry per world, which every zone shares, as the chat hub.

- **Parties** follow rAthena's `party.cpp` and the char server's `int_party.cpp`. `PartyCommand` creates a party, invites by name, answers an invite, leaves, expels, hands over the lead and sets the options. The player who creates a party gets `party_join` with the reason `create` (review decision 32). Only the leader invites, expels, hands over the lead and sets the options. The party breaks up when its leader leaves, as rAthena does.
- **Members** sit in slots, as `party.member[MAX_PARTY]`: a new member takes the first free slot. A party holds up to the ruleset's `party_members` (12 by default). With `block_account_in_same_party`, two characters of one account cannot be in one party.
- **The party window.** `PartyInfo` lists the members with their online state, map, job, level, HP and cell after every change. Members on the same map get `PartyMemberUpdate` when HP changes, and the cell once a second, as `clif_party_hp` and `party_send_xy_timer`.
- **Party chat** is `Chat` with `CHAT_CHANNEL_PARTY`. It passes the mute, the filter and the hooks, reaches every online member on any map, and is logged with the kind `party`.
- **Exp share.** With even exp share, `mob_dead` puts each fighting member's share in the party's pot, and `party_exp_share` splits it among the members on the map who are alive, with `party_even_share_bonus`. The level penalty applies to each member after the split when `level_penalty_exp` is on. Even share needs the online members' base levels within `party_share_level` (15 by default): asking for it outside the range is refused, and a login, logout or level up that breaks the range turns it off, as the char server does.
- **Item share.** With shared pick-up (`item & 1`), a member may take what its party's top attackers may take. With even item share (`item & 2`), a picked up item goes to a member on the map who is alive, at random or in turn by `party_item_share_type`, as `party_share_loot`. Items go through transfers, so the item log names the member who got the item.
- **Friends** follow `clif.cpp`'s friend packets. `FriendCommand` asks an online player, answers a request, or removes a friend from both lists. A list holds up to the ruleset's `friends` (40 by default). Friends see each other log in and out (`FriendStatus`).
- **Party booking.** `PartyBookingCommand` places an ad (level, map, up to 6 jobs and a note), searches ads as `party_booking_search`, and deletes the player's ad. Ads live in memory and end at logout.
- **Party skills** reach the caster's party members, as rAthena does ([decision 0020](../docs/decisions/0020-parties-and-friends.md) lists them).

With a database, every party loads at start, and a character's friend list loads with it. A writer saves each change on its own goroutine in the order it came. Migration 00017 adds `parties`, `party_members` and `character_friends`, allows 100 blocked names, and adds the `party` kind to `chat_log`. Without a database, parties and friend lists last while the server runs.

## Guilds

Guilds follow proposal 0002, review decision 28 ([decision 0021](../docs/decisions/0021-guilds.md)). The social registry of "Parties and friends" holds them too.

- **Guilds** follow rAthena's `guild.cpp` and the char server's `int_guild.cpp`. `GuildCommand` makes a guild (with an Emperium by `guild_emperium_check`), invites by name, answers an invite, leaves with a reason, expels, changes positions, gives a member a position or the master's role, changes the notice, raises a guild skill, asks for an alliance or answers one, makes an enemy, ends a relation, and breaks the guild up. The master cannot leave, and breaks the guild up only as its last member. The player who makes a guild gets `guild_join` with the reason `create` (review decision 32).
- **Members** sit in slots with the master in slot 0. A guild holds `guild_members` (16) and 6 more for each level of Guild Extension. A new member takes the last of `guild_positions` positions. A position names who may invite, expel and use the guild storage, and its exp tax up to `guild_exp_limit` (50). Invites respect block lists, as party invites.
- **The guild window.** `GuildInfo` holds the members, positions, notice, skills, relations, level and exp after every change. Members on the same map get each other's cells every 5 seconds, as `guild_send_xy_timer`. Others see a member's guild, position and emblem through `EntityAppear.guild` and `EntityGuild`.
- **Guild chat** is `Chat` with `CHAT_CHANNEL_GUILD`, as party chat, logged with the kind `guild`.
- **Guild exp.** A member pays its position's tax of base exp at each gain, as `pc_gainexp`'s `guild_payexp`. The guild levels up by `exp_guild.yml` of the world's base and gets a skill point per level, as `guild_calcinfo`. The registry adds the tax at once, where rAthena's map server sends it every 10 seconds.
- **Guild skills.** The master raises them by `guild_skill_tree.yml`. Battle Orders, Regeneration and Restore work on GvG maps only, and Emergency Call where `emergency_call` allows it, which is GvG maps by default. No map is a GvG map until sieges come. Renewal gives each its own cooldown; pre-renewal blocks all four, also after a master logs in (`guild_skill_relog_delay`).
- **Alliances and enemies.** Alliances need the other guild's answer and are on both sides; enemies are on one side. `guild_allies` and `guild_enemies` limit them. `battle_check_target` counts guildmates for `BCT_GUILD`, so GuildOnly skills reach them.
- **Guild storage** opens from core's Guild Warehouse Manager in renewal towns, for one member at a time whose position allows it. Renewal needs the Guild Storage skill: 100 slots and 100 per level, up to the max of `guild_storage` (600). It also opens from `player:open_guild_storage()`, which replaced the core-only `wa.internal` call. Moves go through transfers, and the zone commits the storage with the inventory in one transaction. `StorageCommand.log` shows the last 100 moves, which load from the item log.
- **Emblems.** A master uploads a 24 x 24 BMP, GIF or PNG of at most 50,000 bytes on the account pages (`/account/guild/emblem`). `server/emblem` decodes it, makes magenta transparent, and keeps only its own PNG, which the server serves at `/emblems/<guild>/<version>.png` with a long cache and CORS. Players report a guild's name or emblem with `Report.guild_id`; the report keeps a copy of the emblem.

With a database, every guild loads at start with its members, relations, emblem, storage and storage log. Guild writes run on the transfer commit lane, so a guild's row exists before a commit writes its storage. Migration 00018 adds `guilds`, `guild_members`, `guild_relations`, `guild_emblems` and `guild_storage`, the guild columns of `reports`, and the `guild` kind of `chat_log`. Without a database, guilds last while the server runs.

With `--dev`, `POST /debug/wathena/emblem?guild=<name>` sets an emblem from the request body, and `POST /debug/wathena/guild-storage?name=<character>` opens a player's guild storage, since the development server has no account pages and its maps have no Guild Warehouse Manager.

## Storage, cart and trade

Storage, carts, item restrictions, bound items and trades follow proposal 0002, review decision 29 ([decision 0023](../docs/decisions/0023-storage-cart-trade.md), mods/README.md, "Storage, cart and trade").

- **Storages.** The package `storage` holds the storages of the characters online in one registry per process: an account's storage, the extra storages of `data/storages.yml` (account or character owned), and each character's cart. A storage loads at login, on the loading goroutine, and leaves memory after the last character that uses it logged out and its last commit was queued. A later load waits for that commit. Two characters of one account share one storage object, one opener at a time.
- **Moves.** `StorageCommand` puts and takes items in the open storage (personal or guild), puts and takes them in the cart, and moves them between the cart and the open storage, as rAthena's `storage.cpp` and `pc.cpp`. Every move goes through transfers, and the zone commits the storage's slots as a diff with the characters, in one transaction. The cart is open while the player has one; its weight limit is the rule `max_cart_weight` (8,000).
- **Restrictions.** Item records have `restrictions` from rAthena's `item_trade` (the extract `item_trade.json`), checked on the item and its cards: drops, trades, sales, mail, cart, storage and guild storage.
- **Bound items.** A copy's `bound` (account, guild, party or character) is saved in every item table, keeps bound and unbound copies apart, and follows rAthena's rules for drops, sales, trades, storage, guild storage and the cart. A player who leaves its party loses its party-bound items. One who leaves its guild puts its guild-bound items into the guild storage.
- **Trade.** `TradeCommand` asks a player within 2 cells, answers, offers up to 10 items and zeny, locks and confirms, as rAthena's `trade.cpp`. A player who blocks the requester refuses at once, with the rule `block_list_refuses_trade` (on by default, review decision 32). The `trade` hook may refuse the trade. Every item and zeny moves in one transfer, and one commit holds both characters. A map change or a logout ends the trade.
- **Pages.** Storage, cart and inventory lists go out in pages of at most 100 slots and 32 KiB, 16 pages per tick, so the 64 KiB message limit and the connection's buffer do not cap them.
- **Limits.** `inventory_slots` (base 100, max 200), `storage_slots` (600) and `cart_slots` (100), with the engine ceilings of decision 0023. An inventory holds its usable slots, not the max. Migration 00019 adds `storage_items`, the `bound` columns, the storage columns of `item_log`, and raises the checks to the ceilings. Serials are checked at the end of each transaction.
- **Expanded limits.** `player:expand` changes a player's `inventory_slots`, `storage_slots` and `cart_slots` within each limit's base and max (review decision 32, [decision 0027](../docs/decisions/0027-rule-reading-and-limits.md)). The inventory's size saves in `characters.inventory_size`. The account storage's and the cart's sizes save in `storage_sizes` (migration 00022), in the commit of the storage, and load with the storage. A storage at the base has no row. Every read clamps the value to the range, so a ruleset change applies at once. The player gets `PlayerLimits` at login and after each change, and the client sizes its inventory, storage and cart windows from it.

## Mail, vending, shops and currencies

Mail (RODEX), vending with autotrade and store search, buying stores, the one shop type and currencies follow proposal 0002, review decision 30 ([decision 0024](../docs/decisions/0024-mail-vending-shops.md), mods/README.md, "Mail, vending, shops and currencies").

- **Currencies.** Records of `data/currencies.yml`, with core's `cash` and `kafra`. A character's own currencies save with it, and an account's with the account's storage, which its characters share. Every change goes through transfers, and `zeny_log` names the currency. The client gets `CurrencyUpdate`.
- **Shops.** One registry of the shops of `data/npcs.yml` serves every zone. Costs mix zeny, items and currencies. A stock that every player shares fills again daily, weekly or after a time, and saves in the purchase's commit (`shop_stock`).
- **Mail.** `MailCommand` opens the mailbox, reads, sends, takes and deletes mails, as rAthena's RODEX. The package `mail` holds the mailboxes of the characters online. A mail to an offline character goes to the database, found by name on another goroutine. The sender pays rAthena's fee. Expiry runs once a minute: a mail goes back to its sender after `mail_return_days`, and a returned mail or a script's mail is deleted after `mail_delete_days`.
- **Vending.** The Vending skill opens the setup window. `VendingCommand` opens a shop from cart slots, browses another player's shop and buys from it, as `vending.cpp`, with `vending_tax`. One transfer moves the zeny, the tax and the cart's items, and one commit holds both characters, the cart and the store's row.
- **Buying stores.** `player:open_buying_store` opens the setup, and `BuyingStoreCommand` opens the store, browses one and sells to it, as `buyingstore.cpp`.
- **Autotrade.** `player:autotrade()` closes the connection and keeps the store. Stores save in `stores` and `store_items`. With a database and `autotrade_restore`, `waserver` opens autotrade stores again at start, each owner logged in without a connection. A login of the character ends its autotrade first.
- **Store search.** The package `market` holds a copy of every open store of the world. `StoreSearchCommand` searches it with item, card, price, roll and random option filters, as `searchstore.cpp`. A store opens from a search only on the searcher's map.
- **Migration 00020** adds the currency tables, `shop_stock`, `mails`, `mail_items`, `stores`, `store_items`, `zeny_log.currency`, the mail counters of `characters`, and the `currency` kind of `mod_ids`.

## NPCs

`waserver` also converts rAthena's NPC scripts and shops when it starts, in the same step ([decision 0008](../docs/decisions/0008-converted-npcs.md), mods/README.md, "Core's NPCs"). It takes about 0.17 s for renewal.

- Clicking an NPC within 15 cells starts its `click` handler, or opens its shop. A player has one conversation at a time, and cannot act while it lasts.
- A conversation ends when its script ends, when the player cancels it, leaves, dies, or changes map, and after rAthena's timeouts: 60 seconds for next, menus and close, and 180 seconds for text and number input.
- Shops follow rAthena's buy and sell rules: Discount and Overcharge, weight, and zeny up to 2,147,483,647. A shop's costs may also take items and currencies, and an item may have a stock.
- Kafras save, teleport, heal, and open the account's storage and the guild storage, with rAthena's fee, the free ticket and the Basic Skill check. Pushcart rental says it is not available yet.
- The save point is kept in the `characters` table. A dead character is saved at its save point, and a player who dies goes back there.
- Character and account variables of `wa.variables` save with the character. Server variables save right after they change, and again when the server stops. Without a database they live in memory.

To try them, start in Prontera with some Jellopy to sell and a Butterfly Wing. The sample mods need their permissions approved, in a config file such as:

```yaml
# examples.yml
mods:
  dir: mods/examples
  approve:
    example.halloween: [player.give_items, player.give_zeny, world.announce, events.schedule, events.control, rules.override]
```

```text
go run ./server/cmd/waserver --config examples.yml --assets /path/to/build --maps prontera,prt_in,izlude --dev \
  --start-items 909:300,602:1
```

Stopping:

- On SIGTERM or Ctrl+C the server announces a countdown to every player, at 30, 10, 5, 3, 2 and 1 seconds, then saves and stops. A second signal stops it at once.
- `shutdown_countdown` (`--shutdown-countdown`) sets the countdown. The default is 10 seconds, the most is 10 minutes, and 0 turns it off.

## World events

`waserver` starts the world events of the mods' `data/events.yml` without staff action (strategy section 8.8, mods/README.md, "World events"). It logs each event at start with its trigger and next time:

```text
level=INFO msg="world event" event=example.halloween:Pumpkin_King trigger="daily at 20:00" minutes=60 next="2026-10-31 20:00 PST"
level=INFO msg="world event started" event=example.halloween:Pumpkin_King scheduled=... ends=...
```

- Times match in `time_zone`, an IANA name. The server keeps every time in UTC. On a day clocks change, a time that does not exist runs at the next valid minute, and a time that happens twice runs once. The binary carries the time zone database, so the distroless image needs none.
- A run starts in the zone of the event's map, or of the start map. An event whose map does not run is an error at start.
- Each run is written to the `world_event_runs` table before it starts, so a restart never runs the same time twice. Runs missed while the server was stopped are skipped, except a run whose window is still open at the start: it begins at once and ends at its usual time. Without a database, runs are kept in memory.
- Random triggers draw from the world's secret stream `events`, once at the start of each hour.
- Condition triggers run their Lua function once a minute, in the zone of the event's map.

`mods.events` moves or turns off an event, by its full name:

```yaml
time_zone: Asia/Manila
mods:
  events:
    example.halloween:Pumpkin_King:
      trigger: { weekly: { day: saturday, time: "21:00" } }
      duration_minutes: 30
    example.halloween:Pumpkin_Night:
      disabled: true
```

- `trigger` takes the forms of `data/events.yml`. `duration_minutes` is 1 to 10,080.
- An event that does not exist is an error, so a typo does not pass.
- For a quick test, set the trigger a minute or two ahead.

### Typed events and core's samples

Typed events (mods/README.md, "Typed events", [decision 0025](../docs/decisions/0025-automatic-events.md)) place a boss, an invasion or treasure chests. Core ships one of each, off by default. `enabled: true` turns one on, and may come with a new trigger or duration:

```yaml
mods:
  events:
    core:Field_Boss:         # a Prontera field boss on a field of levels 1 to 30
      enabled: true
      trigger: { daily: "21:00" }
    core:Prontera_Invasion:  # three waves on Prontera
      enabled: true
    core:Treasure_Hunt:      # five chests on fields of levels 1 to 30, a guide in Prontera
      enabled: true
      duration_minutes: 90
```

- A sample's maps must run: `core:Prontera_Invasion` needs `prontera`, and the others need fields of levels 1 to 30, such as `--maps prontera,prt_fild05,prt_fild06,prt_fild07,prt_fild08`. A level band with no field that runs logs an error when the run starts.
- A field's level is the middle level of its spawns, without bosses, from the mode's recorded spawns. Only maps whose ID has `_fild` are fields.
- The zones place the monsters and NPCs. One event manager per process reads what happens to them once a second: deaths of bosses and chests, invasion waves, and the scripts' `run:finish`.
- Rewards go through transfers with the action `event`. Each reward writes a row of `world_event_progress` (migration 00021) in the same transaction as the characters it changed, so a reward is never given twice.
- A typed run that a stop cut short goes on after the restart while its window is open, from its rows: a boss that did not die comes back, opened chests stay opened, and an invasion starts again at the wave it was in. A run that ended before its time has an `end` row and does not.
- A script with `events.control` starts and stops events with `wa.world_events`, and so does a quick test from a mod's NPC.
- Each start and end goes to every player as `WorldEventNotice`, and a player who logs in during a run gets `WorldEventList`. The client shows a banner and a list of the runs.

## Load tests and metrics

### Metrics

`--debug-addr` serves the server's measurements and Go's profiles. It has no access control, so `waserver` refuses an address that is not a loopback address.

```text
go run ./server/cmd/waserver --assets /path/to/build --map prontera --dev \
  --debug-addr 127.0.0.1:8301 --profile-dir /tmp/wathena-profiles
curl 'http://127.0.0.1:8301/debug/wathena/metrics?format=text'
```

| Endpoint | Use |
|---|---|
| `GET /debug/wathena/metrics` | Every measurement since the start or the last reset, as JSON. `?format=text` gives tables. |
| `POST /debug/wathena/reset` | Starts every count again, such as after the bots of a load test have joined. |
| `GET /debug/wathena/items` | With `--dev` only: every item, with its serial, and all zeny that the players and the ground of each zone hold, as JSON, for the duplication tests. |
| `/debug/pprof/` | Go's CPU, heap, goroutine and trace profiles. |

`waserver` also prints the tables when it stops.

What it measures:

- Per zone, the work of each tick: the tick itself, plus the commands, joins and leaves the zone goroutine handled since the tick before. It shows p50, p99, p99.9 and the maximum, ticks over 10 ms (the p99 budget) and over 25 ms (the maximum), and the mean time of each part of a tick: `move` (walks and warps), `act` (monsters, combat, items, regen, saves) and `send` (each player's frame). `step` is the tick alone. Quantiles are within 12.5%.
- A zone over 10 ms for 10 ticks in a row logs a warning with its last 10 tick times. With `--profile-dir`, it also records a 10-second CPU profile there. Warnings for one zone come at most once a minute, and profiles at most once every 5 minutes.
- For the process: CPU in cores, garbage collector CPU, cycles and pause times, scheduler latency, allocation rate, heap, total memory, the `GOMEMLIMIT` in effect, and goroutines.
- Per connection: WebSocket bytes and messages each way, as averages since the connection opened or the last reset, and as the most bytes in one second. Bytes include the WebSocket frame header and the client's mask key. TCP, IP and TLS headers are not counted. TLS adds about 22 bytes per message.

Set `GOMEMLIMIT` to the memory the server may use, such as `GOMEMLIMIT=2GiB`, so the garbage collector works to keep the process under it. The load tests ran without it. Raising `GOGC` did not help in them (strategy section 3.8).

### Bots

`tools/botclient` is a headless client (strategy section 12.1). It lives under `tools` because it is a test tool that the server never imports. It speaks the protocol with the generated message code:

- `Dial` connects, says hello with a session token (a character name for `--dev`), and returns once the bot sees itself.
- A bot tracks its map, its cell (following its own walk over time), its HP, and the entities in view, from the events it gets. It counts its bytes and messages as the server does, and the time from each walk or chat command to the frame that acknowledges it.
- `Play` runs a `Behaviour`: walks to random cells that a path reaches (with an asset build) at a set interval, chat lines at random times, attacks on the nearest monster in view, and walks to warp portals in view, which take the bot to other maps.
- `Scenario.Run` runs many bots: they join at a ramp rate, play for a warmup, and are measured for a duration. With the server's `--debug-addr`, it resets the server's counts when measuring starts and adds the server's measurements to the report.

`tools/wabots` runs a scenario and prints the report. Run it in its own process, so its CPU is not counted as tick work, and at a low priority, so it does not slow the server it measures:

```text
GOMAXPROCS=4 nice -n 15 go run ./tools/wabots --scenario town \
  --assets /path/to/build --metrics http://127.0.0.1:8301
```

The scenarios are in `tools/botclient/scenarios.go`. Each needs the server started as below. The rates follow the busy town estimate in `schema/protocol_test.go`.

| Scenario | Bots | What they do | Server |
|---|---:|---|---|
| `town` | 500 | Walk to a cell within 8 every 3 seconds on average, and say 2 chat lines a second together. | `--map prontera` |
| `crowd` | 500 | Stay within 10 cells of the start cell, so most see each other, as at an event. A walk every 2 seconds and 10 chat lines a second together. | `--map prontera` |
| `spread` | 500 | As `town`, but a third of their walks go to a warp portal in view, so they spread over the maps. | `--map prontera --maps prontera,prt_fild08,prt_in,prt_fild05,prt_fild06,prt_fild07,prt_church,izlude` |
| `fight` | 500 | Attack the nearest monster in view until it dies, walk on to find more, and say 0.5 chat lines a second together. | `--map prt_fild08 --build melee` |
| `smoke` | 50 | A walk every second and 5 chat lines a second together, on the 24 × 24 placeholder map. | `--map placeholder/map --mode none` |

The 500-bot scenarios join 50 bots a second, warm up for 15 seconds (60 for `spread`), and measure for 3 minutes. `smoke` joins 25 a second, warms up for 3 seconds, and measures for 20. Flags change them: `--bots`, `--ramp`, `--warmup`, `--duration`, `--walk-every`, `--walk-radius`, `--chat-rate` (lines a second of all bots together), `--fight`, `--stay`, `--warp-chance`, `--prefix` and `--seed`. `--file` reads a scenario from YAML instead:

```yaml
name: market
bots: 200
ramp: 20
warmup: 10s
duration: 2m
chat_rate: 4
behaviour:
  walk_every: 5s
  walk_radius: 6
  stay: true
```

`--json` writes the report as JSON. `--check` exits with status 1 when a budget is broken: tick work p99 over 10 ms or maximum over 25 ms, or a bot over 8 KB/s from the server on average, 32 KB/s in its busiest second, or 1 KB/s to the server.

Scenario bots use `--dev` sessions, so with a database each bot name becomes a dev account and character, which keeps its map and cell between runs. `botclient.Account` signs up, makes a character and gets a game ticket on the account pages instead, as `TestAccountFlow` does with `WATHENA_TEST_DATABASE_URL`. Scenarios do not use it yet, since the account service limits signups per address.

The bots join at the server's `--map`, or at their saved map with a database. To load other maps, use `spread`, whose bots walk through warps.

### Messages to connections

A zone does not wake each connection's goroutine itself. While a zone runs, its tick collects every message in an outbox and hands it to the zone's sender goroutine in one batch. The sender puts each message on its connection's channel, in order, and closes the channel of a connection that has 64 messages waiting. Before this, the zone goroutine woke 500 connection goroutines one by one each tick. Each wake can enter the kernel, and a trace of the 500-bot test showed the zone goroutine stalled for up to 9 ms between two wakes. A map change waits until the old zone's messages to the player went out, so they come before the new zone's.

### Local checks

The repository has no CI ([decision 0026](../docs/decisions/0026-local-checks.md)). `scripts/check.sh` runs the checks on the local machine, one step at a time, and lists the result of each at the end:

```text
scripts/check.sh quick        # vet, unit and lint, before each commit
scripts/check.sh quick client # the same, and the client's checks
scripts/check.sh              # the default steps, before a push
scripts/check.sh all          # also the Compose stack and the browser tests
scripts/check.sh lint e2e     # some steps
```

- The `unit` step runs `go test` without `-race` on every package but `e2e`. Go's test cache keeps the results of packages that did not change, so after a small change it reruns only that package and the packages that import it. It does not start PostgreSQL: the database tests skip unless `WATHENA_TEST_DATABASE_URL` is set.
- The `test` step runs the `unit` step, then `go test -race ./...`, which includes the end-to-end tests in [e2e](../e2e/README.md).
- Under `-race`, the heavy tests that run on one goroutine skip with `testrace.SkipUnderRace` ([decision 0022](../docs/decisions/0022-test-speed.md)). They run in the first pass.
- The `fuzz` step runs each fuzz test for 10 seconds, 4 at once.
- Without `WATHENA_TEST_DATABASE_URL`, the script starts a `postgres:18` container for the database tests when Docker runs, and removes it at the end. `CHECK_NO_DOCKER=1` turns that off.
- Checks of generated files compare the files before and after the generator, so uncommitted work does not fail them.
- A step whose tool is missing, such as `tinygo` or `pnpm`, is reported as skipped with the reason.
- The script header lists the steps and the versions of the tools they need.

The `loadtest` step runs `TestLoadSmoke` in `tools/botclient`: it builds `waserver` and the placeholder assets, starts the server in its own process, and runs the `smoke` scenario for 20 seconds. No bot may fail. Tick work must stay under 25 ms at p99 and 100 ms at most, 2.5 and 4 times the budgets, since shared machines are noisy. Bandwidth keeps its budgets. The test is skipped unless `WATHENA_LOAD_TEST=1`:

```text
WATHENA_LOAD_TEST=1 go test -count=1 -run TestLoadSmoke -v ./tools/botclient
```

The results of the 500-bot tests are in strategy section 3.8.
