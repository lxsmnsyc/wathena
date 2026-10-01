//go:build webclient

package client

import (
	"embed"
	"io/fs"
)

// The all: prefix keeps files whose names start with a dot or an
// underscore, which Vite may write.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the built web client, with index.html at its root.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
