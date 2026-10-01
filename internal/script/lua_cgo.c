//go:build cgo && !luawasm && (amd64 || arm64)

// Lua and the binding, compiled by cgo. See csrc/all.c.
//
// The Go build cache only sees files in this directory, so the line below
// holds a digest of csrc/ and lua/src/. Changing a C source changes this
// file, and cgo compiles again. TestSourcesDigest checks it; run
// go test -run TestSourcesDigest -update after editing C sources.
//
// sources sha256: 259cbe870c9109b65253886ac2f5101e8077706a2aca1a70ad903033ba2569bb
#include "csrc/all.c"
