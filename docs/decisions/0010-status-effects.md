# 0010: Status Effects Run in a Go Engine Driven by status.yml

- Status: proposed.
- Date: 2026-10-02.
- Code: [server/statuschange](../../server/statuschange), [server/status](../../server/status) (`changes.go`), [server/zone](../../server/zone) (`statuses.go`), [server/modscript](../../server/modscript) (`status.go`), [tools/convert](../../tools/convert) (`status.go`, `usestatus.go`, `npcstatus.go`), [server/store](../../server/store) (migration 00009).
- Builds on [proposal 0002](../proposals/0002-lua-api.md), review decision 19, [decision 0009](0009-stats-as-totals-of-modifiers.md), and strategy section 5.3.

## Decision

- Status effects run in Go, in the package `statuschange`. It ports rAthena's `status_change_start`, `status_change_start_post_delay`, `status_change_end`, `status_change_clear`, `status_change_timer` and `status_get_sc_def`. Each function names the rAthena function it ports.
- The data comes from rAthena's `db/<mode>/status.yml`, through a new extract, `status.json`: icons, durations from the skill named by `DurationLookup`, states, calc flags, opt1 to opt3, options, flags, `Fail`, `EndOnStart`, `EndReturn`, `EndOnEnd` and the script. The engine reads the flags (`NoSave`, `NoDispell`, `RemoveOnDamaged`, `BossResist`, `StopWalking` and the rest) instead of lists in code.
- Each zone has one engine. It keeps every status timer in one heap, by due time. The zone runs it once per tick, as rAthena's timer loop runs `status_change_timer`. Rolls use the zone's secret random stream.
- The engine works on players, monsters and NPCs through one interface, `Unit`. It talks to the zone through another, `Env`: recalculation, icons, options, damage, heals, the script events and the hooks.
- Statuses change stats through `status.Calc` (decision 0009). Its `status_calc_bl_main` port reads the active statuses with the `status_calc_*` functions of rAthena. Status scripts, such as SC_INCSTR's `bonus bStr`, convert once at load to bonus templates that read the status's values. A monster's status is computed again from its base status when a status changes it.
- Integer math follows rAthena, with its casts: `int32` durations, `uint16` level differences in renewal resistance, and the same order of rates.
- Saved statuses follow `chrif_save_scdata`: every status without `NoSave`, with the time to its next timer and its values. Login restores them with `SCSTART_LOADED`, for the reason `"login"`. A status is stored by name, so a mod status keeps working when its ID changes.
- Timed bonuses (`player:add_timed_bonuses`, rAthena's `bonus_script`) are stored with their entries as JSON, their time left, `ends_on`, `stack`, the icon and the kind.

## Why a Go engine

| | Statuses in Lua | Statuses in Go |
|---|---|---|
| Matches rAthena | A second port of every rule, in another language. | One port, next to `status.Calc` and `battle`, which read the same lists. |
| Cost | A Lua call per tick per status. Poison on 500 monsters runs 500 calls a second. | A heap of timers. Only mod statuses and the events call Lua. |
| Mods | Could define any status. | Define statuses with icons, bonuses and ticks (decision 19). Their ticks run in the `status_tick` event. |
| Parity tests | Need a Lua state. | Run against `status_effects.json` directly: 2,640 of 2,640 cases match in each mode. |

## Other choices

- Statuses that a script starts (`start_status`) go through the hooks before Go starts them. The Lua runtime cannot call Lua again from inside a host call, so the prelude asks Go for rAthena's chance and duration, runs the `status_chance` and `status_duration` hooks, and passes their results with the start. Starts that Go makes, such as `cause_status` procs and restored statuses, run the hooks directly.
- A player's stats are computed again at the end of the tick when a script call starts or ends a status, since computed bonuses run Lua. An item's use script is the exception: stats are computed as soon as it returns.
- The events of statuses that start or end inside a script call fire after that call returns.
- Mod statuses get IDs from 100,000, from one registry that every zone shares, so a status has the same ID on every map.
- An interval status ticks once more when its time runs out, as rAthena's poison does. A mod status with `tick_milliseconds` does the same.
- `bonus_script` keeps the first copy of a script unless `BSF_FORCE_REPLACE` is set. The approved API replaces a timed bonus with the same key by default. The converter keys timed bonuses by their script's text, so a second use of the same item extends the bonus instead of keeping the first.
- Statuses with the `StopWalking` flag stop a walk when they start. While a `NoMove` state holds, new walks are refused, as `unit_can_move` refuses them in `unit_walktoxy`. Confusion picks another cell for a walk, as `unit_walktoxy` does with `map_random_dir`.
- `SC_STONE` keeps rAthena's two stages: `SC_STONEWAIT` first, then stone after the wait.
- A map change ends the statuses with `RemoveOnMapWarp` or `RemoveOnChangeMap`. The rest move with the player to the new map without events.

## Left

- The effects of statuses that skills start: about 615 renewal statuses start, save, end, and show their icon and option, but do nothing else yet. Their effects come with the skills in phase 4 steps 3 and 4.
- `SC_BOSSMAPINFO`, Kyrie Eleison's barrier, Golden Thief Bug's immunity to magic (`status_isimmune`), revealing hidden units, the casts of Magic Mushroom, the sitting of Stomach Ache, the wall check of Cloaking, and `sc_end SC_ALL`.
