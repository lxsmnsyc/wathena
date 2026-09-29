package build

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// GCReport describes what GC removed, or would remove.
type GCReport struct {
	Files int
	Bytes int64
	// CacheEntries is how many cache entries were dropped because their
	// file is gone.
	CacheEntries int
}

// GC removes stored files that the current manifest does not use, and
// temporary files that an interrupted build left behind. With dryRun it
// only reports them. It only touches files named as the build names them,
// in the build's two-letter folders.
func GC(dir string, dryRun bool) (*GCReport, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	m, all, err := loadEntries(root)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	// Paths from fs.WalkDir use slashes on every system.
	for _, h := range m.Shards {
		keep[filepath.ToSlash(objectPath(h, "json"))] = true
	}
	for _, e := range all {
		keep[filepath.ToSlash(StoredPath(e.H, extOf[e.T]))] = true
	}

	rep := &GCReport{}
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Only the top level and its two-letter folders hold build
			// files.
			if p != "." && (strings.Contains(p, "/") || !isHexName(p, 2)) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.Contains(p, "/") || keep[p] || !buildFileName(path.Base(p)) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rep.Files++
		rep.Bytes += info.Size()
		if dryRun {
			return nil
		}
		return root.Remove(p)
	})
	if err != nil {
		return nil, err
	}

	// Cache entries whose file is gone can never be reused. After a
	// collection, only files the manifest uses remain.
	cache := loadCache(root)
	kept := map[string]entry{}
	for k, e := range cache {
		p := filepath.ToSlash(StoredPath(e.H, extOf[e.T]))
		if _, err := root.Stat(p); err == nil && keep[p] {
			kept[k] = e
		} else {
			rep.CacheEntries++
		}
	}
	if !dryRun && rep.CacheEntries > 0 {
		if err := saveCache(root, kept); err != nil {
			return nil, err
		}
	}
	return rep, nil
}

// buildFileName reports whether a file name is one the build writes: a
// hash with a known extension, or a temporary file from writing one.
func buildFileName(name string) bool {
	if len(name) < 65 || !isHexName(name[:64], 64) || name[64] != '.' {
		return false
	}
	rest := name[65:]
	if i := strings.Index(rest, ".tmp"); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSuffix(rest, ".br")
	if rest == "json" {
		return true
	}
	_, ok := types[rest]
	return ok
}

func isHexName(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
