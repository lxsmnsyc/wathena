# 0044: Procedural Content

- Status: proposed.
- Date: 2026-10-17.
- Code: [server/content](../../server/content) (`content.go`, `quests.go`, `layers.go`, `templates.go`, `core/data/quest_templates.yml`), [server/store](../../server/store) (`content.go`, migration 00034, `queries/content.sql`), [server/gamedata/content.go](../../server/gamedata/content.go), [server/fields](../../server/fields) (`content.go`, `fields.go`, `core/data/town_templates.yml`), [server/modscript](../../server/modscript) (`generation.go`, `coreworld.go`, `prelude.lua`, `luaapi/spec.go`), [server/zone](../../server/zone) (`generation.go`, `coreworld.go`, `refine.go`), [server/status/equip.go](../../server/status/equip.go), [server/mod](../../server/mod) (`generated.go`, `permission.go`), [server/cmd/waserver/fields.go](../../server/cmd/waserver/fields.go), [tools/wactl/layout.go](../../tools/wactl/layout.go), [tools/modkit/generated.go](../../tools/modkit/generated.go), [tools/convert](../../tools/convert) (`core.go`, `procedural_npcs.yml`, `core/scripts/procedural`), [client/src/world/fields.ts](../../client/src/world/fields.ts), [client/src/render/map.ts](../../client/src/render/map.ts), [client/scripts/content-check.ts](../../client/scripts/content-check.ts), [mods/examples/wayside-shrines](../../mods/examples/wayside-shrines), [e2e/content_test.go](../../e2e/content_test.go), and the rAthena fork's `build_refine` in `tools/wathena/extract.py`.
- Phase 7 step 6b (strategy section 15, "Phase 7 in full"), and proposal 0002, review decision 46.
- Lua API: `wa.generation` with `generation.on("map")` and `generation.quest_template`, and the permission `world.generate`, as review decision 46 lists them, and nothing else.
- Protocol: none. The map info at `/gen/maps/<id>.json` gains `objects`, and lists the added places.

## Context

Steps 1 to 6a made procedural worlds with fields, towns, dungeons, a layout, spawns and quests, but their towns had no services, no quests and no way to change jobs. Review decision 46 fixed the shape: core's service NPCs placed by tag, quest templates as records, quest boards, job guilds, a rebirth NPC, and `wa.generation` for mods. The open questions were:

- Where the services come from, and how they keep classic behavior.
- How a world's quests reach the quest system of step 6a, which keeps quests as records.
- Where guilds, quest boards and the rebirth NPC stand, since the town generator makes places only at buildings' doors.
- What a generation layer reads and returns, and how its objects reach clients, which generate maps themselves.
- How the refiner works, since wAthena had no refine code and rAthena's refiner does not convert.

## Decision

### Content at world creation

`wactl world new` makes the content after the spawns, in `server/content`:

1. The mods' generation layers run once on every generated field, town and dungeon floor, in the layout's order, in the Lua state of the `spawn_table` hooks. Each layer gets its own node, `child(secret, "generation").child(mod).child(map)`. The rules keep what they allow: places snap to open ground within 8 cells, objects must be props of the map's tileset, and bosses must spawn and be `mini_boss` or `field_boss`, on fields and floors only. The bosses join the spawn lists after the hooks, with the clear cells of decision 0042.
2. The quest templates are core's and the mods' records, with those that `generation.quest_template` added.
3. Each town, in number order, gets a `quest_board` place near its square and 6 board quests. Each quest draws a template by weight from the town's secret node `town(n)/quests` (strategy section 8.2), then fills it:
   - A hunt names a monster of the spawn lines of the town's fields within 2 warps, further up to 6 when none fits, in the template's roles, weighted by the line's count. Bosses never count.
   - A collect quest names such a monster with an etc item that stacks among its drops, weighted by the drop's rate. Its record's drop gives the item at the template's `drop_rate` while the quest is in the log.
   - A delivery names one of the 3 towns nearest by warps.
   - A town's quests name a monster, an item, or a town and template only once. A template that nothing fits draws again, 8 tries per quest.
4. Each class family's guild goes to a town: the towns sorted by their band's lowest level, then number, take the families in turn, the first three at most, so every guild is in a low town. The first family, Swordman, stands in the home area's town. A `guild:<family>` place stands near the town's `guild` place, and each core template now has a `guild_hall` with one. For each stage, the guild's secret node draws a hunt and a collect quest from the templates with `uses: [guild]`, of monsters within 5, 10, 20 or 40 levels of 10 (the first jobs) or 45 (the second jobs), on the fields fewest warps from the guild's town.
5. A `rebirth` place stands near the square of the largest town of the highest band (strategy section 11.4): the towns with the highest lowest level, then the most cells, then the lower number.

Quests get IDs from 900,000 in that order. Core's quests keep rAthena's IDs, all below 62,239, and mods' start at 1,000,000. A world holds at most 100,000.

Rewards scale with the town's band, per template `reward` percent: base exp of a tenth of the Swordman table's exp from the band's lowest level to the next, at least 1, job exp three quarters of it, and 50 zeny per level. Guild tests give none, as rAthena's job tests.

Places keep a cell from each other, so two NPCs never share a cell. Content uses integer math and sorted slices, and the secret branch only, so the public seed reveals no quest.

### Storage

Migration 00034 adds `world_places`, `world_objects` and `world_quests`, which `wactl world new` writes with the layout in one transaction, and the game role reads. A `world_quests` row has the town, the purpose (`board` or `guild`, with the guild, stage and step), the template, the kind, the title, the monster and item by name, the count, the drop rate, the delivery's town and the reward. Checks refuse a guild quest without its guild, a delivery without its town, and a hunt without its monster. The bosses go to `world_spawns`.

`waserver` reads them at start. The places join their maps' places, for NPC records with `place`, `wa.places` and the map info. Each quest becomes a core quest record named by its ID, as core's converted quests are: a hunt has one objective, a collect quest one drop, and a delivery nothing. A quest whose monster or item no longer loads is left out with a warning. A new generator version or a mod change never changes a made world's content.

### Core's NPCs

Core's NPCs of procedural worlds are records of `data/npcs.yml` with `place`, so they appear only where a world has their places, never in a classic world. Their scripts are core's own, under `scripts/procedural/`:

| NPC | Place | What it does |
|---|---|---|
| Kafra Employee | `storage` | Core's converted `F_Kafra` with menu 2, storage only, so `F_KafStor` keeps classic's rules: Basic Skill level 6, the Free Ticket for Kafra Storage, and the fee. The fee is 40 zeny below level 20, as Prontera's, and 20 more per 20 levels of the town's band, up to 100. |
| Innkeeper | `inn` | Core's converted `F_InnMaid`: Save at the player's cell, and a rest for 5,000 zeny. |
| Refiner | `blacksmith` | rAthena's `refinemain`, below. |
| Tool, Weapon and Armor Dealer | `shop:tool`, `shop:weapon`, `shop:armor` | Opens core's shop `shop_<kind>_<tier>`, one tier per 20 levels of the town's band, up to 5. A higher tier sells what the lower ones sell and more, at the items' buy prices. |
| Quest Board | `quest_board` | Lists the town's quests with their states, gives one, takes it back done for its reward, ends deliveries to the town, and lets a quest be given up or taken again. |
| Guild masters | `guild:<family>` | The job tests, below. |
| Valkyrie | `rebirth` | The rebirth, below. |

The scripts use the public API for conversations, quests, items, zeny, shops and job changes. What the API has no call for goes through `wa.internal`, which only core may require and which is not API, as the guild storage log does since decision 0021: the world's quests, a quest's exp reward, skill points, weight, and the refiner's two calls.

### Job guilds

The guild masters follow rAthena's job change rules:

- A Novice at job level 10 with Basic Skill level 9 takes the test of the first job: the hunt, then the collect quest, then a talk with the master, which ends in `player:change_job`.
- A first job at job level 40 with no skill points left takes the second test the same way, then picks one of its two second jobs. Bards and Clowns are men, Dancers and Gypsies women.
- A High Novice and a transcendent first job change without a test, as rAthena's job NPCs let a reborn player. `player:change_job` keeps them in the line they had before their rebirth.

The six families are the first jobs of rAthena's Novice: Swordman, Mage, Archer, Acolyte, Merchant and Thief.

### Rebirth

The Valkyrie follows `npc/jobs/valkyrie.txt`: a transcendent or baby job cannot be reborn, and a second class at base level 99 and job level 50 is reborn with `player:rebirth` when it carries nothing, has no zeny, cart, falcon, mount or skill points left. The new High Novice gets a Knife and a Cotton Shirt. rAthena's warp to the town of the old job and its quest log line are left out.

### The refiner

The refiner ports `refinemain` of `npc/merchants/refine.txt`:

- It lists the worn equipment, as `F_getpositionname` names the slots.
- An item that rAthena does not refine, or that is +10 or more, is refused.
- The price, the ore and the chance are `refine.yml`'s `REFINE_COST_NORMAL` for the item's type and weapon or armor level, at its refine level, as `getequiprefinecost` and `getequippercentrefinery` read them. A chance below 100% asks again.
- The ore and the zeny go first. A draw below the chance refines the item by 1, as `successrefitem`: off, refined, shown again, and back on. Any other draw destroys it, as `failedrefitem`.

`refine_db.json` had no costs. The fork's `build_refine` now adds each refine level's Normal cost, `{ rate, price, material, breaking_rate }`, which `status.Tables.RefineCost` reads. The safe refine and multiple refine features of `refinemain`, the HD and enriched ores, and the refiners of +11 and above are left out.

### wa.generation

- `generation.on("map", function(map, node) ... end)` adds a layer in a mod context. `map` is read-only: `id`, `kind`, `biome`, `tileset`, `band`, `width`, `height` and `places`. The node has `child(label)`, `stream()` and `hash(x, y)`, and a stream `integer(minimum, maximum)` and `chance(numerator, denominator)`, from ChaCha8 (the node's `SecretStream`). A layer has no other source of randomness, so `wa.random` raises an error while it runs (strategy section 8.7).
- A layer returns nil or `{ places, objects, bosses }`: at most 32 places `{ tags, x, y }`, 256 objects `{ model, x, y, turn }` and 8 bosses `{ monster, role, x, y, respawn }`. The prelude checks the shapes, and the server the rules above.
- `generation.quest_template(record)` adds a template, a record of `data/quest_templates.yml` with its name, which gets the mod's prefix.
- Both need `world.generate`, which needs the owner's approval and is not flagged. In a running zone the calls record what they get, and nothing runs it.
- `wactl mod test` runs the layers once when it makes a test world, and the test world's quest boards, guilds and rebirth place follow, from its spawn lists before the hooks. The layers' bosses join a list after its hooks.

### Objects in the client

The map info lists the objects, each with its model's asset name, `model/gen/<tileset>/<prop>`. The client adds them to the map's objects when it loads a generated map: each on the middle of its cell, at the ground's height, turned by its quarter turns, as the generator places its own props. The tileset table maps them as it maps the props. Objects never change cells, so the checksum of the cells stays the generator's.

### Quest templates

Records of `data/quest_templates.yml`: `kind` (`hunt`, `collect`, `delivery`), `title` with `{count}`, `{monster}`, `{item}` and `{town}`, `count`, `roles`, `item` (a delivery's), `drop_rate` (a collect quest's), `reward`, `weight` and `uses` (`board`, `guild`). Core has `hunt`, `thin_out`, `collect`, `delivery` (with a Delivery Box) and `message`. Mods change them by name and add their own. `gamedata.LoadMods` checks them, so `waserver` and `wactl mod validate` refuse a bad record.

## Results

A default world of core's records has 30 board quests, 24 guild quests and 12 places. The golden worlds of `tools/wactl/testdata/golden-content.json` fix two such worlds.

## Tests and checks

- `server/content`: `TestGenerate` (a small world: 6 board quests per town from nearby monsters, rewards that rise with the band, guild tests near their stage's level, places apart on open ground, the rebirth place in the higher town, the same result for a secret and another for another), `TestLayers` (snapped places, refused objects and bosses with their notes, a boss with its clear cells, the layers' templates), `TestParseTemplate` and `TestRebirthTown`.
- `tools/wactl`: `TestGoldenContent` (two default worlds), and `TestProceduralWorld` with the Wayside Shrines sample: shrines, stones, wolves after the hooks, and its quest.
- `server/store`: `TestWorldContent`, with PostgreSQL: the content saves and loads in order, a bad row fails the world, and the game role cannot change it.
- `server/modscript`: `TestGeneration` (the map, the node's repeat and spread, the result, `wa.random` refused, a failing layer, the permission, and `quest_template`), and `TestAPIDefinitions` with the new module and handles.
- `server/status`: `TestRefineCost` for both modes.
- `tools/modkit`: `TestProceduralContent` writes 14 cases with the test world's quests and runs them on both bases: a board's hunt, collect quest and delivery with rewards, the Swordman tests of a Novice and a Swordman, a Novice below job level 10, a Swordman with skill points, a High Novice, the Valkyrie, the Kafra's fee, the inn, the tool dealer, and the refiner with and without ore. `TestWaysideShrines` runs the sample's 3 cases with its objects and boss.
- `wactl mod test` of the Wayside Shrines: every town has a shrine with its keeper, the field has its ring of stones and its wolf, and the quest boards ask for offerings.
- `client`: `layerObjects` in `fields.test.ts`.
- e2e, with PostgreSQL: `TestProceduralContent`. A new Novice in a procedural world takes a board quest of its first town, completes it for its zeny, and with job level 10 and Basic Skill 9 written in the database passes the Swordman guild's test and becomes a Swordman. The fixture's new `!spawn` places the hunts' monsters.
- The browser check `content`: a world that `wactl world new` made in a new database; a player walks to the Kafra at the storage place and talks to her, then to the quest board at its place and reads the town's quests.

## Lua API

`wa.generation` and `world.generate`, as review decision 46 lists them, and nothing else. Proposal 0002, next step 32, has the details to review.

## Left

- The trial dungeon of the guild tests, which waits for instanced maps (strategy section 11.4).
- Guilds of the other families: Super Novice, Taekwon, Gunslinger, Ninja and Summoner, and the third jobs.
- The refiner's safe and multiple refines, HD and enriched ores, the refiners of +11 and above, and a log line of a refine: the refine level saves with the character, but no transfer records it.
- Content in dev worlds without a database: their services stand at the templates' places, but they have no quest boards, guilds or rebirth NPC.
- The golden content runs on this machine's architecture only, as the step `gen` runs the golden spawns on both.
- The fork's change to `build_refine` is in its working tree, not committed. `refine_db.json` names the fork's HEAD, `60bed4ccf`, which lacks it.

## Review items

These need a decision before they are built, since they would be new API or change approved behavior:

- A public call for an exp reward, such as `player:give_exp`, which the permission `player.give_exp` names and no call uses. Core's boards use `wa.internal`.
- A Lua way to read a world's quests, so a mod can offer them on its own NPCs.
- Player fields for skill points and weight, which the guilds and the Valkyrie read through `wa.internal`.
- A way for a layer to read a map's cells, such as `map:is_walkable(x, y)`. Today the server snaps places and bosses to open ground.
- A places scope of fields, such as `"field:any"`, so NPC records can stand at places that layers add to fields.
- Objects of a mod's own art, beside the tilesets' props.
- Whether board quests should have time limits or a daily reset, and whether a done quest should be taken again at once, as now.
- Whether `world.generate` should be flagged.
