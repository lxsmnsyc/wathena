// Package luaapi holds the definitions of the Lua API that mods use
// (proposal 0002): every wa.* module, handle type, event and bonus name.
// It is the one source for:
//   - the Lua Language Server definitions that wactl mod new writes into
//     a mod, and that docs/generated/lua holds;
//   - the API reference in docs/generated/lua-api.md.
//
// Tests in server/modscript and server/zone check these definitions
// against the runtime, so they fail when the two disagree.
package luaapi

import (
	"slices"

	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// Param is one parameter. A Type that ends in "?" is optional.
type Param struct {
	Name, Type, Doc string
}

// Func is a function of a module, or a method of a handle.
type Func struct {
	Name   string
	Params []Param
	// Returns is the Lua Language Server type of the result, or "" for
	// none.
	Returns string
	Doc     string
	// Permission is the permission the call needs, or "".
	Permission string
	// Overloads are more signatures, as fun(...) types.
	Overloads []string
	// Provisional marks API that is not in proposal 0002 yet and waits
	// for review.
	Provisional bool
}

// Field is a field of a handle or a table.
type Field struct {
	Name, Type, Doc string
}

// Class is a handle type or a table that the API passes.
type Class struct {
	// Name is the Lua Language Server class, such as wa.Player.
	Name string
	// Handle is the runtime's name of the handle type, such as player, or
	// "" for a plain table. Several classes may share one handle type.
	Handle string
	Doc    string
	Fields []Field
	// Methods are called with a colon.
	Methods []Func
	// Functions are fields that hold functions, called with a dot.
	Functions   []Func
	Testing     bool
	Provisional bool
}

// Module is one wa.* module.
type Module struct {
	// Name is the require name, such as wa.events.
	Name string
	// Local is the usual local name, such as events.
	Local string
	Doc   string
	// Funcs are called with a dot. Methods are called with a colon.
	Funcs   []Func
	Methods []Func
	// Call is set for a module that is called as a function, such as
	// wa.text.
	Call *Func
	// Constants is set for a constant module of server/constants.
	Constants bool
	// Fields are read-only fields of the module, such as wa.rules'
	// limits.
	Fields []Field
	// Testing modules exist only under wactl mod test.
	Testing     bool
	Provisional bool
}

// Event is one event name of wa.events.
type Event struct {
	Name string
	// Kinds are the context kinds that may listen to it.
	Kinds  []string
	Params []Param
	Doc    string
}

// Alias is a named type.
type Alias struct {
	Name string
	// Values are string literals, or Type is a type expression.
	Values []string
	Type   string
	Doc    string
}

var playerParam = Param{"player", "wa.Player", "A player handle."}
var itemParam = Param{"item", "integer", "An item from items.id."}

// Modules lists every module that the runtime has.
var Modules = []Module{
	{
		Name: "wa.context", Local: "context",
		Doc: "The current instance, kind and mod. Calls outside a setup function or a handler raise an error.",
		Methods: []Func{
			{Name: "get_instance", Returns: "wa.ModInstance|wa.Npc|wa.EquippedItem|wa.ItemUse|wa.EventInstance|wa.SkillInstance|wa.PassiveSkillInstance|wa.Monster",
				Doc: "The instance the context owns: the mod, the NPC, the equipped copy, the used item, the event run, the skill, a passive skill of one owner, or the monster of a monster's AI script."},
			{Name: "get_kind", Returns: "wa.ContextKind", Doc: "The context's kind."},
			{Name: "get_mod", Returns: "string", Doc: "The ID of the context's mod."},
			{Name: "get_event", Returns: "wa.EventRun?", Doc: "The run of a typed world event, in the event context of its script (review decision 31), or nil in other contexts."},
		},
	},
	{
		Name: "wa.events", Local: "events",
		Doc: "Event handlers. The context owns them, and removes them when it ends.",
		Funcs: []Func{{
			Name: "on",
			Params: []Param{{"name", "wa.EventName", "The event's name. An unknown name, or an event of another context kind, raises an error."},
				{"handler", "function", "Runs each time the event fires, in registration order."},
				{"options", "wa.DeathOptions|wa.StatusEventOptions|wa.SkillUsedOptions|wa.ChatOptions|wa.TradeOptions|nil", "Only the death, status, skill_used, chat and trade events take options: they narrow which events run the handler."}},
			Returns:    "wa.EventHandle",
			Doc:        "Registers a handler for an event.",
			Permission: "event.listen",
		}},
	},
	{
		Name: "wa.hooks", Local: "hooks",
		Doc: "Hooks change a game result in a chain ordered by priority. Each hook calls next to get the rest of the chain's result. The innermost result is rAthena's value. Hooks only compute: calls that change the game raise an error in them. The context owns them.",
		Funcs: []Func{{
			Name: "on",
			Params: []Param{{"name", "wa.HookName", "The hook's name. An unknown name raises an error."},
				{"hook", "(fun(object: wa.Drop|wa.Exp|wa.StatusHook|wa.SkillHook|wa.MonsterTargetChoice|wa.ItemRoll|wa.ChatMessage|wa.Trade, next: fun(object?: table): integer|boolean|string|wa.Player|wa.Monster|wa.Npc|nil, integer|string|nil): integer|boolean|string|wa.Player|wa.Monster|wa.Npc|nil, integer|string|nil)|fun(map: wa.SpawnMap, spawns: wa.Spawn[], next: fun(spawns?: wa.Spawn[]): wa.Spawn[]): wa.Spawn[]", "Gets the object and next. drop_rate returns a rate per 10,000; experience returns base and job exp; status_chance returns a chance per 10,000; status_duration and cast_time return milliseconds; skill_damage returns the damage percentage; skill_cost returns SP; cast_allowed returns true or false; monster_target returns one of the candidates, or nil for none; item_roll returns the value, which the server clamps to the roll's range; chat returns the text, or nil to block the message; trade returns true, or false and a message to refuse the trade. spawn_table gets the map, a copy of its spawn list and next, and returns the list: next(spawns) runs the rest of the chain on a list, the hook's own by default."},
				{"options", "wa.HookOptions?", ""}},
			Returns:    "wa.HookHandle",
			Doc:        "Adds a hook. Higher priority runs first; equal priorities run in the order they were added. spawn_table needs spawns.modify in place of rules.override. It runs only when wactl world new makes a procedural world, and in the test worlds of wactl mod test.",
			Permission: "rules.override",
		}},
	},
	{
		Name: "wa.schedules", Local: "schedules",
		Doc: "Schedules on the server clock, in the server's time zone (time_zone in the config). A time that does not exist on a day clocks change runs at the next valid minute, and a time that happens twice runs once. A time that passed while the zone slept or the server was stopped runs once, or not at all. The context owns them.",
		Funcs: []Func{
			{Name: "daily", Params: []Param{{"time", "string", "HH:MM, such as \"20:00\"."}, {"handler", "fun()", ""}}, Returns: "wa.Schedule", Doc: "Runs the handler every day at the time."},
			{Name: "weekly", Params: []Param{{"day", "wa.Weekday", ""}, {"time", "string", "HH:MM."}, {"handler", "fun()", ""}}, Returns: "wa.Schedule", Doc: "Runs the handler every week on the day at the time."},
			{Name: "hourly", Params: []Param{{"minute", "integer", "0 to 59."}, {"handler", "fun()", ""}}, Returns: "wa.Schedule", Doc: "Runs the handler every hour at the minute."},
			{Name: "when", Params: []Param{{"condition", "fun(): boolean", "Reads the game and returns true or false. Calls that change the game raise an error in it."}, {"handler", "fun()", ""}, {"options", "wa.WhenOptions?", ""}},
				Returns: "wa.Schedule", Doc: "Runs the handler once the condition has held for lasting_minutes. The condition runs once a minute, at the start of each minute of the server clock. The handler runs once for each time the condition holds."},
		},
	},
	{
		Name: "wa.spawning", Local: "spawning",
		Doc: "Monsters that the context owns. They go when it ends.",
		Funcs: []Func{{
			Name: "monsters", Params: []Param{{"spec", "wa.SpawnSpec", ""}}, Returns: "wa.Monster[]",
			Doc: "Places monsters on the context's map and returns their handles. Without x and y, each lands on a random walkable cell.", Permission: "npc.create",
		}},
	},
	{
		Name: "wa.timers", Local: "timers",
		Doc: "Timers on the zone clock. The context owns them, and cancels them when it ends.",
		Funcs: []Func{
			{Name: "after", Params: []Param{{"milliseconds", "integer", "0 or more."}, {"callback", "fun()", ""}}, Returns: "wa.Timer", Doc: "Runs the callback once, after the time."},
			{Name: "every", Params: []Param{{"milliseconds", "integer", "Above 0."}, {"callback", "fun()", ""}}, Returns: "wa.Timer", Doc: "Runs the callback every time the interval passes. A timer that falls behind skips the runs it missed."},
			{Name: "with_bonuses", Params: []Param{{"milliseconds", "integer", ""}, {"add", "fun()", "Adds bonuses with bonuses.add."}, {"options", "wa.WithBonusesOptions?", ""}},
				Returns: "wa.Timer", Doc: "Runs the function now. The bonuses it adds end after the time. Item contexts only."},
		},
	},
	{
		Name: "wa.items", Local: "items",
		Doc: "Items by name.",
		Funcs: []Func{
			{Name: "id", Params: []Param{{"name", "string", "An AegisName, such as Red_Potion, or a mod item's full name."}}, Returns: "integer", Doc: "The item's ID. An unknown name raises an error.", Permission: "game.read"},
			{Name: "group", Params: []Param{{"name", "string", "The group's name in item_group_db, such as BLUEBOX."}}, Returns: "integer", Doc: "An item group, for player:give_item_from_group.", Permission: "game.read"},
			{Name: "name", Params: []Param{itemParam}, Returns: "string", Doc: "The item's name from rAthena's item database.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.skills", Local: "skills",
		Doc:   "Skills by name.",
		Funcs: []Func{{Name: "id", Params: []Param{{"name", "string", "A skill_db name, such as SM_BASH, or a mod skill's full name, such as example.chain-lightning:CHAIN_LIGHTNING."}}, Returns: "integer", Doc: "The skill's ID. An unknown name raises an error.", Permission: "game.read"}},
	},
	{
		Name: "wa.companions", Local: "companions",
		Doc:   "Companions by name: core's, such as falcon, and the mods' records of data/companions.yml.",
		Funcs: []Func{{Name: "id", Params: []Param{{"name", "string", "A core companion's name, such as falcon, or a mod companion's full name, such as example.familiars:Owl."}}, Returns: "integer", Doc: "The companion's ID. An unknown name raises an error.", Permission: "game.read"}},
	},
	{
		Name: "wa.mounts", Local: "mounts",
		Doc:   "Mounts by name: core's, such as peco_peco, and the mods' records of data/mounts.yml.",
		Funcs: []Func{{Name: "id", Params: []Param{{"name", "string", "A core mount's name, such as peco_peco, grand_peco, gryphon, dragon, warg or madogear, or a mod mount's full name."}}, Returns: "integer", Doc: "The mount's ID. An unknown name raises an error.", Permission: "game.read"}},
	},
	{
		Name: "wa.monsters", Local: "monsters",
		Doc:   "Monsters by name.",
		Funcs: []Func{{Name: "id", Params: []Param{{"name", "string", "An AegisName, such as PORING."}}, Returns: "integer", Doc: "The monster's ID. An unknown name raises an error.", Permission: "game.read"}},
	},
	{
		Name: "wa.text", Local: "text",
		Doc:  "Text for players, from the mod's locales/en.yml. Call the module with a key: text \"greeter.hello\".",
		Call: &Func{Name: "text", Params: []Param{{"key", "string", "A key of locales/en.yml. Nested keys are joined with dots."}}, Returns: "string", Doc: "The text of a key. A missing key raises an error."},
		Funcs: []Func{
			{Name: "item_link", Params: []Param{itemParam}, Returns: "string", Doc: "A link that conversation:say shows as the item's name, as mesitemlink."},
		},
	},
	{
		Name: "wa.maps", Local: "maps",
		Doc: "Maps.",
		Funcs: []Func{
			{Name: "announce", Params: []Param{{"map", "string", "A map name, such as prontera."}, {"text", "string", "At most 255 bytes."}}, Doc: "Shows a line to every player of a map, as mapannounce. Players see the mod's ID with a mod's line."},
			{Name: "find_entities", Params: []Param{{"query", "wa.EntityQuery", "One area, filters, and an order."}}, Returns: "(wa.Player|wa.Monster|wa.Npc)[]",
				Doc: "The entities of an area of the context's map that pass every filter, sorted, and cut to the limit. Ties sort by kind, then by entity. A query with no area, two areas, or a field it does not know raises an error.", Permission: "game.read"},
			{Name: "get_display_name", Params: []Param{{"map", "string", "A map ID, such as \"gen:town:t7\"."}}, Returns: "string?",
				Doc: "The name players see for a map of the world layout (review decision 43), such as \"North Valheim Fields 2\". It changes when staff rename a town. nil for another map, and for every map of a classic world until locale files exist.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.world", Local: "world",
		Doc: "The world.",
		Funcs: []Func{
			{Name: "announce", Params: []Param{{"text", "string", "At most 255 bytes."}}, Doc: "Shows a line to every player of the world, as announce. Players see the mod's ID with a mod's line.", Permission: "world.announce"},
			{Name: "get_shop", Params: []Param{{"name", "string", "A shop of data/npcs.yml: a full record name, or a plain name, which is the calling mod's record, then core's."}}, Returns: "wa.Shop?",
				Doc: "A shop's handle, or nil when there is none. world:get_shop(name), as review decision 30 writes it, works too.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.world_events", Local: "world_events",
		Doc: "Starting and stopping world events of data/events.yml (review decision 31). A request takes effect within a second, on the scheduler's next check.",
		Funcs: []Func{
			{Name: "start", Params: []Param{{"name", "string", "An event's full name, such as core:Field_Boss, or a plain name, which is the calling mod's event."}}, Returns: "boolean",
				Doc: "Starts a run of the event now, for its duration, as if its trigger fired. Returns false when there is no such event, or it runs already. An event the config turned off does not exist.", Permission: "events.control"},
			{Name: "stop", Params: []Param{{"name", "string", "An event's full name, or a plain name, which is the calling mod's event."}}, Returns: "boolean",
				Doc: "Ends the event's run now, with the result \"stopped\". Its monsters and NPCs go. Returns false when there is no such event, or it does not run.", Permission: "events.control"},
		},
	},
	{
		Name: "wa.channels", Local: "channels",
		Doc: "Chat channels, as rAthena's channel system. Core's channels are the records of data/channels.yml, such as #global and #trade. A channel is one for the whole world.",
		Funcs: []Func{
			{Name: "create", Params: []Param{{"spec", "wa.ChannelSpec", ""}}, Returns: "wa.Channel",
				Doc: "Makes a channel that the context owns, and returns it. The channel ends when its owner ends, and its members leave it. The same mod's context in another zone, or after a reload, gets the same channel. A name that core or another mod has raises an error."},
			{Name: "get", Params: []Param{{"name", "string", "A channel's name, with or without its #, such as trade."}}, Returns: "wa.Channel?", Doc: "A running channel: core's or a mod's. nil when none has the name.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.parties", Local: "parties",
		Doc: "Parties, as rAthena's party system (review decision 27). A party is one for the whole world. The write calls replace party_create, party_addmember, party_delmember, party_changeleader, party_changeoption and party_destroy, and skip the leader's rights and the invite, as those commands do.",
		Funcs: []Func{
			{Name: "create", Params: []Param{{"leader", "wa.Player", ""}, {"name", "string", "1 to 24 characters."}}, Returns: "wa.Party?",
				Doc: "Makes a party with the player as its leader, as party_create, and returns it. Exp share starts as each, and the items as each. nil when the name is taken or the leader has a party. The party_join event follows, with the reason \"script\". A test case needs no permission for it.", Permission: "party.manage"},
			{Name: "get", Params: []Param{{"id", "integer", "A party's ID, such as player.party.id."}}, Returns: "wa.Party?", Doc: "A party by ID, or nil.", Permission: "game.read"},
			{Name: "find", Params: []Param{{"name", "string", "A party's name, without regard to case."}}, Returns: "wa.Party?", Doc: "A party by name, or nil.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.guilds", Local: "guilds",
		Doc: "Guilds, as rAthena's guild system (review decision 28). A guild is one for the whole world. The write calls skip the position permissions and the invite, as rAthena's guild script commands. Castles, sieges and guardians come later.",
		Funcs: []Func{
			{Name: "create", Params: []Param{{"master", "wa.Player", ""}, {"name", "string", "1 to 24 characters."}}, Returns: "wa.Guild?",
				Doc: "Makes a guild with the player as its master and returns it. It takes no Emperium, so the script decides the cost. nil when the name is taken or the master has a guild. The guild_join event follows, with the reason \"script\". A test case needs no permission for it.", Permission: "guild.manage"},
			{Name: "get", Params: []Param{{"id", "integer", "A guild's ID, such as player.guild.id."}}, Returns: "wa.Guild?", Doc: "A guild by ID, or nil.", Permission: "game.read"},
			{Name: "find", Params: []Param{{"name", "string", "A guild's name, without regard to case."}}, Returns: "wa.Guild?", Doc: "A guild by name, or nil.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.rules", Local: "rules",
		Doc: "The world's ruleset (strategy section 3.6, review decisions 27 and 32). Read-only. Its calls need no permission. An unknown name raises an error that lists the valid names.",
		Funcs: []Func{
			{Name: "is", Params: []Param{{"flag", "string", "A ruleset flag, such as \"cast_time\", or \"skill_rebalance.<skill>\" for a rebalanced skill or a song, such as \"skill_rebalance.MG_FIREBALL\"."}}, Returns: "boolean",
				Doc: "Whether a ruleset flag is on: true at its renewal value or at on. A skill_rebalance flag is the flag the skill follows: its own, or the base."},
			{Name: "get", Params: []Param{{"name", "string", "A ruleset rule, such as \"basic_skill_check\" or \"party_share_level\"."}}, Returns: "integer|boolean",
				Doc: "The value of a ruleset rule: a boolean for a rule that is yes or no, else an integer."},
			{Name: "get_limit", Params: []Param{{"name", "string", "A limit, such as \"inventory_slots\"."}}, Returns: "integer, integer",
				Doc: "A limit's base, what a player or guild starts with, and its max, where expansions stop. rules.limits holds the base too."},
		},
		Fields: []Field{
			{"limits", "wa.Limits", "The limits that rAthena compiles in, as the ruleset sets them, such as rules.limits.party_members. Each is the limit's base."},
		},
	},
	{
		Name: "wa.places", Local: "places",
		Doc: "Named places of generated maps (review decision 42): cells with tags, such as blacksmith, square, gate, entrance, exit or boss_room, that the map generator records. Content finds places by tag, not by coordinates. A classic world has no places: find returns an empty list and nearest nil.",
		Funcs: []Func{
			{Name: "find", Params: []Param{
				{"scope", "string", "Where to look: \"town:any\", \"town:<id>\" such as \"town:t7\", \"dungeon:any\", \"dungeon:<id>\" such as \"dungeon:crypt\", or \"map:<id>\" such as \"map:gen:field:3:-1\". Another form raises an error."},
				{"tag", "string", "A tag, such as \"blacksmith\" or \"shop:tool\": 1 to 64 lowercase letters, digits, _, -, . and :."}},
				Returns: "wa.Place[]", Doc: "Every place of the scope with the tag, in a fixed order: by map ID, then in the order the generator made them. Empty when none matches.", Permission: "game.read"},
			{Name: "nearest", Params: []Param{playerParam, {"tag", "string", "A tag, as for find."}}, Returns: "wa.Place?",
				Doc: "The place with the tag closest to the player: the fewest warps away on the map graph, then the shortest distance. On another map, the distance counts from where the player would arrive. Maps no warp leads to are left out. nil when no place qualifies.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.towns", Local: "towns",
		Doc: "The towns of the world layout (review decision 43): each with an ID such as t7, a display name, a map and a level band. A classic world has no towns: list returns an empty list, and nearest and get nil.",
		Funcs: []Func{
			{Name: "list", Params: []Param{{"options", "wa.TownListOptions?", ""}}, Returns: "wa.Town[]",
				Doc: "Every town by number, or with visited_by only the towns that player entered.", Permission: "game.read"},
			{Name: "nearest", Params: []Param{playerParam}, Returns: "wa.Town?",
				Doc: "The town the fewest warps from the player's map, ties to the lower number: the player's own town when it stands in one. nil when no town is reached.", Permission: "game.read"},
			{Name: "get", Params: []Param{{"id", "string", "A town's ID, such as \"t7\". Another form raises an error."}}, Returns: "wa.Town?",
				Doc: "A town by its ID, or nil when the world has none with it.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.generation", Local: "generation",
		Doc: "Generation layers (review decision 46): mod code that runs when wactl world new makes a procedural world, on the server only, and in the test worlds of wactl mod test. A layer adds named places, objects and bosses to each generated map. It cannot change terrain cells, which the client generates. Both calls need world.generate. In a running zone they record what they get, and nothing runs it.",
		Funcs: []Func{
			{Name: "on", Params: []Param{{"event", `"map"`, "The event: \"map\", once on each generated field, town and dungeon floor, in the layout's order."},
				{"layer", "fun(map: wa.GenerationMap, node: wa.GenerationNode): wa.GenerationResult?", "Gets the map and the layer's node, and returns nil or what to add."}},
				Doc: "Adds a generation layer in a mod context. The mods' layers run in load order. A layer has no other source of randomness than its node: wa.random raises an error while it runs. A layer that fails is logged and left out.", Permission: "world.generate"},
			{Name: "quest_template", Params: []Param{{"record", "table", "A record of data/quest_templates.yml with its name, such as { name = \"shrine_hunt\", kind = \"hunt\", title = \"Hunt {count} {monster}\", count = { 5, 10 } }. The name gets the mod's prefix."}},
				Doc: "Adds a quest template, which the world's quest boards and job guilds draw from as from the records. Call it in an entry script or a layer. A bad record or a second template of the name raises an error.", Permission: "world.generate"},
		},
	},
	{
		Name: "wa.variables", Local: "variables",
		Doc:   "Saved variables, declared at the top of a file with a scope and a type. They are stored under the mod's ID. A mod declares at most 256 variables, and declaring one again gives the same handle. A mod's values take at most 1 MiB as JSON on one player, and 1 MiB in its server variables.",
		Funcs: variableFuncs(),
	},
	{
		Name: "wa.random", Local: "random",
		Doc: "Random numbers from the zone's secret stream.",
		Funcs: []Func{
			{Name: "integer", Params: []Param{{"minimum", "integer", ""}, {"maximum", "integer", ""}}, Returns: "integer", Doc: "A whole number from minimum to maximum."},
			{Name: "chance", Params: []Param{{"numerator", "integer", ""}, {"denominator", "integer", "Above 0."}}, Returns: "boolean", Doc: "True with a chance of numerator in denominator."},
		},
	},
	{
		Name: "wa.integers", Local: "integers",
		Doc: "Integer division that rounds as C does, toward zero, for scripts converted from rAthena.",
		Funcs: []Func{
			{Name: "divide_truncated", Params: []Param{{"a", "integer", ""}, {"b", "integer", "Not 0."}}, Returns: "integer", Doc: "a / b, rounded toward zero."},
			{Name: "remainder_truncated", Params: []Param{{"a", "integer", ""}, {"b", "integer", "Not 0."}}, Returns: "integer", Doc: "The remainder of divide_truncated."},
		},
	},
	{
		Name: "wa.bonuses", Local: "bonuses",
		Doc: "Item bonuses (proposal 0002a). Equip contexts only.",
		Funcs: []Func{{
			Name: "add",
			Params: []Param{
				{"name", "wa.BonusName", "The bonus name. It is checked when the call runs."},
				{"...", "integer|(fun(): integer)|wa.BonusOptions", "What the bonus applies to, the amount, other numbers, then the options table. A number can be a function: a computed value."},
			},
			Returns: "wa.BonusHandle",
			Doc:     "Adds a bonus to the equipped copy. It ends when the copy is unequipped.",
		}},
	},
	{Name: "wa.races", Local: "races", Doc: "Monster races, from rAthena's RC_ names.", Constants: true},
	{Name: "wa.monster_groups", Local: "monster_groups", Doc: "Monster groups, from rAthena's RC2_ names.", Constants: true},
	{Name: "wa.elements", Local: "elements", Doc: "Elements, from rAthena's ELE_ names.", Constants: true},
	{Name: "wa.sizes", Local: "sizes", Doc: "Sizes, from rAthena's Size_ names.", Constants: true},
	{Name: "wa.monster_classes", Local: "monster_classes", Doc: "Monster classes, from rAthena's CLASS_ names.", Constants: true},
	{Name: "wa.weapon_types", Local: "weapon_types", Doc: "Weapon types, from rAthena's W_ names.", Constants: true},
	{
		Name: "wa.statuses", Local: "statuses", Doc: "Status effects, from rAthena's SC_ names, and the statuses mods define.", Constants: true,
		Funcs: []Func{
			{Name: "define", Params: []Param{{"definition", "wa.StatusDefinition", ""}},
				Doc: "Adds a status in a mod script. It gets a constant in wa.statuses named after it, such as statuses.glow. Its ID stays the same in every zone, and a saved status keeps its name. A name that a status of the game or of another mod has raises an error. A mod defines at most 256 statuses."},
		},
	},
	{Name: "wa.effects", Local: "effects", Doc: "Effects for entity:show_effect, from rAthena's EF_ names.", Constants: true},
	{Name: "wa.headgear_effects", Local: "headgear_effects", Doc: "Effects for player:show_headgear_effect, from rAthena's HAT_EF_ names.", Constants: true},
	{Name: "wa.emotions", Local: "emotions", Doc: "Emotions for npc:show_emotion, from rAthena's ET_ names.", Constants: true},
	{
		Name: "wa.jobs", Local: "jobs", Doc: "Jobs, from rAthena's JOB_ names, and job tests. The job tests work with mod jobs too.", Constants: true,
		Funcs: []Func{
			{Name: "id", Params: []Param{{"name", "string", "A name of wa.jobs, such as knight, or a mod job's full name, such as example.jobs:Desperado."}}, Returns: "integer", Doc: "The job's ID. An unknown name raises an error.", Permission: "game.read"},
			{Name: "is_transcendent", Params: []Param{{"job", "integer", "A job, such as player.job."}}, Returns: "boolean", Doc: "Whether the job is a transcendent job."},
			{Name: "is_baby", Params: []Param{{"job", "integer", ""}}, Returns: "boolean", Doc: "Whether the job is a baby job."},
			{Name: "is_third", Params: []Param{{"job", "integer", ""}}, Returns: "boolean", Doc: "Whether the job is a third job."},
			{Name: "is_fourth", Params: []Param{{"job", "integer", ""}}, Returns: "boolean", Doc: "Whether the job is a fourth job."},
			{Name: "base_job", Params: []Param{{"job", "integer", ""}}, Returns: "integer", Doc: "rAthena's BaseJob: a Lord Knight is a Knight. A job record's base_job."},
			{Name: "first_job", Params: []Param{{"job", "integer", ""}}, Returns: "integer", Doc: "rAthena's BaseClass: a Knight is a Swordman."},
			{Name: "next_jobs", Params: []Param{{"job", "integer", ""}}, Returns: "integer[]",
				Doc: "The jobs a job can change into, in job ID order: the jobs whose record lists it in changes_from. In core's records a Novice changes into a first class or a Super Novice, a High Novice into a transcendent first class, and a first class into its two second classes. Jobs that differ by sex, such as Bard and Dancer, are both listed. Other core jobs have none until third classes exist."},
		},
	},
	{
		Name: "wa.testing", Local: "testing", Testing: true,
		Doc: "Test cases. It exists only under wactl mod test, never in a running server.",
		Funcs: []Func{
			{Name: "case", Params: []Param{{"name", "string", "Unique in the file."}, {"run", "fun(world: wa.TestWorld)", "Runs in a fresh world: the mod and its dependencies loaded, a fixed random seed, and no database."}},
				Doc: "Registers a case. Call it inside the function that the test file returns."},
			{Name: "expect", Params: []Param{{"value", "any", ""}}, Returns: "wa.Expectation", Doc: "Checks a value. A failed check fails the case at the line that called it."},
		},
	},
}

// variableFuncs are the declarations of wa.variables: one per scope and
// type.
func variableFuncs() []Func {
	var out []Func
	for _, scope := range []struct{ name, doc string }{
		{"character", "Saved with the character."},
		{"character_temporary", "Kept until the character logs out."},
		{"account", "Shared by the account's characters."},
		{"server", "One value for the whole world, saved soon after a change."},
		{"instance", "Raises an error until instances exist."},
	} {
		for _, kind := range []string{"integer", "boolean", "string", "table"} {
			class := "wa." + title(kind) + "Variable"
			if scope.name == "server" {
				class = "wa.Server" + title(kind) + "Variable"
			}
			out = append(out, Func{
				Name: scope.name + "_" + kind, Params: []Param{{"name", "string", "1 to 64 letters, digits and _, not starting with a digit."}},
				Returns: class, Doc: "Declares a " + kind + " variable. " + scope.doc, Permission: "storage.mod",
			})
		}
	}
	return out
}

func title(s string) string { return string(s[0]-'a'+'A') + s[1:] }

// stats are the player fields of the allocated stats.
var stats = []string{"str", "agi", "vit", "int", "dex", "luk", "pow", "sta", "wis", "spl", "con", "crt"}

func playerFields() []Field {
	f := []Field{
		{"kind", `"player"`, "The entity's kind."},
		{"name", "string", "The character's name."},
		{"job", "integer", "The job, such as jobs.knight."},
		{"job_class", "integer", "rAthena's eaclass(): the job's class flags."},
		{"base_level", "integer", ""},
		{"job_level", "integer", ""},
		{"zeny", "integer", ""},
		{"hp", "integer", ""},
		{"max_hp", "integer", ""},
		{"sp", "integer", ""},
		{"max_sp", "integer", ""},
		{"map", "string", "The map's name, such as prontera."},
		{"x", "integer", ""},
		{"y", "integer", ""},
		{"character_id", "integer", "The database ID, 0 without a database."},
		{"account_id", "integer", "The database ID, 0 without a database."},
		{"party", "wa.Party?", "The player's party, or nil."},
		{"guild", "wa.Guild?", "The player's guild, or nil."},
		{"sex", `"male"|"female"`, ""},
		{"companion", "integer?", "The player's companion, from companions.id, or nil."},
		{"mount", "integer?", "The player's mount, from mounts.id, or nil."},
		{"cart", "integer?", "The look of the player's cart, from 1, or nil."},
	}
	for _, s := range stats {
		doc := "Allocated points."
		if slices.Index(stats, s) >= 6 {
			doc = "Allocated points. 0 until trait stats exist."
		}
		f = append(f, Field{"base_" + s, "integer", doc})
	}
	return f
}

// Classes lists the handle types and tables of the API.
var Classes = []Class{
	{
		Name: "wa.Player", Handle: "player",
		Doc:    "A player. Each access looks the player up again. A handle to a player who left the map raises an error. Reading needs game.read.",
		Fields: playerFields(),
		Methods: append([]Func{
			{Name: "send_message", Params: []Param{{"text", "string", "1 to 255 bytes."}}, Doc: "Shows a line in the player's chat log, as CHAT_CHANNEL_SYSTEM."},
			{Name: "give_item", Params: []Param{itemParam, {"amount", "integer?", "1 to 30000. 1 by default."}, {"options", "wa.GiveItemOptions?", ""}}, Returns: "boolean", Doc: "Gives items, as getitem. Returns false when they do not fit.", Permission: "player.give_items"},
			{Name: "give_item_from_group", Params: []Param{{"group", "integer", "A group from items.group."}}, Doc: "Gives one use of an item group, as getgroupitem.", Permission: "player.give_items"},
			{Name: "count_item", Params: []Param{itemParam, {"options", "wa.FromOptions?", ""}}, Returns: "integer?", Doc: "How many of the item the player has, as countitem. With from, how many a storage or the cart holds, as storagecountitem and cartcountitem; nil when it cannot be read: no cart, or a storage window open.", Permission: "game.read"},
			{Name: "take_item", Params: []Param{itemParam, {"amount", "integer", "1 or more."}, {"options", "wa.FromOptions?", ""}}, Returns: "boolean", Doc: "Takes items, as delitem, or with from out of a storage or the cart, as storagedelitem and cartdelitem. Returns false, and takes nothing, when there are too few."},
			{Name: "open_storage", Params: []Param{{"name", "string?", "An extra storage of data/storages.yml. A name without a mod prefix is the calling mod's record, then core's. A mod reaches only its own records and core's: another mod's name raises an error. Nothing opens the account's storage."}}, Returns: "boolean", Doc: "Opens the account's storage or an extra storage, as openstorage and openstorage2. Returns false when the player has a storage open, or another character of the account has this one open. Scripts charge any fee."},
			{Name: "open_guild_storage", Returns: "boolean, string?", Doc: "Opens the guild storage, as guildopenstorage. Returns true, or false and why: \"no_guild\", \"no_storage\", \"no_permission\", \"in_use\" or \"already_open\"."},
			{Name: "get_currency", Params: []Param{currencyParam}, Returns: "integer", Doc: "How much of a currency of data/currencies.yml the player has, such as cash for rAthena's #CASHPOINTS. An account's currency is its account's.", Permission: "game.read"},
			{Name: "give_currency", Params: []Param{currencyParam, {"amount", "integer", "1 to 2147483647."}},
				Doc: "Gives a currency. An amount over the currency's max is lost, as pc_getcash. A mod changes its own currencies freely. Another mod's, and core's cash and kafra, need currencies.modify."},
			{Name: "take_currency", Params: []Param{currencyParam, {"amount", "integer", "1 to 2147483647."}}, Returns: "boolean",
				Doc: "Takes a currency, as pc_paycash. Returns false, and takes nothing, when the player has too little. A mod changes its own currencies freely. Another mod's, and core's, need currencies.modify."},
			{Name: "send_mail", Params: []Param{{"mail", "wa.MailSpec", ""}},
				Doc: "Sends a mail to a character, online or not, as rAthena's mail command. The items are made for the mail, and the zeny comes from nowhere: the player pays nothing. A mail to a character who is not online arrives after the call returns. Script mail is not returned to its sender when it expires; it is deleted after mail_delete_days, as int_mail.cpp does with mail of send_id 0.", Permission: "mail.send"},
			{Name: "open_mail", Returns: "boolean", Doc: "Opens the player's mailbox window, as openmail. Returns false when the player is trading, vending, running a buying store or has a storage open, as mail_openmail."},
			{Name: "autotrade", Returns: "boolean",
				Doc: "Closes the player's connection and keeps its vending shop or buying store open, as @autotrade. The store opens again after a restart, as autotrade_restore. The player leaves when its store closes or sells out, or when it logs in again. Returns false when the player has no store open."},
			{Name: "search_stores", Params: []Param{{"search", "wa.StoreSearchSpec", ""}}, Returns: "boolean",
				Doc: "Opens the store search window, as searchstores, over the vending shops and buying stores of every map. Results can filter by price, cards and the ranges of rolls and random options. Returns false when the window is open already."},
			{Name: "open_buying_store", Params: []Param{{"store", "wa.BuyingStoreSpec", ""}}, Returns: "boolean",
				Doc: "Opens the buying store setup window, as buyingstore. The player then picks what to buy. Returns false when the player is vending, has a buying store open, or is trading, as buyingstore_setup."},
			{Name: "can_carry", Params: []Param{itemParam, {"amount", "integer", ""}}, Returns: "boolean", Doc: "Whether the items fit, by weight and slots, as checkweight.", Permission: "game.read"},
			{Name: "pay_zeny", Params: []Param{{"amount", "integer", ""}}, Returns: "boolean", Doc: "Takes zeny. Returns false, and takes nothing, when the player has too little."},
			{Name: "give_zeny", Params: []Param{{"amount", "integer", ""}}, Doc: "Gives zeny. Zeny over 2,147,483,647 is lost.", Permission: "player.give_zeny"},
			{Name: "item_heal", Params: []Param{{"heal", "wa.HealSpec", ""}}, Doc: "Heals as itemheal, with its bonuses from VIT, INT and skills."},
			{Name: "heal_percent", Params: []Param{{"heal", "wa.HealSpec", "Amounts in percent, from -100 to 100."}}, Doc: "Heals as percentheal. A negative amount takes that percent of the maximum HP or SP, and never takes HP below 1 (review decision 56)."},
			{Name: "heal_fully", Doc: "Heals all HP and SP."},
			{Name: "warp", Params: []Param{{"map", "string", ""}, {"x", "integer", ""}, {"y", "integer", ""}}, Returns: "boolean", Doc: "Moves the player after the event. Returns false when the map does not run.", Permission: "player.warp"},
			{Name: "warp_to_random_cell", Returns: "boolean", Doc: "A Fly Wing: a random walkable cell of the map.", Permission: "player.warp"},
			{Name: "warp_to_save_point", Returns: "boolean", Doc: "A Butterfly Wing: the save point.", Permission: "player.warp"},
			{Name: "set_save_point", Params: []Param{{"map", "string", ""}, {"x", "integer", ""}, {"y", "integer", ""}}, Returns: "boolean", Doc: "Sets the save point, as savepoint. Returns false when the map does not run or the cell is not walkable.", Permission: "player.warp"},
			{Name: "show_effect", Params: []Param{{"effect", "integer", "An effect from wa.effects."}}, Doc: "Plays an effect on the player for everyone near, as specialeffect2."},
			findNearby, existsMethod,
			{Name: "show_headgear_effect", Params: []Param{{"effect", "integer", "An effect from wa.headgear_effects."}}, Doc: "rAthena's hateffect. It shows nothing yet."},
			{Name: "get_skill_level", Params: []Param{{"skill", "integer", "A skill from skills.id."}}, Returns: "integer", Doc: "The level of a skill the player has, or 0, as getskilllv.", Permission: "game.read"},
			{Name: "get_equipped", Params: []Param{{"slot", "wa.EquipSlot", ""}}, Returns: "wa.EquippedItem?", Doc: "The copy in a slot, or nil.", Permission: "game.read"},
			{Name: "add_timed_bonuses", Params: []Param{{"key", "string", "1 to 64 bytes. The option on_repeat says what a second call with the same key does."},
				{"milliseconds", "integer", "1 or more."}, {"bonuses", "table", "A list of entries: a bonus name, its arguments, and its options, as in bonuses.add, such as { { \"str\", 5 } }."},
				{"options", "wa.TimedBonusOptions?", ""}}, Returns: "boolean",
				Doc: "Gives the player bonuses for a time, as rAthena's bonus_script. They are saved across logout, unless ends_on has logout. The player's stats change before it returns. Returns true when the bonuses were added or extended, and false when on_repeat ignored them."},
			{Name: "mark_minimap", Params: []Param{{"id", "integer", ""}, {"x", "integer", ""}, {"y", "integer", ""}, {"color", "integer", "0xRRGGBB."}}, Doc: "Marks a cell on the minimap, as viewpoint. A mark with the same ID replaces it."},
			{Name: "unmark_minimap", Params: []Param{{"id", "integer", ""}}, Doc: "Removes a minimap mark."},
			{Name: "open_shop", Params: []Param{{"shop", "string", "A shop record of data/npcs.yml: the mod's own name, or mod:Name."}}, Doc: "Opens a shop, as callshop."},
			{Name: "change_job", Params: []Param{{"job", "integer", "A job from wa.jobs or jobs.id."}, {"options", "wa.ChangeJobOptions?", ""}}, Returns: "boolean",
				Doc: "Changes the player's job, as jobchange with rAthena's pc_jobchange. Job level goes back to 1, and learned skills stay. A transcendent player stays in the line it had before its rebirth. Returns false, and changes nothing, when the job is not a valid next job (jobs.next_jobs) or the player lacks the base or job level that the job's changes_from asks. A male player who changes into a Dancer becomes a Bard, and the reverse. The job_change event follows a change."},
			{Name: "rebirth", Returns: "boolean",
				Doc: "Makes a second class at base level 99 and job level 50 or more a High Novice at base and job level 1, with 100 status points, First Aid and Trick Dead, as rAthena's rebirth. Its second class picks its transcendent jobs. Returns false, and changes nothing, for other players."},
			{Name: "set_companion", Params: []Param{{"companion", "integer?", "A companion from companions.id, or nil to remove it."}}, Returns: "boolean",
				Doc: "Gives the player a companion in place of its current one, as setfalcon. Returns false, and changes nothing, when the player lacks the skills of the companion's requires, such as Falconry Mastery for a falcon."},
			{Name: "set_mount", Params: []Param{{"mount", "integer?", "A mount from mounts.id, or nil to remove it."}}, Returns: "boolean",
				Doc: "Puts the player on a mount in place of its current one, as setriding. The mount's acts_as picks its rules: walk speed, the mounted ASPD rule and spear damage. Returns false, and changes nothing, when the player lacks the skills of the mount's requires, such as Riding for a Peco Peco."},
			{Name: "set_cart", Params: []Param{{"look", "integer?", "A cart look from 1 to 5, or nil to remove the cart."}}, Returns: "boolean",
				Doc: "Gives the player a cart of a look, as setcart. Returns false, and changes nothing, without Pushcart or for a look the client does not have."},
			{Name: "reset_skills", Doc: "Lowers every skill learned with skill points to 0 and refunds the points, as resetskill. Quest skills stay, and so does Basic Skill outside the Novice jobs."},
			{Name: "reset_stats", Doc: "Sets every stat to 1 and gives back the status points of the base level, as resetstatus."},
			{Name: "reset_skill", Params: []Param{{"skill", "integer", "A skill from skills.id."}, {"options", "wa.ResetSkillOptions?", ""}}, Returns: "boolean",
				Doc: "Lowers one skill, to 0 by default, and refunds its skill points. Returns false, and changes nothing, when another learned skill needs the lowered level and with_dependents is not set, when the skill is not above the level, or for a quest skill."},
			{Name: "whisper", Params: []Param{{"text", "string", "1 to 255 bytes."}, {"options", "wa.WhisperOptions?", ""}},
				Doc: "Sends the player a whisper. It does not pass the chat filter or the chat hooks, and the player's block list does not stop it. The player sees the mod's ID with a mod's whisper, as a server line it cannot answer, so it cannot pass for a player's. Core's whispers are plain whispers."},
			{Name: "is_blocking", Params: []Param{{"other", "wa.Player", ""}}, Returns: "boolean", Doc: "Whether the player blocks the other player's name.", Permission: "game.read"},
			{Name: "is_friend", Params: []Param{{"other", "wa.Player", ""}}, Returns: "boolean", Doc: "Whether the other player is on the player's friend list. Only players add and remove friends.", Permission: "game.read"},
			{Name: "get_friend_names", Returns: "string[]", Doc: "The names on the player's friend list, in the order they were added.", Permission: "game.read"},
			{Name: "get_staff_role", Returns: "string?", Permission: "game.read",
				Doc: "The player's staff role, such as \"game_master\", or nil. The owner role is \"owner\". A role counts only when the player logged in with a second factor, a TOTP code, a recovery code or a passkey, and the account has TOTP or a passkey on, as on the admin screens."},
			{Name: "has_staff_permission", Params: []Param{{"permission", "string", "A staff permission, such as \"moderation.mute\". An unknown name raises an error that lists the names."}}, Returns: "boolean", Permission: "game.read",
				Doc: "Whether the player's staff role has a permission. The owner has every permission. It follows the two-factor rule of get_staff_role."},
			{Name: "has_visited", Params: []Param{{"town", "wa.Town", ""}}, Returns: "boolean",
				Doc: "Whether the player's character entered the town, as the teleport service lists it (review decision 43).", Permission: "game.read"},
			{Name: "add_quest", Params: []Param{questParam}, Returns: "boolean",
				Doc: "Adds the quest to the player's quest log as active, with its time limit from now, as setquest. Returns false, and changes nothing, when the log has the quest already, in any state. The quest_add event follows."},
			{Name: "complete_quest", Params: []Param{questParam}, Returns: "boolean",
				Doc: "Marks an active or inactive quest complete, as completequest. It leaves the quest window, and kills stop counting for it. Returns false when the log lacks the quest or holds it as complete. The quest_complete event follows."},
			{Name: "remove_quest", Params: []Param{questParam}, Returns: "boolean",
				Doc: "Takes the quest out of the log, in any state, as erasequest. Returns false when the log lacks it."},
			{Name: "change_quest", Params: []Param{{"from", "integer|string", "A quest the log holds as active or inactive."}, {"to", "integer|string", "A quest the log lacks."}}, Returns: "boolean",
				Doc: "Replaces a quest with another, which starts active with its time limit from now and no kills, as changequest. Returns false, and changes nothing, when the log lacks from, holds it as complete, or has to already. The quest_add event follows for to."},
			{Name: "get_quest_state", Params: []Param{questParam}, Returns: `"none"|"active"|"inactive"|"complete"`,
				Doc: "The quest's state in the player's log. A quest is inactive when the player marked it so in the quest window.", Permission: "game.read"},
			{Name: "get_quest_progress", Params: []Param{questParam, {"objective", "integer", "A hunt objective of the quest, from 1."}}, Returns: "integer?, integer?",
				Doc: "The kills counted for a hunt objective and the kills it asks for, as checkquest with HUNTING reads them. nil when the player lacks the quest or the quest has no such objective. Kills stop counting at the goal.", Permission: "game.read"},
			{Name: "quest_timed_out", Params: []Param{questParam}, Returns: "boolean",
				Doc: "Whether the quest's time limit passed, as checkquest with PLAYTIME. false for a quest without a time limit, and for a quest the player lacks.", Permission: "game.read"},
			{Name: "mute", Params: []Param{{"minutes", "integer", "1 to 5,256,000 (ten years)."}, {"reason", "string", "1 to 255 characters. The player sees it."}},
				Doc: "Mutes the player, as rAthena's SC_NOCHAT: it cannot talk until the end. It replaces a mute that has not ended. The player sees the reason and the end time. The mute is saved.", Permission: "player.sanction"},
			{Name: "jail", Params: []Param{{"minutes", "integer", "1 to 5,256,000 (ten years)."}, {"reason", "string", "1 to 255 characters. The player sees it."}},
				Doc: "Jails the player, as rAthena's pc_jail: it goes to the jail map and cannot warp until the end, then goes back to where it was. It replaces a jail that has not ended. The player sees the reason and the end time. The jail is saved.", Permission: "player.sanction"},
			{Name: "unmute", Returns: "boolean", Doc: "Ends the player's mute. Returns false when it was not muted.", Permission: "player.sanction"},
			{Name: "expand", Params: []Param{{"limit", "wa.ExpandableLimit", "The limit to change."}, {"amount", "integer", "-10,000 to 10,000. A negative amount lowers the limit."}}, Returns: "integer",
				Doc: "Raises one of the player's limits by amount, up to the limit's max, and returns the new value, as rAthena's inventory expansion. A negative amount lowers it, not below the base, and deletes nothing. inventory_slots and cart_slots save with the character, storage_slots with the account, whose characters share it. 0 changes nothing and returns the current value.", Permission: "player.expand_limits"},
			{Name: "reset_stat", Params: []Param{{"stat", `"str"|"agi"|"vit"|"int"|"dex"|"luk"`, ""}, {"options", "wa.ResetStatOptions?", ""}}, Returns: "boolean",
				Doc: "Lowers one stat, to 1 by default, and refunds what raising it cost, by rAthena's cost table. Returns false when the stat is not above the value."},
		}, statusMethods...),
	},
	{
		Name: "wa.Monster", Handle: "monster",
		Doc: "A monster. A killed monster's handle reads while its death event runs, with is_alive false, and raises an error after it. Two handles to one monster are equal with ==. A monster that comes back after it died is a new monster with a new handle. Reading needs game.read. The methods control its AI, as rAthena's unit script commands. They raise an error once the monster is gone.",
		Fields: []Field{
			{"kind", `"monster"`, "The entity's kind."},
			{"id", "integer", "The monster's ID, such as 1002."},
			{"name", "string", ""},
			{"level", "integer", ""},
			{"hp", "integer", ""},
			{"max_hp", "integer", ""},
			{"map", "string", ""},
			{"x", "integer", ""},
			{"y", "integer", ""},
			{"is_alive", "boolean", ""},
			{"target", "wa.Player|wa.Monster|wa.Npc|nil", "The unit its AI follows (target_id), or nil."},
			{"state", "wa.MonsterState", "Its skill state, in rAthena's words."},
			{"master", "wa.Player|wa.Monster|nil", "The unit that summoned it as a slave, or nil."},
		},
		Methods: append([]Func{showEffect("monster"), findNearby, existsMethod,
			{Name: "set_target", Params: []Param{{"target", "wa.Player|wa.Monster|wa.Npc", ""}}, Returns: "boolean",
				Doc: "The monster takes the target and chases it to attack, as unitattack. Returns false when it cannot target it."},
			{Name: "clear_target", Doc: "The monster drops its target and goes idle, as mob_unlocktarget."},
			{Name: "walk_to", Params: []Param{{"x", "integer", ""}, {"y", "integer", ""}}, Returns: "boolean",
				Doc: "The monster walks to a cell, as unitwalk. Its AI waits until it gets there. Returns false when it cannot walk there."},
			{Name: "use_skill", Params: []Param{{"skill", "integer", "From skills.id."}, {"level", "integer", "1 to 100."}, {"target", "wa.Player|wa.Monster|wa.Npc", "The target, or its cell for a ground skill. The monster itself for a skill on itself."}}, Returns: "boolean",
				Doc: "The monster uses a skill with the skill's own cast time, as unitskilluseid. No delay of its skills list applies. Returns false when the cast does not start."},
			{Name: "talk", Params: []Param{{"text", "string", "1 to 255 bytes."}}, Doc: "The monster says a line to the players near it, after its name, as unittalk."},
			{Name: "stop", Doc: "The monster stops walking and attacking, and drops its target, as unitstop."},
		}, statusMethods...),
	},
	{
		Name: "wa.Npc", Handle: "npc",
		Doc: "An NPC: the instance of an NPC context.",
		Fields: []Field{
			{"kind", `"npc"`, "The entity's kind."},
			{"name", "string", "The name players see."},
			{"map", "string", ""},
			{"x", "integer", ""},
			{"y", "integer", ""},
		},
		Methods: append([]Func{
			{Name: "show_emotion", Params: []Param{{"emotion", "integer", "An emotion from wa.emotions."}}, Doc: "Shows an emotion over the NPC, as emotion."},
			{Name: "say_aloud", Params: []Param{{"text", "string", ""}}, Doc: "Says a line of local chat as the NPC, as npctalk. Players within 9 cells hear it."},
			{Name: "show_quest_icon", Params: []Param{playerParam, {"icon", "wa.QuestIcon?", "nil removes the icon."}},
				Doc: "Shows an icon over the NPC to one player, as showevent. It stays while the player is on the map."},
			showEffect("NPC"), findNearby, existsMethod,
		}, statusMethods...),
	},
	{
		Name: "wa.EquippedItem", Handle: "equipped item",
		Doc: "An equipped copy: the instance of an equip context, or what player:get_equipped returns.",
		Fields: []Field{
			{"item", "integer", "The item's ID."},
			{"refine", "integer", ""},
			{"enchant_grade", "integer", "0 for none, 1 to 4 for D to A."},
			{"wearer", "wa.Player", ""},
			{"weapon_type", "integer", ""},
			{"weapon_level", "integer", ""},
			{"armor_level", "integer", ""},
			{"rolls", "table<string, integer>", "The copy's rolled values by name, such as rolls.boss_damage. In a card's script, the card's own rolls. Read-only."},
			{"cards", "wa.CardRolls[]", "The cards in the copy's slots, in slot order, with their rolls."},
			{"random_options", "wa.RandomOption[]", "The copy's random options, in option order."},
		},
		Methods: []Func{
			{Name: "set_roll", Params: []Param{{"name", "string", "A roll of the item, or of the card for a card's copy."}, {"value", "integer", "Clamped to the roll's range."}},
				Doc: "Sets one rolled value. The copy's equip scripts start again after the call, so they read the new value.", Permission: "items.modify_rolls"},
			{Name: "reroll", Params: []Param{{"name", "string?", "One roll. Every roll without it."}},
				Doc: "Rolls values again, as a random give: from the roll's distribution when the item_rolls flag is on, the default otherwise, through the item_roll hooks with source kind \"script\".", Permission: "items.modify_rolls"},
		},
	},
	{
		Name: "wa.CardRolls", Doc: "A card in an equipped copy's slots.",
		Fields: []Field{{"item", "integer", "The card's item ID."}, {"rolls", "table<string, integer>", "The card's rolled values by name."}},
	},
	{
		Name: "wa.RandomOption", Doc: "A random option of a copy, as rAthena's random options.",
		Fields: []Field{{"option", "string", "The option's name in data/random_options.yml, such as max_hp."}, {"value", "integer", ""}, {"param", "integer", ""}},
	},
	{
		Name: "wa.ItemRoll", Doc: "The object of the item_roll hook: one roll of a new copy. The innermost result is the server's roll: from the roll's distribution when the item_rolls flag is on and the copy is rolled, the default otherwise.",
		Fields: []Field{
			{"item", "integer", "The item's ID."},
			{"name", "string", "The roll's name."},
			{"min", "integer", ""}, {"max", "integer", ""}, {"default", "integer", ""},
			{"source", "wa.ItemRollSource", "Where the copy comes from."},
		},
	},
	{
		Name: "wa.ItemRollSource", Doc: "Where a rolled copy comes from: a monster's drop, a script, or an NPC shop.",
		Fields: []Field{
			{"kind", `"drop"|"script"|"shop"`, ""},
			{"monster", "wa.Monster?", "The killed monster, for a drop."},
		},
	},
	{
		Name: "wa.Conversation", Handle: "conversation",
		Doc: "A player talking to an NPC. Methods that wait pause the handler until the player answers. When the player leaves, the waiting call raises \"the conversation was closed\".",
		Methods: []Func{
			{Name: "say", Params: []Param{{"...", "string|integer", "Up to 64 lines, each at most 1,024 bytes."}}, Doc: "Shows lines."},
			{Name: "wait_for_next", Doc: "Waits for the Next button. The next lines replace the shown ones."},
			{Name: "choose", Params: []Param{{"...", "string", "The options. An empty option is hidden and keeps its number."}}, Returns: "integer", Doc: "Shows options and returns the chosen one, from 1."},
			{Name: "ask_number", Params: []Param{{"minimum", "integer", ""}, {"maximum", "integer", ""}}, Returns: "integer", Doc: "Waits for a number in the range."},
			{Name: "ask_text", Params: []Param{{"maximum_length", "integer", "Up to 255."}}, Returns: "string", Doc: "Waits for text."},
			{Name: "close", Doc: "Shows the Close button and waits for it, as close2. The handler goes on after it, and later dialog calls raise an error."},
			{Name: "wait", Params: []Param{{"milliseconds", "integer", "Up to 10 minutes."}}, Doc: "Waits, as sleep2."},
			{Name: "show_image", Params: []Param{{"name", "string", "An image of image/illust."}, {"position", `"left"|"center"|"right"`, ""}}, Doc: "Shows an NPC image, as cutin."},
			{Name: "hide_image", Doc: "Hides the NPC image."},
			{Name: "show_progress", Params: []Param{{"seconds", "integer", "Up to 600."}}, Doc: "Shows a progress bar and waits until it is full. A walk cancels it and ends the conversation."},
		},
	},
	{
		Name: "wa.EventRun", Handle: "event run", Doc: "A run of a typed world event, from context:get_event() (review decision 31). Its monsters and participants are those on the context's map.",
		Methods: []Func{
			{Name: "get_monsters", Returns: "wa.Monster[]", Doc: "The run's monsters that live on the context's map: the boss, the invasion's monsters, or the treasure chests, in the order they spawned.", Permission: "game.read"},
			{Name: "get_participants", Returns: "wa.Player[]", Doc: "The players on the context's map who did at least the rewards' min_share of the damage to the run's monsters there, 1% by default, sorted by name.", Permission: "game.read"},
			{Name: "finish", Params: []Param{{"result", `"won"|"lost"`, ""}},
				Doc: "Ends the run before its time, within a second. An invasion that finishes \"won\" gives its rewards, as when its last wave dies. world_event_end carries the result."},
		},
	},
	{
		Name: "wa.Place", Handle: "place", Doc: "A named place of a generated map, from wa.places (review decision 42). A place never changes while the server runs.",
		Methods: []Func{
			{Name: "get_map", Returns: "string", Doc: "The map's ID, such as \"gen:town:t7\"."},
			{Name: "get_position", Returns: "integer, integer", Doc: "The place's cell, x and y."},
			{Name: "get_tags", Returns: "string[]", Doc: "The place's tags, its main tag first, such as { \"gate\", \"gate:south\" }."},
			{Name: "get_area", Returns: "string", Doc: "The place's area: its map ID without gen:, such as \"town:t7\", \"dungeon:crypt:2\" or \"field:3:-1\"."},
			{Name: "get_band", Returns: "integer, integer", Doc: "The level band of the place's map, its lowest and highest level (review decision 43). A place of a map without a band raises an error.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.GenerationNode", Handle: "generation_node", Doc: "A generation layer's node of the seed tree's secret branch, its own for each mod and map (strategy section 8.2). It works only while the layer runs, and gives the same values for the same world each time.",
		Methods: []Func{
			{Name: "child", Params: []Param{{"label", "string", "1 to 64 bytes, such as \"shrines\"."}}, Returns: "wa.GenerationNode", Doc: "A child node. Each label gives its own values, and the order of calls does not matter."},
			{Name: "stream", Returns: "wa.GenerationStream", Doc: "A stream of numbers from the node. Two streams of one node give the same numbers."},
			{Name: "hash", Params: []Param{{"x", "integer", ""}, {"y", "integer", ""}}, Returns: "integer", Doc: "A number for a cell, the same for the same node and cell, from all 64 bits. It is the first 8 bytes of HMAC-SHA256 of the cell, keyed by the node, so players cannot recover the node from it (review decision 54)."},
		},
	},
	{
		Name: "wa.GenerationStream", Handle: "generation_stream", Doc: "A stream of numbers of a generation node, from the node's secret ChaCha8 stream (strategy section 8.2).",
		Methods: []Func{
			{Name: "integer", Params: []Param{{"minimum", "integer", ""}, {"maximum", "integer", ""}}, Returns: "integer", Doc: "The next whole number from minimum to maximum."},
			{Name: "chance", Params: []Param{{"numerator", "integer", ""}, {"denominator", "integer", "Above 0."}}, Returns: "boolean", Doc: "True with a chance of numerator in denominator."},
		},
	},
	{
		Name: "wa.Town", Handle: "town", Doc: "A town of the world layout, from wa.towns (review decision 43). Two handles of one town are equal.",
		Methods: []Func{
			{Name: "get_id", Returns: "string", Doc: "The town's ID, such as \"t7\", which places.find's scope \"town:t7\" names.", Permission: "game.read"},
			{Name: "get_name", Returns: "string", Doc: "The town's display name, such as \"Valheim\". It changes when staff rename the town.", Permission: "game.read"},
			{Name: "get_map", Returns: "string", Doc: "The town's map ID, such as \"gen:town:t7\".", Permission: "game.read"},
			{Name: "get_band", Returns: "integer, integer", Doc: "The town's level band: its lowest and highest level. Bands rise with the warps from the home area.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.TownListOptions", Doc: "The options of towns.list.",
		Fields: []Field{{"visited_by", "wa.Player?", "Only the towns this player entered."}},
	},
	{
		Name: "wa.Shop", Handle: "shop", Doc: "A shop of data/npcs.yml, from world.get_shop (review decision 30). Its changes hold for the whole world until the server stops: the shop's file sets it again at start. A stock the methods set is saved.",
		Methods: []Func{
			{Name: "add_item", Params: []Param{itemParam, {"options", "wa.ShopItemOptions?", ""}}, Returns: "boolean",
				Doc: "Adds an item, or replaces the item of that ID, as npcshopadditem. Without a cost the price is the item's buy price. Returns false when a cost names an item or a currency that does not exist.", Permission: "shops.modify"},
			{Name: "remove_item", Params: []Param{itemParam}, Returns: "boolean", Doc: "Takes an item out of the shop, as npcshopdelitem. Returns false when the shop does not sell it.", Permission: "shops.modify"},
			{Name: "set_stock", Params: []Param{itemParam, {"stock", "integer", "0 to 2147483647, or -1 for no limit."}}, Returns: "boolean",
				Doc: "Sets an item's stock, which every player shares, and the amount a restock fills it to, as npcshopupdate for a marketshop. Returns false when the shop does not sell the item.", Permission: "shops.modify"},
		},
	},
	{
		Name: "wa.Channel", Handle: "channel", Doc: "A chat channel, from channels.create or channels.get.",
		Methods: []Func{
			{Name: "send", Params: []Param{{"text", "string", "1 to 255 bytes."}}, Doc: "Shows a line of the channel itself to its members, as rAthena's channelmes. It does not pass the chat filter."},
			{Name: "add", Params: []Param{playerParam}, Returns: "boolean", Doc: "Puts the player in the channel, whatever its join rule. Returns false when it was already in it."},
			{Name: "remove", Params: []Param{playerParam}, Returns: "boolean", Doc: "Takes the player out of the channel, even one it may not leave. Returns false when it was not in it."},
			{Name: "get_members", Returns: "string[]", Doc: "The members' character names, sorted. Members may be on any map.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.Party", Handle: "party", Doc: "A party, from player.party or wa.parties. Its leader and members are players of this map: a member on another map is nil or left out.",
		Fields: []Field{
			{"id", "integer", ""},
			{"name", "string", ""},
			{"leader", "wa.Player?", "The leader, or nil when it is offline or on another map."},
			{"leader_name", "string", "The leader's name, online or not."},
			{"exp_share", `"each"|"even"`, "Even shares the exp of a kill among the members on the map, as rAthena's even share."},
			{"item_share", `"each"|"even"`, "Even gives each picked up item to a member on the map."},
			{"item_pickup", `"each"|"shared"`, "Shared lets members pick up the items their party's top attackers may take."},
		},
		Methods: []Func{
			{Name: "get_members", Returns: "wa.Player[]", Doc: "The online members on this map, in the party's slot order.", Permission: "game.read"},
			{Name: "get_member_names", Returns: "string[]", Doc: "Every member's name, online or not, in slot order.", Permission: "game.read"},
			{Name: "add", Params: []Param{playerParam}, Returns: "boolean", Doc: "Puts the player in the party, as party_addmember. Returns false when the player has a party, the party is full, or a character of the same account is in it. The party_join event follows, with the reason \"script\".", Permission: "party.manage"},
			{Name: "remove", Params: []Param{playerParam}, Returns: "boolean", Doc: "Takes the player out of the party, as party_delmember. The leader's removal breaks the party up, as in rAthena. Returns false when the player is not in the party.", Permission: "party.manage"},
			{Name: "set_leader", Params: []Param{playerParam}, Returns: "boolean", Doc: "Makes an online member the leader, as party_changeleader. Returns false when the player is not an online member, or leads already. The party_change event follows.", Permission: "party.manage"},
			{Name: "set_options", Params: []Param{{"options", "wa.PartyOptions", ""}}, Doc: "Changes the options it names, as party_changeoption. Even exp share needs the online members' base levels within the ruleset's party_share_level, else exp share stays each. The party_change event follows a change.", Permission: "party.manage"},
			{Name: "disband", Doc: "Breaks the party up, as party_destroy. Each member gets the party_leave event, with the reason \"disbanded\".", Permission: "party.manage"},
		},
	},
	{
		Name: "wa.PartyOptions", Doc: "A party's options, in party:set_options and the party_change event.",
		Fields: []Field{
			{"exp_share", `"each"|"even"|nil`, ""},
			{"item_share", `"each"|"even"|nil`, ""},
			{"item_pickup", `"each"|"shared"|nil`, ""},
		},
	},
	{
		Name: "wa.PartyMembership", Doc: "The object of the party_join and party_leave events.",
		Fields: []Field{
			{"party", "wa.Party", "The party. After it broke up, its fields still read."},
			{"player", "wa.Player", "The player who joined or left."},
			{"reason", `"invited"|"create"|"left"|"expelled"|"disbanded"|"script"`, "Why: an accepted invite, a party the player made from the client, a leave, an expel, a break up, or a wa.parties call."},
		},
	},
	{
		Name: "wa.PartyChange", Doc: "The object of the party_change event.",
		Fields: []Field{
			{"party", "wa.Party", ""},
			{"leader", "wa.Player?", "The leader, or nil when it is on another map."},
			{"options", "wa.PartyOptions", "The party's options after the change."},
		},
	},
	{
		Name: "wa.Guild", Handle: "guild", Doc: "A guild, from player.guild or wa.guilds. Its master and members are players of this map: a member on another map is nil or left out.",
		Fields: []Field{
			{"id", "integer", ""},
			{"name", "string", ""},
			{"master", "wa.Player?", "The master, or nil when it is offline or on another map."},
			{"master_name", "string", "The master's name, online or not."},
			{"level", "integer", "1 to 50, from exp_guild.yml."},
			{"exp", "integer", "The exp toward the next level."},
			{"notice", "wa.GuildNotice", ""},
			{"member_count", "integer", "Every member, online or not."},
		},
		Methods: []Func{
			{Name: "get_members", Returns: "wa.Player[]", Doc: "The online members on this map, in the guild's slot order. The master is first.", Permission: "game.read"},
			{Name: "get_member_names", Returns: "string[]", Doc: "Every member's name, online or not, in slot order.", Permission: "game.read"},
			{Name: "get_position", Params: []Param{playerParam}, Returns: "wa.GuildPosition?", Doc: "The member's position, or nil for a player who is not a member.", Permission: "game.read"},
			{Name: "get_skill_level", Params: []Param{{"skill", "integer", "A guild skill's ID, such as skills.id(\"GD_EXTENSION\")."}}, Returns: "integer", Doc: "The guild's level of a guild skill, as getgdskilllv.", Permission: "game.read"},
			{Name: "count_item", Params: []Param{itemParam}, Returns: "integer", Doc: "How many of the item the guild storage holds, as guildstoragecountitem.", Permission: "game.read"},
			{Name: "is_allied", Params: []Param{{"other", "wa.Guild", ""}}, Returns: "boolean", Doc: "Whether the guilds are allies.", Permission: "game.read"},
			{Name: "is_enemy", Params: []Param{{"other", "wa.Guild", ""}}, Returns: "boolean", Doc: "Whether this guild has the other as an enemy. Enemies are on one side only.", Permission: "game.read"},
			{Name: "add", Params: []Param{playerParam}, Returns: "boolean", Doc: "Puts the player in the guild with the last position. Returns false when the player has a guild or the guild is full. The guild_join event follows, with the reason \"script\".", Permission: "guild.manage"},
			{Name: "remove", Params: []Param{playerParam}, Returns: "boolean", Doc: "Takes a member out of the guild. Returns false for a player who is not a member, and for the master. The guild_leave event follows, with the reason \"script\".", Permission: "guild.manage"},
			{Name: "set_master", Params: []Param{playerParam}, Returns: "boolean", Doc: "Makes a member the master, as guildchangegm, without guild_leaderchange_delay. Returns false for a player who is not a member, or is the master. The guild_change event follows.", Permission: "guild.manage"},
			{Name: "set_notice", Params: []Param{{"title", "string", "At most 59 characters."}, {"text", "string", "At most 119 characters."}}, Returns: "boolean", Doc: "Sets the guild notice. Returns false when a part is too long. The guild_change event follows.", Permission: "guild.manage"},
			{Name: "give_exp", Params: []Param{{"amount", "integer", "1 to 2147483647."}}, Doc: "Adds exp to the guild, as guildgetexp, which may level it up. The guild_change event follows a level up.", Permission: "guild.manage"},
			{Name: "disband", Doc: "Breaks the guild up, with its members. Its storage is lost, as in rAthena. Each member gets the guild_leave event, with the reason \"disbanded\".", Permission: "guild.manage"},
		},
	},
	{
		Name: "wa.GuildNotice", Doc: "A guild's notice.",
		Fields: []Field{
			{"title", "string", ""},
			{"text", "string", ""},
		},
	},
	{
		Name: "wa.GuildPosition", Doc: "A member's position, from guild:get_position.",
		Fields: []Field{
			{"name", "string", ""},
			{"can_invite", "boolean", ""},
			{"can_expel", "boolean", ""},
			{"can_use_storage", "boolean", ""},
			{"exp_tax", "integer", "The percent of base exp the member gives the guild."},
		},
	},
	{
		Name: "wa.GuildMembership", Doc: "The object of the guild_join and guild_leave events.",
		Fields: []Field{
			{"guild", "wa.Guild", "The guild. After it broke up, its fields still read."},
			{"player", "wa.Player", "The player who joined or left."},
			{"reason", `"invited"|"create"|"left"|"expelled"|"disbanded"|"script"`, "Why: an accepted invite, a guild the player made from the client, a leave, an expel, a break up, or a wa.guilds call."},
		},
	},
	{
		Name: "wa.GuildChange", Doc: "The object of the guild_change event.",
		Fields: []Field{
			{"guild", "wa.Guild", "The guild, whose fields hold the new values."},
			{"change", `"master"|"level"|"notice"|"emblem"`, "What changed."},
		},
	},
	{
		Name: "wa.Limits", Doc: "The limits of the world's ruleset, as rules.limits: each limit's base, what a player or guild starts with. A list over a limit that a ruleset lowered keeps its entries and cannot grow. The engine's ceiling bounds each limit's max.",
		Fields: []Field{
			{"party_members", "integer", "The most members a party holds, as MAX_PARTY. 12 by default, ceiling 256."},
			{"friends", "integer", "The most names a friend list holds, as MAX_FRIENDS. 40 by default, ceiling 1,000."},
			{"block_list", "integer", "The most names a block list holds, as MAX_IGNORE_LIST. 20 by default, ceiling 1,000."},
			{"guild_members", "integer", "The most members a guild holds before Guild Extension, which adds 6 per level up to the max, 76 by default as MAX_GUILD. 16 by default, ceiling 512."},
			{"guild_positions", "integer", "The positions of a guild, as MAX_GUILDPOSITION. 20 by default, ceiling 40."},
			{"guild_allies", "integer", "The most allied guilds of a guild, as max_guild_alliance. 3 by default, ceiling 10."},
			{"guild_enemies", "integer", "The most enemy guilds of a guild, as max_guild_alliance. 3 by default, ceiling 10."},
			{"guild_storage", "integer", "The slots of a guild storage without renewal's Guild Storage skill, as MAX_GUILD_STORAGE. The skill gives 100 and 100 per level, up to the max. 600 by default, ceiling 10,000."},
			{"inventory_slots", "integer", "The inventory slots of a new character, as INVENTORY_BASE_SIZE. 100 by default, with the max 200, MAX_INVENTORY. Ceiling 1,000."},
			{"storage_slots", "integer", "The slots of an account's storage, as MAX_STORAGE. 600 by default, ceiling 10,000."},
			{"cart_slots", "integer", "The slots of a cart, as MAX_CART. 100 by default, ceiling 1,000."},
			{"mail_items", "integer", "The most item stacks one mail holds, as MAIL_MAX_ITEM. 5 by default, ceiling 20."},
			{"mailbox", "integer", "The most mails a mailbox holds. 100 by default, ceiling 1,000."},
		},
	},
	{
		Name: "wa.ExpShare", Doc: "The share of the experience hook's object.",
		Fields: []Field{
			{"base", "integer", ""},
			{"job", "integer", ""},
		},
	},
	{
		Name: "wa.ChannelSpec", Doc: "The channel of channels.create.",
		Fields: []Field{
			{"name", "string", "1 to 19 letters, digits, _ and -, with or without the #. Players see it as #name. Not public, whisper, party or guild."},
			{"display_name", "string?", "What players see before each line, 1 to 24 characters, as rAthena's alias. [name] by default."},
			{"color", "integer?", "The lines' color, 0xRRGGBB. White by default."},
			{"join", `"open"|"invite"|nil`, `"open", the default: any player may join. "invite": only channel:add puts players in.`},
		},
	},
	{
		Name: "wa.WhisperOptions", Doc: "The options of player:whisper.",
		Fields: []Field{{"from", "string?", "The name the player sees, 1 to 24 characters. The NPC's name in an NPC context, else the mod's name."}},
	},
	{
		Name: "wa.ChatMessage", Doc: "The object of the chat event and the chat hook: a line a player says. The hook runs after the server's chat filter, and the event after the line went out.",
		Fields: []Field{
			{"player", "wa.Player", "The speaker."},
			{"channel", "string", `"public" for local chat, "whisper", "party", "guild", or a channel's name without its #, such as trade.`},
			{"text", "string", "The line, after the chat filter."},
			{"to", "string?", "A whisper's target name. nil for other channels."},
			{"map", "string", "The speaker's map."},
		},
	},
	{
		Name: "wa.ChatOptions", Doc: "The options of a chat handler. Each one narrows the lines that run it.",
		Fields: []Field{
			{"channel", "string?", `"public", "whisper", "party", "guild" or a channel's name, with or without its #.`},
			{"map", "string?", "A map name."},
			{"player", "wa.Player?", "One speaker."},
		},
	},
	{
		Name: "wa.ChatTestOptions", Testing: true, Doc: "The options of world:chat.",
		Fields: []Field{
			{"channel", "string?", `"public", the default, "whisper", or a channel's name.`},
			{"to", "string?", "A whisper's target name."},
		},
	},
	{
		Name: "wa.HeardLine", Testing: true, Doc: "A chat line a test player got, from world:get_chat.",
		Fields: []Field{
			{"channel", "string", `"public", "whisper", "system", or a channel's name with its #.`},
			{"from", "string", "The speaker's name, or empty for a system line or a channel's own line."},
			{"text", "string", ""},
		},
	},
	{
		Name: "wa.EventHandle", Handle: "event handle", Doc: "What events.on returns.",
		Methods: []Func{{Name: "remove", Doc: "Removes the handler. A second call does nothing."}},
	},
	{
		Name: "wa.Timer", Handle: "timer", Doc: "What wa.timers returns.",
		Methods: []Func{{Name: "cancel", Doc: "Cancels the timer. A timer of timers.with_bonuses ends its bonuses."}},
	},
	{
		Name: "wa.BonusHandle", Handle: "bonus", Doc: "What bonuses.add returns.",
		Methods: []Func{{Name: "remove", Doc: "Removes the bonus. A second call does nothing."}},
	},
	{
		Name: "wa.TestWorld", Handle: "test world", Testing: true,
		Doc: "The world of one test case. It runs one map: the map of its first new_player or spawn_monster, or prontera when advance comes first.",
		Methods: []Func{
			{Name: "new_player", Params: []Param{{"spec", "wa.NewPlayerSpec", ""}}, Returns: "wa.Player", Doc: "Adds a character at a cell, with full HP and SP."},
			{Name: "talk", Params: []Param{playerParam, {"npc_name", "string", "The NPC's full name, its name in the mod, or the name players see when one NPC of the map has it."}, {"answers", "(integer|string)[]?", "A number picks a menu option, a string fills text input, and an integer fills number input."}},
				Returns: "string", Doc: "Clicks the NPC and answers each step in order. Next is pressed without an answer. Returns everything the NPC said, one line per line. Fails when the conversation asks for more answers than given, or ends with answers left."},
			{Name: "use_item", Params: []Param{playerParam, itemParam}, Doc: "Uses one of the player's items. It waits for the item use interval first. Fails when the player has no such item, or the use is refused."},
			{Name: "equip", Params: []Param{playerParam, itemParam, {"options", "wa.EquipOptions?", ""}}, Returns: "wa.EquippedItem",
				Doc: "Gives the player one copy of the item and puts it on. Returns the equipped copy, as player:get_equipped. Fails when the copy does not fit or cannot be equipped, such as by the player's job or level."},
			{Name: "advance", Params: []Param{{"milliseconds", "integer", ""}}, Doc: "Moves the zone clock forward. Timers fire and conversations that wait go on."},
			{Name: "spawn_monster", Params: []Param{{"spec", "wa.SpawnMonsterSpec", ""}}, Returns: "wa.Monster", Doc: "Places a monster on a cell. It does not come back after it dies."},
			{Name: "kill", Params: []Param{playerParam, {"monster", "wa.Monster", "A living monster: from spawn_monster, get_monsters or spawning.monsters."}}, Doc: "The player kills the monster at once: the death event fires, and the player gets its exp and drops."},
			{Name: "set_time", Params: []Param{{"time", "string", "Such as \"2026-10-31 19:58\"."}, {"time_zone", "string?", "An IANA name, such as Asia/Manila. UTC by default."}},
				Doc: "Sets the server time and time zone the world starts at. Call it before the world has a map. Without it, the world starts at 2026-01-05 12:00 UTC."},
			{Name: "get_time", Returns: "string", Doc: "The server time in the world's time zone, as \"2026-10-31 20:00\"."},
			{Name: "get_monsters", Params: []Param{{"monster", "integer?", "From monsters.id. Every monster without it."}}, Returns: "wa.Monster[]", Doc: "The living monsters of the map, in the order they spawned."},
			{Name: "get_announcements", Returns: "string[]", Doc: "Every line announced to the world or the map so far, in order."},
			{Name: "count_floor_items", Params: []Param{itemParam}, Returns: "integer", Doc: "How many of the item lie on the map, with drops about to fall."},
			{Name: "chat", Params: []Param{playerParam, {"text", "string", "1 to 255 characters."}, {"options", "wa.ChatTestOptions?", ""}},
				Doc: "The player says a line, as a client's chat: it passes the mute, the chat filter and the chat hooks, then the chat event runs."},
			{Name: "get_chat", Params: []Param{playerParam}, Returns: "wa.HeardLine[]", Doc: "The chat lines the player got so far, oldest first: local chat, whispers, channel lines and system lines."},
			{Name: "trade", Params: []Param{{"a", "wa.Player", ""}, {"b", "wa.Player", "Steps next to a."}, {"offer_a", "wa.TradeOffer?", ""}, {"offer_b", "wa.TradeOffer?", ""}}, Returns: "boolean, string?",
				Doc: "Runs a whole trade, as two clients would: a asks, b accepts, both offer, lock and confirm. Returns true when it completed, or false and why: the trade hook's message, or an outcome such as \"restricted\" or \"bound\". A refused offer ends the trade. Trading needs Basic Skill level 1, which a new player from world:new_player lacks: give it skills = { [skills.id(\"NV_BASIC\")] = 9 }."},
			{Name: "close_storage", Params: []Param{playerParam}, Doc: "Closes the player's storage window, as the client's close button."},
			{Name: "set_staff_role", Params: []Param{playerParam, {"role", "string?", "A role of the default staff roles: admin, game_master or moderator. nil takes the role away."}},
				Doc: "Gives the player's account a staff role, as if it logged in with two-factor login, so player:get_staff_role and player:has_staff_permission see it.", Provisional: true},
			{Name: "buy", Params: []Param{playerParam, {"shop", "string", "A shop's full record name of data/npcs.yml, such as example.arena:Prize Desk."}, {"items", "table<integer, integer>", "Item IDs and the amount of each to buy."}}, Returns: "boolean, string?",
				Doc: "Opens a shop for the player, as player:open_shop, and buys, as the shop window's Buy. Returns true when the purchase went through, or false and the outcome, such as \"out_of_stock\", \"not_enough_items\" or \"not_enough_currency\"."},
			{Name: "plan_restart", Params: []Param{{"minutes", "integer", "1 to 10080, as the admin screens allow."}, {"reason", "string", "1 to 255 characters."}},
				Doc: "Plans a restart that many minutes from the test's time, as staff do on the admin screens: restart_planned runs with { at, reason } in the world's zone, and the lines its handlers send go out. Nothing restarts.", Provisional: true},
			{Name: "rotate", Params: []Param{{"dungeon", "string", "A dungeon of the test world, by its ID: its style's name without the mod prefix, such as crypt. Its style must have a rotation."}}, Returns: "string",
				Doc: "Rotates a dungeon at once, as staff force a rotation: its floors take the date of its style's next rotation after the world's time, or after the date it rotated to before, with new layouts and spawns. The points on the old floors move to the nearest town's save spot, and dungeon_rotated runs with { dungeon, date } in the world's zone. Returns the date, such as \"2026-10-23\". A case that runs on one of the dungeon's floors cannot rotate it, since a test world runs one map.", Provisional: true},
			{Name: "change_region", Params: []Param{{"x", "integer", "A field's x on the grid. The test world's field is 0, 0."}, {"y", "integer", "A field's y on the grid."}}, Returns: "string, integer",
				Doc: "Changes the region of a field at once, as staff force a world change: its fields generate again with their next season and a new biome, with new spawns. The points on them move to the region's town's save spot, and region_changed runs with { region, season } in the world's zone. Returns the region, its town's ID such as \"t1\", and its new season. A case that runs on one of the region's fields cannot change it.", Provisional: true},
			{Name: "get_mails", Params: []Param{playerParam}, Returns: "wa.TestMail[]", Doc: "The player's mails, oldest first, after the mail sent so far arrived."},
			{Name: "vend", Params: []Param{{"seller", "wa.Player", "Gets a cart and the Vending skill when it lacks them."}, {"buyer", "wa.Player", "Steps next to the seller."}, {"lines", "wa.TestVendLine[]", ""}}, Returns: "boolean, string?",
				Doc: "Runs a whole vending sale, as two clients would: the seller puts each line's items in its cart and opens a shop, and the buyer buys. The shop closes after it. Returns true when the sale went through, or false and why: the vend hook's message, or an outcome such as \"not_enough_zeny\"."},
		},
	},
	{
		Name: "wa.Expectation", Handle: "expectation", Testing: true,
		Doc: "A check on a value, from testing.expect. Its functions are called with a dot: testing.expect(n).to_be(3).",
		Functions: []Func{
			{Name: "to_be", Params: []Param{{"expected", "any", ""}}, Doc: "The value equals the expected one, with ==."},
			{Name: "to_equal", Params: []Param{{"expected", "table", ""}}, Doc: "The value is a table with the same contents, compared deeply."},
			{Name: "to_contain", Params: []Param{{"part", "any", ""}}, Doc: "A string value contains the text, or a table value has an element equal to the value."},
			{Name: "to_be_true", Doc: "The value is true."},
			{Name: "to_be_false", Doc: "The value is false."},
			{Name: "to_be_nil", Doc: "The value is nil."},
			{Name: "to_fail_with", Params: []Param{{"text", "string", ""}}, Doc: "The value is a function that raises an error containing the text."},
		},
	},
	{
		Name: "wa.ModInstance", Doc: "The instance of a mod context.",
		Fields: []Field{{"id", "string", "The mod's ID."}, {"map", "string", "The zone's map."}},
	},
	{
		Name: "wa.ItemUse", Doc: "The instance of a use context.",
		Fields: []Field{{"item", "integer", "The used item's ID."}},
	},
	{
		Name: "wa.EventInstance", Doc: "The instance of an event context: one run of a world event.",
		Fields: []Field{{"name", "string", "The event's full name, such as example.halloween:Pumpkin_King."}, {"map", "string", "The map of the zone that runs it."}},
	},
	{
		Name: "wa.Death", Doc: "The object of the death event.",
		Fields: []Field{
			{"victim", "wa.Player|wa.Monster", "Readable while the event runs, with is_alive false for a monster. victim.kind says which."},
			{"killer", "wa.Player|wa.Monster|nil", "nil when nothing killed the unit. killer.kind says which."},
			{"map", "string", ""},
			{"x", "integer", ""},
			{"y", "integer", ""},
			{"skill", "integer?", "nil for a normal attack."},
		},
	},
	{
		Name: "wa.JobChange", Doc: "The object of the job_change event.",
		Fields: []Field{
			{"player", "wa.Player", "The player that changed jobs."},
			{"from", "integer", "The job before the change."},
			{"to", "integer", "The new job. A male player who changes into a Dancer is a Bard here."},
		},
	},
	{
		Name: "wa.QuestEvent", Doc: "The object of the quest_add, quest_progress and quest_complete events.",
		Fields: []Field{
			{"player", "wa.Player", "The player whose quest log changed."},
			{"quest", "string", "The quest record's full name, such as example.hunts:wolves, or a core quest's ID as text, such as \"7104\"."},
			{"id", "integer", "The quest's ID."},
			{"objective", "integer?", "quest_progress: the hunt objective that counted a kill, from 1."},
			{"count", "integer?", "quest_progress: the objective's kills now."},
			{"goal", "integer?", "quest_progress: the kills the objective asks for."},
		},
	},
	{
		Name: "wa.DeathOptions", Doc: "The options of a death handler. Each one narrows the deaths that run it.",
		Fields: []Field{
			{"victim", "wa.Player|wa.Monster|nil", "One unit."},
			{"victims", "(wa.Player|wa.Monster)[]?", "Any of these units."},
			{"victim_kind", `"player"|"monster"|nil`, ""},
			{"monster", "integer?", "A monster from monsters.id."},
			{"map", "string?", "A map name."},
		},
	},
	{
		Name: "wa.Drop", Doc: "The object of the drop_rate hook: one drop of a killed monster.",
		Fields: []Field{
			{"player", "wa.Player?", "The killer, or nil."},
			{"monster", "wa.Monster", "The killed monster."},
			{"item", "integer", ""},
			{"map", "string", ""},
			{"rate", "integer", "rAthena's rate, per 10,000."},
		},
	},
	{
		Name: "wa.Exp", Doc: "The object of the experience hook: the exp one player gets for a kill, after the shares, the even party split and the level penalty. With even exp share, each member on the map gets its own hook run.",
		Fields: []Field{
			{"player", "wa.Player", ""},
			{"monster", "wa.Monster", "The killed monster."},
			{"base", "integer", ""},
			{"job", "integer", ""},
			{"party", "wa.Party?", "The player's party, or nil."},
			{"share", "wa.ExpShare", "The player's own share of the kill, before the even party split and the level penalty. 0 and 0 for a member who did not fight."},
			{"guild", "wa.Guild?", "The player's guild, or nil."},
			{"guild_tax", "integer", "The share of base that goes to the guild, by the exp tax of the player's position. The tax applies at the same rate to the base the hook returns."},
		},
	},
	{
		Name: "wa.HookOptions", Doc: "The options of hooks.on.",
		Fields: []Field{{"priority", "integer?", "Higher runs first. 0 by default."}},
	},
	{
		Name: "wa.HookHandle", Handle: "hook handle", Doc: "What hooks.on returns.",
		Methods: []Func{{Name: "remove", Doc: "Removes the hook. A second call does nothing."}},
	},
	{
		Name: "wa.Schedule", Handle: "schedule", Doc: "What wa.schedules returns.",
		Methods: []Func{{Name: "cancel", Doc: "Stops the schedule. A second call does nothing."}},
	},
	{
		Name: "wa.WhenOptions", Doc: "The options of schedules.when.",
		Fields: []Field{{"lasting_minutes", "integer?", "How long the condition must hold, 0 to 10080. 0 by default."}},
	},
	{
		Name: "wa.SpawnSpec", Doc: "The monsters of spawning.monsters.",
		Fields: []Field{
			{"monster", "integer", "From monsters.id."},
			{"map", "string?", "The context's own map, which is the default. Another map raises an error."},
			{"amount", "integer?", "1 to 1000. 1 by default."},
			{"x", "integer?", "With y, the cell. Without them, a random walkable cell for each monster."},
			{"y", "integer?", ""},
			{"area", "integer?", "Spreads the monsters within this many cells of x and y."},
			{"respawn", "boolean?", "true: a dead monster comes back while the context lives, as a spawn line's monster. false by default: dead stays dead."},
			{"respawn_milliseconds", "integer?", "With respawn, how long a dead monster takes to come back, 1000 to 268435455. 5000 by default, as a spawn line without a delay."},
		},
	},
	{
		Name: "wa.Damage", Doc: "The object of the damage event (0002a, \"Item events\").",
		Fields: []Field{
			{"source", "wa.Player|wa.Monster", "source.kind says which."},
			{"target", "wa.Player|wa.Monster", "target.kind says which."},
			{"kind", `"physical"|"magic"|"misc"`, ""},
			{"range", `"melee"|"ranged"|nil`, ""},
			{"skill", "integer?", "nil for a normal attack."},
			{"element", "integer", ""},
			{"is_critical", "boolean", ""},
			{"amount", "integer", ""},
			{"wearer_is_source", "boolean", ""},
		},
	},
	{
		Name: "wa.SkillUse", Doc: "The object of the skill_used event: a skill that took effect, after its cast time.",
		Fields: []Field{
			{"caster", "wa.Player|wa.Monster|wa.Npc", "The wearer, for an item's handler. caster.kind says which."},
			{"skill", "integer", ""},
			{"level", "integer", ""},
			{"target", "wa.Player|wa.Monster|wa.Npc|nil", "nil for a ground skill."},
			{"x", "integer", "The cell the skill took effect at."},
			{"y", "integer", ""},
		},
	},
	{
		Name: "wa.SkillUsedOptions", Doc: "The options of a skill_used handler. Each one narrows the skills that run it.",
		Fields: []Field{
			{"skill", "integer?", "A skill from skills.id."},
			{"caster_kind", `"player"|"monster"|"npc"|nil`, ""},
			{"map", "string?", "A map name."},
		},
	},
	{
		Name: "wa.Cast", Handle: "cast",
		Doc: "The object of the cast event: one cast of the skill, when it takes effect. It stays usable after the event, so timers of the skill's context can use it.",
		Fields: []Field{
			{"caster", "wa.Player|wa.Monster|wa.Npc", "caster.kind says which."},
			{"skill", "integer", ""},
			{"level", "integer", ""},
			{"target", "wa.Player|wa.Monster|wa.Npc|nil", "nil for a ground skill."},
			{"map", "string", ""},
			{"x", "integer", "The cell the skill takes effect at: the target's, or the ground cell."},
			{"y", "integer", ""},
		},
		Methods: []Func{
			{Name: "deal_damage", Params: []Param{{"target", "wa.Player|wa.Monster|wa.Npc", ""}, {"options", "wa.CastDamageOptions?", ""}}, Returns: "wa.CastDamageResult?",
				Doc: "Runs rAthena's damage calculation of the skill on the target, with percent as the skill's damage percentage, after the skill_damage hooks. It applies cards and defense, and fires the damage event after the script's call. Returns nil, and does nothing, when the caster is gone or cannot act, as rAthena's skill_addtimerskill."},
			{Name: "heal", Params: []Param{{"target", "wa.Player|wa.Monster|wa.Npc", ""}, {"amount", "integer", "0 or more."}}, Returns: "integer?",
				Doc: "Heals the target with rAthena's heal bonuses, and returns the HP healed. Returns nil, and does nothing, when the caster is gone or cannot act."},
			{Name: "run_default", Params: []Param{{"options", "wa.RunDefaultOptions?", ""}},
				Doc: "Runs core's own behavior of the skill. It raises an error for a skill that has none, such as a mod's skill."},
		},
	},
	{
		Name: "wa.CastDamageOptions", Doc: "The options of cast:deal_damage. Without one, the skill's data gives the value.",
		Fields: []Field{
			{"percent", "integer?", "The skill's damage percentage, 0 or more. 100 by default."},
			{"hits", "integer?", "The number of hits, 1 to 30."},
			{"type", `"weapon"|"magic"|"misc"|nil`, "The attack type."},
			{"element", "integer?", "An element from wa.elements, not all."},
		},
	},
	{
		Name: "wa.CastDamageResult", Doc: "What cast:deal_damage returns.",
		Fields: []Field{{"amount", "integer", "The damage dealt."}, {"is_critical", "boolean", ""}, {"missed", "boolean", ""}},
	},
	{
		Name: "wa.RunDefaultOptions", Doc: "The options of cast:run_default.",
		Fields: []Field{{"target", "wa.Player|wa.Monster|wa.Npc|nil", "Another target. The cast's target by default."}},
	},
	{
		Name: "wa.SkillInstance", Doc: "The instance of a skill's context: the skill, once per zone. Its cast handlers run for every cast.",
		Fields: []Field{{"skill", "integer", "The skill's ID."}},
	},
	{
		Name: "wa.PassiveSkillInstance", Doc: "The instance of a passive skill's context: one owner's skill, while it is learned. A new context starts when the level changes.",
		Fields: []Field{{"skill", "integer", "The skill's ID."}, {"level", "integer", "The learned level."}, {"owner", "wa.Player", "Who learned the skill."}},
	},
	{
		Name: "wa.SkillHook", Doc: "The object of the skill_damage, cast_time, skill_cost and cast_allowed hooks. The innermost result is rAthena's value.",
		Fields: []Field{
			{"caster", "wa.Player|wa.Monster|wa.Npc", ""},
			{"target", "wa.Player|wa.Monster|wa.Npc|nil", "nil for a ground skill, and for skill_cost."},
			{"skill", "integer", ""}, {"level", "integer", ""},
			{"percent", "integer?", "skill_damage: the damage percentage."},
			{"milliseconds", "integer?", "cast_time: the cast time, after DEX and the cast bonuses."},
			{"sp", "integer?", "skill_cost: the SP cost."},
			{"allowed", "boolean?", "cast_allowed: whether rAthena lets the cast start."},
		},
	},
	{
		Name: "wa.MonsterTargetChoice", Doc: "The object of the monster_target hook: a monster's AI chooses a target. The innermost result is current, rAthena's choice.",
		Fields: []Field{
			{"monster", "wa.Monster", "The monster that chooses."},
			{"candidates", "(wa.Player|wa.Monster|wa.Npc)[]", "The units it may take. The hook returns one of them, or nil."},
			{"current", "wa.Player|wa.Monster|wa.Npc|nil", "rAthena's choice: the target the monster takes without hooks."},
			{"reason", `"aggressive"|"assist"|"hit"|"change"`, "Why it chooses: it looks for a target, a monster of its kind calls for help, a unit hit it, or it changes chase target."},
		},
	},
	{
		Name: "wa.EntityQuery", Doc: "A query of maps.find_entities. It has one area: around with radius, x and y with radius, rectangle, line, or cells. The other fields filter and sort.",
		Fields: []Field{
			{"around", "wa.Player|wa.Monster|wa.Npc|nil", "The area is the cells within radius of the entity. The entity is in its own area: exclude leaves it out."},
			{"map", "string?", "The context's own map, which is the default. Another map raises an error."},
			{"x", "integer?", "With y, the area is the cells within radius of the cell."},
			{"y", "integer?", ""},
			{"radius", "integer?", "0 to 30 cells, with around or x and y."},
			{"shape", `"square"|"circle"|nil`, `With around or x and y. "square" by default, as rAthena's ranges.`},
			{"rectangle", "integer[][]?", "Two opposite corners, such as { { 100, 100 }, { 110, 105 } }. Sides of at most 61 cells."},
			{"line", "wa.QueryLine?", "The cells along a line."},
			{"cells", "integer[][]?", "A list of 1 to 1024 cells, such as { { 100, 100 }, { 101, 100 } }."},
			{"kinds", `("player"|"monster"|"npc")[]?`, "Keeps these kinds. Every kind by default."},
			{"alive", "boolean?", "true, the default, keeps living entities; false keeps dead ones."},
			{"enemies_of", "wa.Player|wa.Monster|wa.Npc|nil", "Keeps the entities this one may attack, by rAthena's rules."},
			{"allies_of", "wa.Player|wa.Monster|wa.Npc|nil", "Keeps the entities this one may help, by rAthena's rules, with itself."},
			{"line_of_sight", "boolean?", "Keeps the entities that the center of the area sees: the entity of around, the cell, the middle of the rectangle, the start of the line, or the first cell."},
			{"exclude", "(wa.Player|wa.Monster|wa.Npc)[]?", "Leaves these out."},
			{"sort", `"nearest"|"farthest"|"lowest_hp"|"random"|nil`, `Distances are from the center of the area. "random" draws from the zone's secret stream.`},
			{"limit", "integer?", "1 to 1000. 1000 by default."},
		},
	},
	{
		Name: "wa.QueryLine", Doc: "The line of a query.",
		Fields: []Field{
			{"from", "integer[]", "A cell, such as { 100, 100 }."},
			{"to", "integer[]", "A cell, at most 61 cells away on each axis."},
			{"width", "integer?", "1 to 15 cells. 1 by default."},
		},
	},
	{
		Name: "wa.HealSpec", Doc: "The amounts of player:item_heal and player:heal_percent. An amount is a number, or { minimum, maximum }.",
		Fields: []Field{{"hp", "integer|integer[]?", ""}, {"sp", "integer|integer[]?", ""}},
	},
	{
		Name: "wa.GiveItemOptions", Doc: "Options of player:give_item.",
		Fields: []Field{{"rent_seconds", "integer?", "Gives rentals, as rentitem. Rentals do not expire yet."},
			{"roll", `"random"|"default"|table<string, integer>|nil`, "How the copies' rolls are set: \"random\" rolls each value, \"default\" (the default) uses the defaults, and a table sets values, each clamped to its range. A table needs items.modify_rolls, except in a test case's own code."},
			{"to", "string?", `Where the copies go: "storage", "cart" or an extra storage's name, instead of the inventory. They follow the place's restrictions and bound rules.`},
			{"bound", "wa.Bound?", "Binds the copies, as getitembound. A bound copy cannot be traded or dropped, and goes only into storage its owner shares."}},
	},
	{
		Name: "wa.TestMail", Doc: "A mail as world:get_mails shows it.", Testing: true,
		Fields: []Field{{"from", "string", ""}, {"title", "string", ""}, {"zeny", "integer", ""}, {"items", "table<integer, integer>", "Item IDs and amounts."},
			{"read", "boolean", ""}, {"returned", "boolean", "Set for a mail that came back to its sender."}},
	},
	{
		Name: "wa.TestVendLine", Doc: "One line of world:vend.", Testing: true,
		Fields: []Field{{"item", "integer", "An item the seller holds."}, {"amount", "integer", "How many go on sale."}, {"price", "integer", "The price of one."}, {"buy", "integer?", "How many the buyer buys. None by default."}},
	},
	{
		Name: "wa.TradeOffer", Doc: "One side of world:trade.",
		Fields: []Field{{"items", "table<integer, integer>?", "Item IDs and the amount of each to offer."}, {"zeny", "integer?", ""}},
	},
	{
		Name: "wa.FromOptions", Doc: "Options of player:count_item and player:take_item.",
		Fields: []Field{{"from", "string", `"storage", "cart" or an extra storage's name of data/storages.yml.`}},
	},
	{
		Name: "wa.WorldEvent", Doc: "The object of world_event_start and world_event_end (review decision 31).",
		Fields: []Field{
			{"name", "string", "The event's full name, such as core:Field_Boss."},
			{"type", `"boss"|"invasion"|"treasure_hunt"|"script"`, `The event's type: "script" for an event without one.`},
			{"result", `"won"|"lost"|"stopped"|nil`, "How the run ended, at world_event_end. A boss is won when it dies, an invasion when its last wave dies, and a treasure hunt when every chest is opened. A run whose time ran out is lost. nil at world_event_start, and for a script event that ran its time."},
		},
	},
	{
		Name: "wa.RestartPlan", Doc: "The object of restart_planned (review decision 36): { at, reason } when staff plan a restart, and { cancelled = true } when they cancel it.",
		Fields: []Field{
			{"at", "integer?", "When the server restarts, in Unix seconds. nil when the restart was cancelled."},
			{"reason", "string?", "Why the server restarts, as staff wrote it. nil when the restart was cancelled."},
			{"cancelled", "boolean?", "true when staff cancelled the planned restart."},
		},
	},
	{
		Name: "wa.DungeonRotation", Doc: "The object of dungeon_rotated (review decision 47).",
		Fields: []Field{
			{"dungeon", "string", "The dungeon's ID, such as crypt."},
			{"date", "string", "The date its new floors carry in their map IDs, such as 2026-10-31: their map IDs are gen:dungeon:<dungeon>:<date>:<floor>."},
		},
	},
	{
		Name: "wa.RegionChange", Doc: "The object of region_changed (review decision 47).",
		Fields: []Field{
			{"region", "string", "The region, by its town's ID, such as t2: the fields whose nearest town is that town."},
			{"season", "integer", "The region's new season, from 1."},
		},
	},
	{
		Name: "wa.GenerationMap", Doc: "The map of a generation layer: a generated field, town or dungeon floor of a new procedural world (review decision 46). It is read-only.",
		Fields: []Field{
			{"id", "string", "The map ID, such as gen:field:3:1, gen:town:t2 or gen:dungeon:crypt:2."},
			{"kind", `"field"|"town"|"dungeon"`, ""},
			{"biome", "string", "A field's biome, such as forest, or a town's or a floor's tileset, such as crypt."},
			{"tileset", "string", "The tileset whose props objects name."},
			{"band", "integer[]", "The level band: the lowest and the highest level."},
			{"width", "integer", "The map's width in cells."},
			{"height", "integer", "The map's height in cells."},
			{"places", "wa.GenerationPlace[]", "The map's generated places, in the generator's order."},
		},
	},
	{
		Name: "wa.GenerationPlace", Doc: "A named place of a generation layer's map, or one a layer returns: its tags, the main tag first, and its cell. The server moves a returned place to the nearest open ground within 8 cells, or leaves it out.",
		Fields: []Field{{"tags", "string[]", "1 to 8 tags of 1 to 64 lowercase letters, digits, _, -, . and :."}, {"x", "integer", ""}, {"y", "integer", ""}},
	},
	{
		Name: "wa.GenerationObject", Doc: "An object a generation layer puts on its map. Clients draw it from the map's info. It does not block the cell.",
		Fields: []Field{{"model", "string", "A prop of the map's tileset, such as rock or well."}, {"x", "integer", "A cell of the map."}, {"y", "integer", ""}, {"turn", "integer?", "0 to 3 quarter turns. 0 by default."}},
	},
	{
		Name: "wa.GenerationBoss", Doc: "A boss a generation layer adds to its field's or dungeon floor's spawns, one monster. The server keeps it 30 cells from warps to towns.",
		Fields: []Field{
			{"monster", "integer", "A monster from monsters.id."},
			{"role", `"mini_boss"|"field_boss"`, ""},
			{"x", "integer?", "Its cell, with y: it appears within 3 cells of the nearest open ground. Left out, anywhere on the map."},
			{"y", "integer?", ""},
			{"respawn", "integer?", "Milliseconds before it comes back, 1,000 to 268,435,455. 45 minutes for a mini boss and 90 for a field boss by default."},
		},
	},
	{
		Name: "wa.GenerationResult", Doc: "What a generation layer adds to its map. Each list is optional.",
		Fields: []Field{
			{"places", "wa.GenerationPlace[]?", "At most 32."},
			{"objects", "wa.GenerationObject[]?", "At most 256."},
			{"bosses", "wa.GenerationBoss[]?", "At most 8."},
		},
	},
	{
		Name: "wa.SpawnMap", Doc: "The map of the spawn_table hook: a field or a dungeon floor of a new procedural world (review decision 44). It is read-only.",
		Fields: []Field{
			{"id", "string", "The map ID, such as gen:field:3:1 or gen:dungeon:crypt:2."},
			{"kind", `"field"|"dungeon"`, ""},
			{"biome", "string", "A field's biome, such as forest, or a floor's tileset, such as crypt."},
			{"band", "integer[]", "The level band: the lowest and the highest level."},
			{"features", "string[]", "The map's features, sorted: water and clearing on fields, room and boss_room on floors."},
		},
	},
	{
		Name: "wa.Spawn", Doc: "One spawn of the spawn_table hook's list. The server keeps aggressive monsters 15 cells from arrivals, and bosses 30 cells from warps to towns, whatever the hook returns.",
		Fields: []Field{
			{"monster", "integer", "A monster from monsters.id."},
			{"count", "integer", "1 to 1,000 monsters."},
			{"respawn", "integer", "Milliseconds before a dead one comes back, 1,000 to 268,435,455."},
			{"role", `"common"|"uncommon"|"plant"|"rare"|"mini_boss"|"field_boss"`, "A boss monster that is common or uncommon is left out."},
			{"area", "wa.SpawnArea?", "Where the monsters appear, or nil for the whole map."},
		},
	},
	{
		Name: "wa.SpawnArea", Doc: "A spawn's area: within w and h cells of x and y.",
		Fields: []Field{{"x", "integer", "0 to 1,024."}, {"y", "integer", ""}, {"w", "integer", ""}, {"h", "integer", ""}},
	},
	{
		Name: "wa.Trade", Doc: "The object of the trade event and the trade hook: a trade between two players. The hook runs before the trade completes, and the event after.",
		Fields: []Field{
			{"players", "wa.Player[]", "The two players: the one who confirmed last first."},
			{"items", "wa.TradedItem[]", "Each offered item."},
			{"zeny", "integer[]", "The zeny each player gave, in the order of players."},
		},
	},
	{
		Name: "wa.MailSpec", Doc: "A mail of player:send_mail (review decision 30).",
		Fields: []Field{
			{"to", "string|integer|wa.Player|nil", "The recipient: a character's name, a character ID, or a player handle. The player itself by default."},
			{"sender", "string?", "The name shown as the sender, at most 23 characters. By default the NPC's name in an NPC context, else the mod's name."},
			{"title", "string", "1 to 39 bytes."},
			{"body", "string?", "At most 499 bytes."},
			{"zeny", "integer?", "0 to 2147483647."},
			{"items", "wa.MailItem[]?", "At most mail_items attachments. The items are made for the mail."},
		},
	},
	{
		Name: "wa.MailItem", Doc: "One attachment of player:send_mail.",
		Fields: []Field{{"item", "integer", "An item from items.id."}, {"amount", "integer?", "1 to 30000. 1 by default."},
			{"roll", `"random"|"default"|table<string, integer>|nil`, "As give_item's roll. A table needs items.modify_rolls, except in a test case's own code."},
			{"bound", "wa.Bound?", "Binds the copies, as give_item's bound."}},
	},
	{
		Name: "wa.StoreSearchSpec", Doc: "The store search of player:search_stores.",
		Fields: []Field{{"uses", "integer", "How many searches the window allows, 1 to 1000."},
			{"effect", `"show"|"open"|nil`, `What a click on a result does: "show" (the default) shows the store's place, as SEARCHSTORE_EFFECT_NORMAL; "open" opens the store, as SEARCHSTORE_EFFECT_REMOTE, when it is on the player's map.`}},
	},
	{
		Name: "wa.BuyingStoreSpec", Doc: "The buying store of player:open_buying_store.",
		Fields: []Field{{"slots", "integer", "How many items the store may buy, 1 to 5, as MAX_BUYINGSTORE_SLOTS."}},
	},
	{
		Name: "wa.ShopItemOptions", Doc: "Options of shop:add_item.",
		Fields: []Field{{"cost", "wa.ShopCost?", "The cost of one. Without it, the item's buy price in zeny."},
			{"stock", "integer?", "How many the shop has for every player, 1 to 2147483647. No limit by default."},
			{"restock", "string?", `When the stock fills again: "daily", "weekly", or a time such as "6h", "30m" or "2d". It needs a stock.`}},
	},
	{
		Name: "wa.ShopCost", Doc: "A cost: zeny, items and currencies, each optional, at most 5 items and currencies together.",
		Fields: []Field{{"zeny", "integer?", "0 to 2147483647. Discount lowers it when the shop allows it."},
			{"items", "wa.ShopCostItem[]?", "Items the purchase takes, for one bought."},
			{"currency", "table<string, integer>?", "Currencies of data/currencies.yml and the amount of each, for one bought, such as { cash = 100 }."}},
	},
	{
		Name: "wa.ShopCostItem", Doc: "An item a cost takes.",
		Fields: []Field{{"item", "integer", "An item from items.id."}, {"amount", "integer?", "1 to 30000. 1 by default."}},
	},
	{
		Name: "wa.ShopSale", Doc: "The object of the shop_buy and shop_sell events: a purchase from a shop, or a sale to it.",
		Fields: []Field{{"player", "wa.Player", ""}, {"shop", "string", "The shop's full record name."},
			{"items", "wa.SoldItem[]", "Each line."}, {"zeny", "integer", "The zeny the player paid, or got for a sale."}},
	},
	{
		Name: "wa.StoreSale", Doc: "The object of the vend and buying_store_sale events and hooks. The hook runs before the sale, and the event after.",
		Fields: []Field{{"seller", "wa.Player", "The player who gives the items: the vendor, or the player who sells to a buying store."},
			{"buyer", "wa.Player", "The player who pays: the vendor's customer, or the buying store's owner."},
			{"items", "wa.SoldItem[]", "Each line, with its price."}, {"zeny", "integer", "The zeny the buyer pays, before the vending tax."}},
	},
	{
		Name: "wa.Mail", Doc: "The object of the mail event and hook. The hook runs before a player's mail goes out, and the event after any mail, a script's too.",
		Fields: []Field{{"from", "string", "The sender's name."}, {"to", "string", "The recipient's name."},
			{"items", "wa.SoldItem[]", "The attachments."}, {"zeny", "integer", "The attached zeny."}},
	},
	{
		Name: "wa.SoldItem", Doc: "One line of a sale or a mail.",
		Fields: []Field{{"item", "integer", "The item's ID."}, {"amount", "integer", ""}, {"price", "integer?", "The price of one, for vend and buying_store_sale."}},
	},
	{
		Name: "wa.TradedItem", Doc: "One item of a trade.",
		Fields: []Field{{"from", "wa.Player", "The player who gave it."}, {"item", "integer", "The item's ID."}, {"amount", "integer", ""}},
	},
	{
		Name: "wa.TradeOptions", Doc: "The options of a trade handler.",
		Fields: []Field{{"player", "wa.Player?", "Only trades of this player."}},
	},
	{
		Name: "wa.WithBonusesOptions", Doc: "Options of timers.with_bonuses.",
		Fields: []Field{{"restart", "wa.Timer?", "Cancels this timer first, if it still runs."}},
	},
	{
		Name: "wa.BonusTarget", Doc: "The against and from options of a bonus.",
		Fields: []Field{
			{"race", "integer?", "From wa.races."}, {"element", "integer?", "From wa.elements."}, {"size", "integer?", "From wa.sizes."},
			{"class", "integer?", "From wa.monster_classes."}, {"monster_group", "integer?", "From wa.monster_groups."},
			{"monster", "integer?", "From monsters.id."}, {"job", "integer?", "From wa.jobs."},
		},
	},
	{
		Name: "wa.BonusOptions", Doc: "The options table of bonuses.add (0002a). Each name takes the options 0002a lists for it.",
		Fields: []Field{
			{"percent", "boolean?", ""}, {"skill", "integer?", ""}, {"item", "integer?", ""}, {"item_group", "integer?", ""},
			{"kind", `"physical"|"magic"|"misc"|nil`, ""}, {"range", `"melee"|"ranged"|nil`, ""}, {"attack", `"normal"|"skill"|nil`, ""},
			{"element", "integer?", ""}, {"weapon_type", "integer?", ""}, {"against", "wa.BonusTarget?", ""}, {"from", "wa.BonusTarget?", ""},
			{"chance", "integer|(fun(): integer)|nil", "Per 10,000."}, {"on", `"attack"|"hit_taken"|"skill_used"|nil`, ""},
			{"target", `"self"|"enemy"|nil`, ""}, {"duration", "integer?", ""}, {"random_level", "boolean?", ""},
			{"stacking", `"sum"|"highest"|"lowest"|"last"|"priority"|nil`, ""}, {"priority", "integer?", ""},
			{"when", "(fun(): boolean)?", "A condition, tracked like a computed bonus."},
		},
	},
	{
		Name: "wa.StatusOptions", Doc: "The options of entity:start_status.",
		Fields: []Field{
			{"milliseconds", `integer|"forever"|nil`, "The duration, 1 to 2,147,483,647, or forever. Without it, the duration rAthena's sc_start looks up for the level, from the status's skill."},
			{"level", "integer?", "rAthena's val1. 1 by default."},
			{"values", "integer[]?", "val2 to val4, for the statuses that read them."},
			{"chance", "integer?", "Per 10,000, before resistance. 10000 by default."},
			{"source", "wa.Player|wa.Monster|wa.Npc|nil", "Who caused the status: its stats and level count for resistance, and events and get_status show it. Without it, the target's own stats count, as rAthena's sc_start."},
			{"cannot_be_avoided", "boolean?", "SCSTART_NOAVOID: no resistance, no chance roll, and no boss immunity."},
			{"keep_duration", "boolean?", "SCSTART_NOTICKDEF: resistance does not shorten the duration."},
			{"keep_chance", "boolean?", "SCSTART_NORATEDEF: resistance does not lower the chance."},
			{"hide_icon", "boolean?", "SCSTART_NOICON: the status shows no icon."},
		},
	},
	{
		Name: "wa.Status", Doc: "An active status, as entity:get_status returns it.",
		Fields: []Field{
			{"status", "integer", ""}, {"level", "integer", "val1."}, {"values", "integer[]", "val2 to val4."},
			{"remaining_milliseconds", `integer|"forever"`, ""},
			{"source", "wa.Player|wa.Monster|wa.Npc|nil", "Who caused it, or nil."},
		},
	},
	{
		Name: "wa.StatusEvent", Doc: "The object of status_start, status_end and status_tick.",
		Fields: []Field{
			{"target", "wa.Player|wa.Monster|wa.Npc", "target.kind says which."},
			{"status", "integer", ""}, {"level", "integer", ""},
			{"source", "wa.Player|wa.Monster|wa.Npc|nil", ""},
			{"milliseconds", `integer|"forever"`, "The duration at status_start, and the time left at status_end and status_tick."},
			{"reason", "string?", `Why it started: "skill", "item", "bonus", "script" or "login". Why it ended: "expired", "removed", "replaced", "death", "dispell", "clearance", "logout" or "damage". nil for status_tick.`},
		},
	},
	{
		Name: "wa.StatusEventOptions", Doc: "The options of a status event handler. Each one narrows the events that run it.",
		Fields: []Field{
			{"status", "integer?", "A status from wa.statuses."},
			{"target_kind", `"player"|"monster"|"npc"|nil`, ""},
			{"map", "string?", "A map name."},
		},
	},
	{
		Name: "wa.StatusHook", Doc: "The object of the status_chance and status_duration hooks: a status that starts. The innermost result is rAthena's value after resistance.",
		Fields: []Field{
			{"target", "wa.Player|wa.Monster|wa.Npc", ""}, {"status", "integer", ""}, {"level", "integer", ""},
			{"source", "wa.Player|wa.Monster|wa.Npc|nil", ""},
			{"chance", "integer?", "status_chance: the chance, per 10,000."},
			{"milliseconds", "integer?", "status_duration: the duration."},
		},
	},
	{
		Name: "wa.StatusDefinition", Doc: "The status of statuses.define.",
		Fields: []Field{
			{"name", "string", "1 to 48 lower case letters, digits and _, starting with a letter. The constant's name in wa.statuses."},
			{"icon", "integer?", "The status icon (EFST_ ID) players see. None by default."},
			{"kind", `"buff"|"debuff"|nil`, `"buff" by default.`},
			{"ends_on", "string[]?", "What ends the status besides its time: death, dispell, clearance, logout, banishing_buster, refresh, lux_anima, magic_gear_change and damage. Nothing by default."},
			{"bonuses", "(fun(level: integer): table)?", "Returns bonus entries for a level, as add_timed_bonuses takes them. They apply to players while the status lasts."},
			{"tick_milliseconds", "integer?", "From 100: status_tick fires at this interval, and once more when the status runs out, as rAthena's statuses that tick."},
		},
	},
	{
		Name: "wa.ChangeJobOptions", Doc: "Options of player:change_job.",
		Fields: []Field{
			{"force", "boolean?", "Skips the job change rules. It needs the permission player.change_job."},
		},
	},
	{
		Name: "wa.ResetSkillOptions", Doc: "Options of player:reset_skill.",
		Fields: []Field{
			{"to_level", "integer?", "The level to lower the skill to. 0 by default."},
			{"with_dependents", "boolean?", "Also lowers to 0, and refunds, the skills that need a higher level of it. Quest skills among them stay."},
		},
	},
	{
		Name: "wa.ResetStatOptions", Doc: "Options of player:reset_stat.",
		Fields: []Field{
			{"to", "integer?", "The value to lower the stat to. 1 by default."},
		},
	},
	{
		Name: "wa.TimedBonusOptions", Doc: "Options of player:add_timed_bonuses.",
		Fields: []Field{
			{"ends_on", "string[]?", "What ends the bonuses besides their time: death, dispell, clearance, logout, banishing_buster, refresh, lux_anima, magic_gear_change and damage."},
			{"on_repeat", `"keep"|"extend"|"add"|"replace"|nil`, `What happens when the player has timed bonuses with the same key. "keep", the default and rAthena's: the new ones are ignored. "extend", rAthena's BSF_FORCE_REPLACE: the first copy keeps its values and takes the new duration when it ends later. "add", rAthena's BSF_FORCE_DUPLICATE: both copies apply. "replace": every copy ends and the new one starts, with its values and duration.`},
			{"icon", "integer?", "A status icon (EFST_ ID) to show while they last."},
			{"kind", `"buff"|"debuff"|nil`, ""},
		},
	},
	{
		Name: "wa.NewPlayerSpec", Testing: true, Doc: "The character of world:new_player.",
		Fields: []Field{
			{"map", "string?", "The map, such as prontera. The world's map by default; a world runs one map."},
			{"x", "integer", ""}, {"y", "integer", ""},
			{"job", "integer?", "From wa.jobs. A novice by default."},
			{"base_level", "integer?", "1 by default."}, {"job_level", "integer?", "1 by default."},
			{"zeny", "integer?", "0 by default."},
			{"items", "integer[][]?", "A list of { item, amount }."},
			{"sex", `"male"|"female"|nil`, "Male by default."},
			{"skills", "integer[][]?", "Learned skills, a list of { skill, level }."},
			{"stats", "table?", "Raised stats, such as { str = 50, dex = 30 }. The others are 1."},
			{"skill_points", "integer?", "Skill points left to spend. 0 by default."},
			{"status_points", "integer?", "Status points left to spend. Those of the base level by default."},
		},
	},
	{
		Name: "wa.EquipOptions", Testing: true, Doc: "Options of world:equip.",
		Fields: []Field{
			{"roll", `"random"|"default"|table<string, integer>|nil`, "How the copy's rolls are set, as the roll option of player:give_item. A table needs no permission here."},
		},
	},
	{
		Name: "wa.SpawnMonsterSpec", Testing: true, Doc: "The monster of world:spawn_monster.",
		Fields: []Field{
			{"monster", "integer", "From monsters.id."},
			{"map", "string?", "The world's map by default."},
			{"x", "integer", ""}, {"y", "integer", ""},
		},
	},
}

// findNearby and existsMethod are methods of every entity handle
// (proposal 0002, review decisions 18 and 20).
var findNearby = Func{Name: "find_nearby", Params: []Param{{"query", "wa.EntityQuery", "A query without an area: it is around the entity, with radius."}}, Returns: "(wa.Player|wa.Monster|wa.Npc)[]",
	Doc: "maps.find_entities with around set to the entity.", Permission: "game.read"}

var existsMethod = Func{Name: "exists", Returns: "boolean", Doc: "False once the entity is removed, has logged out, or has left the zone. It never raises an error."}

// showEffect is show_effect of an entity handle other than a player's.
func showEffect(who string) Func {
	return Func{Name: "show_effect", Params: []Param{{"effect", "integer", "An effect from wa.effects."}}, Doc: "Plays an effect on the " + who + " for everyone near, as specialeffect."}
}

// statusMethods are the status methods of every entity handle (proposal
// 0002, review decision 19).
var statusMethods = []Func{
	{Name: "start_status", Params: []Param{{"status", "integer", "A status from wa.statuses."}, {"options", "wa.StatusOptions?", ""}}, Returns: "boolean",
		Doc: "Starts a status, as sc_start, sc_start2 and sc_start4, with rAthena's resistance and rules. Returns true when it starts. The target's stats change before it returns, as in rAthena. The status_start event runs after the script's call returns. On a player it needs player.status."},
	{Name: "end_status", Params: []Param{{"status", "integer", ""}}, Returns: "boolean", Doc: "Ends a status, as sc_end. Returns true when it was active. The target's stats change before it returns. The status_end event runs after the script's call returns. On a player it needs player.status."},
	{Name: "has_status", Params: []Param{{"status", "integer", ""}}, Returns: "boolean", Doc: "Whether a status is active, as getstatus.", Permission: "game.read"},
	{Name: "get_status", Params: []Param{{"status", "integer", ""}}, Returns: "wa.Status?", Doc: "An active status, or nil.", Permission: "game.read"},
	{Name: "get_statuses", Returns: "wa.Status[]", Doc: "The active statuses, by status.", Permission: "game.read"},
}

// Variable classes: one per type, for the scopes with a player and for
// server variables. They share two runtime handle types.
func init() {
	for _, kind := range []string{"integer", "boolean", "string", "table"} {
		typ := kind
		if kind == "table" {
			typ = "table?"
		}
		player := Class{Name: "wa." + title(kind) + "Variable", Handle: "variable", Doc: "A declared " + kind + " variable of a player."}
		server := Class{Name: "wa.Server" + title(kind) + "Variable", Handle: "server variable", Doc: "A declared " + kind + " server variable."}
		player.Methods = []Func{
			{Name: "get", Params: []Param{playerParam}, Returns: typ, Doc: "The value, or the default: 0, false, \"\", or nil for a table."},
			{Name: "set", Params: []Param{playerParam, {"value", typ, "nil or the default removes the stored value."}}, Doc: "Changes the value."},
		}
		server.Methods = []Func{
			{Name: "get", Returns: typ, Doc: "The value, or the default."},
			{Name: "set", Params: []Param{{"value", typ, ""}}, Doc: "Changes the value."},
		}
		if kind == "integer" {
			player.Methods = append(player.Methods, Func{Name: "add", Params: []Param{playerParam, {"amount", "integer", ""}}, Returns: "integer", Doc: "Adds to the value in one step, and returns the new value."})
			server.Methods = append(server.Methods, Func{Name: "add", Params: []Param{{"amount", "integer", ""}}, Returns: "integer", Doc: "Adds to the value in one step, and returns the new value."})
		}
		Classes = append(Classes, player, server)
	}
}

// Events lists every event name of wa.events.
var Events = []Event{
	{Name: "login", Kinds: []string{"mod", "event"}, Params: []Param{playerParam}, Doc: "A player logs in, in the zone it enters."},
	{Name: "logout", Kinds: []string{"mod", "event"}, Params: []Param{playerParam}, Doc: "A player logs out, before it leaves."},
	{Name: "map_enter", Kinds: []string{"mod", "event"}, Params: []Param{playerParam}, Doc: "A player enters the zone: at login and after a warp."},
	{Name: "death", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"death", "wa.Death", ""}}, Doc: "A player or a monster died on the context's map. Options narrow it."},
	{Name: "base_level_up", Kinds: []string{"mod", "event"}, Params: []Param{playerParam}, Doc: "The player gained a base level."},
	{Name: "job_level_up", Kinds: []string{"mod", "event"}, Params: []Param{playerParam}, Doc: "The player gained a job level."},
	{Name: "job_change", Kinds: []string{"mod", "event"}, Params: []Param{{"change", "wa.JobChange", ""}}, Doc: "The player changed jobs: player:change_job, player:rebirth, or a converted jobchange. It fires after the stats and skills of the new job are set."},
	{Name: "click", Kinds: []string{"npc"}, Params: []Param{playerParam, {"conversation", "wa.Conversation", ""}}, Doc: "A player talks to the NPC."},
	{Name: "touch", Kinds: []string{"npc"}, Params: []Param{playerParam, {"conversation", "wa.Conversation", ""}}, Doc: "A player stepped into the NPC's touch area, the record's touch field, as rAthena's OnTouch. It fires once each time the player comes in: on a step, or when it arrives on a cell of the area. Hidden and dead players fire it too. The handlers run in a conversation with the player, which shows nothing until a handler shows a step (review decision 56). A touch while the player talks to an NPC waits until that conversation ends, and runs only if the player is still in the area then."},
	{Name: "untouch", Kinds: []string{"npc"}, Params: []Param{playerParam}, Doc: "A player who fired touch left the NPC's touch area: a step out of it, a warp, or a logout. A player who left the map can only be compared: reading it raises an error."},
	{Name: "use", Kinds: []string{"item"}, Params: []Param{playerParam}, Doc: "The item is used."},
	{Name: "damage", Kinds: []string{"item"}, Params: []Param{{"hit", "wa.Damage", ""}}, Doc: "A hit the wearer deals or takes: each normal attack that lands. Skill hits come with skills (phase 4 step 3)."},
	{Name: "skill_used", Kinds: []string{"item", "mod", "event"}, Params: []Param{{"use", "wa.SkillUse", ""}}, Doc: "A skill took effect, after its cast time. An interrupted cast does not fire it. An item's handler hears the wearer's skills. A mod or event context hears every skill of its map: options narrow it."},
	{Name: "cast", Kinds: []string{"skill"}, Params: []Param{{"cast", "wa.Cast", ""}}, Doc: "The skill takes effect. A skill whose script has a cast handler runs it instead of core's behavior of the skill."},
	{Name: "unequip", Kinds: []string{"item"}, Params: []Param{playerParam}, Doc: "The copy is unequipped, before its context ends."},
	{Name: "status_start", Kinds: []string{"mod", "event"}, Params: []Param{{"event", "wa.StatusEvent", ""}}, Doc: "A status started on an entity of the context's map, or started again. Options narrow it."},
	{Name: "status_end", Kinds: []string{"mod", "event"}, Params: []Param{{"event", "wa.StatusEvent", ""}}, Doc: "A status ended. Options narrow it."},
	{Name: "status_tick", Kinds: []string{"mod", "event"}, Params: []Param{{"event", "wa.StatusEvent", ""}}, Doc: "A status that ticks did its tick, such as poison taking HP, or a mod status with tick_milliseconds. Options narrow it."},
	{Name: "chat", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"message", "wa.ChatMessage", ""}}, Doc: "A player on the context's map said a line: local chat, a whisper, a party line, or a channel line. It runs after the line went out. Options narrow it. Whispers, party and guild lines reach only a mod with rules.override, which the chat hook needs too."},
	{Name: "party_join", Kinds: []string{"mod", "event"}, Params: []Param{{"join", "wa.PartyMembership", ""}}, Doc: "A player of the context's map joined a party: an accepted invite, or a wa.parties call. It does not fire for a player who makes a party itself."},
	{Name: "party_leave", Kinds: []string{"mod", "event"}, Params: []Param{{"leave", "wa.PartyMembership", ""}}, Doc: "A player of the context's map left its party. A member who is offline gets no event."},
	{Name: "party_change", Kinds: []string{"mod", "event"}, Params: []Param{{"change", "wa.PartyChange", ""}}, Doc: "A party's leader or options changed, by a change made on the context's map."},
	{Name: "guild_join", Kinds: []string{"mod", "event"}, Params: []Param{{"join", "wa.GuildMembership", ""}}, Doc: "A player of the context's map joined a guild: an accepted invite, or a wa.guilds call, which also fires it for the master of a new guild."},
	{Name: "guild_leave", Kinds: []string{"mod", "event"}, Params: []Param{{"leave", "wa.GuildMembership", ""}}, Doc: "A player of the context's map left its guild. A member who is offline gets no event."},
	{Name: "guild_change", Kinds: []string{"mod", "event"}, Params: []Param{{"change", "wa.GuildChange", ""}}, Doc: "A guild got a new master, a level, a notice or an emblem, by a change made on the context's map. A new emblem from the account pages runs it on the map of the master, or of the first online member."},
	{Name: "trade", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"trade", "wa.Trade", ""}}, Doc: "Two players on the context's map completed a trade. Options narrow it."},
	{Name: "shop_buy", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"sale", "wa.ShopSale", ""}}, Doc: "A player on the context's map bought from a shop of data/npcs.yml."},
	{Name: "shop_sell", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"sale", "wa.ShopSale", ""}}, Doc: "A player on the context's map sold items to a shop."},
	{Name: "vend", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"sale", "wa.StoreSale", ""}}, Doc: "A player on the context's map bought from a vending shop."},
	{Name: "buying_store_sale", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"sale", "wa.StoreSale", ""}}, Doc: "A player on the context's map sold items to a buying store."},
	{Name: "mail", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"mail", "wa.Mail", ""}}, Doc: "A mail went out from the context's map: a player's, or a script's player:send_mail."},
	{Name: "world_event_start", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"event", "wa.WorldEvent", ""}}, Doc: "A run of a world event started, on any map. Every zone's handlers run."},
	{Name: "world_event_end", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"event", "wa.WorldEvent", ""}}, Doc: "A run of a world event ended, on any map, with its result. Every zone's handlers run."},
	{Name: "restart_planned", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"restart", "wa.RestartPlan", ""}}, Doc: "Staff planned a server restart from the admin screens, or cancelled it. Every zone's handlers run. At the restart time the server saves every character, disconnects every player and stops, so a handler can end a mod's event in time."},
	{Name: "dungeon_rotated", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"rotation", "wa.DungeonRotation", ""}}, Doc: "A rotating dungeon got new floors, on schedule or because staff forced it. Every zone's handlers run, after the players and points of the old floors moved to the nearest town."},
	{Name: "region_changed", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"change", "wa.RegionChange", ""}}, Doc: "A region's fields generated again with a new biome at the end of a season, or because staff forced it. Every zone's handlers run, after the players and points of its fields moved to the region's town."},
	{Name: "quest_add", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"event", "wa.QuestEvent", ""}}, Doc: "A quest joined a player's log on the context's map: player:add_quest, or the new quest of player:change_quest."},
	{Name: "quest_progress", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"event", "wa.QuestEvent", ""}}, Doc: "A kill raised a hunt objective's count, once per objective. Party members near the kill count it too, as rAthena."},
	{Name: "quest_complete", Kinds: []string{"mod", "npc", "event"}, Params: []Param{{"event", "wa.QuestEvent", ""}}, Doc: "player:complete_quest marked a quest complete."},
}

// questParam is a quest of data/quests.yml.
var questParam = Param{"quest", "integer|string", "A quest of data/quests.yml: its ID, such as 7104, or its name: a full name, such as example.hunts:wolves, or a plain name, which is the calling mod's record, then core's. An unknown quest raises an error."}

// currencyParam is the name of a currency.
var currencyParam = Param{"currency", "string", "A currency of data/currencies.yml: a full name, such as example.arena:tokens, or a plain name, which is the calling mod's record, then core's, such as cash or kafra."}

// EquipSlots are the slot names of player:get_equipped.
var EquipSlots = []string{
	"head_top", "head_middle", "head_low", "armor", "left_hand", "right_hand",
	"garment", "shoes", "left_accessory", "right_accessory", "ammo",
	"costume_head_top", "costume_head_middle", "costume_head_low", "costume_garment",
	"shadow_armor", "shadow_weapon", "shadow_shield", "shadow_shoes",
	"shadow_right_accessory", "shadow_left_accessory",
}

// Aliases lists the named types.
func Aliases() []Alias {
	var events []string
	for _, e := range Events {
		events = append(events, e.Name)
	}
	var bonuses []string
	for _, s := range bonus.Specs {
		bonuses = append(bonuses, s.Name)
	}
	return []Alias{
		{Name: "wa.ContextKind", Values: []string{"mod", "npc", "item", "skill", "monster", "event"}, Doc: "A context's kind."},
		{Name: "wa.EventName", Values: events, Doc: "An event name of wa.events."},
		{Name: "wa.BonusName", Values: bonuses, Doc: "A bonus name of wa.bonuses (proposal 0002a)."},
		{Name: "wa.EquipSlot", Values: EquipSlots, Doc: "An equipment slot."},
		{Name: "wa.HookName", Values: []string{"drop_rate", "experience", "status_chance", "status_duration", "skill_damage", "cast_time", "skill_cost", "cast_allowed", "monster_target", "item_roll", "chat", "trade", "vend", "buying_store_sale", "mail", "spawn_table"}, Doc: "A hook name of wa.hooks."},
		{Name: "wa.Bound", Values: []string{"account", "character", "party", "guild"}, Doc: "Who a bound copy is bound to."},
		{Name: "wa.ExpandableLimit", Values: rules.ExpandableLimits, Doc: "A limit that one player holds, which player:expand changes."},
		{Name: "wa.QuestIcon", Values: []string{"quest", "quest_question", "job", "job_question", "event", "event_question", "warg", "warg_face"}, Doc: "An icon of npc:show_quest_icon, as rAthena's QTYPE_ values 0 to 6 and 8: ! and ? marks for quests, jobs and events, a warg, and a warg's face."},
		{Name: "wa.MonsterState", Values: []string{"idle", "walk", "loot", "dead", "attack", "angry", "chase", "follow"}, Doc: "A monster's skill state, in the words of rAthena's mob_skill_db."},
		{Name: "wa.Weekday", Values: []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}, Doc: "A day of the week."},
	}
}

// ModuleByName returns a module by name.
func ModuleByName(name string) (Module, bool) {
	i := slices.IndexFunc(Modules, func(m Module) bool { return m.Name == name })
	if i < 0 {
		return Module{}, false
	}
	return Modules[i], true
}
