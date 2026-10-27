# Log: review-world-gen

- 2026-10-27: Read GOAL.md, CLAUDE.md, the brief, review-ops-security's review.md and outcome.md, review-npc-api's draft decision, review decisions 60 and 61, the inbox, decisions 0040 to 0047 ("Left" and "Review items"), proposal 0002 next steps 4, 5, 17 and 28 to 33, and strategy.md phase 7.
- Finding: strategy.md has no "Open review items" section any more. The items come from the decisions' "Review items" lists and the proposal's steps, as in review-ops-security.
- Finding: steps 4 and 5 are settled. Review decision 24 made `respawn_milliseconds` and the `new_player` fields final (proposal 0002:856), and review decision 32 made the helpers of review decision 15 stable (0002:931). Four helpers still carry `Provisional: true`: `set_staff_role`, `plan_restart`, `rotate`, `change_region` (spec.go:1247-1255).
- Finding: test worlds already make a town per town template and a dungeon per dungeon style (tools/modkit/generated.go:106-125), so a pick of the template is not needed. They pass no home map (generated.go:128).
- Finding: `place:get_band` and `town:get_band` exist (spec.go:845, :869). Decision 0042 replaced the band formula of decision 0041 (layout.go:949-961). Duplicate dungeons get `-2`, `-3` (layout.go:832).
- Finding: `monsters.get` (decision 0065, spec.go:211) gives level, race, element and size. It runs on the zone's script host (zone/maps.go:1273). No test runs it in the spawn hook state. Spawn tags are not in it.
- Finding: rAthena has no `setquestinfo` command. script.cpp defines `questinfo` and `questinfo_refresh` only; the one hit in the extract is a commented `setquestinfo_req`. Decision 0043's "32 scripts" are `questinfo` users, built in decision 0063.
- Finding: quest commands on another character convert since decision 0066 (tools/convert/npcplayers.go:26-27).
- Finding: `gamedata.IsField` is `strings.Contains(id, "_fild")` (gamedata/world.go:720). Generated fields are `gen:field:x:y`. `FieldLevels` feeds the typed events' `level_band`, so a procedural world has no field for it. Read from the code; not run.
- Finding: `wactl sim economy --config` rates the lists after the hooks and layers (tools/wactl/layout.go:182-196, sim.go:58-93). Economy checks after hooks exist as a report.
- Finding: rotation state is per dungeon (`layout.State.Dates`, table `world_rotations` keyed by dungeon, one change slot per dungeon). Only `PickRotation` limits a world to one (layout/rotation.go:136, wactl/layout.go:203).
- Finding: rotation and region changes run no hook and no layer (waserver/worldchange.go:252-275 plans spawns with `spawns.Plan` only). The handlers `dungeon_rotated` and `region_changed` can use the API of decision 0065.
- Finding: `Manager.Force` has no cancel (rotation/rotation.go:373). A forced change lives in memory for its 10 minute countdown.
- Finding: a released jailed player goes to the save point when the return map does not run (zone/social.go:783). The line in decision 0045's "Left" is stale. No test covers it.
- Finding: the data export holds position and save point, not entered maps or memo points (account/export.go:105-148). `character.Character` has `Visited` and `Memos` (character.go:57-60). The export is built on the game side (adminworld/account.go), so no grant or migration is needed.
- Finding: core's procedural scripts still call `wa.internal` for `skill_points`, `weight` and `give_exp`, which the public API has since decision 0064 (tools/convert/core/scripts/procedural/guild.lua:140, rebirth.lua:36,41, quests.lua:66).
- Finding: board quests have no limit or cooldown. A done quest can be taken again at once (quest_board.lua `offer`, state complete). A quest gives a tenth of a level's base exp at reward 100 (decision 0044).
- Wrote review.md and outcome.md. Read-only: grep and file reads. No build, no tests, no Docker.
