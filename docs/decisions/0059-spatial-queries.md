# 0059: Spatial Queries

- Status: proposed. Not built.
- Date: 2026-10-20.
- Code: [server/zone](../../server/zone) (`query.go`, `entities.go`, `zone.go`, `skillunits.go`, `skills_sage.go`, `skills_bard.go`, `mobai.go`, `mobskills.go`, `combat.go`, `loot.go`, `party.go`, `guild.go`, `cast.go`, `skilltimers.go`, `regen.go`, `touch.go`), [shared/path](../../shared/path) (`path.go`), [server/modscript](../../server/modscript) (`runtime.go`, `items.go`).
- Builds on decision [0050](0050-load-tests.md). Pairs with decision [0055](0055-shared-view-events.md), which handles view events.
- Lua API: none. Protocol: none. Settings: none.

## Context

The findings below come from reading the code on 2026-10-20. None was measured.

The zone keeps an 8 × 8 block index of entities, rebuilt each tick (`entities.go`). `viewEvents` uses it. Almost nothing else does. Most other spatial questions scan every entity in the zone, and many sort the result. Cost then grows with entities × the number of questions, and in towns and big fights both are large.

These are the weak points, most severe first.

1. **Every cell step re-checks every skill unit.** `skillUnitMove` (`skillunits.go`) returns early only when the zone has no skill unit groups. Otherwise each step of each entity runs `unitStatusRefresh`, `fieldStatusRefresh` (`skills_sage.go`), `spiderWebOnPlace` and `songRefresh` (`skills_bard.go`). Those loop over all groups and units, sort groups up to 3 times, and allocate.
2. **`findEntities` scans every entity and sorts.** `query.go` walks the whole entity set for each query and sorts the matches by ID. About 40 call sites use it, including `unitTimer`, which queries once per skill unit every 100 ms.
3. **Monster target search scans every player.** For each monster on each think, `monsterActive` and `enemyCandidates` (`mobai.go`) scan all players, and `enemyCandidates` sorts. `mobActiveSearch` then runs a full A* per candidate.
4. **Each hit by a player in a party takes the world-wide party lock.** `strike` calls `syncBattlePlayer` (`combat.go`), which reaches `ShareByID` in `server/social/party.go` under the registry lock, and `slotsHere` (`loot.go`) builds a map of all players in the zone.
5. **Party and guild updates sort all players inside a loop over changed players.** `partyUpdates` (`party.go`) and `guildUpdates` (`guild.go`) call `sortedPlayers` for each changed player.
6. **About 14 full entity walks per tick.** Each filters by kind: players, casters, pickers, units, items. Sites include `zone.go`, `combat.go`, `loot.go`, `cast.go`, `regen.go` and `skillhooks.go`. `naturalHeal` (`regen.go`) walks all entities instead of `z.players`.
7. **Deaths, drops and mob skill conditions scan every entity.** `die` and `killSlaves` (`combat.go`), `freeNear` and `cellFree` (`loot.go`), and the mob skill conditions in `mobskills.go` each walk everything. An area skill that kills many monsters repeats this for each.
8. **Finding an entity by net ID is a linear scan.** Attack, pickup, skill, NPC click, trade and vending commands scan the player's known entities (`zone.go`, `loot.go`, `cast.go`, `npcs.go`, `trade.go`, `vending.go`).
9. **A* allocates its node table on each search.** `search` in `shared/path/path.go` holds 1,024 nodes. The open set's `update` is a linear scan. Searches run for each walk command, each chase, each reach check and each target candidate.
10. **Timers are found by scanning.** `advanceTimerSkills` (`skilltimers.go`) finds the next timer by a linear scan with `slices.Delete`, twice per tick. The Lua runtime's `Advance` (`server/modscript/runtime.go`) scans every timer each tick and sorts the due ones.
11. **Every step scans every touch NPC.** `touchAreas` (`touch.go`).
12. **Every status change re-runs scripted bonuses.** A status start or end recomputes stats, and `equipment` calls `Values` for every scripted bonus in every worn item and card (`equipment.go`, `server/modscript/items.go`). Each `when` and computed value is a call into Lua.

## Decision

Fix them in this order. Each step must keep `scripts/check.sh` passing, including the parity tests against rAthena. rAthena breaks ties in its own order, and the results must not change.

### 1. Queries through the block index

- `findEntities` reads only the blocks that its area touches. A query with no area keeps scanning all entities.
- Keep the block index current while entities move in a tick, not only at the start. rAthena does this in `map_moveblock`.
- Keep the sort by ID, because rAthena's order decides which targets a skill hits first. Sort only the matches, which are few.
- Move `monsterActive`, `enemyCandidates`, `freeNear`, `cellFree`, the mob skill conditions, `mobLinkSearch` and `mobLootSearch` to the same queries.
- `enemyCandidates` runs A* only on the nearest candidates, in distance order, and stops at the first that it can reach.

### 2. Lists by kind

- Keep lists of players, monsters, casters, pickers, skill units and ground items, updated when an entity is added, removed or changes state.
- The tick phases walk the list they need.
- Ground items get a heap by expiry time.
- Each master keeps the list of its slaves.

### 3. Skill units by cell

- Keep a map from cell to the skill units on it, updated when a unit is placed, moved or removed.
- `skillUnitMove` looks up the old and new cells only.
- Keep the sorted list of groups and mark it dirty when a group is added or removed.
- Touch areas use the same cell map.

### 4. Party and guild members in the zone

- Keep, per zone, a map from party ID and guild ID to their members in the zone. Update it on join, leave and party or guild change.
- `partyUpdates` and `guildUpdates` use it.
- Hits read the party's share settings from the zone's copy. The zone gets a new copy when the party changes, so a hit no longer takes the registry lock.

### 5. Net ID map

- `netIDs` keeps a map from ID back to the entity, next to the slots it has now.

### 6. Search and timers

- Each zone keeps one A* `search`, reused with a generation number so it is not cleared each time. A zone runs on one goroutine, so one is enough.
- Replace the open set's linear `update` with a heap update by index.
- Timer skills and Lua timers use a heap by due time, with the ID as tie breaker so the order stays the same.

### 7. Scripted bonus cache

- Cache the result of each scripted bonus value and `when` condition.
- Record which stats and statuses each one read during the call. Run it again only when one of those changed.
- Bonuses whose script reads something the cache cannot track, such as random values, are not cached.

## Breaks

- Nothing for clients, mods or worlds.
- Results must not change. The parity tests and `e2e` tests are the check. Any change in which target is chosen is a bug in this decision's code.

## Alternatives

- **A quadtree or k-d tree.** Map sizes are fixed and small (at most 1,024 cells a side), so a grid of blocks is simpler and as fast.
- **Run AI for fewer monsters.** rAthena runs hard AI only for monsters near players. This is worth doing too, but it does not fix the scans in combat and skills.
- **Cache query results per tick.** Most queries have different areas and filters, so few results would be reused.

## How to check it worked

- Add benchmarks in `server/zone` for a town with 500 players and 50 skill units, and a field with 2,000 monsters and 100 players. Record ns per tick before and after each step.
- The `wabots` scenarios from decision 0050, especially Fight and Town at 500 and 1,000.
- A CPU profile of the same runs: `findEntities`, `skillUnitMove`, `enemyCandidates` and `path.Search` drop out of the top entries.
- All parity tests pass with no change to their reference data.
