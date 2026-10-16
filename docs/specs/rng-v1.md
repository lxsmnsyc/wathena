# Seed Tree and Random Streams, rng-v1

This spec fixes every algorithm that turns a world secret into generated content. Two implementations that follow it give the same bytes for the same inputs.

- Version: `rng-v1`. Each world records the version it was created with.
- An algorithm never changes within a version. A change needs a design proposal and a new version.
- The Go implementation is `shared/rng`.
- The test vectors are in [`shared/rng/testdata/rng-v1.json`](../../shared/rng/testdata/rng-v1.json). Every implementation must pass them.

## Conventions

- `u64` values wrap modulo 2^64. `int64` values are converted to `u64` as two's complement.
- `BE32(v)` and `BE64(v)` are big-endian encodings. `LE64(b)` reads 8 bytes as a little-endian `u64`.
- `HMAC(key, msg)` is HMAC-SHA256. Its output is 32 bytes.
- `||` joins byte strings.
- Strings are encoded as UTF-8 bytes.

## Nodes

A node is a 32-byte key.

### Root

```text
root = HMAC("wa-seed-v1-root", secret)
```

- `secret` is the world secret, a non-empty byte string.
- The world secret is generated with a secure random source and stored on the server. It is never sent to clients.

### Child

```text
child(parent, label, i1, ..., ik) =
    HMAC(parent, "wa-seed-v1" || BE32(len(label)) || label || BE64(i1) || ... || BE64(ik))
```

- `len(label)` is the length of the label in bytes. It must be below 2^32.
- Any number of indexes can follow the label, including none.
- The length prefix stops `("ab")` and `("a", ...)` from giving the same message. Indexes have a fixed size, so the message has only one reading.
- A child key does not reveal its parent's key or its siblings' keys.

Example path: `root → region(3, -1) → terrain`.

### Labels in use

Labels are part of the spec, because changing one changes every key below it. New labels can be added in a version. Existing labels cannot change.

| Label | Indexes | Use |
|---|---|---|
| `region` | x, y | A world region. |
| `town` | town number | A town. |
| `dungeon:<name>` | date as `YYYYMMDD`, then optional numbers | A dungeon, such as `dungeon:crypt` for 2026-10-31. |
| `floor` | floor number | A dungeon floor. |
| `terrain`, `layout`, `decoration` | none | Public generation steps. |
| `secret` | none | The root of a secret branch. |
| `zone:<map ID>` | none | The secret stream of a classic map's zone, such as `zone:prontera`. It places and moves the map's monsters and rolls its drops and hits. Only the server uses it. |
| `spawns`, `loot`, `puzzles`, `npcs`, `quests` | none | Steps under `secret`. |
| `height`, `water`, `objects` | none | Steps of map generator version 1 below a map's public node ([generator-v1](generator-v1.md)). |

## Streams

### SplitMix64

```text
next():
    state = state + 0x9E3779B97F4A7C15
    return mix64(state)

mix64(z):
    z = (z XOR (z >> 30)) * 0xBF58476D1CE4E5B9
    z = (z XOR (z >> 27)) * 0x94D049BB133111EB
    return z XOR (z >> 31)
```

### Public stream: xoshiro256**

Seeding from a node key:

```text
s[i] = LE64(key[8i .. 8i+8]) for i = 0..3
```

If all four words are 0, they are replaced by the first four outputs of SplitMix64 with state 0. xoshiro cannot use a zero state.

Each output:

```text
result = rotl(s[1] * 5, 7) * 9
t = s[1] << 17
s[2] ^= s[0]
s[3] ^= s[1]
s[1] ^= s[2]
s[0] ^= s[3]
s[2] ^= t
s[3] = rotl(s[3], 45)
return result
```

Public streams are used for content the client can also generate, such as terrain and layout.

### Secret stream: ChaCha8

- The stream is ChaCha8 as defined by the [C2SP chacha8rand spec](https://c2sp.org/chacha8rand), seeded with the node key.
- Each output is one `u64` from that spec.
- Secret streams run on the server only. Players cannot recover the state from outcomes they see.

## Stateless hash

`hash(node, x, y, z)` gives a value for a coordinate without a stream. Any cell can be computed alone, in any order.

```text
k = LE64(key[0..8]) XOR LE64(key[8..16]) XOR LE64(key[16..24]) XOR LE64(key[24..32])
h = mix64(k XOR (x * 0x9E3779B97F4A7C15))
h = mix64(h XOR (y * 0xC2B2AE3D27D4EB4F))
h = mix64(h XOR (z * 0x165667B19E3779F9))
return h
```

Each coordinate is mixed in turn, so nearby coordinates give unrelated values.

## Helpers

All helpers read from any stream. Each read is one `u64` output.

### Bounded integer

`uint64n(n)` returns a value in `[0, n)` with Lemire's multiply-and-reject method. `n` must not be 0.

```text
(hi, lo) = 128-bit product of next() and n
if lo < n:
    threshold = (2^64 - n) mod n
    while lo < threshold:
        (hi, lo) = 128-bit product of next() and n
return hi
```

The rejection rule is part of the spec, because each rejection reads one more value.

### Range

`range(lo, hi)` returns a value in `[lo, hi]`. `lo` must not be above `hi`.

```text
span = u64(hi) - u64(lo) + 1
if span == 0:
    return int64(next())
return lo + int64(uint64n(span))
```

### Shuffle

Fisher-Yates from the last element down:

```text
for i = n - 1 down to 1:
    j = uint64n(i + 1)
    swap elements i and j
```

### Weighted choice

`weighted(w)` takes non-negative integer weights with a total above 0.

```text
v = uint64n(sum of w)
return the first index i where w[0] + ... + w[i] > v
```

The alias method is not used, because building its table in a different order changes results.

### Chance

`chance(p)` takes a Q16.16 probability, where 65536 is 1.

```text
return uint64n(65536) < max(p, 0)
```

## Fixed point

Generation uses no floating point. Fractions are Q16.16: a signed 32-bit integer with 16 fraction bits.

| Operation | Rule |
|---|---|
| `mul(a, b)` | `(int64(a) * int64(b)) >> 16`, with an arithmetic shift. This rounds toward negative infinity. |
| `div(a, b)` | `(int64(a) << 16) / int64(b)`, rounded toward zero. `b` must not be 0. |

Results outside the 32-bit range wrap to 32 bits.

## Test vectors

The vector file has one list for each part of this spec. Each case holds its inputs and the expected outputs.

- 64-bit values are 16-digit lowercase hex strings.
- Keys and messages are lowercase hex.
- `uint64n` cases include `reads`, the number of stream outputs used by 8 calls. Cases with `n = 2^63 + 1` reject about half of all values, so they test the rejection rule.
- `chacha8` cases are checked on the server build only.

The expected values were checked against a second implementation written apart from the Go code. The xoshiro256** and SplitMix64 outputs also match the values published by their authors.
