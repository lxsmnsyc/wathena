package convert

import (
	"embed"
	"io/fs"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/currency"
	"github.com/lxsmnsyc/wathena/server/worldevent"
)

// coreFiles are core's own files, written for wAthena instead of
// converted: the Job Master, a job change NPC (scripts/npcs/job_master.lua).
// Core's companions and mounts come from server/companion, its chat
// channels and chat filter from server/chat, and its currencies from
// server/currency, and its sample world events from server/worldevent.
//
//go:embed core
var coreFiles embed.FS

// coreNPCs are the data/npcs.yml records of core's own NPCs, which follow
// the converted ones.
const coreNPCs = `# Core's own NPCs, written for wAthena.
- {name: "Job Master#prontera", map: prontera, x: 151, y: 191, direction: south, sprite: "1_M_JOBGUIDER", display_name: Job Master, script: scripts/npcs/job_master.lua}
`

// addCoreFiles adds core's own files to the converted ones, and its NPCs
// to data/npcs.yml.
func addCoreFiles(out map[string]string) {
	_ = fs.WalkDir(coreFiles, "core", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := coreFiles.ReadFile(path)
		if err != nil {
			return err
		}
		out[path[len("core/"):]] = string(b)
		return nil
	})
	out["data/npcs.yml"] += coreNPCs
	for _, k := range []companion.Kind{companion.Companions, companion.Mounts} {
		if b, err := companion.CoreFiles.ReadFile("core/" + k.File); err == nil {
			out[k.File] = string(b)
		}
	}
	if b, err := currency.CoreFiles.ReadFile("core/" + currency.File); err == nil {
		out[currency.File] = string(b)
	}
	if b, err := worldevent.CoreFiles.ReadFile("core/" + worldevent.File); err == nil {
		out[worldevent.File] = string(b)
	}
	for _, f := range []string{chat.ChannelFile, chat.FilterFile} {
		if b, err := chat.CoreFiles.ReadFile("core/" + f); err == nil {
			out[f] = string(b)
		}
	}
}
