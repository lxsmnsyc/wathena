# wAthena roadmap: parity with rAthena, then modding

v0.2.0 is tagged with 1:1 parity with rAthena (strategy.md section 14.2a) for everything up to transcendent classes, plus instances, sieges, pets, homunculus, mercenaries and the small systems (chat rooms, marriage, achievements, navigation, stylist, bank, refine and enchant windows). Every in-scope NPC and item script converts and runs in both rulesets, except what a decision refuses on purpose. Classic mode is supported, not experimental. The modding ecosystem (strategy phase 9) follows, then v0.3.0 (third classes, battlegrounds, tier 3 systems).

**Nodes:** 1 active, 10 done, 24 outline, 31 todo
**Must line:** all must work placed by wave 33
**Critical path:** npc-api-events -> instances-core -> instances-content -> security-review-v0.2 -> release-v0.2.0 -> third-classes -> homunculus-s

## v0.2.0
- [x] **check-hygiene**: Check script hygiene: leaked volumes and tool pins [must, S]
- [x] **extract-sync**: Check the extracts reproduce from the rAthena fork and fix the README [must, S]
- [x] **proposal-companions**: Design proposal: pets, homunculus and mercenaries [must, M]
- [x] **proposal-instances**: Design proposal: instances [must, M]
- [x] **proposal-sieges**: Design proposal: castles and sieges [must, M]
- [x] **proposal-small-systems**: Design proposal: chat rooms, marriage, achievements and windows [must, M]
- [x] **review-npc-api**: Review the open NPC API and converter questions [must, M]
- [x] **review-ops-security**: Review the open operations and security questions [must, S]
- [x] **sieges-castles**: Castles: records, registry, ownership and badges [must, M] (after extract-sync, proposal-sieges)
- [x] **review-world-gen**: Review the open world generation and content questions [should, M]
- [~] **npc-api-events**: NPC API: kill events, attached players and handles across maps [must, L, active] (after review-npc-api)
- [ ] **gvg-rules**: GvG map flags and battle rules [must, L, wave 2] (after proposal-sieges, sieges-castles)
- [ ] **npc-api-timers**: NPC API: waits, server clock timers and player timers [must, M, wave 3] (after review-npc-api)
- [ ] **companion-units**: Companion units: follower base, AI and persistence [must, L, wave 4] (after proposal-companions)
- [ ] **instances-core**: Instances: lifecycle, maps, variables and Lua API [must, L, wave 5] (after extract-sync, npc-api-events, proposal-instances)
- [ ] **extract-folders**: Extract rAthena's instance, guild, battleground, custom and mobs NPC folders [must, M, wave 6] (after extract-sync)
- [ ] **sieges-war**: War of Emperium: siege state, guardians, admin and the bot siege [must, L, wave 7] (after gvg-rules, npc-api-timers, sieges-castles)
- [ ] **converter-names**: Convert variable names, getarg, labels and expressions rAthena's parser takes [must, M, wave 8] (after review-npc-api)
- [ ] **homunculus**: Homunculus: creation, skills, evolution and AI [must, L, wave 9] (after companion-units, extract-sync)
- [ ] **instances-content**: Instances: convert and run the memorial dungeons [must, L, outline, wave 10] (after extract-folders, instances-core)
- [ ] **npc-behavior-spike**: Spike: how to prove NPC scripts behave as in rAthena [must, M, wave 11] (after extract-sync)
- [ ] **ui-windows**: Refine, stylist and bank windows [must, L, wave 12] (after extract-sync, proposal-small-systems)
- [ ] **chat-rooms**: NPC and player chat rooms [must, M, wave 13] (after proposal-small-systems)
- [ ] **instances-ui**: Instances: client window, admin screen and staff endpoint [must, M, wave 14] (after instances-core)
- [ ] **item-effects**: Item scripts: autobonus effects, statuses, transforms and bonus scripts [must, M, wave 15]
- [ ] **mercenaries**: Mercenaries: contracts, faith and skills [must, M, wave 16] (after companion-units, extract-sync)
- [ ] **pets**: Pets: taming, eggs, hunger, intimacy and bonuses [must, M, wave 17] (after companion-units)
- [ ] **security-fixes**: Security and operations fixes for v0.2.0 [must, M, wave 18] (after review-ops-security)
- [ ] **sieges-content**: Sieges: convert and run rAthena's castle scripts [must, M, outline, wave 19] (after extract-folders, sieges-war)
- [ ] **sieges-ui**: Sieges: castle window, teleport and admin screens [must, M, wave 20] (after gvg-rules, sieges-war)
- [ ] **converter-commands**: Convert the remaining shop, item, party, guild and storage commands [must, L, wave 21] (after converter-names, review-npc-api)
- [ ] **item-windows**: Item enchant, enchant grade and reform windows [must, L, wave 22] (after proposal-small-systems)
- [ ] **npc-behavior-harness**: NPC behavior parity harness [must, L, outline, wave 23] (after npc-behavior-spike)
- [ ] **window-skills**: Skills that fail before they start: crafting, carts and windows [must, L, wave 24]
- [ ] **achievements**: Achievements and titles [must, M, wave 25] (after proposal-small-systems)
- [ ] **item-skill-use**: Items that cast skills: itemskill, unitskilluseid and delayed use [must, M, wave 26] (after item-effects)
- [ ] **marriage**: Marriage and the wedding skills [must, M, wave 27] (after proposal-small-systems)
- [ ] **monster-gaps**: Monster port gaps: MVP tombs, level ups, warps and missing effects [must, M, wave 28]
- [ ] **navigation**: Client navigation [must, M, wave 29] (after proposal-small-systems)
- [ ] **pre-renewal-parity**: Pre-renewal ruleset parity and classic mode support [must, M, wave 30]
- [ ] **security-review-v0.2**: Security review of the v0.2.0 systems [must, M, wave 31] (after chat-rooms, homunculus, instances-content, instances-ui, mercenaries, npc-api-events, pets, security-fixes, sieges-content, sieges-ui, ui-windows)
- [ ] **bonus-damage-taken**: Bonus sums for damage taken with three options [must, S, wave 32]
- [ ] **release-v0.2.0**: Release v0.2.0: parity with rAthena [must, M, -> main, wave 33] (after achievements, bonus-damage-taken, check-hygiene, converter-commands, converter-names, homunculus, instances-content, item-skill-use, item-windows, marriage, monster-gaps, navigation, npc-api-timers, npc-behavior-harness, pre-renewal-parity, security-review-v0.2, sieges-content, window-skills)
- [ ] **expanded-class-goldens**: Skill damage goldens for Gunslinger, Ninja, Taekwon and Soul Linker [should, M, wave 41] (after extract-sync)
- [ ] **world-gen-fixes**: World generation fixes from review decision 62 [should, M, wave 45] (after review-world-gen)

## modding
- [ ] **signed-releases**: Signed mod releases and signature checks [should, M, outline, wave 35] (after release-v0.2.0)
- [ ] **mod-tooling**: wactl mod pack, purge and convert for community scripts [should, L, outline, wave 36] (after release-v0.2.0)
- [ ] **api-reference-site**: API reference website and modding guide [should, M, outline, wave 37] (after release-v0.2.0)
- [ ] **mod-index**: Mod index format and wactl index support [should, M, outline, wave 38] (after signed-releases)
- [ ] **lua-cost-accounting**: Byte costs for string work in the Lua glue [should, M, outline, wave 42] (after release-v0.2.0)
- [ ] **mod-assets-gaps**: Mod asset gaps: sounds, map folders and mods' maps [should, M, outline, wave 43] (after release-v0.2.0)
- [ ] **modding-exit**: Modding exit check: five outside mods and a forum script [should, M, outline, wave 44] (after api-reference-site, mod-index, mod-tooling)
- [ ] **client-data-template**: Client data guide and template repository [should, S, outline, wave 46] (after release-v0.2.0)
- [ ] **world-gen-api**: World generation reads for mods [could, L, outline, wave 48] (after release-v0.2.0)
- [ ] **world-gen-rules**: World generation records and settings for mods [could, M, outline, wave 49] (after release-v0.2.0)

## v0.3.0
- [ ] **third-classes**: Third classes, then fourth and expanded classes [should, XL, outline, wave 34] (after release-v0.2.0)
- [ ] **battlegrounds**: Battlegrounds [should, L, outline, wave 39] (after release-v0.2.0)
- [ ] **homunculus-s**: Homunculus S: mutation, S classes and skills [should, L, outline, wave 40] (after homunculus, third-classes)
- [ ] **tier3-systems**: Tier 3 systems for parity: VIP, Laphine, reputation, clans, roulette [could, XL, outline, wave 47] (after release-v0.2.0)

## later
- [ ] **character-start-function**: Character start values in the database [later, M, outline, wave -] (after release-v0.2.0)
- [ ] **client-plugin-api**: Web client plugin API [later, L, outline, wave -] (after release-v0.2.0)
- [ ] **distributed-mode**: Distributed deployment and 7,500-bot load tests [later, XL, outline, wave -] (after release-v0.2.0)
- [ ] **headless-client**: Headless web client [later, L, outline, wave -] (after release-v0.2.0)
- [ ] **js-ts-mods**: JS and TS mods, then the scriptc port [later, XL, outline, wave -] (after release-v0.2.0)
- [ ] **lua-state-per-mod**: One Lua state per mod and zone crash recovery [later, L, outline, wave -] (after release-v0.2.0)
- [ ] **other-clients**: Other clients [later, XL, outline, wave -] (after release-v0.2.0)

**Warnings (42):** run `plan.py check` for the list.
