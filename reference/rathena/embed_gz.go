//go:build gzextracts

package rathena

import (
	"embed"
	"io/fs"
)

// Files holds the extracts of both modes, at the same paths as in this
// folder. This build embeds the gzipped copies that go generate wrote
// into the gz folder, and decompresses a file when it is read.
var Files fs.FS = mustSub(gzipped, "gz")

// gzipped is the gz folder. A build with the tag fails here when go
// generate has not made it.
//
//go:embed gz
var gzipped embed.FS

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return GzipFS(sub)
}
