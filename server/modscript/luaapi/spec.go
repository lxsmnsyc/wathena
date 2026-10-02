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
			{Name: "get_instance", Returns: "wa.ModInstance|wa.Npc|wa.EquippedItem|wa.ItemUse",
				Doc: "The instance the context owns: the mod, the NPC, the equipped copy, or the used item."},
			{Name: "get_kind", Returns: "wa.ContextKind", Doc: "The context's kind."},
			{Name: "get_mod", Returns: "string", Doc: "The ID of the context's mod."},
		},
	},
	{
		Name: "wa.events", Local: "events",
		Doc: "Event handlers. The context owns them, and removes them when it ends.",
		Funcs: []Func{{
			Name:       "on",
			Params:     []Param{{"name", "wa.EventName", "The event's name. An unknown name, or an event of another context kind, raises an error."}, {"handler", "function", "Runs each time the event fires, in registration order."}},
			Returns:    "wa.EventHandle",
			Doc:        "Registers a handler for an event.",
			Permission: "event.listen",
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
		Funcs: []Func{{Name: "id", Params: []Param{{"name", "string", "A skill_db name, such as SM_BASH."}}, Returns: "integer", Doc: "The skill's ID. An unknown name raises an error.", Permission: "game.read"}},
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
		Doc:   "Maps.",
		Funcs: []Func{{Name: "announce", Params: []Param{{"map", "string", "A map name, such as prontera."}, {"text", "string", "At most 255 bytes."}}, Doc: "Shows a line to every player of a map, as mapannounce."}},
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
	{Name: "wa.statuses", Local: "statuses", Doc: "Status effects, from rAthena's SC_ names.", Constants: true},
	{Name: "wa.effects", Local: "effects", Doc: "Effects for player:show_effect, from rAthena's EF_ names.", Constants: true},
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
		Methods: []Func{
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
			{Name: "show_headgear_effect", Params: []Param{{"effect", "integer", "An effect from wa.headgear_effects."}}, Doc: "rAthena's hateffect. It shows nothing yet."},
			{Name: "get_skill_level", Params: []Param{{"skill", "integer", "A skill from skills.id."}}, Returns: "integer", Doc: "The level of a skill the player has, or 0, as getskilllv.", Permission: "game.read"},
			{Name: "get_equipped", Params: []Param{{"slot", "wa.EquipSlot", ""}}, Returns: "wa.EquippedItem?", Doc: "The copy in a slot, or nil.", Permission: "game.read"},
			{Name: "start_status", Params: []Param{{"status", "integer", "A status from wa.statuses."}, {"options", "table?", ""}}, Doc: "Raises an error until status effects exist (phase 4)."},
			{Name: "add_timed_bonuses", Params: []Param{{"key", "string", ""}, {"milliseconds", "integer", ""}, {"bonuses", "table", ""}, {"options", "table?", ""}}, Doc: "Raises an error until timed bonuses exist (phase 4)."},
			{Name: "mark_minimap", Params: []Param{{"id", "integer", ""}, {"x", "integer", ""}, {"y", "integer", ""}, {"color", "integer", "0xRRGGBB."}}, Doc: "Marks a cell on the minimap, as viewpoint. A mark with the same ID replaces it."},
			{Name: "unmark_minimap", Params: []Param{{"id", "integer", ""}}, Doc: "Removes a minimap mark."},
			{Name: "open_shop", Params: []Param{{"shop", "string", "A shop record of data/npcs.yml: the mod's own name, or mod:Name."}}, Doc: "Opens a shop, as callshop."},
		},
	},
	{
		Name: "wa.Monster", Handle: "monster",
		Doc: "A monster. A killed monster's handle reads while its monster_killed event runs, with is_alive false, and raises an error after it. Reading needs game.read.",
		Fields: []Field{
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
	},
	{
		Name: "wa.Npc", Handle: "npc",
		Doc: "An NPC: the instance of an NPC context.",
		Fields: []Field{
			{"name", "string", "The name players see."},
			{"map", "string", ""},
			{"x", "integer", ""},
			{"y", "integer", ""},
		},
		Methods: []Func{
			{Name: "show_emotion", Params: []Param{{"emotion", "integer", "An emotion from wa.emotions."}}, Doc: "Shows an emotion over the NPC, as emotion."},
			{Name: "say_aloud", Params: []Param{{"text", "string", ""}}, Doc: "Says a line of local chat as the NPC, as npctalk. Players within 9 cells hear it."},
		},
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
			{Name: "kill", Params: []Param{playerParam, {"monster", "wa.Monster", "A monster from spawn_monster."}}, Doc: "The player kills the monster at once: monster_killed fires, and the player gets its exp and drops."},
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
		Name: "wa.Damage", Doc: "The object of the damage event (0002a, \"Item events\").",
		Fields: []Field{
			{"source", "wa.Player|wa.Monster", ""},
			{"target", "wa.Player|wa.Monster", ""},
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
		Name: "wa.SkillUse", Doc: "The object of the skill_used event.",
		Fields: []Field{
			{"skill", "integer", ""},
			{"level", "integer", ""},
			{"target", "wa.Player?", "nil for a ground skill."},
			{"x", "integer", ""},
			{"y", "integer", ""},
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
		Name: "wa.NewPlayerSpec", Testing: true, Doc: "The character of world:new_player.",
		Fields: []Field{
			{"map", "string?", "The map, such as prontera. The world's map by default; a world runs one map."},
			{"x", "integer", ""}, {"y", "integer", ""},
			{"job", "integer?", "From wa.jobs. A novice by default."},
			{"base_level", "integer?", "1 by default."}, {"job_level", "integer?", "1 by default."},
			{"zeny", "integer?", "0 by default."},
			{"items", "integer[][]?", "A list of { item, amount }."},
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
	{Name: "login", Kinds: []string{"mod"}, Params: []Param{playerParam}, Doc: "A player logs in, in the zone it enters."},
	{Name: "logout", Kinds: []string{"mod"}, Params: []Param{playerParam}, Doc: "A player logs out, before it leaves."},
	{Name: "map_enter", Kinds: []string{"mod"}, Params: []Param{playerParam}, Doc: "A player enters the zone: at login and after a warp."},
	{Name: "monster_killed", Kinds: []string{"mod"}, Params: []Param{playerParam, {"monster", "wa.Monster", ""}}, Doc: "The player killed a monster."},
	{Name: "base_level_up", Kinds: []string{"mod"}, Params: []Param{playerParam}, Doc: "The player gained a base level."},
	{Name: "job_level_up", Kinds: []string{"mod"}, Params: []Param{playerParam}, Doc: "The player gained a job level."},
	{Name: "click", Kinds: []string{"npc"}, Params: []Param{playerParam, {"conversation", "wa.Conversation", ""}}, Doc: "A player talks to the NPC."},
	{Name: "touch", Kinds: []string{"npc"}, Params: []Param{playerParam}, Doc: "A player walks into the NPC's area. It does not fire yet."},
	{Name: "use", Kinds: []string{"item"}, Params: []Param{playerParam}, Doc: "The item is used."},
	{Name: "damage", Kinds: []string{"item"}, Params: []Param{{"hit", "wa.Damage", ""}}, Doc: "A hit the wearer deals or takes. It fires in tests until equipment exists."},
	{Name: "skill_used", Kinds: []string{"item"}, Params: []Param{{"use", "wa.SkillUse", ""}}, Doc: "The wearer uses a skill. It fires in tests until equipment exists."},
	{Name: "unequip", Kinds: []string{"item"}, Params: []Param{playerParam}, Doc: "The copy is unequipped, before its context ends."},
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
