# 0002: C Lua Through Our Own cgo Binding

- Status: accepted.
- Date: 2026-09-29. Updated on 2026-10-01 with the production binding (see "Production binding"), and on 2026-10-02 with nested calls (see "Nested calls").
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

## Production binding

Phase 3 step 1 built the binding into [internal/script](../../internal/script). The decision stands. These parts differ from the prototype, for the reasons given.

- **Lua is vendored.** Lua 5.4.9 is in `internal/script/lua/src`, checked against the SHA-256 of the release tarball, and compiled in one translation unit with the binding. The prototype linked Homebrew's library.
- **Lua uses `_setjmp`.** Plain `setjmp` saves the signal mask with a system call on macOS. It made an empty call cost 300 ns instead of 75 ns. Homebrew builds Lua with `LUA_USE_POSIX`, so the prototype did not have this problem.
- **Host functions are generic.** Go registers any number of host functions. Each is a C closure with an ID. Arguments and results pass through an array in C memory, so each call still crosses cgo once. A call with no arguments costs 66 ns, as before. Two integer arguments add about 30 ns, for decoding and a deferred `recover` that turns Go panics into Lua errors.
- **The CPU budget sees work in C.** The count hook only counts instructions. One `string.find` with a backtracking pattern, or a loop over `table.sort`, could run for hours inside the prototype's budget. Now:
  - Allocations cost 1 unit per 64 bytes, and the allocator refuses once the budget is gone.
  - A patched copy of `lstrlib.c` charges pattern matching and plain `find`. The patch is 8 lines, and a test checks it against the original file.
  - `table.sort`, `table.insert`, `table.unpack`, `string.byte`, `tonumber`, `utf8.len`, `load`, and others are charged for the work they do in C.
- **Limits cannot be caught.** `pcall`, `xpcall`, `coroutine.resume`, and `require` raise a hit limit again. The prototype's `pcall` could catch the CPU error and loop. An error from the count hook runs the `xpcall` message handler while Lua has hooks off, so the handler does not run once a limit is hit.
- **A time limit backs up the budget.** It is 1 second per call by default. Comparing two long strings is one instruction that does not allocate, so only the clock stops a loop of them. The hook reads a cheap clock: `mach_absolute_time` on macOS and the coarse monotonic clock on Linux.
- **Memory is limited per state.** The decision said "per call", but the allocator limits what the state holds. What one call allocates in total is bounded by the CPU budget.
- **Static Linux builds set an 8 MB thread stack.** Lua's limit of 200 nested C calls needs up to 512 KB of C stack, in `string.gsub`. musl's default thread stack is 128 KB. zig's linker happens to set 16 MB, which musl caps at 8 MB, and the build flags now set 8 MB on purpose. `waserver` checks deep recursion at startup.
- **macOS builds need the SDK.** `zig cc` links macOS binaries against system frameworks that Go's packages use. CI builds darwin/arm64 with `zig cc` on a macOS runner. Linux and Windows cross-compile from Linux.
- **`lua.wasm` is built without `wasm-opt`.** clang runs `wasm-opt` when it finds one on PATH. Without it the output is the same on every machine, so CI rebuilds it and compares.

Time per operation on the same Apple M2, with default limits. The prototype was run again on the same day, and matched the table above within 5%.

| Workload | Prototype | Production, cgo | Production, wazero |
|---|---:|---:|---:|
| `static_equip` | 224 ns | 316 ns | 1.54 µs |
| `computed` | 168 ns | 246 ns | 1.72 µs |
| `hit_taken` | 171 ns | 256 ns | 1.48 µs |
| `npc_dialog` | 2.07 µs | 2.32 µs | 137 µs |
| `npc_dialog`, paused by host functions | none | 2.72 µs | 110 µs |
| `equip_with_require` | 361 ns | 449 ns | 2.40 µs |
| `compute_1000` | 262 µs | 146 µs | 344 µs |
| `new_state` | 1.10 ms | 0.95 ms | 3.26 ms |
| `load_1000_items` | 4.03 ms | 3.66 ms | 11.3 ms |
| `mod_state_200_items` | 1.05 ms | 0.91 ms | 3.09 ms |
| Empty call | 73 ns | 84 ns | 1.12 µs |
| Call from Lua into Go | 66 ns | 66 ns | 121 ns |

- Item and proc calls cost 80 to 90 ns more than the prototype's, because of the generic host functions. They are still cheaper than every other runtime measured above: golua took 748 to 879 ns, and gopher-lua 536 to 654 ns.
- Loading and pure Lua work are faster, since Lua is compiled in one unit with the binding instead of linked as a shared library.
- `npc_dialog`, paused by host functions, is the dialog as NPC scripts will run it: a thread that Go resumes, with `mes` and `menu` as host functions that pause it.
- The infinite loop stops after 21 ms and the 1 GB table after 18 ms, as with the prototype.
- A state with 1,000 items uses 606 KB, and one mod of 200 items 155 KB.

## Nested calls

Added on 2026-10-02 for status effects ([decision 0010](0010-status-effects.md)).

When a script starts or ends a status, rAthena computes the player's stats at once, inside `status_change_start` and `status_change_end`. In wAthena that runs computed bonuses and `when` conditions, which are Lua, from inside the script's host call. A host function may now make that one kind of call:

- `State.Call1Nested` runs on the thread that called the host function, inside the outer call. wAthena uses it only to compute stats.
- It shares the outer call's CPU budget, time limit, and memory limit. A limit it hits stops the outer call when the host function returns.
- It cannot yield: it runs under `lua_pcall`, so a yield raises an error instead of pausing the outer thread.
- Errors stay inside it, so no Lua error crosses a Go frame. This keeps it safe in wazero, which cannot unwind through a host function.
- Every other call into the state from a host function still gets `ErrReentrant`.

The rules are in [internal/script/README.md](../../internal/script/README.md), "Nested calls". The cgo build and `lua.wasm` run the same tests.

## Follow-up

- Done in phase 3 step 1: the binding in `internal/script`, fuzz tests on `glue.c`, and `zig cc` release builds in CI.
- Report golua's memory accounting panic upstream anyway, since other Go projects use it.
