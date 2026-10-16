package mod

// QuestKind is data/quests.yml (proposal 0002, review decision 45).
// Core's file is converted from rAthena's quest_db.yml, and names each
// quest by its ID, such as 7104, which mods change by that name. A mod's
// records are named with its ID, such as example.hunts:wolves, and get
// IDs from 1,000,000. server/gamedata reads the fields.
var QuestKind = Kind{Name: "quests", File: "data/quests.yml", CoreNames: true, LocalNames: true}
