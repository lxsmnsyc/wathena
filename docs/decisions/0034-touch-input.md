# 0034: Touch Input

- Status: proposed.
- Date: 2026-10-15.
- Code: [client/src/viewer](../../client/src/viewer) (`gestures.ts`, `joystick.ts`, `touch.ts`, `touchControls.ts`, `windows.ts`, `settings.ts`, `main.ts`, `player.ts`, `online.ts`, `viewer.css`), [client/index.html](../../client/index.html), [client/vite.config.ts](../../client/vite.config.ts), [client/src/admin/admin.css](../../client/src/admin/admin.css), [server/account/static/style.css](../../server/account/static/style.css), [client/scripts](../../client/scripts) (`browser-check.ts`, `game-check.ts`), [server/account](../../server/account) (`samples_test.go`).
- Builds proposal 0002, review decision 39, and phase 6 step 7 of strategy section 15. No Lua API changes. No protocol changes.

## Decision

### Pointer types

The client picks its input by the pointer that is used, so a mouse and a finger work on one device:

- `InputMode` (`touch.ts`) follows the last `pointerdown`. A touch sets the class `touch` on the page's body, and a mouse or a pen clears it. A page starts in touch mode when the browser reports `(hover: none) and (pointer: coarse)`.
- The mouse and the keyboard work as before. The game view's mouse handler ignores touch pointers, and OrbitControls keeps the mouse but no longer handles touch (`controls.touches` is empty).
- A pen acts as a mouse.

### Gestures on the game view

`GestureRecognizer` (`gestures.ts`) is plain logic. It takes the touch pointers of the game view with their times and returns gestures:

| Gesture | What it does |
|---|---|
| Tap: a touch that ends within 500 ms and moves at most 10 px | What a left click does: walk, attack, pick up, talk, browse a shop, or pick a skill's target. |
| Press: a touch held still for 500 ms | What a right click does: on another player, the player menu. While a skill waits for its target, it cancels the skill. On the ground, it starts a hold. |
| Hold: one finger that moved past 10 px, or that goes on after a press on the ground | Walks toward the finger until it lifts. |
| Two fingers | The change in their distance zooms. A twist, and a sideways move of their middle, turn the camera. An upward move tilts it. |

- A press on a monster, an NPC or an item does nothing, as a right click there does nothing.
- A third finger is ignored. After two fingers, nothing happens until every finger lifts.
- A `pointercancel` ends a gesture without a tap.

### Walking by joystick and by a held finger

RO has no command to walk in a direction. The client asks for a walk to a cell, and the server finds the path. So both walk by asking for cells (`joystick.ts`):

- The joystick asks for a cell 4 cells ahead in the pushed direction, turned by the camera's angle, from the cell the player steps into now. A cell that is not walkable, or whose path is more than 2 steps longer than the distance, is too far: the client tries closer cells, then 45 degrees to each side, so the player slides along walls. It uses the client's pathfinder, which gives the server's paths.
- A finger held on the ground asks for the cell under the finger. The camera follows the player, so a finger held still keeps walking the same way.
- `WalkRequests` sends a new cell at most every 250 ms, and never the same cell twice in a row. That is at most 4 walks a second.
- Letting the joystick go sends one more walk, to the cell one step past the cell the player steps into. The server keeps the step it is on and walks from its end, so a walk to that cell alone would change nothing. The player stops after one or two cells.
- The gateway has no rate limit of its own for walks. Commands go into the zone's queue of 1,024 commands for all its players, and a full queue ends the connection with "Too many actions at once". The limit of 4 walks a second stays far below that.
- The joystick also walks the player of the map viewer, without a server.

### On-screen controls

They show while the body has the class `touch`:

- The joystick, 128 px, in the lower left corner. The new Options window has "Show the joystick on touch screens", saved in the browser (`wathena.settings` in `localStorage`). Tap to walk works either way.
- The hotkey bar of F1 to F9, in a 3 by 3 grid of 52 px slots in the lower right corner.
- The window buttons are the existing ones in the top bar, at least 44 px. Below 600 px wide, they fold into a Menu button.
- A Chat button. It opens the chat with its tabs and input over the game view and focuses the input, which opens the on-screen keyboard. A Send button sends the line. Send on an empty line, or the focus leaving the chat, closes it. While closed, the chat shows its latest lines and lets touches through to the game view.
- A Cancel skill button while a skill waits for its target.

### Windows

- Every window has a title bar (`windows.ts`) with its name. The bar moves the window with a mouse, a pen or a finger, and a press on a window brings it to the front.
- Below 600 px wide, windows open as full-width panels under the top bar and above the joystick and the hotkey bar, so skills and items can still be dragged to the bar. A panel's bar moves it up and down only.
- While items move between the inventory and another window (trade, storage, cart, mail, a shop's or a buying store's setup), the inventory is a sheet at the bottom and the other window stays at the top, so both show.
- Buttons, inputs, selects, list rows and item slots are at least 44 px below 600 px wide.

### Long presses on windows

`installTouchUI` (`touch.ts`) handles touch on everything but the game view:

- A long press of 500 ms on a draggable row (inventory, storage, cart and skill rows) starts a drag. A copy of the row follows the finger, and its border turns green over a place that takes it. Lifting the finger there drops the row through the same `dragstart`, `dragover` and `drop` handlers as a mouse drag, with a `DataTransfer` the client makes. So every place items move takes a finger drag: the game view (drop), the inventory, storage, cart and trade windows, and the hotkey bar for skills.
- A long press lifted without moving does the row's right click, such as storing an item or offering it in a trade. A long press on anything else does its right click at once, such as clearing a hotkey slot.
- Two taps on one row within 450 ms do its double click: use or equip an item, or cast a skill.
- The browser's own long press menu, touch drag and double click are turned off for touch, so each action happens once. Text fields keep the browser's own menu.
- While a finger drags, `touchmove` is cancelled, so lists do not scroll under it.

### Browser behavior

- The viewport has `viewport-fit=cover`, and the controls keep clear of the safe areas.
- `touch-action: manipulation` on the page stops double-tap zoom. The game view has `touch-action: none`, no text selection and no long press callout. The page has `overscroll-behavior: none`, which stops pull-to-refresh. In touch mode, the page's text cannot be selected, except in text fields.
- Text fields are 16 px in touch mode, since Safari zooms into smaller ones.
- The on-screen keyboard covers the page without resizing it. The client reads how much it covers from `visualViewport` into `--keyboard`, and the open chat sits above it.
- Portrait and landscape both work. A phone held sideways is wider than 600 px, so it has normal windows, shorter where the height is small.

### Account pages and admin screens

- The account pages (`server/account/static/style.css`) break long names and table cells, use a fixed table layout below 600 px wide, and have 44 px buttons and inputs.
- The admin screens (`client/src/admin/admin.css`) break long text. Below 600 px wide, a wide table scrolls inside its own box, and buttons and inputs are 44 px.
- Neither scrolls sideways at 375 px wide.

### Rendering resolution

The renderer's pixel ratio is the device's, at most 2 (`setPixelRatio(Math.min(devicePixelRatio, 2))`), which the client already did. A Pixel 7 at 2.625 draws its 412 × 839 view at 824 × 1678.

### Development server on a phone

`vite.config.ts` gains two settings for a test on a phone:

- `WATHENA_GAME`, such as `http://127.0.0.1:8300`, makes the dev server forward `/ws`, `/client-data.json` and `/emblems` to the game server. The phone then needs to reach the dev server alone, and `waserver --dev` stays on the loopback address it requires.
- `WATHENA_HTTPS_CERT` and `WATHENA_HTTPS_KEY` serve the dev server over HTTPS. The client checks every asset with `crypto.subtle`, which browsers give only to secure pages, and `http://` on a local network address is not one.

## Frame times

Measured with `frameTimes` of `game-check.ts`: 10 seconds of `requestAnimationFrame` in a Pixel 7 profile (412 × 839, device pixel ratio 2.625, drawn at 2), with Chrome's CPU throttling. "Online" is a player on the placeholder map with 3 monsters, 7 NPCs and a few players. "Viewer" is the map viewer with 40 random characters. Budget: 30 FPS on mid-range phones (strategy section 3.8). Chrome's "mid-tier mobile" throttling is 4×, and "low-end mobile" is 6×.

| Browser | Scene | CPU | FPS | p50 | p95 |
|---|---|---:|---:|---:|---:|
| Chrome 155 headless, Metal GPU, Apple M2 | Online | 1× | 60.0 | 16.7 ms | 16.7 ms |
| | Online | 4× | 60.0 | 16.7 ms | 16.8 ms |
| | Online | 6× | 60.0 | 16.7 ms | 16.7 ms |
| | Viewer, 40 characters | 1× | 60.0 | 16.7 ms | 16.8 ms |
| | Viewer, 40 characters | 4× | 60.0 | 16.7 ms | 16.7 ms |
| | Viewer, 40 characters | 6× | 60.0 | 16.7 ms | 16.8 ms |
| Playwright's Chromium, SwiftShader | Online | 1× | 34.5 | 33.3 ms | 33.6 ms |
| | Online | 4× | 24.2 | 35.0 ms | 51.6 ms |
| | Viewer, 40 characters | 1× | 28.9 | 33.3 ms | 50.0 ms |
| | Viewer, 40 characters | 4× | 20.6 | 50.0 ms | 52.0 ms |

- With a GPU, the frames stay at 60 FPS at 6× throttling, so the main thread's work per frame is under 16 ms on a CPU 6 times slower than an M2.
- SwiftShader draws with the CPU, and throttling slows its drawing too. It is the floor of `pnpm check:browsers`, not a phone. `check:browsers` prints these numbers as `info` lines, and does not fail on them.
- The placeholder map is small. A town such as Prontera from the kRO build, on a real phone, is the measure that counts: see "Test on a real phone".

## Test on a real phone

The phone and the Mac must be on the same network. The dev server listens on the Mac's local address only while the test runs. Anyone on that network can then play any character of the `--dev` server, so stop both servers afterwards.

1. Find the Mac's address, such as `192.168.1.20`:

   ```text
   ipconfig getifaddr en0
   ```

2. Build an asset build, from the repository's root. The kRO client gives real maps. `--source placeholder` works without game files:

   ```text
   go run ./tools/wactl assets build --source grf:data/kro/client --out ~/wathena-build
   ```

3. Start the game server on the loopback address, trusting the page's origin:

   ```text
   go run ./server/cmd/waserver --assets ~/wathena-build --map prontera --dev \
     --start-job 1:50:40 --start-items 501:10,909:20,1201:1,2101:1 \
     --origins https://192.168.1.20:5173
   ```

   With the placeholder build, use `--map placeholder/map`.

4. Make a certificate for the address, valid for 7 days, outside the repository:

   ```text
   openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 7 \
     -subj /CN=wathena-dev -addext "subjectAltName=IP:192.168.1.20" \
     -keyout ~/wathena-dev-key.pem -out ~/wathena-dev-cert.pem
   ```

5. Start the dev server on the local address:

   ```text
   cd client
   WATHENA_GAME=http://127.0.0.1:8300 WATHENA_ASSETS=http://127.0.0.1:8300/assets \
     WATHENA_HTTPS_CERT=$HOME/wathena-dev-cert.pem WATHENA_HTTPS_KEY=$HOME/wathena-dev-key.pem \
     pnpm dev --host 192.168.1.20
   ```

   macOS may ask whether node may accept connections. Allow it for the test.

6. On the phone, open `https://192.168.1.20:5173/?server=wss://192.168.1.20:5173/ws&name=Phone` and accept the certificate warning. A second phone or a desktop tab with another name gives a partner for trades.

7. Afterwards, stop both servers with Ctrl+C and delete the two `.pem` files.

On Android, `adb reverse` avoids the network and the certificate. With the phone on USB and USB debugging on, run `adb reverse tcp:5173 tcp:5173`, start the game server without `--origins` (it trusts `http://localhost:5173` by default), start the dev server with `WATHENA_GAME` and `WATHENA_ASSETS` only and without `--host`, and open `http://localhost:5173/?server=ws://localhost:5173/ws&name=Phone`.

What to try, in portrait and in landscape:

- Tap the ground to walk. Hold a finger on the ground and move it around. Push the joystick, let it go, and check the player stops within two cells.
- Tap a monster to attack it. Long-press another player for the menu.
- Pinch to zoom, and twist or drag with two fingers to turn the camera.
- Tap an NPC, and answer the dialog.
- Open Skills from the Menu, learn a skill with "+", long-press it and drag it to F1. Tap F1, then Cancel skill, then F1 and a monster.
- Open Items. Double-tap a weapon to equip it. Long-press an item and drag it onto the game view.
- Move a window by its title bar.
- Tap Chat, type a line, and tap Send. Check the line shows above the keyboard.
- Trade an item with the second page by a long-press drag into the trade window.
- In Options, hide the joystick.
- Check that a double tap does not zoom the page, that pulling down does not reload it, and that a long press selects no text.
- Open the account pages and `/admin` on the phone and check nothing scrolls sideways. These need the server with accounts (server/README.md, "Running with accounts").
- Note the frame rate in a busy place, such as with the browser's FPS meter where it has one.

## Tests

- `client`: `gestures.test.ts` (taps, presses, holds, the slop, a late timer, pinches with scale, turn and middle, a turn across ±π, a hold ended by a second finger, extra fingers, cancels), `joystick.test.ts` (the dead zone, the knob, the direction with the camera's turn, the 8 directions, aiming closer and sliding along walls, a blocked way, a long path, the stop cell, and the request limit, at most 4 a second for a target that changes every frame), `settings.test.ts`.
- `server/account`: `TestPageSamples` renders every page with sample data, and writes them for the browser check with `WATHENA_PAGE_SAMPLES`.
- `pnpm check:browsers` gains three checks. `CHECKS` picks checks by name.
  - `phone`: every admin screen and every account page at 375 px wide, without sideways scroll.
  - `touch`, in Chromium, on a `waserver --dev` with the end-to-end fixture mod on the placeholder map, through the dev server's `WATHENA_GAME` forwarding:
    - An iPhone 13 profile: touch mode with the controls and the canvas at 2×, tap to walk, a press then a hold beside the player, the joystick both ways with the stop after the release and the count of messages sent, attacking a Dummy, the Quartermaster's dialog, learning Bash and dragging it to F1, F1 with Cancel skill and then a cast on a Dummy, the inventory as a full-width panel with 44 px rows, a double tap that equips the Knife, a drag of Jellopy onto the game view, moving a panel by its bar, chat with Send read by the second phone, a trade of a Guard with the second phone by a long press on its player, Accept, a drag into the trade window, OK and Trade, and then pinch and twist.
    - A Pixel 7 profile: the second player of the trade, the joystick both ways, and the inventory as a panel.
    - An iPad (gen 7) profile: windows are not panels and the window buttons show without a menu, tap to walk and the joystick.
    - An iPhone 13 landscape profile: the controls fit without overlap, windows fit the height, and the joystick walks.
    - A desktop page with a mouse: no touch controls, click to walk, a left drag turns and the wheel zooms, Alt+S, a mouse drag of a skill to F3, Alt+E and a double click that equips, a right click on another player for the menu, and moving a window by its bar.
    - Frame times at Pixel 7 size at 1× and 4× CPU throttling, printed.

## Other choices

- A walk-direction command in the protocol, as some clients have. Review decision 39 rules out protocol changes, and walks to cells give the same paths on both sides.
- Joystick walks to the farthest cell in a line. A far cell takes long to stop, and a long path around a wall goes where the player did not push.
- OrbitControls' own touch handling. Its one-finger turn takes the gesture that walks, and it has no twist.
- The browser's own touch drag of HTML drag and drop. Safari and Chrome on phones start it differently or not at all, and it scrolls lists. The client's drag uses the same handlers, so each window's code did not change.
- A window frame that wraps each window. The windows build their content once in their constructors, so a bar added afterwards stays, and no window's code changed.

## Left

- The equipment window and the hotkey bar take no item drops, so an item is equipped by a double tap, and the hotkey bar holds skills only. Both were so before for the mouse (review item).
- A pen acts as a mouse, so it has no hold to walk or joystick walk (review item).
- Frame times on a real mid-range phone in a town of the kRO build. The user's test on a real phone gives the first numbers.
- A press on a monster or NPC does nothing. RO has no right-click menu for them.
