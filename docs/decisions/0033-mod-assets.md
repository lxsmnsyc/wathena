# 0033: Mod Assets

- Status: proposed.
- Date: 2026-10-14.
- Code: [internal/assets](../../internal/assets) (`name.go`, `pack/`, `build/check.go`, `serve/serve.go`, `model/pack.go`, `texture/texture.go`), [server/modassets](../../server/modassets), [server/mod](../../server/mod) (`art.go`, `items.go`, `npcs.go`), [server/gamedata](../../server/gamedata) (`world.go`, `jobs.go`, `mods.go`), [server/skill](../../server/skill) (`mods.go`), [server/companion](../../server/companion), [server/cmd/waserver](../../server/cmd/waserver) (`config.go`, `main.go`), [client/data](../../client/data) (`merge.go`), [client/src](../../client/src) (`assets/art.ts`, `assets/store.ts`, `data/clientData.ts`, `data/tables.ts`, `sw/sw.ts`, `viewer/`), [tools/modkit](../../tools/modkit) (`art.go`, `pack.go`, `validate.go`), [tools/wactl](../../tools/wactl), [mods/examples/lantern](../../mods/examples/lantern), [e2e](../../e2e) (`art_test.go`, the fixture's art).
- Builds proposal 0002, review decision 38, and phase 6 step 6 of strategy section 15. Builds on strategy sections 10.1 and 10.5 and [assets-v1](../specs/assets-v1.md). No Lua API changes. No protocol changes.

## Decision

### Sources

A mod keeps its art's sources in `art/`. One folder may hold both kinds:

- The pack format of assets-v1, with `art/pack.yaml` naming the license. PNG sprites with an optional YAML frame file, PNG, JPEG and WebP textures, glTF binary models, WAV sounds, Opus in WebM, MP3 music and JSON tables.
- Loose RO files: `.spr` and `.act`, `.bmp`, `.tga`, `.wav`, `.pal`, `.rsm`, `.rsm2`, `.str`, and maps as `.gat`, `.gnd` and `.rsw`. The GRF importer's parsers and converters read them.

`internal/assets/pack` reads both. `wactl assets build` gains two sources for a server's own build: `pack:<folder>` for the pack format and `ro:<folder>` for loose RO files.

- A file's path without its extension is its name, under `mod/<mod id>/` for a mod. A file must be in a namespace of its type, such as a sprite under `sprite/`.
- A file the build cannot use is left out and listed with the reason: a wrong folder, a sprite without its `.act`, a map without its `.gnd` or `.gat`, or a format that is not converted yet. `wactl mod build-art` and `wactl mod pack` fail on any.
- References inside loose files find the folder's own files first, without regard to case, and otherwise get the names the GRF importer gives. A model's or map's texture `p` is `texture/<p>`, a map's model is `model/<p>`, a sound is `sound/<p>`, and an STR effect at `effect/<path>.str` finds its textures in `texture/effect/<folder>/`.
- A sprite event that names a sound the folder has gets the folder's prefix, so a mod's sprite can name its own sound.
- A glTF model must fit the model profile already. Its `asset:` image names that the folder has get the prefix. Converting other glTF files, such as ones with embedded images, is left for later.

The YAML frame file of a pack sprite is new. assets-v1, "Pack sprites", describes it:

| Field | Meaning |
|---|---|
| `grid` | `[width, height]`: frames cut left to right, then top to bottom. |
| `frames` | `[[x, y, width, height], ...]`, in place of `grid`. |
| `origin` | `[x, y]`, the pixel on the entity's cell. The default is the middle of the bottom row. |
| `anchors` | `[[x, y], ...]` from the origin, the same on every frame. |
| `actions` | Actions in RO's order. Each has `frames` for all 8 directions, or `directions` with 8 lists, `interval_ms` (150 by default), and `events` from a position in the list to a sound name or a marker. |

- An 8-bit indexed PNG keeps its palette, and index 0 is transparent, as in RO. Other PNG files become RGBA pages.
- A sprite without a frame file is one still frame in 8 directions.

### Names

Mod art is named `mod/<mod id>/<namespace>/<record name>`, such as `mod/example.lantern/icon/item/Paper_Lantern`. assets-v1 gains these rules, without a new contract version, since readers never checked names:

- A name under `mod/` has a valid mod ID as its second segment, and at least one segment after the namespace.
- Upper case is allowed after `mod/<mod id>/<namespace>/`, since record names keep their case.
- The type of a mod's name comes from its first segment after the mod ID.
- A pack in a server's own build keeps lower case names. A file with upper case is rejected.

### Builds

- `wactl mod build-art [<folder>]` builds `art/` into the mod's `assets/`, with `manifest.json` at its root. `--out` builds elsewhere, and `--fast` uses a low Brotli level.
- `wactl mod pack` builds `art/` again into `assets/` inside the `.zip` file, and leaves out a build in the folder. The build's time is fixed, so the same files pack to the same bytes. Built files are stored in the zip without compression, since they are Brotli or WebP already.
- `wactl assets check --build <folder> --mod <mod id>` checks a mod's build with the server's rules, and `--max-mb` its limit.

### Loading and serving

`server/modassets` loads each mod in load order when the server starts:

- A mod with `assets/manifest.json` is checked with `build.CheckFS`, which reads a build in a folder or in a `.zip` file: every file exists, matches its hash and size, and decodes within the asset limits, every name is valid, under `mod/<mod id>/`, and in a namespace of its type, and the build's distinct files hold at most `mods.max_asset_mb` MiB before compression, 200 by default. Any problem stops the start with the list. The server converts nothing.
- A mod with `art/` and no build starts with a warning. Its records show placeholders.
- A build is served at `/assets/mods/<mod id>/<version>/` by `internal/assets/serve`, with the headers and caching of core's build: `manifest.json` with `no-cache`, hashed files for a year with `immutable`, Brotli files as `br`, `nosniff`, and `Access-Control-Allow-Origin: *`. A file of a `.zip` that cannot seek is read into memory, up to 64 MiB.

### Records

A field names a mod's art or core's. A value with a `/` is an asset name, so the values these fields took before still work.

| Kind | Field | Names |
|---|---|---|
| Items | `icon`, `image` | Textures. |
| Items | `drop_sprite`, `view_sprite` | Sprites. |
| Monsters | `sprite` | A sprite, or the AegisName of a monster as before. A new monster with a sprite of art starts from a Poring's stats. |
| Monsters | `sounds` | A table of `attack`, `damage`, `die` and `move` to sounds. |
| NPCs | `sprite` | A sprite, or an NPC sprite name or ID as before. |
| Jobs | `look` | A job's name as before, or a table with `job`, the job a new job starts from, and `m` and `f`, body sprites. |
| Skills | `icon` | A texture, or a skill's name as before. |
| Skills | `effects` | A table of `caster`, `effect` and `hit` to lists of effects. |
| Companions, mounts | `look` | A sprite, or a core look as before. A sprite of art needs `acts_as`, whose option bit shows it. |

A mod's own record without a field uses the art its build has under the record's name: `icon/item/<name>`, `image/item/<name>` and `sprite/item/<name>` for items, `sprite/body/<name>/<m or f>` for jobs, and `icon/skill/<name>` for skills. Monsters and NPCs name their sprite, since the field is required.

NPC sprites of art have no sprite ID. The server gives each name an ID from 1,000,000 at each start, in the order of the names, and sends it as `Look.body`.

### Client data

`GET /client-data.json` on the game server tells the client which art each record uses. It is JSON with `version: 1`, served with `no-cache`, an ETag and `Access-Control-Allow-Origin: *`:

| Field | Content |
|---|---|
| `mods` | The builds to load after core's, in load order: `id`, `version` and `assets`, the URL path. |
| `items`, `monsters`, `npc_sprites`, `jobs`, `skills`, `companions`, `mounts` | The art of records by ID, as a decimal string. |
| `tables` | The client data tables merged with the mods' tables, by file name, when a mod has any. |

- A field that goes, changes name or changes meaning raises the version. New fields keep it.
- The protocol does not change. Proposal 0001 sends data larger than a message over HTTP, and the client needs the art before the first `EntityAppear`, so it loads the client data before it connects, as it loads core's manifest. Welcome is unchanged and the protocol version stays 1.

### Client data tables

A mod may ship `client/jobs.yml`, `client/weapons.yml`, `client/effects.yml` and `client/skills.yml`. `client/data` merges them over core's, in load order:

- An entry with the key of an existing entry replaces it: `look` for looks and `id` for the other lists. Others are added.
- Each merged table passes the checks of core's table. A table may name a mod's art, such as an effect `mod/example.lantern/effect/glow`.
- A table is at most 4 MiB, and YAML aliases are refused.
- `wactl client-data validate [<mod folder>...]` checks core's tables and the mods' over them. `wactl mod validate` checks them too. [docs/client-data.md](../client-data.md) is the guide.

### Validation

`wactl mod validate` reports:

- each art name a record of the mod names under `mod/` that the mod it names does not provide, from that mod's `art/` sources or its built `assets/`;
- art under the ID of a mod that does not load with it;
- files of `art/` a build would leave out;
- client data tables that do not pass their checks.

Names of core's art are not checked, since validate has no server build. `wactl assets check` lists those.

### Web client

- The client loads core's manifest, then `client-data.json` from the game server before it connects, then each mod's `manifest.json`. A mod build that fails to open is logged, and its art shows placeholders.
- `AssetStore` resolves names under `mod/<mod id>/` in that mod's build, and logs a missing name once, mods' names too.
- `assets/art.ts` names the art of an item's icon and dropped sprite, a monster's and an NPC's sprite, and a job's body by the client data, and by ID without an entry.
- A missing item icon shows `placeholder/texture`. Sprites already fell back to `placeholder/sprite`.
- The merged tables replace the built-in ones, after every table is checked.
- The Service Worker caches a mod's files as core's, under keys of their own.

### A fix on the way

A mod's build converts textures and sounds at once, and `TestPackSource` found a data race under `-race`: wazero 1.12 sets its version string without a lock when the first runtimes start together. `internal/wasmhost` now makes runtimes one at a time. A GRF build could hit the same race.

## Sample

[mods/examples/lantern](../../mods/examples/lantern), `example.lantern`, has original art drawn by code, CC0:

- the Paper Lantern, whose icon and dropped sprite are found by its name, Lantern Oil, whose `icon` field names the same icon, and the Unlit Lantern, which has no art and shows the placeholder;
- the Lantern Ghost, an animated indexed sprite of 4 frames with a YAML frame file, spawned on prontera and the placeholder map;
- the Lamplighter, an NPC with an RGBA sprite who sells the lanterns;
- `client/skills.yml`, which adds an effect to Bash's hits.

## Tests

- `internal/assets`: `TestModNames`, with the names, `SplitMod` and `TypeOf`.
- `internal/assets/pack`: `TestPackSource` (names, types, the frame file, the palette, the prefixed sound event, rejected files, the mod checks of a build and a second build from the cache), `TestCorePackNames`, `TestParseInfo`, `TestSpriteSpecErrors`, `TestLooseRO` (sprites with their sounds, an STR effect with its texture, BMP and TGA of one name, rejected halves), `TestClientFiles` (a monster sprite, a sound, a model with its textures, an effect and the smallest map from a game client, with `WATHENA_CLIENT`), and the fuzz tests `FuzzSpriteSpec`, `FuzzSpriteImage` and `FuzzParseInfo`.
- `server/mod`: `TestArtFields`. `server/gamedata`: `TestMonsterArt` and `TestJobLookArt`. `server/cmd/waserver`: `TestModAssetLimitConfig`.
- `client/data`: `TestMerge`, `TestMergeErrors` and `FuzzMerge`.
- `server/modassets`: `TestServeModBuild` (headers, Brotli, caching, wrong versions and paths, the client data's ETag), `TestServeZipMod` (a packed mod, and packing twice gives the same bytes), `TestRefusedBuilds` (a corrupt file, a file with another hash, a missing file, a build over the limit, another mod's names, a bad table) and `TestArtWithoutBuild`.
- `tools/modkit`: `TestLantern` validates the sample, runs its tests, builds its art and checks its client data.
- `e2e`: `TestModArt` reads the client data of the fixture's Test Charm and Dummy by the IDs the server gave, and downloads both through the build's manifest and shards, as the client does.
- `client`: `clientData.test.ts`, with the mod builds after core's, the missing names logged once, the tables, the refused client data and the names of `art.ts`.

## Other choices

- A field in Welcome for the client data's URL. The client needs the art before the first frame, and Welcome comes with it, so the client would hold frames until the download. A fixed path on the game server needs no protocol change.
- Upper case names lowered to fit the old rule. Two records that differ by case would then share art, and authors would have to know the rule for every field.
- A separate source for each kind of file in a mod. One folder with both kinds is simpler for authors, and the extensions do not overlap except `.wav`, which converts the same way.
- Letting a mod's build replace core's names. A mod changes core's art through a record's fields, which name its own art, so one mod cannot change what every other mod and core show without saying so.
- Checking the mods' builds in the background after the start. A broken build found later would leave the client with names that do not load. The check of a 200 MiB build takes seconds.
