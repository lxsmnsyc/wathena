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
- The top bar shows HP, base and job levels, and exp as a percent of the next level.
- The Status button or Alt+A opens the status window: STR to LUK with the cost of the next point and a "+" button that raises the stat by 1, and ATK, MATK, DEF, MDEF, HIT, FLEE, CRIT, ASPD, status points and weight. "+" is enabled when the status points pay for the raise.
- Enter opens the chat input, Enter sends the line and clears it, and Escape leaves the input. Enter on an empty line also leaves it. While the input has focus, game shortcuts such as Alt+A do nothing. The log keeps the last 100 lines as "Name : text", and each line shows above the speaker for 5 seconds. Lines are cleaned as the server cleans them and cut to 255 code points. The client keeps the server's rate (5 lines, then 1 per second) and says so in the log instead of sending more. Chat is only ever shown as text.
- Item names come from the build's `table/items`, loaded the first time an item shows, with the slot count as "Knife [3]". Descriptions come from `table/itemdesc`, loaded the first time one shows (docs/specs/assets-v1.md, "Table (JSON)"). An item the build does not name shows as "Item 501". Names from a Korean client are Hangul, so the page lists Korean fonts after `system-ui`. Items show their identified name, since the server does not send whether an item is identified yet.
- Effects the server sends, such as level ups, play from `data/effects.yml`. An effect part can also be a sound, or a cylinder that the client draws, as roBrowser's `CYLINDER` effects (`src/render/cylinder.ts`).
- Warp portals are NPCs with sprite 45. They show effect 321, `EF_WARPZONE2`: two blue rings of `texture/effect/ring_blue` that rise and shrink, four copies each, one second apart, as roBrowser draws them. roBrowser's sparks are sprite particles and do not show yet.
- Walking into a warp changes the map. On `MapChange`, the client clears every entity, shows a loading screen, and loads the new map from the asset store, which the Service Worker caches. Events after `MapChange` wait until the map has loaded. Clicks do nothing while it loads. In development builds, `viewer.mapLoads()` lists each map change with its time, from `MapChange` to the map being ready, and the files and bytes it read.

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
