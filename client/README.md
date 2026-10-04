# client

TypeScript web client. See docs/strategy.md, section 10.

It is at an early stage: a map viewer for development, built on the parts the game client will use.

| Folder | Content |
|---|---|
| `src/assets/` | Loads assets by name from a build. Checks every file against its SHA-256 from the manifest. |
| `src/formats/` | Decoders for the binary asset formats in docs/specs/assets-v1.md. |
| `src/render/` | three.js scenes: the ground, water, and models of a map, lit as RO lights them, sprites that face the camera, and characters built from body, head, and equipment sprites, baked effects, and cylinder effects such as warp portals. |
| `src/sw/` | The Service Worker, which caches assets by hash and checks them before caching. |
| `src/world/` | Game rules the client shares with the server, such as pathfinding, from `shared/` compiled to WebAssembly. |
| `src/viewer/` | The map viewer page. |
| `data/` | Client data tables, such as `jobs.yml`. |

## Map viewer

Build assets with `wactl assets build`, then serve the build and start the dev server:

```text
wactl assets serve --build /path/to/build
pnpm install
pnpm dev
```

Open `http://localhost:5173/?map=prontera`. Monsters, an NPC, and player characters with random jobs, hair, dyes, and equipment wander near the map's center. `sprites=mob/1002,npc/117` picks the sprites, as names without `sprite/`, `characters=30` sets how many characters appear, `effect=effect/firewall` plays effects in a row at the center, and `distance=20` moves the camera closer. Click the ground to walk: the player follows the shared pathfinding module, which gives rAthena's paths. A Service Worker caches assets across visits, and `nosw` turns it off, for measuring first visits. The Sound button plays the map's music, its placed sounds, and the sounds sprites make. The status line shows how many files the map needed, the bytes sent over the network, and the load time. `WATHENA_ASSETS` changes where the dev server forwards `/assets` (default `http://127.0.0.1:8200`).

`server=ws://localhost:8300/ws` plays online against `waserver` (`server/cmd/waserver`). The client gets a one-time game ticket from the account service and sends it in `Hello` (docs/decisions/0004-account-service-and-game-tickets.md):

- `account=` names the account service. By default it is the game server's `/account/`, such as `http://localhost:8300/account/`.
- A player who is not logged in, or has not chosen a character, goes to the account pages, and comes back after Play.
- The Characters button leaves the game for the character list. When the game ends, the page goes back there after 3 seconds, or at once when the ticket was not valid.
- The client and the account service must be on the same site, such as both on `localhost`. The browser only sends the login cookie with the ticket request then.
- `name=Tester` instead plays the character of that name on a `waserver --dev` server.

A production build (`pnpm build`) that `waserver all` serves plays on `/ws` of its own origin without `server=`, so players open the server's URL alone. `viewer` opens the map viewer instead. `waserver` embeds `dist/` when built with the `webclient` tag (server/README.md, "All-in-one").

The server places every character and item:

- Click a unit to attack it, and click the ground to walk. From 90% weight, the client says the player is carrying too much instead of attacking.
- Items on the ground show their sprite, `sprite/item/<id>`, and jump and bounce when they drop. Hover an item to see its name and amount under it. Click an item to pick it up. The chat log says what the player got and used.
- The Items button or Alt+E opens the inventory window: each slot's icon, `icon/item/<id>`, the item's name, and the amount, with the slots used and the weight as "current / max" and a percent. Click a row to show the item's description beside the window, with RO's color codes drawn as colors. Double-click a row, or press Enter on it, to use the item. Only healing items work for now, and a failed use says why. The Drop button, the Delete key, or dragging a row onto the game view drops the item. A stack of more than one asks how many first. Heals show as green numbers, and potions and herbs play their effect on the user.
- Worn items show in the equipment window, not in the inventory window. Double-click equipment in the inventory window, or press Enter on it, to put it on. Each item shows its refine level, such as "+7 Knife [3]". Its cards show in its tooltip and description.
- Double-click a card, or press Enter on it, to put it into an item. A list shows the items that can take it: not worn, not a card, worn where the card fits, and with a free card slot. Choose one to insert the card. When no item can take it, the client says so.
- The Equip button or Alt+Q opens the equipment window: head top, head mid, head low, armor, right and left hand, garment, shoes, right and left accessory, and ammo. Each shows the worn item with its refine level and its cards. Costumes and shadow gear share one row at the end. Click a worn item to take it off. Failed equips and card inserts say why in the chat log.
- Players show their weapon, shield, headgear and garment. The weapon sprite comes from the weapon type's view in `data/weapons.yml`. When a player's equipment changes, the client loads the new look and keeps the player's place, direction and action.
- Players show their falcon or warg beside them, `sprite/companion/<look>`, and their cart one cell behind, `sprite/cart/<n>`, from their option bits. A mounted player shows its mounted body: a Peco Peco rider, a third class mount such as a Rune Knight's dragon, or the job's costume mount look from `data/jobs.yml`. They change when the options or look change.
- The top bar shows HP, base and job levels, and exp as a percent of the next level.
- The Status button or Alt+A opens the status window: STR to LUK with the cost of the next point and a "+" button that raises the stat by 1, and ATK, MATK, DEF, MDEF, HIT, FLEE, CRIT, ASPD, status points and weight. "+" is enabled when the status points pay for the raise.
- The Skills button or Alt+S opens the skill window: each skill with its level, maximum level and SP cost, the skill points left, and a "+" button that learns a level while points remain. Skills not learned yet show the skills they need. Passive skills are listed but cannot be cast. Double-click a skill or press Use to cast it, and pick a lower level in its level list. Skills show as text badges, since the asset build has no skill icons yet.
- The hotkey bar holds 9 skills on F1 to F9. Drag a skill from the skill window onto a slot, or right-click it in the window to put it in the first free slot. Right-click a slot to clear it. Slots are saved in the browser for each character. A slot or a skill on cooldown is dimmed and shows the seconds left, including the delay after a cast.
- Self skills cast at once. Enemy and support skills change the cursor and wait for a click on a unit. Support skills can also target the player. Ground skills wait for a click on a cell, and the cell under the pointer is marked. Escape or a right click cancels. Failed skills say why in the chat log.
- A cast shows a bar over the caster and the aura of the skill's element under it, until the cast ends or stops. Skill effects, ground skills such as Fire Wall and Safety Wall, and damage numbers show as the server sends them. A multi-hit skill shows each hit, then the total. Heal and First Aid show green numbers.
- Enter opens the chat input, Enter sends the line and clears it, and Escape leaves the input. Enter on an empty line also leaves it. While the input has focus, game shortcuts such as Alt+A do nothing. The log keeps the last 100 lines as "Name : text", and each line shows above the speaker for 5 seconds. Lines are cleaned as the server cleans them and cut to 255 code points. The client keeps the server's rate (5 lines, then 1 per second) and says so in the log instead of sending more. Chat is only ever shown as text.
- Tabs above the chat log pick where the input sends a line. "All" shows every line and sends local chat. Each chat channel the player is in, such as #global or #trade, has a tab that shows its lines as "[Alias] Name : text" and sends to the channel. Each whisper partner has a tab, opened when the player whispers someone or gets a whisper. Whispers show as "(From Name) : text" and "(To Name) : text". A tab with new lines turns yellow. The × after a tab closes a whisper tab or leaves a channel. Notices show on every tab.
- Commands in the chat input:
  - `/w Name text` whispers, and `/w "Name with spaces" text` takes a name with spaces. `/w Name` alone opens the whisper tab.
  - `#trade text` says a line in #trade, and `#trade` alone shows its tab.
  - `/join #name`, `/leave #name`, `/channels` and `/members #name` join, leave, list the channels, and list a channel's members.
  - `/block Name` and `/unblock Name` change the block list.
  - `/report Name [reason]` reports a player to the staff.
  - `/p text` or `%text` says a line to the party, and `/p` alone shows the Party tab.
  - `/g text`, `/guild text` or `$text` says a line to the guild, and `/g` alone shows the Guild tab.
- The Blocked button opens the block list: each blocked name with an Unblock button, a field to block a name, and the count out of the server's limit (`Welcome.limits.block_list`, 20 by default). The panel says when the list is full.
- The Party button or Alt+Z opens the party window. Without a party it has a form to make one: a name and the two item options. In a party it lists the members in join order, with a star on the leader, a green dot for those online, each member's level, job and map, and an HP bar. The count shows as "n / max", from `Welcome.limits.party_members`. The leader can invite a player by name, expel a member, make another member the leader, and turn even exp share, even item share and shared pick-up on or off. The Leave party button leaves it. While the player has a party, a Party tab shows after All. Party lines show in it and on All, in green.
- A party invite or a friend request shows a prompt with Accept and Decline. It declines by itself after 30 seconds. The log says how each party and friend action went, such as "Bob joined the party." or "Only the party leader can do that.", and who got an item that even item share gave.
- The Friends button or Alt+H opens the friend list beside the block list: each friend with a dot that is green while the friend is online, a Whisper button and a Remove button, a field to add a friend by name, and the count out of `Welcome.limits.friends`. The log says when a friend logs in or out.
- The Booking button or Alt+B opens party booking. The player can place one ad with a level, a map, up to 6 jobs and a note, and delete it. A search by level and by map or job lists the ads found with their level, map, jobs, note and time. Whisper opens a whisper tab to the ad's owner, and Invite, shown to a party leader, invites the owner to the party.
- The Guild button or Alt+G opens the guild window. Without a guild it has a form to make one by name, which may take an Emperium. In a guild it has six tabs:
  - Info: the name and emblem, the level and an exp bar toward the next level, the members as "n / max", the average level, the master, and the player's position. The master can disband the guild after typing its name. Others can leave it with a reason.
  - Members: each member with a green dot while online, a star on the master, the position, level, job, map and the exp it gave the guild. A position that allows it can invite a player by name and expel a member with a reason. The master can give each member a position. Position 0 hands the guild over, after a confirm.
  - Positions: the `Welcome.limits.guild_positions` positions (20 by default) with their names, invite, expel and storage rights, and exp tax. Only the master can change them. The server limits the exp tax by its own rule.
  - Notice: the title and the text. Only the master can change them.
  - Skills: each guild skill with its level and maximum, and the skill points. The master raises a skill with "+" when the server says it can be raised.
  - Relations: the allied and enemy guilds, out of `guild_allies` and `guild_enemies`. The master can ask an online player's guild to be allies, make it an enemy, and end a relation. The "!" after a guild reports its name.
- While the player has a guild, a Guild tab shows after All and Party. Guild lines show in it and on All, in blue. A guild invite or an alliance request shows a prompt that declines by itself after 30 seconds, as a party invite does. The log says how each guild action went.
- Players in a guild show their emblem, their name, and "Guild [Position]" under them. The emblem is `/emblems/<guild_id>/<version>.png` on the game server, loaded once per version. An emblem that does not load shows nothing.
- A right click on another player opens a menu: Whisper, Invite to party for a party leader, Invite to guild for a player without a guild, and Report guild name and Report emblem, which ask for a second click.
- A guild storage NPC opens the storage window with the stored items and the slots used. Drag an inventory row onto it, or right-click the row, to store the item. Take, a double click, or dragging a row onto the inventory window takes it out. A stack of more than one asks how many first. The Log button lists the last moves with their time. Close tells the server, and the log says why a storage action failed.
- A line another player said has a "!" button. Click it, then click "Report?" within 3 seconds, to report the line to the staff. The log says whether the report was sent.
- The log says why a line was not sent: a mute, with its end in local time and its reason, a word the server does not allow, a channel the player is not in, or "You're talking too fast!" for a channel's delay. It also says when a mute or a jail starts and ends, when a whisper's target is not online or blocks the player, and why a channel command failed.
- Item names come from the build's `table/items`, loaded the first time an item shows, with the slot count as "Knife [3]". Descriptions come from `table/itemdesc`, loaded the first time one shows (docs/specs/assets-v1.md, "Table (JSON)"). An item the build does not name shows as "Item 501". Names from a Korean client are Hangul, so the page lists Korean fonts after `system-ui`. Items show their identified name, since the server does not send whether an item is identified yet.
- Effects the server sends, such as level ups, play from `data/effects.yml`. An effect part can also be a sound, or a cylinder that the client draws, as roBrowser's `CYLINDER` effects (`src/render/cylinder.ts`).
- Warp portals are NPCs with sprite 45. They show effect 321, `EF_WARPZONE2`: two blue rings of `texture/effect/ring_blue` that rise and shrink, four copies each, one second apart, as roBrowser draws them. roBrowser's sparks are sprite particles and do not show yet.
- Walking into a warp changes the map. On `MapChange`, the client clears every entity, shows a loading screen, and loads the new map from the asset store, which the Service Worker caches. Events after `MapChange` wait until the map has loaded. Clicks do nothing while it loads. In development builds, `viewer.mapLoads()` lists each map change with its time, from `MapChange` to the map being ready, and the files and bytes it read.
- Click an NPC to talk to it. The dialog window opens beside the player, with the NPC's name and its lines. RO's color codes, such as `^FF0000`, are drawn as colors, and the text is only ever shown as text. Next, Close, menus, and number and text inputs answer the server, and Escape cancels. Clicks on the game view do nothing while the window is open, except during a progress bar.
- NPC images (`cutin`) show at the left, center or right of the screen from `image/illust/<name>`, and hide when the dialog window closes.
- Emotions play over a unit from `sprite/effect/emotion`, one action per emotion. NPC speech shows over the NPC and in the chat log. Announcements show in the chat log in their color.
- The progress bar shows over the player for the time the server sends.
- Clicking a shop NPC, or a script's `open_shop`, opens the shop window: a Buy list with prices, a Sell list with the items the shop buys and their prices, an amount for each row, and the zeny, the totals and the weight. The window checks zeny and weight first, as the server does, and shows the server's answer.
- The client has no minimap yet, so it ignores minimap marks.

Map ready after a warp, measured in Chrome on an Apple M2 with `waserver` and the Vite dev server on the same machine (budget: 1.5 s when cached, strategy section 3.8):

| Load | prt_in | prt_fild08 | prontera |
|---|---:|---:|---:|
| First visit, from the network | 387 to 445 ms | 288 to 352 ms | |
| Map already in the page's memory | | | 167 to 183 ms |
| After a reload, from the Service Worker cache | | 102 to 153 ms | 246 to 283 ms |

## Checks

```text
pnpm check    # type check
pnpm lint
pnpm format:check
pnpm test
pnpm build
pnpm check:browsers   # render, audio, and cache checks in real browsers
```

`check:browsers` builds and serves the placeholder pack, then checks in Chromium that the viewer renders a map without errors, that Opus test sounds decode to their exact length, and that a second visit loads assets from the Service Worker. `BROWSERS=chromium,firefox,webkit` adds the other engines, after `pnpm exec playwright-core install`. CI runs it in Chromium.

The WMAP decoder test reads `src/formats/testdata/sample.wmap`, which the Go encoder's tests write. After changing the format, run `go test ./internal/assets/wmap -update` from the repository root.
