// Package core holds core's spawn data (proposal 0002, review decision
// 44): data/spawn_tags.yml, the spawn tags and densities that
// tools/spawntags derives from rAthena's spawn files, and
// data/spawn_themes.yml, core's spawn theme records, which the core mod
// carries. It holds only the files, so the converter can add them to
// core without the zones' packages.
package core

import "embed"

// Files are core's data/spawn_tags.yml and data/spawn_themes.yml, under
// data/.
//
//go:embed data
var Files embed.FS

// TagsFile is the spawn tags' file, and ThemesFile the spawn themes'.
const (
	TagsFile   = "data/spawn_tags.yml"
	ThemesFile = "data/spawn_themes.yml"
)
