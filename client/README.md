# client

TypeScript web client. See docs/strategy.md, section 10.

It is at an early stage: a map viewer for development, built on the parts the game client will use.

| Folder | Content |
|---|---|
| `src/assets/` | Loads assets by name from a build. Checks every file against its SHA-256 from the manifest. |
| `src/formats/` | Decoders for the binary asset formats in docs/specs/assets-v1.md. |
| `src/render/` | three.js scenes: the ground, water, and models of a map, lit as RO lights them, sprites that face the camera, and characters built from body, head, and equipment sprites, and baked effects. |
| `src/viewer/` | The map viewer page. |
| `data/` | Client data tables, such as `jobs.yml`. |

## Map viewer

Build assets with `wactl assets build`, then serve the build and start the dev server:

```text
wactl assets serve --build /path/to/build
pnpm install
pnpm dev
```

Open `http://localhost:5173/?map=prontera`. Monsters, an NPC, and player characters with random jobs, hair, dyes, and equipment wander near the map's center. `sprites=mob/poring,npc/4_f_kafra1` picks the sprites, as names without `sprite/`, `characters=30` sets how many characters appear, `effect=effect/firewall` plays effects in a row at the center, and `distance=20` moves the camera closer. The Sound button plays the map's music, its placed sounds, and the sounds sprites make. The status line shows how many files the map needed, the bytes sent over the network, and the load time. `WATHENA_ASSETS` changes where the dev server forwards `/assets` (default `http://127.0.0.1:8200`).

## Checks

```text
pnpm check    # type check
pnpm lint
pnpm format:check
pnpm test
pnpm build
```

The WMAP decoder test reads `src/formats/testdata/sample.wmap`, which the Go encoder's tests write. After changing the format, run `go test ./internal/assets/wmap -update` from the repository root.
