# Log: sieges-castles

- 2026-10-31: built in the main checkout on feat/sieges-castles, one worker, no worktree. Numbers taken: decision 0067, migration 00043. No review decision or proposal number.
- plan.py stopped on release-v0.2.0 (a release node needs from and target since the skill update). Gave it from: main, target: main on this branch.
- Fork ~/Documents/GitHub/rathena, branch wathena-reference: added build_castles and build_map_flags to tools/wathena/extract.py, commit f99a928d1 (amended once to fix MAX_ZENY to INT_MAX; the data did not change). Ran `extract.py castles map_flags` only and copied the four files.
- castle_db.yml: 44 renewal, 34 pre-renewal; pre-renewal enables no teleport.
- The constants extract has no CD_ENABLED_GUARDIAN00 to 07: script_constants.hpp makes them in a loop that the extractor's parser does not follow. The converter maps them.
- TestRunConvertedNPCs failed after the castle commands converted: Okolnir's gift NPC builds an empty map name and R.map_runs passed it to maps.count_players. map_runs now treats "" as a map that does not run. It passes on main because the NPC did not convert there.
- Core castle names are plain (Kriemhild), as other core records; the proposal's examples write core:Kriemhild.
- Database tests ran against postgres:18 in colima.
