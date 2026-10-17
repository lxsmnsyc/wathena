package mod

// DungeonStyleKind is data/dungeon_styles.yml and TownTemplateKind
// data/town_templates.yml (proposal 0002, review decision 42). Core's
// own files add the core styles and templates with plain names, such as
// crypt and hamlet, which mods change by name. A mod's records are named
// with its ID, such as example.market-town:market_town. server/fields
// reads the fields.
//
// NamingStyleKind is data/naming_styles.yml (review decision 43): the
// naming styles of town names, one per biome. server/layout reads the
// fields.
//
// SpawnThemeKind is data/spawn_themes.yml (review decision 44): the
// spawn themes that raise monsters' weights on generated maps.
// server/spawns reads the fields.
//
// QuestTemplateKind is data/quest_templates.yml (review decision 46):
// the quest templates that a procedural world's quest boards and job
// guilds draw from when `wactl world new` makes the world. server/content
// reads the fields.
var (
	QuestTemplateKind = Kind{Name: "quest templates", File: "data/quest_templates.yml", CoreNames: true, LocalNames: true}
	SpawnThemeKind    = Kind{Name: "spawn themes", File: "data/spawn_themes.yml", CoreNames: true, LocalNames: true}
	NamingStyleKind   = Kind{Name: "naming styles", File: "data/naming_styles.yml", CoreNames: true, LocalNames: true}
	DungeonStyleKind  = Kind{Name: "dungeon styles", File: "data/dungeon_styles.yml", CoreNames: true, LocalNames: true}
	TownTemplateKind  = Kind{Name: "town templates", File: "data/town_templates.yml", CoreNames: true, LocalNames: true}
)
