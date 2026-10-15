# Client Data Tables

The web client keeps tables that the official client keeps in its executable and its Lua files (strategy section 10.5). They map game IDs to asset names and to how the client shows them. This guide explains each table, how to find the right sprite or effect in a game client, how a mod changes the tables, and how to test a change.

The tables are YAML files in [client/data](../client/data). The web client builds them in, and the server sends them again, merged with the mods' tables, in its client data ([decision 0033](decisions/0033-mod-assets.md)).

## The tables

| Table | Lists | Key | What an entry says |
|---|---|---|---|
| `jobs.yml` | `looks` | `look` | A body sprite: `sprite/body/<look>/<m or f>`. `grf_sprite` is its file name in the game client, `race` is `human` or `doram`, and `sex` is set for a look that exists for one sex only. |
| `jobs.yml` | `jobs` | `id` | A job ID's look, and its `mount` look while riding. |
| `weapons.yml` | `weapons` | `id` | A weapon type's `view`, the weapon sprite of items without their own view, and its `attack` and `hit` sounds, one picked at random. |
| `effects.yml` | `effects` | `id` | An RO effect ID's `parts`: an STR `effect`, `variants` of STR effects, a `sound`, or a `cylinder` the client draws. |
| `skills.yml` | `skills` | `id` | A skill's effects by moment: `caster`, `effect` and `hit`, as RO effect IDs, and `aura: false` for a skill without a cast aura. |
| `skills.yml` | `units` | `id` | A skill unit's (`UNT_`) effect. |
| `skills.yml` | `auras` | `id` | An element's cast aura. |

Each file's comments explain its fields. Every name in a table is an asset name of [assets-v1](specs/assets-v1.md), never a file path.

Some things need no table: monsters, NPCs and items are named by ID, such as `sprite/mob/1002` and `icon/item/501`, and a mod's records name their art in their own fields (mods/README.md, "Art").

## Finding a sprite or an effect

The GRF importer names a game client's files with its own rules (assets-v1, "Names"). To find a name:

1. Build the client's assets: `wactl assets build --source placeholder --source grf:<client folder> --out <build>`.
2. List names by prefix: `wactl assets ls --build <build> effect/` or `wactl assets ls --build <build> sprite/body/`.
3. For a body sprite, the client's sprite folder `data\sprite\인간족\몸통\<sex>\<name>_<sex>.spr` gives `grf_sprite: <name>`. Add a look with that name, and point a job at it.
4. For an effect, roBrowser's effect table names the STR file of most effect IDs. `data\texture\effect\<name>.str` is `effect/<name>`.
5. `wactl assets check --build <build>` lists the names the tables refer to that the build lacks. The client shows a placeholder for them.

The map viewer shows a sprite or an effect by name: open the client with `?sprites=mob/1002` or `?effect=effect/firewall`.

## Tables in a mod

A mod ships tables in `client/`, with the file name of the table it changes, such as `client/skills.yml`. A table holds only the entries the mod adds or changes:

```yaml
# client/skills.yml: Bash also plays Magnum Break's flash on each hit.
skills:
  - id: 5
    constant: SM_BASH
    caster: [16]
    hit: [1, 17]
```

- An entry with the key of an existing entry replaces it whole, so it repeats the fields it keeps. Other entries are added.
- Mods apply in load order, so a later mod's entry wins.
- A table may name the mod's own art, such as an effect part `effect: mod/example.lantern/effect/glow`.
- Each merged table must pass the checks of core's table. A mod whose table fails does not load.
- A table is at most 4 MiB, and YAML aliases are not allowed.

The server merges core's tables with every active mod's and sends them in `/client-data.json`. The client replaces its built-in tables with them before it connects. The same file, in format version 2, also names the art and the names of mods' records ([decision 0035](decisions/0035-phase-6-exit-check.md)).

## Testing a change

- `wactl client-data validate` checks core's tables. `wactl client-data validate <mod folder>...` checks the mods' tables merged over core's, in the order given.
- `wactl mod validate` checks a mod's tables too, merged with the mods it depends on.
- `go test ./client/data` and `pnpm test` in `client/` check core's tables after an edit.
- To see a change, run the server with the mod (`wactl dev`, or `waserver --mods`) and open the client. `/client-data.json` on the server shows the merged tables.
