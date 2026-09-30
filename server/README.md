# server

Go services: gateway, account, world, and zone. See docs/strategy.md, section 4.

| Package | Content |
|---|---|
| `zone` | One map: its players, their walks through `shared/path`, and what each sees within 14 cells, in 50 ms ticks. |
| `gateway` | The WebSocket protocol from docs/proposals/0001-protocol.md: Hello first within 10 seconds, version and size checks, and commands to the zone. |
| `cmd/waserver` | A development server in one process: an asset build over HTTP and one zone over WebSocket. |

## Development server

There is no account service yet, so `--dev` takes any valid character name as the session token. Never expose a `--dev` server to the internet.

```text
go run ./server/cmd/waserver --assets /path/to/build --map prontera --dev
```

It serves the build at `http://127.0.0.1:8300/assets/` and the game at `ws://127.0.0.1:8300/ws`. With the web client's dev server:

```text
WATHENA_ASSETS=http://127.0.0.1:8300/assets pnpm dev
```

then open `http://localhost:5173/?server=ws://127.0.0.1:8300/ws&name=Alice` in two tabs with different names. Click the ground to walk. Each tab sees the other walk.
