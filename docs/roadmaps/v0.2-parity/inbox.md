# Inbox

Ideas and parked items. The curator looks at this list at every checkpoint.

- 2026-10-26: `wa.storage` and `wa.exports` (proposal 0002 next step 2). Parked: no script needs them yet.
- 2026-10-26: Back up the kRO client at `data/kro/client`. It is the only copy (strategy.md "Known issues").
- 2026-10-26: Runs on a real amd64 machine and in Safari (strategy.md "Left:" lines of phases 1-7).
- 2026-10-26: Live economy dashboards, rate settings, monster skills and a model player with skills in the simulator (decision 0046 "Left").
- 2026-10-26: Guild trial dungeon (needs instances), guilds of other families, refiner safe limit and +11 and above (decision 0044 "Left"). Look again after instances-core.
- 2026-10-26: Quest art (decision 0043 "Left").
- 2026-10-26: Map cache on disk, locale files for name templates, cave chambers as places, placeholder buildings (decisions 0040, 0041 "Left").
- 2026-10-26: Touch input extras: item drops on the equipment window and hotbar, pen input, phone frame times. Touch is frozen (decision 0034).
- 2026-10-26: Density scaling by player count, map flags on generated maps (decision 0042 "Left").
- 2026-10-26: Mod switches after start reaching added zones; world quests after a change (decision 0045 "Left").
- 2026-10-26: `MemoryDenyWriteExecute` in the systemd unit, pending wazero testing (deploy/README.md).
- 2026-10-27: Deferred by review decision 60 to later: `OnNPCKillEvent`, `attachrid` without force, reads of another map's player, statuses on a monster of another map, `schedules.yearly`, a level per staff role, reading a limit without permission, chat channel commands, party result codes, guild calls without sieges, `countbound`, `openstorage2` modes, `getitembound3/4`, `guildstoragedelitem`, `npcshopitem`, `npcshopattach`, `mail` arrays, `searchstores` on another map. No rAthena NPC script uses them; mods may ask.
- 2026-10-27: `--at`, a time of day for the backup schedule (review decision 61, A2). Workaround in the deploy guide.
- 2026-10-27: Other Prometheus measurements (review decision 61, B2).
- 2026-10-27: From review decision 62, deferred to later: mods' own tilesets (generator version 4 and a proposal first); a Lua read of a map's stored spawns; the theme chance as a setting; a cancel for a forced world change; locale text and a banner for the change warnings.
- 2026-10-31: Bug on main: `NPCDirectory.mentions` (player events) and `namesLabel` (labels, review decision 60 A2) read an unstarted zone's NPC sources from `z.Mods`, so a mod reloaded before the zone starts is checked by its old files. Needs `npcSource` to see `ReloadMod`'s snapshot safely from other goroutines. Fix PR of its own.
- 2026-11-06: tools/modkit loads the mod data again for every test case: `modkit.(*Env).Load` (`gamedata.LoadMods`, `mod.(*Set).Items`, the overlay merge) is 35% of its CPU time and garbage collection over half (profile of 2026-11-06, PR #15). Load the read-only data once per package and give each case a fresh world; a semaphore on parallel worlds also helps `wactl mod test`. `should`, with perf-pass.
