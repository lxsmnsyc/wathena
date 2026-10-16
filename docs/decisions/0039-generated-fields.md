# 0039: Generated Fields

- Status: proposed.
- Date: 2026-10-16.
- Code: [shared/gen](../../shared/gen) (`field.go`, `v2*.go`, `wasm`), [server/fields](../../server/fields), [server/cmd/waserver](../../server/cmd/waserver) (`fields.go`, `config.go`), [server/store](../../server/store) (migration 00030, `World.PublicNode`), [tools/wactl/world.go](../../tools/wactl/world.go), [client/src/world](../../client/src/world) (`fields.ts`, `fieldGenerator.ts`, `fieldWorker.ts`, `generator.ts`, `gen.wasm`), [client/src/assets/store.ts](../../client/src/assets/store.ts), [client/src/viewer/main.ts](../../client/src/viewer/main.ts), [client/scripts](../../client/scripts) (`field-check.ts`, `browser-check.ts`, `game-check.ts`), [internal/assets/placeholder](../../internal/assets/placeholder), [internal/assets/model/simple.go](../../internal/assets/model/simple.go), [scripts/check.sh](../../scripts/check.sh), [scripts/fuzz.sh](../../scripts/fuzz.sh).
- Spec: [generator-v2](../specs/generator-v2.md).
- Phase 7 step 2 (strategy section 15, "Phase 7 in full").
- No protocol change: the fields' data goes over HTTP. No Lua API changes.

## Context

Step 1 built the generator core and a small grid (decision 0038). Step 2 makes fields a player can walk: terrain from biome rules, validation with retries, a WMAP map the renderer draws, a server that runs fields, and a client that generates them itself and downloads them when it cannot (strategy sections 8.2 to 8.5 and 8.12). Three questions needed answers:

- Which seed node a field uses, and how the game server gets it without the secret root.
- How a field's map reaches the renderer, the same on the server and in every browser.
- How the generator names art when wAthena ships none.

## Decision

### Public nodes and who reads what

The root's first level splits in two: `child(root, "public")` and `child(root, "secret")`. A field's node is `child(public, "field", x, y)`.

| Who | Reads | Can derive |
|---|---|---|
| The migrator role (`wactl world new`) | the world seed | everything |
| The game role (`waserver`) | `world.public_node`, the key of the public branch | every field's public node and the climate; no secret branch, no root |
| A client | the public node of each field it enters | that field's terrain only |

- `wactl world new` derives the public branch from the new seed and stores its key in the world row (migration 00030). The game role may read that column. It still cannot read `seed`.
- `waserver` derives each field's node from the public branch and sends it at `/gen/maps/<id>.json`. HMAC-SHA256 is one-way, so a field's node reveals no sibling, no parent and no secret branch (strategy section 8.3).
- The climate, which picks each field's biome, uses `child(public, "climate")`. Only the server runs it. A client gets the biome index with the node.
- Step 5 (spawns) will need secret branches on the server. It will store the key of `child(root, "secret")`, or of narrower nodes under it, in a column of its own, so a server that only runs fields never holds it.
- Strategy section 8.2's tree puts `terrain` and `secret` side by side under each region. The split at the top differs from it on purpose: one 32-byte key then covers every public map, and the game role can hold it alone.
- A world made before migration 00030 has no public node. `waserver` refuses such a procedural world and says to make it again. So does a world on generator 1, which makes no fields. New worlds get generator 2. No public world exists yet.
- A `--dev` server without a database makes a procedural world when `--fields` is given. Its secret is `wathena-dev-world:` followed by `--seed`, so a test gets the same fields on each run. Play randomness still comes from the zones' own streams, not from this tree.

### Generator version 2

- Version 2 is a new `Generator` in the registry, with `Field`, `Biomes`, `Tilesets` and `Climate`. Version 1 stays as it was, with its golden file.
- Biomes are data: a table of six (grassland, forest, desert, snow, swamp, beach) with height octaves, warp, terraces, ridges, basins or a beach slope, the water share and shallow depth, rivers, the slope limit and cliff type, the walkable range, props and thickets, ground materials and light.
- The field steps follow section 8.5 (spec, "Field steps"): height with domain warping, water and rivers, portal areas, cliffs, thickets, connectivity with roads, objects by Poisson-disk sampling, and materials. Each step uses its own child node.
- Integer and Q16.16 math only. `TestGuard` covers the new files. Lattice gradients are hashed once per lattice point, and the slow value noises once per lattice point too, so a cell costs additions and a few multiplications.
- The water level comes from a quantile of the heights, not from a fixed value. Each biome then gets the water share it names, whatever the noise gave.
- Terraces and thickets make the blocked share. Pure noise at the amplitudes that look right gave 95% to 99% walkable cells, outside every range of section 8.5.
- Props keep their 8 neighbors walkable, so a prop never splits a walkable area. Thicket props stand on cells that are already blocked.

### Validation and retries

- An attempt passes when every portal reaches the first one and the walkable share is in the biome's range. Connectivity is also built by the roads, so the check catches only what the roads could not fix.
- A failed attempt runs again from `child(node, "attempt", n)`, up to 8 attempts. Then the field is `ErrNoField`, an error that lists each attempt's problem. `waserver` does not start, and logs the field's public node for investigation. It never loops.
- Section 8.5 asks for a fallback template after 8 failures. This step makes it an error instead, as the task asked. 1 of 48 test fields and 2 of 11 golden fields needed a retry, and none needed more than 2.
- The fuzz test `FuzzFieldV2` generates fields from any key, size from 32 to 191 and biome, and checks each, or that the error is `ErrNoField`.

### The map: WMAP from the generator

- The generator writes the field's WMAP itself, in `shared/gen`. It builds f32 values from fixed-point values with integer math, so the bytes are the same in Go and in WebAssembly on every machine. The golden file holds each field's WMAP SHA-256 besides the grid checksum.
- The server serves those bytes for download, and a client that generates the field gets the same bytes. The existing renderer and pathfinder draw and walk it with no changes.
- The renderer reads a WMAP given as bytes (`MapScene.load` takes them), so a generated field never goes through the asset build.

### Art: tilesets and the tileset table

- The WMAP names abstract assets: `texture/gen/<tileset>/<slot>` for ground and water, and `model/gen/<tileset>/<prop>` for props. The generator's `Tilesets` lists the slots of each tileset.
- `waserver` serves a tileset table at `/gen/tilesets.json`: the asset each of those names stands for. By default every slot maps to the placeholder pack, which gains `placeholder/ground/<material>`, `placeholder/prop/<prop>` and `placeholder/model`, all drawn by code.
- `--tilesets` (or `procedural.tilesets`) names a YAML table that points slots at the operator's own build, such as one from the kRO client. Unknown tilesets or slots and invalid names are errors. wAthena ships no kRO art.
- The client's asset store resolves these names through the table, one level deep, before the manifest. A name that resolves to nothing shows the usual placeholder.
- The table stays outside the WMAP, so a changed table never changes a field's bytes or its checksum.

### The server

- A procedural world runs the fields that `--fields` (`procedural.fields`) lists, as `x:y` or `x:y:biome`, or `0:0` when none is listed. Each is `--field-size` cells square, 300 by default. A biome left out comes from the climate.
- The fields generate at start, several at once. Each runs in a zone named by its map ID, such as `gen:field:0:0`. Asset maps run beside them only when `--maps` lists them. The first field is the start map unless `--map` names a field.
- `/gen/maps/<id>.json` gives the node, the size, the biome, the grid checksum and the WMAP's path, SHA-256 and length. `/gen/maps/<id>.wmap` sends the WMAP, gzip-compressed for clients that accept it, with its SHA-256 as the ETag. The paths need no login and allow any origin, as assets do.
- For development, touching fields link their facing portals with warps: east to west and north to south. Step 4's map graph replaces this.

### The client

- After `Welcome` names a procedural world, the viewer loads the tileset table and sets a field loader. A map name that starts with `gen:` loads through it.
- The loader fetches the field's JSON, then generates the field with `gen.wasm` in a Web Worker. It uses the result when the checksum matches the server's. When it differs, when the module fails to load or lacks the version, or with `?nogen`, it downloads the server's WMAP and checks its SHA-256.
- A classic world still never fetches `gen.wasm`.
- `viewer.fieldLoads()` lists each field load with its source, its reason for a download, and its times, for the checks.

### Performance

A 400 × 400 field, from the golden file, on the development Mac (Apple M2):

| Where | Time |
|---|---:|
| Go, darwin/arm64 (`BenchmarkFieldV2`, the mean of the six biomes) | 53 to 64 ms |
| `gen.wasm` in Node 26 | 79 to 111 ms |
| Chromium | 75 to 91 ms |
| WebKit | 79 to 99 ms |
| Firefox | 690 to 735 ms, and 998 to 1008 ms right after the exit check |

- Section 8.5 budgets about a second in a Web Worker. Chromium and WebKit are far within it. Firefox is within it on an idle machine and at its edge on a busy one. It is 8 to 10 times slower than Chromium, as it was for version 1 (354 ms against 35 ms in the same run).
- A field runs in the worker, so a slow engine delays the map, not the page. The field check measured 55 to 83 ms for a 160 × 160 field in Chromium's worker, with the module's start.
- A 400 × 400 field's WMAP is about 4 MB, and 430 to 520 KB with gzip.
- An attempt reuses one set of large slices, which a pool keeps between attempts and fields, and the WMAP is written once at its full size. A 1024 × 1024 field takes about 450 ms in Node.
- TinyGo's heap in the module only grows, and large fields fragment it: twelve 1024 × 1024 fields in a row grew it to 1 GiB. The client stops its worker after a field when the module holds more than 256 MiB, and the next field starts a new one.

### Determinism

The 11 golden fields of `golden-v2.json` and the 8 grids of `golden-v1.json` match, with the WMAP hashes of the fields, on:

| Platform | Where |
|---|---|
| Go, darwin/arm64 | `scripts/check.sh gen` and `unit` |
| Go, darwin/amd64 under Rosetta | `scripts/check.sh gen` |
| TinyGo, native and wasip1 in wasmtime | `scripts/check.sh tinygo` |
| `gen.wasm` in Node 26 | `scripts/check.sh gen` and `client` |
| `gen.wasm` in Chromium, Firefox and WebKit | `scripts/check.sh browsers`, the check `gen` |

### Tests and checks

- `shared/gen`: `TestGolden` for both versions, `TestFieldV2` (48 fields of every biome: cell types, every walkable cell reached from the first portal, the walkable share, props on blocked cells, the WMAP's cells against the grid, and the same field twice), `TestFieldV2Retries` (8 attempts and the error, and a passing attempt equals its node's field), `TestFieldV2Inputs`, `TestClimateV2`, `TestF32Bits`, `TestFieldIDs`, `FuzzFieldV2` and `BenchmarkFieldV2`.
- `server/fields`: `TestGenerate` (nodes under the public branch, a client's field from the key alone has the same checksum and WMAP), `TestServe` (info, WMAP, gzip, ETag, tileset table, 404 and 405), `TestTilesets`, `TestWarps` and `TestParseSpec`.
- `server/cmd/waserver`: `TestFieldsConfig` and `TestFieldMaps`. `server/store`: `TestWorldMode` reads the public node as the game role. `tools/wactl`: `TestPublicNode`.
- The client: `fields.test.ts` (the generated field when the checksums match, a download when they differ, when the module fails or is missing, and a refused download with the wrong SHA-256), the alias test in `store.test.ts`, and `generator.test.ts` with version 2's golden fields.
- `scripts/check.sh gen` runs the field tests on both architectures. `fuzz` builds with the procedural tag, so `FuzzFieldV2` runs.
- `pnpm check:browsers` has the check `field`: a `waserver` built with the tag runs a `--dev` world with a grassland and a desert field. A page generates its field in the worker with the server's checksum and walks three legs, each confirmed by the server, and a page with `?nogen` downloads a map with the same cells. The check `gen` compares version 2's checksums and WMAP hashes in all three engines.

### The exit check's Training Dummy

The last browser run failed `exit mod skill: no Training Dummy stands clear in sight`. A Lantern Ghost had spawned next to the only Training Dummy, at a random cell. The checks' copy of the lantern mod now places its three ghosts on fixed cells, away from the Dummies, the Training Dummy, the NPCs and the cells the checks walk to (decision 0037, "Lantern Ghosts on fixed cells"). The mod in `mods/examples` does not change.

## Lua API

None. Mods cannot add biomes, tilesets or generation layers yet. `wa.generation` (step 6) is the place for them, and so is a mod's own tileset table.

## Left

- Global coordinates (section 8.5, "World scale"): the noise is sampled per field, so terrain does not continue across a border. Shared noise needs a node shared by every field, which a client would then hold. Step 4 decides it with the map graph.
- Places, feature tags and spawn areas (field step 6 and the spawn-area check of section 8.5), and the fallback template after 8 failures.
- Blend weights between materials: WMAP has one texture per surface. The tile colors blend between surfaces, but the textures do not.
- Lighting is per biome. Fog has no WMAP section.
- Navigation data for the client's navigation window.
- Generated maps are not cached on disk on the server or in the browser's cache. The server generates its fields at each start.
- A run on a real amd64 machine and in Safari.
