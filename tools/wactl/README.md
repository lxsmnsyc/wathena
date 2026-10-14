# wactl

Command-line tool for worlds, mods, maps, and assets. See docs/strategy.md, sections 3.9, 6.9 and 10.1.

## Commands

`wactl assets build --source <source> [--source <source> ...] --out <folder> [--fast] [--codepage <code page>]`

Builds assets from sources, applied in order, into an output folder with a manifest (docs/specs/assets-v1.md). Sources are `placeholder`, `grf:<client folder>`, `pack:<folder>` and `ro:<folder>`. The GRF source converts monster, NPC, body, head, headgear, weapon, shield, and garment sprites, hair and body palettes for human and doram characters, textures, UI images, and illustrations, item icons, images, card art, and dropped-item sprites named by item ID, sounds, RSM models, maps with their lightmaps, STR effects, the MP3 music in the client's `BGM` folder, and the item text tables `table/items` and `table/itemdesc`. Body, weapon, shield, and garment names use the looks in `client/data/jobs.yml`. Rebuilds reuse unchanged assets from the cache in the output folder. `--fast` uses a low Brotli level for development builds. `--codepage` is the code page of the client's text, such as item names: `cp949` by default, for the Korean client, or `cp1252` and others for translated clients (docs/specs/assets-v1.md, "Item text from a game client").

`pack:<folder>` reads original art in the pack format: `pack.yaml` with the license, PNG sprites with YAML frame files, PNG, JPEG and WebP textures, glTF binary models that fit the profile, WAV sounds, Opus in WebM, MP3 music and JSON tables (docs/specs/assets-v1.md, "Pack format for original art"). `ro:<folder>` reads loose RO files: `.spr` with `.act`, `.bmp`, `.tga`, `.wav`, `.pal`, `.rsm`, `.str`, and maps as `.rsw` with `.gnd` and `.gat` ("Loose RO files"). In both, a file's path without its extension is its name, and a file in the wrong folder for its type is rejected with the reason.

`wactl assets check --build <folder> [--list] [--json] [--mod <mod id> [--max-mb <n>]]`

Verifies every file in a build against its hash and decodes it, and lists names that maps, models, sprites, effects, or the client data tables refer to but the build lacks. It exits with status 1 if any file is broken. Missing names are warnings; the client shows placeholders for them. `--mod` checks a mod's build, such as its `assets/`, with the server's rules: names under `mod/<mod id>/` in namespaces of their types, and at most `--max-mb` MiB, 200 by default.

`wactl assets gc --build <folder> [--dry-run]`

Removes files the current build no longer uses, such as outputs of older converter versions.

`wactl assets ls --build <folder> [<prefix> ...]`

Lists the names in a build with their type and size, optionally only those that start with a prefix, such as `sprite/head/`.

`wactl assets serve --build <folder> [--addr <host:port>]`

Serves a build over HTTP as the game server will: `manifest.json` without caching, and files named by their hash with long caching, a fixed content type, and Brotli for binary formats. The web client's dev server forwards `/assets` to it. The default address is `127.0.0.1:8200`.

`wactl assets scan --client <folder> [--read]`

Lists what a game client folder contains, grouped by asset type. The folder needs a `DATA.INI` and the GRF files it names. With `--read`, it also decompresses every file and reports the speed and any damaged entries.

`wactl assets sprites --client <folder> [--preview <folder> --names <names>] [--brotli <0-11>] [--every <n>]`

Converts every monster and NPC sprite into WSPR and reports sizes and failures. With `--preview`, it writes a PNG of the first idle frame of each listed sprite. This checks the sprite importer. The full build comes later.

`wactl convert items [--mode renewal|pre-renewal] [--out <folder>] [--report <file>]`

Converts rAthena's item scripts from the extracts in `reference/rathena`, as `waserver` does when it starts. `--out` writes core's `data/items.yml` and `scripts/items/*.lua` for reading. `--report` writes the coverage report, such as `docs/generated/item-scripts-renewal.md`. Without either, it prints a summary. See tools/convert/README.md.

`wactl convert npcs [--mode renewal|pre-renewal] [--out <folder>] [--report <file>]`

Converts rAthena's NPC scripts and shops from the extracts in `reference/rathena`, as `waserver` does when it starts. `--out` writes core's `data/npcs.yml`, `scripts/npcs/*.lua`, `scripts/functions/*.lua` and `scripts/lib/rathena.lua` for reading. `--report` writes the coverage report, such as `docs/generated/npc-scripts-renewal.md`. Without either, it prints a summary. See tools/convert/README.md.

`wactl migrate status`, `wactl migrate up`

Lists the database migrations, or applies the pending ones. They need the migrator role's URL in `WATHENA_MIGRATE_DATABASE_URL`, or in a file that `WATHENA_MIGRATE_DATABASE_URL_FILE` names, such as a Docker secret. `--database` also takes it, but shows the password in process lists. See server/README.md, "Running with PostgreSQL".

`wactl world new --admin <name> --admin-password-file <file or -> [--name <world name>] [--ruleset renewal|pre-renewal]`

Makes a world in a database that `deploy/postgres/roles.sql` prepared. It applies pending migrations, then makes the world's row and the first staff account in one transaction:

- The world row has the name, the ruleset, and a secret 32-byte root seed from `crypto/rand` (docs/specs/rng-v1.md). `waserver` refuses to start without it, and when its `--mode` is not the ruleset.
- The staff account has the given name and password, with the account service's rules. The password is the first line of the file, or of standard input for `-`. Staff accounts have no staff tools yet.

It refuses a database that already has a world. It needs the migrator role, as `wactl migrate` does.

`wactl world backup --out <file or ->`, `wactl world restore --in <file or ->`

Write the whole database with `pg_dump` in its custom format, or load such a backup with `pg_restore` in one transaction. They run as the migrator role, and pass its password to the PostgreSQL tools in `PGPASSWORD`, not on the command line. A backup file is made readable by its owner only. Restore refuses a database that has any migration applied, so it only loads into a new database made with `roles.sql`. They need `pg_dump` and `pg_restore` 18, which the `wathena/wactl` image has (deploy/README.md, "Backups").

`wactl mod new <id> [--dir <folder>]`, `wactl mod validate [path] [--mods <folder>] [--show]`, `wactl mod test [path] [--mods <folder>] [--run <text>] [--assets <build>]`, `wactl mod pack [path] [--out <folder>]`, `wactl mod build-art [path] [--out <folder>] [--fast]`, `wactl mod install <git URL | .zip | folder> --mods-dir <folder> [--ref <ref>] [--sha256 <hex>] [--public-key <key>] [--replace]`, `wactl mod types [path]`

The mod tools (strategy section 6.9). `new` makes a mod from a template that passes `validate` and `test`. `validate` checks a mod, and `--show` prints the merged records it touches. `test` runs `tests/*_test.lua` in headless worlds with `wa.testing`. `pack` writes a versioned `.zip` and a SHA-256 file, with the mod's `art/` built into `assets/` inside it. `build-art` builds a folder mod's `art/` into its `assets/`, which the server reads, and fails on any file it cannot use. `validate` also reports art names the mod's records name that the mod does not provide, and checks its client data tables. `install` checks the checksum and a minisign signature when given, validates, shows the permissions that need the owner's approval, and copies the mod into a server's mods folder without approving anything. `types` writes the Lua Language Server definitions. `validate`, `test` and `--mode` check against the reference data built into wactl. mods/README.md, "Tools", has the details and examples. The code is in tools/modkit.

`wactl client-data validate [<mod folder> ...]`

Checks the client data tables in `client/data` against their schema, then each named mod's `client/*.yml` merged over them in the order given (docs/client-data.md).

`wactl dev [path] --assets <build> [--mods <folder>] [--server <waserver>] [--map <map>] [--addr <host:port>] [--debug-addr <host:port>]`

Runs `waserver --dev` on loopback addresses with the mod and the mods of `--mods`, and approves what they request. It watches the mod's files: a Lua or locale change reloads the mod's scripts through the reload endpoint, and a change to `mod.yaml` or `data/` restarts the server. `--assets` defaults to `$WATHENA_ASSETS`. The `waserver` binary is `--server`, the one next to `wactl`, or the one on `PATH`.
