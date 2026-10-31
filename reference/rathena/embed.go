//go:build !gzextracts

package rathena

import (
	"embed"
	"io/fs"
)

// Files holds the extracts of both modes, at the same paths as in this
// folder: the JSON files themselves in this build.
var Files fs.FS = files

// files lists the extracts. internal/gzextracts reads this list, so the
// gzipped copies are the same files.
//
//go:embed extracts/renewal/progression.json extracts/renewal/client_tables.json
//go:embed extracts/renewal/items.json extracts/renewal/spawns.json
//go:embed extracts/renewal/job_stats.json extracts/renewal/warps.json
//go:embed extracts/renewal/script_constants.json extracts/renewal/item_scripts.json extracts/renewal/item_groups.json
//go:embed extracts/renewal/npcs.json extracts/renewal/npcs-*.json extracts/renewal/item_prices.json
//go:embed extracts/renewal/equipment.json extracts/renewal/refine_db.json extracts/renewal/monster_groups.json
//go:embed extracts/renewal/status.json extracts/renewal/skills.json extracts/renewal/skill_trees.json
//go:embed extracts/renewal/monsters.json extracts/renewal/random_options.json extracts/renewal/item_trade.json extracts/renewal/shops.json extracts/renewal/quests.json
//go:embed extracts/renewal/castles.json
//go:embed extracts/pre-renewal/progression.json extracts/pre-renewal/client_tables.json
//go:embed extracts/pre-renewal/items.json extracts/pre-renewal/spawns.json
//go:embed extracts/pre-renewal/job_stats.json extracts/pre-renewal/warps.json
//go:embed extracts/pre-renewal/script_constants.json extracts/pre-renewal/item_scripts.json extracts/pre-renewal/item_groups.json
//go:embed extracts/pre-renewal/npcs.json extracts/pre-renewal/npcs-*.json extracts/pre-renewal/item_prices.json
//go:embed extracts/pre-renewal/equipment.json extracts/pre-renewal/refine_db.json extracts/pre-renewal/monster_groups.json
//go:embed extracts/pre-renewal/status.json extracts/pre-renewal/skills.json extracts/pre-renewal/skill_trees.json
//go:embed extracts/pre-renewal/monsters.json extracts/pre-renewal/random_options.json extracts/pre-renewal/item_trade.json extracts/pre-renewal/shops.json extracts/pre-renewal/quests.json
//go:embed extracts/pre-renewal/castles.json
var files embed.FS
