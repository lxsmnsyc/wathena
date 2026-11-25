---
id: narra-study
title: "Study: port the server from Go to Narra"
status: todo
priority: later
type: spike
outline: false
milestone: later
size: M
spike: true
gate: true
added: 2026-11-25
owns:
  - docs/decisions/**
  - docs/roadmaps/v0.2-parity/nodes/narra-study/**
reserves: []
provides: []
depends_on:
  - on: release-v0.2.0
    kind: hard
    why: the study waits for the parity release, and Narra 0.1.0 is still changing
---

# Study: port the server from Go to Narra

## Goal

Decide whether to port the server from Go to [Narra](https://github.com/lxsmnsyc/narra), the maintainer's language: memory safe without a garbage collector, native binaries through C, TypeScript-like syntax. Write a decision record in the style of decision 0061 (the scriptc study): what was tested, what a port would gain and lose, and the conditions for starting it.

## Context

The server is about 214,000 lines of Go, not counting tests and generated code. Decision 0061 kept Go and Lua after scriptc failed its tests. Narra may avoid scriptc's main blockers, since it calls C directly through `extern { } from` blocks:

- Lua: call C Lua 5.5 directly, without the cgo crossing (73 ns per call, decision 0061) or the reentry rules.
- PostgreSQL through libpq and Argon2id through libargon2, both called over C.
- Concurrency: `std/thread`, `std/channel` and `std/async` against one goroutine per zone and three per connection.

Test each with a small program, as decision 0061 did, on Narra's current release:

1. PostgreSQL: queries, transactions, `COPY` and a pool through libpq; and a replacement for sqlc.
2. Protobuf: wAthena's `schema` messages encode and decode to the same bytes as Go (a generator, or a library over C).
3. WebSocket server with TLS, tested against the browser client.
4. A 50 ms zone tick with less than 1 ms average drift. Compare tick work and memory with Go on decision 0061's tick test.
5. Lua 5.5 states per zone, calls in both directions, and the Lua CPU budget.
6. Argon2id, sha256, HMAC and `randomBytes`.
7. Tooling: profiling, a race check (Narra rejects data races in safe code), tests and fuzzing, against Go's `pprof`, `-race` and `go test`.
8. Client code: `shared/` (pathfinding, map generation) runs in the browser as Go compiled to WebAssembly today. Narra's wasm32-wasi target leaves out threads and `std/net`; check whether `shared/` fits.
9. Narra's stability: syntax and std are still changing at 0.1.0.

Also estimate the cost of a port: an order of packages, and whether it can go package by package over C (Go and Narra in one binary) or must happen all at once.

- Source: maintainer, 2026-11-25; decision 0061

## Acceptance

- [ ] A decision record (next free number) with each test's result and how to run it again, and a recommendation: port, port later on named conditions, or keep Go.
- [ ] The maintainer reviews the record before narra-port starts (gate).

## Sizing

M: about 9 small test programs and one decision record, as decision 0061 was. No server code changes.
