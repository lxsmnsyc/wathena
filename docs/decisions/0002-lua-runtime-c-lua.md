# 0002: C Lua Through Our Own cgo Binding

- Status: accepted.
- Date: 2026-09-29.
- Supersedes: [0001](0001-lua-runtime.md).

## Decision

wAthena runs C Lua 5.4 through a small cgo binding of its own.

- The binding is one C file, `glue.c`, with a Go wrapper. The benchmark version is [tools/luabench/glue](../../tools/luabench/glue).
- Each state has its own allocator with a memory limit, and a count hook that charges a CPU budget per call.
- Host functions take plain integers and strings, so each call from Lua into Go crosses cgo once.
- Only the base, coroutine, table, string, math, and utf8 libraries are opened. `require` is replaced by a loader that reads module sources from the mod, not from files.
- `glue.c` also builds to WebAssembly and runs in wazero with no changes (see "WebAssembly"). This keeps a pure-Go option open.

## Why

The rule agreed before the run was:

1. The runtime must pass every check with limits on: identical results, the NPC dialog, the infinite loop, and the 1 GB table.
2. Among those that pass, the lowest per-call cost on item and proc workloads wins.

Only two configurations passed every check: C Lua through our binding and C Lua on wazero. C Lua through our binding was faster on every workload:

- Item and proc calls cost 175 to 235 ns with limits on, against 690 to 880 ns for golua with only a CPU limit.
- The NPC dialog costs 2.1 µs, against 14.4 µs for golua.
- Loading 1,000 item scripts takes 4.0 ms, against 22.9 ms for golua.
- A state with 1,000 items uses 0.6 MB, against 1.8 MB of Go heap for golua.
- A call into Lua costs 73 ns, and each call back into Go about 66 ns.

golua with both limits fails the NPC dialog, as found in decision 0001.

## Costs of cgo and how they are handled

| Cost | Handling |
|---|---|
| Cross-compiling needs a C compiler for each target. | Release builds use `zig cc` as the C compiler. It targets Linux (static with musl), Windows, and macOS from one machine. CI builds every release target. |
| A crash in C takes down the process. | Lua's C code is mature and widely used. `glue.c` is small and has fuzz tests. Scripts cannot load bytecode or C libraries. |
| The race detector and Go profiler cannot see inside C. | Lua states are used by one zone goroutine at a time, so there is no shared C state to race on. Lua time shows in profiles as cgo calls. |
| A cgo call holds an OS thread while it runs. | Script calls are short, and the CPU limit bounds them. |
| Lua is built from source. | The Lua version and its SHA-256 are pinned. The build script checks both. |

## Benchmark

Tool: [tools/luabench](../../tools/luabench). Raw results: [tools/luabench/results/2026-09-29-apple-m2.json](../../tools/luabench/results/2026-09-29-apple-m2.json).

Setup:

- Apple M2, 8 GB, macOS, Go 1.27.1.
- C Lua 5.4.9, arnodel/golua v0.3.0, yuin/gopher-lua v1.1.2, wazero v1.12.0, aarzilli/golua (latest).
- "Limits" means a CPU limit of 10,000,000 instructions (a 50 ms timeout for gopher-lua) and 16 MB of memory, per call.
- All engines give the same results on every workload they can run.

Workloads, in addition to those in decision 0001:

| Workload | What it runs |
|---|---|
| `equip_with_require` | An item handler that calls a function from a shared module loaded with `require`. |
| `mod_state_200_items` | Create a state and load one mod: a shared module of 51 functions and 200 item scripts that require it. |
| `new_state` | Now also loads that mod, so it is the full cost of a ready state. |

Time per operation, with limits on:

| Workload | C Lua, own binding | C Lua, wazero | golua, CPU limit only | gopher-lua | C Lua, aarzilli |
|---|---:|---:|---:|---:|---:|
| `static_equip` | 235 ns | 1.45 µs | 879 ns | 536 ns | 914 ns |
| `computed` | 174 ns | 1.34 µs | 748 ns | 598 ns | 681 ns |
| `hit_taken` | 181 ns | 1.36 µs | 806 ns | 654 ns | 740 ns |
| `npc_dialog` | 2.10 µs | 211 µs | 14.4 µs | failed | failed |
| `equip_with_require` | 367 ns | 1.73 µs | 1.67 µs | 951 ns | 1.04 µs |
| `compute_1000` | 252 µs | 369 µs | 566 µs | 390 µs | 259 µs |
| `new_state` | 1.12 ms | 3.11 ms | 5.93 ms | 4.69 ms | 1.12 ms |
| `load_1000_items` | 4.00 ms | 11.7 ms | 22.9 ms | 17.2 ms | 4.07 ms |
| `mod_state_200_items` | 1.08 ms | 3.08 ms | 5.69 ms | 4.22 ms | 1.07 ms |

golua with both limits matched the CPU-only numbers where it ran, and failed the dialog.

Memory after load:

| Engine | 1,000 items | One mod, 200 items | Measure |
|---|---:|---:|---|
| C Lua, own binding | 605 KB | 159 KB | Lua allocator |
| C Lua, wazero | 453 KB | 121 KB | Lua allocator, 32-bit pointers |
| C Lua, wazero | 1,174 KB | 546 KB | Go heap, including WebAssembly memory |
| golua | 1,774 KB | 523 KB | Go heap |
| gopher-lua | 22,481 KB | 4,802 KB | Go heap, default options |

Sandbox checks, with limits on:

| Engine | Infinite loop | 1 GB table | NPC dialog |
|---|---|---|---|
| C Lua, own binding | Stopped after 20 ms. | Stopped after 17 ms. | Works. |
| C Lua, wazero | Stopped after 82 ms. | Stopped after 67 ms. | Works. |
| golua | Stopped after 53 ms. | Stopped after 103 ms. | Panics with the memory limit. Works with the CPU limit only. |
| gopher-lua | Stopped after 50 ms. | No limit. | Fails after the first call. |
| C Lua, aarzilli | Stopped after 21 ms. | No limit. | Go functions get the wrong state inside coroutines. |

## WebAssembly

C Lua on wazero passed every check and needs no cgo. It is slower for these reasons:

- Lua's errors and coroutine yields use setjmp and longjmp. In WebAssembly these become exceptions, which wazero supports only as an experimental feature.
- Measured on a function that does nothing: a protected call costs about 1.1 µs, against 73 ns through cgo.
- A coroutine yield costs about 17 µs, and a caught error about 65 µs.
- A call from Lua into Go costs about 90 ns, close to cgo.

An earlier attempt made Go handle Lua's errors directly. It failed because wazero cannot recover a Go panic inside a nested call and then continue the outer call.

wazero is worth measuring again when its exception support improves. `glue.c` is shared, so only the Go side of the binding would change. wasi-sdk 34 builds `lua.wasm` with [tools/luabench/wasm/build.sh](../../tools/luabench/wasm/build.sh).

## What the binding does differently from aarzilli/golua

- Go functions receive the `lua_State` of the running coroutine, not the main thread.
- Host functions are typed C functions, not a Go function registry looked up on each call.
- Limits live in `glue.c`: an allocator with a byte limit, and a hook every 1,000 instructions that charges a per-call budget. New coroutines inherit the hook.

## Follow-up

- Move the binding into `internal/script` with fuzz tests on `glue.c`.
- Add `zig cc` cross-compiling for release targets to CI.
- Report golua's memory accounting panic upstream anyway, since other Go projects use it.
