# 0009: Stats Are Computed Whole From Equipment and Bonuses at Each Change

- Status: proposed.
- Date: 2026-10-02.
- Code: [server/status](../../server/status) (`status.go`, `equip.go`, `bonus.go`), [server/item](../../server/item) (`equip.go`, `wear.go`), [server/battle](../../server/battle) (`bonus.go`), [server/zone](../../server/zone) (`equipment.go`), [tools/convert](../../tools/convert) (`equipment_test.go`).
- Builds on [proposal 0002](../proposals/0002-lua-api.md), [0002a](../proposals/0002a-bonus-names.md), [decision 0007](0007-converted-item-scripts.md), and strategy section 5.3.

## Decision

- A player's stats are a pure function of the character: job, levels, raised stats, learned skills, and what it wears. `status.Calc` ports `status_calc_pc_sub` and `status_calc_bl_main` with equipment, and keeps no state between calls.
- The zone calls it again whenever an input changes: equip, unequip, a card, a refine, a level up, a raised stat, and a bonus that starts or ends, such as a timed bonus of `timers.with_bonuses`.
- Item scripts do not run again on each calculation. Each equipped copy and each card keeps one equip context from the time it is equipped (proposal 0002). At each calculation the zone reads the bonuses each context holds, runs their computed values and `when` conditions, and hands the active ones to `status.Calc` as plain values.
- `status.Calc` applies the bonuses in rAthena's order: equipment scripts by equip index, then the ammunition, then cards by the slot of the item that holds them. Static bonuses of item data come first in their item's list.
- Stacking (0002a, "Stacking") is resolved before the bonuses apply:
  - "highest" and "lowest" keep one bonus of their set;
  - in a one-value set, the highest priority wins, and among equal priorities the last in rAthena's order.
- Each bonus then goes through a port of `pc_bonus` to `pc_bonus5`, by name and options. It changes the same field rAthena changes, with the same rules for the left hand, the shield and the arrow (`lr_flag`).
- Bonuses that attacks read go to `battle.Player`: the sums of `"damage"` and `"damage_taken"` by 0002a's groups, critical and range rates, DEF piercing and so on. Bonuses that later systems read, such as procs, skill costs and drops, are kept out for now.

## Why compute everything again

| | Event-driven totals (strategy 5.3) | Whole calculation at each change |
|---|---|---|
| Matches rAthena | Needs care: rAthena rebuilds every field from zero, so rounding, caps and the order of rates come out of that. | By construction: the code follows `status_calc_pc` line by line. |
| Cost | Small updates. | 5.4 µs for ten items with 30 bonuses (`BenchmarkCalcEquipped`), plus 250 to 320 ns per computed value in Lua. Changes happen on player actions, not every tick. |
| The full-recalculation check of strategy 5.3 | A debug mode compares the two. | Always true: there is only the full calculation. |
| Code | Each bonus needs an inverse for removal, and each rate a way to undo it. | One path. |

The event-driven design remains possible later, behind the same `status.Calc` result, if a profile shows the calculation in a tick.

## Other choices

- The set of a one-value bonus does not include its argument. `attack_element` with fire and with water are one set, so `stacking = "priority"` can choose between them. Each hand and the arrow keep their own `attack_element` set, as rAthena keeps `rhw.ele`, `lhw.ele` and `arrow_ele`.
- Equipment data comes from a new extract, `equipment.json`, with `refine_db.json` for refines and `monster_groups.json` for the race groups that `bAddRace2` and `bSubRace2` read. The rAthena fork's `extract.py` needs the matching functions; their output is identical (reference/rathena/README.md, "Equipment").
- The reference was recorded on arm64 Linux, where `char` is unsigned. Pre-renewal stores DEF and MDEF in a `signed char` but caps them to `CHAR_MIN` and `CHAR_MAX`, which are 0 and 255 there. `status` follows the recording on every platform, as it already follows the recording's fused multiply.
- `item_noequip.txt` restrictions apply: an item or card restricted on normal maps keeps its DEF and ATK, but its script does not run. Every map is a normal map until PvP and GvG maps exist.
- Card slots are a list, in memory, in the database and in the protocol (strategy 6.8). The database allows 16 cards and refines up to 255, so a ruleset can raise `MAX_SLOTS` or `MAX_REFINE` without a migration. The world uses rAthena's 4 and 20 (10 in pre-renewal).
