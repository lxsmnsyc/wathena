// Package core holds core's dungeon styles and town templates,
// data/dungeon_styles.yml and data/town_templates.yml (proposal 0002,
// review decision 42), which the core mod carries. It holds only the
// files, so the converter can add them to core without the zones'
// packages.
package core

import "embed"

// Files are core's data/dungeon_styles.yml and data/town_templates.yml,
// under data/.
//
//go:embed data
var Files embed.FS
