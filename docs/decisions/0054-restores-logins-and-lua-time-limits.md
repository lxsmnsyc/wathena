# 0054: Restores, Logins and Lua Time Limits

- Status: proposed.
- Date: 2026-10-20.
- Code: [server/zone](../../server/zone) (`worldcalls.go`, `zone.go`, `guild.go`, `storage.go`, `scripts.go`, `transfers.go`, `call_test.go`, `inventory_test.go`), [server/adminworld](../../server/adminworld) (`restore.go`, `adminworld_test.go`), [server/store](../../server/store) (`store.go`, `hold.go`, `commit_test.go`), [server/gateway](../../server/gateway) (`gateway.go`, `session_test.go`), [server/cmd/waserver](../../server/cmd/waserver) (`main.go`, `tools.go`), [internal/script](../../internal/script) (`script.go`, `README.md`, `limits_test.go`), [server/modscript](../../server/modscript) (`stall_test.go`), [e2e](../../e2e) (`dup.go`, `restore.go`, `restore_test.go`).
- Docs: [review 2026-10](../security/review-2026-10.md), [threat model](../security/threat-model.md), [server/README.md](../../server/README.md), [e2e/README.md](../../e2e/README.md).
- Phase 8 follow-up (strategy section 15, "Phase 8"). Builds on decisions [0002](0002-lua-runtime-c-lua.md), [0029](0029-player-search-and-item-history.md) and [0037](0037-reliable-browser-checks.md).
- Lua API: none. Protocol: none. Settings: none.

## Context

- On 2026-10-19, one run of `TestDuplicationRestoresWithDatabase` under heavy load (about 20 GB of swap) failed a full audit with `item 909: 241, was 240`. A bot's login then timed out. Two later runs passed.
- Commit 8976e90 had changed `RestoreMove` so its request outlives the run's end, because a restore committed after its request ended was not counted.
- Review 2026-10 listed L34 as a known issue: a stale session can commit after a second login.
- Each Lua call had a wall-clock limit of 1 second. A machine that stalls ended zones' preludes, and 63 zones lost their Lua state at startup in one run.

## Decision

### Zone calls are all or nothing

- Cause of the extra Jellopy: `Zone.do` handed a function to the zone's goroutine and then waited for its result or for the caller's context. When the context ended first, `do` returned the error, and the zone still ran the function later.
- `adminworld.Restore` then acted as if nothing had happened:
  - It ended the offline holder's session with its unchanged copy, which still held the stack.
  - It let go of the store hold, the storages and the mailboxes.
  - The zone then moved the stack into the restore's mail. The commit dropped the ended session ("transfer saved without a character whose session ended") and wrote the mail.
  - The stack was then in the holder's inventory and in the mail. A restore of one item splits the stack under a new serial, so the audit sees one Jellopy more and no serial twice.
- Fix: `Zone.call`, which `do` and the other calls from other goroutines use (`Audit`, `ReloadMod`, `OpenGuildStorageFor`, `forPlayer`):
  - A call whose caller gives up before the zone starts it never runs.
  - A call the zone started is waited for, and its result is returned, even after the caller's context ends.
  - A panic in the call stops the zone, and the caller gets `ErrStopped`.
- `adminworld.Restore` reads the names for its answer with a fresh context, so a restore that happened says so.

### One live session per character (L34)

- `Store.Login` is `Load` for a player's login. It waits until every earlier session of the character ended: its final save is written, or it went stale. The gateway's `Characters` in `waserver` use it.
- An older session that outlasts `KickWait` can then not commit a trade, a sale, a drop, a restore or a mail after the new login read the character.
- `gateway.LoadWait`, 30 seconds, bounds the wait. A login that gives up gets "Your character is still leaving the game. Try again in a moment." and a warning in the log.
- The gateway's memory characters, used without a database, wait the same way.
- `Store.Load` keeps its behavior for the store's own tests and tools: an older session goes stale.
- A commit with a stale entry still writes the rest. Failing it as a whole would not undo the other party's items in memory, which its later saves write anyway. With `Login`, a second login no longer makes stale entries.

### The test's counting

- 8976e90's change to `RestoreMove` is kept. The request outlives the run's end so the count of audit rows is exact. The item totals never depended on it, so it could not hide a copy in an audit. It did hide the symptom of the zone call bug: more audit rows than answered restores.
- The bots' move count no longer counts a cart's `StorageRemove` that no add pairs with. That is a restore taking a stack from the cart, which had made `CheckDatabase` fail its pick-up count.

### Stress test and tracing

- `TestDuplicationCutRestoresWithDatabase`: bots leave for up to a second, restores prefer serials in the saved inventories of bots that are away, and each restore's request gives up after 100 to 200 ms. Soon after the request starts, the start zone stalls for 400 ms, half the time after it takes its next call and before it runs it.
- The stalls come from `POST /debug/wathena/stall?ms=&map=&next=1`, with `--dev` only (`Zone.Stall` and `Zone.StallNextCall`).
- A failed audit now logs a trace: each stack of the items whose totals differ, as it came, went or changed since the last full audit, with serial and holder, the server's warnings, and the item log lines of the serials that came.
- The audit also fails when one character is in the world twice.

Trace of a run of the stress test on the code before the fix (3 of 6 runs failed):

```text
dup.go:329: during the moves: item 909: 241, was 240
trace:   came: serial 01a15712-b008-7e4c-9193-6fa528ee7fd1, 1 with mail:4
server: level=WARN msg="transfer saved without a character whose session ended" character=6
trace:   log 10:10:04.936: restore of 1, serial 01a15712-b008-..., from inventory 6 to mail 4
```

Another failed run moved a whole stack the same way, and the audit named the serial held by `Cut1` and `mail:11`.

### Lua time limits

- The CPU budget measures a call's work: 10,000,000 units per call and per resume, and each mod's budget per tick (review decision 52). Neither reads a clock.
- The wall-clock limit of each call and resume (`script.DefaultTimeout`) goes from 1 second to 30 seconds. It is a backstop for a call stuck in work the CPU budget does not see, such as comparing long strings over and over.
- Each place, decided:

| Place | Before | Now |
|---|---|---|
| The prelude (`Runtime.init`) | CPU budget, 1 s wall clock | CPU budget, 30 s backstop |
| Entry scripts and setup functions (`StartMods`, reloads) | CPU budget, 1 s | CPU budget, 30 s backstop |
| Event handlers, timers, hooks, item, skill and NPC scripts | CPU budget per call and per tick, 1 s | the same budgets, 30 s backstop |
| Conversation steps (thread resumes) | CPU budget per resume, 1 s | CPU budget per resume, 30 s backstop |
| Generation layers (`wactl world new`) | CPU budget, 1 s | CPU budget, 30 s backstop |
| `wactl mod validate` and `wactl mod test` | defaults, 1 s | defaults, 30 s backstop |
| Zone tests (`testLimits`) | no wall clock | the defaults, as production |
| `internal/script` tests of the backstop | 50 ms with CPU off | unchanged |
| Fuzz tests (`fuzzLimits`) | 1 s with a small CPU budget | unchanged, so a fuzz input stays quick |
| `tools/luabench` | its own limits | unchanged, it compares engines |

- The C glue does not change, so `lua.wasm` does not either. A limit on the thread's CPU time instead of the wall clock would need both, and wasi has no thread clock.

## Tests

- `server/zone`: `TestCallIsAllOrNothing`. It fails before the fix: the started call answered the deadline and had not finished.
- `server/adminworld`: `TestRestoreThatTimesOutInItsZone`, with PostgreSQL. The zone waits on the lock of the fence's storage past the restore's deadline. Before the fix: `jellopy 11, was 10`.
- `server/store`: `TestLoginWaitsForOlderSession`. With `Load` in place of `Login`, it fails: `zeny 1100 in all, was 1000`.
- `server/gateway`: `TestMemoryLoginWaitsForOlderSession`.
- `internal/script`: `TestStallDoesNotStopCall`, a host function that sleeps 1.5 s in a call, and an endless loop that the CPU budget stops. It fails with the old limit.
- `server/modscript`: `TestStalledSetupKeepsMod`, a mod setup that a stalled host stretches past 1.5 s, and a looping setup that the CPU budget stops while the other mod keeps working. It fails with the old limit.
- `e2e`: `TestDuplicationCutRestoresWithDatabase`, as above.

Runs on 2026-10-20, with PostgreSQL 18:

- Before the fix: `TestDuplicationRestoresWithDatabase` passed 20 of 20 runs, and the stress test failed 3 of 6.
- After the fix: both passed 20 of 20 runs together, and 3 of 3 each with `-race`. One earlier run of the stress test failed on a cart move that a trade refused with `STORAGE_OUTCOME_BUSY`, which changes nothing, so the moves now count it as refused.
- The full e2e package passed once.

## Other choices

- Failing a commit with a stale entry as a whole, as review 2026-10 suggested. It leaves the other party's items in memory, and its later saves write them.
- Refusing a second login while the first session lives. Waiting up to `LoadWait` lets the usual case, a session that is still saving, through.
- A clock of the thread's CPU time for the backstop. It needs a change to the C glue and a new `lua.wasm`.
- Keeping 1 second for the prelude and longer for the rest. A stall hits every call alike.

## Left

- A mod that compares long strings in a loop can hold its zone for up to 30 seconds per call, up from 1.
- Other dev debug calls that change state through `Zone.call` now finish when started. Callers in the admin API that read their answer with the request's context, other than restores, may still report an error for a change that happened.
