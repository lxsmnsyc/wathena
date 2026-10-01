// Package rathena embeds the reference files that waserver reads at
// startup, so a release binary needs no copy of this folder.
//
// Only waserver imports it. Tests and tools read the folder on disk
// through internal/reference.
package rathena

import "embed"

// Files holds the extracts and monster stats of both modes, at the same
// paths as in this folder.
//
//go:embed extracts/renewal/progression.json extracts/renewal/client_tables.json
//go:embed extracts/renewal/items.json extracts/renewal/spawns.json
//go:embed extracts/renewal/job_stats.json extracts/renewal/warps.json
//go:embed extracts/renewal/script_constants.json extracts/renewal/item_scripts.json extracts/renewal/item_groups.json
//go:embed extracts/renewal/npcs.json extracts/renewal/item_prices.json
//go:embed extracts/pre-renewal/progression.json extracts/pre-renewal/client_tables.json
//go:embed extracts/pre-renewal/items.json extracts/pre-renewal/spawns.json
//go:embed extracts/pre-renewal/job_stats.json extracts/pre-renewal/warps.json
//go:embed extracts/pre-renewal/script_constants.json extracts/pre-renewal/item_scripts.json extracts/pre-renewal/item_groups.json
//go:embed extracts/pre-renewal/npcs.json extracts/pre-renewal/item_prices.json
//go:embed renewal/mob_status.json pre-renewal/mob_status.json
var Files embed.FS
