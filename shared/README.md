# shared

Go package compiled to WebAssembly with TinyGo for the web client: pathfinding, cell checks, and map generation. See docs/strategy.md, section 3.3.

Packages:

- `rng`: the seed tree and random streams in [docs/specs/rng-v1.md](../docs/specs/rng-v1.md).
- `path`: walk paths, lines of sight, and walk timing, ported from rAthena's `src/map/path.cpp`. Its tests run the 9,000 recorded queries in `reference/rathena/*/pathfinding.json` on the six recorded maps, whose cells are in `path/testdata`. `path/wasm` builds it for the web client:

  ```text
  tinygo build -o client/src/world/path.wasm -target=wasm-unknown -opt=2 -no-debug ./shared/path/wasm
  ```

  The client's tests run the same queries through the built module.
- `gen`: the map generators of procedural worlds, their version registry and the grid checksum in [docs/specs/generator-v1.md](../docs/specs/generator-v1.md), and version 2's generated fields with their WMAP in [docs/specs/generator-v2.md](../docs/specs/generator-v2.md). The generators build only with the tag `procedural`. `TestGuard` fails on floating point, map types and clock or machine imports in `gen` and `rng`. `TestGolden` checks the checksums in `gen/testdata`. `gen/wasm` builds it for the web client:

  ```text
  tinygo build -o client/src/world/gen.wasm -target=wasm-unknown -opt=2 -no-debug -tags procedural ./shared/gen/wasm
  ```

  `scripts/check.sh gen` runs the golden maps and the field tests in Go on arm64 and amd64 and the golden maps in the module in Node, and `scripts/check.sh browsers` in Chromium, Firefox and WebKit.
