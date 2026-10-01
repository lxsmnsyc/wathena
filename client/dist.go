//go:build !webclient

package client

import "io/fs"

// Dist returns the built web client, or nil when the binary was built
// without the webclient tag.
func Dist() fs.FS { return nil }
