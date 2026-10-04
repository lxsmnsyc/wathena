package mod

// CurrencyKind is data/currencies.yml (proposal 0002, review decision
// 30): currencies such as rAthena's cash points. Core's own file adds
// cash and kafra with plain names, which mods change by name. A mod's
// records are named with its ID, such as example.arena:tokens.
// server/gamedata reads the fields.
var CurrencyKind = Kind{Name: "currencies", File: "data/currencies.yml", Fixed: []string{"id"}, CoreNames: true, LocalNames: true}
