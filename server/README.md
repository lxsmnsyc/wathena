# server

Go services: gateway, account, world, and zone. See docs/strategy.md, section 4.

| Package | Content |
|---|---|
| `zone` | One map: its players and monsters, local chat to every player within 9 cells of the speaker, their walks through `shared/path`, and what each player sees within 14 cells, in 50 ms ticks. Monsters wander as rAthena's `mob_randomwalk`, from the zone's secret random stream. Units attack a target at their attack delay, walking into range first. A monster that can attack strikes back. Dead monsters respawn after their respawn time, and dead players return to the start after 3 seconds. A kill gives exp to the players who fought the monster and drops items around it. Items lie on the ground for a minute, and players pick them up with the looter rights rAthena gives the top attackers. Each player has rAthena's inventory: 100 slots, stacks up to 30000, and a weight limit that pick-ups respect. Players use items, whose scripts run in Lua (see "Item scripts"), and drop items at their feet with no looter rights. HP and SP regenerate as rAthena's natural regen, which stops from 70% weight in renewal (50% in pre-renewal). From 90% weight, attacks stop. Warps send a player who steps into their area to their target, as rAthena's `npc_touch_areanpc` and `pc_setpos`: to another map as a message to that map's zone, or within the map. A zone without players sleeps. Each tick finds what a player sees through an index of 8 × 8 cell blocks, as rAthena's `BLOCK_SIZE`, and hands its messages to a sender goroutine in one batch (see "Load tests and metrics"). |
| `metrics` | Measurements for the budgets in strategy section 3.8: tick work per zone, garbage collector pauses, CPU, memory, goroutines, and bytes per connection. A zone over its tick budget for 10 ticks in a row logs a warning and can write a CPU profile. See "Load tests and metrics". |
| `world` | Several zones in one process, one goroutine per map. It finds zones by map name, puts a joining character in the zone of its saved map, and waits at shutdown for every zone and every player between zones. `LoadWarps` reads `reference/rathena/extracts/<mode>/warps.json`. |
| `item` | The inventory, ported from rAthena's `pc_additem`, `pc_delitem` and `pc_inventoryblank`, the overweight levels of `pc_updateweightstatus`, the heal of `pc_itemheal` and `pc_percentheal`, and the item groups of `getgroupitem`, with the item data from `reference/rathena/extracts/<mode>/items.json` and `item_groups.json`. |
| `battle` | Attacks, ported from rAthena's `battle.cpp` in renewal mode and checked against the recorded reference. |
| `status` | Player stats from job, levels, raised stats and skills, ported from rAthena's `status_calc_pc` for a player with no equipment, in both modes, and checked against every recorded player. Raising stats with status points, ported from `pc_statusup`, `pc_need_status_point` and `pc_maxparameter`, with the costs of both modes and the job limits from `conf/battle/player.conf`. |
| `progression` | Exp tables, level ups, status and skill points, the renewal level penalty, exp shares, drop rates and rolls, where drops land, and pick-up rights, ported from rAthena's `pc.cpp`, `mob.cpp` and `map.cpp` with the default `conf/battle` settings. |
| `character` | A saved character as plain data: what the zone loads and hands to the store. |
| `store` | PostgreSQL persistence: goose migrations in `migrations`, sqlc queries in `queries` generated into `db`, a pool of save workers, and game ticket redemption. `storetest` makes throwaway databases for integration tests. See [decision 0003](../docs/decisions/0003-database-migrations-and-queries.md). |
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

Players gain exp and levels from kills, with the exp tables, drops and level penalty from `reference/rathena/extracts/<mode>/progression.json`. Each base or job level up computes their stats again, and a base level up heals HP and SP fully. Players spend status points from the client's status window, and each raise computes their stats again. Local chat reaches every player within view, the speaker included; other channels are ignored for now. Without a database, characters and the inventory only live in memory. See "Running with PostgreSQL".

`--start-items` gives every new character items, for development only, such as `--start-items 501:5,512:10` for 5 Red Potions and 10 Apples. Without it, characters start with nothing and get items from drops. With a database, a character only gets them when it is made.

One character has one session. A second login with the same name, in any case, kicks the first, as rAthena does. The first page shows "Your character logged in somewhere else." The second continues once the first has saved.

## Running with PostgreSQL

`waserver` keeps characters in PostgreSQL 18 or later when it gets a database URL, from `--database` or the `WATHENA_DATABASE_URL` environment variable. Use the environment variable, so the password does not show in process lists. Accounts need a second URL, for the account role, in `WATHENA_ACCOUNT_DATABASE_URL`.

With a database:

- It refuses to start while migrations are pending.
- With `--dev`, a token that is a new name makes an account and a character of that name, marked as a dev account. Dev accounts have no password and cannot log in on the account pages.
- Characters load when they log in and are saved every 30 seconds when they changed, when they log out, and when the server stops (Ctrl+C or SIGTERM).
- Characters are also saved when they arrive on another map, at their new cell.
- The level, exp, status and skill points, raised stats, HP, SP, map and position, look, zeny and inventory are saved. Items keep their serial numbers.

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
| `debug_addr` | `--debug-addr` | none | Loopback address for measurements and profiles. |
| `profile_dir` | `--profile-dir` | none | Folder for CPU profiles of slow ticks. |
| `shutdown_countdown` | `--shutdown-countdown` | `10s` | The countdown players see before the server stops, up to `10m`. `0s` turns it off. See "NPCs". |
| `mods.dir` | `--mods` | none | The folder of installed mods. See "Mods". |
| `mods.ruleset` | `--ruleset` | the core ruleset of `mode` | The world's ruleset mod. |
| `mods.strict` | `--mods-strict` | `false` | Data overlay warnings become errors. |
| `mods.approve` | none | none | Approved permissions, by mod ID. |
| `mods.disable` | none | none | Installed mods that must not load. |

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

`waserver` loads the mods in `mods.dir` at startup (strategy section 6, [decision 0006](../docs/decisions/0006-mod-loader.md)). [mods/README.md](../mods/README.md) is the guide for mod authors. `mods/examples/greeter` is a small sample.

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
- Items from mods get IDs from 2,000,000 the first time they load. With a database the IDs are kept in the `mod_ids` table, and never change or go to another item. Without a database they live in memory, like the characters.
- Each zone runs the mods' scripts in its own Lua state. A mod whose scripts fail is logged and left off in that zone. A mod whose handlers fail 10 times in a minute is turned off in that zone.
- A zone makes its Lua state at start when a mod has entry scripts, and otherwise when a player first uses an item there.

## Item scripts

`waserver` converts rAthena's item scripts for its `mode` when it starts, and logs `item and npc scripts converted` with the time it took, about 0.9 s for renewal ([decision 0007](../docs/decisions/0007-converted-item-scripts.md), mods/README.md, "Core's item scripts"). They are the files of the `core` mod.

- A used item runs its Lua `use` handler: heals, items, item boxes, effects and warps.
- An item whose script needs what the server does not have yet, such as status effects, is refused and kept. The player sees a chat line that says what it needs.
- Butterfly Wings and `player:warp_to_save_point()` go to the character's save point. A new character's save point is the start cell of the start map (`--map`).
- Rentals are given as normal items, since items do not expire yet.

To try them, give a new character some items:

```text
go run ./server/cmd/waserver --assets /path/to/build --map prt_fild08 --dev \
  --start-items 501:5,603:3,601:3,602:2,506:2
```

These are Red Potions, Old Blue Boxes, Fly Wings, Butterfly Wings, and Green Potions, which need status effects and are refused.

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
- Until the ruleset flags drive the game code (phase 4), flags only decide which mods may load.

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

## NPCs

`waserver` also converts rAthena's NPC scripts and shops when it starts, in the same step ([decision 0008](../docs/decisions/0008-converted-npcs.md), mods/README.md, "Core's NPCs"). It takes about 0.17 s for renewal.

- Clicking an NPC within 15 cells starts its `click` handler, or opens its shop. A player has one conversation at a time, and cannot act while it lasts.
- A conversation ends when its script ends, when the player cancels it, leaves, dies, or changes map, and after rAthena's timeouts: 60 seconds for next, menus and close, and 180 seconds for text and number input.
- Shops follow rAthena's buy and sell rules: Discount and Overcharge, weight, and zeny up to 2,147,483,647.
- Kafras save, teleport and heal. Storage and pushcarts say they are not available yet.
- The save point is kept in the `characters` table. A dead character is saved at its save point, and a player who dies goes back there.
- Character and account variables of `wa.variables` save with the character. Server variables save right after they change, and again when the server stops. Without a database they live in memory.

To try them, start in Prontera with some Jellopy to sell and a Butterfly Wing:

```text
go run ./server/cmd/waserver --assets /path/to/build --maps prontera,prt_in,izlude --dev \
  --mods mods/examples --start-items 909:300,602:1
```

Stopping:

- On SIGTERM or Ctrl+C the server announces a countdown to every player, at 30, 10, 5, 3, 2 and 1 seconds, then saves and stops. A second signal stops it at once.
- `shutdown_countdown` (`--shutdown-countdown`) sets the countdown. The default is 10 seconds, the most is 10 minutes, and 0 turns it off.

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

### CI

CI's `loadtest` job runs `TestLoadSmoke` in `tools/botclient`: it builds `waserver` and the placeholder assets, starts the server in its own process, and runs the `smoke` scenario for 20 seconds. No bot may fail. Tick work must stay under 25 ms at p99 and 100 ms at most, 2.5 and 4 times the budgets, since CI runners are shared. Bandwidth keeps its budgets. The test is skipped unless `WATHENA_LOAD_TEST=1`:

```text
WATHENA_LOAD_TEST=1 go test -count=1 -run TestLoadSmoke -v ./tools/botclient
```

The results of the 500-bot tests are in strategy section 3.8.
