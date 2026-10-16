# Map Generator Versions, generator-v1

This spec fixes how a procedural world names its map generator, what a generator returns, and how a grid's checksum is computed. A client that generates maps itself, such as the web client or a client from strategy section 10.7, follows it to get the same grid as the server.

- The Go implementation is `shared/gen`. The web client runs the same code as `client/src/world/gen.wasm`, built with TinyGo.
- The seed tree and random streams are in [rng-v1](rng-v1.md).
- The golden checksums are in [`shared/gen/testdata`](../../shared/gen/testdata), one file per generator version. Every implementation must match them.
- Decision [0038](../decisions/0038-generator-core.md) has the reasons.
- Version 2, generated fields, is in [generator-v2](generator-v2.md). It adds a biome input, the public branch of the seed tree, and the module's field exports.

## World mode and versions

A world records three values when it is made, and they never change:

| Value | Classic world | Procedural world |
|---|---|---|
| mode | `classic` | `procedural` |
| generator version | 0 | 1 or more |
| rng version | empty | such as `rng-v1` |

- `Welcome.world` sends them to the client at login ([proposal 0001](../proposals/0001-protocol.md)). A server that sends no mode runs a classic world.
- A generator version never changes. A change to any output, checksum included, needs a new version number. Old versions stay until no world uses them.
- Each generator version names the rng spec version it uses. A world's rng version must be the one its generator version names.
- A client that lacks a world's generator version, or whose module names another rng version for it, does not generate that world's maps. It downloads them from the server ([generator-v2](generator-v2.md), "Client").

## Rules for generators

- Integer and Q16.16 fixed-point math only. Floating point can give different results on different machines.
- No iteration over hash maps or other unordered sets.
- Any randomness comes from the seed tree: child nodes, the public stream, and the stateless hash of rng-v1. Never from the clock or the machine.

## Inputs

- A public node: a 32-byte key from the seed tree. The server sends a map's public node to the client, never the world root or a secret branch. A field's node is `child(child(root, "public"), "field", x, y)` ([generator-v2](generator-v2.md), "Seed tree").
- The map's width and height, each from 1 to 1024 cells.

## Grid

- Cells go row by row from y = 0, the south edge, as in the WMAP format ([assets-v1](assets-v1.md), "Map (WMAP)").
- Each cell has a type, from RO's GAT values:

  | Type | Meaning | WMAP flags |
  |---|---|---|
  | 0 | walkable | walkable, shootable |
  | 1 | blocked | none |
  | 3 | walkable water | walkable, shootable, water |
  | 5 | gap: blocks walking, not projectiles | shootable |

- Each cell has 4 corner heights as Q16.16 values in cells, up positive, in WMAP's order: (x, y), (x+1, y), (x, y+1), (x+1, y+1). A WMAP writer converts them to `f32` by dividing by 65536.
- The grid has a water level, a Q16.16 value in cells, up positive.

## Checksum

The checksum is the SHA-256 of this byte string:

```text
"wa-grid-v1"
BE16(width) || BE16(height)
BE32(water level)
for each cell, row by row from y = 0:
    type (1 byte) || BE32(h0) || BE32(h1) || BE32(h2) || BE32(h3)
```

- `BE16` and `BE32` are big-endian. Heights and the water level are encoded as 32-bit two's complement.
- The checksum is written as 64 lowercase hex digits.

## Version 1

Version 1 is a small grid that proves the pipeline and its determinism. It uses rng-v1. Q16.16 math follows rng-v1, "Fixed point": `mul(a, b) = (a * b) >> 16` with an arithmetic shift.

From the public node `n`, it derives three children: `height = child(n, "height")`, `water = child(n, "water")` and `objects = child(n, "objects")`.

1. **Water level.** The first value of `water`'s public stream, as `Range(-65536, 32768)` of rng-v1.
2. **Lattice values.** For lattice point (lx, ly) of octave `o` with amplitude `a`:

   ```text
   unit = hash(height, lx, ly, o) >> 47      a Q16.16 value in [0, 2)
   value = mul(unit, a) - a
   ```

3. **Vertex heights.** Vertex (x, y), for x from 0 to width and y from 0 to height, is the sum of two octaves:

   | Octave `o` | Spacing `s` (`shift`) | Amplitude `a` |
   |---|---|---|
   | 0 | 32 cells (5) | 4.0 (262144) |
   | 1 | 8 cells (3) | 1.0 (65536) |

   Each octave interpolates its lattice bilinearly:

   ```text
   lx = x >> shift, ly = y >> shift
   tx = (x & (s - 1)) << (16 - shift), ty likewise
   south = v(lx, ly) + mul(v(lx+1, ly) - v(lx, ly), tx)
   north = v(lx, ly+1) + mul(v(lx+1, ly+1) - v(lx, ly+1), tx)
   octave = south + mul(north - south, ty)
   ```

   Additions and subtractions wrap to 32 bits, as Q16.16 values do.
4. **Cells.** Cell (x, y) takes the heights of vertices (x, y), (x+1, y), (x, y+1) and (x+1, y+1). Its type is the first that applies:
   - The mean of the 4 corners, `(h0 + h1 + h2 + h3) >> 2` in 64-bit math, is below the water level minus 1.0: blocked (1).
   - The mean is below the water level: walkable water (3).
   - The highest corner minus the lowest is above 0.5 (32768): gap (5).
   - `hash(objects, x, y, 0) mod 64` is 0: blocked (1), a rock.
   - Otherwise walkable (0).

## Golden files

`shared/gen/testdata/golden-v<version>.json` holds:

- `version` and `rng`: the generator version and its rng spec.
- `maps`: each map's `key` (the public node, hex), `width`, `height`, `water` (the water level as an integer), `counts` (the cells of types 0, 1, 3 and 5), and `checksum`.
- A map with `secret` and `region` has the key `child(child(root(secret), "region", x, y), "terrain")`. The Go tests check that derivation too.

The maps cover one cell, odd sizes, 400 × 400, a 1024-cell side, region indexes near 2^40, and an all-zero key.

## WebAssembly module

`gen.wasm` is built for `wasm-unknown` and imports nothing. It exports its memory, `_initialize`, and the functions below. TinyGo's runtime adds a few other exports, which clients ignore.

| Export | Use |
|---|---|
| `key() → ptr` | The 32-byte buffer for the public node's key. |
| `generate(version, width, height, biome) → code` | Makes the grid: 0, -1 for a version the module lacks, -2 for a size outside 1 to 1024. Version 1 ignores the biome. [generator-v2](generator-v2.md) lists the other codes and exports. |
| `types() → ptr` | Width × height cell types. |
| `heights() → ptr` | 4 × width × height `i32` corner heights, little-endian. |
| `water() → i32` | The water level. |
| `checksum() → ptr` | The 32-byte checksum. |
| `latest() → i32` | The newest generator version in the module. |
| `rng_version(version) → n` | The rng spec version `rng-v<n>` the generator version uses, or 0 when the module lacks it. |

Buffers stay valid until the next call to `generate`. Memory can grow during `generate`, so a caller makes its views after the call.
