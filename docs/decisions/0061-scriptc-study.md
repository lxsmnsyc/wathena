# 0061: scriptc Study

- Status: study. No decision. Revisit when the conditions in "When to look again" are met.
- Date: 2026-10-20.
- Answers the open question "JS and TS, and scriptc" in strategy section 17.
- Code: none. The test programs were throwaway and are not in the repository. "How the tests were run" describes them so they can be written again.
- Lua API: none. Protocol: none. Settings: none.

## Summary

- A full port of the server to TypeScript built with [scriptc](https://scriptc.dev/) is the cleanest long-term end state. It removes all friction between scripts and the server for core content, and gives the server and the web client one language.
- It is blocked today. scriptc 0.3.6 has no working PostgreSQL driver and no working protobuf library, its `setInterval` drifts, and closures that change outer variables are about 10 times slower than in Go.
- Replacing only Lua with TS, while the server stays in Go, makes the friction worse, not better.
- Recommendation: keep Go and Lua. Look again when the blockers below are fixed. If TS for mod authors is wanted sooner, compile TS to Lua with [TypeScriptToLua](https://typescripttolua.github.io/) and generate `.d.ts` files from `server/modscript/luaapi/spec.go`.

## What was tested

All tests ran on 2026-10-20 on an Apple M2 with 8 GB of memory, with scriptc 0.3.6, Go 1.27.2 and Node 26.4.0. The machine was running other work, so p99 times were noisy. Only p50 ranges from two runs are given for tick work.

| Requirement | Result | Notes |
|---|---|---|
| Build and run a native program | Works | About 5 s for a first build, 0.4 to 1.8 s after. Binaries are 0.1 to 0.4 MB. |
| 50 ms zone tick | Works | See "Speed and memory". |
| Integer results the same as Node and Go | Works | The tick test gave the same checksum in all three, with `Math.imul` and `\| 0`. |
| WebSocket server | Partly | The `ws` package does not compile: `SC2013`, it ships no `.d.ts` of its own. A small server written on the `node:http` `upgrade` event works: 1,000 connections and 70 KB frames echoed with no errors, as fast as Node. |
| Protobuf | Fails | `@bufbuild/protobuf` compiles, then stops at start: "`DataView.getBigInt64` is part of the standard library types but has no scriptc lowering yet". |
| PostgreSQL | Fails | `pg` does not compile (`SC2013`, its types are in `@types/pg`). The build of `postgres` (postgres.js) used one core at 100% for 20 minutes without finishing, and was stopped. |
| Worker threads, one zone each | Works | 4 and 8 zones ran in parallel. Messages between threads are copied. `@scriptc/threads` can share read-only data. |
| Crypto | Partly | sha256, HMAC, scrypt, `randomBytes` and `timingSafeEqual` work, as fast as Node. `argon2Sync` does not exist. wAthena uses Argon2id (`server/account/password.go`). |
| Calling C | Works | A program called C Lua 5.5 through a small C wrapper, passing a `Uint8Array`: 22 µs to make a state, run a chunk and close it. libpq and libargon2 could be reached the same way. This was not tested. |
| WebAssembly modules | Not run | Read from the docs only. See "Lua replaced by TS, server still in Go". |

### Speed and memory

The tick test is the zone loop in its simplest form: entities walk on a 400 × 400 map, an 8 × 8 block index is rebuilt each tick, and each player gets a move event for every entity in view that moved. It was written in TS for scriptc and Node, and ported to Go with the same random numbers, map and events.

| | scriptc 0.3.6 | Go 1.27 | Node 26 |
|---|---|---|---|
| Tick work p50, 500 players and 2,000 monsters (32,574 events per tick) | 2.7 to 3.6 ms | 3.0 to 4.2 ms | 3.0 to 4.2 ms |
| Tick work p50, 1,000 players and 4,000 monsters (129,256 events per tick) | 9.2 to 10.4 ms | 11.5 to 11.8 ms | 9.7 to 9.8 ms |
| Largest resident memory, 1,000 players | 2.5 MB | 11 MB | 140 MB |
| Average interval of a 50 ms timer | 54 to 64 ms | 50.0 ms | 51 ms |
| A loop of 4 million steps in a plain function | about 0 ms (compiled away) | 1.4 to 2.5 ms | 3.0 ms |
| The same loop in a closure that changes an outer variable | 26 to 40 ms | 2.5 to 2.8 ms | 6.4 ms |

- Speed of plain code is about the same in all three. scriptc's p50 was a little lower.
- Memory is scriptc's clear gain: about 4 times less than Go. It counts references and frees memory at once, so it has no collector pauses in this test. Its cycle collector was not measured.
- `setInterval` waits its full interval after the callback ends, so the tick drifts by the work time. A zone loop would schedule each tick against a fixed deadline instead.
- A closure that changes a variable outside it is about 10 times slower than in Go. The variable is boxed. Async functions keep their state the same way.

## What a full port would improve

- **No boundary between scripts and the server.** Converted NPC, item and skill scripts become ordinary functions that call zone code. This removes:
  - the cgo crossing, measured at 73 ns, on every API call and every player field read;
  - copying values and tables across, and handles that must be kept and released;
  - the reentry rules: `ErrReentrant` and the 8 nested calls of `Call1Nested`;
  - `prelude.lua` (3,298 lines), the Lua API glue, and the two Lua engine builds.
- **Dialogs as async functions.** `await conversation.choose(...)` runs in the same runtime, without coroutine handling in Go.
- **One type system** from the protocol schema to the scripts. Converter output is type-checked when the server builds.
- **One language for the server and the client.** The code in `shared/` (pathfinding, map generation) is Go compiled to WebAssembly for the client today. In TS it could run natively on the server and as plain code in the browser.
- **Memory.** Lower memory per zone matters with hundreds of maps. The tick test used 4 times less memory than Go.
- **Small binaries and fast builds.**

## What a full port would lose

1. **Protection from scripts that never end.** The Lua CPU budget also stops converter bugs. A converted rAthena `while` or `goto` loop that never ends would stop the zone's thread for good, since compiled code cannot be interrupted. The port would need a watchdog that ends and restarts a zone's worker, which loses the zone's state since the last save, and loop counters inserted by the converter.
2. **Reloading scripts while the server runs.** scriptc has no `eval`, `new Function` or loader. Every script change needs a server build and restart. rAthena's owners use `@reloadscript` today.
3. **Goroutines.** Concurrency is worker threads that copy messages. wAthena runs one goroutine per zone and three per connection.
4. **Mature libraries.** Go's `coder/websocket`, `pgx`, `sqlc`, `google.golang.org/protobuf`, `x/crypto/argon2`, `pprof` and `-race` would each need a replacement, or code written against C libraries through FFI.

### Third-party mods would still need a boundary

Without `eval`, a server owner's mods can only:

- **Be compiled into the server.** There is no friction, but there is no sandbox either, and every change to a mod needs a build. This is rAthena's model, where custom scripts are trusted, and it gives up the permission system of decision 0006.
- **Run as WebAssembly.** The sandbox stays, but scriptc's module format is stricter than Lua (see the next section).

A stable mod API is wanted in either case, so that changes inside the server do not break mods. That layer is a boundary by design, but in one process it is cheap.

## Lua replaced by TS, server still in Go

This makes the friction worse:

- **Native code linked into Go:** still a cgo boundary, now between two runtimes, Go's collector and scriptc's reference counts, which cannot share objects. Mods have no sandbox and need a server build.
- **WebAssembly modules run by `wazero`:** scriptc's [module docs](https://scriptc.dev/docs/wasm) say:
  - "Host callbacks run synchronously… Reentry is rejected." wAthena needs nested calls for stats (decision 0010) and item rolls (decision 0014).
  - A call cannot pause and resume later, so NPC dialogs cannot wait for the player inside a call. Converted scripts would become state machines.
  - Callback results "must be scalars or void". Strings and bytes are lent only for the call. Handles, tables and events need hand-written encoding.
  - After a trap or panic the instance must be thrown away. A Lua state stays usable after it hits a limit.
  - There is no instruction budget. `wazero` stops a call only by context cancellation, which is wall-clock time.
  - Mod authors would need scriptc, zig and the runtime pack at matching versions.

## When to look again

Look again when all of these are true. Each can be checked with a short test like the ones above.

1. A PostgreSQL driver compiles and passes a test of queries, transactions and `COPY`, or libpq through FFI is shown to work with pooling.
2. `@bufbuild/protobuf`, or another protobuf library, encodes and decodes wAthena's `schema` messages with the same bytes as Go.
3. A WebSocket server library compiles, or the small server is kept and tested against the browser client.
4. A timer or loop keeps a 50 ms tick with less than 1 ms average drift.
5. Closures that change outer variables cost less than twice their cost in Go.
6. Argon2id is available, from `node:crypto` or through FFI.
7. scriptc reaches 1.0, or a release with a promise about breaking changes.

Then, as strategy section 17 says, build a prototype of one zone and its tick before deciding. The prototype must also measure:

- the cycle collector's pauses with entities, parties and guilds that refer to each other;
- a watchdog for a zone whose script never ends;
- tick timing of async dialog code resumed between ticks.

## Until then

- Keep the Go server and the Lua runtime.
- Remove the friction where it costs the most, in Go: decision [0056](0056-shared-lua-setup.md) for setting up states, and the scripted bonus cache in decision [0059](0059-spatial-queries.md).
- If mod authors want TypeScript, add it as a source language that compiles to Lua with TypeScriptToLua, and generate `.d.ts` files from the Lua API spec. Check that its `async` and `await` map onto Lua coroutines for dialogs before relying on them.
- Another option for core content: have the rAthena converter write Go instead of Lua. Core scripts would then call zone code directly, as in a full port, and Lua would stay only for third-party mods. This needs its own study.

## How the tests were run

So the tests can be written again:

- scriptc was installed with `npm install scriptc` in an empty folder whose `package.json` has `"type": "module"`, and built with `scriptc build file.ts -o out`.
- **Tick test:** about 70 lines. 400 × 400 map, 8-cell blocks, view range 14, players placed in a 60 × 60 square and monsters across the map, an LCG for random numbers (`seed = imul(seed, 1103515245) + 12345`), 200 ticks on a 50 ms timer, times of the first 10 ticks dropped. Peak memory was read with `/usr/bin/time -l`.
- **WebSocket test:** a server on `node:http` `upgrade` with the handshake from `node:crypto` and frame parsing by hand, against Node's built-in `WebSocket` client with 200 × 200, 50 × 20 (70 KB frames) and 1,000 × 20 messages.
- **Threads:** one worker per zone, each doing about 4 ms of integer work per tick for 60 ticks.
- **FFI:** a C function `lua_eval(const uint8_t *, int32_t)` linked with `/opt/homebrew/lib/liblua.a`, declared with `ffi_format` 6 and `bytes` and `i32` parameters.
