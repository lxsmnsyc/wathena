# 0001: Lua Runtime

- Status: superseded by [0002](0002-lua-runtime-c-lua.md).
- Date: 2026-09-29.

The benchmark below was run before the harness added our own C Lua binding, C Lua on wazero, and the `require` workloads. Decision 0002 has the full results.

## Decision

wAthena uses **arnodel/golua** (Lua 5.4, pure Go), with these conditions:

- Every call into Lua runs with a CPU limit.
- The memory limit is applied only to calls that do not resume a coroutine. golua's memory accounting panics when a coroutine is resumed in a later call (see "Findings").
- The binding layer recovers from panics inside golua and returns them as script errors.
- The binding layer stays the only code that talks to the runtime, so C Lua can replace golua later.

C Lua 5.4 through our own small cgo binding stays the fallback. gopher-lua is dropped as a fallback.

## Why

- golua is the only runtime tested that has Lua 5.4 integers, a CPU limit that works across coroutine resumes, and a memory limit, with no cgo.
- Its speed is enough. With limits on, a script call costs under 1 µs. The 10 ms tick budget (strategy section 3.8) allows over 10,000 script calls per zone per tick. Static bonus tables (strategy section 5.3, rule 5) remove most calls.
- gopher-lua is faster on host calls, but:
  - It is Lua 5.1, with float numbers and no integer division or bitwise operators.
  - Its only limit is a Go context. A coroutine keeps the context it was created with, so an NPC dialog fails on the next resume after that context ends.
  - It has no memory limit.
  - It used 22 MB of Go heap per state with default options, against 1.8 MB for golua.
- C Lua is fastest for pure Lua work and loading, but:
  - It needs cgo, which makes cross-compiling harder.
  - Each call from Lua into Go crosses cgo, so host calls were about 2 times slower than golua.
  - The binding tested, aarzilli/golua, passes the main thread's state to Go functions called inside a coroutine, so the NPC dialog read the wrong stack. A binding of our own would need to fix this.

## Benchmark

Tool: [tools/luabench](../../tools/luabench). Raw results: [tools/luabench/results/2026-09-29-apple-m2.json](../../tools/luabench/results/2026-09-29-apple-m2.json).

Setup:

- Apple M2, 8 GB, macOS, Go 1.27.1.
- arnodel/golua v0.3.0, yuin/gopher-lua v1.1.2, C Lua 5.4.9 through aarzilli/golua.
- "Limits" means:
  - golua: a runtime context per call with 10,000,000 CPU ticks and 16 MB.
  - gopher-lua: a 50 ms context per call.
  - C Lua: an instruction count hook of 10,000,000, reset before each call.
- The scripts are in [tools/luabench/scripts.go](../../tools/luabench/scripts.go). The gopher-lua version uses `math.floor` and host functions in place of `//` and `&`.
- All engines give the same results on every workload they can run.

Workloads:

| Workload | What it runs |
|---|---|
| `static_equip` | Poring Card `on_equip`: two host calls. |
| `computed` | A computed bonus that reads one player field. |
| `hit_taken` | A proc: a flag test, a chance roll, and a host call. |
| `npc_dialog` | The Healer dialog as a coroutine. Go resumes it 4 times. |
| `compute_1000` | Pure Lua: fill, sort, and sum 1,000 integers, and build a string. |
| `new_state` | Create a state with the standard library and load the workload scripts. |
| `load_1000_items` | Create a state and load 1,000 item scripts, each as its own chunk. |

Time per operation:

| Workload | golua | golua + limits | golua + CPU limit | gopher-lua | gopher-lua + limits | C Lua | C Lua + limits |
|---|---:|---:|---:|---:|---:|---:|---:|
| `static_equip` | 435 ns | 815 ns | 807 ns | 194 ns | 653 ns | 894 ns | 1.01 µs |
| `computed` | 331 ns | 717 ns | 740 ns | 241 ns | 598 ns | 649 ns | 896 ns |
| `hit_taken` | 529 ns | 773 ns | 870 ns | 290 ns | 749 ns | 701 ns | 773 ns |
| `npc_dialog` | 10.98 µs | failed | 15.64 µs | 15.88 µs | failed | failed | failed |
| `compute_1000` | 463 µs | 516 µs | 605 µs | 332 µs | 395 µs | 263 µs | 257 µs |
| `new_state` | 206 µs | 215 µs | 223 µs | 170 µs | 173 µs | 62 µs | 59 µs |
| `load_1000_items` | 22.6 ms | 22.4 ms | 22.7 ms | 16.5 ms | 26.2 ms | 4.2 ms | 4.2 ms |

Memory with 1,000 item scripts loaded:

| Engine | Measure | Result |
|---|---|---|
| golua | Go heap per state | 1.8 MB |
| gopher-lua | Go heap per state | 22 MB |
| C Lua | `collectgarbage("count")` | 0.7 MB |

Sandbox checks:

| Engine | Infinite loop | 1 GB table |
|---|---|---|
| golua + limits | Stopped after 51 ms. | Stopped after 103 ms at 16 MB. |
| gopher-lua + limits | Stopped after 51 ms. | No limit. Not run. |
| C Lua + limits | Stopped after 20 ms. | No limit with the tested binding. Not run. |

## Findings

- **golua memory limit and coroutines.** A coroutine created in one runtime context and resumed in another panics with `Too much mem released`. NPC dialogs do this on every resume. With only a CPU limit, dialogs work. This should be reported upstream.
- **gopher-lua context and coroutines.** A coroutine keeps the context it was created with. After that context ends, the next resume fails with `context canceled`.
- **aarzilli/golua and coroutines.** Go functions called inside a coroutine get the main thread's state, so they read and push on the wrong stack.
- **Cost of limits.** Starting a golua runtime context adds about 400 ns per call. The binding layer can batch several handlers of one event into one context.
- **Loading.** golua compiles 1,000 item scripts in about 23 ms, so a full item database of about 20,000 scripts loads in about 0.5 s per state. This fits the 30 second startup budget, but loading should share compiled chunks between zones where golua allows it.
- **Project health.** golua has one main maintainer and about 160 stars. Its last commit was on 2026-08-31. The binding layer and the C Lua fallback cover the risk.

## Follow-up

- Report the memory accounting panic to arnodel/golua with a small test case.
- Build the binding layer (`internal/script`) with per-call CPU limits, panic recovery, and a memory limit for calls that do not resume coroutines.
- Measure memory per mod and zone under the 16 MB budget once real scripts exist.
