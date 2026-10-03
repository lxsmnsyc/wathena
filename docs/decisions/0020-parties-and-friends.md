# 0020: Parties, Friends and Limits

- Status: proposed.
- Date: 2026-10-04.
- Code: [server/social](../../server/social), [server/zone](../../server/zone) (`party.go`, `partyscripts.go`, `loot.go`), [server/progression](../../server/progression) (`kill.go`, `floor.go`), [server/rules](../../server/rules) (`limits.go`), [server/mod](../../server/mod) (`ruleset.go`), [server/modscript](../../server/modscript) (`party.go`), [server/store](../../server/store) (`social.go`, migration 00017), [server/gateway](../../server/gateway) (`party.go`), [tools/convert](../../tools/convert) (`npcparty.go`), [client](../../client) (`party.ts`, `friends.ts`, `booking.ts`), [e2e](../../e2e) (`party_test.go`), [mods/examples/party-hall](../../mods/examples/party-hall).
- Phase 5, step 4. Builds on strategy sections 3.6, 6.8 and 11.3, proposal 0002, review decision 27, and decisions [0017](0017-e2e-and-transfers.md), [0018](0018-chat-and-moderation.md) and [0019](0019-ruleset-flags.md).

## Decision

### One registry per world

- The package `social` has a registry that every zone of the process shares, as the chat hub. It holds every party, the friend lists of the characters it has seen, and the party booking ads.
- A zone hands it a player at login, on a map change and at logout. The registry hands each player what changed through its session, as `Notice` values, and never blocks: a zone that is far behind drops them. A map change sends the player its party again, since notices sent while it moved were dropped.
- With a database, every party loads at start, and the registry gives new party IDs from the highest saved one. A character's friend list loads with the character. The registry keeps the list of a character it has seen since the process started, since it is newer.
- Each change goes to `store.SocialStore`, which writes the changes in order on its own goroutine, one transaction each. Without a database, parties and friend lists last while the server runs.
- Several processes need the registry to go through PostgreSQL or another bus, as the chat hub. That is left to the multi-process deployment.

### Parties

They follow rAthena's `party.cpp` and the char server's `int_party.cpp`:

- Members sit in slots, as `party.member[MAX_PARTY]`. A new member takes the first free slot. The party window, the even split and the item share go in slot order.
- Only the leader invites, expels, hands over the lead and sets the options. A new leader must be online, and on the leader's map with `change_party_leader_samemap`.
- An invite needs the target online, without a party or an open invite. With `block_account_in_same_party`, a character of an account that is in the party is refused. A player who blocks the leader refuses the invite at once, without seeing it (decision 0018). rAthena does not check its ignore list here.
- The party breaks up when its leader leaves or is removed, as the char server does. rAthena marks leaderless parties as a TODO.
- Even exp share needs the online members' base levels within `party_share_level`. Asking for it outside the range keeps exp share each and tells the leader. A login, a logout, a level up, a join or a leave that breaks the range turns it off, as `int_party_check_lv`. rAthena's family exception needs marriage and adoption, which do not exist yet.
- HP goes to the members on the same map when it changes, and cells once a second. rAthena sends HP to the members in sight with `party_hp_mode` 0. The web client shows HP for every member on the map.
- Party chat passes the mute, the filter and the chat hooks, reaches every online member on any map, and is logged with the kind `party`. A member who blocks the speaker does not see it.

### Exp and item share

- `progression.KillWith` puts each fighting member's share of an even share party in the party's pot, as `mob_dead`'s `pt[]`, before the level penalty. Members of each share parties get their own exp as players without a party.
- `progression.ShareExp` is `party_exp_share`: the pot divided by the members on the map who are alive, with `party_even_share_bonus`, then the level penalty for each member when `level_penalty_exp` is on (`RENEWAL_EXP`). Individual rewards come first, then each party's, in the order the damage log first names the party.
- `progression.Rights.CanTakeParty` is `pc_takeitem`'s check with shared pick-up: a member may take what its party's top attackers may take while they are online. Ground items keep their looters' names for this.
- `social.SharesLoot`, `RoundRobin` and `RandomPick` are `party_share_loot`'s pick. A member who cannot carry the item is skipped, and the picker gets it when no one can. Every item moves through a transfer, so the item log names the member who got it.
- `PartyItemShared` tells the members on the map who got a weapon, armor or card, as `clif_party_show_picker` with its defaults.

### Friends

- They follow `clif.cpp`'s friend packets: a request needs the other player online and room on the requester's list; with `friend_auto_add`, an accepted request puts each player on the other's list; a removal takes each name off the other's list, also when the other player is offline.
- A player who blocks the requester refuses at once, without seeing the request. rAthena checks only `@noask`.
- Friends see each other log in and out.

### Party booking

- It follows `party_booking_*`: one ad per character, with a level, a map and up to 6 jobs. A search by level finds ads from 15 below it to it, by map or by job. As in rAthena, a search with both a map and a job finds nothing.
- An ad has a note of up to 80 characters, which rAthena's ads lack. Ads live in memory and end at logout, as `map_quit`.
- The web client has the booking window (strategy section 11.3).

### Limits and rules

- A ruleset mod sets limits in `provides.ruleset.limits` and rules in `provides.ruleset.rules` (review decision 27). `server/rules` lists each one with rAthena's value and its bounds. A value outside the bounds, or an unknown name, fails the load.
- Limits: `party_members` (12, at most 64), `friends` (40, at most 200) and `block_list` (20, at most 100). Rules: `party_share_level` (15), `party_even_share_bonus` (0), `party_item_share_type` (0), `change_party_leader_samemap`, `block_account_in_same_party` and `friend_auto_add` (yes).
- Lowering a limit deletes nothing: a list over it keeps its entries and cannot grow. Strategy section 6.8 said a lower limit is refused at startup. Review decision 27 replaced that.
- The block list of decision 0018 now follows `block_list`. Migration 00017 lets `character_blocks` hold 100 names, and `party_members` slots go up to 63.
- The client gets the limits in `Welcome.limits`. Lua reads them as `rules.limits.<name>`.

### Lua API

Review decision 27, as built:

- `player.party`, party handles with `id`, `name`, `leader`, `leader_name`, `exp_share`, `item_share` and `item_pickup`, and `get_members` and `get_member_names`.
- `wa.parties`: `create`, `get` and `find`, and the party methods `add`, `remove`, `set_leader`, `set_options` and `disband`, with the new permission `party.manage`.
- The events `party_join`, `party_leave` and `party_change`.
- The `experience` hook's object has `party` and `share`.
- `player:is_friend` and `player:get_friend_names`.
- `wa.rules` with `rules.limits`.

## Why one registry and not the chat hub

| | A registry of its own | Parties in the chat hub |
|---|---|---|
| What it holds | Parties, friend lists and ads, also of offline characters. | Online players and channels. |
| Writes | An ordered writer of its own. | The chat log's batch writer, which may drop lines. |
| Locks | Its own. It reads block lists from the hub. | One lock for chat and parties. |

The registry asks the hub whether a player blocks another, and the hub never calls the registry, so the two locks cannot wait on each other.

## Parity

A new recording per mode, `party_share.json`, from rAthena fork commit `50df331ca`. It records every `pc_gainexp` call of 69 kills and 108 pick-ups per mode: parties with each and even exp share, members who are dead or on another map, `party_even_share_bonus` at 0 and 10, the level penalty, and every item option and share type with full inventories and the random draws rAthena made.

- Exp: 69 of 69 kills in renewal and 69 of 69 in pre-renewal (`TestPartyExpShareMatchesReference`). Each recording also matches on the other base with `level_penalty_exp` set to its mode (`TestPartyExpShareFlag`).
- Loot: 108 of 108 pick-ups in each mode (`TestLootShareMatchesReference`).
- `party_share_level` is the char server's, which the recorder does not run. Its tests follow `int_party.cpp`.

## Party skills

The party skills that phase 4 left failing now reach party members:

- Party statuses: Angelus, Gloria, Magnificat, and renewal Impositio Manus, Suffragium and Loud Exclamation.
- Redemptio revives dead members in its area. Pre-renewal also takes the caster's SP and the exp cost.
- Blacksmith party buffs: Weapon Perfection, Power Thrust, Adrenaline Rush and its advanced form. Also Wind Walker.
- Twilight Pharmacy 2 and 3 check the jobs of online members. The Taekwon and Star Gladiator party bonuses, and pre-renewal TK_POWER, count members.
- Renewal songs reach members. Ensembles take the first partner in the party.
- Marionette Control and Ki Translation target party members only.
- Battle Chant's support effects reach members. Dispell outside PvP works on party players only.
- Hidden traps show to the caster's party. Aid Condensed Potion heals members in its area.
- Devotion protects a member from skills and normal attacks while the member is in range. Guard, Defending Aura, Shield Reflect and Endure pass to devoted players. The Crusader's death ends the link.

`battle_check_target` adds the party flag when the masters of source and target share a party, and PartyOnly and GuildOnly skills follow `skill_castend_id_check`.

Not ported:

- The Devotion link line, which needs a protocol event.
- Guild reach of GuildOnly skills, which needs guilds.
- GvG and battleground exceptions, mercenary Devotion and Redemptio's map flag, which need map flags and mercenaries.

## Other choices

- The `experience` hook runs once for each player who gets exp. With even share, `share` is the member's own share before the split, `{ 0, 0 }` for a member who did not fight.
- `party_join` and `party_leave` run in the zone of the player who joined or left, so the player is a handle there. A member who is offline gets no event. `party_change` runs in the zone where the change was made.
- A party's leader and members are handles of players in the context's zone. `party.leader` is nil when the leader is on another map, and `get_members` leaves out members on other maps.
- A test case's own code needs no permission for `wa.parties`, so a test can set up a party.
- A party that broke up stays readable for the last 1,024 parties, so a `party_leave` handler can read its name.
- An invite stays open until it is answered or the player logs out, as rAthena. The web client declines an invite it shows after 30 seconds.

## Left

- Guild chat, guilds and the guild limits (step 5), and the storage, cart, inventory and card slot limits (later steps).
- The family exp share exception, which needs marriage and adoption.
- `idle_no_share`, zeny from monsters and the party's share of homunculus exp.
- Instances, party-bound items and `@partyoption`-style staff tools.
- Events for members who are offline, and a `party_join` for a player who makes a party itself.
- The registry across processes.
