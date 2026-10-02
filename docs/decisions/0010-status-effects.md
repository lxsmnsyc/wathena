# 0010: Status Effects Run in a Go Engine Driven by status.yml

- Status: proposed.
- Date: 2026-10-02. Updated the same day after the build: stats change inside the script's call, and timed bonuses follow `bonus_script` for a repeated key (see "Changes after the build").
- Code: [server/statuschange](../../server/statuschange), [server/status](../../server/status) (`changes.go`), [server/zone](../../server/zone) (`statuses.go`), [server/modscript](../../server/modscript) (`status.go`), [tools/convert](../../tools/convert) (`status.go`, `usestatus.go`, `npcstatus.go`), [server/store](../../server/store) (migrations 00009 and 00010), [internal/script](../../internal/script) (nested calls).
- Builds on [proposal 0002](../proposals/0002-lua-api.md), review decision 19, [decision 0009](0009-stats-as-totals-of-modifiers.md), and strategy section 5.3.

## Decision

- Status effects run in Go, in the package `statuschange`. It ports rAthena's `status_change_start`, `status_change_start_post_delay`, `status_change_end`, `status_change_clear`, `status_change_timer` and `status_get_sc_def`. Each function names the rAthena function it ports.
- The data comes from rAthena's `db/<mode>/status.yml`, through a new extract, `status.json`: icons, durations from the skill named by `DurationLookup`, states, calc flags, opt1 to opt3, options, flags, `Fail`, `EndOnStart`, `EndReturn`, `EndOnEnd` and the script. The engine reads the flags (`NoSave`, `NoDispell`, `RemoveOnDamaged`, `BossResist`, `StopWalking` and the rest) instead of lists in code.
- Each zone has one engine. It keeps every status timer in one heap, by due time. The zone runs it once per tick, as rAthena's timer loop runs `status_change_timer`. Rolls use the zone's secret random stream.
- The engine works on players, monsters and NPCs through one interface, `Unit`. It talks to the zone through another, `Env`: recalculation, icons, options, damage, heals, the script events and the hooks.
- Statuses change stats through `status.Calc` (decision 0009). Its `status_calc_bl_main` port reads the active statuses with the `status_calc_*` functions of rAthena. Status scripts, such as SC_INCSTR's `bonus bStr`, convert once at load to bonus templates that read the status's values. A monster's status is computed again from its base status when a status changes it.
- Integer math follows rAthena, with its casts: `int32` durations, `uint16` level differences in renewal resistance, and the same order of rates.
- Saved statuses follow `chrif_save_scdata`: every status without `NoSave`, with the time to its next timer and its values. Login restores them with `SCSTART_LOADED`, for the reason `"login"`. A status is stored by name, so a mod status keeps working when its ID changes.
- Timed bonuses (`player:add_timed_bonuses`, rAthena's `bonus_script`) are stored with their entries as JSON, their time left, `ends_on`, the icon and the kind. Each copy is its own row, so several copies of one key come back at login.

## Why a Go engine

| | Statuses in Lua | Statuses in Go |
|---|---|---|
| Matches rAthena | A second port of every rule, in another language. | One port, next to `status.Calc` and `battle`, which read the same lists. |
| Cost | A Lua call per tick per status. Poison on 500 monsters runs 500 calls a second. | A heap of timers. Only mod statuses and the events call Lua. |
| Mods | Could define any status. | Define statuses with icons, bonuses and ticks (decision 19). Their ticks run in the `status_tick` event. |
| Parity tests | Need a Lua state. | Run against `status_effects.json` directly: 2,640 of 2,640 cases match in each mode. |

## Other choices

- Statuses that a script starts (`start_status`) go through the hooks before Go starts them. A host call may call Lua again only to compute stats, so the prelude asks Go for rAthena's chance and duration, runs the `status_chance` and `status_duration` hooks, and passes their results with the start. Starts that Go makes, such as `cause_status` procs and restored statuses, run the hooks directly.
- A player's stats are computed again at once when a status starts or ends, as rAthena's `status_calc_bl` inside `status_change_start` and `status_change_end`. This holds inside a script's call too (see "Changes after the build").
- The events of statuses that start or end inside a script call fire after that call returns:
  - A `status_start` handler that starts a status cannot loop inside one call.
  - rAthena has no such events, so no rAthena order applies.
- Mod statuses get IDs from 100,000, from one registry that every zone shares, so a status has the same ID on every map.
- An interval status ticks once more when its time runs out, as rAthena's poison does. A mod status with `tick_milliseconds` does the same.
- A timed bonus with a key the player has follows `pc_bonus_script_add`, through the option `on_repeat` (see "Changes after the build"). The converter keys timed bonuses by their script's text, as rAthena tells copies apart by that text.
- Statuses with the `StopWalking` flag stop a walk when they start. While a `NoMove` state holds, new walks are refused, as `unit_can_move` refuses them in `unit_walktoxy`. Confusion picks another cell for a walk, as `unit_walktoxy` does with `map_random_dir`.
- `SC_STONE` keeps rAthena's two stages: `SC_STONEWAIT` first, then stone after the wait.
- A map change ends the statuses with `RemoveOnMapWarp` or `RemoveOnChangeMap`. The rest move with the player to the new map without events.

## Changes after the build

The user approved two changes on 2026-10-02.

### Stats change inside the script's call

Before, a script that started a status saw the new stats only at the end of the tick, since computing stats runs computed bonuses and `when` conditions in Lua. Now:

- A host function may make a nested call into its own state to compute stats ([internal/script/README.md](../../internal/script/README.md), "Nested calls", and [decision 0002](0002-lua-runtime-c-lua.md), "Nested calls"). It shares the outer call's limits and cannot yield. Every other re-entry still gets `ErrReentrant`.
- `start_status`, `end_status`, `add_timed_bonuses`, `bonuses.add`, a bonus's `remove`, and the bonus calls of `timers` compute the player's stats before they return. A script reads the new values after the call, as in rAthena.
- A `when` condition reads the stats of the last calculation, as rAthena's `readparam` reads `battle_status` during `status_calc_pc`. A condition on a stat that a status changes sees the change at the next calculation, which may be in the same call.
- While equipment changes and while a player joins, stats wait, and the change computes them once at its end.
- Changes that no script's call makes, such as a `with_bonuses` timer that runs out, are computed at the end of the tick. The special case for item use scripts is gone.
- Events still fire after the outer call, for the reasons above.

### Repeated timed bonuses

`player:add_timed_bonuses` takes the option `on_repeat`, which replaces `stack`. The name is `on_repeat` because `repeat` is a Lua keyword. It decides what a call does when the player has timed bonuses with the same key. Only the first copy with that key counts, as in `pc_bonus_script_add`:

| `on_repeat` | rAthena | Effect |
|---|---|---|
| `"keep"`, the default | no flag | The new bonuses are ignored. |
| `"extend"` | `BSF_FORCE_REPLACE` (0x400) | The first copy keeps its values and takes the new end when it is later. Otherwise the call is ignored. |
| `"add"` | `BSF_FORCE_DUPLICATE` (0x800) | A second copy applies too. |
| `"replace"` | none | Every copy ends, and the new one starts with its values and time. |

- The method returns true when the bonuses were added or extended, and false when they were ignored.
- A second use of an item with a random value no longer draws the value again under the default.
- The converter writes `"extend"` for 0x400 and `"add"` for 0x800, and nothing for neither.
- Migration 00010 drops the `stack` column. Saved copies come back with `"add"`, so each copy returns.

## Left

- The effects of statuses that skills start: about 615 renewal statuses start, save, end, and show their icon and option, but do nothing else yet. Their effects come with the skills in phase 4 steps 3 and 4.
- `SC_BOSSMAPINFO`, Kyrie Eleison's barrier, Golden Thief Bug's immunity to magic (`status_isimmune`), revealing hidden units, the casts of Magic Mushroom, the sitting of Stomach Ache, the wall check of Cloaking, and `sc_end SC_ALL`.
