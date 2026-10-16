# Making a mod

A mod adds or changes game content without changes to the server (strategy section 6). This guide covers what the mod loader and tools support today, at the end of phase 4. The scripting API is [proposal 0002](../docs/proposals/0002-lua-api.md), at version 1.0. It follows semantic versioning (strategy section 6.12): breaking changes only come with a new major version, and new API starts under `wa.experimental`.

- [core](core) is the base game as a mod. It is built into the server. Its files are rAthena's item scripts, NPCs and shops, which the server converts when it starts ("Item scripts" and "Core's NPCs" below).
- [examples/greeter](examples/greeter) is a small sample: an item, an NPC with a conversation and saved variables, a staff-only NPC, a shop, a script that greets players, and tests.
- [examples/halloween](examples/halloween) is the phase 3 exit check: a hat, a witch who gives it once, Pumpkin Porings with drops, a night that doubles their drops through a `drop_rate` hook, and the Pumpkin King, a boss that a world event brings every evening.
- [examples/verve](examples/verve) adds a status effect: `statuses.define`, a `status_tick` handler, and a tonic that starts the status, with tests.
- [examples/lucky-clover](examples/lucky-clover) adds a charm with rolled values, a monster that drops it with random options, an `item_roll` hook, and a smith who rerolls it ("Rolled item values" below).
- [examples/desperado](examples/desperado) is the phase 4 exit check: a new job, the Desperado, with a new skill, Quick Draw, a hat only it wears, a mentor who changes jobs, and a `job_change` handler ("Jobs" below).
- [examples/classic-casting](examples/classic-casting) is a ruleset mod: a renewal world with pre-renewal cast time, element table, cooldowns and Mystical Amplification ("Rulesets" below).
- [examples/party-hall](examples/party-hall) makes parties with `wa.parties`, greets members on the party events, and reads `rules.limits` ("Parties and friends" below).
- [examples/guild-hall](examples/guild-hall) makes guilds with `wa.guilds`, follows the guild events and reads the guild limits ("Guilds" below).
- [examples/bank](examples/bank) has a banker who opens the account's storage and an extra storage, a bound Bank Card, and a `trade` hook and event ("Storage, cart and trade" below).
- [examples/lantern](examples/lantern) has art of its own: item icons, a hat's and a shield's worn sprites, a companion's sprite, a monster sprite with sounds, an NPC sprite, the Lantern Keeper job with body sprites and its skill Lantern Flare with an icon and an effect, and a client data table ("Art" below). It is the phase 6 exit check's mod run.
- [examples/market-town](examples/market-town) is for procedural worlds: a town template with a lamp shop, a dungeon style, and NPCs that stand at named places instead of a map and a cell ("Places" below).
- [examples/home-village](examples/home-village) is for procedural worlds: a home map in the map folder format, where new characters start, and a naming style for grassland towns ("Home maps" and "Naming styles" below).
- [examples/hunting-board](examples/hunting-board) has a hunt quest as a record, and the Hunt Master, who gives it, shows a quest icon, counts the kills left and pays for it, in classic and procedural worlds ("Quests" below).
- `wactl mod new` makes a new mod that passes `wactl mod validate` and `wactl mod test` ("Tools" below).

Server owners install mods in the folder that `mods.dir` names. [server/README.md](../server/README.md), "Mods", covers the server side.

## Folders

A mod is a folder, or a `.zip` file with the same files at its root or in its one top folder.

```text
greeter/
  mod.yaml              the manifest
  data/
    items.yml           item overlay
    monsters.yml        monster overlay
    npcs.yml            NPC placements and shops
    events.yml          world events
    town_templates.yml  town templates of procedural worlds
    dungeon_styles.yml  dungeon styles of procedural worlds
    naming_styles.yml   naming styles of town names
  maps/
    village/            a map folder, such as a home map
  locales/
    en.yml              text for players, read with wa.text
  scripts/
    main.lua            an entry script that mod.yaml lists
    npc/greeter.lua     an NPC's entry script, which npcs.yml names
    lib/messages.lua    a helper module
  tests/
    greeter_test.lua    tests that wactl mod test runs
  art/                  the art's sources ("Art" below)
  assets/               the art's build, which wactl mod build-art makes
  client/
    skills.yml          client data tables over core's ("Art" below)
  .luarc.json           editor settings for the Lua Language Server
```

Names that start with `.`, such as `.luarc.json`, `.wathena` and `.git`, are not part of the mod. The server ignores them, and `wactl mod pack` leaves them out.

## The manifest

```yaml
id: example.greeter
name: Greeter
version: 0.1.0
api: 1.0
description: Greets players when they log in.
authors:
  - name: Example Author
license: MIT
depends:
  - id: core
    version: ">=0.1"
load_after:
  - example.seasonal-lib
supports:
  rulesets:
    base: [renewal, pre_renewal]
    flags:
      skill_cooldown: on
permissions:
  - player.give_items
scripts:
  - scripts/main.lua
```

| Field | Required | Meaning |
|---|---|---|
| `id` | yes | Lowercase words of letters, digits, `_` and `-`, separated by dots. `core`, `core.*`, `wa` and `wa.*` are reserved. |
| `name` | yes | The name people see. |
| `version` | yes | A semantic version, such as `1.2.0`. |
| `api` | yes | The API version the mod is written for: `1.0`, or a range that includes it, such as `">=1.0 <2.0"`. A range without 1.0 is refused. `experimental`, the value before API 1.0, is refused too. |
| `description`, `authors`, `license` | no | Shown to server owners. Each author needs a `name`. |
| `depends` | no | Mods that must be installed and load first. `version` is a range: `>=1.0 <2.0`, `^1.2`, `~1.2`, `1.2.3`, and alternatives with `||`. |
| `load_after` | no | Mods to load after when they are installed. A missing one is ignored. |
| `supports.rulesets` | no | The ruleset bases the mod works with, and values flags must have. A flag can list several values. Without it, the mod loads on any ruleset. |
| `provides.ruleset` | no | Makes the mod a ruleset mod (below). |
| `permissions` | no | What the mod needs (below). |
| `scripts` | no | Entry scripts that run once per zone, in the mod's context. Paths are under `scripts/`. |

Unknown fields are errors, so a typo does not pass. Every error names the file and the field.

## Load order

Mods load after `core`, after their `depends`, and after the mods in `load_after` that are installed. Among mods that could load next, the lowest ID goes first, so the order is the same on every start. A cycle is an error that shows it, such as `example.a -> example.b -> example.a`.

## Rulesets

A world has one ruleset: a base, `renewal` or `pre_renewal`, and a value for each flag of strategy section 3.6. `core.ruleset.renewal` and `core.ruleset.pre_renewal` set every flag to their base's value.

A ruleset mod starts from another ruleset mod and changes flags:

```yaml
id: example.classic-plus
name: Classic Plus
version: 1.0.0
api: 1.0
provides:
  ruleset:
    extends: core.ruleset.pre_renewal
    flags:
      cast_time: renewal
      level_penalty_drop: on
```

- A ruleset mod can extend another ruleset mod. Flags apply from the core ruleset up.
- `fourth_job_traits: on` needs the renewal base.
- A content mod whose `supports.rulesets` does not match is refused, with the reason.

What each flag changes ([decision 0019](../docs/decisions/0019-ruleset-flags.md) has the details):

| Flag | Values | What it changes |
|---|---|---|
| `cast_time` | `renewal`, `pre_renewal` | Fixed and variable cast time, or cast time by DEX only, and how cast bonuses count. On a pre-renewal base, renewal's cast time makes 20% of each skill's cast time fixed. |
| `skill_cooldown` | `on`, `off` | Renewal's skill cooldowns, or pre-renewal's. Pre-renewal still has cooldowns for some skills, such as Provoke. |
| `aspd` | `renewal`, `pre_renewal` | The ASPD formula, shield penalty and bonus stacking. |
| `stat_point_cost` | `renewal`, `pre_renewal` | The status point cost of raising a stat. The two agree below 100. |
| `level_penalty_exp`, `level_penalty_drop` | `on`, `off` | Exp and drop rates by the level gap between player and monster. |
| `level_damage_mod` | `on`, `off` | The renewal versions of skills deal more above base level 99. |
| `element_table` | `renewal`, `pre_renewal` | The element table and the field bonuses of Volcano, Deluge and Violent Gale. |
| `size_table` | `renewal`, `pre_renewal` | The size penalty of knuckles and whips. |
| `status_resist` | `renewal`, `pre_renewal` | Status effect chance and duration against the target's stats. |
| `heal_formula` | `renewal`, `pre_renewal` | Heal, Sanctuary and the other heal skills, and the parts of item heals that differ. |
| `boss_protection` | `renewal`, `pre_renewal` | Whether Ankle Snare and Skid Trap catch bosses, and Blade Stop on bosses. |
| `ensemble_songs` | `renewal`, `pre_renewal` | Songs and dances as buffs on the performer's party, or as ground areas, with their values, ensemble fatigue and Longing for Freedom. |
| `homunculus` | `renewal`, `pre_renewal` | Nothing yet: homunculi do not exist. |
| `busy_restrictions` | `on`, `off` | The busy message for items, skills and NPC clicks while talking, and no NPC clicks while casting. |
| `fourth_job_traits` | `on`, `off` | Trait stats, RES and MRES. |
| `item_rolls` | `on`, `off` | Random values for items with roll ranges ("Rolled item values" below). |
| `skill_rebalance.<skill>` | `renewal`, `pre_renewal` | One skill's own formulas and effects, and its status's. `<skill>` is a `skill_db` name, such as `HW_MAGICPOWER`. |

- The songs and the skills with a `skill_rebalance` flag take their `skill_db` record from the flag's mode, such as pre-renewal's ground area for Basilica. A mod's `data/skills.yml` applies over it.
- `skill_rebalance` works for the 94 skills of `rules.RebalancedSkills` in `server/rules`. Another core skill loads with a warning, and only its record changes. An unknown skill or a song is an error: songs follow `ensemble_songs`.
- `wactl mod test` runs a ruleset mod's tests in worlds whose ruleset is the mod. Use `--mode pre-renewal` for a mod with the pre-renewal base.

### Limits and rules

A ruleset mod also sets the limits that rAthena compiles in, and the values of rAthena's conf files that rulesets own (proposal 0002, review decisions 27 and 29):

```yaml
provides:
  ruleset:
    extends: core.ruleset.renewal
    limits:
      party_members: 20
      friends: 80
      inventory_slots: { base: 150, max: 300 }
    rules:
      party_share_level: 30
      party_even_share_bonus: 10
```

Each limit has a range:

- `base` is what a player or guild starts with. `max` is where expansions stop, such as Guild Extension or `player:expand` ("Rules and limits" below).
- A plain number sets the base. A max left out is the default max, or the base when the base is higher.
- The engine has a ceiling per limit. A base or a max above it, or a base above the max, fails the load. The ceilings come from the benchmark of [decision 0023](../docs/decisions/0023-storage-cart-trade.md).

| Limit | rAthena's base | Default max | Ceiling | What it limits |
|---|---|---|---|---|
| `party_members` | 12 (`MAX_PARTY`) | 12 | 256 | Members of a party. |
| `friends` | 40 (`MAX_FRIENDS`) | 40 | 1000 | Names on a friend list. |
| `block_list` | 20 (`MAX_IGNORE_LIST`) | 20 | 1000 | Names on a block list. |
| `guild_members` | 16 | 76 (`MAX_GUILD`) | 512 | Members of a guild. Guild Extension adds 6 per level, up to the max (`guild_calcinfo`). |
| `guild_positions` | 20 (`MAX_GUILDPOSITION`) | 20 | 40 | Positions of a guild. |
| `guild_allies` | 3 (`max_guild_alliance`) | 3 | 10 | Allied guilds of a guild. |
| `guild_enemies` | 3 (`max_guild_alliance`) | 3 | 10 | Enemy guilds of a guild. |
| `guild_storage` | 600 (`MAX_GUILD_STORAGE`) | 600 | 10000 | Slots of a guild storage without renewal's Guild Storage skill. The skill gives 100 and 100 per level, up to the max. |
| `inventory_slots` | 100 (`INVENTORY_BASE_SIZE`) | 200 (`MAX_INVENTORY`) | 1000 | Inventory slots of a new character. |
| `storage_slots` | 600 (`MAX_STORAGE`) | 600 | 10000 | Slots of an account's storage. |
| `cart_slots` | 100 (`MAX_CART`) | 100 | 1000 | Slots of a cart. |
| `mail_items` | 5 (`MAIL_MAX_ITEM`) | 5 | 20 | Item stacks of one mail. |
| `mailbox` | 100 | 100 | 1000 | Mails of a mailbox. |

| Rule | rAthena's value | Values | What it changes |
|---|---|---|---|
| `party_share_level` | 15 | 0 to 1000 | Even exp share needs the online members' base levels within this range. |
| `party_even_share_bonus` | 0 | 0 to 1000 | Exp bonus in percent for each member after the first that even exp share counts. |
| `party_item_share_type` | 0 | 0 to 3 | Bit 1: even item share only for items with a first looter. Bit 2: members in turn, not at random. |
| `change_party_leader_samemap` | yes | yes, no | A new leader must be on the leader's map. |
| `block_account_in_same_party` | yes | yes, no | Two characters of one account cannot be in one party. |
| `friend_auto_add` | yes | yes, no | An accepted friend request puts each player on the other's list. |
| `guild_emperium_check` | yes | yes, no | Making a guild from the client takes an Emperium. |
| `guild_exp_limit` | 50 | 0 to 99 | The highest exp tax of a guild position. |
| `emergency_call` | 11 | 0 to 31 | Where Emergency Call works. Bits: 1 outside sieges, 2 in sieges, 4 outside GvG maps, 8 on GvG maps, 16 not on nowarpto maps. |
| `guild_skill_relog_delay` | 300000 | 0 to 86400000 | Milliseconds a master's guild skills wait after a login or becoming master, in pre-renewal (the `skill_cooldown` flag). |
| `guild_leaderchange_delay` | 1440 | 0 to 1000000000 | The wait between master changes. rAthena compares it with seconds, though its comment says minutes. |
| `guild_alliance_onlygm` | no | yes, no | Only a guild master may accept an alliance. |
| `max_cart_weight` | 8000 | 0 to 1000000000 | The weight a cart holds, in tenths, as item weights (`max_cart_weight`). |
| `basic_skill_check` | yes | yes, no | Trading needs Basic Skill level 1, and core's Kafras ask for level 6 before storage (`basic_skill_check`). |
| `block_list_refuses_trade` | yes | yes, no | A player who blocks the requester refuses a trade request at once (review decision 32). rAthena does not check its ignore list for trades, so no matches rAthena. |
| `vending_tax` | 500 | 0 to 10000 | The vending tax in hundredths of a percent. |
| `vending_tax_min` | 100000000 | 0 to 2147483647 | The smallest purchase the vending tax applies to. |
| `vending_max_value` | 1000000000 | 1 to 2147483647 | The highest price of a vending line. |
| `vending_over_max` | yes | yes, no | A shop may open when its sales would pass the zeny cap. |
| `min_npc_vendchat_distance` | 3 | 0 to 30 | How many cells a vending shop or a buying store keeps from NPCs. |
| `autotrade_restore` | yes | yes, no | Autotrade stores open again after a restart (`feature.autotrade`). |
| `searchstore_querydelay` | 10 | 0 to 3600 | Seconds between two store searches. |
| `searchstore_maxresults` | 30 | 1 to 1000 | The most results of one store search. |
| `mail_zeny_fee` | 2 | 0 to 100 | The fee of mailed zeny, in percent. |
| `mail_attachment_price` | 2500 | 0 to 2147483647 | The zeny for each attached item. |
| `mail_attachment_weight` | 2000 | 0 to 1000000000 | The most weight of a mail's items, in whole weight units, or 0 for no limit. |
| `mail_daily_count` | 100 | 0 to 1000000 | The most mails a character sends a day, or 0 for no limit. |
| `mail_delay` | 1000 | 1000 to 3600000 | Milliseconds between two mails of a character. |
| `mail_return_days` | 15 | 0 to 3650 | Days before a mail goes back to its sender, or 0 for never (char_athena.conf). |
| `mail_delete_days` | 15 | 0 to 3650 | Days before a mail that came back, or a script's mail, is deleted, or 0 for never. |
| `mail_return_empty` | yes | yes, no | A mail without attachments goes back to its sender too. Off, it is deleted. |

- A value outside its bounds, or a name that is not a limit or a rule, fails the mod's load.
- A ruleset mod that does not set one keeps what it extends. The core rulesets have rAthena's values.
- Lowering a limit deletes nothing. A list over the limit keeps its names and cannot grow.
- The client gets every limit's base and max at login, and the player's own values of the limits it holds. Lua reads the base as `rules.limits.<name>`, the base and max with `rules.get_limit(name)`, and the rules with `rules.get(name)` ("Rules and limits" below).
- Phase 5 step 5 added the guild limits and rules, step 6 the ranges, the ceilings, and the inventory, storage and cart limits, and step 7 the mail limits and the vending, store search and mail rules. Card slots have the ceiling 16 (`slots` of `data/items.yml`) and no limit yet.
- [examples/classic-casting](examples/classic-casting) is a sample ruleset mod: a renewal world with pre-renewal casting.

## Permissions

| Permission | Default |
|---|---|
| `game.read`, `npc.create`, `event.listen`, `storage.mod` | Granted to every mod. |
| `player.give_items`, `player.give_zeny`, `player.give_exp`, `player.warp`, `player.status`, `player.change_job`, `world.announce`, `events.schedule` | Needs the owner's approval. |
| `party.manage` | Needs the owner's approval. It makes parties and changes their members, leader and options (review decision 27). |
| `guild.manage` | Needs the owner's approval. It makes guilds and changes their members, master, notice and exp, and breaks them up (review decision 28). |
| `mail.send`, `currencies.modify`, `shops.modify` | Needs the owner's approval. They send mail with items and zeny, change another mod's currencies, core's too, and change shops (review decision 30). |
| `events.control` | Needs the owner's approval. It starts and stops world events at any time with `wa.world_events` (review decision 31). |
| `player.expand_limits` | Needs the owner's approval. It raises and lowers a player's inventory, cart and storage slots with `player:expand`, within the ruleset's limits (review decision 32). |
| `spawns.modify` | Needs the owner's approval. It adds `spawn_table` hooks, which change the monsters a new procedural world spawns (review decision 44). |
| `rules.override`, `admin.commands`, `net.http`, `items.modify_rolls`, `player.sanction` | Needs approval, and is logged as a warning at each start. `player.sanction` (mute and jail players) is flagged for review in proposal 0002, review decision 26. |

- A permission that needs approval must be in `permissions`, and the owner approves it in the server config under `mods.approve`.
- A mod with a permission that is not approved does not load, and the server does not start. The error names the permissions.
- Calls check permissions when they run. A call without its permission raises an error that names it, such as `player:give_item needs the permission player.give_items`.

## Data overlays

Files in `data/` add records or change fields of existing ones. Each file is a list of records with a `name`.

- A new record is named with the mod's ID: `example.greeter:Welcome_Cookie`.
- A name without a prefix is a core record. A field set there changes the core record. A core name that does not exist is an error, so a typo does not make a new record.
- To change another mod's record, write its full name, and load after that mod.
- A mod cannot delete a record.
- When two mods set the same field, the later mod wins and the server logs a warning. With `mods.strict`, it is an error.
- Fields are changed whole: setting `heal` replaces all of `heal`.

### Items: data/items.yml

```yaml
- name: example.greeter:Welcome_Cookie
  type: Healing
  weight: 10
  stackable: true
  script: scripts/items/welcome_cookie.lua
- name: Red_Potion
  weight: 50
- name: Poring_Card
  bonuses: [[luk, 3], [perfect_dodge, 1]]
```

- Names are AegisNames, such as `Red_Potion`.
- Fields are those of the item extract: `type`, `weight` (in tenths), `stackable`, `stack_limit`, `unique_id`, `no_consume`, `no_drop`, `delay_ms`, `delay_status`, `min_level`, `max_level`, `gender`, and `job_restricted`, and `display_name`, `buy`, `sell` and `no_sell`: the name `items.name` gives, the prices of NPC shops in zeny, and whether shops buy the item. `name` is the record's key, so the name players see is `display_name`, as for NPCs.
- `script` is the item's entry script, a path under the mod's `scripts/`. It belongs to the mod that sets it.
- `description` is the item's description in the client, text of up to 4,096 bytes ("Names in the client" below).
- `bonuses` are static bonuses of an equipment item ("Static bonuses" below).
- `rolls` are values each new copy rolls, at most 8 ("Rolled item values" below).
- Equipment has the fields of the equipment extract, as rAthena's item_db names them:
  - `location`: the positions the item is worn in, as a list such as `[head_top, head_middle]`. The names are the slot names of `player:get_equipped`, and `both_hand` and `both_accessory`. A card's location is where it can be put.
  - `subtype`: the weapon type of a weapon, or the ammunition type of ammunition, as rAthena's `W_` and `AMMO_` numbers.
  - `atk`, `matk` (renewal only), `def`, `range`, `slots` (up to 16; the world uses at most 4, rAthena's `MAX_SLOTS`), `weapon_level`, `armor_level` and `refineable`.
  - `view`: the sprite the item shows on a character, as item_db's `View`. An item with a `view_sprite` gets a view of its own ("Art" below).
  - `jobs` and `classes`: who may wear it. `jobs` is a list of job names, such as `[knight, example.desperado:Desperado]`, or three decimal strings of rAthena's job bit masks. `classes` holds the `ITEMJ_` bits. Leave them out for everyone.
    - A core job's name sets the bits of rAthena's `Jobs` for its class, as item_db does: `knight` also lets a Lord Knight or a Rune Knight wear the item, unless `classes` leaves them out.
    - A mod job's name lets that job wear the item, whatever its class.
    - A job with `equips_as` follows the masks of that job ("Jobs" below).
  - `no_equip`: the map types where its script does not run, as `item_noequip.txt`: 1 normal maps, 2 PvP, 4 GvG, 8 battlegrounds.
- `restrictions` lists where the item may not go, from rAthena's `item_trade`: `no_drop`, `no_trade`, `no_sell`, `no_cart`, `no_storage`, `no_guild_storage` and `no_mail`. A record that sets it sets the whole list. `no_drop` and `no_sell` stay in step with it. A card's restrictions also hold for the item it is in, as `itemdb_isrestricted`.
- `unsupported` says what the item's script needs that the server does not have yet. A usable item with it is refused, and the player sees the text. Core sets it on converted items.
- `icon`, `image`, `drop_sprite` and `view_sprite` name the item's art ("Art" below).
- `id` and `aegis_name` cannot be set.
- A new item is an `Etc` item until `type` says otherwise.

### Storages: data/storages.yml

Extra storages, as rAthena's premium storages of `inter_server.yml` (proposal 0002, review decision 29). Core has none: the account's storage is not a record.

```yaml
- name: vault
  display_name: Bank Vault
  slots: 50
  owner: account
```

- A record becomes `<mod ID>:<name>`, such as `example.bank:vault`, with an ID from 10,000.
- `display_name` is the name the storage window shows, 1 to 24 characters. It is the name without the mod prefix when left out.
- `slots` is 1 to the ceiling of `storage_slots`.
- `owner` is `account`, which the account's characters share, or `character`. It is `account` when left out.

### Currencies: data/currencies.yml

Currencies, such as rAthena's cash points and Kafra points (proposal 0002, review decision 30). Core has `cash` (`#CASHPOINTS`) and `kafra` (`#KAFRAPOINTS`), on the account.

```yaml
- name: tokens
  display_name: Arena Tokens
  owner: character
  max: 1000
```

- A record becomes `<mod ID>:<name>`, such as `example.arena:tokens`, with an ID from 10,000.
- `display_name` is the name players see, 1 to 24 characters. It is the name without the mod prefix when left out.
- `owner` is `account`, which the account's characters share, or `character`. It is `account` when left out.
- `max` is the most a player holds, 1 to 2,147,483,647, the default. A give over it is lost.

### NPCs: data/npcs.yml

```yaml
- name: Greeter#prontera
  map: prontera
  x: 158
  y: 200
  direction: south
  sprite: 4_F_KAFRA1
```

- A name without a prefix is the mod's own NPC: `Greeter#prontera` in `example.greeter` is `example.greeter:Greeter#prontera`. Core's NPCs are named `core:<rAthena's unique name>`, such as `core:kaf_prontera`.
- Players see the name up to `#`, or `display_name` when it is set. An empty `display_name` shows no name.
- `map`, `x`, `y` and `sprite` are required, or `place` in place of `map`, `x` and `y` (below). `sprite` is an NPC sprite name, such as `4_F_KAFRA1`, its number, or a sprite of art, such as `mod/example.lantern/sprite/npc/Lamplighter` ("Art" below).
- `place: {scope, tag}` puts the NPC at every named place of a generated map that is in the scope and has the tag, such as `{scope: "town:any", tag: blacksmith}` (proposal 0002, review decision 42). The scopes are those of `places.find` ("Places" below). The NPC appears once the server has generated its maps, at each place's cell, with the record's other fields. A shop at several places is one shop, with one stock. A classic world has no places, so the NPC does not appear there, and neither does it in a world where no place matches.
- `direction` is `south` (the default), `southwest`, `west`, `northwest`, `north`, `northeast`, `east` or `southeast`.
- `script` names the NPC's entry script. It must exist. Its setup function runs once for each NPC that names it ("NPC scripts" below).
- `shop` makes the NPC a shop: a list of `{item, price}`, or a table with `items`, `buys_from_players` and `discount` (review decision 30). A price left out, or -1, is the item's `buy` price. A click on a shop without a click handler opens the shop.
- An item of the table form may have a `cost` in place of its price: `zeny`, `items` (a list of `{item, amount}`) and `currency` (a map of currency names to amounts), each optional, at most 5 items and currencies together. This covers rAthena's itemshop, pointshop, cashshop and barter shops.
- `stock` limits an item for every player, as a marketshop's stock. `restock` fills it again: `daily`, `weekly` (from Monday), or a time such as `30m`, `6h` or `2d` after the last fill. The stock is saved.
- `buys_from_players: false` hides the sell tab. It is on by default.
- `discount: false` keeps Discount and Compulsion Discount from lowering the shop's zeny prices, as rAthena's `shop` line with `no`. It is on by default, as in rAthena. Items and currencies are never discounted. In the list form, `discount` goes beside `shop`.
- A record with `shop` and no `map` is a shop that only scripts open, with `player:open_shop`.

```yaml
- name: Tool Dealer#prt1
  map: prt_in
  x: 126
  y: 76
  sprite: "53"
  shop:
    - {item: Red_Potion}
    - {item: Fly_Wing, price: 60}
- name: Cookie Stand
  shop:
    - {item: "example.greeter:Welcome_Cookie", price: 25}
- name: Prize Desk
  shop:
    buys_from_players: false
    items:
      - item: Red_Potion
        cost: {zeny: 20, currency: {tokens: 2}}
      - item: Wing_Of_Fly
        cost: {items: [{item: Jellopy, amount: 5}], currency: {tokens: 1}}
        stock: 3
        restock: daily
```

```yaml
# A smith at the door of every town's smithy, and a guard at every gate.
- name: Smith Helper
  place: {scope: "town:any", tag: blacksmith}
  sprite: 4_M_02
  script: scripts/npc/smith_helper.lua
- name: Gate Guard
  place: {scope: "town:any", tag: gate}
  sprite: 8W_SOLDIER
```

### Dungeon styles: data/dungeon_styles.yml

A procedural world's dungeons follow a style (strategy section 8.5, proposal 0002, review decision 42, decision 0040). Core has `cave`, `crypt`, `tower` and `ruins`. A mod changes them by name and adds its own, named with its ID.

```yaml
- name: cellar
  method: rooms
  floors: 2
  floor_size: [64, 64]
  room_size: [6, 12]
  tileset: tower
  level_step: 3
```

- `method` is `cave`, cellular automata with tunnels, or `rooms`, rooms without overlap joined by corridors with a few loops.
- `floors` is the floors of a dungeon of the style, 1 to 20. A server may run another count.
- `floor_size` is a floor's width and height in cells: even, 48 to 512.
- `room_size` is the smallest and largest side of a room in cells: even, 6 to 40, and the largest at most a third of the floor's smaller side. A cave may leave it out.
- `tileset` is a dungeon tileset of the map generator: `cave`, `crypt`, `tower` or `ruins` in version 3. It picks the textures, the props and the light.
- `level_step` is how many levels each floor's level band is above the floor before it, 1 to 50.
- Each floor has an `entrance`. Every floor but the last has an `exit` in the room farthest from the entrance, and the last floor has a `boss_room` there instead. Rooms have `room` places.

### Town templates: data/town_templates.yml

A procedural world's towns follow a template. Core has `hamlet`, `village` and `city`. A mod changes them by name and adds its own.

```yaml
- name: market_town
  size: 96
  gates: 2
  walls: true
  tileset: town
  decorations: [well, crate, bush]
  buildings:
    - {model: lamp_shop, footprint: [6, 6], places: ["shop:lamp"]}
    - {model: smithy, footprint: [8, 6], places: [blacksmith]}
    - {model: house, footprint: [6, 6], count: 8}
```

- `size` is the town map's side in cells: even, 64 to 256. The town stands on a flat plateau inside it, with a margin of rough ground.
- `gates` is 2 to 4. Each gate has a main road to the central square. `walls` puts a wall around the town, and a hedge of trees stands there without it.
- `tileset` is a town tileset of the map generator, `town` in version 3. `decorations` are props of the tileset that stand on empty lots: `tree`, `bush`, `well`, `crate`, `lamp`, `fountain`, `tower` or `gate`.
- `buildings` are placed in order, each with:
  - `model`: the building's model slot in the tileset, such as `smithy`. The server's tileset table maps it to art, and without art the building shows a placeholder house of its footprint. A model has one footprint in every template, and may not be the name of one of the tileset's props.
  - `footprint`: its width and depth in cells, facing south: even, 2 to 12.
  - `count`: how many, 1 to 64, 1 by default.
  - `places`: the tags of the named places at its door, such as `blacksmith`, `inn`, `storage` or `shop:tool`. Each takes its own cell on the street at the door, so the building's width must have 2 cells per tag. A building with places goes near the square and must fit, or the town is made again. Buildings without places fill the lots that remain.
- Every town has a `square`, and a `gate` and a `portal` per gate.

A template that cannot make a town in 8 tries stops the server and fails `wactl mod validate`.

Every core template has a `teleport` place, where the server's teleport service stands (decision 0041). A mod's template needs one for the service to stand in its towns.

### Naming styles: data/naming_styles.yml

A procedural world's towns take their names from a naming style, by the biome of the field they stand near (strategy section 8.10, proposal 0002, review decision 43, decision 0041). Core has `norse` for grassland, `sylvan` for forest, `dune` for desert, `frost` for snow, `marsh` for swamp and `coast` for beach. A mod changes them by name and adds its own. The last record for a biome, in load order, is that biome's style, so a mod's own record replaces core's.

```yaml
- name: meadow
  biome: grassland
  syllables:
    start: [wil, ash, bram, fern]
    middle: [a, e, en]
    end: [ford, ley, ton, brook]
  patterns: ["{start}{end}", "{start}{middle}{end}"]
```

- `biome` is a biome of the map generator: `grassland`, `forest`, `desert`, `snow`, `swamp` or `beach`.
- `syllables` are lists of 1 to 256 syllables by part name. A syllable is 1 to 12 ASCII letters. A part name is lowercase letters, digits and `_`.
- `patterns` are 1 to 32 patterns of 1 to 8 parts in braces. A name joins one syllable of each part, in lower case but its first letter.
- A name has 4 to 12 letters, no other town or the home map has it in any case, and the chat filter blocks none of its words. A name that fails is drawn again, up to 100 times. Some pattern must be able to make a name of 4 to 12 letters, or the record is refused.
- The world keeps its names from the day it was made. A changed style names only new worlds.

### Home maps: maps/<id>/

A mod may ship a home map in the map folder format (strategy section 8.12, [docs/specs/map-folder-v1.md](../docs/specs/map-folder-v1.md)): `map.yaml`, `cells.bin`, `materials.bin`, `objects.yaml` and `places.yaml` under `maps/<id>/`. A map tagged `home` in `map.yaml` is the world's home area when `wactl world new` makes the world with the mod: new characters start at its `start` place, and its first `portal` place links to the first town. Its map ID is `gen:home:<id>`.

- Clients download a home map, since they cannot generate it.
- `wactl map validate maps/<id>` and `wactl mod validate` check the files and run the generator's checks: every place on a walkable cell and reached from the first place, and a `portal` on a home map.
- When several mods have a home map, the config's `procedural.layout.home` names one as `mod:id`, or `none` for the first town.
- The mod must stay installed, since the server reads the map at each start.
- `flags: [nomemo]` forbids Warp Portal memo points on the map.

`mods/examples/home-village` is a sample with a home map and a naming style.

### Spawn tags

A procedural world picks each field's and dungeon floor's monsters once, when `wactl world new` makes it (strategy section 8.11, proposal 0002, review decision 44, decision 0042). It picks among the monsters with spawn tags, by biome, band and role, with the limits of section 8.11. Core's tags come from rAthena's spawn files: each map family's monsters weigh their share in its biome. A monster record sets or changes them with `spawn`:

```yaml
- name: example.mossy-fields:MOSS_PORING
  sprite: PORING
  level: 6
  spawn:
    biomes: {grassland: 1000, forest: 1000}
    role: common
    themes: [mossy]
```

- `biomes` maps biome names to weights from 1 to 1,000. The field biomes are `grassland`, `forest`, `desert`, `snow`, `swamp` and `beach`, and a dungeon floor's biome is its style's tileset: `cave`, `crypt`, `tower` or `ruins`. Core's weights are per 1,000 of a biome's monsters, so 1,000 makes a monster the likeliest of its biome.
- `role` is `common`, `uncommon`, `plant`, `rare`, `mini_boss` or `field_boss`. Left out, it stays as it was, or is `uncommon`, or `mini_boss` or `field_boss` for a boss. A boss, mob_db's boss class or an MVP, is never `common` or `uncommon`: such a record is an error.
- `themes` names spawn themes that raise the monster.
- Each field given replaces the monster's: a record that sets `biomes` replaces all of them. A new mod monster has no tags until it sets them, so it never spawns on generated maps without them.
- The level, race, element and size come from the monster's data. A level far from a map's band makes a monster unlikely there: within the band it weighs its full weight, and 4% less per level outside.
- The world keeps its spawns. A mod added later changes only new worlds, and a removed mod's lines are left out, with a warning at each start.

### Spawn themes: data/spawn_themes.yml

A field or floor draws a theme with a chance of 1 in 3. The monsters that match it weigh `boost` percent of their weight there. Core has `undead`, `insect_swarm`, `wild_beasts`, `overgrown`, `demons`, `deep_water`, `constructs`, `embers` and `frost`.

```yaml
- name: mossy
  biomes: [grassland, forest, swamp]
  races: [plant]
  elements: [earth]
  boost: 300
```

- `biomes` are where the theme may apply. Left out, it applies everywhere.
- A monster matches by `races` (names of `wa.races`), by `elements` (names of `wa.elements`), or by naming the theme in its `spawn.themes`, with or without the mod prefix.
- `boost` is 100 to 1,000, 300 by default.

`mods/examples/mossy-fields` is a sample with spawn tags, a theme and a `spawn_table` hook.

### Monsters: data/monsters.yml

```yaml
- name: example.halloween:PUMPKIN_PORING
  sprite: PORING
  display_name: Pumpkin Poring
  level: 12
  hp: 260
  attack: 22
  str: 12
  base_exp: 90
  job_exp: 70
  element: ghost
  drops:
    - {item: Pumpkin, rate: 2000}
    - {item: "example.halloween:Pumpkin_Hat", rate: 50}
- name: PORING
  hp: 80
```

- Names are AegisNames in upper case, such as `PORING`. A new monster is named with the mod's ID.
- `sprite` names the monster whose sprite players see. A new monster needs it, and starts from that monster's stats, with no exp and no drops. `sprite` may name a sprite of art instead, such as `mod/example.lantern/sprite/mob/Lantern_Ghost`. A new monster then starts from a Poring's stats.
- `sounds` names the monster's sounds of art, by `attack`, `damage`, `die` and `move` ("Art" below).
- The fields are rAthena's `mob_db.yml` values. The server computes the rest from them, as rAthena's `status_calc_mob`: hit, flee, the defense and magic defense from stats, the attack range of each hit, and the magic attack. There are no fields to set those; a mod raises them with stats or bonuses.
  - `hp`, `sp`, `level`, `str`, `agi`, `vit`, `int`, `dex` and `luk`.
  - `attack`: one number, rAthena's `Attack`. In pre-renewal it can also be `[minimum, maximum]`, rAthena's `Attack` and `Attack2`.
  - `magic_attack`: renewal only, rAthena's `Attack2`.
  - `defense`, `magic_defense`, `resistance` and `magic_resistance`.
  - `speed` (milliseconds per cell), `attack_range`, `skill_range`, `chase_range`, `attack_delay`, `attack_motion` and `damage_motion`.
  - `mvp_exp` and `mvp_drops`, a list of at most 3 `{item, rate}`, which go to the MVP of a monster with the `mvp` mode.
  - `damage_taken`: the percent of damage the monster takes, 0 to 100.
- `element` (from `wa.elements`, with `element_level` 1 to 4), `race` (from `wa.races`) and `size` (from `wa.sizes`) are names.
- `class` is `normal`, `boss`, `guardian`, `battlefield` or `event`. As in rAthena, `boss` adds the modes `detector`, `status_immune` and `knockback_immune`.
- `ai` is rAthena's AI type, 1 to 27, which sets the monster's modes (see "Monster AI", below).
- `modes` is a list of rAthena's modes in lower case, such as `can_move`, `aggressive`, `looter` and `mvp`. They add to the modes of the AI type.
- `skills` is the monster's skills list, converted from rAthena's `mob_skill_db.txt` for core's monsters. It replaces the monster's list (see "Monster skills", below).
- `script` names the monster's AI script, which runs for each spawned monster (see "Monster AI", below).
- `base_exp` and `job_exp` are scaled by the exp rates. `drops` is a list of `{item, rate}`, with rates per 10,000, at most 20. The drop rate settings apply to them as to core's. A drop can add `random_option_group`, a group of `data/random_option_groups.yml`: the dropped copy rolls its random options ("Random options" below). `mvp_drops` take it too.
- `display_name` is the name players see. `id` cannot be set.
- `spawn` sets the monster's spawn tags for procedural worlds ("Spawn tags", above).
- `monsters.id "example.halloween:PUMPKIN_PORING"` finds a mod's monster. In a classic world, monsters appear on maps through scripts (`spawning.monsters`, below). There is no `data/spawns.yml` yet.

### World events: data/events.yml

```yaml
- name: Pumpkin_King
  trigger: { daily: "20:00" }
  duration_minutes: 60
  script: scripts/events/pumpkin_king.lua
  map: prt_fild08
```

The world event scheduler starts events without staff action (strategy section 8.8). A mod that adds or changes an event needs the permission `events.schedule`.

| Field | Meaning |
|---|---|
| `trigger` | What starts the event (below). |
| `duration_minutes` | How long a run lasts: 1 to 10,080 minutes. |
| `script` | The entry script of a run. Its setup function runs in a new event context. Optional for a typed event. |
| `map` | The map whose zone runs the event. Without it, the world's start map. |
| `type` | `boss`, `invasion` or `treasure_hunt` for a typed event ("Typed events" below). Without it, a script event. |
| `rewards` | What a typed event gives ("Typed events" below). |

| Trigger | Starts the event |
|---|---|
| `daily: "20:00"` | Every day at that time. |
| `weekly: { day: saturday, time: "20:00" }` | Every week. |
| `monthly: { day: 1, time: "00:00" }` | Every month. A month without the day, such as the 31st, has no run. |
| `hourly: { minute: 30 }` | Every hour. |
| `random: { chance_per_hour: 500 }` | At the start of each hour, with a chance per 10,000 from a secret stream. |
| `condition: { module: example.bosses.conditions, function: town_is_quiet, lasting_minutes: 60 }` | Once the function has returned true for that long. |

Rules:

- Times are in the server's time zone, `time_zone` in the config (server/README.md). On a day clocks change, a time that does not exist runs at the next valid minute, and a time that happens twice runs once.
- Each run is one context of kind `"event"`. `context:get_instance()` is `{ name, map }`. What the run makes goes when it ends: monsters, handlers, timers, schedules and hooks.
- An event has one run at a time. A time that comes while a run lasts is skipped.
- Each run is recorded before it starts, so a restart never runs the same time twice. Runs whose window passed while the server was stopped are skipped. A run whose window is still open when the server starts begins at once and ends at its usual time. A run cut short by a stop does not start again.
- A condition is a function of a helper module that the mod can `require`. The module returns a table that holds it. It runs once a minute in the zone of the event's map, and only reads the game: a call that changes it raises an error. After a run ends, it must hold for `lasting_minutes` again.
- A sleeping zone keeps its clock still, so the timers of a run on a map without players wait. The run still ends on time.
- The server owner can move or turn off an event in the config, under `mods.events` (server/README.md, "World events").

```lua
-- scripts/events/pumpkin_king.lua
local events = require "wa.events"
local monsters = require "wa.monsters"
local spawning = require "wa.spawning"
local world = require "wa.world"

local PUMPKIN_KING = monsters.id "example.halloween:PUMPKIN_KING"

return function()
  local king = spawning.monsters { monster = PUMPKIN_KING }[1]
  world.announce("The Pumpkin King has risen on the Prontera fields!")
  events.on("death", function(death)
    world.announce(death.killer.name .. " defeated the Pumpkin King!")
  end, { victim = king })
end
```

#### Typed events

An event with `type` is a typed event (review decision 31): core runs it from its fields, and a script is optional. The types are `boss`, `invasion` and `treasure_hunt`. The type's fields sit beside `trigger` and `duration_minutes`.

```yaml
# A boss on a field of levels 20 to 40. The players who did at least 1%
# of its damage each get the whole reward.
- name: Orc_Night
  type: boss
  trigger: { daily: "21:00" }
  duration_minutes: 60
  monsters: [ORK_HERO, ORC_LORD]
  level_band: [20, 40]
  announce: true
  rewards: { to: participants, zeny: 10000, items: [{ item: Old_Blue_Box, amount: 1 }] }

# Waves of monsters attack a town.
- name: Prontera_Raid
  type: invasion
  trigger: { weekly: { day: saturday, time: "20:00" } }
  duration_minutes: 30
  town: prontera
  waves:
    - monsters: [{ monster: PORING, amount: 20 }]
      start: cleared
    - monsters: [{ monster: SAVAGE_BABE, amount: 10 }]
      start: "120s"
  rewards: { to: everyone_on_map, items: [{ item: Red_Potion, amount: 10 }] }

# Chests on random cells, with NPCs that give hints.
- name: Hunt
  type: treasure_hunt
  trigger: { daily: "15:00" }
  duration_minutes: 60
  chests: 5
  maps: [prt_fild08, prt_fild07]
  chest: TREASURE_BOX1
  hint_npcs:
    - { map: prontera, x: 156, y: 191, sprite: 4_M_MANAGER, display_name: Treasure Guide }
  rewards: { to: killer, zeny: 1000 }
```

| Type | Fields |
|---|---|
| `boss` | `monsters`: a list of monster names; the run picks one. `level_band: [lowest, highest]` or `maps`. `announce`: true by default. |
| `invasion` | `town`: a map. `waves`: each with `monsters`, a list of `{ monster, amount }` (amount 1 by default, at most 500 per wave), and `start`: `cleared`, or a time after the wave before started, such as `"120s"`, `"5m"` or `"1h"`. At most 50 waves. |
| `treasure_hunt` | `chests`: 1 to 200. `level_band` or `maps`. `chest`: a monster name, whose drops are the loot. `hint_npcs`: at most 20 `{ map, x, y, sprite, display_name }`; the sprite is `4_M_MANAGER` and the name "Treasure Hunter" by default. |

- A monster name is an AegisName or a mod's full name. A plain name is the event's mod's record first. The monster must have stats.
- `level_band` picks the fields whose level falls in the band. A field is a map whose ID has `_fild`, as rAthena names its fields, and its level is the middle level of its spawns, weighted by their amounts, without bosses. `maps` lists maps of any kind instead.
- A boss lands on a random walkable cell of a random map, and leaves when the run ends. With `announce`, the world hears when it appears, who killed it with the MVP, the player with the most damage dealt and taken, as `mob_dead` finds it, and when it leaves.
- Invaders enter at walkable cells at the town's edges. They attack players on sight, chase them as rAthena's monster AI does, and with no target walk toward the nearest player on the map. The raid is won when the last wave has started and every invader died, and lost when the time runs out. Each wave and the result are announced.
- Each chest lands on a random walkable cell of a random map of the event. A hint NPC tells how many chests are still there, and for up to three of them the direction and distance from the NPC on its map, or the map and the rough cell. The hunt is won when every chest is opened.
- `map` sets the zone that runs the script. Without it, the script runs on the boss's map, the town, or the first hint NPC's map.
- A map an event names must run, or the server does not start. A hint NPC on a map that does not run is skipped, with a warning.

`rewards` is `{ to, items, zeny, exp, currency, min_share }`:

| Field | Meaning |
|---|---|
| `to` | `killer`: the player who killed the boss or opened the chest, or for an invasion the player who killed its last monster. `participants`: the players on the map who did at least `min_share` percent of the damage, 1 by default. `everyone_on_map`: every player on the map. Each gets the whole reward. |
| `items` | A list of `{ item, amount }`. It needs `player.give_items`. |
| `zeny` | It needs `player.give_zeny`. |
| `exp` | `{ base, job }`. It needs `player.give_exp`. |
| `currency` | A mapping of currency names to amounts. Another mod's currency, core's too, needs `currencies.modify`. |

- A boss's and a chest's rewards come at their death, an invasion's when it is won. Only players on the map get them.
- Items that do not fit, and zeny past 2,147,483,647, come by mail from the event.
- Rewards go through transfers, with the action `event` in the item and zeny logs.
- A reward is saved with its progress in one commit, so a restart never gives it twice.

A typed event's script gets the run:

```lua
local context = require "wa.context"
local events = require "wa.events"

return function()
  local run = context:get_event()
  local boss = run:get_monsters()[1]
  events.on("death", function()
    for _, player in ipairs(run:get_participants()) do
      player:send_message("Thank you, " .. player.name .. ".")
    end
  end, { victim = boss })
end
```

- `context:get_event()` is the run in a typed event's script, and nil elsewhere.
- `run:get_monsters()` returns the run's living monsters on the context's map. `run:get_participants()` returns the players on the map with at least `min_share` of the damage to the run's monsters there. Both need `game.read`.
- `run:finish("won")` or `run:finish("lost")` ends the run within a second. An invasion that finishes won gives its rewards.

World events of any type:

```lua
local events = require "wa.events"
local world_events = require "wa.world_events"

return function()
  events.on("world_event_end", function(event)
    -- event.name, event.type ("boss", "invasion", "treasure_hunt" or
    -- "script") and event.result ("won", "lost", "stopped" or nil).
  end)
  world_events.start("Prontera_Raid")
  world_events.stop("example.other:Night")
end
```

- `world_event_start` and `world_event_end` run in every zone for every run.
- `world_events.start(name)` starts a run now for its duration, and `world_events.stop(name)` ends the run, with the result `stopped`. They take effect within a second and return false when the event does not exist, already runs, or does not run. A plain name is the calling mod's event. They need `events.control`.
- Restarts: a typed run that a stop cut short goes on while its window is open, from its progress: a boss that did not die comes back, chests that were opened stay opened, and an invasion starts again at the wave it was in. A run that ended before its time, such as a boss that died, does not.
- Core has one sample event of each type: `core:Field_Boss`, `core:Prontera_Invasion` and `core:Treasure_Hunt`. They are off until the server owner turns them on (server/README.md, "World events").
- The client shows each start and end, and a list of the runs that last.
- [examples/halloween](examples/halloween) has an event of each type, a script that uses the run, and tests that drive the clock.

### Skills: data/skills.yml

```yaml
- name: example.chain-lightning:CHAIN_LIGHTNING
  display_name: Chain Lightning
  icon: MG_LIGHTNINGBOLT
  max_level: 5
  type: magic
  target: enemy
  element: wind
  range: 9
  cast_time: [1000, 1200, 1400, 1600, 1800]
  after_cast_delay: 1000
  requirements:
    sp: [20, 24, 28, 32, 36]
  script: scripts/skills/chain_lightning.lua
- name: MG_LIGHTNINGBOLT
  cast_time: 500
```

Records are rAthena's `skill_db.yml` under readable names (proposal 0002, review decision 20). Core's skills use the same records, so a mod changes a core skill by its name, such as `MG_LIGHTNINGBOLT`. A new skill is named with the mod's ID.

A number is one value for every level, or a list with one value per level from level 1. Levels past the end of a list take its last value.

| Field | Meaning |
|---|---|
| `display_name` | The name players see. |
| `icon` | An existing skill, such as `MG_LIGHTNINGBOLT`, whose icon and animation the skill shows, or an icon of art, a texture ("Art" below). |
| `effects` | Effects of art by moment: `caster`, `effect` and `hit`, each a list of effect names. |
| `max_level` | 1 to 13. A new skill has 1 by default. |
| `type` | `weapon`, `magic`, `misc` or `none`, as rAthena's `BF_` types. |
| `target` | `passive`, `enemy`, `ground`, `self` or `support`. A new skill is passive by default. |
| `element` | A name of `wa.elements`, such as `wind`, or `weapon` (the weapon's element), `endowed` or `random`. |
| `range` | 0 to 30 cells. |
| `cast_time`, `fixed_cast_time`, `after_cast_delay`, `cooldown` | Milliseconds, up to one hour. A negative `fixed_cast_time`, from -100, is a percent of the cast time in renewal, as in rAthena. |
| `requirements` | What a cast costs and needs, below. |
| `hit` | `normal`, `single` or `multiple`: how the client shows the hits. |
| `hit_count` | Hits per cast, -30 to 30. A negative count shows one number for all hits, as in rAthena. A new skill has 1 by default. |
| `damage_flags` | A list of `no_damage`, `splash`, `splash_split`, `ignore_attack_card`, `ignore_element`, `ignore_defense`, `ignore_flee`, `ignore_defense_card`, `critical`, `ignore_long_card` and `simple_defense`. It replaces the skill's flags. |
| `splash` | The splash area in cells, 0 to 30, or -1 for the whole screen. |
| `knockback` | Cells, 0 to 20. |
| `script` | The skill's entry script (see "Skills" under Scripts). |

| Requirement | Meaning |
|---|---|
| `sp`, `hp` | The cost, 0 to 100,000. |
| `sp_rate`, `hp_rate` | A percent, -100 to 100: positive of the current value, negative of the maximum. |
| `zeny` | The zeny cost. |
| `spirit_spheres` | Spirit spheres used, 0 to 15. |
| `weapons` | A list of weapon types from `wa.weapon_types`, such as `[dagger, one_handed_sword]`. An empty list allows any weapon. |
| `items` | A list of up to 10 `{item, amount}`. `amount` is 1 by default. |
| `companion` | A companion of `data/companions.yml`, such as `falcon`. The caster's companion must be it or act as it. |

- `id` cannot be set. A new skill gets an ID from 20,000.
- `skills.id "example.chain-lightning:CHAIN_LIGHTNING"` finds a mod's skill.
- Setting `script` on a skill the mod did not add, such as a core skill, replaces that skill's behavior. It needs the permission `rules.override`.
- A script must be a file of the mod that sets it. A missing file is an error at start.

### Skill trees: data/skill_trees.yml

```yaml
- job: mage
  skill: example.chain-lightning:CHAIN_LIGHTNING
  max_level: 5
  requires:
    - {skill: MG_LIGHTNINGBOLT, level: 3}
```

| Field | Meaning |
|---|---|
| `job` | A job's name in `wa.jobs`, such as `mage`, a mod job's full name, or its ID. |
| `skill` | A skill's name: core's, or a mod's full name. |
| `max_level` | The most levels the job may learn, up to the skill's `max_level`. |
| `requires` | Skills the job must have first, each `{skill, level}`. |

- An entry adds the skill to the job's tree, or replaces the job's entry for it.
- Jobs that take the job's tree in rAthena's `skill_tree.yml` (`Inherit`) get the entry too, such as the Wizard and the High Wizard for the Mage. A job that lists the skill itself keeps its own entry. A new job whose `inherits` lists the job gets it too.
- A later mod's entry for the same job and skill wins.

### Jobs: data/jobs.yml

Job records follow proposal 0002, review decision 24. Core's jobs are records too: rAthena's `job_stats.yml`, `job_aspd.yml`, `job_basepoints.yml`, `job_exp.yml` and the `Inherit` lists of `skill_tree.yml`, converted when the server starts. The job tables, the exp tables, `jobs.next_jobs` and the skill trees of new jobs come from the records. [examples/desperado](examples/desperado) adds a job.

```yaml
- name: example.desperado:Desperado
  display_name: Desperado
  look: gunslinger
  tier: second
  changes_from:
    - {job: gunslinger, job_level: 40}
  inherits: [novice, gunslinger]
  max_job_level: 50
  exp_table: knight
  hp: {factor: 110, increase: 500}
  aspd: {revolver: 55, rifle: 70}
  bonus_stats:
    - {level: 1, dex: 1}
- name: knight
  max_weight: 30000
```

| Field | Meaning |
|---|---|
| `display_name` | The name players see. A new job without it shows its name without the mod prefix. |
| `look` | A job whose sprites the job shows. A new job needs it. Every field a new job leaves out comes from its look. On a core job, `look` changes only the sprites. It may also be a table with `job`, the job a new job starts from, and `m` and `f`, body sprites of art ("Art" below). |
| `tier` | `novice`, `first`, `second`, `third`, `fourth` or `expanded`. |
| `transcendent`, `baby` | Whether the job is a transcendent or a baby job. |
| `base_job` | rAthena's `BaseJob`, which `jobs.base_job` returns. |
| `changes_from` | The jobs that may change into this one, each `{ job, base_level, job_level }`. The levels are optional. `jobs.next_jobs` and `player:change_job` read it. `[]` leaves the job to scripts with `force`. |
| `inherits` | Jobs whose skill trees the job's tree takes, as rAthena's `Inherit`, such as `[novice, swordman, knight]`. |
| `equips_as` | A job whose equipment the job may wear: item `jobs` masks and `classes` read that job's class. |
| `max_base_level`, `max_job_level` | Up to 275. The exp tables must cover them. |
| `max_stats` | The most STR to LUK. By default, rAthena's maximum for the job's class. |
| `max_weight` | The weight limit, in tenths, before STR. |
| `hp`, `sp` | `{ factor, increase }`, as `job_stats.yml`'s `HpFactor` and `HpIncrease`, with rAthena's formula. |
| `hp_table`, `sp_table` | One value per base level, in place of the formula. |
| `aspd` | Base ASPD by weapon type, such as `{ fist: 40, revolver: 55 }`, as `job_aspd.yml`. Renewal also has `shield`. A weapon type left out keeps its value. |
| `bonus_stats` | The stats each job level adds, each `{ level, str, agi, vit, int, dex, luk }`, as `job_stats.yml`'s `BonusStats`. Renewal also takes `pow`, `sta`, `wis`, `spl`, `con` and `crt`. |
| `exp_table` | A job whose exp tables the job uses. |
| `base_exp`, `job_exp` | The exp from each level to the next, in place of `exp_table`. |

- New jobs get IDs from 10,000. `jobs.id "example.desperado:Desperado"` finds one.
- rAthena's rules that read the class, such as dual wielding or the maximum stats of a class, treat a new job as its look's class with its own `tier`, `transcendent` and `baby`. The Desperado above is a second job of the Gunslinger's line, as the Rebellion.
- A new job's stats, weight and maximum stats come from its own record, never from its look.
- On core's jobs, `tier`, `transcendent`, `baby`, `base_job` and `inherits` come from rAthena and cannot change.
- Core's `changes_from` holds rAthena's job change rules without levels, since `jobchange` checks none. Core's Job Master checks rAthena's levels itself.
- The client shows the look's sprites and the `display_name`, which the server sends with the job.
- A new job keeps a companion or a mount when it meets the record's `requires`, such as Falconry Mastery for a falcon (see "Companions, mounts and carts").
- A character whose mod job was removed loads as a Novice, at most at job level 10, and the server logs a warning. Its skills, stats and items stay.

### Companions, mounts and carts: data/companions.yml and data/mounts.yml

Companion and mount records follow proposal 0002, review decision 25. Core's records are files of the core mod: the falcon in `data/companions.yml`, and the Peco Peco, Grand Peco, gryphon, dragon, warg and Madogear in `data/mounts.yml`. A mod adds its own records and changes core's by name. [examples/familiars](examples/familiars) adds an Owl and a Horse.

```yaml
# data/companions.yml
- name: example.familiars:Owl
  display_name: Owl
  look: falcon
  acts_as: falcon
  requires:
    - {skill: HT_FALCON, level: 1}
  bonuses:
    - [dex, 2]
# data/mounts.yml
- name: example.familiars:Horse
  display_name: Horse
  look: peco_peco
  acts_as: peco_peco
  requires:
    - {skill: KN_RIDING, level: 1}
  speed: 20
```

| Field | Meaning |
|---|---|
| `display_name` | The name players see. By default the name without the mod prefix. |
| `look` | The sprite. Companions: `falcon` or `warg`. Mounts: `peco_peco`, `grand_peco`, `gryphon`, `dragon`, `dragon_brown`, `dragon_gray`, `dragon_blue`, `dragon_red`, `warg` or `madogear`. Or a sprite of art, with `acts_as`, whose option bit shows it ("Art" below). |
| `acts_as` | The core record whose rules apply. A companion that acts as `falcon` works with the falcon's skills, such as Blitz Beat. A mount that acts as `peco_peco`, `grand_peco` or `gryphon` follows the riding rules, and `dragon`, `warg` and `madogear` their own: walk speed, the mounted ASPD rule with Cavalier Mastery or Dragon Training, the weight limit, and spears against medium targets. A core record acts as itself. A new record without `acts_as` has a look and bonuses only. |
| `requires` | Skills the player needs, each `{ skill, level }`. `level` is 1 by default. |
| `bonuses` | Static bonuses while the player has the record, as an item's `bonuses`, such as `[[dex, 2]]`. |
| `speed` | Mounts only: the walk speed bonus in percent, 0 to 50, in place of the rules' bonus. A Peco Peco and a dragon have 25, a warg 15 plus 5 per level of Warg Rider. |

| Call | What it does |
|---|---|
| `companions.id(name)`, `mounts.id(name)` | A record's ID: core's from 1, a mod's from 10,000. They need `game.read`. |
| `player:set_companion(companion)` | Gives the player a companion in place of its current one, or removes it with `nil`. Returns false, and changes nothing, when the player lacks the skills of `requires`, as `setfalcon` without Falconry Mastery. |
| `player:set_mount(mount)` | The same for a mount, as `setriding`. |
| `player:set_cart(look)` | Gives the player a cart of a look from 1 to 5, or removes it with `nil`, as `setcart`. Returns false without Pushcart, or for a look above 5. |
| `player.companion`, `player.mount`, `player.cart` | The record IDs and the cart's look, or `nil`. Read-only. |

- A player has at most one companion and one mount.
- A job change removes a companion or a mount whose `requires` the new job does not meet, and a cart without Pushcart, as `pc_jobchange`. It always removes the Madogear, as rAthena does. `player:reset_skills` removes those whose skills it resets, as `resetskill`, and `player:reset_skill` those whose skills the player no longer has.
- The client shows the look: the falcon or warg beside the player, the cart behind it, and the mounted sprite of the player's job. A job without a mounted sprite, such as a Hunter on a Peco Peco, shows its costume mount. A record whose `look` is art shows its art instead ("Art" below).
- The character saves the record names and the option bits of their looks. A record whose mod was removed goes when the character loads, with a warning in the log.
- The converter maps `setfalcon`, `setriding` and `setcart` to these calls, and `checkfalcon`, `checkriding` and `checkcart` to `player.companion ~= nil`, `player.mount ~= nil` and `player.cart ~= nil`.

### Chat channels and the chat filter: data/channels.yml and data/chat_filter.yml

Chat records follow proposal 0002, review decision 26. Core's records are files of the core mod: rAthena's default channels `#global`, `#support`, `#trade` and `#map` in `data/channels.yml`, and reserved staff titles in `data/chat_filter.yml`. Server owners add their word lists in a mod of their own.

```yaml
# data/channels.yml: change a core channel by name.
- name: trade
  color: 0x00FF00
  autojoin: true
# data/chat_filter.yml: words of the filter.
- {name: grief, action: replace}
- {name: spamsite, action: reject, match: part}
- {name: gamemaster, action: reserve}
```

| Channel field | Meaning |
|---|---|
| `display_name` | What players see before each line, as rAthena's alias, such as `[Trade]`. |
| `color` | The lines' color, written 0xRRGGBB. |
| `join` | `open`: any player may join. `invite`: only `channel:add` puts players in. |
| `autojoin` | Players join it at login. It needs `join: open`. |
| `can_leave`, `can_chat` | rAthena's `leave` and `chat` options. `#global` cannot be left, as rAthena's. |
| `delay_milliseconds` | The least time between two lines of one member, 1000 by default, as rAthena's `delay`. |
| `kind` | `world`, or `map`: one channel per map, as rAthena's `#map`. A member hears the members on its own map and leaves it on a map change. |

- A mod changes core's channels by name. It makes its own channels with `channels.create`, not with records.
- `public`, `whisper`, `party` and `guild` are not channel names.

| Filter field | Meaning |
|---|---|
| `action` | `replace` (the default) turns the word into asterisks. `reject` refuses the whole line, and the speaker is told. `reserve` only refuses character names that have it, such as a staff title. |
| `match` | `word` (the default) finds the word alone, so `grief` does not match `griefer`. `part` finds it inside longer words too. |

- Case does not matter. A mod's word is the record name after the mod's ID.
- Every word, whatever its action, refuses new character names that have it. The account pages and `--dev` logins check names.

### Quests: data/quests.yml

Quests, as rAthena's `quest_db.yml` (proposal 0002, review decision 45). Core's records are rAthena's quests of the mode, converted when the server starts, and named by their IDs, such as `"7104"`.

```yaml
- name: porings
  title: Poring Patrol
  time_limit: 1h
  targets:
    - {monster: PORING, count: 3}
    - {count: 10, min_level: 10, max_level: 20, race: plant, map: prt_fild08, map_name: Prontera Fields}
  drops:
    - {monster: PORING, item: Jellopy, rate: 5000}
- name: "2221"
  targets:
    - {monster: NOVUS_, count: 20}
```

- A record becomes `<mod ID>:<name>`, such as `example.hunting-board:porings`, with an ID from 1,000,000. A core quest is changed by its ID as the name, in quotes.
- `title` is the name the quest window shows, 1 to 255 characters. It is required.
- `time_limit` is left out for none. It is a duration, such as `3h`, `2h30m` or `1d`, or a number of seconds: the quest ends that long after it starts. `{ at: "04:00" }` ends it at the next 04:00 on the server's clock, `{ at: "04:00", days: 7 }` seven days after that, and `{ at: "04:00", weekday: monday }` on the next Monday at 04:00, as rAthena's `TimeLimit`. A quest that runs out stays in the log, as in rAthena: scripts check `player:quest_timed_out`.
- `targets` are at most 3 hunt objectives. Each has `count`, the kills it asks for, 0 to 65,535.
  - With `monster`, it counts that monster's kills on any map, and takes no filter.
  - Without it, it counts every kill that passes its filters: `min_level` and `max_level` (a `max_level` alone makes `min_level` 1), `race`, `size` and `element` (names of `wa.races`, `wa.sizes` and `wa.elements`), `map` (the player's map, such as `prt_fild08` or `gen:field:2:3`), and `monsters`, a list that the monster must be in. `map_name` is the text the quest window shows.
- `drops` are items each kill gives while the quest is active or inactive, straight to the inventory: `monster` (every monster when left out), `item`, `count` (1 by default; more only for items that stack), and `rate` out of 10,000. A drop that does not fit is lost, and the player is told.
- A field set on a record replaces the whole field, as for other records: setting `targets` replaces every objective.

Kinds not overlaid yet: spawns and achievements.

## Art

A mod ships its own art ([decision 0033](../docs/decisions/0033-mod-assets.md)). [examples/lantern](examples/lantern) has, drawn by code:

- item icons, a dropped item sprite, and the worn sprites of a hat and a shield;
- a companion's sprite, the Lantern Wisp, which the Lantern Master gives a Lantern Keeper;
- an animated monster sprite with sounds;
- an NPC sprite;
- a job's body sprites;
- a skill's icon, and its hit effect as a loose RO `.str` file with its texture;
- a client data table.

### Sources: art/

`art/` holds the sources, in two kinds that can mix:

- The pack format of [assets-v1](../docs/specs/assets-v1.md), "Pack format for original art": `art/pack.yaml` with the art's `license`, PNG sprites with an optional YAML frame file, PNG, JPEG and WebP textures, glTF binary models, WAV sounds, Opus in WebM, MP3 music and JSON tables.
- Loose RO files ("Loose RO files" there): `.spr` with `.act`, `.bmp`, `.tga`, `.wav`, `.pal`, `.rsm`, `.str`, and maps as `.rsw` with `.gnd` and `.gat`.

```text
art/
  pack.yaml                        name: Lantern Festival art, license: CC0-1.0
  icon/item/Paper_Lantern.png      a 24 x 24 icon
  sprite/item/Paper_Lantern.png    the item on the ground
  sprite/mob/Lantern_Ghost.png     4 frames of 40 x 48 in a row
  sprite/mob/Lantern_Ghost.yaml    its frames and actions
  sprite/npc/Lamplighter.png       one still frame
  sprite/headgear/Lantern_Hat.png  the hat on a head, with a frame file
  sprite/body/Lantern_Keeper/m.png the job's body, with a frame file
  icon/skill/LANTERN_FLARE.png     a skill icon
  effect/flare.str                 a loose RO effect
  texture/effect/flare.bmp         its texture
  sound/Lantern_Ghost/die.wav      a monster sound
```

A file's path without its extension is its name, under `mod/<mod id>/`: `art/icon/item/Paper_Lantern.png` is `mod/example.lantern/icon/item/Paper_Lantern`. The folder is the namespace, so it says the type: sprites under `sprite/`, icons under `icon/`, textures under `texture/`, and so on. Names keep their case after the namespace.

A sprite's frame file cuts the image into frames and lists the actions, in RO's order: idle, walk, attack, hurt and dead for a monster. assets-v1, "Pack sprites", has every field:

```yaml
grid: [40, 48]
origin: [20, 46]
actions:
  - {frames: [0, 1], interval_ms: 400}
  - {frames: [0, 1], interval_ms: 160}
  - {frames: [0, 2, 2, 0], interval_ms: 120, events: {2: atk}}
  - {frames: [2, 0], interval_ms: 100}
  - {frames: [3]}
```

### Building: assets/

- `wactl mod build-art` builds `art/` into the mod's `assets/`, which the server reads for a folder mod. Run it after each change to `art/`. `--fast` builds quicker for development.
- `wactl mod pack` builds `art/` into the `.zip` file. A build in the folder is not packed.
- A file the build cannot use, such as a sprite in `icon/` or a sound in a format that is not converted yet, fails both with the reason.
- The server checks every file of `assets/` at startup, and refuses to start with a broken build, one over the size limit (`mods.max_asset_mb`, 200 MiB by default), or one with names outside `mod/<its mod id>/`. A mod with `art/` and no `assets/` starts, and shows placeholders.

### Art fields

A record names its art with these fields. The value is an asset name: the mod's own art, another mod's, or core's.

| Records | Field | Names |
|---|---|---|
| Items | `icon`, `image` | An icon and a large image, textures. |
| Items | `drop_sprite`, `view_sprite` | The sprite on the ground, and the sprite drawn on a character. |
| Monsters | `sprite`, `sounds` | The sprite, and sounds by `attack`, `damage`, `die` and `move`. |
| NPCs | `sprite` | The sprite. |
| Jobs | `look` | A table with `m` and `f` body sprites, and `job`, the job a new job starts from. |
| Skills | `icon`, `effects` | An icon, a texture, and effects by `caster`, `effect` and `hit`. |
| Companions, mounts | `look` | The sprite, with `acts_as`. |

```yaml
- name: example.lantern:Lantern_Oil
  type: Etc
  icon: mod/example.lantern/icon/item/Paper_Lantern
```

A mod's own record without a field uses the art named after it, when the build has it: `icon/item/<name>`, `image/item/<name>` and `sprite/item/<name>` for items, `sprite/body/<name>/<m or f>` for jobs, and `icon/skill/<name>` for skills. So `example.lantern:Paper_Lantern` shows `mod/example.lantern/icon/item/Paper_Lantern` without a field.

The client shows a placeholder for art the builds lack, and logs the name once. `wactl mod validate` reports each art name a record names under `mod/` that the mod it names does not provide.

The web client draws item icons, dropped items, monster and NPC sprites, job bodies, skill icons and effects, monster sounds, worn item sprites, and companions and mounts from art (review decisions 40 and 41):

- An item with a `view_sprite` gets a view ID of its own from 1,000,000 at each start. Looks send it for headgear, garments and shields, and in `weapon_view` for the right hand's weapon, so all of them draw their sprite (decision 0036).
- A mod's job comes in looks by its own ID, and the client data names the job it looks like. Its body is its art, and its weapon, head and other parts are those of that job.
- A skill's `effects` replace the effects of the client tables at their moments.
- Looks carry the record IDs of a player's companion and mount (decision 0036). The client draws a companion of art beside the player, as the falcon, and a mount of art at the player's cell, under the body. A player on a mount of art shows its job's body on foot, not the mounted sprite of the record it acts as.

### Names in the client

The client shows the names of a mod's records from the server's client data (review decision 40): items, monsters, skills and jobs that a mod adds, and the `display_name` a mod sets on core's records. An item's `description` field is shown under its name in the inventory:

```yaml
- name: example.lantern:Paper_Lantern
  display_name: Paper Lantern
  description: |-
    A lantern of thin paper that Lantern Ghosts carry.
    ^777777Weight:^000000 1
```

A description is text of up to 4,096 bytes, one line per line break. `^` and six hex digits set the color of the text after them, as in RO's item descriptions.

### Client data tables: client/

A mod may change the web client's tables with `client/jobs.yml`, `client/weapons.yml`, `client/effects.yml` and `client/skills.yml`. An entry replaces core's entry with the same key, and others are added. [docs/client-data.md](../docs/client-data.md) explains each table. `wactl client-data validate <mod folder>` checks them.

## Text: locales/en.yml

Text for players lives in `locales/en.yml`, a mapping of keys to text. Nested keys are joined with dots:

```yaml
greeter:
  hello: Hello! Welcome to Prontera.
```

`text "greeter.hello"` from `wa.text` reads it. Players have no language setting yet, so the English file is the only one read. A key the file lacks raises an error.

## IDs

The server gives each new record an ID in the mod range of its kind the first time it loads it. Items start at 2,000,000, monsters at 100,000, skills at 20,000, jobs, companions and mounts at 10,000, and quests at 1,000,000. Achievements start at 1,000,000 too, for when that kind gets overlays.

- An ID never changes, and never goes to another name, even when the mod is removed. The item comes back with its ID when the mod is installed again.
- Scripts find IDs by name: `items.id "example.greeter:Welcome_Cookie"`.

## Scripts

Scripts are Lua 5.4 in a sandbox (internal/script/README.md). There are no globals from the server. Every part of the API comes from `require`.

### Entry scripts and helper modules

- An entry script is a file that `scripts` in mod.yaml lists. It must return a setup function. A file that returns anything else fails to load, with the file name in the error.
- Code outside the setup function runs once per zone, when the file loads. It can `require` modules and look up IDs. Calls that need a context, such as `events.on`, fail there.
- The setup function runs once per zone, in the mod's context. Locals inside it belong to that zone's instance.
- A helper module is any other file the mod loads with `require`. It can return any value. It loads once per zone.

### require names

| Name | Loads |
|---|---|
| `wa.events` | An API module. |
| `example.greeter.lib.messages` | `scripts/lib/messages.lua` of `example.greeter`. |
| `example.seasonal-lib.util` | `scripts/util.lua` of `example.seasonal-lib`, only when it is in `depends`. |

- A name is a mod ID, a dot, and the path under `scripts/` with dots for folders and no `.lua`.
- When several mod IDs match, the longest wins.
- A mod can require its own files and the files of mods in its `depends`. Any other mod's file is an error that names the missing dependency.
- Folder and file names under `scripts/` use letters, digits, `_` and `-`.
- A file of a dependency runs in that dependency's environment. The value it returns is shared with every mod that requires it.

### Isolation

- Each mod has its own global table in each zone. One mod cannot read or change another mod's globals.
- The standard libraries (`string`, `table`, `math`, `utf8`, `coroutine`) are copies per mod. Changing one changes only the mod's own copy.
- API modules, player handles, and event and timer handles are read-only. Writing to them, `rawset` on them, or `setmetatable` on them raises an error.
- `load` runs code in the mod's own environment unless it is given another table.

### The context

The runtime sets a hidden context before it calls a setup function, a handler, or a timer. It owns what they create. When it ends, its handlers and timers go with it. A mod's context ends when the mod is reloaded, or when its handlers fail 10 times in a minute in that zone. Staff can also turn a mod off in a zone, which pauses its code there without ending its contexts ("Mods turned off by staff" below).

```lua
local context = require "wa.context"

return function()
  context:get_kind()     -- "mod"
  context:get_mod()      -- "example.greeter"
  context:get_instance() -- a read-only table: id (the mod ID) and map (the zone's map)
end
```

### Events

```lua
local events = require "wa.events"

return function()
  local handle = events.on("login", function(player) end)
  handle:remove()
end
```

| Event | Handler arguments | When |
|---|---|---|
| `login` | `player` | A player logs in, in the zone it enters. |
| `logout` | `player` | A player logs out, before it leaves. |
| `map_enter` | `player` | A player enters the zone: at login and after a warp. |
| `death` | `death` | A player or a monster died on the context's map ("Death" below). Mod, NPC and event contexts. |
| `base_level_up` | `player` | The player gained a base level. |
| `job_level_up` | `player` | The player gained a job level. |
| `job_change` | `change` | The player changed jobs: `change` is `{ player, from, to }`. It fires after the new job's stats and skills are set ("Jobs" above). Mod and event contexts. |
| `click` | `player`, `conversation` | NPC contexts only: a player talks to the NPC ("NPC scripts" below). |
| `status_start`, `status_end`, `status_tick` | `event` | A status started, ended or ticked on the context's map ("Status effects" below). Mod and event contexts. |
| `skill_used` | `use` | A skill took effect on the context's map, after its cast time ("Skills" below). Mod, event and item contexts. |
| `chat` | `message` | A player on the context's map said a line, after it went out ("Chat" below). Mod, NPC and event contexts. |
| `cast` | `cast` | Skill contexts only: the skill takes effect ("Skills" below). |
| `world_event_start`, `world_event_end` | `event` | A run of a world event started or ended, on any map: `event` is `{ name, type, result }` ("Typed events" below). Mod, NPC and event contexts. |
| `restart_planned` | `restart` | Staff planned a server restart, or cancelled it, in every zone: `restart` is `{ at, reason }` or `{ cancelled = true }` ("Planned restarts" below). Mod, NPC and event contexts. |

Event contexts can listen to the events of mod contexts.

`touch`, for NPCs whose area a player walks into, is a name that registers and does not fire yet.

- `events.on` checks the name. An unknown name, or an event of another context kind, such as `click`, raises an error during setup.
- Handlers run in registration order. A failing handler is logged and does not stop the others.
- A handler that runs over the CPU or memory limit stops with an error. The zone goes on.

### Timers

```lua
local timers = require "wa.timers"

return function()
  local once = timers.after(5000, function() end)
  local repeating = timers.every(60000, function() end)
  once:cancel()
end
```

- Times are in milliseconds of the zone clock. Timers run on the zone's ticks, every 50 ms.
- A zone without players sleeps, and its clock stops. Timers wait while it sleeps.
- A repeating timer that falls behind skips the runs it missed.
- A context can hold 10,000 handlers and timers.

### Players

Handlers get players as read-only handles. Each access looks the player up again. A handle to a player who left the zone raises an error.

| Field | Meaning |
|---|---|
| `kind` | `"player"`. Every entity handle has `kind`. |
| `name`, `job`, `base_level`, `job_level` | The character's name, job ID and levels. |
| `hp`, `max_hp`, `sp`, `max_sp`, `zeny` | Current and maximum HP and SP, and zeny. |
| `map`, `x`, `y` | The zone's map name, such as `prontera`, and the cell. |
| `character_id`, `account_id` | Database IDs, 0 without a database. |
| `party` | `nil`. Parties do not exist yet. |

| Method | Permission |
|---|---|
| `player:send_message(text)` | None. Shows a line in the player's chat log, sent as `CHAT_CHANNEL_SYSTEM`. |
| `player:give_item(item, amount)` | `player.give_items`. Returns `false` when the items do not fit. |
| `player:count_item(item)` | `game.read`. |

### NPC scripts

An NPC's entry script runs its setup function once for each placed NPC that names it, in an NPC context. The NPC is `context:get_instance()`. Locals of the setup function belong to that NPC, as rAthena's `.variables`. A zone starts its NPCs' scripts when the first player enters it.

```lua
local events = require "wa.events"
local emotions = require "wa.emotions"
local context = require "wa.context"

return function()
  local npc = context:get_instance()   -- name, map, x and y
  events.on("click", function(player, conversation)
    conversation:show_image("kafra_01", "right")
    npc:show_emotion(emotions.thanks)
    conversation:say("[" .. npc.name .. "]", "Hello!")
    conversation:wait_for_next()
    if conversation:choose("Save", "Cancel") == 1 then
      player:set_save_point("prontera", 156, 191)
    end
  end)
end
```

A click starts a conversation: the NPC's click handlers run in a thread of their own, which pauses while it waits for the player.

| Call | Meaning |
|---|---|
| `conversation:say(...)` | Shows lines: strings or numbers, up to 64, each at most 1,024 bytes. RO color codes, such as `^FF0000`, color them. |
| `conversation:wait_for_next()` | Waits for the Next button. The next lines replace the shown ones. |
| `conversation:choose(...)` | Shows options and returns the chosen one, from 1. An empty option is hidden and keeps its number. |
| `conversation:ask_number(minimum, maximum)` | Waits for a number in the range, from -2147483648 to 2147483647. |
| `conversation:ask_text(maximum_length)` | Waits for text of 1 to 255 characters at most. It is cleaned as chat is. |
| `conversation:close()` | Shows the Close button and waits for it, as rAthena's `close2`. The handler goes on after it, and later dialog calls raise an error. |
| `conversation:wait(milliseconds)` | Waits up to 10 minutes, as `sleep2`. |
| `conversation:show_image(name, position)` | Shows `image/illust/<name>`, as `cutin`, at `"left"`, `"center"` or `"right"`. |
| `conversation:hide_image()` | Hides it. The client also hides it when the window closes. |
| `conversation:show_progress(seconds)` | Shows a progress bar over the player and waits until it is full, up to 600 seconds. A walk cancels it and ends the conversation. |
| `npc:show_emotion(emotion)` | Shows an emotion over the NPC, from `wa.emotions`, such as `emotions.thanks`. |
| `npc:say_aloud(text)` | Says a line of local chat as the NPC, as `npctalk`. Players within 9 cells hear it. |

Rules, as rAthena's:

- A player clicks NPCs within 15 cells. A dead player, or one already talking, clicks nothing.
- While a player talks, it cannot walk, attack, pick up, use or drop items.
- A step times out after 60 seconds, or 180 seconds for input, and the conversation ends.
- A handler that returns with lines on show leaves the Close button. One that returns without showing anything closes the window.
- When the player leaves, dies, changes map, or the conversation times out, the waiting call raises the error "the conversation was closed". A handler may catch it with `pcall`, but cannot talk any more, and the player is gone when it left. The runtime does not log it as a failure.
- A warp asked for in a conversation happens when the conversation pauses or ends, and ends it.
- A reload ends the mod's NPC contexts, and starts them again with the new files. Conversations that run finish on the old version.

### Saved variables: wa.variables

```lua
local variables = require "wa.variables"

local visits = variables.character_integer "visits"     -- saved with the character
local met = variables.account_boolean "met"             -- shared by the account's characters
local busy = variables.character_temporary_integer "busy" -- until logout
local greetings = variables.server_integer "greetings"  -- the whole world, saved soon after a change
local notes = variables.character_table "notes"         -- plain data

return function()
  events.on("click", function(player, conversation)
    local n = visits:add(player, 1)
    met:set(player, true)
    greetings:add(1)
    notes:set(player, { last = n, seen = { 1, 2, 3 } })
  end)
end
```

- Scopes: `character`, `character_temporary`, `account`, `server`, and `instance`, which raises an error until instances exist. Types: `integer`, `boolean`, `string`, and `table`.
- A declaration names a variable of the running mod. Two mods can use the same name. One mod declares a name with one type.
- `get(player)` and `set(player, value)`, or `get()` and `set(value)` for a server variable. `add(player, amount)` changes an integer in one step and returns the new value.
- Defaults: 0, false, "", and nil for a table. Setting nil, or the default, removes the stored value.
- A string is at most 4,096 bytes. A table holds numbers, booleans, strings and tables, with keys that are all strings, or 1 to n, and is at most 64 KB as JSON. `get` returns a copy.
- Names are 1 to 64 letters, digits and `_`, not starting with a digit.
- Character and account variables are saved with the character, in the same transaction. Two characters of one account that change the same account variable at once: the last save wins.
- Without a database, variables live in memory until the server stops.
- Declaring needs `storage.mod`, which every mod has.

### Shops, save points and announcements

| Call | Permission | Meaning |
|---|---|---|
| `player:open_shop(name)` | None | Opens a shop of `data/npcs.yml` by record name, as `callshop`. A name without a mod ID is the running mod's own shop. `mod:Name` opens another mod's shop. |
| `player:set_save_point(map, x, y)` | `player.warp` | Sets the save point, as `savepoint`. Butterfly Wings and death take the player there. Returns false when the map does not run or the cell is not walkable. |
| `player:can_carry(item, amount)` | `game.read` | `checkweight`: whether the items fit, by weight and slots. |
| `player:pay_zeny(amount)` | None | Takes zeny. Returns false, and takes nothing, when the player has too little. |
| `player:give_zeny(amount)` | `player.give_zeny` | Gives zeny. Zeny over 2,147,483,647 is lost. |
| `player:take_item(item, amount)` | None | `delitem`. Returns false, and takes nothing, when the player has too few. The amount must be 1 or more, or the call raises an error. |
| `player:heal_fully()` | None | Heals all HP and SP. |
| `player:mark_minimap(id, x, y, color)` | None | Marks a cell on the minimap, as `viewpoint`. A mark with the same ID replaces it. |
| `player:unmark_minimap(id)` | None | Removes a mark. |
| `items.name(item)` | `game.read` | The item's name from rAthena's item database. |
| `text.item_link(item)` | None | A link `say` shows as the item's name, as `mesitemlink`. |
| `maps.announce(map, text)` | None | A line for every player of a map, as `mapannounce`. |
| `world.announce(text)` | `world.announce` | A line for every player of the world, as `announce`. |

Player handles also have `sex`: `"male"` or `"female"`.

Shops follow rAthena's `npc_buylist` and `npc_selllist`: a purchase checks the zeny, the weight and the free slots, and a sale checks every slot, before anything changes. Then zeny and items change in one step. Discount and Compulsion Discount lower what the player pays, and Overcharge raises what the player gets.

### Monsters

Monster handles, from `death`, `spawning.monsters` and entity queries, work like player handles. Their fields are read-only; the methods of "Monster AI", below, change what a monster does. Every entity handle has a `kind` field: `"player"`, `"monster"` or `"npc"`, so `death.victim.kind` and `death.killer.kind` say what died and what killed it.

| Field | Meaning |
|---|---|
| `kind` | `"monster"`. |
| `id`, `name`, `level` | The monster's ID, such as 1002, its name and level. |
| `hp`, `max_hp` | Current and maximum HP. |
| `map`, `x`, `y` | The zone's map name and the cell. |
| `is_alive` | `false` for a killed monster. |

- A killed monster is gone from the map, but its handle stays readable until the death event's handlers have run.
- After that, reading the handle raises an error, like a handle to a player who left. A timer that the handler starts cannot read it.
- Two handles to one monster are equal with `==`. A monster that comes back after it died is a new monster, with a new handle.
- Reading needs `game.read`.

### Death

```lua
local events = require "wa.events"

return function()
  events.on("death", function(death)
    if death.killer then
      death.killer:send_message("You defeated " .. death.victim.name .. ".")
    end
  end, { victim_kind = "monster" })
end
```

The death object has `victim` (a player or monster handle), `killer` (a handle, or nil when nothing killed it), `map`, `x`, `y`, and `skill` (nil for a normal attack). It fires for deaths on the context's map. Options narrow it:

| Option | The handler runs for |
|---|---|
| `victim` | One player or monster handle. |
| `victims` | Any handle of a list, such as what `spawning.monsters` returned. |
| `victim_kind` | `"player"` or `"monster"`. |
| `monster` | Monsters of one ID, from `monsters.id`. |
| `map` | One map. A context sees its own map's deaths only. |

### Chat

Chat follows proposal 0002, review decision 26. [examples/town-crier](examples/town-crier) uses each part.

```lua
local channels = require "wa.channels"
local events = require "wa.events"
local hooks = require "wa.hooks"

return function()
  local lounge = channels.create { name = "lounge", display_name = "[Lounge]", color = 0xFFA500, join = "invite" }
  hooks.on("chat", function(message, next)
    local text = next(message)
    if text ~= nil and text:find("https?://") then
      return nil
    end
    return text
  end)
  events.on("chat", function(message)
    if message.text == "!lounge" then
      lounge:add(message.player)
      message.player:whisper("Welcome to the lounge.", { from = "Host" })
    end
  end, { channel = "public" })
end
```

A line goes through these steps:

1. The gateway checks the chat rate limits, which are server settings, with lower limits in a character's first hour.
2. The zone refuses the line of a muted player, with the end time.
3. The server's chat filter replaces or rejects words (`data/chat_filter.yml` above).
4. The `chat` hooks run. Each returns the text, or nil to stop the line. They need `rules.override`, and only compute.
5. The line goes out: to the players in range, the whisper's target on any map, or the channel's members. Players who block the speaker do not see it.
6. The `chat` event runs.

The message of the event and the hook is `{ player, channel, text, to, map }`:

- `channel` is `"public"` for local chat, `"whisper"`, `"party"`, `"guild"`, or a channel's name without its `#`, such as `"trade"`. Guild chat does not exist yet.
- `to` is a whisper's target name, or nil.
- The event's options `channel`, `map` and `player` narrow it. It is a world event: a handler hears every player of its context's map.

| Call | Permission | Meaning |
|---|---|---|
| `player:whisper(text, { from })` | None | Sends the player a whisper. `from` is the name it sees, by default the NPC's name in an NPC context, or the mod's name. It does not pass the filter or the hooks, and blocks do not stop it. `player:send_message` stays for system lines. |
| `player:is_blocking(other)` | `game.read` | Whether the player blocks the other player's name. |
| `player:mute(minutes, reason)` | `player.sanction` | Mutes the player, as rAthena's `SC_NOCHAT`. It replaces a mute that has not ended. |
| `player:jail(minutes, reason)` | `player.sanction` | Jails the player, as rAthena's `pc_jail`: it goes to the jail map and cannot warp until the end, then goes back to where it was. |
| `player:unmute()` | `player.sanction` | Ends a mute. Returns false when the player was not muted. |
| `channels.create { name, display_name, color, join }` | None | Makes a channel that the context owns. |
| `channels.get(name)` | `game.read` | Finds a channel, core's or a mod's, with or without its `#`, or nil. |
| `channel:send(text)` | None | A line of the channel itself, as rAthena's `channelmes`. |
| `channel:add(player)`, `channel:remove(player)` | None | Puts a player in, or takes it out, whatever the channel's rules. They return false when nothing changed. |
| `channel:get_members()` | `game.read` | The members' names, sorted. Members may be on any map. |

- Minutes are 1 to 5,256,000, and reasons 1 to 255 characters. The player sees the reason and the end time. Mutes and jails are saved, and the database keeps every one for the staff tools.
- A channel is one for the whole world. Each zone runs the mod's setup, and every zone's context of the mod shares the channel. It ends when the mod's last context ends, and its members leave it. A name that core or another mod has raises an error.
- Bans and reports are staff tools, not Lua API.
- `player.sanction` is flagged: the server logs it at each start.

### Parties and friends

Parties and friends follow proposal 0002, review decision 27. [examples/party-hall](examples/party-hall) uses each part.

```lua
local events = require "wa.events"
local parties = require "wa.parties"
local rules = require "wa.rules"

return function()
  events.on("chat", function(message)
    if message.text == "!party" and message.player.party == nil then
      local party = parties.create(message.player, message.player.name .. "'s Hall")
      if party ~= nil then
        party:set_options { exp_share = "even" }
      end
    end
  end, { channel = "public" })
  events.on("party_join", function(join)
    local size = rules.limits.party_members
    join.player:whisper("Welcome to " .. join.party.name .. ". It holds up to " .. size .. ".", { from = "Hall" })
  end)
end
```

`player.party` is the player's party, or nil. A party has the read-only fields `id`, `name`, `leader`, `leader_name`, `exp_share` (`"each"` or `"even"`), `item_share` (`"each"` or `"even"`) and `item_pickup` (`"each"` or `"shared"`).

| Call | Permission | Meaning |
|---|---|---|
| `party:get_members()` | `game.read` | The online members on this map, in slot order. |
| `party:get_member_names()` | `game.read` | Every member's name, online or not. |
| `parties.create(leader, name)` | `party.manage` | Makes a party, as `party_create`. Returns nil when the name is taken or the leader has a party. |
| `party:add(player)`, `party:remove(player)` | `party.manage` | As `party_addmember` and `party_delmember`. They return false when nothing changed. Removing the leader breaks the party up. |
| `party:set_leader(player)` | `party.manage` | As `party_changeleader`: the player must be an online member. |
| `party:set_options { exp_share, item_share, item_pickup }` | `party.manage` | As `party_changeoption`. Even exp share still needs the members' levels within `party_share_level`. |
| `party:disband()` | `party.manage` | As `party_destroy`. |
| `parties.get(id)`, `parties.find(name)` | `game.read` | Find a party, or nil. |
| `player:is_friend(other)`, `player:get_friend_names()` | `game.read` | Read the friend list. Only players add and remove friends. |
| `rules.limits.<name>` | None | The world's limits, read-only: `party_members`, `friends` and `block_list`. |

Events:

- `party_join` and `party_leave` get `{ party, player, reason }`. `reason` is `"invited"`, `"create"`, `"left"`, `"expelled"`, `"disbanded"` or `"script"`. They run on the map of the player who joined or left. A member who is offline gets no event. A player who makes a party from the client gets `party_join` with `"create"` (review decision 32).
- `party_change` gets `{ party, leader, options }`, after a change of the leader or the options. It runs on the map where the change was made.
- The `experience` hook's object has `party` (or nil) and `share`, `{ base, job }`: the player's own share of the kill, before the even split and the level penalty. With even exp share, the hook runs once for each member on the map.

Rules:

- A party is one for the whole world, and lives on after its members log out. Its leader and members are handles of players on this map: a member on another map is nil in `party.leader`, and left out of `get_members`.
- The leader leaving breaks the party up, as in rAthena.
- In `wactl mod test`, a test case's own code needs no permission to make and change parties.

### Guilds

Guilds follow proposal 0002, review decision 28. [examples/guild-hall](examples/guild-hall) uses each part.

```lua
local events = require "wa.events"
local guilds = require "wa.guilds"

return function()
  events.on("chat", function(message)
    if message.text == "!guild" and message.player.guild == nil then
      guilds.create(message.player, message.player.name .. "'s Guild")
    end
  end, { channel = "public" })
  events.on("guild_change", function(change)
    if change.change == "level" then
      for _, member in ipairs(change.guild:get_members()) do
        member:send_message(change.guild.name .. " is level " .. change.guild.level .. ".")
      end
    end
  end)
end
```

`player.guild` is the player's guild, or nil. A guild has the read-only fields `id`, `name`, `master` (a player of this map, or nil), `master_name`, `level`, `exp`, `notice` (`{ title, text }`) and `member_count`.

| Call | Permission | Meaning |
|---|---|---|
| `guild:get_members()` | `game.read` | The online members on this map, in slot order. The master is first. |
| `guild:get_member_names()` | `game.read` | Every member's name, online or not. |
| `guild:get_position(player)` | `game.read` | `{ name, can_invite, can_expel, can_use_storage, exp_tax }`, or nil for a player who is not a member. |
| `guild:get_skill_level(skill)` | `game.read` | A guild skill's level, as `getgdskilllv`. Take the ID from `skills.id`. |
| `guild:is_allied(other)`, `guild:is_enemy(other)` | `game.read` | The relation with another guild. Enemies are on one side only. |
| `guilds.create(master, name)` | `guild.manage` | Makes a guild. It takes no Emperium, so the script decides the cost. Returns nil when the name is taken or the master has a guild. |
| `guild:add(player)`, `guild:remove(player)` | `guild.manage` | They return false when nothing changed. The master cannot be removed. |
| `guild:set_master(player)` | `guild.manage` | As `guildchangegm`, without the master change delay. |
| `guild:set_notice(title, text)` | `guild.manage` | Returns false for a title over 59 characters or a text over 119. |
| `guild:give_exp(amount)` | `guild.manage` | As `guildgetexp`. The guild may level up. |
| `guild:disband()` | `guild.manage` | Breaks the guild up with its members. Its storage is lost, as in rAthena. |
| `guilds.get(id)`, `guilds.find(name)` | `game.read` | Find a guild, or nil. |
| `rules.limits.<name>` | None | Also `guild_members`, `guild_positions`, `guild_allies`, `guild_enemies` and `guild_storage`. |

Events:

- `guild_join` and `guild_leave` get `{ guild, player, reason }`, with the reasons of the party events. They run on the map of the player who joined or left. `guilds.create` fires `guild_join` with `"script"` for the master. A player who makes a guild from the client gets `guild_join` with `"create"` (review decision 32).
- `guild_change` gets `{ guild, change }`, where `change` is `"master"`, `"level"`, `"notice"` or `"emblem"`. It runs on the map where the change was made. A new emblem from the account pages runs it on the map of the master, or of the first online member.
- The `experience` hook's object has `guild` (or nil) and `guild_tax`: the share of `base` that the player's position gives the guild. The tax applies at the same rate to the base the hook returns.

Rules:

- A guild is one for the whole world. Its master and members are handles of players on this map.
- Players make guilds from the client with an Emperium, invite by position, and the master changes positions, the notice, the skills and the relations. Guild storage opens from core's Guild Warehouse Manager in renewal towns, and from `player:open_guild_storage()` ("Storage, cart and trade" below).
- In `wactl mod test`, a test case's own code needs no permission to make and change guilds.

### Storage, cart and trade

Storage, carts, bound items and trades follow proposal 0002, review decision 29. [examples/bank](examples/bank) uses each part.

```lua
local events = require "wa.events"
local hooks = require "wa.hooks"

return function()
  hooks.on("trade", function(trade, next)
    if trade.zeny[1] > 100000 or trade.zeny[2] > 100000 then
      return false, "Trades of more than 100000 zeny go through the bank."
    end
    return next(trade)
  end)
  events.on("trade", function(trade)
    for _, p in ipairs(trade.players) do
      p:send_message("Your trade went through.")
    end
  end)
end
```

| Call | Permission | Meaning |
|---|---|---|
| `player:open_storage()` | None | Opens the account's storage, as `openstorage`. Scripts charge any fee. |
| `player:open_storage(name)` | None | Opens an extra storage of `data/storages.yml`, as `openstorage2`. A name without a prefix is the calling mod's record, then core's. Returns false when the player has a storage open, or another character of the account has this one open. |
| `player:open_guild_storage()` | None | Opens the guild storage, as `guildopenstorage`. Returns true, or false and why: `"no_guild"`, `"no_storage"`, `"no_permission"`, `"in_use"` or `"already_open"`. |
| `player:count_item(item, { from = place })` | `game.read` | The amount in a storage or the cart, as `storagecountitem` and `cartcountitem`. nil when it cannot be read: no cart, or a storage window open. |
| `player:give_item(item, amount, { to = place, bound = kind })` | `player.give_items` | Gives items into a storage or the cart, with its restrictions and bound rules. `bound` is `"account"`, `"character"`, `"party"` or `"guild"`, as `getitembound`. |
| `player:take_item(item, amount, { from = place })` | None | Takes items from a storage or the cart, as `storagedelitem` and `cartdelitem`. Takes nothing when there are too few. |
| `guild:count_item(item)` | `game.read` | The amount in the guild storage, as `guildstoragecountitem`. |

A place is `"storage"`, the account's storage, `"cart"`, or an extra storage's name.

Events and hooks:

- `trade` gets `{ players, items, zeny }` after a trade on the context's map completed. `items` lists `{ from, item, amount }`, and `zeny` the zeny each player gave, in the order of `players`. The option `player` narrows it to one player's trades.
- The `trade` hook gets the same object before the trade completes, and returns true, or false and a message that both players see. It needs `rules.override`.

Rules:

- Bound copies cannot be traded, dropped or sold. Storage and carts take account-bound copies, guild storage guild-bound ones, and a trade takes a guild-bound copy to a guildmate. A player who leaves its party loses its party-bound items, and one who leaves its guild puts its guild-bound items into the guild storage.
- A trade needs Basic Skill level 1 with `basic_skill_check`. In `wactl mod test`, give the players `NV_BASIC` before `world:trade`.
- A player who blocks the requester refuses a trade request at once with `block_list_refuses_trade`, which is on by default.
- `wa.testing` has `world:trade(a, b, offer_a, offer_b)`, which runs a whole trade and returns true, or false and why, and `world:close_storage(player)`.

### Rules and limits

Scripts read the world's ruleset, and change the limits one player holds, as proposal 0002, review decision 32. [examples/bank](examples/bank) sells inventory slots, and [examples/classic-casting](examples/classic-casting) reads its flags in a test.

```lua
local rules = require "wa.rules"

-- A bigger bag, up to the ruleset's max.
local function expand_bag(player)
  local _, max = rules.get_limit("inventory_slots")
  if player:expand("inventory_slots", 0) >= max then
    return false
  end
  return player:expand("inventory_slots", 10)
end
```

| Call | Permission | Meaning |
|---|---|---|
| `rules.is(flag)` | None | True when a ruleset flag is on: at its renewal value, or at `on`. `skill_rebalance.<skill>` is the flag the skill follows, its own or the base, for a rebalanced skill or a song. |
| `rules.get(name)` | None | A ruleset rule's value: a boolean for a yes or no rule, else an integer. |
| `rules.get_limit(name)` | None | A limit's base and max, as two values. |
| `rules.limits.<name>` | None | A limit's base. |
| `player:expand(limit, amount)` | `player.expand_limits` | Raises one of the player's limits by amount, up to the limit's max, and returns the new value. A negative amount lowers it, not below the base. 0 returns the value. |

Rules:

- An unknown flag, rule or limit raises an error that lists the valid names.
- `player:expand` takes `inventory_slots`, `storage_slots` and `cart_slots`, from -10,000 to 10,000. `inventory_slots` and `cart_slots` save with the character, and `storage_slots` with the account, whose characters share it.
- A saved value stays within the limit's range: a ruleset that raises the base or lowers the max applies at the next read. Lowering deletes nothing. Slots past the size keep their items and take no new one.
- The player gets the new values at once, and at each login.
- rAthena's inventory expansion uses its own client window and the vouchers' item IDs. wAthena has no such window yet: a mod sells slots from an NPC, as the Bank does.

### Planned restarts

Staff plan server restarts from the admin screens (proposal 0002, review decision 36, server/README.md, "Staff and admin tools"). `restart_planned` tells every zone's mod, NPC and event contexts:

```lua
local events = require "wa.events"
local world = require "wa.world"

return function()
  events.on("restart_planned", function(restart)
    if restart.cancelled then
      return
    end
    -- restart.at is the restart's time in Unix seconds, restart.reason
    -- what staff wrote.
    world.announce("The arena closes before the restart: " .. restart.reason)
  end)
end
```

| Field | Type | Meaning |
|---|---|---|
| `at` | integer or nil | When the server restarts, in Unix seconds. nil for a cancel. |
| `reason` | string or nil | Why, as staff wrote it. nil for a cancel. |
| `cancelled` | `true` or nil | Set when staff cancelled the planned restart. |

Rules:

- It runs once per plan and once per cancel, in every zone, after the plan or the cancel.
- The server announces the restart to players itself, 30, 15, 10, 5 and 1 minute before and 30 seconds before. A mod need not.
- At the restart's time, the server saves every character, disconnects every player and stops. Contexts end as at any stop. Data a mod keeps in `wa.variables` is saved as usual.
- A plan lives only in the running server. A restart before its time drops it without a cancel event.
- In `wactl mod test`, `world:plan_restart(minutes, reason)` fires it ("Testing a mod"). [examples/town-crier](examples/town-crier) tells its channel about a planned restart, and its test plans one.

### Mods turned off by staff

Staff with `mods.disable` turn a mod off in one zone or every zone from the admin screens (review decision 36, [decision 0031](../docs/decisions/0031-world-events-and-mods-tools.md)). It stays off across restarts until it is turned on. Core and its ruleset mods cannot be turned off.

In a zone where a mod is off:

- Its event handlers, timers, hooks, schedules and world event conditions do not run.
- Its NPCs stay on the map and do not answer clicks. A conversation with one of them closes at once, and its script does not go on.
- Its item scripts, skill scripts and monster AI scripts do not start. A usable item whose script is the mod's is not used up.
- A timer that comes due while it is off is skipped: a timer that runs once is dropped, and a repeating one keeps its rhythm.
- Its contexts stay. Its handlers, timers and hooks run again when it is turned on, with the Lua values they had.
- Its data stays: variables, server variables and saved rows are not touched.
- Monsters it spawned stay on the map. Their AI scripts' contexts stay, and run again when it is on.
- Bonuses it already gave stay until they end.
- A world event run of its own goes on in the scheduler. Its event context is skipped in that zone, and ends with the run.

A mod that is off when the zone starts runs no entry script and no NPC setup there. They run when it is turned on. Equipment worn while it is off gives its script's bonuses after it is worn again, and a monster spawned while it is off runs its AI script after it spawns again.

The admin screens count each mod's script errors by zone since the server started: failed handlers, timers, hooks, schedules, clicks and setup functions, with the last error and its time.

### Staff roles

Scripts read a player's staff role, as proposal 0002, review decision 33. The roles come from the server's config and the admin screens (server/README.md, "Staff and admin tools"). [examples/greeter](examples/greeter) has a Staff Desk that talks only to staff.

```lua
local events = require "wa.events"

return function()
  events.on("click", function(player, conversation)
    if not player:has_staff_permission("moderation.mute") then
      conversation:say("This desk is for staff.")
      return
    end
    conversation:say("Welcome back. Your role is " .. player:get_staff_role() .. ".")
  end)
end
```

| Call | Permission | Meaning |
|---|---|---|
| `player:get_staff_role()` | `game.read` | The role's name, such as `"game_master"`, `"owner"` for the config's owners, or nil. |
| `player:has_staff_permission(name)` | `game.read` | Whether the role has a staff permission, such as `"moderation.mute"`. The owner has every permission. |

Rules:

- A role counts only when the player logged in with two-factor login: a TOTP code, a recovery code or a passkey, on an account that has TOTP or a passkey on. Otherwise both calls answer as for a player without a role.
- A role given or taken on the admin screens applies at once.
- The staff permissions are their own list, not mod permissions: `staff.assign`, `audit.read`, `audit.review`, `players.read`, `players.read_ip`, `items.history`, `items.restore`, `items.create`, `moderation.reports`, `moderation.mute`, `moderation.kick`, `moderation.jail`, `moderation.ban`, `moderation.ban_permanent`, `commands.warp`, `commands.recall`, `commands.hide`, `world.read`, `world.announce`, `world.restart`, `events.control`, `mods.read` and `mods.disable`. An unknown name raises an error that lists them. `group.*` is a config form and is not a name here.
- In `wactl mod test`, `world:set_staff_role(player, role)` gives a test player a role of the default roles, `admin`, `game_master` or `moderator`, as if it logged in with two-factor login. nil takes it away. It is provisional.

### Mail, vending, shops and currencies

Mail (RODEX), vending, buying stores, shop changes and currencies follow proposal 0002, review decision 30. [examples/arena-market](examples/arena-market) uses each part.

```lua
local events = require "wa.events"
local hooks = require "wa.hooks"
local world = require "wa.world"

return function()
  hooks.on("vend", function(sale, next)
    if sale.zeny > 1000000 then
      return false, "Sales over a million zeny go through the guild."
    end
    return next(sale)
  end)
  events.on("vend", function(sale)
    sale.seller:give_currency("tokens", 1)
  end)
  events.on("login", function(player)
    player:send_mail({ title = "Welcome", body = "A gift.", items = { { item = 501, amount = 5 } } })
  end)
  world.get_shop("Prize Desk"):set_stock(601, 10)
end
```

| Call | Permission | Meaning |
|---|---|---|
| `player:get_currency(name)` | `game.read` | A currency's amount. An account's currency is its account's. |
| `player:give_currency(name, n)` | `currencies.modify` for another mod's currency | Gives a currency. An amount over its max is lost, as `pc_getcash`. |
| `player:take_currency(name, n)` | `currencies.modify` for another mod's currency | Takes a currency. Returns false, and takes nothing, when the player has too little. |
| `player:send_mail({ to, sender, title, body, zeny, items })` | `mail.send` | Sends a mail, as rAthena's `mail` command. `to` is a name, a character ID or a player handle, the player by default. The recipient may be offline. `items` lists `{ item, amount, roll, bound }`, made for the mail. |
| `player:open_mail()` | None | Opens the mailbox, as `openmail`. Returns false when the player trades, vends, runs a buying store or has a storage open. |
| `player:autotrade()` | None | Closes the connection and keeps the player's store open, as `@autotrade`. Returns false without a store. |
| `player:search_stores({ uses, effect })` | None | Opens the store search, as `searchstores`. `effect` is `"show"` (the default) or `"open"`. |
| `player:open_buying_store({ slots })` | None | Opens the buying store setup with 1 to 5 slots, as `buyingstore`. |
| `world.get_shop(name)` | `game.read` | A shop's handle, or nil. `world:get_shop(name)` works too. |
| `shop:add_item(item, { cost, stock, restock })` | `shops.modify` | Adds or replaces an item, as `npcshopadditem`. |
| `shop:remove_item(item)` | `shops.modify` | Takes an item out, as `npcshopdelitem`. |
| `shop:set_stock(item, n)` | `shops.modify` | Sets an item's stock and its restock amount, -1 for no limit. |

A currency name without a prefix is the calling mod's record, then core's: `cash` and `kafra`. A mod changes its own currencies without a permission.

Events and hooks:

- `shop_buy` and `shop_sell` get `{ player, shop, items, zeny }` after a purchase from a shop of `data/npcs.yml`, or a sale to it, on the context's map.
- `vend` gets `{ seller, buyer, items, zeny }` after a vending sale, and `buying_store_sale` the same after a sale to a buying store: there the seller gives the items and the buyer is the store's owner. `items` lists `{ item, amount, price }`, and `zeny` is what the buyer paid, before the vending tax.
- `mail` gets `{ from, to, items, zeny }` after a mail went out: a player's or a script's. `from` and `to` are names, since the recipient may be offline.
- The `vend`, `buying_store_sale` and `mail` hooks get the same object before the sale or the player's mail, and return true, or false and a message. They need `rules.override`. Script mail skips the mail hook.

Rules:

- A vending shop opens with the Vending skill and a cart. Items with `no_trade`, bound copies and unidentified copies cannot be vended.
- A buying store buys items of rAthena's `BuyingStore` flag that its owner holds one of already. Mod items have no field for the flag yet.
- `wa.testing` has `world:buy(player, shop, items)`, `world:get_mails(player)` and `world:vend(seller, buyer, lines)`.

### Entity queries

```lua
local maps = require "wa.maps"

-- The nearest enemy of the caster within 5 cells of a monster, that is
-- not in the list hit.
local next_target = maps.find_entities {
  around = monster, radius = 5, enemies_of = caster,
  exclude = hit, sort = "nearest", limit = 1,
}[1]

-- The same query, around the monster.
local near = monster:find_nearby { radius = 5, enemies_of = caster, exclude = hit, sort = "nearest", limit = 1 }
```

`maps.find_entities(query)` returns a list of entity handles of the context's map. It needs `game.read`. The query has one area:

| Area | Fields |
|---|---|
| Around an entity | `around` and `radius`. The entity is in its own area: `exclude` leaves it out. |
| Around a cell | `x`, `y` and `radius`. `map` may be given, and must be the context's map. |
| A rectangle | `rectangle = { { x1, y1 }, { x2, y2 } }`, two opposite corners. |
| A line | `line = { from = { x, y }, to = { x, y }, width = 3 }`. `width` is 1 by default. |
| Cells | `cells = { { x, y }, ... }`, up to 1,024. |

- `radius` is 0 to 30 cells. `shape` is `"square"`, the default and rAthena's, or `"circle"`. Both go with an area around an entity or a cell.
- Filters: `kinds` (a list of `"player"`, `"monster"` and `"npc"`), `alive` (true by default; false finds dead entities), `enemies_of` or `allies_of` an entity by rAthena's attack rules, `line_of_sight`, and `exclude`, a list of handles.
- `sort` is `"nearest"`, `"farthest"`, `"lowest_hp"` or `"random"`. `limit` is 1 to 1,000. Ties sort by entity.
- Distances and `line_of_sight` are from the center of the area: the entity, the cell, the middle of the rectangle, the start of the line, or the first cell.
- A query without an area, with two areas, or with a field it does not know raises an error.

Every entity handle also has:

- `entity:find_nearby(query)`: the same query, around the entity. The query cannot name another area.
- `entity:exists()`: false once the entity is removed, has logged out, or has left the zone. It never raises an error, so a timer checks it before it reads a handle.
- `entity:show_effect(effect)`: plays an effect from `wa.effects` on the entity, for everyone near.

### Places

A procedural world's generated maps record named places: cells with tags, such as `blacksmith`, `square`, `gate`, `entrance`, `exit` or `boss_room` (proposal 0002, review decision 42). Content finds places by tag, not by coordinates.

```lua
local places = require "wa.places"

-- Every blacksmith of every town, in a fixed order.
for _, p in ipairs(places.find("town:any", "blacksmith")) do
  local x, y = p:get_position()
  print(p:get_map(), x, y, p:get_area())
end

-- The closest inn to a player: the fewest warps away, then the nearest.
local inn = places.nearest(player, "inn")
```

- `places.find(scope, tag)` returns every place of the scope with the tag. The order is fixed: by map ID, then in the order the generator made them. The scopes are `"town:any"`, `"town:<id>"` such as `"town:t7"`, `"dungeon:any"`, `"dungeon:<id>"` such as `"dungeon:crypt"`, and `"map:<id>"` such as `"map:gen:field:3:-1"`. Another scope raises an error.
- `places.nearest(player, tag)` returns the place with the tag closest to the player: the fewest warps away on the map graph, then the shortest distance. On another map, the distance counts from where the player would arrive. Maps no warp leads to are left out. It returns nil when no place qualifies.
- A place has `get_map()`, the map ID such as `gen:town:t7`, `get_position()`, its x and y, `get_tags()`, its tags with the main tag first, and `get_area()`, its map ID without `gen:`, such as `town:t7`, `dungeon:crypt:2` or `field:3:-1`.
- Both calls need `game.read`. A classic world has no places: `find` returns an empty list and `nearest` nil.

The places of each kind of map:

| Map | Tags |
|---|---|
| Field | `portal` with `portal:south`, `portal:east`, `portal:north` or `portal:west`, and `clearing` |
| Dungeon floor | `entrance`, `exit` (not on the last floor), `boss_room` (the last floor only), and `room` |
| Town | `square`, `gate` and `portal` with their side, such as `gate:north`, and the `places` of the template's buildings |

Town IDs are `t1`, `t2` and on. A dungeon's ID is its style's name without the mod prefix, such as `crypt`, then `crypt-2` and on for more of one style. A mod's home map has its own `places.yaml`, and its scope is `"map:gen:home:<id>"`.

- `place:get_band()` returns the lowest and highest level of the place's map (review decision 43).

### Towns

A procedural world's layout has towns, each with an ID such as `t7`, a display name from its naming style, a map, and a level band that rises with the warps from the home area (proposal 0002, review decision 43, decision 0041).

```lua
local towns = require "wa.towns"
local maps = require "wa.maps"

for _, town in ipairs(towns.list { visited_by = player }) do
  local lo, hi = town:get_band()
  print(town:get_id(), town:get_name(), town:get_map(), lo, hi)
end
local here = towns.nearest(player)
print(player:has_visited(towns.get("t2")), maps.get_display_name(player.map))
```

- `towns.list()` returns every town by number, and `towns.list { visited_by = player }` the towns the player entered.
- `towns.nearest(player)` returns the town the fewest warps from the player's map, ties to the lower number, or nil.
- `towns.get(id)` returns the town with an ID such as `"t7"`, or nil. Another form raises an error.
- A town has `get_id()`, `get_name()`, `get_map()` and `get_band()`. Two handles of one town are equal.
- `player:has_visited(town)` tells whether the player entered the town.
- `maps.get_display_name(map)` returns a generated map's display name, such as `"North Valheim Fields 2"`, or nil for another map. A name changes when staff rename the town with `@renametown`.
- Every call needs `game.read`. A classic world has no towns: `list` returns an empty list, `nearest` and `get` nil, and display names nil.
- The teleport service of each town is the server's, not a script's, and mods cannot change it yet.

### Quests

Each character has a quest log, saved with it (proposal 0002, review decision 45, decision 0043). [examples/hunting-board](examples/hunting-board) uses each part.

```lua
local events = require "wa.events"

return function()
  events.on("click", function(player, conversation)
    if player:get_quest_state("porings") == "none" then
      player:add_quest("porings")
      return
    end
    local count, goal = player:get_quest_progress("porings", 1)
    if count >= goal and not player:quest_timed_out("porings") then
      player:complete_quest("porings")
    end
  end)
  events.on("quest_progress", function(event)
    event.player:send_message(event.count .. " of " .. event.goal)
  end)
end
```

| Call | Permission | Meaning |
|---|---|---|
| `player:add_quest(quest)` | None | Adds the quest as active, with its time limit from now, as `setquest`. Returns false when the log has it already, in any state. |
| `player:complete_quest(quest)` | None | Marks an active or inactive quest complete, as `completequest`. Returns false when the log lacks it or holds it as complete. |
| `player:remove_quest(quest)` | None | Takes the quest out of the log, in any state, as `erasequest`. |
| `player:change_quest(from, to)` | None | Replaces an active or inactive quest with a new active one, as `changequest`. Returns false when the log lacks `from`, holds it as complete, or has `to` already. |
| `player:get_quest_state(quest)` | `game.read` | `"none"`, `"active"`, `"inactive"` or `"complete"`. |
| `player:get_quest_progress(quest, objective)` | `game.read` | The kills of a hunt objective, from 1, and the kills it asks for, or nil. |
| `player:quest_timed_out(quest)` | `game.read` | Whether the quest's time limit passed. False without a limit. |
| `npc:show_quest_icon(player, icon)` | None | Shows an icon over the NPC to one player, as `showevent`, or removes it with nil. |

- A quest is an ID, such as `7104`, or a record name: a full name, or a plain name, which is the calling mod's record first, then core's. An unknown quest raises an error.
- The icons are rAthena's: `quest` and `quest_question` (the "!" and "?" marks), `job`, `job_question`, `event`, `event_question`, `warg` and `warg_face`. An icon stays while the player is on the map, and shows again when the NPC comes back into view. A map change drops it.
- A kill counts for the quests of the player that killed the monster. When that player has a party, every member on the map within 14 cells of the monster counts it instead, dead or not, as rAthena. Counts stop at the goal. Complete quests count nothing.
- The player marks quests active or inactive in the quest window. Inactive quests still count kills, as in rAthena.
- The events `quest_add` (a new quest, also the new quest of `change_quest`), `quest_progress` (once per objective a kill raised) and `quest_complete` get `{ player, quest, id }`, where `quest` is the record's full name and `id` its ID. `quest_progress` adds `objective`, `count` and `goal`. They run in mod, NPC and event contexts of the player's map.
- A log entry whose record is gone, such as a removed mod's quest, stays saved and hidden, and comes back with the mod.

### Spawning monsters

```lua
local spawning = require "wa.spawning"

return function()
  -- 15 Pumpkin Porings that come back after they die, while the mod runs.
  spawning.monsters { monster = PUMPKIN_PORING, amount = 15, respawn = true }
  -- 3 near a cell, that stay dead.
  local guards = spawning.monsters { monster = PORING, amount = 3, x = 150, y = 180, area = 5 }
end
```

- `spawning.monsters { map, monster, amount, x, y, area, respawn }` places monsters on the context's map and returns their handles. `map` may be left out. Another map raises an error, so a mod script, which runs in every zone, checks `context:get_instance().map` first.
- Without `x` and `y`, each monster lands on a random walkable cell. `area` spreads them within that many cells of `x`, `y`.
- `amount` is 1 to 1,000. By default a dead monster stays dead. `respawn = true` makes it come back as a spawn line's monster does in rAthena: after `respawn_milliseconds` (1,000 or more), or after 5,000 ms without it, rAthena's delay for a spawn line that gives none. Monster data has no respawn time, so the delay belongs to the call.
- The context owns the monsters: they go when it ends, such as when an event's run ends or the mod reloads.
- It needs `npc.create`, which every mod has.

### Schedules

```lua
local schedules = require "wa.schedules"

return function()
  schedules.daily("00:00", function() end)
  schedules.weekly("saturday", "20:00", function() end)
  local s = schedules.hourly(30, function() end)
  s:cancel()
  schedules.when(function() return #players_here() == 0 end, function() end, { lasting_minutes = 60 })
end
```

- Schedules follow the server clock, in the server's time zone. A time that does not exist on a day clocks change runs at the next valid minute. A time that happens twice runs once.
- A mod script runs in every zone, so its schedules run in every zone. Check `context:get_instance().map` to run on one map.
- A sleeping zone wakes for its schedules. A time that passed more than once, such as while the server was stopped, runs at most once.
- `schedules.when` runs its condition once a minute, at the start of each minute. The handler runs once the condition has held for `lasting_minutes`, once for each time it holds. The condition only reads the game: a call that changes it raises an error.
- The context owns its schedules. Each returns a handle with `cancel()`. They need no permission.

### Hooks

```lua
local hooks = require "wa.hooks"

return function()
  hooks.on("drop_rate", function(drop, next)
    local rate = next(drop)
    if drop.monster.id == PUMPKIN_PORING then
      return rate * 2
    end
    return rate
  end, { priority = 10 })
end
```

| Hook | Object | Returns |
|---|---|---|
| `drop_rate` | `drop`: `player` (the killer, or nil), `monster`, `item`, `map`, `rate` (rAthena's rate, per 10,000) | The rate, kept to 0 to 10,000. |
| `experience` | `exp`: `player`, `monster`, `base`, `job` (the exp the player gets, after the shares and the level penalty) | `base, job`. Below 0 counts as 0. |
| `status_chance` | `start`: `target`, `status`, `level`, `source`, `chance` (rAthena's chance after resistance, per 10,000) | The chance, kept to 0 to 10,000. |
| `status_duration` | `start`: `target`, `status`, `level`, `source`, `milliseconds` (rAthena's duration after resistance) | The duration, kept to 1 to 2,147,483,647. |
| `skill_damage` | `hook`: `caster`, `target`, `skill`, `level`, `percent` (the skill's damage percentage) | The percentage, kept to 0 to 1,000,000. |
| `cast_time` | `hook`: `caster`, `target` (nil for a ground skill), `skill`, `level`, `milliseconds` (rAthena's cast time) | The cast time, kept to 0 to one hour. |
| `skill_cost` | `hook`: `caster`, `skill`, `level`, `sp` (rAthena's SP cost) | The SP cost. Below 0 counts as 0. |
| `cast_allowed` | `hook`: `caster`, `target`, `skill`, `level`, `allowed` (whether rAthena lets the cast start) | `true` or `false`. |
| `monster_target` | `choice`: `monster`, `candidates` (the units it may take), `current` (rAthena's choice, or nil), `reason` (`"aggressive"`, `"assist"`, `"hit"` or `"change"`) | One of `candidates`, or nil for none. Anything else counts as a failure of the hook. |
| `item_roll` | `roll`: `item`, `name` (the roll), `min`, `max`, `default`, `source` (`{ kind = "drop", monster }`, `{ kind = "script" }` or `{ kind = "shop" }`) | The value, clamped to the roll's range. The innermost result is the server's roll ("Rolled item values" below). |
| `chat` | `message`: `player`, `channel`, `text`, `to`, `map` ("Chat" below) | The text, or nil to stop the line. It runs after the server's chat filter. |
| `spawn_table` | `map`, `spawns` and `next` ("The spawn_table hook" below) | The spawn list. |

- Hooks run in a chain. Higher priority runs first; equal priorities run in the order they were added. `next(object)` returns the rest of the chain's result. The innermost result is rAthena's value.
- To pass a changed object, pass a table with the same fields to `next`.
- A hook only computes: a call that changes the game raises an error in it. A hook that fails is logged and left out.
- Hooks run in the zone of the kill. A hook of a mod script runs in every zone. A hook of an event run changes kills on the event's map, until the run ends.
- `hooks.on` needs `rules.override`, which the server owner approves and the server logs at each start, and `spawn_table` needs `spawns.modify` instead. It returns a handle with `remove()`.

#### The spawn_table hook

```lua
local hooks = require "wa.hooks"
local monsters = require "wa.monsters"

local MOSS_PORING = monsters.id "example.mossy-fields:MOSS_PORING"

return function()
  hooks.on("spawn_table", function(map, spawns, next)
    local list = next(spawns)
    if map.kind == "field" and map.band[1] < 20 then
      table.insert(list, { monster = MOSS_PORING, count = 12, respawn = 5000, role = "common" })
    end
    return list
  end)
end
```

- It runs only when `wactl world new` makes a procedural world, once per field and dungeon floor, in a Lua state that ran the mods' entry scripts on no map. A server never runs it, since a made world keeps its spawns. In `wactl mod test`, it runs when a case first runs on a generated field or floor.
- `map` is read-only: `id`, `kind` (`"field"` or `"dungeon"`), `biome`, `band` (`{ lowest, highest }`) and `features` (`water`, `clearing`, `room` and `boss_room`).
- `spawns` is a copy the hook may change: a list of at most 64 `{ monster, count, respawn, role, area }`. `monster` is from `monsters.id`, `count` 1 to 1,000, `respawn` the milliseconds before a dead monster comes back, 1,000 or more, `role` a role, and `area` `{ x, y, w, h }`, within `w` and `h` cells of `x` and `y`, or nil for the whole map.
- `next(spawns)` runs the rest of the chain on a list, the hook's own when left out, and returns its list. The innermost list is the one the world drew.
- A list with a bad entry fails the hook, which is left out. The server keeps its own rules over the result: aggressive monsters stay 15 cells from arrivals and bosses 30 cells from warps to towns, and a boss as `common` or `uncommon` is left out with a warning.
- It needs `spawns.modify`, and a mod context.

### Status effects

Every entity handle has the status methods of proposal 0002, review decision 19: players, monsters and NPCs.

```lua
local statuses = require "wa.statuses"

player:start_status(statuses.blessing, { milliseconds = 60000, level = 10 })
if player:has_status(statuses.poison) then
  player:end_status(statuses.poison)
end
local s = player:get_status(statuses.blessing)
-- s.status, s.level, s.values (val2 to val4), s.remaining_milliseconds, s.source
```

`start_status` options:

| Option | Default | Meaning |
|---|---|---|
| `milliseconds` | The duration rAthena's `sc_start` looks up for the level | 1 to 2,147,483,647, or `"forever"` (`INFINITE_TICK`). |
| `level` | 1 | val1. |
| `values` | `{}` | val2 to val4, for the statuses that read them. |
| `chance` | 10000 | Per 10,000, before resistance. |
| `source` | None | The entity that caused it. Its stats and level count for resistance. Without it, the target's own count, as `sc_start`. |
| `cannot_be_avoided` | false | `SCSTART_NOAVOID`: no resistance, no roll, no boss immunity. |
| `keep_duration` | false | `SCSTART_NOTICKDEF`: resistance does not shorten it. |
| `keep_chance` | false | `SCSTART_NORATEDEF`: resistance does not lower the chance. |
| `hide_icon` | false | `SCSTART_NOICON`. |

- `start_status` returns true when the status starts. rAthena's rules apply: a weaker copy of an active status does nothing and returns true, and a status that fails its roll returns false.
- `end_status` returns true when the status was active. `get_statuses()` lists every active status, by status.
- A player's stats change before `start_status` or `end_status` returns, so the script reads the new values. Events of the status run after the script's call.
- Converted item scripts set `keep_duration`, as rAthena's `sc_start` does in item scripts. Converted NPC scripts set `cannot_be_avoided`.

Events of mod and event contexts, with the options `status`, `target_kind` and `map`:

```lua
events.on("status_end", function(event)
  -- event.target, event.status, event.level, event.source,
  -- event.milliseconds (the time left), event.reason
end, { status = statuses.poison, target_kind = "player" })
```

- `status_start`: `reason` is `"skill"`, `"item"`, `"bonus"` (a proc such as `cause_status`), `"script"`, or `"login"` for a status restored from a save. `milliseconds` is the duration.
- `status_end`: `reason` is `"expired"`, `"removed"`, `"replaced"`, `"death"`, `"dispell"`, `"clearance"`, `"logout"` or `"damage"`.
- `status_tick`: each tick of a status that ticks, such as poison, and each tick of a mod status. It has no `reason`.
- The events of statuses that a script call starts or ends run after that call returns.

A mod script defines new statuses with `statuses.define` in its setup function:

```lua
statuses.define {
  name = "verve",
  icon = 241, -- a status icon (EFST_ ID) the client has
  kind = "buff",
  ends_on = { "death", "dispell" },
  bonuses = function(level)
    return { { "max_hp", 100 * level } }
  end,
  tick_milliseconds = 5000,
}
-- statuses.verve is now its ID.
```

- The name is 1 to 48 lower case letters, digits and `_`. A name that a status of the game or of another mod has raises an error.
- `bonuses` returns bonus entries, as `add_timed_bonuses` takes them. They apply to players while the status lasts. A status restored at login runs it again.
- With `tick_milliseconds`, `status_tick` fires at that interval, and once more when the status runs out.
- Mod statuses get IDs from 100,000. A saved status keeps its name, so it comes back when its ID changes.

The sample mod in [examples/verve](examples/verve) defines a status, listens to its ticks, and has an item that starts it.

### Timed bonuses

```lua
player:add_timed_bonuses("tonic", 60000, { { "str", 5 }, { "max_hp", 100 } },
  { ends_on = { "death" }, icon = 241, kind = "buff" })
```

- The bonuses last the time given. The player's stats change before the call returns.
- The option `on_repeat` says what a call does when the player has timed bonuses with the same key:
  - `"keep"`, the default and rAthena's: the new bonuses are ignored.
  - `"extend"`: the first copy keeps its values and takes the new duration when it is longer.
  - `"add"`: both copies apply.
  - `"replace"`: the copies end, and the new one starts with its values and duration.
- The method returns true when the bonuses were added or extended, and false when they were ignored.
- `ends_on` lists what also ends them: `death`, `dispell`, `clearance`, `logout`, `banishing_buster`, `refresh`, `lux_anima`, `magic_gear_change` and `damage`.
- They are saved across logout, unless `ends_on` has `logout`.
- Converted `bonus_script` lines key the bonuses by the script's text. Their flags 0x400 and 0x800 become `"extend"` and `"add"`.

### Skills

A skill whose data has `script` runs that entry script (proposal 0002, review decision 20). Its setup function runs in a context of kind `"skill"`.

```lua
local events = require "wa.events"

return function()
  events.on("cast", function(cast)
    local result = cast:deal_damage(cast.target, { percent = 150 })
    if result ~= nil and not result.missed then
      cast:heal(cast.caster, result.amount // 10)
    end
  end)
end
```

- The setup function of an active skill runs once per zone, at the skill's first cast. `context:get_instance()` is `{ skill }`. The context ends when the mod reloads.
- The `cast` event runs when the skill takes effect, after the cast time. A skill whose script has a `cast` handler runs it instead of core's behavior of the skill.
- The cast object has `caster`, `skill`, `level`, `target` (nil for a ground skill), `map`, `x` and `y`. It stays usable after the event, so a timer can use it.

| Method | What it does |
|---|---|
| `cast:deal_damage(target, options)` | rAthena's damage calculation of the skill, with `percent` as the skill's damage percentage (100 by default), after the `skill_damage` hooks. It applies cards and defense, and fires the `damage` event. The options `hits`, `type` (`"weapon"`, `"magic"` or `"misc"`) and `element` replace the data's values. It returns `{ amount, is_critical, missed }`. |
| `cast:heal(target, amount)` | Heals with rAthena's heal bonuses, and returns the HP healed. |
| `cast:run_default(options)` | Runs core's behavior of the skill, with `target` to change the target. A skill without one, such as a mod's skill, raises an error. |

- When the caster is gone or cannot act, `deal_damage` and `heal` do nothing and return nil, as rAthena's `skill_addtimerskill`. A chain of timers stops there.
- The `damage` and `death` events that a call causes run after the script's call.

A passive skill's script runs while a player has the skill learned. Each player and skill has its own context, whose `context:get_instance()` is `{ skill, level, owner }`. When the level changes, the context ends and a new one starts with the new level. It ends when the player unlearns the skill or leaves the zone.

The `skill_used` event of mod and event contexts fires for every skill that takes effect on the context's map. An interrupted cast does not fire it. `use` has `caster`, `skill`, `level`, `target` (nil for a ground skill), `x` and `y`. Options narrow it:

```lua
events.on("skill_used", function(use)
  use.caster:send_message("You cast " .. use.level .. " levels of Chain Lightning.")
end, { skill = CHAIN_LIGHTNING, caster_kind = "player" })
```

| Option | The handler runs for |
|---|---|
| `skill` | One skill, from `skills.id`. |
| `caster_kind` | `"player"`, `"monster"` or `"npc"`. |
| `map` | One map. |

The sample mod in [examples/chain-lightning](examples/chain-lightning) adds a skill, puts it in the Mage's tree, and makes its bolt jump between enemies with `maps.find_entities`.

### Job changes and resets

The job calls follow proposal 0002, review decision 21, and rAthena's `pc_jobchange`, `pc_resetskill`, `pc_resetstate` and its rebirth NPC.

```lua
local jobs = require "wa.jobs"

-- A Swordman at job level 40 picks a second class.
for _, job in ipairs(jobs.next_jobs(player.job)) do
  -- jobs.knight, jobs.crusader
end
if player:change_job(jobs.knight) then
  player:show_effect(effects.angel)
end
```

| Call | What it does |
|---|---|
| `jobs.id(name)` | A job by its name in `wa.jobs`, such as `"knight"`, or a mod job's full name. |
| `jobs.next_jobs(job)` | The jobs a job can change into, in job ID order: the jobs whose record lists it in `changes_from`. In core's records a Novice changes into a first class or a Super Novice, a High Novice into a transcendent first class, and a first class into its two second classes. Bard and Dancer are both listed, and so are Clown and Gypsy. Other core jobs have none until third classes exist. |
| `player:change_job(job, { force })` | Changes the job, as `jobchange`. Job level goes back to 1, job exp to 0, and learned skills stay. A male player who asks for a Dancer becomes a Bard, and the reverse. Returns false, and changes nothing, when the job is not a next job of the player's, when the player lacks the levels of the job's `changes_from`, or when a transcendent player asks for a job outside the line it had before its rebirth. `force = true` skips these checks and needs `player.change_job`. The `job_change` event follows. |
| `player:rebirth()` | A second class at base level 99 and job level 50 or more becomes a High Novice at base and job level 1, with every stat at 1, 100 status points, First Aid and Trick Dead, as the rebirth NPC's `jobchange Job_Novice_High` and `resetlvl(1)`. Its second class picks its transcendent line. Returns false for other players. |
| `player:reset_skills()` | Every skill learned with skill points goes back to 0, and the points come back, as `resetskill`. |
| `player:reset_stats()` | Every stat goes back to 1, and the status points become those of the base level, 52 more for a transcendent job, as `resetstatus`. |
| `player:reset_skill(skill, { to_level, with_dependents })` | Lowers one skill, to 0 by default, and refunds its points. Returns false, and changes nothing, when a learned skill needs a higher level of it, unless `with_dependents = true`, which lowers those skills to 0 too and refunds them. |
| `player:reset_stat(stat, { to })` | Lowers one stat (`"str"` to `"luk"`), to 1 by default, and refunds what raising it cost, by rAthena's cost table: `2 + (value - 1) / 10` points per step below 100 in renewal, and `1 + (value + 9) / 10` in pre-renewal. Returns false when the stat is not above the value. |

- Quest skills, which cost no skill points, are never lowered or refunded. Basic Skill stays for every job but the Novice and the High Novice, as in `resetskill`.
- The changes follow `pc_jobchange`: a base level above the new job's maximum comes down and the stats reset; a change into a transcendent job adds 52 status points, and a change out of one takes them back or resets the stats; equipment the new job cannot wear comes off; and a companion, a mount or a cart goes when the new job does not meet its `requires` (see "Companions, mounts and carts").
- The job level at the change from a first class to a second class is saved (rAthena's `change_level_2nd`). The skill tree reads it: a second class sees its own skills once it spent the points of that many first class job levels.
- Every call computes the player's stats before it returns, and sends the new skill list.
- The job tests of `wa.jobs` (`is_transcendent`, `base_job`, `first_job` and the others) work with mod jobs. `first_job` of a mod job is the first job of its class's line.

Core places a Job Master in Prontera (`core:Job Master#prontera`). It changes a Novice or a High Novice at job level 10 with Basic Skill 9 into a first class (a Super Novice needs base level 45 too), a first class at job level 40 into a second class, and gives a rebirth to a second class at base level 99 and job level 50. rAthena's job change quests mostly do not convert yet: the coverage reports in [docs/generated](../docs/generated) count them as "Job change NPCs".

The sample mod in [examples/job-trainer](examples/job-trainer) adds a trainer that lowers one skill or one stat for a fee, and its tests walk the Job Master's paths.

### Monster AI

Monsters run rAthena's AI ([decision 0013](../docs/decisions/0013-monster-ai.md)). A monster record's `ai` is rAthena's AI type, which is a set of modes:

| AI | Modes |
|---:|---|
| 01 | `can_move`, `can_attack` |
| 02 | `can_move`, `looter`, `can_attack` |
| 03 | `can_move`, `assist`, `can_attack`, `change_target_melee` |
| 04 | `can_move`, `aggressive`, `can_attack`, `angry`, `change_target_melee`, `change_target_chase` |
| 05 | `can_move`, `aggressive`, `can_attack`, `change_target_chase` |
| 06 | none (a plant) |
| 07 | `can_move`, `looter`, `assist`, `can_attack`, `change_target_melee` |
| 08 | `can_move`, `aggressive`, `can_attack`, `change_target_melee`, `change_target_chase`, `target_weak` |
| 09 | `can_move`, `aggressive`, `cast_sensor_idle`, `can_attack`, `change_target_melee`, `change_target_chase` |
| 10 | `aggressive`, `can_attack` |
| 11 | `aggressive`, `can_attack` |
| 12 | `can_move`, `aggressive`, `can_attack`, `change_target_chase` |
| 13 | `can_move`, `aggressive`, `assist`, `can_attack`, `change_target_melee`, `change_target_chase` |
| 17 | `can_move`, `cast_sensor_idle`, `can_attack` |
| 19 | `can_move`, `aggressive`, `cast_sensor_idle`, `can_attack`, `change_target_melee`, `change_target_chase` |
| 20 | `can_move`, `aggressive`, `cast_sensor_idle`, `can_attack`, `cast_sensor_chase`, `change_target_melee`, `change_target_chase` |
| 21 | `can_move`, `aggressive`, `cast_sensor_idle`, `can_attack`, `cast_sensor_chase`, `change_chase`, `change_target_melee`, `change_target_chase` |
| 24 | `can_move`, `no_random_walk`, `can_attack` |
| 25 | `can_move` |
| 26 | `can_move`, `aggressive`, `cast_sensor_idle`, `can_attack`, `cast_sensor_chase`, `change_chase`, `change_target_melee`, `change_target_chase`, `random_target` |
| 27 | `aggressive`, `can_attack`, `random_target` |

Types 14 to 16, 18, 22 and 23 do not exist in rAthena. `modes` adds to the modes of the type.

Monster handles have these fields and methods:

| Field or method | Meaning |
|---|---|
| `target` | The unit its AI follows, or nil. |
| `state` | Its state in rAthena's words: `idle`, `walk`, `loot`, `dead`, `attack`, `angry`, `chase` or `follow`. |
| `master` | The unit that summoned it as a slave, or nil. |
| `monster:set_target(unit)` | It takes the unit as target and chases it, as `unitattack`. Returns false when it cannot target it. |
| `monster:clear_target()` | It drops its target and goes idle, as `mob_unlocktarget`. |
| `monster:walk_to(x, y)` | It walks to the cell, as `unitwalk`. Its AI waits until it gets there. Returns false when it cannot walk there. |
| `monster:use_skill(skill, level, target)` | It uses the skill with the skill's own cast time, as `unitskilluseid`. A ground skill goes on the target's cell. No delay of its skills list applies. Returns false when the cast does not start. |
| `monster:talk(text)` | It says a line to the players near it, after its name. |
| `monster:stop()` | It stops walking and attacking, and drops its target. |

- The methods change the game, so they cannot run in a hook. They raise an error once the monster is gone.
- The `monster_target` hook (see "Hooks") changes the targets the AI picks. Its innermost result is rAthena's choice.

A monster record's `script` names an AI script. Its setup function runs once for each monster of the record that spawns, in a context of kind `"monster"`. `context:get_instance()` returns the monster. The context ends, with its timers, when the monster dies or leaves the map, or the mod reloads.

```lua
-- scripts/monsters/alpha.lua
local context = require "wa.context"
local timers = require "wa.timers"

return function()
  local alpha = context:get_instance()
  timers.every(1000, function()
    if alpha.target ~= nil and alpha.hp < alpha.max_hp / 2 then
      alpha:talk("To me, my pack!")
    end
  end)
end
```

In mod tests, `world:advance` runs the AI, and tests read `monster.target` and `monster.state`. [examples/wolf-pack](examples/wolf-pack) has an Alpha Wolf that calls its pack with a skill and sends it after its prey, and a hook that keeps the wolves away from Novices.

### Monster skills

A monster record's `skills` is rAthena's `mob_skill_db.txt` with readable names. Core's monsters have the lists of the mode. Each entry:

| Field | Meaning |
|---|---|
| `skill` | The skill, by name, such as `NPC_SUMMONSLAVE` or `MG_FIREBOLT`. |
| `level` | The skill level. For a summon, how many monsters it calls. |
| `state` | When the entry is checked: `any`, `idle`, `walk`, `loot`, `dead`, `attack`, `angry`, `chase`, `follow` or `anytarget`. Default `any`. |
| `rate` | The chance per 10,000 each check. Default 10,000. |
| `cast_time` | The cast time in milliseconds, which replaces the skill's own. |
| `delay` | The milliseconds before the entry can be used again. |
| `cancelable` | Whether a hit stops the cast. |
| `target` | `target`, `random_target`, `self`, `friend`, `master`, or `around1` to `around8`. Default `target`. |
| `condition` | One of the conditions below. Default `always`. |
| `condition_value` | The condition's value, such as a percent of HP or a status name. |
| `summons` | Monster names for summon and class change skills. |
| `emotion` | An emotion of `wa.emotions`, such as `surprise`, shown when the skill takes effect. A list of two, such as `[kik, surprise]`, shows both in turn, as rAthena's `NPC_EMOTION` with an emotion in its `Emotion` column. |
| `modes` | Only for `NPC_EMOTION` and `NPC_EMOTION_ON`: a list of mode names, as the monster's `modes`. The change is the status `SC_MODECHANGE`, which lasts until the monster dies. See below. |
| `chat` | A line the monster says when it uses the entry. |
| `chat_color` | The line's color as `0xRRGGBB`, from `0x000001` to `0xFFFFFF`, such as `0xFF8000`, as the `Color` of rAthena's `mob_chat_db.yml`. Default `0xFF0000`. |

Conditions, one to one with rAthena's, from `always` to `trickcasting`: `always`, `hp_below`, `hp_between`, `friend_hp_below`, `friend_hp_between`, `has_status`, `lacks_status`, `friend_has_status`, `friend_lacks_status`, `attackers_above`, `attackers_at_least`, `slaves_below`, `slaves_at_most`, `hit_melee`, `hit_ranged`, `hit_by_skill`, `after_skill`, `targeted_by_cast`, `target_casting`, `hit_while_stuck`, `master_hp_below`, `master_attacked`, `spawned`, `summoned_by_alchemist`, `monsters_nearby_above`, `hit_by_ground_skill` and `damage_above`.

- Monsters use player skills through the skill framework, and the `NPC_` skills that only monsters use. [docs/generated/monster-skills-renewal.md](../docs/generated/monster-skills-renewal.md) and its pre-renewal twin list the rows of core's lists and the parts that do not convert.
- A list has at most 50 entries, as rAthena's `MAX_MOBSKILL`.
- `modes` changes the monster's modes, as rAthena's `NPC_EMOTION` and `NPC_EMOTION_ON`:
  - `NPC_EMOTION` sets the monster's AI modes, from `can_move` to `random_target`, to the list. The other modes stay. A monster that is attacking or chasing drops its target.
  - `NPC_EMOTION` with `modes: []` ends the change, so the monster has its own modes again.
  - `NPC_EMOTION_ON` adds the list to the monster's modes. A list without `aggressive` takes `aggressive` away.
  - A change piles up with the one before it, as `SC_MODECHANGE` in rAthena. A change that leaves the monster's own modes ends the one before it.

```yaml
# A mod's skills for core's Wolf.
- name: WOLF
  skills:
    - {skill: NPC_EMOTION, state: chase, rate: 2000, delay: 5000, target: self, emotion: [kik, surprise], modes: [can_move, can_attack]}
    - {skill: NPC_EMOTION, state: idle, rate: 2000, delay: 5000, target: self, emotion: hng, modes: []}
    - {skill: NPC_EMOTION_ON, state: attack, delay: 30000, target: self, emotion: fret, modes: [can_move, aggressive, can_attack, angry]}
    - {skill: AL_HEAL, state: any, rate: 500, target: self, condition: hp_below, condition_value: 30, chat: "Awoo!", chat_color: 0xFF8000}
```

### Rolled item values

An item can declare values that each new copy rolls ([proposal 0002](../docs/proposals/0002-lua-api.md), review decision 23, and strategy section 5.4). [examples/lucky-clover](examples/lucky-clover) uses each part.

```yaml
- name: example.lucky-clover:Clover_Charm
  type: Armor
  location: [both_accessory]
  rolls:
    luck: {min: 1, max: 5, default: 3, distribution: uniform}
    fortune:
      display_name: Clover fortune
      default: 1
      distribution: weighted
      hidden: true
      weights:
        - {value: 1, weight: 70}
        - {value: [2, 4], weight: 25}
        - {value: 5, weight: 5}
  bonuses:
    - [luk, {roll: luck}]
    - [crit, {roll: fortune}]
```

- `rolls` maps a roll's name to `min`, `max`, `default`, `distribution`, `hidden` and `display_name`. A name is lower-case letters, digits and `_`. An item has at most 8 rolls. Values are whole numbers that fit in 32 bits.
- `distribution` is `uniform`, `triangular` (the default) or `weighted`.
  - `uniform`: each value from `min` to `max` is as likely.
  - `triangular`: a value's chance falls in a straight line from `default` to one past each end, so values near the default are common and `min` and `max` are rare.
  - `weighted`: `weights` is a list of `{ value, weight }`, where `value` is one number or `[min, max]`. A tier is picked by weight, then a value of its range. `min` and `max` may be left out: they are the lowest and highest values of the tiers.
- `hidden: true` keeps the value from the client until the copy is identified. A copy that a drop or a random give rolls starts unidentified when its item has a hidden roll. The client shows "?" in place of the value. Identifying does not exist yet ("Left" in [decision 0014](../docs/decisions/0014-rolled-items.md)).
- A static bonus takes `{ roll: name }` in place of its amount. The client names the roll after the first static bonus that reads it, such as "Physical damage against bosses", or after its name.
- `display_name`, 1 to 64 bytes, is the name the client shows in place of the static bonus's words, such as "Clover fortune". The unit of the bonus stays, such as "%".
- When values are rolled:
  - Monster drops and MVP prizes roll each value from its distribution.
  - `player:give_item(item, amount, { roll = "random" })` rolls them too. `"default"`, the default, uses the defaults, and so do NPC shops.
  - `{ roll = { luck = 5 } }` sets values, each clamped to its range. Rolls it leaves out get their defaults. It needs `items.modify_rolls`, except in a test case's own code.
  - With the ruleset flag `item_rolls` off, the default of core's rulesets, every roll is its default, as rAthena. Random options roll either way, as rAthena rolls them.
- The `item_roll` hook ("Hooks" above) gets every roll of a new copy: from drops, gives (also inside a script's call) and shops. With the flag off, `next()` returns the default.
- Equipped handles have `rolls`, a read-only table such as `equipped.rolls.luck`; `cards`, a list of `{ item, rolls }` for the cards in the slots; and `random_options`, a list of `{ option, value, param }`. In a card's script, `context:get_instance()` reads the item that holds the card, but its `rolls` are the card's own.
- `equipped:set_roll(name, value)` sets one value, clamped. `equipped:reroll(name)` rolls one value again, and `equipped:reroll()` every value, as a random give with the source `{ kind = "script" }`. Both need `items.modify_rolls`. The copy's equip scripts start again after the call, so they read the new values. The handle stays the same. Only equipped copies have them until inventory copies have handles.
- Stacking: stackable copies stack when their rolls and random options are equal. A card with 31 and a card with 29 take two slots. Unidentified copies with hidden rolls never stack. A part of a stack that a player drops and someone picks up gets a new serial, and the server logs the stack it came from.
- The server checks saved rolls against the item's data when a character enters a zone: a value outside its range is clamped, a roll the item no longer has is dropped, and a roll the item gained gets its default. Each change is logged.

### Random options

rAthena's random options become core's `data/random_options.yml` and `data/random_option_groups.yml`. [docs/generated/random-options-renewal.md](../docs/generated/random-options-renewal.md) lists core's options with rAthena's names. Pre-renewal has none, as rAthena.

```yaml
# data/random_options.yml
- name: example.demo:glow
  display_name: Glow
  bonus: [[luk, {roll: value}], [max_hp, {roll: param, multiply: 10}]]

# data/random_option_groups.yml
- name: example.demo:SHINY
  slots:
    - [{option: max_hp, min: 20, max: 60, chance: 6000}, {option: "example.demo:glow", min: 1, max: 3, chance: 4000}]
  max_random: 1
  random:
    - {option: luk, min: 1, max: 3, chance: 1000}
```

- An option has `display_name`, what the client shows, and `bonus`, a list of bonus entries as static bonuses. `{ roll: value }` is the option's value and `{ roll: param }` its param. `multiply` and `divide` scale them, as rAthena's `getrandomoptinfo(ROA_VALUE) / 10`. Core's options also have `id`, rAthena's option ID, which a mod cannot change.
- Core's options have readable names from their first bonus, such as `max_hp` for `VAR_MAXHPAMOUNT`, `physical_damage_against_boss` for `CLASS_DAMAGE_BOSS_USER`, and `fire_damage_taken` for `ATTR_TOLERACE_FIRE`.
- A group keeps rAthena's rules (`s_random_opt_group::apply`): `slots` lists, for each option index from the first, the options it can get with `min`, `max`, `param` and `chance` per 10,000. Each slot picks an option at random and keeps it when its chance passes, trying up to three times the slot's options, and keeps one without its chance after that. Then each index below `max_random` without an option tries one of `random`. A copy has at most 5 options.
- Groups keep rAthena's names, such as `AS_WEAPON_1`. Core's monster drops have rAthena's groups. `player:give_item_from_group` rolls the options of an entry's group for equipment, as `getgroupitem`.
- Options apply after every card, as rAthena's `status_calc_pc_sub`.

### Items

`items.id(name)` from `wa.items` returns an item's ID by name, and raises an error for an unknown name. It needs `game.read`.

## Item scripts

An item's `script` field names its entry script. The setup function runs in an item context (proposal 0002, "Item scripts"):

- A usable item runs its setup on each use, then its `use` handlers with the player. The context ends after the use.
- An equipment item or card runs its setup when a copy is equipped. Its bonuses, handlers and timers last until the copy is unequipped. A copy that is still worn when its player leaves the map ends without `unequip`, and starts again where the player arrives.
- The wearer's stats are computed again from every worn item when equipment, levels or raised stats change, and when a timed bonus starts or ends. Computed bonuses and `when` conditions run at each calculation (server/README.md, "Equipment").

```lua
-- A usable item.
local events = require "wa.events"
local items = require "wa.items"

local BLUE_BOX = items.group "BLUEBOX"

return function()
  events.on("use", function(player)
    player:item_heal { hp = { 45, 65 } }
    player:give_item_from_group(BLUE_BOX)
  end)
end
```

```lua
-- An equipment item.
local bonuses = require "wa.bonuses"
local context = require "wa.context"
local races = require "wa.races"

return function()
  local equipped = context:get_instance()
  local player = equipped.wearer
  bonuses.add("str", 2)
  bonuses.add("damage", 10, { kind = "physical", against = { race = races.demi_human } })
  bonuses.add("atk", function() return equipped.refine * 5 end)
end
```

### When a use happens

- A used item is spent before its script runs, as in rAthena. A `DelayConsume` item, such as a Fly Wing, is spent after its script, and only when the script did not fail.
- Warps that a script asks for happen after the `use` event, once the item is spent.
- An item with `unsupported` set, or with no script, is refused before anything is spent.

### Item events

| Event | Handler arguments | When |
|---|---|---|
| `use` | `player` | The item is used. |
| `damage` | `hit` | A hit the wearer deals or takes. `hit` has `source`, `target`, `kind`, `range`, `skill`, `element`, `is_critical`, `amount` and `wearer_is_source` (0002a, "Item events"). `source.kind` and `target.kind` say whether each is a player or a monster. |
| `skill_used` | `use` | A skill of the wearer took effect. `use` has `caster` (the wearer), `skill`, `level`, `target` (nil for a ground skill), `x` and `y`. |
| `unequip` | `player` | The copy is unequipped, before its context ends. Converted `UnEquipScript`s run on it. |

`damage` fires for each normal attack that lands, and `unequip` when a player takes the copy off.

### Bonuses

`wa.bonuses` follows proposal 0002a. `bonuses.add(name, ..., options)` checks the name, the arguments and the options when it is called.

- The amount can be a function: a computed bonus. It runs again when a value it read changes, and cannot change the game: calls such as `player:give_item` or `random.integer` fail inside it.
- A skill level, as in `bonuses.add("skill", BASH, function() ... end)`, and the `chance` option can be functions too. A granted skill at level 0 grants nothing.
- The trigger of a proc is the `on` option: `bonuses.add("cause_status", statuses.stun, { chance = 300, on = "hit_taken" })`.
- The `when` option is a condition. The bonus applies while the function returns true. It is tracked like a computed bonus, and every bonus takes it:

  ```lua
  bonuses.add("unbreakable_weapon", { when = function() return equipped.refine >= 10 end })
  bonuses.add("skill", TELEPORT, 1, { when = function() return player.base_level >= 99 end })
  ```

  Converted scripts use `when` for every bonus that rAthena adds under a condition.
- `bonuses.add` returns a handle. `handle:remove()` removes that one bonus, for a bonus added in one handler and removed in another:

  ```lua
  local granted
  events.on("base_level_up", function(player)
    if player.base_level >= 99 and not granted then
      granted = bonuses.add("skill", TELEPORT, 1)
    end
  end)
  ```

  The context still removes the bonus when it ends. A second `remove()` does nothing.
- `timers.with_bonuses(ms, fn, { restart = timer })` adds the bonuses `fn` adds for `ms` milliseconds. Their handles work too: `remove()` ends one early.
- Bonuses only exist in the context of an equipped copy. `bonuses.add` in a use handler raises an error.

### Static bonuses

Equipment whose bonuses never change needs no Lua. Its `bonuses` field lists them. Each entry is the bonus name, then the arguments of `bonuses.add`, with constants and records written by name:

```yaml
- name: example.greeter:Lucky_Ring
  type: Armor
  bonuses:
    - [luk, 3]
    - [damage, 10, {kind: physical, against: {race: demi_human}}]
    - [cause_status, stun, {chance: 300}]
    - [cast, MG_FIREBOLT, 3, {chance: 500, on: hit_taken}]
```

The server checks every entry at start with the rules of `bonuses.add`. `when` is a function, so static bonuses cannot have it. An amount can be `{ roll: name }`, a rolled value of the copy ("Rolled item values" above). [Decision 0007](../docs/decisions/0007-converted-item-scripts.md) explains the format.

### Item API

| Call | Permission | Meaning |
|---|---|---|
| `player:item_heal { hp = n, sp = n }` | None | rAthena's `itemheal`, with its bonuses from VIT, INT and skills. An amount can be `{ min, max }`, drawn as `rand`, HP first. |
| `player:heal_percent { hp = n, sp = n }` | None | rAthena's `percentheal`. |
| `player:give_item(item, amount, { rent_seconds = s, roll = r })` | `player.give_items` | rAthena's `getitem`, and `rentitem` with `rent_seconds`. Rentals do not expire yet. `roll` is `"random"`, `"default"` or a table of values ("Rolled item values" above); a table also needs `items.modify_rolls`. |
| `player:give_item_from_group(group)` | `player.give_items` | rAthena's `getgroupitem`: one use of an item group from `items.group "BLUEBOX"`, with rAthena's group rules. |
| `player:warp(map, x, y)` | `player.warp` | Moves the player after the event. Returns false when the map does not run. |
| `player:warp_to_random_cell()` | `player.warp` | A Fly Wing: a random walkable cell of the map, as `pc_randomwarp`. |
| `player:warp_to_save_point()` | `player.warp` | A Butterfly Wing: the save point, or the start map's start cell without one. |
| `player:show_effect(effect)` | None | rAthena's `specialeffect2`: everyone near sees it. |
| `player:show_headgear_effect(effect)` | None | rAthena's `hateffect`. It shows nothing yet: the protocol has no message for it. |
| `player:get_skill_level(skill)` | `game.read` | rAthena's `getskilllv`. |
| `player:get_equipped(slot)` | `game.read` | The copy in a slot, such as `right_hand`, as an equipped handle, or nil. |
| `player:start_status(status, options)` | None | rAthena's `sc_start`, `sc_start2` and `sc_start4` ("Status effects" below). |
| `player:add_timed_bonuses(key, ms, bonuses, options)` | None | rAthena's `bonus_script` ("Timed bonuses" below). |

Heals need no approved permission.

Equipped handles, from `context:get_instance()` in an equip script or from `player:get_equipped`, have `item`, `refine`, `enchant_grade` (0 for none, 1 to 4 for D to A), `wearer`, `weapon_type`, `weapon_level` and `armor_level`, and `rolls`, `cards` and `random_options` with the methods `set_roll` and `reroll` ("Rolled item values" above).

Player handles gained `base_str` to `base_luk` (allocated points), and `job_class` (rAthena's `eaclass()`). `base_pow` to `base_crt` are 0 until trait stats exist.

Modules:

| Module | Gives |
|---|---|
| `wa.random` | `random.integer(min, max)` and `random.chance(numerator, denominator)`, from the zone's secret stream. |
| `wa.integers` | `divide_truncated` and `remainder_truncated`, which round as C does. |
| `wa.skills`, `wa.monsters` | `skills.id "SM_BASH"`, `monsters.id "PORING"`. |
| `wa.races`, `wa.elements`, `wa.sizes`, `wa.monster_classes`, `wa.monster_groups`, `wa.weapon_types`, `wa.statuses`, `wa.effects`, `wa.jobs` | Constants. |
| `wa.headgear_effects` | The constants of `show_headgear_effect`. |
| `wa.emotions` | The constants of `npc:show_emotion`, from rAthena's `ET_` names, such as `emotions.thanks`. |
| `wa.variables`, `wa.text`, `wa.maps`, `wa.world` | Saved variables, text, and announcements ("NPC scripts"). |
| `wa.schedules`, `wa.spawning`, `wa.hooks` | Schedules, monsters and hooks ("Schedules", "Spawning monsters", "Hooks"). |
| `wa.world_events` | `world_events.start` and `world_events.stop` ("Typed events"). |

Constant names come from rAthena's by a fixed rule (0002a, "Constants used in arguments", lists the spelled-out names): the prefix goes, the rest is in lower case, and a few names are spelled out. So `RC_DemiHuman` is `races.demi_human`, `SC_STUN` is `statuses.stun`, `EF_HEAL` is `effects.heal`, and `W_2HSWORD` is `weapon_types.two_handed_sword`. A name that starts with a digit gets an `n`: `statuses.n2011rwc_scroll`.

`wa.jobs` also has job tests: `jobs.is_transcendent`, `jobs.is_baby`, `jobs.is_third`, `jobs.is_fourth`, `jobs.base_job` (rAthena's `BaseJob`: a Lord Knight is a Knight) and `jobs.first_job` (rAthena's `BaseClass`: a Knight is a Swordman). Each takes a job, such as `player.job`.

### Core's item scripts

`waserver` converts rAthena's item scripts when it starts, from the mode's `item_scripts.json` in `reference/rathena/extracts` ([tools/convert](../tools/convert/README.md)):

- `data/items.yml` sets `script`, `bonuses` or `unsupported` on every item with a script.
- `scripts/items/<AegisName>.lua` is the entry script of each item that needs Lua. A name that is not a valid file name uses `item_<id>`.
- Each file starts with the item's rAthena scripts as a comment.

The files are not in the repository ([decision 0007](../docs/decisions/0007-converted-item-scripts.md)). To read them:

```text
go run ./tools/wactl convert items --mode renewal --out /tmp/core-items
```

To change a core item, set its `script` or `bonuses` in a mod's `data/items.yml`. The coverage report of each mode is in [docs/generated](../docs/generated).

A zone loads an item's script the first time a player uses it there, and keeps at most 256 loaded.

## Core's NPCs

`waserver` converts rAthena's NPC scripts and shops when it starts, from the mode's `npcs.json` and `item_prices.json` in `reference/rathena/extracts` ([decision 0008](../docs/decisions/0008-converted-npcs.md)):

- `data/npcs.yml` places every NPC that converts and every zeny shop.
- `scripts/npcs/<name>.lua` is the entry script of a script NPC. Its duplicates name the same file.
- `scripts/functions/<name>.lua` is an rAthena function, which scripts load when they first call it.
- `scripts/lib/rathena.lua` has helpers the converted scripts share: arrays from 0, menus written as `"A:B"`, and `end` from inside a function.

To read them:

```text
go run ./tools/wactl convert npcs --mode renewal --out /tmp/core-npcs
```

To change a core NPC, set its fields in a mod's `data/npcs.yml` with its full name, such as `core:kaf_prontera`. Kafra pushcart rental says it is not available yet.

Core also has NPCs of its own, written for wAthena: the Job Master (see "Job changes and resets"). The converter maps `jobchange` to `player:change_job` with `force`, `jobchange Job_Novice_High` and the `resetlvl(1)` after it to `player:rebirth`, `resetskill` and `resetstatus` to the reset calls, and the `Upper` parameter to the job tests. The coverage report of each mode is in [docs/generated](../docs/generated).

## Tools

`wactl` has the mod commands (strategy section 6.9). They carry the reference data, so they work outside the repository. Build `wactl` with `go build ./tools/wactl`, or run it with `go run ./tools/wactl` in the repository.

| Command | What it does |
|---|---|
| `wactl mod new <id> [--dir <folder>]` | Makes a mod from a template: a manifest, an NPC with a conversation that uses saved variables, an item overlay, a test, `locales/en.yml`, a README, and the editor definitions. It passes `validate` and `test` at once. |
| `wactl mod validate [path] [--mods <folder>] [--show]` | Checks the mod: the manifest, the data files and their IDs, the rulesets it loads on, its permissions, the Lua syntax of every file, that entry scripts return a function, that required modules exist, and that test files register cases. `--show` prints the final value of every record the mod adds or changes, and which mod set each field. |
| `wactl mod test [path] [--mods <folder>] [--run <text>] [--assets <build>]` | Runs the mod's tests ("Testing a mod" below). |
| `wactl dev [path] --assets <build> [--mods <folder>]` | Runs a local server with the mod, and reloads it when its files change. |
| `wactl mod pack [path] [--out <folder>]` | Validates the mod, then writes `<id>-<version>.zip` and `<id>-<version>.zip.sha256` into `dist`. |
| `wactl mod install <git URL \| .zip \| folder> --mods-dir <folder>` | Checks a mod and installs it for a server ("Installing a mod" below). |
| `wactl mod types [path]` | Writes the editor definitions into `.wathena/types` again, and `.luarc.json` when it is missing. |

- `path` is the mod's folder, or the working folder when it is left out. `validate` and `test` also take a `.zip` file.
- `--mods` names a folder of other mods, where the mod's dependencies are, as `mods.dir` does for the server.
- The commands check against renewal data. `--mode pre-renewal` checks against pre-renewal data.
- Problems are printed with their file and line, and the command exits with status 1.

```text
$ wactl mod new example.demo
made example.demo in example.demo:
  README.md
  data/items.yml
  data/npcs.yml
  locales/en.yml
  mod.yaml
  scripts/npc/guide.lua
  tests/guide_test.lua
  .luarc.json
  .wathena/types/ (26 files)
next: cd example.demo && wactl mod validate && wactl mod test
$ cd example.demo
$ wactl mod validate
example.demo 0.1.0 (.)
loads on: core.ruleset.renewal, core.ruleset.pre_renewal
checked: 2 Lua files, 2 test cases
permissions:
  player.give_items    needs the owner's approval: give items to players
nothing is approved. To approve, review the mod, then add to the server config:
  mods:
    approve:
      example.demo: [player.give_items]
ok
```

### Testing a mod

Test files are `tests/*_test.lua`. Like an entry script, a test file returns a function. That function registers cases with `testing.case` from `wa.testing` ([proposal 0002](../docs/proposals/0002-lua-api.md), review decision 15).

```lua
local testing = require "wa.testing"
local items = require "wa.items"
local variables = require "wa.variables"

local BADGE = items.id "example.demo:Guide_Badge"
local visits = variables.character_integer "visits"

return function()
  testing.case("the guide gives one badge", function(world)
    local player = world:new_player { map = "prontera", x = 156, y = 183 }
    local said = world:talk(player, "Guide#prontera", { 1 })
    testing.expect(said).to_contain("Here is your badge.")
    testing.expect(player:count_item(BADGE)).to_be(1)
    testing.expect(visits:get(player)).to_be(1)
  end)
end
```

- Each case runs in a fresh headless world: the real zone code without a network, with the mod, its dependencies and core loaded, core's converted NPCs and item scripts, a fixed random seed, and no database. A case takes a few milliseconds.
- A world runs one map: the map of its first `new_player` or `spawn_monster`, or `prontera` when `advance` comes first. Warps to other maps do not happen.
- Without `--assets`, every cell is walkable. With an asset build, the map's real cells are used.
- With a `wactl` built with `-tags procedural`, every test world also has generated maps, the same in every case (decision 0040): the field `gen:field:0:0`, 96 cells square, a town per town template from `gen:town:t1`, core's first and then the mods' in load order, and a dungeon per dungeon style, named by the style without its mod prefix, such as `gen:dungeon:cellar:2`. `wa.places` finds their places in any case, and `world:new_player { map = "gen:town:t4", x = 48, y = 45 }` runs the case on one of them, with the NPCs placed by tag. A `wactl` without the tag has no generated maps, and a case that names one fails with a message that says so. The Market Town sample's tests show it.
- Test code runs as the mod: its calls need the mod's permissions, and the variables it declares are the mod's, so a test reads what the mod's NPCs saved.
- `wa.testing` exists only in `wactl mod test`. A server never has it, and never loads `tests/`.

| Call | What it does |
|---|---|
| `testing.case(name, function(world))` | Registers a case. |
| `world:new_player { map, x, y, job, base_level, job_level, zeny, items = { {item, amount} }, sex, skills = { {skill, level} }, stats = { str = 50 }, skill_points, status_points }` | Adds a character at a walkable cell, with full HP and SP, and returns its player handle. Only `x` and `y` are required. `sex` is `"male"` by default. `skills` are learned skills, `stats` the raised stats (the others are 1), and `skill_points` and `status_points` the points left to spend. |
| `world:talk(player, npc_name, answers)` | Clicks the NPC and answers each step in order: a number picks a menu option, a string fills text input, and an integer fills number input. Next is pressed without an answer, and waits and progress bars run on the clock. Returns the NPC's lines, one per line. Fails when the conversation asks for more answers than given, when one does not fit, or when some are left over. |
| `world:use_item(player, item)` | Uses one of the player's items. Fails when the player has none, or the use is refused. |
| `world:equip(player, item, { roll })` | Gives the player one copy of an item and puts it on. Returns the equipped copy, as `player:get_equipped` does. `roll` is the roll option of `player:give_item`: `"random"`, `"default"` (the default) or a table of values, which needs no permission here. Fails when the copy does not fit or cannot be equipped. |
| `world:advance(milliseconds)` | Moves the zone clock. Timers fire. The clock starts at 0 and moves only with this call, conversation waits, and the item use interval. |
| `world:spawn_monster { monster, map, x, y }` | Places a monster from `monsters.id` on a cell and returns its handle. It does not come back after it dies. |
| `world:kill(player, monster)` | The player kills a living monster: `death` fires, and the player gets its exp and drops. After it, the monster's handle raises an error. |
| `world:set_time(time, time_zone)` | Sets the server time and time zone the world starts at, such as `"2026-10-31 19:58"` and `"Asia/Manila"`. Call it before the world has a map. Without it, the world starts at 2026-01-05 12:00 UTC. |
| `world:get_time()` | The server time, as `"2026-10-31 20:00"`. |
| `world:get_monsters(monster)` | Handles of the living monsters of the map, of one ID or all, in the order they spawned. |
| `world:get_announcements()` | Every line announced to the world or the map so far. |
| `world:count_floor_items(item)` | How many of an item lie on the map. |
| `world:chat(player, text, { channel, to })` | The player says a line, as a client's chat: `channel` is `"public"` (the default), `"whisper"` with `to`, or a channel's name. It passes the mute, the filter and the hooks, then the `chat` event runs. |
| `world:get_chat(player)` | The chat lines the player got so far, each `{ channel, from, text }`: `channel` is `"public"`, `"whisper"`, `"system"`, or a channel's name with its `#`. |
| `world:plan_restart(minutes, reason)` | Plans a restart that many minutes from the world's time, 1 to 10,080, with a reason of 1 to 255 characters, as staff do on the admin screens. `restart_planned` runs with `{ at, reason }`, and the lines its handlers send go out. Nothing restarts. It is provisional. |

The last five, and world events in test worlds, were added in phase 3 step 6 (proposal 0002, review decisions 16 and 17). `world:chat`, `world:get_chat`, `world:trade`, `world:close_storage`, `world:buy`, `world:get_mails` and `world:vend` came with phase 5, and review decision 32 made them stable. `world:plan_restart` came with review decision 41. A test world runs the world events of its map, and of no map, on its clock: `world:advance` past an event's time starts it, and past its end ends it.
| `testing.expect(value)` | Checks a value with `.to_be(x)`, `.to_equal(table)` (deep), `.to_contain(text_or_value)`, `.to_be_true()`, `.to_be_false()`, `.to_be_nil()`, or `.to_fail_with(text)` for a function that should raise an error containing the text. |

A failed check stops the case and prints the file and line of the check. A case also fails when the server logs a warning that names the mod, such as a handler that failed. `wactl mod test` exits with status 1 when any case fails, so CI can run it.

```text
$ wactl mod test
tests/guide_test.lua
  ok   the guide gives one badge (7 ms)
  FAIL the guide counts visits (5 ms)
       tests/guide_test.lua:28: expected "[Guide]\nWelcome to Prontera!\nYou have visited me 2 times.\nCome back any time." to contain "You have visited me 3 times."
2 cases: 1 passed, 1 failed (1.6 s)
```

### A local server: wactl dev

`wactl dev` runs `waserver` on loopback addresses with the mod and watches its files:

```text
$ wactl dev --assets <build>
wactl dev: ready at http://127.0.0.1:8300; editing . reloads it
wactl dev: changed scripts/npc/guide.lua
wactl dev: reloaded example.demo in 1 zones
wactl dev: changed data/items.yml
wactl dev: the manifest or data changed: restarting the server
wactl dev: ready at http://127.0.0.1:8300; editing . reloads it
```

- A change to a Lua file or `locales/en.yml` reloads the mod's scripts in every zone, through the server's reload endpoint. A Lua file that does not compile is reported and not reloaded.
- A change to `mod.yaml` or a data file restarts the server.
- The server runs with `--dev`, `--shutdown-countdown 0`, and the mods of `--mods`. It approves what the mods request, for development only.
- `--server` names the `waserver` binary. By default it is the one next to `wactl`, then the one on `PATH`. `--addr` and `--debug-addr` set the addresses, `127.0.0.1:8300` and `127.0.0.1:8301` by default, and `--map` the start map.
- Ctrl-C stops the server and `wactl dev`.

The reload endpoint also works without `wactl dev`: run `waserver --dev --debug-addr 127.0.0.1:8301 --mods <folder>`, and after a change:

```text
curl -X POST 'http://127.0.0.1:8301/debug/wathena/mods/reload?mod=example.greeter'
```

### Packing and installing

`wactl mod pack` validates the mod, then writes `dist/<id>-<version>.zip`, with the files in one top folder, and `dist/<id>-<version>.zip.sha256` in `sha256sum`'s format. A mod with `art/` gets its art built into `assets/` in the archive ("Art"). The same files give the same archive.

`wactl mod install <source> --mods-dir <folder>` installs a mod into a server's mods folder:

- A Git URL is cloned. `--ref` checks out a tag, branch or commit, and the commit is printed.
- A `.zip` file is checked against `--sha256`, or against `<file>.sha256` when it exists. With `--public-key`, `<file>.minisig` must be a matching [minisign](https://jedisct1.github.io/minisign/) signature. Authors sign with `minisign -Sm <file>`.
- A folder is copied as it is. A folder or Git mod with `art/` and no `assets/` needs `wactl mod build-art` in the mods folder, or it shows placeholders.
- The mod must pass `wactl mod validate` against core and the mods already installed.
- The permissions it requests are shown, with those that need approval. Nothing is approved: the owner reviews them and adds the printed lines to the server config.
- A mod with the same ID already installed is an error, unless `--replace` is given.

```text
$ wactl mod install dist/example.demo-0.1.0.zip --mods-dir /srv/mods
checksum ok: SHA-256 fcf439cc...c69a (from dist/example.demo-0.1.0.zip.sha256)
example.demo 0.1.0: Demo
permissions:
  player.give_items    needs the owner's approval: give items to players
nothing is approved. To approve, review the mod, then add to the server config:
  mods:
    approve:
      example.demo: [player.give_items]
installed /srv/mods/example.demo-0.1.0.zip
```

### Editor support

The Lua Language Server gives completion and type warnings for every `wa.*` module, handle, event name and bonus name.

- `wactl mod new` writes the definitions into `.wathena/types` and a `.luarc.json` that names them. `wactl mod types` writes them again after a `wactl` update.
- The definitions come from [server/modscript/luaapi](../server/modscript/luaapi), which tests check against the runtime. The same source makes the [API reference](../docs/generated/lua-api.md) and the [constants](../docs/generated/lua-constants.md).
- In this repository, `docs/generated/lua` holds them, and `examples/greeter/.luarc.json` names that folder.
