# 0008: Converted NPCs Are Made at Startup From a Smaller Extract

- Status: proposed.
- Date: 2026-10-02.
- Code: [tools/convert](../../tools/convert) (`npc*.go`, `rathena.lua`), [server/zone](../../server/zone) (`npcs.go`, `shops.go`), [server/modscript](../../server/modscript) (`npcs.go`, `variables.go`, `services.go`), [mods/README.md](../../mods/README.md), "NPC scripts" and "Core's NPCs".
- Builds on [proposal 0002](../proposals/0002-lua-api.md), decision 13, and [decision 0007](0007-converted-item-scripts.md).

## Decision

- `waserver` converts rAthena's NPC scripts and shops when it starts, as it converts item scripts (decision 0007). The input is two new extracts of each mode, built into the binary: `npcs.json` and `item_prices.json`. The output is core's files in memory: `data/npcs.yml`, `scripts/npcs/*.lua`, `scripts/functions/*.lua` and `scripts/lib/rathena.lua`.
- The converted files are not checked in. `wactl convert npcs --out <folder>` writes them for reading. The coverage reports, `docs/generated/npc-scripts-<mode>.md`, and the output of 15 sample NPCs and functions, `tools/convert/testdata/golden/npcs`, are checked in and checked by `go test ./tools/convert`.
- The NPC files are those that `npc/<mode>/scripts_main.conf` loads, in its order, as for warps. The extract keeps every shop and every function, and the script NPCs of the folders the first release needs: `kafras`, `merchants`, `cities`, `airports`, `guides` and `other`. It only counts the script NPCs of the other folders, such as `quests` and `instances`.
- A script NPC is placed when its own script converts whole. A function may hold parts that do not convert. They become calls that fail when a script reaches them, and the NPCs that call the function count as partly converted.
- Zeny shops become data in `data/npcs.yml`. Cash, item, point and market shops are reported, not converted.

## Why at startup, and why a smaller extract

| | Checked in under mods/core | Made at startup |
|---|---|---|
| Files in the repository | 965 Lua files and a data file for renewal, 3.1 MB, and about as many for pre-renewal. | None. The extracts add 2 files per mode, 10.9 MB in all. |
| Review of a converter change | Hundreds of changed files. | The golden samples and the reports. |
| Server start | No work. | 0.17 s for renewal's NPCs, next to 0.7 s for its items. |

rAthena's `npc/` folder is 29 MB, mostly quests and instances that need features wAthena does not have. Keeping those bodies out keeps the extract at 5.3 MB for renewal and 3.2 MB for pre-renewal. The extractor keeps the folder list in one constant, `SCRIPT_FOLDERS`, and the report lists the counts it left out, so a later step adds folders as their features arrive.

## How rAthena's script maps onto Lua

| rAthena | Converted |
|---|---|
| The script from its start to the first `On` label | The `click` handler. |
| `OnInit:` | The setup function. Other event labels, such as `OnTouch` and `OnTimer`, do not convert yet. |
| `.@x` | A local of the Lua function, declared at its top, so `goto` never enters a local's scope. |
| `.x` | A local of the setup function, shared by the NPC's clicks. |
| `x`, `@x`, `#x` and `##x`, `$x` | `wa.variables` of the character, the character until logout, the account, and the server. A string variable's name gets `_text`, since rAthena keeps `x` and `x$` apart. |
| Arrays | Lua tables with keys from 0. Arrays in variables are table variables with string keys. |
| `x += n` on a saved integer | `variable:add(player, n)`. |
| Labels and `goto`, and `menu` | Lua labels and `goto`. |
| `callsub L, ...` | A local function that runs from the label to the end of the script's top level. |
| `callfunc "F", ...` | `require("core.functions.F")(player, conversation, npc, ...)`, loaded on first call, so functions can call each other. |
| `switch` | `goto` to each case, so a case without `break` runs on into the next, as in C. |
| `end` or `close` in a function | `R.stop()`, which `R.run` in the handler catches. |
| `checkre()` | Folded per mode, as rAthena's `RENEWAL` define is. |
| `F_KafStor`, `F_KafCart`, `openstorage`, `setcart` | `R.not_available`: "Storage is not available yet." |
| `F_ClearGarbage` | Nothing: it clears variables of old rAthena versions. |

## Rules taken from rAthena

- Conversations follow `npc_click` and `npc_scriptcont`: 15 cells of reach, one conversation per player, answers checked against the step, and `SECURE_NPCTIMEOUT`'s timeouts. While a player talks it cannot act (`pc_cant_act`), so it cannot walk away. A conversation ends when the player leaves, dies (`pc_dead`), or changes map.
- Shops follow `npc_buylist`, `npc_selllist`, `pc_modifybuyvalue` and `pc_modifysellvalue`. wAthena adds the items to a copy of the inventory first, so two lines of one item that fit one by one and not together change nothing, where rAthena would take the zeny.
- A dead character is saved at its save point, as `pc_makesavestatus`. A player who dies goes back to its save point after the respawn wait, as `pc_respawn`.

## Other choices

- The extract writes one NPC per line, so a change to one NPC is a one-line diff.
- A conversation's pending call raises "the conversation was closed" when it ends without its script. A reload lets conversations that run finish on the old version, while their context has no handlers or timers.
- An NPC image is hidden when the dialog window closes, so a script that ends without `cutin "",255` leaves none on the screen.
