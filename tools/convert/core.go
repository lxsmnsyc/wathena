package convert

import (
	"embed"
	"io/fs"
)

// coreFiles are core's own files, written for wAthena instead of
// converted: the Job Master, a job change NPC (scripts/npcs/job_master.lua).
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
}
