# Log: review-npc-api

- 2026-10-27: Branch renamed to docs/review-npc-api; the worktree was on 38d3fb1 (main), so it was reset to b9a32ba as the task said.
- Read GOAL.md, CLAUDE.md, the brief, decisions 0052 to 0054 and 0062 to 0066, proposal 0002 decisions 54 to 59 and next steps 11 to 18 and 38 to 45, and the briefs of npc-api-events, npc-api-timers, converter-names and converter-commands.
- Counts: the generated reports leave out battleground, custom, guild*, instances and mobs. For commands found only there, counted files in ~/Documents/GitHub/rathena/npc with grep -rlw (test folder left out).
- Finding: most "needed for the rest" commands of steps 12 to 16 have 0 users in the fork's npc tree (countbound, openstorage2, npcshopitem, guildskill, channel_*, party_create...). Recommended defer.
- Finding: settled by later work: getnpctimer and startnpctimer (0064), strnpcinfo(0) (0062, 0064), checkweight lists, computed monster labels, areamonster rectangle, rentitem, schema.Version (release-v0.2.0 brief), the forms of review decision 59 (0066).
- Finding: player:get_skill_level still reads learned levels only (server/zone/itemscripts.go:244), while the zone's skillLevel counts item and temporary levels (skills.go:44).
- Finding: rAthena's npc_checknear skips the range only for class < 0 (npc.cpp 2155); since 0062, view -1 is sprite none, so the HIDDEN_NPC item became a rule for sprite none.
- Finding: no official script calls attachrid with force false, or uses OnNPCKillEvent or schedules-style OnDay labels.
- Wrote review.md with 61 entries and the draft of review decision 60. No code changed, no builds or tests run.
