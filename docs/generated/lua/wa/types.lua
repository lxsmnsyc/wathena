---@meta _
-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.

---A context's kind.
---@alias wa.ContextKind
---| "mod"
---| "npc"
---| "item"
---| "skill"
---| "monster"
---| "event"

---An event name of wa.events.
---@alias wa.EventName
---| "login"
---| "logout"
---| "map_enter"
---| "death"
---| "base_level_up"
---| "job_level_up"
---| "job_change"
---| "click"
---| "touch"
---| "use"
---| "damage"
---| "skill_used"
---| "cast"
---| "unequip"
---| "status_start"
---| "status_end"
---| "status_tick"
---| "chat"
---| "party_join"
---| "party_leave"
---| "party_change"
---| "guild_join"
---| "guild_leave"
---| "guild_change"
---| "trade"
---| "shop_buy"
---| "shop_sell"
---| "vend"
---| "buying_store_sale"
---| "mail"
---| "world_event_start"
---| "world_event_end"
---| "restart_planned"
---| "quest_add"
---| "quest_progress"
---| "quest_complete"

---A bonus name of wa.bonuses (proposal 0002a).
---@alias wa.BonusName
---| "str"
---| "agi"
---| "vit"
---| "int"
---| "dex"
---| "luk"
---| "all_stats"
---| "pow"
---| "sta"
---| "wis"
---| "spl"
---| "con"
---| "crt"
---| "all_trait_stats"
---| "patk"
---| "smatk"
---| "res"
---| "mres"
---| "hplus"
---| "crate"
---| "max_hp"
---| "max_sp"
---| "max_ap"
---| "natural_hp_recovery"
---| "natural_sp_recovery"
---| "hp_gain_over_time"
---| "sp_gain_over_time"
---| "hp_loss_over_time"
---| "sp_loss_over_time"
---| "no_hp_recovery"
---| "no_sp_recovery"
---| "atk"
---| "weapon_atk"
---| "refine_atk"
---| "matk"
---| "hidden_matk"
---| "weapon_matk"
---| "def"
---| "status_def"
---| "mdef"
---| "status_mdef"
---| "attack_range"
---| "hit"
---| "flee"
---| "perfect_dodge"
---| "crit"
---| "crit_chance_taken"
---| "perfect_hit_chance"
---| "double_attack_chance"
---| "move_speed"
---| "aspd"
---| "variable_cast_time"
---| "fixed_cast_time"
---| "after_cast_delay"
---| "cooldown"
---| "sp_cost"
---| "uninterruptible_cast"
---| "uninterruptible_cast_everywhere"
---| "always_endure"
---| "damage"
---| "damage_taken"
---| "crit_damage"
---| "non_crit_damage"
---| "crit_damage_taken"
---| "skill_ratio"
---| "ignore_size_penalty"
---| "immunity"
---| "damage_taken_cap"
---| "damage_reflected"
---| "reflected_damage_taken"
---| "ignore_def"
---| "ignore_mdef"
---| "ignore_res"
---| "ignore_mres"
---| "def_as_damage"
---| "attack_element"
---| "defense_element"
---| "cause_status"
---| "coma"
---| "block_healing"
---| "cast"
---| "drain_hp"
---| "drain_sp"
---| "remove_hp"
---| "remove_sp"
---| "deal_max_hp"
---| "deal_max_sp"
---| "set_def"
---| "set_mdef"
---| "break_weapon"
---| "break_armor"
---| "transform_monster"
---| "reflect_magic"
---| "gain_zeny_on_kill"
---| "status_resistance"
---| "skill"
---| "knockback"
---| "no_gemstone_cost"
---| "no_magic_gear_fuel_cost"
---| "hp_per_hit"
---| "sp_per_hit"
---| "hp_per_kill"
---| "sp_per_kill"
---| "experience"
---| "drop_rate"
---| "extra_drop"
---| "steal_chance"
---| "healing_done"
---| "healing_received"
---| "item_hp_recovery"
---| "item_sp_recovery"
---| "full_recovery_on_revive"
---| "unbreakable_weapon"
---| "unbreakable_armor"
---| "unbreakable_helm"
---| "unbreakable_shield"
---| "unbreakable_garment"
---| "unbreakable_shoes"
---| "equipment_break_chance"
---| "unstrippable_weapon"
---| "unstrippable_armor"
---| "unstrippable_helm"
---| "unstrippable_shield"
---| "splash_range"
---| "no_knockback"
---| "see_hidden"
---| "hidden_from_detectors"
---| "max_weight"

---An equipment slot.
---@alias wa.EquipSlot
---| "head_top"
---| "head_middle"
---| "head_low"
---| "armor"
---| "left_hand"
---| "right_hand"
---| "garment"
---| "shoes"
---| "left_accessory"
---| "right_accessory"
---| "ammo"
---| "costume_head_top"
---| "costume_head_middle"
---| "costume_head_low"
---| "costume_garment"
---| "shadow_armor"
---| "shadow_weapon"
---| "shadow_shield"
---| "shadow_shoes"
---| "shadow_right_accessory"
---| "shadow_left_accessory"

---A hook name of wa.hooks.
---@alias wa.HookName
---| "drop_rate"
---| "experience"
---| "status_chance"
---| "status_duration"
---| "skill_damage"
---| "cast_time"
---| "skill_cost"
---| "cast_allowed"
---| "monster_target"
---| "item_roll"
---| "chat"
---| "trade"
---| "vend"
---| "buying_store_sale"
---| "mail"
---| "spawn_table"

---Who a bound copy is bound to.
---@alias wa.Bound
---| "account"
---| "character"
---| "party"
---| "guild"

---A limit that one player holds, which player:expand changes.
---@alias wa.ExpandableLimit
---| "inventory_slots"
---| "storage_slots"
---| "cart_slots"

---An icon of npc:show_quest_icon, as rAthena's QTYPE_ values 0 to 6 and 8: ! and ? marks for quests, jobs and events, a warg, and a warg's face.
---@alias wa.QuestIcon
---| "quest"
---| "quest_question"
---| "job"
---| "job_question"
---| "event"
---| "event_question"
---| "warg"
---| "warg_face"

---A monster's skill state, in the words of rAthena's mob_skill_db.
---@alias wa.MonsterState
---| "idle"
---| "walk"
---| "loot"
---| "dead"
---| "attack"
---| "angry"
---| "chase"
---| "follow"

---A day of the week.
---@alias wa.Weekday
---| "sunday"
---| "monday"
---| "tuesday"
---| "wednesday"
---| "thursday"
---| "friday"
---| "saturday"

---A player. Each access looks the player up again. A handle to a player who left the map raises an error. Reading needs game.read.
---@class wa.Player
---@field kind "player" The entity's kind.
---@field name string The character's name.
---@field job integer The job, such as jobs.knight.
---@field job_class integer rAthena's eaclass(): the job's class flags.
---@field base_level integer
---@field job_level integer
---@field zeny integer
---@field hp integer
---@field max_hp integer
---@field sp integer
---@field max_sp integer
---@field map string The map's name, such as prontera.
---@field x integer
---@field y integer
---@field character_id integer The database ID, 0 without a database.
---@field account_id integer The database ID, 0 without a database.
---@field party wa.Party? The player's party, or nil.
---@field guild wa.Guild? The player's guild, or nil.
---@field sex "male"|"female"
---@field companion integer? The player's companion, from companions.id, or nil.
---@field mount integer? The player's mount, from mounts.id, or nil.
---@field cart integer? The look of the player's cart, from 1, or nil.
---@field base_str integer Allocated points.
---@field base_agi integer Allocated points.
---@field base_vit integer Allocated points.
---@field base_int integer Allocated points.
---@field base_dex integer Allocated points.
---@field base_luk integer Allocated points.
---@field base_pow integer Allocated points. 0 until trait stats exist.
---@field base_sta integer Allocated points. 0 until trait stats exist.
---@field base_wis integer Allocated points. 0 until trait stats exist.
---@field base_spl integer Allocated points. 0 until trait stats exist.
---@field base_con integer Allocated points. 0 until trait stats exist.
---@field base_crt integer Allocated points. 0 until trait stats exist.
local wa_Player = {}

---Shows a line in the player's chat log, as CHAT_CHANNEL_SYSTEM.
---@param text string 1 to 255 bytes.
function wa_Player:send_message(text) end

---Gives items, as getitem. Returns false when they do not fit.
---
---Needs the permission `player.give_items`.
---@param item integer An item from items.id.
---@param amount? integer 1 to 30000. 1 by default.
---@param options? wa.GiveItemOptions
---@return boolean
function wa_Player:give_item(item, amount, options) end

---Gives one use of an item group, as getgroupitem.
---
---Needs the permission `player.give_items`.
---@param group integer A group from items.group.
function wa_Player:give_item_from_group(group) end

---How many of the item the player has, as countitem. With from, how many a storage or the cart holds, as storagecountitem and cartcountitem; nil when it cannot be read: no cart, or a storage window open.
---
---Needs the permission `game.read`.
---@param item integer An item from items.id.
---@param options? wa.FromOptions
---@return integer?
function wa_Player:count_item(item, options) end

---Takes items, as delitem, or with from out of a storage or the cart, as storagedelitem and cartdelitem. Returns false, and takes nothing, when there are too few.
---@param item integer An item from items.id.
---@param amount integer 1 or more.
---@param options? wa.FromOptions
---@return boolean
function wa_Player:take_item(item, amount, options) end

---Opens the account's storage or an extra storage, as openstorage and openstorage2. Returns false when the player has a storage open, or another character of the account has this one open. Scripts charge any fee.
---@param name? string An extra storage of data/storages.yml. A name without a mod prefix is the calling mod's record, then core's. Nothing opens the account's storage.
---@return boolean
function wa_Player:open_storage(name) end

---Opens the guild storage, as guildopenstorage. Returns true, or false and why: "no_guild", "no_storage", "no_permission", "in_use" or "already_open".
---@return boolean, string?
function wa_Player:open_guild_storage() end

---How much of a currency of data/currencies.yml the player has, such as cash for rAthena's #CASHPOINTS. An account's currency is its account's.
---
---Needs the permission `game.read`.
---@param currency string A currency of data/currencies.yml: a full name, such as example.arena:tokens, or a plain name, which is the calling mod's record, then core's, such as cash or kafra.
---@return integer
function wa_Player:get_currency(currency) end

---Gives a currency. An amount over the currency's max is lost, as pc_getcash. A mod changes its own currencies freely. Another mod's, and core's cash and kafra, need currencies.modify.
---@param currency string A currency of data/currencies.yml: a full name, such as example.arena:tokens, or a plain name, which is the calling mod's record, then core's, such as cash or kafra.
---@param amount integer 1 to 2147483647.
function wa_Player:give_currency(currency, amount) end

---Takes a currency, as pc_paycash. Returns false, and takes nothing, when the player has too little. A mod changes its own currencies freely. Another mod's, and core's, need currencies.modify.
---@param currency string A currency of data/currencies.yml: a full name, such as example.arena:tokens, or a plain name, which is the calling mod's record, then core's, such as cash or kafra.
---@param amount integer 1 to 2147483647.
---@return boolean
function wa_Player:take_currency(currency, amount) end

---Sends a mail to a character, online or not, as rAthena's mail command. The items are made for the mail, and the zeny comes from nowhere: the player pays nothing. A mail to a character who is not online arrives after the call returns. Script mail is not returned to its sender when it expires; it is deleted after mail_delete_days, as int_mail.cpp does with mail of send_id 0.
---
---Needs the permission `mail.send`.
---@param mail wa.MailSpec
function wa_Player:send_mail(mail) end

---Opens the player's mailbox window, as openmail. Returns false when the player is trading, vending, running a buying store or has a storage open, as mail_openmail.
---@return boolean
function wa_Player:open_mail() end

---Closes the player's connection and keeps its vending shop or buying store open, as @autotrade. The store opens again after a restart, as autotrade_restore. The player leaves when its store closes or sells out, or when it logs in again. Returns false when the player has no store open.
---@return boolean
function wa_Player:autotrade() end

---Opens the store search window, as searchstores, over the vending shops and buying stores of every map. Results can filter by price, cards and the ranges of rolls and random options. Returns false when the window is open already.
---@param search wa.StoreSearchSpec
---@return boolean
function wa_Player:search_stores(search) end

---Opens the buying store setup window, as buyingstore. The player then picks what to buy. Returns false when the player is vending, has a buying store open, or is trading, as buyingstore_setup.
---@param store wa.BuyingStoreSpec
---@return boolean
function wa_Player:open_buying_store(store) end

---Whether the items fit, by weight and slots, as checkweight.
---
---Needs the permission `game.read`.
---@param item integer An item from items.id.
---@param amount integer
---@return boolean
function wa_Player:can_carry(item, amount) end

---Takes zeny. Returns false, and takes nothing, when the player has too little.
---@param amount integer
---@return boolean
function wa_Player:pay_zeny(amount) end

---Gives zeny. Zeny over 2,147,483,647 is lost.
---
---Needs the permission `player.give_zeny`.
---@param amount integer
function wa_Player:give_zeny(amount) end

---Heals as itemheal, with its bonuses from VIT, INT and skills.
---@param heal wa.HealSpec
function wa_Player:item_heal(heal) end

---Heals as percentheal.
---@param heal wa.HealSpec Amounts in percent.
function wa_Player:heal_percent(heal) end

---Heals all HP and SP.
function wa_Player:heal_fully() end

---Moves the player after the event. Returns false when the map does not run.
---
---Needs the permission `player.warp`.
---@param map string
---@param x integer
---@param y integer
---@return boolean
function wa_Player:warp(map, x, y) end

---A Fly Wing: a random walkable cell of the map.
---
---Needs the permission `player.warp`.
---@return boolean
function wa_Player:warp_to_random_cell() end

---A Butterfly Wing: the save point.
---
---Needs the permission `player.warp`.
---@return boolean
function wa_Player:warp_to_save_point() end

---Sets the save point, as savepoint. Returns false when the map does not run or the cell is not walkable.
---
---Needs the permission `player.warp`.
---@param map string
---@param x integer
---@param y integer
---@return boolean
function wa_Player:set_save_point(map, x, y) end

---Plays an effect on the player for everyone near, as specialeffect2.
---@param effect integer An effect from wa.effects.
function wa_Player:show_effect(effect) end

---maps.find_entities with around set to the entity.
---
---Needs the permission `game.read`.
---@param query wa.EntityQuery A query without an area: it is around the entity, with radius.
---@return (wa.Player|wa.Monster|wa.Npc)[]
function wa_Player:find_nearby(query) end

---False once the entity is removed, has logged out, or has left the zone. It never raises an error.
---@return boolean
function wa_Player:exists() end

---rAthena's hateffect. It shows nothing yet.
---@param effect integer An effect from wa.headgear_effects.
function wa_Player:show_headgear_effect(effect) end

---The level of a skill the player has, or 0, as getskilllv.
---
---Needs the permission `game.read`.
---@param skill integer A skill from skills.id.
---@return integer
function wa_Player:get_skill_level(skill) end

---The copy in a slot, or nil.
---
---Needs the permission `game.read`.
---@param slot wa.EquipSlot
---@return wa.EquippedItem?
function wa_Player:get_equipped(slot) end

---Gives the player bonuses for a time, as rAthena's bonus_script. They are saved across logout, unless ends_on has logout. The player's stats change before it returns. Returns true when the bonuses were added or extended, and false when on_repeat ignored them.
---@param key string 1 to 64 bytes. The option on_repeat says what a second call with the same key does.
---@param milliseconds integer 1 or more.
---@param bonuses table A list of entries: a bonus name, its arguments, and its options, as in bonuses.add, such as { { "str", 5 } }.
---@param options? wa.TimedBonusOptions
---@return boolean
function wa_Player:add_timed_bonuses(key, milliseconds, bonuses, options) end

---Marks a cell on the minimap, as viewpoint. A mark with the same ID replaces it.
---@param id integer
---@param x integer
---@param y integer
---@param color integer 0xRRGGBB.
function wa_Player:mark_minimap(id, x, y, color) end

---Removes a minimap mark.
---@param id integer
function wa_Player:unmark_minimap(id) end

---Opens a shop, as callshop.
---@param shop string A shop record of data/npcs.yml: the mod's own name, or mod:Name.
function wa_Player:open_shop(shop) end

---Changes the player's job, as jobchange with rAthena's pc_jobchange. Job level goes back to 1, and learned skills stay. A transcendent player stays in the line it had before its rebirth. Returns false, and changes nothing, when the job is not a valid next job (jobs.next_jobs) or the player lacks the base or job level that the job's changes_from asks. A male player who changes into a Dancer becomes a Bard, and the reverse. The job_change event follows a change.
---@param job integer A job from wa.jobs or jobs.id.
---@param options? wa.ChangeJobOptions
---@return boolean
function wa_Player:change_job(job, options) end

---Makes a second class at base level 99 and job level 50 or more a High Novice at base and job level 1, with 100 status points, First Aid and Trick Dead, as rAthena's rebirth. Its second class picks its transcendent jobs. Returns false, and changes nothing, for other players.
---@return boolean
function wa_Player:rebirth() end

---Gives the player a companion in place of its current one, as setfalcon. Returns false, and changes nothing, when the player lacks the skills of the companion's requires, such as Falconry Mastery for a falcon.
---@param companion? integer A companion from companions.id, or nil to remove it.
---@return boolean
function wa_Player:set_companion(companion) end

---Puts the player on a mount in place of its current one, as setriding. The mount's acts_as picks its rules: walk speed, the mounted ASPD rule and spear damage. Returns false, and changes nothing, when the player lacks the skills of the mount's requires, such as Riding for a Peco Peco.
---@param mount? integer A mount from mounts.id, or nil to remove it.
---@return boolean
function wa_Player:set_mount(mount) end

---Gives the player a cart of a look, as setcart. Returns false, and changes nothing, without Pushcart or for a look the client does not have.
---@param look? integer A cart look from 1 to 5, or nil to remove the cart.
---@return boolean
function wa_Player:set_cart(look) end

---Lowers every skill learned with skill points to 0 and refunds the points, as resetskill. Quest skills stay, and so does Basic Skill outside the Novice jobs.
function wa_Player:reset_skills() end

---Sets every stat to 1 and gives back the status points of the base level, as resetstatus.
function wa_Player:reset_stats() end

---Lowers one skill, to 0 by default, and refunds its skill points. Returns false, and changes nothing, when another learned skill needs the lowered level and with_dependents is not set, when the skill is not above the level, or for a quest skill.
---@param skill integer A skill from skills.id.
---@param options? wa.ResetSkillOptions
---@return boolean
function wa_Player:reset_skill(skill, options) end

---Sends the player a whisper. It does not pass the chat filter or the chat hooks, and the player's block list does not stop it.
---@param text string 1 to 255 bytes.
---@param options? wa.WhisperOptions
function wa_Player:whisper(text, options) end

---Whether the player blocks the other player's name.
---
---Needs the permission `game.read`.
---@param other wa.Player
---@return boolean
function wa_Player:is_blocking(other) end

---Whether the other player is on the player's friend list. Only players add and remove friends.
---
---Needs the permission `game.read`.
---@param other wa.Player
---@return boolean
function wa_Player:is_friend(other) end

---The names on the player's friend list, in the order they were added.
---
---Needs the permission `game.read`.
---@return string[]
function wa_Player:get_friend_names() end

---The player's staff role, such as "game_master", or nil. The owner role is "owner". A role counts only when the player logged in with a second factor, a TOTP code, a recovery code or a passkey, and the account has TOTP or a passkey on, as on the admin screens.
---
---Needs the permission `game.read`.
---@return string?
function wa_Player:get_staff_role() end

---Whether the player's staff role has a permission. The owner has every permission. It follows the two-factor rule of get_staff_role.
---
---Needs the permission `game.read`.
---@param permission string A staff permission, such as "moderation.mute". An unknown name raises an error that lists the names.
---@return boolean
function wa_Player:has_staff_permission(permission) end

---Whether the player's character entered the town, as the teleport service lists it (review decision 43).
---
---Needs the permission `game.read`.
---@param town wa.Town
---@return boolean
function wa_Player:has_visited(town) end

---Adds the quest to the player's quest log as active, with its time limit from now, as setquest. Returns false, and changes nothing, when the log has the quest already, in any state. The quest_add event follows.
---@param quest integer|string A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error.
---@return boolean
function wa_Player:add_quest(quest) end

---Marks an active or inactive quest complete, as completequest. It leaves the quest window, and kills stop counting for it. Returns false when the log lacks the quest or holds it as complete. The quest_complete event follows.
---@param quest integer|string A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error.
---@return boolean
function wa_Player:complete_quest(quest) end

---Takes the quest out of the log, in any state, as erasequest. Returns false when the log lacks it.
---@param quest integer|string A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error.
---@return boolean
function wa_Player:remove_quest(quest) end

---Replaces a quest with another, which starts active with its time limit from now and no kills, as changequest. Returns false, and changes nothing, when the log lacks from, holds it as complete, or has to already. The quest_add event follows for to.
---@param from integer|string A quest the log holds as active or inactive.
---@param to integer|string A quest the log lacks.
---@return boolean
function wa_Player:change_quest(from, to) end

---The quest's state in the player's log. A quest is inactive when the player marked it so in the quest window.
---
---Needs the permission `game.read`.
---@param quest integer|string A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error.
---@return "none"|"active"|"inactive"|"complete"
function wa_Player:get_quest_state(quest) end

---The kills counted for a hunt objective and the kills it asks for, as checkquest with HUNTING reads them. nil when the player lacks the quest or the quest has no such objective. Kills stop counting at the goal.
---
---Needs the permission `game.read`.
---@param quest integer|string A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error.
---@param objective integer A hunt objective of the quest, from 1.
---@return integer?, integer?
function wa_Player:get_quest_progress(quest, objective) end

---Whether the quest's time limit passed, as checkquest with PLAYTIME. false for a quest without a time limit, and for a quest the player lacks.
---
---Needs the permission `game.read`.
---@param quest integer|string A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error.
---@return boolean
function wa_Player:quest_timed_out(quest) end

---Mutes the player, as rAthena's SC_NOCHAT: it cannot talk until the end. It replaces a mute that has not ended. The player sees the reason and the end time. The mute is saved.
---
---Needs the permission `player.sanction`.
---@param minutes integer 1 to 5,256,000 (ten years).
---@param reason string 1 to 255 characters. The player sees it.
function wa_Player:mute(minutes, reason) end

---Jails the player, as rAthena's pc_jail: it goes to the jail map and cannot warp until the end, then goes back to where it was. It replaces a jail that has not ended. The player sees the reason and the end time. The jail is saved.
---
---Needs the permission `player.sanction`.
---@param minutes integer 1 to 5,256,000 (ten years).
---@param reason string 1 to 255 characters. The player sees it.
function wa_Player:jail(minutes, reason) end

---Ends the player's mute. Returns false when it was not muted.
---
---Needs the permission `player.sanction`.
---@return boolean
function wa_Player:unmute() end

---Raises one of the player's limits by amount, up to the limit's max, and returns the new value, as rAthena's inventory expansion. A negative amount lowers it, not below the base, and deletes nothing. inventory_slots and cart_slots save with the character, storage_slots with the account, whose characters share it. 0 changes nothing and returns the current value.
---
---Needs the permission `player.expand_limits`.
---@param limit wa.ExpandableLimit The limit to change.
---@param amount integer -10,000 to 10,000. A negative amount lowers the limit.
---@return integer
function wa_Player:expand(limit, amount) end

---Lowers one stat, to 1 by default, and refunds what raising it cost, by rAthena's cost table. Returns false when the stat is not above the value.
---@param stat "str"|"agi"|"vit"|"int"|"dex"|"luk"
---@param options? wa.ResetStatOptions
---@return boolean
function wa_Player:reset_stat(stat, options) end

---Starts a status, as sc_start, sc_start2 and sc_start4, with rAthena's resistance and rules. Returns true when it starts. The target's stats change before it returns, as in rAthena. The status_start event runs after the script's call returns.
---@param status integer A status from wa.statuses.
---@param options? wa.StatusOptions
---@return boolean
function wa_Player:start_status(status, options) end

---Ends a status, as sc_end. Returns true when it was active. The target's stats change before it returns. The status_end event runs after the script's call returns.
---@param status integer
---@return boolean
function wa_Player:end_status(status) end

---Whether a status is active, as getstatus.
---
---Needs the permission `game.read`.
---@param status integer
---@return boolean
function wa_Player:has_status(status) end

---An active status, or nil.
---
---Needs the permission `game.read`.
---@param status integer
---@return wa.Status?
function wa_Player:get_status(status) end

---The active statuses, by status.
---
---Needs the permission `game.read`.
---@return wa.Status[]
function wa_Player:get_statuses() end

---A monster. A killed monster's handle reads while its death event runs, with is_alive false, and raises an error after it. Two handles to one monster are equal with ==. A monster that comes back after it died is a new monster with a new handle. Reading needs game.read. The methods control its AI, as rAthena's unit script commands. They raise an error once the monster is gone.
---@class wa.Monster
---@field kind "monster" The entity's kind.
---@field id integer The monster's ID, such as 1002.
---@field name string
---@field level integer
---@field hp integer
---@field max_hp integer
---@field map string
---@field x integer
---@field y integer
---@field is_alive boolean
---@field target wa.Player|wa.Monster|wa.Npc|nil The unit its AI follows (target_id), or nil.
---@field state wa.MonsterState Its skill state, in rAthena's words.
---@field master wa.Player|wa.Monster|nil The unit that summoned it as a slave, or nil.
local wa_Monster = {}

---Plays an effect on the monster for everyone near, as specialeffect.
---@param effect integer An effect from wa.effects.
function wa_Monster:show_effect(effect) end

---maps.find_entities with around set to the entity.
---
---Needs the permission `game.read`.
---@param query wa.EntityQuery A query without an area: it is around the entity, with radius.
---@return (wa.Player|wa.Monster|wa.Npc)[]
function wa_Monster:find_nearby(query) end

---False once the entity is removed, has logged out, or has left the zone. It never raises an error.
---@return boolean
function wa_Monster:exists() end

---The monster takes the target and chases it to attack, as unitattack. Returns false when it cannot target it.
---@param target wa.Player|wa.Monster|wa.Npc
---@return boolean
function wa_Monster:set_target(target) end

---The monster drops its target and goes idle, as mob_unlocktarget.
function wa_Monster:clear_target() end

---The monster walks to a cell, as unitwalk. Its AI waits until it gets there. Returns false when it cannot walk there.
---@param x integer
---@param y integer
---@return boolean
function wa_Monster:walk_to(x, y) end

---The monster uses a skill with the skill's own cast time, as unitskilluseid. No delay of its skills list applies. Returns false when the cast does not start.
---@param skill integer From skills.id.
---@param level integer 1 to 100.
---@param target wa.Player|wa.Monster|wa.Npc The target, or its cell for a ground skill. The monster itself for a skill on itself.
---@return boolean
function wa_Monster:use_skill(skill, level, target) end

---The monster says a line to the players near it, after its name, as unittalk.
---@param text string 1 to 255 bytes.
function wa_Monster:talk(text) end

---The monster stops walking and attacking, and drops its target, as unitstop.
function wa_Monster:stop() end

---Starts a status, as sc_start, sc_start2 and sc_start4, with rAthena's resistance and rules. Returns true when it starts. The target's stats change before it returns, as in rAthena. The status_start event runs after the script's call returns.
---@param status integer A status from wa.statuses.
---@param options? wa.StatusOptions
---@return boolean
function wa_Monster:start_status(status, options) end

---Ends a status, as sc_end. Returns true when it was active. The target's stats change before it returns. The status_end event runs after the script's call returns.
---@param status integer
---@return boolean
function wa_Monster:end_status(status) end

---Whether a status is active, as getstatus.
---
---Needs the permission `game.read`.
---@param status integer
---@return boolean
function wa_Monster:has_status(status) end

---An active status, or nil.
---
---Needs the permission `game.read`.
---@param status integer
---@return wa.Status?
function wa_Monster:get_status(status) end

---The active statuses, by status.
---
---Needs the permission `game.read`.
---@return wa.Status[]
function wa_Monster:get_statuses() end

---An NPC: the instance of an NPC context.
---@class wa.Npc
---@field kind "npc" The entity's kind.
---@field name string The name players see.
---@field map string
---@field x integer
---@field y integer
local wa_Npc = {}

---Shows an emotion over the NPC, as emotion.
---@param emotion integer An emotion from wa.emotions.
function wa_Npc:show_emotion(emotion) end

---Says a line of local chat as the NPC, as npctalk. Players within 9 cells hear it.
---@param text string
function wa_Npc:say_aloud(text) end

---Shows an icon over the NPC to one player, as showevent. It stays while the player is on the map.
---@param player wa.Player A player handle.
---@param icon? wa.QuestIcon nil removes the icon.
function wa_Npc:show_quest_icon(player, icon) end

---Plays an effect on the NPC for everyone near, as specialeffect.
---@param effect integer An effect from wa.effects.
function wa_Npc:show_effect(effect) end

---maps.find_entities with around set to the entity.
---
---Needs the permission `game.read`.
---@param query wa.EntityQuery A query without an area: it is around the entity, with radius.
---@return (wa.Player|wa.Monster|wa.Npc)[]
function wa_Npc:find_nearby(query) end

---False once the entity is removed, has logged out, or has left the zone. It never raises an error.
---@return boolean
function wa_Npc:exists() end

---Starts a status, as sc_start, sc_start2 and sc_start4, with rAthena's resistance and rules. Returns true when it starts. The target's stats change before it returns, as in rAthena. The status_start event runs after the script's call returns.
---@param status integer A status from wa.statuses.
---@param options? wa.StatusOptions
---@return boolean
function wa_Npc:start_status(status, options) end

---Ends a status, as sc_end. Returns true when it was active. The target's stats change before it returns. The status_end event runs after the script's call returns.
---@param status integer
---@return boolean
function wa_Npc:end_status(status) end

---Whether a status is active, as getstatus.
---
---Needs the permission `game.read`.
---@param status integer
---@return boolean
function wa_Npc:has_status(status) end

---An active status, or nil.
---
---Needs the permission `game.read`.
---@param status integer
---@return wa.Status?
function wa_Npc:get_status(status) end

---The active statuses, by status.
---
---Needs the permission `game.read`.
---@return wa.Status[]
function wa_Npc:get_statuses() end

---An equipped copy: the instance of an equip context, or what player:get_equipped returns.
---@class wa.EquippedItem
---@field item integer The item's ID.
---@field refine integer
---@field enchant_grade integer 0 for none, 1 to 4 for D to A.
---@field wearer wa.Player
---@field weapon_type integer
---@field weapon_level integer
---@field armor_level integer
---@field rolls table<string, integer> The copy's rolled values by name, such as rolls.boss_damage. In a card's script, the card's own rolls. Read-only.
---@field cards wa.CardRolls[] The cards in the copy's slots, in slot order, with their rolls.
---@field random_options wa.RandomOption[] The copy's random options, in option order.
local wa_EquippedItem = {}

---Sets one rolled value. The copy's equip scripts start again after the call, so they read the new value.
---
---Needs the permission `items.modify_rolls`.
---@param name string A roll of the item, or of the card for a card's copy.
---@param value integer Clamped to the roll's range.
function wa_EquippedItem:set_roll(name, value) end

---Rolls values again, as a random give: from the roll's distribution when the item_rolls flag is on, the default otherwise, through the item_roll hooks with source kind "script".
---
---Needs the permission `items.modify_rolls`.
---@param name? string One roll. Every roll without it.
function wa_EquippedItem:reroll(name) end

---A card in an equipped copy's slots.
---@class wa.CardRolls
---@field item integer The card's item ID.
---@field rolls table<string, integer> The card's rolled values by name.
local wa_CardRolls = {}

---A random option of a copy, as rAthena's random options.
---@class wa.RandomOption
---@field option string The option's name in data/random_options.yml, such as max_hp.
---@field value integer
---@field param integer
local wa_RandomOption = {}

---The object of the item_roll hook: one roll of a new copy. The innermost result is the server's roll: from the roll's distribution when the item_rolls flag is on and the copy is rolled, the default otherwise.
---@class wa.ItemRoll
---@field item integer The item's ID.
---@field name string The roll's name.
---@field min integer
---@field max integer
---@field default integer
---@field source wa.ItemRollSource Where the copy comes from.
local wa_ItemRoll = {}

---Where a rolled copy comes from: a monster's drop, a script, or an NPC shop.
---@class wa.ItemRollSource
---@field kind "drop"|"script"|"shop"
---@field monster wa.Monster? The killed monster, for a drop.
local wa_ItemRollSource = {}

---A player talking to an NPC. Methods that wait pause the handler until the player answers. When the player leaves, the waiting call raises "the conversation was closed".
---@class wa.Conversation
local wa_Conversation = {}

---Shows lines.
---@param ... string|integer Up to 64 lines, each at most 1,024 bytes.
function wa_Conversation:say(...) end

---Waits for the Next button. The next lines replace the shown ones.
function wa_Conversation:wait_for_next() end

---Shows options and returns the chosen one, from 1.
---@param ... string The options. An empty option is hidden and keeps its number.
---@return integer
function wa_Conversation:choose(...) end

---Waits for a number in the range.
---@param minimum integer
---@param maximum integer
---@return integer
function wa_Conversation:ask_number(minimum, maximum) end

---Waits for text.
---@param maximum_length integer Up to 255.
---@return string
function wa_Conversation:ask_text(maximum_length) end

---Shows the Close button and waits for it, as close2. The handler goes on after it, and later dialog calls raise an error.
function wa_Conversation:close() end

---Waits, as sleep2.
---@param milliseconds integer Up to 10 minutes.
function wa_Conversation:wait(milliseconds) end

---Shows an NPC image, as cutin.
---@param name string An image of image/illust.
---@param position "left"|"center"|"right"
function wa_Conversation:show_image(name, position) end

---Hides the NPC image.
function wa_Conversation:hide_image() end

---Shows a progress bar and waits until it is full. A walk cancels it and ends the conversation.
---@param seconds integer Up to 600.
function wa_Conversation:show_progress(seconds) end

---A run of a typed world event, from context:get_event() (review decision 31). Its monsters and participants are those on the context's map.
---@class wa.EventRun
local wa_EventRun = {}

---The run's monsters that live on the context's map: the boss, the invasion's monsters, or the treasure chests, in the order they spawned.
---
---Needs the permission `game.read`.
---@return wa.Monster[]
function wa_EventRun:get_monsters() end

---The players on the context's map who did at least the rewards' min_share of the damage to the run's monsters there, 1% by default, sorted by name.
---
---Needs the permission `game.read`.
---@return wa.Player[]
function wa_EventRun:get_participants() end

---Ends the run before its time, within a second. An invasion that finishes "won" gives its rewards, as when its last wave dies. world_event_end carries the result.
---@param result "won"|"lost"
function wa_EventRun:finish(result) end

---A named place of a generated map, from wa.places (review decision 42). A place never changes while the server runs.
---@class wa.Place
local wa_Place = {}

---The map's ID, such as "gen:town:t7".
---@return string
function wa_Place:get_map() end

---The place's cell, x and y.
---@return integer, integer
function wa_Place:get_position() end

---The place's tags, its main tag first, such as { "gate", "gate:south" }.
---@return string[]
function wa_Place:get_tags() end

---The place's area: its map ID without gen:, such as "town:t7", "dungeon:crypt:2" or "field:3:-1".
---@return string
function wa_Place:get_area() end

---The level band of the place's map, its lowest and highest level (review decision 43). A place of a map without a band raises an error.
---
---Needs the permission `game.read`.
---@return integer, integer
function wa_Place:get_band() end

---A generation layer's node of the seed tree's secret branch, its own for each mod and map (strategy section 8.2). It works only while the layer runs, and gives the same values for the same world each time.
---@class wa.GenerationNode
local wa_GenerationNode = {}

---A child node. Each label gives its own values, and the order of calls does not matter.
---@param label string 1 to 64 bytes, such as "shrines".
---@return wa.GenerationNode
function wa_GenerationNode:child(label) end

---A stream of numbers from the node. Two streams of one node give the same numbers.
---@return wa.GenerationStream
function wa_GenerationNode:stream() end

---A number for a cell, the same for the same node and cell, from all 64 bits.
---@param x integer
---@param y integer
---@return integer
function wa_GenerationNode:hash(x, y) end

---A stream of numbers of a generation node, from the node's secret ChaCha8 stream (strategy section 8.2).
---@class wa.GenerationStream
local wa_GenerationStream = {}

---The next whole number from minimum to maximum.
---@param minimum integer
---@param maximum integer
---@return integer
function wa_GenerationStream:integer(minimum, maximum) end

---True with a chance of numerator in denominator.
---@param numerator integer
---@param denominator integer Above 0.
---@return boolean
function wa_GenerationStream:chance(numerator, denominator) end

---A town of the world layout, from wa.towns (review decision 43). Two handles of one town are equal.
---@class wa.Town
local wa_Town = {}

---The town's ID, such as "t7", which places.find's scope "town:t7" names.
---
---Needs the permission `game.read`.
---@return string
function wa_Town:get_id() end

---The town's display name, such as "Valheim". It changes when staff rename the town.
---
---Needs the permission `game.read`.
---@return string
function wa_Town:get_name() end

---The town's map ID, such as "gen:town:t7".
---
---Needs the permission `game.read`.
---@return string
function wa_Town:get_map() end

---The town's level band: its lowest and highest level. Bands rise with the warps from the home area.
---
---Needs the permission `game.read`.
---@return integer, integer
function wa_Town:get_band() end

---The options of towns.list.
---@class wa.TownListOptions
---@field visited_by wa.Player? Only the towns this player entered.
local wa_TownListOptions = {}

---A shop of data/npcs.yml, from world.get_shop (review decision 30). Its changes hold for the whole world until the server stops: the shop's file sets it again at start. A stock the methods set is saved.
---@class wa.Shop
local wa_Shop = {}

---Adds an item, or replaces the item of that ID, as npcshopadditem. Without a cost the price is the item's buy price. Returns false when a cost names an item or a currency that does not exist.
---
---Needs the permission `shops.modify`.
---@param item integer An item from items.id.
---@param options? wa.ShopItemOptions
---@return boolean
function wa_Shop:add_item(item, options) end

---Takes an item out of the shop, as npcshopdelitem. Returns false when the shop does not sell it.
---
---Needs the permission `shops.modify`.
---@param item integer An item from items.id.
---@return boolean
function wa_Shop:remove_item(item) end

---Sets an item's stock, which every player shares, and the amount a restock fills it to, as npcshopupdate for a marketshop. Returns false when the shop does not sell the item.
---
---Needs the permission `shops.modify`.
---@param item integer An item from items.id.
---@param stock integer 0 to 2147483647, or -1 for no limit.
---@return boolean
function wa_Shop:set_stock(item, stock) end

---A chat channel, from channels.create or channels.get.
---@class wa.Channel
local wa_Channel = {}

---Shows a line of the channel itself to its members, as rAthena's channelmes. It does not pass the chat filter.
---@param text string 1 to 255 bytes.
function wa_Channel:send(text) end

---Puts the player in the channel, whatever its join rule. Returns false when it was already in it.
---@param player wa.Player A player handle.
---@return boolean
function wa_Channel:add(player) end

---Takes the player out of the channel, even one it may not leave. Returns false when it was not in it.
---@param player wa.Player A player handle.
---@return boolean
function wa_Channel:remove(player) end

---The members' character names, sorted. Members may be on any map.
---
---Needs the permission `game.read`.
---@return string[]
function wa_Channel:get_members() end

---A party, from player.party or wa.parties. Its leader and members are players of this map: a member on another map is nil or left out.
---@class wa.Party
---@field id integer
---@field name string
---@field leader wa.Player? The leader, or nil when it is offline or on another map.
---@field leader_name string The leader's name, online or not.
---@field exp_share "each"|"even" Even shares the exp of a kill among the members on the map, as rAthena's even share.
---@field item_share "each"|"even" Even gives each picked up item to a member on the map.
---@field item_pickup "each"|"shared" Shared lets members pick up the items their party's top attackers may take.
local wa_Party = {}

---The online members on this map, in the party's slot order.
---
---Needs the permission `game.read`.
---@return wa.Player[]
function wa_Party:get_members() end

---Every member's name, online or not, in slot order.
---
---Needs the permission `game.read`.
---@return string[]
function wa_Party:get_member_names() end

---Puts the player in the party, as party_addmember. Returns false when the player has a party, the party is full, or a character of the same account is in it. The party_join event follows, with the reason "script".
---
---Needs the permission `party.manage`.
---@param player wa.Player A player handle.
---@return boolean
function wa_Party:add(player) end

---Takes the player out of the party, as party_delmember. The leader's removal breaks the party up, as in rAthena. Returns false when the player is not in the party.
---
---Needs the permission `party.manage`.
---@param player wa.Player A player handle.
---@return boolean
function wa_Party:remove(player) end

---Makes an online member the leader, as party_changeleader. Returns false when the player is not an online member, or leads already. The party_change event follows.
---
---Needs the permission `party.manage`.
---@param player wa.Player A player handle.
---@return boolean
function wa_Party:set_leader(player) end

---Changes the options it names, as party_changeoption. Even exp share needs the online members' base levels within the ruleset's party_share_level, else exp share stays each. The party_change event follows a change.
---
---Needs the permission `party.manage`.
---@param options wa.PartyOptions
function wa_Party:set_options(options) end

---Breaks the party up, as party_destroy. Each member gets the party_leave event, with the reason "disbanded".
---
---Needs the permission `party.manage`.
function wa_Party:disband() end

---A party's options, in party:set_options and the party_change event.
---@class wa.PartyOptions
---@field exp_share "each"|"even"|nil
---@field item_share "each"|"even"|nil
---@field item_pickup "each"|"shared"|nil
local wa_PartyOptions = {}

---The object of the party_join and party_leave events.
---@class wa.PartyMembership
---@field party wa.Party The party. After it broke up, its fields still read.
---@field player wa.Player The player who joined or left.
---@field reason "invited"|"create"|"left"|"expelled"|"disbanded"|"script" Why: an accepted invite, a party the player made from the client, a leave, an expel, a break up, or a wa.parties call.
local wa_PartyMembership = {}

---The object of the party_change event.
---@class wa.PartyChange
---@field party wa.Party
---@field leader wa.Player? The leader, or nil when it is on another map.
---@field options wa.PartyOptions The party's options after the change.
local wa_PartyChange = {}

---A guild, from player.guild or wa.guilds. Its master and members are players of this map: a member on another map is nil or left out.
---@class wa.Guild
---@field id integer
---@field name string
---@field master wa.Player? The master, or nil when it is offline or on another map.
---@field master_name string The master's name, online or not.
---@field level integer 1 to 50, from exp_guild.yml.
---@field exp integer The exp toward the next level.
---@field notice wa.GuildNotice
---@field member_count integer Every member, online or not.
local wa_Guild = {}

---The online members on this map, in the guild's slot order. The master is first.
---
---Needs the permission `game.read`.
---@return wa.Player[]
function wa_Guild:get_members() end

---Every member's name, online or not, in slot order.
---
---Needs the permission `game.read`.
---@return string[]
function wa_Guild:get_member_names() end

---The member's position, or nil for a player who is not a member.
---
---Needs the permission `game.read`.
---@param player wa.Player A player handle.
---@return wa.GuildPosition?
function wa_Guild:get_position(player) end

---The guild's level of a guild skill, as getgdskilllv.
---
---Needs the permission `game.read`.
---@param skill integer A guild skill's ID, such as skills.id("GD_EXTENSION").
---@return integer
function wa_Guild:get_skill_level(skill) end

---How many of the item the guild storage holds, as guildstoragecountitem.
---
---Needs the permission `game.read`.
---@param item integer An item from items.id.
---@return integer
function wa_Guild:count_item(item) end

---Whether the guilds are allies.
---
---Needs the permission `game.read`.
---@param other wa.Guild
---@return boolean
function wa_Guild:is_allied(other) end

---Whether this guild has the other as an enemy. Enemies are on one side only.
---
---Needs the permission `game.read`.
---@param other wa.Guild
---@return boolean
function wa_Guild:is_enemy(other) end

---Puts the player in the guild with the last position. Returns false when the player has a guild or the guild is full. The guild_join event follows, with the reason "script".
---
---Needs the permission `guild.manage`.
---@param player wa.Player A player handle.
---@return boolean
function wa_Guild:add(player) end

---Takes a member out of the guild. Returns false for a player who is not a member, and for the master. The guild_leave event follows, with the reason "script".
---
---Needs the permission `guild.manage`.
---@param player wa.Player A player handle.
---@return boolean
function wa_Guild:remove(player) end

---Makes a member the master, as guildchangegm, without guild_leaderchange_delay. Returns false for a player who is not a member, or is the master. The guild_change event follows.
---
---Needs the permission `guild.manage`.
---@param player wa.Player A player handle.
---@return boolean
function wa_Guild:set_master(player) end

---Sets the guild notice. Returns false when a part is too long. The guild_change event follows.
---
---Needs the permission `guild.manage`.
---@param title string At most 59 characters.
---@param text string At most 119 characters.
---@return boolean
function wa_Guild:set_notice(title, text) end

---Adds exp to the guild, as guildgetexp, which may level it up. The guild_change event follows a level up.
---
---Needs the permission `guild.manage`.
---@param amount integer 1 to 2147483647.
function wa_Guild:give_exp(amount) end

---Breaks the guild up, with its members. Its storage is lost, as in rAthena. Each member gets the guild_leave event, with the reason "disbanded".
---
---Needs the permission `guild.manage`.
function wa_Guild:disband() end

---A guild's notice.
---@class wa.GuildNotice
---@field title string
---@field text string
local wa_GuildNotice = {}

---A member's position, from guild:get_position.
---@class wa.GuildPosition
---@field name string
---@field can_invite boolean
---@field can_expel boolean
---@field can_use_storage boolean
---@field exp_tax integer The percent of base exp the member gives the guild.
local wa_GuildPosition = {}

---The object of the guild_join and guild_leave events.
---@class wa.GuildMembership
---@field guild wa.Guild The guild. After it broke up, its fields still read.
---@field player wa.Player The player who joined or left.
---@field reason "invited"|"create"|"left"|"expelled"|"disbanded"|"script" Why: an accepted invite, a guild the player made from the client, a leave, an expel, a break up, or a wa.guilds call.
local wa_GuildMembership = {}

---The object of the guild_change event.
---@class wa.GuildChange
---@field guild wa.Guild The guild, whose fields hold the new values.
---@field change "master"|"level"|"notice"|"emblem" What changed.
local wa_GuildChange = {}

---The limits of the world's ruleset, as rules.limits: each limit's base, what a player or guild starts with. A list over a limit that a ruleset lowered keeps its entries and cannot grow. The engine's ceiling bounds each limit's max.
---@class wa.Limits
---@field party_members integer The most members a party holds, as MAX_PARTY. 12 by default, ceiling 256.
---@field friends integer The most names a friend list holds, as MAX_FRIENDS. 40 by default, ceiling 1,000.
---@field block_list integer The most names a block list holds, as MAX_IGNORE_LIST. 20 by default, ceiling 1,000.
---@field guild_members integer The most members a guild holds before Guild Extension, which adds 6 per level up to the max, 76 by default as MAX_GUILD. 16 by default, ceiling 512.
---@field guild_positions integer The positions of a guild, as MAX_GUILDPOSITION. 20 by default, ceiling 40.
---@field guild_allies integer The most allied guilds of a guild, as max_guild_alliance. 3 by default, ceiling 10.
---@field guild_enemies integer The most enemy guilds of a guild, as max_guild_alliance. 3 by default, ceiling 10.
---@field guild_storage integer The slots of a guild storage without renewal's Guild Storage skill, as MAX_GUILD_STORAGE. The skill gives 100 and 100 per level, up to the max. 600 by default, ceiling 10,000.
---@field inventory_slots integer The inventory slots of a new character, as INVENTORY_BASE_SIZE. 100 by default, with the max 200, MAX_INVENTORY. Ceiling 1,000.
---@field storage_slots integer The slots of an account's storage, as MAX_STORAGE. 600 by default, ceiling 10,000.
---@field cart_slots integer The slots of a cart, as MAX_CART. 100 by default, ceiling 1,000.
---@field mail_items integer The most item stacks one mail holds, as MAIL_MAX_ITEM. 5 by default, ceiling 20.
---@field mailbox integer The most mails a mailbox holds. 100 by default, ceiling 1,000.
local wa_Limits = {}

---The share of the experience hook's object.
---@class wa.ExpShare
---@field base integer
---@field job integer
local wa_ExpShare = {}

---The channel of channels.create.
---@class wa.ChannelSpec
---@field name string 1 to 19 letters, digits, _ and -, with or without the #. Players see it as #name. Not public, whisper, party or guild.
---@field display_name string? What players see before each line, 1 to 24 characters, as rAthena's alias. [name] by default.
---@field color integer? The lines' color, 0xRRGGBB. White by default.
---@field join "open"|"invite"|nil "open", the default: any player may join. "invite": only channel:add puts players in.
local wa_ChannelSpec = {}

---The options of player:whisper.
---@class wa.WhisperOptions
---@field from string? The name the player sees, 1 to 24 characters. The NPC's name in an NPC context, else the mod's name.
local wa_WhisperOptions = {}

---The object of the chat event and the chat hook: a line a player says. The hook runs after the server's chat filter, and the event after the line went out.
---@class wa.ChatMessage
---@field player wa.Player The speaker.
---@field channel string "public" for local chat, "whisper", "party", "guild", or a channel's name without its #, such as trade.
---@field text string The line, after the chat filter.
---@field to string? A whisper's target name. nil for other channels.
---@field map string The speaker's map.
local wa_ChatMessage = {}

---The options of a chat handler. Each one narrows the lines that run it.
---@class wa.ChatOptions
---@field channel string? "public", "whisper", "party", "guild" or a channel's name, with or without its #.
---@field map string? A map name.
---@field player wa.Player? One speaker.
local wa_ChatOptions = {}

---The options of world:chat.
---
---It exists only in tests that wactl mod test runs.
---@class wa.ChatTestOptions
---@field channel string? "public", the default, "whisper", or a channel's name.
---@field to string? A whisper's target name.
local wa_ChatTestOptions = {}

---A chat line a test player got, from world:get_chat.
---
---It exists only in tests that wactl mod test runs.
---@class wa.HeardLine
---@field channel string "public", "whisper", "system", or a channel's name with its #.
---@field from string The speaker's name, or empty for a system line or a channel's own line.
---@field text string
local wa_HeardLine = {}

---What events.on returns.
---@class wa.EventHandle
local wa_EventHandle = {}

---Removes the handler. A second call does nothing.
function wa_EventHandle:remove() end

---What wa.timers returns.
---@class wa.Timer
local wa_Timer = {}

---Cancels the timer. A timer of timers.with_bonuses ends its bonuses.
function wa_Timer:cancel() end

---What bonuses.add returns.
---@class wa.BonusHandle
local wa_BonusHandle = {}

---Removes the bonus. A second call does nothing.
function wa_BonusHandle:remove() end

---The world of one test case. It runs one map: the map of its first new_player or spawn_monster, or prontera when advance comes first.
---
---It exists only in tests that wactl mod test runs.
---@class wa.TestWorld
local wa_TestWorld = {}

---Adds a character at a cell, with full HP and SP.
---@param spec wa.NewPlayerSpec
---@return wa.Player
function wa_TestWorld:new_player(spec) end

---Clicks the NPC and answers each step in order. Next is pressed without an answer. Returns everything the NPC said, one line per line. Fails when the conversation asks for more answers than given, or ends with answers left.
---@param player wa.Player A player handle.
---@param npc_name string The NPC's full name, its name in the mod, or the name players see when one NPC of the map has it.
---@param answers? (integer|string)[] A number picks a menu option, a string fills text input, and an integer fills number input.
---@return string
function wa_TestWorld:talk(player, npc_name, answers) end

---Uses one of the player's items. It waits for the item use interval first. Fails when the player has no such item, or the use is refused.
---@param player wa.Player A player handle.
---@param item integer An item from items.id.
function wa_TestWorld:use_item(player, item) end

---Gives the player one copy of the item and puts it on. Returns the equipped copy, as player:get_equipped. Fails when the copy does not fit or cannot be equipped, such as by the player's job or level.
---@param player wa.Player A player handle.
---@param item integer An item from items.id.
---@param options? wa.EquipOptions
---@return wa.EquippedItem
function wa_TestWorld:equip(player, item, options) end

---Moves the zone clock forward. Timers fire and conversations that wait go on.
---@param milliseconds integer
function wa_TestWorld:advance(milliseconds) end

---Places a monster on a cell. It does not come back after it dies.
---@param spec wa.SpawnMonsterSpec
---@return wa.Monster
function wa_TestWorld:spawn_monster(spec) end

---The player kills the monster at once: the death event fires, and the player gets its exp and drops.
---@param player wa.Player A player handle.
---@param monster wa.Monster A living monster: from spawn_monster, get_monsters or spawning.monsters.
function wa_TestWorld:kill(player, monster) end

---Sets the server time and time zone the world starts at. Call it before the world has a map. Without it, the world starts at 2026-01-05 12:00 UTC.
---@param time string Such as "2026-10-31 19:58".
---@param time_zone? string An IANA name, such as Asia/Manila. UTC by default.
function wa_TestWorld:set_time(time, time_zone) end

---The server time in the world's time zone, as "2026-10-31 20:00".
---@return string
function wa_TestWorld:get_time() end

---The living monsters of the map, in the order they spawned.
---@param monster? integer From monsters.id. Every monster without it.
---@return wa.Monster[]
function wa_TestWorld:get_monsters(monster) end

---Every line announced to the world or the map so far, in order.
---@return string[]
function wa_TestWorld:get_announcements() end

---How many of the item lie on the map, with drops about to fall.
---@param item integer An item from items.id.
---@return integer
function wa_TestWorld:count_floor_items(item) end

---The player says a line, as a client's chat: it passes the mute, the chat filter and the chat hooks, then the chat event runs.
---@param player wa.Player A player handle.
---@param text string 1 to 255 characters.
---@param options? wa.ChatTestOptions
function wa_TestWorld:chat(player, text, options) end

---The chat lines the player got so far, oldest first: local chat, whispers, channel lines and system lines.
---@param player wa.Player A player handle.
---@return wa.HeardLine[]
function wa_TestWorld:get_chat(player) end

---Runs a whole trade, as two clients would: a asks, b accepts, both offer, lock and confirm. Returns true when it completed, or false and why: the trade hook's message, or an outcome such as "restricted" or "bound". A refused offer ends the trade. Trading needs Basic Skill level 1, which a new player from world:new_player lacks: give it skills = { [skills.id("NV_BASIC")] = 9 }.
---@param a wa.Player
---@param b wa.Player Steps next to a.
---@param offer_a? wa.TradeOffer
---@param offer_b? wa.TradeOffer
---@return boolean, string?
function wa_TestWorld:trade(a, b, offer_a, offer_b) end

---Closes the player's storage window, as the client's close button.
---@param player wa.Player A player handle.
function wa_TestWorld:close_storage(player) end

---Gives the player's account a staff role, as if it logged in with two-factor login, so player:get_staff_role and player:has_staff_permission see it.
---
---Provisional: not reviewed in proposal 0002 yet.
---@param player wa.Player A player handle.
---@param role? string A role of the default staff roles: admin, game_master or moderator. nil takes the role away.
function wa_TestWorld:set_staff_role(player, role) end

---Opens a shop for the player, as player:open_shop, and buys, as the shop window's Buy. Returns true when the purchase went through, or false and the outcome, such as "out_of_stock", "not_enough_items" or "not_enough_currency".
---@param player wa.Player A player handle.
---@param shop string A shop's full record name of data/npcs.yml, such as example.arena:Prize Desk.
---@param items table<integer, integer> Item IDs and the amount of each to buy.
---@return boolean, string?
function wa_TestWorld:buy(player, shop, items) end

---Plans a restart that many minutes from the test's time, as staff do on the admin screens: restart_planned runs with { at, reason } in the world's zone, and the lines its handlers send go out. Nothing restarts.
---
---Provisional: not reviewed in proposal 0002 yet.
---@param minutes integer 1 to 10080, as the admin screens allow.
---@param reason string 1 to 255 characters.
function wa_TestWorld:plan_restart(minutes, reason) end

---The player's mails, oldest first, after the mail sent so far arrived.
---@param player wa.Player A player handle.
---@return wa.TestMail[]
function wa_TestWorld:get_mails(player) end

---Runs a whole vending sale, as two clients would: the seller puts each line's items in its cart and opens a shop, and the buyer buys. The shop closes after it. Returns true when the sale went through, or false and why: the vend hook's message, or an outcome such as "not_enough_zeny".
---@param seller wa.Player Gets a cart and the Vending skill when it lacks them.
---@param buyer wa.Player Steps next to the seller.
---@param lines wa.TestVendLine[]
---@return boolean, string?
function wa_TestWorld:vend(seller, buyer, lines) end

---A check on a value, from testing.expect. Its functions are called with a dot: testing.expect(n).to_be(3).
---
---It exists only in tests that wactl mod test runs.
---@class wa.Expectation
local wa_Expectation = {}

---The value equals the expected one, with ==.
---@param expected any
function wa_Expectation.to_be(expected) end

---The value is a table with the same contents, compared deeply.
---@param expected table
function wa_Expectation.to_equal(expected) end

---A string value contains the text, or a table value has an element equal to the value.
---@param part any
function wa_Expectation.to_contain(part) end

---The value is true.
function wa_Expectation.to_be_true() end

---The value is false.
function wa_Expectation.to_be_false() end

---The value is nil.
function wa_Expectation.to_be_nil() end

---The value is a function that raises an error containing the text.
---@param text string
function wa_Expectation.to_fail_with(text) end

---The instance of a mod context.
---@class wa.ModInstance
---@field id string The mod's ID.
---@field map string The zone's map.
local wa_ModInstance = {}

---The instance of a use context.
---@class wa.ItemUse
---@field item integer The used item's ID.
local wa_ItemUse = {}

---The instance of an event context: one run of a world event.
---@class wa.EventInstance
---@field name string The event's full name, such as example.halloween:Pumpkin_King.
---@field map string The map of the zone that runs it.
local wa_EventInstance = {}

---The object of the death event.
---@class wa.Death
---@field victim wa.Player|wa.Monster Readable while the event runs, with is_alive false for a monster. victim.kind says which.
---@field killer wa.Player|wa.Monster|nil nil when nothing killed the unit. killer.kind says which.
---@field map string
---@field x integer
---@field y integer
---@field skill integer? nil for a normal attack.
local wa_Death = {}

---The object of the job_change event.
---@class wa.JobChange
---@field player wa.Player The player that changed jobs.
---@field from integer The job before the change.
---@field to integer The new job. A male player who changes into a Dancer is a Bard here.
local wa_JobChange = {}

---The object of the quest_add, quest_progress and quest_complete events.
---@class wa.QuestEvent
---@field player wa.Player The player whose quest log changed.
---@field quest string The quest record's full name, such as example.hunts:wolves, or a core quest's ID as text, such as "7104".
---@field id integer The quest's ID.
---@field objective integer? quest_progress: the hunt objective that counted a kill, from 1.
---@field count integer? quest_progress: the objective's kills now.
---@field goal integer? quest_progress: the kills the objective asks for.
local wa_QuestEvent = {}

---The options of a death handler. Each one narrows the deaths that run it.
---@class wa.DeathOptions
---@field victim wa.Player|wa.Monster|nil One unit.
---@field victims (wa.Player|wa.Monster)[]? Any of these units.
---@field victim_kind "player"|"monster"|nil
---@field monster integer? A monster from monsters.id.
---@field map string? A map name.
local wa_DeathOptions = {}

---The object of the drop_rate hook: one drop of a killed monster.
---@class wa.Drop
---@field player wa.Player? The killer, or nil.
---@field monster wa.Monster The killed monster.
---@field item integer
---@field map string
---@field rate integer rAthena's rate, per 10,000.
local wa_Drop = {}

---The object of the experience hook: the exp one player gets for a kill, after the shares, the even party split and the level penalty. With even exp share, each member on the map gets its own hook run.
---@class wa.Exp
---@field player wa.Player
---@field monster wa.Monster The killed monster.
---@field base integer
---@field job integer
---@field party wa.Party? The player's party, or nil.
---@field share wa.ExpShare The player's own share of the kill, before the even party split and the level penalty. 0 and 0 for a member who did not fight.
---@field guild wa.Guild? The player's guild, or nil.
---@field guild_tax integer The share of base that goes to the guild, by the exp tax of the player's position. The tax applies at the same rate to the base the hook returns.
local wa_Exp = {}

---The options of hooks.on.
---@class wa.HookOptions
---@field priority integer? Higher runs first. 0 by default.
local wa_HookOptions = {}

---What hooks.on returns.
---@class wa.HookHandle
local wa_HookHandle = {}

---Removes the hook. A second call does nothing.
function wa_HookHandle:remove() end

---What wa.schedules returns.
---@class wa.Schedule
local wa_Schedule = {}

---Stops the schedule. A second call does nothing.
function wa_Schedule:cancel() end

---The options of schedules.when.
---@class wa.WhenOptions
---@field lasting_minutes integer? How long the condition must hold, 0 to 10080. 0 by default.
local wa_WhenOptions = {}

---The monsters of spawning.monsters.
---@class wa.SpawnSpec
---@field monster integer From monsters.id.
---@field map string? The context's own map, which is the default. Another map raises an error.
---@field amount integer? 1 to 1000. 1 by default.
---@field x integer? With y, the cell. Without them, a random walkable cell for each monster.
---@field y integer?
---@field area integer? Spreads the monsters within this many cells of x and y.
---@field respawn boolean? true: a dead monster comes back while the context lives, as a spawn line's monster. false by default: dead stays dead.
---@field respawn_milliseconds integer? With respawn, how long a dead monster takes to come back, 1000 to 268435455. 5000 by default, as a spawn line without a delay.
local wa_SpawnSpec = {}

---The object of the damage event (0002a, "Item events").
---@class wa.Damage
---@field source wa.Player|wa.Monster source.kind says which.
---@field target wa.Player|wa.Monster target.kind says which.
---@field kind "physical"|"magic"|"misc"
---@field range "melee"|"ranged"|nil
---@field skill integer? nil for a normal attack.
---@field element integer
---@field is_critical boolean
---@field amount integer
---@field wearer_is_source boolean
local wa_Damage = {}

---The object of the skill_used event: a skill that took effect, after its cast time.
---@class wa.SkillUse
---@field caster wa.Player|wa.Monster|wa.Npc The wearer, for an item's handler. caster.kind says which.
---@field skill integer
---@field level integer
---@field target wa.Player|wa.Monster|wa.Npc|nil nil for a ground skill.
---@field x integer The cell the skill took effect at.
---@field y integer
local wa_SkillUse = {}

---The options of a skill_used handler. Each one narrows the skills that run it.
---@class wa.SkillUsedOptions
---@field skill integer? A skill from skills.id.
---@field caster_kind "player"|"monster"|"npc"|nil
---@field map string? A map name.
local wa_SkillUsedOptions = {}

---The object of the cast event: one cast of the skill, when it takes effect. It stays usable after the event, so timers of the skill's context can use it.
---@class wa.Cast
---@field caster wa.Player|wa.Monster|wa.Npc caster.kind says which.
---@field skill integer
---@field level integer
---@field target wa.Player|wa.Monster|wa.Npc|nil nil for a ground skill.
---@field map string
---@field x integer The cell the skill takes effect at: the target's, or the ground cell.
---@field y integer
local wa_Cast = {}

---Runs rAthena's damage calculation of the skill on the target, with percent as the skill's damage percentage, after the skill_damage hooks. It applies cards and defense, and fires the damage event after the script's call. Returns nil, and does nothing, when the caster is gone or cannot act, as rAthena's skill_addtimerskill.
---@param target wa.Player|wa.Monster|wa.Npc
---@param options? wa.CastDamageOptions
---@return wa.CastDamageResult?
function wa_Cast:deal_damage(target, options) end

---Heals the target with rAthena's heal bonuses, and returns the HP healed. Returns nil, and does nothing, when the caster is gone or cannot act.
---@param target wa.Player|wa.Monster|wa.Npc
---@param amount integer 0 or more.
---@return integer?
function wa_Cast:heal(target, amount) end

---Runs core's own behavior of the skill. It raises an error for a skill that has none, such as a mod's skill.
---@param options? wa.RunDefaultOptions
function wa_Cast:run_default(options) end

---The options of cast:deal_damage. Without one, the skill's data gives the value.
---@class wa.CastDamageOptions
---@field percent integer? The skill's damage percentage, 0 or more. 100 by default.
---@field hits integer? The number of hits, 1 to 30.
---@field type "weapon"|"magic"|"misc"|nil The attack type.
---@field element integer? An element from wa.elements, not all.
local wa_CastDamageOptions = {}

---What cast:deal_damage returns.
---@class wa.CastDamageResult
---@field amount integer The damage dealt.
---@field is_critical boolean
---@field missed boolean
local wa_CastDamageResult = {}

---The options of cast:run_default.
---@class wa.RunDefaultOptions
---@field target wa.Player|wa.Monster|wa.Npc|nil Another target. The cast's target by default.
local wa_RunDefaultOptions = {}

---The instance of a skill's context: the skill, once per zone. Its cast handlers run for every cast.
---@class wa.SkillInstance
---@field skill integer The skill's ID.
local wa_SkillInstance = {}

---The instance of a passive skill's context: one owner's skill, while it is learned. A new context starts when the level changes.
---@class wa.PassiveSkillInstance
---@field skill integer The skill's ID.
---@field level integer The learned level.
---@field owner wa.Player Who learned the skill.
local wa_PassiveSkillInstance = {}

---The object of the skill_damage, cast_time, skill_cost and cast_allowed hooks. The innermost result is rAthena's value.
---@class wa.SkillHook
---@field caster wa.Player|wa.Monster|wa.Npc
---@field target wa.Player|wa.Monster|wa.Npc|nil nil for a ground skill, and for skill_cost.
---@field skill integer
---@field level integer
---@field percent integer? skill_damage: the damage percentage.
---@field milliseconds integer? cast_time: the cast time, after DEX and the cast bonuses.
---@field sp integer? skill_cost: the SP cost.
---@field allowed boolean? cast_allowed: whether rAthena lets the cast start.
local wa_SkillHook = {}

---The object of the monster_target hook: a monster's AI chooses a target. The innermost result is current, rAthena's choice.
---@class wa.MonsterTargetChoice
---@field monster wa.Monster The monster that chooses.
---@field candidates (wa.Player|wa.Monster|wa.Npc)[] The units it may take. The hook returns one of them, or nil.
---@field current wa.Player|wa.Monster|wa.Npc|nil rAthena's choice: the target the monster takes without hooks.
---@field reason "aggressive"|"assist"|"hit"|"change" Why it chooses: it looks for a target, a monster of its kind calls for help, a unit hit it, or it changes chase target.
local wa_MonsterTargetChoice = {}

---A query of maps.find_entities. It has one area: around with radius, x and y with radius, rectangle, line, or cells. The other fields filter and sort.
---@class wa.EntityQuery
---@field around wa.Player|wa.Monster|wa.Npc|nil The area is the cells within radius of the entity. The entity is in its own area: exclude leaves it out.
---@field map string? The context's own map, which is the default. Another map raises an error.
---@field x integer? With y, the area is the cells within radius of the cell.
---@field y integer?
---@field radius integer? 0 to 30 cells, with around or x and y.
---@field shape "square"|"circle"|nil With around or x and y. "square" by default, as rAthena's ranges.
---@field rectangle integer[][]? Two opposite corners, such as { { 100, 100 }, { 110, 105 } }. Sides of at most 61 cells.
---@field line wa.QueryLine? The cells along a line.
---@field cells integer[][]? A list of 1 to 1024 cells, such as { { 100, 100 }, { 101, 100 } }.
---@field kinds ("player"|"monster"|"npc")[]? Keeps these kinds. Every kind by default.
---@field alive boolean? true, the default, keeps living entities; false keeps dead ones.
---@field enemies_of wa.Player|wa.Monster|wa.Npc|nil Keeps the entities this one may attack, by rAthena's rules.
---@field allies_of wa.Player|wa.Monster|wa.Npc|nil Keeps the entities this one may help, by rAthena's rules, with itself.
---@field line_of_sight boolean? Keeps the entities that the center of the area sees: the entity of around, the cell, the middle of the rectangle, the start of the line, or the first cell.
---@field exclude (wa.Player|wa.Monster|wa.Npc)[]? Leaves these out.
---@field sort "nearest"|"farthest"|"lowest_hp"|"random"|nil Distances are from the center of the area. "random" draws from the zone's secret stream.
---@field limit integer? 1 to 1000. 1000 by default.
local wa_EntityQuery = {}

---The line of a query.
---@class wa.QueryLine
---@field from integer[] A cell, such as { 100, 100 }.
---@field to integer[] A cell, at most 61 cells away on each axis.
---@field width integer? 1 to 15 cells. 1 by default.
local wa_QueryLine = {}

---The amounts of player:item_heal and player:heal_percent. An amount is a number, or { minimum, maximum }.
---@class wa.HealSpec
---@field hp integer|integer[]?
---@field sp integer|integer[]?
local wa_HealSpec = {}

---Options of player:give_item.
---@class wa.GiveItemOptions
---@field rent_seconds integer? Gives rentals, as rentitem. Rentals do not expire yet.
---@field roll "random"|"default"|table<string, integer>|nil How the copies' rolls are set: "random" rolls each value, "default" (the default) uses the defaults, and a table sets values, each clamped to its range. A table needs items.modify_rolls, except in a test case's own code.
---@field to string? Where the copies go: "storage", "cart" or an extra storage's name, instead of the inventory. They follow the place's restrictions and bound rules.
---@field bound wa.Bound? Binds the copies, as getitembound. A bound copy cannot be traded or dropped, and goes only into storage its owner shares.
local wa_GiveItemOptions = {}

---A mail as world:get_mails shows it.
---
---It exists only in tests that wactl mod test runs.
---@class wa.TestMail
---@field from string
---@field title string
---@field zeny integer
---@field items table<integer, integer> Item IDs and amounts.
---@field read boolean
---@field returned boolean Set for a mail that came back to its sender.
local wa_TestMail = {}

---One line of world:vend.
---
---It exists only in tests that wactl mod test runs.
---@class wa.TestVendLine
---@field item integer An item the seller holds.
---@field amount integer How many go on sale.
---@field price integer The price of one.
---@field buy integer? How many the buyer buys. None by default.
local wa_TestVendLine = {}

---One side of world:trade.
---@class wa.TradeOffer
---@field items table<integer, integer>? Item IDs and the amount of each to offer.
---@field zeny integer?
local wa_TradeOffer = {}

---Options of player:count_item and player:take_item.
---@class wa.FromOptions
---@field from string "storage", "cart" or an extra storage's name of data/storages.yml.
local wa_FromOptions = {}

---The object of world_event_start and world_event_end (review decision 31).
---@class wa.WorldEvent
---@field name string The event's full name, such as core:Field_Boss.
---@field type "boss"|"invasion"|"treasure_hunt"|"script" The event's type: "script" for an event without one.
---@field result "won"|"lost"|"stopped"|nil How the run ended, at world_event_end. A boss is won when it dies, an invasion when its last wave dies, and a treasure hunt when every chest is opened. A run whose time ran out is lost. nil at world_event_start, and for a script event that ran its time.
local wa_WorldEvent = {}

---The object of restart_planned (review decision 36): { at, reason } when staff plan a restart, and { cancelled = true } when they cancel it.
---@class wa.RestartPlan
---@field at integer? When the server restarts, in Unix seconds. nil when the restart was cancelled.
---@field reason string? Why the server restarts, as staff wrote it. nil when the restart was cancelled.
---@field cancelled boolean? true when staff cancelled the planned restart.
local wa_RestartPlan = {}

---The map of a generation layer: a generated field, town or dungeon floor of a new procedural world (review decision 46). It is read-only.
---@class wa.GenerationMap
---@field id string The map ID, such as gen:field:3:1, gen:town:t2 or gen:dungeon:crypt:2.
---@field kind "field"|"town"|"dungeon"
---@field biome string A field's biome, such as forest, or a town's or a floor's tileset, such as crypt.
---@field tileset string The tileset whose props objects name.
---@field band integer[] The level band: the lowest and the highest level.
---@field width integer The map's width in cells.
---@field height integer The map's height in cells.
---@field places wa.GenerationPlace[] The map's generated places, in the generator's order.
local wa_GenerationMap = {}

---A named place of a generation layer's map, or one a layer returns: its tags, the main tag first, and its cell. The server moves a returned place to the nearest open ground within 8 cells, or leaves it out.
---@class wa.GenerationPlace
---@field tags string[] 1 to 8 tags of 1 to 64 lowercase letters, digits, _, -, . and :.
---@field x integer
---@field y integer
local wa_GenerationPlace = {}

---An object a generation layer puts on its map. Clients draw it from the map's info. It does not block the cell.
---@class wa.GenerationObject
---@field model string A prop of the map's tileset, such as rock or well.
---@field x integer A cell of the map.
---@field y integer
---@field turn integer? 0 to 3 quarter turns. 0 by default.
local wa_GenerationObject = {}

---A boss a generation layer adds to its field's or dungeon floor's spawns, one monster. The server keeps it 30 cells from warps to towns.
---@class wa.GenerationBoss
---@field monster integer A monster from monsters.id.
---@field role "mini_boss"|"field_boss"
---@field x integer? Its cell, with y: it appears within 3 cells of the nearest open ground. Left out, anywhere on the map.
---@field y integer?
---@field respawn integer? Milliseconds before it comes back, 1,000 to 268,435,455. 45 minutes for a mini boss and 90 for a field boss by default.
local wa_GenerationBoss = {}

---What a generation layer adds to its map. Each list is optional.
---@class wa.GenerationResult
---@field places wa.GenerationPlace[]? At most 32.
---@field objects wa.GenerationObject[]? At most 256.
---@field bosses wa.GenerationBoss[]? At most 8.
local wa_GenerationResult = {}

---The map of the spawn_table hook: a field or a dungeon floor of a new procedural world (review decision 44). It is read-only.
---@class wa.SpawnMap
---@field id string The map ID, such as gen:field:3:1 or gen:dungeon:crypt:2.
---@field kind "field"|"dungeon"
---@field biome string A field's biome, such as forest, or a floor's tileset, such as crypt.
---@field band integer[] The level band: the lowest and the highest level.
---@field features string[] The map's features, sorted: water and clearing on fields, room and boss_room on floors.
local wa_SpawnMap = {}

---One spawn of the spawn_table hook's list. The server keeps aggressive monsters 15 cells from arrivals, and bosses 30 cells from warps to towns, whatever the hook returns.
---@class wa.Spawn
---@field monster integer A monster from monsters.id.
---@field count integer 1 to 1,000 monsters.
---@field respawn integer Milliseconds before a dead one comes back, 1,000 to 268,435,455.
---@field role "common"|"uncommon"|"plant"|"rare"|"mini_boss"|"field_boss" A boss monster that is common or uncommon is left out.
---@field area wa.SpawnArea? Where the monsters appear, or nil for the whole map.
local wa_Spawn = {}

---A spawn's area: within w and h cells of x and y.
---@class wa.SpawnArea
---@field x integer 0 to 1,024.
---@field y integer
---@field w integer
---@field h integer
local wa_SpawnArea = {}

---The object of the trade event and the trade hook: a trade between two players. The hook runs before the trade completes, and the event after.
---@class wa.Trade
---@field players wa.Player[] The two players: the one who confirmed last first.
---@field items wa.TradedItem[] Each offered item.
---@field zeny integer[] The zeny each player gave, in the order of players.
local wa_Trade = {}

---A mail of player:send_mail (review decision 30).
---@class wa.MailSpec
---@field to string|integer|wa.Player|nil The recipient: a character's name, a character ID, or a player handle. The player itself by default.
---@field sender string? The name shown as the sender, at most 23 characters. By default the NPC's name in an NPC context, else the mod's name.
---@field title string 1 to 39 bytes.
---@field body string? At most 499 bytes.
---@field zeny integer? 0 to 2147483647.
---@field items wa.MailItem[]? At most mail_items attachments. The items are made for the mail.
local wa_MailSpec = {}

---One attachment of player:send_mail.
---@class wa.MailItem
---@field item integer An item from items.id.
---@field amount integer? 1 to 30000. 1 by default.
---@field roll "random"|"default"|table<string, integer>|nil As give_item's roll. A table needs items.modify_rolls, except in a test case's own code.
---@field bound wa.Bound? Binds the copies, as give_item's bound.
local wa_MailItem = {}

---The store search of player:search_stores.
---@class wa.StoreSearchSpec
---@field uses integer How many searches the window allows, 1 to 1000.
---@field effect "show"|"open"|nil What a click on a result does: "show" (the default) shows the store's place, as SEARCHSTORE_EFFECT_NORMAL; "open" opens the store, as SEARCHSTORE_EFFECT_REMOTE, when it is on the player's map.
local wa_StoreSearchSpec = {}

---The buying store of player:open_buying_store.
---@class wa.BuyingStoreSpec
---@field slots integer How many items the store may buy, 1 to 5, as MAX_BUYINGSTORE_SLOTS.
local wa_BuyingStoreSpec = {}

---Options of shop:add_item.
---@class wa.ShopItemOptions
---@field cost wa.ShopCost? The cost of one. Without it, the item's buy price in zeny.
---@field stock integer? How many the shop has for every player, 1 to 2147483647. No limit by default.
---@field restock string? When the stock fills again: "daily", "weekly", or a time such as "6h", "30m" or "2d". It needs a stock.
local wa_ShopItemOptions = {}

---A cost: zeny, items and currencies, each optional, at most 5 items and currencies together.
---@class wa.ShopCost
---@field zeny integer? 0 to 2147483647. Discount lowers it when the shop allows it.
---@field items wa.ShopCostItem[]? Items the purchase takes, for one bought.
---@field currency table<string, integer>? Currencies of data/currencies.yml and the amount of each, for one bought, such as { cash = 100 }.
local wa_ShopCost = {}

---An item a cost takes.
---@class wa.ShopCostItem
---@field item integer An item from items.id.
---@field amount integer? 1 to 30000. 1 by default.
local wa_ShopCostItem = {}

---The object of the shop_buy and shop_sell events: a purchase from a shop, or a sale to it.
---@class wa.ShopSale
---@field player wa.Player
---@field shop string The shop's full record name.
---@field items wa.SoldItem[] Each line.
---@field zeny integer The zeny the player paid, or got for a sale.
local wa_ShopSale = {}

---The object of the vend and buying_store_sale events and hooks. The hook runs before the sale, and the event after.
---@class wa.StoreSale
---@field seller wa.Player The player who gives the items: the vendor, or the player who sells to a buying store.
---@field buyer wa.Player The player who pays: the vendor's customer, or the buying store's owner.
---@field items wa.SoldItem[] Each line, with its price.
---@field zeny integer The zeny the buyer pays, before the vending tax.
local wa_StoreSale = {}

---The object of the mail event and hook. The hook runs before a player's mail goes out, and the event after any mail, a script's too.
---@class wa.Mail
---@field from string The sender's name.
---@field to string The recipient's name.
---@field items wa.SoldItem[] The attachments.
---@field zeny integer The attached zeny.
local wa_Mail = {}

---One line of a sale or a mail.
---@class wa.SoldItem
---@field item integer The item's ID.
---@field amount integer
---@field price integer? The price of one, for vend and buying_store_sale.
local wa_SoldItem = {}

---One item of a trade.
---@class wa.TradedItem
---@field from wa.Player The player who gave it.
---@field item integer The item's ID.
---@field amount integer
local wa_TradedItem = {}

---The options of a trade handler.
---@class wa.TradeOptions
---@field player wa.Player? Only trades of this player.
local wa_TradeOptions = {}

---Options of timers.with_bonuses.
---@class wa.WithBonusesOptions
---@field restart wa.Timer? Cancels this timer first, if it still runs.
local wa_WithBonusesOptions = {}

---The against and from options of a bonus.
---@class wa.BonusTarget
---@field race integer? From wa.races.
---@field element integer? From wa.elements.
---@field size integer? From wa.sizes.
---@field class integer? From wa.monster_classes.
---@field monster_group integer? From wa.monster_groups.
---@field monster integer? From monsters.id.
---@field job integer? From wa.jobs.
local wa_BonusTarget = {}

---The options table of bonuses.add (0002a). Each name takes the options 0002a lists for it.
---@class wa.BonusOptions
---@field percent boolean?
---@field skill integer?
---@field item integer?
---@field item_group integer?
---@field kind "physical"|"magic"|"misc"|nil
---@field range "melee"|"ranged"|nil
---@field attack "normal"|"skill"|nil
---@field element integer?
---@field weapon_type integer?
---@field against wa.BonusTarget?
---@field from wa.BonusTarget?
---@field chance integer|(fun(): integer)|nil Per 10,000.
---@field on "attack"|"hit_taken"|"skill_used"|nil
---@field target "self"|"enemy"|nil
---@field duration integer?
---@field random_level boolean?
---@field stacking "sum"|"highest"|"lowest"|"last"|"priority"|nil
---@field priority integer?
---@field when (fun(): boolean)? A condition, tracked like a computed bonus.
local wa_BonusOptions = {}

---The options of entity:start_status.
---@class wa.StatusOptions
---@field milliseconds integer|"forever"|nil The duration, 1 to 2,147,483,647, or forever. Without it, the duration rAthena's sc_start looks up for the level, from the status's skill.
---@field level integer? rAthena's val1. 1 by default.
---@field values integer[]? val2 to val4, for the statuses that read them.
---@field chance integer? Per 10,000, before resistance. 10000 by default.
---@field source wa.Player|wa.Monster|wa.Npc|nil Who caused the status: its stats and level count for resistance, and events and get_status show it. Without it, the target's own stats count, as rAthena's sc_start.
---@field cannot_be_avoided boolean? SCSTART_NOAVOID: no resistance, no chance roll, and no boss immunity.
---@field keep_duration boolean? SCSTART_NOTICKDEF: resistance does not shorten the duration.
---@field keep_chance boolean? SCSTART_NORATEDEF: resistance does not lower the chance.
---@field hide_icon boolean? SCSTART_NOICON: the status shows no icon.
local wa_StatusOptions = {}

---An active status, as entity:get_status returns it.
---@class wa.Status
---@field status integer
---@field level integer val1.
---@field values integer[] val2 to val4.
---@field remaining_milliseconds integer|"forever"
---@field source wa.Player|wa.Monster|wa.Npc|nil Who caused it, or nil.
local wa_Status = {}

---The object of status_start, status_end and status_tick.
---@class wa.StatusEvent
---@field target wa.Player|wa.Monster|wa.Npc target.kind says which.
---@field status integer
---@field level integer
---@field source wa.Player|wa.Monster|wa.Npc|nil
---@field milliseconds integer|"forever" The duration at status_start, and the time left at status_end and status_tick.
---@field reason string? Why it started: "skill", "item", "bonus", "script" or "login". Why it ended: "expired", "removed", "replaced", "death", "dispell", "clearance", "logout" or "damage". nil for status_tick.
local wa_StatusEvent = {}

---The options of a status event handler. Each one narrows the events that run it.
---@class wa.StatusEventOptions
---@field status integer? A status from wa.statuses.
---@field target_kind "player"|"monster"|"npc"|nil
---@field map string? A map name.
local wa_StatusEventOptions = {}

---The object of the status_chance and status_duration hooks: a status that starts. The innermost result is rAthena's value after resistance.
---@class wa.StatusHook
---@field target wa.Player|wa.Monster|wa.Npc
---@field status integer
---@field level integer
---@field source wa.Player|wa.Monster|wa.Npc|nil
---@field chance integer? status_chance: the chance, per 10,000.
---@field milliseconds integer? status_duration: the duration.
local wa_StatusHook = {}

---The status of statuses.define.
---@class wa.StatusDefinition
---@field name string 1 to 48 lower case letters, digits and _, starting with a letter. The constant's name in wa.statuses.
---@field icon integer? The status icon (EFST_ ID) players see. None by default.
---@field kind "buff"|"debuff"|nil "buff" by default.
---@field ends_on string[]? What ends the status besides its time: death, dispell, clearance, logout, banishing_buster, refresh, lux_anima, magic_gear_change and damage. Nothing by default.
---@field bonuses (fun(level: integer): table)? Returns bonus entries for a level, as add_timed_bonuses takes them. They apply to players while the status lasts.
---@field tick_milliseconds integer? From 100: status_tick fires at this interval, and once more when the status runs out, as rAthena's statuses that tick.
local wa_StatusDefinition = {}

---Options of player:change_job.
---@class wa.ChangeJobOptions
---@field force boolean? Skips the job change rules. It needs the permission player.change_job.
local wa_ChangeJobOptions = {}

---Options of player:reset_skill.
---@class wa.ResetSkillOptions
---@field to_level integer? The level to lower the skill to. 0 by default.
---@field with_dependents boolean? Also lowers to 0, and refunds, the skills that need a higher level of it. Quest skills among them stay.
local wa_ResetSkillOptions = {}

---Options of player:reset_stat.
---@class wa.ResetStatOptions
---@field to integer? The value to lower the stat to. 1 by default.
local wa_ResetStatOptions = {}

---Options of player:add_timed_bonuses.
---@class wa.TimedBonusOptions
---@field ends_on string[]? What ends the bonuses besides their time: death, dispell, clearance, logout, banishing_buster, refresh, lux_anima, magic_gear_change and damage.
---@field on_repeat "keep"|"extend"|"add"|"replace"|nil What happens when the player has timed bonuses with the same key. "keep", the default and rAthena's: the new ones are ignored. "extend", rAthena's BSF_FORCE_REPLACE: the first copy keeps its values and takes the new duration when it ends later. "add", rAthena's BSF_FORCE_DUPLICATE: both copies apply. "replace": every copy ends and the new one starts, with its values and duration.
---@field icon integer? A status icon (EFST_ ID) to show while they last.
---@field kind "buff"|"debuff"|nil
local wa_TimedBonusOptions = {}

---The character of world:new_player.
---
---It exists only in tests that wactl mod test runs.
---@class wa.NewPlayerSpec
---@field map string? The map, such as prontera. The world's map by default; a world runs one map.
---@field x integer
---@field y integer
---@field job integer? From wa.jobs. A novice by default.
---@field base_level integer? 1 by default.
---@field job_level integer? 1 by default.
---@field zeny integer? 0 by default.
---@field items integer[][]? A list of { item, amount }.
---@field sex "male"|"female"|nil Male by default.
---@field skills integer[][]? Learned skills, a list of { skill, level }.
---@field stats table? Raised stats, such as { str = 50, dex = 30 }. The others are 1.
---@field skill_points integer? Skill points left to spend. 0 by default.
---@field status_points integer? Status points left to spend. Those of the base level by default.
local wa_NewPlayerSpec = {}

---Options of world:equip.
---
---It exists only in tests that wactl mod test runs.
---@class wa.EquipOptions
---@field roll "random"|"default"|table<string, integer>|nil How the copy's rolls are set, as the roll option of player:give_item. A table needs no permission here.
local wa_EquipOptions = {}

---The monster of world:spawn_monster.
---
---It exists only in tests that wactl mod test runs.
---@class wa.SpawnMonsterSpec
---@field monster integer From monsters.id.
---@field map string? The world's map by default.
---@field x integer
---@field y integer
local wa_SpawnMonsterSpec = {}

---A declared integer variable of a player.
---@class wa.IntegerVariable
local wa_IntegerVariable = {}

---The value, or the default: 0, false, "", or nil for a table.
---@param player wa.Player A player handle.
---@return integer
function wa_IntegerVariable:get(player) end

---Changes the value.
---@param player wa.Player A player handle.
---@param value integer nil or the default removes the stored value.
function wa_IntegerVariable:set(player, value) end

---Adds to the value in one step, and returns the new value.
---@param player wa.Player A player handle.
---@param amount integer
---@return integer
function wa_IntegerVariable:add(player, amount) end

---A declared integer server variable.
---@class wa.ServerIntegerVariable
local wa_ServerIntegerVariable = {}

---The value, or the default.
---@return integer
function wa_ServerIntegerVariable:get() end

---Changes the value.
---@param value integer
function wa_ServerIntegerVariable:set(value) end

---Adds to the value in one step, and returns the new value.
---@param amount integer
---@return integer
function wa_ServerIntegerVariable:add(amount) end

---A declared boolean variable of a player.
---@class wa.BooleanVariable
local wa_BooleanVariable = {}

---The value, or the default: 0, false, "", or nil for a table.
---@param player wa.Player A player handle.
---@return boolean
function wa_BooleanVariable:get(player) end

---Changes the value.
---@param player wa.Player A player handle.
---@param value boolean nil or the default removes the stored value.
function wa_BooleanVariable:set(player, value) end

---A declared boolean server variable.
---@class wa.ServerBooleanVariable
local wa_ServerBooleanVariable = {}

---The value, or the default.
---@return boolean
function wa_ServerBooleanVariable:get() end

---Changes the value.
---@param value boolean
function wa_ServerBooleanVariable:set(value) end

---A declared string variable of a player.
---@class wa.StringVariable
local wa_StringVariable = {}

---The value, or the default: 0, false, "", or nil for a table.
---@param player wa.Player A player handle.
---@return string
function wa_StringVariable:get(player) end

---Changes the value.
---@param player wa.Player A player handle.
---@param value string nil or the default removes the stored value.
function wa_StringVariable:set(player, value) end

---A declared string server variable.
---@class wa.ServerStringVariable
local wa_ServerStringVariable = {}

---The value, or the default.
---@return string
function wa_ServerStringVariable:get() end

---Changes the value.
---@param value string
function wa_ServerStringVariable:set(value) end

---A declared table variable of a player.
---@class wa.TableVariable
local wa_TableVariable = {}

---The value, or the default: 0, false, "", or nil for a table.
---@param player wa.Player A player handle.
---@return table?
function wa_TableVariable:get(player) end

---Changes the value.
---@param player wa.Player A player handle.
---@param value? table nil or the default removes the stored value.
function wa_TableVariable:set(player, value) end

---A declared table server variable.
---@class wa.ServerTableVariable
local wa_ServerTableVariable = {}

---The value, or the default.
---@return table?
function wa_ServerTableVariable:get() end

---Changes the value.
---@param value? table
function wa_ServerTableVariable:set(value) end

