# Outcome: proposal-instances

## Built

- docs/proposals/0003-instances.md: design proposal 0003 (Problem, Change sections 1 to 13, Breaks, Alternatives, 14 questions for review).
- docs/roadmaps/v0.2-parity/contracts/instances.md: Interface, Behavior, Stub and Contract tests filled from the proposal. Still `status: draft`, version 1, for the orchestrator to freeze after the maintainer accepts.
- Numbers taken: proposal 0003. No decision, review decision or migration number used (the design needs no migration; next migration stays 00043).

## Deviations

- None from the brief. The brief asks to freeze the contract; per the worker instructions it stays draft until the maintainer accepts.

## Stubs left

- None. The contract has no stub: instances-core builds the real system.

## Gotchas

For instances-core:

- **Owned paths look wrong.** The design needs changes outside the listed paths: `server/social/**` (party and guild hooks: leave, expel, break up, block rules), `server/mod/**` (`InstanceKind`, permission `instances.manage`), `server/gamedata/**` (loading `data/instances.yml`), `server/rules/**` (five ruleset rules), `server/cmd/waserver/**` (wiring the registry, the `newZone` factory, config `instances`, metrics), `server/staff/**` (`world.instances`), `server/admin/**`, `server/adminworld/**` and `schema/proto/wathena/admin/**` (World screen, destroy endpoint, audit), `reference/rathena/extracts/**` plus the fork's `tools/wathena/extract.py` (`instances.json`, base map flags), `server/character/**` or `server/store/**` if the save point rule at logout lives there, and `client/src/**` beyond `net` (the memorial dungeon window). `server/world/**` is owned but the world needs only small changes; most code goes to the new `server/instance/**` and `server/zone/**`.
- **Size L is tight.** Lifecycle, zones, variables, NPC copies, Lua API, converter, protocol, client window, admin screen and the e2e test are more than one L node. Recommend splitting: instances-core (registry, zones, copies, variables, Lua, converter, e2e) and a second node for the client window, admin screen and staff endpoint.
- `startNPCs` is lazy (first player). Instance zones must start NPCs at build, then fire `instance_start` across zones in order.
- `NPCDirectory.AddZone` keeps the first record of a name: copies need their own names (`<full name>:<id>`). `NPCKey` of a copy must return the base record name so `.x` stays shared.
- `mapZone` adds `map/` to every name; instance and `gen:` IDs must be looked up as they are.
- `MapChange.map` and `Welcome.map` send `z.Map`; add `Zone.Asset` and send it.
- `Zone.Evacuate` sends everyone to one cell; instances need each player's own save point, including late arrivals (`forward`).
- `World.JoinCharacter` sends a character whose map does not run to the start map; instance map IDs must go to the save point instead, or back into the instance when reconnect allows.
- Measure the Lua state cost per instance map before setting `max_running` (decision 0056 is not built). Each instance map compiles the prelude and `rathena.lua`.
- Map flags: only `nomemo` is built. Copy flags from the extract; leave enforcement to instances-content.

For instances-content:

- Depends on the base map flags `noteleport`, `nobranch` and `partylock` having effects (question 13 recommends building them there).
- Scripts lean on `donpcevent instance_npcname(...)+"::OnX"`, `hideonnpc`, NPC timers, labeled `monster`/`areamonster` on `'map_name$`, `mobcount`, `killmonster` and lockout quests; most map to existing API calls once the converter folds `instance_npcname`.
- Base maps come from `instance_db.yml`; NPC lines on `@` maps convert as placed records that load as templates.

## Proposed learnings

- A zone is keyed by map ID, and the asset the client loads can differ (`Zone.Asset`) | instance maps reuse base map assets | server/zone | yes
- Moves onto instance maps go only through the instance registry | keeps players out of others' instances | server/zone, server/instance | yes

## Verification

- Design only. No builds, tests or `scripts/check.sh` runs, as the node asks.
- Checked the proposal for em-dashes: none.

## Size

- Estimated M, actual M.
