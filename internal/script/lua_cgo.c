//go:build cgo && !luawasm && (amd64 || arm64)

// Lua and the binding, compiled by cgo. See csrc/all.c.
//
// The Go build cache only sees files in this directory, so the line below
// holds a digest of csrc/ and lua/src/. Changing a C source changes this
// file, and cgo compiles again. TestSourcesDigest checks it; run
// go test -run TestSourcesDigest -update after editing C sources.
//
// sources sha256: e7271426709740cff7ff86c982673523834a038c0b67591688d3be56f6d0d464
#include "csrc/all.c"
