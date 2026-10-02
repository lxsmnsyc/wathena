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
			{Name: "get_instance", Returns: "wa.ModInstance|wa.Npc|wa.EquippedItem|wa.ItemUse|wa.EventInstance|wa.SkillInstance|wa.PassiveSkillInstance",
				Doc: "The instance the context owns: the mod, the NPC, the equipped copy, the used item, the event run, the skill, or a passive skill of one owner."},
			{Name: "get_kind", Returns: "wa.ContextKind", Doc: "The context's kind."},
			{Name: "get_mod", Returns: "string", Doc: "The ID of the context's mod."},
		},
	},
	{
		Name: "wa.events", Local: "events",
		Doc: "Event handlers. The context owns them, and removes them when it ends.",
		Funcs: []Func{{
			Name: "on",
			Params: []Param{{"name", "wa.EventName", "The event's name. An unknown name, or an event of another context kind, raises an error."},
				{"handler", "function", "Runs each time the event fires, in registration order."},
				{"options", "wa.DeathOptions|wa.StatusEventOptions|wa.SkillUsedOptions|nil", "Only the death, status and skill_used events take options: they narrow which events run the handler."}},
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
				{"hook", "fun(object: wa.Drop|wa.Exp|wa.StatusHook|wa.SkillHook, next: fun(object?: table): integer|boolean, integer?): integer|boolean, integer?", "Gets the object and next. drop_rate returns a rate per 10,000; experience returns base and job exp; status_chance returns a chance per 10,000; status_duration and cast_time return milliseconds; skill_damage returns the damage percentage; skill_cost returns SP; cast_allowed returns true or false."},
				{"options", "wa.HookOptions?", ""}},
			Returns:    "wa.HookHandle",
			Doc:        "Adds a hook. Higher priority runs first; equal priorities run in the order they were added.",
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
			{Name: "announce", Params: []Param{{"map", "string", "A map name, such as prontera."}, {"text", "string", "At most 255 bytes."}}, Doc: "Shows a line to every player of a map, as mapannounce."},
			{Name: "find_entities", Params: []Param{{"query", "wa.EntityQuery", "One area, filters, and an order."}}, Returns: "(wa.Player|wa.Monster|wa.Npc)[]",
				Doc: "The entities of an area of the context's map that pass every filter, sorted, and cut to the limit. Ties sort by kind, then by entity. A query with no area, two areas, or a field it does not know raises an error.", Permission: "game.read"},
		},
	},
	{
		Name: "wa.world", Local: "world",
		Doc:   "The world.",
		Funcs: []Func{{Name: "announce", Params: []Param{{"text", "string", "At most 255 bytes."}}, Doc: "Shows a line to every player of the world, as announce.", Permission: "world.announce"}},
	},
	{
		Name: "wa.variables", Local: "variables",
		Doc:   "Saved variables, declared at the top of a file with a scope and a type. They are stored under the mod's ID.",
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
				Doc: "Adds a status in a mod script. It gets a constant in wa.statuses named after it, such as statuses.glow. Its ID stays the same in every zone, and a saved status keeps its name. A name that a status of the game or of another mod has raises an error."},
		},
	},
	{Name: "wa.effects", Local: "effects", Doc: "Effects for entity:show_effect, from rAthena's EF_ names.", Constants: true},
	{Name: "wa.headgear_effects", Local: "headgear_effects", Doc: "Effects for player:show_headgear_effect, from rAthena's HAT_EF_ names.", Constants: true},
	{Name: "wa.emotions", Local: "emotions", Doc: "Emotions for npc:show_emotion, from rAthena's ET_ names.", Constants: true},
	{
		Name: "wa.jobs", Local: "jobs", Doc: "Jobs, from rAthena's JOB_ names, and job tests.", Constants: true,
		Funcs: []Func{
			{Name: "is_transcendent", Params: []Param{{"job", "integer", "A job, such as player.job."}}, Returns: "boolean", Doc: "Whether the job is a transcendent job."},
			{Name: "is_baby", Params: []Param{{"job", "integer", ""}}, Returns: "boolean", Doc: "Whether the job is a baby job."},
			{Name: "is_third", Params: []Param{{"job", "integer", ""}}, Returns: "boolean", Doc: "Whether the job is a third job."},
			{Name: "is_fourth", Params: []Param{{"job", "integer", ""}}, Returns: "boolean", Doc: "Whether the job is a fourth job."},
			{Name: "base_job", Params: []Param{{"job", "integer", ""}}, Returns: "integer", Doc: "rAthena's BaseJob: a Lord Knight is a Knight."},
			{Name: "first_job", Params: []Param{{"job", "integer", ""}}, Returns: "integer", Doc: "rAthena's BaseClass: a Knight is a Swordman."},
			{Name: "next_jobs", Params: []Param{{"job", "integer", ""}}, Returns: "integer[]",
				Doc: "The jobs a job can change into, in job ID order: a Novice into a first class or a Super Novice, a High Novice into a transcendent first class, a first class into its two second classes. Jobs that differ by sex, such as Bard and Dancer, are both listed. Other jobs have none until third classes exist."},
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
		{"party", "nil", "nil: parties do not exist yet."},
		{"sex", `"male"|"female"`, ""},
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
			{Name: "count_item", Params: []Param{itemParam}, Returns: "integer", Doc: "How many of the item the player has, as countitem.", Permission: "game.read"},
			{Name: "take_item", Params: []Param{itemParam, {"amount", "integer", "1 or more."}}, Returns: "boolean", Doc: "Takes items, as delitem. Returns false, and takes nothing, when the player has too few."},
			{Name: "can_carry", Params: []Param{itemParam, {"amount", "integer", ""}}, Returns: "boolean", Doc: "Whether the items fit, by weight and slots, as checkweight.", Permission: "game.read"},
			{Name: "pay_zeny", Params: []Param{{"amount", "integer", ""}}, Returns: "boolean", Doc: "Takes zeny. Returns false, and takes nothing, when the player has too little."},
			{Name: "give_zeny", Params: []Param{{"amount", "integer", ""}}, Doc: "Gives zeny. Zeny over 2,147,483,647 is lost.", Permission: "player.give_zeny"},
			{Name: "item_heal", Params: []Param{{"heal", "wa.HealSpec", ""}}, Doc: "Heals as itemheal, with its bonuses from VIT, INT and skills."},
			{Name: "heal_percent", Params: []Param{{"heal", "wa.HealSpec", "Amounts in percent."}}, Doc: "Heals as percentheal."},
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
			{Name: "change_job", Params: []Param{{"job", "integer", "A job from wa.jobs."}, {"options", "wa.ChangeJobOptions?", ""}}, Returns: "boolean",
				Doc: "Changes the player's job, as jobchange with rAthena's pc_jobchange. Job level goes back to 1, and learned skills stay. A transcendent player stays in the line it had before its rebirth. Returns false, and changes nothing, when the job is not a valid next job (jobs.next_jobs). A male player who changes into a Dancer becomes a Bard, and the reverse."},
			{Name: "rebirth", Returns: "boolean",
				Doc: "Makes a second class at base level 99 and job level 50 or more a High Novice at base and job level 1, with 100 status points, First Aid and Trick Dead, as rAthena's rebirth. Its second class picks its transcendent jobs. Returns false, and changes nothing, for other players."},
			{Name: "reset_skills", Doc: "Lowers every skill learned with skill points to 0 and refunds the points, as resetskill. Quest skills stay, and so does Basic Skill outside the Novice jobs."},
			{Name: "reset_stats", Doc: "Sets every stat to 1 and gives back the status points of the base level, as resetstatus."},
			{Name: "reset_skill", Params: []Param{{"skill", "integer", "A skill from skills.id."}, {"options", "wa.ResetSkillOptions?", ""}}, Returns: "boolean",
				Doc: "Lowers one skill, to 0 by default, and refunds its skill points. Returns false, and changes nothing, when another learned skill needs the lowered level and with_dependents is not set, when the skill is not above the level, or for a quest skill."},
			{Name: "reset_stat", Params: []Param{{"stat", `"str"|"agi"|"vit"|"int"|"dex"|"luk"`, ""}, {"options", "wa.ResetStatOptions?", ""}}, Returns: "boolean",
				Doc: "Lowers one stat, to 1 by default, and refunds what raising it cost, by rAthena's cost table. Returns false when the stat is not above the value."},
		}, statusMethods...),
	},
	{
		Name: "wa.Monster", Handle: "monster",
		Doc: "A monster. A killed monster's handle reads while its death event runs, with is_alive false, and raises an error after it. Two handles to one monster are equal with ==. A monster that comes back after it died is a new monster with a new handle. Reading needs game.read.",
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
		},
		Methods: append([]Func{showEffect("monster"), findNearby, existsMethod}, statusMethods...),
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
			{Name: "advance", Params: []Param{{"milliseconds", "integer", ""}}, Doc: "Moves the zone clock forward. Timers fire and conversations that wait go on."},
			{Name: "spawn_monster", Params: []Param{{"spec", "wa.SpawnMonsterSpec", ""}}, Returns: "wa.Monster", Doc: "Places a monster on a cell. It does not come back after it dies."},
			{Name: "kill", Params: []Param{playerParam, {"monster", "wa.Monster", "A living monster: from spawn_monster, get_monsters or spawning.monsters."}}, Doc: "The player kills the monster at once: the death event fires, and the player gets its exp and drops."},
			{Name: "set_time", Params: []Param{{"time", "string", "Such as \"2026-10-31 19:58\"."}, {"time_zone", "string?", "An IANA name, such as Asia/Manila. UTC by default."}},
				Doc: "Sets the server time and time zone the world starts at. Call it before the world has a map. Without it, the world starts at 2026-01-05 12:00 UTC."},
			{Name: "get_time", Returns: "string", Doc: "The server time in the world's time zone, as \"2026-10-31 20:00\"."},
			{Name: "get_monsters", Params: []Param{{"monster", "integer?", "From monsters.id. Every monster without it."}}, Returns: "wa.Monster[]", Doc: "The living monsters of the map, in the order they spawned."},
			{Name: "get_announcements", Returns: "string[]", Doc: "Every line announced to the world or the map so far, in order."},
			{Name: "count_floor_items", Params: []Param{itemParam}, Returns: "integer", Doc: "How many of the item lie on the map, with drops about to fall."},
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
		Name: "wa.Exp", Doc: "The object of the experience hook: the exp one player gets for a kill, after the shares and the level penalty.",
		Fields: []Field{
			{"player", "wa.Player", ""},
			{"monster", "wa.Monster", "The killed monster."},
			{"base", "integer", ""},
			{"job", "integer", ""},
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
			{"respawn_milliseconds", "integer?", "With respawn, how long a dead monster takes to come back, 1000 to 268435455. 5000 by default, as a spawn line without a delay. Provisional."},
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
		Fields: []Field{{"rent_seconds", "integer?", "Gives rentals, as rentitem. Rentals do not expire yet."}},
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
		Doc: "Starts a status, as sc_start, sc_start2 and sc_start4, with rAthena's resistance and rules. Returns true when it starts. The target's stats change before it returns, as in rAthena. The status_start event runs after the script's call returns."},
	{Name: "end_status", Params: []Param{{"status", "integer", ""}}, Returns: "boolean", Doc: "Ends a status, as sc_end. Returns true when it was active. The target's stats change before it returns. The status_end event runs after the script's call returns."},
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
	{Name: "click", Kinds: []string{"npc"}, Params: []Param{playerParam, {"conversation", "wa.Conversation", ""}}, Doc: "A player talks to the NPC."},
	{Name: "touch", Kinds: []string{"npc"}, Params: []Param{playerParam}, Doc: "A player walks into the NPC's area. It does not fire yet."},
	{Name: "use", Kinds: []string{"item"}, Params: []Param{playerParam}, Doc: "The item is used."},
	{Name: "damage", Kinds: []string{"item"}, Params: []Param{{"hit", "wa.Damage", ""}}, Doc: "A hit the wearer deals or takes: each normal attack that lands. Skill hits come with skills (phase 4 step 3)."},
	{Name: "skill_used", Kinds: []string{"item", "mod", "event"}, Params: []Param{{"use", "wa.SkillUse", ""}}, Doc: "A skill took effect, after its cast time. An interrupted cast does not fire it. An item's handler hears the wearer's skills. A mod or event context hears every skill of its map: options narrow it."},
	{Name: "cast", Kinds: []string{"skill"}, Params: []Param{{"cast", "wa.Cast", ""}}, Doc: "The skill takes effect. A skill whose script has a cast handler runs it instead of core's behavior of the skill."},
	{Name: "unequip", Kinds: []string{"item"}, Params: []Param{playerParam}, Doc: "The copy is unequipped, before its context ends."},
	{Name: "status_start", Kinds: []string{"mod", "event"}, Params: []Param{{"event", "wa.StatusEvent", ""}}, Doc: "A status started on an entity of the context's map, or started again. Options narrow it."},
	{Name: "status_end", Kinds: []string{"mod", "event"}, Params: []Param{{"event", "wa.StatusEvent", ""}}, Doc: "A status ended. Options narrow it."},
	{Name: "status_tick", Kinds: []string{"mod", "event"}, Params: []Param{{"event", "wa.StatusEvent", ""}}, Doc: "A status that ticks did its tick, such as poison taking HP, or a mod status with tick_milliseconds. Options narrow it."},
}

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
		{Name: "wa.HookName", Values: []string{"drop_rate", "experience", "status_chance", "status_duration", "skill_damage", "cast_time", "skill_cost", "cast_allowed"}, Doc: "A hook name of wa.hooks."},
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
