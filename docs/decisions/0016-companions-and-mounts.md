# 0016: Companions, Mounts and Carts

- Status: proposed.
- Date: 2026-10-03.
- Code: [server/companion](../../server/companion), [server/gamedata](../../server/gamedata) (`companions.go`), [server/zone](../../server/zone) (`companions.go`), [server/status](../../server/status) (`status.go`, `changes.go`), [server/modscript](../../server/modscript) (`companions.go`), [server/store](../../server/store) (migration 00014), [tools/convert](../../tools/convert) (`npcexpr.go`), [client](../../client) (`src/viewer/mounts.ts`), [mods/examples/familiars](../../mods/examples/familiars).
- Builds on [decision 0012](0012-second-classes-and-job-changes.md), [decision 0015](0015-job-definitions.md), and [proposal 0002](../proposals/0002-lua-api.md), review decision 25.

## Decision

- Companions and mounts are records of `data/companions.yml` and `data/mounts.yml`, with the fields of review decision 25. Core's records are files of the core mod, kept in `server/companion/core`:
  - Companions: `falcon`.
  - Mounts: `peco_peco`, `grand_peco` and `gryphon`, which are all rAthena's `OPTION_RIDING`, and `dragon`, `warg` and `madogear`.
- A record's `acts_as` picks one of rAthena's option checks: `pc_isfalcon`, `pc_isriding`, `pc_isridingdragon`, `pc_isridingwug` or `pc_ismadogear`. The zone reads these checks from the player's records, not from the option bits. So a mod's record that acts as the falcon works with Blitz Beat and the other falcon skills, and one that acts as the Peco Peco follows the riding rules.
- A record's `look` sets the option bits that show it, as rAthena's `OPTION_FALCON`, `OPTION_RIDING`, `OPTION_DRAGON1` to `OPTION_DRAGON5`, `OPTION_WUG`, `OPTION_WUGRIDER` and `OPTION_MADOGEAR`. The protocol already carries the option bits, so it needed no change.
- The mounted rules of `status_calc_pc` and `status_calc_speed` now cover every core mount:
  - Walk speed: 25% faster on a Peco Peco or a dragon, 15% plus 5% per level of Warg Rider on a warg, and the Madogear's own speed, which ignores other statuses.
  - ASPD: the riding penalty less Cavalier Mastery, and the dragon's less Dragon Training, in both modes.
  - Weight: 10,000 with Riding, 5,000 plus 2,000 per level of Dragon Training, and 15,000 on a Madogear.
  - Spears hit medium targets as large ones, and Spear Mastery adds 5 per level, on a Peco Peco or a dragon.
  - Mainframe Restructure adds DEF on a Madogear.
- A mount record's `speed` replaces the walk speed bonus of its rules. Without it, the rules decide.
- Record `bonuses` are static bonuses, which `status.Calc` runs after the timed bonuses while the player has the record.
- A skill's requirements take `companion`. The caster's companion must be that record or act as it.
- The calls are those of review decision 25. `player:set_companion` and `player:set_mount` return false when the player does not meet `requires`. `player:set_cart` returns false without Pushcart, as `pc_setcart`, and for a look above 5, as rAthena's cart table without `NEW_CARTS`.
- A job change removes what the new job does not meet `requires` for, and the Madogear always, as `pc_jobchange`. `resetskill` removes what needs a skill the reset takes, as `pc_resetskill`.
- Migration 00014 saves the record names beside the option bits of migration 00012. A character with empty names gets core's records from its bits. A name whose record is gone is dropped when the character loads, with a warning.
- A character whose mod job was removed loads as a Novice, at most at job level 10, with a warning. Its skills, stats and items stay. This was the open item of decision 0015.

## Why the looks are option bits

| | Option bits from the look | A new protocol field for the look |
|---|---|---|
| Core's companions and mounts | Exactly what rAthena sends. | The same, in a new form. |
| Mod records | Limited to the looks rAthena has. | Any sprite. |
| Protocol and client change | None in the protocol. The client reads bits it already gets. | A field and its handling. |

Mods can only reuse existing sprites until the client data tables of phase 6 exist, so the option bits cover every look a record can name today.

## Other choices

- `pc_setriding`'s check of `SC_ALL_RIDING`, the rental mount, is left out: the rental mount is not ported.
- The client shows a falcon or a warg as `sprite/companion/<look>` beside the player, and a cart as `sprite/cart/<n>` behind it. The GRF importer does not name these sprites yet, so builds show the placeholder sprite.
- The client picks the mounted body of a third class from its option bits, as the game client does, such as `rune_knight2` for a Rune Knight on a dragon. A job without a mounted sprite shows the job's costume mount from `jobs.yml`. The server keeps rAthena's swap for the Peco Peco riders, now with the baby Knight and Crusader.
- The converter maps `checkfalcon`, `checkriding` and `checkcart` to `player.companion ~= nil`, `player.mount ~= nil` and `player.cart ~= nil`. For core's records this is rAthena's result, except that `checkriding` is also true on a dragon, a warg or a Madogear.

## Parity

A new recording per mode, `mount_status.json`, from rAthena fork commit `5e50b7ad9`: mounted and unmounted players of the Knight and Crusader lines, the babies, and in renewal the Rune Knight, Royal Guard, Ranger and Mechanic, with no weapon, a one-handed spear, a two-handed spear and a one-handed sword. `TestMountParity` matches the battle status, the weight limit, the option bits and a forced-minimum normal attack on an Orc Warrior in 74 of 74 cases in renewal and 48 of 48 in pre-renewal. The damage of the 19 renewal cases with the Taurus Sword is left to the equipment tests, since golden zones do not run item scripts.

## Left

- The warg as a companion (`OPTION_WUG`) and the dragon colors as records, with `setdragon`, `setmadogear`, `checkdragon`, `checkwug` and `ismounting` in the converter.
- Cart storage, which needs cart items.
- Companions that fight on their own, companion events, and more than one companion at a time (review decision 25).
- Names for the companion and cart sprites in the GRF importer.
- The rental mount, `SC_ALL_RIDING`.
