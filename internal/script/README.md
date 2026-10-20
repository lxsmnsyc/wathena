# script

The Lua runtime: C Lua 5.4 through wAthena's own binding ([decision 0002](../../docs/decisions/0002-lua-runtime-c-lua.md)). All server code talks to Lua through this package (strategy section 3.2). The public `wa.*` API will be built on top of it.

## Layout

| Path | Content |
|---|---|
| `lua/src` | Lua 5.4.9, the files the build uses, unchanged. `lua/update.sh` vendors them from the release tarball, whose SHA-256 it pins. `lua/SHA256SUMS` lists each file's hash, and `lua/LICENSE` is Lua's MIT license. |
| `csrc/glue.c` | The binding. The cgo build and `lua.wasm` both compile it. |
| `csrc/glue_abi.h` | The layouts Go and C share. |
| `csrc/lstrlib_charged.c` | Lua's string library, changed to charge pattern matching to the CPU budget. `vendor_test.go` lists each change and checks the file against the original. |
| `csrc/all.c` | Lua and the binding in one translation unit. |
| `lua_cgo.c`, `engine_cgo.go` | The cgo build. |
| `wasm/lua.wasm`, `engine_wazero.go` | The WebAssembly build, which runs in wazero. `wasm/build.sh` makes `lua.wasm` with wasi-sdk 34. |

## API

- `New(Options)` makes a state with limits and a source of modules for `require`.
- `Load` runs a chunk of source and returns what it returns. `Call` and `Call1` call a function or a global by name.
- `Register` and `Func` make Lua functions from Go functions. A host function reads its arguments from `*Call`, returns results with `Return`, and raises a Lua error by returning an error. A panic becomes a Lua error too.
- `Call.Yield` pauses the thread that Go resumed. `NewThread` and `Thread.Resume` run a script that pauses for the player, such as an NPC dialog.
- Values are `Value`: nil, booleans, integers, floats, strings, tables (copied, up to 16 levels deep), and handles to functions, threads, and userdata. A handle from a call is valid until the next call. `Keep` turns it into a `Ref` that lasts until `Release`.
- Refs and threads that are garbage collected without `Release` or `Close` are freed at a later call.

A state is used by one goroutine at a time, as each zone owns its own (strategy section 4.1). A second goroutine that enters while Lua runs makes the state panic. A host function cannot call `Call`, `Load`, or `Resume` on its own state. It gets `ErrReentrant`. It can call `Keep`, `Release`, `SetGlobal`, `Func`, `Register`, and `Call1Nested`.

## Nested calls

`Call1Nested` lets a host function call Lua on its own state. wAthena uses it for two things. Every other call from a host function still gets `ErrReentrant`.

- Computing a player's stats again, which runs computed bonuses and `when` conditions, inside a script's call that changed them (decision 0010).
- The `item_roll` hooks of a copy that a script makes inside its call, such as `player:give_item` with `roll = "random"` or `equipped:reroll` (decision 0014).

Rules:

- It runs on the thread that called the host function, inside the outer call. It is not a new call.
- The outer call's CPU budget, time limit, and memory limit apply to it.
- When it hits a limit, it returns the error. The outer call then stops with that limit when the host function returns, even if the host function ignores the error. `pcall` cannot catch it.
- It cannot pause. `Call.Yield` and `coroutine.yield` on the outer thread raise a Lua error inside it. Coroutines it creates itself can yield.
- Handles from the host function's arguments expire after it, since its host calls reuse the same slots. Keep them first.
- At most `MaxNested` (8) nested calls can be open at once.
- Lua errors inside it, even running out of memory while it reads its arguments, are caught inside it. None crosses the Go frame of the host function, which matters for `lua.wasm`.

## Sandbox

- Libraries: base, coroutine, math, string, table, and utf8. The debug, io, os, and package libraries are not compiled in.
- Removed: `dofile`, `loadfile`, `print`, `warn`, `string.dump`, `math.random`, and `math.randomseed`. Scripts get random numbers from the host, so seeded tests repeat (strategy section 5.5).
- `load` and `Load` accept source text only. Bytecode is refused before it is parsed.
- `require` loads modules from `Options.Modules`, never from files. A module loads once per state. A module that requires itself while it loads is an error. A module that fails can be loaded again.
- `setmetatable` refuses tables with `__gc`, so no script code runs during garbage collection.
- `collectgarbage` only accepts `"count"`.
- `getmetatable("")` returns `false`, so scripts cannot change string methods for others.

The environment of each mod, and read-only shared tables (strategy section 6.6), are built on top of this package by [server/modscript](../../server/modscript), with `load` and an environment table ([decision 0006](../../docs/decisions/0006-mod-loader.md)). Chunks that `Load` and this package's `require` run share the globals.

## Limits

| Limit | Default | Applies to | Mechanism |
|---|---|---|---|
| CPU | 10,000,000 | each call and each resume | A count hook every 1,000 instructions. Allocations cost 1 per 64 bytes. Pattern matching, plain `find`, `table.sort`, `table.insert`, and other work in C is charged too. |
| Memory | 16 MB | the state | The allocator refuses to grow past it. |
| Owner memory | none | each owner of memory | `SetOwnerLimit`. The allocator refuses to grow an owner past it while a call runs. |
| Time | 30 seconds | each call and each resume | A wall-clock backstop for a call stuck in work the CPU budget does not see, such as comparing long strings over and over. The hook reads the clock from the first 1,000 instructions on. It is long because a machine that stalls stretches every call's clock time (decision 0054). The CPU budget is what limits a script's work. |

## Owners of memory

A state has `MaxOwners` (512) owners of memory, so one state can hold the code of many mods, each with its own limit (server/modscript, review decision 52).

- `SetOwner` sets the owner of the code that runs. A host function may call it, so a call can switch owners, such as between the hooks of one chain. Go writes it to the state's header, without a call into C.
- Each block of memory starts with 8 bytes that hold its owner. A block that grows belongs to the owner that runs, which pays for all of it. A block that shrinks or is freed keeps its owner, so a collection while another owner runs gives the memory back to the right owner.
- `OwnerMemory` returns an owner's bytes. The owners' bytes add up to `Memory`, which does not count the 8 bytes of each block.
- `SetOwnerLimit` limits an owner while a call runs. A call that would go past it stops with `ErrMemoryLimit`. Lua first collects garbage and tries again, so garbage does not count against an owner for long.
- Owner 0 holds everything no other owner does. An owner out of range is owner 0.
- `Spent` counts the CPU units charged since the state began, from every source of the per-call budget. Go reads it at each switch of owners to give each mod a budget per tick. The count hook charges 1,000 units at a time, so a short call may show none, and the next call that reaches the count pays for it.

When a limit stops a call:

- The call returns an `*Error` that matches `ErrCPULimit`, `ErrMemoryLimit`, or `ErrTimeLimit` with `errors.Is`, and names the line.
- `pcall`, `xpcall`, `coroutine.resume`, `coroutine.wrap`, and `require` cannot catch it. An `xpcall` message handler does not run.
- A thread that hits a limit is done.
- The state stays usable. What the call changed stays changed. Memory it still references stays in use: a script that fills the state through a global blocks later calls until Go clears that global with `SetGlobal`, since no limit applies between calls.

Lua's own limits also apply: 200 nested C calls, and 1,000,000 stack slots. Deep recursion ends in a Lua error. The thread that runs Lua needs about 512 KB of C stack for 200 nested `string.gsub` calls. Static musl builds set the thread stack to 8 MB with `-Wl,-z,stack-size=8388608`, since musl's default is 128 KB. `SelfTest` checks it, and `waserver` runs `SelfTest` at startup.

## Integers

Lua 5.4 follows strategy section 5.5 already. Integers are 64-bit and wrap on overflow, `/` returns a float, and `//` rounds toward negative infinity. Integers pass between Go and Lua exactly. `Call.Int` accepts a float only when it has an exact integer value, and never a string.

## Building

- With cgo, the default on amd64 and arm64, Go compiles Lua and `glue.c` with the C compiler. On Linux it links `libm`.
- With `CGO_ENABLED=0`, the build tag `luawasm`, or another architecture, the same API runs `lua.wasm` in wazero. It needs no C compiler. It is 2 to 60 times slower, most for dialogs, since wazero's WebAssembly exceptions are slow (decision 0002).
- Release builds use `zig cc`. `scripts/release.sh` and the Dockerfile show the flags. macOS builds need the macOS SDK, so `scripts/release.sh` builds them only on a Mac with `xcrun`.

The Go build cache only tracks files in this folder. `lua_cgo.c` holds a digest of `csrc/` and `lua/src/`, so changing a C source changes it. After editing C sources:

```text
go test -run 'TestSourcesDigest|TestPatchedLstrlib' -update ./internal/script
WASI_SDK=/path/to/wasi-sdk-34 internal/script/wasm/build.sh
```

CI rebuilds `lua.wasm` and fails when it differs.

To update Lua, follow the steps at the top of `lua/update.sh`.

## Tests

- `go test ./internal/script` runs the cgo build, and `go test -tags luawasm ./internal/script` runs the same tests on `lua.wasm`.
- `luabench_test.go` runs the workloads of `tools/luabench`, checks that their results match what every engine gave in decision 0002, and has the benchmarks.
- `nested_test.go` checks nested calls: values, threads, yields, limits, depth, and handles.
- `FuzzSource` runs random Lua source. `FuzzHostCalls` runs random sequences of calls, host functions, nested calls, threads, and refs. `FuzzExchange` writes random values straight into the memory C reads.
