# Log: npc-api-timers

- Nothing of the work existed on main: `timers.wait`, the clock option, player timers and the converter forms were all absent.
- Waits: handlers of NPC contexts run in a thread (`runWaitable`), only for NPC contexts, so other contexts pay nothing. A wait is a timer with a `wait` field.
- Server clock timers live in `r.timers` with `server` set. `Advance` and `RunServerTimers` share `runDue`. `NextSchedule` includes them so a sleeping zone wakes.
- Dead end avoided: a server timer behind the CPU budget in a sleeping zone would spin the zone loop, since the budget window follows the frozen zone clock. They skip the budget check.
- Player timers sit on the zone's `Player` and ride the handoff. No migration needed (00044 unused). Review decision 63 is used for the details.
- The attached NPC timer is one `NpcTimer` per player name. rAthena shares one tick between them; we do not.
- The reports show: initnpctimer, stopnpctimer, setnpctimer, startnpctimer and OnInit refusals drop; sleep converts in more labels.
- Open doubt: remote player handles cannot add timers.
