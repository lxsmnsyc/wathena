package main

import (
	"log/slog"

	"github.com/lxsmnsyc/wathena/internal/buildinfo"
	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// loadCore converts the mode's item and NPC scripts, or reads them from
// the cache folder that cache_dir names (server/README.md, "Converted
// scripts cache"). A cache that cannot be read or written only logs a
// warning.
func loadCore(cfg options, mode reference.Mode, log *slog.Logger) (*convert.Core, convert.CacheStatus, error) {
	o := convert.CacheOptions{Dir: cfg.coreCacheDir()}
	if o.Dir != "" {
		id, err := buildinfo.ID()
		if err != nil {
			log.Warn("no build ID for the converted scripts cache; converting without it", "err", err)
			o.Dir = ""
		}
		o.Version = id
	}
	core, r, err := convert.LoadCoreCached(mode, o)
	if err != nil {
		return nil, r.Status, err
	}
	if r.ReadErr != nil {
		log.Warn("converted scripts cache damaged; converted again", "path", r.Path, "err", r.ReadErr)
	}
	if r.WriteErr != nil {
		log.Warn("converted scripts cache not written; the next start converts again", "path", r.Path, "err", r.WriteErr)
	}
	return core, r.Status, nil
}
