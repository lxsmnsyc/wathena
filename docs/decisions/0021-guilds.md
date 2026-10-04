# 0021: Guilds

- Status: proposed.
- Date: 2026-10-04.
- Code: [server/social](../../server/social) (`guild.go`, `guildrelations.go`, `guildstorage.go`, `guildview.go`, `guildtables.go`), [server/zone](../../server/zone) (`guild.go`, `guildscripts.go`, `skills_guild.go`), [server/emblem](../../server/emblem), [server/account](../../server/account) (`emblem.go`), [server/store](../../server/store) (`guild.go`, `commit.go`, migration 00018), [server/rules](../../server/rules) (`limits.go`), [server/modscript](../../server/modscript) (`guild.go`), [server/gateway](../../server/gateway) (`guild.go`), [server/cmd/waserver](../../server/cmd/waserver) (`guilds.go`, `audit.go`), [tools/convert](../../tools/convert) (`npcguild.go`), [client](../../client) (`guild.ts`, `storage.ts`, `emblems.ts`, `playerMenu.ts`), [e2e](../../e2e) (`guild_test.go`), [mods/examples/guild-hall](../../mods/examples/guild-hall).
- Phase 5, step 5. Builds on strategy sections 6.8, 7.6, 7.10 and 11.3, proposal 0002, review decision 28, and decisions [0017](0017-e2e-and-transfers.md), [0018](0018-chat-and-moderation.md), [0019](0019-ruleset-flags.md) and [0020](0020-parties-and-friends.md).

## Decision

### Guilds in the social registry

- The registry of decision 0020 holds the guilds too, with the same lock, inbox and Store. A zone hands it a player at login, on a map change and at logout. A map change sends the player its guild again.
- With a database, every guild loads at start. New guild IDs start above the highest saved ID and the highest ID the item log or the reports name, so a new guild never shows an old guild's storage log.

### Guilds

They follow rAthena's `guild.cpp` and the char server's `int_guild.cpp`:

- Members sit in slots, as `guild.member[]`. The master is in slot 0, and a master change swaps slots, as `guild_gm_changed`. A new member takes the first free slot and the last position.
- A guild holds `guild_members` (16) and 6 more for each level of Guild Extension, as `guild_calcinfo`.
- Positions follow `mapif_parse_CreateGuild`: "GuildMaster" with every right, "Position 2" and on, and "Newbie" last. A position names who may invite, expel and use the guild storage (`GUILD_PERM_*`), and its exp tax, capped at `guild_exp_limit`.
- Only the master changes positions, the notice, the skills, the relations and the master, and breaks the guild up, as `clif.cpp`'s `gmaster_flag` checks. Inviting and expelling follow the position.
- An invite needs the target online, without a guild or an open invite, party invites included (`invite_request_check`). A player who blocks the inviter refuses at once, without seeing the invite (decision 0018).
- A master change waits `guild_leaderchange_delay`. rAthena compares it with seconds, though its comment says minutes, and wAthena does the same: 1440 seconds by default.
- The master cannot leave. rAthena's client hides the button; its server would leave the guild without a master in slot 0.
- Breaking a guild up needs its exact name and the master as its only member, as `guild_break`. The guild's relations end, and its storage goes with it, as `storage_guild_delete`. Each lost stack gets an item log line with the action `guild`.
- Expulsions keep the last 32 names and reasons, as `MAX_GUILDEXPULSION`.

### Guild exp and skills

- `pc_gainexp` takes the position's tax from base exp before anything else: `exp * tax / 100`, or all of it at 100, as `guild_payexp`. The member's own exp count grows by the tax.
- The guild levels up by `exp_guild.yml` of the world's base and gets a skill point per level, as `guild_calcinfo`, up to level 50. Exp stops at `MAX_GUILD_EXP`.
- rAthena's map server keeps the tax in a cache for 10 seconds and the char server adds it then. The registry adds it at once. Each write of a guild's exp waits on the commit lane, and a later write takes its place, so many kills make one write.
- The master raises skills by `guild_skill_tree.yml`. The registry checks the requirements, which rAthena leaves to its client.

### Guild skills

Only the master uses guild skills, at the guild's level:

- Battle Orders, Regeneration and Restore reach the caster and its guild members in the 15 cell splash. Battle Orders adds 5 STR, INT and DEX, Regeneration raises regen, and Restore heals 90% of HP and SP. They work on GvG maps only, as `skill_check_condition_castbegin`'s `map_flag_gvg2`. No map is a GvG map until sieges come, so they fail elsewhere with the skill failure message, as in rAthena. Zone tests set the zone's `GvG` flag.
- Emergency Call brings every online member but the caster to the caster's cell, on any map, at rAthena's offsets in slot order, after its cast time. A cell that cannot be stood on puts the member on the caster's cell. `emergency_call` (11) allows it outside sieges and on GvG maps only. A ruleset that sets 15 allows it on every map.
- Renewal gives each skill its skill_db cooldown. Pre-renewal blocks all four for the skill's Duration2, as `guild_block_skill`, and blocks them for `guild_skill_relog_delay` when a master logs in or becomes master. The `skill_cooldown` flag picks the mode.
- Official Guild Approval, Kafra Contract, Guard Research, Permanent Guard Up and Development are learned and do nothing yet: they need castles. Guild Extension raises the member limit. Glory of Guild has level 0 in rAthena's tree, and `require_glory_guild` is off.
- Guild Storage, in renewal's tree, sets the guild storage's size.

Left for sieges: the guild auras (Leadership, Glory of Wounds, Soul Cold, Hawk Eyes), Charge Shout Flag and Beating, Emergency Move, the item Emergency Call, the castle skills, and the GvG map flag itself.

### Guild reach of skills

`battle_check_target` adds `BCT_GUILD` for units of one guild, and `BCT_GUILDALLY` for allied guilds unless the flag holds `BCT_SAMEGUILD`. rAthena's `BCT_GUILD` holds `BCT_SAMEGUILD`, so it reaches guildmates only. GuildOnly skills, such as Potion Pitcher and the Sage endows, now reach guildmates, the step 4 part that waited for guilds.

### Alliances and enemies

- A master asks the guild of an online player to be allies. The player answers; with `guild_alliance_onlygm` it must be a master. An alliance is on both sides and ends any enemy relation between the two guilds, as `guild_reply_reqalliance`.
- A master makes another guild an enemy at once. An enemy relation is on one side only, as `guild_opposition`.
- `guild_allies` and `guild_enemies` limit each kind. rAthena counts both against `max_guild_alliance`.

### Guild chat

`Chat` with `CHAT_CHANNEL_GUILD` passes the mute, the filter and the chat hooks, reaches every online member on any map, and is logged with the kind `guild`. A member who blocks the speaker does not see it. The `chat` event's `channel` is `"guild"`.

### Guild storage

- One member at a time has the storage open, as `gstor->status`. The member's position must allow it (`GUILD_PERM_STORAGE`).
- With Guild Storage in the world's tree, which is renewal's, the guild needs the skill, and the storage holds 100 slots and 100 more per level, as `OFFICIAL_GUILD_STORAGE`, up to `guild_storage`. Without it in the tree, the storage holds `guild_storage`.
- rAthena built at the recorder's packet version refuses guild storage in pre-renewal: its tree lacks the skill. wAthena follows rAthena without `OFFICIAL_GUILD_STORAGE` there instead.
- Moves go through `server/transfer`, with the storage's own lock held, in the zone of the member who has it open. The zone commits the storage with the inventory in one transaction: `Store.CommitWith` writes the storage's slots as a diff, after locking the guild's row.
- Guild writes run on the transfer commit lane (`Store.Lane`), each in a savepoint, so a guild's row is written before a commit that writes its storage, and a commit after the guild broke up finds it gone and skips it.
- The storage closes when its member leaves, is expelled, changes maps or logs out, and when the guild breaks up.
- The storage log keeps the last 100 moves in memory. It loads from the item log at start, the last 90 days, as `storage_guild_log_read`.
- An equipped item cannot go in. rAthena's client does not offer it.
- Guild storage opens from core's converted Guild Warehouse Manager, which stands in 27 renewal towns. It reaches the storage through `wa.internal`, a module only the core mod can require. It is not API. The Kafras' guild storage option goes through `F_KafStor`, which also opens the personal storage of step 6, so it still says that storage is not available.
- The converter now places the duplicates of a script NPC that has no map of its own, such as `GuildWarehouse`, when the script converts. That places 889 duplicates in renewal, up from 153, and 150 in pre-renewal, up from 92. NPCs on instance maps, whose names hold "@", are not placed, since instances do not exist yet.
- The item audit of the duplication tests lists every guild storage, read on the goroutine of the first zone with players.

### Emblems

- A master uploads the emblem on the account pages, `/account/guild/emblem`, for a guild one of the account's characters masters.
- `server/emblem` checks it: at most 50,000 bytes, as rAthena's `MAX_EMBLEM_SIZE`; a BMP, GIF or PNG; 24 by 24 pixels, checked from the header before the pixels are decoded. It makes magenta transparent, as the client draws emblems, and encodes a new PNG. The upload is never kept or served (strategy section 7.6).
- The emblem's version goes up by one. The server serves `/emblems/<guild>/<version>.png` with a cache of a year and `Access-Control-Allow-Origin: *`, since the URL changes with each version.
- Members get `GuildInfo` with the new version, and players in sight get `EntityGuild`. `guild_change` runs with `"emblem"`.
- Moderation: players report a guild's name or emblem with `Report.guild_id` and `emblem`. The report keeps a copy of the emblem as it was, and the chat the reporter saw. The staff queue is still left (decision 0018).

### Limits and rules

- Limits, review decision 28: `guild_members` (16, at most 200), `guild_positions` (20, at most 40), `guild_allies` (3, at most 10), `guild_enemies` (3, at most 10) and `guild_storage` (500, at most 2,000). The client gets them in `Welcome.limits`, and Lua reads them as `rules.limits`.
- Rules from rAthena's conf files: `guild_emperium_check` (yes), `guild_exp_limit` (50), `emergency_call` (11), `guild_skill_relog_delay` (300000), `guild_leaderchange_delay` (1440) and `guild_alliance_onlygm` (no).
- Migration 00018 holds slots up to 259, positions up to 40, and storage slots up to 1,999.

### Lua API

Review decision 28, as built:

- `player.guild`, guild handles with `id`, `name`, `master`, `master_name`, `level`, `exp`, `notice` and `member_count`, and `get_members`, `get_member_names`, `get_position`, `get_skill_level`, `is_allied` and `is_enemy`.
- `wa.guilds`: `create`, `get` and `find`, and the guild methods `add`, `remove`, `set_master`, `set_notice`, `give_exp` and `disband`, with the new permission `guild.manage`.
- The events `guild_join`, `guild_leave` and `guild_change`.
- The `experience` hook's object has `guild` and `guild_tax`.
- `rules.limits` has the guild limits.

## Parity

A new recording per mode, `guild.json`, and an extract, `guilds.json`, from rAthena fork commit `f30328d04`:

- Tables: `exp_guild.yml` (49 levels) and `guild_skill_tree.yml` (19 skills in renewal, 15 in pre-renewal) match `CoreGuildTables`, with the guild limits and rules (`TestCoreGuildTablesMatchExtracts`).
- Exp tax: 225 of 225 `pc_gainexp` calls in each mode, at 8 rates, 9 amounts and the three `exp_flag` values, with and without a Battle Manual (`TestGuildTaxMatchesReference`). `exp_flag` 2 is not taxed, and the tax comes before `pc_calcexp`'s bonuses.
- Kills: 10 of 10 in each mode, alone and in parties with each and even share (`TestGuildTaxOnKillsMatchesReference`). The tax comes after the party split and the level penalty.
- Guild skills: 17 of 17 casts in each mode, 153 player results each: statuses with their values, Battle Orders' stats, Restore's heal, Emergency Call's cells on the map and on another map, failures, skill blocks, and second casts (`TestGuildSkillsMatchReference`). The member with rAthena's `disable_call` is left out: wAthena has no such setting yet.
- Relog blocks: the recording's 4 cases, a master's login blocks the four skills in pre-renewal only (`TestGuildSkillRelogDelay`). `guild_gm_changed` is not recorded, since the recorder's players have no client connection.

## Other choices

- `guild_change` runs in the zone where the change was made. A new emblem from the account pages has no zone: it runs in the zone of the master, or of the first online member.
- `guild:get_position` returns nil for a player who is not a member. `guild:remove` returns false for the master.
- `guilds.create` fires `guild_join` with `"script"` for the master. A player who makes a guild from the client gets no `guild_join`, as for parties.
- `guild:set_master` skips `guild_leaderchange_delay`, as `guildchangegm` does.
- `guild:disband` breaks a guild up with its members, which `guild_break` refuses for players.
- The guild window, invites, guild chat, guild storage, the player menu and emblems over players are in the client. The client sizes its lists from the limits.
- Guild names pass the chat filter and the reserved names, as party names.
- A guild that broke up stays readable for the last 1,024 guilds, so a `guild_leave` handler can read its name.

## Left

- Sieges, castles and guardians (tier 2), with the siege-only guild skills listed above and the GvG map flag.
- The guild skill auras, which work outside sieges with rAthena's `guild_aura` 31 but need skill units that follow the master.
- The Kafras' guild storage option (`F_KafStor`, with personal storage in step 6), and rAthena's `disable_call` setting for Emergency Call.
- Item restrictions of guild storage (`Trade.NoGuildStorage`), which the item extract lacks, and rental items.
- Guild-bound items, guild instances and `@gstorage`-style staff tools.
- Public API to open the guild storage, raise guild skills, read the guild storage and warp a guild (review items).
- The staff queue for reports of guild names and emblems.
- Guilds across processes.
