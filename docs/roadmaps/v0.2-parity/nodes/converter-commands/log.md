# Log: converter-commands

- 2026-11-05: started. Numbers: decision 0072, review decision 65 (64 was last), no migration planned (next would be 00044).
- Checked first: none of the ten items convert. Baseline rows (renewal, scripts / this part converts): jobname 2/0, delitem2 1/0, isequippedcnt 6/0, getequiprandomoption 1/0, mesitemlink 33/20, callshop 75/64, setunitdata 26/21, getpartymember 7/5, freeloop 4/0, barter lines 114: 103 full, 6 partly, 5 not. Pre-renewal: delitem2 1/0, freeloop 1/0, getpartymember 6/5.
- Source scripts are in reference/rathena/extracts/<mode>/npcs*.json, not the fork's npc/ tree (the fork has no delitem2 outside custom).
- delitem2's one user (the_sign_quest.txt Knight#ss) takes a named item: card1 254 (CARD0_CREATE), cards 3 and 4 the character ID. Its countitem2 just before refuses at run time (R.cards). Plan: a `signed_by` option of count_item and take_item, so both work.
- getequiprandomoption ROA_ID: the zone knows each option's rAthena ID (item.RandomOption.ID). Plan: an `id` field on wa.RandomOption instead of a name table in rathena.lua.
- mesitemlink: the web client draws a link's item from its own names (dialog.ts label), the link text is only the title. So brackets and a given name only change the string.
- Step 1 built. jobs.name reads a Go table of rAthena's job_name (server/modscript/jobnames.go, made from pc.cpp and map_msg.conf), and `Tables.ModJobName` for mod jobs (a small server/status addition: modscript.Jobs is satisfied by *status.Tables in tools/convert tests, so the method had to go there).
- wa.jobs exists only with constants loaded, so the zone tests cannot call jobs.name; tools/convert's run test does.
- countitem2 now goes through `matchOptions` too (one edit in npcgroup3.go), so the sign quest's countitem2 runs.
- count_item with a refine now also matches the signer (0 = unsigned) and counts rentals, as script_countitem_sub's exact match reads no rental time.
- Step 1 diff: about 880 lines without generated files. Going on.
- Step 2: G7 needs a protocol field and a shop_stock key per entry for an item sold twice; GOAL.md wants a proposal before a protocol change, so only refined costs and 6 cost lines are built (no protocol change). Left for the orchestrator.
- G10: names in the script's text did not cover `"shop_ext171_" + .@str$`; added shops starting with the name's fixed text.
- Step 3: G15 had nothing left to map; only UNPC_GROUP_ID (client group mark) is refused, with a clearer reason.
- Tests: tools/convert full, server/modscript/..., mod, shop, status, gamedata, zone full, check.sh vet lint: pass.
