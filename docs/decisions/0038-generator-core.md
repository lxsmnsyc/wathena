# 0038: Generator Core

- Status: proposed.
- Date: 2026-10-16.
- Code: [shared/gen](../../shared/gen), [client/src/world](../../client/src/world) (`generator.ts`, `gen.wasm`), [client/src/net/connection.ts](../../client/src/net/connection.ts), [client/scripts/browser-check.ts](../../client/scripts/browser-check.ts), [server/gateway/worldmode.go](../../server/gateway/worldmode.go), [server/cmd/waserver](../../server/cmd/waserver) (`checkWorldMode`), [server/store](../../server/store) (migration 00029, `World`), [tools/wactl/world.go](../../tools/wactl/world.go), [schema/proto/wathena/v1/session.proto](../../schema/proto/wathena/v1/session.proto), [scripts/check.sh](../../scripts/check.sh).
- Spec: [generator-v1](../specs/generator-v1.md).
- Phase 7 step 1 (strategy section 15, "Phase 7 in full").
- Protocol change: `Welcome.world`, recorded in [proposal 0001](../proposals/0001-protocol.md), "World mode". No Lua API changes.

## Context

Procedural worlds generate their maps from the world seed, on the server and in the web client (strategy sections 8.1 to 8.5). The two sides must get the same grid on every machine. This step builds the core that later steps add terrain, dungeons and towns to:

- a versioned generator package on the seed tree of `shared/rng`,
- the world mode, chosen when the world is made,
- the generator in the client as WebAssembly,
- a check that one grid is the same in Go on arm64 and amd64, in TinyGo's WebAssembly in Node, and in Chrome, Firefox and WebKit.

## Decision

### The generator package

`shared/gen` holds the generators and their registry:

- `Generator` has a `Version`, the rng spec it uses (`RNG`, such as `rng-v1`), and `Grid(node, Params) (Grid, error)`.
- `Lookup(version)` finds any version the build has, so old versions stay callable. `Latest()` is the version new worlds get. `Versions()` lists them, oldest first.
- `Grid` has the cell types, 4 corner heights per cell in Q16.16, and the water level. `Grid.Checksum()` is the SHA-256 of a fixed encoding (spec, "Checksum").
- Version 1 is small on purpose: two octaves of value noise, a water level, gaps on steep cells and rocks. It proves the pipeline. Phase 7 step 2 adds biome terrain as version 2.
- A 400 × 400 grid with its checksum takes 18 ms in Go on an Apple M2.

### Rules and their guard

- `TestGuard` parses the source of `shared/gen`, `shared/gen/wasm` and `shared/rng`, without tests, and fails on:
  - `float32`, `float64`, `complex64` and `complex128`, and float or imaginary literals,
  - functions of package `math` other than its `Max` and `Min` limits,
  - map types, so no code can range over a map,
  - imports of `math/rand`, `time`, `os` and `unsafe`.
- `TestGuardFinds` checks that the guard reports each kind.
- The guard reads source, not types. A map that another package returns would pass it. The generators import only `shared/rng` and the standard library's hashing, so none does.

### The build tag

- `shared/gen` builds in both variants. With the tag `procedural`, `Procedural` is true and the registry holds the generators. Without it, the registry is empty and `Modes()` is only `classic`.
- `DefaultMode()` is `procedural` with the tag and `classic` without it, as strategy section 8.1 says.
- The generators, their golden test and the WebAssembly command build only with the tag. The types, the checksum and the registry functions build in both, so the server and `wactl` need no tags of their own.

### The world mode

- Migration 00029 adds `mode` (`classic` or `procedural`) and `rng_version` to the world row, beside the existing `generator_version`. A check ties them: a classic world has 0 and `''`, a procedural world a version and an `rng-vN`. The game role may read both columns. Worlds made before it are classic.
- `wactl world new --mode classic|procedural` chooses the mode, by default `gen.DefaultMode()`. A procedural world records the newest generator and its rng version. A `wactl` without the tag refuses `--mode procedural` and says to build with `-tags procedural`.
- `waserver` refuses to start on a world it cannot run: a procedural world in a build without the tag, a generator version the build lacks, or an rng version that is not the one the generator uses. A `--dev` server without a world runs classic.

### The protocol

- `Welcome.world` is a `WorldInfo` with the mode and a procedural world's generator and rng versions. A missing `world` is classic, so older servers and clients agree.
- Strategy section 8.1 said the server tells the client "which modes it supports". The world runs one mode, so `Welcome` names that mode and its versions instead. That is what the client needs to decide whether to load the module.
- The protocol version stays 1. `v1` can change freely before the first public release (proposal 0001, "Versions").

### The client

- `shared/gen/wasm` is the module, built with TinyGo for `wasm-unknown` with `-opt=2 -no-debug -tags procedural` into `client/src/world/gen.wasm` (93 KB), committed as `path.wasm` is. `scripts/check.sh tinygo` rebuilds it and fails when it differs.
- `client/src/world/generator.ts` loads it, checks its exports, and runs `generate` with a node key, returning the types, heights, water level and checksum.
- `generatorFor(world)` loads the module only for a procedural world, and fails when the module lacks the world's generator version or rng version.
- The viewer imports `generator.ts` dynamically after `Welcome`, only for a procedural world. A classic world never fetches `gen.wasm`. Nothing generates maps yet; step 2 does.

### The determinism check

The golden file `shared/gen/testdata/golden-v1.json` holds 8 maps: 1 × 1, 7 × 5, 64 × 64, 33 × 129, 200 × 150 with region indexes near 2^40, 400 × 400, 1024 × 8, and 48 × 48 with an all-zero key. Each has its key, water level, counts of each cell type, and checksum. They run on each platform:

| Platform | Where | Result |
|---|---|---|
| Go, darwin/arm64 | `scripts/check.sh gen` and `unit`, `TestGolden` | 8 of 8 match |
| Go, darwin/amd64 under Rosetta | `scripts/check.sh gen`: the test binary built with `GOARCH=amd64` | 8 of 8 match |
| TinyGo, native and wasip1 in wasmtime | `scripts/check.sh tinygo` | 8 of 8 match |
| `gen.wasm` in Node 26 | `scripts/check.sh gen` and `client`, `generator.test.ts` | 8 of 8 match |
| `gen.wasm` in Chromium, Firefox and WebKit | `scripts/check.sh browsers`, the check `gen` | 8 of 8 match in each |

- The browser check loads the client's own `generator.ts` from the Vite server in each browser, so the loader is tested as well. It runs in all three browsers whatever `BROWSERS` says. A browser that does not start is skipped with a message.
- On a Linux amd64 machine, the `gen` step builds the arm64 binary and skips it with a message unless the machine can run it. On a Mac without Rosetta it says to install Rosetta.
- The 400 × 400 map took 32 to 35 ms in Chromium and WebKit and 315 ms in Firefox, against 18 ms in Go.
- A changed checksum fails `TestGolden`. `go test -tags procedural ./shared/gen -run TestGolden -update` rewrites only the newest version's file, and only for a version that no world uses yet.

### Both build variants in check.sh

- `vet` runs `go vet ./...` without and with `-tags procedural`.
- `unit` and `test` run every package without the tag, then `go test -tags procedural` on the packages the tag changes: `shared/gen` and the packages that import it (`server/cmd/waserver`, `tools/wactl`). Other packages build the same either way.
- `lint` runs golangci-lint on everything, then with `--build-tags procedural` on the same packages.
- `tinygo` runs the shared packages without and with the tag, and checks both modules.
- `gen` is a new default step with the platform runs above.

## Lua API

None. `wa.generation` comes with phase 7 step 6.

## Left

- Which public node a map uses, and how the server sends it with the checksum, are phase 7 step 2. So is the download of a map when the checksums differ or the client lacks the version.
- `waserver` does not generate maps yet, and the game role still cannot read the world seed. Step 2 decides how the server derives public nodes.
- Strategy section 8.1 asks that CI build both variants on every pull request. The repository has no CI (decision 0026), so `scripts/check.sh` does it.
- The Dockerfile and the release builds in `scripts/check.sh lua` build without the tag, as classic-only servers. A procedural release build waits until procedural mode is playable.
- The determinism check ran on one Mac. A run on a real amd64 machine and in Safari on macOS and iOS is still to do. Playwright's WebKit is the same engine, not Safari.
