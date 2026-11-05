# Log: extract-folders

- 2026-11-05: built in the main checkout on chore/extract-folders, no worktree, no worker. Decision 0069. No migration, proposal or review decision number.
- The first placement run placed 1,277 more script NPCs, including 865 from the guild folders and 209 from battleground. TestRunConvertedNPCs then failed on guild Kafras (no guilds in the test host) and on an investment NPC whose OnInit reaches agitcheck. Fix: countOnlyFolders, by the folder of the NPC's source.
- The first copy of the part files ran in zsh. The glob for pre-renewal guild3 matched nothing and stopped the loop before the pre-renewal copies, so they were copied again with bash and nullglob.
