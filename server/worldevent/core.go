package worldevent

import "embed"

// CoreFiles are core's data/events.yml, in core/: one sample typed event
// of each type, off by default (review decision 31). The server owner
// turns them on with mods.events.<name>.enabled.
//
//go:embed core
var CoreFiles embed.FS

// File is the events file of a mod.
const File = "data/events.yml"
