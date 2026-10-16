# 0001: Web Client Protocol

- Status: accepted.
- Schema: [schema/proto/wathena/v1](../../schema/proto/wathena/v1).

## Problem

The web client and the server need one message format. It must be:

- Small, to fit the network budgets in strategy section 3.8.
- Generated for Go and TypeScript from one schema.
- Safe to decode from untrusted input, with fuzz tests.
- Able to change over time without breaking older clients by accident.

## Change

### Format

- Messages are Protocol Buffers (proto3), in the `wathena.v1` package.
- Go code is generated with `protoc-gen-go` and committed under `schema/gen/go`. It uses the opaque API, so fields are read and set through methods.
- TypeScript code is generated with `protobuf-es` and committed under `client/src/gen`.
- `buf lint` checks the schema. CI regenerates the Go code and fails if it differs from the committed code.

### Transport

- One WebSocket per session, over TLS, with binary frames.
- Each WebSocket message holds exactly one `ClientMessage` or `ServerMessage`.
- WebSocket compression (`permessage-deflate`) is off. Compressing chat that an attacker can send together with private data, such as whispers, lets a network observer guess that data from message sizes.

### Session start

1. The player logs in on the account service's page and chooses a character there. The client then fetches a one-time **game ticket** from the account service, with the login cookie, which the client never sees (strategy section 7.6, [decision 0004](../decisions/0004-account-service-and-game-tickets.md)).
2. The client opens the WebSocket and sends `Hello` as its first message. `Hello.session_token` holds the ticket as its text: 43 characters of unpadded base64url for 32 random bytes. The ticket is in `Hello`, not in the URL, so it never appears in access logs.
3. The server checks the protocol version, then uses up the ticket: it deletes the ticket and reads its character in one statement, so a ticket works once. It then sends `Welcome`, or `Disconnect` with a reason:
   - `DISCONNECT_REASON_SESSION_INVALID` for a ticket that does not exist, was used, or expired. Tickets expire 60 seconds after they are made. The client sends the player back to the account pages.
   - `DISCONNECT_REASON_VERSION_UNSUPPORTED` for a client outside the supported versions.
4. The server closes a connection that sends anything other than `Hello` first, or no `Hello` within 10 seconds.
5. The gateway accepts the WebSocket only from the trusted client origins the server owner lists, from pages on its own host, or without an `Origin` header (not a browser page).

A connection belongs to the login session that made its ticket. When that session ends, at logout or when a new login in the same browser replaces it, the server sends `DISCONNECT_REASON_LOGGED_OUT` and closes the connection ([decision 0005](../decisions/0005-two-factor-login-and-passkeys.md)).

A `waserver --dev` server, which only listens on a loopback address, instead takes the character's name as the token.

### Versions

- `schema.Version` is the protocol version. It increases by 1 for each release that changes the protocol.
- The server accepts clients from `schema.MinVersion` to `schema.Version`. Older clients get `DISCONNECT_REASON_VERSION_UNSUPPORTED`.
- Within `wathena.v1`, changes are additive only: new fields, new messages, and new `oneof` cases. Field numbers are never reused.
- Clients skip `Event` cases they do not know. The server ignores `Command` cases it does not know and counts them toward the rate limit.
- A change that cannot be additive goes to a new package, `wathena.v2`, with its own proposal.
- `buf breaking` runs in CI from the first public release. Before then, `v1` can still change freely.

### Messages

Client to server, in `ClientMessage`:

| Case | Use |
|---|---|
| `hello` | Session start. |
| `ping` | Latency and keepalive. |
| `command` | A player action: walk, attack, use or learn a skill, chat, NPC click or reply, buy from or sell to a shop, use item, drop item, pick up, raise a stat. |

Server to client, in `ServerMessage`:

| Case | Use |
|---|---|
| `welcome` | Session accepted, with server time, tick, the player's own entity ID, the map's asset name, the world's limits, and the world's mode. |
| `pong` | Reply to `ping`, with server time. |
| `frame` | Everything visible that happened in one tick. |
| `disconnect` | Reason for closing, shown to the player. |

`ClientMessage.seq` increases by 1 per message. `Frame.ack_seq` returns the last one processed, so the client knows which commands the server has seen.

### Tick frames

- The server sends at most one `Frame` per player per tick (50 ms). Ticks with nothing visible send nothing.
- A `Frame` holds a list of `Event` values: entities appearing, vanishing, moving, and stopping, stat changes, damage, skill casts, chat, NPC dialog steps, effects, and map changes.
- Times inside a frame are milliseconds from the start of that tick, so events keep their exact timing within the 50 ms.

### Encoding rules

- **Entity IDs are per connection.** The server gives each visible entity a 32-bit `net_id` for that connection. Database IDs for accounts, characters, and items are never sent. This stops clients from tracking players across sessions by ID.
- **Movement sends a path, not positions.** `EntityMove` holds the start cell, the walk speed, and the steps as 4-bit directions, numbered as `EntityAppear.dir`: 0 south, turning clockwise. The client animates the walk. The server only sends a new message when a walk starts, changes, or stops. A unit in the middle of a step finishes it, so a changed walk starts with that step.
- **Other players' HP is a fraction.** `EntityAppear.hp_permille` is HP out of 1000, and `Damage.target_hp_permille` updates it after each hit. Exact HP goes only to the player, their party, and their homunculus.
- **Text is already translated.** NPC dialog and messages arrive in the player's language, chosen from `Hello.language` (strategy section 6.13).
- **Ground items are entities.** They appear with `ENTITY_KIND_ITEM` and a `GroundItem` holding the item ID, amount, and place inside the cell. `PickUp` names the item's entity ID. Everyone in view gets `ItemTaken` and then a vanish with `VANISH_REASON_PICKED_UP`. The picker also gets `InventoryAdd`, or `PickUpFailed` when nothing happened. The reasons follow rAthena's `pc_takeitem` and `pc_additem`: the item is gone, another player has the right to it for now, it would pass the maximum weight (`OVERWEIGHT`), the inventory has no free slot (`INVENTORY_FULL`), or the stack would pass 30000 or the item's stack limit (`TOO_MANY`). An item that cannot be taken stays on the ground.
- **The inventory is slots.** The first frame carries `InventoryUpdate`: every slot that holds an item, with its index from 0, the item ID and the amount, and `size`, the slots that take new items (100, rAthena's `inventory_slots`). Slots are rAthena's inventory indexes, so a stack keeps its slot. `InventoryAdd` names the slot an amount entered, and `InventoryRemove` the slot an amount left, with the reason (used or dropped). A slot left with nothing is empty. Slots go up to 199, rAthena's `MAX_INVENTORY`.
- **Using and dropping items name a slot.** `UseItem` uses one item from a slot, as rAthena's `pc_useitem`. Everyone in view, the user included, gets `ItemUsed` with the user and the item ID, as `clif_useitemack` goes to the area. The user also gets `InventoryRemove`, the new HP and SP, and, for HP, a `Damage` with `DAMAGE_KIND_HEAL` whose source and target are the user and whose amount is the HP healed. Only the user gets the heal number. Only items whose script is a plain heal run for now; others fail with `USE_ITEM_FAILURE_NOT_SUPPORTED` and are not spent. `DropItem` drops an amount from a slot on a free cell 1 or 2 cells from the player, as rAthena's `pc_dropitem`. The item appears as a ground item that anyone may take at once. Failures come as `UseItemFailed` or `DropItemFailed` with a reason. The gateway drops a `UseItem` or `DropItem` whose slot is 200 or more, or whose amount is over 30000, without a reply.
- **Own stats are single numbers.** `StatUpdate` sends each changed stat once. HP and MAX_HP go together after damage, healing, or a new maximum. The others follow rAthena's status window (`clif_updatestatus`):
  - STR to LUK are totals, with job bonuses.
  - ATK, DEF and MDEF are the sum of the window's two numbers (`pc_leftside_*` plus `pc_rightside_*`). In renewal, ATK is status ATK plus weapon ATK.
  - MATK is the sum of both numbers in renewal. In pre-renewal the window shows a range, and MATK is its maximum.
  - HIT and FLEE are hit and flee. CRITICAL is critical rate in whole points (rAthena's value divided by 10).
  - ASPD is the displayed ASPD, `(2000 - amotion) / 10` rounded down: amotion 440 is ASPD 156.
  - WEIGHT and MAX_WEIGHT are in tenths of a weight unit, as rAthena's `weight` and `max_weight`. WEIGHT is sent on join and after every inventory change.
  - STR_COST to LUK_COST are the status points that raising each stat by 1 costs, as rAthena's `SP_USTR` to `SP_ULUK`. They are 0 at the stat's maximum.
- **Raising a stat spends status points.** `RaiseStat` names one of STR to LUK and an amount from 1 to 255. The server raises the stat as far as the points and the job's maximum allow, as rAthena's `pc_statusup`, and sends the changed stats, costs and STATUS_POINTS. A raise that fails changes nothing and sends nothing. The gateway drops a `RaiseStat` with another stat or amount.
- **Local chat reaches 9 cells.** `Chat` with `CHAT_CHANNEL_LOCAL` goes to every player within 9 cells of the speaker on both axes, as rAthena's `AREA_CHAT_WOC` does, the speaker included, as a `ChatMessage` with `from` set to that connection's ID for the speaker and `from_name` set to the speaker's name. Dead players can talk, as in rAthena. A staff member hidden with `@hide` is heard too, as rAthena sends its line to the area, with `from` 0, since the listener does not know its entity. The client shows such a line in the log without a bubble. The other channels are accepted and ignored until they exist. The gateway checks the text before the zone sees it:
  - Text that is not valid UTF-8 is dropped.
  - Control characters (Unicode category Cc, such as tab and newline) become spaces.
  - Bidirectional formatting characters are removed: U+061C, U+200E, U+200F, U+202A to U+202E, and U+2066 to U+2069. They could reorder the line or the name next to it.
  - Spaces at both ends are trimmed. A line that is then empty, or longer than 255 characters, is dropped. A character is a Unicode code point.
  - Clients show chat with `textContent`, never as HTML (strategy section 7.5).
- **Server text to one player.** A `ChatMessage` with `CHAT_CHANNEL_SYSTEM` is text from the server to one player alone, such as a mod script's `player:send_message`. `from` is 0 and `from_name` is empty. Clients show it in the chat log, never above a speaker. A client never sends it: the gateway drops chat on any channel but local.
- **Appearance uses table IDs.** `Look` holds sprite and palette IDs from the client data tables (strategy section 10.5).
- **Companions and mounts.** `Look.companion` and `Look.mount` are the record IDs of a player's companion and mount, from `data/companions.yml` and `data/mounts.yml`, or 0 for none. `/client-data.json` names the art of the records that have it, so the client draws a companion of art beside the player and a mount of art under it. The option bits still show the core record each acts as, for clients that do not read the IDs. A player on a mount of art shows its job's body on foot, not the mounted job ([decision 0036](../decisions/0036-phase-6-review-changes.md)).
- **Warp portals are NPCs.** A warp appears as `ENTITY_KIND_NPC` with `look.body` 45, rAthena's `JT_WARPNPC` sprite, and no name. The client draws effect 321 (`EF_WARPZONE2`) on it, as roBrowser does for job 45. Hidden warps (rAthena's `JT_HIDDEN_WARP_NPC`, 139) never appear. A player who walks into a warp's area goes to its target, as rAthena's `npc_touch_areanpc`. Warps are NPCs rather than a kind of their own, because rAthena's script NPCs with the warp sprite will show the same way once scripts run.
- **Other NPCs show their sprite.** An NPC that a mod places appears as `ENTITY_KIND_NPC` with `look.body` set to its NPC sprite ID, its facing in `dir`, and its name. The client loads the sprite `sprite/npc/<look.body>`.
- **NPC conversations.** `NpcClick` names an NPC by its entity ID. It reaches NPCs within 15 cells on both axes, rAthena's `AREA_SIZE + 1` (`npc_checknear`). A dead player, or one already talking, clicks nothing. An NPC with a script starts a conversation. An NPC with a shop and no script opens the shop.
  - The server sends `NpcDialog` steps. A step's lines add to the dialog window, which opens when it is closed. A step with a prompt waits for an answer: `next`, a `menu` of options, `input_number` from `number_min` to `number_max`, `input_text` of at most `text_max` characters, or `close`. `end` closes the window at once.
  - Every step that waits has a new `dialog` ID. `NpcReply` quotes it. The server drops an answer that quotes another ID, does not fit the prompt (a menu option out of range or hidden, a number out of range, text too long), or comes from more than 15 cells away, as `npc_scriptcont` drops it.
  - A menu option is answered from 1. An empty option is hidden and keeps its number. `close` cancels a menu or an input, and ends the conversation, as `select`'s 255.
  - A script that ends with lines on show leaves a Close button. The player still talks until it presses it.
  - While a player talks, the server ignores its walks, attacks, pick ups, item uses and drops (`pc_cant_act`). Chat and stat raises still work.
  - A step times out as rAthena's `SECURE_NPCTIMEOUT`: 60 seconds for next, menus and close, and 180 seconds for input. The window then ends.
  - A conversation ends when the player leaves, dies, or changes map.
  - Lines may hold RO color codes, `^RRGGBB`, and item links, `<ITEMLINK>name<INFO>item ID</INFO></ITEMLINK>`. Clients show them as text, never as HTML.
- **NPC illustrations, progress bars, emotions and marks.** `NpcImage` shows `image/illust/<name>` at the left, center or right of the screen, or hides it for an empty name. The client also hides it when the dialog window closes. `NpcProgress` shows a progress bar over the player for `duration_ms`; the conversation goes on when it is full, and a walk command cancels it and ends the conversation, as `clif_progressbar_abort`. `Emotion` shows an emotion icon over an entity, from `sprite/effect/emotion`, whose action number is the emotion's `ET_` value. `MinimapMark` marks a cell on the minimap, or removes the mark with the same `id`.
- **NPC speech and announcements.** An NPC that speaks aloud sends local chat: a `ChatMessage` with `CHAT_CHANNEL_LOCAL`, `from` set to the NPC's ID and `from_name` to its name, to players within 9 cells. `CHAT_CHANNEL_ANNOUNCEMENT` is a line for every player of a map or of the world, with an optional `color` as 0xRRGGBB. The server sends a countdown this way before it stops.
- **Shops.** `ShopOpen` lists what the NPC sells, with the price for this player and the weight of one, and the inventory slots the shop buys, with their price. Prices follow `pc_modifybuyvalue` and `pc_modifysellvalue`: Discount and Compulsion Discount lower what the player pays when the shop allows it, and Overcharge raises what the player gets.
  - `ShopBuy` names the shop's NPC and 1 to 100 lines of an item and an amount. The server checks the lines in `npc_buylist`'s order: an item the shop sells, a stack within its limit, the zeny, the weight, then the free slots. Equipment is bought one at a time. Then zeny and items change in one step, or nothing changes.
  - `ShopSell` names 1 to 100 slots with amounts. The server checks every line, each slot at most once, before anything changes. Zeny over 2,147,483,647 is lost, as `pc_getzeny`.
  - `ShopClose` closes the window without trading. The server answers each with `ShopResult`, and the window closes. Items leave with `INVENTORY_REMOVE_REASON_SOLD`, and zeny comes as `STAT_ID_ZENY`.
  - A shop opened by a click needs its NPC within 15 cells. A shop a script opened has no NPC: its `npc` is 0.
  - The gateway drops a `ShopBuy` or `ShopSell` with no lines, more than 100, a slot of 200 or more, an amount of 0 or over 30000, or item 0, without a reply. It drops an `NpcReply` without an answer, and cleans `input_text` as chat.
- **Items a script takes.** `InventoryRemove` has `INVENTORY_REMOVE_REASON_TAKEN` for items a script took, as `delitem`.
- **Equipment.** `InventoryItem` and `InventoryAdd` carry an item instance's `refine` and `cards` (a list, strategy section 6.8), where the item can be worn (`location`, as rAthena's `EQP_` bits) and whether it is a card. `InventoryItem.equip` holds where it is worn.
  - `EquipItem` names a slot and the positions asked for, or 0 for every position the item takes. The server answers with `ItemUnequipped` for each item that came off, then `ItemEquipped`, or with `EquipFailed`, as `clif_equipitemack`.
  - `UnequipItem` names a slot. The server answers with `ItemUnequipped`, also for ammunition that came off with a bow or gun.
  - `InsertCard` names a card's slot and an item's slot. The server answers with an `InventoryRemove` of the card (`INVENTORY_REMOVE_REASON_CARD_INSERTED`) and `CardInserted`, or with `InsertCardFailed`.
  - The gateway drops these commands without a reply when a slot is 200 or more.
  - A player's `Look` carries its equipment as rAthena's `view_data`: the weapon type of its attacks, the shield's view, and the views of its headgear and garment. `LookChange` sends a new look to everyone who sees the player.
  - An item whose record names a `view_sprite` gets a view from 1,000,000 at each start, which `/client-data.json` names. Headgear, garments and shields send it as their view. A weapon sends it in `Look.weapon_view`, and `weapon` stays the weapon type ([decision 0036](../decisions/0036-phase-6-review-changes.md)).
- **Rolled item values.** `InventoryItem` and `InventoryAdd` carry a copy's `rolls`, the `card_rolls` of each card in the order of `cards`, and its random `options` (strategy section 5.4, [decision 0014](../decisions/0014-rolled-items.md)).
  - An `ItemRoll` has the roll's `name`, the `label` the client shows, its `value`, `min`, `max` and `unit` ("%" or empty). The client writes "Damage against bosses +31% (15 to 35)".
  - A hidden roll of a copy that is not identified has `known` false and `value` 0, so the client never gets a hidden value.
  - An `ItemOption` has the option's `name`, `label`, `value` and `unit`, and `flag` for an option without an amount, whose label alone shows.
  - `ItemRollsChange` sends a slot's new rolls and options, as when a mod rerolls an equipped copy. The client never sends roll values.
- **Job names.** `STAT_ID_JOB` is the player's job ID, which may be a mod job's ID from 10,000. `JobName` names the job with its record's `display_name`, after login and after each job change, since the client data tables know only rAthena's jobs. The look's `body` is the job's `look`, whose sprites the client has.
- **Monster chat colors.** A monster's chat line is local chat whose `color` is the skills list entry's `chat_color`, rAthena's red by default.
- **Status effects.** `StatusIcon` shows or hides one of the player's own status icons, as `clif_status_change`: the icon's `EFST_` ID, `on`, the time left in `remaining_ms` (-1 for a status without an end, 0 when the icon goes), and up to three values. Only the player gets its own icons. On a map change, the new map sends the icons of the statuses that moved with the player again, after `MapChange`.
  - `Options` holds what others see of a unit's statuses, as rAthena's `opt1`, `opt2`, `opt3` and `option`: `body` is one state such as 1 stone, 2 frozen, 3 stunned or 4 asleep; `health` has bits such as 1 poisoned, 2 cursed, 4 silenced and 16 blind; `effect` has visual effect bits; `option` has bits such as 2 hiding and 4 cloaking.
  - `EntityAppear.options` carries them when a unit comes into view. `OptionChange` sends new options to everyone who sees the unit, as `clif_changeoption`.
- **Skills.** `SkillList` lists the player's skills, as `clif_skillinfoblock`: each skill's ID, level, maximum level in the job's tree, names, target type, SP cost at its level, range, whether a skill point can raise it now, the skill whose icon and effects it shows, and the skills it needs. The first frame carries it, and it comes again after a skill-up and after a job level up. Skill points come as `STAT_ID_SKILL_POINTS`.
  - `LearnSkill` names a skill. The server raises it by one level when a skill point allows it, as `pc_skillup`, and sends the new list. A skill-up that fails sends nothing.
  - `UseSkill` names a skill, a level, and an entity or a cell. The server checks it as `clif_parse_UseSkillToId` and `clif_parse_UseSkillToPos` do, then casts it as `unit_skilluse_id2` and `unit_skilluse_pos2`. A target out of range makes the player walk into range first, and the skill is cast when the walk gets there.
  - Everyone who sees the caster gets `SkillCast` when a cast starts, with the cast time and the skill's element. A skill without a cast time sends it with `cast_ms` 0. `SkillCastCancel` shows a cast that stopped, such as from a hit.
  - Skill damage is a `Damage` with `skill_id` set: one per target, with the total in `amount` and the hit count in `hits`. A hit from a ground unit has the unit as its source, or the caster when the viewer does not see the unit.
  - `SkillEffect` shows a skill that took effect without damage, as `clif_skill_nodamage`, with `value` for what it shows, such as the HP a heal gave. `ok` is false when the skill failed on its target. A ground skill sends it with the cell, as `clif_skill_poseffect`.
  - Ground skills place units: entities of `ENTITY_KIND_SKILL_UNIT` with a `SkillUnit` holding rAthena's `UNT_` ID, the skill, and the caster. They vanish with `VANISH_REASON_EXPIRED`.
  - `SkillFailed` tells the player why a skill did not start or finish, as `clif_skill_fail`. `SkillCooldown` tells the player how long until a skill can be used again, or with `skill_id` 0, until any skill can (the after-cast delay).
  - A mod's skill shows another skill's icon and effects: `SkillInfo.icon`, and the `skill_id` of the events, carry that skill's ID.
  - The gateway drops a `UseSkill` or `LearnSkill` whose skill ID is 0 or past the int32 range, a `UseSkill` whose level is past it, and one whose cell is over 32767, without a reply.
- **A warp to another map is a map change, on the same connection.** The first frame from the new map starts with `MapChange`: the new map's asset name, the player's cell, and the player's own entity ID there. Then, as after `Welcome`:
  - The player's own `EntityAppear` follows, and then the inventory and stats again.
  - Entity IDs start again on the new map. The client removes every entity it showed before the change.
  - The client loads the new map, and applies the events after `MapChange`, in order, once it has loaded.
  - Frames from the old map can arrive before the change. Commands sent while the player moves between maps are dropped, as rAthena ignores them during a map change, and `Frame.ack_seq` continues from the old map.
  - Other players on the old map see the player vanish with `VANISH_REASON_OUT_OF_SIGHT`, as rAthena's `pc_setpos` with `CLR_OUTSIGHT`.
  - A warp within the same map sends no `MapChange`. The player's own `EntityAppear` comes again at the new cell, as after a revive.
- **World mode.** `Welcome.world` is a `WorldInfo`: the world's `mode`, `WORLD_MODE_CLASSIC` or `WORLD_MODE_PROCEDURAL`, and for a procedural world its `generator_version` and `rng_version`, such as 1 and `rng-v1` (strategy section 8.1, [generator-v1](../specs/generator-v1.md), [decision 0038](../decisions/0038-generator-core.md)). The client loads its map generator module only for a procedural world. A client takes a `Welcome` without `world`, or with `WORLD_MODE_UNSPECIFIED`, as a classic world. The protocol version stays 1, since `v1` can change freely before the first public release.
- **World layout.** A procedural world sends `WorldMap` after login and each time the character enters a map for the first time (proposal 0002, review decision 43, [decision 0041](../decisions/0041-world-layout.md)):
  - It holds the maps the character visited, with their display names and level bands, and the maps their warps lead to, without them. `links` are the warps between those maps, each pair once.
  - Each `WorldMap` replaces the one before. A classic world sends none.
  - Display names of generated maps also come with each map's info at `/gen/maps/<id>.json`, over HTTP.
  - The command `Memo` saves a Warp Portal memo point at the player's cell, as the client's `/memo`. Its answer is a system line.
- **Quests.** The quest log follows rAthena's quest window (proposal 0002, review decision 45, [decision 0043](../decisions/0043-quests.md)):
  - `QuestList` comes after login and after each map change, and replaces the list before. It holds the active and inactive quests in the order the character took them. Complete quests are never in it.
  - A `QuestInfo` has the quest's `id` and `title`, `active`, `limit_ms`, the time left until its time limit ends (0 once it passed, -1 without a limit), and its `objectives`. A `QuestObjective` has a `label` (the monster's name, or the text of the objective's filters, as rAthena's `clif_quest_string`), the `monster` ID or 0, the `count` and the `goal`, and `min_level` and `max_level`.
  - `QuestUpdate` adds a quest or replaces the one with the same `id`: a new quest, a kill counted, or a change between active and inactive. `QuestRemove` takes one out: removed, changed into another, or complete.
  - The command `QuestActive` marks a quest active or inactive, as `clif_parse_questStateAck`. The answer is a `QuestUpdate`. A quest the log lacks, or holds as complete, gets no answer.
  - `QuestIcon` shows an icon over an NPC to this player alone, as rAthena's `showevent`: `npc` is the NPC's entity ID on the connection, and `icon` a name of `npc:show_quest_icon`, or empty to remove it. The server sends it again when the NPC comes back into view. A map change drops the icons.

### Limits

| Limit | Value |
|---|---|
| Largest server message | 64 KB (`schema.MaxMessageBytes`). Larger data, such as maps, uses HTTP. |
| Largest client message | 2 KB (`schema.MaxClientMessageBytes`). |
| Chat line | 255 characters, as Unicode code points, after cleaning. |
| Chat rate | Per connection: a burst of 5 lines, then 1 per second. Lines over the rate are dropped without a reply, as rAthena's `min_chat_delay`. Over 60 dropped lines, less 1 per second, close the connection with `DISCONNECT_REASON_RATE_LIMITED`. |
| Rates | Per connection and per command type, set in the gateway (strategy section 7.4). |

A message over its limit, or one that fails to decode, closes the connection with `DISCONNECT_REASON_BAD_MESSAGE`.

### Bandwidth check

`schema/protocol_test.go` builds sample events, measures their encoded size, and estimates bytes per second. It adds 26 bytes per message for the WebSocket and TLS headers.

| View | Assumptions | Estimate | Budget |
|---|---|---:|---:|
| Busy town | 100 players in view, each starting a walk every 3 s, 2 chat lines/s. | 1.9 KB/s | 8 KB/s |
| Town peak | 300 players in view, each starting a walk every 2 s, 10 chat lines/s. | 6.1 KB/s | 32 KB/s |
| Field | 15 monsters and a party of 3, 6 hits/s, 1 skill/s. | 0.7 KB/s | 2 KB/s |

Encoded sizes: a 10-step walk is 26 bytes, a player appearing is 68 bytes, and a hit is 21 bytes. The test fails if an estimate goes over its budget.

The 500-bot load tests measured the real traffic (strategy section 3.8, "Load test results"). Without TLS, a bot in a busy town got 1.2 KB/s on average, and a bot in a crowd of up to 500 players in view got 8.5 KB/s.

## Breaks

Nothing is deployed yet, so nothing breaks. Phase 3 step 4 added `ShopBuy`, `ShopSell`, `ShopClose`, `NpcImage`, `NpcProgress`, `Emotion`, `MinimapMark`, `ShopOpen` and `ShopResult`, the `end` prompt and the input limits of `NpcDialog`, `ChatMessage.color`, `CHAT_CHANNEL_ANNOUNCEMENT`, and two remove reasons. All are additive. `CHAT_CHANNEL_SYSTEM` is a new enum value that older clients ignore. `MapChange` is a new `Event` case, and warp portals are a new use of `ENTITY_KIND_NPC`, so the protocol version stays 1. The inventory changed messages that no deployed client used: `InventoryAdd` gained `slot`, and `UseItem.slot` now refers to `InventoryUpdate`, which exists. Phase 4 step 1 added `EquipItem`, `UnequipItem`, `InsertCard`, `ItemEquipped`, `ItemUnequipped`, `EquipFailed`, `CardInserted`, `InsertCardFailed` and `LookChange`, the equipment fields of `InventoryItem` and `InventoryAdd`, and a remove reason, and gave the equipment fields of `Look` their meaning. All are additive, so the protocol version stays 1. Phase 4 step 6 added `ItemRoll`, `CardRolls`, `ItemOption`, `ItemRollsChange`, and the roll fields of `InventoryItem` and `InventoryAdd`. They are additive too, so the protocol version stays 1. Phase 4 step 7 added `JobName` and gave `ChatMessage.color` a use in local chat. Both are additive, so the protocol version stays 1. Review decision 41 of proposal 0002 added `Look.weapon_view`, `Look.companion` and `Look.mount`, and local chat with `from` 0 from a hidden staff member. Older clients ignore the fields and show the line in the log, so the protocol version stays 1 ([decision 0036](../decisions/0036-phase-6-review-changes.md)). Phase 7 step 6a added `QuestList`, `QuestUpdate`, `QuestRemove`, `QuestIcon` and the command `QuestActive`, all new cases of `Event` and `Command`, so the protocol version stays 1 ([decision 0043](../decisions/0043-quests.md)).

## Alternatives

| Option | Why not |
|---|---|
| FlatBuffers | Reads without copying, but messages are larger, and Go decoding of untrusted input needs a separate verify step. The copy it saves is small at these message sizes. |
| Cap'n Proto | TypeScript support is weak. |
| MessagePack or CBOR without a schema | No generated types, no checks for breaking changes, and field names cost bytes unless mapped by hand. |
| JSON | 3 to 5 times larger, and slower to parse in large frames. |
| A schema language of our own | Code generation, fuzzing, and breaking-change checks would all be ours to build and maintain. |
| gRPC-Web or Connect | Built for request and response. The game needs a server-driven stream every tick. Services still use gRPC between themselves (strategy section 4.1, item 6). |

## Open points for later proposals

- Messages for party, guild, trade, vending, storage, mail, and cash, item, point and market shops.
- Admin tool messages (strategy section 10.6), which may use a separate endpoint.
