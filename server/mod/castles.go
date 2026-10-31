package mod

// CastleKind is data/castles.yml (proposal 0004, section 1): guild
// castles. Core's file is converted from rAthena's castle_db.yml and
// names each castle by its rAthena name, such as Kriemhild. A mod's
// records are named with its ID and get IDs from 10,000. id and map
// cannot change on another mod's record. teleport holds the economy
// fields cost and siege_cost, so changing another mod's teleport needs
// data.core_override. server/gamedata reads the fields.
var CastleKind = Kind{Name: "castles", File: "data/castles.yml", Fixed: []string{"id", "map"}, CoreNames: true, LocalNames: true, Economy: []string{"teleport"}}
