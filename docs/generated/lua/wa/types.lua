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
---| "click"
---| "touch"
---| "use"
---| "damage"
---| "skill_used"
---| "unequip"

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
---@field party nil nil: parties do not exist yet.
---@field sex "male"|"female"
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

---How many of the item the player has, as countitem.
---
---Needs the permission `game.read`.
---@param item integer An item from items.id.
---@return integer
function wa_Player:count_item(item) end

---Takes items, as delitem. Returns false, and takes nothing, when the player has too few.
---@param item integer An item from items.id.
---@param amount integer 1 or more.
---@return boolean
function wa_Player:take_item(item, amount) end

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

---Raises an error until status effects exist (phase 4).
---@param status integer A status from wa.statuses.
---@param options? table
function wa_Player:start_status(status, options) end

---Raises an error until timed bonuses exist (phase 4).
---@param key string
---@param milliseconds integer
---@param bonuses table
---@param options? table
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

---A monster. A killed monster's handle reads while its death event runs, with is_alive false, and raises an error after it. Two handles to one monster are equal with ==. A monster that comes back after it died is a new monster with a new handle. Reading needs game.read.
---@class wa.Monster
---@field id integer The monster's ID, such as 1002.
---@field name string
---@field level integer
---@field hp integer
---@field max_hp integer
---@field map string
---@field x integer
---@field y integer
---@field is_alive boolean
local wa_Monster = {}

---An NPC: the instance of an NPC context.
---@class wa.Npc
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

---An equipped copy: the instance of an equip context, or what player:get_equipped returns.
---@class wa.EquippedItem
---@field item integer The item's ID.
---@field refine integer
---@field enchant_grade integer 0 for none, 1 to 4 for D to A.
---@field wearer wa.Player
---@field weapon_type integer
---@field weapon_level integer
---@field armor_level integer
local wa_EquippedItem = {}

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
---
---Provisional: not reviewed in proposal 0002 yet.
---@param time string Such as "2026-10-31 19:58".
---@param time_zone? string An IANA name, such as Asia/Manila. UTC by default.
function wa_TestWorld:set_time(time, time_zone) end

---The server time in the world's time zone, as "2026-10-31 20:00".
---
---Provisional: not reviewed in proposal 0002 yet.
---@return string
function wa_TestWorld:get_time() end

---The living monsters of the map, in the order they spawned.
---
---Provisional: not reviewed in proposal 0002 yet.
---@param monster? integer From monsters.id. Every monster without it.
---@return wa.Monster[]
function wa_TestWorld:get_monsters(monster) end

---Every line announced to the world or the map so far, in order.
---
---Provisional: not reviewed in proposal 0002 yet.
---@return string[]
function wa_TestWorld:get_announcements() end

---How many of the item lie on the map, with drops about to fall.
---
---Provisional: not reviewed in proposal 0002 yet.
---@param item integer An item from items.id.
---@return integer
function wa_TestWorld:count_floor_items(item) end

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
---
---Provisional: not reviewed in proposal 0002 yet.
---@class wa.EventInstance
---@field name string The event's full name, such as example.halloween:Pumpkin_King.
---@field map string The map of the zone that runs it.
local wa_EventInstance = {}

---The object of the death event.
---@class wa.Death
---@field victim wa.Player|wa.Monster Readable while the event runs, with is_alive false for a monster.
---@field victim_kind "player"|"monster"
---@field killer wa.Player|wa.Monster|nil nil when nothing killed the unit.
---@field map string
---@field x integer
---@field y integer
---@field skill integer? nil for a normal attack.
local wa_Death = {}

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

---The object of the experience hook: the exp one player gets for a kill, after the shares and the level penalty.
---@class wa.Exp
---@field player wa.Player
---@field monster wa.Monster The killed monster.
---@field base integer
---@field job integer
local wa_Exp = {}

---The options of hooks.on.
---@class wa.HookOptions
---@field priority integer? Higher runs first. 0 by default.
local wa_HookOptions = {}

---What hooks.on returns.
---
---Provisional: not reviewed in proposal 0002 yet.
---@class wa.HookHandle
local wa_HookHandle = {}

---Removes the hook. A second call does nothing.
function wa_HookHandle:remove() end

---What wa.schedules returns.
---
---Provisional: not reviewed in proposal 0002 yet.
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
---@field respawn boolean? true: a dead monster comes back after 5 seconds while the context lives, as a normal spawn. false by default: dead stays dead.
local wa_SpawnSpec = {}

---The object of the damage event (0002a, "Item events").
---@class wa.Damage
---@field source wa.Player|wa.Monster
---@field target wa.Player|wa.Monster
---@field kind "physical"|"magic"|"misc"
---@field range "melee"|"ranged"|nil
---@field skill integer? nil for a normal attack.
---@field element integer
---@field is_critical boolean
---@field amount integer
---@field wearer_is_source boolean
local wa_Damage = {}

---The object of the skill_used event.
---@class wa.SkillUse
---@field skill integer
---@field level integer
---@field target wa.Player? nil for a ground skill.
---@field x integer
---@field y integer
local wa_SkillUse = {}

---The amounts of player:item_heal and player:heal_percent. An amount is a number, or { minimum, maximum }.
---@class wa.HealSpec
---@field hp integer|integer[]?
---@field sp integer|integer[]?
local wa_HealSpec = {}

---Options of player:give_item.
---@class wa.GiveItemOptions
---@field rent_seconds integer? Gives rentals, as rentitem. Rentals do not expire yet.
local wa_GiveItemOptions = {}

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
local wa_NewPlayerSpec = {}

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

