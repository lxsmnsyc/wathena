# wAthena

wAthena is a game server and web client for playing with game files from Ragnarok Online. It is a rebuild of [rAthena](https://github.com/rathena/rathena) with a Go server, Lua scripting, a browser client, community mods, and procedurally generated worlds.

wAthena is not affiliated with or endorsed by Gravity or the rAthena team.

## Status

Early planning. There is no playable build yet.

- The plan is in [docs/strategy.md](docs/strategy.md).
- Reference data recorded from rAthena is in [reference/rathena](reference/rathena).

## Repository layout

| Folder | Content |
|---|---|
| `server/` | Go services: gateway, account, world, and zone. |
| `shared/` | Go package compiled to WebAssembly for the client: pathfinding and map generation. |
| `schema/` | Message and data schemas, with code generation. |
| `client/` | TypeScript web client, including client data tables. |
| `mods/core/` | The core mod: rulesets, converted scripts, and game data. |
| `tools/wactl/` | Command-line tool for worlds, mods, and maps. |
| `tools/convert/` | Converter from rAthena scripts and data. |
| `docs/` | Guides, specifications, and design proposals. |
| `deploy/` | Docker Compose files and example configuration. |
| `reference/` | Reference data from rAthena, used by parity tests. |
| `internal/` | Go packages shared by the server and tools, such as the reference data loader. |

## Game files

wAthena does not include or host any Gravity assets. Each player loads sprites, textures, models, and sounds from their own game files in the browser. Mods can ship original assets they have the right to share.

## License

wAthena is licensed under the GNU General Public License, version 3 or later. See [LICENSE](LICENSE).

Mods that use only the public Lua API, YAML data, and assets are covered by the mod exception in [MOD-EXCEPTION.md](MOD-EXCEPTION.md), so mod authors can choose their own license.

Data and scripts converted from rAthena stay under GPL v3.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) and the [code of conduct](CODE_OF_CONDUCT.md). Report security problems as described in [SECURITY.md](SECURITY.md).
