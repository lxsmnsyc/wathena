# shared

Go package compiled to WebAssembly with TinyGo for the web client: pathfinding, cell checks, and map generation. See docs/strategy.md, section 3.3.

Packages:

- `rng`: the seed tree and random streams in [docs/specs/rng-v1.md](../docs/specs/rng-v1.md).
- `path`: walk paths, lines of sight, and walk timing, ported from rAthena's `src/map/path.cpp`. Its tests run the 9,000 recorded queries in `reference/rathena/*/pathfinding.json` on the six recorded maps, whose cells are in `path/testdata`. `path/wasm` builds it for the web client:

  ```text
  tinygo build -o client/src/world/path.wasm -target=wasm-unknown -opt=2 -no-debug ./shared/path/wasm
  ```

  The client's tests run the same queries through the built module.
