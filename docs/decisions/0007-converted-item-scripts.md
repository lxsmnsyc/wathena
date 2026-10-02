# 0007: Converted Item Scripts Are Made at Startup, and Static Bonuses Are Item Data

- Status: proposed.
- Date: 2026-10-01.
- Code: [tools/convert](../../tools/convert), [server/bonus](../../server/bonus), [server/modscript](../../server/modscript), [mods/README.md](../../mods/README.md), "Item scripts".
- Builds on [proposal 0002](../proposals/0002-lua-api.md), [0002a](../proposals/0002a-bonus-names.md) and [decision 0006](0006-mod-loader.md).

## Decision

- `waserver` converts rAthena's item scripts when it starts, from `reference/rathena/extracts/<mode>/item_scripts.json`, which is built into the binary. The result is core's files in memory: `data/items.yml` and `scripts/items/<AegisName>.lua`. The mod loader reads them like any mod's files.
- The converted files are not checked in. `wactl convert items --out <folder>` writes them for reading.
- What is checked in, and checked by `go test ./tools/convert`:
  - the coverage reports, `docs/generated/item-scripts-<mode>.md`;
  - the converted output of 23 sample items, `tools/convert/testdata/golden`;
  - the comparison with rAthena's `item_bonuses.json` and `refine.json`, `tools/convert/testdata/equipment_parity_<mode>.md` (decision 0009).
- Equipment whose scripts are only constant bonuses becomes item data, not Lua (strategy section 5.3, rule 5). Each bonus is a list: the name, then the arguments of `bonuses.add`, with constants and records by name.

## Where the converted scripts live

Measured on renewal and pre-renewal together:

| | Checked in under mods/core | Made at startup |
|---|---|---|
| Files in the repository | 13,595 Lua files and 2 data files, 15.9 MB | None. The input extracts add 3 files per mode, 11.6 MB, which the tests need anyway. |
| Review of a converter change | Thousands of changed files. | The golden samples, the report, and the parity numbers change. Each is small. |
| A stale output | Needs a CI check that runs the converter. | Cannot happen. |
| Server start | No work. | 0.7 s to convert, and 0.4 s for the item overlay of 20,053 items. |
| Binary size | 16 MB more of Lua. | 12.3 MB more of JSON extracts, which gzip to 1.1 MB. |
| Hand fixes (strategy 5.6, step 6) | Edit the file, and keep the converter from writing it. | A mod overrides an item's `script` or `bonuses` in its `data/items.yml`. |

Startup conversion costs about a second, and the repository stays small. `wactl convert items --out` writes the scripts for reading.

## Static bonuses as item data

```yaml
- {name: Poring_Card, bonuses: [[luk, 2], [perfect_dodge, 1]]}
- {name: Knight_Of_Abyss_Card, bonuses: [[damage, 25, {kind: physical, against: {class: boss}}]]}
```

- An entry has the shape of a `bonuses.add` call, and of an entry of `player:add_timed_bonuses` (0002a, "bonus_script"). One reader serves both, and a mod author reads one form.
- Names are used, not numbers: `demi_human`, `SM_BASH`, `Red_Potion`, `PORING`. A typo fails at start.
- `server/bonus` checks every entry with the rules of `bonuses.add`. The Lua runtime, the item data and the converter use the same table, so they accept the same bonuses.
- One line per item keeps the file readable and its diffs small.
- 6,125 renewal items are static. The server applies them without Lua when equipment exists (phase 4).

## Other choices

- A zone makes its Lua state when a player first uses an item, unless a mod has entry scripts. The 988 maps of a kRO build hold no state until someone uses an item there.
- A zone keeps at most 256 item scripts loaded. Loading one more drops the oldest. Contexts that run keep what they use.
- Converted equipment scripts run symbolically. A condition on a game value becomes the bonus's `when` option, and a game value in an amount becomes a computed bonus, so the bonus follows the value without rerunning the script.
