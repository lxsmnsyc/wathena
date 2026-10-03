# Lucky Clover

A sample mod for rolled item values and random options of proposal 0002, review decision 23 (mods/README.md, "Rolled item values").

- The Clover Charm is an accessory with two rolls in `data/items.yml`: `luck`, uniform from 1 to 5, and `fortune`, a hidden weighted roll with three tiers that the client shows as "Clover fortune". Its static bonuses read them with `{ roll: luck }` and `{ roll: fortune }`.
- Ten Clover Porings live on prt_fild09. They drop the charm with the random options of the group `example.lucky-clover:CLOVER` in `data/random_option_groups.yml`.
- The entry script adds an `item_roll` hook: a charm that a Clover Poring drops never rolls a luck below 2.
- The Clover Smith in Prontera rerolls the luck of a worn charm for 2,000 zeny with `equipped:reroll("luck")`, which needs `items.modify_rolls`.

Run `wactl mod test mods/examples/lucky-clover`. The tests equip charms with `world:equip` and check their rolls, errors, drops, the smith's reroll and the smith's refusal. `TestLuckyClover` in `tools/modkit` checks in Go what a test cannot read: the rolls and random options of dropped charms, and the hook.
