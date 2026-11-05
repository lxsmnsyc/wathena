# 0070: NPC API Timers: Waits, Server Clock Timers and Player Timers

- Status: built. Details to review.
- Date: 2026-11-05.
- Code: [server/modscript](../../server/modscript) (`wait.go`, `runtime.go`, `host.go`, `world.go`, `npcworld.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone](../../server/zone) (`playertimers.go`, `worldcalls.go`, `warp.go`, `zone.go`, `npcworld.go`), [tools/convert](../../tools/convert) (`npctimer.go`, `npcscript.go`, `npcexpr.go`, `rathena.lua`).
- Docs: [lua-api.md](../generated/lua-api.md), [mods/README.md](../../mods/README.md) ("Timers").
- Builds the npc-api-timers items of review decision 60 of [proposal 0002](../proposals/0002-lua-api.md): A4, C1, C2, C3 and C4. Review decision 63 records the details.
- Lua API: adds to API 1.0. Protocol: none. Settings: none. Data: none. Store: none (player timers live in the zone). Permission: none new.

## Context

- Decision 0064 built NPC timers over `timers.after`, which follows the zone clock: a label that came due while the map slept ran when it woke. `R.sleep` raised an error in a label without a player, and `addtimer`, `deltimer`, timers attached to a player and timer commands on another NPC did not convert.

## Decision

### Lua API

- `timers.wait(milliseconds)` (A4): 0 to 10 minutes. It runs in a handler of an NPC context that has no conversation. Those handlers now run in a thread: `npc_event` handlers sent without a player or with a player of another map, callbacks of `timers.after` and `timers.every`, and schedule handlers. A timer resumes the thread. The wait ends when the context ends. Elsewhere it raises an error, and with a conversation the error names `conversation:wait`.
- `{ clock = "zone" | "server" }` on `timers.after` and `timers.every` (C1): a server clock timer follows wall time (`Options.Clock`) and wakes a sleeping zone through `NextSchedule`, as schedules do. A wait follows the clock its handler started on: the server clock for the callback of a server clock timer and for a schedule handler.
- `player:add_timer(milliseconds, label)` and `player:remove_timer(label)` (C2): `label` is `OnX` of the context's NPC or `<NPC>::OnX`. The zone keeps the timers on its `Player`. They go with the player in a handoff, with the time left, and end at logout. When one is due, the label runs as `npc:fire_event(label, { player })` does, in the NPC's zone.

### Rules

- A server clock timer is not held back by a mod's CPU budget. A sleeping zone's clock does not move, so the budget window would never reopen.
- A player has at most 32 timers (MAX_EVENTTIMER). `remove_timer` ends the first match and returns whether one ended.
- The timer of another mod's NPC (`<mod>:<NPC>::OnX`) needs `npcs.control`.

### Converter

- `R.sleep` without a conversation is `timers.wait`.
- `OnInit` may wait. A wait there cannot pause the setup function, so a server clock timer of 0 runs `OnInit` once the setup returns. An `OnInit` that shows a dialog is still refused.
- NPC timers wait on a server clock timer (C1).
- A timer attached to the player (C3): `initnpctimer` and `startnpctimer` with a flag use `NpcTimer:of(player, true)`, another timer of the same labels, kept by the player's name. It runs each label as `npc:fire_event(label, { player })` and ends when that fails, as rAthena's "Attached player not found". Plain commands act on the player's attached timer when it has one.
- Timer commands on another NPC (C4): every NPC with timers listens to the hidden labels `WaTimerInit`, `WaTimerStart`, `WaTimerStop` and `WaTimerSet0`. The commands fire them with `R.fire`, or with `R.doevent` and the player for an attached timer, which needs an NPC on the same map. `setnpctimer` there takes the tick 0 only.
- `addtimer` and `deltimer` map to `player:add_timer` and `player:remove_timer`. A label without `::` finds no NPC, as in rAthena, and converts to a comment.

## Breaks

- Converted NPC timers use `R.npc_timer(labels, npc)` and labels `{ tick, function, event label }`, and run through the server clock. `Runtime.Advance` no longer runs them: a host calls `RunServerTimers` as well. The zone does it in `runSchedules`.
- `Runtime.Shift` leaves server clock timers alone.

## Alternatives

- **Resumable handlers for every context** (A4, option 2 in full). Only NPC contexts need it; others have no converted script that sleeps.
- **A zone-wide wake for every NPC timer** (C1, option 3). Changes `wa.timers` for mods too.
- **A world-wide player timer service** (C2). A timer on the `Player` moves with the handoff and needs no other store.
- **Split each label at `sleep`** (A4, option 3). Loops around `sleep` do not split.

## Details to review

- `player:add_timer` works on a player of this map only. A handle of a player of another map raises an error until the player arrives.
- A plain NPC name in a label resolves when the timer is due (the mod's record, then core's), as `find` does. An NPC that is gone then ends the timer without an event.
- An attached NPC timer shares no tick with the NPC's own timer. rAthena shares `nd->u.scr.timer` between them.
- `stopnpctimer` with a flag acts on the player's attached timer; in rAthena it detaches first.
- `setnpctimer` with a tick other than 0 on another NPC, `getnpctimer` of another NPC, and an attach flag that is computed, are refused.
- `schedules.yearly` stays deferred (review decision 60, C5).

## How to check it worked

- `go test ./server/modscript -run 'TestWait|TestShiftLeavesServerTimers'`.
- `go test ./server/zone -run 'TestPlayerTimers|TestServerTimerWakesZone|TestWaitZoneClock'`.
- `go test ./tools/convert -run 'TestBuiltinsConvert|TestRunNPCTimerTick|TestRunSleepWithoutPlayer|TestRunTimerCommands|TestRunTimerAttached|TestNPCReportsCurrent'`.
