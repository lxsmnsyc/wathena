// Package core holds core's naming styles, data/naming_styles.yml
// (proposal 0002, review decision 43), which the core mod carries. It
// holds only the file, so the converter can add it to core without the
// zones' packages.
package core

import "embed"

// Files are core's data/naming_styles.yml, under data/.
//
//go:embed data
var Files embed.FS
