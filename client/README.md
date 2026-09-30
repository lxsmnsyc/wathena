# client

TypeScript web client. See docs/strategy.md, section 10.

It is at an early stage: a map viewer for development, built on the parts the game client will use.

| Folder | Content |
|---|---|
| `src/assets/` | Loads assets by name from a build. Checks every file against its SHA-256 from the manifest. |
| `src/formats/` | Decoders for the binary asset formats in docs/specs/assets-v1.md. |
| `src/render/` | three.js scenes: the ground, water, and models of a map, lit as RO lights them, sprites that face the camera, and characters built from body, head, and equipment sprites, and baked effects. |
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

Open `http://localhost:5173/?map=prontera`. Monsters, an NPC, and player characters with random jobs, hair, dyes, and equipment wander near the map's center. `sprites=mob/1002,npc/4_f_kafra1` picks the sprites, as names without `sprite/`, `characters=30` sets how many characters appear, `effect=effect/firewall` plays effects in a row at the center, and `distance=20` moves the camera closer. Click the ground to walk: the player follows the shared pathfinding module, which gives rAthena's paths. A Service Worker caches assets across visits, and `nosw` turns it off, for measuring first visits. The Sound button plays the map's music, its placed sounds, and the sounds sprites make. The status line shows how many files the map needed, the bytes sent over the network, and the load time. `WATHENA_ASSETS` changes where the dev server forwards `/assets` (default `http://127.0.0.1:8200`).

`server=ws://127.0.0.1:8300/ws&name=Tester` plays online against `waserver` (`server/cmd/waserver`). The server places every character and item:

- Click a unit to attack it, and click the ground to walk.
- Items on the ground show their sprite, `sprite/item/<id>`, and jump and bounce when they drop. Click an item to pick it up. The inventory list shows its icon, `icon/item/<id>`, its ID, and the amount.
- The top bar shows HP, base and job levels, and exp as a percent of the next level.
- The Status button or Alt+A opens the status window: STR to LUK with the cost of the next point and a "+" button that raises the stat by 1, and ATK, MATK, DEF, MDEF, HIT, FLEE, CRIT, ASPD, status points and weight. "+" is enabled when the status points pay for the raise.
- Enter opens the chat input, Enter sends the line and clears it, and Escape leaves the input. Enter on an empty line also leaves it. While the input has focus, game shortcuts such as Alt+A do nothing. The log above the inventory keeps the last 100 lines as "Name : text", and each line shows above the speaker for 5 seconds. Lines are cleaned as the server cleans them and cut to 255 code points. The client keeps the server's rate (5 lines, then 1 per second) and says so in the log instead of sending more. Chat is only ever shown as text.
- Effects the server sends, such as level ups, play from `data/effects.yml`. An effect part can also be a sound.

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
