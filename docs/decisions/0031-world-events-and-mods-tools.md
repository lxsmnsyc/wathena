# 0031: World, Events and Mods Tools

- Status: proposed.
- Date: 2026-10-13.
- Code: [server/admin](../../server/admin) (`tools.go`), [server/adminworld](../../server/adminworld) (`tools.go`), [server/restart](../../server/restart), [server/worldevent](../../server/worldevent) (`control.go`, `def.go`, `runs.go`, `scheduler.go`), [server/modscript](../../server/modscript) (`switches.go`, `worldevents.go`), [server/zone](../../server/zone) (`worldtools.go`), [server/world](../../server/world) (`tools.go`), [server/metrics](../../server/metrics) (`window.go`), [server/store](../../server/store) (`worldtools.go`, migration 00026, `queries/worldtools.sql`), [server/gateway](../../server/gateway), [server/account](../../server/account) (`service.go`), [server/cmd/waserver](../../server/cmd/waserver) (`main.go`, `tools.go`), [schema/proto](../../schema/proto), [client](../../client) (`src/admin`, `src/net`), [e2e](../../e2e) (`tools.go`), [deploy](../../deploy/README.md).
- Builds proposal 0002, review decision 36, and phase 6 step 4 of strategy section 15. Builds on decisions [0025](0025-automatic-events.md), [0028](0028-staff-roles-and-audit.md), [0029](0029-player-search-and-item-history.md) and [0030](0030-moderation-and-gm-commands.md). It adds one Lua world event, `restart_planned`.

## Decision

### Permissions

Step 4 uses the names that the catalog reserved in step 1: `world.read`, `world.announce`, `world.restart`, `events.control`, `mods.read` and `mods.disable`. The default roles are unchanged:

- `admin` has every group.
- `game_master` has `world.read`, `world.announce` and `events.control`.
- `moderator` has none of them.

### Admin API

| Call | Permission | Kind |
|---|---|---|
| `GET /world` | `world.read` | read |
| `POST /announce` | `world.announce` | change |
| `POST /restart` | `world.restart` | change, with a recent login |
| `DELETE /restart?reason=` | `world.restart` | change, with a recent login |
| `GET /events` | `events.control` | read |
| `POST /events/{name}/start` | `events.control` | change |
| `POST /events/{name}/stop` | `events.control` | change |
| `PATCH /events/{name}` | `events.control` | change |
| `POST /events/{name}/reset` | `events.control` | change |
| `GET /mods` | `mods.read` | read |
| `POST /mods/{id}/disable` | `mods.disable` | change |
| `POST /mods/{id}/enable` | `mods.disable` | change |

- Every call has the checks of step 1: the rate limit, the CSRF token of a change, the session, the role with its two-factor rule, and the permission.
- Every change writes an audit row: `world.announce`, `world.restart`, `world.restart_cancel`, `events.start`, `events.stop`, `events.change`, `events.reset`, `mods.disable` and `mods.enable`.
  - Announcements and restarts change nothing in the database, so the admin API writes their rows on the account role, as it does for IP searches.
  - Event changes and mod switches are game data. The world writes them with their audit rows in one transaction on the game role. Starts and stops write their rows on the game role after the scheduler took them.
- An event's name in the path is URL-escaped, such as `core%3AField_Boss`.

### World view

`GET /world` answers with:

- Each zone: its map, its players, its living monsters, and its ticks of the last minute with their work's p50, p99 and maximum and the ticks over the 10 ms budget. A zone without players sleeps and does not tick, and shows as asleep.
- The process: CPU cores and memory from the measurements of [server/metrics](../../server/metrics), goroutines, the start time, the uptime and the version. CPU is measured since the start, or since the last `POST /debug/wathena/reset`.
- The players online by name, at most 1,000, with the count of all of them.
- The planned restart, if any, and the server clock.

How it is measured:

- Each zone keeps its last 1,280 ticks with their times, more than a minute of 50 ms ticks. The ring is made at the zone's first tick, so a zone that never had a player costs nothing. The zone takes a mutex once per tick, and a reader takes it once.
- Monsters and players are read on each zone's goroutine. A world of 988 maps answers in a few milliseconds.
- The screen loads it every 5 seconds. The admin rate limit allows that: 60 calls at once, then 1 a second.

### Announcements

`POST /announce` takes `{ text, map }`:

- The text is 1 to 255 characters on one line. Players see it as an announcement, as rAthena's `announce`.
- An empty map is the world. A map that does not run answers `not_found`.

### Planned restarts

`POST /restart` takes `{ at, reason, message }`, and `DELETE /restart` cancels the plan ([server/restart](../../server/restart)):

- One plan at a time. A second plan answers `conflict`. The time is 1 minute to 7 days ahead of the server clock. The reason is 1 to 255 characters. The message is at most 255 characters, and the announcements say the reason when it is empty.
- Players get announcements 30, 15, 10, 5 and 1 minute before, and 30 seconds before, such as "The server restarts in 5 minutes. New maps arrive!". A plan made later than a mark skips it. When the clock passes several marks at once, only the last one is announced.
- In the last minute, new logins are refused:
  - The gateway answers a new game connection with `DISCONNECT_REASON_RESTART` and "The server restarts in less than a minute: <reason>. Come back in a few minutes.". It does not use up the ticket.
  - `POST /account/ticket` answers 503 with `{"error": "restarting", "message"}`. The account pages still work.
- At the time, waserver stops as on SIGTERM, without the shutdown countdown: it stops taking players, disconnects every player with `DISCONNECT_REASON_RESTART` and "The server is restarting: <reason>. Come back in a few minutes.", saves every character, writes the chat log and the party changes, and exits with status 75 (`EX_TEMPFAIL`). The process manager starts it again (deploy/README.md, "Planned restarts").
- A cancel announces "The planned server restart is cancelled.".
- The plan lives in memory. A restart before its time, a crash or a stop, drops it. Staff plan it again after such a restart.
- The planner follows the server clock, so `--dev`'s clock moves it.
- Mods get `restart_planned` in every zone, with `{ at, reason }` when a restart is planned and `{ cancelled = true }` when it is cancelled. `at` is in Unix seconds.

### Events

`GET /events` lists every world event of core and the mods, the ones that are off too:

- Whether it is on, its trigger in the form of `data/events.yml` with a sentence for people, its duration and its next run.
- The run that lasts, with its scheduled time, its start and its end.
- The latest 5 results since the server started: won, lost, stopped, or ended for a script event that ran its time. A restart forgets them.
- The error of the last run that did not start, and when.
- The staff change over the config, if any, with who made it and when, and what the config says.

Changes:

- `PATCH /events/{name}` takes `{ on, trigger, durationMinutes, reason }`. Fields that are not set keep their value. A change keeps the fields of the change before it. The trigger is checked as `mods.events` checks it.
- `POST /events/{name}/reset` deletes the change: the event follows the config again. An event without a change answers `conflict`.
- `POST /events/{name}/start` starts a run now, even of an event that is off. A run that lasts answers `conflict`. `POST /events/{name}/stop` ends the run that lasts, as `world_events.stop` does. An event without one answers `conflict`.
- Event changes and mod switches run one at a time.
- A change is saved in `event_overrides` first, then goes to the scheduler, which applies it at its next check, within a second, on its own goroutine. The call waits for it, so the answer shows the new next run.
- A new trigger plans the next time from now. A run that lasts keeps its end. A new duration applies to the next run.
- An event that is off starts no runs by its trigger, and a run of it that a stop cut short does not go on. A run that lasts when staff turn it off goes on until its end.
- `world_events.start` and `world_events.stop` of mods see the events that are on, as before.

At start, the config's settings come first (`mods.events`), then the changes from the database:

- A change of an event that no longer exists is left out, with a warning.
- A saved trigger that no longer reads is left out, with a warning.
- An event that staff turned on but whose maps do not run stays off, with a warning. A change that turns on such an event answers `bad_request`. An event that the config turns on still fails the start, as before.

### Mods

`GET /mods` lists every loaded mod, core first:

- Its ID, name and version, and whether it is built in.
- Its signature status: `builtin`, `folder`, `unsigned` for a `.zip` without a signature, or `unverified` for a `.zip` with a `.minisig` next to it, which the server does not check.
- The permissions it holds, those its manifest requests, and those pending: requested and not approved. A mod with pending permissions does not load, so a loaded mod has none.
- Where staff turned it off, with who, why and when.
- Its Lua errors in each zone since the server started, with the last error and its time, and the total.
- The maps that run, for the zone choice.

Error counts:

- The runtime counts each failed handler, timer, hook, schedule, condition and NPC click, and each setup function or entry script that failed, per mod and zone. Counting takes a mutex only when an error happens.
- The zone owns the counts, so a new Lua state keeps them. Errors keep their last 500 bytes.

`POST /mods/{id}/disable` takes `{ map, reason }`, and `/enable` takes `{ map, reason }`. An empty map is every zone:

- Turning a mod off needs a reason. Turning it on does not.
- Off everywhere replaces the mod's zone rows. On everywhere deletes every row of the mod. On in a zone deletes that zone's row, and answers `conflict` while the mod is off everywhere.
- Turning off where it is off, or on where it is not off, answers `conflict`.
- Core and its ruleset mods cannot be turned off. The API refuses `core` at once, and the world refuses every built-in mod.
- The switch is saved in `mod_switches` first, then goes to the zones. Each zone applies it on its goroutine. At start, waserver reads the switches and gives each zone its mods that are off before it runs.

A mod that is off in a zone ([server/modscript](../../server/modscript), `switches.go`):

- Its event handlers, timers, hooks, schedules and world event conditions are skipped there. Its NPCs do not answer clicks, and their conversations close at once without running more of their scripts. Its item scripts, skill scripts and monster AI scripts do not start, and an item whose script is the mod's is not used up.
- A timer that comes due while the mod is off is skipped: a timer that runs once is dropped, and a repeating one keeps its rhythm.
- Its contexts stay. Its handlers, timers and hooks stay registered and run again once it is on. Its Lua state keeps its tables, and its variables, server variables and database rows are not touched.
- Monsters it spawned stay on the map and keep their AI scripts' contexts, which are skipped while it is off.
- Bonuses it gave stay until they end. Computed bonuses still compute.
- A run of a world event whose script is the mod's goes on in the scheduler. Its context is skipped in that zone.
- A mod that is off when the zone starts runs no entry script and no NPC setup there. They run when it is turned on. An item that is worn while its mod is off gives its script's bonuses when it is worn again after the mod is on. A monster spawned while its mod is off has no AI script until it spawns again.

### Rows (migration 00026)

- `event_overrides`: one row per event, with `enabled`, `trigger` (JSON in the form of `data/events.yml`) and `duration_minutes`, each NULL for the config's value, and `changed_by`, `changed_by_id` and `changed_at`. A row sets at least one value.
- `mod_switches`: one row per mod and map, where map `''` is every zone, with a reason and who and when. `core` is refused by a check.
- The game role reads and writes both. The account role has no rights on them.

### Protocol

- `DISCONNECT_REASON_RESTART` (11) is new. Its message has the reason.
- The admin messages are `WorldView`, `ZoneView`, `ProcessView`, `OnlinePlayer`, `AnnounceRequest`, `AnnounceResult`, `RestartRequest`, `RestartPlan`, `EventList`, `EventInfo`, `EventRun`, `EventResult`, `EventOverride`, `EventChange`, `EventAction`, `ModList`, `ModInfo`, `ModSwitch`, `ModZone` and `ModSwitchRequest`.

### Admin screens

- World: the zones (sleeping ones on request), the process, the players online and the planned restart with a countdown, loaded every 5 seconds while the tab shows. With `world.announce`, an announcement box for the world or a map. With `world.restart`, the restart planner: a time in the viewer's time zone, a reason, a message, and a cancel button.
- Events: the table with start, stop, edit and reset buttons, and a form for whether it is on, its trigger by kind and its duration.
- Mods: the table with the signature, the permissions, the zones where it is off and the errors by zone. With `mods.disable`, a zone choice, a reason, and off and on buttons. Core has none.

### Game client

- `DISCONNECT_REASON_RESTART` shows its message, and the page waits 10 seconds before it goes back to the character list, as for a kick.
- A ticket refused with 503 shows the server's message.

### Development endpoints

With `--dev`:

- `POST /debug/wathena/announce?text=&map=` announces as `POST /announce` does.
- `POST /debug/wathena/restart?in=&reason=&message=` plans a restart that long from the server clock, and `DELETE /debug/wathena/restart` cancels it.

## Tests

- `server/metrics`: `TestRecent` for the window.
- `server/restart`: the announcements with a fake clock, the login refusal in the last minute, the end, a clock that jumps, and the plan's rules.
- `server/worldevent`: `TestChangeTrigger`, `TestTurnOffAndManualRuns`, `TestOffAtLoad`, `TestStartFailure`, `TestTriggerValue` and `TestSettingsKeepOff`.
- `server/modscript`: `TestModOffStopsScripts` (handlers, timers, hooks and schedules stop and come back, data stays), `TestModOffAtStart`, `TestErrorLog` and `TestRestartPlanned`, a Lua handler of `restart_planned`.
- `server/zone`: `TestModOffInZone`, `TestModOffStopsNPCClicks` and `TestModsOffAtStart`.
- `server/store`: `TestEventOverrides`, `TestModSwitches` and `TestWorldToolsRights`.
- `server/admin`: HTTP tests of every call with a fake world: `TestWorldCalls`, `TestRestartCalls` (the recent login), `TestEventCalls`, `TestModCalls` and `TestToolsRateLimit`, with their refusals, the CSRF token and the audit rows.
- `server/adminworld`, with PostgreSQL and a running world: `TestModSwitchesSurviveRestart`, `TestEventOverridesSurviveRestart` and `TestWorldViewAndRestart`.
- `server/gateway`: `TestClosedRefusesLogins`. `server/account`: `TestTicketsClosedForRestart`.
- `e2e`: `TestAnnouncementsAndCancel` (announcements to the world and a map, `restart_planned` in the fixture mod, a cancel) and `TestPlannedRestart` (the announcements, a refused login, the disconnect with the reason, exit status 75, and a start again with the saved character).
- `client`: tests of the API calls, the tabs, the world screen's cells, countdown and restart form, the trigger form, the event change, the mod switch checks, the restart disconnect and the ticket refusal. `pnpm check:browsers` opens the world, events and mods screens with a faked admin API.

## Other choices

- Ending a disabled mod's contexts, as a mod that fails too often. Its timers and handlers would then need its setup to run again on enable, which makes NPCs and channels again, and spawned monsters would go. Skipping its code keeps its state as it was, and turning it on resumes it.
- Saving the planned restart in the database, so it survives a restart. Review decision 36 asks for a plan that a restart drops, and a plan from before a crash may be wrong after it.
- Reusing `DISCONNECT_REASON_SERVER_SHUTDOWN`. A planned restart has a reason staff wrote, and the client waits longer so it can be read.
- A pending list of mods that did not load for missing permissions. The loader refuses to start with such a mod, so there is nothing to list. A mod disabled in `mods.disable` does not load and does not show.

## Left

- A network form of the tools for the distributed build (phase 10).
- Signature checks on the server: it has no trusted keys, and `wactl mod install` does not keep the `.minisig`.
- Results of event runs survive only until a restart.
- `wa.testing` has no way to plan a restart, so a mod test cannot fire `restart_planned` (review item). Closed: [decision 0036](0036-phase-6-review-changes.md) adds `world:plan_restart`.
- A start or stop whose HTTP call times out still runs at the scheduler's next check, without its audit row.
- A restart plan or cancel whose audit row fails to write stays in effect, and the call answers `unavailable`.
- A cancel that comes as a mark is announced may show before that mark's line.
