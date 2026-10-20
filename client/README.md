# client

TypeScript web client. See docs/strategy.md, section 10.

It is at an early stage: a map viewer for development, built on the parts the game client will use.

| Folder | Content |
|---|---|
| `src/assets/` | Loads assets by name from a build. Checks every file against its SHA-256 from the manifest. |
| `src/formats/` | Decoders for the binary asset formats in docs/specs/assets-v1.md. |
| `src/render/` | three.js scenes: the ground, water, and models of a map, lit as RO lights them, sprites that face the camera, and characters built from body, head, and equipment sprites, baked effects, and cylinder effects such as warp portals. |
| `src/sw/` | The Service Worker, which caches assets by hash and checks them before caching. |
| `src/world/` | Game rules the client shares with the server, from `shared/` compiled to WebAssembly: pathfinding (`path.wasm`) and the map generator of procedural worlds (`gen.wasm`). |
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
- The Items button or Alt+E opens the inventory window: each slot's icon, `icon/item/<id>`, the item's name, and the amount, with the slots used and the weight as "current / max" and a percent. Under the weight it shows the zeny and a "Currencies" line with each balance from the last `CurrencyUpdate`, such as "Currencies: Cash 1,000". Click a row to show the item's description beside the window, with RO's color codes drawn as colors. Double-click a row, or press Enter on it, to use the item. Only healing items work for now, and a failed use says why. The Drop button, the Delete key, or dragging a row onto the game view drops the item. A stack of more than one asks how many first. Heals show as green numbers, and potions and herbs play their effect on the user.
- Worn items show in the equipment window, not in the inventory window. Double-click equipment in the inventory window, or press Enter on it, to put it on. Each item shows its refine level, such as "+7 Knife [3]". Its cards show in its tooltip and description.
- Double-click a card, or press Enter on it, to put it into an item. A list shows the items that can take it: not worn, not a card, worn where the card fits, and with a free card slot. Choose one to insert the card. When no item can take it, the client says so.
- The Equip button or Alt+Q opens the equipment window: head top, head mid, head low, armor, right and left hand, garment, shoes, right and left accessory, and ammo. Each shows the worn item with its refine level and its cards. Costumes and shadow gear share one row at the end. Click a worn item to take it off. Failed equips and card inserts say why in the chat log.
- Players show their weapon, shield, headgear and garment. The weapon sprite comes from the weapon type's view in `data/weapons.yml`. When a player's equipment changes, the client loads the new look and keeps the player's place, direction and action.
- Players show their falcon or warg beside them, `sprite/companion/<look>`, and their cart one cell behind, `sprite/cart/<n>`, from their option bits. A companion or mount record with art shows its art instead ("Mod names and art"). A mounted player shows its mounted body: a Peco Peco rider, a third class mount such as a Rune Knight's dragon, or the job's costume mount look from `data/jobs.yml`. They change when the options or look change.
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
  - `/memo` saves the player's cell as a Warp Portal memo point, as in RO. The server answers with a system message.
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
- A right click on another player opens a menu: Whisper, Trade, Browse shop or Browse store for a player with an open shop, Invite to party for a party leader, Invite to guild for a player without a guild, and Report guild name and Report emblem, which ask for a second click.
- A storage NPC opens the storage window with the stored items and the slots used. Its title is "Storage" for the account's storage, "Guild storage", or the display name of an extra storage. Drag an inventory row onto it, or right-click the row, to store the item. Take, a double click, or dragging a row onto the inventory window takes it out. A stack of more than one asks how many first. The window shows 100 rows per page, with Previous and Next, so a storage of 10,000 slots stays fast. It says "Loading items..." until the server has sent every page. The Log button of the guild storage lists the last moves with their time. Close tells the server, and the log says why a storage action failed, such as "This item cannot be stored." or "This bounded item cannot be stored there."
- While the player has a cart, the Cart button or Alt+W opens the cart window: the cart's items, the slots used out of its size, and its weight as a bar and as "current / max". Items move between the inventory and the cart as they do for storage. While a storage is open, a cart row's To storage button, or dragging the row onto the storage window, moves it into the storage, and a storage row's To cart button, or dragging it onto the cart window, moves it into the cart. Closing the window only hides it. It closes by itself when the player loses the cart.
- Item tooltips and the inventory's item description say who a copy is bound to, such as "Bound to account", and what the item cannot do, such as "Cannot be traded" or "Cannot be stored".
- Trade from the player menu sends a trade request. A request from another player shows a prompt that declines by itself after 30 seconds. When both agree, the trade window opens with two panes, the player's offer and the partner's, of up to 10 items and an amount of zeny each. Drag an inventory row onto the window, or right-click the row, to offer it. A stack of more than one asks how many first. Zeny is offered once. OK locks the player's offer, and a locked pane has a yellow border. When both offers are locked, Trade completes the trade once both players pressed it. Cancel ends the trade. The log says how each step went, such as "This item cannot be traded." or "This bounded item cannot be traded to that character.", and shows a mod's reason when a mod refused the trade.
- `Welcome.limits` gives each limit's base and max (`ranges`). The client keeps both, and uses the base when a window has no size from the server.
- A line another player said has a "!" button. Click it, then click "Report?" within 3 seconds, to report the line to the staff. The log says whether the report was sent.
- The log says why a line was not sent: a mute, with its end in local time and its reason, a word the server does not allow, a channel the player is not in, or "You're talking too fast!" for a channel's delay. It also says when a mute or a jail starts and ends, when a whisper's target is not online or blocks the player, and why a channel command failed.
- Item names come from the build's `table/items`, loaded the first time an item shows, with the slot count as "Knife [3]". Descriptions come from `table/itemdesc`, loaded the first time one shows (docs/specs/assets-v1.md, "Table (JSON)"). An item the build does not name shows as "Item 501". Names from a Korean client are Hangul, so the page lists Korean fonts after `system-ui`. Items show their identified name, since the server does not send whether an item is identified yet.
- Effects the server sends, such as level ups, play from `data/effects.yml`. An effect part can also be a sound, or a cylinder that the client draws, as roBrowser's `CYLINDER` effects (`src/render/cylinder.ts`).
- Warp portals are NPCs with sprite 45. They show effect 321, `EF_WARPZONE2`: two blue rings of `texture/effect/ring_blue` that rise and shrink, four copies each, one second apart, as roBrowser draws them. roBrowser's sparks are sprite particles and do not show yet.
- Walking into a warp changes the map. On `MapChange`, the client clears every entity, shows a loading screen, and loads the new map from the asset store, which the Service Worker caches. Events after `MapChange` wait until the map has loaded. Clicks do nothing while it loads. In development builds, `viewer.mapLoads()` lists each map change with its time, from `MapChange` to the map being ready, and the files and bytes it read.
- Click an NPC to talk to it. The dialog window opens beside the player, with the NPC's name and its lines. RO's color codes, such as `^FF0000`, are drawn as colors, and the text is only ever shown as text. Next, Close, menus, and number and text inputs answer the server, and Escape cancels. Clicks on the game view do nothing while the window is open, except during a progress bar.
- NPC images (`cutin`) show at the left, center or right of the screen from `image/illust/<name>`, and hide when the dialog window closes.
- Emotions play over a unit from `sprite/effect/emotion`, one action per emotion. NPC speech shows over the NPC and in the chat log. Announcements show in the chat log in their color.
- The progress bar shows over the player for the time the server sends. An `NpcProgressBar` shows over its NPC, in its color, until it is full.
- An NPC's title (`setunittitle`) shows under its name when the pointer is over it. An NPC whose `Look.npc_view` is a monster shows the monster's sprite, and one whose view is a job shows a character with the look's hair, colors and headgear.
- `EffectRemove` stops an effect that plays on an entity. `SoundPlay` plays a sound from its entity, or from the player itself. `MusicPlay` plays background music until the next map change.
- `DigitsShow` shows a clock at the top of the screen (`src/viewer/digits.ts`): a value for 5 seconds, or a counter that moves once a second.
- A server line with a color, as `dispbottom`'s, shows in that color. `QuestLogOpen` opens the quest window.
- The client has no minimap, so minimap marks and the marks of quest icons are not drawn.
- A `SoundPlay` with `repeat` plays again and again until the map changes (decision 0064).
- A player whose `Look.npc_view` is a monster, after `player:transform`, shows the monster's sprite. `Look.cloth_color` above 0 draws the body with `palette/body/<look>/<sex>/<color>`.
- An item's tooltip says who it is named for, whether it is broken, and when a rental ends. A rental that ended leaves the inventory with a line in the chat log. `BookOpen` shows nothing yet: the asset build has no book texts.
- Clicking a shop NPC, or a script's `open_shop`, opens the shop window. It shows:
  - The shop's name.
  - A Buy list. Each row shows the item's full cost: zeny, items with how many the player has, and currencies with the player's balance. A shop that limits an item shows how many are left, such as "12 left".
  - A Sell list with the items the shop buys and their prices. It shows only for a shop that buys from players.
  - An amount for each row, and the zeny, the totals and the weight.
- The shop window checks stock, zeny, cost items, currencies and weight first, as the server does. Cost items count the inventory's items that are not worn. The log shows the server's answer, such as "The shop does not have that many Jellopy left."
- The Mail button or Alt+M opens the mailbox. It shows:
  - The mails, newest first, with the sender, title, date, a dot for an unread mail, the zeny and item count, a Returned mark, and the expiry date.
  - The mails used out of the mailbox size, and the mails sent today out of the daily limit.
- Click a mail to read it. The window shows its text, its zeny and its attachments, with their refine, cards and rolls in the tooltip. Take zeny, Take items and Take all move them into the inventory. Delete works once the mail is empty.
- The Write tab writes a mail:
  - The recipient's name, a title of 1 to 39 bytes, and a text of at most 499 bytes. Byte counters show under each.
  - Zeny, and up to `Welcome.limits.mail_items` items from the inventory. Pick an item and an amount, or right-click an inventory row. Worn, bound and no-mail items cannot be attached, and the list says why.
  - The fee as an estimate: 2% of the zeny and 2500 zeny per item, rAthena's defaults. The server's ruleset may differ.
- A new mail shows a line in the chat log and the unread count on the Mail button. The log says how each mail action went, and shows a mod's reason when a mod refused it.
- Casting Vending (skill 41) opens the shop setup window. Enter a title of 1 to 79 bytes, and an amount and a price (1 to 1,000,000,000 zeny) for up to the allowed number of cart items. Open opens the shop, and Cancel closes the window. While the shop is open, its window shows what is left, and Close shop closes it. The log says each sale, such as "Alice bought 3 x Jellopy for 1,500 zeny."
- A player with an open shop shows its title on a board over the player. A buying store's board is blue. Click the board or the player, or use the player menu, to browse it.
- The vending browse window lists the shop's items with their price, amount and an amount input, and the total. Buy buys them.
- A buying store opens from `player:open_buying_store`. Its setup window takes a title, a zeny limit, and up to the allowed number of lines of an item from the inventory, an amount (1 to 9,999) and a price (1 to 99,990,000 zeny). While the store is open, its window shows what it still buys and the zeny left, and Close store closes it. The log says each purchase.
- The buying store browse window lists what the store buys. Under each line, it lists the inventory slots that hold the item, with an amount input each. Sell sells them.
- The store search window opens from `player:search_stores`. Choose vending shops or buying stores, type item names or IDs, and optionally cards, a price range, and roll and random option filters with a name and a range. Search runs the query, and the window shows the searches left. Each result shows the store's title, owner, map and cell, the item with its rolls, the amount and the price. Next page shows more. Click a result to show where the store is, as "prontera (150, 180)", or to open it when the search allows that.
- When the player goes on autotrade, the server ends the session with "Your shop stays open while you are away." The client shows the server's message for every disconnect, and a line for the reason when the message is empty.
- The client has no minimap yet, so it ignores minimap marks.
- The World Map button or Ctrl+\` (the key left of 1) opens the world map window. See "Procedural worlds".
- The Quests button or Alt+U opens the quest window. See "Quests".

Map ready after a warp, measured in Chrome on an Apple M2 with `waserver` and the Vite dev server on the same machine (budget: 1.5 s when cached, strategy section 3.8):

| Load | prt_in | prt_fild08 | prontera |
|---|---:|---:|---:|
| First visit, from the network | 387 to 445 ms | 288 to 352 ms | |
| Map already in the page's memory | | | 167 to 183 ms |
| After a reload, from the Service Worker cache | | 102 to 153 ms | 246 to 283 ms |

## Procedural worlds

`Welcome.world` names the world's mode, and for a procedural world its map generator version and rng spec version (docs/decisions/0038-generator-core.md). For a procedural world only, the viewer imports `src/world/generator.ts` and loads `gen.wasm`, and logs an error when the module lacks the world's version. A classic world never fetches it. The module comes from `shared/gen/wasm`:

```text
tinygo build -o client/src/world/gen.wasm -target=wasm-unknown -opt=2 -no-debug -tags procedural ./shared/gen/wasm
```

`generator.test.ts` checks it against the golden checksums of `shared/gen/testdata` in Node, and `pnpm check:browsers` does the same in Chromium. Only Chromium is supported. `GEN_BROWSERS=chromium,firefox,webkit` adds the others for a one-off check.

A map whose name starts with `gen:`, such as `gen:field:0:0`, is a generated field (docs/decisions/0039-generated-fields.md):

1. The viewer fetches `/gen/maps/<id>.json` from the game server: the field's public node, size, biome and checksum.
2. `fieldGenerator.ts` generates the field with `gen.wasm` in a Web Worker (`fieldWorker.ts`), so the page keeps drawing. A worker whose module grew past 256 MiB stops after its field, and the next field starts a new one.
3. When the grid's checksum matches the server's, the viewer draws the field's own WMAP. Otherwise, or when the module fails or lacks the world's version, it downloads `/gen/maps/<id>.wmap` and checks its SHA-256. `?nogen` skips the generator, so every field downloads.
4. `/gen/tilesets.json` maps the field's `texture/gen/...` and `model/gen/...` names to assets. The asset store resolves them before the manifest (`AssetStore.setAliases`).

`viewer.fieldLoads()` lists each field load with its source and times. A 400 × 400 field takes 75 to 99 ms in Chromium and WebKit and 690 to 735 ms in Firefox. With `WATHENA_GAME`, the dev server also forwards `/gen` to the game.

Dungeon floors and towns, such as `gen:dungeon:crypt:2` and `gen:town:t1`, load the same way (docs/decisions/0040-dungeons-towns-places.md). Their JSON adds `kind` and `params`: the worker passes the params to `generate_map` (`Generator.generateMap`), and the loader compares the checksum and falls back to the download as for fields. `viewer.fieldLoads()` records each load's kind, and a floor's style or a town's template as its `biome`. A town of 192 cells takes about 40 ms in Chromium and WebKit and 262 ms in Firefox, and a dungeon floor under 11 ms and 86 ms. The client makes each map's named places too, but does not use them.

A world layout (proposal 0002, review decision 43) adds these:

- A mod's home map has the kind `home`. Its JSON has no `key` and no `params`, so the client never generates it. It always downloads the WMAP from the `wmap` path and checks its SHA-256. `viewer.fieldLoads()` records it with the kind `home`.
- The JSON of each map of the layout has `displayName`, such as "North Valheim Fields", and `levels`. Online, the map field of the top bar shows the display name, then the world map's name, then the map ID. Staff can rename towns while the server runs, so the JSON is fetched with `no-cache` on each load.
- The server sends `WorldMap` after login and each time the character enters a map for the first time. Each one replaces the one before.
- The World Map button or Ctrl+\` (the key left of 1) opens the world map window. It draws the maps found so far as an SVG that fits the window's width, also on a phone:
  - Each field sits at its grid cell, with north up. Lines are the warps between maps.
  - Towns and the home map sit at the top of their seat field's cell. A dungeon's floors stack at the bottom of its entrance field's cell.
  - A visited map shows its name and level band, such as "Valheim 1-10". A map not visited yet is a small "?" without a name.
  - The current map has a white ring.
- A classic world sends no `WorldMap`, and the window says the world has no world map.
- `viewer.worldMap()` returns the last world map as `{ maps, links }`, or undefined, and `viewer.displayName()` returns the current map's display name. The browser checks read them.

A world that `wactl world new` made has procedural content (docs/decisions/0044-procedural-content.md). The JSON of a map lists the places that content and generation layers added after the generated ones, such as `quest_board`, and `objects`: the models that the mods' generation layers put on the map, each `{ model, x, y, turn }` with an asset name such as `model/gen/forest/rock`. The generated WMAP does not have them, so the viewer adds them to the map's objects when it loads the map (`layerObjects` in `src/world/fields.ts`): each stands on the middle of its cell, at the ground's height, turned by its quarter turns, as the generator places its own props, and the tileset table maps it as it maps those.

## Quests

The server keeps each character's quest log (proposal 0002, review decision 45). The client shows it as rAthena's quest window does:

- The Quests button or Alt+U opens the quest window. It lists the active quests, then the inactive ones. Complete quests are never in the log.
- Each quest shows its title, its timer and its hunt objectives:
  - The timer counts down from the time left that the server sent, such as "Time left 0:59:12" or "Time left 2 days 3:04:05". It shows "Time is up" at 0, and "No time limit" for a quest without a limit.
  - Each objective shows its label, the kills so far out of the goal, and the level range when it is set, such as "Poring 2/3 Lv 10-20". A finished objective is green.
- Set inactive and Set active move a quest between the two groups. The server answers with a `QuestUpdate`.
- The window says "You have no quests." when the log is empty.
- `QuestList` replaces the log after login and after each map change. `QuestUpdate` adds or replaces one quest, and `QuestRemove` takes one out.
- A `QuestIcon` shows an icon over an NPC until the server removes it, the NPC goes out of sight, or the map changes. The asset pack has no quest icon art, so the icon is a round badge:
  - "!" or "?" in yellow for `quest` and `quest_question`, green for `job` and `job_question`, and orange for `event` and `event_question`.
  - "W" in gray for `warg` and `warg_face`.
  - "!" in blue for `daily_quest`, "!" in green for `job_quest`, "!" and "?" in orange for `click_me` and `event3`, and "P" in orange for `jumping_poring` (review decision 58).
- `viewer.quests()` returns the log as `[{ id, title, active, limitMs, timeLeftMs, objectives }]`, and `viewer.questIcons()` returns the icons as `[{ npc, name, icon }]`. The browser checks read them. The window is `#quest-window`, its button `#quest-toggle`, each quest an `li` with `data-quest-id`, and its button a `.quest-state`.

## Mod names and art

The server's `/client-data.json`, format version 2, names the records of mods and their art (docs/decisions/0035-phase-6-exit-check.md). The client shows:

- mod items' names, card slots and descriptions everywhere item names show: the inventory and its details, tooltips, shops, trade, mail, storage, the cart, vending, store search, chat lines and items on the ground;
- mod monsters' names on hover, mod skills' names in the skill window and on the hotkey bar, and mod jobs' names in the level line, the party, guild and booking windows;
- skill icons of art in the skill window and on the hotkey bar, in place of the text badge, and a skill's `effects` at their moments;
- monster sounds of art for attacks, hurts, deaths and walks, while sound is on;
- an item's `view_sprite` on the head, the back, the arm or in the hand, by the view the server gave it: headgear, garments and shields by their view, and a weapon by the look's `weapon_view` (docs/decisions/0036-phase-6-review-changes.md);
- companions and mounts of art, by the record IDs in the look: a companion beside the player, as the falcon, and a mount at the player's cell, under the body on foot;
- a mod job's body sprites, with the head, weapon and other parts of the job it looks like.

The name of a unit or an item on the ground shows under it while the mouse is over it.

## Touch

Touch input is frozen as built in decision 0034 (docs/decisions/0034-touch-input.md). The touch code stays, but it will not be extended. Its browser checks run only on demand (see Checks).

Phones and tablets play with touch (docs/decisions/0034-touch-input.md). The client follows the pointer last used, so a mouse and a finger work on one device, and the mouse and keyboard work as before. While touch is used:

- A tap does what a left click does. A finger held on the ground walks toward it. A long press of half a second on another player opens the player menu, and cancels a skill that waits for a target. Two fingers zoom by pinching, and turn the camera by a twist or a sideways drag.
- The joystick in the lower left corner walks the player in the pushed direction, by walks to a cell 4 cells ahead, at most 4 a second. Letting it go stops the player within two cells. Options hides it.
- The hotkey bar is a 3 by 3 grid of larger slots in the lower right corner. A Chat button opens the chat over the game view with a Send button, above the on-screen keyboard. A Cancel skill button shows while a skill waits for its target.
- A long press on an item or skill row starts a drag. Lift it over a window that takes it, or the game view to drop an item. A long press lifted in place does the row's right click. Two taps do a double click.
- Below 600 pixels wide, windows are full-width panels under the top bar, the window buttons fold into a Menu button, and buttons and slots are at least 44 pixels. Every window moves by its title bar, with any pointer.
- The page does not zoom on a double tap, reload on a pull or select text on a long press.

For a test on a phone, the dev server can forward the game and serve HTTPS: `WATHENA_GAME=http://127.0.0.1:8300` forwards `/ws`, `/client-data.json`, `/emblems`, the account pages at `/account/` and the admin API at `/admin/api/` to a game server, so the login cookie and the client share one site, and `WATHENA_HTTPS_CERT` and `WATHENA_HTTPS_KEY` name a certificate and its key. The decision's "Test on a real phone" has the steps.

## Checks

```text
pnpm check    # type check
pnpm lint
pnpm format:check
pnpm test
pnpm build
pnpm check:browsers   # render, audio, and cache checks in real browsers, and the exit check
CHECKS=touch,phone pnpm check:browsers   # the touch and phone checks, which are not in the default run
```

Touch input is frozen as built in decision 0034, so the default `check:browsers` run leaves out the touch and phone checks:

- `CHECKS=touch,phone` runs them. They are the touch run of `game-check.ts` and the checks at 375 pixels wide.
- With `touch` in `CHECKS`, the exit check plays Bob and Dora on a Pixel 7 with touch, and the quests check adds a phone page with touch. For example, `CHECKS=exit,quests,touch`.
- Without `touch`, the exit check plays all its players on desktops with the mouse, and the quests check plays only its desktop page.

`check:browsers` builds and serves the placeholder pack, then checks in Chromium that the viewer renders a map without errors, that Opus test sounds decode to their exact length, and that a second visit loads assets from the Service Worker. With `CHECKS=touch,phone`, it checks the admin screens and the account pages at 375 pixels wide, and plays on a `waserver --dev` with the end-to-end fixture mod: a scripted touch run on an iPhone and an Android phone profile, a tablet and a phone held sideways, a mouse run on a desktop page, and frame times at phone size with Chrome's CPU throttling, printed as `info` lines. It then runs the phase 6 exit check (`exit-check.ts`, docs/decisions/0035-phase-6-exit-check.md): three players on desktops play every tier 1 system against a real `waserver` with PostgreSQL, a staff account with TOTP mutes a player, and the lantern mod's item, monster, skill and job show their names and art. It needs `WATHENA_TEST_DATABASE_URL`, which `scripts/check.sh browsers` sets, and `EXIT_STEPS`, a regular expression, runs only the steps it matches. `CHECKS` runs only the checks it names, such as `CHECKS=render,quests`. `SCREENSHOTS` names a folder for screenshots of failed steps. `BROWSERS=chromium,firefox,webkit` adds the other engines, after `pnpm exec playwright-core install`. `scripts/check.sh browsers` runs it in Chromium. The check `field` builds `waserver` with `-tags procedural` and runs a `--dev` world of two generated fields: a page generates its field in the worker with the server's checksum and walks three legs, and a page with `?nogen` downloads a map with the same cells. The check `gen` always runs in Chromium, Firefox and WebKit, or the browsers `GEN_BROWSERS` names: it loads `gen.wasm` through `generator.ts` and compares the golden checksums of `shared/gen/testdata`, and each field's WMAP hash. A browser that does not start is skipped with a message. The check `places` runs a `--dev` world with a field, a hamlet and a crypt of two floors, and the Market Town sample mod: a player walks from the field into the town, talks to the Town Guide that stands on the square by its place, and walks into the crypt's first floor, and each map is generated in the page with the server's checksum. The check `gen` covers dungeon floors and towns too, with their places' hashes. The check `layout` runs a `--dev --layout` world of two fields and a town with the Home Village sample mod (docs/decisions/0041-world-layout.md): a player starts on the home map, which the page downloads, walks through its portal into the town, which the page generates, the map field shows each map's display name, and the World Map window shows both maps as visited. The check `quests` runs a `--dev` world with the Hunting Board sample mod, whose Hunt Master the check moves to the start map and whose quest it points at the fixture's Dummies (docs/decisions/0043-quests.md): a desktop page with the mouse sees her "!" icon, takes the quest, reads its timer and objective in the quest window, kills three Dummies, sees "Dummy 3/3" and the "?" icon, marks the quest inactive and active again, and turns it in. With `touch` in `CHECKS`, a phone page with touch also takes the quest, opens the window through the menu, and marks it inactive. The check `content` makes a small procedural world with a `wactl` built with `-tags procedural` in a new database of `WATHENA_TEST_DATABASE_URL`, and runs it with development sessions (docs/decisions/0044-procedural-content.md): a player in the first town walks to the Kafra Employee at the storage place and talks to her, and walks to the quest board at its `quest_board` place and reads the town's quests. A click of a walk on a crowded square may open an NPC's dialog, so the check closes it and walks on, and walks through the connection when no click gets there. The check `rotation` runs a `--dev --layout` world whose one dungeon rotates (docs/decisions/0045-rotation.md): a hidden player warps into the rotating first floor, sees the warning in the chat log when the server clock moves to 9 minutes before the rotation, follows the server to the town at the time, and warps into the new first floor, whose map ID carries the new date and which the page generates with the server's checksum.

### Writing a browser check

A check must pass on a busy machine, where a frame of SwiftShader can take half a second. It waits for the signal that a state changed, not for a time (docs/decisions/0037-reliable-browser-checks.md):

- Wait for what the page shows once the server answered: a window that opens, a row that goes, a button that turns enabled, a line in the chat log, or a damage number.
- Before a tap or a click on a unit or a cell, wait for a still view. `settle(page)` waits until the player stands and the camera has reached it (`viewer.settled()`). `unitScreen` and the exit check's `Player.still` also wait until the unit stands (`walking` in `viewer.units()`).
- After a walk, wait for the player's cell with `walkedTo`, not for a time.
- Touch events carry their own times. `Fingers.tap` and `Fingers.doubleTap` send their events together, 60 ms apart, and the client reads gesture times from `event.timeStamp`. A busy page sees the gesture as a phone would send it.
- Targets stand still. The fixture's Dummies cannot move, and the checks give the lantern mod's Lantern Ghosts `no_random_walk`.
- Do not retry a step that missed. A retry hides the cause. Retry only what a player would do again.
- Timeouts are upper bounds for a busy machine, such as 10 or 15 seconds. Keep fixed waits for what lasts a time on purpose, such as a finger on the joystick.

The WMAP decoder test reads `src/formats/testdata/sample.wmap`, which the Go encoder's tests write. After changing the format, run `go test ./internal/assets/wmap -update` from the repository root.
