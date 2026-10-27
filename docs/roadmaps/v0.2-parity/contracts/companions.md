---
name: companions
version: 1
status: frozen
defined_by: proposal-companions
---

# Contract: companions

## Purpose

The interface between the companions design and the nodes that build on it: companion-units, pets, mercenaries and homunculus in v0.2.0, and homunculus-s in v0.3.0. Frozen 2026-10-27, when the maintainer accepted design proposal 0005 ("Decisions in review" 1 to 18; decisions 8 and 13 move Homunculus S to v0.3.0).

Source: [docs/proposals/0005-companions.md](../../../proposals/0005-companions.md). Section numbers below are the proposal's. "Follower" is the proposal's word for a pet, homunculus or mercenary unit; decision 0016's "companions" (falcon, warg records) are unchanged.

## Interface

Zone (section 1): `entity.follower *follower` (kind, owner, state, `walkState`, AI, `farAt`, `sp`); `walkState` embedded in `monster` and `follower` with `entity.walker()`; `battle.CalcMisc(unit, ...)` with `UnitMonster`, `UnitPet`, `UnitHomunculus`, `UnitMercenary`; `z.ownerOf(e)`, `e.sourcePlayer()`; `progression.DamageLog` source flag (`normal`, `homunculus`, `pet`); `companion.FollowerKind` (`Spawn`, `Save`, `Load`, `Think`, `OnOwnerDeath`, `OnOwnerLogout`, `Info`) and `FollowerState` (`PetState`, `HomunculusState`, `MercenaryState`). A follower travels in `character.Character` (`Pet`, `Homunculus`, `Mercenary`, `MercenaryGuilds`).

Data (section 5): kinds `mod.PetKind`, `mod.HomunculusKind`, `mod.MercenaryKind`; files `data/pets.yml` (keyed by fixed `monster`; economy field `capture`), `data/homunculi.yml` (fixed `id`; `evolution`, `stats`, `skills` with `use`), `data/mercenaries.yml` (fixed `id`, `guild`, `skills`). Core's files are converted from new extracts `pets.json`, `homunculi.json` (with the exp table) and `mercenaries.json`, kept in `server/companion/core/data/`. Pet `bonuses` are intimacy tiers with optional `procs`; `support` is the `SupportScript` block; `evolution` lists targets and items.

Rules (section 12), in `server/rules/limits.go`, with rAthena's shipped values: `pet_catch_rate`, `pet_legacy_formula`, `pet_distance_check`, `pet_hide_check`, `pet_rename`, `pet_friendly_rate`, `pet_hungry_delay_rate`, `pet_equip_required`, `pet_unequip_destroy`, `pet_attack_support`, `pet_damage_support`, `pet_support_min_friendly`, `pet_status_support`, `pet_support_rate`, `pet_attack_exp_to_master`, `pet_attack_exp_rate`, `pet_lv_rate`, `pet_max_stats`, `pet_max_atk1`, `pet_max_atk2`, `pet_disable_in_gvg`, `pet_ignore_infinite_def`, `pet_master_dead`, `pet_walk_speed`, `pet_evolution`, `pet_autofeed`, `pet_autofeed_rate`; `hom_setting`, `homunculus_friendly_rate`, `hom_rename`, `homunculus_evo_intimacy_need`, `homunculus_evo_intimacy_reset`, `hvan_explosion_intimate`, `homunculus_auto_vapor`, `homunculus_max_level`, `homunculus_exp_gain`, `homunculus_starving_rate`, `homunculus_starving_delay`, `homunculus_autoloot`, `homunculus_autofeed`, `homunculus_autofeed_rate`, `hom_delay_reset_vaporize`; `mercenary_autoloot`, `mer_idle_no_share`, `mercenary_kill_faith`; `follower_follow_range`, `follower_chase_range`, `follower_teleport_delay`. The ruleset flag `homunculus` gets its effects. Map flag `nopetcapture` for `maps.set_flag` (needs `maps.control`) and `map_flags.json`.

Database (section 9): migration 00044 `character_followers` (companion-units; `character_id`, `kind`, `record`, `name`, `hp`, `sp`, `state` jsonb); 00045 `pets` and `pet_id` on `inventory`, `storage_items`, `guild_storage`, `mail_items` (pets); 00046 `character_homunculi` and `character_homunculus_skills` (homunculus); 00047 `character_mercenary_guilds` (`character_id`, `guild` 0 to 2, `faith`, `calls`) (mercenaries). Numbers are taken in merge order after 00043 (`castles`, proposal 0004). Game role: SELECT, INSERT, UPDATE, DELETE.

Lua (section 6), new modules `wa.pets`, `wa.homunculi`, `wa.mercenaries`:

- `pets.get`, `pets.id`, `homunculi.get`, `homunculi.id`, `mercenaries.get`, `mercenaries.id` (`game.read`).
- Player: `player:start_taming(lure?, mode?)`, `player:create_pet_egg(pet)`, `player:create_homunculus(class?)`, `player:hire_mercenary(class, ms)` (`companions.manage`); `player:open_egg_list()` (none). Fields `player.pet`, `player.homunculus`, `player.mercenary`.
- `mercenaries.get_faith(player, guild)`, `set_faith(player, guild, delta)`, `get_calls`, `set_calls`.
- `wa.Pet` fields `id`, `info`, `name`, `renamed`, `level`, `hunger`, `intimacy`, `grade`, `accessory`, `autofeed`, `egg`, `game_id`; methods `set_intimacy`, `add_intimacy`, `set_hunger`, `feed`, `rename`, `return_to_egg`, `evolve`, `set_accessory`.
- `wa.Homunculus` fields `id`, `info`, `class`, `name`, `renamed`, `level`, `exp`, `hp`, `sp`, `max_hp`, `max_sp`, the six stats, `skill_points`, `hunger`, `intimacy`, `grade`, `state`, `autofeed`, `game_id`; methods `call`, `vaporize`, `revive`, `evolve`, `mutate`, `morph`, `shuffle`, `add_intimacy`, `set_hunger`, `feed`, `add_exp`, `learn_skill`, `skill_level`, `delete`. `mutate`, `morph` and `shuffle` belong to homunculus-s (v0.3.0); until then they return nil and a reason.
- `wa.Mercenary` fields `id`, `info`, `class`, `name`, `level`, `guild`, `faith`, `calls`, `kills`, `remaining_ms`, `hp`, `sp`, `game_id`; methods `heal`, `add_status`, `dismiss`.
- Events: `pet_tame`, `pet_hatch`, `pet_return`, `pet_feed`, `pet_intimacy`, `pet_evolve`, `pet_lost`, `homunculus_create`, `homunculus_call`, `homunculus_vaporize`, `homunculus_level`, `homunculus_evolve`, `homunculus_lost`, `mercenary_hire`, `mercenary_end`, `mercenary_kill`. Hook `pet_catch_rate` (`rules.override`).
- Mod permission `companions.manage` (approval, not flagged). Staff permissions `players.companions` and `commands.companions`.
- `wa.testing`: `new_player` options `pet`, `homunculus`, `mercenary`; `world:tick_followers(ms)`.

Converter (section 7): item commands `pet`, `bpet`, `getpetinfo`, `mercenary_create`, `mercenary_sc_start`, `mercenary_heal`, `homevolution`, `addhomintimacy`; NPC commands `gethominfo`, `checkhomcall`, `morphembryo`, `hommutate`, `homshuffle`, `getmercinfo`, `mercenary_get_faith`, `mercenary_set_faith`, `mercenary_get_calls`, `mercenary_set_calls`, `mercenary_delete`; helpers `R.petinfo`, `R.hominfo`, `R.homcall`, `R.mercinfo`; constants `PETINFO_*`, `PET_INTIMATE_*`, the mercenary guild constants; the three records from `pet_db.yml`, `homunculus_db.yml` and `exp_homun.yml`, `mercenary_db.yml`.

Protocol (section 8): events `PetInfo`, `PetEggList`, `PetCatch` (`PetCatchStage`), `HomunculusInfo` (`HomunculusState`), `MercenaryInfo`, `FollowerSkills`, `FollowerResult` (`FollowerAction`, `FollowerOutcome`); commands `PetCommand` (`PetAction`), `PetHatch`, `PetCatchTarget`, `PetEvolve`, `HomunculusCommand` (`HomunculusAction`), `MercenaryCommand`, `FollowerOrder` (`move`, `attack`, `skill`, `come`, `hold`), `FollowerLearnSkill`. Field numbers are the next free at build time (events from 121 at the time of writing). `EntityAppear` of kinds `ENTITY_KIND_PET`, `ENTITY_KIND_HOMUNCULUS`, `ENTITY_KIND_MERCENARY`.

Client (section 10), in `client/src/viewer`: `follower.ts`, `pet.ts`, `petEggs.ts`, `petCatch.ts`, `homun.ts`, `homunSkills.ts`, `merc.ts`.

Admin (section 13): `GET /players/{id}` shows followers; `POST /players/{id}/followers`, `DELETE /players/{id}/followers/{kind}` (staff `players.companions`); audit actions `followers.create`, `followers.remove`; GM commands `@makepet`, `@petfriendly`, `@pethungry`, `@makehomun`, `@homlevel`, `@homfriendly`, `@homhungry`, `@homevolution`, `@mercenary`.

## Behavior

- A follower is a zone entity that is not a monster: monster counts, labels, kills, drops and `killmonsterall` never see it.
- A follower travels in the owner's character snapshot through handoffs. It appears beside the owner after a map change or teleport, and is placed beside the owner after 3 s beyond 14 cells (pet, homunculus) or 15 (mercenary).
- Pets are never targets and never enemies. Homunculi and mercenaries are on the owner's side, are targets for monsters, and `hom_setting` bits apply. A mercenary cannot attack the Emperium.
- The AI is wAthena's (rAthena's is the client's): orders replace each other; without an order a follower stays within 3 cells of the owner; a homunculus or mercenary attacks what attacks the owner, then what the owner attacks, then an aggressive monster in view; default skill use rules come from the skill's kind and a record's `use` block. The AI is deterministic given the zone's random stream.
- A follower's saved state is written with the character save. Skill cooldowns, statuses and positions are not saved. A removed record loads as no follower with a warning; an egg stays an item.
- Pets: taming checks and rate as `pet_catch_process_end`; one pet per player; eggs do not stack, a hatched egg cannot be traded, stored in a guild storage, mailed or vended; hunger, intimacy, feeding, owner death, rename, accessory, autofeed and evolution as rAthena; the highest bonus tier whose intimacy is met applies while the pet is hatched, with no accessory check; `support` runs only with `pet_status_support`.
- Homunculi: one per player; creation picks one of the 8 base classes; call, vaporize (HP at least 80%) and revive per the Alchemist skills; growth, exp table, skill points, stat weight, hunger, intimacy, evolution (`homunculus_evo_intimacy_need`, reset to `homunculus_evo_intimacy_reset`) as rAthena; the flag `homunculus` picks renewal's 30 minute call, DEF, MDEF, ASPD, stat weight and the 10 percent exp share, or pre-renewal's.
- Mercenaries: one per player; hiring from `player:hire_mercenary` adds a call; the contract runs only while the owner is online; contract end +1 faith, death -1, every `mercenary_kill_faith` kills +1 faith and a random 5 minute buff; the owner's death removes it without a faith change.
- Rules take rAthena's shipped values in both modes.
- Homunculus S is not in v0.2.0: `morphembryo`, `hommutate` and `homshuffle` convert to calls that are refused with a reason; saves keep `prev_class` so v0.3.0 needs no migration change.
- Performance: if 500 bots with three followers each are over the tick budget, decision 0055 comes before the documented capacity.

## Stub

None. Nodes build on companion-units' real base. Its tests register a test kind that is not shipped.

## Contract tests

Named in proposal 0005 section 16:

- companion-units: `walkState` extraction with `mob_ai.json` goldens unchanged, owner link and map change, `isEnemy` for the three kinds, AI orders and follow, `battle.CalcMisc` per unit, damage log flag and exp shares, `character_followers` save and load (PostgreSQL), e2e of a test follower across a warp and a restart.
- pets: record loading, catch rate and checks, egg creation (taming, drop, shop), hatch, return, hunger and intimacy timeline, feed branches, evolution, bonus tiers and procs, `getpetinfo` recalculation, `pets` rows and reaping (PostgreSQL), converter goldens, e2e tame-hatch-feed-evolve.
- homunculus: record loading, creation, call, vaporize, revive, growth, exp table, stat weight under each flag setting, hunger and intimacy, evolution, skill tree, the 16 regular skills and the 3 Alchemist skills against recordings, exp share in both modes, saves (PostgreSQL), converter goldens, e2e call-level-feed-evolve.
- homunculus-s (v0.3.0): mutation, S growth, the 42 `MH_*` skills against recordings, `homun_s.txt` conversion.
- mercenaries: record loading, hiring, contract timer, faith, calls, kills, deaths, the Emperium rule, the 41 skills against recordings, saves (PostgreSQL), converter goldens, e2e hire-fight-die.
- Recorder files the fork adds, per mode: `follower_status`, `pet_life`, `homunculus_life`, `follower_skill`, `mercenary_life`; extracts `pets.json`, `homunculi.json`, `mercenaries.json`.
- Performance (companion-units): the `companions` wabots scenario at 500 bots.

## Changes

- v1: filled from proposal 0005 and frozen with the maintainer's decisions. Homunculus S, its rules and its converter rows are v0.3.0.
