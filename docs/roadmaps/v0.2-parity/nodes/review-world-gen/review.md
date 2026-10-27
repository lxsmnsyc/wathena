# Review: open world generation and content questions

Draft for the maintainer, 2026-10-27. Not yet approved.

Review of the items in brief.md "Steps", checked against the code at 727ffd1. The sources are decisions 0040 to 0047, proposal 0002 next steps 4, 5, 17 and 28 to 33, and strategy.md phase 7. strategy.md no longer has an "Open review items" section (the roadmap replaced it), so the items come from the decisions' "Review items" lists and the proposal's steps. Items the inbox already parks (density scaling, map flags, map cache, locale files for name templates, cave chambers, quest art, live economy dashboards, the guild trial dungeon) are not repeated here.

States: **settled** (a later decision or the code already decides it), **built** (the code does it), **open**.
Recommendations: **keep** (accept as built), **accept** (with the exact change and node), **defer** (with a milestone), **refuse**.

None of this blocks classic parity. Two accepted items are small bugs or gaps in the default procedural world and go to v0.2.0 (`should`). The mod-facing reads go to the modding milestone.

## Summary

| # | Item | Recommendation | Node |
|---|---|---|---|
| A1 | `wa.testing` pick of a test world's template | settled: a town per template | none |
| A2 | Mods' own tilesets | defer: later, generator v4 and a proposal first | none |
| A3 | A floor's level band in Lua | settled: `place:get_band` | none |
| A4 | NPC placed by tag with no match | keep: info line | none |
| A5 | IDs for 2 dungeons of one style | settled: `crypt`, `crypt-2` | none |
| B1 | Lua read of graph distances and teleport prices | accept: modding | world-gen-api (new) |
| B2 | Record field for server services on NPCs | refuse | none |
| B3 | `wa.testing` start on a mod's home map | accept: modding, test worlds run mods' maps | mod-assets-gaps |
| B4 | Band formula in small worlds | settled by decision 0042 | none |
| B5 | Longer `@renametown` names | refuse | none |
| B6 | Entered maps and memo points in the data export | accept: v0.2.0 | world-gen-fixes (new) |
| C1 | Monster level, race, element and tags in the spawn hook | accept: tags in modding; level, race, element settled | world-gen-api, world-gen-fixes (test) |
| C2 | Lua read of a map's stored spawns | defer: later | none |
| C3 | Dungeon style spawn biome | accept: modding | world-gen-rules (new) |
| C4 | Theme chance setting | defer: later (inbox) | none |
| C5 | Flag `spawns.modify` | accept: v0.2.0 | world-gen-fixes |
| D1 | Lua list of a player's quests and read of a quest record | accept: modding | world-gen-api |
| D2 | Quest event options | refuse | none |
| D3 | Quest calls for another character | settled by decision 0066 | none |
| D4 | `setquestinfo` | settled: not an rAthena command | none |
| D5 | `questinfo`, `open_quest_ui`, minimap mark (decision 0043's other items) | settled by decision 0063 | none |
| E1 | Lua read of a world's quests | accept: modding | world-gen-api |
| E2 | A layer's read of map cells | accept: modding | world-gen-api |
| E3 | Places scope of fields | accept: modding | world-gen-api |
| E4 | Objects of a mod's own art | accept: modding | mod-assets-gaps |
| E5 | Board quest limits and repeats | accept: modding | world-gen-rules |
| E6 | Flag `world.generate` | accept: v0.2.0 | world-gen-fixes |
| E7 | Exp reward call, skill points and weight fields (decision 0044's other items) | settled by decision 0064; clean up `wa.internal` | world-gen-fixes |
| F1 | Hooks and layers on rotating floors and changed fields | accept hooks: modding; refuse layers | world-gen-rules |
| F2 | Lua read of rotation date or season | accept: modding | world-gen-api |
| F3 | Forced changes across a restart | refuse | none |
| F4 | A cancel for a forced change | defer: later | none |
| F5 | New bands with a world change | refuse | none |
| F6 | More than one rotating dungeon | accept: modding | world-gen-rules |
| F7 | Locale text and a banner for warnings | defer: later | none |
| F8 | Jail return points on removed maps | settled by code; add a test | world-gen-fixes |
| G1 | The model player and targets | keep | none |
| G2 | Economy checks after `spawn_table` hooks; a mod's own targets | settled: `wactl sim economy` rates after hooks; refuse a mod's targets and an automatic re-check | none |
| G3 | Lua read of a field's simulated rates | refuse | none |
| G4 | Every family's guild in a town of the first bands | settled by decision 0047 | none |
| H1 | `content.GuildLevel` as a `procedural.layout` setting | refuse (inbox) | none |
| H2 | Release as archives per target | settled: dropped on 2026-10-20 | none |
| I1 | Step 4: `wa.testing` provisional details | settled; make the last four helpers stable | world-gen-fixes |
| I2 | Step 5: `respawn_milliseconds` | settled by review decision 24 | none |
| I3 | Step 17: reward lists for automatic events | accept: modding | world-gen-rules |
| I4 | Step 17: a map type in map data for `level_band` | accept a narrow fix: v0.2.0; refuse a general map type | world-gen-fixes |

Totals by row (46): 19 accept (B1, B3, B6, C1, C3, C5, D1, E1 to E6, F1, F2, F6, I1, I3, I4), 13 settled (A1, A3, A5, B4, D3, D4, D5, E7, F8, G2, G4, H2, I2), 7 refuse (B2, B5, D2, F3, F5, G3, H1), 5 defer (A2, C2, C4, F4, F7), 2 keep (A4, G1). Accepted work by node: world-gen-fixes 5 (B6, C5, E6, I1, I4) plus three checks (C1 test, E7 cleanup, F8 test); world-gen-api 7 (B1, C1 tags, D1, E1, E2, E3, F2); world-gen-rules 5 (C3, E5, F1, F6, I3); mod-assets-gaps 2 (B3, E4).

## A. Decision 0040: dungeons, towns and places (step 28)

### A1. `wa.testing` pick of a test world's template

- Source: decisions/0040-dungeons-towns-places.md:167; proposal 0002 step 28.
- State: settled. A test world makes one town per town template, in record order, and one dungeon per dungeon style (tools/modkit/generated.go:106-125). A mod's template and style are there with no pick. `wactl mod validate` makes the same maps, so a template that cannot make a town fails validation (decision 0040, "Mod tests on generated maps"). A case finds its town with `wa.places`.
- Options: a `world:` call that takes a template and a size; keep.
- Recommendation: keep. The size of the test field (96 cells, generated.go) and of a town come from the template, which the mod controls. No call is needed.

### A2. Mods' own tilesets

- Source: decisions/0040-dungeons-towns-places.md:168; "Mods name tilesets of the generator" (0040, "Art").
- State: open. A style's `tileset` must be one of the generator's dungeon tilesets (cave, crypt, tower, ruins) and a template's the town tileset (server/fields/records.go:259-285). Tilesets, with their materials and props, are code in shared/gen (v3.go:69-97) and in `gen.wasm`. The operator's table (`procedural.tilesets`, server/cmd/waserver/config.go:148) only maps the generator's names to assets.
- Options:
  1. A mod ships tileset records. The client's wasm must know the new materials and props, so the generator gets a new version, the golden files change and the seed spec needs a proposal.
  2. Keep. A mod picks one of the four dungeon looks and sets art through the operator's table.
- Recommendation: defer to later. Needs generator version 4 and a design proposal first. Props alone (E4) do not need it.

### A3. A floor's level band in Lua

- Source: decisions/0040:169.
- State: settled. `place:get_band()` and `town:get_band()` return the lowest and highest level (spec.go:845, :869), added by review decision 43. A place of a floor reads its floor's band.
- Recommendation: none. Nothing to build.

### A4. NPC placed by tag with no match

- Source: decisions/0040:170; proposal step 28.
- State: built. The NPC does not appear and the server logs an info line (server/cmd/waserver/fields.go:418). A classic world has no places, so a warning would fire for every such NPC in every classic world.
- Options: keep the info line; a warning only in procedural worlds; an error.
- Recommendation: keep. A mod that ships an NPC for `place` tags works in both world modes by design.

### A5. Dungeon IDs for 2 dungeons of one style

- Source: proposal step 28 ("until the map graph names dungeons").
- State: settled by decision 0041. A dungeon's ID is its style's name, then `-2`, `-3` for more of one style (server/layout/layout.go:832; decisions/0041:33). `places.find("dungeon:crypt", ...)` finds the first only; `dungeon:crypt-2` the second. The test world copies the rule (modkit/generated.go:117).
- Recommendation: none.

## B. Decision 0041: world layout (step 29)

### B1. Lua read of graph distances or teleport prices

- Source: decisions/0041-world-layout.md:117; proposal step 29.
- State: open. The teleport service is the server's: prices are `procedural.teleport_base` plus `teleport_step` per warp between towns (decisions/0041:61; config.go:165-166). `towns.nearest` counts warps already, but nothing exposes the count.
- Options:
  1. `towns.get_distance(a, b)` returns the warps between two town handles, or nil when no way leads there. `towns.get_teleport_price(from, to)` returns the zeny the server's service charges, from the same function. Both need `game.read`.
  2. Keep. A mod cannot write its own teleport service.
- Recommendation: accept option 1, in the modding milestone, node `world-gen-api`. Forms are for the node to confirm against the code of the service.

### B2. Record field for server services on NPCs

- Source: decisions/0041:118.
- State: open. The "Teleport Service" NPC stands at each town's `teleport` place and the server drives its menu (decisions/0041:61). Mods cannot set an NPC's service.
- Options: a `service: teleport` record field; keep, with B1 so mods write their own service in Lua.
- Recommendation: refuse the field. With B1 a mod writes a teleport NPC in a script: read the price, `pay_zeny`, `player:warp`. The prices are settings. A field adds a second path to the server's menu code.

### B3. `wa.testing` start on a mod's home map

- Source: decisions/0041:119.
- State: open. `checkMaps` reads and validates a mod's map folders (tools/modkit/generated.go:200-230), but the test world passes no home map to the generator (generated.go:128, `fields.GenerateLayout(g, public, lay, nil)`), so `gen:home:<id>` does not exist there. Mod-assets-gaps already has "zones that run mods' maps" (brief).
- Options:
  1. Test worlds run the mod's map folders, home maps included, and `world:new_player { map = "gen:home:village" }` takes them.
  2. A narrow call only for home maps.
- Recommendation: accept option 1, in the modding milestone, as a step of `mod-assets-gaps`. It is the same work: a zone from a mod's map folder.

### B4. The band formula in small worlds

- Source: decisions/0041:120; proposal step 29.
- State: settled by decision 0042. The normalized formula is gone. A map starts at `1 + band_step × max(0, d - d0)` with d the warps from the home area and d0 the fewest warps of any field (server/layout/layout.go:949-961). A small world climbs by 8 per warp, not to level 99 in a few maps. `procedural.layout.band_step` changes the 8.
- Recommendation: none.

### B5. Longer `@renametown` names

- Source: decisions/0041:121.
- State: built. A name is 4 to 12 ASCII letters (server/layout/names.go:33-34; strategy.md:1748). It must be unique in any case and pass the chat filter. Derived names ("North Valheim Fields 2") use it.
- Options: allow spaces and up to 24 characters; keep.
- Recommendation: refuse. Town names are proper nouns of one word, the same rule as generated names, and staff can pick another word. No mod or owner asked.

### B6. Entered maps and memo points in the data export

- Source: decisions/0041:122; decision 0032 (data export).
- State: open. The export has each character's position and save point (server/account/export.go:123-126), not its entered maps or memo points. They are personal data of the character, saved in `character_maps` and `character_memos` (migration 00031) and read into `character.Character.Visited` and `Memos` (character.go:57-60). The export is built on the game side (server/adminworld/account.go), so the account role needs no new grant and no migration.
- Options:
  1. Add `entered_maps` (map IDs in the order entered) and `memo_points` (`slot`, `map`, `x`, `y`) to `ExportCharacter`. The format stays 1: decision 0032 says new fields keep the version.
  2. Keep. Cost: the export leaves out data the server keeps about a character.
- Recommendation: accept option 1, in v0.2.0, node `world-gen-fixes`. Test: `TestExport` with a visited map and a memo point; `TestExportHasNoSecrets` stays green. decisions/0032 lists the new fields.

## C. Decision 0042: spawns (step 30)

### C1. Monster level, race, element and tags in the spawn hook

- Source: decisions/0042-spawns.md:113; proposal step 30.
- State: partly settled. `monsters.get(id)` gives level, race, element, size, HP, exp and modes (spec.go:211, :1423; decision 0065). It runs on the zone's script host, which reads `Mods.Monsters` (zone/maps.go:1273-1281). The hook state is a zone with no map (zone/spawntable.go:48-57), so it should work there, but the only test runs `monsters.get` in an ordinary zone (zone/maps_test.go:86). Spawn tags (`biomes`, `role`, `themes` of a monster record) are not in `wa.MonsterInfo`.
- Options for tags: add `spawn = { biomes, role, themes }` to the record `monsters.get` returns (nil when the monster has none); keep.
- Recommendation: add a test that a `spawn_table` hook and a generation layer call `monsters.get` (node `world-gen-fixes`). Accept the `spawn` field of `monsters.get` in the modding milestone, node `world-gen-api`. A hook that writes its own selection needs the tags.

### C2. Lua read of a map's stored spawns

- Source: decisions/0042:114.
- State: open. The lists are stored in `world_spawns` and read by `waserver` only. A test can read `world:get_monsters()`, and a mod's `maps.count_monsters`, `maps.find_entities` read a running map.
- Recommendation: defer to later. No script or mod needs the list itself, and the stored lines come from a secret branch that players must not predict (decision 0042, "Who reads what"). A Lua read in a running zone must leave out nothing the lists reveal that monsters in view do not. Reopen when a mod asks.

### C3. Dungeon style field for its spawn biome

- Source: decisions/0042:115.
- State: open. A floor's biome is its tileset (server/fields/spawns.go:76), and a style can name only the generator's four dungeon tilesets (A2). A mod's monsters may name any biome in `spawn.biomes` (1 to 64 lowercase letters, digits and `_`), but no dungeon can draw them.
- Options: a style field `spawn_biome` (same character rules), read where `sm.Biome = p.Style.Tileset`; keep.
- Recommendation: accept, in the modding milestone, node `world-gen-rules`. It is one record field, and the coverage and golden tests stay as they are when the field is left out. `wactl mod validate` checks the name.

### C4. The theme chance as a setting

- Source: decisions/0042:116.
- State: built as a constant: `ThemeChance = 1` in 3 (server/spawns/themes.go:30).
- Options: `spawns.theme_chance` setting; a per-theme chance; keep.
- Recommendation: defer to later and list it in the inbox. Changing it changes every new world's lists and its goldens, and no owner asked. Core's themes can already be replaced by name.

### C5. Flag `spawns.modify`

- Source: decisions/0042:117.
- State: built, not flagged (server/mod/permission.go:57). A flagged permission logs a warning for each mod that has it (permission.go:14-16). It changes nothing else. A `spawn_table` hook can change the monsters of every field and floor of a new world, as `data.core_override` can change core's drops and `rules.override` formulas, and both of those are flagged.
- Options: flag it; keep.
- Recommendation: accept, in v0.2.0, node `world-gen-fixes`. One line plus the permission docs and a test. The cost is one warning line per mod. `world.generate` follows the same rule (E6).

## D. Decision 0043: quests (step 31)

### D1. Lua list of a player's quests and read of a quest record

- Source: decisions/0043-quests.md:103; proposal step 31.
- State: open. The calls are `add_quest`, `complete_quest`, `remove_quest`, `change_quest`, `get_quest_state`, `get_quest_progress`, `quest_timed_out` (spec.go:643-655). Nothing lists the log or reads a record. The data is in server/quest and `character.Character`.
- Options: `player:get_quests(state?)` returns a list of `{ id, name, state, ends_at? }`; `quests.get(quest)` returns `{ id, name, title, time_limit, targets, drops }` from the record, nil when unknown. Both need `game.read`. Keep.
- Recommendation: accept, in the modding milestone, node `world-gen-api`. A mod's quest-giver can then show the player's open quests.

### D2. Quest event options

- Source: decisions/0043:106.
- State: the events `quest_add`, `quest_progress` and `quest_complete` give `{ player, quest, id }` and run for every quest (spec.go:2060-2062).
- Options: `events.on("quest_complete", { quest = "mod:name" }, fn)`; keep, and the handler tests `event.quest`.
- Recommendation: refuse. One `if event.quest ~= "mod:name" then return end` does it, and an option needs a filter in every context that runs the event.

### D3. Quest calls for another character

- Source: decisions/0043:108.
- State: settled by decision 0066. `setquest`, `completequest`, `erasequest` and `changequest` take a character ID and convert over `attachrid` (tools/convert/npcplayers.go:26-27). The Lua calls need no ID: the handle is the character.
- Recommendation: none.

### D4. `setquestinfo`

- Source: decisions/0043:96; brief item.
- State: settled. rAthena has no such command. The fork's script.cpp defines `questinfo` and `questinfo_refresh` only; the one `setquestinfo_req` in the extract is a commented line marked "officially disabled" (Guest Lecturer Mayssel, `npc/re/jobs/novice/academy.txt`). Decision 0043's "32 scripts" are `questinfo` users.
- Recommendation: none. Nothing to build or refuse.

### D5. `questinfo`, `open_quest_ui`, the minimap mark

- Source: decisions/0043:96, :103, :105.
- State: settled by decision 0063. `npc:add_quest_icon` has `mark` and `condition` (docs/generated/lua-api.md:3852-3853), `player:open_quest_log` and `refresh_quest_icons` exist (spec.go:580-581). The reports show `questinfo` converting in 330 of 350 renewal rows; the 16 left have a computed condition (npc-scripts-renewal.md:116). That is a converter matter for converter-commands.
- Recommendation: none here.

## E. Decision 0044: procedural content (step 32)

### E1. Lua read of a world's quests

- Source: decisions/0044-procedural-content.md:140; proposal step 32.
- State: open. Core's boards read them through `wa.internal.world_quests` (prelude.lua:2891; quests.lua:19).
- Options: `world.get_quests({ town?, purpose? })` returns the stored board and guild quests as `{ id, kind, title, town, ... }`; keep.
- Recommendation: accept, in the modding milestone, node `world-gen-api`. It lets a mod offer the world's quests on its own NPCs. Core's boards then use the public call and `wa.internal.world_quests` goes.

### E2. A layer's read of map cells

- Source: decisions/0044:141.
- State: open. A layer's `map` has no cells (proposal step 32). The server snaps places to open ground within 8 cells and bosses within 3, and leaves out what breaks a rule.
- Options: `map:is_walkable(x, y)` and `map:find_walkable(x, y, radius)` in the layer; keep.
- Recommendation: accept `is_walkable`, in the modding milestone, node `world-gen-api`. Layers run once per map at world creation, so a read of a 1,024 square map costs nothing at run time. `find_walkable` only if the node finds it needed.

### E3. A places scope of fields

- Source: decisions/0044:142.
- State: open. Scopes are `town:any`, `town:<id>`, `dungeon:any`, `dungeon:<id>` and `map:<id>` (server/places/places.go:154-178). An NPC record can stand at a place a layer adds to a field only through `map:gen:field:x:y`, which a mod cannot know.
- Options: `field:any`; `field:<x>:<y>`; keep.
- Recommendation: accept `field:any`, in the modding milestone, node `world-gen-api`. It matches `town:any` and `dungeon:any`. `Scope.Matches` needs `gen.ParseFieldID`.

### E4. Objects of a mod's own art

- Source: decisions/0044:143.
- State: open. A layer's object must be a prop of the map's tileset (decision 0044, "Content at world creation" rule 1). Objects never change cells (decision 0044, "Objects in the client"), so their models are server data the client loads by asset name through the tileset table.
- Options:
  1. A mod ships a records file that adds prop names to a tileset, each with a model of the mod's art. Only layers' objects use them, so the generator and the golden maps do not change.
  2. Wait for mods' own tilesets (A2).
- Recommendation: accept option 1, in the modding milestone, as a step of `mod-assets-gaps`, which already owns mod art loading. Objects are not generator output.

### E5. Board quest limits and repeats

- Source: decisions/0044:144.
- State: open. A done board quest can be taken at once, as often as the player wants (tools/convert/core/scripts/procedural/quest_board.lua, `offer` removes a complete quest and adds it again). Generated quests have no time limit (server/quest/quest.go:57 `Limit` is not set by server/content). A quest pays a tenth of the Swordman table's exp from the band's lowest level, scaled by the template's `reward` percent (decision 0044). A board of 6 quests per town repeats without end. `wactl sim economy` rates fields only; quest rewards are in decision 0046's "Left".
- Options:
  1. Template fields `repeat` (`always` by default, `daily`, `once`) and `time_limit` (the quest record's form). The board script offers a done quest again only when `repeat` allows it. Defaults keep today's behavior.
  2. Change core's templates to `daily` at once.
  3. Keep.
- Recommendation: accept option 1, in the modding milestone, node `world-gen-rules`. Leave core's templates at `always` until the node measures a repeating player with the economy simulator, then decide core's default in that node's decision. Cost: a template field and a record field of generated quests; a world made before keeps its stored quests.

### E6. Flag `world.generate`

- Source: decisions/0044:145.
- State: built, not flagged (permission.go:59). A layer can add bosses, places, objects and quest templates to every map of a new world.
- Recommendation: accept the flag, in v0.2.0, node `world-gen-fixes`, with C5.

### E7. Exp reward call, skill points and weight (decision 0044's other items)

- Source: decisions/0044:136, :138.
- State: settled by decision 0064. `player:give_exp` (permission `player.give_exp`, spec.go:582), and the player fields `weight`, `max_weight` and `skill_points` (spec.go:517-519) exist. Core's scripts still use the `wa.internal` copies: `internal.skill_points` (guild.lua:140, rebirth.lua:41), `internal.weight` (rebirth.lua:36), `internal.give_exp` (quests.lua:66). `refine_info` and `refine` have public forms too (`items.refine_cost`, `equipped:add_refine`, decision 0064), but the refiner keeps rAthena's `refinemain` rules and stays as is.
- Recommendation: clean up in v0.2.0, node `world-gen-fixes`. Core scripts use the public calls, and `skill_points`, `weight` and `give_exp` leave `wa.internal` (prelude.lua:2883-2918). Check that core holds `player.give_exp`.

## F. Decision 0045: rotation (step 33)

### F1. Hooks and layers on rotating floors and changed fields

- Source: decisions/0045-rotation.md:121; proposal step 33.
- State: open, known. A rotation or a region change plans spawns from the derived keys with `spawns.Plan` and the pool only (server/cmd/waserver/worldchange.go:252-275). No `spawn_table` hook and no layer runs. So a mod's rule, such as "no undead on crypt floors", holds for the stored world and stops at the first rotation. `waserver` has no world secret, only derived keys (migration 00035).
- Options:
  1. Run the `spawn_table` hooks at each change, in a hook state whose stream comes from the change's derived key (`NewSpawnHooks`, zone/spawntable.go:48 takes a seed). The hooks see the same input as at creation. Layers stay at creation.
  2. Run layers too. Their places, objects and bosses need storing and a places index change at run time, and the content (quests that name a field) has no owner after a change.
  3. Keep. A mod uses `dungeon_rotated` and `region_changed` with the calls of decision 0065 (`spawning.monsters`, `maps.*`), which run after the new maps run.
- Recommendation: accept option 1 for hooks, in the modding milestone, node `world-gen-rules`. Refuse option 2: the handler plus decision 0065's calls cover what a layer adds, and the run-time storage is large. Test: a hook that removes a monster also removes it from a rotated floor.

### F2. Lua read of a rotation date or season

- Source: decisions/0045:122.
- State: open. The state is `layout.State.Dates` and `Seasons` (server/layout/rotation.go:326), shown to staff through the events tool (adminworld/tools.go:297). The Lua events give the date and season once.
- Options: `world.get_rotation(dungeon)` returns `{ date, next_at }`; `town:get_season()` returns the region's season; keep.
- Recommendation: accept, in the modding milestone, node `world-gen-api`. A mod can show "the crypt changes in 3 hours".

### F3. Forced changes across a restart

- Source: decisions/0045:120; proposal step 33.
- State: a forced change waits in memory for its 10 minute countdown (rotation/rotation.go:373-450). A restart in those minutes loses it.
- Options: store the forced change and its due time in `world_rotations` or a new table; keep.
- Recommendation: refuse. A schema change for a 10 minute window. A staff member who restarts the server can force the change again. A stored order could also run later than staff meant.

### F4. A cancel for a forced change

- Source: decisions/0045:122.
- State: no cancel exists (`Manager` has `Force`, `Check`, `Status`, no cancel). Players on the maps are warned and moved at the time.
- Options: an admin route `POST rotations/{dungeon}/cancel` and `region` form, with `events.control`, an audit row and a screen button; keep.
- Recommendation: defer to later and list in the inbox. It changes the admin API and its screens, and the cost of a wrong force is a move to town.

### F5. New level bands with a world change

- Source: decisions/0045:121; strategy.md 8.8.
- State: decision 0045 keeps bands. A changed region keeps its fields' IDs, links and bands.
- Options: draw new bands at a region change; keep.
- Recommendation: refuse. Bands feed the board quests, the guild towns, `place:get_band`, the economy checks and each player's save and memo points. A new biome and new spawns already change a region.

### F6. More than one rotating dungeon

- Source: decisions/0045:123.
- State: open, small. The state is per dungeon: `State.Dates` is a map, `world_rotations` has one row per dungeon, and the manager holds one change slot per dungeon (rotation.go `Change.key`). Only `PickRotation` returns the first dungeon whose style has a rotation (server/layout/rotation.go:136), and `wactl world new` stores that one (tools/wactl/layout.go:203).
- Options: `procedural.layout.rotating`, the number of rotating dungeons (1 to 8, default 1); a style field; keep.
- Recommendation: accept the setting, in the modding milestone, node `world-gen-rules`. The default 1 keeps today's worlds and goldens. No schema change. Test: a world with 2 rotating dungeons rotates both, and each rotation's points move.

### F7. Locale text and a banner for the warnings

- Source: decisions/0045:124.
- State: the warnings are English announcements (worldchange.go `Warn`). Locale files for core do not exist yet (inbox: locale files for name templates).
- Recommendation: defer to later with the locale files. A banner needs a client message, so it needs a protocol change.

### F8. Jail return points on removed maps

- Source: decisions/0045:118; proposal step 33.
- State: settled by code. `unjail` sends a freed player to the save point when the return map does not run (server/zone/social.go:778-790; the code dates from commit 629f6ba). The line in decision 0045's "Left" is stale: it names a case the code handles. No test covers a removed return map.
- Recommendation: add the test in v0.2.0, node `world-gen-fixes`: jail a player on a map, remove that map, release the player, expect the save point.

## G. Decision 0046: economy simulator (phase 7 exit)

### G1. The model player and targets

- Source: decisions/0046-economy-and-phase-7-exit.md:205-206.
- State: built. A plain melee model, enough to compare fields and worlds (decision 0046, "The model player"). Bot runs agree with the simulator within 20% on base exp and on zeny for 3 of 4 fields; the fourth is a spread of rolled drops (decision 0046, exit check).
- Options: keep; take targets from more than the medians; a model with skills (inbox).
- Recommendation: keep. The inbox already holds "a model player with skills" and the dashboards. The 4-field band 91 to 99 is as good as rAthena's own data gives.

### G2. Economy checks after `spawn_table` hooks, and a mod's own targets

- Source: decisions/0046:207.
- State: settled in part. `wactl world new` checks lists before the hooks (decision 0046, "Lua API"). `wactl sim economy --config waserver.yml` makes the world in memory with the mods and prints each field's rates against the targets, after the hooks and layers, and marks `MISS` (tools/wactl/layout.go:182-196; sim.go:58-93). A mod author or owner gets the check after hooks on demand.
- Options: re-check and adjust automatically after hooks; report only at `world new`; a mod's targets; keep.
- Recommendation: keep the on-demand report. Refuse an automatic re-check that adjusts a hook's list, since the hook wrote it on purpose. Refuse a mod's own targets: a mod that wants other rates reads `wactl sim economy`'s numbers and sets its own. A line in the mod guide names the command (node `world-gen-fixes` docs).

### G3. Lua read of a field's simulated rates

- Source: decisions/0046:208.
- State: open. The simulator is Go code over the loaded game data (server/economy).
- Recommendation: refuse. It would put the simulator into every Lua state, and `wactl sim economy` gives the numbers offline.

### G4. Every family's guild in a town of the first bands

- Source: decisions/0046:205.
- State: settled by decision 0047 (`content.GuildTowns`, server/content/quests.go:316-334).

## H. Decision 0047: guild towns and the release build

### H1. `content.GuildLevel` as a `procedural.layout` setting

- Source: decisions/0047-guild-towns-and-release-build.md:100.
- State: a constant of 20 (server/content/quests.go:314). The hosts are the towns whose band starts at 20 or lower, the first three. With none, the lowest town hosts every guild (quests.go:334).
- Options: a setting; keep.
- Recommendation: refuse, and note it in the inbox. A world with `band_step` 20 or more has one host town, and the fallback is the lowest town: guilds stay near level 1. A setting changes where guilds stand in every new world, and its place would be content, not layout.

### H2. The release as archives per target

- Source: decisions/0047:101.
- State: settled. On 2026-10-20 the project chose to publish no binaries or images (decisions/0047 "Left"; strategy 2.2). Releases are tags and `scripts/release.sh` is a build helper.

## I. Proposal 0002 steps 4, 5 and 17

### I1. Step 4: the provisional `wa.testing` details

- Source: proposal 0002:1189; GOAL.md Coverage.
- State: settled but for four helpers. Review decision 32 made the helpers of review decision 15 stable (0002:931), and review decision 24 made the `new_player` fields stable (0002:856). `world:set_staff_role`, `world:plan_restart`, `world:rotate` and `world:change_region` still carry `Provisional: true` (spec.go:1247-1255), so the generated docs say "not reviewed in proposal 0002 yet" (docs/generated/lua-api.md:4029-4071).
- Recommendation: accept them as built. Remove the four flags and regenerate the docs, node `world-gen-fixes`. `rotate`, `change_region` and `plan_restart` are as proposal 0002 describes them (0002:1474, :1544); `set_staff_role` serves the default roles (0002:1349).

### I2. Step 5: `respawn_milliseconds`

- Source: proposal 0002:1190.
- State: settled by review decision 24 (0002:856): `respawn_milliseconds` is final. The param has no provisional flag (spec.go:1375). The step 5 text is a record of its time.
- Recommendation: none.

### I3. Step 17: reward lists for automatic events

- Source: proposal 0002:1305-1320; decisions/0025-automatic-events.md:52.
- State: open. `rewards` is one mapping with one `to` of `killer`, `participants` or `everyone_on_map` (server/worldevent/typed.go:425-440). A boss event cannot give the killer a rare item and the participants zeny.
- Options: `rewards` takes a list of mappings, each with its own `to`; a mapping stays valid as a list of one. Keep.
- Recommendation: accept the list, in the modding milestone, node `world-gen-rules`. Each reward keeps its own progress unit; the permissions per kind stay as now.

### I4. Step 17: a map type in map data for `level_band`

- Source: proposal 0002:1307.
- State: open and worse than the step says. `level_band` picks fields through `gamedata.IsField`, which is `strings.Contains(id, "_fild")` (server/gamedata/world.go:720), and `FieldLevels` skips every other ID. A procedural world's fields are `gen:field:x:y` (shared/gen/field.go:33). So typed events with `level_band` find no field in a procedural world, the default world mode. `maps` lists work, but a mod cannot know the generated IDs. Read from the code; not run.
- Options:
  1. `IsField` is also true for IDs that `gen.ParseFieldID` reads. Classic names stay as they are.
  2. A general map type in map data (field, town, dungeon, other), set by the layout, by the converter and by mod maps. A larger change, with no user for towns or dungeons yet.
  3. Keep.
- Recommendation: accept option 1 now, in v0.2.0, node `world-gen-fixes`, with a test: a procedural world's fields give `FieldLevels` and a `level_band` boss event picks one. Refuse option 2 until a mod asks for events on dungeon floors.

## Questions for the maintainer

1. Typed events (I4): fix `level_band` so it picks generated fields in v0.2.0, and refuse a general map type in map data? Recommended: yes. Today the events cannot pick a field in a procedural world.
2. Flags (C5, E6): flag `spawns.modify` and `world.generate`, so the loader logs a warning for each mod that has one? Recommended: yes. It is one line each and logs only.
3. `wa.testing` (I1): make `set_staff_role`, `plan_restart`, `rotate` and `change_region` stable, and call steps 4 and 5 done? Recommended: yes.
4. Data export (B6): add entered maps and memo points to each character, format version still 1? Recommended: yes.
5. Settled rows: accept A1, A3, A5, B4, D3, D4, D5, E7, F8, G2, G4, H2 and I2 as settled, with the evidence above, and `wactl sim economy --config` as the check after hooks? Recommended: yes.
6. `wa.internal` (E7): core's scripts use the public `skill_points`, `weight` and `give_exp`, and the three leave `wa.internal`? Recommended: yes.
7. Mod-facing reads (B1, C1 tags, D1, E1, E2, E3, F2): build them in the modding milestone as node `world-gen-api`? Recommended: yes. Forms: `towns.get_distance`, `towns.get_teleport_price`, `monsters.get`'s `spawn` field, `player:get_quests`, `quests.get`, `world.get_quests`, `map:is_walkable`, the scope `field:any`, `world.get_rotation`, `town:get_season`.
8. Teleport service (B2): refuse a record field for services, since mods write their own service from B1's reads? Recommended: yes.
9. Rotation (F1, F6): run `spawn_table` hooks at rotations and region changes, refuse layers there (handlers and decision 0065's calls cover them), and add `procedural.layout.rotating` for more than one rotating dungeon, default 1? Recommended: yes, node `world-gen-rules`.
10. Board quests (E5): add template fields `repeat` and `time_limit` with defaults that keep today's behavior, and decide core's default after a measurement? Recommended: yes.
11. Dungeon style `spawn_biome` (C3) and reward lists (I3): build both in `world-gen-rules`? Recommended: yes.
12. Mod art (B3, E4): `mod-assets-gaps` gets two steps, test worlds that run a mod's map folders (home maps too) and prop records for layers' objects? Recommended: yes. Mods' own tilesets (A2) wait for generator v4 and a proposal.
13. Refuse: longer `@renametown` names (B5), quest event options (D2), stored forced changes (F3), new bands at a world change (F5), an automatic economy re-check and a mod's targets (G2), Lua reads of simulated rates (G3), a `GuildLevel` setting (H1)? Recommended: yes to all.
14. Defer to later with inbox lines: stored spawns read (C2), theme chance (C4), cancel of a forced change (F4), locale text and banner for warnings (F7), mods' own tilesets (A2)? Recommended: yes.
15. Tests only (C1, F8): add a test that hooks and layers call `monsters.get`, and a test that a jailed player whose return map is gone goes to the save point, in `world-gen-fixes`? Recommended: yes.

## Proposed nodes

These are proposals for the orchestrator. This node edits no other brief.

- **world-gen-fixes** (new). Type feature, milestone v0.2.0, priority should, size S to M, gate false, depends_on review-world-gen (hard, the gate). Goal: the small accepted items. Steps:
  1. I4: `gamedata.IsField` reads generated field IDs; test with a procedural world and a `level_band` boss event.
  2. C5, E6: flag `spawns.modify` and `world.generate` (server/mod/permission.go); permission docs and `TestPermissions`.
  3. I1: remove `Provisional` from the four `wa.testing` helpers (spec.go:1247-1255); regenerate docs/generated/lua-api.md and docs/generated/lua.
  4. B6: `entered_maps` and `memo_points` in `ExportCharacter` (server/account/export.go, server/adminworld/account.go); docs in decisions/0032.
  5. E7: core's procedural scripts call the public API; remove the three calls from `wa.internal` (prelude.lua, server/modscript/coreworld.go, tools/convert/core/scripts/procedural).
  6. C1: a test that `monsters.get` works in a `spawn_table` hook and in a layer.
  7. F8: the jail test.
  8. G2: one line in the mod guide for `wactl sim economy --config`.
  `owns`: server/gamedata/**, server/worldevent/**, server/mod/permission.go, server/account/export.go, server/adminworld/account.go, server/zone/social_test.go, tools/convert/core/scripts/procedural/**, docs/decisions/** (record next number 0067; security-fixes also takes one, so take it when the node starts). `shared`: server/modscript/luaapi/spec.go, server/modscript/prelude.lua, docs/generated/lua-api.md, docs/generated/lua/**, docs/proposals/0002-lua-api.md. Acceptance: each step has the test named above; `scripts/check.sh quick`.
- **world-gen-api** (new). Type feature, milestone modding, priority could, size M to L, depends_on release-v0.2.0 (hard). Goal: the mod-facing reads: B1, the tags of C1, D1, E1, E2, E3, F2, with their forms in an amendment to proposal 0002 (a review decision records them). All need `game.read`. `map:is_walkable` is only in layers. `owns`: server/places/**, server/modscript/** (the generation, places, towns, quests files), server/quest/**, server/layout/** (distance and price helpers), docs/decisions/**. Each call has a test in `wactl mod test` and a line in the generated reference.
- **world-gen-rules** (new). Type feature, milestone modding, priority could, size M, depends_on release-v0.2.0 (hard). Goal: record and setting changes: C3 (`spawn_biome`), E5 (`repeat`, `time_limit` on quest templates and the board), F1 (hooks at rotations and region changes), F6 (`procedural.layout.rotating`), I3 (reward lists). Each keeps today's behavior by default. `owns`: server/fields/**, server/content/**, server/layout/**, server/rotation/**, server/worldevent/**, server/cmd/waserver/worldchange.go, tools/convert/core/scripts/procedural/quest_board.lua, docs/decisions/**. No schema change; a world made before keeps its stored quests and rotation.
- **mod-assets-gaps** (existing, modding). Add two steps: (a) test worlds run a mod's map folders, home maps included, and `world:new_player { map }` takes `gen:home:<id>` (B3); (b) records that add props to a tileset for layers' objects, with the model of the mod's art (E4).
- **Inbox, no node:** theme chance setting (C4), `content.GuildLevel` setting (H1), stored spawns read (C2), cancel of a forced change (F4), locale text and banner for warnings (F7), mods' own tilesets with generator v4 (A2). Today's inbox lines for the other phase 7 "Left" items stay.

## Draft review decision 62

62. World generation and content review (review-world-gen, decisions 0040 to 0047, and next steps 4, 5, 17 and 28 to 33). The user approved these on (date):
    - **Settled, nothing to build:**
      - Test worlds make a town per template and a dungeon per style; a floor's band is `place:get_band`; two dungeons of one style are `crypt` and `crypt-2`; the band formula is decision 0042's.
      - Quest calls for another character convert (decision 0066); `setquestinfo` is not an rAthena command; `questinfo`, `open_quest_ui` and the minimap mark are built (decision 0063).
      - `player:give_exp`, `weight` and `skill_points` are public (decision 0064).
      - A freed jailed player whose return map is gone goes to the save point.
      - Economy checks after hooks run as `wactl sim economy --config`.
      - Guild towns follow decision 0047; the archives item is dropped.
      - `respawn_milliseconds` and the `new_player` fields are final (review decision 24); steps 4 and 5 are done.
    - **Kept as built:** the info line for an NPC placed by tag with no match; the model player and the targets of decision 0046.
    - **Built in world-gen-fixes (v0.2.0):**
      - Typed events' `level_band` picks generated fields.
      - `spawns.modify` and `world.generate` are flagged permissions.
      - `set_staff_role`, `plan_restart`, `rotate` and `change_region` of `wa.testing` are stable.
      - The data export holds each character's entered maps and memo points, format version 1.
      - Core's scripts use the public calls, and `skill_points`, `weight` and `give_exp` leave `wa.internal`.
      - Tests: `monsters.get` in hooks and layers, and the release of a jailed player whose return map is gone.
    - **Built in the modding milestone:**
      - world-gen-api: `towns.get_distance(a, b)`, `towns.get_teleport_price(from, to)`, a `spawn` field in `monsters.get`'s record, `player:get_quests(state?)`, `quests.get(quest)`, `world.get_quests(options?)`, `map:is_walkable(x, y)` in layers, the places scope `field:any`, `world.get_rotation(dungeon)` and `town:get_season()`. All need `game.read`.
      - world-gen-rules: the dungeon style field `spawn_biome`; the quest template fields `repeat` and `time_limit`, with defaults that keep today's behavior; `spawn_table` hooks at rotations and region changes; `procedural.layout.rotating`, the number of rotating dungeons, 1 by default; `rewards` of automatic events as a list.
      - mod-assets-gaps: test worlds run a mod's map folders, home maps included; records that add props to a tileset for layers' objects.
    - **Deferred to later:** mods' own tilesets (generator version 4 and a proposal first); a Lua read of a map's stored spawns; the theme chance as a setting; a cancel for a forced world change; locale text and a banner for the change warnings.
    - **Refused:** a record field for server services on NPCs; longer `@renametown` names; options for quest events; layers on rotating floors and changed fields; stored forced changes; new level bands at a world change; an automatic economy re-check after hooks and a mod's own targets; a Lua read of simulated rates; `content.GuildLevel` as a setting; a general map type in map data.
    - Additions to API 1.0 only; no protocol or schema change.
