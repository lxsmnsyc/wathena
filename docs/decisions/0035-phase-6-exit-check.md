# 0035: Phase 6 Exit Check

- Status: proposed.
- Date: 2026-10-15.
- Code: [client/scripts](../../client/scripts) (`exit-check.ts`, `game-check.ts`, `browser-check.ts`), [client/src](../../client/src) (`assets/art.ts`, `data/clientData.ts`, `viewer/itemText.ts`, `viewer/skills.ts`, `viewer/hotkeys.ts`, `viewer/hud.ts`, `viewer/online.ts`, `viewer/main.ts`, `viewer/windows.ts`, `admin/sanctions.ts`), [client/vite.config.ts](../../client/vite.config.ts), [server/modassets](../../server/modassets), [server/mod](../../server/mod) (`items.go`), [server/gamedata](../../server/gamedata) (`mods.go`), [server/zone](../../server/zone) (`gmcommands.go`, `social.go`, `jobchange.go`, `scripts.go`), [server/admin](../../server/admin) (`moderation.go`), [tools/testdb](../../tools/testdb), [scripts/check.sh](../../scripts/check.sh), [mods/examples/lantern](../../mods/examples/lantern), [e2e/testdata/mods/fixture](../../e2e/testdata/mods/fixture).
- Builds proposal 0002, review decision 40, and phase 6 step 8 of strategy section 15. No Lua API changes. The protocol's messages do not change. `/client-data.json` moves to format version 2.

## Decision

### Mod text in the client

`/client-data.json` has format version 2:

| Field | Version 2 |
|---|---|
| `items` | Each item's art as before, and for an item a mod adds or renames: `name`, `slots`, the card slots, and `description`, its lines. An item with a `view_sprite` also has `view`, the view ID the server gave it. |
| `monsters` | Art as before, and `name` for a monster a mod adds or renames. |
| `skills` | Art as before, and `name` for a skill a mod adds or renames. |
| `jobs` | `name`, `body` with the body sprites by sex, and `look`, the job a mod's job looks like. The sprites moved from the entry into `body`, which is why the version changed. |

- An item record of a mod may set `description`: text of up to 4,096 bytes, one line per line break, with RO's color codes. It is kept apart from the item data, as the art fields are, and only the client reads it.
- A name comes with a record a mod adds, and with a core record whose `display_name` a mod set. Core's records keep the names of the asset build's tables and the client's tables.
- The client checks every field and refuses version 1. `art.ts` reads the names: `ItemTexts` puts mod items' names, slots and descriptions over the tables, and item search finds them; `jobName`, the skill window, the hotkey bar and the unit names use the names of mods' jobs, skills and monsters.
- The client shows the name of a unit under the mouse, as it already did for items on the ground, so monsters' names show in the page.

### Mod art in the client

- **Skill icons.** `SkillIcons` loads a skill's icon of art once, and the skill window and the hotkey bar show it in place of the text badge. Skills without art keep the badge.
- **Skill effects.** A skill's `effects` replace the client table's effects of the skill at their moment, `caster`, `effect` or `hit`. The effects are STR effects by asset name, as `EffectCache` already loads.
- **Monster sounds.** A monster's `attack`, `damage`, `die` and `move` sounds play at those moments, through the sound path of sprite events, while sound is on.
- **Worn item sprites.** Looks carry view IDs, not items. So the server gives each item with a `view_sprite` a view of its own from 1,000,000 at each start, in item ID order, as NPC sprites of art get sprite IDs. The client data names the view, and the client draws the view sprite for headgear and garments. Weapons and shields show their type's sprites, as before.
- **Job bodies.** Step 6's client drew a job's body by the look's job ID, but looks sent the job a mod's job looks like, so a mod's body never showed. Now a look sends a mod's job, and any job whose record names body sprites, by its own ID (`zone.Mods.OwnLookJobs`). The client data's `look` names the job it looks like, and the client takes the head, weapon, shield and garment sprites from that job. Speeds, ASPD and the other rules still read the look job.
- **Companions and mounts.** Not drawn. A look carries only the option bit of the core record a companion or mount acts as, so the client cannot tell which record a player has. Drawing them needs a field in the protocol, left for review.

### Moderation

- A line that starts with `@` is a command only for a player whose staff role, for this login, has the permission of at least one command. For every other player it is normal chat, logged and hooked as any line, as in rAthena. Staff with a command permission still get "Unknown command." for a command they lack or that does not exist.
- Lifting a permanent ban needs `moderation.ban_permanent`, as giving one does. The admin screens hide the Lift button of a permanent ban without it.

### The exit check

`exit-check.ts` runs in `pnpm check:browsers` as the check `exit`, Chromium only, and `scripts/check.sh browsers` gives it a PostgreSQL database.

- `tools/testdb` makes a database for the run on the server of `WATHENA_TEST_DATABASE_URL`, with the service roles, and drops it at the end. `wactl world new` applies the migrations and makes the world and the staff account Admin with a random password.
- Phase 1 runs `waserver --dev` with the game role, the end-to-end fixture mod and the lantern mod. Players start as Merchants at base level 50 and job level 40, with Red Potions, Jellopy, an Emperium, a Knife, a Guard and Apples. Ann plays on a 1,280 by 800 desktop page with the mouse, Bob on a Pixel 7 profile with touch, and Cid on a 1,024 by 700 desktop page.
- Phase 2 runs `waserver` with accounts on the same database. Dora signs up on a Pixel 7 profile, and Admin logs in on a desktop page. The Vite dev server forwards the account pages and the admin API, so the login cookie and the client share one site.
- Each step resets its players first: it closes the dialog, shops, storages and windows. A step reports its time, and the run its total.
- Clicks and taps find units through the development hook. A walk avoids windows and units, and a click on a unit retries after a step, since the lantern mod's ghosts walk across the map.
- The phone renders at one device pixel per CSS pixel. SwiftShader draws on the CPU, and three players at the Pixel 7's ratio of 2.625 slowed every step. The touch check plays phones at their full ratio.
- The fixture gains what the check needs: the Training Dummy, worth a million exp, placed by `!trainer`, and "Guild storage" at the Kafra. The lantern mod gains a hat, a job, a skill and monster sounds, which are the mod run. Its Lantern Ghost no longer loots, as a Poring does, so drops stay on the ground for players.

## Exit criteria and their proof

Strategy section 15, "Phase 6", has three exit criteria.

### A player can do every tier 1 activity in the web client

The basic game:

| Activity | Step | What it proves |
|---|---|---|
| Walk | `desktop walk`, `phone walk, fight and loot` | A click and a tap on the ground walk the player. |
| Fight and loot | `desktop fight and loot`, `phone walk, fight and loot` | A click and a tap on a Dummy attack it until it dies, and on its drops pick them up. |
| Level up | `desktop level up`, `mod skill` | A Training Dummy takes Ann from base level 50 to 51, and the Lantern Keeper from job level 1 to 2. |
| Job change | `mod job` | The Lantern Master changes Ann from a Merchant into a Lantern Keeper. |
| Equip | `desktop equip`, `phone NPC, equip and skill`, `mod worn sprite` | A double click and a double tap put on a Knife and the Lantern Hat. |
| Skills | `desktop skill`, `phone NPC, equip and skill`, `mod skill` | Mammonite from the skill window and from F1, and Lantern Flare, hit monsters. |
| NPCs | `desktop NPC talk and zeny`, `phone NPC, equip and skill`, `guild storage`, `storage`, `mod job` | The Quartermaster's menu, the Kafra's and the Lantern Master's dialog. |

Every tier 1 system of strategy section 11.3:

| System | Step | What it proves |
|---|---|---|
| Chat: public and whisper | `chat public and whisper` | Bob and Cid hear Ann. Ann and Bob whisper both ways, Bob on the phone. |
| Chat: party and guild | `party and party chat`, `guild, guild chat and guild skills` | Bob reads Ann's party line, Cid Ann's guild line. |
| Chat channels | `chat channel` | Bob and Cid join `#trade`, and Bob reads Cid's line. |
| Friends | `friends` | Ann adds Bob, Bob accepts on the phone, and Ann's list shows him. |
| Party | `party and party chat` | Bob makes a party on the phone and invites Ann, who accepts. |
| Party booking | `party booking` | Cid places an ad, Bob finds it on the phone and invites Cid from it. |
| Guild and guild skills | `guild, guild chat and guild skills` | Ann makes a guild with an Emperium, Cid joins, and Ann raises 20 guild skills up to Guild Storage Expansion. |
| Guild storage | `guild storage` | The Kafra opens the guild storage, and Ann stores Jellopy. |
| Guild emblems | `phone guild and emblem upload` | Dora makes a guild on the phone, uploads a PNG on the account page, and the guild window shows the 24 by 24 emblem. |
| Trade | `trade` | Ann trades a Guard and 100 zeny to Bob, who accepts and confirms on the phone. |
| Vending | `cart and vending setup`, `store search and vending` | Cid opens a shop from the cart, and Bob buys from it. |
| Store search | `store search and vending` | Bob finds Cid's Paper Lantern by name on the phone and opens the shop from the result. |
| Mail | `mail` | Ann sends Bob zeny and Jellopy, and Bob takes all on the phone. |
| NPC shops, barter | `NPC shops of every cost kind`, `mod item names and icons` | Zeny at the Tool Dealer and the Lamplighter; zeny, items and a currency together, a currency alone, and items alone at the Exchange. |
| Storage | `storage` | Ann stores Jellopy at the Kafra and takes some back. |
| Cart | `cart and vending setup` | Cid gets a cart, moves lanterns in and one back. |

### Staff can find a player, view item history, and apply a sanction from the admin tools

| Step | What it proves |
|---|---|
| `phone sign up and play` | Dora signs up on the account pages, makes a character and plays. |
| `admin TOTP login` | Admin logs in, turns TOTP on with a code from its key, logs out, and logs in with a password and the next code. `/admin` shows the staff screens. |
| `admin item history and mute` | Admin finds Dora, follows her Jellopy's serial to its history ("Now: Dora (inventory)"), and mutes her for 30 minutes. Dora's chat shows the mute with its reason, and refuses her next line. |

### Items, monsters, skills, and jobs added by a mod display correctly

| Record | Step | Names in the page | Art |
|---|---|---|---|
| Items | `mod item names and icons`, `mod worn sprite`, `mod monster and drop`, `cart and vending setup` | The shop lists Paper Lantern, Lantern Oil, Unlit Lantern and Lantern Hat. The inventory names them and shows the lantern's description. An item on the ground shows its name on hover. | Icons and the dropped sprite load from `/assets/mods/example.lantern/`. The hat's worn sprite loads from there, and its red shows on Ann. |
| Monster | `mod monster and drop` | Hovering a Lantern Ghost shows "Lantern Ghost". | Its sprite shows on the map since phase 6 step 6. |
| Skill | `mod skill` | The skill window lists "Lantern Flare". | Its icon loads with a width, and its hit effect loads from the mod's build when Ann casts it. |
| Job | `mod job` | The level line names the Lantern Keeper. | Its body sprite loads from the mod's build, and its green robe shows on Ann. |

The server's ERROR lines of both phases, and the page errors of every player, fail the check.

## Time

The final run of `scripts/check.sh browsers` on 2026-10-15, on the development Mac with 8 GB of memory, took 912 seconds, with 71 checks passed. The exit check took 504 of them:

- Phase 1 took about 430 seconds, phase 2 about 10, and the database, the world and the two server starts the rest.
- The slowest steps: the phone's NPC, equip and skill step (58 s), the mod skill with the Merchant's 38 points spent first (49 s), the cart and vending setup (43 s), the mail taken on the phone (38 s), and the mod monster and drop (33 s).
- Taps through the Chrome DevTools Protocol take about 300 ms each on this machine, and SwiftShader draws three pages at 15 to 25 frames a second, so the phone's steps are the slow ones.

The steps retry what a moving unit can spoil: a click that a walking ghost takes, a cast on a monster that walked off, and a long press on a player who moves. A step that fails reports its line in `exit-check.ts`, and `SCREENSHOTS` saves each player's page.

## Tests

- `server/zone`: `TestGMCommandsNeedRoles` with players whose `@` lines are chat, and `TestChangeIntoModJob` with a look by the mod job's own ID.
- `server/admin`: `TestSanctionCalls` lifts a permanent ban with and without `moderation.ban_permanent`.
- `server/mod`: `TestItemOverlay` and `TestItemOverlayErrors` with descriptions.
- `server/modassets`: version 2. `tools/modkit`: `TestLantern` checks the names, the description, the hat's view, the ghost's sounds, the job's body and look, and the skill's icon and effects of version 2.
- `e2e`: `TestModArt` reads the charm's name and description and the Dummy's name. `TestCommandsNeedPermission` says a player's `@` lines as chat.
- `client`: `clientData.test.ts` with version 2 and the refused shapes, `itemText.test.ts`, `hud.test.ts`, `skills.test.ts` and `online.test.ts` with mod names, icons, sounds, effects and view sprites, and `sanctions.test.ts` with the lift of a permanent ban.
- The lantern mod's own tests: the job change at the Lantern Master, Lantern Flare, and the hat.

## Other choices

- One server for both phases. `--dev` has no account pages, and accounts have no `--start-job`, so a run with accounts only would level Novices to Merchants first.
- A protocol field for a worn item's sprite. A view ID per item needs no protocol change, as NPC sprites of art did.
- Sending the client every core record's name. The asset build's tables have them, so only mods' records need it.

## Left

- Companions and mounts of art are not drawn, since looks do not say which record a player has. Closed: [decision 0036](0036-phase-6-review-changes.md) sends the record IDs in looks, and the client draws their art.
- Weapons and shields with a `view_sprite` keep their type's sprites. Closed: [decision 0036](0036-phase-6-review-changes.md) draws a shield's art by its view and a weapon's by `weapon_view`.
- The check plays one map. Warps between maps are in the end-to-end tests.
