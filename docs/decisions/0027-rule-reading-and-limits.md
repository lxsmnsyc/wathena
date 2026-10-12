# 0027: Rule Reading and Limit Expansion

- Status: proposed.
- Date: 2026-10-12.
- Code: [server/rules](../../server/rules) (`read.go`), [server/modscript](../../server/modscript) (`rules.go`), [server/zone](../../server/zone) (`limits.go`, `storage.go`, `trade.go`), [server/storage](../../server/storage), [server/store](../../server/store) (`storage.go`, migration 00022), [server/social](../../server/social), [tools/convert](../../tools/convert) (`npcexpr.go`), [client](../../client) (`limits.ts`), [e2e](../../e2e) (`limits_test.go`), [mods/examples/bank](../../mods/examples/bank), [mods/examples/classic-casting](../../mods/examples/classic-casting).
- Builds proposal 0002, review decision 32. Builds on decisions [0019](0019-ruleset-flags.md), [0020](0020-parties-and-friends.md), [0021](0021-guilds.md) and [0023](0023-storage-cart-trade.md).

## Decision

### Reading rules

- `wa.rules` has `rules.is(flag)`, `rules.get(name)` and `rules.get_limit(name)`. They need no permission. `rules.limits.<name>` stays the base.
- `rules.is` is true when a flag is at its renewal value or at `on`, as the fields of `rules.Rules`. `skill_rebalance.<skill>` is the flag the skill follows, its own or the base, for a skill of `rules.RebalancedSkills` or `rules.SongSkills`.
- `rules.get` returns a boolean for a yes or no rule, and an integer for the others. `rules.Rules.Values` holds every rule's value. Rules made without values, such as the core rulesets' `rules.Base`, read rAthena's.
- `rules.get_limit` returns two values: the base and the max.
- An unknown name raises an error that lists the valid names.

### Expanding limits

- `player:expand(limit, amount)` changes one of the limits a player holds and returns the new value. It needs the new permission `player.expand_limits`, which needs the owner's approval.
- The limits a player holds are `inventory_slots`, `storage_slots` and `cart_slots` (`rules.ExpandableLimits`). The amount is -10,000 to 10,000. The value stays within the limit's base and max.
- `inventory_slots` saves in `characters.inventory_size`, as before. The character joins the next commit.
- `storage_slots` belongs to the account's storage and `cart_slots` to the cart. Each `storage.Storage` holds its size, as the account's storage holds the account's currencies. The size commits with the storage and saves in `storage_sizes` (migration 00022). A size at the base has no row, so a ruleset that raises the base raises it too.
- Every read clamps a saved value to the limit's range. A ruleset that lowers the max applies at the next read. Lowering deletes nothing: slots past the size keep their items and take no new one, as decision 0023.
- The player gets `PlayerLimits` with its value of each limit at login and after each change. The client sizes the inventory window, the account's storage window and the cart from it.

### Joins on create

- A player who makes a party from the client gets `party_join` with the reason `"create"`. A player who makes a guild from the client gets `guild_join` with `"create"`. `parties.create` and `guilds.create` keep `"script"`.

### Block lists and trades

- The ruleset rule `block_list_refuses_trade`, yes by default, decides whether a player who blocks the requester refuses a trade request at once. No matches rAthena, which does not check its ignore list for trades.

### Test helpers

- The helpers of `wa.testing` that phase 5 added are stable: `world:chat`, `world:get_chat`, `world:trade`, `world:close_storage`, `world:buy`, `world:get_mails` and `world:vend`, with their classes. The definitions keep the `Provisional` field for later API.

### Converter

- `basicskillcheck()` reads `rules.get("basic_skill_check")`. `R.basic_skill_check` is gone.
- `checkre(1)` to `checkre(5)` read the flag of their define with `rules.is`: `cast_time` (`RENEWAL_CAST`), `level_penalty_drop` (`RENEWAL_DROP`), `level_penalty_exp` (`RENEWAL_EXP`), `level_damage_mod` (`RENEWAL_LVDMG`) and `aspd` (`RENEWAL_ASPD`). Before, every `checkre` gave the converter's mode. rAthena's NPC scripts use `checkre(3)` 141 times, many of them to pick quest exp.
- `checkre(0)` is `RENEWAL`, the base, which a ruleset cannot mix. It stays the converter's mode.
- The coverage reports do not change: both commands converted before, with rAthena's defaults.

## Inventory expansion items

The reference fork of rAthena has no `expandinventory`, `expandinventory_ack` or `getinventorysize` script commands. Its inventory expansion is a client window: `clif_parse_inventory_expansion_request` and `clif_parse_inventory_expansion_confirm` take one of the vouchers 25791, 25792 or 25793 and add 10 slots, up to `MAX_INVENTORY`. The vouchers are `Etc` items without scripts. So no converted script and no core item calls `player:expand`. A mod sells slots from an NPC, as [examples/bank](../../mods/examples/bank) does.

## Tests

- `server/modscript`: `TestRulesRead`, `TestRulesReadDefaults` and `TestPlayerExpand` check the calls, their errors and the permission.
- `server/zone`: `TestExpandLimits` checks the ranges, the commits, `PlayerLimits`, and an account's two characters sharing the storage's size. `TestRestoredInventoryClamped` checks a saved size outside the range. `TestTradeBlockListRule` checks the rule. `TestKafraBasicSkillRule` runs core's converted Kafra with `basic_skill_check` off.
- `server/social`: the create tests check the `"create"` events.
- `server/store`: `TestStorageSizes` saves and loads the sizes, and deletes the row of a size at the base.
- `tools/convert`: `TestRuleReadsConvert`.
- `wactl mod test`: the Bank sells inventory slots and reads limits and rules. Classic Casting reads its flags.
- `e2e`: `TestExpandLimits`, `TestExpandLimitsInTheDatabase` (a new login and a restart with PostgreSQL) and `TestTradeBlockListRule`.

## Other choices

- A limit the player holds as a table of name and value. Three limits have three places to save already: the character's row, the account's storage and the cart. A table would add a fourth.
- Saving an expansion as an amount over the base. A ruleset that changes the base would then change every player's slots. A saved size, clamped on read, keeps what the player has.
- `rules.get_limit` returning a table. Two values read well in Lua, `local base, max = rules.get_limit(name)`, and match `rules.limits`, which holds plain numbers.

## Left

- A client window for rAthena's inventory expansion vouchers.
- Expanding the other limits a player holds, such as `friends`, `block_list` and `mailbox`, and extra storages.
- Reading a player's value of a limit without `player.expand_limits`.
- The converter review of review decision 32: the script commands that need new API.
