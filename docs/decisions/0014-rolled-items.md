# 0014: Rolled Item Values and Random Options

- Status: proposed.
- Date: 2026-10-03.
- Code: [server/item](../../server/item) (`roll.go`, `random.go`, stacking in `inventory.go`), [server/zone](../../server/zone) (`rolls.go`, drops in `loot.go`), [server/mod](../../server/mod) (`random.go`, rolls in `items.go`), [server/modscript](../../server/modscript) (`rolls.go`), [server/store](../../server/store) (migration 00013), [tools/convert](../../tools/convert) (`randomopt.go`), [mods/examples/lucky-clover](../../mods/examples/lucky-clover).
- Builds on [decision 0009](0009-stats-as-totals-of-modifiers.md), strategy section 5.4, and [proposal 0002](../proposals/0002-lua-api.md), review decision 23.

## Decision

- An item's data declares its rolls: `min`, `max`, `default`, `distribution` and `hidden`. Values are whole numbers that fit in 32 bits, and an item has at most 8 rolls.
  - `uniform` draws each value with the same chance.
  - `triangular` gives a value at or below the default the weight `(v - min + 1) * (max - default + 1)`, and one above it `(max - v + 1) * (default - min + 1)`. Both sides meet at the default. The draw is exact: one number from the stream, then a binary search over the closed form of the weights.
  - `weighted` picks a tier by weight, then a value of the tier's range.
- New copies roll from the zone's secret random stream: monster drops, MVP prizes, and gives with `roll = "random"`. Gives without it and NPC shops use the defaults. With `item_rolls` off, every roll is its default, as rAthena.
- The `item_roll` hooks run for every roll of every new copy, defaults included, so a hook can change what shops sell. A copy that a script makes inside its call, through `give_item` or `reroll`, runs the hooks as a nested call (internal/script, "Nested calls"). The skill hooks skip nested calls instead, but review decision 23 names the source `{ kind = "script" }`, which only happens inside a call.
- rAthena's random options become core's `data/random_options.yml` and `data/random_option_groups.yml`. `s_random_opt_group::apply` is ported draw for draw. Options roll whatever `item_rolls` says, as rAthena: on drops with a group, and on equipment from an item group entry with a group, as `getgroupitem`.
- An option's `bonus` is a list of bonus entries, since `ATTR_TOLERACE_ALLBUTNOTHING` gives eight. `{ roll: value }` and `{ roll: param }` read the option's value and param. Ten options divide the value by 10, and three negate it, so a roll in a bonus takes `multiply` and `divide`.
- Options apply after every card, in their own loop, as `status_calc_pc_sub`. A weapon in the left hand applies its options to the left hand.
- A copy's extra state is its rolls, its cards' rolls, its options, and whether it is identified. Stacking compares all of it, and an unidentified copy of an item with a hidden roll never stacks.
- A copy that is rolled, and whose item has a hidden roll, starts unidentified. The client gets a hidden roll's range, not its value, until the copy is identified.
- The client shows each roll as its label, value and range. The label comes from the first static bonus that reads the roll, through `bonus.Describe`, or from the roll's name. Random options use their `display_name`, which the converter writes with the same function.

## Storage

| | One `rolls` column for everything | Columns per kind (chosen) |
|---|---|---|
| A copy's rolls | `{"rolls": {...}, "options": [...]}` | `rolls` is `{"boss_damage": 31}`, as strategy section 5.4 shows. |
| Card rolls | Inside the same object. | `card_rolls`, a list in the order of `cards`. `cards` stays `integer[]`. |
| Checks | One shape check. | A type check per column. |
| The down migration | Drops one column. | Drops four columns. |

- Options are saved by name, not by rAthena's ID. A mod's options have no ID, and core's names are part of the API.
- The server checks every copy when a character enters a zone: values outside a roll's range are clamped, rolls and options the data no longer has are dropped, and rolls the item gained get their default. Each change is logged.

## Card handles

Strategy section 5.4 shows a card's script reading `equipped.rolls.boss_damage`. Before this step, a card's equip context shared the item's handle, so it would read the item's rolls.

- Each card's context now gets its own handle. It reads the item's fields, such as `item` and `refine`, as before. Its `rolls` are the card's own, and `set_roll` and `reroll` change the card's rolls.
- `player:get_equipped` still returns the item's handle. `cards` on it lists every card with its rolls.

## Rerolls

- `set_roll` and `reroll` change the slot at once and send `ItemRollsChange`. The copy's equip contexts end quietly and start again once the script's call returns, since a host call cannot start Lua. The handles stay the same, so a script that holds one reads the new values after the call.
- Unequip handlers do not run for a reroll: the copy stays on.

## Parity

- `item_rolls` off: every roll is its default, which is what rAthena's items have. Core's items have no rolls, so with the flag off every core item matches rAthena.
- Random options: `random_option_rolls.json` from fork commit `8e1c56558`. With every draw at its lowest, all 108 renewal groups and all 179 monster drops with a group give rAthena's options. Over 100,000 seeded calls per group, the count of options per copy, each option's share at each index, and the values' ends and means match within six standard errors in 3,363 of 3,363 cells. Pre-renewal has no options, as rAthena.
- Distributions: 100,000 draws of each distribution stay in range and pass a chi-square test (`TestRollDistributions`).

## Other choices

- Monsters that loot keep a picked up copy's refine, cards and rolls, and drop them when they die.
- A part of a stack dropped on the ground keeps the stack's serial as its origin. The copy picked up later gets a new serial, or joins a stack, and the server logs `item split` with both serials and the rolls. There is no item log table yet, so this is a log line.
- `wa.testing` cannot equip items or read an inventory copy's rolls. The sample mod tests what a test can read, and `TestLuckyClover` in `tools/modkit` checks the rest in Go, through two Go helpers of `zone.Headless`.

## Left

- Identifying items: Item Appraisal and Magnifiers need a window. Until then, rolled copies with hidden rolls stay unidentified, and their hidden values never show. They can still be worn.
- Rolls on crafting and quest rewards, roll searches in vending, and handles for inventory copies, which review decision 23 leaves for later.
- The ten `RACE_WEAPON_TOLERACE_*` options (`bSubRace` with `BF_WEAPON`), which have no `damage_taken` sum. No group uses them.
- Item names and descriptions of mod items in the client's tables (phase 6). The client shows a mod item as "Item 2000000".
- Item logs in the database.
