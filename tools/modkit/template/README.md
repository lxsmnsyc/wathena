# {%.Name%}

A wAthena mod made with `wactl mod new {%.ID%}`.

## Files

| Path | What it is |
|---|---|
| `mod.yaml` | The manifest: ID, version, dependencies and permissions. |
| `data/items.yml` | A new item, Guide_Badge. |
| `data/npcs.yml` | The guide NPC in Prontera. |
| `scripts/npc/guide.lua` | The guide's conversation, with saved variables. |
| `locales/en.yml` | The text players read. |
| `tests/guide_test.lua` | Tests of the guide. |
| `.luarc.json`, `.wathena/types` | Editor support for the Lua Language Server. |

## Commands

```text
wactl mod validate          check the manifest, data, scripts and tests
wactl mod test              run the tests
wactl dev --assets <build>  run a local server that reloads the mod
wactl mod pack              build a .zip with a checksum file
wactl mod types             write the editor definitions again
```

The mod guide is mods/README.md in the wAthena repository.
