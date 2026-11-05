# Outcome: npc-api-timers

spent: not measured (about one session, one active day).

## Built

- `timers.wait(milliseconds)` (A4): server/modscript/wait.go. NPC-context handlers that run without a conversation run in a thread (`runWaitable`): `npc_event` handlers sent without a player or with a player of another map, `timers.after` and `timers.every` callbacks, and schedule handlers. A wait is a `timer` with a `wait` field. The context ending closes the thread.
- `{ clock = "zone" | "server" }` on `timers.after` and `timers.every` (C1): server/modscript/host.go, runtime.go (`runDue`, `RunServerTimers`; `Shift` skips server timers), world.go (`NextSchedule` includes server timers). server/zone/worldcalls.go `runSchedules` runs them, so a sleeping zone wakes. A wait follows the clock its handler started on (schedule handlers: the server clock).
- `player:add_timer(ms, label)` and `player:remove_timer(label)` (C2): server/modscript/wait.go (`PlayerTimerHost`), server/zone/playertimers.go. The timers live on the zone `Player`, ride the `handoff` with the time left, and run the label as an event with the player (max 32).
- Converter: `R.sleep` uses `timers.wait`; `OnInit` may wait (a server clock timer of 0 runs it); NPC timers wait on the server clock; timers attached to a player (C3, `NpcTimer:of(player, true)` in tools/convert/rathena.lua); timer commands on another NPC (C4, hidden labels `WaTimerInit`, `WaTimerStart`, `WaTimerStop`, `WaTimerSet0`); `addtimer` and `deltimer` (tools/convert/npctimer.go, npcscript.go, npcexpr.go).
- Docs: decision 0070, review decision 63 in proposal 0002, mods/README.md "Timers", regenerated docs/generated (lua-api.md, lua/wa/timers.lua, types.lua, npc-scripts-*.md, classic-coverage.md).
- Reports (renewal): initnpctimer, stopnpctimer, startnpctimer and setnpctimer have no refusals left; OnInit refusals 40 to 36; `sleep` refusals 21 to 17; script NPCs placed 6,768 to 6,777.
- Nothing of this existed before (checked first).

## Deviations

- Review decision number 63 used, as a details entry (not a new accepted item). No migration (00044 unused).
- Touched outside the brief's paths: mods/README.md (three lines on the new calls), docs/proposals/0002-lua-api.md (decision 63; in the brief's shared list), tools/convert tests (npc_builtins_test.go, npc_group3_test.go changed for the new output; npc_timers_test.go new), server/zone/npcworld.go (`runNPCEventAway` finds a local player when the zone has no player directory), server/zone/warp.go and zone.go (handoff field). server/schedule was not changed: server clock timers needed none of it.
- `schedules.yearly` left deferred, as review decision 60 says.

## Stubs left

- None.

## Gotchas

- Converted NPC timers now run on the server clock. A host that drives a `modscript.Runtime` by hand must call `RunServerTimers(now)` as well as `Advance`, or NPC timers never fire (zones do it in `runSchedules`).
- Server clock timers skip the CPU budget check on purpose (see log.md).
- An attached NPC timer is a separate timer per player name; rAthena shares the tick with the NPC's own timer. `stopnpctimer` with a flag stops the attached timer.
- `player:add_timer` fails for a handle of a player of another map. `OnInit` with a `sleep` now runs after the setup returns, on the first schedule check, not inside it.
- `setnpctimer` on another NPC takes the tick 0 only, and `getnpctimer` of another NPC is refused.
- Hidden timer labels appear in every NPC file with timers; a changed output of all timer NPCs is expected in diffs.

## Proposed learnings

- Call `RunServerTimers` wherever `RunSchedules` runs | the two share one wake-up in `NextSchedule` | server/zone | no

## Verification

- `scripts/check.sh vet lint`: passed (log: /private/tmp/claude-501/-Users-alexis-Documents-GitHub-wathena/ab4336fe-fed3-4016-b4d9-8c75ad9ce7d6/scratchpad/timers-check.log).
- Per-package `go test -p 1 -count=1 -timeout 45m` of the changed packages and their importers: see the narrow check result in the final report and the same log.
- Targeted tests while editing: TestWait*, TestShiftLeavesServerTimers (modscript); TestPlayerTimers*, TestServerTimerWakesZone, TestWaitZoneClock (zone); TestBuiltinsConvert, TestRunNPCTimerTick, TestRunSleepWithoutPlayer, TestRunTimerCommandsOnAnotherNPC, TestRunTimerAttachedToPlayer, TestNPCReportsCurrent (convert).

## Size

- Estimated M, actual M.
