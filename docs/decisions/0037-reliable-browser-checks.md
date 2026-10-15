# 0037: Reliable Browser Checks

- Status: proposed.
- Date: 2026-10-16.
- Code: [client/scripts](../../client/scripts) (`game-check.ts`, `exit-check.ts`), [client/src/viewer](../../client/src/viewer) (`touch.ts`, `main.ts`, `player.ts`, `online.ts`), [server/zone](../../server/zone) (`zone.go`, `entities.go`, `loot_test.go`, and the test zones' Lua limits in `inventory_test.go` and other tests), [client/README.md](../../client/README.md).
- Changes decision [0035](0035-phase-6-exit-check.md): the exit check's steps no longer retry a click that missed. No Lua API changes. No protocol changes.

## Context

`scripts/check.sh browsers` failed in 2 of 4 runs on the development Mac (Apple M2, 8 GB, often busy), in steps that the last change did not touch:

- `iPhone 13 touch attack`
- `exit phone NPC, equip and skill`
- `exit mod monster and drop`

Earlier fixes added retries to some touch steps: a double tap sent up to three times, a tap on a Dummy and a long press on Bob tried again. The retries hid the causes. `TestFoodRaisesStr` in `server/zone` also failed once under load.

## Decision

Each check waits for a signal that the state changed, and the test world holds still where a check needs a still target. The causes and their fixes:

### A kill between ticks lost the monster's vanish (a server bug)

- Cause: a skill command, such as Bash or Mammonite, kills a monster between ticks. At the start of the next tick, `entitySet.compact` freed the monster's slot. The monster's drop took the slot in the same tick. When the zone built the player's events, the new item's ID replaced the monster's entry in the player's `netIDs` without a vanish. The damage and the death then found no ID for the player. The client showed no damage number and kept the dead monster for good. A later click on it did nothing.
- Fix: the zone frees slots at the end of a tick, after every player saw the removals of the tick and of the commands before it. `TestKillBetweenTicksVanishes` kills a monster between ticks and checks the vanish and the drop.
- This failed `exit phone NPC, equip and skill` ("no damage showed") and the touch check's Bash cast. The old retries cast again on another Dummy.

### A busy page turned taps into presses (a client bug)

- Cause: the game view's gestures and the page's long press and double tap read `performance.now()` when the page handled an event. The protocol waits until the page has handled each touch event, so the check's tap went out as a down, a long frame, then an up. With SwiftShader on a busy machine a frame can take half a second. The tap then lasted past 500 ms and became a press, which walks instead of attacking, and two taps of a double tap ended up more than 450 ms apart. A real phone has the same problem when a frame runs long.
- Fix: the client reads gesture times from `event.timeStamp`, the time the finger moved. The page's long press also fires on a move or a lift that comes after the long press time, when its timer did not run yet. The check's `Fingers.tap` and `Fingers.doubleTap` send their events together, with times 60 ms apart that end now. Every event of a `Fingers` gets a time, in order.
- This is the likely cause of the `iPhone 13 touch attack` failures and of the double taps that needed a retry. The runs before did not fail that step, since its retries hid it. A tap held over 500 ms walks toward the Dummy instead of attacking it. The double tap retry is gone.

### The check tapped a Dummy that was not on the screen

- Cause: `unitScreen` prefers a unit that no other unit covers, and counted the player as a cover. Next to the player, the near Dummy counted as covered, so the check chose another Dummy, which could be off the screen.
- Fix: the player covers nothing, since a tap never picks the player. Units on the screen come first.
- The step failed this way in a run under load after the retries were gone ("unit Dummy is not on the screen"). The old attack step had the same choice.

### Screen points read while the view moved

- Cause: the camera follows the player and eases toward it. The checks read a unit's screen point after a fixed wait, while the player or the camera still moved, and the tap landed beside the unit.
- Fix: the development hook gains `viewer.settled()`, true when the player stands and the camera has reached it, and `walking` for each unit. `settle(page)`, `unitScreen` and the exit check's `Player.still` wait for both before reading a screen point. Walks wait for the player's cell (`walkedTo`), and a trade waits until its Trade button is enabled, instead of fixed waits.

### Lantern Ghosts wandered

- Cause: the lantern mod's three ghosts wander. In the exit check a ghost walked off before a click landed, or stood over a Dummy or an NPC and took the click. `exit mod monster and drop` failed with "the Lantern Ghost did not die".
- Fix: the checks' copy of the lantern mod gives the ghost `modes: [no_random_walk]`, as the fixture's Dummies already stand still. The ghost still fights back. The mod in `mods/examples` does not change.

### A player stood on a monster's cell

- Cause: a click picks the unit nearest the pointer. Two units on one cell are at one point, so a right click on Bob on a Dummy's cell could pick the Dummy.
- Fix: before Ann opens Bob's menu, Bob steps off a crowded cell (`Player.standClear`).

### TestFoodRaisesStr and the Lua wall clock

- Cause: each Lua call has a wall-clock limit of one second besides its CPU budget (decision 0002). The zone's first script use runs the prelude, which copies every constant table. A pause of the test process for more than a second, as on a busy machine, ended the prelude with "time limit exceeded". The zone then turned its scripts off, and the dish started no status.
- Fix: the zone tests give their Lua states no wall-clock limit (`testLimits`). The CPU budget still stops a script that runs too long. The production limit does not change.

### Retries

The checks no longer retry a click, a tap, a press, a cast or a kill that missed. A step that fails reports the player's cell, the target mode, the units in sight with their IDs, the ground items, the end of the chat log and the page errors, so the cause shows in the report. The exit check's pick-up still taps again after a step aside, since a Dummy that comes back on the item's cell takes the tap, which a player would also do.

## Measurements

On the development Mac, with 4 busy loops (`yes`) on its 8 cores, each run being `CHECKS=touch,exit` (the touch check and the exit check):

| | Runs | Runs that failed | Failed steps |
|---|---:|---:|---|
| Before | 4 | 2 | `exit mod monster and drop` once, `exit phone NPC, equip and skill` once |
| After | 4 | 0 | none |

The steps also took less time, since no retry ran: `exit mod monster and drop` took 14 to 55 seconds before and 17 to 18 after. `exit phone NPC, equip and skill` took 71 to 215 seconds before and 66 to 98 after. The exit check took 696 to 898 seconds before and 634 to 688 after.

`TestFoodRaisesStr` under a process paused for 1.3 seconds again and again: 3 failures in 150 runs before, 0 in 100 after. `go test -race -count=50 -run TestFoodRaisesStr ./server/zone` passed after.

## Other choices

- More retries, or longer timeouts. They hide a cause and make a failing run slower.
- A fixed CPU throttling in the checks. It does not remove a race, it only moves it.
- Touch times after now, such as a tap whose lift is 60 ms ahead. One run with them timed out on every later scroll of three phone pages. The cause was not found, and times that end now did not show it again.
- A server endpoint for the checks to read units. The page's own view is what a click acts on, so the checks read the page.

## Left

- The production Lua state keeps its one-second wall-clock limit for the prelude. A pause of the server process over a second while a zone makes its Lua state turns that zone's scripts off until it restarts. The prelude runs inside the limited call of `internal/script`, and a limit per call needs a change to the C glue and a new `lua.wasm`.
