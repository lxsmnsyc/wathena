# 0036: Phase 6 Review Changes

- Status: proposed.
- Date: 2026-10-15.
- Code: [server/store](../../server/store) (migration 00028, `world.go`, `queries/world.sql`, `queries/accounts.sql`), [tools/wactl](../../tools/wactl) (`world.go`), [server/zone](../../server/zone) (`zone.go`, `persist.go`, `chat.go`, `gmcommands.go`, `equipment.go`, `jobchange.go`, `headless.go`), [server/companion](../../server/companion), [server/item](../../server/item), [server/account](../../server/account) (`service.go`, `export.go`), [server/adminworld](../../server/adminworld) (`adminworld.go`, `moderation.go`, `account.go`), [server/cmd/waserver](../../server/cmd/waserver) (`main.go`), [server/modscript](../../server/modscript) (`prelude.lua`, `luaapi/spec.go`), [tools/modkit](../../tools/modkit) (`test.go`), [schema/proto](../../schema/proto) (`event.proto`), [client/src](../../client/src) (`assets/art.ts`, `render/characters.ts`, `viewer/online.ts`, `viewer/mounts.ts`, `viewer/main.ts`), [client/scripts](../../client/scripts) (`exit-check.ts`), [mods/examples/lantern](../../mods/examples/lantern), [mods/examples/town-crier](../../mods/examples/town-crier).
- Builds proposal 0002, review decision 41. Changes decisions [0028](0028-staff-roles-and-audit.md), [0030](0030-moderation-and-gm-commands.md), [0031](0031-world-events-and-mods-tools.md), [0032](0032-account-page.md), [0033](0033-mod-assets.md) and [0035](0035-phase-6-exit-check.md). The Lua API gains `world:plan_restart` in `wa.testing`. The protocol gains three fields of `Look`, recorded in [proposal 0001](../proposals/0001-protocol.md).

## Decision

### accounts.staff

- Migration 00028 drops `accounts.staff`. The column gave no role. Roles come from `staff.owners` and `staff_roles` (decision 0028).
- `wactl world new --admin` makes a plain account. After its line it prints the `staff.owners` lines to add to `waserver.yml`:

  ```text
  made the world "wAthena" (renewal) and the account Admin (id 1)
  The account has no staff role yet. To make it the owner, add staff.owners to waserver.yml and restart waserver:
  staff:
    owners: [Admin]
  ```

### @hide

As rAthena, which keeps `OPTION_INVISIBLE` in the character's saved options (`pc_makesavestatus`) and clears it at login without the hide command (`pc_authok`):

- The character saves `@hide` as the bit 0x40 in `characters.options`. The zone keeps the bit out of the options that looks and option changes send.
- A login comes back hidden while its role has `commands.hide`. The role counts only after a second factor (decision 0028). Any other login comes back visible, and its next save clears the bit.
- A hidden staff member's local chat reaches every player within 9 cells, other staff too, as `clif_parse_GlobalMessage` sends it to the area. The listener does not know the speaker's entity, so the line has `from` 0 and the speaker's name. Block lists still hide it. The client shows such a line in the log without a bubble.

### Bans and sessions

- A ban closes the account's game connections, as before. Then the account service ends the account's sessions on the account pages, which the admin screens share, deletes its logins that wait for a code, and deletes its game tickets that nobody used, in one transaction of the account role.
- The game role gets no new rights. `adminworld.World.SetSessions` takes the account service after it is made, and the ban calls `EndAccountSessions`.
- A failure is logged, and the ban stays in effect. Logins and tickets refuse a banned account already (decision 0030).

### wa.testing: world:plan_restart

- `world:plan_restart(minutes, reason)` plans a restart as the admin screens do. `restart_planned` runs with `{ at, reason }` in the test world's zone, and the lines its handlers send go out. Nothing restarts.
- The minutes are 1 to 10,080 and the reason 1 to 255 characters, as the planner's limits. `at` is the test world's time plus the minutes.
- It is provisional. The Town Crier sample tells its `#events` channel about a planned restart, and its test uses the helper.

### Data export

- The export gains `variables`: the mods' saved variables of the account at the top, and of each character in the character. Each is `{ mod, name, value }`, with the value as the JSON the mod saved, sorted by mod and name.
- New fields keep format version 1. Decision 0032's table lists them.
- `TestExportHasNoSecrets` reads the field names of the JSON, not every string, since a variable's name is data a mod chose.

### Art of companions, mounts, weapons and shields

- `Look.companion` and `Look.mount` are the record IDs of the player's companion and mount, or 0. `/client-data.json` already names the art of records by ID, so the client draws a companion of art beside the player, where the falcon sits, and a mount of art at the player's cell, behind the body.
- A player on a mount of art shows its job's body on foot. The server leaves the mount's option bits out of the body's job, and the client leaves them out of the body's look. The option bits still go out, so the rules and older clients see the record each acts as.
- `Look.weapon_view` is the view of the right hand's weapon when its record names a `view_sprite`, from 1,000,000. `Look.weapon` stays the weapon type. A shield sends its view in `Look.shield` already, so the client draws a shield's art by it, as headgear and garments.
- `item.ArtViews` holds the first view of art, which `gamedata.ItemArtViews` names, so the zone can tell a view of art.

### Lantern sample and the exit check

- The Lamplighter sells the Lantern Shield, a shield with one card slot and a worn sprite.
- The Lantern Wisp is a companion with a sprite of art. It acts as a falcon, since a look of art needs `acts_as`, and has no `requires`. The Lantern Master gives it to a Lantern Keeper, and a second click sends it home.
- The exit check's mod run buys and wears the shield, and counts the blue it adds on Ann. A new step, `mod companion`, takes the wisp from the Lantern Master and counts the glow it adds beside Ann. Both check that the page loaded the sprites from the mod's build.

## Tests

- `server/store`: `TestNewWorld` checks the account without the column, and that neither service role makes a world.
- `tools/wactl`: `TestOwnersHint` reads the printed lines as YAML.
- `server/zone`: `TestGMHide` hears a hidden game master's line by name, and not out of range. `TestGMHideLastsAcrossLogins` saves the bit, logs in hidden, ends it with a second `@hide`, and comes back visible without a second factor or without `commands.hide`. `TestArtViewsInLooks` and `TestLooksCarryCompanionsAndMounts` check the new look fields and the body on foot.
- `server/account`: `TestBanEndsAccountSessions` ends the sessions, a login that waits for its code and a ticket, and keeps another account's session. `TestExport` and `TestExportHasNoSecrets` read the variables.
- `server/adminworld`: `TestSanctions` checks that a ban ends the account's sessions. `TestAccountPageExport` exports variables from the database.
- `tools/modkit`: `TestWorldMethods` plans a restart and checks its errors. `TestLantern` checks the shield's view and the wisp's art in the client data.
- `wactl mod test`: the Town Crier announces a planned restart. The lantern mod wears the shield and gets the wisp.
- `e2e`: `TestCommandsNeedPermission` hears the hidden game master, reads the saved bit from PostgreSQL, and sees a login without a second factor come back visible.
- `client`: `lookOf` draws a weapon and a shield of art, and the art of companion and mount records. `Character` draws a mount behind every part.
- `pnpm check:browsers`: the exit check's `mod worn sprite` and `mod companion` steps.

## Other choices

- A field for each record's art in the look. The client data names the art by record ID already, and an ID is a few bytes.
- Sending the IDs only for records with art. Every record's ID is simpler to explain, and a client may want to name a record.
- Sending a weapon's view of art in `Look.weapon`. That field is the weapon type, so its meaning would change, which the protocol's rules forbid within `v1`.
- Granting the game role the rights to end sessions. The account service already owns the sessions, and the role has fewer rights this way.

## Left

- Signature checks with trusted keys (phase 9), Ogg and MP3 sounds, pack map folders and zones that run mods' maps, and the inventory expansion voucher window, as review decision 41 says.
- The converter review of review decision 32.
- A left hand weapon of art shows no art, since the look has one weapon view.
- A network form of `EndAccountSessions` for the distributed build (phase 10).
