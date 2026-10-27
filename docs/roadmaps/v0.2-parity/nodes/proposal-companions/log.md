# Log: proposal-companions

## 2026-10-27

- Read GOAL.md, the roadmap CLAUDE.md, the brief, the empty contract, proposals 0003 and 0004, the sieges contract and outcome, and the four downstream briefs (companion-units, pets, homunculus, mercenaries).
- Read decision 0016 and the code it describes: server/companion (records of falcon and mounts, no unit), server/zone/companions.go, server/modscript/companions.go, server/mod/companions.go.
- Name clash to handle in the text: the code calls the falcon and warg records "companions". The new units get another word, "follower".
- Read the zone: entities.go (entity set), zone.go (entity, Player, handoff in warp.go), mobai.go (monster AI is tied to `entity.monster`; summons use `summonThink`), query.go (`isEnemy`), loot.go:213 (egg drop already goes to the first looter's inventory), skilldispatch.go (skills take `src *entity`).
- Protocol: `EntityKind` already has PET, HOMUNCULUS and MERCENARY (event.proto:290). Event field numbers: 114 used, 0003 takes 115 and 116, 0004 takes 117 to 120. Migrations: 0004 reserved 00043; this proposal names 00044 to 00046.
- Store: characters save through `character.Character` and per-table writes in one save (character_quests is the model). No skill cooldowns are saved for players, so followers do not save them either.
- rAthena findings that shape the design:
  - Homunculus and mercenary AI is client side (the kRO client's Lua AI). The server only takes move, attack and skill requests. wAthena's web client has no such AI, so the server must run it, and no golden can cover "when" a follower acts.
  - Pet bonus `Script` runs in `status_calc_pc` whenever a pet exists, with no equip check. `pet_equip_required` only gates `SupportScript` and the old skill bonus. All 93 renewal and 48 pre-renewal scripts are intimacy tier chains; 9 use `petautobonus`. No pet has a `SupportScript`.
  - pets and homunculi differ per mode (db/pre-re and db/re files: 57 and 107 pets, 8 and 13 homunculus entries). Mercenary records are shared (44) plus 20 renewal ones.
  - The ruleset flag `homunculus` exists and is "nothing yet" (decision 0019). Its rAthena sites: stat weight, ASPD, DEF and MDEF, SC_HOMUN_TIME, the exp share.
  - Pets are not valid battle targets (battle_check_target falls to "invalid target"); homunculi and mercenaries are.
  - Mercenary contract time only runs while the owner is online (saved as remaining time).
- Decided: split homunculus into regular and S (mutation, MH_ skills); keep companion-units, pets, mercenaries; generic follower table in 00044 so the base's e2e has a real saved kind (mercenary uses it); typed tables for pets (00045) and homunculi (00046).
- Wrote docs/proposals/0005-companions.md and filled contracts/companions.md.
