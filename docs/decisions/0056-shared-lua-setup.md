# 0056: Shared Lua Setup

- Status: proposed. Not built.
- Date: 2026-10-20.
- Code: [internal/script](../../internal/script) (`script.go`, `csrc/glue.c`, `engine_cgo.go`, `engine_wazero.go`), [server/modscript](../../server/modscript) (`runtime.go`, `host.go`, `prelude.lua`, `items.go`), [server/zone](../../server/zone) (`scripts.go`, `zone.go`).
- Follows decision [0001](0001-lua-runtime.md), which says loading "should share compiled chunks between zones", and decision [0002](0002-lua-runtime-c-lua.md).
- Lua API: none. Protocol: none. Settings: none.

## Context

Each zone has its own Lua state, used only by the zone's goroutine (strategy section 4.1). That stays. Zones run in parallel, and a script that hits a limit or fails stops only its own zone. One state for all zones would make every zone's scripts wait for each other.

Most loading is already lazy:

- The server reads each mod's Lua files from disk once, into `mod.Scripts` (`server/mod/scripts.go`). Every zone uses the same source strings.
- Core has no entry scripts. With only core, no zone makes a state at boot.
- An item's script loads on the first use of that item in the zone (`setup` in `server/modscript/items.go`).
- NPC scripts start on the first player join (`startNPCs` in `server/zone/npcs.go`), and only for the NPCs placed on that zone's map.
- Skill scripts load on first use (`skillContext` in `server/modscript/skills.go`).
- Item, skill and monster data are read through host functions, not copied into Lua. Places, the atlas, quests and texts stay in Go too.

What is still done again in every state:

1. **The prelude is compiled and run in every state.** `prelude.lua` has 3,298 lines. `Runtime.init` loads it as text in each new state.
2. **Every file is compiled again in each state that uses it.** `scripts/lib/rathena.lua` and the converted functions are used by many NPCs, so most zones with players compile them. The sandbox accepts only text (`glue_lload` in `csrc/glue.c`, and `Load` in `script.go`), so there is no way to share compiled code.
3. **The constant modules are copied twice into each state.** `constantsTable` (`server/modscript/host.go`) builds a Lua table of every constant module. Then the prelude copies each module into a new table (the loop over `H.constants()` in `prelude.lua`).
4. **Every zone makes a state at boot when any mod has entry scripts.** `startScripts` (`server/zone/scripts.go`) runs from `Zone.Run` for all zones, and a world has hundreds of maps (strategy section 3.7 starts all 988 kRO maps). Each state then runs every mod's setup, even in zones no player has entered.

No measurement shows how much each of these costs. Decision 0001's estimate of 0.5 s per state for 20,000 item scripts was for golua and bulk loading, and neither applies now.

## Decision

Measure first, then remove the repeated work in this order. Each step must keep `scripts/check.sh` passing.

### 1. Measure a state

Add a benchmark in `server/modscript` and record the results in this file:

- Time and memory (`Memory`, and Go heap) to make a state with core and the prelude.
- The same, plus starting the NPCs of a busy town such as `prontera`.
- The number of states and the total Lua memory after boot with one mod that has an entry script, for a world of all maps.

Stop after this step if the numbers are small. The steps below are worth their code only where the measurement shows a cost.

### 2. Make states lazily in every zone

- `startScripts` makes the state at boot only for zones that are awake. A sleeping zone makes it when it wakes (`Zone.wake`), before its first tick.
- Check first what a mod's setup expects in a zone with no players. Timers do not run in a sleeping zone, since its ticker is off. A mod that counts on its setup having run in every zone, for example to register places or shared data, must still work.
- This is likely the largest saving when mods have entry scripts, since most maps have no players.

### 3. Build the constants once per state

- The prelude uses the tables from `H.constants()` directly and makes them read-only, without the copy. The `jobs` module's functions are added to its table.
- If the measurement shows the tables are large, serve them through `__index` on a host function, so each state holds none of them.

### 4. Compile once, load compiled code in each state

- The server compiles each source file the first time any state needs it, in a private state, with `lua_dump`. It keeps debug information, so errors still name the file and line.
- A cache in `internal/script`, shared by all states, holds the compiled code by mod, path and a hash of the source. A reload makes a new source string, so it misses the cache.
- A new internal load path accepts compiled code only from that cache. `load`, `require` from mods, and `Load` keep refusing bytecode. Compiled code never comes from disk, a mod, or a player.
- The prelude uses the same cache.
- Charge the same CPU units as compiling the text would (`256 + len/2` in `glue_lload`), so mod budgets do not change with the cache.
- Both engines need it: the cgo build and `lua.wasm`. They run the same Lua 5.4.9, so compiled code from one state loads in any other state of the same build.

## Breaks

- Nothing for clients, mods or worlds.
- Step 2 changes when a mod's setup runs in a sleeping zone. A mod that expects its setup in every zone at boot would see it later. The check in step 2 decides whether this needs a note in mods/README.md.
- Step 4 adds a second way into the Lua loader. Its tests must show that the cache path cannot be reached from Lua or from mod files.

## Alternatives

- **One Lua state for all zones.** C Lua allows one thread in a state at a time. All zones would wait on one lock, and a script that hits a limit would stop every zone. Not chosen.
- **Copy a set-up state.** In the `lua.wasm` build a state is one block of linear memory, so a state made once could be copied into each zone. The cgo build cannot do this, and refs held by Go and host function tables would need care. Not chosen now. It can be tried if steps 2 to 4 are not enough.
- **Accept bytecode from mod files.** Lua does not check bytecode, so a hand-made chunk can crash the server or escape the sandbox. Not chosen.

## How to check it worked

- Run the benchmark from step 1 before and after each step. Record the results here.
- Boot a world of all maps with one mod that has an entry script. The number of states after boot equals the number of awake zones.
- `scripts/check.sh` passes, including `internal/script`'s sandbox and limit tests.
- Change the status to accepted when the steps that the measurement justified are built.
