// Package core holds core's quest templates, data/quest_templates.yml
// (proposal 0002, review decision 46), which the core mod carries. It
// holds only the file, so the converter can add it to core without the
// zones' packages.
package core

import "embed"

// Files are core's data/quest_templates.yml, under data/.
//
//go:embed data
var Files embed.FS

// TemplatesFile is the quest templates' file.
const TemplatesFile = "data/quest_templates.yml"
