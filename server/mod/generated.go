package mod

// DungeonStyleKind is data/dungeon_styles.yml and TownTemplateKind
// data/town_templates.yml (proposal 0002, review decision 42). Core's
// own files add the core styles and templates with plain names, such as
// crypt and hamlet, which mods change by name. A mod's records are named
// with its ID, such as example.market-town:market_town. server/fields
// reads the fields.
var (
	DungeonStyleKind = Kind{Name: "dungeon styles", File: "data/dungeon_styles.yml", CoreNames: true, LocalNames: true}
	TownTemplateKind = Kind{Name: "town templates", File: "data/town_templates.yml", CoreNames: true, LocalNames: true}
)
